# Task Queue — Track 1

**A numbered queue. Do them in order.** Each step is what comes next when the previous one is
green — the thing you would otherwise have to ask for.

Steps marked **[me]** are tests written *after* your implementation. If you are working alone,
they become yours: implement first, then ask *"what would prove this wrong?"*, then write that.
Steps marked **[spa]** can be skipped entirely — the frontend is off the critical path
(`TIMELINE.md` §3) and no Rust waits on it.

**Scope: Track 1 only** (Phases 1 → 2 → 3, to the 🏁). Track 2 gets broken down when Track 2
starts, not before.

---

## Where you are

**Updated 2026-08-15. → Step 2.**

`crates/document-core` exists and is the only crate. `Page`, `Block`, `Op` with `invert()`,
`History` with undo/redo and atomic rollback. **18 tests green, and `wasm32-unknown-unknown`
builds clean.** `inline.rs`'s `Span` is provisional — Step 3 deletes it.

**Step 1 is done.** `PageId::new` now takes a `Uuid` instead of manufacturing one, and the
`uuid` feature moved out of `[workspace.dependencies]` so `document-core` inherits no rng.
Whoever generates ids adds `features = ["v7"]` to its own manifest.

### Why the queue is not in story order

`document-core` is `wasm32`-clean and needs no infrastructure — pure `cargo test`, per
`CLAUDE.md`. So **every pure-logic step comes first** (Steps 1–17), and Postgres is stood up once,
at Step 18, when something finally needs it. Story D-01 is "create a page" and would normally be
first; it is at Step 19 because it is the first thing that cannot run without a database.

---

## Open decisions

Yours — ADR-005 puts design on your side of the line. Each blocks a step below.

| # | Decision | Blocks |
|---|---|---|
| ~~1~~ | ~~Where UUIDs are generated.~~ **Resolved.** `document-core` never manufactures an id; it receives one. Randomness is ambient authority and a crate that must compile into a `wasm32` sandbox cannot reach for it — the same reason `CLOUD_PORTABILITY.md` §2 has `Clock` behind a trait. | — |
| ~~2~~ | ~~v4 or v7.~~ **Resolved by #1** — v7 needs an rng too (48-bit timestamp **plus 74 random bits**), so it moved out of the model with v4. The service that generates ids uses v7, for the index locality `DATA_MODEL.md` §7 wants. | — |
| **3** | **`BlockId(u64)` contradicts the schema.** `docs.blocks.id` is `UUID`, and `lld/document-service.md` §105 lists `PageId`, `BlockId`, `UserId` all wrapping `Uuid` (v7). `RFC-001` §9 also wants Yjs-style `ItemId` for anchors, which a `u64` counter cannot become. `BlockId` is already in all four `Op` variants. | Step 3 |
| **7** | **Op names drift from the ISA.** Code has `UpdateBlockKind` / `UpdateBlockContent` / `DeleteBlock { after, block }`; RFC-002 §2 says `SetBlockKind` / `SetBlockContent { block, content, prev }` / `DeleteBlock { id, tombstone }`. Rename the code or amend the RFC — but they must agree before ops are persisted, because `collab.ops.kind` stores the name as text. | Step 5 |
| **8** | **`Heading { level: u8 }` accepts 0 and 255.** DATA_MODEL says `heading_1..3`. Same class as `Title`: `lld/document-service.md` §105 puts validation in `TryFrom`, unskippable. Also `CodeBlock { language }` — DATA_MODEL puts `language` inside `content`, not in the kind. | Step 3 |
| **4** | **`Op::DeleteBlock`'s `after`.** `apply` ignores it, `invert` depends on it, nothing checks they agree — so a wrong `after` applies cleanly and restores the block to the wrong place on undo. | Step 9 |
| **5** | **Soft or hard delete for pages.** | Step 30 |
| **6** | **Anchor representation** (`RFC-001` §9) — appears in op payloads, which are append-only forever. | Step 36 |

---

# Phase 1 — Documents

