# Task Breakdown — Track 1

Subtasks for each story in `USER_STORIES.md`, with an owner on every line.

**Scope: Track 1 only** (Phases 1 → 2 → 3, up to the 🏁). Tracks 2–6 are deliberately not
broken down — they are months out, the breakdown would be guesswork, and guesswork in a task
list reads as a commitment. Extend this file one track at a time, when the track starts.

---

## Who owns what

The line was agreed 2026-08-07 and recorded in ADR-005. It is not renegotiated
per task.

| | Owner |
|---|---|
| All Rust — services, `crates/`, **and the `wasm32` editor core** | **You** |
| Design: data model, API contracts, ADRs and RFCs | **You** |
| The TypeScript SPA in `web/` — shell, routing, panels, DOM plumbing, API client | **Me** |
| Verification: Rust test suites, written **after** your implementation | **Me** |
| Go orchestration references — saga sequencing, NATS choreography, retry/backoff | **Me** |
| Go answer keys for DSA items — **only after you have attempted them** | **Me** |
| Mockups, and keeping `docs/` true as the code moves | **Me** |

**The line does not move.** Where the SPA needs a document operation and the `wasm-bindgen`
binding does not exist, the call is **stubbed** — never reimplemented in TypeScript. Crossing
that line once quietly deletes Phase 3.

Two consequences worth internalising:

- **My tests come after your code, not before.** The loop: you decide a module is done → I name what to build next **and the reading
  for it**, never how → you design and implement it alone → **only then** do I write tests,
  which may or may not pass.
  Written first, a test silently chooses the invariant for you, and choosing what is worth
  asserting is the most valuable judgment in this project. Written after, a red bar is a
  surprise you have to reconcile against a design you already own. **Red bars are yours.** I
  do not explain a failure unless you attach a hypothesis to the question.
- **Frontend is off the critical path.** It runs in parallel and finishes ahead of the backend
  it talks to, which is only true if the two handoffs below land on time.

⚠ marks a **handoff** — work of mine that stalls completely until yours lands.

---

## Phase 1 — Documents

### D-01 · Create a page

| Subtask | Owner |
|---|---|
| `pages` schema + migration; `DATA_MODEL.md` updated first | You |
| `PageId`, `Title` newtypes — validate on construction | You |
| Repository trait + sqlx impl, trait declared in the same file | You |
| `#[sqlx::test]` suite: create, fetch, not-found, duplicate title | Me |
| gRPC `PageService.Create`, and its REST shape in `docs/api/pages.md` | You |
| SPA: page list, create action, empty state | Me |

### D-02 · Type into a block and it saves itself

The keystone story. Everything in `document-core` is currently underneath it.

| Subtask | Owner |
|---|---|
| Replace `Vec<Span>` with flat text + marks over byte ranges (RFC-001 §2) | You |
| `InsertText` / `DeleteText` ops at an offset | You |
| Delete's inverse carries the destroyed text *and* its marks | You |
| Failing tests: offsets, non-ASCII, empty block, out-of-range | Me |
| `proptest` law — random op sequence, inverted in reverse, returns to start | Me |
| Op log + replay; a test that replay reproduces the live page | Me |
| Persist ops; the block row becomes a projection, not the truth | You |
| SPA: per-block `contenteditable`, no save button anywhere in the UI | Me |

### D-03 · Enter splits, Backspace removes

| Subtask | Owner |
|---|---|
| `SplitBlock` / `MergeBlock` ops, both invertible | You |
| Caret placement rules — where a writer expects it, written down first | You |
| Failing tests: split at start / middle / end, merge across kinds | Me |
| SPA: key handling, caret restoration after a re-render | Me |

### D-04 · Block prefixes become block kinds

| Subtask | Owner |
|---|---|
| Input-rule scanner — bounded backward scan, borrowing `&'src str` | You |
| One undo step per rule firing (RFC-001 §3) | You |
| Failing tests: each prefix, the escape case, the undo-step case | Me |
| `compiler.html` already runs a reference scanner — read it, don't port it | Me ✔ |

### D-05 · Inline marks

