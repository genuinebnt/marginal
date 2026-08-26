# ADR-011 — Go + TypeScript MVP First, Rust Port Later in a Separate Repo

**Date:** 2026-08-26
**Status:** Accepted
**Related:** Supersedes ADR-005 (Go reference as answer key); overrides ADR-004's
"revisit only after the 🏁" clause for the editor-core-language question;
suspends ADR-002 (Rust depth primary) for the duration of this track.
**Deciders:** @genuinebasilnt

---

## Context

ADR-005 considered and rejected "have an AI write the whole product in
another language and port it to Rust" as the primary method, for three
stated reasons:

1. **It deletes the DSA objective.** Porting an AI-written BK-tree teaches
   the shape of a BK-tree, not why the triangle inequality permits subtree
   pruning.
2. **The return curve is inverted.** A GC language has nothing to port for
   the highest-value Rust content — `crossbeam-epoch`, arena allocation,
   `MaybeUninit`, `repr(align)`, `Arc<RwLock<T>>` vs. channel ownership. It
   saves the least effort exactly where the work is most ambitious.
3. **Reading a large port you didn't design is slower than designing your
   own.**

The decision now is to do exactly what ADR-005 rejected, deliberately.

## Decision

Build the Track 1 MVP (Documents → Auth → Collaboration, the 🏁) completely
in Go (services, clean architecture, gRPC internally / REST at the gateway)
and TypeScript (SPA + editor-core logic, no `wasm32` boundary), with Claude
writing the implementation directly rather than scaffolding it for mentor-mode
learning. Once it is fully tested and deployable, hand-port it to Rust in a
**new, separate repository** — not this one. Extra features (Tracks 2–6)
are built on top only after that port, in the new repo's own time.

### Answering ADR-005's three objections directly

**On the DSA objective:** it isn't deleted, it's resequenced. Every
algorithmic decision in this codebase — the op log's invertibility law, the
mark-coalescing/normalisation rules, the anchor model, whatever indexing
`search-service` eventually needs — still has to be reasoned about to
implement in Go; Go has no `todo!()`-shaped shortcut for "why does this
algorithm work." What's deferred, not deleted, is doing that reasoning a
second time in Rust's ownership model. The port becomes "I already know why
this works, now express it under a different type system" rather than
"figure out why this works while also fighting the borrow checker" — a
narrower, later problem, not an absent one.

**On the inverted return curve:** accepted as a real cost, not a solved
problem. Arena allocation, epoch reclamation, `MaybeUninit`, `repr(align)`,
and ownership-vs-channel choices genuinely have nothing to port from a Go
reference — this was true under ADR-005 and stays true here. The difference
is what's being optimized for right now: a complete, demonstrable product
that exists and works, on a timeline that supports showing it in an
interview. That is a legitimate objective competing with Rust depth, not a
rationalization for skipping the hard parts forever — the Rust port is where
those GC-shaped gaps get filled in, deliberately, the same way ADR-005's
stage 5 always intended the hardest DSA items to be attempted for real
before any reference existed.

