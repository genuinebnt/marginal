# Marginal — Data Model

**Database:** PostgreSQL 18 · **Access:** sqlx with compile-time checked queries (ADR-003)

**One database per service** (ADR-003) — realised in the resident deployment as **one instance with a schema and role per service** (ADR-010 §3). No cross-schema joins — data needed elsewhere travels as events and is materialised locally, or is composed at the gateway. See §1.

---

## 1. The Central Rule

> **The op log is the source of truth. Block rows are a projection.**

```
   collab.ops  ──replay──▶  docs.blocks
   (append-only,            (materialised current state,
    never UPDATE,            rebuildable from ops,
    never DELETE)            exists for query performance)

   owned by                 owned by
   collaboration-service    document-service
```

A test must prove the projection can be rebuilt by replay. Snapshots exist for performance, never for correctness.

### Who owns which schema, and the one exception

**Database per service — one PostgreSQL instance each**, not schemas in a shared one
(**ADR-003**). Isolation is enforced by the network, so one service's database failing
degrades only that service.

> The schema names below (`auth`, `docs`, `collab`, `history`) remain as *namespaces within* each
> service's own database. They are no longer schemas sharing one instance by default.

**Four schemas exist, and most services do not have one.** "Database per service" makes it sound
as though every service gets storage; here only four of eleven are Postgres-backed, and the
interesting ones deliberately are not — Postgres is the wrong tool for a rope or an inverted index.

| Service | Postgres schema | Its other durable state |
|---|---|---|
| `auth-service` | **`auth`** | Redis — the `jti` revocation blocklist |
| `document-service` | **`docs`** | — |
| `history-service` | **`history`** | Object storage — Parquet snapshots (§5) |
| `collaboration-service` | **`collab`** — `ops` and its own outbox | **The local WAL on the filesystem**; Redis for presence, lease, and the instance registry; the rope in memory |
| `search-service` | none | A Tantivy index on a local volume |
| `diagnostics-service` | none | In-memory, materialised from NATS. *Whether the reverse index earns a schema is an open question — RFC-003 §Open questions* |
| `api-gateway` | none | Stateless. Redis for rate limits |

**What makes it real rather than cosmetic:**

- **No cross-schema joins.** `docs` never joins `auth`, `collab` or `history`. Data needed elsewhere travels as events and is materialised locally
- **No cross-schema foreign keys.** `docs.pages.created_by` references a user by plain `UUID` with no constraint, validated at the application layer (§4). Same reasoning as § *Why `actor_id` has no foreign key*
- Each schema migrates independently, on its own service's cadence

**Local development runs one Postgres container per service too.** It costs RAM and nothing else,
and a local topology that does not match the deployed one hides exactly the failure isolation this
design exists to provide (`CLOUD_PORTABILITY.md` §1).

### The isolation must be a grant, not a convention

> **ADR-010 — in the deployed estate, the grant is the *primary* mechanism, not defence in depth.**
> Eleven Postgres instances do not fit the cost ceiling, so the resident deployment is **one
> serverless Postgres with a schema and a login role per service**. Everything below is unchanged
> except which layer enforces it: there is no network boundary between schemas, so the grant *is*
> the boundary. Local development and Tier S sessions still run an instance per service, and a
> service that respects its grant is extractable to its own instance by changing a connection
> string.

With one instance per service the network enforces the boundary, so this would be **defence in
depth** rather than the primary mechanism. It still earns its place either way: it scopes what a
compromised service can reach *inside its own database*, and it makes the ownership exception below
visible as a grant rather than a paragraph.

**One Postgres role per service, granted only on its own schema:**

```sql
CREATE ROLE docs_svc LOGIN PASSWORD :'from_secret_manager';
GRANT USAGE ON SCHEMA docs TO docs_svc;
GRANT SELECT, INSERT, UPDATE ON ALL TABLES IN SCHEMA docs TO docs_svc;
-- and no grant whatsoever on auth or history
```

| What it buys | |
|---|---|
| **Enforcement** | A cross-schema join fails at the database instead of passing review |
| **Blast radius** | A compromised `document-service` cannot read `auth.users.password_hash` |
| **Ownership becomes visible** | `collab_svc` is granted on `collab` **and on nothing in `docs`** — the op log is its own, and `document-service` materialises `blocks` from published events rather than from a shared table. Drift shows up as a permission error |

