# The Rust track — archived, not deleted

This folder holds documents only. No Rust code lives here or anywhere else
in this repo. See `docs/architecture/adr/ADR-011-go-typescript-mvp-then-rust-port.md`
for why: the Track 1 MVP is being built in Go + TypeScript first, fully
tested and deployable, and the hand-port to Rust happens afterward in a
**new, separate repository** — not this one.

## What's here

- `agents.md` — the mentor-mode rules that governed the original Rust
  attempt: pseudo-code scaffolding, the "you write the Rust" boundary, the
  DSA/book reading map. None of it governs the Go+TS build (see the
  top-level `.agents/agents.md` instead).
- `TASKS.md` — the 41-step queue as it stood when the attempt paused, frozen
  at Step 2. Its **Open decisions** table (#3–#8) is the useful part: real
  product questions (`BlockId` as `UUID` not `u64`, the `Op` ISA field
  shapes, `Heading` level validation, soft/hard delete, anchor
  representation) that were caught before the Rust code got far. Some of
  these are already resolved in the Go implementation by following
  `RFC-001`/`RFC-002`/`DATA_MODEL.md` directly — check `docs/porting/OPEN_QUESTIONS.md`
  for which ones are still genuinely open.
- `learning/` — the per-phase Rust/DSA reading list (Rust for Rustaceans,
  Crust of Rust episodes, Skiena, DDIA, Database Internals, Rust Atomics and
  Locks, etc.), unchanged from the original track.

## If you're starting the Rust-port repo

1. Read `TASKS.md` § *Where you are* and the open-decisions table first —
   it's exact, not a summary.
2. Read `docs/porting/PROGRESS.md` and `docs/porting/PORTING_GUIDE.md` in
   the Go+TS repo for how the port stopped/what's implemented.
3. Treat the **Go/TS codebase as the primary reference**, not this folder —
   these docs are historical context for *why* things are shaped the way
   they are, not a spec to satisfy. The actual spec (`RFC-001`, `RFC-002`,
   `DATA_MODEL.md`, `docs/api/`) lives at the top level of the Go+TS repo and
   is language-agnostic; it applies to the Rust port unchanged.
4. Port test-by-test: `testdata/*/​*.json` golden vectors in the Go+TS repo
   are the behavior spec. If a Go/TS test passes a vector, the Rust
   equivalent should too.
5. `agents.md` here still describes a workable mentor-mode loop (scaffold →
   you write the Rust → tests turn real) if you want to keep learning Rust
   deeply during the port rather than transliterating quickly.
