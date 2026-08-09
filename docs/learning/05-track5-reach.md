# Track 5 — Reach & extensibility · Phases 17, 18, 19, 21

`17 Publishing → 18 Plugins → 19 Assistant → 21 Related content`

> **Gated on the 🏁**, same as Track 4 (ADR-009 § Guard Rails).

**Phase 21 is the DSA-densest phase in the project** — PageRank, Union-Find, MST, Dijkstra,
Louvain, set cover, sequence alignment, LSH. If you like algorithms, this is the payoff phase, and
`ui-mockups/graph-algorithms.html`, `graph.html`, and `discover.html` all run pieces of it already.

---

# Phase 17 — Publishing & Distribution · `publishing-service`

**An unauthenticated public read path.** The engineering is caching and pre-rendering; the
interesting reading is on analytics sketches and columnar storage.

**What you must be able to decide alone at the end:** why static pre-render beats SSR here, what a
cache key must include, and why a sketch is a *privacy mechanism* rather than an optimisation.

## Before you build

### Mandatory

| Resource | Type | The decision it unlocks |
|---|---|---|
| Flajolet et al. — [**HyperLogLog**](https://algo.inria.fr/flajolet/Publications/FlFuGaMe07.pdf) §2–4 | paper (2007) | Cardinality from leading zeros. Read §2–4; the analysis in §5+ is optional. `analytics.html` runs this with 64 registers beside the exact answer |
| Cormode & Muthukrishnan — [**Count-Min Sketch**](https://dsf.berkeley.edu/cs286/papers/countmin-latin2004.pdf) | paper (2005) | Frequency estimation with one-sided error. Short paper, and the error bound is the whole point |
| Dunning — [**t-digest**](https://arxiv.org/abs/1902.04023) | paper | Rank-based statistics in constant space. The clustering intuition matters more than the maths |
| [Cloudflare — Counting things, a lot of different things](https://blog.cloudflare.com/counting-things-a-lot-of-different-things/) | blog | The three sketches in production, chosen by problem. Read before the papers if they feel dry |
| [**HTTP caching**](https://developer.mozilla.org/en-US/docs/Web/HTTP/Caching) — MDN + [Cache-Control cookbook](https://csswizardry.com/2019/03/cache-control-for-civilians/) | docs/blog | `immutable`, `stale-while-revalidate`, and ETags. A CDN-fronted public path lives or dies on this |
| [Cloud CDN docs](https://cloud.google.com/cdn/docs/caching) | docs | Cache keys, invalidation cost, and signed URLs. Invalidation is slow and expensive — design so you rarely need it |

### Optional

| Resource | Type | Why |
|---|---|---|
| [Parquet file format](https://parquet.apache.org/docs/file-format/) + [Arrow columnar spec](https://arrow.apache.org/docs/format/Columnar.html) | docs | `AnalyticsSink` writes Parquet locally, BigQuery in cloud |
| [Polars internals](https://pola.rs/posts/) | blog | Arrow layout, SIMD kernels, rayon parallelism — in Rust. **Read the source, do not only use it**; it is the best columnar-engine code you can read |
| [`hyperloglogplus`](https://docs.rs/hyperloglogplus/) / [`sketches-ddsketch`](https://docs.rs/sketches-ddsketch/) | docs | Implementations to compare yours against, after |
| [DDSketch paper](https://arxiv.org/abs/1908.10693) | paper | A t-digest alternative with *relative*-error guarantees. Arguably the better choice; worth knowing why |
| [Plausible / GoatCounter on cookieless analytics](https://www.goatcounter.com/help/gdpr) | docs | The product framing: no cookies, no cross-site identifiers, sketches so identity is never retained |
| [RSS 2.0 spec](https://www.rssboard.org/rss-specification) / [JSON Feed](https://www.jsonfeed.org/version/1.1/) | spec | If feeds are in scope |

## After it works

| Resource | Why after |
|---|---|
| Compare each sketch against the exact answer at 10⁴, 10⁶, 10⁸ | Not a resource. `analytics.html` shows error beside every estimate for a reason: **a sketch that hides its error is indistinguishable from a wrong number** |
| [BigQuery `HLL_COUNT`](https://cloud.google.com/bigquery/docs/reference/standard-sql/hll_functions) | You hand-rolled HLL. BigQuery ships it. Benchmark, then decide which ships |

---

# Phase 18 — Plugins & Extensibility · `plugin-service`

**Untrusted code execution.** The entire phase is a security boundary, and the reading is
correspondingly weighted toward sandboxing rather than features.

**What you must be able to decide alone at the end:** fuel vs epoch interruption, what a capability
manifest must enumerate, why there is no registry, and how to fuzz a host/guest boundary.

## Before you build

### Mandatory

| Resource | Type | The decision it unlocks |
|---|---|---|
| [**Wasmtime security docs**](https://docs.wasmtime.dev/security.html) + [sandboxing](https://docs.wasmtime.dev/examples-rust-linking.html) | docs | The threat model of the runtime you are embedding. Read what Wasmtime does *not* protect against |
| [**Interrupting execution**](https://docs.wasmtime.dev/examples-interrupting-wasm.html) + [`Config::epoch_interruption`](https://docs.wasmtime.dev/api/wasmtime/struct.Config.html) | docs | **Fuel vs epoch.** Fuel instruments the compiled code to count instructions precisely and costs throughput; epochs are a coarse deadline at ~10% overhead. You need one, and the choice is a real trade |
| [**Safe Module Termination with Wasmtime Epoch-Based Interruption**](https://www.systemshardening.com/articles/wasm/wasmtime-epoch-interruption-security/) | article | The security-focused treatment of the same choice, including what happens to guest state on termination |
| [**WASI Preview 2 / Component Model**](https://component-model.bytecodealliance.org/) + [`wit-bindgen`](https://github.com/bytecodealliance/wit-bindgen) | docs | The typed interface between host and guest. WIT is how a capability manifest becomes a compile-time contract rather than a README |
| [**Principle of least authority / capability-based security**](https://en.wikipedia.org/wiki/Capability-based_security) then [Mark Miller's *Robust Composition*](https://web.archive.org/web/20200220081406/http://www.erights.org/talks/thesis/markm-thesis.pdf) §on capabilities | wiki/thesis | **No ambient authority.** A plugin cannot ask for the filesystem; it receives exactly the handles it was granted. This is the idea the whole design rests on — read the wiki page at minimum |
| [Zed's extension API](https://zed.dev/docs/extensions/developing-extensions) or [Figma's plugin sandbox writeup](https://www.figma.com/blog/how-we-built-the-figma-plugin-system/) | docs/blog | **Figma's is the one to read.** A production plugin system that had to solve "untrusted code near a document model" and explains why the obvious designs fail |
| `cargo-fuzz` on the host boundary | tool | Every value crossing from guest to host is attacker-controlled. This is not optional; ADR-009 already requires it |

### Optional

| Resource | Type | Why |
|---|---|---|
| [`wasmtime` Rust embedding API](https://docs.rs/wasmtime/) | docs | `Store`, `Linker`, `ResourceLimiter`. The memory-limit knob lives on `ResourceLimiter` |
| [Extism](https://extism.org/) | framework | A plugin framework built on Wasmtime. Read its design; you may not want the dependency but the shape is instructive |
| [Shopify Functions / Fastly Compute](https://shopify.engineering/shopify-webassembly) | blog | WASM for untrusted business logic at scale. Cold-start and limits in practice |
| [Spectre and WASM](https://webassembly.org/docs/security/) | docs | The side-channel limits of the sandbox. Relevant to what you promise users |
| [`wasmtime` fuzzing infrastructure](https://github.com/bytecodealliance/wasmtime/tree/main/fuzz) | repo | How the runtime fuzzes itself. A model for fuzzing your host functions |

## After it works

### Mandatory

| Resource | Why after |
|---|---|
| Run `/project:security-review` | An untrusted-code boundary is the highest-value review in the project |
| Write a deliberately hostile plugin: infinite loop, memory bomb, ops it is not permitted to emit | Not a resource. All three must fail safely. If any succeeds, the phase is not done |

---

# Phase 19 — Assistant & Semantic Search · `assistant-service`

**Two things: vector retrieval, and an LLM that emits `Op`s instead of prose.** The security
constraint is the interesting part — permission-filtered retrieval, at the index rather than after.

**What you must be able to decide alone at the end:** how HNSW navigates, what recall@k costs,
why a post-filtered permission check collapses recall, and why the assistant emits ops.

## Before you build

### Mandatory

| Resource | Type | The decision it unlocks |
|---|---|---|
| Malkov & Yashunin — [**HNSW**](https://arxiv.org/abs/1603.09320) | paper | **The algorithm.** Layer assignment, greedy descent, `M`/`efConstruction`/`ef` and the neighbour-selection heuristic. `discover.html` implements this and measures recall@5 |
| [Pinecone — HNSW explained](https://www.pinecone.io/learn/series/faiss/hnsw/) | tutorial | The paper made visual. Read first if the paper is heavy; it does not replace the paper |
| Malkov et al. — [NSW (the predecessor)](https://en.wikipedia.org/wiki/Small-world_network) or the [small-world background](https://en.wikipedia.org/wiki/Small-world_network) | paper/wiki | Why *navigable small world* graphs support greedy search at all. Ten minutes of context that makes HNSW obvious rather than magic |
| [**pgvector**](https://github.com/pgvector/pgvector) — README + the [indexing section](https://github.com/pgvector/pgvector#indexing) | repo | What you will benchmark against. Read how it does HNSW and IVFFlat, and its filtering limitations |
| `ROADMAP.md` § *Build HNSW, then measure it against pgvector* | project doc | **Read your own decision.** The benchmark must include the permission filter, because that is where post-filtering collapses |
| [**Filtered vector search**](https://www.pinecone.io/learn/vector-search-filtering/) — the pre- vs post-filter problem | article | Why filtering after retrieval destroys recall: you asked for 10 and 9 get filtered out. This is the security-relevant part of the phase |
| [OWASP **Top 10 for LLM Applications**](https://owasp.org/www-project-top-10-for-large-language-model-applications/) — LLM01 prompt injection, LLM06 sensitive disclosure | reference | **A leaked retrieval is laundered through model prose.** The threat model for a retrieval system over private documents |
| [Anthropic — tool use docs](https://docs.claude.com/en/docs/agents-and-tools/tool-use/overview) | docs | The assistant emits `Op`s, which means structured tool output, not text. This is the mechanism |

### Optional

| Resource | Type | Why |
|---|---|---|
| [ANN-Benchmarks](https://ann-benchmarks.com/) | site | Recall/QPS curves across every ANN library. The honest way to size expectations |
| [DiskANN](https://suhasjs.github.io/files/diskann_neurips19.pdf) | paper | The billion-scale answer. You will never need it; the design contrast is instructive |
| [`instant-distance`](https://github.com/instant-labs/instant-distance) / [`hnsw_rs`](https://docs.rs/hnsw_rs/) | repo | Rust HNSW implementations to read after writing yours |
| [Product quantization / IVF](https://www.pinecone.io/learn/series/faiss/product-quantization/) | tutorial | The other family of ANN methods. Know why graph methods won for this workload |
| [Embedding models overview](https://docs.voyageai.com/docs/embeddings) or [MTEB leaderboard](https://huggingface.co/spaces/mteb/leaderboard) | docs/site | Choosing an embedding model. Dimensionality is a storage decision |
| [Anthropic — prompt caching](https://docs.claude.com/en/docs/build-with-claude/prompt-caching) | docs | Relevant once the assistant is on a real budget |
| [Chunking strategies for retrieval](https://www.pinecone.io/learn/chunking-strategies/) | article | Your chunks are **blocks**, which is a better answer than most. Read to confirm |

## After it works

### Mandatory

| Resource | Why after |
|---|---|
| Benchmark HNSW vs pgvector **with the permission filter in the benchmark** | The ROADMAP predicts pgvector wins. Find out. Either result is a finding worth writing down |
| Try to make the assistant leak a document the actor cannot read | Not a resource. If it can, the filter is in the wrong place |

### Optional

| Resource | Why |
|---|---|
| [Anthropic engineering blog](https://www.anthropic.com/engineering) | Agent design patterns, once you have a working assistant to improve |

---

# Phase 21 — Related Content

**The algorithm phase.** More DSA per week than anything else in the project, and it is the one
place a *numerical* algorithm appears.

**What you must be able to decide alone at the end:** why PageRank needs damping, why Union-Find
cannot undo, when Dijkstra is required over BFS, and what a set-cover approximation guarantees.

## Before you build

### Mandatory, grouped by feature

**Similarity and near-duplicates**

| Resource | Type | The decision it unlocks |
|---|---|---|
| Broder — [**On the resemblance and containment of documents**](https://www.cs.princeton.edu/courses/archive/spring13/cos598C/broder97resemblance.pdf) | paper | Shingling and resemblance. The foundation under both MinHash and SimHash |
| Charikar — [**SimHash**](https://www.cs.princeton.edu/courses/archive/spring04/cos598B/bib/CharikarEstim.pdf) + Manku et al. — [**Detecting near-duplicates for web crawling**](https://research.google/pubs/detecting-near-duplicates-for-web-crawling/) | paper | SimHash and the *practical* Hamming-distance search over fingerprints. The Manku paper is the one with the engineering |
| [Mining of Massive Datasets](http://www.mmds.org/) Ch. 3 — **LSH** | free book | **The best treatment of LSH anywhere.** Banding, the S-curve, and how to pick bands/rows for a target threshold. Free PDF |

**Graph ranking and structure**

| Resource | Type | The decision it unlocks |
|---|---|---|
| Page & Brin — [**The PageRank Citation Ranking**](https://snap.stanford.edu/class/cs224w-readings/Brin98Anatomy.pdf) | paper | Power iteration, damping, and dangling nodes. **Damping is not decoration** — without it the iteration does not converge to anything useful |
| Skiena — Ch. *Weighted Graph Algorithms* | owned | MST (Kruskal), shortest path (Dijkstra), and the union-find that Kruskal needs |
| [**cp-algorithms — DSU**](https://cp-algorithms.com/data_structures/disjoint_set_union.html) | reference | Path compression + union by rank, with the complexity proof sketch. The reference implementation to check yours against |
| [**cp-algorithms — Dijkstra**](https://cp-algorithms.com/graph/dijkstra.html) + [MST Kruskal](https://cp-algorithms.com/graph/mst_kruskal.html) | reference | Both, tersely. Your first *weighted* path problem — everything before this was unweighted, which is why BFS sufficed |
| Blondel et al. — [**Louvain**](https://arxiv.org/abs/0803.0476) | paper | Modularity-optimising community detection. Short paper, greedy algorithm, and modularity itself is the idea to understand |
| `ROADMAP.md` § *The hard problem hiding in all of this* | project doc | **Union-Find cannot split.** Deleting a link may break a component and un-unioning is not possible. Read your own note before you build on DSU |

**Dynamic connectivity — the hard part**

| Resource | Type | The decision it unlocks |
|---|---|---|
| [**cp-algorithms — Dynamic connectivity**](https://cp-algorithms.com/data_structures/deleting_in_log_n.html) (offline) | reference | The offline trick: process a query tree with a rollback-capable DSU. Often enough, and far simpler than the online structure |
| [Link-cut trees — USACO Guide](https://usaco.guide/adv/link-cut-tree) + [Codeforces tutorial](https://codeforces.com/blog/entry/80383) | tutorial | The online answer: splay trees over preferred paths. **Read only if the offline version is genuinely insufficient** — this is a large amount of machinery |

**Alignment and coverage**

| Resource | Type | The decision it unlocks |
|---|---|---|
| [Needleman–Wunsch](https://en.wikipedia.org/wiki/Needleman%E2%80%93Wunsch_algorithm) + [affine gap penalties (Gotoh)](https://en.wikipedia.org/wiki/Gap_penalty#Affine) | wiki | Global alignment for block correspondence. **Myers answers "what changed"; NW answers "which blocks correspond"** — a three-way merge display needs the second. Affine gaps are what make a five-block insertion read as one edit |
| Skiena — Ch. *Combinatorial Search and Heuristic Methods* + the catalog entry for **Set Cover** | owned | Greedy set cover and its ln(n) approximation bound. The minimum reading set is exactly this |
| [Set cover — the greedy bound](https://en.wikipedia.org/wiki/Set_cover_problem) | lecture notes | Why greedy is ln(n)-approximate and why you cannot do better in polynomial time. One page of the proof is worth it |

**Geometry and topology (optional-adjacent but recorded)**

| Resource | Type | The decision it unlocks |
|---|---|---|
| [Fortune's algorithm](https://en.wikipedia.org/wiki/Fortune%27s_algorithm) + [Voronoi/Delaunay duality](https://en.wikipedia.org/wiki/Delaunay_triangulation) | wiki | `graph.html` computes exact Voronoi by half-plane intersection in O(n²) and names Fortune's as the O(n log n) replacement. Read to understand what you would be replacing |
| [Computational Topology: An Introduction](https://www.maths.ed.ac.uk/~v1ranick/papers/edelcomp.pdf) — Edelsbrunner & Harer, Ch. IV | book | Betti numbers, boundary maps, and why β₂ needs a *chosen* complex. `graph-algorithms.html` computes β₀/β₁/β₂ and the reasoning is here |

### Optional

| Resource | Type | Why |
|---|---|---|
| Barnes & Hut — [A hierarchical O(N log N) force-calculation algorithm](https://www.nature.com/articles/324446a0) or [this explainer](http://arborjs.org/docs/barnes-hut) | paper/blog | The quadtree that replaces the O(n²) force loop. The explainer is enough |
| [`ui-mockups/graph-algorithms.html`](../ui-mockups/graph-algorithms.html) + [`graph.html`](../ui-mockups/graph.html) + [`discover.html`](../ui-mockups/discover.html) | mockups | **Read the source of all three.** They run BFS wavefronts, Betti numbers, exact Voronoi with its Delaunay dual, and HNSW with measured recall. Cheaper than any paper on this list |
| [Mining of Massive Datasets](http://www.mmds.org/) Ch. 5 (link analysis), Ch. 10 (social graphs) | free book | PageRank variants and community detection, textbook-style |
| [`petgraph` algorithms](https://docs.rs/petgraph/latest/petgraph/algo/index.html) | docs | What you could have used. Read after implementing, to compare |
| [Topological sort](https://cp-algorithms.com/graph/topological-sort.html) | reference | Reading order is a topological sort — **not** longest-path or CPM, which is the mistake this project already corrected once |

## After it works

| Resource | Why after |
|---|---|
| Measure PageRank convergence: L1 delta per iteration, and the iteration count at three damping factors | Not a resource. "Convergence criteria" is a ROADMAP row and it only means something with a curve |
| [Skiena](https://www.algorist.com/) Part II catalog — reread the entries you used | The catalog is a lookup table. After using six entries, the surrounding ones become legible |