**Migrate as the owner, run as the restricted role.** DDL rights and runtime rights are different
concerns; conflating them means a service can drop its own tables. Two roles per schema — an owner
used only by `sqlx migrate`, and a login role used by the running service.

**When:** this is pointless today with one service and one schema. It starts mattering the moment
**Phase 2** puts password hashes into the same instance, which is the natural place to do it.

### Where a "join" happens now that there is no shared database

With one database per service, `JOIN` across services is not a thing you decide against — it is not
expressible. Assembly moves to one of three places, and **the choice is per data shape, not global.**

| Pattern | Use it for | Example here |
|---|---|---|
| **Local materialisation from events** | Small, slowly changing reference data needed on a **hot path** | `display_name` and `cursor_color`. Every page load and every presence update needs them; each service keeps a tiny `users` projection fed by `auth.user_updated` |
| **Synchronous gRPC** | One-off lookups **off** the hot path | `AuthService.GetUser` for an admin screen |
| **API composition at the gateway** | The *client* needs one response spanning services | A page view that carries page metadata, backlinks, and comment counts |

> **The hot-path rule is the one that matters.** Resolving a display name over gRPC on every page
> load would put `auth-service`'s availability back on the read path — undoing precisely the
> isolation that made JWT-verified-locally-at-the-gateway worth doing. **If it is needed on every
> read, materialise it; do not call for it.**

