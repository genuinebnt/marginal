# Marginal — Learning Path

**Purpose: you should never need to ask anyone what to build next, or why.** Every phase in
`ROADMAP.md` gets a reading list here — what to read *before* you write code so the decisions
are yours, and what to read *after* it works so you understand what you did.

This is the only document in the project written to be *followed* rather than consulted.

---

## The contract

| | |
|---|---|
| **Mandatory** | Read it before starting. Skipping it means making a decision you cannot defend, and you will have to unmake it later. Every mandatory item names the specific decision it unlocks |
| **Optional** | Read it if the phase is going well and you want depth, or later when the topic comes back. Nothing here is filler — it is *deferrable*, not disposable |
| **Post-build** | Read *after* the code works. Deliberately after: several of these are retrospectives and critiques that only land once you have made the mistake yourself |

**Prereqs are capped on purpose.** No phase asks for more than roughly a day of reading before
you start. A phase whose prerequisites take a week has a scoping problem, not a learning problem.
If a mandatory list here grows past ~6 items, that is a signal the phase should be split.

---

## What you already own

You have a strong shelf. **This curriculum is built around it** — the books and courses below
are the spine, and web links are only used where the shelf has a genuine gap.

| Asset | Where it carries the weight |
|---|---|
| **Rust for Rustaceans** — Gjengset | Ch. *Project Structure* is Phase 0's whole answer. *Designing Interfaces*, *Testing*, *Unsafe*, *Concurrency*, *Async* each own a phase |
| **Rust Atomics and Locks** — Bos ([free online](https://marabos.nl/atomics/)) | Phase 3 and 5. Ch. 3 *Memory Ordering* is the one to read twice |
| **Designing Data-Intensive Applications, 2nd ed.** — Kleppmann & Riddle | The distributed spine: encoding & evolution (1), replication (3), transactions (3), consistency & consensus (3, 10), stream processing (6) |
| **Database Internals** — Petrov | Part I for Phase 1 indexes and Phase 3 WAL; **Part II for failure detection, gossip, anti-entropy and Merkle trees** — Phase 10 nearly chapter for chapter |
| **The Algorithm Design Manual** — Skiena | Graph traversal and weighted graphs (4, 7, 8, 21), DP (6), and the **catalog in Part II** as the lookup table for every algorithm row in the ROADMAP |
| **Crafting Interpreters** — Nystrom ([free online](https://craftinginterpreters.com/)) | Phase 1 input rules and Phase 3's op ISA. Part I is scanning and recursive descent; Part II is a bytecode VM, which is what an op log *is* |
| **Microservice Patterns** — Richardson | Phase 8 sagas, Phase 9 gateway, Phase 12 production-readiness. The saga chapter is the reference, not a blog post |
| **Fly.io Gossip Glomers** ([link](https://fly.io/dist-sys/)) | Phase 3 and 10, as *exercises* rather than reading. Challenge 4 is a CRDT counter; 5a–5c are a replicated log |
| **CMU 15-445 / 15-721** — Pavlo ([15-445](https://15445.courses.cs.cmu.edu/), [15-721](https://15721.courses.cs.cmu.edu/)) | 15-445 for B+tree indexes, WAL and recovery; **15-721 for MVCC**, which is Phase 6's palimpsest |
| **MIT 6.5840** ([schedule](https://pdos.csail.mit.edu/6.824/schedule.html)) | The paper canon. Linearizability, Raft, Spanner, Chain Replication — see [`papers.md`](papers.md) for which phase wants which |
| **KAIST Rust courses** — Jeehoon Kang ([cs431 concurrency](https://github.com/kaist-cp/cs431), [cs420 compilers](https://github.com/kaist-cp/cs420)) | cs431 is the best structured treatment of **crossbeam epoch reclamation** anywhere — Phase 3. cs420 backs the analysis phases |
| **Jon Gjengset — *Crust of Rust*** ([playlist](https://www.youtube.com/playlist?list=PLqbS7AVVErFiWDOAVrPt7aYmnuuOLYvOa)) | Watch the episode matching whatever you are stuck on. `Pin`/`Future`, subtyping/variance, atomics, and channels each map to a phase |
| **BurntSushi** ([blog](https://burntsushi.net/)) | Phase 7 is largely his work — the FST post is the single best resource for the term dictionary |
| **matklad** ([blog](https://matklad.github.io/)) | Phase 0 project structure, Phase 4 incrementality (he built rust-analyzer), and the best writing anywhere on *how to structure a Rust codebase* |
| **Hello Interview** (system design sub) | Phase 9 and 12, and the *presentation* layer. Their collaborative-editor and consistent-hashing breakdowns cover exactly your topology — see [`interview-and-dsa.md`](interview-and-dsa.md) §1 |
| **Striver A2Z / SDE sheet + NeetCode 150** | **Drills, not curriculum.** They sharpen the pattern recognition the phases assume. Mapped problem-by-problem to the phase that needs each pattern in [`interview-and-dsa.md`](interview-and-dsa.md) §2 |
| **Educative.io** (subscription) | Fast ramp on an unfamiliar topic before committing to a book. Best used for *breadth in an hour*, never as the primary source — see [`interview-and-dsa.md`](interview-and-dsa.md) §3 |
| **Rust forum + your saved links** | Where to go when a specific type does not typecheck. Not a curriculum |

> **Where an owned book covers a topic, the web link is marked *optional*.** Buying nothing and
> reading nothing twice is the goal. If you find a web link that duplicates a chapter you own,
> it is a bug in this document — delete it.

---

## How to use this, per phase

```
1. Read the phase's § Before you build — Mandatory list.        (~half a day)
2. Write the failing tests.                                      (agents.md stage 1)
3. Build it.
4. Read § After it works — Mandatory.                            (~2 hours)
5. Only then move on.
```

Step 4 is the one people skip and the one that compounds. The retrospectives and critiques
are placed after the build because **a critique of a design you have not attempted reads as
trivia; the same critique after you have made the mistake reads as a rule.**

---

## Files

| File | Covers |
|---|---|
| [`00-foundations.md`](00-foundations.md) | Cross-phase Rust and tooling. Read alongside everything, not once |
| [`01-track1-mvp.md`](01-track1-mvp.md) | **Phases 0, 1, 2, 3** — foundation, documents, auth, collaboration |
| [`02-track2-differentiators.md`](02-track2-differentiators.md) | **Phases 4, 5, 6** — diagnostics, undo, history |
| [`03-track3-distributed.md`](03-track3-distributed.md) | **Phases 7, 8, 9, 10** — search, saga, gateway, session routing |
| [`04-track4-platform.md`](04-track4-platform.md) | **Phases 13, 14, 15, 16, 20** — RBAC, comments, notifications, editor, settings |
| [`05-track5-reach.md`](05-track5-reach.md) | **Phases 17, 18, 19, 21** — publishing, plugins, assistant, related content |
| [`06-track6-cloud.md`](06-track6-cloud.md) | **Phases 11, 12** — containers, CI, Kubernetes, IaC, observability |
| [`papers.md`](papers.md) | The paper canon, annotated once, referenced from every phase |
| [`people.md`](people.md) | Who to follow per domain, and what each is actually good for |
| [`interview-and-dsa.md`](interview-and-dsa.md) | Hello Interview, Striver/NeetCode drills mapped per phase, and how to use Educative without it eating the schedule |
| [`codebases.md`](codebases.md) | **GitHub repos to read**, by phase and by skill — plus how to read a codebase, and how to build review skill from PR threads |

---

## People, in one line each

Full list with reasoning in [`people.md`](people.md). The short version — these nine are worth
a feed subscription:

| Person | Domain | Why |
|---|---|---|
| [Alice Ryhl](https://ryhl.io/blog/) | async Rust | Tokio maintainer. Actors, blocking, cancellation — the async writing that is actually correct |
| [Ralf Jung](https://www.ralfj.de/blog/) | `unsafe`, UB | Wrote Miri and Stacked Borrows. When your `unsafe` is wrong, he explained why years ago |
| [matklad](https://matklad.github.io/) | project structure, incrementality | rust-analyzer. Read him before deciding how to lay out a crate |
| [BurntSushi](https://burntsushi.net/) | search, automata, perf | `ripgrep`, `regex`, `fst`, `memchr`. Owns Phase 7 |
| [Martin Kleppmann](https://martin.kleppmann.com/) | CRDT, local-first | DDIA and Peritext. Owns Phase 3's model |
| [Kyle Kingsbury](https://aphyr.com/) / [Jepsen](https://jepsen.io/) | consistency, testing | The consistency hierarchy, and proof that your CP claim needs testing |
| [Raph Levien](https://raphlinus.github.io/) | ropes, editors, 2D | Built xi-editor and wrote the retrospective. Owns the rope |
| [Marc Brooker](https://brooker.co.za/blog/) | cloud, distributed practice | AWS. Retries, jitter, timeouts, load shedding — the operational half |
| [Phil Eaton](https://notes.eatonphil.com/) | databases, simulation testing | The best current writing on deterministic simulation testing |

---

## Maintenance rule

**This document tracks `ROADMAP.md` § Execution Order.** Phases are identified by number and
name here, so if a phase is renumbered, split, or cut:

1. Move its section to the right track file — do not leave a stub
2. If a phase is **cut**, delete its section. Resources with no owning phase are how a reading
   list becomes a bookmark folder
3. If a phase is **added**, it needs a section here before it starts. A phase with no reading
   list is a phase where the decisions will be made by whoever is nearest, which defeats the
   entire point of this file

The same rule that keeps the ROADMAP honest applies here: **a resource with no named decision
it unlocks does not belong.** Not "good to know" — *which call does this let me make alone?*

Links verified reachable **8 August 2026**. Rot is inevitable; a dead link is a bug report
against this file.
