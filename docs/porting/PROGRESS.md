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

---

## 2026-08-26 — `collaboration-service`: character-granular `Op`

**`internal/ops`** — `InsertText`/`DeleteText`, applied against a
`doctext.Text` via `Apply(text, op) (inverse Op, error)`. A real structural
difference from `document-core`'s `Op`, documented in the package comment
rather than treated as an inconsistency to fix: `document-core`'s
`Op.Invert()` is a pure function of the op's own fields because block/page
ids are caller-supplied and known before applying, but `InsertText`'s
inverse is a `DeleteText` naming the `ItemID`s the insert itself creates —
those don't exist until `Apply` runs, so the inverse is a return value of
applying, not a standalone method.

`DeleteText.Range` anchors to the deleted items themselves (`Start` biased
`Before` the first, `End` biased `After` the last), not their surviving
neighbors — chosen so that re-resolving `Range.Start` right after the
delete comes back `Detached` at exactly the gap the deletion left, which is
exactly where the delete's own inverse (an `InsertText`) needs to land.
Verified this holds under a concurrent edit landing between the delete and
its undo (`TestApplyInverseIsInvolutionAcrossConcurrentEdit`): the undo
lands next to the anchored gap, not a stale numeric offset.

`SetMark` deferred again here, same reason as before — it needs anchor-based
mark storage on live text, which `doctext.Text` doesn't have yet.

**Verified:** deterministic tests (insert at start/mid-document, empty-input
no-ops on both variants, delete round-trip, unknown-anchor rejection) plus
`TestPropertyApplyInverseRoundTrips` (rapid) — the invertibility law at this
package's granularity, `apply(inverse, apply(op, text)) == text`, driven
over random insert/delete sequences with anchors drawn from (and sometimes
already-tombstoned) ids the text has actually seen. `gofmt`/`go vet`/
`go build`/`go test -race ./...`/`golangci-lint run ./...` all clean.
`govulncheck` still flags only the pre-existing Go 1.26.1 stdlib CVEs
(fixed in 1.26.2, unrelated to this repo's code — noted in the previous
`auth-service` entry too).

**Next:** WAL framing and `collab.ops` persistence for these ops, then the
session/WebSocket layer — the last major piece before the MVP's 🏁.

---

## 2026-08-26 — `collaboration-service`: the permanent wire format + local WAL

Two infrastructure pieces, built and tested standalone before anything
wires them to a live session — same order as `rope`→`anchor`→`doctext`.

**`internal/oplog`** ports RFC-002 §4's `LoggedOp` field-for-field
(`id`/`version`/`actor`/`clock`/`op`), plus the two DATA_MODEL.md columns
that must exist from op #1 (`actor_kind`, `undo_group`) and `page_id`.
`New` is the one place a fresh `LoggedOp` gets constructed, so the
versioning rule (RFC-002 §4 rule 1: a `version` field stamped from day
one) can't be forgotten by a caller. `Marshal`/`Unmarshal` box the `Op`
field through `ops.MarshalOp`/`UnmarshalOp` (a `"type"`-tagged envelope,
same convention as `document-core`'s `operation_json.go` — added to
`internal/ops` this entry too) since `encoding/json` can't dispatch an
interface field to a concrete type on its own. `Unmarshal` rejects any
`Version` it doesn't recognize by name (`ErrUnsupportedVersion`) rather
than guessing — RFC-002 §4 rule 4's "old versions decode forever" implies
a decoder must know exactly which versions it *does* support, not silently
best-effort a mismatch.

