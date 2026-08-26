# Porting Guide — how the future Rust port should approach this codebase

This was first written while only `documentcore` existed (Phase C of the
original Go+TS plan). It's now rewritten against the actual state of the
finished Track 1 MVP — read `docs/porting/PROGRESS.md` for the chronological
blow-by-blow; this doc is the orientation layer on top of that, kept short on
purpose. The actual port happens in a new, separate repo (`ADR-011`) — this
guide is what that repo's first sessions should read before touching any code.

## Status, as of the end of the Go+TS build

**Track 1 (Documents → Auth → Collaboration) is feature-complete, end to
end** — not a skeleton, not a subset. All five services run together via
`docker compose up --build`: `document-service`, `auth-service`,
`collaboration-service`, `notification-service`, `api-gateway`, each with its
own Postgres, plus Redis and NATS. The frontend (`web/`) is a real block
editor — drag-to-reorder, slash menu, inline marks with a bubble menu, live
presence, live cursor tracking, backlinks, a nested page tree — not mockup
screens. `CLAUDE.md`'s own "Track 1 editor is feature-complete" section and
`docs/porting/PROGRESS.md`'s dated entries (there are several dozen) are the
two places that describe *what*, in detail; this guide is only about *how to
port it*.

**Stated, still-open gaps carried into the port, not hidden**: page-link
marks aren't a real inline mark kind yet (backlinks are a regex scan over
plain text); mark/cursor offsets are UTF-16 JS indices client-side, not the
byte offsets the Go backend persists (identical for ASCII, wrong for
multi-byte text); `session.Manager` never idle-evicts a session; a
`code_block`'s cursor has no precise on-screen position (no
`getClientRects()` equivalent for a `<textarea>`). Each is a small, bounded,
already-named decision — port them as decisions, not accidents rediscovered
later.

## The one fact that changes how you read this guide

**Only one module has real golden test vectors**: `documentcore`
(`testdata/document-core/marks.json`, consumed by its own Go tests). Every
other module — `collaboration-service`'s entire CRDT/WAL/session stack,
`document-service`'s repo layer, `auth-service`'s domain logic, everything —
is pinned down **only** by its own Go `_test.go` files, with no separate
data-driven fixture a Rust implementation could consume directly. This
guide's step 2 ("read the golden vectors") was written aspirationally for a
world where every module got the `documentcore` treatment; it didn't happen,
because the MVP's actual bottleneck was always feature surface, not
fixture infrastructure.

**Practical consequence**: for any module beyond `documentcore`, porting
means reading the Go `_test.go` files as the behavior spec and either (a)
hand-porting each test case's *scenario* into an equivalent Rust test against
the new implementation, or (b) spending an afternoon extracting the highest-
value scenarios into `testdata/<module>/*.json` first, then porting against
those the way `documentcore` already demonstrates. (b) is better if the
module's behavior is intricate enough that language-agnostic fixtures pay for
themselves (the CRDT core, definitely; a thin CRUD repo, probably not).
Decide per module — don't blanket-apply either choice.

## Order of operations, per module

1. **Read the spec first, not the Go/TS code.** `RFC-001`, `RFC-002`,
   `DATA_MODEL.md`, and `docs/api/` are language-agnostic and didn't change
   for this build — they're still the actual spec. `docs/api/collaboration.md`
   in particular now documents the `"cursor"` frame added late in the Go
   build (2026-08-26) — RFC-002 itself was never amended for it, since
   cursor position is presence-shaped, not an op.
