# Track 2 — The differentiators · Phases 4, 5, 6

`4 Diagnostics → 5 Undo/Redo → 6 History`

These three are what make Marginal *not a notebook clone*. They are also where the compiler
theory from [§4 of foundations](00-foundations.md#4-compiler-theory--for-the-document-model-ops-and-diagnostics)
pays off, and where Phase 3's invertibility discipline turns undo from a project into a
consequence.

---

# Phase 4 — Diagnostics · `diagnostics-service`

**An incremental compiler front end for prose.** Symbol table, reverse index, salsa-style
invalidation, and the fact dependency graph.

**What you must be able to decide alone at the end:** what a query-based incremental engine is,
why a reverse index is required rather than convenient, which tier a new check belongs in, and
how an arena-per-pass allocator avoids a million small frees.

## Before you build

### Mandatory

| Resource | Type | The decision it unlocks |
|---|---|---|
| [**Salsa book**](https://salsa-rs.github.io/salsa/) — *Overview* + *How Salsa works* | docs | **The engine's design.** Queries, memoisation, and revision-based invalidation. RFC-003 §4's incrementality is salsa-shaped; read the real thing before reimplementing its ideas |
| Niko Matsakis — [**Salsa architecture walkthrough**](https://www.youtube.com/watch?v=_muY4HjSqVw) | video (~1h) | The algorithm explained by its author, walking the source. Denser than the book and worth it |
| [**rustc query system**](https://rustc-dev-guide.rust-lang.org/query.html) + [incremental compilation](https://rustc-dev-guide.rust-lang.org/queries/incremental-compilation.html) | dev guide | The production version of the same idea, at scale. The red/green algorithm is the part to understand |
| matklad — [**Resilient LL Parsing Tutorial**](https://matklad.github.io/2023/05/21/resilient-ll-parsing-tutorial.html) | blog | If you deferred it from Phase 1, it is mandatory now. Analysis runs on *broken* input, always |
| **Crafting Interpreters** Ch. **Resolving and Binding** | owned | A symbol table, built. `[[link]]` name → page id with dangling detection is exactly this chapter's problem |
| [`bumpalo`](https://docs.rs/bumpalo/) docs + [arena allocation in Rust](https://manishearth.github.io/blog/2021/03/15/arenas-in-rust/) — Manish Goregaokar | docs/blog | Arena-per-pass, reset wholesale. The analysis allocates many short-lived nodes and frees none individually |
| [Generational indices / `slotmap`](https://docs.rs/slotmap/) | docs | How a stale handle is *caught* rather than dereferenced. Pairs with the arena |
| [`rayon` FAQ](https://github.com/rayon-rs/rayon/blob/main/FAQ.md) + [rayon inside async — the trap](https://ryhl.io/blog/async-what-is-blocking/) | docs/blog | Analyzers are independent and CPU-bound: the one genuinely parallel workload. **And the trap: never `par_iter` from an async fn** — two thread pools competing for the same cores |

### Optional

| Resource | Type | Why |
|---|---|---|
| matklad — [Explaining rust-analyzer](https://www.youtube.com/playlist?list=PLhb66M_x9UmrqXhUuAaLwO_UT8XORQV8m) | video series | Red-green trees and IDE architecture, by its author. Long; extremely good |
| [Adapton](https://en.wikipedia.org/wiki/Incremental_computing) | project/papers | The academic ancestor of salsa. Read if you want the theory of incremental computation rather than one implementation |
| [KAIST cs420](https://github.com/kaist-cp/cs420) — dataflow analysis lectures | owned course | Formal treatment of the analysis half of a compiler |
| [LSP specification](https://microsoft.github.io/language-server-protocol/specifications/lsp/3.17/specification/) — *Diagnostics* + *Code Action* | spec | You are building diagnostics and quick fixes. The industry's data model for both, worth matching where it is sane |
| [`petgraph`](https://docs.rs/petgraph/) | docs | Only if you decide not to hand-roll the dependency graph. Hand-rolling is the learning; this is the escape hatch |
| Skiena — Ch. *Graph Traversal* | owned | Three-colour DFS and connected components, formally. If `graph-algorithms.html` was enough, skip |

### Read the mockups

`ui-mockups/facts.html` runs the fact graph's topological invalidation, and
`ui-mockups/graph-algorithms.html` runs the cycle and component detection. Both are faster to
read than a paper and they cannot drift from what they claim.

## After it works

| Resource | Why after |
|---|---|
| [Salsa's `#[salsa::tracked]` internals](https://github.com/salsa-rs/salsa) — read the source | Now that you have built invalidation, read how a mature version handles cycles and durability |
| [Rust Performance Book](https://nnethercote.github.io/perf-book/) — *Heap Allocations* | The arena decision was a performance decision. This is how to verify it paid |
| [Semantic — GitHub's incremental analysis](https://github.com/github/semantic) | A different architecture for the same problem, at a scale you will not reach. Interesting rather than necessary |

---

# Phase 5 — Undo / Redo

**The phase that should feel small.** If invertibility was designed in at Phase 3 (RFC-002 §3),
undo is `apply(invert(op))` plus a data structure. If it does not feel small, something upstream
is wrong and this is where you find out.

**What you must be able to decide alone at the end:** what an undo group is, why undo is
per-actor, and whether a lock-free stack is justified here.

## Before you build

### Mandatory

| Resource | Type | The decision it unlocks |
|---|---|---|
| RFC-002 §3 + `DATA_MODEL.md` § *Two columns that must exist from op #1* | project docs | **Read your own docs first.** Undo pops the newest `undo_group` for this actor, not the newest op. That rule is the whole phase |
| **Rust Atomics and Locks** Ch. 9 *Building Our Own Locks* + Ch. 10 *Ideas and Inspiration* | ✅ | Before the Treiber stack. Ch. 10 covers lock-free stacks and queues specifically |
| [Treiber stack](https://en.wikipedia.org/wiki/Treiber_stack) + [KAIST cs431](https://github.com/kaist-cp/cs431) lock-free stack assignment | wiki/course | Implement with CAS before reaching for a library. The ABA problem is why epoch reclamation exists |
| [`ManuallyDrop` docs](https://doc.rust-lang.org/std/mem/struct.ManuallyDrop.html) + Rustonomicon *Drop Flags* | docs | Ownership transfer in a lock-free node. The subtle part |
| [Operational transformation vs CRDT for undo](https://www.figma.com/blog/how-figmas-multiplayer-technology-works/) — the undo section | blog | Figma's undo design in a multiplayer system. Short, and the problem statement is exactly yours: undoing your op when others landed after it |

### Optional

| Resource | Type | Why |
|---|---|---|
| [Automerge undo/redo discussion](https://github.com/automerge/automerge/discussions) | repo | How a real CRDT library handles it. Search rather than read linearly |
| [`loom` + a lock-free stack example](https://docs.rs/loom/latest/loom/) | docs | Loom-test the stack. Non-negotiable if it ships, optional if you keep a `Mutex<Vec<_>>` |
| Jon Gjengset — [Crust of Rust: atomics and memory ordering](https://www.youtube.com/watch?v=rMGWeSjctlY) | video | Reinforcement for Bos Ch. 3 |

## After it works

| Resource | Why after |
|---|---|
| `proptest` law: `undo(do(x)) == x` for **interleaved multi-actor** histories | Not a resource — a test. It is the phase's real deliverable, and writing it after the code is the only time you can |
| [Why undo is hard in collaborative editors](https://www.inkandswitch.com/peritext/) — Peritext §on undo | Reread the section you skimmed in Phase 3. It reads differently now |

---

# Phase 6 — History · `history-service`

**Event sourcing, CQRS, MVCC, and the diff.** Also the palimpsest, which is a *consequence* of a
persistent data structure rather than a feature.

**What you must be able to decide alone at the end:** what event sourcing costs, why a snapshot
is never an origin, why Myers beats the DP table, and what "persistent data structure" means
precisely.

## Before you build

### Mandatory

| Resource | Type | The decision it unlocks |
|---|---|---|
| **DDIA** Ch. *Stream Processing* — the **event sourcing** and **CQRS** sections | owned | The canonical treatment. Command/event distinction, deriving current state, and why the log is the truth |
| **Microservice Patterns** — Richardson, Ch. *Developing business logic with event sourcing* | owned | Event sourcing at the service level: snapshots, versioning, and the pitfalls chapter. Written for your topology |
| Martin Fowler — [Event Sourcing](https://martinfowler.com/eaaDev/EventSourcing.html) + [CQRS](https://martinfowler.com/bliki/CQRS.html) | article | Short, precise definitions. Useful because both terms are used loosely everywhere else |
| James Coglan — [**The Myers diff algorithm, parts 1–3**](https://blog.jcoglan.com/2017/02/12/the-myers-diff-algorithm-part-1/) | blog series | **The best explanation of Myers that exists.** Read all three parts. Then you can implement the DP table first and argue against it honestly |
| Myers — [An O(ND) Difference Algorithm and Its Variations](http://www.xmailserver.org/diff2.pdf) | paper (1986) | The original, after Coglan. Dense, and the divide-and-conquer variation in §4b is the practical one |
| Skiena — Ch. *Dynamic Programming*, LCS section | owned | The table, formally. `ui-mockups/diff.html` runs it live |
| [**CMU 15-721**](https://15721.courses.cs.cmu.edu/) — **MVCC lectures** | owned course | **This is the palimpsest.** Multi-version storage, tombstones, garbage collection of old versions. Pavlo's MVCC lectures are the best treatment available |
| Okasaki — *Purely Functional Data Structures* Ch. 2 (or [the thesis, free](https://www.cs.cmu.edu/~rwh/students/okasaki.pdf)) | book/thesis | What "persistent" means: structural sharing, and why an old version costs nothing to keep |
| [**Chandy–Lamport snapshots**](https://lamport.azurewebsites.net/pubs/chandy.pdf) | paper | Conceptual, and short. A consistent snapshot of a distributed session without stopping it |

### Optional

| Resource | Type | Why |
|---|---|---|
| [Parquet format](https://parquet.apache.org/docs/file-format/) | docs | Snapshots are Parquet. Read the columnar layout section — it is the same SoA-vs-AoS reasoning one level up |
| [Apache Arrow layout spec](https://arrow.apache.org/docs/format/Columnar.html) | spec | The other major zero-copy answer alongside `rkyv`. Reading both is worth more than either |
| [`similar` crate](https://docs.rs/similar/) | docs | A good Rust diff implementation to compare yours against. Read *after* writing yours |
| [Histogram/patience diff](https://luppeng.wordpress.com/2020/10/10/when-to-use-each-of-the-git-diff-algorithms/) | blog | Git's other algorithms and when each wins. Useful for the block-move detection question |
| [Database Internals](https://www.databass.dev/) Ch. *Transaction Processing and Recovery* | owned | Reread with MVCC in mind |
| [Greg Young — Event sourcing talks](https://www.youtube.com/results?search_query=greg+young+event+sourcing) | video | The person who named it. Opinionated; the "versioning an event store" talk is the useful one |

## After it works

### Mandatory

| Resource | Why after |
|---|---|
| Reread `DATA_MODEL.md` §9 *The Log Is Never Truncated* | You now know what compaction would have cost. Confirm the decision or write the ADR that changes it |
| [Versioning an event sourced system](https://leanpub.com/esversioning) — Young (free to read online) | The problem you now definitely have: `encoding_version` exists, and this is the catalogue of migration strategies for an append-only log |

### Optional

| Resource | Why |
|---|---|
| [`ui-mockups/history.html`](../ui-mockups/history.html) — reread the palimpsest source | You implemented the real one. Compare against the mockup's 229-char toy and see what the toy elides |
| [Datomic's architecture](https://docs.datomic.com/pro/overview/architecture.html) | A commercial database built entirely on immutable facts plus time. The most extreme version of your design |