**On reading being slower than designing:** doesn't apply here the way it
applied to an AI-authored Go snippet used as a crib sheet mid-build. The
port is a distinct, later phase, in a separate repo, with the design
questions already settled (`RFC-001`/`RFC-002`/`DATA_MODEL.md` don't change)
and a working reference implementation to check behavior against test-by-test.
That's closer to porting your own prior work than to reading someone else's.

### What actually changes

| | Under ADR-005 | Under ADR-011 |
|---|---|---|
| Who writes the MVP | User, Rust, mentor-mode scaffolds | Claude, Go + TypeScript, directly |
| Go's role | Narrow: orchestration reference + late DSA answer-key, per feature | Primary: the actual MVP implementation |
| Editor core | Rust → `wasm32` (ADR-004) | Native TypeScript, no `wasm32` (overrides ADR-004's "not before the 🏁" clause — see below) |
| Rust | Concurrent with the Go build | Deferred entirely to a future, separate repo |
| Design docs (RFCs, DATA_MODEL, ADR-001/003/007/008/009/010) | Authoritative | Unchanged, still authoritative — govern the Go build the same way |

**ADR-004's override, made explicit:** that ADR states the Rust-editor-core
question should be "revisited after the 🏁, never before." This decision
revisits it before the 🏁, on purpose — the whole premise of building the
MVP in Go+TS is that the editor core is TypeScript for now too. ADR-004's
React SPA choice otherwise stands unchanged.

**ADR-002 (Rust depth primary):** suspended, not repealed. It re-applies in
full, unchanged, to the future Rust-port repo.

### Guardrails carried over

- **Idiomatic Go/TS first.** Portability is not achieved by contorting Go
  into Rust shapes — Go errors stay `(T, error)`, not a ported `Result`;
  interfaces stay small and declared at the point of use, not one-trait-
  per-type mirrors. Portability comes from shared *behavior* (golden test
  vectors, matching module/service boundaries) and documentation, exactly
  the risk ADR-005 named about Go-shaped Rust, mirrored the other direction.
- **The deleted Rust attempt is not a design to reproduce.** It had known-
  wrong shapes its own open-decisions list (`docs/rust/TASKS.md`) already
  flagged — `BlockId(u64)` instead of `Uuid`, `Op` variant names drifted
  from `RFC-002`'s ISA, unvalidated `Heading{level}`. The Go implementation
  follows the RFCs/DATA_MODEL directly, not the deleted draft.
- **Feature depth, not surface area, is the complexity budget.** Each
  service is scaffolded as a standalone code area (not phased in one at a
  time) since they share no code. The limiting factor on scope is not route
  count or number of services — it's how far any one feature is pushed past
  "solid and complete" into speculative sophistication a Track-1 demo
  doesn't need.

## Consequences

- `docs/rust/` in this repo holds documents only (the old `agents.md`,
  `learning/`, `TASKS.md`) — a waypoint for whoever starts the future
  Rust-port repo, not a spec the Go code has to match.
- `docs/porting/` tracks progress, open questions, and the porting approach
  as the MVP is built, so context survives session compaction without being
  re-derived or hallucinated.
- CI, benchmarks, and test suites for the Go+TS MVP are designed so the
  future Rust port has something concrete to match against — golden JSON
  test vectors under `testdata/`, and recorded baseline benchmarks.

## Resources

Same as ADR-005's, now aimed at Go/TS instead of the crib-sheet role:
[Effective Go](https://go.dev/doc/effective_go) for the idiom this decision
insists on keeping; the RFCs and `DATA_MODEL.md` as the specs that didn't
change; `docs/porting/PORTING_GUIDE.md` for the future hand-port's approach.

---

## Addendum — the editor core is Go compiled to wasm, not native TypeScript

**Added 2026-08-26, same day, after further discussion.**

The decision above originally said the editor core is native TypeScript,
"no `wasm32` for this track — `ADR-011` overrides `ADR-004`." That's
narrowed: **all business logic, including the editor core, is written once,
in Go — `services/document-service/internal/documentcore` — and compiled to
`GOOS=js GOARCH=wasm` for browser use. TypeScript is views and a thin JSON
bridge only, never a second implementation of the logic.**

**Why this is better than the plan it replaces.** The original plan had
document-core implemented twice — once in Go (for document-service),
once natively in TypeScript (for the browser) — kept in sync only by
running the same `testdata/document-core/*.json` golden vectors against
both. That's real duplication risk: two implementations of mark
coalescing, op preconditions, and invertibility, agreeing only as long as
someone remembers to keep both updated. Compiling the one Go
implementation to wasm removes the second implementation entirely — there
is exactly one `Page.Apply`, one `Content.AddMark`, and both
document-service and the browser call it.

**This also un-loses something ADR-011's original text traded away.**
ADR-004 always specified the editor core compiles to `wasm32` — that was
true for the Rust design and stays true here, just with `GOOS=js` (Go's
wasm target) standing in for `wasm32-unknown-unknown` (Rust's) until the
port happens. The wasm boundary itself was never the thing being removed;
only its source language changed, temporarily. Porting to Rust later means
recompiling `internal/documentcore` to `wasm32-unknown-unknown` instead of
`js/wasm` — the JS-side loader (`web/src/document-core/wasm.ts`) needs
minimal changes, since it already treats the module as "JSON in, JSON out"
rather than assuming anything Go-specific about it.

**The JSON boundary is deliberately stringly-typed, not a rich `js.Value`
marshaling scheme** (`cmd/wasm/main.go`) — every exported function takes
and returns JSON strings, `{value, error}` envelopes. That's the same
shape a real HTTP/gRPC call to document-service would use, so views never
need to know whether they're calling local wasm or the network — and it's
the shape that will still make sense once the wasm module is Rust instead
of Go.

**What stays thin on the TypeScript side, and why that's not "logic
creeping back into TS":** `web/src/document-core/history.ts`'s undo/redo
stacks are two arrays and push/pop — bookkeeping, not an implementation of
undo/redo *semantics* (every apply and invert call still goes through the
wasm module). `types.ts`'s object-literal builders (`paragraph()`,
`bold()`, ...) are shape declarations with no validation — Go's
`Page.Apply` is the only place a `Heading{level: 0}` gets rejected.