2. **Read the golden test vectors, where they exist** (`testdata/<module>/*.json`
   — today, only `documentcore`'s). Everywhere else, read the Go `_test.go`
   files with the same intent — they're standing in for fixtures that were
   never extracted (see above).
3. **Read the Go implementation third**, as a worked reference — not a
   template to transliterate. TypeScript has no implementation of its own to
   read for the editor core (`ADR-011`'s addendum: `web/`'s `document-core/`
   is views and a JSON bridge over Go compiled to wasm) — that port is
   Go → Rust, full stop, with `web/` needing only a wasm-target change, not a
   logic port. The rest of `web/` (screens, `useCollabPage`, `api/` REST
   clients) stays TypeScript regardless of what the backend is written in —
   there is no TS→Rust step anywhere in this plan. Watch for `// PORT-NOTE:`
   comments in the Go source (sparse today — most of the real GC-leaning
   decisions are called out inline in prose instead, cross-referenced below).
4. **Write the Rust idiomatically**, not shaped to match Go's structure.
   Small interfaces become traits where Rust's type system wants them, not
   because Go had an interface there. `(T, error)` becomes `Result<T, E>`
   with a real `thiserror` taxonomy, not a mechanical rename. A Go `map`
   guarded by one `sync.Mutex` (this codebase's default concurrency shape —
   see below) is a legitimate `Arc<Mutex<HashMap<...>>>` in Rust, but check
   whether the access pattern actually wants a `DashMap`, an actor task with
   an `mpsc` channel, or `tokio::sync::RwLock` instead before defaulting to
   the direct translation.
5. **Run the same golden vectors (or hand-ported test scenarios) against the
   Rust implementation** before trusting it's equivalent.

## Suggested port order, and why

This follows the actual dependency graph, not the services table's port
numbers:

1. **`documentcore`** — no dependencies on anything else in the repo; both
   `document-service` (wasm bridge) and `collaboration-service` (block-op
   session state) import it. Has real golden vectors already. Start here —
   it's also the only module the *original* Rust-mentor track ever
   scoped, so there's precedent to check against in `docs/rust/`.
2. **`collaboration-service/internal/{rope,anchor,doctext,ops,pageop,oplog}`**
   — the character-level CRDT core: a rope, stable item identities across
   concurrent inserts/deletes, anchor-based ranges that survive concurrent
   edits, the invert/apply laws RFC-002 §3 specifies. This is the actual
   "hard Rust content" ADR-005 originally worried a Go reference would have
   nothing to port for (arena-style allocation patterns, ownership of shared
   mutable rope state, `MaybeUninit`-adjacent tricks a rope implementation
   tends to want) — it's real, and it's here, not in `documentcore`. No
   golden vectors exist for this tier yet; extracting them (option (b)
   above) is worth the afternoon before starting this module specifically.
3. **`collaboration-service/internal/{wal,flush,opstore,outbox,session}`** —
   the durability and concurrency shell around the CRDT core: file-based WAL
   with crash recovery, a batched flush loop, one mutex-guarded `Session` per
   page, a `Manager` keyed by page id. This is where the Go code leans
   hardest on GC and goroutines doing implicit work: `Session`'s single
   `sync.Mutex` serializing all access is a direct `Arc<Mutex<Session>>` or
   an actor-per-page `tokio` task (pick based on whether you want blocking
   critical sections or message-passing — a real design call, not a rename);
   the WAL's `os.File` + manual record framing has no automatic Rust
   equivalent; `session.Manager`'s "never idle-evict" simplification (stated
   above) is inherited as-is unless the port wants to fix it.
4. **`collaboration-service/internal/wsapi`** — the WebSocket transport.
   Thin by design (decode a frame, call into `session`, encode the result),
   so it ports close to mechanically once #2–#3 exist — but "which async
   runtime and WS crate" (`axum` + `tokio-tungstenite` is the obvious choice)
   is itself a real decision the Go side never had to make.
5. **`document-service`** — `pages` (CRUD + LTREE paths + sort-key
   fractional ordering), `blockproj` (NATS-consumed projection materializing
   `docs.blocks`/`docs.page_links`), `pagerepo`/`blockrepo` (sqlc-generated
   — the Rust equivalent is hand-written `sqlx::query!` or `sea-query`, since
   Rust has no direct sqlc analogue that matches this repo's exact
   conventions). Mostly mechanical; the one real port decision is
   `pathLabel`/`childPath`'s LTREE-label encoding (UUID hyphens stripped,
   `p`-prefixed) — copy the scheme exactly, it's a stored format, not an
   implementation detail.