**Read once before Step 3:** `docs/learning/01-track1-mvp.md` § Phase 1 § *Before you build*, the
mandatory rows. Then `lld/document-core.md` for the editor core and `lld/document-service.md` for
the service — Phase 1 is both.

---

## Part A — `document-core`, no infrastructure (Steps 1–17)

Nothing here needs Postgres, Docker, or a network. Pure `cargo test`.

### ✅ Step 1 · Make `wasm32` build — **done**

```
rustup target add wasm32-unknown-unknown
cargo build -p document-core --target wasm32-unknown-unknown
```

Resolved Open decisions #1 and #2. **Re-run this after every step in Part A** — Steps 3–16 are
exactly where an accidental dependency on the host creeps back in.

### Step 2 · Clean the lints, commit

`cargo clippy --fix` — two mechanical warnings. Then commit; Step 3 deletes a file and you want a
baseline to diff against.

**Done when:** `cargo clippy` silent, working tree clean.
**Then stop.** The rest of the refactor list is deferred until friction demands it
(`PROJECT_STRUCTURE.md` §5).

### Step 3 · Flat text + marks over byte ranges

**Spec:** RFC-001 §2 — read the whole section. Then `lld/document-core.md` §3 (`inline.rs`) and
**§9 (byte vs char vs grapheme)**, which is the one that bites.

Delete `Span`. A block holds a `String` and a list of marks over **byte** ranges.

**Done when:** `Span` is gone, `Block` holds flat text, and existing tests compile again.

### Step 4 · **[me]** Tests for the mark model

Boundaries, non-ASCII, empty block, out-of-range.

### Step 5 · `InsertText` / `DeleteText` at an offset

Two new `Op` variants. Both apply, both invert, both refuse an out-of-range or
non-char-boundary offset.

**Done when:** the existing round-trip law passes for both new variants.

### Step 6 · **[me]** Tests for text ops

### Step 7 · Delete's inverse carries the destroyed text **and** its marks

**Spec:** RFC-002 §3 — invertibility is a design constraint, not a later feature.

**Done when:** undoing a delete restores formatting, not just characters.

### Step 8 · **[me]** `proptest` law

Random op sequence, inverted in reverse order, returns to the starting document. 1000 cases.

### Step 9 · Fix `Op::DeleteBlock`'s `after` (Open decision #4)

The invariant hole. Write the test that exposes it first, then whichever fix the test argues for.

### Step 10 · `SplitBlock` / `MergeBlock`

**Story:** D-03. Write the **caret placement rules in prose first** — a rule a test can check —
then implement.

**Done when:** both ops invert; the prose rule exists.

### Step 11 · **[me]** Tests: split at start / middle / end, merge across kinds

### Step 12 · The input-rule scanner

**Story:** D-04. **Spec:** RFC-001 §3, and `lld/document-core.md` §3 `scan.rs`.
**New Rust:** a lifetime on a type — the scanner borrows `&'src str` rather than allocating per
token. `ROADMAP.md` calls this the one place in Phase 1 that forces a lifetime.

Bounded backward scan. No grammar, no recursion, a fixed lookbehind.

**Done when:** no per-token allocation, and you can name the bound.
**Read first, don't port:** `ui-mockups/compiler.html` runs a reference scanner.

### Step 13 · One undo step per rule firing

**Done when:** Cmd+Z restores the literal `## `, and a second Cmd+Z undoes the typing.

### Step 14 · **[me]** Tests: each prefix, the escape case, the undo-step case

### Step 15 · Inline marks — rules and delimiter removal

