# Track 3 — Distributed systems · Phases 7, 8, 9, 10

`7 Search → 8 Saga → 9 Gateway → 10 Session routing`

Where the theory from [§5 of foundations](00-foundations.md#5-distributed-systems-theory--the-ladder)
becomes code. Phase 7 is the DSA-heaviest phase in the project; Phase 10 is the most
distributed-systems-heavy.

---

# Phase 7 — Search & Backlinks · `search-service`

**Largely BurntSushi's work.** Inverted index, BM25, FST term dictionary, Levenshtein automata,
and the graph algorithms on backlinks.

**What you must be able to decide alone at the end:** what a posting list is, why BM25 rather
than TF-IDF, why an FST beats a trie for the term dictionary, and how a DFA and a trie get walked
in lockstep.

## Before you build

### Mandatory, in order

| # | Resource | Type | The decision it unlocks |
|---|---|---|---|
| 1 | [**Introduction to Information Retrieval**](https://nlp.stanford.edu/IR-book/) — Manning, Ch. 1–2, 6 | free book | **The textbook.** Ch. 1 the inverted index, Ch. 2 tokenisation and postings, Ch. 6 term weighting. Free online, and the only IR reading you strictly need |
| 2 | [**BM25 explained**](https://www.elastic.co/blog/practical-bm25-part-1-how-shards-affect-relevance-scoring-in-elasticsearch) — Elastic, parts 1–3 | blog | Why BM25 beats TF-IDF: saturation and length normalisation, with `k1`/`b` explained rather than asserted. Implement before adopting Tantivy |
| 3 | BurntSushi — [**Index 1,600,000,000 Keys with Automata and Rust**](https://burntsushi.net/transducers/) | blog | **The single best resource in this phase.** FSTs from scratch — ordered sets and maps as automata, construction, and why this is what Lucene and Tantivy use for the term dictionary. Long and worth every minute |
| 4 | Paul Masurel — [**Of tantivy, a search engine in Rust**](https://fulmicoton.com/posts/behold-tantivy/) + [**Tantivy ARCHITECTURE.md**](https://github.com/quickwit-oss/tantivy/blob/main/ARCHITECTURE.md) | blog/repo | The library you will adopt, explained by its author. Read after building the naive index so the gap is legible |
| 5 | [**Levenshtein automata can be simple and fast**](https://julesjacobs.com/2015/06/17/disqus-levenshtein-simple-and-fast.html) — Jules Jacobs | blog | The practical construction, in a readable form. Then the intersection with the trie/FST is the actual technique |
| 6 | Schulz & Mihov — [Fast string correction with Levenshtein automata](https://en.wikipedia.org/wiki/Levenshtein_automaton) · or Mike McCandless — [**Lucene's FuzzyQuery is 100 times faster in 4.0**](https://blog.mikemccandless.com/2011/03/lucenes-fuzzyquery-is-100-times-faster.html) | paper/blog | The origin and the production impact. **Read McCandless if you read only one** — it is the story of replacing brute force with an automaton and measuring 100× |
| 7 | [**Roaring bitmaps**](https://roaringbitmap.org/) — the paper linked there | site/paper | Array / bitset / RLE containers chosen by cardinality. Posting-list intersection is where your index spends its time |
| 8 | Skiena — Ch. *Graph Traversal* + Ch. *Weighted Graph Algorithms* | owned | BFS shortest path for link distance, and the groundwork for Phase 21's Dijkstra |

### Optional

| Resource | Type | Why |
|---|---|---|
| [`fst` crate docs](https://docs.rs/fst/) | docs | The implementation behind the blog post. Read the `Automaton` trait — it is how you compose the Levenshtein DFA with the FST |
| Mike McCandless — [Changing Bits](https://blog.mikemccandless.com/) | blog | Lucene internals for a decade. Search it for whatever you are implementing |
| [Tantivy's `SegmentReader`](https://github.com/quickwit-oss/tantivy) source | repo | Segments, merges, and deletes. The part of a search engine that is really a storage engine |
| [Quickwit blog](https://quickwit.io/blog) | blog | Tantivy's commercial descendants. Good writing on scaling an index |
| [Manning IR](https://nlp.stanford.edu/IR-book/) Ch. 3 (tolerant retrieval), Ch. 5 (index compression) | free book | Ch. 5 is where postings compression lives if you want the variable-byte and gamma coding detail |
| [`memchr` + SIMD substring search](https://github.com/BurntSushi/memchr) | repo | Read the algorithm docs. BurntSushi explains Two-Way and packed comparison better than the papers |
| Skiena — Ch. *Data Structures*, trie section | owned | For the `[[link]]` autocomplete |
| [BK-tree explained](https://signal-to-noise.xyz/post/bk-tree/) | blog | Metric trees and triangle-inequality pruning. Alternative to the automaton — worth knowing why you might pick either |

## After it works

### Mandatory

| Resource | Why after |
|---|---|
| Benchmark your naive index against Tantivy with `criterion`, then read [ARCHITECTURE.md](https://github.com/quickwit-oss/tantivy/blob/main/ARCHITECTURE.md) again | The gap is the lesson. The ROADMAP asks for exactly this comparison and it only means something with numbers |
| **DDIA** Ch. *Storage and Retrieval* — the LSM-tree / SSTable sections | Tantivy's segments are an LSM in disguise. This chapter explains the shape you have been using |

### Optional

| Resource | Why |
|---|---|
| [Lucene's `FuzzyQuery` source](https://github.com/apache/lucene) | The production implementation of what you built |
| [Anti-entropy / read repair](https://docs.datastax.com/en/cassandra-oss/3.0/cassandra/operations/opsRepairNodesReadRepair.html) | Your index is reconciled against Postgres. This is the vocabulary |

---

# Phase 8 — Page-Delete Saga

**Small phase, one big idea.** The saga is a well-documented pattern; the work is the state
machine and the compensations.

**What you must be able to decide alone at the end:** orchestration vs choreography, what a
compensating transaction can and cannot undo, and why the blast radius is computed before the
saga starts.

## Before you build

### Mandatory

| Resource | Type | The decision it unlocks |
|---|---|---|
| **Microservice Patterns** — Richardson, Ch. **Managing transactions with sagas** | owned | **The reference for this phase.** Orchestration vs choreography, compensatable/pivot/retriable steps, and countermeasures for lost isolation. Read the whole chapter |
| Garcia-Molina & Salem — [**Sagas**](https://www.cs.cornell.edu/andru/cs711/2002fa/reading/sagas.pdf) | paper (1987) | The original, 8 pages. Worth it for the precise definition of a compensating transaction |
| Skiena — Ch. *Graph Traversal*, topological sort | owned | Saga steps ordered by dependency. Also the reachability closure for the blast radius |
| **DDIA** Ch. *Transactions* — the *weak isolation* sections | owned | A saga has no isolation. This chapter is what you are giving up, named precisely |
| [`ui-mockups/v2/index.html § 08 GRAPH ALGORITHMS`](../../ui-mockups/v2/index.html) — *Blast radius* mode | mockup | Forward reachability, running. Faster than prose |
| [Outbox pattern](https://microservices.io/patterns/data/transactional-outbox.html) — microservices.io | pattern | You already built the outbox in Phase 1. This is the canonical write-up, and the *polling publisher* vs *transaction log tailing* choice |

### Optional

| Resource | Type | Why |
|---|---|---|
| [`FOR UPDATE SKIP LOCKED`](https://www.postgresql.org/docs/current/sql-select.html#SQL-FOR-UPDATE-SHARE) | docs | The outbox poller's concurrency primitive. Read the exact semantics — this is a place people guess |
| [Idempotent consumers](https://microservices.io/patterns/communication-style/idempotent-consumer.html) | pattern | At-least-once plus idempotence. You dedupe on `OpId` |
| [Dead letter queues](https://docs.nats.io/nats-concepts/jetstream/consumers) — NATS consumer docs | docs | Max delivery and DLQ in JetStream specifically |
| [Temporal / durable execution](https://temporal.io/blog/what-is-durable-execution) | blog | The industry's answer to hand-written sagas. Know what you chose not to use |

## After it works

| Resource | Why after |
|---|---|
| **Microservice Patterns** Ch. *Managing transactions with sagas* — the *countermeasures* section, reread | Semantic lock, commutative updates, pessimistic view, reread value, version file, by-value. You will have needed at least one |
| Chaos-test it: kill the orchestrator mid-saga | Not a resource. `lifecycle_state` exists so a crash resumes rather than restarts — prove it |

---

# Phase 9 — API Gateway · `api-gateway`

**The only public component.** The reading is about edge concerns: fan-out latency, shedding,
rate limits, and HTTP/2 realities.

**What you must be able to decide alone at the end:** why p99 at the edge is the slowest of N,
the difference between rate limiting and load shedding, and what HTTP/2 multiplexing does not fix.

## Before you build

### Mandatory

| Resource | Type | The decision it unlocks |
|---|---|---|
| [**AWS Builders' Library — Timeouts, retries and backoff with jitter**](https://aws.amazon.com/builders-library/timeouts-retries-and-backoff-with-jitter/) | article | Twenty minutes and it permanently changes how you write a retry. Jitter is not optional |
| [**AWS Builders' Library — Using load shedding to avoid overload**](https://aws.amazon.com/builders-library/using-load-shedding-to-avoid-overload/) | article | **Load shedding ≠ rate limiting.** Rate limiting is per-client and steady-state; shedding is systemic and under duress, must be cheap, and must protect in-flight work |
| Dean & Barroso — [**The Tail at Scale**](https://research.google/pubs/the-tail-at-scale/) | paper | **Why your gateway's p99 is the slowest of N services, not the average.** Hedged requests, tied requests, micro-partitions. Eight pages, and it is the single most cited paper in edge design |
| **Microservice Patterns** — Richardson, Ch. **External API patterns** | owned | API gateway vs BFF, composition, and the protocol-translation concerns you are about to hit |
| [HTTP/2 — head-of-line blocking explained](https://blog.cloudflare.com/http-3-vs-http-2/) — Cloudflare | blog | Many gRPC streams over one connection are multiplexed at HTTP/2 and still serialised at TCP. One large replay delays small calls sharing it (ADR-007) |
| [`tower` docs](https://docs.rs/tower/) — `Service`, `Layer`, and the [tower guides](https://github.com/tower-rs/tower/tree/master/guides) | docs | The middleware model your gateway is made of. Read `Service` carefully — it is the abstraction, not a helper |
| [Token bucket vs sliding window](https://blog.cloudflare.com/counting-things-a-lot-of-different-things/) — Cloudflare | blog | The two rate-limit algorithms with their trade-offs, from people who do it at scale |

### Optional

| Resource | Type | Why |
|---|---|---|
| [`tower-governor`](https://docs.rs/tower_governor/) / [`governor`](https://docs.rs/governor/) | docs | GCRA rate limiting in Rust. Read `governor`'s docs on GCRA — it is more elegant than token bucket |
| [Envoy's architecture overview](https://www.envoyproxy.io/docs/envoy/latest/intro/arch_overview/arch_overview) | docs | The industrial gateway. Read the circuit-breaking and outlier-detection sections for vocabulary |
| [gRPC-Web / grpc-gateway](https://github.com/grpc-ecosystem/grpc-gateway) | repo | The alternative to hand-writing REST↔gRPC translation. You are hand-writing it; know the option |
| [Release It!](https://pragprog.com/titles/mnee2/release-it-second-edition/) — Nygard, stability patterns chapters | book | Circuit breaker, bulkhead, timeout, steady state. The canonical catalogue |
| [Marc Brooker on timeouts](https://brooker.co.za/blog/2022/02/28/retries.html) | blog | Shorter and sharper than the Builders' Library piece |

## After it works

| Resource | Why after |
|---|---|
| Reread [The Tail at Scale](https://research.google/pubs/the-tail-at-scale/) with your own latency numbers | The paper is abstract until you have a p99 you are unhappy with |
| Brooker — [Give Your Tail a Nudge](https://brooker.co.za/blog/2022/10/21/nudge.html) + [Tail Latency Might Matter More Than You Think](https://brooker.co.za/blog/2021/04/19/latency.html) | Hedging and tail latency with the maths. Implement only after measuring — hedging a slow dependency can make things worse |

---

# Phase 10 — Session Routing

**The most distributed-systems-dense phase.** Consistent hashing, leases, fencing, failure
detection, SWIM. **Database Internals Part II is nearly chapter-for-chapter this phase.**

**What you must be able to decide alone at the end:** why a lease and not a record, why a fencing
token and not a heartbeat, why a Redis TTL is not a lock, and how a wrongly-suspected node
refutes its own death.

## Before you build

### Mandatory, in order

| # | Resource | Type | The decision it unlocks |
|---|---|---|---|
| 1 | **DDIA** Ch. *Sharding* (or *Partitioning*) | owned | Hash vs range partitioning, rebalancing, request routing. The chapter your whole phase is an instance of |
| 2 | Karger et al. — [**Consistent Hashing and Random Trees**](https://www.cs.princeton.edu/courses/archive/fall09/cos518/papers/chash.pdf) | paper (1997) | The original. Then: [Lamping & Veach — **Jump Consistent Hash**](https://arxiv.org/pdf/1406.2294.pdf), 6 pages, O(1) space. **Implement both and benchmark** — the ROADMAP asks for exactly this |
| 3 | **Database Internals** Part II Ch. **Failure Detection** | owned | Heartbeats, timeouts, and **phi-accrual** — an adaptive detector that outputs a suspicion level rather than a boolean. This chapter is the phase's reference |
| 4 | Hayashibara et al. — [**The φ Accrual Failure Detector**](https://ieeexplore.ieee.org/document/1028914) · or the [Akka implementation notes](https://doc.akka.io/docs/akka/current/typed/failure-detector.html) | paper/docs | The maths behind the suspicion level. **Read the Akka docs if the paper is hard to find** — they explain the same thing operationally |
| 5 | Kleppmann — [**How to do distributed locking**](https://martin.kleppmann.com/2016/02/08/how-to-do-distributed-locking.html) | blog | **Mandatory, non-negotiable.** Why a Redis TTL is not a lock, why Redlock is not safe, and why a **fencing token** is what makes any of it correct. This is the argument behind `ROADMAP.md` § *Ownership must be a lease, not a record* |
| 6 | **DDIA** Ch. *Consistency and Consensus* — the *leader and lock* + *fencing token* sections | owned | The same argument in book form, with the pause-and-resume failure drawn |
| 7 | Das, Gupta, Motivala — [**SWIM**](https://research.cs.cornell.edu/projects/Quicksilver/public_pdfs/SWIM.pdf) | paper | Scalable weakly-consistent membership. Random probing, **indirect probes through k peers**, suspicion with refutation, and **incarnation numbers** — how a wrongly-suspected node refutes its death. The prettiest idea on your roadmap |
| 8 | HashiCorp — [**Lifeguard: SWIM-ing with Situational Awareness**](https://arxiv.org/pdf/1707.00788) + [the blog post](https://www.hashicorp.com/blog/making-gossip-more-robust-with-lifeguard) | paper/blog | SWIM's false-positive problem and the three extensions that fix it. Read after SWIM |
| 9 | **Database Internals** Part II Ch. **Anti-Entropy and Dissemination** | owned | Gossip dissemination and **Merkle-tree anti-entropy** — the reconciliation you need after a partition or lease loss |
| 10 | [**Gossip Glomers**](https://fly.io/dist-sys/) challenges 5a–5c | owned exercises | A replicated log, built. The closest thing to a rehearsal for this phase |

### Optional

| Resource | Type | Why |
|---|---|---|
| [`memberlist`](https://github.com/hashicorp/memberlist) source | repo | The production SWIM implementation. Go, readable, and the source of Lifeguard |
| Quickwit — [**Chitchat**: decentralized cluster membership in Rust](https://quickwit.io/blog/chitchat) | blog | SWIM-adjacent gossip in Rust, with the design decisions explained. The closest reference implementation to what you would write |
| [`chitchat` crate](https://github.com/quickwit-oss/chitchat) | repo | The code behind that post |
| [Consistent hashing with bounded loads](https://arxiv.org/abs/1608.01350) | paper | The refinement that stops one node getting hot. Read if hashing alone is unbalanced |
| [Rendezvous / HRW hashing](https://en.wikipedia.org/wiki/Rendezvous_hashing) | wiki | The third option. Simpler than a ring, often better |
| [`turmoil`](https://github.com/tokio-rs/turmoil) — partition tests | repo | Phase 3 mandatory; here it is what makes the split-brain test possible at all |
| Raft — [extended paper](https://raft.github.io/raft.pdf) | paper | You are **not** implementing consensus (single owner per page means no write quorum). Read §1–5 anyway, once, so you can say precisely why you did not need it |

## After it works

### Mandatory

| Resource | Why after |
|---|---|
| Fault-inject with `turmoil`: partition an instance from Redis but **not** from clients | The exact shape that produces split-brain. If your fencing tokens are right, this test passes; if they are decorative, it fails |
| [Jepsen analyses](https://jepsen.io/analyses) — pick two systems you use | Reading how real systems violated their own claims, after making a claim of your own |

### Optional

| Resource | Why |
|---|---|
| Kleppmann — [lecture notes](https://www.cl.cam.ac.uk/teaching/2223/ConcDisSys/dist-sys-notes.pdf) §8 (consensus) | Now that you know why you avoided it |
| [Murat Demirbas on failure detectors](http://muratbuffalo.blogspot.com/) | Search his blog. He reviews this literature better than anyone |
| [MIT 6.5840](https://pdos.csail.mit.edu/6.824/schedule.html) — Chain Replication, Spanner | See [`papers.md`](papers.md). Read after this phase, not during |
