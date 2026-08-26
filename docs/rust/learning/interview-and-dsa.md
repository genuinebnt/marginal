# Hello Interview · Striver / NeetCode · Educative

Three subscriptions you already pay for, and none of them is a curriculum for this project. They
are **drills, ramps, and rehearsal** — used correctly they save weeks; used as the primary source
they will quietly replace building with studying.

| Asset | What it is good for | What it is not |
|---|---|---|
| **Hello Interview** | System-design *vocabulary* and the rehearsal for explaining Marginal out loud | Not a design authority. Interview answers are compressed by design and Marginal is not compressed |
| **Striver / NeetCode** | Making the classical algorithms *automatic* so a phase is not blocked on remembering how DSU works | Not coverage. Zero of these lists touch fractional indexing, FSTs, HNSW, PageRank, or CRDTs |
| **Educative** | An hour to decide whether a topic is what you think it is, before committing to a book | Never the source you cite. Interactive courses optimise for progress bars, not depth |

---

# 1. Hello Interview — where it actually maps

Their content is organised as **core concepts** plus **system breakdowns**. Both are useful here
but for different reasons.

## Read before the phase

| Their material | Phase | Why it earns the time |
|---|---|---|
| **Collaborative document editor / Google Docs** breakdown | 3 | **This is your project, compressed to 45 minutes.** Read it early — before Phase 3 — specifically to notice what an interview answer *omits*: anchors, invertibility, WAL durability, IME. The omissions are the map of where the real difficulty lives |
| **Consistent hashing** deep dive | 10 | The ring, virtual nodes, and rebalancing, explained operationally. Read *before* Karger's paper as the ramp |
| **API gateway / rate limiting** | 9 | Token bucket vs sliding window, and where limiting belongs in the request path |
| **Caching** deep dive | 17 | Cache keys, invalidation, and the CDN layer. Pairs with the HTTP-caching reading |
| **WebSockets / real-time delivery** | 3, 10 | Connection state, sticky routing, and reconnection. Your session-routing problem stated at interview granularity |
| **Monitoring & observability** | 12 | The four golden signals, before the SRE book chapters |
| **Their delivery framework** (how to structure an answer) | — | Use it to *present* Marginal. Requirements → estimates → API → data model → high level → deep dives is a genuinely good order, and it is roughly how your own docs are already arranged |

## The honest caveat

Interview system design and real system design diverge in three specific ways, and knowing which
is which protects you:

| Interview answer | Marginal's reality |
|---|---|
| "Use a CRDT for collaborative text" — one sentence | RFC-001 §9, an anchor type, tombstone GC, and Peritext | 
| "Shard by document id with consistent hashing" | Leases, fencing tokens, φ-accrual, SWIM, split-brain tests |
| "Store the document as JSON in a blob store" | An op log as the source of truth, with a projection that replay must reproduce |

**Use Hello Interview to learn the shape of an answer and the names of things.** Do not use it to
decide anything. A design that fits on a whiteboard has already been simplified past the point
where the interesting decisions live — and those decisions are the entire reason this project
exists.

## Where it is genuinely better than a paper

Two places:

- **Estimation.** Their capacity-estimation material is better than anything academic. `TIMELINE.md`
  and `RFC-002 §7`'s "15k editors × 2 keystrokes/sec, batched 20:1" is exactly this skill.
- **Trade-off articulation.** Saying *why* out loud, quickly, without hedging. That is a real
  skill and papers do not teach it.

---

# 2. Striver / NeetCode — drills, mapped per phase

**Do these the week *before* the phase, not during.** The point is that when Phase 21 needs
Union-Find you are thinking about *link deletion*, not about path compression.

Every problem below is here because the phase uses the pattern. **Nothing is listed for
completeness.** If a phase is not listed, no drill helps it — Phase 3's rope and CRDT have no
LeetCode analogue, which is itself worth noticing.

## Phase 1 — Documents

