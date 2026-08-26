# The paper canon

Papers, annotated once, referenced from every phase file. **Ordered by the phase that needs
them, not by fame** — reading the MIT 6.5840 list in course order is not optimal for this
project, because half of it is about consensus you deliberately do not implement.

| Column | Meaning |
|---|---|
| **Tier** | ★ read it · ☆ skim it · ○ know it exists |
| **Cost** | Rough honest reading time for a first pass |

> **How to read a paper you do not need all of.** Abstract → introduction → the one section
> named below → conclusion. Skip the evaluation unless you are about to reimplement it. Most
> papers here are cited for a single idea, and that idea is named in the *Read for* column.

---

## Phase 1 — Documents

| Tier | Paper | Read for | Cost |
|---|---|---|---|
| ☆ | Boehm, Atkinson, Plass — [Ropes: an Alternative to Strings](https://www.cs.tufts.edu/comp/150FP/archive/hans-boehm/ropes.pdf) (1995) | Provenance only. The modern designs (Zed's SumTree, `ropey`) are better teachers | 30 min |
| ○ | Bayer & McCreight — B-trees (1972) | You will use B-trees, not build them. **Database Internals** Part I is the better route | — |

---

## Phase 3 — Collaboration · *the densest paper set in the project*

| Tier | Paper | Read for | Cost |
|---|---|---|---|
| ★★ | Litt, Lim, Kleppmann, van Hardenberg — [**Peritext**](https://www.inkandswitch.com/peritext/) (2022) | **The document model.** Formatting spans anchored to stable character ids, converging deterministically. RFC-001 §9 rests on this | 2 h |
| ★ | Shapiro, Preguiça, Baquero, Zawirski — [A comprehensive study of CRDTs](https://inria.hal.science/inria-00555588/document) (2011) | §3 for counters (Phase 14's PN-Counter), §4 for sets. **Do not read all 50 pages** | 1 h for §3–4 |
| ★ | Lamport — [Time, Clocks, and the Ordering of Events](https://lamport.azurewebsites.net/pubs/time-clocks.pdf) (1978) | Happens-before, and why a Lamport timestamp cannot detect concurrency — hence vector clocks | 45 min |
| ★ | Herlihy & Wing — [Linearizability](https://cs.brown.edu/~mph/HerlihyW90/p463-herlihy.pdf) (1990) | The precise definition you are claiming for ops. Also on the MIT list | 1 h |
| ☆ | Weiss, Urso, Molli — [Logoot](https://inria.hal.science/inria-00432368/document) (2009) | Dense identifiers for sequences. The family your `SortKey` belongs to | 45 min |
| ☆ | Weidner et al. — [Fugue](https://arxiv.org/abs/2305.00583) (2023) | Maximal non-interleaving, provably. What Loro implements and what YATA/RGA can get wrong | 1 h |
| ☆ | Chandy & Lamport — [Distributed Snapshots](https://lamport.azurewebsites.net/pubs/chandy.pdf) (1985) | A consistent snapshot without stopping the system. Short and beautiful | 40 min |
| ○ | Fischer, Lynch, Paterson — [FLP impossibility](https://groups.csail.mit.edu/tds/papers/Lynch/jacm85.pdf) (1985) | Why consensus cannot be guaranteed with one faulty process. Know the result; the proof is optional | — |
| ○ | Ongaro & Ousterhout — [Raft](https://raft.github.io/raft.pdf) (2014) | **You do not implement consensus.** Read §1–5 once so you can say precisely why single-owner-per-page removes the need | 2 h |

---

## Phase 6 — History

| Tier | Paper | Read for | Cost |
|---|---|---|---|
| ★ | Myers — [An O(ND) Difference Algorithm and Its Variations](http://www.xmailserver.org/diff2.pdf) (1986) | The edit-graph formulation, and §4b's divide-and-conquer variation which is the practical one. **Read Coglan's blog series first** | 2 h |
| ★ | Okasaki — [Purely Functional Data Structures](https://www.cs.cmu.edu/~rwh/students/okasaki.pdf) (thesis, 1996) Ch. 2 | Structural sharing. What "persistent" means, and why keeping every version is cheap | 1 h |

---

## Phase 7 — Search

| Tier | Paper | Read for | Cost |
|---|---|---|---|
| ★ | Schulz & Mihov — Fast string correction with Levenshtein automata (2002) · fallback: [McCandless on FuzzyQuery](https://blog.mikemccandless.com/2011/03/lucenes-fuzzyquery-is-100-times-faster.html) | A DFA accepting everything within edit distance *k*. **The blog post is the better first read** and carries the 100× measurement | 1 h |
| ★ | Chambi, Lemire et al. — [Better bitmap performance with Roaring bitmaps](https://arxiv.org/abs/1402.6407) (2014) | Array/bitset/RLE containers by cardinality. Posting-list intersection is where the index spends its time | 1 h |
| ☆ | Robertson & Zaragoza — [The Probabilistic Relevance Framework: BM25 and Beyond](https://www.staff.city.ac.uk/~sbrp622/papers/foundations_bm25_review.pdf) (2009) | Where BM25's saturation and length normalisation come from. **Elastic's blog series is enough for most purposes** | 2 h |
| ○ | Zobel & Moffat — Inverted files for text search engines (2006) | The survey. Manning's IR book Ch. 1–2 covers what you need | — |

---

## Phase 8 — Saga

| Tier | Paper | Read for | Cost |
|---|---|---|---|
| ★ | Garcia-Molina & Salem — [Sagas](https://www.cs.cornell.edu/andru/cs711/2002fa/reading/sagas.pdf) (1987) | The original definition of a compensating transaction. Eight pages | 40 min |

---

## Phase 9 — Gateway

| Tier | Paper | Read for | Cost |
|---|---|---|---|
| ★★ | Dean & Barroso — [**The Tail at Scale**](https://research.google/pubs/the-tail-at-scale/) (2013) | **Why fan-out makes p99 the slowest of N.** Hedged and tied requests, micro-partitions. Eight pages and it changes how you design an edge | 1 h |

---

## Phase 10 — Session routing

| Tier | Paper | Read for | Cost |
|---|---|---|---|
| ★ | Karger et al. — [Consistent Hashing and Random Trees](https://www.cs.princeton.edu/courses/archive/fall09/cos518/papers/chash.pdf) (1997) | The ring. §4 is the part you implement | 1 h |
| ★ | Lamping & Veach — [A Fast, Minimal Memory, Consistent Hash Algorithm](https://arxiv.org/pdf/1406.2294.pdf) (2014) | Jump hash: O(1) space, seven lines of code. **Implement both and benchmark** — a ROADMAP row | 30 min |
| ★★ | Das, Gupta, Motivala — [**SWIM**](https://research.cs.cornell.edu/projects/Quicksilver/public_pdfs/SWIM.pdf) (2002) | Random probing, **indirect probes through k peers**, suspicion with refutation, **incarnation numbers**. The prettiest idea on the roadmap | 1.5 h |
| ★ | Hayashibara et al. — The φ Accrual Failure Detector (2004) · fallback: [Akka's docs](https://doc.akka.io/docs/akka/current/typed/failure-detector.html) | A suspicion *level* instead of a boolean. **Database Internals** Part II covers this too | 1 h |
| ☆ | HashiCorp — [Lifeguard](https://arxiv.org/pdf/1707.00788) (2017) | SWIM's false positives and the three fixes. Read after SWIM | 45 min |
| ☆ | Mirrokni, Thorup, Zadimoghaddam — [Consistent hashing with bounded loads](https://arxiv.org/abs/1608.01350) (2016) | Stops one node getting hot. Read if plain hashing is unbalanced | 45 min |
| ○ | van Renesse & Schneider — [Chain Replication](https://www.cs.cornell.edu/home/rvr/papers/OSDI04.pdf) (2004) | On the MIT list. Not your topology, but the cleanest strong-consistency-without-Paxos design | 1 h |
| ○ | Corbett et al. — [Spanner](https://research.google/pubs/spanner-googles-globally-distributed-database/) (2012) | TrueTime, and what buying a clock gets you. Read for perspective on why you order by vector clock instead | 2 h |

---

## Phase 13 — Identity & RBAC

| Tier | Paper | Read for | Cost |
|---|---|---|---|
| ★ | Pang et al. — [**Zanzibar**](https://research.google/pubs/zanzibar-googles-consistent-global-authorization-system/) (2019) | Authorization as relationship tuples; "who can see this" as graph reachability. §2–3 for the data model | 1.5 h |
| ☆ | Ferraiolo & Kuhn — [Role-Based Access Controls](https://csrc.nist.gov/CSRC/media/Publications/conference-paper/1992/10/13/role-based-access-controls/documents/ferraiolo-kuhn-92.pdf) (1992) | The original RBAC definition, so you use the vocabulary correctly | 40 min |

---

## Phase 17 — Publishing & analytics

| Tier | Paper | Read for | Cost |
|---|---|---|---|
| ★ | Flajolet, Fusy, Gandouet, Meunier — [HyperLogLog](https://algo.inria.fr/flajolet/Publications/FlFuGaMe07.pdf) (2007) | §2–4. Cardinality from leading zeros, and the bias correction | 1.5 h |
| ★ | Cormode & Muthukrishnan — [Count-Min Sketch](https://dsf.berkeley.edu/cs286/papers/countmin-latin2004.pdf) (2005) | Frequency with one-sided error, and the error bound that makes it usable | 1 h |
| ★ | Dunning — [t-digest](https://arxiv.org/abs/1902.04023) (2019) | Rank statistics in constant space. The clustering intuition, not the proofs | 1 h |
| ☆ | Masson, Rim, Lee — [DDSketch](https://arxiv.org/abs/1908.10693) (2019) | *Relative*-error percentiles. Arguably better than t-digest; know why you might switch | 45 min |

---

## Phase 18 — Plugins

| Tier | Paper | Read for | Cost |
|---|---|---|---|
| ☆ | Miller — [Robust Composition](https://web.archive.org/web/20200220081406/http://www.erights.org/talks/thesis/markm-thesis.pdf) (thesis, 2006) | The capability chapters. **No ambient authority** as a principle rather than a slogan. Read selectively | 2 h |
| ○ | Haas et al. — [Bringing the Web up to Speed with WebAssembly](https://dl.acm.org/doi/10.1145/3062341.3062363) (2017) | The WASM design paper. Formal semantics and the sandbox argument | 1.5 h |

---

## Phase 19 — Assistant & semantic search

| Tier | Paper | Read for | Cost |
|---|---|---|---|
| ★★ | Malkov & Yashunin — [**HNSW**](https://arxiv.org/abs/1603.09320) (2016) | Layer assignment, greedy descent, the neighbour-selection heuristic, and the `M`/`ef` parameters. `discover.html` implements this | 2 h |
| ☆ | Malkov et al. — Navigable Small World graphs (2014) | The predecessor. Why greedy search on a small-world graph works at all | 1 h |
| ○ | Subramanya et al. — [DiskANN](https://suhasjs.github.io/files/diskann_neurips19.pdf) (2019) | Billion-scale ANN on SSD. You will never need it; the contrast is instructive | 1.5 h |

---

## Phase 21 — Related content · *the DSA-densest set*

| Tier | Paper | Read for | Cost |
|---|---|---|---|
| ★ | Page & Brin — [The PageRank Citation Ranking](https://snap.stanford.edu/class/cs224w-readings/Brin98Anatomy.pdf) (1999) | Power iteration, damping, dangling nodes. Your first numerical algorithm | 1 h |
| ★ | Broder — [On the resemblance and containment of documents](https://www.cs.princeton.edu/courses/archive/spring13/cos598C/broder97resemblance.pdf) (1997) | Shingling and resemblance. The base of MinHash and SimHash | 45 min |
| ★ | Manku, Jain, Das Sarma — [Detecting near-duplicates for web crawling](https://research.google/pubs/detecting-near-duplicates-for-web-crawling/) (2007) | **The engineering** of Hamming-distance search over SimHash fingerprints. More useful than Charikar's paper | 1 h |
| ★ | Blondel et al. — [Louvain](https://arxiv.org/abs/0803.0476) (2008) | Modularity, and a greedy algorithm that works absurdly well. Short | 45 min |
| ☆ | Charikar — [Similarity estimation from rounding algorithms](https://www.cs.princeton.edu/courses/archive/spring04/cos598B/bib/CharikarEstim.pdf) (2002) | SimHash's origin. Manku's paper covers the practice | 1 h |
| ☆ | Barnes & Hut — [A hierarchical O(N log N) force-calculation algorithm](https://www.nature.com/articles/324446a0) (1986) | The quadtree approximation. [This explainer](http://arborjs.org/docs/barnes-hut) is enough for most purposes | 45 min |
| ○ | Sleator & Tarjan — [A Data Structure for Dynamic Trees](https://www.cs.cmu.edu/~sleator/papers/dynamic-trees.pdf) (1983) | Link-cut trees. **Only if the offline dynamic-connectivity trick is genuinely insufficient** | 2 h |
| ○ | Edelsbrunner & Harer — [Computational Topology](https://www.maths.ed.ac.uk/~v1ranick/papers/edelcomp.pdf) Ch. IV | Betti numbers and boundary maps. `graph-algorithms.html` computes β₀/β₁/β₂ | 2 h |

---

## The MIT 6.5840 list, mapped

You have the [course schedule](https://pdos.csail.mit.edu/6.824/schedule.html). Here is which of
its papers serves this project, and which does not.

| Paper | Verdict for Marginal |
|---|---|
| **Linearizability** (Herlihy & Wing) | ★ **Read.** You claim linearizable writes |
| **Raft** | ○ Read §1–5 once, to justify *not* implementing consensus |
| **Spanner** | ○ Read for perspective on clocks. TrueTime is the road not taken |
| **Chain Replication** | ○ Strong consistency without Paxos. Elegant, not your topology |
| **ZooKeeper** | ○ You use Redis leases + fencing tokens instead. Read §2 to know what a coordination service offers |
| **GFS** | ☆ Skim. The single-master design argument is relevant to one-owner-per-page |
| **MapReduce** | ○ Historical. `rayon` is your parallelism story |
| **FaRM**, **IronFleet**, **Memcached at Facebook** | ○ Out of scope. IronFleet is interesting if formal verification ever tempts you |
| **Bitcoin**, **Practical BFT**, **SUNDR** | ○ Not applicable. Marginal has no Byzantine actors — though [Kleppmann's BFT CRDT paper](https://martin.kleppmann.com/papers/bft-crdt-papoc22.pdf) shows where the research went |
| **Ray**, **On-demand Container Loading** | ○ Skip |

> **Nine of nineteen are worth your time, and only two are ★.** That is not a criticism of the
> course — it is a distributed *systems* course and Marginal is a distributed *application*. The
> difference is that you own one page at a time and never need agreement between replicas.

---

## Reading budget, honestly

| Set | Papers at ★ or above | Time |
|---|---|---|
| Phase 3 | 4 | ~5 h |
| Phase 7 | 2 | ~2 h |
| Phase 9–10 | 4 | ~4 h |
| Phase 17 | 3 | ~3.5 h |
| Phase 19 | 1 | ~2 h |
| Phase 21 | 4 | ~3.5 h |
| **All ★ papers, whole project** | **~18** | **~22 hours** |

Twenty-two hours of paper reading spread over a year. **That is the entire mandatory research
load** — the rest of the learning is books you own and blog posts. Papers feel like the expensive
part and they are not; the expensive part is Phase 3.