**`internal/wal`** is the local durability layer ARCHITECTURE.md's
collaboration-service sequence diagram names: a client's op is acknowledged
after this log's `fsync`, not after Postgres. Framing is byte-for-byte
RFC-002 §6/§8's `[4-byte len][record][4-byte crc32]` — deliberately the
same on the WAL as the wire, one encoder/decoder/test-set for both, though
only the WAL side is built this entry. `Recover` is **a recovering parser,
not a strict one** (RFC-002 §6's explicit framing): a torn tail — a
partial record an unclean shutdown left behind — resyncs cleanly and
stops, returning `validUpTo` for `Truncate` to cut the garbage off before a
fresh `Writer` resumes; a checksum mismatch on an otherwise
complete-and-correctly-sized frame is treated as real corruption
(`ErrChecksum`), not a torn write — those are different failure modes with
different correct responses, and the code distinguishes them rather than
lumping "anything wrong" into one basket.

**Deliberately not built this entry, and said so in the package doc
rather than implied:** RFC-002 §6's 64MB segment-rotation-then-delete-
once-flushed scheme (one `Writer` is one segment/one file for now — a
demo session's op log isn't expected to approach that size) and §8's
`rkyv` zero-copy decode (Rust-specific serialization machinery; this
package only owns the durability framing, not payload encoding, and
Go's own JSON payload is what's actually flowing through it).

**Verified:** `oplog` — round-trip (`Marshal`→`Unmarshal` preserves every
field including a nil vs. set `UndoGroup`), version-rejection, invalid-
actor-kind rejection. `wal` — round-trip replay, missing-file-is-empty,
propagating a replay callback's own error, and the two hardest cases
named above: a **simulated torn tail** (a length prefix claiming 256 bytes
followed by 17, mimicking exactly what a `SIGKILL` mid-write leaves) resyncs
and truncates cleanly, and a flipped body byte on an otherwise-intact frame
is reported as `ErrChecksum`, not silently accepted or misclassified as
torn. `TestPropertyRecoverReplaysExactlyWhatWasAppended` (rapid) drives
variable-length (including zero-length) records through real disk I/O and
checks byte-for-byte replay. `gofmt`/`go vet`/`go build`/`go test
-race ./...`/`golangci-lint run ./...` all clean across the whole module;
`govulncheck` unchanged from the prior entry (same pre-existing Go 1.26.1
stdlib CVEs, nothing new from this code).

**Next:** wire `oplog`+`wal` to real Postgres persistence — `collab.ops`
migration + sqlc repo (append-only, matching DATA_MODEL.md's schema
exactly) and the `collab` outbox table, plus the batched flush loop
(WAL → Postgres) that RFC-002 §7 calls out as what makes the write volume
survivable. Then the session/WebSocket layer that actually drives a live
edit through rope → ops → oplog → WAL → (batched) Postgres — the MVP's
final major piece.

---

## 2026-08-26 — `collaboration-service`: `collab.ops` persisted over real Postgres

Closed the "Next" from the last entry: `oplog.LoggedOp` now has a durable,
queryable Postgres home, matching DATA_MODEL.md's `collab.ops` schema
field-for-field (`internal/migrate/migrations/00001_collab_ops.sql`) —
append-only, no `FOREIGN KEY` on `page_id` or `actor_id` (cross-schema,
and agent/plugin/system actors have no `auth.users` row, per that doc's
own notes), `actor_kind`/`undo_group` present from row #1. `collab.outbox`
is created in the same migration — DATA_MODEL.md's "one outbox per
publishing service," collaboration-service's own.

**`internal/opstore`** is the Postgres port (`sqlc`-generated
`internal/collabrepo`, small `Repo` interface at its point of use, same
shape as document-service's `pages.Repo`). `Append` writes the op row and
its outbox event in one transaction, and is **idempotent on `LoggedOp.ID`**
via `ON CONFLICT (id) DO NOTHING` — RFC-002 §4 rule 5 names `id` as
exactly the dedup key ops need because the flush loop that will call this
(still the "Next," below) delivers at-least-once: a crash between the
Postgres commit and the WAL's own bookkeeping can redeliver an already-
flushed op on restart, and that redelivery must be a silent no-op, not a
duplicate row or a second outbox event.

`ops.MarshalOp`/`TypeName` (added this entry, refactored out of the type
switch `MarshalOp` already had) is the one place an op variant's name is
spelled — both the JSON envelope's `"type"` field and `collab.ops.kind`
read the same string, so there's no second naming scheme that could drift
from the first.

**Verified:** unit-level (`oplog`, `ops`) unchanged and still passing; 4
integration tests against real Postgres 18 via testcontainers-go —
append+list round-trip (including the outbox row landing in the same
transaction), the idempotent-duplicate-append case (asserting exactly one
`collab.ops` row *and* exactly one `collab.outbox` row survive a repeated
`Append`), oldest-first ordering (cross-checked against UUIDv7's own
chronological sort, not just `created_at`), and per-page scoping.
`gofmt`/`go vet`/`go build`/`go test -race`/`golangci-lint run`
(both with and without `-tags=integration`) all clean; `govulncheck`
unchanged in substance (still only the pre-existing Go 1.26.1 stdlib
CVEs — one more call-graph trace than last entry's count, same underlying
toolchain issue, nothing new from this code).

**Next:** the batched WAL→Postgres flush loop (RFC-002 §7 — batching is
what makes 30k ops/second survivable, not a micro-optimisation; Go's
idiomatic fit is a bounded buffered channel plus a flush goroutine, not
`crossbeam::ArrayQueue`/a hand-written `Stream`, which are Rust-specific).
Then the session/WebSocket layer that actually drives a live edit through
rope → ops → oplog → WAL → (batched) Postgres — the MVP's final major
piece.

---

## 2026-08-26 — `collaboration-service`: the batched WAL→Postgres flush loop

Closed RFC-002 §7. `opstore.Repo` grew `AppendBatch` — the same
transactional, idempotent-on-id semantics `Append` already had, but for a
whole slice of `LoggedOp` at once, as **two pipelined round trips total**
regardless of batch size (one `pgx.Batch` for the op rows via a new
`sqlc :batchone`-annotated `InsertOpBatch` query, then a second batch for
outbox events — but only for whichever ops the first batch's own results
say were actually new, so a redelivered duplicate inside a batch still
costs zero extra writes). `ON CONFLICT DO NOTHING` per-statement inside a
`pgx.Batch` behaves exactly like the single-statement case
(`pgx.ErrNoRows` on `Scan` for a skipped row) — confirmed by a new
integration test that redelivers one already-flushed op mixed into an
otherwise-fresh batch and checks both the op count and the outbox count
land on exactly the two genuinely-new ops.

**`internal/flush`** is RFC-002 §7's drain loop — the piece that actually
calls `AppendBatch`. Go's idiomatic fit for the RFC's
`crossbeam::ArrayQueue` + hand-written `Stream`/`Waker` is a bounded
buffered channel plus one goroutine (`Loop.run`): `Enqueue` blocks under
backpressure rather than dropping, a batch flushes on whichever comes
first — `BatchSize` ops accumulated (default 20, RFC-002 §7's own worked
example) or `Interval` elapsed since the last flush (default 200ms, a
staleness bound against Postgres, not client-visible latency, since the
client was already acknowledged at the WAL fsync) — and a failing flush
retries with capped exponential backoff up to `MaxAttempts` (default 5)
before surfacing to an injectable `OnError` and moving on, rather than
wedging the whole loop retrying one batch forever. No Rust-specific
zero-copy/`Stream` machinery ported — Go's scheduler and channels already
do that job.

**A real concurrency bug caught before it shipped:** `Enqueue`'s original
`select` raced `l.stopped` against `l.pending <- op` as equally-ready
cases — once `Stop()` had already closed `stopped` but the buffered
channel still had room, `select`'s pseudo-random tie-break meant an
`Enqueue` call after `Stop()` could non-deterministically report success
for an op that would then sit in the channel forever, since `run()` had
already exited. Fixed with an upfront non-blocking `select` on `l.stopped`
before the blocking one — makes "already stopped" checks deterministic
without changing the (inherently racy, and correct to be racy) behavior of
an `Enqueue` truly concurrent with `Stop()` itself. `TestEnqueueAfterStopReturnsErrStopped`
would have been flaky roughly half the time without this fix; it's stable
across `-race -count=15` now.

**Verified:** `opstore` — 3 new integration tests (`AppendBatch` inserts
all + counts only-new, skips an already-flushed op mixed into a batch,
no-ops on an empty slice), alongside the 4 from the prior entry, all
against real Postgres 18. `flush` — unit tests against a small
hand-written fake behind `opstore.Repo` (not "mocking infrastructure";
this is `Loop`'s own scheduling/retry logic, independent of whether the
repo underneath is Postgres) covering batch-size flush, interval flush,
`Stop` draining buffered ops before returning, `Enqueue` after `Stop`,
`Enqueue` respecting the caller's own `ctx` cancellation, retry-then-
succeed, give-up-then-`OnError`, and `ctx` cancellation behaving like
`Stop`. `TestConcurrentEnqueueLosesNothing` (20 goroutines × 15 ops each,
`-race`, 10 repeats) is this package's actual correctness property: every
enqueued op reaches `AppendBatch` exactly once, never lost or duplicated,
regardless of how batching/retry slices the timing. `go.uber.org/goleak`
wired via `TestMain` — this is the repo's first long-lived-goroutine
component (`Loop.Start`/`Stop`), and the first place `.agents/agents.md`'s
"concurrent code needs `-race`, `goleak`" rule actually applied. `gofmt`/
`go vet`/`go build`/`go test -race`/`golangci-lint run` (with and without
`-tags=integration`) all clean; `govulncheck` unchanged in substance.

**Next:** the session/WebSocket layer — the MVP's final major piece,
wiring together `rope`/`ops`/`oplog`/`wal`/`flush` into an actual live
per-page editing session that a client connects to.

---

## 2026-08-26 — `collaboration-service`: the live session core

**`internal/session`** is ARCHITECTURE.md §4's "Request Flow — Live
Editing," minus the transport: `Session` owns one page's live
`doctext.Text`, a `wal.Writer`, a `flush.Loop`, and a registry of
`Subscriber`s to broadcast to. `ApplyClientOp` is the whole pipeline —
`can_apply` (RFC-002 §5, injectable via `CanApplyFunc`, always-allow
today) → apply to the rope → WAL-sync (**the actual ack point**, matching
"the client is acknowledged after the local WAL sync, not after
Postgres") → broadcast to every subscriber except the submitter (whose ack
is the method's own return value) → best-effort enqueue into the flush
loop. A flush-enqueue failure doesn't fail the op — by that point it's
already durable and already visible to every other client, so it's
reported via an injectable `onFlushEnqueueError` hook instead of rejecting
an op that, from the protocol's point of view, already succeeded.

**Open is a replay plus reconciliation**, exactly per ARCHITECTURE.md §4's
own framing ("session open is a replay, and it reads only this service's
own database"): replay every confirmed op from `opstore.ListForPage` into
a fresh rope, then `wal.Recover` the page's local WAL segment and, for
each record whose `ID` isn't already among the confirmed ones, apply it
too and collect it for an immediate `AppendBatch` — closing exactly the
gap a crash between WAL-sync and the next scheduled flush would leave.
Once reconciled, the old WAL segment is deleted and a fresh one opened —
this repo's answer to RFC-002 §6's "delete once Postgres confirms," done
at session-open granularity instead of continuous segment rotation (which
`internal/wal`'s own doc comment already deferred as unneeded at this
scale). `Manager` is the one-`Session`-per-page registry a transport layer
calls `Get` against; sessions stay open until `CloseAll` — no idle-eviction
timer yet (ARCHITECTURE.md names "keep the doc-actor warm" as strictly
cheaper than a snapshot system, but this repo's demo scale doesn't need
even that optimization yet, so it's a documented gap, not a silent one).

**Verified:** unit tests against a small hand-written in-memory
`opstore.Repo` fake (not "mocking infrastructure" — this exercises
`Session`/`Manager`'s own scheduling and reconciliation logic, independent
of what's actually underneath the `Repo` interface) covering empty-open,
replaying confirmed history, `ApplyClientOp`'s ack shape, broadcast
correctly excluding the submitting subscriber, unsubscribe, `can_apply`
denial leaving the rope untouched, `Manager` reuse/distinctness/`CloseAll`,
and the two scenarios that actually justify this package's existence: **a
simulated crash** (WAL handle abandoned before the flush loop ever drains)
where the next `Open` reconstructs the identical pre-crash rope and
re-drives the un-flushed ops into the repo, and confirmation that an
already-confirmed op's leftover WAL record is skipped on replay rather
than double-applied (which would have silently corrupted the text —
`"hihi"` instead of `"hi"`). `TestConcurrentApplyClientOpSerializesCorrectly`
(15 goroutines × 10 ops, `-race`, 10 repeats) is the doc-actor property:
every op lands exactly once regardless of submission order, and every
subscriber receives every op except its own. `go.uber.org/goleak` via
`TestMain` again, since `Session`/`flush.Loop` both run background
goroutines. `gofmt`/`go vet`/`go build`/`go test -race`/`golangci-lint run`
(with and without `-tags=integration`) all clean; `govulncheck` unchanged
in substance; the `opstore` integration suite re-verified green.

**Next:** the WebSocket transport itself — an HTTP endpoint
(`/collab/pages/:id`, per `docs/api/README.md`'s existing table) that
upgrades a connection, registers it as a `session.Subscriber`, decodes
incoming client-op frames into `ops.Op`, and calls `ApplyClientOp` — plus
`docs/api/collaboration.md` documenting that wire contract (currently only
a placeholder row in `docs/api/README.md`). That closes Track 1's 🏁: log
in, write a page, edit live with someone.

---

## 2026-08-26 — Wire format cleanup: lowercase JSON tags on `anchor`/`ops`

Caught before it leaked into the browser-facing contract: `internal/anchor`
and `internal/ops`'s structs had no `json:"..."` tags, so `InsertText`/
`DeleteText`/`Anchor`/`AnchorRange`/`ItemID` would have serialized with
capitalized Go field names (`At`, `Text`, `Item`) — inconsistent with
`documentcore`'s already-established wire convention (lowercase tags,
e.g. `operation.go`'s `json:"id"`/`json:"after"`), which the wasm/TS bridge
already depends on. Fixed: lowercase tags added throughout, and `Bias` now
has `MarshalJSON`/`UnmarshalJSON` rendering `"before"`/`"after"` instead of
a raw `0`/`1`, matching `BlockKind`/`MarkKind`'s own tagged-string
convention on the documentcore side. Pinned with new tests
(`TestAnchorAndAnchorRangeUseLowercaseWireFields`, `TestMarshalOpUsesLowercaseWireFields`,
plus round-trip/rejection tests for the new `Bias` marshaling). Full
module regression (`-race`, integration, lint) still clean.

---

## 2026-08-26 — `deploy/terraform`: GCP IaC for the three Track 1 services

Built ahead of live GCP credentials, at explicit request ("build everything
without [creds], I'll add them later for testing") — written against
`ADR-008`/`ADR-010`/`CLOUD_ROADMAP.md`/`CLOUD_PORTABILITY.md`, self-reviewed
rather than `terraform plan`/`apply`'d (no credentials exist yet). Root
module + `network`/`postgres`/`redis-vm`/`cloud-run-service` child modules;
full setup/teardown steps and cost-posture reasoning in
`deploy/terraform/README.md`, which cites the specific doc section behind
every non-obvious choice — worth reading in full before the first `apply`,
not summarized further here.

**One real bug this surfaced, fixed immediately:** `uuidv7()` is a
**Postgres 18** builtin; the migrations for all three services still had
`id UUID PRIMARY KEY DEFAULT uuidv7()` on four columns, but Cloud SQL's
managed Postgres only goes up to 17 — `apply`ing these migrations there
would fail with `function uuidv7() does not exist`. Checked each column
before touching anything: `docs.pages.id`, `auth.users.id`, and
`auth.refresh_tokens.id` are all generated application-side already
(`uuid.Must(uuid.NewV7())` / `domain.NewUserID()` / `domain.NewJti()`), so
their `DEFAULT` was already-dead schema decoration — removed outright, no
behavior change. `collab.outbox.id` was the one real dependency (Go never
set it, relying entirely on the Postgres-side default) — fixed to match
the rest of the codebase's convention instead of chasing PG18
compatibility: `opstore.go` now generates the id application-side
(`uuid.Must(uuid.NewV7())`) for both `Append` and `AppendBatch`, `id`
added to `InsertOutboxEvent`/`InsertOutboxEventBatch`'s column list, sqlc
regenerated. Direct edits to the existing `CREATE TABLE` statements, not
new migrations — nothing is deployed yet, so there's nothing to `ALTER`
around. Verified: full regression across all three services'
integration suites (document-service, auth-service, collaboration-service)
green.

**Real, deliberately-not-fixed gaps the README documents in detail**
(each is an application-code decision beyond this Terraform's scope, not
silently worked around): `collaboration-service`'s local WAL is lost on an
*abrupt* kill between flush intervals (accepted at current no-real-load
scale, per `ADR-010` §1's "idle is not wrong" framing — revisit if real
concurrent editors show up); Cloud Run forwards one port but
`auth-service` opens two (gRPC `:9006` + HTTP `:8006`), so this deployment
picks HTTP as ingress and **`AuthService`'s gRPC is not externally
reachable through it as-is** — needs either h2c multiplexing or the
still-out-of-scope `api-gateway`; `auth-service`'s RS256 key is
in-memory (`keys.NewInMemoryStore()`), so it mints a fresh keypair every
cold start — blocking for anything beyond a single always-warm instance
until `CLOUD_ROADMAP.md` Phase 2's Cloud KMS wiring lands in Go. Also
noted: no `Dockerfile` exists in any service yet — needed both for this
Terraform's image-push step and for local `docker compose` (see "Next").

---

## 2026-08-26 — Scope decision: `notification-service` added to Track 1

Reopens part of `ADR-011`'s scope cut, at explicit user request after being
asked to confirm (`notification-service` was one of the 8 services
`CLAUDE.md` names as out-of-scope for this repo). Per `CLAUDE.md`'s own
rule ("a still-out item needs an ADR first"), recording the decision here
rather than silently scaffolding it: `notification-service` is now in
scope alongside `document-service`/`auth-service`/`collaboration-service`.
**What it actually notifies about for Track 1 is still open** — nothing in
the 🏁 user story (log in, write a page, edit live with someone) currently
produces an event worth notifying on; the nearest candidates are
transactional auth email (welcome-on-register) or a presence signal
("someone joined your page"). **Scaffolded**: `services/notification-service/` (own `go.mod`, added to
`go.work`), health-probe-only `cmd/main.go` on `:8007` (per
`ARCHITECTURE.md`'s original port assignment) — identical shape to the
original three services' first scaffold commit, no business logic.
`CLAUDE.md`'s Services table, out-of-scope list, and Layout section
updated to match. Its real logic stays blocked on two things still out of
this repo's scope: a trigger event (`ROADMAP.md` Phase 14's
mentions/comments — Track 1 has nothing else worth notifying on) and an
event bus (`CLAUDE.md`'s NATS/Pub/Sub adapter, still "deferred").

---

## 2026-08-26 — `collaboration-service`: the WebSocket transport — Track 1's 🏁

Closed the last piece named in this log: `internal/wsapi` wires
`internal/session` to a real browser-reachable WebSocket at
`/collab/pages/{id}` (Go 1.22+ `http.ServeMux` path values, no router
dependency needed for one route). `github.com/coder/websocket` +
its `wsjson` helper for the transport — chosen over `gorilla/websocket`
for native `context.Context` support, matching every other API in this
codebase. Full wire contract documented in the new `docs/api/collaboration.md`
(also fixed `docs/api/README.md`'s stale Phase-tracking table, which still
referenced `crates/proto` and showed Pages/Auth as unbuilt from before the
Go pivot).

**One real concurrency design point, tested explicitly:**
`Session.ApplyClientOp` broadcasts to every subscriber while holding the
whole session's mutex (`internal/session`'s single-doc-actor model) — a
`Subscriber.Deliver` that blocks on a slow client's TCP write would stall
every other client on that page. `connSubscriber.enqueue` is a
non-blocking send into a bounded per-connection channel; on a full buffer
it cancels that connection's own context (forcing a disconnect) instead of
blocking the caller. `TestSlowConsumerIsDisconnectedNotAllowedToStallOthers`
proves this the way that matters: a connection that never reads is forced
off while a second, live connection keeps submitting and getting acked
throughout, `-race` clean.

**Before this landed, fixed a wire-format inconsistency it would have
otherwise baked in:** `internal/anchor`/`internal/ops`'s structs had no
`json:"..."` tags (would've serialized as `At`/`Text`/`Item`, not matching
`documentcore`'s established lowercase-tag convention the wasm/TS bridge
already depends on). Added tags throughout; `anchor.Bias` now marshals as
`"before"`/`"after"` instead of a raw `0`/`1`. Also added
`oplog.LoggedOp.MarshalJSON`/`UnmarshalJSON` (delegating to the existing
`Marshal`/`Unmarshal` functions) so a `LoggedOp` composes as an ordinary
field inside `wsapi`'s server frames via `encoding/json`'s normal
interface dispatch, rather than every caller needing to know to call
`Marshal` and nest the raw bytes by hand.

**Verified:** unit tests (`httptest.Server` + real `websocket.Dial` calls,
not a mock transport) covering connect→snapshot, submit→ack,
second-client→broadcast (and confirming the submitter never gets its own
op echoed back), missing/invalid actor header rejection before upgrade,
invalid page id rejection, an unknown message type getting an `error`
frame **without** disconnecting, and the slow-consumer test above.
`go.uber.org/goleak` via `TestMain` again. Then a **live smoke test**
end-to-end: real `postgres:18-alpine` in Docker, `go run ./cmd` for real
(not `go test`), a standalone WS client dialing in, confirming the actual
byte-for-byte wire shape (`{"id":"...","version":1,...,"op":{"at":null,"text":"...","type":"InsertText"},...}`)
matches `collaboration.md` exactly. `gofmt`/`go vet`/`go build`/
`go test -race`/`golangci-lint run` (with and without `-tags=integration`)
all clean; `govulncheck` unchanged in substance.

**`cmd/main.go` wired for real**: `DATABASE_URL` → migrate → `pgxpool` →
`opstore.PostgresRepo` → `session.Manager` (local WAL dir via
`COLLAB_WAL_DIR`, defaulting to `./data/collab-wal` — a persistent volume
in any real deployment, per `deploy/terraform/README.md`'s own note on
this) → `wsapi.Handler` mounted alongside `/health`.

All three Track 1 services (`document-service`, `auth-service`,
`collaboration-service`) now have real, tested, runnable business logic
end to end. **What's still missing before any of it is reachable from an
actual browser** (not just a raw WS/gRPC client): `document-service` and
`auth-service` are gRPC-only with no browser-callable HTTP surface
(`api-gateway` is out of scope; needs at least a thin REST↔gRPC shim), and
`web/` is still the unmodified Vite scaffold with no real screens. Next:
the gateway/REST shim, then real frontend screens, then a local
`docker compose` stack tying all of it together for a running demo.

---

## 2026-08-26 — GCP Terraform expanded: GCS/CDN, Cloud Build, Pub/Sub

At explicit request, for GCP learning exposure — three additions to
`deploy/terraform/`, each picked because it ties to something real in this
repo rather than being speculative infra, and each genuinely scale-to-zero
or pay-per-use (no idle cost added):

- **`modules/frontend-hosting`** — a GCS bucket configured for static
  website hosting, for `web/`'s Vite build output. Deliberately **not** a
  Load Balancer + Cloud CDN: `ADR-010`'s own Tier R/S split puts Cloud
  Storage in Tier R (zero idle) and Cloud Load Balancing/CDN in Tier S
  (hourly, never scales to zero) — a bucket matches the cost posture every
  other choice in this module already follows. `CLOUD_PORTABILITY.md`'s
  actual long-term target for this slot is Firebase Hosting, not a raw
  bucket; noted as a same-shape, zero-idle-cost stand-in rather than
  silently diverging from that doc.
- **`modules/cloud-build`** — CI replacing the README's manual
  `docker build`/`push` steps, wired to Artifact Registry. Free tier
  covers 120 build-minutes/day; bills per-minute after, no idle cost.
  Connecting a real Git repo needs one manual console step Terraform
  can't finish headlessly — documented plainly rather than implied
  automatic. Can't actually run yet: no `Dockerfile` exists per service
  (a known gap already in the README; someone else is handling that).
- **`modules/pubsub`** — one topic + pull subscription for
  `collab.ops_flushed`, the one real outbox event type that exists in the
  Go code today (`opstore.go`'s `OutboxEventOpAppended` constant) —
  `document-service` has no outbox table at all yet, so it contributes
  none. This reverses the previous entry's "deliberately not provisioned"
  call on Pub/Sub — the underlying fact hasn't changed (**nothing
  publishes to it yet**, no poller, no `EventBus` adapter implemented),
  only the ask has: `CLAUDE.md`'s stack table already names Pub/Sub as the
  deferred cloud `EventBus` adapter, so this is provisioning ahead of that
  Go-side work landing, made cheap by Pub/Sub's own zero-idle-cost shape,
  not "nothing built speculatively" being abandoned.

`deploy/terraform/README.md` updated throughout — new sections for all
three, extended "What this provisions"/"Known limitations"/"Judgment
calls". Still no `terraform init/plan/apply` run (no credentials); a
manual brace-balance + close read substituted for `terraform fmt`/`validate`
since the CLI isn't installed in this environment.

---

## 2026-08-26 — `api-gateway`: minimum REST↔gRPC shim to reach a browser

Reopens another slice of `ADR-011`'s scope cut — `api-gateway` was one of
the "8 out of scope" services; **this is not that service.** No RS256
verification, no rate limiting, no circuit breaker, no WS consistent-hash
routing (all named in the original design, `ARCHITECTURE.md`). Just REST↔
gRPC translation for `document-service` and `auth-service`, implementing
`pages.md` §2 and the newly-written `auth.md` §2 (auth.md previously had
no REST-mapping section at all — added it, following pages.md's exact
conventions: same error shape, same status-translation table plus the one
code auth needs constantly that pages.md doesn't, `UNAUTHENTICATED`→401).
`collaboration-service`'s WebSocket isn't proxied — a persistent
connection isn't a request/response resource to translate.

**A real cross-module visibility problem, solved before any handler code:**
both backends' generated gRPC client stubs lived under `internal/genproto`,
and Go's internal-package rule blocks any importer whose path isn't rooted
under that same module — `api-gateway`, a separate module, categorically
could not import them. Fixed at the root, not worked around: moved both
services' `genproto` out of `internal/` (`document-service/genproto/documentv1`,
`auth-service/genproto/authv1`), updated each `.proto`'s `go_package`,
regenerated cleanly via each service's existing `gen-proto.sh` (hand-editing
the generated `.pb.go` files was tried first and rejected — the import
path is embedded in a length-prefixed byte string inside the compiled
`FileDescriptor`, and a sed replace across strings of different lengths
would have silently corrupted it). `go.work`'s workspace mode resolves the
cross-module import with zero `replace` directives and no `go.sum` needed
— `api-gateway`'s `go.mod` never grew a `require` line for either backend
module at all. Full integration regression on both existing services
(`document-service`, `auth-service`) reconfirmed green after the move.

**Structure:** `internal/apierror` (the one shared error shape + gRPC-status→
HTTP table, used by both `pagesrest` and `authrest`), `internal/actorctx`
(forwards the `X-Actor-Id`/`X-Actor-Kind` header stand-in onto outgoing
`actor-id` gRPC metadata — same temporary-auth convention `pages.md`/
`auth.md` already documented, just relocated to where a browser can
actually reach it), `internal/pagesrest` and `internal/authrest` (six
routes each, matching each doc's §2 table exactly, including
`LifecycleState`'s enum→lowercase-string rendering and `GetUser`'s JSON
never containing anything password-shaped).

**Verified:** unit tests per REST package against an in-process fake gRPC
backend (`bufconn`, not a mock of this gateway's own logic — a real
`grpc.Server` implementing the real generated `*ServiceServer` interface,
same shape a live backend presents) covering request/response translation,
actor-id forwarding, gRPC-status→HTTP mapping (including the
`UNAUTHENTICATED`→401 case with byte-identical error bodies for both
credential-failure causes), query-param parsing, and malformed-JSON
rejection. `gofmt`/`go vet`/`go build`/`go test -race`/`golangci-lint run`
clean; `govulncheck` unchanged in substance (same pre-existing stdlib
CVE class as every other service in this repo). Then a **full live smoke
test**: real Postgres ×2 (separate databases) + real Redis + `go run`
for `document-service`, `auth-service`, and `api-gateway` all at once —
register (hit the single-use bootstrap-claim correctly on a repeat
attempt, `409 conflict`), login, create a page, list pages, get a page,
get a user, all via `curl` against real REST/JSON, decoding the real
RS256 JWT to extract the actor id exactly the way a browser client would.

**What's left before an actual browser can run the whole thing:** `web/`
is still the unmodified Vite scaffold — real screens (login, page list,
the editor wired to `collaboration.md`'s WebSocket contract) don't exist
yet — and there's no `docker compose` stack or per-service `Dockerfile`
to stand the whole system up with one command instead of hand-starting
five processes. Both still open; see this entry's Terraform sibling above
for the Cloud Build piece that's already waiting on those `Dockerfile`s.

---

## 2026-08-26 — Dockerfiles + `docker-compose.yml`: the whole stack, one command

Closed the remaining gap from the last entry. Every service now has a
`Dockerfile`, multi-stage (`golang:1.25-alpine` build → `gcr.io/distroless/
static-debian12` runtime), and a root `docker-compose.yml` brings up all
five services + one Postgres per service (mirroring `DATA_MODEL.md`'s
"database per service," same posture `deploy/terraform/modules/postgres`
uses for the cloud) + Redis + a Vite dev-server container for `web/`, with
health-check-gated startup ordering.

Four services build with their **own directory** as context (no
cross-module dependency); `api-gateway` builds from the **repo root**
instead — it imports `document-service`'s and `auth-service`'s `genproto`
packages across module boundaries via `go.work`, and workspace mode
requires every `use`-listed module to physically exist on disk, so a
narrower context would fail. `collaboration-service`'s image runs as root,
not `:nonroot` like the other three — it writes a local WAL file per open
page (`internal/wal`) under `COLLAB_WAL_DIR`, typically a mounted volume a
non-root UID has no guaranteed write access to without extra `chown`
machinery.

**A real bug Docker's isolated build caught that nothing else would have:**
`auth-service`'s own `go.mod` never declared `github.com/pressly/goose/v3`
as a dependency, despite `internal/migrate/migrate.go` importing it
directly — every `go build`/`go test` all session had silently succeeded
anyway because `go.work`'s workspace mode resolves the build list across
*all* workspace modules together, and `document-service`'s `go.mod`
happens to require `goose` too. The moment Docker copied only
`services/auth-service/` into an isolated build context — no `go.work`, no
sibling modules — the masked gap surfaced immediately:
`no required module provides package github.com/pressly/goose/v3`. Fixed
at the root with `GOWORK=off go mod tidy` (which also reorganized the
whole file into proper direct/indirect sections, and dropped
`pgregory.net/rapid`, leftover cruft from initial scaffolding that
auth-service never actually imports). Then **proactively checked every
other standalone service** the same way (`GOWORK=off go build`/`go vet`
per module) rather than assuming the fix was isolated — all others were
already clean.

**Two more real bugs, same shape, found by the actual `docker compose up`
run, not by any test:** both `auth-service`'s and `collaboration-service`'s
generic-internal-error paths silently swallowed the real underlying error
without logging it anywhere — `auth-service`'s `toStatus` default branch
(contradicting `auth.md`'s own status table, which says every `INTERNAL`
is "logged as: `error`"), and `wsapi.ServeHTTP`'s `manager.Get` failure
handling. Both surfaced as a completely opaque `"internal error"`/`"opening
session failed"` with zero server-side trace — chased down by adding an
unconditional trace log at the very top of the gRPC handler to prove the
request wasn't even reaching application code, which led to the real
cause both times: **stale zombie processes from earlier standalone `go
run` smoke tests this session**, still bound to the same ports on the
host via IPv6 wildcard binds (`lsof -i` showed them; `pkill -f "go run
./cmd"` had only ever killed the `go run` wrapper, never the compiled
child binary it execs), answering requests instead of the real containers
and failing because *their* backing Postgres containers had already been
torn down. Not application bugs — but the missing server-side logging that
made them so hard to diagnose **is** a real, permanent gap, now fixed in
both places (`slog.Error` with the actual `err`), independent of this
specific false alarm.

**Verified, in order:** all 5 images build clean; each runs standalone and
answers its own `/health`; the full `docker compose up --build` stack
reaches all-healthy (3 Postgres + Redis + all 5 app containers); the Vite
dev server serves `web/` at `:5173`; the **full REST flow through the
composed stack** — register (correctly hitting the single-use bootstrap
claim, `409 conflict`, on a repeat), login, create a page, list pages —
all via `curl` against `api-gateway:8000`, decoding a real RS256 JWT for
the actor id exactly as a browser would; and the **collaboration
WebSocket** through the composed stack, confirmed against `:8002` directly
(not proxied — `docs/api/collaboration.md`). Also hit, understood, and
distinguished from a real bug: a Docker Desktop/OrbStack port-forwarding
hiccup where recreating one container (via `docker compose up -d
<service>`, not a full `up`) intermittently dropped another already-running
container's host port mapping until `docker compose restart` on the
affected container — an environment quirk, not application code, noted
here so a future session doesn't re-diagnose it from scratch.

**Track 1's "minimum to reach portable code" is now fully closed**: all
five services have real logic, all are containerized, `docker compose up`
brings up the whole system with one command, and the full user-facing flow
(register/login, create/list pages, live WebSocket editing) works
end-to-end against real Postgres/Redis in containers. What remains is
`web/`'s actual screens — still the unmodified Vite scaffold — which is
now purely a frontend task with a fully working backend underneath it.

---

## 2026-08-26 — `notification-service`: real logic, at explicit request

Closed the gap the earlier scope-decision entry left open: `notification-service`
was scaffolded as a health-probe-only skeleton because its documented
trigger (mentions/comments, `ROADMAP.md` Phase 14) isn't in this repo's
scope. Re-reading `DATA_MODEL.md` §10's event-topic table found the actual
answer already written down: `auth.user_registered` → `notification` is
the **one** event topic Track 1 can genuinely produce — every other
`auth.*`/`docs.*` topic needs sharing, RBAC, or deactivation, none in
scope. Built exactly that, real logic end to end, nothing further invented.

**`auth-service` gained its own outbox** (`auth.outbox`, added directly to
the existing single migration file — not live yet, so no `ALTER`
needed — same convention as `collab.outbox`/`docs` would use). `Register`
writes an `auth.user_registered` row in the **same transaction** as the
user insert (`internal/outbox.WriteUserRegistered`, called with the
tx-scoped `*authrepo.Queries` — never the pool-level one, or the event
could be durable without the user existing, or vice versa, on a partial
failure). A new `internal/outbox.Poller` claims unpublished rows
(`FOR UPDATE SKIP LOCKED`, matching `DATA_MODEL.md`'s own outbox
prescription exactly) and publishes each to NATS, marking `published_at`
only after a successful publish — a batch that partially fails rolls back
entirely, so an already-published row *can* be redelivered on the next
tick, which is fine and expected: NATS is at-least-once, and every
consumer must dedupe (RFC-002 §4 rule 5's reasoning, restated in
`DATA_MODEL.md`'s outbox section).

**`notification-service` gained its own Postgres** (`notify.notifications`,
`source_event_id UNIQUE` as the dedup key — the outbox row's own id, not
something notification-service invents), a plain **core NATS** subscriber
(not JetStream — a deliberate gap, documented in `internal/notify`'s
package comment: core NATS has no redelivery of its own, so a message
published while this service is down is lost, not queued; acceptable for
a "missed welcome notification," not something worth JetStream's added
operational surface at this repo's scope — revisit if a notification type
is ever added whose loss actually matters), and `GET /notifications`
(`X-Actor-Id` header stand-in, same convention as every other service),
reached directly by the browser, same as `collaboration-service`'s
WebSocket — nothing here is gRPC, so `api-gateway` has no reason to proxy it.

The two services' event payload/envelope shapes (`UserRegisteredEvent`,
the `{id, payload}` wire envelope) are independent, matching struct
definitions in each module rather than a shared package — unlike
`api-gateway`'s genproto imports, these are small and asymmetric
(publisher vs. subscriber), so duplicating ~10 lines was the better call
than standing up a third shared module for it.

**Two more silent-error-swallow bugs, same shape as the last entry's
two, found the same way (by actually running `docker compose up`, not by
any test):** `notify`'s `GET /notifications` handler and (already fixed
in the previous entry, for reference) the same pattern in `auth-service`
and `wsapi`. Both new ones logged nothing on failure — fixed with
`slog.Error` at the actual failure point, same discipline. This makes
**three occurrences of the identical bug class across three different
services this session** — worth naming as a pattern: **error-mapping code
that converts a real error into a generic client-facing message must log
the real error before discarding it, every time, as a matter of course**,
not something to remember per call site.

**Verified:** `notify` — 3 unit tests (`HandleUserRegistered`'s decode/
dedup-call logic against a hand-written fake, not a mock of infrastructure)
plus 4 integration tests against real Postgres (round-trip, idempotent
`Create`, per-user scoping) and one real embedded `nats-server` instance
(not Docker — realistic enough: same client library and wire protocol,
just no container needed for this piece) proving the actual point of the
package: a message published to `auth.user_registered` ends up as a
queryable row. `auth-service`'s `internal/outbox` — 2 integration tests
(publish-and-mark, and confirming an already-published row is never
redelivered by a later poll) against real Postgres + embedded NATS, plus
a new `TestRegisterWritesUserRegisteredOutboxEvent` in the existing
`authservice` integration suite confirming the outbox row's exact shape
(unpublished, correct payload) right after `Register` returns. Then a
**live smoke test** end to end — real NATS, Postgres, Redis in Docker,
`go run ./cmd` for both services for real — register via `grpcurl`,
decode the JWT for the actor id, confirm the welcome notification appears
via `curl`. Then the **same flow again through the full composed stack**
(`docker compose up --build`, now including a shared `nats` service and
`notification-service`'s own `notification-db`) — register through
`api-gateway`, welcome notification retrievable from
`notification-service` directly. `gofmt`/`go vet`/`go build`/
`go test -race`/`golangci-lint run` (with and without `-tags=integration`,
and in both `go.work` and `GOWORK=off` standalone mode, per the last
entry's lesson) all clean on both services; `govulncheck` unchanged in
substance.

`docker-compose.yml` updated: a shared `nats` service (health-checked via
its monitoring endpoint), `notification-service`'s own `notification-db`,
`NATS_URL` wired into both `auth-service` and `notification-service`.
`CLAUDE.md`'s Services table, event-bus stack line, and Layout section
updated to match — `notification-service` is no longer a skeleton.

Track 1's backend is now, genuinely, feature-complete for the 🏁 plus the
one real notification flow. `web/` remains the only unmodified piece.

---

## 2026-08-26 — `collaboration-service`: `boundaries` — closing a real wire-protocol gap found while building the frontend

Starting the frontend's editor screen surfaced a real gap in
`collaboration.md`'s own wire contract: a plain-text client (a
`<textarea>`, not a rich editor with its own anchor bookkeeping) has no
way to construct a valid `DeleteText` at all — every `Anchor` names an
`ItemID`, and the only `ItemID`s a client ever learns about are the ones
named in ops it already submitted or observed, never the ones a fresh
insert just created. Fixed at the protocol level, not worked around
client-side: `doctext.Text` gained `Boundaries()`, naming the whole live
document as one `AnchorRange` (nil once empty) — `DeleteText` only ever
needs a range's first and last item, never every item in between
(`internal/ops`'s own `DeleteText` doc comment already said this; this
just gives a client the same reasoning). `anchor.Log` gained the
reverse-lookup `ItemAt(pos)` this needs.

**A real bug caught before it shipped, by writing the test first:** an
initial `ItemAt` reused `sliceIndexForLiveOffset` for the lookup — wrong,
because that method's actual contract is "the insertion point before the
pos-th live item," which can legitimately land on a *preceding tombstoned
run*. `TestItemAtSkipsALeadingTombstoneRun` (three items, first
tombstoned) caught it immediately: `ItemAt(0)` returned the tombstoned
item at slice index 0 instead of the real first live item. Fixed with
`ItemAt`'s own dedicated scan rather than reusing a method whose contract
doesn't actually match.

`Session.ApplyClientOp` now returns a `CommitResult{Op, Boundaries}`
instead of a bare `LoggedOp` — `Subscriber.Deliver` takes the same
struct, so every broadcast carries the same boundary info the submitter's
own ack does. `wsapi`'s `ack`/`broadcast` frames grew an optional
`boundaries` field (omitted once the document is empty).
`docs/api/collaboration.md` documents the whole mechanism and the
"replace everything" editing strategy it enables — see that doc's new
§3 subsection.

**Verified:** `anchor` — the tombstone-skip regression test plus
out-of-bounds/empty-log cases. `doctext` — empty-is-nil,
spans-first-to-last (and deleting via exactly that range genuinely
empties the text — not just asserted, executed), and a tombstoned-first-
character case mirroring `anchor`'s own. `wsapi` — an ack's `boundaries`
is non-nil after an insert, a broadcast carries the identical
`boundaries` its triggering ack did, and (the real end-to-end proof) a
`DeleteText` built from an insert's own ack `boundaries` genuinely empties
the document and the next ack's `boundaries` comes back nil. Full module
regression (`-race`, `golangci-lint`, both with and without
`-tags=integration`) clean throughout.

**One more gap caught immediately after, before any frontend code
consumed it:** the `snapshot` frame didn't carry `boundaries` at all — a
client reconnecting to an already non-empty page would see the text but
have no valid anchor to build its *first* edit from, only after its own
first successful op. Fixed with `Session.Boundaries()` (same lock
discipline as `Session.Text()`) called once when a connection opens;
`TestReconnectingToANonEmptyPageGetsBoundariesInTheSnapshot` (dial, edit,
disconnect, dial again to the same warm session) proves it. Full
regression clean again.

**A third gap, also caught before any frontend code shipped with it
baked in:** `X-Actor-Id`/`X-Actor-Kind` as headers is unusable from an
actual browser — the WebSocket browser API has no mechanism to set custom
headers on the upgrade request at all, full stop. `actorFromRequest` now
also accepts `?actor_id=`/`?actor_kind=` query parameters (the header wins
if both are present), and `docs/api/collaboration.md` §1 documents both
forms and says plainly which one a real browser must use.
`TestActorIdViaQueryParamWorksLikeTheHeader` pins it. This would have been
a "my WebSocket client mysteriously always gets 401" bug discovered only
once real browser code existed to hit it — worth having caught by writing
the browser-side hook first and checking every assumption it made against
the actual API surface, rather than after.

**Next:** the frontend itself — this was a prerequisite discovered
mid-build, not a detour.

---

## 2026-08-26 — `web/`: real screens, at explicit request ("complete frontend for mvp")

`web/` is no longer the unmodified Vite scaffold — the last piece named
in every prior entry. Design system copied verbatim from
`docs/ui-mockups/mockup.css` per that directory's own rule ("if a mockup
and a doc disagree, the doc wins" — this is the doc) — tokens, dark/light
theme handling, and every reused CSS class (`.rail`, `.tree-item`,
`.avatar`, `.auth`, `.canvas`/`.doc`, etc.) come from there, not
reinvented. `signin.html`'s real assertion — a self-hosted instance's
first screen is first-run setup, not a login — carried through
faithfully: `AuthPage`'s "Fresh instance" mode really does become
permanently unavailable after one success, because `auth-service`'s
`Register` really is bootstrap-only (`auth.md` §1), not a UI-only
restriction the mockup only implied.

**Structure:** `src/api/{http,auth,pages,notifications}.ts` (REST clients;
one shared error shape from `pages.md`/`auth.md` §2), `src/auth/AuthContext.tsx`
(token storage, the actor id every other client call needs — decoded
client-side from the JWT's own `sub` claim, never re-verified, since
verification is `auth-service`'s job and a browser has no business
re-deriving trust it was already handed), `src/collab/useCollabSocket.ts`
(the WebSocket client), `src/screens/{AuthPage,PagesScreen,EditorPane}.tsx`.

**The one significant scope decision, stated plainly rather than
faked:** `EditorPane` edits **plain flat text** over `collaboration-service`'s
character-level rope, not `document-core`'s block tree — the two layers
were never reconciled (`document-service` has no consumer for
`collab.ops_flushed` yet), so a rich block editor wired to live
collaboration doesn't exist. The 🏁 ("log in, write a page, edit live
with someone") only requires live collaborative text, which this
delivers for real; claiming more would have meant faking it. Also real,
not faked: the "presence" avatar stack shows actual actor ids observed
via real `broadcast` frames, not the mockups' scripted `setTimeout` peer.

**Three real backend gaps found and fixed *while building the frontend*,
before any of them shipped baked into it** (each has its own detailed
entry immediately above this one): (1) `doctext.Text.Boundaries()` +
`anchor.Log.ItemAt()` — a plain-text client had no way to construct a
valid `Anchor` at all without the server naming the document's boundary
anchors on every ack/broadcast/snapshot; a real bug in the first `ItemAt`
attempt (reusing the wrong method's contract, silently returning a
tombstoned item) was caught by writing the regression test *before*
trusting the implementation. (2) The `snapshot` frame didn't carry
`boundaries`, breaking reconnect-to-a-non-empty-page. (3) `X-Actor-Id` as
a header is entirely unusable from a real browser — the WebSocket browser
API cannot set custom headers on the upgrade request, at all, ever —
fixed with an `?actor_id=` query-param fallback the header still takes
priority over. All three were caught by actually writing the consuming
frontend code and checking every assumption against the real API surface
before wiring it in, not discovered later as a support ticket.

**A real client-side bug fixed the same way, in `EditorPane` itself:** an
initial debounce implementation let an incoming remote broadcast overwrite
the textarea while the user was still mid-keystroke on their own edit,
silently destroying unsent local text. Fixed with an `editingRef` guard
that suppresses remote-driven `draft` updates for the duration of a
pending local edit. The remaining, honest limitation this doesn't solve
(documented in the component's own comment): two people typing at once
still resolves as "whoever's debounced whole-document replace lands last
wins entirely," not a per-keystroke merge — a real, stated cost of the
whole-document-replace strategy `docs/api/collaboration.md`'s `boundaries`
field enables, not a hidden one.

**Verified:** `tsc -b` (typecheck) and `vite build` (production build)
both clean; `oxlint` clean except two benign `set-state-in-effect`
warnings for legitimate external-system synchronization (resetting hook
state on prop change before a new WebSocket connects; syncing `activePage`
from a route param) — read, judged intentional, left as warnings rather
than restructured away. Every new source module confirmed to transform
successfully through Vite's dev server (a 200, not the 500 a syntax/
resolution error would produce) against the live `docker compose` stack.
The collaboration WebSocket flow re-verified end-to-end using the **exact
mechanism the browser frontend uses** — `?actor_id=` query param, no
header at all — against the composed stack: connect, snapshot, submit an
`InsertText`, receive an ack with populated `boundaries` matching the
`AnchorRange` shape `src/collab/types.ts` expects byte-for-byte.

**What is honestly NOT verified, and why:** no browser-automation tool
was available this session, so the actual React runtime — state updates,
DOM rendering, real user interaction (typing, clicking, two tabs staying
in sync visually) — was never exercised end-to-end the way "start the dev
server and use the feature in a browser" calls for. Type-checking, a
production build, dev-server module transforms, and the underlying wire
protocol are all real and all passed, but that verifies the frontend
*compiles and talks to the right shapes*, not that it *renders and
behaves correctly* for an actual user. Manual browser testing (register →
create a page → edit → open a second tab → confirm live sync) is the
one remaining step before calling this done, and it needs a human or a
future session with browser tooling, not a claim of success here.

---

## 2026-08-26 — Two real cross-origin bugs, found only by the user actually trying it in a browser

Exactly the gap the entry above named: the two bugs below were invisible
to every test, every `curl`, and every non-browser smoke test all session
— they only exist because a real browser enforces same-origin rules no
Go-side test or `curl` call reproduces, and `web/` (`:5173`) calling the
backend services (`:8000`, `:8002`) is genuinely cross-origin.

**1. `api-gateway` had no CORS headers at all.** The user's first register
attempt failed with a generic "Something went wrong" — the browser blocked
reading the response (Chrome DevTools' "Failed to load response data / No
data found for resource" is the tell), and `auth-service`'s own logs
showed the request never arrived. Fixed with `github.com/go-chi/cors`,
configured via `CORS_ALLOWED_ORIGINS` (comma-separated; defaults to `"*"`)
— safe as a default because nothing in this codebase's REST surface uses
cookies or any other ambient credential a wildcard origin could leak
(every service's actor identity is the same unauthenticated header/query
stand-in `pages.md`/`auth.md`/`collaboration.md` all document).
`notification-service` (plain `net/http`, not chi) got the equivalent
hand-written `withCORS` middleware, including handling the `OPTIONS`
preflight `X-Actor-Id` triggers as a non-simple header. Verified with a
real preflight (`curl -X OPTIONS` with `Origin`/`Access-Control-Request-*`
headers) against the live container, then the actual `register` call
with an `Origin` header — both correct.

**2. `coder/websocket`'s `Accept` rejects cross-origin upgrades by
default** — a security default in the library itself, not a bug in it.
The user got past login, created a page, and the editor showed
"Disconnected" with the textarea disabled — `collaboration-service`'s own
logs showed nothing (the `Accept` failure path had no logging either,
same recurring gap as three earlier entries; fixed alongside this). Fixed
with `wsapi.NewHandler` now taking `*websocket.AcceptOptions` explicitly,
and `cmd/main.go`'s `wsAcceptOptions()` defaulting to
`InsecureSkipVerify: true` (same "nothing here has a credential to leak"
reasoning as the CORS fix), configurable to a real `OriginPatterns`
allowlist via `COLLAB_ALLOWED_ORIGINS`.

**Caught the actual failure mode with a new test, not just fixed and
moved on:** every existing `wsapi` test dials from the same process via
`httptest.Server`, so the request's `Host` and `Origin` always genuinely
matched — none of them could ever have caught this, which is exactly why
it shipped. `TestCrossOriginConnectionRejectedByDefault` sets a real,
different `Origin` header and confirms `nil` options reject it (the log
line it captures — `request Origin "localhost:5173" is not authorized for
Host "127.0.0.1:...`" — is the literal error the user's browser hit);
`TestCrossOriginConnectionAllowedWithInsecureSkipVerify` confirms the
actual fix resolves the identical dial. Full regression clean; both fixes
verified against the real running `docker compose` stack — a preflight +
register through `api-gateway` with an `Origin` header, and the exact
browser-style WS dial (query-param actor auth, cross-origin) against
`collaboration-service`, both now succeeding end-to-end.

**Process note:** registering a real test account through the live
gateway during verification consumed the one bootstrap admin slot — the
whole stack (`docker compose down -v && up --build -d`) was reset
afterward so the user's own registration attempt would land on a
genuinely fresh instance, not "instance already claimed."

---

## 2026-08-26 — Block/live-text reconciliation, backend half: `collaboration-service` now speaks both ISA tiers as one system

The user asked for full `editor.html` support (rich block editing: slash
menu, bubble menu, drag reorder, headings/quotes/code/dividers) except
Compiler view, the Live-session link, Facts, and Search, and — when asked
how block editing should relate to live collaboration, given today's
frontend only ever edited flat text over a whole-page rope — explicitly
chose **"reconcile the two into one system"** over keeping them separate.
This entry is the backend half of that; the frontend hasn't been touched
yet (see below).

**Architecture, following `DATA_MODEL.md`'s own already-stated design**
(collaboration-service owns `collab.ops`, publishes events;
document-service materializes `docs.blocks` by replaying them — this was
spec'd, just never built): `document-core` moved out of
`document-service/internal/` into its own module, `services/documentcore`
(`marginal/documentcore`) — collaboration-service needed the exact same
`Page`/`Op`/`Apply`/`Invert` logic for block-structure ops, and "never a
second implementation" (already the rule for the wasm bridge) applies
here too. `document-service`'s `cmd/wasm` entrypoint now imports the new
module path; nothing else about it changed. Added `documentcore.TypeName`
(matching `internal/ops`' existing convention) so the new union package
below didn't need its own duplicate type-name switch.

New package `internal/pageop` in `collaboration-service`: the op union a
live session actually applies. `pageop.Block{Op documentcore.Op}` is a
structural, whole-page-scope op (`InsertBlock`/`DeleteBlock`/
`SetBlockKind`/`SetBlockContent`/`SetTitle`/`MoveBlock`); `pageop.Text{
BlockID, Op ops.Op}` is a character-granular op scoped to exactly one
block's own live rope. `Marshal`/`Unmarshal` nest each tier's own
existing type-tagged envelope (`documentcore.MarshalOp`/`ops.MarshalOp`,
both unchanged) inside one `"scope"`-tagged wire shape — see
`docs/api/collaboration.md` §2 for the exact JSON.

`internal/oplog.LoggedOp.Op` is now typed `pageop.Op` (was `ops.Op`) —
this is the one field-type change every other package (`wal`, `flush`,
`opstore`) didn't need to know about, since none of them inspect an op's
concrete type, only pass `LoggedOp`/its marshaled bytes through.
`opstore.go` (which separately calls `TypeName`/`MarshalOp`/`UnmarshalOp`
for `collab.ops`'s `kind`/`payload` columns) now calls `pageop`'s
versions of the same three functions.

`internal/session.Session` no longer holds one whole-page `doctext.Text`.
It holds a `documentcore.Page` (structure) plus `map[documentcore.BlockID]
*doctext.Text` (one live rope per block). `InsertBlock` seeds a fresh rope
from the op's initial content; `DeleteBlock` discards it; `SetBlockContent`
reseeds it wholesale (a deliberate last-write-wins replace — the op
already only applies when its `prev` matches current content, so this
isn't a new looseness). Every character op on a block re-syncs that
block's `Content.Text` in `Page` afterward, so a later `SetBlockContent`'s
precondition check still sees current state. `ApplyClientOp` now
type-switches on `pageop.Block` vs `pageop.Text`; `CommitResult.Boundaries`
is set only for a `Text` op (a `Block` op has no rune-offset anchor to
report). `Session.Text()`/`Boundaries()` (whole-page) are replaced by
`Session.Snapshot()`, returning title + every block's id/kind/live-text/
boundaries — what a connecting client needs to render the whole page.

`internal/wsapi`'s `serverMessage` gained a `Snapshot *session.Snapshot`
field (replacing the old flat `Text`); the client-submitted `op` now
decodes via `pageop.Unmarshal` instead of `ops.UnmarshalOp`. All 8 test
files across `oplog`/`opstore`/`session`/`wsapi`/`flush` that constructed
raw `ops.InsertText` values directly were updated to build blocks first
(`pageop.Block{Op: documentcore.InsertBlock{...}}`) before submitting a
`pageop.Text` against them — the same pattern every test now follows.
Full suite green under `-race`, including `goleak`.

**Docker build side effect:** `collaboration-service` now imports
`marginal/documentcore` across a module boundary, the same situation
`api-gateway` was already in — its Dockerfile had to switch from building
in its own directory to building from the repo root with `go.work`
copied in (mirroring `api-gateway/Dockerfile` exactly), and
`docker-compose.yml`'s build context/dockerfile path updated to match.
Verified with a real `docker build` from the repo root, plus a
`document-service` rebuild to confirm its own (unrelated, still
directory-scoped) Dockerfile wasn't affected.

**Still open, not done in this pass:**
- The frontend (`web/`) hasn't been updated to the new protocol at all —
  `EditorPane`/`useCollabSocket` still speak the old flat-text wire shape.
  Building the actual rich block editor UI (the user's original ask) is
  next.
- `document-service` still has no consumer for `collab.ops_flushed` — a
  committed block op is authoritative in the live session's memory only;
  nothing persists it into a queryable `docs.blocks` row yet. Needed
  before a page's block content survives every client disconnecting and
  the session being evicted (today: never evicted, so this is latent, not
  yet observable — `internal/session`'s own doc comment on why
  idle-eviction is deferred).
- `documentcore.Page.Title` inside a live session is unused — the
  frontend was never wired to emit `SetTitle`, and `document-service`'s
  own `docs.pages.title` (via `RenamePage`) stays the actual source for a
  page's displayed title. Deliberately not solved this pass: syncing title
  between the two services is a narrower, separable problem from block
  content reconciliation.

---

## 2026-08-26 — Dashboard/editor split, and local multi-device sharing

Two small frontend pieces, ahead of the rich block editor work: the user
pointed out that once the editor screen grows a second side panel (the
inspector, still to come), a "create a new page" affordance crammed into
that three-column layout would be buried. Fixed by splitting the old
combined `PagesScreen` into two screens (Google Docs/Sheets' own pattern):
`DashboardScreen` (`/pages`) — a `.card.link` grid of existing pages plus
a "Blank page" tile, the one place creation happens — and `EditorScreen`
(`/pages/:id`) — rail (navigation only now, no create form) + the live
editor canvas. `PagesScreen.tsx` deleted.

**Local multi-device joining**, since the user asked how someone else
joins a page for testing without any real URL/sharing infrastructure:
the page's own URL (`/pages/<id>`) already *is* the share link — anyone
who opens it and logs in (any account; there's no per-page permission
check yet, matching this repo's out-of-scope RBAC) reaches the same
`collaboration-service` session, already proven by this session's own
multi-connection tests. What was actually missing was two hardcoded
`localhost`s: `web/src/api/config.ts`'s default `GATEWAY_URL`/`COLLAB_URL`/
`NOTIFICATIONS_URL` now derive from `window.location.hostname` instead of
a literal string, and `vite.config.ts` gained `server.host = true` —
docker-compose.yml's `web` service was already passing `--host 0.0.0.0`
so this only matters for a bare `npm run dev`, but the frontend's
hardcoded-`localhost` defaults would have broken a second device
regardless of how the dev server itself was bound (a device's own
"localhost" resolves to itself, not the host machine). `EditorScreen`
also gained a "Copy link" button (`navigator.clipboard`, with a
`window.prompt` fallback) so the current page's URL is one click away.
Verified: `tsc --noEmit`, `oxlint`, and the actual running dev-server
container picked up every change via HMR with no errors, `Network:
http://192.168.117.13:5173/` confirming the LAN bind.

**Not done:** `collaboration-service` wasn't rebuilt this pass — it's
still running the pre-reconciliation image on purpose, since the
frontend still speaks the old flat-text wire protocol (previous entry).
Rebuilding it now would break live editing until `EditorPane`/
`useCollabSocket` are updated to the new `pageop`-tagged protocol, which
is still the next piece of work.

---

## 2026-08-26 — Frontend rewired to the `pageop` protocol; `collaboration-service` rebuilt

The piece the previous entry deferred: `web/src/collab/types.ts` and
`useCollabSocket.ts` now speak `internal/pageop`'s wire shape instead of
the pre-reconciliation flat-text one. `EditorPane` itself needed **zero**
changes — the hook's external contract (`text`/`state`/`peers`/
`replaceText`) is unchanged, which was the point of keeping that contract
stable while everything underneath it moved.

`useCollabSocket` maps the flat-text UI onto exactly one implicit block:
the page's first block if the snapshot already has one, or a fresh
paragraph block the client creates itself (`InsertBlock`, client-generated
id via `crypto.randomUUID()`) if the page is still empty. Every `"text"`
op it sends/receives is scoped to that one block id; `"block"`-scope
broadcasts from elsewhere are otherwise ignored (nothing for a flat-text
view to render). Documented as a deliberate, narrow gap: two clients
connecting to a genuinely empty page in the same instant can each create
their own implicit block (a real but rare race — the same class of
last-write-wins limitation the whole-block-replace strategy itself
already has). A real multi-block editor doesn't have this problem because
block creation stops being implicit.

`collaboration-service` was rebuilt and recreated in the running
`docker compose` stack (`docker compose build collaboration-service &&
docker compose up -d --no-deps collaboration-service`) — the image built
clean, `/health` came back `ok` immediately.

**Verified with a standalone Go smoke-test client**
(`/private/tmp/.../scratchpad/wssmoke`, `github.com/coder/websocket`,
mirroring the real browser mechanism: query-param actor id, `Origin:
http://localhost:5173`), not just unit tests, because this is exactly
the class of bug (real wire-format mismatch between two independently
typed languages) that a same-process Go test can't catch — it dialed the
actual running container end-to-end: connect with zero blocks → snapshot
correctly empty → send `InsertBlock` → ack matches → send `InsertText` →
ack carries `boundaries` → a second connection joins and its own snapshot
shows the block with the live text → second client deletes and replaces
the text → first client receives both ops as broadcasts, in order, with
matching `boundaries`. Every field name and nesting matched the
hand-written TS types on the first try. `tsc --noEmit` and `oxlint` both
clean (one pre-existing `set-state-in-effect` lint warning, unchanged
from before this pass, not a regression).

**Still not done:** this is still the flat single-block compatibility
shim, not the rich multi-block editor (slash menu, bubble menu, drag
reorder, headings/quotes/code/dividers) the user actually asked for.
That's next, and now has a verified-working backend under it.

---

## 2026-08-26 — The rich block editor: `RichEditorPane` + `InspectorRail`, replacing the flat-text shim

`useCollabSocket`/`EditorPane` (yesterday's flat-text compatibility shim)
deleted, replaced by a real multi-block editor speaking `internal/pageop`
directly — no more "one implicit block."

**`web/src/collab/useCollabPage.ts`** (new, replaces `useCollabSocket`):
tracks the whole page's block order (`orderRef: string[]`) and each
block's live kind/text/boundaries (`liveRef: Map<id, {...}>`) from the
server's own frames alone — no separate optimistic local model, same
choice as before and for the same reason (every op round-trips in well
under a keystroke). Exposes `insertBlock(afterId, kind)`,
`deleteBlock(id)`, `setBlockKind(id, kind)`, `setBlockText(id, text)`
alongside `blocks`/`state`/`peers`. `web/src/collab/blockKind.ts` maps
`documentcore.BlockKind`'s tagged-object wire shape to a flat
`BlockKindKey` union a `<select>` can bind to (`paragraph`/`heading1-3`/
`quote`/`code_block`/`divider`).

**`web/src/screens/RichEditorPane.tsx`** (new, replaces `EditorPane`):
paragraph/heading/quote render as real `contentEditable` `<p>`/`<h1-3>`/
`<blockquote>` elements — free typography from `design-system.css`'s
existing `.doc h1`/`h2`/`h3`/`blockquote` tag-selector rules, and Enter
always means "new block" (RFC-001: a block is the text unit, not a line),
so these never need a literal newline. `code_block` is deliberately a
plain `<textarea>` instead of `contentEditable` — sidesteps
`contentEditable`'s notoriously inconsistent newline/line-break DOM
behavior entirely, since code is the one kind that legitimately holds
multiple lines within itself. `divider` has no text at all, just a
`<hr>`. Each block gets a small hover-revealed toolbar (kind `<select>` +
delete) in the left margin, new CSS in `design-system.css`
(`.block-row`/`.block-toolbar`/`.block-kind-select`/`.block-divider`/
`.editable`/`.code-editable`). Backspace on an empty block deletes it and
focuses the previous one; merge-on-backspace-into-a-non-empty-block is a
deliberately unbuilt nicety, not a bug.

**`web/src/screens/InspectorRail.tsx`** (new): the right-side tab rail
from `editor.html`. **Outline** and **People** are real, computed
straight from `useCollabPage`'s own state (heading blocks; peer actor
ids). **Checks/Backlinks/Comments/History** are honest empty-state
panels stating plainly which out-of-scope service each needs
(`diagnostics-service`/`search-service`/a comments feature/
`history-service`, all `ADR-011`-deferred) — present in the tab chrome to
match `editor.html`, never populated with invented data.

**`EditorScreen`** now calls `useCollabPage` once and passes the same
`CollabPage` value to both `RichEditorPane` and `InspectorRail` — each
independently calling the hook would open a second WebSocket to the same
page for no reason (and each connection gets its own subscriber id, so
the two could observe their own broadcasts inconsistently).

**Verified two ways against the real running stack** (no backend rebuild
needed this pass — nothing server-side changed): the existing flat-text
smoke test still passes unmodified, and a **new structural smoke test**
(`/private/tmp/.../scratchpad/wssmoke/structural`) exercises exactly the
op sequence the client's own reducer assumes — `InsertBlock` ×2 (order
becomes `[A, B]`), `SetBlockKind` (A → heading level 2), `MoveBlock`
(order becomes `[B, A]`), `DeleteBlock` (order becomes `[A]`) — checking
each step via a **fresh connection's own snapshot**, not just the
submitting connection's ack, so this also re-confirms a new client
joining mid-session sees correct structure. All four steps passed
first try; every field name/nesting the TS reducer (`applyStructural` in
`useCollabPage.ts`) assumes matched the server exactly.
`tsc --noEmit`/`oxlint` both clean (same two pre-existing
`set-state-in-effect` warnings as before, no new categories).

**Known, accepted gaps** (stated in the new files' own doc comments, not
hidden): two clients opening a genuinely empty page in the same instant
no longer race to create an implicit block (that scenario doesn't exist
anymore — block creation is always explicit now); a structural op whose
recorded prior state (`SetBlockKind.from`, `DeleteBlock.after`,
`MoveBlock.from`) has gone stale from a concurrent edit surfaces as a
console-logged `error` frame and is simply not applied — no retry/rebase,
matching this pass's stated scope.

**Not done:** inline marks/bubble menu, drag-to-reorder, a floating "/"
slash menu, and `docs.blocks` materialization (`document-service` still
has no consumer for `collab.ops_flushed`) — all still open per the
previous entries.

---

## 2026-08-26 — Real bug: the `pageop` rewrite broke the WAL/`collab.ops` wire format for any page touched before it

The user reported the editor showing "Disconnected" again. `collaboration-service`'s
own logs had the real cause: `session: open: recovering local WAL:
... oplog: unmarshaling op: pageop: unknown op scope ""`. Any page that
had ops applied under the *old* flat-text protocol (`oplog.LoggedOp.Op`
typed `ops.Op`, no `"scope"` field at all) left WAL records — and, for
anything already flushed, `collab.ops` rows — that `pageop.Unmarshal` now
rejects outright, since it always expects a `"scope"` key. `session.open`
fails before a session can even exist for that page, so every WS
connection attempt hits `wsapi: opening session failed` and the browser
just sees a closed socket.

This is a real violation of RFC-002 §4's own rule ("this encoding can
never break — only additive change is allowed"): the `pageop` envelope
wasn't an additive change to the wire format, it wrapped *everything* in
a new required field. That rule is exactly why `collab.ops`/the WAL are
supposed to be handled carefully — it was correctly identified as a
target for exactly this kind of care in earlier entries, and this pass
didn't apply it. Recorded here as a real miss, not smoothed over.

**Fix applied — a full local reset, not a compatibility shim**, per the
standing project rule that nothing here is deployed yet so schema/wire
breaks get reset, not migrated around: stopped `collaboration-service`
and `collab-db`, removed both (`marginal_collab-db-data`,
`marginal_collab-wal-data`), recreated fresh. `collab-db`'s migration ran
clean on the empty volume; `collaboration-service` came up with no
errors. This only resets *live block content* — `document-service`'s own
database (page metadata: id, title, tree position) is untouched, so the
page list itself survived; any block text typed before today's
reconciliation pass is gone. Re-verified both smoke tests
(`wssmoke` and `wssmoke/structural`) against the freshly reset instance —
both passed clean.

**Not fixed, deliberately:** no backward-compatibility decoding for the
pre-`pageop` wire format was added. Doing that would be solving a problem
this repo's own stated scope says doesn't exist yet (no real deployment,
no real data to preserve across a breaking change) — the reset is the
correct-sized fix at this stage, not a shim to maintain going forward.
Worth remembering for the *next* wire-format change though: this is
exactly the failure mode RFC-002 §4 warns about, and next time there may
be real data worth not resetting.

---

## 2026-08-26 — Second real bug: empty page had no way to create a first block

Right after the reset above, the user reported "can't type anything."
Real cause, not a connection issue: `useCollabPage` (this session's
earlier rewrite) deliberately dropped the old flat-text shim's
auto-create-an-implicit-block behavior — correct for a genuine multi-block
editor, but nothing replaced it, so a page with zero blocks (every page,
after the reset) rendered nothing to click into and had no keyboard entry
point (Enter/Backspace only exist inside an already-rendered block).

Fixed with an effect in `RichEditorPane` that inserts one empty paragraph
once the page is confirmed empty. The one real subtlety: `state ===
"open"` fires on the WebSocket handshake, strictly *before* the separate
"snapshot" frame arrives — checking `blocks.length === 0` at that point
would be true for every page load, empty or not, and wrongly fire on
non-empty pages too. Fixed by adding an explicit `ready` flag to
`useCollabPage` (true only once a snapshot frame has actually been
processed) and gating on that instead. Also added `key={page.id}` on
`RichEditorPane` in `EditorScreen` — without it, navigating between pages
doesn't remount the component, so its refs (including the new
"already created a first block" guard) would wrongly carry over from the
previous page.

## Multi-user testing: `Register` is bootstrap-only, by design

The user's second tester hit "instance already claimed" trying to
register their own account. Not a bug: `authservice.Register` is a
one-time bootstrap claim (LLD §7) — "after that, registration is
invitation-only" (`ADR-001`, self-hosted, not public sign-up). No invite
mechanism exists in this repo's scope (real RBAC/multi-user surface,
`ADR-011`-deferred). `collaboration-service`'s WebSocket itself has no
auth check at all (`actor_id` is an unverified query param) — the only
real blocker to a second person joining is the frontend's login gate.

**Chosen fix (user-selected, of three offered):** a minimal, explicitly
dev-only second-user path, not a public invite feature. New
`services/auth-service/cmd/seeduser` — a CLI, never exposed over gRPC,
that inserts one more user row directly (reusing `internal/users`,
`internal/passwordhash`, and `internal/outbox` exactly as `Register`
does, minus the bootstrap lock/count-check that only matters for the
*first* user). Run against the live stack via a throwaway
`golang:1.25-alpine` container joined to the compose network (auth-db has
no host-published port, by design) — created `tester2@example.com` this
way, confirmed to work.

---

## 2026-08-26 — Real backlinks: `document-service` now materialises `docs.blocks`/`docs.page_links`

The user asked for backlinks to be built for real now ("later we need
this for connected components, graphs, vector embeddings etc"), which
meant finally building the piece every earlier entry deferred: a
consumer for `collab.ops_flushed`. Two real gaps closed together, since
backlinks need this and nothing else does yet:

**Collaboration-service's outbox had no publisher.** `opstore.go` was
already writing a row per flushed op (`collab.outbox`), but nothing ever
polled it — `docs/porting/PROGRESS.md`'s own migration comment said as
much. New `internal/outbox` (mirrors `auth-service`'s own package
field-for-field: `FOR UPDATE SKIP LOCKED` claim, at-least-once publish,
mark-published, same shape) — the one difference from auth-service's
version is `wireEvent` carries `AggregateID` (the page id) explicitly,
since `collab.outbox.payload` is only ever the op itself and never names
its own page. Added the two sqlc queries it needs
(`ClaimUnpublishedOutboxEvents`/`MarkOutboxEventsPublished`), wired into
`cmd/main.go` (NATS connect + `poller.Run` in its own goroutine).

**`document-service`'s new `internal/blockproj`** consumes
`collab.ops_flushed` and rewrites `docs.blocks`/`docs.page_links` for the
affected page on every event — new migration
`00002_docs_blocks_and_links.sql` (no `parent_id`/`path` LTREE yet,
unlike `DATA_MODEL.md`'s fuller sketch — every block kind that exists
today is flat; a tree column earns its keep only once a nesting kind
does), new `internal/blockrepo` (sqlc package, `pgx.Batch` bulk
insert — multi-arg `unnest()` doesn't type-check under sqlc's static
analyzer without a live DB connection configured, so this mirrors
`collabrepo`'s own `:batchexec` pattern instead of fighting that).

The one real design decision: **a full per-page rewrite on every event,
not an incremental patch**, and **no anchor/rope replay** —
`internal/blockproj`'s own doc comment explains why the latter is
correct, not a shortcut: every real client in this repo only ever sends a
`Text` op as "delete everything, insert everything" (the same
whole-block-replace strategy `useCollabPage.ts` already relies on), so
treating the most recent `InsertText.text` as a block's current full
content reproduces the same end state a true anchor-resolving replay
would, without duplicating `internal/anchor`/`internal/doctext` across a
module boundary. Structural ops decode via `documentcore.UnmarshalOp`
directly — shared, not reimplemented.

`[[Page Title]]` links are scanned from each block's plain text (a
regex, since `internal/doctext` has no real mark storage yet — RFC-003
§2's `PageLink` mark doesn't exist as a mark, only as this text pattern);
`target_page` resolves via a case-insensitive title lookup against this
service's own `docs.pages`, arbitrarily-but-deterministically picking the
lowest id if titles collide (duplicate-title diagnosis is a separate,
unbuilt concern), and stays `NULL` (dangling) otherwise.

**Verified two ways.** A new integration test suite
(`blockproj_integration_test.go`, real Postgres via testcontainers, five
tests) covers insert→text materialization, kind-change + move ordering,
delete, link resolution *and* dangling, and — the one that matters most
for a projector — **rehydration after a simulated process restart** (a
second, independent `Projector` against the same database must pick up
exactly where the first left off, proving the lazy-load-from-`docs.blocks`
path works, since this service can never read `collab.ops` directly,
ADR-003). All five passed. Then verified against the **real running
stack**: created two real pages through `api-gateway`, submitted a block +
text op with a `[[Backlink Target]]` link through `collaboration-service`
via a throwaway WS client, and confirmed both `docs.blocks` and a
**resolved** (non-dangling) `docs.page_links` row appeared.

**A real bug the live check caught that the integration tests
couldn't:** the first live attempt produced nothing at all — outbox rows
existed in `collab.outbox` but `published_at` stayed `NULL` forever, and
neither service logged an error. Cause: `collaboration-service`'s Docker
image had never been rebuilt after the outbox poller code was added (it
was last rebuilt for the earlier WAL-format-reset entry, before this
one) — the running container simply didn't have the poller in it yet.
Fixed with `docker compose build collaboration-service && docker compose
up -d --no-deps collaboration-service`; the whole backlog of previously
unpublished events flushed through immediately once the correct image
was running. **Both `document-service` and `collaboration-service`'s
Dockerfiles now build from the repo root** (mirroring `api-gateway`'s
own) since both import `marginal/documentcore` across a module boundary
now — `document-service`'s hadn't needed that before `blockproj` existed.

**Still open:** no REST endpoint yet exposes `docs.blocks`/backlinks to
the frontend (`InspectorRail`'s Backlinks tab is still the honest
empty-state placeholder from the rich-editor pass) — the materialization
pipeline is real and tested, but nothing reads it yet. Also still open
from earlier entries: live presence, the left rail's nested page tree,
and general editor polish.

---

## 2026-08-26 — Backlinks: the read path, closing out the feature end-to-end

The previous entry's materialization pipeline was real but unread —
this closes that gap. `PageService` gained a seventh RPC,
`ListBacklinks` (`docs/api/pages.md`'s six-RPC contract plus one: it
lives here only because `docs.page_links` is in this service's own
database, not because it's page metadata like the rest of the set —
the proto's own comment says so). Reads `docs.page_links` via a new
`pages.Repo.ListBacklinks` method (a separate `blockrepo.Queries` call
against the same pool `pages.PostgresRepo` already holds — a different
sqlc package, `internal/blockrepo`, from this file's own `pagerepo`),
scoped by ownership the same way `GetPage` is (`Repo.Get` first, so a
backlink list isn't public just because a page id is known).

`api-gateway`'s `pagesrest` gained `GET /pages/{id}/backlinks` and a
`backlinkJSON` conversion — a real test (`TestBacklinksTranslatesRequestAndResponse`)
using the same hand-written-fake-gRPC-server pattern every other handler
test here already uses.

Frontend: `api/pages.ts` gained `getBacklinks`; `InspectorRail`'s
Backlinks tab is no longer the honest-placeholder — it's a real
`BacklinksPanel` fetching on mount/page-change (plus a manual refresh
button, since materialization runs async through the outbox/NATS
pipeline, not instantly on keystroke) and rendering real rows, including
a "deleted" pill for a backlink from a since-removed page (matching
`editor.html`'s own mockup row for exactly this case).

**Verified against the real running stack, not just the unit test**:
created two fresh pages through `api-gateway`, submitted a block + text
op with a `[[REST Target]]` link through `collaboration-service`, waited
for the async pipeline, then read it back through the actual
`GET /pages/{id}/backlinks` REST call (not a direct DB query this time)
— got back exactly the expected JSON, confirming the whole chain end to
end: WS → NATS → projection → gRPC → REST → JSON.

**Still open:** live presence, the left rail's nested page tree, and
general editor polish (slash menu, drag reorder, marks) — unchanged from
the previous entry.

---

## 2026-08-26 — Live presence: real join/leave, not an op-broadcast heuristic

Closes the first of the three items the previous entry left open.
`docs/api/collaboration.md`'s WS contract had no join/leave signal at
all — `useCollabSocket`'s old "peers" set only ever grew from actor ids
seen submitting an op, meaning someone who joined and never typed was
invisible, and someone who left was never removed. Real presence now
exists, end to end.

**`internal/session`:** `Subscriber` gained `DeliverPresence(e
PresenceEvent)` alongside the existing `Deliver`. `Subscribe`'s signature
changed to `Subscribe(actorID uuid.UUID, sub Subscriber) (subID uint64,
present []uuid.UUID, unsubscribe func())` — tracks `presence
map[uuid.UUID]int` (connection count per actor, not just per subscriber),
so a second browser tab from the same person doesn't look like a second
person: only the *first* connection for an actor broadcasts "joined,"
only the *last* closing broadcasts "left." `present` is every actor
already here at join time, so a client doesn't have to wait for a future
event to learn who's already on the page. Two new session-level tests
(`TestSubscribeReportsAlreadyPresentActorsAndBroadcastsJoinLeave`,
`TestSubscribeIgnoresASecondConnectionFromTheSameActor`) pin both the
join/leave broadcast and the multi-tab-dedup property directly.

**`internal/wsapi`:** `serverMessage` gained `Present []string` (on the
initial "snapshot" only) and `ActorID`/`Joined *bool` (on a new "presence"
type). Two new wire-level tests
(`TestPresenceJoinAndLeaveNotifyOtherConnections`,
`TestPresenceIgnoresASecondConnectionFromTheSameActor`) exercise this
over real WebSocket connections — the second one needed a small new test
helper, `startReader`/`requireFrame`/`requireNoFrame`, because
asserting "nothing arrives within N ms" can't use a context-deadline
`wsjson.Read` directly: `coder/websocket` treats a context
cancellation/timeout *during* Read as fatal to the connection (it closes
the socket to unblock the blocked read), so a deadline-bearing probe read
would permanently break the connection for every later assertion in the
same test. Reading continuously into a channel from one dedicated
goroutine per connection and racing that against a plain timer avoids it.

**Frontend:** `useCollabPage`'s `peers` is no longer inferred from op
broadcasts — seeded from the snapshot's `present` list, updated by the
new `"presence"` message type. `InspectorRail`'s People tab (already
real) now shows genuine live presence — "here," not "editing," since
someone can be present without having typed anything yet.

This was a **purely additive wire-format change** (new optional fields,
new message/type variants — nothing existing changed shape), unlike the
`pageop` rewrite two entries back — no storage reset was needed this
time, and the two standalone smoke-test scripts (flat-text and
structural) both still passed against the rebuilt image without
modification to their core assertions (the structural one needed its
generic `read()` helper to skip past interleaved presence frames, since
it opens fresh "verify" connections mid-test with new actor ids — not a
product bug, just the test script's own read loop needing to tolerate a
message type it didn't emit before).

**Not built:** cursor position or "paragraph 4"-style location within
presence (`docs/ui-mockups/editor.html`'s own aspiration) — this is
"someone is on this page," not where. **Still open:** the left rail's
nested page tree and general editor polish (slash menu, drag reorder,
marks).

---

## 2026-08-26 — The left rail: a real nested page tree

Closes the second of the three items the previous entry left open. No
backend changes at all — `CreatePage`/`ListPages`/`RenamePage`/
`ReparentPage`/`DeletePage` already supported everything this needed;
this pass is entirely `web/`.

**`web/src/screens/usePageTree.ts`** (new): lazily loaded, one
`ListPages` call per expanded node — `ListPages` is a filter (direct
children of one parent), not a subtree walk (`pages.md` § List: "It is a
filter, not a subtree walk"), so there is no "fetch everything" call to
make in the first place. State is a flat `nodes: Record<id, Page>` plus
`childrenByParent: Record<parentId, id[] | undefined>` (`undefined` =
never fetched) rather than a nested tree of objects, which keeps updates
after create/delete/reparent simple (patch one array, not walk a tree).
Exposes `createChildPage`, `renamePage`, `deleteNode`, `moveNode` — the
last of these is a reparent that, after the server call succeeds, refetches
*both* the old and new parent's children rather than computing the new
order client-side (`internal/sortkey`'s fractional-index recomputation is
document-service's job, not this hook's to guess at).

**`web/src/screens/PageTreeRail.tsx`** (new, replaces `EditorScreen`'s
flat page list): the real tree — twisty expand/collapse, per-node "new
sub-page"/"delete" buttons, a filter box, and drag-and-drop reparent/
reorder via native HTML5 DnD (three drop zones per row: top quarter =
"insert before," middle half = "make a child of," bottom quarter =
"insert after" — dropping computes the right `parent_id`/`after` pair for
`ReparentPage` from `usePageTree`'s own `childrenByParent` ordering).
`DashboardScreen`'s existing grid is unaffected other than a clarifying
comment — it already only ever listed root pages (an accident before
nesting existed, since every page used to be a root; now deliberate:
sub-pages are reached by drilling into the tree, not listed flat on the
dashboard).

**Two real, stated limitations, not oversights:**
- **The filter box only searches what's already loaded/expanded.**
  There's no "search everything" call available (`ListPages` can't do
  it, and `search-service` is out of this repo's scope per the stack
  table's own "Search — deferred" line) — this is a lighter-weight
  convenience over the currently-visible tree, not real search.
- **No "Recently deleted" section**, unlike `editor.html`'s own mockup —
  `pages.md` § Delete is explicit that soft-deleted pages never appear in
  `ListPages` and there is no separate endpoint to list them at all.
  Building that would need a new backend RPC this pass didn't add; noted
  here rather than faked with a client-side "pages I just deleted in this
  session" list, which wouldn't reflect deletions from another device.

**Verified against the real running stack** (no backend rebuild needed —
nothing server-side changed): every exact API call the new hook/component
make — `POST /pages` with `parent_id`, `GET /pages?parent_id=`,
`PATCH /pages/{id}/parent` with `{"parent_id":""}` (promote to root),
`PATCH /pages/{id}/title`, `DELETE /pages/{id}` — run via `curl` against
the live `api-gateway`, all correct. `tsc --noEmit` and `oxlint` both
clean (same pre-existing warnings as before, no new ones).

**Still open:** general editor polish — a floating "/" slash menu,
drag-to-reorder for *blocks* within a page (the tree's own drag-and-drop
is for *pages*, a separate concern), and inline marks (bold/italic/
link — still blocked on `internal/doctext` gaining real mark storage).

---

## 2026-08-26 — Editor polish: slash menu, block drag-to-reorder; and a real ADR-001 reversal on `Register`

**Slash menu and block drag-to-reorder** (`RichEditorPane.tsx`): typing
`/` at the end of a block's text opens a floating kind picker
(`.slash`, new CSS reusing `.palette-item`), positioned below the block
rather than at the caret (computing a caret's screen position inside
`contentEditable` is real complexity for little benefit over anchoring to
the block's own rect). Choosing a kind strips the trailing `/` from the
*live DOM text*, not from `blocks` state — text sync is debounced, so
`blocks` wouldn't have the just-typed `/` yet at trigger time. Each
block's toolbar gained a drag handle (⠿); dropping on another block's top
or bottom half reorders via the same `MoveBlock` op the page tree already
uses for pages, now exposed from `useCollabPage` as `moveBlock`.

## A real, user-requested reversal: `Register` is no longer bootstrap-only

The user hit "instance already claimed" trying to log a second person in
(really: they'd clicked the wrong tab — "Fresh instance" instead of
sign-in, an honest mix-up this session's own copy invited), and after
using the `seeduser` CLI workaround, said directly: **build real,
separate registration — like Google Docs, not multi-tenant, just more
than one person able to get their own account.** This reopens `ADR-001`'s
"self-hosted, invitation-only, no public sign-up" clause specifically for
`Register` — a deliberate architecture reversal, not a bug fix, done at
explicit user request exactly the way every other scope reopening this
session was handled: named plainly, then built.

**`authservice.Register`** no longer runs under `pg_advisory_xact_lock`
or checks `count(*) > 0` — it just hashes, inserts, issues tokens, writes
the same `auth.user_registered` outbox event as before. Uniqueness is
`auth.users`' own `UNIQUE(email)` constraint (`users.Insert` already
mapped a collision to `ErrEmailTaken`before this pass; it just could never
be reached, since the old bootstrap check always fired first on a
call that could reach the insert). Removed as dead code alongside the
gate itself: `ErrInstanceAlreadyClaimed`, `bootstrapLockKey`,
`users.Count`/`CountUsers` (no other caller). **`services/auth-service/cmd/seeduser`
deleted** — the whole reason it existed was to bypass a gate that no
longer exists; a real user now just registers normally.

**A real, separate bug this surfaced:** `api-gateway`'s `apierror.go`
had no mapping for `codes.AlreadyExists` at all — it fell through to the
default case and returned `500 internal_error` for what should be a
`409 conflict`. `ErrEmailTaken` had a comment saying as much already
("kept for completeness, not currently a live path") because the old
Register could never actually reach it in practice. Fixed by adding the
mapping; verified with a real duplicate-email registration attempt
against the live stack — `409 {"error":"conflict","message":"email
already registered"}`, not a 500.

**Frontend:** `AuthPage`'s "Fresh instance"/"Claim this instance" mode
replaced with an ordinary "Register"/"Create your account" mode — no more
administrator framing, no more "registration closes behind it" copy,
password minimum corrected from the old bootstrap flow's 12-character
hint down to the 8 actually enforced (`domain.NewPassword`).

**Verified against the real running stack** (rebuilt `auth-service` +
`api-gateway`): a genuine third account (`third@example.com`) registered
successfully after two pre-existing accounts already existed — impossible
under the old model — and a duplicate-email retry correctly got `409`,
not a crash. Integration tests replaced to match: `TestRegisterIsBootstrapOnly`
→ `TestRegisterAllowsMultipleDistinctUsers` (+ new
`TestRegisterRejectsDuplicateEmail`); `TestConcurrentRegisterClaimsOnlyOneAdmin`
→ `TestConcurrentRegisterWithDistinctEmailsAllSucceed` (all N now succeed,
not just one) + new `TestConcurrentRegisterWithSameEmailAllowsOnlyOne`
(the one concurrency property that must still hold — the UNIQUE
constraint, not an advisory lock, is what now serializes it). All four
pass against real Postgres. Full workspace build/vet/race-test and
`tsc`/`oxlint` all clean.

**Still open:** inline marks (bold/italic/link) — the one item left from
the original "notion-like editor" ask, and the only one that's a real,
sizeable lift: `internal/doctext` has no mark storage, and even a
scoped-down version (marks via whole-block `SetBlockContent` replace,
skipping live per-keystroke mark sync) still needs a rich `contentEditable`
renderer with cursor-position-safe re-rendering and selection-to-byte-offset
mapping — the risky part, not the backend. Not started; worth scoping
with the user before attempting rather than rushing it.

---

## 2026-08-26 — Inline marks, a bubble menu, and an insert-element bar: the editor is feature-complete for this pass

The user asked to finish the editor to match `editor.html` and then stop —
this closes the one item every earlier entry called "the risky part."

**A real backend gap found and fixed first**: `documentcore.Page.Apply`
already stored a block's whole `Content` (text *and* marks together) when
a `SetBlockContent`/`InsertBlock` op landed — the marks were never lost in
`collaboration-service`'s own memory. But `session.BlockSnapshot` only
ever exposed `Text`, never `Marks`, so a mark would silently vanish the
moment a client reconnected, even though the session itself still had it.
Fixed by adding `Marks []documentcore.Mark` to `BlockSnapshot`, populated
from `s.page.Blocks[i].Content.Marks` in `Snapshot()` — safe to read
alongside the rope's live text because nothing else in the session ever
touches marks except the same op that changes them. New test
(`TestSnapshotIncludesMarksSetViaSetBlockContent`) pins it; full suite
still green.

**The design tradeoff, made explicit rather than discovered later**:
marks can only travel on a `SetBlockContent` op (`internal/doctext`'s live
rope has no mark storage of its own — nothing else in the wire protocol
can carry `Content.Marks` at all). So **once a block has any mark, every
future edit to it — including plain typing — routes through
`SetBlockContent` instead of the fast anchor-based `InsertText`/
`DeleteText`**, because a Text op has no way to carry marks and would
silently strand them. An unmarked block keeps full real-time
character-level CRDT merging exactly as before; a marked block trades
that for whole-block last-write-wins (still with `SetBlockContent`'s own
`Prev` precondition, so a genuine conflict is *rejected*, not corrupted —
the same class of tradeoff `DeleteBlock`/`MoveBlock`/`SetBlockKind`
already accept elsewhere in this codebase, not a new risk).

**`web/src/collab/marks.ts`** (new): a hand-ported mirror of
`documentcore.Content`'s `AddMark`/`RemoveMark` split-or-merge algorithm
(same case split: disjoint / fully covered / hole-in-the-middle / edge
trim), plus three pieces documentcore doesn't need because it isn't a
live editor: `isFullyMarked` (toggle-button state), `shiftMarksForEdit`
(a plain common-prefix/common-suffix diff — marks entirely in the
untouched prefix or suffix survive, shifted; anything overlapping the
actually-changed middle is dropped rather than guessed at — a real,
accepted limitation, not full rich-text-editing correctness), and
`renderMarkedHTML` (text+marks → escaped, self-contained HTML — the only
markup it ever emits is its own fixed tag set, text content is always
escaped, link `href` is attribute-escaped). 17 new unit tests, including
one asserting a `javascript:`-URL link mark can't inject markup. Known,
stated gap: offsets are JS string indices (UTF-16 code units), not the
byte offsets `documentcore` actually persists — identical for ASCII,
off-by-a-little for multi-byte text; not fixed this pass, same class of
simplification `doctext`'s own byte/rune note already accepts.

**`RichEditorPane.tsx`**: `EditableTextBlock` now renders
`renderMarkedHTML(text, marks)` into `innerHTML` instead of plain
`textContent` (still skipped while the block is actively being typed
into, the same `editingRef` guard as before — nothing new needed there).
A `selectionchange` listener raises a bubble menu (Bold/Italic/Strike/
Code/Link) on any non-collapsed selection inside a `.editable` block —
never inside a `code_block`, which structurally isn't `.editable` at all,
so `editor.html`'s "bubble menu suppressed inside code" rule holds for
free. `rangeToTextOffsets` walks the block's text nodes with a
`TreeWalker` to convert the browser's DOM `Range` into the plain-text
character offsets marks are stored in. Clicking a button toggles the mark
over the selection (`isFullyMarked` decides add vs. remove) and sends one
`SetBlockContent`.

**The insert-element bar**: a persistent "+" in every block's toolbar
(alongside the existing drag handle, kind selector, and delete), opening
the same kind-picker popup the "/" trigger already used — but inserting a
*new* block after the current one instead of converting it. Both now
share one `kindMenu` state (`{mode: "convert" | "insert", ...}`) and one
`chooseKind` handler.

**Verified**: 17 new Vitest unit tests for `marks.ts` (full suite: 21
passed) traced by hand against the exact segment/tag-order logic, plus a
new standalone Go WS smoke test
(`/private/tmp/.../scratchpad/wssmoke/marks`) against the real rebuilt
`collaboration-service` — insert a block, apply a bold mark via
`SetBlockContent`, then open a **fresh** connection and confirm its own
snapshot carries the mark with the right range and kind. Passed on the
real running stack, not just in a unit test. Full Go workspace build/vet/
race-test and `tsc`/`oxlint` both clean; no storage reset needed (a
purely additive wire-format change, same as the presence pass — new
optional field, nothing existing changed shape).

**Not built, stated plainly**: page-link marks (`[[Page Title]]` as a
real inline mark, distinct from `internal/blockproj`'s plain-text-regex
backlink scan — RFC-001's `PageLink` mark kind exists in `documentcore`
but nothing in the bubble menu offers it), and multi-byte-safe mark
offsets. This closes the editor work for this pass — full stop, per the
user's own framing.

---

## 2026-08-26 — Second real bug, found via live multi-user testing: `document-service` still owner-scoped every page

The user registered a second real account and reported it directly:
their page list showed "No pages yet" and a shared link sat on "Loading…"
forever. Root cause, confirmed by reading the code rather than guessing:
`document-service`'s `pagerepo/queries.sql` still filtered every one of
`GetPage`/`ListPages`/`RenamePage`/`ReparentPage`/`SoftDeletePage` on
`created_by = <actor>` — the exact per-creator private-ownership model
[Register's own reversal](#a-real-user-requested-reversal-register-is-no-longer-bootstrap-only)
earlier this session had already established was wrong for this product
("like Google Docs, not multi-tenant, just more than one person able to
get their own account"). A second account's `actor-id` structurally can
never equal the first account's `created_by`, so every page the first
user made 404'd for anyone else — silently, by design of the *old* model,
which is exactly what made it look like an empty-state bug rather than an
authorization one.

**Fix, same shape as the Register reversal**: stripped every
`created_by`/owner filter out of `internal/pagerepo/queries.sql` (kept
`created_by` as a selected/stored column — authorship is still recorded,
just no longer an access filter) and regenerated with `sqlc generate`.
`internal/pages/repo.go`'s `Repo` interface lost the `owner uuid.UUID`
parameter on `Get`/`List`/`Rename`/`Reparent`/`Delete` entirely (`Create`
is unaffected — it still records `np.CreatedBy` as authorship).
`internal/pages/api.go`'s six call sites updated to match; each handler
still calls `actorID(ctx)` and returns its error unchanged, so an
unauthenticated caller is still rejected — only the *ownership* check is
gone, not the *authentication* one. `ListBacklinks`'s `Repo.Get` call,
previously an ownership gate, is now a plain existence check (its own
updated doc comment says so).

**Tests**: `internal/pages/repo_integration_test.go`'s
`TestPagesAreScopedToTheirOwner`/`TestCreateCannotNestUnderAnotherActorsPage`
(which asserted the now-wrong behavior) replaced with
`TestPagesAreVisibleToEveryActor`/`TestCreateCanNestUnderAnyExistingPage`,
asserting the opposite: any actor's `Get`/`Rename`/`Reparent`/`Delete`
succeeds on any page, `List` returns every page on the instance, and
`Create` can nest under or position after a page it didn't make. Every
other integration test in the file had its now-unused `owner`/`other`
argument dropped from the `Repo` call sites (mechanical — `Create` still
takes `CreatedBy` unchanged). Full `internal/pages` and `internal/blockproj`
integration suites pass against real Postgres via testcontainers-go
(`go test -tags=integration`), not mocked.

**`docs/api/pages.md`** rewritten to match — the old "every page read or
write is scoped to its `created_by`" section now documents the reversal
itself, the same way `docs/api/auth.md` documents Register's. User story
A-04 ("only I can read or change my pages") no longer holds as written;
not edited in `USER_STORIES.md` itself, same precedent as A-01 after the
Register reversal — this file records intended-at-design-time behavior,
current reality lives here.

**Verified against the real running stack**, not just integration tests:
rebuilt and redeployed `document-service`'s Docker image
(`docker compose up -d --build document-service`); with the real
`tester2@example.com` account (registered earlier this session) sending
`X-Actor-Id` for its own id, `GET /pages` returned all 7 pages on the
instance, including 6 created by the first real account and 1 by a
third — confirmed by cross-checking `docs.pages.created_by` directly
against `auth.users` — and `GET /pages/{id}` on one of the first
account's pages returned it in full, not `404`. `api-gateway` needed no
changes — it's a pure REST↔gRPC shim and this reversal is entirely
document-service-side.

---

## 2026-08-26 — Three real bugs found live in one sitting: block-toolbar overlap, cursor reset, dropped edits

The user reported three things back-to-back while trying the editor:
"whats this. make it like notion" (a screenshot of the always-visible
block toolbar — "+", drag handle, a `<select>`, "×" — sitting in the
prose itself), then "i cant type this thing is on top" once the toolbar
was actually blocking clicks into the text, then "cursor reset to start
of line" while typing, then "backlinks not working." All four turned out
to share one root: `RichEditorPane.tsx`'s per-block toolbar and its
per-block debounced-text sync, neither of which had been exercised by a
real browser session before. Verified throughout with headless Chromium
(Playwright, launched ad hoc — not a committed test suite) driving the
actual `docker compose` stack as the real `tester2@example.com` account,
not unit tests: log in through the real form, click into real
`contentEditable` blocks, read back real Postgres rows after each step.

**Bug 1 — the toolbar physically overlapped the text it sat next to.**
`.block-toolbar` was `position: absolute; left: -84px` (a left-margin
gutter, like Notion's), but the row it held — "+", a drag handle, a
`<select class="block-kind-select">`, and a "×" button — measured out to
~146px wide (two 20px icon buttons, a 74px-max-width select, three 4px
gaps). Since 146 > 84, the toolbar's *own right edge* landed 62px inside
the block's own content box, eating clicks meant for typing — exactly
"this thing is on top." Fixed by finishing the "make it like notion"
redesign that was already the plan (deferred earlier this session for
this exact bug report to take priority): the persistent `<select>` and
"×" are gone from the always-rendered row entirely, leaving only "+"
(insert) and "⠿" (drag handle). Dragging the handle still reorders;
*clicking* it (no pointer movement, so the browser never fires
`dragstart`) now opens the same kind-picker popup "/" and "+" already
use, with a "Delete" row appended (`kindMenu`'s union gained a third
`"handle"` variant carrying `{blockId, top, left}`). Two icon buttons is
~44px — comfortably inside the 84px gutter with room to spare. Verified:
`.block-toolbar`'s right edge now measures 40px *before* the editable
block's left edge, not 62px past it.

**Bug 2 — typing, then a caret jump to the start of the line.**
`EditableTextBlock` renders marks as real HTML (`innerHTML`), guarded by
an `editingRef` meant to skip re-render while the user is mid-edit — but
the ref was cleared back to `false` the instant the debounced
`onChangeText` fired, not on blur. The debounce fires 400ms after the
user's *last* keystroke — the common case, not an edge case — so the
guard was already down by the time the server's own echo came back and
changed `text`, and the next render reassigned `innerHTML` even though
the content was already correct, which unconditionally collapses a
contentEditable's caret to offset 0. Traced by instrumenting a real
Playwright session: typed `"AAA"`, read `selection.getRangeAt(0)
.startOffset` (27, correct), waited past the debounce, read it again (0
— the jump), typed `"BBB"`, and watched it land at the front instead of
the end. Fixed by replacing the timing-based guard with a real one: skip
the `innerHTML` write whenever `ref.current === document.activeElement`,
full stop. A focused block never gets clobbered — not by its own echo,
not by a remote peer's edit either, which is the same last-write-wins
call `SetBlockContent`'s marked-block tradeoff already makes. Verified:
the identical trace now holds the caret at the typed offset straight
through the debounce round trip, and typing two bursts with a pause
between them produces the exact expected string, not an interleaved one.
`CodeBlockField`'s parallel `editingRef`-timing guard (same shape, a
controlled `<textarea>` instead of raw `innerHTML`) was fixed identically
for consistency, on suspicion rather than a direct repro.

**Bug 3 — "backlinks not working" was really a dropped-edit race, not a
backlinks bug.** Reproducing "type `[[Page Title]]`, do something else"
end to end (a fresh page via the real REST API, typed into via a real
browser, `docs.page_links` read directly) showed the backlinks feature
itself — `blockproj`'s regex projection, the `GET /pages/{id}/backlinks`
endpoint, the Inspector panel — working correctly when the edit actually
reached the server. It often didn't: the block-text debounce has no
flush-before-teardown of any kind, so typing a link and then clicking a
different page inside the 400ms window drops it, silently, no error
anywhere. The first fix attempt — flush the pending edit in a
`useEffect` cleanup on unmount — turned out to fire too late: switching
pages runs `useCollabPage`'s own `[pageId]`-dependent effect first
(closes the old socket, empties `liveRef`/`orderRef`, points `socketRef`
at the new page, *synchronously*, all before this block's parent ever
re-renders with an empty `blocks` array), and only *that* re-render
unmounts the block — by which point the flush has nothing valid left to
send into. Confirmed by reproducing with a raw `page.goto()` (still
failed) and then with an actual left-rail click, both empty. The fix
that actually works: flush on `onBlur` instead of (in addition to) on
unmount — a DOM blur fires synchronously the moment focus moves to
whatever was clicked, which is *before* React processes the click that
changes pages, so the send still reaches the still-live old session.
Confirmed by tracing raw WebSocket frames: the `InsertText` op went out
5ms after a scripted `blur()` call, not after the 400ms timer. End to
end: typed `[[Target Page XYZ]]`, blurred immediately, and — after the
normal WAL-flush (200ms) → outbox-poll (500ms) → `blockproj` pipeline
latency, not a bug, just physics — `docs.page_links` had the resolved
row and the target page's Backlinks tab showed it.

**Also noticed, not fixed this pass:** one pre-existing page (`test`,
from earlier in this session) fails to open at all —
`collaboration-service` logs "anchor refers to an item this text never
saw" on every connection attempt, a WAL/replay corruption from before
today's changes, unrelated to any of the three bugs above. Left alone;
worth a dedicated look if it recurs on a page that matters.

Full `tsc --noEmit` and `oxlint` clean on every change. All test pages
created during this verification pass were deleted via the real
`DELETE /pages/{id}` endpoint afterward, not left cluttering the
account.

**Same pass, one more real gap closed: the page title had no editing UI
at all.** `RenamePage` (gRPC + REST) and `usePageTree`'s `renamePage`
wrapper both existed and worked, but nothing in any screen ever called
the wrapper — `RichEditorPane` rendered `page.title` as a plain, static
`<h1>`. Added `PageTitle`, a contentEditable h1 with the same debounce/
blur-flush/focus-guard shape `EditableTextBlock` now uses (Enter blurs
instead of inserting a block; an emptied title is never sent, since
`validateTitle` rejects empty server-side and the title just reverts to
its last confirmed value on blur). `EditorScreen` wires it to the raw
`renamePage` API call and updates its own `activePage` state on success —
`PageTreeRail` runs its own separate `usePageTree` instance and won't
pick up the new title until it next reloads that part of the tree, a
known, accepted staleness rather than plumbing cross-hook state sharing
for this pass. Verified live: renamed a page through the UI, confirmed
the new title in `docs.pages` directly, and confirmed it survives a full
page reload.

---

## 2026-08-26 — Live cursor tracking: real caret + selection, not just presence

User request, and a real reversal of a line this doc used to carry
verbatim: `collaboration.md` said "presence answers 'who's here,' not
'where'" as a stated, deliberate scope cut. Asked directly for "cursor
tracking and currently editing users" — given a choice between a small
per-block "someone's editing this" badge and the full Notion-style live
caret/selection, picked the full version.

**Backend (`collaboration-service`)**: `session.Subscriber` gained
`DeliverCursor(e CursorEvent)` alongside `DeliverPresence`; `CursorEvent`
is `{ActorID, BlockID *documentcore.BlockID, Start, End int}` —
`BlockID` nil means "not focused anywhere." Purely ephemeral, the same
treatment `PresenceEvent` already gets: never touches the WAL, `page`,
`blocks`, or `clock`. `Session` gained a `cursors map[uuid.UUID]CursorEvent`
alongside its existing `presence` map, a `SetCursor` method
(fire-and-forget, no ack, broadcasts to every subscriber except the
sender), and `broadcastCursorLocked` mirroring `broadcastPresenceLocked`.
`Subscribe`'s signature grew a `cursors []CursorEvent` return value —
every already-present actor's last-known cursor, seeded into the
snapshot the same way `present` already seeds who's here — and its
`unsubscribe` closure now clears the departing actor's cursor (broadcasts
a `BlockID: nil` event) in the same moment it broadcasts their leave, so
a gone actor's caret can't linger. `wsapi`: `clientMessage` gained a
`"cursor"` variant (`cursorPayload{BlockID, Start, End}`); `serverMessage`
gained `Cursors []cursorWire` (on `"snapshot"`) and `Cursor *cursorWire`
(on `"cursor"`); `readLoop` dispatches `"op"`/`"cursor"` via a switch now
instead of an if-else. Two new session-level tests
(`TestSetCursorBroadcastsAndSeedsLaterJoinersSnapshot`,
`TestUnsubscribeClearsTheLeavingActorsCursor`) lock in: broadcast excludes
the sender, a later joiner's `Subscribe` is seeded with still-current
cursors, clearing a cursor removes it from future joiners' seeds, and an
actor leaving clears their own cursor for everyone still there. Full
`go build`/`vet`/`test -race` clean across the whole module, including
existing `wsapi`/`session` suites (fake `Subscriber` in
`fakerepo_test.go` gained `DeliverCursor` + `cursorSnapshot()`, matching
its existing `presenceSnapshot()` pattern).

**Frontend**: `types.ts` gained `CursorWire` and the `"cursor"` variants
on both `ServerMessage` and `ClientMessage`. `useCollabPage` gained a
`cursors: Map<actorId, PeerCursor>` (seeded from the snapshot, updated on
every `"cursor"` frame, cleared on a peer's `"presence"` leave as a
belt-and-suspenders alongside the server's own clear) and a `setCursor`
function. `RichEditorPane`'s existing `selectionchange` listener — which
already resolved "which `.editable` block, what offsets" for the bubble
menu — now reports cursor position on *every* caret move, not just a
real (non-collapsed) selection; a new `offsetsToRange` (the inverse of
the existing `rangeToTextOffsets`) turns a peer's reported offsets back
into a DOM `Range` for rendering. A `code_block`'s plain `<textarea>`
isn't reachable through `window.getSelection()` at all, so
`CodeBlockField` reports its own cursor directly off `onSelect`/`onClick`/
`onChange`, clearing on blur.

**Rendering**: a `useEffect` recomputes every peer's on-screen caret/
selection whenever `cursors` or `blocks` changes (DOM measurement has to
run after commit) into `peerCarets: Map<actorId, {rects, caretRect}>`,
rendered as `position: fixed` overlays (`.peer-caret`, `.peer-selection`)
— the same viewport-relative-coordinates convention `.bubble`/`.slash`
already use. One shared colour (`--violet`) for every peer, distinguished
by an initials tag, not a per-person rainbow palette — this design
system treats "another person" as one colour everywhere else (`.avatar.peer`),
and a new arbitrary colour set would contradict that stated intent.
**Named gaps, not silently faked**: a `code_block` peer shows in the
header avatar list (still "present") but gets no inline caret —
`<textarea>` selection has no `getClientRects()` equivalent; no
scroll/resize listener recomputes a peer's caret position, so it can
drift a little between their own next move and a window resize; display
name is actor-id initials, not `auth-service`'s real `display_name`
(discovered this exists — `RegisterRequest.display_name` — while testing;
wiring per-peer name lookups is a separate, out-of-scope-for-this-pass
enhancement).

**Verified live**, not just unit tests: two real browser contexts (two
separately registered accounts, `tester2@example.com` and
`cursortest2@example.com`) on the same page via Playwright, driving the
real rebuilt `collaboration-service`. One real bug caught this way: a
collapsed cursor (`start === end`) still rendered a zero-width
`.peer-selection` highlight, because `Range.getClientRects()` on a
collapsed range can return one zero-width rect instead of an empty list
in Chromium — fixed by skipping `getClientRects()` entirely when
`start === end`. After the fix: B selecting 5 characters shows up on A's
screen as a real violet highlight with a floating initials tag at
exactly B's selection; B collapsing to a plain caret correctly drops the
highlight but keeps the caret; B blurring out of the block entirely
correctly removes the caret altogether. Full `tsc --noEmit`/`oxlint`
clean. Test pages and accounts created for this verification were left
in place only where reused across passes; the throwaway page was deleted
via the real `DELETE /pages/{id}` endpoint afterward.

---

## 2026-08-26 — `docs/porting/PORTING_GUIDE.md` rewritten against the actual finished MVP

User request: write the porting guide's next real pass, from where the
Rust port should start given everything actually built by this point —
the doc still read as if only `documentcore` existed (Phase C of the
original plan), never updated as `document-service`/`auth-service`/
`collaboration-service`/`notification-service`/`api-gateway`/`web/` all
landed since.

Rewritten with: an honest "Status" section pointing at `CLAUDE.md`'s own
"editor is feature-complete" section and this doc's own dated entries
rather than re-deriving them; a new, load-bearing finding — **only
`documentcore` has real golden JSON test vectors**
(`testdata/document-core/marks.json`); every other module (all of
`collaboration-service`'s CRDT/WAL/session stack, `document-service`'s
repo layer, `auth-service`'s domain logic) is pinned down only by its own
Go `_test.go` files, with no separate fixture a Rust port could consume —
the original guide's "read the golden vectors" step was written
aspiratively for a world where every module got that treatment, and it
didn't happen; a concrete **suggested port order** by actual dependency
graph (`documentcore` → the CRDT core `rope`/`anchor`/`doctext`/`ops`/
`pageop`/`oplog` → the durability shell `wal`/`flush`/`opstore`/`outbox`/
`session` → `wsapi` → `document-service` → `auth-service` →
`notification-service`/`api-gateway` in parallel → `web/` unchanged except
the wasm target), each step naming its real Rust design decisions instead
of a generic list (e.g. `Session`'s one-mutex model vs. an actor-per-page
`tokio` task; the WAL's file framing has no stdlib equivalent; which
async runtime/WS crate `wsapi` needs). Corrected one claim before
publishing: an early draft said `cmd/wasm` has its own smoke tests — it
doesn't; the real one is `web/src/document-core/wasm.test.ts`, checked
directly against the file tree before writing it down.

---

## 2026-08-26 — Real regression bug: "editing same doc parallelly not syncing"

User report, live, right after the cursor-tracking pass landed. Traced to
the cursor-jump fix from earlier this same day: `EditableTextBlock`'s
`innerHTML`-sync effect had been changed to skip the write entirely
whenever the block had DOM focus (to protect a typing caret from resetting
to offset 0). That fix was too broad — a peer who simply **clicked into**
the same block someone else was editing (focused, not even typing) never
saw the other person's edits arrive, and — worse — blurring away
afterward didn't fix it either, since a blur alone doesn't re-run an
effect keyed on `[text, marks]`; only some *later, unrelated* prop change
would ever unstick it. Reproduced directly with two real logged-in browser
contexts (Playwright) before touching any code: A types into a shared
block; B, merely focused in that same block, never sees A's text — not
even after B blurs.

**Correct fix**: never skip the write — always apply an incoming
`text`/`marks` change — but preserve the caret/selection across it instead
of avoiding it. The effect now saves the current selection's plain-text
offsets (`rangeToTextOffsets`) before reassigning `innerHTML`, then
restores them afterward (`offsetsToRange`, the same inverse helper the
cursor-tracking pass added) — but only when the block is still focused; a
blurred block has nothing local to protect. This fixes both bugs at once:
the caret no longer jumps to the start on a debounce round-trip (the
original report), and a focused peer now sees a concurrent edit live (this
report). `CodeBlockField`'s parallel guard (a controlled `<textarea>`, not
`innerHTML`) got the analogous fix: `selectionStart`/`selectionEnd` saved
in a ref before `setDraft`, restored in a second effect keyed on `draft`
that only fires right after an external sync (a local edit via
`handleChange` leaves the ref `null`, so it's a no-op there).

**Verified live**, re-running the exact two-browser-context reproduction
after the fix: B, still focused in the shared block, now sees A's edit
arrive in real time; a follow-up run of the original cursor-jump repro
(type, wait past the debounce, type more) confirms the caret still holds
its position and the final text is uncorrupted — both fixed
simultaneously, not traded off against each other. Full `tsc --noEmit`/
`oxlint` clean.

## 2026-08-26 — Full regression pass across every service, plus a benchmark baseline refresh

User request, after the sync-loss fix above: a full regression sweep of
everything (not just what today's changes touched) and a refreshed
benchmark baseline meant to stand as the actual comparison point for the
future Rust port.

**Regression**: `go build`/`go vet`/`go test -race` (unit tier) clean
across all six Go modules (`documentcore`, `document-service`,
`auth-service`, `collaboration-service`, `notification-service`,
`api-gateway`). The integration tier (`-tags=integration -race`, real
Postgres/NATS via testcontainers-go) surfaced **one real, pre-existing
data race**, unrelated to anything built this session:
`auth-service/internal/outbox`'s `TestPollerSkipsAlreadyPublishedRows`
incremented a plain `int` (`deliveries++`) from a NATS subscription
callback's own goroutine while reading it from the test's goroutine, with
no synchronization between them at all — a genuine race the detector
correctly caught, not a flaky-container false positive (confirmed by
reading the full race report: both stack frames pointed at this test's own
code, one at the callback, one at the final `require.Equal`). Fixed with
`atomic.Int32`; reran 3x under `-race` to confirm it's actually fixed, not
just no-longer-observed. Frontend regression (`tsc --noEmit`, `oxlint`,
`vitest run`) was already clean — 21/21 Vitest tests passing.

**Benchmarks**: re-running `documentcore`'s and `rope`'s existing
benchmark suites to refresh the recorded baseline surfaced two more real
issues, written up in full in `docs/porting/BENCHMARKS.md` (not
duplicated here):
- `documentcore`'s `BenchmarkPageApplyInsertBlock` had a real bug — it
  never removed a block after inserting one, so the page it measured
  against grew without bound over the whole benchmark run, and since
  `InsertBlock` is a linear scan over `Page.Blocks`, the reported `ns/op`
  swung by ~100x depending on `-benchtime` alone (a `-benchtime`-dependent
  number is not a real per-op cost). Rewritten to hold the page at a
  constant, realistic 200 blocks — insert, then immediately apply the
  op's own `Invert()` outside the timer — and verified stable across three
  very different iteration counts.
- `rope`'s benchmarks turned out NOT to have the same class of bug (`Rope`
  is immutable — `Insert`/`Delete` return a new value, so a discarded
  return value never accumulates) — but `BENCHMARKS.md`'s own
  `RopeStringManySequentialInserts` entry had a plain arithmetic mistake:
  dividing the raw per-batch `ns/op`/`allocs/op` by 1000 to get a
  per-insert figure was done inconsistently (the `B/op` division was
  right, matching a fresh remeasurement exactly; `ns/op` and `allocs/op`
  were both off by roughly 200x). Corrected in place; explicitly scoped as
  a measurement/documentation fix only — whether ~511 allocations per
  insert is itself worth optimizing in `rope` was not investigated this
  pass.

Both benchmark fixes needed the same underlying instinct: don't trust an
absolute number without checking whether it's stable under a different
`-benchtime`/iteration count first. `docs/porting/BENCHMARKS.md` now
records the Go version and exact reproduce command per entry, since a
future Rust-port comparison depends on knowing what these numbers actually
measured.

---

## 2026-08-26 — The cursor-reset bug came back: real second cause, found and fixed

User reported the caret-jump regression was back after the earlier fix, and
asked for it to be tested properly rather than patched again on a guess —
fair, given the first fix (focus-based skip) had itself caused the
"parallel editing not syncing" regression fixed earlier the same day. This
time: reproduce first, in isolation, before touching code.

**Reproduced solo** (one browser, no peer at all) with an instrumented
Playwright script dumping DOM `innerHTML`/caret offset/active element at
each step: typing a first chunk of text and waiting past the debounce
round trip held the caret correctly (offset unchanged) — but typing a
*second* chunk and waiting past *that* round trip reset the caret to 0,
even though the text itself stayed correct. The first edit never showed
the bug; only the second (and every edit after) did — the concrete clue.

**Root cause**: `setBlockText` (`useCollabPage.ts`) implements "replace
this block's whole text" (its own documented strategy) by sending a
`DeleteText` covering the block's *entire* existing range, followed by an
`InsertText` of the whole new text — but only once the block already has
prior `boundaries` from an earlier edit. The very first edit to an empty
block has no boundaries yet, so only the `InsertText` half fires — which
is exactly why the bug never showed up on the first edit and always did
from the second onward. Both ops get their own **ack** (sent only to the
sender), and `onmessage`'s "ack"/"broadcast" cases were unified into one
handler that `publish()`ed unconditionally either way. The `DeleteText`
half's ack — correctly, per that handler's "anything but InsertText means
the text is now empty" reducer, since the block genuinely *is* empty
server-side for that instant mid-replace — got published into `blocks` on
its own, re-rendering the block's contentEditable as empty for the moment
before the `InsertText` ack arrived and refilled it. `EditableTextBlock`'s
caret-preserving sync effect (from the earlier fix) saves the selection's
plain-text offsets before a content write and restores them after, but an
emptied element has no text nodes left for `offsetsToRange` to reattach a
saved offset to — the restore silently no-ops, and the caret lands at the
browser's own default for a freshly-cleared element: offset 0. Not a
CRDT/anchor bug at all — `internal/anchor`/`doctext`'s actual Peritext-style
item-id scheme (RFC-001 §9, chosen at design time specifically by reading
the Peritext paper rather than inventing the design — `RFC-001` §9/§ Resources,
`docs/architecture/lld/collaboration-service.md` §3) is untouched and
correct; this was a plain React/DOM-sync bug in how the client's own
op-confirmation was being conflated with a remote peer's broadcast.

**Fix**: split "ack" and "broadcast" into separate cases. "ack" still
updates `liveRef`'s internal `boundaries`/`text` (still needed — a future
local edit's `DeleteText` needs correct boundaries, and `deleteBlock`'s
tombstone needs correct text) but no longer publishes unconditionally; a
new `pendingTextAcksRef` (per block, how many of *this* block's own
text-scope ops are still owed an ack) coalesces a two-op replace batch
into exactly one `publish()`, once the *last* ack of the batch lands — so
`blocks` (and anything reading it, like `InspectorRail`'s Outline) still
ends up correct, just never exposed to the transient all-empty half-state
in between. A single-op edit (first edit to a block, or any edit once
`newText` is empty) behaves exactly as before — the counter reaches 0 on
the very first and only ack.

**Verified live**, not just re-running the earlier repro: the exact
solo-typing repro (now passes — offset stays at the true end through the
second round trip); a 10-round stress test, each round waiting past the
full debounce+round-trip before the next, typing distinct words — 0
failures, caret at the exact expected offset every single time, final
text uncorrupted; converting a block to a heading and editing it twice
confirms `InspectorRail`'s Outline still updates correctly (this fix's
main risk — the earlier commit message drafts skew toward "never publish
on ack," which would have silently gone stale — was caught and corrected
before landing, by tracing exactly what else reads `collab.blocks`); and
a combined two-peer test (one focused-and-idle throughout, exercising the
sync-loss fix from earlier; the other typing across two full rounds,
exercising this fix) — both peers end up with identical, correct text,
and the typing peer's own caret lands at the true end. Full `tsc --noEmit`/
`oxlint`/`vitest run` (21/21) clean. All five pages created during this
verification pass were deleted afterward via the real `DELETE /pages/{id}`
endpoint.

---

## 2026-08-26 — Real bug: peer initials collided ("why both says 01?")

Chased a reported "cursor asymmetry" (one peer's cursor not showing to
the other) through three different join-order scenarios via real
two-session Playwright tests — none reproduced an actual asymmetry; both
directions worked every time. The user's own follow-up named the real
issue directly: both peers' initials tags showed "01". Root cause: actor
ids are UUIDv7s, which front-load a millisecond timestamp (RFC 9562 §5.7)
— `tester2@example.com` and `cursortest2@example.com`, registered minutes
apart in the same session, are `01a03e22-...` and `01a03ebd-...`: identical
leading two hex digits. `actorId.slice(0, 2).toUpperCase()` (both the
header's peer avatars and the new peer-caret tag) collided for exactly
the case that matters most — two people testing together, created close
in time — making the earlier "asymmetry" report likely two carets with
the same label overlapping, not one caret missing.

Fixed with `actorTag()`: a small string hash over the *whole* id (not a
leading substring), mapped to two characters from a 36-symbol alphabet —
spreads the id's actual entropy (concentrated in its later, random bits
per RFC 9562) across both displayed characters instead of reading only
its coarse, slowly-changing timestamp prefix. Verified directly against
the two real colliding ids: `01a03e22-...` → "PI", `01a03ebd-...` → "ET".
Applied to both call sites (header avatar, peer-caret tag) — they'd
silently drifted to two different literal expressions of the same bug
before this pass gave them one shared helper. `tsc --noEmit`/`oxlint`
clean.

**Still a known, accepted gap**: this is still an id-derived tag, not the
account's real `display_name`/email (`auth-service`'s `RegisterRequest`
already has one, noted but not wired up when it surfaced during the
cursor-tracking pass) — two different real accounts can still coincidentally
hash to the same two characters (a real, if much smaller, collision
space than a plain UUID slice's). Wiring actual per-peer names would need
a `GetUser` lookup this pass didn't add.

---

## 2026-08-26/27 — Full backend code review, fixed top to bottom

A code review of every Go module aimed at idiomatic-Go/Rust-port
cleanliness turned up 7 sections of findings (collaboration-service,
documentcore, document-service, auth-service, notification-service,
api-gateway, cross-cutting); all were fixed, verified, and committed in
the order the review presented them — real bugs first within each
section. Real bugs fixed along the way: `session.open` bound a page's
Postgres-flush goroutine to the first WebSocket request's own context
(the first client disconnecting silently killed that page's flush loop
forever); `Content.Equal` replacing `reflect.DeepEqual` for
`SetBlockContent`'s precondition (nil vs. non-nil-empty `Marks` slice);
`blockproj.applyTextOp` wiping a block's projected text to empty on a
bare `DeleteText` or a real `NoOp` event, with no later event to correct
it; `wireEvent.Payload` was `[]byte` in all four copies of that envelope
(document-service, both outbox producers, notification-service) —
`encoding/json` base64-encodes a plain `[]byte`, so every NATS message's
"payload" field was an opaque base64 string instead of the readable JSON
it actually held, a real cross-language wire-format trap for the future
Rust port; `sessions.Rotate` returning a populated `RotationResult`
alongside its error on only two of several return paths, replaced with a
typed `*ReuseDetectedError` (wraps `ErrRefreshTokenReused`, still
`errors.Is`-able); every `cmd/main.go`'s outbound-request/gRPC-call path
had no timeout of its own; api-gateway had no request body size limit;
the wasm bridge indexed `args[i]` with no argument-count check, and an
unrecovered panic inside a `js.Func` callback aborts the entire wasm
module, not just one call.

Two new shared modules came out of the cross-cutting section, both by
explicit user decision on how far to take deduplication: `marginal/envconfig`
(EnvOr/RequiredEnv — byte-for-byte identical across all five
`cmd/main.go` files, zero reason to diverge) and `marginal/outboxpoll`
(the FOR UPDATE SKIP LOCKED claim-publish-mark polling loop
auth-service's and collaboration-service's own `internal/outbox`
packages each independently implemented; each service still owns its own
wireEvent shape and sqlc row type, plugged into the shared `Poller` via
`ClaimFunc`/`MarkPublishedFunc`/`BuildEnvelopeFunc` closures). Both
required converting auth-service's and notification-service's Dockerfiles
from a self-contained single-directory build context to the repo-root
context document-service/collaboration-service/api-gateway already used
for the identical reason (a cross-module import resolved through
`go.work`), plus the matching `docker-compose.yml` updates.

A user-approved "full sweep" also converted every genuine table-driven
test candidate across the backend (~15 conversions) — sibling test
functions with an identical body differing only in input/expected
output, merged into one test with a `[]struct{...}` table and `t.Run`
subtests. Left alone, deliberately: timing/attack-narrative tests,
property tests, multi-step transaction/concurrency scenarios, and
anything where forcing a table would trade clarity for a stylistic
checkbox — a plurality of the ~200 flagged test functions turned out to
already be correctly standalone, not an oversight to fix.

Verified per commit (`go build`/`go vet`/`go test -race`, plus
`-tags=integration` against real Postgres/NATS/Redis via
testcontainers-go where applicable) and, at the end, live: rebuilt and
redeployed all five services' Docker images, then ran a real end-to-end
smoke test through the actual running stack — register (exercises the
new `outboxpoll.Poller` end to end: auth outbox → NATS →
notification-service's welcome notification, confirmed via
`GET /notifications`) and a real WebSocket session against
collaboration-service (`InsertBlock` + `InsertText` → collab outbox →
`outboxpoll.Poller` → NATS → document-service's `blockproj`), confirming
the materialised block's text and kind in `docs.blocks` matched exactly
what was sent.

---

## 2026-08-27 — RFC-001 §1 nesting, Stage 1: documentcore

Implemented real block nesting in `documentcore` — `List`, `ListItem`,
`Toggle`, `Image` as real `BlockTag`s, `Quote`/`Toggle`/`List`/`ListItem`
as real containers (a `Parent *BlockID` field on `Block`, kept in
depth-first order). `InsertBlock` gained `Parent`; `MoveBlock` gained
`FromParent`/`ToParent` and now relocates a whole subtree as one unit
when the moved block is a non-empty container — the only op that can
touch more than one block per call. New invariants: no cycles
(`CycleError`), a container must be empty to delete or convert to a leaf
(`ContainerNotEmptyError`), a `ListItem`'s child must be `List` or
`Paragraph` (`InvalidListChildError`). RFC-001 §1 and `DATA_MODEL.md`'s
Blocks section (also corrected to match the real `pgx`/`sqlc`-based
schema, which had drifted from `DATA_MODEL.md`'s pre-`ADR-011` sqlx-era
sketch) are updated. Full detail, including the design reasoning, in
commit `67c903d`'s own message.

**Stage 1 only when this landed — since closed by Stage 2, same day:**
document-service's `internal/blockproj` now projects nesting too (see
next entry). `collaboration-service` needed no changes for correctness —
`session.Session` calls `documentcore.Page.Apply` directly with no
special-casing of block structure, so live nested editing already worked
end-to-end at the CRDT/session layer from Stage 1 onward; only the
read-model projection was behind, until Stage 2.

**Still open, Stage 3 (frontend) not started:**

- **`web/`'s editor** has no rendering, insertion UI, or drag-and-drop
  for `List`/`Toggle`/`Image` yet — `RichEditorPane`'s block-kind set is
  still the pre-nesting five.
- **`Image`'s `FileId`** has no backing upload/asset pipeline in this
  repo at all (stated in RFC-001 §1 itself, not a silent gap).

---

## 2026-08-27 — RFC-001 §1 nesting, Stage 2: document-service

`internal/blockproj` rewritten to hold a real `documentcore.Page` per
tracked page and call `page.Apply(op)` directly, instead of its own
hand-rolled `pageState`/`blockState` + order-tracking functions that
duplicated `Page.Apply`'s own depth-first-order/cycle/containment
bookkeeping — the package's own doc comment already said this shouldn't
happen twice. A side effect: marks are now actually projected into
`docs.blocks.content` (previously silently dropped, a real pre-existing
gap this change happens to close). New migration `00003` (not an edit to
the already-applied `00002`) adds `parent_id`/`path` to `docs.blocks`,
mirroring `docs.pages`' own adjacency-list shape one level deeper; `path`
is computed by `persist()` in one forward pass since `page.Blocks` is
already depth-first-ordered.

Verified against real Postgres (three new integration tests: parent_id/
path materialise correctly, a whole-subtree `MoveBlock` survives the
round trip, a fresh `Projector` rehydrates `Parent` from `parent_id`
after a restart) and live: rebuilt document-service's AND
collaboration-service's Docker images (collaboration-service also links
`documentcore` and needed rebuilding to pick up Stage 1 — the first live
smoke test after Stage 1 alone landed showed exactly the "flat
projection" gap the note above described, from the stale container, not
a code bug; rebuilding both fixed it), then ran a real WebSocket
InsertBlock(quote) + InsertBlock(paragraph, parent=quote) + InsertText
sequence and confirmed `docs.blocks` shows the correct `parent_id` and
an LTREE `path` extending the parent's own. Full detail in commit
`695d8d8`'s own message.

**Next: Stage 3 (frontend rendering) — not started.**

Next, in order: `blockproj` + the `docs.blocks` migration (Stage 2, the
one piece required before nested blocks are queryable/durable at all),
then frontend rendering (Stage 3).