| Pattern | Problems | Where the phase uses it |
|---|---|---|
| Binary search on a sorted range | *Search in Rotated Sorted Array* · *Find Minimum in Rotated Sorted Array* | `binary_search_by` / `partition_point` locating a `sort_key` in a sibling list |
| Intervals & merging | *Merge Intervals* · *Insert Interval* · *Non-overlapping Intervals* | **Span coalescing** — merging adjacent identical mark sets in one left-to-right pass |
| Tree DFS/BFS on a parent-child structure | *Binary Tree Level Order Traversal* · *Subtree of Another Tree* | Block-tree render and cascade delete |
| String scanning | *Longest Substring Without Repeating Characters* | The bounded backward scan in an input rule |

**Skip** the whole two-pointer and sliding-window section for this phase. It does not appear.

## Phase 4 — Diagnostics

| Pattern | Problems | Where the phase uses it |
|---|---|---|
| **Cycle detection in a directed graph** | *Course Schedule* · *Graph Valid Tree* | `LinkCycle` — and note that the standard solution *is* three-colour DFS even when tutorials call it "visited + visiting" |
| **Connected components** | *Number of Connected Components in an Undirected Graph* · *Number of Islands* | `OrphanPage` — components, not `backlinks == 0` |
| Topological sort | *Course Schedule II* · *Alien Dictionary* | Dirty-mark propagation through the fact dependency graph |
| Hash map with the entry pattern | *Group Anagrams* · *Top K Frequent Elements* | Symbol-table insert-or-update in one lookup |

## Phase 6 — History

| Pattern | Problems | Where the phase uses it |
|---|---|---|
| **LCS** | *Longest Common Subsequence* | The diff table. **Do this one before reading Myers** — the DP table has to be automatic before the argument against it means anything |
| **Edit distance** | *Edit Distance* | Levenshtein, which returns in Phase 7 as the BK-tree metric |
| DP traceback | *Longest Increasing Subsequence* (reconstruct the sequence, not just its length) | The DP table is half the algorithm; walking it backwards produces the edit script |

> **Reconstruct, do not just count.** Most solutions to these return a number. Force yourself to
> return the actual subsequence — that is the traceback, and it is the half the phase needs.

## Phase 7 — Search & Backlinks

| Pattern | Problems | Where the phase uses it |
|---|---|---|
| **Trie** | *Implement Trie (Prefix Tree)* · *Design Add and Search Words Data Structure* · *Word Search II* | `[[link]]` autocomplete. **The second problem is the important one** — wildcard matching over a trie is one step from walking a Levenshtein DFA in lockstep with it |
| **BFS shortest path** | *Word Ladder* · *Rotting Oranges* | Link distance in the backlinks panel. *Rotting Oranges* is multi-source BFS, which is the wavefront in `graph-algorithms.html` |
| Sorted-list intersection | *Intersection of Two Arrays* (do the two-pointer version, not the hash-set one) | **Posting-list intersection.** The two-pointer merge is literally the inverted-index operation |
| Heap for top-k | *Top K Frequent Elements* · *Kth Largest Element in a Stream* | BM25 result ranking |

## Phase 8 — Saga

| Pattern | Problems | Where the phase uses it |
|---|---|---|
| Topological sort | *Course Schedule II* | Ordering saga steps by dependency |
| Reachability / transitive closure | *Pacific Atlantic Water Flow* · *Clone Graph* | The delete blast radius — forward reachability before the saga starts |
| State machines | *Design* problems generally | `lifecycle_state` resuming rather than restarting |

## Phase 15 — Notifications

| Pattern | Problems | Where the phase uses it |
|---|---|---|
| **Min-heap via `Reverse`** | *Task Scheduler* · *Kth Largest Element in a Stream* · *Find Median from Data Stream* | Scheduled digests. The last one is two heaps and is the closest classical problem to a streaming quantile |
| Fan-out design | *Design Twitter* | Fan-out on write vs read, at a scale you will never reach, with the trade named |

## Phase 21 — Related Content

