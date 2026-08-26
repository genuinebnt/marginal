# Marginal — Go + TypeScript Build Rules

## What this file is

**Build rules for the Go+TS MVP track.** `CLAUDE.md` carries the project
facts — stack, services, current phase — and is loaded every session. Do not
duplicate it here; when the two disagree, `CLAUDE.md` and the ADRs win.

**This is not mentor-mode.** Per `ADR-011`, Claude writes the Go and
TypeScript implementation directly — services, tests, docs, the lot. The
old scaffold-and-wait loop that governed the Rust attempt lives at
`docs/rust/agents.md` now, for the future Rust-port repo. Read
`docs/porting/PROGRESS.md` before doing anything else in a new or
compacted session — it is the record of what's actually done, not what a
stale summary implies.

**Primary objective for this track: a genuinely complete, demo-quality
Track 1 MVP** (Documents → Auth → Collaboration) — interview-showcase
quality, not a token skeleton, built in Go + TypeScript, fully tested and
deployable. Rust depth (ADR-002) is suspended, not abandoned; it resumes in
the future Rust-port repo.

---

## Core Principles

### 1. Business logic lives in Go once; TypeScript is views + a JSON bridge

Per `ADR-011`'s addendum: **all document-core logic is Go**, compiled to
`GOOS=js GOARCH=wasm` for the browser (`services/document-service/cmd/wasm`)
and used server-side as an ordinary package. TypeScript never reimplements
it — `web/src/document-core/` is wire types (`types.ts`), the wasm loader
(`wasm.ts`), and thin bookkeeping (`history.ts`'s undo/redo stacks — two
arrays, no semantics of its own). If a view needs a document-core behavior
that doesn't exist yet, the fix is a new exported Go function compiled into
the wasm module, never a parallel TS implementation.

Within Go, write idiomatic Go — not Rust-shaped Go:

- Errors are `(T, error)` returns, not a hand-rolled `Result[T]`.
- Interfaces are small, declared at their point of use (consumer side),
  colocated per `CLOUD_PORTABILITY.md`'s port-and-adapter convention — not
  one interface mirroring one future Rust trait.
- No abstraction exists solely because "Rust will need this later." If Go
  doesn't need a seam, don't build one.

Portability for the future Rust port comes from three things instead:
**one wasm boundary already in the right shape** (recompile
`internal/documentcore` to `wasm32-unknown-unknown` instead of `js/wasm` —
the JSON-in/JSON-out contract barely changes), **golden JSON test vectors**
under `testdata/` (§ Testing Philosophy), and **documentation** (§4's
doc-comment and `PORT-NOTE` conventions).

### 2. Ship minimal, refactor on friction — carried over unchanged

Same governing principle as the Rust track, restated for this one: start
with the simplest struct/type and the most obvious function signature.
Let real usage reveal faults. Refactor on friction, not on a schedule.
Embrace churn — it's expected while the system is small. A version that
compiles and passes tests beats a "perfect" one that's never finished.

### 3. Feature depth, not surface area, is the complexity budget

Route count, handler count, and query count do **not** make a service
complex — they just make it big. What makes a service hard to port later is
depth: a feature pushed past "solid and complete" into speculative
sophistication a Track-1 demo doesn't need (a hand-rolled search index where
Track 1 doesn't need one yet, a generalized plugin surface, premature
extensibility). Each of the three MVP services is scaffolded as its own
standalone code area, since they share no code and add no cross-service
cognitive load — but each feature inside them gets assessed the way an
engineer scoping a real deadline would: build it fully and solidly, then
stop. Don't stub for simplicity's sake either — "demo-quality" means the
golden path and its real edge cases work, not that corners are cut.

---

## Testing Philosophy — behavior, not language

The goal: a Rust port later can reuse the *test cases*, not the test code.

- **Golden test vectors** (`testdata/<module>/*.json`) encode input →
  expected-output as data, not code, and are consumed by Go's test suite
  today, the Rust port's later. Write these for anything with pure logic
  and no I/O — `document-core`'s `Page`/`Op`/`Content`/`History` is the
  first and clearest case. TypeScript does **not** get its own copy of
  these tests — it has no reimplementation to test against them; see §1.
