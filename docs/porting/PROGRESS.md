# Porting Progress Log

Read this first in any new or compacted session — it's the record of what's
actually done, not a summary to re-derive from memory. Append short entries
as work lands. Don't let this balloon into a second `docs/rust/TASKS.md`.

---

## 2026-08-26 — Pivot to Go+TS MVP first

Decided (see `ADR-011`): build the Track 1 MVP (Documents → Auth →
Collaboration) completely in Go + TypeScript, Claude writing it directly,
before any Rust work. The Rust hand-port happens later, in a new separate
repo. Reasoning: ADR-005's three objections to this shape (deletes DSA
objective, GC has nothing to port for the hardest Rust content, reading a
port is slower than designing one) are accepted as real costs, traded for a
complete, demo-quality product on a nearer timeline.

Repo reorg done: `crates/document-core` and the root Cargo workspace deleted
(not archived — RFC-001/RFC-002/DATA_MODEL.md are the spec now, not the
deleted draft, which had known-wrong shapes its own open-decisions list
already flagged). Rust-mentor-mode docs (`.agents/agents.md`,
`docs/learning/`, `docs/planning/TASKS.md`) moved to `docs/rust/` as a
doc-only waypoint. New top-level `.agents/agents.md` written for direct
Go/TS implementation.

Scope for the MVP: standalone skeleton code areas for all three Track 1
services (`document-service`, `auth-service`, `collaboration-service`) now;
real logic lands one at a time, starting with `document-service`'s
`document-core`. Complexity budget is feature depth, not route/service
count — see `.agents/agents.md` §3.

**Next:** scaffold `services/` and `web/` workspaces (Phase B), then
implement `document-core` in both (Phase C) — see `PORTING_GUIDE.md`.

---

## 2026-08-26 — Layout correction: `services/` at root, `docs/rust/`

Two corrections after the initial reorg: the Go backend doesn't need an
extra `go/` wrapper directory — it lives at `services/<name>/` directly,
keeping `web/` (frontend) and `services/` (backend) as the two clearly
separate top-level areas. And the archived Rust docs move under
`docs/rust/` rather than a top-level `rust/`, since a new Rust repo will be
created separately later rather than resuming in this one — `rust/` at the
top level implied more permanence than intended.

`services/document-service`, `services/auth-service`,
`services/collaboration-service` scaffolded: each its own Go module
(`go.work` at repo root), `cmd/main.go` with a health-probe-only HTTP
server, no business logic yet. `services/document-service/internal/documentcore/`
exists as an empty package dir, ready for Phase C.

---

## 2026-08-26 — `document-core` implemented in Go; TS reduced to views + WASM bridge

**Architecture correction, same day:** the plan up to this point had
`document-core` implemented twice — natively in Go and natively in
TypeScript, kept in sync via shared `testdata/document-core/*.json`
vectors. Per direction: **that's wrong. Business logic is Go only,
compiled to `GOOS=js GOARCH=wasm` for the browser; TypeScript is views and
a thin JSON bridge, never a second implementation.** See `ADR-011`'s
addendum for the full reasoning — this also restores the `wasm32` boundary
ADR-004 always specified for the editor core, just with Go standing in for
Rust until the port.

**`internal/documentcore` implemented** (`ids.go`, `block.go`, `inline.go`,
`operation.go`, `operation_json.go`, `page.go`, `history.go`), following
`RFC-001`/`RFC-002`/`DATA_MODEL.md` directly rather than the deleted Rust
draft's shapes:

- `BlockID`/`PageID` are `Uuid`, never manufactured internally (received
  from the caller only) — resolves `docs/rust/TASKS.md` open decisions
  #1–#3.
