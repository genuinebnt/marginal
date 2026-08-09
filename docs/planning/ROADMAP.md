# Marginal — Roadmap

**Product:** a self-hosted collaborative knowledge platform (ADR-001, expanded by ADR-009)
**Primary objective:** really good Rust learning (ADR-002)

---

## Stack at a Glance

| Layer | Technology |
|---|---|
| HTTP | Axum + Tower + Tokio |
| Database | PostgreSQL 18 + sqlx (JSONB, LTREE, `uuidv7()`, recursive CTEs) |
| Cache / presence | Redis |
| Event bus | NATS JetStream |
| Object storage | MinIO (local) / Cloud Storage (cloud) |
| Search | Tantivy (in-process) |
| gRPC | tonic + prost — the east-west default, all four RPC modes (ADR-007) |
| Frontend | React 19 + TypeScript SPA (Vite), Tailwind v4, Radix UI (ADR-004) |
| Editor core | **Rust → `wasm32`** — document model, rope, marks, selection, ops |
| API contract | `utoipa` → OpenAPI → `openapi-typescript` |
| IaC / hosting | Terraform (HCL) → **Google Cloud, the primary target** (ADR-008) |
| Observability | OpenTelemetry → Jaeger/Cloud Trace + Prometheus + Grafana |

---

## Execution Order

**Density:** ●●● deep Rust · ●●○ real but familiar · ○○○ no Rust

### Track 1 — MVP

| Step | Phase | Density | Ships |
|---|---|---|---|
| 1 | **1 — Documents** | ●●○ | Pages, blocks, tree, **outbox**, block-granular ops, **single-user block editor that saves** |
| 2 | **2 — Auth** | ●●○ | Argon2id, RS256, refresh rotation, login |
| 3 | **3 — Collaboration** | ●●● | Rope, CRDT, WAL, live cursors — **the demo** |

> 🏁 **MVP.** Log in, write a page, edit it live with someone else. Track 1 ends on the
> deepest phase, so shipping early costs no depth.

> **Phase 0 is not a step.** Foundation work is pulled in on demand by the service that
> first needs it — see § Phase 0 below. Building `libs/` before a consumer exists is the
> speculative abstraction `PROJECT_STRUCTURE.md` §5 forbids ("extract on the third use").

### Track 2 — The differentiators

| Step | Phase | Density | Ships |
|---|---|---|---|
| 5 | **4 — Diagnostics** | ●●● | Analyzers, symbol table, incremental engine, squiggles |
| 6 | **5 — Undo / Redo** | ●●○ | Per-actor undo across collaborative edits |
| 7 | **6 — History** | ●●○ | Event-sourced replay, snapshots, scrubber (CQRS) |

### Track 3 — Distributed systems

| Step | Phase | Density | Ships |
|---|---|---|---|
| 8 | **7 — Search & backlinks** | ●●● | Tantivy, inverted index, `[[link]]` autocomplete |
| 9 | **8 — Page-delete saga** | ●●○ | Choreographed saga, compensation, idempotency |
| 10 | **9 — API Gateway** | ●●○ | Tower stack, rate limiting, circuit breaker |
| 11 | **10 — Session routing** | ●●○ | Consistent hashing, failure detector, rehash |

### Track 4 — Knowledge platform (ADR-009)

> **Gated on the 🏁 above.** Ordered by dependency: comments need anchors, notifications
> need comments, publishing needs RBAC, plugins need the diagnostics engine, and the
> assistant needs the search index. That is why 18 and 19 sit after Tracks 2 and 3, not
> next to their siblings.

| Step | Phase | Density | Ships |
|---|---|---|---|
| 4a | **13 — Identity, spaces & RBAC** | ●●○ | Users, roles, LTREE permission inheritance in `can_apply` |
| 4b | **14 — Comments & reactions** | ●●● | Anchored threads, mentions, PN-Counter reactions |
| 4c | **15 — Notifications** | ●●○ | `notification-service` — the outbox's first subscriber |
| 4d | **16 — The full editor** | ●●● | All block directives, tables, footnotes, slash menu, fonts, reader modes, ⌘K |
| 4e | **20 — Settings & admin** | ●●○ | Instance/user/page settings, feature flags, theme families |

### Track 5 — Reach & extensibility (ADR-009)

| Step | Phase | Density | Ships |
|---|---|---|---|
| 5a | **17 — Publishing & distribution** | ●●○ | Public pages via static pre-render, feeds, sitemap, newsletter, analytics |
| 5b | **18 — Plugins** | ●●● | `wasmtime` sandbox, fuel metering, capability-scoped block kinds and analyzers |
| 5c | **19 — Assistant & semantic search** | ●●● | AI that emits **ops**, HNSW vector index |
| 5d | **21 — Related content** | ●●● | SimHash + LSH, PageRank, and the intersections — merge assistant, bridge suggestions, reading order, semantic paths |

### Track 6 — Cloud hardening

> **Cloud is not a track at the end, and it is not optional.** Google Cloud is the primary
> deployment target (ADR-008 § Amendment), so **a phase is not done until its increment in
> `CLOUD_ROADMAP.md` §2 is deployed.** Phase 1 already includes Terraform, Cloud SQL, Cloud
> Storage, Secret Manager, and a first deploy. What remains here is the work that only makes
> sense once all eleven services exist.

| Step | Phase | Density | Ships |
|---|---|---|---|
| 6a | **11 — Containers & CI** | ○○○ | Multi-stage builds, `cargo-chef`, keyless GitHub Actions via Workload Identity Federation |
| 6b | **12 — Observability & hardening** | ○○○ | Cloud Trace, Managed Prometheus, Grafana, SLOs, DR drill, cost review |

**Interleavable at any time:** `libs/doc` and `libs/diagnostics` need no infrastructure — pure `cargo test`. Pick them up whenever momentum stalls elsewhere.

> **This document has no dates.** Order is a dependency question and belongs here; *when* is
> an estimate and belongs in `TIMELINE.md`, which is explicitly not binding. A slip moves
> dates, never the order.

---

## Mockup Coverage

`docs/ui-mockups/` is seventeen pages of the finished product. Every surface drawn there has an
owning phase below — if a mockup shows something this table cannot place, either the phase
list is short or the mockup is wrong, and **the doc wins**.

| Mockup surface | Phase |
|---|---|
| **home** — landing, install, pitch | 17 |
| **home** — pricing tiers | *see § The hosted tier* |
| **signin** — login, refresh rotation | 2 |
| **signin** — first-run claim, invitation-only | 2 |
| **editor** — page tree, drag reorder, lifecycle badges | 1 |
| **editor** — input rules, diagnostics gutter, quick fix | 1, 4 |
| **editor** — presence, live cursors, session panel | 3 |
| **editor** — outline tab, bubble menu, slash menu, panel takeover, ⌘K | 16 |
| **editor** — backlinks tab | 7 |
| **editor** — comments tab, assistant proposing ops | 14, 19 |
| **reader** — outline rail, reading tools, focus, progress, sidenotes | 16 |
| **reader** — reactions, comment thread | 14 |
| **reader** — published badge | 17 |
| **search** — results, facets, snippets, link graph, index lag | 7 |
| **search** — "did you mean", typo tolerance | 7 (Levenshtein automaton) |
| **discover** — HNSW traversal, "compared N of M pages" | 19, 21 |
| **facts** — named definitions, transclusion, topological invalidation | 4 |
| **compiler** — lexer → AST → block tree | **1** |
| **compiler** — op emission + the projection check | 3 |
| **trace** — op apply/invert stepping, the invertibility law checked live | 3, 5 |
| **analytics** — HyperLogLog, Count-Min, t-digest, live stream | 17 |
| **analytics** — range and compare controls | 17 |
| *not drawn* — similar-block hint, SimHash/LSH bucketing | 21 |
| *not drawn* — merge assistant, bridge suggestions, reading order | 21 |
| **history** — scrubber, replay, snapshots, restore | 6 |
| **history** — palimpsest, tombstoned persistent sequence | 6 |
| **history** — op stream by actor, per-actor undo | 5 |
| **admin** — outbox depth, op-log lag, service health, uptime | 12 |
| **admin** — people, spaces, roles | 13 |
| **admin** — backups and verified restore | 11 |
| **settings** — three scopes, feature flags, theme families | 20 |
| **settings** — grammar toggle | 16 (RFC-003 §2.1) |
| **settings** — plugins toggle | 18 |
| **graph** — components, orphans, cycles | 4 |
| **graph** — shortest path, link distance | 7 |
| **graph** — blast radius, forward reachability | 8 |
| **graph** — BFS as a wavefront, frontier widths per level | 7 |
| **graph** — Betti numbers, clique complex, workspace sigil | 21 |
| **graph** — Voronoi territory, Delaunay dual vs link adjacency | 21 |
| **diff** — LCS / Myers, edit script, block moves | 6 |
| **perf** — latency percentiles, queue depth, flame graph | 12 |
| **perf** — SIMD scan, wasm bundle treemap | 3 (`memchr`, `twiggy`) |

### Deferred pending an ADR — a structural query language

A `jq`/CSS-selector language over the block tree — *all Rust code blocks*, *all headings under X*,
*everything with a dangling link*. Recorded rather than built, because it sits on a boundary that
has to be decided in writing first.

| | |
|---|---|
| **Why it is tempting** | A query language needs a **query parser with precedence**, which is the one Pratt parser Marginal otherwise never forces you to write (`learning/00-foundations.md` §4). That is a real learning argument, not a product one |
| **Why it is blocked** | A read-only query over *one page's* tree is plainly in scope. A workspace-wide version that **aggregates** is edging into ADR-001's out-of-scope *databases / views / relations / rollups* — and the reason that is out has not changed: cross-page aggregation has no owner |
| **What the ADR must decide** | Exactly where the line falls between *filter one tree* and *aggregate across pages*, and whether the parser is worth building for its own sake if the answer is the narrow one |

**Do not build the narrow version and let it grow.** That is precisely how the out-of-scope list
gets crossed by accident, one reasonable increment at a time.

### The hosted tier

`home.html` advertises a paid hosted plan, and ADR-001 cuts multi-tenancy. Both can be true:
**hosted means one single-tenant deployment per customer**, not many customers in one
database. That is an operations and billing concern, not an architectural one, and it needs
nothing from this roadmap.

If it ever means *shared* tenancy, that is ADR-009's `tenant_id`-on-every-table work and
needs its own ADR first. The mockup's prices are illustrative either way.

### The algorithm pages run the algorithm

Nine of the seventeen execute what they draw, so the picture cannot drift from the thing it
describes:

| Page | What actually runs |
|---|---|
| `graph.html` | force simulation with alpha decay; **exact Voronoi** by half-plane intersection, and the Delaunay dual read back off the cells |
| `graph-algorithms.html` | BFS shortest path, flood fill, three-colour DFS, forward reachability, all-pairs diameter, **BFS as an animated wavefront**, and Betti numbers — β₁ from the GF(2) rank of the triangle boundary map, β₂ from the Euler characteristic |
| `compiler.html` | the whole editor front end — block lexer, inline parser producing marks over **byte** ranges, recursive-descent AST, lowering to a block tree, op emission, and then a **projection check** that replays the ops and compares |
| `facts.html` | a dependency DAG with topological dirty propagation, cycle rejection by three-colour DFS, duplicate-definition detection |
| `history.html` | one paragraph backed by a **tombstoned persistent sequence** — every version is the filter `ins ≤ v < del` over a single array |
| `trace.html` | op apply and invert, with the invertibility law checked live |
| `diff.html` | the LCS table and its traceback |
| `discover.html` | HNSW traversal, and recall@5 measured against brute force |
| `analytics.html` | HyperLogLog, Count-Min Sketch, t-digest |
| `perf.html` | real percentile maths, a squarified treemap, a benchmark timed on the machine that opens the page |

Three are worth reading before their phase starts. `graph.html` demonstrates why `OrphanPage`
is a **connected-components** problem rather than `backlinks == 0` — a mutually-linked pair
with nothing pointing in still has backlinks — and why cycle detection needs three colours
rather than a visited set: a visited set answers *seen before*, not *on the current path*.
`diff.html` shows the O(n·m) table and then argues against it, which is the actual Phase 6
decision. `compiler.html` is the one to read before Phase 3: its projection check is the
executable form of *the op log is the source of truth and block rows are a projection replay
must reproduce* — a claim that can go red, which is the only kind worth making.

### Not drawn yet

Specced with an owning phase but no mockup: notifications inbox (15), media library (1),
plugin directory (18), space and role editor (13), offline and reconnect state (3), and the
rest of Phase 21 — the similar-block hint, related-pages panel, merge assistant, reading
order, and the minimum reading set.

---

## Correctness Tooling (ADR-002)

Not optional for the phases listed.

