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

---

## 2026-08-26 — `PageService` implemented over real Postgres

Continuing document-service: `docs.pages` (title, LTREE tree position,
lifecycle) is now a working gRPC service, matching `docs/api/pages.md`
field-for-field.

**`internal/sortkey`** — fractional-index sort keys (`Between(prev, next)`),
so a page is reordered by writing one row, never renumbering siblings.
Two real bugs surfaced by testing before it was correct: appending a fixed
filler digit after matching *prev's* own digit (not just next's) could
silently regenerate the same key back (`Between("zi", "")` looping to
`"zi"`); and the exhaustion check fired on seeing a single `'0'` digit
instead of checking whether `next` had more room past it, wrongly
rejecting legitimate keys like `Between("", "0i")`. Both pinned as
regression tests; `TestPropertyBetweenOrdering` (rapid) checks the
ordering law generatively.

**`services/document-service/proto/document.proto`** — `PageService`,
matching `docs/api/pages.md` exactly. Generated via `protoc` directly
(`buf` isn't installed; for one proto file, `protoc` +
`protoc-gen-go`/`protoc-gen-go-grpc` — already available — is a fine
substitute). `scripts/gen-proto.sh` regenerates
`internal/genproto/documentv1`.

**`internal/pagerepo`** (sqlc-generated) + **`internal/pages`** (domain
model, `Repo` port + Postgres adapter, gRPC `Server`) implement 5 of 6
RPCs: `CreatePage`/`GetPage`/`ListPages`/`RenamePage`/`DeletePage`.
**`ReparentPage` is deliberately deferred** — it needs a transactional
subtree LTREE rewrite (`docs/api/pages.md` § Reparent: "every descendant's
path is rewritten in the same transaction"), which is a large enough unit
of work to be its own increment rather than rushed alongside the rest.
`DeletePage` is a simple soft delete for now too, not the full
cascade-to-subtree saga (`ARCHITECTURE.md` §5) — also deferred.

Notable decisions:
- `docs.pages.id` is generated **application-side** (`uuid.NewV7()`), not
  by a Postgres default — the row's `path` needs the id as its own final
  LTREE label, so it must be known before the `INSERT`, not after.
- LTREE labels can't contain hyphens; a page's path label is `"p" +
  hex(uuid without dashes)` — matches the convention `docs/api/pages.md`'s
  own example already showed (`"path": "p018f2b1c..."`).
- `pages.PageID` is its own type, not `documentcore.PageID` — pages
  (metadata) and documentcore (block content) are separate bounded
  contexts for now; sharing a type would couple them for no current
  benefit (extraction is for the *second* consumer that actually needs it).
- `api-gateway` doesn't exist in this repo's scope — `document-service`
  reads `actor-id` directly off gRPC metadata as a stand-in, documented in
  `docs/api/pages.md` as temporary scaffolding, not the real trust
  boundary.
- Migrations live at `internal/migrate/migrations/` (not a top-level
  `migrations/`) so they can be `//go:embed`ded — the same schema a real
  deployment applies at startup (`migrate.Up`) is what integration tests
  run against.

**Verified three ways:** `go test ./internal/pages/...` unit-level;
`go test -tags=integration ./internal/pages/...` against real Postgres 18
via testcontainers-go (4 tests: CRUD round trip, sibling ordering by
sort_key, nested LTREE path on a child page, anchor-under-wrong-parent
rejection) — all passing; and a live manual smoke test (`docker run
postgres:18-alpine`, `go run ./cmd`, `grpcurl`) exercising
Create→List→Rename→Delete→Get end-to-end, including the missing-`actor-id`
`UNAUTHENTICATED` rejection and the post-delete `NOT_FOUND`. `golangci-lint
run ./...` clean (0 issues) — `golangci-lint`/`staticcheck`/`govulncheck`
needed reinstalling via `go install ...@latest` since the previously
installed copies were built with an older Go than the local 1.26.1
toolchain requires (a correction to an earlier commit's claim that pinning
`go.mod`'s directive to 1.23 had fixed this — it hadn't; these tools check
the actual installed toolchain, not the `go` directive).
`govulncheck` separately flags 1.26.1's own stdlib CVEs (fixed in 1.26.2)
— a local Go upgrade to consider, unrelated to this repo's code.

---

## 2026-08-26 — `ReparentPage` implemented: the transactional subtree rewrite

Closed the one RPC deferred from the previous entry. `internal/pages.Reparent`
runs in a single `pgx.Tx`:

1. Resolve the page being moved and (if a target parent is named) that
   parent — reject with `ErrCycle` (`FAILED_PRECONDITION`) if the target is
   the page itself or one of its own descendants (checked via LTREE path
   prefix comparison in Go, not a special query).
2. Compute the new `sort_key` via the same `nextSortKey` logic `Create`
   uses, now parameterized with an `excludeID` so a page being reordered
   among its current siblings doesn't find itself as its own neighbor.
3. Update the page's own row (`parent_id`, `path`, `sort_key`).
4. If the path actually changed (i.e. the parent changed — reordering
   alone doesn't touch it), rewrite every descendant's path in the same
   transaction (`RewriteDescendantPaths`, using LTREE's `subpath()` to
   swap the old ancestor prefix for the new one while preserving each
   descendant's own trailing labels) — docs/api/pages.md's "a concurrent
   reader sees all old paths or all new ones, never a mixture."

`ReparentPageRequest.parent_id`'s three-way optional (absent = leave
alone, present-empty = promote to root, present-set = new parent) is
modeled as `ParentChange{Change bool; ParentID *PageID}` rather than a
bare `*PageID`, since a plain pointer can't distinguish "leave alone" from
"promote to root" (both would otherwise look like `nil`).

**Verified:** 4 new integration tests against real Postgres 18 (move to a
new parent + descendant cascade, promote to root, reorder within the same
parent, both cycle-rejection cases) — all passing alongside the 4 from the
previous entry (8 total). Live smoke test via `grpcurl`: reparenting a page
under its own child correctly returns `FAILED_PRECONDITION`; promoting a
page to root correctly clears its parent. `golangci-lint run ./...` still
0 issues.

---

## 2026-08-26 — `auth-service` implemented: the whole `AuthService` contract

A repo already existed for this: `docs/architecture/lld/auth-service.md`,
written for the original Rust track, turned out to be extremely
normative — RPC names, the claims shape, the full error-mapping table, the
rotation state machine, the bootstrap race, every named invariant in its
§9 algorithms table. None of that needed re-deriving; only the language
changed. `docs/api/auth.md` fills the gaps it explicitly left open (exact
message shapes, token lifetimes, Argon2id parameters, lockout thresholds,
the cursor-color palette) grounded in OWASP/RFC citations for the
security parameters and in what real collaborative editors (Notion,
Google Docs) do for the product-facing ones.

**`internal/domain`** — `Email`, `Password` (redacted `String()`/`GoString()`
so an accidental `%v` on an enclosing struct can't leak it — the LLD's
"single highest-consequence mistake available in this service"),
`PasswordHash`, `UserID`, `Jti`, `DisplayName`, `CursorColor` +
`AssignCursorColor` (deterministic on user id, from a fixed 8-color
palette).

**`internal/passwordhash`** — Argon2id, PHC-string encoded, constant-time
`Verify` (`crypto/subtle`), and a `Dummy` — a real hash of a fixed string,
computed once, verified against on every "unknown email" path so the
timing is indistinguishable from a genuine wrong-password rejection. A
timing test (named in the LLD, `TestUnknownEmailAndWrongPasswordTakeSimilarTime`)
samples both paths and asserts the medians land within tolerance — it
measured a 1.00 ratio on the first run.

**`internal/keys`** — RS256 keypair + JWKS (RFC 7517, hand-encoded — the
shape is a standard, not a decision). **`internal/sessions`** — `Claims`
issuance/verification with `Validation` constructed explicitly (`jwt.WithValidMethods`
pinning RS256, so the token's own header never gets a vote — the classic
JWT alg-confusion hole; a test forges an `alg: none` token and confirms
it's rejected) — and the rotation state machine: hash the presented
token, and either it's unrecognized (nothing to revoke), expired
(nothing to revoke), reused (**revoke the entire chain** — found by a
recursive CTE walking `parent_id` up to the root, then a second one
walking back down to every descendant), or a legitimate rotation (revoke
the old row, insert a new one, same transaction).

**`internal/lockout`** (Redis, per-account, not per-IP — "an attacker
distributing attempts across many IPs defeats edge limiting entirely,"
LLD §12) and **`internal/blocklist`** (Redis, `jwt:blocklist:{jti}`) are
new — the LLD calls both out as this service's job, not the (nonexistent)
gateway's.

**`internal/authservice`** ties it together — the only layer that opens
multi-table transactions (registration's user-insert + first-refresh-token
insert; rotation's revoke-old + insert-new) and enforces the
constant-time/lockout ordering. `Register` is also the bootstrap claim
(LLD §7): a `pg_advisory_xact_lock`-guarded count-then-insert, so two
concurrent claims on a fresh instance can't both create an administrator —
verified with 10 concurrent goroutines racing to register, exactly one
winning.

**Verified:** unit tests for domain/passwordhash/keys/sessions (incl. the
timing test and the alg-confusion rejection); 9 integration tests against
real Postgres 18 + Redis via testcontainers-go (bootstrap + the concurrent
race, unknown-email/wrong-password/lockout, rotation happy path, the LLD's
named `reuse_of_a_rotated_token_revokes_the_whole_family` — rotate 3
times, replay token #1, assert all 4 dead — revoke/revoke-all with
session isolation); a live `grpcurl` smoke test against real Postgres +
Redis exercising Register→Authenticate→Refresh→replay end-to-end,
confirming the two `UNAUTHENTICATED` credential messages really are
byte-identical. `golangci-lint run ./...` clean.

**Deferred** (documented in `docs/api/auth.md` § 3, not silently
dropped): per-IP rate limiting (gateway's job, no gateway exists here);
JWKS key-rotation tooling (the `KeyStore` interface supports multiple
verification keys, nothing drives an actual rotation yet — one signing
key for the process lifetime); the `api-gateway`/cookie/CSRF boundary
(refresh tokens return directly in `TokenPair`, not an `HttpOnly` cookie).

---

## 2026-08-26 — `/security-review` run; one real finding, fixed

Reviewed auth-service (the mandated boundary) and, incidentally,
document-service's actor-id stand-in while checking RBAC-adjacent items.

**Real finding, not hypothetical:** document-service's `GetPage`/
`ListPages`/`RenamePage`/`DeletePage`/`ReparentPage` never checked a
page's `created_by` against the calling actor at all — any caller could
read, rename, delete, or reparent any page, and `ListPages` returned
every page in the system regardless of owner. Directly violated user
story A-04. Fixed by scoping `created_by` into the `WHERE` clause of every
affected query (not an application-level check after fetching) —
see `docs/api/pages.md`'s new section and the two new integration tests
(`TestPagesAreScopedToTheirOwner`, `TestCreateCannotNestUnderAnotherActorsPage`).

**Minor test-coverage gap, fixed:** the alg-confusion test only forged an
`alg: none` token, not the more specific HS256-signed-with-the-RSA-public-
key-bytes attack the LLD actually names. Added
`TestVerifyRejectsHS256SignedWithThePublicKeyBytes` — confirms
`jwt.WithValidMethods` rejects it regardless of keyfunc behavior.

**Checked and already correct, no changes needed:** Argon2id-only password
storage, RS256 + explicit claim validation, constant-time password/dummy-hash
comparison (`crypto/subtle`), refresh-token single-use + family revocation,
blocklist TTL matching actual remaining token lifetime, parameterized SQL
throughout (no string-built queries anywhere), LTREE labels derived only
from internally-generated UUIDs (never from user text, so no path-injection
surface), no email/password/token ever logged.

**Confirmed, already documented, not fixed (correctly out of scope for
this repo):** no per-IP/distributed-attempt rate limiting (gateway's job,
no gateway exists); JWKS key-rotation tooling; the api-gateway/cookie/CSRF
boundary. These remain real gaps for a production deployment, not silent
ones — `docs/api/auth.md` §3 already named them before this review ran.

---

## 2026-08-26 — document-service's remaining Phase 1 pieces closed out

**Delete now cascades to descendants** — one transaction, the same LTREE
`path <@ ...` pattern `Reparent`'s descendant rewrite already uses, applied
to `lifecycle_state` instead of `path`. Idempotent over an
already-cascaded subtree; an unrelated sibling untouched.

**The "outbox" item from the last entry was a scoping mistake, corrected
here, not implemented:** re-reading `ARCHITECTURE.md` §5, the full delete
saga coordinates with `search-service`, `diagnostics-service`, and
`history-service` for a final hard-delete — **none of which exist in this
repo** (`ADR-011` scopes this repo to `document-service`/`auth-service`/
`collaboration-service` only). A saga can't coordinate with participants
that don't exist, so it isn't attempted. `lifecycle_state` stays
`'deleting'` forever in this repo — `docs/api/pages.md` now says this
plainly instead of implying it's a pending TODO. This also means a
generic outbox-publishing mechanism isn't needed yet either — there's
currently nothing in this repo's scope for document-service to publish
*to* (no other service subscribes to its events). It'll get built when
`collaboration-service` actually needs to consume something from it, not
before.

Document-service's Phase 1 is now complete for this repo's scope: all six
`PageService` RPCs, ownership enforcement, cascading delete.

**Next:** `collaboration-service` has no business logic yet — the
remaining major piece for the MVP finish line ("🏁 log in, write a page,
edit live with someone"). Starting with its core data structures (a rope
for the live document text, character-granular ops, anchors) as pure
logic first, same order document-core was built in — persistence and the
WebSocket/session layer come after that's solid. Per `ROADMAP.md`'s Track
1 order.

---

## 2026-08-26 — `collaboration-service`: the rope, first piece of the CRDT core

`internal/rope` — the live-editing-session text representation RFC-001 §2
calls the "CRDT working format" (as opposed to document-service's flat
`spans` JSONB, the storage/wire side of that split). Immutable/persistent
(`Insert`/`Delete` return a new `Rope`, structural sharing with the old
one) — the idiomatic fit for Go here, and it makes every op trivially
safe to reason about across goroutines with no lock, unlike a mutate-in-
place design would be.

Balanced via a scapegoat-tree-style amortized rebuild: `concat` checks the
resulting depth against a `2*log2(leaf count)`-ish threshold and rebuilds
from the leaves when exceeded, rather than incremental rotations — simpler
to get right, and `TestManyInsertsStayBalanced` (2000 sequential
appends-at-end — the exact pattern that degrades an unbalanced tree to a
linked list) confirms depth stays near-logarithmic instead of linear.

**Verified two ways beyond example tests:** `TestPropertyMatchesNaiveStringReference`
(rapid, 2000 iterations) is the standard way to test a rope — apply the
same random sequence of inserts/deletes to a `Rope` and to a plain Go
string, assert they always match; this is what would actually catch a
split/concat/rebalance bug, not a handful of hand-picked examples. And
benchmarks (`docs/porting/BENCHMARKS.md`) measuring **~700x** faster than
naive string slicing for a middle-insert on a ~900KB document — the
concrete reason a rope exists over `string`, not just an assertion that
it should be faster.

`golangci-lint` clean.

---

## 2026-08-26 — `collaboration-service`: Anchors, and the rope+identity integration

Corrected the stated order from the last entry: character-granular ops
reference `Anchor`/`AnchorRange` directly in their own field types
(RFC-002 §2), so Anchors had to come before them, not after.

**`internal/anchor`** — RFC-001 §9's stable-position scheme: `ItemID`
(Lamport `{actor, counter}`, permanent once assigned), `Bias`
(`Before`/`After`, disambiguating which side of an item a position sits
on), `Anchor`, `AnchorRange`, and `Resolved` (`At`/`Detached`/`Unknown`).
`Log` tracks every character ever inserted, live or tombstoned, and
answers "what live offset is this `ItemID` at now" — deliberately
correctness-first (O(n) per operation, a full index rebuild on mutation),
with the exact, narrow optimization path named in its own doc comment
(an order-statistics/Fenwick-tree rank query) for when that's actually
measured to matter, not before (`.agents/agents.md` §2's "ship minimal").

Scoped to this repo's actual architecture, not full Yjs/Automerge-style
peer-to-peer merge: `collaboration-service` is one doc-actor per page
(`ARCHITECTURE.md` — "one document, one owner, at any time"), so every
concurrent client's op is applied by a single serializing process, not
merged across replicas that never talked to each other. Anchors here only
need to resolve against *current* state, not reconcile divergent
histories — a materially smaller problem, and the package doc comment
says so explicitly rather than silently under-delivering against what
"CRDT" usually implies.

**`internal/doctext`** integrates `rope` (content) with `anchor.Log`
(identity) into what a live session actually edits: rune-offset
`InsertAt`/`DeleteRange`, `Resolve`. Rune offsets, not byte offsets — a
cursor position is conceptually "the Nth character," and a byte offset
could split one; the rope stays byte-indexed internally (matching
document-service's stored spans) with a documented, deliberately-O(n)-for-now
conversion at the boundary, same tradeoff and same reasoning as `Log`'s.

**Verified:** a property test for `Log` (rapid, 3000 iterations, against a
plain reference model — same differential-testing approach the rope's
own tests use) and end-to-end tests proving the actual point of this
subsystem: an anchor composed against the document *before* a concurrent
edit lands still resolves to the right character *after* that edit lands,
and a subsequent insert positioned via that resolved offset lands in the
right place. `golangci-lint` clean throughout.

**Next:** character-granular `Op` (`InsertText`/`DeleteText`/`SetMark`,
RFC-002 §2's Phase-3 tier) as a real Op type with `Invert()`, built on top
of `doctext.Text` — mirroring `document-core`'s `Op`/`Invert` shape but
character-granular. Then WAL framing and `collab.ops` persistence, then
the session/WebSocket layer.