**Story:** D-05. `**`, `_`, `` ` ``, `~~`, `[]()`.

**Done when:** the mark applies and the delimiters are **not in the stored text**.

### Step 16 · Mark maintenance under edits

Grow, shrink, split, merge, normalise. **Never re-parse stored content** — the asterisks are gone.

**Done when:** typing inside a bold run stays bold; typing at its edge is decided and tested.

### Step 17 · **[me]** Mark-boundary tests, fuzz seeds, idempotence proptest

> **`document-core` is now feature-complete for the MVP.** Re-run the `wasm32` build before moving
> on — Steps 3–16 are exactly where an accidental dependency creeps in.

---

## Part B — `document-service` and infrastructure (Steps 18–26)

The first steps that need something running.

### Step 18 · Postgres, migration, `Settings`

**Spec:** `DATA_MODEL.md`, `lld/document-service.md`. **Docs before code** — update
`DATA_MODEL.md` first if the schema differs from what is written.

`docker compose up` with Postgres 18. One migration. A `Settings` struct that refuses to start on
a missing variable.

**Done when:** `sqlx migrate run` is clean and the service starts.

### Step 19 · `pages` schema + `PageId` / `Title` newtypes

**Story:** D-01. Validate on construction — an invalid title cannot be constructed, only rejected.

### Step 20 · `PageRepo` trait + sqlx impl, **same file**

`PROJECT_STRUCTURE.md` §4: every external dependency sits behind a trait declared in the same file
as its impl.

### Step 21 · **[me]** `#[sqlx::test]` suite

Create, fetch, not-found, duplicate title. Real Postgres, isolated database per test — never a mock.

### Step 22 · gRPC `PageService.Create` + the REST shape

Write `docs/api/pages.md` §2 as you go. `utoipa` annotations are mandatory (`docs/api/README.md`).

**Done when:** `grpcurl` creates a page and the REST mapping is written down.

### Step 23 · Persist ops; make `blocks` a projection

**The central rule** (`DATA_MODEL.md` §1): the op log is the source of truth, block rows are a
projection.

**Done when:** deleting every `blocks` row and replaying `ops` rebuilds the page exactly.

### Step 24 · **[me]** Replay test

Proves Step 23's claim, and it is the test that must never be deleted.

### Step 25 · ⚠ **Handoff — the thin gateway**

~150-line proxy plus JWT verification. **The SPA is blocked until this lands** (`TIMELINE.md` §6).

### Step 26 · Terraform — first cloud deploy

Serverless Postgres, Cloud Storage, Secret Manager, one Cloud Run deploy, budget alert.
**`min = 0`** (ADR-010 §1).

**Done when:** `terraform apply` then `terraform destroy`, both clean.

---

## Part C — the rest of Phase 1 (Steps 27–31)

> **Not in the MVP gate.** The 🏁 is D-01…D-05, A-01…A-04, C-01…C-04. Steps 27–31 are real work
> that moves first if time compresses. Know that before spending three days on drag-and-drop.

### Step 27 · `SortKey` + `key_between` — fractional indexing