This resolves a contradiction that existed between §1 above (*"travels as NATS events and is
materialised locally"*) and `lld/auth-service.md` §4, where `GetUser` was described as the way other
services resolve a display name. Both are correct — for different call sites — and the table above
is the rule for choosing.

**API composition is the gateway's job and nothing else's.** A service must not call two other
services to build a response; that is a distributed join with a service's name on it, and it turns
one slow dependency into a slow dependency for everybody (`ROADMAP.md` § Tail latency amplification).

### Ownership transfers for the life of a session

**No service writes another's tables.** `collaboration-service` owns `collab.ops` and its own
outbox; `document-service` owns `docs.pages`, `docs.blocks` and `docs.page_links`, materialising
`blocks` by replaying the op events the first publishes.

That leaves one question the schema alone cannot answer: **while a session is live, who is
authoritative for a page's content?**

**The rule: ownership of a page's block data transfers for the life of a session.** RFC-001 §2 already says it — *while a session is live the rope is authoritative; the
JSONB is a checkpoint, not the truth.* So `collaboration-service` is not a competing writer; it is
the **current owner**, flushing a checkpoint of state it holds authoritatively, and
`document-service` is the owner at every other time.

The alternatives were worse:

| Alternative | Why not |
|---|---|
| Flush through `document-service` over gRPC | Doubles write latency on the hot path and forces an API shaped like *"here is a rope projection"*, which is not a document operation |
| Keep both services writing `docs.blocks` | What was done first, and what ADR-003 reversed. Two writers on one table is exactly what database-per-service forbids, and the "ownership transfers for the life of a session" rule was the justification it needed. The rule survives — it still decides who is authoritative — but it no longer has to excuse a shared table |

**What was chosen instead:** `collaboration-service` gets `collab`, owns `ops` and its own outbox,
and publishes op events. `document-service` materialises `docs.blocks` by replaying them. The cost
is that `docs.blocks` lags the live rope during a session, which `ARCHITECTURE.md` § The couplings
records as the one remaining coupling and calls deliberate — read-your-writes reads from the
doc-actor.

**Two consequences that follow, and both are binding:**

1. **Each table has exactly one writer**, so a migration is one service's problem. `collab.ops` is `collaboration-service`'s; `docs.blocks` is `document-service`'s. What still crosses the line is the **event contract** between them (§10), and that is what must be rolled out consumers-before-producers (`ROADMAP.md` Phase 11)
2. **The transfer must be explicit, not implied.** It is the lease in `collaboration-service`'s `ownership/` module — a page has one owner at a time, enforced by a fencing token, and that lease is what decides who is authoritative for a page's content while a session is live

---

## 2. Entity Relationships

> **This diagram spans three databases.** Relationships crossing a service boundary —
> `users → pages`, `users → ops`, `pages → ops`, `ops → snapshots` — are **application-level
> references, not foreign keys**, and cannot be enforced or joined (ADR-003). Only
> edges within one service's database are real constraints.

```mermaid
erDiagram
    users ||--o{ pages : creates
    users |o--o{ ops : "authors (only when actor_kind = user)"
    pages ||--o{ blocks : contains
    pages ||--o{ ops : "scoped to"
    pages ||--o{ page_links : "links from"
    pages ||--o{ snapshots : "has"
    blocks ||--o{ blocks : "parent of"
    blocks ||--o{ page_links : "contains link"
    files ||--o{ blocks : "referenced by"

    users {
        uuid id PK
        text email UK
        text password_hash
        text display_name
        text cursor_color
        timestamptz created_at
    }
    pages {
        uuid id PK
        uuid created_by FK
        text title
        ltree path
        uuid parent_id FK
        text sort_key
        timestamptz deleted_at
        text lifecycle_state
    }
    blocks {
        uuid id PK
        uuid page_id FK
        uuid parent_id FK
        ltree path
        text sort_key
        text kind
        jsonb content
        smallint content_version
        timestamptz deleted_at
    }
    ops {
        uuid id PK
        uuid page_id FK
        uuid actor_id "no FK — see § Why actor_id has no foreign key"
        text actor_kind "user | agent | plugin | system"
        uuid undo_group "one gesture, one group; NULL is a group of one"
        smallint encoding_version
        text kind
        jsonb payload
        jsonb vector_clock
        timestamptz created_at
    }
    page_links {
        uuid id PK
        uuid from_block FK
        uuid from_page FK
        text target_title
        uuid target_page FK
    }
    snapshots {
        uuid id PK
        uuid page_id FK
        uuid up_to_op FK
        text object_key
        timestamptz created_at
    }
    files {
        uuid id PK
        uuid uploaded_by FK
        text object_key
        text mime_type
        bigint size_bytes
    }
```

---

## 3. `auth` Schema

```sql
CREATE SCHEMA auth;

CREATE TABLE auth.users (
    id             UUID PRIMARY KEY DEFAULT uuidv7(),
    email          TEXT NOT NULL UNIQUE,
    -- Argon2id PHC string. Never a raw hash, never a separate salt column:
    -- the PHC format carries algorithm, parameters, and salt together, so
    -- parameters can be upgraded without a schema migration.
    password_hash  TEXT NOT NULL,
    display_name   TEXT NOT NULL,
    -- Assigned at signup so collaborators are visually distinguishable.
    cursor_color   TEXT NOT NULL,
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE auth.refresh_tokens (
    id          UUID PRIMARY KEY DEFAULT uuidv7(),
    user_id     UUID NOT NULL REFERENCES auth.users(id) ON DELETE CASCADE,
    -- SHA-256 of the token, never the token itself: a database leak must not
    -- yield usable credentials.
    token_hash  BYTEA NOT NULL UNIQUE,
    -- Rotation chain. On refresh the old row is revoked and a new one issued
    -- with parent_id set. Reuse of a revoked token means theft — revoke the
    -- entire chain.
    parent_id   UUID REFERENCES auth.refresh_tokens(id),
    expires_at  TIMESTAMPTZ NOT NULL,
    revoked_at  TIMESTAMPTZ,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX ON auth.refresh_tokens (user_id) WHERE revoked_at IS NULL;
```

Access tokens are **not** stored — they are RS256-signed JWTs verified locally by the gateway (ADR-007). Revocation is a Redis blocklist keyed by `jti` with a TTL matching token lifetime.

---

## 4. `docs` Schema

### Pages

```sql
CREATE SCHEMA docs;
CREATE EXTENSION IF NOT EXISTS ltree;

CREATE TABLE docs.pages (
    id           UUID PRIMARY KEY DEFAULT uuidv7(),
    created_by   UUID NOT NULL,           -- auth.users(id), no FK: cross-schema
    title        TEXT NOT NULL,
    parent_id    UUID REFERENCES docs.pages(id),
    -- Materialised ancestry, e.g. 'root.a1b2.c3d4'. Subtree queries via <@
    -- without a recursive CTE on the hot path.
    path         LTREE NOT NULL,
    -- Fractional index: a page is reordered by writing ONE row, never by
    -- renumbering siblings. Lexicographic ordering, midpoint generation.
    sort_key     TEXT NOT NULL,
    -- Saga state (ARCHITECTURE §5). A crash mid-delete resumes, not restarts.
    lifecycle_state TEXT NOT NULL DEFAULT 'active'
        CHECK (lifecycle_state IN ('active','deleting','deleted')),
    deleted_at   TIMESTAMPTZ,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX ON docs.pages USING GIST (path);
CREATE INDEX ON docs.pages (parent_id, sort_key) WHERE deleted_at IS NULL;
-- Title uniqueness is NOT enforced: duplicates are a diagnostic
-- (RFC-003 DuplicateTitle), not a constraint violation. Enforcing it here
-- would make a legitimate in-progress edit fail at the database.
CREATE INDEX ON docs.pages (lower(title)) WHERE deleted_at IS NULL;
```

### Blocks

```sql
CREATE TABLE docs.blocks (
    id              UUID PRIMARY KEY DEFAULT uuidv7(),
    page_id         UUID NOT NULL REFERENCES docs.pages(id) ON DELETE CASCADE,
    parent_id       UUID REFERENCES docs.blocks(id),
    path            LTREE NOT NULL,
    sort_key        TEXT NOT NULL,
    kind            TEXT NOT NULL,
    content         JSONB NOT NULL DEFAULT '{}',
    -- Content shapes evolve. Old rows keep old shapes forever, so the shape
    -- version is stored per row (ADR-001 seam #2). Additive-only evolution.
    content_version SMALLINT NOT NULL DEFAULT 1,
    deleted_at      TIMESTAMPTZ,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX ON docs.blocks USING GIST (path);
CREATE INDEX ON docs.blocks (page_id, sort_key) WHERE deleted_at IS NULL;
CREATE INDEX ON docs.blocks USING GIN (content jsonb_path_ops);
```

### `content` shape per `kind`

```json
// paragraph | heading_1..3 | quote | toggle | bulleted_list | numbered_list | todo_list
{ "spans": [{ "text": "Hello", "bold": true, "link": "https://…" }] }

// todo_list adds
{ "checked": false, "spans": [...] }

// code — NO spans; code is never formatted (RFC-001 §1 grammar)
{ "code": "fn main() {}", "language": "rust" }

// image
{ "file_id": "uuid", "caption": "optional", "width_ratio": 0.6 }

// divider
{}
```

Mark keys are serialised in a fixed order and absent means false — never `"bold": false` (RFC-001 §2).

### The op log — `document-service` in Phase 1, `collaboration-service` from Phase 3

> **Ownership moves once** (ADR-003). Phase 1 ships a single-user editor that saves, so
> `document-service` writes **block-granular ops** (RFC-002 §2.1) and applies them to `blocks`
> itself. At Phase 3 the table moves to `collaboration-service`, character-granular ops arrive with
> the rope, and `document-service` switches to materialising `blocks` from op events.
>
> Either way §1's central rule holds: `ops` is the source of truth, `blocks` is the projection.

```sql
-- APPEND ONLY. No UPDATE. No DELETE. This is the source of truth.
CREATE TABLE collab.ops (
    id               UUID PRIMARY KEY,      -- UUIDv7 from the client; the dedup key
    -- NO foreign key: `pages` is document-service's, this table is
    -- collaboration-service's. Cross-schema references are plain UUID,
    -- validated at the application layer (ADR-003).
    page_id          UUID NOT NULL,

    -- NO foreign key, deliberately. Once the assistant and plugins are actors
    -- (ADR-009), not every actor has a row in auth.users. See § Why actor_id
    -- has no foreign key — do not "fix" this in review.
    actor_id         UUID NOT NULL,
    actor_kind       TEXT NOT NULL DEFAULT 'user'
                     CHECK (actor_kind IN ('user','agent','plugin','system')),

    -- One user gesture, one group. NULL means a group of one.
    undo_group       UUID,

    -- Encoding version, present from op #1. History replay must decode ops
    -- written by every prior release, forever (RFC-002 §4).
    encoding_version SMALLINT NOT NULL,
    kind             TEXT NOT NULL,         -- 'insert_text', 'delete_block', …
    payload          JSONB NOT NULL,        -- carries inverse data: deleted text, subtree
    vector_clock     JSONB NOT NULL,
    created_at       TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX ON collab.ops (page_id, created_at);
CREATE INDEX ON collab.ops (page_id, actor_id, created_at);  -- per-user undo
```

`payload` carries inversion data — deleted text, the removed subtree, `from` values. Deletes are the large ops precisely because they must be invertible (RFC-002 §3).

### Two columns that must exist from op #1

The op log is **append-only and never rewritten**. Anything a later phase needs from an op
record either exists now or is added with a default that is *true of every historical row*.
Both of these clear that bar, which is the only reason they are cheap:

| Column | Needed by | Why not later |
|---|---|---|
| `actor_kind` | `can_apply` authorizing a plugin differently from a person (18); history rendering three actor colours, which `ui-mockups/history.html` already does | Backfilling `'user'` is *correct* — every op written before the assistant existed was a user op. Add it after and the default is a guess instead of a fact |
| `undo_group` | Per-actor undo (5). One paste is many ops; one `## ` keystroke is `SetBlockKind` + `DeleteText`; accepting one assistant proposal is N ops | NULL degrades to a group of one, so this is genuinely addable later — it is here because it costs nothing now and saves a migration on a table you cannot rewrite |

> **Undo pops the newest *group* belonging to this actor, not the newest op.** Without that
> rule ⌘Z undoes one twentieth of a paste and the user sees garbage. The group is assigned by
> whoever originates the gesture — the client for paste and input rules, the assistant for a
> proposal batch — never by the server, which cannot know where a gesture began.

`actor_kind` is `TEXT` with a `CHECK`, not a Postgres `ENUM`: extending a check constraint is
ordinary DDL, while extending an enum is a special case with its own transaction rules. The
neighbouring `kind` column already sets that precedent.

### Why `actor_id` has no foreign key

`docs.users` lives in the auth schema and holds people. `agent`, `plugin`, and `system` actors
have no row there and never will. A foreign key would either block those ops or force fake user
rows for a language model and a WASM module — so the constraint is **absent on purpose**, and
`actor_kind` is what makes the reference interpretable.

### Outbox — one per publishing service

> **Two of these exist**, one in each publishing service's own database — `document-service` for
> page lifecycle events, `collaboration-service` for op events. That is what database-per-service
> requires, and it removes the coupling where one service being down silenced the bus for everyone
> (`ARCHITECTURE.md` §1).

```sql
-- Postgres write + NATS publish is a dual write: two systems, no distributed
-- transaction. If COMMIT succeeds and publish fails, the event is lost
-- permanently — search never indexes it, history has a hole, nothing notices.
-- The event row is written in the SAME transaction; a poller publishes it.
CREATE TABLE <schema>.outbox (   -- one per publishing service: docs, collab
    id            UUID PRIMARY KEY DEFAULT uuidv7(),
    aggregate_id  UUID NOT NULL,
    event_type    TEXT NOT NULL,
    payload       JSONB NOT NULL DEFAULT '{}',
    published_at  TIMESTAMPTZ,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
-- Partial index: the poller only scans unpublished rows, so it stays small
-- however large the table grows. Claim with FOR UPDATE SKIP LOCKED.
CREATE INDEX ON <schema>.outbox (created_at) WHERE published_at IS NULL;
```

At-least-once, not exactly-once. A crash between publish and stamp republishes — correct and unavoidable, so **every consumer must be idempotent**.

### Page links

```sql
-- Forward and reverse index for [[Page Link]] resolution. Serves BOTH
-- diagnostic invalidation and the backlinks panel (RFC-003 §3).
CREATE TABLE docs.page_links (
    id            UUID PRIMARY KEY DEFAULT uuidv7(),
    from_page     UUID NOT NULL REFERENCES docs.pages(id) ON DELETE CASCADE,
    from_block    UUID NOT NULL REFERENCES docs.blocks(id) ON DELETE CASCADE,
    -- The literal text written. Retained even when resolution succeeds, so a
    -- rename can re-resolve, and a dangling link survives as a diagnostic.
    target_title  TEXT NOT NULL,
    target_page   UUID REFERENCES docs.pages(id),   -- NULL = dangling
    UNIQUE (from_block, target_title)
);
CREATE INDEX ON docs.page_links (lower(target_title));   -- reverse lookup on rename
CREATE INDEX ON docs.page_links (target_page) WHERE target_page IS NOT NULL;
```

### Files

```sql
CREATE TABLE docs.files (
    id          UUID PRIMARY KEY DEFAULT uuidv7(),
    uploaded_by UUID NOT NULL,
    object_key  TEXT NOT NULL UNIQUE,
    mime_type   TEXT NOT NULL,
    size_bytes  BIGINT NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

Bytes go browser → S3/MinIO directly via presigned PUT, bypassing every service. Only metadata is stored here.

---

## 5. `history` Schema

```sql
CREATE SCHEMA history;

-- CQRS read model, projected from collab.ops. Snapshot bodies live in object
-- storage; only pointers are relational.
CREATE TABLE history.snapshots (
    id          UUID PRIMARY KEY DEFAULT uuidv7(),
    page_id     UUID NOT NULL,
    -- Replay from this op forward to reconstruct any later state.
    -- Cross-database: `ops` lives in collaboration-service. Plain UUID, no FK.
    up_to_op    UUID NOT NULL,
    object_key  TEXT NOT NULL,
    op_count    INTEGER NOT NULL,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX ON history.snapshots (page_id, created_at DESC);

-- Edit bursts grouped into user-meaningful entries, so the scrubber shows
-- sessions rather than keystrokes.
CREATE TABLE history.sessions (
    id          UUID PRIMARY KEY DEFAULT uuidv7(),
    page_id     UUID NOT NULL,
    actor_ids   UUID[] NOT NULL,
    first_op    UUID NOT NULL,
    last_op     UUID NOT NULL,
    started_at  TIMESTAMPTZ NOT NULL,
    ended_at    TIMESTAMPTZ NOT NULL
);
CREATE INDEX ON history.sessions (page_id, started_at DESC);
```

---

### Snapshot format — Parquet

`object_key` points at a **Parquet** file, not a blob of serialised ops. Columnar, compressed,
self-describing, and — the reason it wins — **queryable in place**: a cold snapshot in object
storage can be read directly by Polars or DuckDB without restoring anything first.

The alternative was an opaque `rkyv` or `bincode` dump, which is faster to write and useless
to anyone who is not this exact binary. Snapshots are the one artifact that outlives the
release that wrote them, so a self-describing format earns its cost.

---

## 6. Redis Keys

| Key | Type | TTL | Purpose |
|---|---|---|---|
| `jwt:blocklist:{jti}` | string | token lifetime | Revocation without a per-request RPC |
| `presence:{page_id}` | hash | 30s refresh | Who is on the page, cursor colour |
| `ratelimit:{actor}:{route}` | string | window | Token bucket at the gateway |
| `collab:instances` | hash | 10s heartbeat | Instance registry for consistent-hash routing |
| `collab:page:{page_id}` | string | session | Which instance owns this page |
| `snapshot:lock:{page_id}` | string | 60s | Distributed lock + fencing token for the snapshot worker |
| `dedup:op:{op_id}` | string | 1h | Idempotent consumer guard |

---

## 7. Type Mapping

```
PostgreSQL              Rust (crates/domain)
──────────              ──────────────────
UUID                ↔   PageId, BlockId, OpId, UserId, FileId  (newtypes)
TEXT                ↔   String / &str
TEXT (kind)         ↔   BlockKind        (serde rename_all = "snake_case")
JSONB               ↔   BlockContent     (tagged enum, versioned)
LTREE               ↔   MaterialisedPath (newtype, validated)
TEXT (sort_key)     ↔   SortKey          (fractional index newtype)
JSONB               ↔   VectorClock
TIMESTAMPTZ         ↔   OffsetDateTime   (time crate)
```

Every id is a distinct newtype over `Uuid` — the compiler rejects passing a `BlockId` where a `PageId` belongs. Cross-schema references are plain `UUID` with no FK, validated at the application layer.

---

## 8. Invariants Not Expressible as Constraints

Enforced in Rust and covered by `proptest`:

| Invariant | Why not a constraint |
|---|---|
| Sibling `sort_key`s are strictly ordered and distinct | Fractional index generation is application logic |
| `blocks.path` matches the `parent_id` chain | LTREE cannot self-reference for validation |
| `content` shape matches `kind` | JSONB is unvalidated by design; enforced by the tagged enum plus `content_version` |
| Adjacent spans have distinct mark sets | Normalisation invariant (RFC-001 §2) |
| Replaying `ops` reproduces `blocks` | The projection property — a test, not a constraint |
| `collab.ops` is never truncated | See §9 — a decision, not an omission |
| No cycles in `parent_id` | Would need a recursive check on every write |

---

## 9. The Log Is Never Truncated

`collab.ops` grows forever. That is a decision, not an omission, and it is recorded because the
absence of a compaction mechanism reads like a gap otherwise.

**Snapshots are performance, never origin.** A snapshot lets replay start late; it does not
become the new beginning of history. Truncating behind one would break the invariant that
replaying `ops` reproduces `blocks`, and it would put a discontinuity in the middle of three
readers that walk the log: per-actor undo, history scrubbing, and Merkle reconciliation.

The growth is bounded by human typing. Batched ~20:1 (RFC-002 §7), one active editor produces a
few megabytes a year. **Phase 12 gets a measurement, not a mechanism** — a dashboard row for
op-log size and growth rate. If the number ever justifies compaction, that is an ADR with real
data behind it rather than a precaution built on a guess.

### The assistant must not stream into the log

A model streams tokens; ops are discrete. One `InsertText` per token would flood the log and
make undo useless. The assistant's output is **buffered client-side and emitted as one op batch
sharing one `undo_group`** when the proposal is accepted. This is not only a volume argument —
it is what makes a proposal reviewable before it lands, which ADR-009 §7 requires anyway.

---

## 10. Event Topics and Subscriptions

The bus inventory, in one place. `ARCHITECTURE.md` §2 draws the graph; this is the table the
Terraform `pubsub` module and the `NatsBus` stream config are both generated from, so a topic
that is not here does not exist.

**One topic per event type, not one per service.** A subscriber that wants two event types takes
two subscriptions rather than filtering a firehose — the cost is a Terraform block and the gain
is that redelivery and dead-lettering are scoped to the thing that actually failed.

### Topics

| Topic | Publisher | Ordering key | Consumed by |
|---|---|---|---|
| `docs.page_created` | `document-service` | `page_id` | search · diagnostics · history |
| `docs.page_renamed` | `document-service` | `page_id` | search · **diagnostics** · publishing |
| `docs.page_deleted` | `document-service` | `page_id` | search · diagnostics · history · notification |
| `docs.block_updated` | `document-service` | `page_id` | search · diagnostics |
| `docs.block_deleted` | `document-service` | `page_id` | search · diagnostics |
| `docs.page_shared` | `document-service` | `page_id` | notification |
| `docs.page_published` | `document-service` | `page_id` | publishing · search |
| `collab.ops_flushed` | `collaboration-service` | `page_id` | **document-service** · history · search · diagnostics · analytics |
| `auth.user_registered` | `auth-service` | `user_id` | notification |
| `auth.user_updated` | `auth-service` | `user_id` | every service holding a `users` projection (§1) |
| `auth.user_deactivated` | `auth-service` | `user_id` | document · notification · search |
| `auth.role_granted` | `auth-service` | `user_id` | document (local permission read-model) |
| `auth.role_revoked` | `auth-service` | `user_id` | document · search (refilter results) |

> **`docs.page_renamed` is the expensive one.** It invalidates diagnostics on every page that
> links to the renamed page (RFC-003 §4), so it is the topic most likely to need its own
> backpressure story before any other.

> **`collab.ops_flushed` is the load-bearing one.** `document-service` materialises `blocks` by
> replaying it (ADR-003). A gap here is not a stale index, it is a wrong page — so
> this is the one subscription where a dead-letter must page someone rather than be swept up
> later.

### Subscription naming

`<consumer>-<topic-suffix>-sub` — `search-page-renamed-sub`, `history-ops-flushed-sub`. The
consumer leads because the failure question is always *"which service is behind?"*, and a name
sorted by consumer answers it without a lookup.

### The two adapters must agree on these, and they do not agree for free

ADR-010 §2 puts `NatsBus` on local and self-host, `PubSubBus` on cloud. The columns above are the
contract both satisfy, and the places they differ are exactly where consumers break:

| | NATS JetStream | Pub/Sub |
|---|---|---|
| Ordering | stream sequence number, total per stream | **ordering key**, per key only |
| Redelivery | `AckWait` expiry, `max_deliver` | `ackDeadline`, exponential backoff |
| Dead letter | max-deliver → DLQ subject | dead-letter topic after `maxDeliveryAttempts` |
| Replay | `seek` to sequence or timestamp | `seek()` to timestamp or snapshot, retention ≤ 31 days |

**Every consumer is idempotent regardless** — dedupe on `OpId` or event id (RFC-002 §4) — which
is what makes the difference survivable. The integration suite runs each consumer against both
adapters; a consumer that passes on only one is a consumer that is relying on ordering it was
never promised.