| Subtask | Owner |
|---|---|
| Inline rules: `**`, `_`, `` ` ``, `~~`, `[]()`  — delimiters removed | You |
| Mark maintenance under text edits: grow, shrink, split, merge, normalise | You |
| Never re-parse stored content — the asterisks are gone once stored | You |
| Failing tests + fuzz seeds for mark boundaries and non-ASCII | Me |
| SPA: bubble menu, keyboard shortcuts | Me |

### D-06 · Drag to reorder

| Subtask | Owner |
|---|---|
| `SortKey` newtype; `Ord` must agree with Postgres `COLLATE "C"` | You |
| `key_between` fractional index — **attempt before asking for the Go key** | You |
| Failing tests: midpoint, exhaustion, ordering vs the DB's collation | Me |
| Go answer key for `key_between` — after your attempt, not before | Me |
| ⚠ `wasm-bindgen` export for `key_between` | You |
| SPA: drag-and-drop calling across the wasm boundary | Me |

### D-07 · Nested page tree

| Subtask | Owner |
|---|---|
| LTREE column + recursive CTE for subtree queries | You |
| Move semantics: reparent, cycle rejection | You |
| Failing tests: deep nesting, move into own descendant, orphan detection | Me |
| SPA: sidebar tree, expand/collapse, drag to reparent | Me |

### D-08 · Paste from anywhere

| Subtask | Owner |
|---|---|
| Sanitiser — **an XSS boundary**, treat input as adversarial | You |
| HTML → block tree → normalise; unknown structure degrades to paragraph | You |
| Failing tests: Google Docs span soup, Word `<o:p>`, script payloads | Me |
| `cargo-fuzz` target for paste — never panics on any byte string | Me |
| Security review of the boundary before it merges | Me |

### D-09 · Drop an image

| Subtask | Owner |
|---|---|
| Presigned PUT; the browser uploads directly, bypassing the service | You |
| `image` block kind; the page holds a reference, never bytes | You |
| Failing tests: presign, expiry, oversize rejection | Me |
| SPA: drop target, upload progress, broken-reference state | Me |

### D-10 · Delete a page

| Subtask | Owner |
|---|---|
| Soft vs hard delete decision, recorded in `DATA_MODEL.md` | You |
| **The outbox** — event row written in the same transaction as the delete | You |
| Poller: `FOR UPDATE SKIP LOCKED`, at-least-once, idempotent consumers | You |
| Failing tests: commit-succeeds-publish-fails leaves no lost event | Me |
| Go reference for the outbox poller loop and its backoff | Me |
| SPA: delete affordance, confirmation, optimistic removal | Me |

### Phase 1, not attached to one story

| Subtask | Owner |
|---|---|
| Verify `document-core` builds for `wasm32-unknown-unknown` — **do this now** | You |
| Terraform: Cloud SQL, Cloud Storage, Secret Manager, one Cloud Run deploy | You |
| ⚠ **Handoff, ~week 4:** the thin gateway — ~150-line proxy + JWT verification | You |
| SPA runs against mocks generated from `docs/api/pages.md` until that lands | Me |
| `utoipa` annotations on every endpoint → OpenAPI → `openapi-typescript` | You |

---

## Phase 2 — Auth

### A-01 · First-run setup

| Subtask | Owner |
|---|---|
| Detect "no users exist" and expose setup instead of login | You |
| Failing tests: setup available once, and exactly once | Me |
| SPA: the setup screen — a fresh instance's first screen is not a login | Me |

### A-02 · Register, log in, stay logged in

| Subtask | Owner |
|---|---|
| `argon2` PHC strings — parameters upgradable without a migration | You |
| RS256 signing; the gateway verifies locally, no per-request RPC | You |
| Refresh rotation with a parent chain | You |
| Failing tests: rotation, expiry, clock skew, wrong signature | Me |
| SPA: forms, token storage, silent refresh | Me |

### A-03 · Log out, and the session dies

| Subtask | Owner |
|---|---|
| Redis blocklist keyed by `jti`, TTL to natural expiry | You |
| Reuse of a revoked refresh token revokes the whole chain | You |
| Failing tests: revoked token refused before expiry; reuse detection | Me |
| Security review — timing, `subtle` comparisons, error uniformity | Me |

### A-04 · Only I can read my pages

| Subtask | Owner |
|---|---|
| Ownership check at the gateway, enforced server-side | You |
| Refusals do not leak existence — same response either way | You |
| Failing tests: cross-account read, write, and enumeration | Me |
| SPA: 403/404 handling that does not leak either | Me |

---

## Phase 3 — Collaboration

### C-01 · See changes as they are typed

| Subtask | Owner |
|---|---|
| `collaboration-service`: rope per document, one owner per page | You |
| WebSocket session handling, join/leave, snapshot on join | You |
| Failing tests: two clients converge; late joiner catches up | Me |
| ⚠ **Handoff, ~week 14:** `wasm-bindgen` signatures — *signatures only* | You |
| SPA: transport, reconnect, applying remote ops through wasm stubs | Me |

The handoff is the one to protect. It is the exported surface — build a document, apply a
keystroke and get ops back, apply a remote op, query selection, produce render state. Deciding
that shape determines what is Rust and what is DOM, so it is design work and it is yours.

### C-02 · No merge conflict UI, ever

| Subtask | Owner |
|---|---|
| Transform on both sides; sequencer rebases each arrival onto its `basedOn` | You |
| **Attempt the transform before asking for any reference** | You |
| Failing tests: all four op kinds against each other | Me |
| Go reference for sequencer *choreography* only — never `xform` itself | Me |
| Audit that no conflict UI exists anywhere in `web/` | Me |

### C-03 · Presence

| Subtask | Owner |
|---|---|
| Redis presence with TTL; caret and selection broadcast | You |
| Caret rides the transform — it must not drift on a remote edit | You |
| Failing tests: caret position after concurrent insert before/after it | Me |
| SPA: presence stack, remote carets, actor colours | Me |

### C-04 · Responsive on a bad connection

| Subtask | Owner |
|---|---|
| Local prediction; rollback by real inverses, never snapshot restore | You |
| One op in flight; pending queue transformed in place while waiting | You |
| Failing tests: rollback correctness under loss, jitter, reordering | Me |
| `netcode.html` runs all of this already — read it before you start | Me ✔ |

### C-05 · Disconnect, reconnect, keep the work

| Subtask | Owner |
|---|---|
| Sequence gaps, replay requests, idempotency by `(site, n)` | You |
| Retransmit sends the **frozen wire form**, not the locally transformed op | You |
| Failing tests: gap replay, duplicate delivery, long offline | Me |
| The long-offline half has no mockup — it needs a design note from you first | You |

### C-06 · Intention survives the merge

| Subtask | Owner |
|---|---|
| Decide what instrument proves it — convergence alone does not | You |
| Failing tests: two editors in one paragraph produce the intended text | Me |
| GKE arrives here — `collaboration-service` is what makes it necessary | You |

---

## Extending this file

When Track 2 starts, break down its stories the same way and append. Do not do it sooner.

Two rules that keep the split honest:

- **If a subtask of mine has no failing test in it, look again** — my job on the Rust side is
  the spec, and a subtask that is not a spec is probably one I should not be doing.
- **If a subtask of yours is a DOM detail, it is mine.** If one of mine is a document
  operation, it is yours, and the correct move is a stub plus a note here.