- **Property-based tests** in Go (`pgregory.net/rapid`) for algebraic laws:
  `apply(invert(op), apply(op, page)) == page` for every op, every page
  (`RFC-002` §3's invertibility law).
- **Contract-level tests** for anything crossing a service boundary: assert
  against the `.proto`/OpenAPI contract (request in, response out), not
  against internal implementation details. These port almost unchanged —
  a Rust gRPC client hitting the same contract is testing the same thing.
- **WASM bridge integration tests** (`web/src/document-core/wasm.test.ts`,
  Vitest, runs against the real compiled `.wasm`) prove the TS↔Go boundary
  works — JSON marshaling, error propagation — not the document-core
  behavior itself, which is already pinned Go-side. Don't duplicate a
  behavior assertion here that a Go test already makes.
- **Unit tests** for everything else, Go table-driven style. `cmd/wasm` has
  none of its own — it's a thin `syscall/js` adapter with no branching
  logic worth a unit test; `go vet`/`go build` under `GOOS=js GOARCH=wasm`
  is the check. (Note: `go build ./...`/`go test ./...` from the module
  root, run under the host `GOOS`/`GOARCH`, correctly report `cmd/wasm` as
  unbuildable — that's expected, not a regression; use
  `scripts/build-wasm.sh` or `GOOS=js GOARCH=wasm go vet ./cmd/wasm/...`.)
- **Never mock infrastructure.** `testcontainers-go` against real Postgres/
  NATS, same rule the Rust track had.

## Documentation Convention

- **Doc comments cite the spec, not the code.** A doc comment on an
  exported Go func/type or TS export should say *why*, referencing the RFC/
  ADR/DATA_MODEL section it implements — not restate the signature.
- **`// PORT-NOTE:` flags GC-dependent choices.** Anywhere a Go/TS
  implementation leans on garbage collection to avoid a decision Rust will
  force (an allocation pattern, a lifetime, an ownership transfer that's
  free here and won't be there), leave a `// PORT-NOTE:` comment saying
  what the choice was and why. This is the concrete mechanism for "don't
  let this be silently lost by the time the Rust port starts."
- **`docs/porting/PROGRESS.md` is a log, not a spec.** Append short entries
  as work lands — decisions made, why, what's next. Don't let it go stale;
  don't let it balloon into a second TASKS.md either.

## Concurrency & Security Tooling

- `go test -race` always, in every CI run and locally before a commit that
  touches goroutines or shared state.
- `go.uber.org/goleak` in any test suite that spawns goroutines, to catch
  leaks.
- Native fuzzing (`go test -fuzz`) for anything parsing untrusted input
  (paste sanitization, request bodies) — mirrors the Rust track's
  `cargo-fuzz` requirement for the same surfaces.
- `golangci-lint` (staticcheck, errcheck, `gosec`) in CI — `gosec` is the
  concrete stand-in for the old "Vulnerabilities" review step: SQL
  injection (parameterized queries only, `sqlc`-generated), path traversal,
  timing attacks on auth comparisons, unchecked errors.
- `govulncheck` against dependencies in CI.
- Apply `/code-review`, a security-review pass, and `/project:simplify`'s
  spirit (idiom + simplicity) at the end of each vertical slice, not just
  at PR time — this is a standing practice for this track, not an
  occasional ask.

## Git Commits

Commit per unit of work, with a message that explains *why*, not just what
— small enough that `git log --oneline` reads like normal incremental
development and a specific commit can be checked out to see how the
codebase looked at that point. Never squash the whole session into one
commit.

## Reference High-Quality Patterns

When writing Go or TypeScript, look to established, widely-used patterns
before inventing one — reduces bugs, and gives the future Rust port a
well-known shape to compare against. Go: standard project layout
conventions, `sqlc`'s generated-code pattern, Google/Uber Go style guides.
TypeScript: the existing stack table's choices (Vite/React/Radix) plus
`fast-check`'s own docs for property-test idioms.

---

## Go/TS Patterns to Apply

| Pattern | When to use |
|---|---|
| Errors as values (`(T, error)`) | Every fallible Go function — never panic for expected failure |
| Small interfaces at point of use | One interface per external dependency, declared where it's consumed (`CLOUD_PORTABILITY.md`'s ports-and-adapters) |
| Table-driven tests | Go unit tests with multiple input/output cases |
| Context propagation (`context.Context`) | Every Go call that can block on I/O or needs cancellation/timeout |
| Discriminated unions + exhaustiveness | TS sum types (`Op`, `BlockKind`) — a `switch` with a `never` default catches missing cases at compile time |
| `sqlc`-generated queries | All Postgres access — compile-time-checked, closest analogue to sqlx |
| Repository interface + impl, same file/package | Mirrors the Rust track's "trait + impl in the same file" rule |

---

## Situational Response Table

| Situation | What You Do |
|---|---|
| Starting a new/compacted session | Read `docs/porting/PROGRESS.md` first — don't re-derive or assume prior decisions |
| Implementing a new module/service | Write it directly: types, logic, tests, doc comments, `PORT-NOTE`s where relevant |
| A feature could go deeper | Assess against §3 — build it solid and complete, stop before speculative sophistication |
| Finishing a vertical slice | Apply code review + security review + simplify passes before moving on |
| A design question isn't answered by the RFCs/DATA_MODEL | Check `docs/porting/OPEN_QUESTIONS.md`; if genuinely new, ask, don't guess |
| Something crosses a service boundary | Test it against the contract (`.proto`/OpenAPI), not implementation details |