6. **`auth-service`** — Argon2id hashing, RS256 JWT + JWKS, refresh-token
   rotation with reuse detection, account lockout. Security-sensitive:
   port test-by-test against the Go behavior, not just "make the crypto
   library calls" — the *state machine* (lockout thresholds, one-time
   refresh tokens, family revocation on reuse) is the actual spec, and it's
   proven out in `internal/authservice`'s integration tests, not in any doc.
   Rust's `argon2`/`jsonwebtoken` crates cover the primitives; the state
   machine around them is this module's real port work.
7. **`notification-service`** and **`api-gateway`** — small (424 and 695
   lines respectively) and mechanical: a NATS consumer + Postgres write +
   one `GET` route; a REST↔gRPC (or, in Rust, REST↔whatever `collaboration-
   service`/`document-service`/`auth-service` expose) translation shim. Port
   these last, or even in parallel with #5–#6 — nothing else depends on
   them.
8. **`web/`** — stays TypeScript. The only change the port causes is
   swapping `documentcore`'s wasm target (see below); every screen,
   `useCollabPage`, and REST client is unaffected regardless of backend
   language, including the cursor-tracking and rich-editor work that landed
   well after this guide's first draft.

## What ports cleanly vs. what doesn't

**Ports directly:** service/module boundaries (`ARCHITECTURE.md`,
`DATA_MODEL.md`), the op ISA and its invertibility law, the gRPC/OpenAPI/
WebSocket contracts (`docs/api/`), the golden test vectors that exist, the
database schemas, the LTREE path-encoding scheme, `sortkey`'s fractional
ordering algorithm (pure logic, no Go-specific behavior).

**Needs real Rust design work** (concrete examples, not the generic list the
original draft of this guide had):
- `collaboration-service/internal/rope`/`anchor`/`doctext`'s CRDT core —
  the actual hard content, see port-order step 2 above.
- `Session`'s one-mutex-per-page model and `Manager`'s session lifecycle —
  `Arc<Mutex<T>>` vs. an actor task with channels is a real choice.
- `wal.Writer`'s file-based append + `wal.Recover`'s crash-recovery replay —
  no direct Rust stdlib equivalent; needs its own framing/`fsync` strategy.
- The ephemeral presence/cursor maps (`Session.presence`, `Session.cursors`)
  — currently plain Go maps behind the same mutex as everything else; worth
  deciding in Rust whether they deserve separate synchronization from the
  durable-op path they currently share a lock with.
- `flush.Loop`'s ticker-driven batching and `outbox.Poller`'s NATS-publish
  polling — both are goroutines-with-a-ticker in Go; Rust's equivalent is a
  `tokio::time::interval` task, straightforward once the runtime choice
  from port-order step 4 is made.

## The wasm boundary carries over almost unchanged

`services/document-service/cmd/wasm` compiles `internal/documentcore` to
`GOOS=js GOARCH=wasm`, exposing `documentcoreNewPage`/`documentcoreApplyOp`/
`documentcoreInvertOp` as JSON-string-in, `{value,error}`-JSON-out
functions. The Rust port's equivalent is a `wasm32-unknown-unknown` crate
exposing the same three functions with `wasm-bindgen`, same JSON contract.
`web/src/document-core/wasm.ts` (the loader) and `types.ts` (wire types)
need no changes at all for this swap — they never assumed anything
Go-specific. Only `web/public/`'s build step changes (a Rust build instead
of `scripts/build-wasm.sh`).

Note the honest caveat from `CLAUDE.md`: `documentcore.wasm` is built and
gitignored but **not yet wired to any screen** — the running editor's live
collaboration goes through `collaboration-service`'s own Go-side
`documentcore.Page`/`doctext.Text`, not through the browser wasm build. The
wasm boundary is real and tested — `web/src/document-core/wasm.test.ts`
proves the JSON bridge end-to-end against the real compiled `.wasm` binary
(page creation, applying an op, undo/redo) — just not yet load-bearing for
the live product. The port should preserve the boundary but shouldn't
assume porting it also means porting a live wasm call path that doesn't
exist yet.