- `Op` variants and field names match `RFC-002` §2's ISA exactly
  (`InsertBlock`, `DeleteBlock{Tombstone, After}`,
  `SetBlockKind{From,To}`, `SetBlockContent{Block,Prev,Content}`,
  `SetTitle`, `MoveBlock`) — resolves open decision #7. Character-granular
  ops (`InsertText`/`DeleteText`/`SetMark`) are out of scope until Phase 3
  (collaboration-service's rope exists).
- `Heading{level}` validated at construction (1..=3) — resolves open
  decision #8. `CodeBlock{language}` modeled as a block-level attribute on
  `BlockKind`, not inside `Content` — a deliberate call, noted in-code, to
  revisit against `DATA_MODEL.md`'s schema when document-service's repo
  layer is built.
- **Every op that records a prior value is precondition-checked against
  current state before applying** (`SetBlockKind.From`,
  `SetBlockContent.Prev`, `SetTitle.From`, `DeleteBlock`/`MoveBlock`'s
  `After`) — this is the fix for open decision #4 (`DeleteBlock.After` was
  unchecked in the deleted draft), generalised to every op rather than
  patched as a one-off.
- `Content`'s three previously-`todo!()` methods (`remove_mark`,
  `normalise`, `marks_at`) are fully implemented: mark removal
  trims/splits/drops by range, `normalise` merges touching/overlapping
  same-kind marks and keeps canonical sort order, `marksAt` is a half-open
  range query.
- Full JSON (de)serialization on every type (`BlockKind`, `MarkKind`,
  `Content`, `Op` via `MarshalOp`/`UnmarshalOp`) — needed for the wasm
  boundary now, and it's the same shape `DATA_MODEL.md`'s JSONB columns
  will eventually use, so it's not wasm-only plumbing.

**Tests:** `testdata/document-core/marks.json` — 20 golden vectors for
`Content` (add/remove/query), run by Go's suite (`inline_test.go`).
Deterministic scenario tests for every `Op` variant including precondition
rejection (`page_test.go`), `History` undo/redo/eviction/atomic-rollback
(`history_test.go`), an invert-is-an-involution check plus a `rapid`
property test for the apply/invert round-trip law (`property_test.go`), a
`MarshalOp`/`UnmarshalOp` round-trip test (`operation_json_test.go`), and
benchmarks for `Page.Apply`/`History.Undo` (`benchmark_test.go`) —
baseline: ~770ns/op insert, ~430ns/op content-set, ~390ns/op undo+redo pair
on an Apple M4 Pro (`go test ./internal/... -bench=. -benchmem`).

**WASM wiring:** `cmd/wasm/main.go` exports `documentcoreNewPage`,
`documentcoreApplyOp`, `documentcoreInvertOp` via `syscall/js` —
JSON-string in, `{value, error}` JSON out. `scripts/build-wasm.sh` builds
`web/public/documentcore.wasm` and copies the matching `wasm_exec.js`
(both gitignored — build output). `web/src/document-core/`: `types.ts`
(wire types + literal builders, no logic), `wasm.ts` (the loader — works
identically under Vitest/Node and the real browser), `history.ts` (thin
undo/redo bookkeeping delegating every apply/invert to the wasm call).
`wasm.test.ts` proves the bridge end-to-end against the real compiled
binary (insert, a precondition-error rejection, invert round-trip,
History undo/redo) — 4 tests, not a re-test of document-core's own
behavior, which is already covered Go-side.

**Verified:** `gofmt`/`go vet`/`go build`/`go test -race`/benchmarks all
clean in `services/document-service` under the host `GOOS`/`GOARCH` *and*
`GOOS=js GOARCH=wasm`; `npm run build` (`tsc -b && vite build`) and
`npm test` (Vitest, via `pretest` → `build:wasm`) both clean in `web/`.
`cmd/wasm` needed a `stub.go` (`//go:build !(js && wasm)`, trivial
`main(){}`) so the host-target `go build ./...`/`go test ./...` don't
report it as unbuildable — the real implementation
(`main.go`/`json.go`) is tagged `//go:build js && wasm`.

**Next:** `auth-service` and `collaboration-service` still have no business
logic — Phase 1 continues with document-service's HTTP/gRPC handlers and
Postgres repo layer next, per `ROADMAP.md`'s Track 1 order.
