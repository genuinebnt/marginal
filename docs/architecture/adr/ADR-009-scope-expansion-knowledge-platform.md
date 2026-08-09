# ADR-009 — Scope Expansion: Notebook → Knowledge Platform

**Date:** 2026-08-07
**Status:** Accepted
**Amends:** ADR-001 (§ What it deliberately does not do)
**Related:** ADR-002 (Rust depth), ADR-004 (SPA, no SSR), ADR-007 (gRPC east-west)
**Deciders:** @genuinebasilnt

---

## Context

ADR-001 scoped Marginal tight — a collaborative markdown notebook — and cut comments,
notifications, publishing, workspaces, RBAC, plugins, and semantic search as "feature
breadth without new learning". That cut was correct at the time: the previous scope was
13 services and 24 phases, and **Phase 1 stalled**.

Two things have changed.

**The architecture proved extensible.** The op log with `encoding_version`, anchors
instead of offsets, `content_version` per row, and the outbox mean most of this list is
additive rather than structural. That was analysed feature by feature before this ADR was
written, not assumed.

**The learning argument reversed.** ADR-001 judged these features to teach nothing new.
That is true of a naive implementation and false of the adaptations below: anchored
comments that survive concurrent edits, reaction counters as a real CRDT, WASM plugins
with capability-based security and fuel metering, and an assistant that emits **ops rather
than text** are each new Rust, not repetition.

**The risk is unchanged and must be stated plainly.** This is the same shape of scope that
killed v1. What makes it survivable is not optimism — it is the ordering rules in
§ Guard Rails, which are part of this decision, not commentary on it.

---

## Decision

Marginal grows from a collaborative notebook into a **self-hosted knowledge platform**:
the same document core, plus identity, discussion, distribution, extensibility, and
assistance.

Each feature below is recorded with the **adaptation** that makes it fit Marginal's
architecture. Copying the shape these features take in a CMS would break invariants the
document core depends on.

### 1. Identity, spaces, and RBAC

Multi-user with real permissions, replacing ADR-001's "single-tenant, auth stays minimal".

**Adaptation:** permissions are enforced at `can_apply(op, actor)` — the chokepoint that
already exists (`RFC-002` §1). No second authorization path is created, and no handler
performs its own check.

Permissions **inherit down the LTREE path**: a grant on a page applies to its subtree
unless overridden. That makes evaluation a tree walk with memoisation rather than a row
lookup, and makes `docs.pages.path` load-bearing for authorization as well as ordering.

### 2. Comments, reactions, mentions

**Adaptation — comments are not ops.** They do not mutate the block tree, so they are not
in the op ISA and do not need inverses. They are a separate aggregate **anchored to block
ranges using RFC-002 anchors**, which is what makes a comment survive concurrent edits to
the text it points at. A comment on deleted text becomes orphaned, not corrupt.

**Adaptation — reactions are a CRDT counter**, not a row plus a lock. Concurrent reactions
from multiple clients converge without coordination; a PN-Counter is the smallest thing
that works and is genuinely new Rust.

Mentions are a parse product of the existing `[[link]]` grammar, extended to `@user`.

### 3. Notifications

A new service consuming the outbox — **the first real subscriber**, which is what finally
justifies the poller (`LLD document-service` §7).

**Adaptation:** delivery is at-least-once and consumers dedupe, exactly like every other
NATS consumer. Digest batching is a time-window fold over the event stream, not a cron
job over a table.

### 4. Publishing, feeds, sitemap, newsletter

**Adaptation — publishing is static pre-render, not SSR.** ADR-004 deleted server-side
rendering deliberately, and this does not bring it back. Publishing a page renders it to
static HTML **at publish time** into object storage, fronted by the CDN. Feeds, sitemap,
and OG images are generated artifacts written by the same job.

This is strictly better than SSR for public pages — no render cost per request, no runtime
to keep alive — and it keeps the SPA/no-SSR decision intact.

The newsletter is double opt-in with notify-on-publish, driven by the same publish event.

### 5. Analytics

**Adaptation:** first-party, privacy-preserving, and **scoped to published pages only**. A
private notebook has no audience, so instrumenting private editing would collect data
nobody will read. Ingest is append-only and lives with `publishing-service` until it earns
its own store (`PROJECT_STRUCTURE.md` §5 — extract on the third use).

### 6. Plugins

**Adaptation — plugins extend the two seams that already exist**, and nothing else:
custom **block kinds** (renderer + input rule) and custom **diagnostic analyzers**. This
is not arbitrary code injection; it is a capability-scoped extension of RFC-001's grammar
and RFC-003's analyzer set.

Sandboxing is `wasmtime` with **fuel metering** (instructions) *and* **epoch interruption**
(wall clock — a plugin blocked in a host call burns no fuel), an explicit WIT-defined host
surface, and no ambient authority: an ungranted capability is absent from the world rather
than denied at the door.

**A plugin never mutates the tree.** Like the assistant in §7, a plugin that wants to change
a document returns **proposed ops**, which pass `can_apply` and land in the log attributed to
the plugin. Writing blocks directly would bypass the authorization chokepoint, break
per-actor undo, and desynchronise peers. Same rule, second consumer.