| Pattern | Problems | Where the phase uses it |
|---|---|---|
| **Union-Find with path compression + union by rank** | *Redundant Connection* · *Number of Connected Components* · *Accounts Merge* | Components *and* Kruskal's cycle check. **Do all three** — this is the phase's central structure |
| **MST** | *Min Cost to Connect All Points* | Bridge suggestions over the contracted component graph. Prim in the standard solution; write Kruskal too, because that is what needs DSU |
| **Dijkstra** | *Network Delay Time* · *Cheapest Flights Within K Stops* | Semantic six-degrees, where explicit links cost 1 and inferred edges cost more. **Your first weighted path problem** |
| Greedy with an approximation argument | *Jump Game II* · *Gas Station* | Set cover is greedy with an ln(n) bound. These build the greedy instinct; the bound comes from the lecture notes |
| Sequence alignment | *Edit Distance* (again) → then read Needleman–Wunsch | Alignment for block correspondence. NW is edit distance with a scoring matrix and affine gaps |

## What the lists do **not** cover

Worth stating plainly, because the absence is easy to miss:

**Fractional indexing · ropes · CRDT convergence · FSTs and automaton intersection · HNSW ·
HyperLogLog / Count-Min / t-digest · PageRank and power iteration · Louvain · Voronoi ·
Betti numbers · LSH banding · epoch-based reclamation · lock-free structures.**

That is roughly **half** the algorithm surface in `ROADMAP.md`. NeetCode and Striver make you
fluent in the interview canon; this project's interesting algorithms are outside it. Do the drills
to remove friction, then read the papers — do not expect the drills to substitute.

## How much time this deserves

| | |
|---|---|
| **Per phase** | 4–8 problems, ~3 hours, in the week before the phase starts |
| **Whole project** | ~35 problems total across six phases |
| **Red flag** | If you have done 300 problems and written no `crates/domain`, the drills have become procrastination. **The list is a warm-up, not the workout** |

---

# 3. Educative — how to use it without losing weeks

Educative's strength is **structured breadth, fast**. Its weakness is that interactive courses
feel like progress and are shallower than the books you already own.

## The rule

> **One hour of Educative to decide whether a topic is what you think it is. Then switch to the
> primary source.** Never cite an Educative course as the reason for a decision — if you cannot
> point at a book chapter, paper, or spec, the decision is not yet grounded.

## Where it is genuinely the right first stop

| Topic | Why Educative first | Then go to |
|---|---|---|
| **Grokking System Design** (and the advanced one) | Fastest ramp on vocabulary before Phase 9/12. Overlaps Hello Interview; pick one | AWS Builders' Library, SRE Book |
| **Rust courses** | Only if you want a second explanation of something *Rust for Rustaceans* left unclear | The book you own; it is better |
| **gRPC / protobuf courses** | Faster than the spec for a first pass on the four RPC modes | [Google AIPs](https://google.aip.dev/) and [protobuf docs](https://protobuf.dev/) — these are the authority |
| **Kubernetes / Docker courses** | Genuinely useful for Phase 11–12 mechanics, which are tedious to learn from reference docs | GCP docs for anything GKE-specific |
| **Distributed systems courses** | Skip. **You own DDIA and have MIT 6.5840 and Kleppmann's Cambridge course.** No Educative course competes with those | — |
| **Design patterns / OOP courses** | Skip. Rust is not the language these are written for, and `PROJECT_STRUCTURE.md` already rejects the layer-first architecture they teach | — |

## The trap, named

Educative, Hello Interview, Striver, and NeetCode are all optimised for **interview outcomes**,
which reward *recognising* a solved problem quickly. This project rewards the opposite: sitting
with an *unsolved* problem — how an anchor survives a deletion, whether two-page atomicity is
possible — long enough to make a defensible call.

Both skills are worth having. **Only one of them ships Marginal.** Budget accordingly: these four
assets together should consume well under 5% of your time on this project.

---

## Where this leaves the schedule

| Activity | Share of time |
|---|---|
| Building | ~70% |
| Reading the mandatory lists in the phase files | ~20% |
| Papers (all ★, whole project ≈ 22 h) | ~5% |
| **Drills, Educative, Hello Interview** | **~5%** |

If the last row grows, the first row is what shrank.