| Tool | Phase | Why |
|---|---|---|
| **[`loom`](https://docs.rs/loom)** | 3 | Model-checks interleavings. Lock-free code with a hand-chosen `Ordering` that has not been loom-tested is code that happens to work on your machine |
| **[Miri](https://github.com/rust-lang/miri)** | 3, 4 | UB under `MaybeUninit`, `repr(align)`, epoch reclamation |
| **[`cargo-fuzz`](https://rust-fuzz.github.io/book/)** | 1, 3, 4 | Never-panic laws over adversarial input: paste HTML, WAL bytes, block trees |
| **[`syn`](https://docs.rs/syn) + [`quote`](https://docs.rs/quote)** | 0 | Proc macros are a distinct skill from `macro_rules!` |
| **Hand-written `Future`/`Stream`** | 3 | `poll`, `Pin`, `Waker` by hand — the op-buffer flush |
| **`proptest`** | 1, 3, 4, 5 | Invertibility, convergence, normalisation, incremental equivalence |
| **Doctests** | 1+ | Every public item in `libs/domain` and `libs/doc` carries an example that compiles as part of `cargo test`. Documentation that cannot rot, and it forces you to use your own API before shipping it |
| **[`cargo-mutants`](https://mutants.rs/)** | after 1 | You write tests before the code (`agents.md` § stage 1). This answers the question that follows — *are they any good?* It mutates the source and reports which mutants survive |
| **[`criterion`](https://docs.rs/criterion)** | 1, 3, 7 | Statistically sound benchmarks. "It got faster" without a confidence interval is a guess |
| **`cargo-flamegraph` / `dhat`** | 3, 12 | Where the time and the allocations actually go. `perf.html` *draws* these; something has to produce them |
| **[`turmoil`](https://github.com/tokio-rs/turmoil)** | 3, 10 | Deterministic network simulation. Partition, delay, and drop messages against the real code, reproducibly — the only way a split-brain test is worth writing |
| **Linearizability checker** | 3 | You claim CP. Prove it against recorded histories rather than asserting it in a table |
| **`cargo-deny` / `cargo-audit`** | 11 | Licences, advisories, and duplicate dependencies. The cheapest real security work available |
| **[`cargo-semver-checks`](https://github.com/obi1kenobi/cargo-semver-checks)** | 11 | `libs/proto` is a contract eleven services depend on. A silent breaking change there is a runtime failure in four of them |

> **Two of these are cheap and almost nobody does them.** Doctests cost one CI line and make
> every example provably compile. `cargo-mutants` is the only tool that grades a test suite
> rather than the code — and a project that writes tests first should want that grade.

> **Read [Rust Atomics and Locks (Mara Bos)](https://marabos.nl/atomics/) before Phase 3.**
> `SeqCst` everywhere is not a plan.

---

## Rust, DSA & Concepts Map

Every concept is reached through a real feature, not a side exercise. **A row with no
"where" does not belong in this table** — that is the rule that keeps the list finishable
rather than encyclopaedic.

> **What to *read* for each of these lives in [`docs/learning/`](../learning/README.md)**, split by
> track and split again into prerequisites and post-build. This table says what you will learn; that
> directory says where to learn it from. The same rule governs both: a resource with no named
> decision it unlocks does not belong.

Language and standard library come first because they are the axis most easily skipped:
nothing breaks without them, you simply write a more allocating, less expressive version of
the same program.

### The Rust language surface

| Concept | Where | Phase |
|---|---|---|
| **Lifetime-parametric types** | The input-rule and paste parser borrows from `&'src str` and yields spans pointing into it. Nothing else in this project forces a type to carry a lifetime | 1 |
| **`Cow<'src, str>`** | Normalisation allocates; everything else stays borrowed. The type that makes "usually free" expressible | 1 |
| **`#[serde(borrow)]`** | Zero-copy op decode — deserialize borrowing from the frame buffer rather than copying out of it | 3 |
| **Variance and `PhantomData`** | The typestate `Op<Unchecked>` → `Op<Authorized>`; a marker with no runtime cost, and why its variance matters | 13 |
| **`std::thread::scope`** | Scoped threads can borrow from the stack. The clearest demonstration of what lifetimes buy | 4 |
| **The `Ord` / `Eq` contract** | `SortKey` is only correct if its Rust ordering matches Postgres's `COLLATE "C"` byte order. A wrong `Ord` here silently reorders pages | 1 |
| **`Borrow` vs `AsRef` vs `Deref`** | On the newtypes. `Borrow` demands that `Hash` and `Eq` agree with the borrowed form — a stricter contract than it looks | 1 |
| **`IntoIterator` / `FromIterator` / `Extend`** | `collect()` into a rope; extending a block tree from an op batch | 3 |
| **`Index` / `IndexMut`** | Rope slicing by byte range, and why it must panic rather than clamp | 3 |
| **Blanket impls + sealed traits** | The analyzer trait is sealed so a plugin cannot implement it directly — it registers through the capability surface instead | 4, 18 |
| **GATs / lending iterator** | A chunk iterator yielding `&'a str` borrowed from the rope, without allocating a `String` per chunk | 3 |
| **dyn-compatibility** | Why `async fn` in a trait is not object-safe, and what `async_trait` actually generates to work around it (ADR-007) | 1 |
| **Monomorphisation vs `dyn`** | Measure both for `PageRepo` — compile time, binary size, and call overhead. The answer is boring; obtaining it is not | 1 |
| **`unsafe` with a written contract** | The rope's leaf nodes: a type invariant stated on the struct, `// SAFETY:` on every block justifying it, and a public API where misuse is impossible. Miri checks the reasoning, it does not supply it | 3 |

### The standard library, properly

| Concept | Where | Phase |
|---|---|---|
| **`Read` / `Write` / `Seek`** | The WAL — length-prefixed records, CRC32, and a recovery reader that skips a torn tail | 3 |
| **`BufWriter` and `flush()` vs `sync_data()`** | Flushing a buffer is not durability. Getting this wrong means acknowledging an op that a power cut eats | 3 |
| **`BufRead` / `read_exact` / `ErrorKind`** | Recovery: `UnexpectedEof` on the last record is normal, anywhere else is corruption | 3 |
| **`VecDeque`** | The BFS frontier in every graph algorithm — `Vec` gives you DFS by accident | 4, 7 |
| **`BinaryHeap` + `Reverse`** | Scheduled notification digests as a min-heap; the standard library ships a max-heap only | 15 |
| **`BTreeMap` range queries** | The consistent-hashing ring — `range(hash..).next()` *is* the lookup | 10 |
| **The `entry` API** | Symbol table insert-or-update in one lookup rather than two | 4 |
| **`binary_search_by` / `partition_point`** | Locating a `sort_key` in a sorted sibling list | 1 |
| **`dedup_by` / `retain` / `drain`** | Span coalescing, tombstone sweep, draining the op buffer | 1, 3 |
| **`split_at_mut` / `chunks` / `windows`** | Two mutable halves of a rope leaf; sliding windows in the scanner | 3 |
| **Custom hashers** | `rustc-hash` for the internal op-id map — SipHash's DoS resistance is worthless on ids you generated yourself | 3 |
| **Cargo features must be additive** | Cargo takes the *union* across the whole graph. A feature that removes or changes API breaks builds depending on who else is in your dependency tree — nondeterministically | 0, 11 |
| **Feature unification and `resolver`** | You already set `resolver = "3"`. Know what it bought: v2 stopped unifying features across build-, dev-, and target-dependencies; v3 adds MSRV-aware version selection | 11 |
| **`dep:` syntax** | An optional dependency implicitly creates a same-named feature. `dep:` suppresses it so feature names are yours, not your dependencies' | 0 |
| **Feature-gated public API** | `#[cfg(feature)]` on a public item means the API *shape* varies by feature — which fights semver checking and docs.rs unless you add `doc(cfg(…))` | 11 |

### Trees & ordering

| Concept | Where | Phase |
|---|---|---|
| Adjacency list + materialised path (LTREE) | Block tree storage; subtree queries via `<@` | 1 |
| Recursive CTE | Tree reconstruction where LTREE is insufficient | 1 |
| **Fractional indexing** | `sort_key` — reorder by writing one row; greedy midpoint string generation | 1 |
| Interval DP | Optimal `sort_key` rebalancing when keys grow too long | 1 |
| DFS / BFS | Cascade delete, tree render, path computation | 1 |
| **Rope** | In-session text editing — `O(log n)` insert/delete at arbitrary positions | 3 |
| Trie | `[[link]]` and `/command` autocomplete | 7 |
| BK-tree | Fuzzy page-title matching — metric tree, triangle-inequality pruning | 7 |
| **Merkle tree** | Op-log reconciliation — find the divergence point in `O(log n)` instead of shipping the whole log (RFC-002 §9) | 3, 10 |

### Concurrency & lock-free

| Concept | Where | Phase |
|---|---|---|
| **Atomics + memory ordering** | Op sequence generator; `SeqCst` vs `AcqRel` vs `Relaxed` | 3 |
| **CAS loop** | Monotonic op counter without a mutex | 3 |
| **`crossbeam::ArrayQueue`** | Bounded lock-free ring buffer for op batching | 3 |
| **Epoch-based reclamation** | Shared op-log nodes freed safely without a GC | 3 |
| Treiber stack | Undo stack — implement with CAS before reaching for a library | 5 |
| `ManuallyDrop` | Lock-free node ownership transfer | 5 |
| `Arc<RwLock>` vs channels | Session state ownership — when each is right | 3 |

### Graphs — the `[[page link]]` graph

Pages plus their links form a directed graph. This is the richest untapped DSA surface in the product.

| Concept | Where | Phase |
|---|---|---|
| **Cycle detection (DFS with colouring)** | The `LinkCycle` analyzer — `A → B → A`. Three-colour DFS, not a visited set | 4 |
| **Connected components** | Orphan detection — pages unreachable from any root | 4 |
| **BFS shortest path** | "How many hops from this page to that one" in the backlinks panel | 7 |
| **Reachability / transitive closure** | "Everything reachable from here" — and the delete saga's blast radius | 8 |
| **Topological sort** | Ordering saga steps by dependency | 8 |
| **Incremental graph maintenance** | A rename mutates edges; the reverse index must not be rebuilt wholesale (RFC-003 §4) | 4 |
| **All-pairs BFS → diameter** | "How far apart can two pages be" in the graph panel. O(V·E), which is fine at notebook scale and worth knowing *why* it stops being fine | 7 |
| **Force-directed layout** | Positioning the graph panel — repulsion between all pairs, attraction along edges, cooled over fixed iterations | 7 |
| **Barnes–Hut quadtree** | The force sim is O(n²) per step. A quadtree approximating distant clusters as one mass drops it to O(n log n) — the moment the graph outgrows a few hundred pages | 7 |
| **PageRank / power iteration** | Ranking backlinks by *importance* rather than recency. An eigenvector problem solved by repeated sparse matrix-vector multiply — the first numerical algorithm in this project | 21 |
| **Damping and dangling nodes** | A page with no outbound links is a rank sink. The damping factor is not decoration; without it the iteration does not converge to anything useful | 21 |
| **Convergence criteria** | When to stop iterating — L1 delta below a threshold, not a fixed round count | 21 |
| **Union-Find (disjoint set)** | Connected components *and* Kruskal's cycle check — one structure, two jobs. Path compression plus union by rank | 4, 21 |
| **Kruskal's MST** | Bridge suggestions: the fewest, highest-similarity links that would connect an unlinked workspace, over the contracted component graph | 21 |
| **Dynamic connectivity** | Union-Find cannot split. Deleting a link may break a component, and reversing a union is not possible — either recompute or reach for Euler-tour / link-cut trees | 21 |
| **Dijkstra** | Semantic six-degrees: explicit links cost 1, inferred edges cost more. The first *weighted* path problem here — everything else is unweighted, which is why BFS sufficed | 21 |
| **Louvain community detection** | Modularity-optimising clustering, used to re-rank search within your current neighbourhood | 21 |

### Dynamic programming

| Concept | Where | Phase |
|---|---|---|
| **Levenshtein edit distance** | The BK-tree's metric — the tree is *useless* without it, and the triangle inequality only holds because it is a true metric | 7 |
| **LCS / Myers diff** | The version-history diff view — "what changed between these two revisions" | 6 |
| **Traceback → edit script** | The DP table is only half the algorithm; walking it backwards is what produces insertions and deletions (`ui-mockups/diff.html`) | 6 |
| **O(n·m) vs O(n·d)** | The full table is quadratic in *memory*, which is why Myers walks the edit graph instead. Implement the table first so the argument is yours | 6 |
| **Needleman–Wunsch** | Global sequence alignment for the merge assistant. Myers answers "what changed"; NW answers "which blocks correspond", which is what a three-way merge display needs | 21 |
| **Affine gap penalties** | A five-block insertion is one edit, not five. Scoring gap *opening* separately from gap *extension* is what makes an alignment read like a human would describe it | 21 |
| **Interval DP** | Optimal `sort_key` rebalancing — minimum re-keying operations when keys grow too long | 1 |
| Memoised subtree render | Cache render output per block id so one edit re-renders one block | 1 |

### Greedy

| Concept | Where | Phase |
|---|---|---|
| **Fractional index midpoint** | Greedily pick the shortest string between two sort keys | 1 |
| Span coalescing | Merge adjacent identical mark sets in one left-to-right pass | 1 |
| Merkle chunk boundaries | Greedy fixed-size chunking; content-defined is the alternative | 3 |

### Hash-based structures

| Concept | Where | Phase |
|---|---|---|
| **Bloom filter** | Op-id dedup fast path — no false negatives is what makes it safe to skip Redis (RFC-002 §10) | 3 |
| **Consistent hashing ring** | `BTreeMap<u64, NodeId>` — binary-search lookup for "who owns this page" | 10 |
| **Jump hash** | O(1) space, no ring structure — six lines. Implement both, benchmark, note the trade | 10 |
| Hash-based content chunking | Merkle leaf boundaries must be content- or index-defined, never time-defined | 3 |
| **SimHash** | *Similarity-preserving* hashing — the inverse goal of every other hash here. Similar text yields hashes with small Hamming distance instead of uniformly scattered ones | 21 |
| **Sandboxing untrusted code** | Fuel bounds instructions, epochs bound wall clock, `StoreLimits` bounds memory. You need all three, and knowing why is the lesson | 18 |
| **Capability-based security** | An ungranted capability is absent from the world rather than denied at the door. There is nothing to call | 18 |
| **WIT / component model** | A generated, versioned interface instead of hand-rolled pointer-and-length marshalling — which is where sandbox escapes live | 18 |
| **LSH banding** | Comparing a block against every other block is O(n²). Band the hash, bucket by band, and only compare within a bucket — probabilistic recall traded for a linear scan | 21 |
| **Hamming distance at scale** | `count_ones()` over XOR, and the one place SIMD genuinely pays here: comparing a 64-bit fingerprint against thousands at once | 21 |
| **HyperLogLog** | Distinct collaborators from 64 registers holding leading-zero runs. **Constant memory, and no identity retained** — which is what makes the privacy claim structural rather than a policy | 17 |
| **Count-Min Sketch** | Top pages and search terms from a fixed `d × w` table. The estimate is the **minimum** across rows because a collision can only inflate a counter — it never under-counts | 17 |
| **t-digest** | Percentiles from adaptive centroids, fine at the tails and coarse in the middle. Contrast with `perf.html`'s fixed-bucket HDR histogram: same problem, opposite trade | 17 |
| **Sketch mergeability** | All three merge associatively, which is why a per-pod sketch can be summed centrally without shipping raw events | 12, 17 |

### Threads, parallelism & async

| Concept | Where | Phase |
|---|---|---|
| **`rayon` data parallelism** | Analyzers over blocks are independent and CPU-bound — the one genuinely parallel workload. **Server-side only — see § Rayon and wasm32 cannot both be unconditional** | 4 |
| **`spawn_blocking`** | Tantivy indexing blocks; running it on the async runtime starves everything else | 7 |
| **`std::thread` vs tokio task** | When a real OS thread is correct (CPU-bound, long-lived) versus a task | 4, 7 |
| **rayon inside an async runtime** | The trap: rayon's pool and tokio's pool are two thread pools competing for the same cores. Bridge with `spawn_blocking` or a dedicated pool and a oneshot back — never `par_iter` straight from an async fn | 4 |
| **Parallel replay** | Rebuilding many pages' projections for a snapshot — independent per page, so embarrassingly parallel | 6 |
| Runtime sizing | Worker threads vs blocking pool; measure, do not guess |  |
| **Hand-written `Future`/`Stream`** | `poll_next`, `Pin<&mut Self>`, storing and waking a `Waker` — the flush task | 3 |
| **`Pin` and `!Unpin`** | Why a self-referential future cannot move, and what `Pin` actually guarantees | 3 |
| **Cancellation safety** | `select!` drops the losing future mid-`await`. **The async Rust footgun** — a WebSocket read cancelled between "op received" and "op applied" silently loses it | 3 |
| **`JoinSet` / structured concurrency** | Saga step fan-out with bounded lifetime | 8 |
| `tokio::sync::watch` / `broadcast` | Shutdown signalling; op fan-out to session subscribers | 3 |
| **`tokio-console`** | Diagnosing a stalled flush task or a task that never yields | 3 |
| Graceful shutdown | `SIGTERM` → drain queue → sync WAL → close sockets → exit | 12 |

### Memory & layout

| Concept | Where | Phase |
|---|---|---|
| `MaybeUninit<[Node; N]>` | Rope node arrays — initialise without zeroing | 3 |
| `#[repr(C)]` / `#[repr(align(64))]` | Op structs; false-sharing avoidance | 3 |
| `typed-arena` / `slotmap` | Op log — frequent alloc, rare individual free | 3 |
| **Arena reset per pass** | The incremental diagnostics engine allocates many short-lived nodes per analysis and frees none of them individually — `bumpalo`, reset wholesale between passes. Generational indices catch a stale handle | 4 |
| Varint / LEB128 | Op binary encoding — most offsets fit in 1–2 bytes | 3 |
| **`Bytes` refcounted buffers** | Op fan-out to N subscribers — one allocation, N atomic increments, not N copies | 3 |
| **`rkyv` zero-copy decode** | Cast into the buffer instead of allocating; validate first, since the wire is attacker-controlled | 3 |
| CRC32 (`crc32fast`) | WAL per-record checksum — **hardware-accelerated via SSE4.2/CLMUL**; benchmark against a naive table implementation | 3 |
| **False sharing** | `#[repr(align(64))]` on per-instance counters; two atomics in one cache line serialise cores that never touch the same value | 3 |
| **SoA vs AoS** | Op batch layout — iterating one field over 1000 ops touches fewer cache lines as SoA | 3 |
| Branch prediction | The op dispatch `match` — order arms by frequency; measure, do not assume | 3 |
| Cache hierarchy reasoning | Why the rope's node size should relate to a cache line, not to a round number | 3 |

### SIMD — honestly a thin surface here

The previous scope had a SIMD-heavy analytics phase; it was cut. What remains is real but modest, and worth doing for the *reasoning* rather than the wins:

| Concept | Where | Phase |
|---|---|---|
| **`memchr`** | Delimiter scanning in input rules and the paste sanitiser — SIMD-accelerated byte search over the block's text | 1 |
| **`simdutf8`** | UTF-8 validation on the wire decode path; the standard library's validator is scalar | 3 |
| CRC32 intrinsics | `crc32fast` already uses them — read how, and benchmark against a table-driven version | 3 |
| Auto-vectorisation | Check what LLVM does to the span-coalescing loop with `cargo asm`; the skill is *reading* the output | 1 |
| Tantivy's internal SIMD | Conceptual — posting-list intersection and BM25 scoring. Understand what you get for free before hand-rolling | 7 |
| **Bulk Hamming distance** | XOR then `count_ones()` over a slab of 64-bit SimHash fingerprints. Auto-vectorises well, and unlike the rest of this table the win is real rather than educational | 21 |

> If SIMD depth is a goal in itself, it needs a different project. Do not inflate this list to pretend otherwise.

### Measurement & visualisation

Everything the performance console draws (`ui-mockups/perf.html`) has to be computed first.

| Concept | Where | Phase |
|---|---|---|
| **Log-bucket histograms (HDR)** | Latency percentiles. A linear histogram spends all its resolution on the common case and none on the tail, which is the only part that matters | 3 |
| **Percentiles, not means** | p99 ack latency is the editor's felt performance. A mean hides exactly the stalls a user notices | 3 |
| **Reservoir sampling** | Keeping a bounded, unbiased sample of op latencies from an unbounded stream | 3 |
| **Flame-graph aggregation** | Folding stack samples into a prefix tree, then laying it out — the data structure is a trie again | 12 |
| **Squarified treemap** | The wasm bundle breakdown — recursive area subdivision that keeps cells near-square, because long thin rectangles are unreadable | 11 |
| **Welford's algorithm** | Streaming mean and variance in one pass without storing samples | 12 |
| **Columnar vs row storage** | Snapshots are Parquet and analytics is columnar. The same reasoning as SoA-vs-AoS in § Memory & layout, one level up | 6, 17 |
| **Vectorised execution** | Batch-at-a-time instead of tuple-at-a-time — why a columnar engine wins on aggregation. Read [`polars`](https://github.com/pola-rs/polars) rather than only using it: Arrow layout, SIMD kernels, and `rayon` parallelism, in Rust | 17 |
| **Arrow memory format** | The other major zero-copy answer alongside `rkyv`. Reading both is worth more than either | 17 |

### Distributed systems

| Concept | Where | Phase |
|---|---|---|
| **Vector clocks** | Causal ordering of ops across instances | 3 |
| **CRDT convergence** | Sequence CRDT over the rope; no merge dialog ever | 3 |
| **WAL + crash recovery** | `O_APPEND` + `sync_data`; recovery skips torn records | 3 |
| **Outbox pattern** | Dual-write between Postgres and NATS | 1 |
| **Idempotent consumers** | At-least-once delivery; dedupe on `OpId` | 1 |
| **Event sourcing** | The op log *is* the event store | 6 |
| **CQRS** | `history-service` as a projection | 6 |
| **Saga + compensation** | Page deletion across four services | 8 |
| **Consistent hashing** | WebSocket routing by `page_id` | 10 |
| **Jump hash** | O(1) space alternative — implement both, benchmark | 10 |
| **Leases + fencing tokens** | Page ownership. A *recorded* owner that pauses and resumes causes split-brain; a **lease** with a TTL does not (ADR-001) | 10 |
| **φ-accrual failure detector** | Instance heartbeats; rehash on loss | 10 |
| **Merkle anti-entropy** | Replica reconciliation after partition or lease loss | 3, 10 |
| **CAP in practice** | Ops are **CP** — one owner per page, linearizable writes, unavailable during handoff. Search and diagnostics are **AP** — eventually consistent, always available. Two consistency models in one product, chosen deliberately | 3, 4, 7 |
| **PACELC** | Even with no partition, the projection lag between op-ack and Postgres is a latency/consistency trade | 3 |
| **Read-your-writes** | After an ack, a page reload must show your edit — but the projection may lag. Read from the doc-actor, not the projection | 3 |
| **Lamport vs vector clocks** | A Lamport timestamp totally orders but cannot detect concurrency; a vector clock can. Know why the op log needs the latter | 3 |
| **Clock skew** | Why ops are never ordered by wall clock, and why UUIDv7's timestamp is an *index* hint, not an ordering authority | 3 |
| **Exactly-once is impossible** | Two Generals. At-least-once plus idempotent consumers is the achievable target | 1 |
| **Retry storms + jitter** | Every client reconnecting simultaneously after a deploy. Exponential backoff **with jitter**, not without | 3, 10 |
| **Dead letter queue** | Outbox rows that fail repeatedly must leave the hot path, not be retried forever | 1 |
| **Bulkhead isolation** | A slow diagnostics stream must not exhaust the doc-actor's task budget | 4 |
| **Split-brain** | Two nodes believing they own one page. Fencing tokens, not heartbeats, are the fix | 10 |
| **Chandy-Lamport snapshots** | Conceptual — a consistent snapshot of a distributed session without stopping it | 6 |
| Quorum (deliberately absent) | Single-owner-per-page means no write quorum. Worth knowing *why* it does not apply here rather than forcing it in | — |
| **Distributed lock + fencing token** | Snapshot worker mutual exclusion | 6 |
| **Anti-entropy / read repair** | Search index reconciled against Postgres | 7 |
| Two Generals / at-least-once | Why exactly-once is impossible | 1 |
| **SWIM membership** | Decentralised failure detection — random probing, *indirect* probes through k peers, suspicion with refutation, and membership piggybacked on the probe traffic. Removes the Redis registry from the membership path entirely | 10 |
| **Incarnation numbers** | How a wrongly-suspected node refutes its own death. Consensus-lite, and the prettiest idea in SWIM | 10 |
| **A TTL in Redis is not a lock** | The lease store must be linearizable for a TTL alone to be safe, and Redis is not. Fencing tokens are what make it safe anyway — know *why*, do not cargo-cult it | 10 |
| **Tail latency amplification** | The gateway fans out to N services, so p99 is the slowest of N, not the average. **Hedged requests** are the mitigation | 9 |
| **Load shedding ≠ rate limiting** | Rate limiting is per-client and steady-state; shedding is systemic and under duress. It must be cheap, and it must protect in-flight work by rejecting new work | 9 |
| **Head-of-line blocking** | All east-west traffic on gRPC means many streams over one HTTP/2 connection — multiplexed per stream, still serialised at TCP. One large replay delays small calls sharing it (ADR-007) | 9 |
| **Rolling deploys, mixed versions** | Old and new code read the same op log during a rollout. **Consumers deploy before producers, always** | 11 |
| **Serializability vs linearizability** | Both live here: Postgres gives serializable transactions, the doc-actor gives linearizable ops. Different guarantees, constantly conflated | 3, 6 |
| **Idempotency keys** | `CreatePage` is not idempotent today. Client-generated ids are the fix, and the reason is a distributed-systems one (`docs/api/pages.md` §3) | 1 |

### Rayon and `wasm32` cannot both be unconditional

Two rules in this project collide and the collision has not been decided:

| Rule | Source |
|---|---|
| `libs/diagnostics` must stay **`wasm32`-clean** — it runs in the browser | `CLAUDE.md` § Crate Layout |
| Analyzers use **`rayon`** — the one genuinely parallel workload | § Threads, parallelism & async, Phase 4 |

**Rayon does not work in wasm without `wasm-bindgen-rayon`**, which requires `SharedArrayBuffer`,
which requires **COOP/COEP headers** on every response — a real deployment constraint that breaks
some CDN, iframe, and embedding setups, and one that has nothing to do with diagnostics.

Three ways out. Decide before Phase 4, not during:

1. **Feature-gate the parallelism.** `#[cfg(not(target_arch = "wasm32"))]` on the rayon path, a
   sequential fallback in the browser. **The default answer** — the browser analyses one page, the
   server analyses a workspace, so the workloads genuinely differ in size
2. **Sequential everywhere**, and drop the rayon row. Honest, and loses a Concepts Map item
3. **Adopt `wasm-bindgen-rayon`** and accept COOP/COEP. Only if a profile shows single-threaded
   browser analysis is actually too slow — which it probably is not, for one page

Option 1 keeps both rules true and costs one `cfg`. It is recorded here rather than assumed because
**a `cfg` that silently changes the concurrency model is exactly the kind of thing that should be a
written decision**, and because the wasm CI gate will catch it as a build failure if it is not made.

### Verifying the distributed claims

The concept table above is nearly complete. What is missing is **evidence**: `loom` checks
concurrency, `proptest` checks algebra, Miri checks `unsafe`, `cargo-fuzz` checks parsers —
and nothing at all checks the distributed design, which contains the hardest claims in the
project.

| Tool | What it would settle | Phase |
|---|---|---|
| **[`turmoil`](https://github.com/tokio-rs/turmoil)** | Runs the real code against a *simulated* network where you control partitions, latency, and drops — deterministically, in milliseconds. Written by the tokio team for exactly this | 3, 10 |
| **Linearizability checking** | ROADMAP claims ops are **CP with linearizable writes**. A Jepsen-style history checker turns that from a design intention into a tested property | 3 |
| **Fault injection in tests** | Kill the owner mid-flush. Partition an instance from Redis but not from clients — the exact shape that produces split-brain | 10 |

> **A partition test you cannot run deterministically is a partition test you will not run.**
> That is the whole argument for simulation over chaos engineering at this scale: chaos
> finds problems in production, simulation finds them in CI, and only one of those reproduces.

### Compilers & analysis

| Concept | Where | Phase |
|---|---|---|
| **Op ISA + invertibility** | Ops as an instruction set; `apply(invert(op), apply(op,t)) == t` | 3 |
| Delimiter scanner | Input rules — bounded backward scan per keystroke | 1 |
| Tree lowering to multiple backends | Block tree → DOM and → HTML from one tree | 1 |
| Normalisation to canonical form | Span merging; must be idempotent | 1 |
| **Symbol table + reference resolution** | `[[link]]` name → page id; dangling detection | 4 |
| **Query-based incremental computation** | `salsa`-style memo + invalidation for diagnostics | 4 |
| Reverse index / dependency graph | Rename invalidates diagnostics in untouched blocks | 4 |
| Grammar-driven tokenisation | `syntect` for code blocks, compiled to `wasm32` | 1 |
| Recovery vs validation | WAL **recovers** a torn tail; the wire protocol **rejects** a bad frame. Same framing, opposite answers | 3 |
| Position anchoring | Marks *and* diagnostic spans use one mechanism | 3, 4 |

### Search

| Concept | Where | Phase |
|---|---|---|
| Inverted index | Hand-roll it first: posting lists, two-pointer intersection | 7 |
| BM25 scoring | Ranking; implement before adopting Tantivy | 7 |
| Roaring bitmaps | Posting list intersection — array/bitset/RLE containers by cardinality | 7 |
| Tantivy segments | Then replace the naive index and benchmark the gap | 7 |
| **Positional postings** | `(block_id, positions[])` rather than `(block_id)`. Buys phrase search and proximity ranking, and costs index size — the trade is the lesson | 7 |
| **Levenshtein automaton × trie** | A DFA accepting everything within edit distance *k*, walked in lockstep with the trie so whole subtrees prune. **What Lucene and Tantivy actually do**, and the rare one | 7 |
| **Automaton intersection** | The general shape: two state machines advanced together, taking only transitions both accept. Regex-over-index is the same trick | 7 |
| **Op-driven index maintenance** | An `InsertText` touches a handful of postings, not the document. Same incremental-recompute discipline as the diagnostics engine, applied to a second consumer | 7 |

### Security

| Concept | Where | Phase |
|---|---|---|
| Argon2id + PHC strings | Password hashing; parameters upgradable without migration | 2 |
| RS256 + local verification | Gateway verifies with a cached public key — no per-request RPC | 2, 9 |
| Refresh-token rotation | Reuse of a revoked token means theft — revoke the chain | 2 |
| Timing-safe comparison | Token and hash comparison | 2 |
| HTML sanitisation allowlist | Paste is an XSS boundary | 1 |
| Presigned URLs | Direct browser→S3 upload without proxying bytes | 1 |
| Token bucket / sliding window | Gateway rate limiting | 9 |

---

## Phases

### Phase 0 — Foundation ●●○ · *pulled, not pushed*

**Phase 0 is a backlog, not a step.** Nothing here is built up front. Each item is pulled in
by the first service that genuinely needs it, and `libs/` extraction follows
`PROJECT_STRUCTURE.md` §5: inline it, duplicate on the second use, extract on the third.

**Build immediately** (the floor — deferring these costs more than doing them):

| Item | Why it cannot wait |
|---|---|
| Workspace root `Cargo.toml` (`resolver = "3"`) | Retrofitting `[workspace]` means moving `Cargo.lock` and re-resolving every path dep |
| `migrations/0001_init.sql` | The schema is the contract (§8 checklist) — and `sqlx::query_as!` will not compile without it |
| **`libs/proto` feature split** | One feature per service — `document`, `auth`, `collab`. `search-service` has no business compiling the auth protobufs, and designing an *additive* feature set is a different skill from consuming one |
| `docker-compose.yml` — **Postgres only** | Redis, NATS, MinIO, Jaeger, Prometheus, Grafana each arrive with the feature that needs them |
| `config.rs` + `Settings` | "Missing required variable ⇒ fail to start" is a design property; retrofitting it means auditing every field |

**Pull in when the trigger fires:**

| Item | Trigger |
|---|---|
| `libs/domain` | Third consumer of the id newtypes / `Op`. Until then: `src/domain.rs` inside the owning service |
| `libs/infra` | Second service needing telemetry or the `AppError`→`ApiError` chain |
| `libs/macros` — proc macro (`syn`+`quote`) | The third hand-written id newtype. Write the expansion by hand first — that is the point, and it stays on ADR-002's required-tooling list regardless of when it lands |
| `libs/test-utils` — Testcontainers | A test needs NATS or Redis. `#[sqlx::test]` alone covers Phase 1 |
| `libs/proto` | Phase 6 or 9, at the first gRPC pair (ADR-006) |
| Minimal `api-gateway` | The SPA needs one origin — i.e. when `web/` starts calling more than one service |
| CI — fmt, clippy, test, `cargo sqlx prepare --check` | First green test suite worth protecting |
| `cargo-fuzz` + `cargo-miri`, advisory in CI | Phase 1 paste sanitiser / Phase 3 WAL |

**The one trap:** id newtypes and `Op` are ultimately destined for a zero-dependency,
`wasm32`-clean crate. Hanging `sqlx::Type`, `FromRow`, or `utoipa::ToSchema` derives on
them while they live inside a service turns the later extraction from a file move into
unpicking three dependencies that cannot follow them to `wasm32`. Keep the ids
dependency-honest from the start; map at the repo boundary with `#[sqlx(transparent)]`.

**Still true:** `libs/doc` and `libs/diagnostics` need no infrastructure and are
interleavable at any time.

**Study first:** [Zero To Production](https://www.zero2prod.com/) Ch. 1–3 · [The Little Book of Rust Macros](https://veykril.github.io/tlborm/) · [12factor](https://12factor.net/)

| Concept | Where |
|---|---|
| Cargo workspace, `resolver = "3"`, path deps | Linking `libs/` into services |
| `tracing` + `tracing-subscriber` + bunyan | `infra::telemetry` |
| `config` crate + `APP__` env override | `infra::config::get_configuration::<T>()` |
| `thiserror` — `AppError` vs `ApiError` | `infra::error`, one-way `From` |
| `macro_rules!` — `define_id!` | Id newtypes in `libs/domain` |
| **Proc macros — `syn` + `quote`** | `libs/macros`; write the expansion by hand before generating it |
| `#[cfg(target_arch = "wasm32")]` gating | `libs/doc`, `libs/diagnostics` must build for both targets |

**Minimal gateway now, not Phase 9.** The SPA needs one origin from its first request, and downstream services expect `X-Actor-Id` injected. Build a thin reverse proxy plus JWT verification (~150 lines); the full Tower stack lands in Phase 9 behind the same public contract.

**Frontend:** Vite + React 19 + TS strict, Tailwind v4, Radix, TanStack Router/Query, `vite-plugin-wasm`, the `utoipa` → `openapi-typescript` script wired **before** there are endpoints to drift from, and a health dashboard hitting every `/health`.

---

### Phase 1 — Documents ●●○

**Build:** pages and blocks CRUD, tree operations, the outbox, image upload, and the block editor.

**Rust to reach for deliberately here:** the input-rule scanner borrows from `&'src str`
rather than allocating per token — the one place in Phase 1 that forces a lifetime onto a
type. `SortKey`'s `Ord` impl must agree with Postgres's `COLLATE "C"`, so the trait contract
is load-bearing rather than derived. Every public item in `domain.rs` gets a doctest.

**Study first:** [Zero To Production](https://www.zero2prod.com/) Ch. 3–5 · [Figma on fractional indexing](https://www.figma.com/blog/realtime-editing-of-ordered-sequences/) · [Why ContentEditable Is Terrible](https://medium.engineering/why-contenteditable-is-terrible-122d8a40e480) · DDIA Ch. 11 (logs as message storage)

| Concept | Where |
|---|---|
| `sqlx::query_as!`, `FromRow`, `PgPool` | Repositories, compile-time checked |
| `#[sqlx::test]` | Isolated database per test |
| LTREE + recursive CTE | Tree queries |
| **Fractional indexing** | `sort_key` midpoint generation |
| JSONB ↔ tagged enum | `BlockContent` with `content_version` |
| `Arc<dyn Repo>` vs `impl Repo` | Trait objects vs monomorphisation — measure |
| `TryFrom` for validation | Title, path, sort-key newtypes |
| Presigned PUT (`aws-sdk-s3`) | Browser uploads directly, bypassing services |

**The outbox is this phase, not a later one.** `document-service` is the first service that both writes Postgres and publishes NATS. Commit succeeds, publish fails, event lost permanently — search never indexes it, history has a hole, nothing notices. Write the event row in the same transaction; poll with `FOR UPDATE SKIP LOCKED`; accept at-least-once and make every consumer idempotent.

**Editor work (RFC-001):** per-block `contenteditable`, hand-rolled — **no TipTap/ProseMirror/Lexical/Slate**. Input rules, span normalisation, paste sanitisation (an XSS boundary), `syntect`→`wasm32` highlighting. `proptest` laws plus `cargo-fuzz` on paste.

**Frontend:** page tree sidebar, block editor shell, slash menu, drag-and-drop reordering calling `key_between` over `wasm-bindgen`, optimistic edits.

---

### Phase 2 — Auth ●●○

**Build:** registration, login, refresh rotation, gateway verification.

**Study first:** [OWASP Authentication Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Authentication_Cheat_Sheet.html) · [Zero To Production](https://www.zero2prod.com/) Ch. 10 · RFC 8725 (JWT best practices)

| Concept | Where |
|---|---|
| `argon2` PHC strings | Parameters upgradable without migration |
| RS256 signing; **local verification** | Gateway holds the public key — no per-request RPC |
| Refresh rotation with a parent chain | Reuse of a revoked token ⇒ revoke the chain |
| `subtle` constant-time comparison | Timing-attack resistance |
| Redis blocklist keyed by `jti` | Revocation with a TTL |
| tonic unary RPC | Introspection and key rotation (ADR-007) |
| **First-run claim** | A fresh instance has no accounts. The first person to reach it becomes admin and registration closes behind them — a self-hosted product's first screen is setup, not a login (`ui-mockups/signin.html`) |
| Invitation-only by default | Registration is closed unless an admin opens it (ADR-009 §9) |

**Frontend:** first-run setup screen, login/signup, `httpOnly`+`Secure`+`SameSite=Strict` refresh cookie, access token **in memory only** — never `localStorage` — and single-flight silent refresh so N concurrent 401s trigger one refresh.

---

### Phase 3 — Collaboration ●●●

**The richest phase in the project.** Read RFC-002 in full before starting.

**Build:** WebSocket sessions, the rope, sequence CRDT, vector clocks, the op WAL, the batching queue, live cursors.

**Study first:** [Rust Atomics and Locks](https://marabos.nl/atomics/) — **all of it** · [Peritext](https://www.inkandswitch.com/peritext/) · [Crust of Rust](https://www.youtube.com/playlist?list=PLqbS7AVVErFiWDOAVrPt7aYmnuuOLYvOa) on atomics and `Pin` · [tokio on cancellation safety](https://docs.rs/tokio/latest/tokio/macro.select.html#cancellation-safety) — **before writing the WebSocket loop** · DDIA Ch. 5 & 9 · Gossip Glomers challenges 1–4 **in Rust**

| Concept | Where |
|---|---|
| **Rope** | In-session text; `MaybeUninit` node arrays |
| **Sequence CRDT** | Convergence with no merge dialog |
| **Vector clocks** | Causal ordering across instances |
| **Op ISA + invertibility** | Every op defines its inverse (RFC-002 §3) |
| **Position anchors** | Marks and diagnostics both ride this |
| **Atomics + `Ordering`** | Op sequence CAS loop |
| **`ArrayQueue`** | Bounded lock-free batching; back-pressure is real |
| **Epoch reclamation** | Freeing shared nodes without a GC |
| **Hand-written `Stream`** | The flush task — `poll_next`, `Pin`, `Waker` |
| **WAL + CRC32** | Durability before ack; recovery skips torn records |
| **`Write` / `BufWriter` / `sync_data`** | Flushing a buffer is not durability — the ack is only honest after the fsync |
| **`Read` / `BufRead` / `ErrorKind`** | Recovery: `UnexpectedEof` on the final record is expected, anywhere else is corruption |
| **A sound `unsafe` abstraction** | Rope leaves over `MaybeUninit`: invariant on the type, `// SAFETY:` per block, a public API that cannot be misused. Miri checks the reasoning; it does not supply it |
| **GAT lending iterator** | Rope chunks yielded as `&'a str` borrowed from the tree, no allocation per chunk |
| tonic bidi + client streaming | document↔collaboration, collaboration→history |
| **Cancellation safety** | `select!` drops the losing future mid-`await`. A WebSocket read cancelled between "op received" and "op applied" loses it silently — the async footgun that matters most here |
| **`Pin` and `!Unpin`** | What `Pin` guarantees, and why the hand-written `Stream` needs it |
| **CAP, chosen deliberately** | Ops are CP (one owner, linearizable); search and diagnostics are AP. Two models in one product |
| **`memchr` + `simdutf8`** | SIMD byte search in the scanner; SIMD UTF-8 validation on the wire |
| **`Bytes` fan-out** | Relay one op to N subscribers with one allocation |
| **`rkyv` zero-copy wire** | Validate then cast; `criterion` against `bincode` |
| **Bloom filter dedup** | In-memory fast path before the Redis check |
| **Merkle tree over the op log** | Reconnect without replaying everything |
| **Compiler view** | The parse pipeline made visible — raw text → tokens → AST → block tree, side by side, each stage a real intermediate representation the parser actually produces (`ui-mockups/compiler.html`). Real internals rather than a metaphor, which is the same reason the op trace works |
| **BFS as a wavefront** | The graph panel animates a traversal as an expanding frontier — one ring per hop. The most legible way to watch a traversal execute |
| **Op trace view** | A debugger for the log: step an op, watch the rope change, hit invert and watch it undo. `apply(invert(op), apply(op, tree)) == tree` made visible rather than asserted — the proptest law with a UI (`ui-mockups/trace.html`) |
| **Cursor motion trails** | Remote carets leave a fading trail during fast movement only. Positions arrive *sampled*, so the trail between jumps is interpolated and partly fictional — know that before tuning it |

**The representation boundary (RFC-001 §2):** `spans` is storage, `rope + anchored marks` is the working format, exactly one conversion site each way. Span array indices are **not** stable under concurrent edit.

**Tooling:** `loom` for every `Ordering` you reasoned about · Miri over the rope and epoch code · `cargo-fuzz` on the WAL reader · `proptest` for convergence and invertibility · a real `SIGKILL` mid-write recovery test · **`turmoil` for the partition tests** — drop the link mid-session and assert the client's replay produces no lost ops and no duplicates, deterministically.

**Frontend:** live cursors, presence avatars, reconnect with exponential backoff and jitter, IndexedDB offline queue with in-order replay. **No JS merge logic — ever.**

---

### Phase 4 — Diagnostics ●●●

**Build:** the analyzer set, symbol table, reverse index, incremental engine, gRPC server streaming, quick fixes. Read RFC-003 first.

**Study first:** [salsa](https://github.com/salsa-rs/salsa) · [rust-analyzer architecture](https://github.com/rust-lang/rust-analyzer/blob/master/docs/dev/architecture.md) · [LSP spec](https://microsoft.github.io/language-server-protocol/) — diagnostic and code-action shapes

| Concept | Where |
|---|---|
| **Symbol table** | Page name → id; `Unique`/`Ambiguous`/`Missing` |
| **Reverse index** | Rename invalidates referrers — and powers backlinks |
| **Query-based incrementality** | Memo + invalidation; incremental result **must equal** full recompute |
| **Arena per analysis pass** | Many short-lived nodes, freed wholesale — `bumpalo` with a reset between passes, generational handles to catch a stale reference |
| **`rayon` without fighting tokio** | Analyzers over blocks are the one embarrassingly parallel workload. Bridge through `spawn_blocking` or a dedicated pool — `par_iter` from an async fn puts two thread pools on the same cores |
| **Sealed analyzer trait** | Plugins register through the capability surface rather than implementing the trait directly (Phase 18) |

### The tree is the analyser's cheapest advantage

Three checks that are trivial with a typed tree and impossible in a text editor without heuristics
that get it wrong. None needs new infrastructure; all of them ride the reverse index that §4 of
RFC-003 already builds.

| Feature | New surface it names |
|---|---|
| **Analysis scoped by node kind** | Never spell-check inside `code`. Never grammar-check inside `quote` — those are someone else's words. Never count a code block toward reading time. **The single cheapest false-positive removal available**, and it exists only because the tree is typed |
| **Structural outline lints** | Skipped heading level (h1 → h3), a section with exactly one subsection, a list with one item, a document with no h1. Pure tree shape, zero NLP, `hint` severity only |
| **Cross-references at block granularity** | The reverse index is page → page. Make it **block → block** and a figure or definition can show *"3 references"* inline, clicking through to the exact blocks. Same structure, finer key; the cost is index size and it is worth measuring before committing |

**The rule these obey, and the one they must keep obeying:** every one is a fact about *tree shape*.
The moment a check needs to know what prose *means*, it belongs in the rejected pile with citation
detection — see the design note in § Fact dependency graph below.

### Fact dependency graph — a build system for prose

The engine you build here is salsa-shaped query invalidation. Give it a **second dependency
kind** and it becomes a genuinely new product capability: define a value once, reference it
elsewhere, and every reference is flagged stale the moment the definition changes — the way
touching a header recompiles what included it.

| Concept | Where |
|---|---|
| **Explicit edges, not inferred citations** | Define `p99-latency = 180ms` in one block; reference it as a transclusion. **Do not detect that prose "cites" a number** — that is an open NLP problem and would be ~70% right. An explicit name makes the edge exact and the invalidation deterministic |
| **Dirty-mark propagation** | Mark the definition dirty, propagate forward through the dependency DAG in topological order, re-check only what is genuinely downstream. `make`, Bazel, and rust-analyzer are the same algorithm |
| **Cycle rejection** | `a = {{b}}`, `b = {{a}}`. Three-colour DFS again — the analyzer already has it |
| **Contradiction for free** | Two definitions of one name is a hash-lookup collision, not a satisfiability problem. This is why 2-SAT over extracted prose claims was considered and dropped: explicit facts make the useful half trivial and the hard half unnecessary |

Prior art worth knowing: Roam and Obsidian both have block transclusion. **Neither
invalidates.** The dirty propagation is the novel half, and explicit edges are what make it
possible at all.
| Dependency graph | `resolve()` depends on all titles; a rename fans out widely |
| tonic **server streaming** | Results pushed as computed (ADR-006) |
| Graceful degradation | Service dies ⇒ stream fails to open ⇒ editing unaffected |
| Quick fixes as ops | Undoable, synced, in history for free |
| **`rayon` parallel analyzers** | Analyzers over blocks are independent and CPU-bound — the one genuinely data-parallel workload in the project |
| **Cycle detection** | `LinkCycle` — three-colour DFS, not a visited set |
| **Connected components** | Orphan pages unreachable from any root |
| **Bulkhead isolation** | A slow stream must not exhaust the doc-actor's task budget |

**Tooling:** `proptest` for incremental equivalence — the test that catches real invalidation bugs · `cargo-fuzz` for never-panic over arbitrary trees.

**Frontend:** gutter markers, dotted/dashed underlines by severity — **never red** — hover tooltips, one-click fixes, and an inspector panel listing all diagnostics for the page.

---

### Phase 5 — Undo / Redo ●●○

**Build:** per-actor undo across interleaved collaborative edits, operation collapsing.

**Study first:** [Undo Support in Cooperative Work](https://dl.acm.org/doi/10.1145/193233.193247) (Prakash & Knister, 1994) · [Command pattern](https://refactoring.guru/design-patterns/command)

| Concept | Where |
|---|---|
| Command pattern over ops | Inversion already guaranteed by RFC-002 §3 |
| **Per-actor scoping** | Naive global undo reverts other people's work |
| Undo transformation | Inverting your op against ops that landed after |
| Treiber stack (lock-free) | Build with CAS before using a library |
| `VecDeque` bounded history | Ring buffer with a depth limit |
| Monotonic stack | Collapsing a typing run into one undo unit |

---

### Phase 6 — History ●●○

**Build:** op replay, snapshots to object storage, session grouping, the scrubber, restore.

**Study first:** [Event Sourcing — Fowler](https://martinfowler.com/eaaDev/EventSourcing.html) · DDIA Ch. 11 · [The Log — Kreps](https://engineering.linkedin.com/distributed-systems/log-what-every-software-engineer-should-know-about-real-time-datas-unifying-abstraction)

| Concept | Where |
|---|---|
| **Event sourcing** | Replay the log to any point; rows are a projection |
| **CQRS** | A read model separate from the write path |
| Snapshot + delta replay | Snapshot, then apply ops forward |
| **Persistent data structure for point-in-time** | Snapshot-plus-replay is fine for *scrubbing* and O(ops-since-snapshot) for *querying*. "What did we believe on 3 March" wants a path-copying persistent tree over the log — O(log n), structural sharing through `Arc`, and the ownership story is the interesting part. **This is MVCC**, which is how Postgres answers the same question about rows |
| **The palimpsest view** | Every historical version of a paragraph ghosted into one image — older text fainter, current text opaque. Spatial superposition instead of a timeline, and honest: it layers real versions rather than inventing a transition |
| **In-place scrub, not a side panel** | The scrubber pins to the top of the document and morphs the content itself. Scrub instantly — snapshot plus replay is fast — and animate only on release or Play; diffing consecutive states at 60fps while dragging is not worth it |
| **Parquet snapshot format** | Columnar, compressed, self-describing — and queryable in place from object storage without restoring. The alternative was an opaque `rkyv` dump, fast to write and useless to anything but this exact binary (`DATA_MODEL.md` § Snapshot format) |
| **Cloud SQL read replica** | History is the cold path (ADR-001) — replay reads belong on a replica, not the write primary |
| **Distributed lock + fencing token** | Two snapshot workers must not both write |
| tonic client streaming | Batched op ingest with flow control |
| Session grouping | Bursts become user-meaningful entries |
| **LCS / Myers diff** | The revision diff view — what changed between two points |
| Chandy-Lamport (conceptual) | A consistent snapshot of a live distributed session |

**The projection test:** replaying `docs.ops` from empty must reproduce `docs.blocks` exactly. If it does not, the log is not the source of truth.

---

### Tree diff — the algorithm Myers cannot do

Phase 6's diff is Myers over sequences, which answers *what text changed*. You have a **tree**, so
the better question is *what structure changed* — "this section moved and gained a bullet" rather
than "twelve lines differ".

That is **tree edit distance**, and it is a genuinely deeper dynamic-programming problem than LCS:
the classical [Zhang–Shasha](https://epubs.siam.org/doi/10.1137/0218082) algorithm computes it in
O(n²m²) worst case with keyroot decomposition, and the later refinements are worth reading for how
they buy that down.

| | |
|---|---|
| **Why it belongs here** | `diff.html` already exposes the LCS table and argues against it. This is the same move one level up: build the classical algorithm, then find out what it costs |
| **New DSA** | Tree edit distance · postorder keyroots · the insert/delete/relabel cost model. **Nothing else in the project reaches this** |
| **The honest trade** | Myers is O(nd) and fast; tree diff is polynomial with a bad exponent. On a page-sized tree that is fine, and knowing *where* it stops being fine is the lesson |
| **Where it shows** | Block moves in the history diff, and the merge assistant (21), which needs *correspondence* rather than *change* |

**Do Myers first.** The comparison is the point, and a tree diff you cannot contrast with a
sequence diff teaches half as much.

---

### Phase 7 — Search & Backlinks ●●●

**Build:** a naive inverted index first, then Tantivy; trie autocomplete, BK-tree fuzzy matching, the backlinks panel.

**Study first:** [Introduction to Information Retrieval](https://nlp.stanford.edu/IR-book/) Ch. 1–6 — **before writing a posting list** · [Schulz & Mihov, *Fast String Correction with Levenshtein Automata*](https://citeseerx.ist.psu.edu/doc/10.1.1.16.652) · BurntSushi on [`fst`](https://blog.burntsushi.net/transducers/) — read *after* your own attempt

### Two fuzzy searches, deliberately

`BK-tree` and the Levenshtein automaton solve the same problem by opposite means, and
implementing both is the point rather than a redundancy.

| | Prunes by | Good at |
|---|---|---|
| **BK-tree** | Triangle inequality in a metric space — a subtree whose distance bound excludes the query is skipped | A modest set of whole terms, e.g. page titles |
| **Levenshtein automaton × trie** | Automaton intersection — a trie subtree with no surviving automaton state is never entered | A large shared-prefix vocabulary, e.g. every token in the notebook |

The automaton is the rarer skill and the harder build. Two implementation notes that are not
in the papers: construct it for **k ≤ 2** using a universal automaton rather than generating
per-query (arbitrary *k* is where this stops being tractable by hand), and expect the
lockstep traversal — not the automaton — to be where the bugs are.

| Concept | Where |
|---|---|
| Inverted index by hand | Posting lists, two-pointer intersection |
| BM25 | Ranking, implemented before adopting Tantivy |
| Roaring bitmaps | Container types chosen by cardinality |
| Tantivy segments + merge | Then replace and benchmark the gap |
| Trie | `[[` and `/` autocomplete |
| **BK-tree** | Fuzzy titles — triangle inequality prunes subtrees |
| **Anti-entropy** | Index reconciled against Postgres |
| **Levenshtein distance** | The BK-tree's metric — the triangle inequality only holds because it is a true metric, and the tree is useless without it |
| **`spawn_blocking`** | Tantivy indexing blocks; on the async runtime it starves every other task |
| **Positional postings** | `(block_id, positions[])`. Phrase search and proximity ranking fall out; index size is the price |
| **Levenshtein automaton × trie** | A DFA accepting all strings within edit distance *k*, advanced in lockstep with the trie so a subtree that cannot match is never entered. This is what Tantivy does underneath, via `fst` |
| **Op-driven index maintenance** | `InsertText` dirties a handful of postings, not a document. The diagnostics engine's incremental discipline, second consumer |
| **BFS shortest path** | Link distance between two pages in the backlinks panel |
| **Force-free graph layout** | The link-graph panel — a fixed radial layout beats a physics simulation nobody can read (`ui-mockups/search.html`) |
| Facets over the index | Scope by space, restrict to titles, code blocks, or comments |
| **Surfacing index lag** | The index has its own cadence and may trail the write path. The UI states the lag rather than implying a transaction |
| Purge-pending results | A deleted page leaves the index on the saga's schedule, so it appears marked rather than vanishing (ARCHITECTURE §5) |

---

### Phase 8 — Page-Delete Saga ●●○

**Build:** choreographed deletion across four services.

**Study first:** [Saga pattern](https://microservices.io/patterns/data/saga.html) · DDIA Ch. 9

| Concept | Where |
|---|---|
| **Choreography vs orchestration** | Compare both; note the debuggability trade |
| **Forward-only compensation** | A deleted search segment cannot be un-deleted |
| Idempotent handlers | Every step may run twice |
| Timeouts as decisions | A silent service is indistinguishable from a slow one |
| Persisted state machine | `lifecycle_state` — a crash resumes, not restarts |
| **Topological sort** | Ordering saga steps by dependency |
| **`JoinSet` / structured concurrency** | Step fan-out with a bounded lifetime |
| Reachability | The delete's blast radius through the link graph |

ADR-005 stage 3 is **most useful here** — saga sequencing is exactly the orchestration a Go reference should cover. The Rust remains yours.

---

### Phase 9 — API Gateway ●●○

**Build:** the full Tower stack, replacing Phase 0's thin proxy behind the same contract.

**Study first:** [Your Server as a Function](https://monkey.org/~marius/funsrv.pdf) (Eriksen, 2013) · [Tower docs](https://docs.rs/tower)

| Concept | Where |
|---|---|
| `tower::Service` + `ServiceBuilder` | Every middleware layer |
| `BoxCloneService` | Type-erased dynamic routing |
| Atomic counters | Lock-free rate limit state |
| Token bucket / sliding window | Implement both, compare |
| Circuit breaker | closed → open → half-open |
| W3C trace context | Propagation from the browser inward |
| **Tail latency amplification** | Fanning out to N services makes p99 the slowest of N. Measure the fan-out p99 against each dependency's p99 and watch them diverge |
| **Hedged requests** | Re-issue to a second replica once a request passes its own p95. Costs a little throughput, buys back most of the tail — and must be capped, or it *is* a retry storm |
| **Load shedding** | Distinct from rate limiting. Under duress, reject new work cheaply and early to protect work already in flight. A rejection after the expensive part is worse than useless |
| **Head-of-line blocking** | One HTTP/2 connection multiplexes streams but still serialises at TCP. A large `history-service` replay delays small `document-service` calls sharing it — pool connections per destination (ADR-007) |

---

### Phase 10 — Session Routing ●●○

**Build:** consistent-hash WebSocket routing, instance registry, failure detection, rehash.

**Study first:** [Maglev](https://research.google/pubs/pub44824/) (Google, 2016) · [Kleppmann on distributed locks and fencing](https://martin.kleppmann.com/2016/02/08/how-to-do-distributed-locking.html) — **read before designing the lease** · Gossip Glomers challenge 7 (Raft) **in Rust** first

| Concept | Where |
|---|---|
| Consistent hashing ring | `BTreeMap<u64, NodeId>`, binary-search lookup |
| **Jump hash** | O(1) space, no ring structure; implement both and benchmark |
| **Leases + fencing tokens** | The mechanism that actually prevents two owners |
| φ-accrual failure detector | Heartbeat TTL; absence ⇒ suspicion, not certainty |
| Merkle reconciliation | A rejoining node catches up without full replay |
| Session migration | Reconnect, resync, replay the local queue — no lost ops, no duplicates |
| **SWIM membership** | The decentralised alternative to the Redis registry — see below |

### SWIM, after the Redis registry works

Build the centralised registry first: Redis, heartbeat with TTL, φ-accrual over the
inter-arrival times. It works, it is a hundred lines, and it makes the weakness obvious —
**membership now depends on the availability of one Redis, and on a store whose consistency
model you have already decided you cannot trust for leases.**

Then implement [SWIM](https://www.cs.cornell.edu/projects/Quicksilver/public_pdfs/SWIM.pdf)
and delete that dependency. Four ideas, none of which the φ-accrual detector teaches:

| Idea | Why it is worth implementing rather than reading about |
|---|---|
| **Random probing** | Each node probes one random peer per period. Detection time is independent of cluster size, and load per node is constant — the property a heartbeat-to-everyone design does not have |
| **Indirect probes (`ping-req`)** | Before declaring a peer dead, ask *k* other nodes to probe it. This separates "I cannot reach it" from "it is down" — a distinction a centralised detector structurally cannot make |
| **Suspicion** | A suspected node is not immediately evicted; it is announced as suspect and given time. Trades detection latency for a large drop in false positives |
| **Incarnation numbers** | A wrongly-suspected node **refutes its own death** by broadcasting a higher incarnation. Consensus-lite, and the prettiest mechanism in the paper |

Dissemination is infection-style: membership updates piggyback on the probe traffic
already flowing, so there is no separate gossip channel and no broadcast storm. Convergence
is O(log n) rounds.

**Rust:** [`foca`](https://github.com/caio/foca) is a transport-agnostic SWIM implementation
worth reading before writing your own — it separates the protocol from the I/O, which is
the design decision that makes it testable under `turmoil`.

**The comparison is the lesson.** Two failure detectors, one centralised and one not,
measured on the same cluster: detection latency, false-positive rate under a slow node, and
behaviour when the coordinator itself is partitioned away.

### Ownership must be a lease, not a record

A registry entry saying "node 2 owns page X" is insufficient. Node 2 can pause — a long GC, a network partition, `SIGSTOP` — while the cluster concludes it is dead and hands the page to node 5. When node 2 resumes it still believes it owns the page and accepts ops. **That is split-brain on the write path**, and the CRDT will not save you: both nodes write to the same op log with divergent state.

The fix is a **lease with a TTL plus a fencing token**: a monotonically increasing number issued with the lease. Every write carries it, and storage rejects any write bearing a token lower than the highest it has seen. A resumed node's writes are rejected because its token is stale.

> ⚠️ **This is the deepest rabbit hole in the project.** Lease-based ownership handoff with correct fencing is genuinely multi-week work on its own, and it is where distributed systems stops being pattern-matching. Budget for it, and read Kleppmann on fencing before writing a line.

**The hardest frontend test in the project:** kill a pod mid-edit and assert the document converges with no duplicate or lost ops.

---

### Phase 11 — Containers, CI & Self-host Operations ○○○

Multi-stage Dockerfiles with `cargo-chef`, distroless runtime, `docker compose` production profile, GitHub Actions (fmt → clippy → test → build → push), and the **generated-client drift gate** that fails on a dirty OpenAPI regeneration.

**Study first:** [cargo-chef](https://github.com/LukeMathWalker/cargo-chef) · [Docker multi-stage](https://docs.docker.com/build/building/multi-stage/) · [PostgreSQL continuous archiving](https://www.postgresql.org/docs/current/continuous-archiving.html)

**Self-host operations** — ADR-001 promises `docker compose up`, and this is the phase that
makes the promise true. Ported in shape from `genuine-folio`, which already solves it:

| Deliverable | Why |
|---|---|
| `justfile` | Eleven services, sqlx-cli, migrations, protoc. `just dev` and `just check` are worth more than they look |
| `docker-compose.prod.yml` + Caddy | Automatic TLS and reverse proxy. Currently there is no production compose at all |
| `server-setup.sh` | Firewall, swapfile, unattended upgrades, non-root deploy user. Idempotent |
| **Backups + verified restore** | The one that matters most. See below |
| `cargo-deny`, `cargo-audit` | Licence policy, advisory database, duplicate-dependency budget |
| `cargo-semver-checks` on `libs/proto` | Eleven services share that contract; a silent break there fails at runtime, not compile time |
| `cargo-mutants` in a nightly job | Too slow for every push, too useful to skip |
| **`cargo hack --feature-powerset`** | Compiles every feature combination. Almost nobody runs it, and it is how you find that `--no-default-features` has not built in six months |
| **Deploy ordering: consumers first** | During a rollout, old and new code read the same op log and the same protobuf. A consumer that cannot yet decode a new producer's output loses events silently — so consumers always ship first, and the rule belongs in the pipeline rather than in someone's memory |

**Backups are a product surface, not a runbook.** A blog is regenerable from git; a notebook
holds writing that exists nowhere else, so losing it is losing the product. The bar:

| Property | Meaning |
|---|---|
| Scheduled and retained | Daily, then weekly to a year — configured, not remembered |
| **Restore-tested automatically** | The nightly job restores into a scratch database *and replays the op log* to prove the projection rebuilds. A backup nobody has restored is a hypothesis |
| Visible in-product | `ui-mockups/admin.html` § Backups — an operator should not need shell access to know whether they are safe |
| Portable out | `pg_dump` plus the object store. No proprietary export format |

---

### Phase 12 — Kubernetes, IaC, Observability ○○○

See `CLOUD_ROADMAP.md`. K8s manifests, **two distinct HPA strategies** — `collaboration-service` on a custom WebSocket-connection metric, `diagnostics-service` on CPU — Terraform for VPC/Cloud SQL/Memorystore/GCS/GKE, graceful `SIGTERM` draining, OTel → Cloud Trace, Prometheus, Grafana, SLOs.

**In-product health.** The metrics that matter are surfaced inside the app, not only in
Grafana — an operator running one self-hosted instance will not stand up a monitoring stack
(`ui-mockups/admin.html`). The top row is **outbox depth** and **op-log lag**, because those
are the two that fail quietly; service uptime and session counts are context for them. A
degradable service reads amber and pages nobody.

**Study first:** [Kubernetes concepts](https://kubernetes.io/docs/concepts/) · [Terraform GCP provider](https://registry.terraform.io/providers/hashicorp/google/latest/docs) · *Borg* (Verma et al., 2015)

---

## Phases 13–20 — Knowledge Platform (ADR-009)

> **None of these start before Track 1 ships.** ADR-009 § Guard Rails is binding: each
> phase must name new Rust or be cut, and the order below is dependency-driven.

---

### Phase 13 — Identity, Spaces & RBAC ●●○

**Build:** users beyond a single admin, spaces, roles, invitations, and permission
enforcement inside `can_apply(op, actor)` — no second authorization path.

**Study first:** [NIST RBAC model](https://csrc.nist.gov/projects/role-based-access-control) · [Google Zanzibar](https://research.google/pubs/pub48190/) — read for the shape, not to build it · OWASP Access Control Cheat Sheet

| Concept | Where |
|---|---|
| **Permission inheritance down LTREE** | A grant on a page applies to its subtree; `path` becomes load-bearing for authz, not just ordering |
| **Memoised tree walk** | Resolving an effective permission without re-walking ancestors per op |
| Typestate for authorization | `Op<Unchecked>` → `Op<Authorized>` — the compiler refuses an unchecked op reaching a repo |
| Bitflags for capability sets | `bitflags` over a `Vec<Role>`; set algebra instead of iteration |
| `proptest` on `can_apply` | No grant path produces access a deny should have stopped |

**Security gate:** every change here runs `/project:security-review`. This is the phase
where a bug is a breach.

---

### Phase 14 — Comments, Reactions & Mentions ●●●

**Build:** anchored comment threads, resolve/unresolve, `@mentions`, and reactions.
Write RFC-004 first.

**Study first:** [Peritext](https://www.inkandswitch.com/peritext/) — re-read §anchoring · [CRDT counters](https://crdt.tech/) · Prakash & Knister on anchor rebasing

| Concept | Where |
|---|---|
| **Anchor rebasing** | A comment must survive concurrent edits to the text it points at — the same mechanism as marks, now with a second consumer |
| **Orphan detection** | Text a comment anchored to was deleted; orphaned, never corrupt |
| **PN-Counter CRDT** | Reactions converge without coordination or a row lock |
| Comments are **not** ops | They do not mutate the tree, so no ISA entry and no inverse (ADR-009 §2) |
| Threading as an adjacency list | Reuses the tree machinery from Phase 1 |

---

### Phase 15 — Notifications ●●○

**Build:** `notification-service` — the outbox's first real subscriber, which is what
finally justifies the poller.

**Study first:** DDIA Ch. 11 · [NATS JetStream consumers](https://docs.nats.io/nats-concepts/jetstream/consumers) · [Exactly-once is a lie](https://www.confluent.io/blog/exactly-once-semantics-are-possible-heres-how-apache-kafka-does-it/)

| Concept | Where |
|---|---|
| **Time-window fold** | Digest batching as a stream fold, not a cron over a table |
| **Idempotent consumers** | At-least-once delivery, deduped on event id |
| Back-pressure | A slow email provider must not stall the stream |
| `tokio::time` interval vs sleep | Drift accumulation in a long-running batcher |
| Per-user preference fan-out | One event, N delivery decisions |

---

### Phase 16 — The Full Editor ●●●

**Build:** all block directives — callouts, timelines, tabs, columns, math, diagrams,
embeds — plus tables-as-layout, footnotes, slash menu, drag handles, multi-block
selection, fonts, reader modes, and ⌘K. Amend RFC-001's grammar first.

**Study first:** [Why ContentEditable Is Terrible](https://medium.engineering/why-contenteditable-is-terrible-122d8a40e480) — again · [Unicode segmentation](https://unicode.org/reports/tr29/) · [Prince of Persia's copy protection](https://www.youtube.com/watch?v=hn3wJ1_1Zsg) — for how paste sanitisation feels

| Concept | Where |
|---|---|
| **Grammar extension** | Every directive is a `BlockKind` + input rule + grammar entry; `SetBlockKind` already carries `from`/`to`, so conversion stays invertible |
| **Multi-block selection ops** | One user gesture compiling to N ops that must undo as one unit |
| Grapheme clusters | Drag handles and selection must not split a family emoji |
| Paste sanitisation per kind | An XSS boundary — `cargo-fuzz` it, again |
| ⌘K as composition | Client-side over existing search and commands — **no service** |
| **`harper-core` → `wasm32`** | Grammar and spelling as a *second diagnostic source* with its own cadence, never a tenth analyzer (RFC-003 §2.1) |
| Reader prefs as view state | Font, width, spacing must never enter the block tree — the toggle-collapse rule again, with worse consequences |
| **Outline panel** | A tree walk filtered to headings and notable kinds; one implementation, two placements — editor tab and reader rail |
| **Panel takeover** | Selecting a configurable block replaces the tabs rather than adding an eighth one (`ui-mockups/editor.html`) |
| Focus mode, reading progress | Both view state. Progress carries scroll position, which matters more once the scrollbar is gone |
| **Spring scroll in focus mode** | Overshoot-and-settle typewriter scroll. Two catches: it must die under `prefers-reduced-motion`, and taking over scrolling breaks find-in-page |
| **In-place edit heat** | Word-level edit frequency rendered as texture on the prose itself, derived from the op log. **Not amber→red** — that ramp means *diagnostic* and would collide with squiggles on the same text; use neutral→violet. Historical positions must map through **anchors**, which rebase, so a naive offset replay heats the wrong words. Needs a per-space toggle: in a shared space this is soft surveillance |
| **⌘K preview has two cost classes** | `find broken links` runs an analyzer you already have and previews instantly. `summarize this page` is a model call — preview on selection or a long debounce, and show the user which kind a command is |
| Footnotes as sidenotes | Margin when the column leaves room, inline when it does not — a rendering choice over one tree |
| **Structural selection expansion** | ⌘↑ grows the selection to the enclosing node: word → span → sentence → block → list → section. Every good code editor has this and almost no prose editor does, because they hold text and you hold a tree. An ancestry walk plus anchor ranges — **the most *felt* AST feature per hour spent** |
| **Grammar-position-aware completion** | The EBNF in RFC-001 §1 becomes **data rather than prose**, so the slash menu offers only block kinds legal at the cursor and paste rejects invalid nesting. Every grammar rule turns into a lint for free — the payoff for having written the grammar before the code |
| **Snippets keyed by code-block language** | A `code` block already carries `{"language": "rust"}` and you already bundle the grammar. Language-scoped completions inside it are nearly free, and impossible without the tree knowing what kind of block the cursor is in |
| **Section code lens** | Reading time and reference count rendered faint above a heading, computed per subtree. **Reading time only** — a "complexity score" is a number that looks authoritative and measures little |
| **Extract section to a page** | Select a heading, extract the subtree to a new page, leave a link. Pure AST surgery — but it **writes two pages**, so it takes the target-page-only shape from § *The merge assistant writes to one page*: ops on the new page, removal from the source as a separate action with its own undo group |

---

### Phase 17 — Publishing & Distribution ●●○

**Build:** public pages, `publishing-service`, RSS/JSON feeds, sitemap, OG images,
newsletter with double opt-in, and first-party analytics.

**Study first:** [RFC 4287 (Atom)](https://datatracker.ietf.org/doc/html/rfc4287) · [HTTP caching](https://developer.mozilla.org/en-US/docs/Web/HTTP/Caching) · [Plausible's data model](https://plausible.io/data-policy)

| Concept | Where |
|---|---|
| **Static pre-render at publish time** | ADR-004's no-SSR decision holds — publish renders HTML into object storage, CDN serves it. No render per request |
| **Conditional GET** | ETag and `If-None-Match` on feeds; a poller that never re-downloads |
| Cache invalidation on publish | The other hard problem, at last |
| Double opt-in state machine | Typestate over a subscription lifecycle |
| **Privacy by data structure, not policy** | HyperLogLog counts distinct people from 64 bytes containing no identities; Count-Min counts page frequency without storing a page. **You cannot leak what you never stored**, and memory is constant however large the workspace grows |
| **Show the error or do not show the number** | Every estimate is displayed beside its exact counterpart. A sketch that hides its error is indistinguishable from a wrong number (`ui-mockups/analytics.html`) |
| Privacy-preserving counting | No cookies, no cross-site identifiers; published pages only (ADR-009 §5) |
| **Generated share card** | A poster-style image per page — title, three extracted sentences, an edit sparkline, and its four nearest links. Doubles as the OG image. **TextRank is PageRank on a sentence graph**, so it is Phase 21's algorithm reused rather than a new one |
| **The public site itself** | Landing page, install instructions, and the pitch — pre-rendered by the same pipeline that publishes a page, because it *is* one (`ui-mockups/home.html`). A self-hosted product still has to be chosen before it is installed |

---

### Phase 18 — Plugins & Extensibility ●●●

**Build:** `plugin-service` — a `wasmtime` host running untrusted WebAssembly components
that extend two seams: custom block kinds and custom diagnostic analyzers. **Write RFC-005
first**; the capability model is the design, and the sandbox is just its enforcement.

**Study first:** [Component Model & WIT](https://component-model.bytecodealliance.org/) — **before writing any host function** · [wasmtime embedding](https://docs.wasmtime.dev/embed.html) · [Capability-based security](https://en.wikipedia.org/wiki/Capability-based_security) · [Fuel and epoch interruption](https://docs.wasmtime.dev/api/wasmtime/struct.Config.html)

#### The interface is the product

| Concept | Where |
|---|---|
| **WIT-defined world** | The host interface is declared in [WIT](https://component-model.bytecodealliance.org/design/wit.html) and bindings are *generated* for both sides. A hand-rolled `extern "C"` boundary is an FFI, not a plugin system |
| **Interface versioning** | A plugin built against world `v1` must keep working on a `v2` host, or refuse to load with a readable reason. Never trap halfway through |
| **Component model over raw modules** | Typed, composable, and it removes the manual pointer-and-length marshalling that is where sandbox escapes historically live |

#### A plugin never mutates the tree

The rule that makes plugins fit rather than fight the architecture — and it is the same
rule as the assistant (ADR-009 §7):

```
   plugin  →  returns render output, or proposed Op(s)
                              │
                     can_apply(op, plugin_actor)
                              │
                          the op log
```

A plugin that wanted to write blocks directly would bypass the one authorization chokepoint,
break per-actor undo, and desynchronise every peer. Emitting ops means a plugin's edits are
attributable, invertible, and undoable **as the plugin**, exactly like a person's.

#### Bounding untrusted code

| Concept | Where |
|---|---|
| **Fuel metering** | Bounds *instructions*. A plugin cannot spin forever |
| **Epoch interruption** | Bounds *wall clock* — and you need both, because a plugin blocked in a host call burns no fuel. Confusing the two is the classic mistake |
| **`StoreLimits`** | Memory, table size, and instance count per plugin. An OOM inside the sandbox must not touch the host |
| **Stack depth** | Deep recursion is a trap, not a crash |
| **Determinism required** | No wall clock, no randomness, no network by default. An analyzer that returns different results for identical input breaks the incremental engine's memoisation and makes squiggles flicker (RFC-003 §4) |

#### Capabilities are declared, granted, and unreachable otherwise

| Concept | Where |
|---|---|
| **Manifest** | A plugin declares what it needs — read blocks of the current page, persist N bytes, reach one named host. Capability-based security without a manifest is only an API shape |
| **Grant at install** | The user sees the request and grants it. `settings.html` has the instance-level toggle; per-plugin grants live with it |
| **No ambient authority** | An ungranted capability is *absent from the world*, not present-and-denied. There is nothing to call |

#### Making it fast enough to run per keystroke

| Concept | Where |
|---|---|
| **`InstancePre` + pooling allocator** | Instantiating a module per invocation is far too slow for an analyzer that runs on every block. Pre-instantiate, pool, and reset |
| **Async + fuel-based yielding** | A long-running plugin yields rather than blocking the host thread, which is what keeps degradation graceful |
| **Cost is the user's** | A slow plugin slows only its own results. Diagnostics are degradable (ADR-001), so a plugin that exhausts its fuel loses its squiggles and nothing else |

#### Trust, distribution, and debugging

| Concept | Where |
|---|---|
| **Signed artifacts** | A plugin is a signed component file installed from disk. **No registry** — that is a marketplace, and marketplaces need moderation, payments, and takedowns (ADR-001's failure mode wearing a new hat) |
| **Fuzz the host boundary** | Every host function receives attacker-controlled arguments. `cargo-fuzz` on the boundary, not only on the parsers |
| **A plugin test harness** | Authors need golden tests and a readable error surface. "trap" is not a diagnostic |
| **Visible failure** | When a plugin misbehaves the user learns which one, and disabling it is one click. An anonymous failure is indistinguishable from your bug |

---

### Phase 19 — Assistant & Semantic Search ●●●

**Build:** `assistant-service`, and **one embedding index serving two consumers** —
Discover's related pages (Phase 21) and the assistant's retrieval. Write RFC-006 first.

**Study first:** [HNSW paper](https://arxiv.org/abs/1603.09320) (Malkov & Yashunin) · [pgvector](https://github.com/pgvector/pgvector) · [fastembed-rs](https://github.com/Anush008/fastembed-rs) · Anthropic's tool-use docs for the streaming shape

| Concept | Where |
|---|---|
| **The assistant emits `Op`s, not text** | ADR-009 §7 — per-actor undo, collaboration, and audit all come free; writing text directly would forfeit all three |
| **HNSW graph index** | Approximate nearest neighbour — a real graph DSA with a tunable recall/latency trade. Build it, then see below |
| Cosine vs dot product | Why normalisation changes which one is correct |
| **Blocks are already the chunks** | Generic RAG spends its first week on chunking strategy. RFC-001's block tree is a semantic segmentation that already exists — embed per block, not per arbitrary window |
| **Op-driven re-embedding** | An `InsertText` dirties one block. Re-embed on flush, debounced — never per keystroke, or the embedding cost dominates the edit path |
| **Local embeddings** | `fastembed` or `candle` keeps the self-hosted build working without an API key, and is more interesting Rust than the index |
| Streaming responses under back-pressure | Tokens arriving faster than the editor applies them |
| **Degradable by construction** | The assistant is never on the editing path; if it dies, editing is unaffected |

### Build HNSW, then measure it against pgvector

Same pattern as Phase 7 ("naive inverted index first, then Tantivy") and Phase 10
("implement both and benchmark"). Implement HNSW in Rust — `ui-mockups/discover.html`
already proves the design — then benchmark it honestly against
[`pgvector`](https://github.com/pgvector/pgvector), **with the permission filter in the
benchmark rather than left out of it.**

Expect pgvector to win, for three reasons that have nothing to do with speed:

| | Why it matters |
|---|---|
| **Permission-filtered search** | From Phase 13 a reader must never see a page they lack access to. A standalone index can only *post*-filter — ask for k=100, drop what they cannot see, hope 5 survive — and recall collapses exactly when access is narrow. Postgres filters and ranks in one query because the vector and the ACL are in the same database |
| **Transactional freshness** | The embedding commits with the block, so it cannot drift. Compare Phase 7, which needs anti-entropy *because* Tantivy is a separate store |
| **One less thing to operate** | No second index to build, back up, or reconcile |

**The security case is sharper for the assistant than for Discover.** A leaked search
result is a title the reader should not have seen. A leaked *retrieval* is that page
summarised into prose by the model — laundered, unattributed, and impossible for the reader
to know they were not meant to see. Retrieval must be filtered at the index, not after it.

Keeping a hand-rolled index you cannot filter would mean choosing worse software to protect
an exercise already finished. Build it for the learning; adopt what wins.

---

### Phase 20 — Instance Settings & Admin ●●○

**Build:** instance settings, feature flags, admin dashboard, theme families, and the
three-way settings split from ADR-009 §9.

**Study first:** [Feature flags](https://martinfowler.com/articles/feature-toggles.html) — Fowler · [12factor config](https://12factor.net/config) — and where it stops applying

| Concept | Where |
|---|---|
| **Three tiers of configuration** | Compile time, startup, runtime — see below. *"Which tier does this knob belong in"* is the recurring judgement |
| **Runtime config vs startup config** | `Settings` stays the startup schema; feature flags are hot and versioned. Knowing which is which |
| `arc-swap` for hot config | Readers never block on a config reload |
| Three settings scopes | Instance, user, page — different owners, different lifetimes (ADR-009 §9) |
| Theme applied before first paint | No flash of wrong theme; a rendering constraint, not a styling one |

### The three tiers

| Tier | Mechanism | Changes | Cost to change |
|---|---|---|---|
| **Compile time** | Cargo features | what exists in the binary | rebuild + redeploy |
| **Startup** | `Settings`, `APP__` env | how it is wired | restart |
| **Runtime** | feature flags, `arc-swap` | behaviour | none |

Put a knob one tier too low and you redeploy to change a timeout. One tier too high and you
ship dead code behind a flag nobody ever removes.

**The one already decided correctly:** `CLOUD_PORTABILITY.md` §2 selects the `ObjectStore`
and `EventBus` implementation at **startup**, not by Cargo feature. Compile-time selection
would mean one binary cannot run both ways — and, worse, that the integration tests exercise
a different binary than the one that ships.

---

### Phase 21 — Related Content ●●●

**Build:** "you may have already written this" while typing, and a **Discover** panel ranking
related pages. Three independent signals — lexical similarity (SimHash), semantic similarity
(embeddings over the Phase 19 HNSW index), and graph centrality (PageRank) — fused into one
surface.

**Study first:** [Charikar, *Similarity Estimation Techniques from Rounding Algorithms*](https://www.cs.princeton.edu/courses/archive/spring04/cos598B/bib/CharikarEstim.pdf) (SimHash) · [Mining of Massive Datasets](http://www.mmds.org/) Ch. 3 (LSH) · [The PageRank Citation Ranking](http://ilpubs.stanford.edu:8090/422/)

| Concept | Where |
|---|---|
| **SimHash** | A 64-bit fingerprint per block where *similar text produces similar bits*. The opposite goal of every other hash in this project, which exist to scatter |
| **LSH banding** | Comparing one block against every other is O(n²). Split the fingerprint into bands, bucket by band, compare only within a bucket. Recall becomes probabilistic and tunable |
| **Hamming distance** | `(a ^ b).count_ones()`. Comparing one fingerprint against thousands is the one place in this project SIMD genuinely pays |
| **Shingling** | *What* you hash matters more than the hash. Word k-shingles, not characters, or every paragraph is similar to every other |
| **PageRank by power iteration** | Repeated sparse matrix-vector multiply until the L1 delta settles. The first numerical algorithm here — convergence is a property you check, not assume |
| **Damping + dangling nodes** | A page with no outbound links is a rank sink. Without damping the iteration converges to something useless |
| **HNSW as the semantic signal** | The Phase 19 vector index — **one index, two consumers**, not a second store. SimHash finds near-*duplicates* lexically; embeddings + HNSW find pages that are *about* the same thing in different words. Different question, different answer |
| **ANN recall is a dial, not a bug** | `ef` trades recall against nodes visited. The honest metric is **recall@k at a given `ef`** — "compared 40 of 580 pages" is the speed half; without recall it is only half the story |
| **Combining three rankings** | Lexical similarity, semantic similarity, and graph centrality disagree. Reciprocal rank fusion beats a hand-tuned weighted sum and needs no tuning |

### The merge assistant writes to one page

Aligning two pages is a read across both; **applying the merge is a write to one.**
`collaboration-service` owns exactly one page per instance, so there is no atomic two-page op
batch — the same wall that keeps databases and rollups out of scope (ADR-001), surfacing inside
a feature ADR-009 said yes to.

Three ways out. The third is the decision:

1. Make it a saga — Phase 8 machinery, real complexity, partial-failure UI for a convenience
2. Sequential with partial failure — honest and ugly
3. **Model the merge as ops on the TARGET page only.** Content is copied in; deleting the source
   is a separate user action with its own undo group

Option 3 needs no new machinery, preserves one-owner-per-page, and produces better UX besides:
two reversible actions instead of one irreversible cross-page transaction. Recorded here so
Phase 21 never quietly asks for a second ownership tier.

### What the intersections unlock

The three signals above are the phase's core. Everything below reuses that machinery and
costs one algorithm each — build the ones that appeal, not all of them. They are listed in
the order I would build them.

#### Duplicate & merge assistant — LSH → components → alignment

*"You have written near-identical content in three places. Here is a proposed merge."*

Three paradigms chained, each doing what it is actually good at:

| Step | Algorithm | Why this one |
|---|---|---|
| Find candidates | SimHash + LSH banding | Probabilistic, cheap, avoids the O(n²) comparison |
| Group them | Connected components over the near-duplicate graph | **Clusters, not pairs.** Three mutually similar pages collapse into one group; pairwise suggestions would show three separate prompts for one problem |
| Propose the merge | **Needleman–Wunsch** global alignment | Myers gives an edit script; NW gives an *alignment with gaps*, which is what a side-by-side three-way merge needs |

**The output is proposed `Op`s, not merged text.** `MoveBlock`, `DeleteBlock`, `InsertText`
— passed through `can_apply`, landing in the log, undoable as one unit. A text-level merge
would bypass the chokepoint and forfeit undo, the same mistake §7 avoids for the assistant.

#### Bridge suggestions — Kruskal over the component graph

*"These clusters are semantically close and share no links. Here is the link that connects them."*

**Union-Find does double duty**: it is the connected-components detector *and* the cycle
check inside Kruskal's. One structure, two jobs, in one feature.

**Pick the framing before building it**, because the two produce different algorithms:

| Framing | Right answer |
|---|---|
| *"Connect my whole workspace with the fewest, best links"* — a one-shot health action | **MST over the contracted component graph.** Minimises total connecting weight |
| *"Suggest a link on this page"* — inline | **Top-k cross-component pairs by similarity.** A sort. No MST involved |

MST is the elegant one and it is only correct for the first.

#### Reading order — topological sort, not longest path

*"Read these six pages in this order."*

Order that respects prerequisites is a **topological sort** — which Phase 8 already needs
for saga step ordering, so this is that algorithm in a second domain.

**Not longest-path/CPM.** Longest path finds the deepest single chain; it says nothing about
the pages that are not on it, so it cannot order a set.

The weak link is edge inference, not the sort. *"More central means more foundational"* is a
guess — a hub page is as likely to be a summary you read last. Use **explicit links only**
for edges, topologically sort those, break ties by centrality, and say in the UI which pages
were ordered by dependency and which by heuristic.

#### Semantic six degrees — weighted, so Dijkstra

*"How are these two pages connected?"* — even when no chain of links exists.

Augment the link graph with similarity edges, then find the shortest path. **Weight it**:
an explicit link costs 1, a semantic edge costs more the further apart the embeddings are.
Unweighted threshold edges densify the graph until everything is two hops and the answer
stops being interesting.

That also earns **Dijkstra**, which nothing else here justifies — § Graphs specifies BFS
precisely because today's graph is unweighted.

#### Minimum reading set — greedy set cover

*"New hire on the infra team — here are the six pages that cover it."*

Define coverage over the **link-graph neighbourhood** (a page covers itself plus its 1-hop
links), not over extracted concepts — then it is exact and needs no NLP.

Exact set cover is NP-hard; **greedy is provably within ln(n) of optimal**, which is a rare
clean opportunity to reason about an approximation bound rather than an exact algorithm.

Pairs with the reading-order feature above: **set cover picks which pages, topological sort
picks the order.**

#### Two small ones

| Feature | Change |
|---|---|
| **Similarity-weighted PageRank** | Replace binary edge weights with cosine similarity. Small change, meaningfully better ranking |
| **Community-boosted search** | **Louvain** community detection blended into BM25 as a re-rank. ⚠ It builds a filter bubble inside your own notes — the result in the *other* cluster is exactly what search is for. Make the boost visible and labelled, never silent |
| **Silo score, not a silo map** | Louvain on the bipartite user↔page graph projected to user-user (edge weight = pages co-edited) measures how siloed a workspace is. **Ship the aggregate number, not the per-person map** — "here is who actually collaborates" is an org-surveillance tool, and this is a notebook |
| **Voronoi workspace map** | Territory instead of nodes and edges, via Delaunay triangulation. **Fortune's algorithm** is genuinely new DSA here. Honest framing required: it is a Voronoi over a *2-D projection* of embeddings, and adjacency in a projection is not adjacency in embedding space |
| **Betti numbers** | β₀ is the component count you already compute; β₁ is `edges − nodes + components`. Entry-level algebraic topology, and a generative glyph derived from them is genuinely unique to a given workspace |

### The hard problem hiding in all of this

**Union-Find cannot undo.** It supports union, not split. Your link graph is edited
constantly, and deleting one `[[link]]` may split a component — with no way to reverse a
union.

So a live components view over an editable graph forces a real choice:

- **Recompute on deletion.** Correct, trivial, and fine at notebook scale — but measure
  where it stops being fine rather than assuming.
- **Dynamic connectivity** — Euler-tour trees or link-cut trees, which support both
  directions.

This is the most advanced thing in the phase, and it is not a topic anyone assigned: it
falls out of the feature. Discovering a constraint is worth more than reading about one.

**Why this is a phase and not a feature.** Everything else in this project is exact —
B-trees, tries, graph traversal, CRDT convergence. This is the only **approximate** algorithm
family in it: probabilistic recall, tunable precision, an answer that is usually right by
construction rather than always right by proof. That is a genuinely different way to think
and there is nowhere else in Marginal it appears.

**Degradable, and it must say so.** A missed similar block costs nothing; a false one is
noise. Surface it as a hint, never a block on typing, and never with a modal.

---

## Status — Phase 1, `document-service`

**There is no code.** The repository is documentation only: this roadmap, the RFCs and ADRs, the
LLDs in `docs/architecture/lld/`, the reading lists in `docs/learning/`, and the mockups. Every
line of Rust is yours to write, in whatever layout you decide.

That includes the startup path — config, telemetry, routing, graceful drain — and the test suite.
Earlier drafts of this document described a scaffold that has since been deleted deliberately, so
that the structure is derived rather than inherited.

**The order everything else assumes** (`lld/document-service.md` §11 and `lld/libs-doc.md` §8 are
the detail):

| | |
|---|---|
| 1 | Workspace, `docker-compose.yml`, the first migration |
| 2 | Domain types — ids, `SortKey`, validation on construction |
| 3 | `pages` — repo and transport |
| 4 | The editor front end — lexers, parser, lowering |
| 5 | `blocks`, the op write path, the outbox |

> **The LLDs describe a structure you have not committed to.** They were written before the layout
> was yours to choose. Treat them as an argument to push back on, not a spec to obey — and where
> your layout turns out better, **the document is what is wrong.**

**Two things worth honouring whatever the structure**, because they are cheap now and expensive
later — the op log is append-only, so its schema cannot be rewritten:

| | |
|---|---|
| `ops.actor_kind` and `ops.undo_group` **in the first migration** | Backfilling `'user'` is only *correct* before the assistant exists (`DATA_MODEL.md` §1) |
| `sort_key TEXT COLLATE "C"` · ids generated in Rust, never `DEFAULT uuidv7()` | Existing rows sort wrong, or carry ids you cannot un-generate |

### The wasm32 rule needs a gate

`libs/domain`, `libs/doc`, and `libs/diagnostics` must stay `wasm32`-clean and
infrastructure-free — they run in the browser (ADR-004), which is also what keeps them
Miri-reachable and fuzzable. Right now that rule lives in prose, and prose does not fail a
build. It will be broken by an innocent `tokio` import and discovered months later when the
browser bundle stops compiling.

```
rustup target add wasm32-unknown-unknown
cargo check -p domain -p doc -p diagnostics --target wasm32-unknown-unknown
```

`check`, not `build` — it catches the whole class in seconds. Add it with the first workflow,
not at Phase 16 when three phases already depend on those crates staying clean. Best
value-per-effort item in the project: one CI line converts a convention into an invariant.
