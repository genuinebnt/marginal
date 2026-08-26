# Codebases to read

**Reading good code is the highest-leverage habit on this whole list**, and the one most people
never build. Papers give you the idea; a codebase gives you the *hundred decisions* the paper did
not mention — error types, module boundaries, where the `unsafe` went, what got a comment and what
did not.

This file is organised by **what you want out of it**: new concepts, mastering ones you half-know,
code patterns, algorithms, and review skill. Paths are given as *modules and directories* rather
than line numbers, because line numbers rot in weeks.

---

## §0 How to read a codebase — the method

Cloning a repo and scrolling is not reading. Four approaches, each for a different goal:

| Goal | Method |
|---|---|
| **Understand a concept** | Find the *type* that names it, read its definition and doc comment, then find every place it is constructed. The constructors tell you the invariants |
| **Learn a pattern** | Read the **public API surface only** — `lib.rs`, the `pub use` re-exports, and the doc examples. Then guess the internals before looking |
| **Understand an algorithm** | Read the tests first. A good test suite is the specification, and it tells you the edge cases before you have to infer them |
| **Build review skill** | Read the **git history and PR discussions**, not the current state. Code shows what was decided; history shows *why*, and reviews show what nearly shipped instead |

**Three questions to answer for every codebase you read.** Write the answers down — this is what
turns browsing into learning:

1. What is the **central type**, and what invariant does it protect?
2. Where is the **boundary** — what is `pub`, what is `pub(crate)`, and why is the line there?
3. What did they do that **surprised you**, and is it a trick or a mistake?

> **Read with your own code open.** The value is comparison. "They put the trait in the same file
> as the impl too" or "they used one struct where I used three" is worth more than a chapter.

---

## §1 The five to read properly

If you read nothing else on this page, read these — each maps directly onto a subsystem you will
build, and each is exceptionally well written. The last one you wrote yourself.