**Story:** D-06. **Spec:** [Greenspan's notebook](https://observablehq.com/@dgreensp/implementing-fractional-indexing),
then Figma's post. `TIMELINE.md` assumption 3 budgets **~16h, not 8** — it is the first genuinely
subtle thing in this project.

**The trap:** `SortKey`'s `Ord` must agree with Postgres `COLLATE "C"`, and the default collation
is not byte order.

**Attempt it before asking for the Go answer key.**

### Step 28 · **[me]** Tests: midpoint, exhaustion, ordering vs the DB collation

### Step 29 · LTREE page tree + cycle rejection

**Story:** D-07. Moving a page into its own descendant is refused. Orphan detection is a
**connected-components** problem, not `backlinks == 0`.

### Step 30 · Paste sanitiser — **an XSS boundary**

**Story:** D-08. Allowlist, not denylist. Treat every input as adversarial. `cargo-fuzz` it; it
must never panic on any byte string. Security review before it merges.

### Step 31 · Image upload + page delete with the outbox

**Stories:** D-09, D-10. Presigned PUT so bytes never pass through Rust. Then the **outbox** —
event row in the same transaction as the delete, poller with `FOR UPDATE SKIP LOCKED`,
at-least-once, idempotent consumers. Resolve Open decision #5 first.

---

# Phase 2 — Auth (Steps 32–35)

**Reading:** `docs/learning/01-track1-mvp.md` § Phase 2. **Do not reach for a managed identity
provider** — building this is the point, and it would break self-hosting (ADR-001).

### Step 32 · First-run setup

**Story:** A-01. A fresh instance's first screen is not a login. Available once, and exactly once.

### Step 33 · Register, log in, stay logged in

**Story:** A-02. `argon2` PHC strings so parameters upgrade without a migration. RS256 signing,
**verified locally at the gateway with no per-request RPC** — that decoupling is why
`auth-service` being down does not break authenticated requests. Refresh rotation with a parent
chain.

### Step 34 · Log out and mean it

**Story:** A-03. Redis blocklist keyed by `jti`, TTL to natural expiry. Reuse of a revoked refresh
revokes the **whole chain**. Security review: timing, `subtle` comparisons, error uniformity.

### Step 35 · Ownership enforcement

**Story:** A-04. Enforced server-side. **Refusals must not reveal existence** — 404 and 403
indistinguishable to a prober, timing included.

---

# Phase 3 — Collaboration (Steps 36–41)

**The hardest phase.** `TIMELINE.md` assumption 5 puts it at **~130h of Rust and calls that a
floor**. Read `lld/collaboration-service.md` **completely** before Step 36 — this is the one
service you cannot build bottom-up one slice at a time.

### Step 36 · Confirm the anchor representation (Open decision #6)

**Spec:** RFC-001 §9. Yjs/Peritext-style item ids, not offset-plus-origin.

**Do this before any op ships.** Anchors appear in op payloads and op payloads are append-only
forever — getting it wrong is not a refactor.

### Step 37 · The rope and the doc-actor

**Story:** C-01. One rope per document, **one owner per page** — an actor, not `Arc<RwLock<Rope>>`.
The rope is owned, not contended, and an actor makes that structural rather than a discipline.

Read [Zed's Rope & SumTree post](https://zed.dev/blog/zed-decoded-rope-sumtree) first.

### Step 38 · ⚠ **Handoff — `wasm-bindgen` signatures only**

The exported surface: build a document, apply a keystroke and get ops back, apply a remote op,
query selection, produce render state. **Signatures, not implementations.** That shape decides
what is Rust and what is DOM, so it is design work and it is yours.

### Step 39 · Transform on both sides

**Story:** C-02. The sequencer rebases each arrival onto its `basedOn`. **Attempt the transform
before asking for any reference** — the Go reference covers choreography only, never `xform`.

`ui-mockups/netcode.html` runs all of this already. Read it.

### Step 40 · Presence, and the caret that rides the transform

**Story:** C-03. Redis presence with TTL so a crashed client disappears without a leave message.
The caret must not drift when a remote edit lands before it.

### Step 41 · Local prediction and rollback

**Story:** C-04. Rollback by **real inverses** — the same `Op::invert` undo uses — never snapshot
restore. One op in flight; the pending queue transformed in place.

> 🏁 **The MVP gate closes here.** D-01…D-05, A-01…A-04, C-01…C-04 all true at once.
> Log in, write a page, edit live with someone.

---

## After the 🏁

C-05 (reconnect and keep the work) and C-06 (intention survives the merge) finish Phase 3.
Then Track 2 — Diagnostics, Undo/Redo, History — and `ADR-009` § Guard Rails becomes binding:
nothing from Tracks 4–5 starts before this gate.

Break Track 2 into steps when Track 2 starts. Not before.

---

## Standing rules

- **Commit at every green bar.** A clean baseline is what makes a red bar mean something.
- **Docs before code** when the schema, an endpoint, the op set, or an analyzer changes —
  `CLAUDE.md` § Documentation Rules names the four files to check.
- **Never mock infrastructure.** `#[sqlx::test]` and Testcontainers hit the real thing.
- **`document-core` stays `wasm32`-clean.** Verify the target build; do not assume it.
- **A *done when* that could not fail is not a *done when*.** Rewrite it until it could.