**Analyzers must be deterministic** — no clock, no randomness, no network by default. An
analyzer returning different results for identical input breaks the incremental engine's
memoisation and makes squiggles flicker (RFC-003 §4).

### 7. The assistant (AI)

**Adaptation — the assistant emits `Op`s, not text.** This is the decision that makes AI
fit rather than fight the architecture:

```
   prompt → assistant-service → proposed Op(s) → can_apply(op, ai_actor) → op log
```

Because AI edits are ops authored by a distinct actor id, three things come free:

- **Per-actor undo (Phase 5) works on AI edits** — undo the assistant without undoing your
  own work
- Collaborators see AI edits arrive exactly like a peer's
- Every AI mutation is in the audit trail, attributable, and invertible

An assistant that wrote text directly into blocks would violate RFC-002 §1 and lose all
three. **"The UI never mutates the tree" now reads "nothing mutates the tree except an op."**

Semantic search is an embedding index inside `search-service`, which already owns its
index and rebuild cadence.

### 8. The full editor, fonts, reader modes, ⌘K

Block directives (callouts, timelines, tabs, columns, math, diagrams, embeds), tables as
layout, footnotes, slash menu, drag handles, multi-block selection.

**Adaptation:** every one is a `BlockKind` plus an input rule plus a grammar entry in
RFC-001. `SetBlockKind` already carries `from` and `to`, so conversions are invertible
without new op design.

⌘K is a client-side composition over existing search and commands — **not a service**.

### 9. Settings, split three ways

| Scope | Owner | Examples |
|---|---|---|
| Instance | `auth-service` (admin) | branding, feature flags, registration policy |
| User | `auth-service` (profile) | theme, fonts, reader mode, notification preferences |
| Page | `document-service` | visibility, publish slug, comment policy |

Three different owners, three different lifetimes. One "settings service" would couple them.

---

## New service boundaries

Four new services, each passing ADR-001's test — differing in **scaling profile, state,
failure mode, or deploy cadence**, not merely owning a different noun.

| Service | Port | Boundary justification |
|---|---|---|
| `notification-service` | 8007 | Bursty fan-out unrelated to editing RPS; **degradable** — a lost notification costs nothing, unlike a lost op |
| `publishing-service` | 8008 | **Unauthenticated public read path**, CDN-fronted, cacheable. Opposite security posture and scaling curve to every other service |
| `plugin-service` | 8009 | **Untrusted code execution.** Isolation is the entire reason it exists; a crash or a fuel exhaustion must not touch the editor |
| `assistant-service` | 8010 | External API dependency with unbounded latency; **degradable**, and must never sit on the editing path |

Seven services becomes eleven. That is a real cost, recorded deliberately.

**Comments and reactions get no service.** They share `document-service`'s scaling profile,
state model, and failure mode — they are a `comments/` slice. Owning a different noun is
not sufficient.

---

## What stays out of scope

Unchanged from ADR-001, and now with a structural reason rather than a scope one:

| Still cut | Why it would hurt |
|---|---|
| Databases, tables, relations, rollups | `docs.ops.page_id` is `NOT NULL` and `collaboration-service` owns exactly one page per instance. Cross-page aggregation has **no owner** — it needs a second ownership tier, which is a distributed-systems redesign, not a feature |
| Formula language / expression VM | Self-contained and portable, but worthless without databases |
| Spatial canvas | Fractional indexing is 1-D. 2-D positions need different convergence, and "every op is invertible" degrades to LWW |
| Mobile apps | The Rust core ports; the editor UI is a second full client |

Any of these still needs its own ADR, and the first one needs a redesign, not an ADR.

---

## Guard Rails

These are part of the decision. Without them this ADR recreates the failure it inherited.

1. **Nothing here starts before Track 1 ships.** The 🏁 in `ROADMAP.md` — log in, write a
   page, edit it live with someone — is unmoved and still comes first.
2. **Every new phase names the new Rust it teaches, or it is cut.** ADR-002 still governs;
   this ADR does not suspend it. A phase that is CRUD in a new costume gets deleted.
3. **Phases are ordered by dependency, not by appetite.** Plugins need the diagnostics
   engine; the assistant needs the search index; publishing needs RBAC. The order in
   `ROADMAP.md` § Execution Order reflects that and is not negotiable by enthusiasm.
4. **One track in flight at a time.** The v1 failure mode was breadth-first.
5. **The document core is closed to changes from these features.** If a feature needs the
   op ISA, the anchor model, or the block grammar to change, that is an RFC amendment with
   its own review — not an incidental edit.

---

## Consequences

- `docs/architecture/rfc/` gains RFC-004 (comments and anchoring), RFC-005 (plugin
  capability model), and RFC-006 (assistant op generation) before their phases begin.
- `DATA_MODEL.md` gains `comments`, `reactions`, `permissions`, `subscriptions`, and
  `published_pages`; `auth` gains roles and preferences.
- The delete saga grows steps — a deleted page must purge comments, notifications,
  published artifacts, and index entries. `ARCHITECTURE.md` §5 is amended.
- `can_apply` becomes the busiest and most security-critical function in the codebase.
  It gets `proptest` coverage and a `/project:security-review` gate on every change.
- Eleven services will not run comfortably on a 4 GB developer machine. Local development
  needs a profile that starts a subset — a `just` recipe per track.