| Repo | Read | Why this one, for this project |
|---|---|---|
| [**rust-analyzer**](https://github.com/rust-lang/rust-analyzer) | `crates/ide-db`, `crates/hir`, `crates/parser`, and [`docs/dev/architecture.md`](https://github.com/rust-lang/rust-analyzer/blob/master/docs/book/src/contributing/architecture.md) | **The closest architectural relative to Marginal.** An incremental analyser over a syntax tree that must never reject broken input, structured as a workspace of small crates. Phases 1, 4, 16. Start with the architecture doc — matklad wrote it as a guided tour |
| [**ropey**](https://github.com/cessen/ropey) | `src/rope.rs`, `src/tree/`, and the `Metric` handling | The best-documented rope in Rust. Read `tree/node.rs` for how a B-tree of text chunks maintains its summaries. Phase 3 |
| [**tantivy**](https://github.com/quickwit-oss/tantivy) | [`ARCHITECTURE.md`](https://github.com/quickwit-oss/tantivy/blob/main/ARCHITECTURE.md) first, then `src/termdict/`, `src/postings/` | A production search engine, in Rust, by an author who documents. Phase 7 is this repo plus BurntSushi's FST post |
| [**zero-to-production**](https://github.com/LukeMathWalker/zero-to-production) | The whole thing, chapter by chapter | Axum + sqlx + tracing + config + Testcontainers, with the *reasoning* in the accompanying book. If your `document-service` layout feels arbitrary, this is the comparison |
| **`~/projects/genuine-folio`** | `domain/`, then `infra/discover.rs`, `domain/graph.rs`, `infra/render.rs` | **Your own prior art, and the closest reference implementation to Marginal that exists** — wikilinks, graph, pgvector discover, comrak+syntect rendering, grammar checking, already shipped. See [§2.5](#25-genuine-folio--your-own-prior-art) for what to take and what to leave |

---

## §2 By phase

### Phase 0–1 — structure, domain types, Postgres

| Repo | Read | For |
|---|---|---|
| [rust-analyzer](https://github.com/rust-lang/rust-analyzer) | the workspace `Cargo.toml` and crate list | **How a flat many-crate workspace is actually laid out.** Compare to `PROJECT_STRUCTURE.md` §5 |
| [zero-to-production](https://github.com/LukeMathWalker/zero-to-production) | `src/configuration.rs`, `src/telemetry.rs`, `tests/api/` | Config layering, tracing setup, and integration-test harness patterns — the boilerplate you already have, done by someone else |
| [sqlx](https://github.com/launchbadge/sqlx) | `sqlx-macros-core/src/query/`, and the `Encode`/`Decode` impls for Postgres types | How compile-time query checking works, and **what sqlx does not have a type for** — which is why LTREE needs `::text` casts |
| [dtolnay/thiserror](https://github.com/dtolnay/thiserror) + [anyhow](https://github.com/dtolnay/anyhow) | `src/lib.rs` of both, plus the doc comments | The canonical error-handling split. Small enough to read fully in an hour, and dtolnay's API design is worth studying on its own |
| [uuid](https://github.com/uuid-rs/uuid) | `src/v7.rs` | UUIDv7 generation. Short, and you depend on the timestamp ordering property |
| [Neon](https://github.com/neondatabase/neon) | `libs/utils/`, `pageserver/src/tenant/` | Serious Rust against Postgres internals. Read for how a large Rust service is organised, not for the storage design |

### Phase 2 — auth

| Repo | Read | For |
|---|---|---|
| [RustCrypto/password-hashes](https://github.com/RustCrypto/password-hashes) | `argon2/src/lib.rs` and the PHC string parsing | How a PHC string encodes algorithm + params + salt. The reason parameters upgrade without a migration |
| [jsonwebtoken](https://github.com/Keats/jsonwebtoken) | `src/validation.rs`, `src/decoding.rs` | **Read `Validation`'s defaults carefully.** What is checked by default and what is not is a security decision you inherit |
| [zero-to-production](https://github.com/LukeMathWalker/zero-to-production) | the auth chapters' code | Sessions, password verification with a constant-time compare, and the timing-attack mitigation in context |

### Phase 3 — collaboration · *the richest reading in the project*

| Repo | Read | For |
|---|---|---|
| [**diamond-types**](https://github.com/josephg/diamond-types) | `src/list/`, and the benchmarks | **The fastest sequence CRDT in Rust**, by the author of the "CRDTs go brrr" post. Read for how far the constants can be pushed |
| [**automerge**](https://github.com/automerge/automerge) | `rust/automerge/src/op_set/`, `src/storage/` | **The columnar op-log storage format.** Directly relevant to your `collab.ops` payload encoding. Also read their `OpId` type |
| [**loro**](https://github.com/loro-dev/loro) | `crates/loro-internal/src/container/richtext/` | Rich-text CRDT with **movable trees** — and their richtext module is a Peritext-family implementation you can read |
| [**y-crdt**](https://github.com/y-crdt/y-crdt) | `yrs/src/block.rs`, `yrs/src/types/text.rs` | YATA in Rust. The `Block` type is the item-id design RFC-001 §9 chose |
| [**crossbeam**](https://github.com/crossbeam-rs/crossbeam) | `crossbeam-epoch/src/`, `crossbeam-queue/src/array_queue.rs` | **Read `array_queue.rs` line by line before using `ArrayQueue`.** Then `crossbeam-epoch` — this is the epoch reclamation KAIST cs431 teaches |
| [**tokio**](https://github.com/tokio-rs/tokio) | `tokio/src/sync/mpsc/`, `tokio/src/sync/watch.rs`, `tokio/src/macros/select.rs` | Channel implementations, and **`select!`'s implementation is where cancellation safety becomes concrete** |
| [ropey](https://github.com/cessen/ropey) + [crop](https://github.com/nomad/crop) | both `src/tree/` | Two ropes, different trade-offs. `crop` is smaller and easier to read end to end |
| [zed](https://github.com/zed-industries/zed) | `crates/rope/`, `crates/text/` | Production rope + anchors + selections. **`crates/text/src/anchor.rs` is the anchor type you are designing** |
| [turmoil](https://github.com/tokio-rs/turmoil) | `src/sim.rs`, `examples/` | How deterministic network simulation is implemented, which tells you what it can and cannot simulate |
| [sled](https://github.com/spacejam/sled) | `src/pagecache/`, and the [simulation guide](https://sled.rs/simulation.html) | Lock-free Rust and crash-safety testing, with strong opinions. Read the WAL/recovery parts for Phase 3 |
| [TigerBeetle](https://github.com/tigerbeetle/tigerbeetle) | [`docs/internals/vsr.md`](https://github.com/tigerbeetle/tigerbeetle/blob/main/docs/internals/vsr.md), `src/vsr/` | Zig, not Rust — read it anyway. **A system designed around deterministic simulation from day one**, with unusually good internal docs |

### Phase 4 — diagnostics

| Repo | Read | For |
|---|---|---|
| [**salsa**](https://github.com/salsa-rs/salsa) | `src/function/`, `src/revision.rs`, and the book | **The engine.** Read `revision.rs` first — the whole invalidation model is revision comparison |
| [rust-analyzer](https://github.com/rust-lang/rust-analyzer) | `crates/hir-def/`, `crates/ide-diagnostics/` | Salsa at scale, plus a real diagnostics architecture with quick fixes |
| [rustc](https://github.com/rust-lang/rust) | `compiler/rustc_query_system/src/dep_graph/` | The red/green algorithm in production. Dense; read the [dev guide](https://rustc-dev-guide.rust-lang.org/queries/incremental-compilation-in-detail.html) alongside |
| [bumpalo](https://github.com/fitzgen/bumpalo) | `src/lib.rs` | Arena allocation, small enough to read fully. The `alloc` fast path is a nice piece of code |
| [rayon](https://github.com/rayon-rs/rayon) | `rayon-core/src/registry.rs`, the `ParallelIterator` trait | Work stealing, and **how a parallel iterator is actually built from a trait**. Excellent trait design |

### Phase 5–6 — undo, history, diff

| Repo | Read | For |
|---|---|---|
| [similar](https://github.com/mitsuhiko/similar) | `src/algorithms/myers.rs` | Myers in readable Rust. **Read after implementing your own**, to compare |
| [imara-diff](https://github.com/pascalkuthe/imara-diff) | `src/histogram/` | A faster diff with histogram heuristics. The performance-oriented counterpart |
| [im](https://github.com/bodil/im-rs) | `src/vector/` | Persistent data structures in Rust — structural sharing, which is Phase 6's palimpsest mechanism |
| [arrow-rs](https://github.com/apache/arrow-rs) / [parquet](https://github.com/apache/arrow-rs/tree/master/parquet) | the `parquet` crate's writer | Snapshots are Parquet. Read the row-group and column-chunk writing path |

### Phase 7 — search

| Repo | Read | For |
|---|---|---|
| [**fst**](https://github.com/BurntSushi/fst) | `src/raw/build.rs`, `src/automaton/` | **FST construction, and the `Automaton` trait that lets you intersect a Levenshtein DFA with the term dictionary.** The single most instructive read in this phase |
| [tantivy](https://github.com/quickwit-oss/tantivy) | `src/termdict/`, `src/postings/`, `src/query/bm25.rs` | BM25 as actually implemented, and the postings encoding |
| [regex](https://github.com/rust-lang/regex) | `regex-automata/src/dfa/`, and the `regex-automata` docs | **Automata done properly.** Lazy DFA construction, and BurntSushi's documentation here is better than most textbooks |
| [memchr](https://github.com/BurntSushi/memchr) | `src/arch/`, and the algorithm docs | SIMD substring search with the reasoning written down. The best example of *documented* SIMD code in Rust |
| [roaring-rs](https://github.com/RoaringBitmap/roaring-rs) | `src/bitmap/container.rs` | Container-type selection by cardinality. The idea is simple and the code shows the bookkeeping |

### Phase 8–10 — saga, gateway, routing

| Repo | Read | For |
|---|---|---|
| [**chitchat**](https://github.com/quickwit-oss/chitchat) | the whole crate — it is small | **SWIM-family gossip in Rust, readable in an afternoon.** The closest reference implementation to what Phase 10 needs |
| [memberlist](https://github.com/hashicorp/memberlist) | `state.go`, `net.go` | Go, and the canonical SWIM implementation. Read `state.go` for probe/suspect/refute in code |
| [tower](https://github.com/tower-rs/tower) | `tower/src/limit/`, `tower/src/retry/`, `tower/src/load_shed/` | **`Service` and `Layer` as a composition model**, plus the retry and shed middlewares you need in Phase 9 |
| [tonic](https://github.com/hyperium/tonic) | `tonic/src/transport/`, the generated-code shape in `examples/` | What `tonic-build` generates, so gRPC stops being magic |
| [axum](https://github.com/tokio-rs/axum) | `axum/src/extract/`, `axum-core/src/response/` | **`FromRequestParts` and `IntoResponse` are a masterclass in trait-based ergonomics.** Read to understand why your handlers can take arbitrary argument tuples |
| [governor](https://github.com/boinkor-net/governor) | `src/gcra.rs` | GCRA rate limiting — more elegant than token bucket, and the code is short |
| [hyper](https://github.com/hyperium/hyper) | `src/proto/h2/` | HTTP/2 framing and flow control, for when head-of-line blocking becomes real |

### Phase 11–12 — cloud, CI, observability

| Repo | Read | For |
|---|---|---|
| [cargo-chef](https://github.com/LukeMathWalker/cargo-chef) | `README.md` and `src/recipe.rs` | The Docker layer-caching trick for Rust, and *why* it works |
| [tracing](https://github.com/tokio-rs/tracing) | `tracing/src/lib.rs`, `tracing-subscriber/src/layer/` | **The `Layer` trait is how you compose subscribers.** Read before fighting a subscriber configuration |
| [opentelemetry-rust](https://github.com/open-telemetry/opentelemetry-rust) | `opentelemetry-sdk/src/trace/`, the propagator impls | Context propagation across a gRPC boundary. The part that breaks silently |
| [terraform-google-modules](https://github.com/terraform-google-modules) | the `kubernetes-engine` module | **Read a well-structured Terraform module before writing yours.** Variable design and output surface are the transferable parts |

### Phase 13–21 — platform and reach

| Repo | Read | For |
|---|---|---|
| [**wasmtime**](https://github.com/bytecodealliance/wasmtime) | `crates/wasmtime/src/runtime/store.rs`, `crates/wasmtime/src/engine.rs`, `fuzz/` | `Store`, `Linker`, `ResourceLimiter`, epoch interruption. **Also read `fuzz/` as a model for fuzzing your own host boundary** |
| [wit-bindgen](https://github.com/bytecodealliance/wit-bindgen) | the examples and generated code | How a WIT interface becomes typed host/guest bindings — your capability manifest as a compile-time contract |
| [extism](https://github.com/extism/extism) | `runtime/src/` | A plugin framework's host API. Read the design even if you do not take the dependency |
| [SpiceDB](https://github.com/authzed/spicedb) | the schema language and `internal/graph/` | Zanzibar in practice. **`internal/graph/` is authorization as graph reachability**, which is Phase 13's central idea |
| [pgvector](https://github.com/pgvector/pgvector) | `src/hnswbuild.c`, `src/hnswscan.c` | C, and worth it — **HNSW inside a database, which is what you benchmark against.** Note how it handles filtering |
| [instant-distance](https://github.com/instant-labs/instant-distance) | `src/lib.rs` | HNSW in readable Rust. Read after writing yours |
| [polars](https://github.com/pola-rs/polars) | `crates/polars-core/src/chunked_array/`, `crates/polars-arrow/` | **Arrow layout + SIMD kernels + rayon in one codebase.** The best columnar-engine code you can read, and directly relevant to Phase 17 |
| [petgraph](https://github.com/petgraph/petgraph) | `src/algo/` | Every classical graph algorithm in idiomatic Rust. **Read after implementing each one** — the comparison is the lesson |
| [helix](https://github.com/helix-editor/helix) | `helix-core/src/selection.rs`, `helix-core/src/transaction.rs` | **`Transaction` is an invertible change set** — the closest published analogue to your `Op` with `invert`. Phase 5 and 16 |
| [zed](https://github.com/zed-industries/zed) | `crates/editor/src/`, `crates/multi_buffer/` | A production editor's state management. Large; read `multi_buffer` for how anchors survive edits |

---

## §2.5 `genuine-folio` — your own prior art

`~/projects/genuine-folio` is the **closest reference implementation to Marginal that exists**, and
you wrote it. 309 commits, ~56k lines, 74 Rust files, edition 2024, axum + sqlx + pgvector +
Testcontainers + OpenTelemetry. Read it before every phase whose product surface it already covers.

> **The framing that makes it useful: folio is the answer key for Marginal's *product* questions and
> a counter-example for its *architecture* questions.** One monolith with one deploy cadence is a
> different problem from eleven services — so take the domain reasoning and leave the layering.

### What it already solved, and which phase collects it

| Read in folio | For Marginal phase |
|---|---|
| `domain/wiki.rs` — flat rows → tree, with **dangling-parent and cycle rescue** so a chapter never silently vanishes | 1 tree, 4 `LinkCycle` |
| `infra/links.rs` + `document_links` — `[[wikilink]]` extraction and storage | 4 symbol table, 7 backlinks |
| `domain/graph.rs` — typed edges ranked by strength, strongest-wins dedup per unordered pair, tag-edge threshold | 21, and `ui-mockups/graph.html` |
| **`infra/discover.rs`** — pgvector HNSW with `ef_search` set explicitly **and reported in the response** | **19** — and it is the same honesty principle as `analytics.html` showing error beside every estimate |
| `infra/render.rs` — `comrak` + `syntect` + `two-face`, ~1700 lines with tests | 1 (RFC-001 §7), 16 |
| `infra/grammar.rs` — `harper-core` | RFC-003 §2.1's grammar source |
| `infra/auth.rs` — argon2 + jsonwebtoken | 2 |
| revisions · comments · reactions · notifications · analytics · feeds · newsletter · theme/fonts | 6, 14, 15, 17, 20 |

### What to steal outright

| Pattern | Where |
|---|---|
| **Comments that carry quantitative reasoning** | `domain/graph.rs`: *"a six-part series joined pairwise would draw fifteen edges and look like a cluster, where the thing it actually is is a chain."* This is the BurntSushi habit — name the rejected alternative — arrived at independently |
| **Log-level discipline in the error mapper** | `api/error.rs`: validation → no log · upstream failure → `warn` · rate-limited → `info` ("expected/transient, not a bug") · internal → `error`. Most codebases log everything at one level |
| **`parse_slug_or_404`** | An unparsable slug is a 404, not a 400, *with the reasoning written down*. Exactly the kind of decision `lld/document-service.md` §8 has to make repeatedly |
| **Build-profile comments** | `Cargo.toml`: `panic stays "unwind" on purpose: a panicking request must not abort the server` — and `opt-level = "s"` justified by "the workload is I/O-bound, not CPU-bound" |
| **`justfile` with reasoning** | `set dotenv-override := true` because a global `DATABASE_URL` from another project would otherwise win. A real bug, prevented and documented |

### What **not** to carry over

**One inversion generates almost all of it:**

> **folio: the document is the truth, and history is copies of it.**
> **Marginal: the log is the truth, and the document is a projection of it** (`DATA_MODEL.md` §1).

Read the table below *before* Phase 1's schema work. Every row is a place where folio made the
correct call for a portfolio blog and the wrong one for this.

#### The data model — the part that matters

| In folio | Why Marginal differs |
|---|---|
| **`document_revisions` snapshots `body_markdown` in full on every save** | Forecloses **per-actor undo** (a revision has no actor and no inverse), **scrubbing** (you can only land on a save boundary), **live collaboration** (whole-body last-write-wins), **cheap diff** (recomputed at read time from two bodies), and **lossless restore** (copying an old body back discards everything after it). Phases 3, 5, 6 all depend on the inverse design |
| **`order: i32` for tree placement** (`domain/wiki.rs:12`) | Reordering rewrites every sibling. **Fractional `sort_key` exists precisely so a reorder writes one row** |
| **Tree placement in `metadata jsonb` as `{book, parent, order}`** — no FK, no constraint, no index | This is *why* `wiki.rs` needs dangling-parent and cycle rescue at read time. **The defensive code is compensating for a model that permits the problem.** Marginal has `parent_id REFERENCES docs.pages(id)`, an LTREE path, and a **write-time** cycle check — prevent, do not repair |
| **`body_html` stored beside `body_markdown`** | A rendered projection cached inside the source-of-truth row with no rebuild story. Change the renderer and every row is silently stale. Marginal's projections must be **rebuildable by replay, proven by a test** |
| **No optimistic concurrency** — `on conflict (slug) do update … updated_at = now()` | Two admin tabs is a silently lost update: no version column, no error. Marginal has vector clocks and one owner per page |
| **Five transactions in the whole backend; no outbox** | Marginal's outbox **must** share a transaction with the op write — which is why `lld/document-service.md` has write methods take `&mut Transaction`, never `&PgPool` |
| **Search is a generated `tsvector` over title + summary only** | The body is not indexed at all. Phase 7 needs positional postings at **block** granularity, and its own index cadence |
| **Mixed identity** — `slug` is the upsert key, `id` is the stable key | Marginal makes `PageId` the identity and deliberately does **not** make title unique: `DuplicateTitle` is a diagnostic, not a constraint violation |

#### The architecture

| In folio | Why Marginal differs |
|---|---|
| `domain/` → `app/` → `infra/` → `api/` layering | `PROJECT_STRUCTURE.md` §5 mandates **feature-first slices**. Legitimate for one deployable; wrong for eleven services |
| `app/ports.rs` — 18 traits in one file | Marginal: **every trait is declared in the same file as its primary impl** |
| `app/document_use_cases.rs` — 738 lines | This is the `application/usecases/<entity>/` shape `PROJECT_STRUCTURE.md` names explicitly and rejects |

#### Craft

| In folio | Why Marginal differs |
|---|---|
| `WikiRow.slug: String` despite `Slug` existing | The newtype stops at the domain boundary. In Marginal the newtype **is** the boundary |
| 34 of 73 files carry tests | Fine for a portfolio. Below what `agents.md`'s TDD contract asks of Marginal |

> **None of this is folio being badly built.** For a single-deployable portfolio blog, snapshot
> revisions, integer ordering, and JSONB placement are cheap, obvious, and adequate — the right
> calls. They are wrong for Marginal only because **Marginal made a harder promise.** That is the
> distinction to hold on to: a design is not better in the abstract, it is better *against a stated
> requirement*, and folio's requirements were different.

> **Two exercises worth doing, in this order.**
>
> 1. **Before Phase 1's schema.** Open folio's `migrations/0001_documents.sql` and
>    `0006_document_revisions.sql` beside `DATA_MODEL.md` §4, and for each difference say which
>    requirement forced it. If you cannot name the requirement, one of the two schemas is wrong.
> 2. **Any time.** Re-read `app/document_use_cases.rs` against `agents.md` § strict review rules and
>    write down what you would change now. Reviewing your own months-old code is
>    [§5](#5-building-code-review-skill)'s highest-value practice, and you have a 738-line file to
>    do it on.

---

## §3 Code patterns — read for idiom, not for the domain

These are the repos to read when the question is *"what does good Rust look like?"*

| Repo | The pattern it teaches |
|---|---|
| [**dtolnay/**](https://github.com/dtolnay) — `thiserror`, `anyhow`, `serde`, `syn`, `quote`, `cxx` | **API design as a craft.** Every one of these has a smaller public surface than you would guess. Read `thiserror` and `anyhow` fully — they are short and they set the standard |
| [**dtolnay/proc-macro-workshop**](https://github.com/dtolnay/proc-macro-workshop) | **Do this, do not just read it.** Five macro exercises with test suites, designed as a course. The best way to learn `syn`/`quote` for `define_id!` |
| [**std**](https://doc.rust-lang.org/std/) — click `[src]` on anything | The best-reviewed Rust you can read. Start with `VecDeque`, `BTreeMap::range`, `Vec::dedup_by`, `slice::partition_point` — all four are ROADMAP rows |
| [axum](https://github.com/tokio-rs/axum) | Trait-based ergonomics. How `FromRequestParts` makes variadic handlers work |
| [tower](https://github.com/tower-rs/tower) | Middleware as composition. `Service`/`Layer` is one of the best abstractions in the ecosystem |
| [clap](https://github.com/clap-rs/clap) | Builder + derive as two faces of one API, and excellent doc comments |
| [ripgrep](https://github.com/BurntSushi/ripgrep) | **A complete, production binary you can read end to end.** Crate splitting, error handling, and performance work with the reasoning written down |
| [rust-analyzer](https://github.com/rust-lang/rust-analyzer) | `crates/stdx/` — what a project's own "missing stdlib" crate looks like, and matklad's [style guide](https://github.com/rust-lang/rust-analyzer/blob/master/docs/book/src/contributing/style.md) |
| [typed-builder](https://github.com/idanarye/rust-typed-builder) / [typestate examples](https://cliffle.com/blog/rust-typestate/) | Compile-time state machines — Phase 13's `Op<Unchecked>` → `Op<Authorized>` |

> **matklad's [style guide](https://github.com/rust-lang/rust-analyzer/blob/master/docs/book/src/contributing/style.md) is worth more than most books on Rust style.** It is opinionated, justified, and short. Read it in Phase 0 and again in Phase 4.

### BurntSushi specifically — what makes it good, and how to read it

Andrew Gallant's code has a reputation, and it is earned for **reasons you can copy** rather than
for general brilliance. Six of them, each with the file that demonstrates it:

| What he does | Where you can see it |
|---|---|
| **Documentation is the deliverable, not an afterthought.** Module docs explain the *design space* and why he picked a point in it, before any API appears | [`regex-automata`'s crate docs](https://docs.rs/regex-automata/latest/regex_automata/) — effectively a textbook on finite automata that happens to ship as rustdoc |
| **A tiny public API over a large implementation.** `memchr` is thousands of lines of SIMD behind about six public functions | [`memchr` docs](https://docs.rs/memchr/latest/memchr/) — read the public surface, then `src/arch/` and notice the ratio |
| **One trait as the composition point.** The whole reason a Levenshtein DFA can be intersected with a term dictionary is that he exposed the right abstraction and nothing more | [`fst::Automaton`](https://docs.rs/fst/latest/fst/automaton/trait.Automaton.html) — **the single most instructive API in Phase 7** |
| **Benchmarking with published methodology.** He built a whole harness because he thought existing regex benchmarks were dishonest, and wrote up why | [`rebar`](https://github.com/BurntSushi/rebar) — read its README as an argument, not a tool |
| **`unsafe` with a scalar oracle.** Every SIMD path has a safe fallback and tests that compare the two exhaustively | `memchr`'s `src/arch/all/` (scalar) against `src/arch/x86_64/` (SIMD) |
| **He writes the blog post that *is* the paper**, then links it from the code | [transducers](https://burntsushi.net/transducers/) · [regex internals](https://burntsushi.net/regex-internals/) · [ripgrep](https://burntsushi.net/ripgrep/) · [csv](https://burntsushi.net/csv/) |

**Reading order — easiest to hardest.** Do not start with `regex`; it will put you off.

1. **[`memchr`](https://github.com/BurntSushi/memchr)** — read the crate docs, then `src/arch/all/memchr.rs`. Small, self-contained, and the scalar version is legible before you look at any SIMD. *Phase 1: this is your input-rule delimiter scan.*
2. **[`fst`](https://github.com/BurntSushi/fst)** — [the transducers post](https://burntsushi.net/transducers/) first, then `src/automaton/mod.rs`, then `src/raw/build.rs`. *Phase 7: this is the term dictionary.*
3. **[`bstr`](https://github.com/BurntSushi/bstr)** — read the crate docs on **why byte strings are not `String`**. *Directly relevant: your marks are byte ranges, and this is the crate that takes byte-oriented text seriously.*
4. **[`regex-automata`](https://docs.rs/regex-automata/latest/regex_automata/)** — read the **module docs as prose**, cover to cover, before any source. Lazy DFA construction, the one-pass engine, and why there are several engines rather than one.
5. **[`ripgrep`](https://github.com/BurntSushi/ripgrep)** — `crates/ignore/` is the interesting one: gitignore semantics are genuinely hard and the code shows it honestly. Also read [`GUIDE.md`](https://github.com/BurntSushi/ripgrep/blob/master/GUIDE.md) as an example of documentation written for users rather than for the author.
6. **[`aho-corasick`](https://github.com/BurntSushi/aho-corasick)** — hardest of the set. Read only if multi-pattern matching becomes relevant.

**The caveat, and it matters for this project.** His code is **library code tuned for thousands of
unknown callers.** Yours is application code with one known caller. Copy the *documentation
standard*, the *tiny-public-API instinct*, the *scalar oracle for `unsafe`*, and the *benchmark
honesty*. Do **not** copy the generic parameterisation, the trait layering, or the `no_std` +
feature-matrix machinery — in a service those are the over-abstraction `ROADMAP.md`'s speed rules
call a review failure. See §6 below.

> **The one habit to steal immediately:** he writes down *why not*, and where. Every non-obvious
> choice has a comment naming the alternative he rejected. That is exactly what `lld/document-service.md`
> §12 and `RFC-001 §9` already do for you — his code is the proof it works at scale.

### Nine more, each with a direct hook into Marginal

Selected because their code is exemplary **and** you will use or imitate the exact thing. Ordered
by when Marginal needs them.

> **If you only act on five things from this section.** The nine below look equally weighted and
> are not — these five change what you *do*, the rest change how you write:
>
> | | Why it is not just reading |
> |---|---|
> | **[`bat`](https://github.com/sharkdp/bat) `src/assets.rs`** | **A problem you have, already solved.** `syntect` + `two-face` with a grammar allowlist against a bundle budget is RFC-001 §7 verbatim. Read before you write it yourself |
> | **[`insta`](https://github.com/mitsuhiko/insta)** | **A tooling decision, not a recommendation.** Adopt it — block trees, ASTs, op lists, and diagnostics output are exactly what snapshot testing is for |
> | **[`hashbrown`](https://github.com/rust-lang/hashbrown) `src/raw/mod.rs`** | Literally a ROADMAP row ("SwissTable index notes"). This is its source |
> | **[`bytes`](https://github.com/tokio-rs/bytes)** | Literally a ROADMAP row (op fan-out: one allocation, N atomic increments). You implement against it in Phase 3 |
> | **[`thiserror`](https://github.com/dtolnay/thiserror) + [`anyhow`](https://github.com/dtolnay/anyhow), read in full** | **The best hour available on API design.** Both are short, and together they are the error split you are already implementing |

#### David Tolnay ([dtolnay](https://github.com/dtolnay)) — API design · Phase 0, 1

The most disciplined API author in the ecosystem. Every crate has a smaller public surface than you
would guess, and the macro error messages are better than most compilers'.

| Read | For |
|---|---|
| [`thiserror`](https://github.com/dtolnay/thiserror) + [`anyhow`](https://github.com/dtolnay/anyhow) — **both in full, ~1 h** | The canonical error split. Library errors are enumerated types; application errors are opaque with context. This is `AppError`/`ApiError` |
| [**proc-macro-workshop**](https://github.com/dtolnay/proc-macro-workshop) — **do it, do not read it** | Five macro exercises with test suites. The way to learn `syn`/`quote` for `define_id!` |
| [**case-studies**](https://github.com/dtolnay/case-studies) | Short write-ups of gnarly macro and trait problems with the reasoning. Unusual and excellent |
| [`serde`](https://github.com/serde-rs/serde) — `serde/src/de/mod.rs` docs | Where `#[serde(borrow)]` comes from, which is Phase 3's zero-copy op decode |

**Steal:** doc comments on every public item with a compiling example. **Do not steal:** his level of
`no_std` and feature gating — that is library work.

#### Amanieu d'Antras ([Amanieu](https://github.com/Amanieu)) — low-level data structures · Phase 3

Wrote the hash table that *became* `std::collections::HashMap`. If you want to see a data structure
done to the limit with the reasoning intact, this is it.

| Read | For |
|---|---|
| [**hashbrown**](https://github.com/rust-lang/hashbrown) — `src/raw/mod.rs` | **SwissTable.** Control bytes, SIMD group probing, and the load-factor maths. `ROADMAP.md` has a "SwissTable index notes" row; this is that row's source |
| [**parking_lot**](https://github.com/Amanieu/parking_lot) — `src/raw_mutex.rs` | A mutex with an eight-bit fast path. Read *after* Bos Ch. 9, as the production version of the same exercise |
| [`thread_local-rs`](https://github.com/Amanieu/thread_local-rs) | Per-thread state without contention. Relevant if the diagnostics arena becomes per-worker |

#### Carl Lerche ([carllerche](https://github.com/carllerche)) — async abstraction design · Phase 3, 9

| Read | For |
|---|---|
| [**bytes**](https://github.com/tokio-rs/bytes) — [`Bytes`](https://docs.rs/bytes/latest/bytes/struct.Bytes.html) and `src/bytes.rs` | **A ROADMAP row you will implement against:** op fan-out to N subscribers as one allocation and N atomic increments, not N copies. Small crate, big idea |
| [**tower**](https://github.com/tower-rs/tower) — the `Service` trait, then `retry/`, `load_shed/` | Middleware as composition. Phase 9's whole design |
| [`prost`](https://github.com/tokio-rs/prost) — the generated-code shape | What your protobuf becomes. Worth reading once so `tonic-build` stops being magic |

#### Mara Bos ([m-ou-se](https://github.com/m-ou-se)) — the standard library's sync primitives · Phase 3, 5

You own her book. **The code that came out of it is in `std`.**

| Read | For |
|---|---|
| [`library/std/src/sync/`](https://github.com/rust-lang/rust/tree/master/library/std/src/sync) — `mpmc/`, `once_lock.rs`, `mutex.rs` | The rewritten sync primitives, with the same reasoning as the book applied to production constraints. **Read a chapter, then read its `std` counterpart** |

#### Nick Fitzgerald ([fitzgen](https://github.com/fitzgen)) — arenas and WASM tooling · Phase 4, 16

| Read | For |
|---|---|
| [**bumpalo**](https://github.com/fitzgen/bumpalo) — `src/lib.rs` | Arena allocation, readable in full. The `alloc` fast path is a genuinely nice piece of code. **Phase 4 mandatory** |
| [`twiggy`](https://github.com/rustwasm/twiggy) + [`walrus`](https://github.com/rustwasm/walrus) | WASM bundle analysis and IR manipulation. Phase 16's size budget |

#### Armin Ronacher ([mitsuhiko](https://github.com/mitsuhiko)) — testing ergonomics · Phase 1 onward

| Read | For |
|---|---|
| [**insta**](https://github.com/mitsuhiko/insta) — [docs](https://docs.rs/insta/latest/insta/) | **Snapshot testing, and you should probably adopt it.** Your block tree, AST, op list, and diagnostics output are all large structured values whose tests are painful to write by hand. `compiler.html`'s stages are exactly the shape insta is for |
| [`similar`](https://github.com/mitsuhiko/similar) — `src/algorithms/myers.rs` | Myers in readable Rust. Phase 6, after writing your own |

#### David Peter ([sharkdp](https://github.com/sharkdp)) — the one with a direct code hit

| Read | For |
|---|---|
| [**bat**](https://github.com/sharkdp/bat) — `src/assets.rs`, `src/printer.rs` | **`syntect` + `two-face` integration, solved.** Marginal needs exactly this for code blocks, compiled to `wasm32` with a grammar allowlist against the bundle budget (RFC-001 §7). `bat` is where the grammar-bundling problem was already worked out |
| [`hyperfine`](https://github.com/sharkdp/hyperfine) | Statistically honest CLI benchmarking. Useful alongside `criterion` for whole-binary timing |

#### Jon Gjengset ([jonhoo](https://github.com/jonhoo)) — concurrent structures with documentation

You own his book and watch his streams; his **code** is worth a separate look.

| Read | For |
|---|---|
| [`left-right`](https://github.com/jonhoo/left-right) | A concurrency primitive — wait-free reads via two copies and a deferred swap — with **exceptionally good design docs**. A real alternative to `Arc<RwLock<_>>` for read-heavy state |
| [`inferno`](https://github.com/jonhoo/inferno) | Flamegraph generation in Rust. `perf.html` draws these; this produces them. Phase 12 |
| [`flurry`](https://github.com/jonhoo/flurry) | A port of Java's `ConcurrentHashMap`. Read the porting notes — a good study in translating a design across memory models |

#### Steven Fackler ([sfackler](https://github.com/sfackler)) — Postgres from the wire up · Phase 1

| Read | For |
|---|---|
| [**rust-postgres**](https://github.com/sfackler/rust-postgres) — `postgres-protocol/src/`, `postgres-types/src/` | **The wire protocol and the type mapping.** Read `postgres-types` when sqlx confuses you about a type — this is the layer underneath, and it is where LTREE's absence becomes visible |

#### Honourable mentions, by narrow use

| Who / what | When |
|---|---|
| [Ed Page](https://github.com/epage) — [`clap`](https://github.com/clap-rs/clap), [`toml`](https://github.com/toml-rs/toml) | **Deprecation and API-evolution discipline.** Read clap's changelogs to see how a breaking change is communicated |
| [Sean McArthur](https://github.com/seanmonstar) — [`hyper`](https://github.com/hyperium/hyper), [`h2`](https://github.com/hyperium/h2) | Phase 9, when HTTP/2 flow control and head-of-line blocking become real |
| [matklad](https://github.com/matklad) — [`once_cell`](https://github.com/matklad/once_cell), [`xshell`](https://github.com/matklad/xshell) | Two tiny crates that show his API instincts without the rust-analyzer scale |
| [Luca Palmieri](https://github.com/LukeMathWalker) — [`pavex`](https://github.com/LukeMathWalker/pavex) | A compile-time-DI web framework. Read the design docs for a contrasting take on the layering `PROJECT_STRUCTURE.md` rejects |
| [RustCrypto](https://github.com/RustCrypto) — [`traits`](https://github.com/RustCrypto/traits) | Phase 2. Read the trait design; **do not** implement a primitive |

### The pattern across all of them

Every codebase above shares four properties, and they are the actual lesson:

1. **The public API is much smaller than the implementation.** If yours is not, you have leaked internals.
2. **Non-obvious decisions carry a comment naming the rejected alternative.**
3. **Tests read as a specification**, so the edge cases are discoverable without inferring them.
4. **Performance claims come with a benchmark**, and the benchmark's methodology is written down.

Those four are also `agents.md` § strict review rules, arrived at independently. That convergence is
the reason to read code at all.

---

## §4 Algorithms — read the implementation next to the paper

| Algorithm | Read | Note |
|---|---|---|
| Fractional index | [fractional_index](https://github.com/drifting-in-space/fractional_index) | Small. Read after writing `key_between`, to check your alphabet handling |
| Rope | [ropey](https://github.com/cessen/ropey) `src/tree/`, [crop](https://github.com/nomad/crop) | Two designs |
| Sequence CRDT | [diamond-types](https://github.com/josephg/diamond-types), [y-crdt](https://github.com/y-crdt/y-crdt) `yrs/src/block.rs` | Performance-first vs spec-first |
| FST / automata | [fst](https://github.com/BurntSushi/fst), [regex-automata](https://github.com/rust-lang/regex/tree/master/regex-automata) | The `Automaton` trait is the composition point |
| Myers diff | [similar](https://github.com/mitsuhiko/similar) `src/algorithms/myers.rs` | Readable |
| Union-Find, MST, Dijkstra | [petgraph](https://github.com/petgraph/petgraph) `src/algo/` + [cp-algorithms](https://cp-algorithms.com/) | Idiomatic vs terse-and-correct |
| HNSW | [instant-distance](https://github.com/instant-labs/instant-distance), [pgvector](https://github.com/pgvector/pgvector) `src/hnsw*.c` | Rust and C, and the C one is your benchmark target |
| HLL / CMS / t-digest | [hyperloglogplus](https://github.com/tabac/hyperloglog.rs), [sketches-ddsketch](https://github.com/mheffner/rust-sketches-ddsketch) | Compare error handling to yours |
| Roaring bitmaps | [roaring-rs](https://github.com/RoaringBitmap/roaring-rs) | Container selection |
| Epoch reclamation | [crossbeam-epoch](https://github.com/crossbeam-rs/crossbeam/tree/master/crossbeam-epoch) | Read with KAIST cs431 open |
| Consistent hashing / gossip | [chitchat](https://github.com/quickwit-oss/chitchat), [memberlist](https://github.com/hashicorp/memberlist) | Rust and Go |

---

## §5 Building code-review skill

This is the part nobody plans for. **Review skill comes from reading reviews**, not from reading code.

### Read the discussion, not just the diff

| Where | What you learn |
|---|---|
| [**rust-lang/rfcs**](https://github.com/rust-lang/rfcs) — pick a merged RFC and read its whole thread | **The single best training material for design argument.** How an objection is raised, addressed, or accepted as a cost. Read the RFCs for `?`, `async/await`, and `impl Trait` |
| [rust-lang/rust](https://github.com/rust-lang/rust/pulls?q=is%3Apr+is%3Aclosed+label%3AT-libs-api) — closed libs-api PRs | API review in practice. Watch how a reviewer asks "what does this name promise?" |
| [tokio PRs](https://github.com/tokio-rs/tokio/pulls?q=is%3Apr+is%3Aclosed) | Correctness review on concurrent code. Look for the comments that catch a missing `Ordering` justification |
| [rust-analyzer PRs](https://github.com/rust-lang/rust-analyzer/pulls?q=is%3Apr+is%3Aclosed) | matklad's review comments are short, direct, and almost always about *structure* rather than style |
| [Automerge / Loro issue threads](https://github.com/automerge/automerge/issues) | Domain review: someone reports a convergence bug and the thread narrows it to a rule. This is how CRDT bugs actually get found |

### Read a *changelog* and a *deprecation*

| Where | What you learn |
|---|---|
| [tokio releases](https://github.com/tokio-rs/tokio/releases) | How a mature project communicates a breaking change, and what they consider breaking |
| [Rust release notes](https://github.com/rust-lang/rust/blob/master/RELEASES.md) | Read one release fully. Half of it is things you did not know existed |
| [serde's `#[non_exhaustive]` and semver discussions](https://github.com/serde-rs/serde/issues) | Why `#[non_exhaustive]` exists, from people who got it wrong once |

### Deliberate practice

Three exercises, in order of value:

1. **Review your own Phase 1 code six weeks later**, against `agents.md` § strict review rules. Write the findings down before fixing anything. This is the highest-value one and costs nothing.
2. **Do [proc-macro-workshop](https://github.com/dtolnay/proc-macro-workshop)**, then compare your solution to the reference. The gap is a review of your own work by someone better.
3. **Pick a small crate you depend on and read it fully** — `thiserror`, `bumpalo`, `fractional_index`, `chitchat`. Then write down three things you would change and three you would not. If you cannot fill both lists, you were browsing, not reviewing.

### The questions a good reviewer asks

Worth internalising, because these are what `/project:review` is checking for:

- What invariant does this type protect, and can a caller break it?
- What happens on the error path — and has the error path been *run*?
- Is this abstraction earning its indirection, or is it a trait wrapping a trait?
- What does this name promise that the implementation does not deliver?
- Which of these three structs could be one struct with stacked derives?
- Is the test asserting behaviour, or asserting the implementation?

---

## §6 Read critically — the anti-pattern list

Not everything in a famous repo is good. Things to notice rather than copy:

| Pattern you will see | Why it may not be for you |
|---|---|
| Deep generic bounds on every function | Often a library concern. An application service does not need `where T: Into<Cow<'a, str>> + Clone` |
| `Arc<Mutex<HashMap<...>>>` as shared state | Works, and usually the wrong shape. `PROJECT_STRUCTURE.md` prefers an actor with a channel — read tokio's `sync` docs on why |
| Builder types for three-field structs | Ceremony. `ROADMAP.md`'s speed rules call this out as a review failure |
| A `service.rs` for CRUD | Explicitly rejected by `PROJECT_STRUCTURE.md` §5.3. You will see it constantly in Rust web tutorials |
| `unwrap()` in library code | Fine in tests and examples. In a service it is a panic you chose not to name |
| Row → Domain → DTO conversion chains | Three types where derives on one would do. Common in ported-from-Java Rust |

> **The habit to build: when a repo does something differently from you, decide which is right
> rather than assuming they are.** Sometimes it is you — a library optimises for a thousand
> unknown callers and you have one known one.

---

## §7 A reading schedule that actually happens

Twenty minutes, three times a week, beats a weekend binge that never repeats.

| When | Read |
|---|---|
| **Now** | matklad's [style guide](https://github.com/rust-lang/rust-analyzer/blob/master/docs/book/src/contributing/style.md) · [`thiserror`](https://github.com/dtolnay/thiserror) in full · rust-analyzer's [architecture doc](https://github.com/rust-lang/rust-analyzer/blob/master/docs/book/src/contributing/architecture.md) |
| **During Phase 1** | [zero-to-production](https://github.com/LukeMathWalker/zero-to-production) alongside your own service · `std`'s `partition_point` and `dedup_by` · **folio's `domain/wiki.rs` and `api/error.rs`** |
| **Before Phases 4, 7, 19, 21** | **folio's `infra/links.rs`, `domain/graph.rs`, `infra/discover.rs`** — you already solved these once ([§2.5](#25-genuine-folio--your-own-prior-art)) |
| **During Phase 3** | [ropey](https://github.com/cessen/ropey) `src/tree/` · [crossbeam-queue](https://github.com/crossbeam-rs/crossbeam/tree/master/crossbeam-queue) `array_queue.rs` · [zed](https://github.com/zed-industries/zed) `crates/text/src/anchor.rs` |
| **During Phase 4** | [salsa](https://github.com/salsa-rs/salsa) `src/revision.rs` |
| **During Phase 7** | [fst](https://github.com/BurntSushi/fst) `src/raw/build.rs` |
| **Once a month, forever** | One merged [Rust RFC](https://github.com/rust-lang/rfcs) thread, start to finish |