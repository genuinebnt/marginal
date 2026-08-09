# People to follow

Not a popularity list. **Each entry answers "when I am stuck on X, whose archive do I search?"**
If a name here has no phase attached, it does not belong.

Subscribe by RSS where possible — most of these post rarely and irregularly, which is exactly why
a feed reader beats remembering to check.

---

## Rust — language, `unsafe`, async

| Person | Where | Search their archive when | Phase |
|---|---|---|---|
| **Alice Ryhl** | [ryhl.io/blog](https://ryhl.io/blog/) · Tokio maintainer | Anything async: actors, blocking, cancellation, `spawn_blocking`, channel choice. **The async writing that is actually correct** | 3, 4, 15 |
| **Mara Bos** | [marabos.nl](https://marabos.nl/) · [Atomics and Locks](https://marabos.nl/atomics/) · Rust library team lead | Atomics, memory ordering, locks, and what the standard library guarantees | 3, 5 |
| **Ralf Jung** | [ralfj.de/blog](https://www.ralfj.de/blog/) · wrote Miri, Stacked/Tree Borrows | Your `unsafe` is rejected by Miri and you do not know why. Also: what UB *is* in Rust, formally | 3 |
| **Jon Gjengset** | [thesquareplanet.com](https://thesquareplanet.com/) · [Crust of Rust](https://www.youtube.com/playlist?list=PLqbS7AVVErFiWDOAVrPt7aYmnuuOLYvOa) · [Rust for Rustaceans](https://rust-for-rustaceans.com/) | `Pin`, variance, atomics, channels, iterators, lifetimes. **Watch the episode matching your confusion** — the long unedited format is the value | all |
| **Niko Matsakis** | [smallcultfollowing.com/babysteps](https://smallcultfollowing.com/babysteps/) · Rust lang team | *Why* a language feature is the shape it is. Also salsa, which he wrote | 0, 4 |
| **matklad** (Alex Kladov) | [matklad.github.io](https://matklad.github.io/) · built rust-analyzer | **Project structure, incrementality, parsing for editors, testing philosophy.** The highest hit rate on this list for Marginal specifically | 0, 1, 4, 16 |
| **BurntSushi** (Andrew Gallant) | [burntsushi.net](https://burntsushi.net/) · `ripgrep`, `regex`, `fst`, `memchr`, `bstr`, [`rebar`](https://github.com/BurntSushi/rebar) | Automata, search, byte-oriented text, and **how to benchmark honestly**. **Phase 7 is largely his output**, and [`codebases.md` §3](codebases.md) has a reading order for his repos specifically — start at `memchr`, never at `regex` | 1, 7 |
| **withoutboats** | [without.boats](https://without.boats/) | The design history of async Rust. Read when you want to know why `Pin` exists rather than how to use it | 3 |
| **Amos** (fasterthanlime) | [fasterthanli.me](https://fasterthanli.me/) | Long, patient explanations of how a thing actually works end to end. Best when a whole subsystem is opaque | all |
| **Nicholas Nethercote** | [nnethercote.github.io/perf-book](https://nnethercote.github.io/perf-book/) · [blog](https://nnethercote.github.io/) | Performance, profiling, and compile times. Read the book's *Profiling* chapter before your first optimisation | 1, 3, 7 |
| **Manish Goregaokar** | [manishearth.github.io](https://manishearth.github.io/) | Arenas, Unicode, and the parts of Rust that touch i18n. His arena post is Phase 4 mandatory | 4, 16 |
| **Predrag Gruevski** | [predr.ag](https://predr.ag/blog/) · `cargo-semver-checks` | Semver, API evolution, and how to break things on purpose | 11 |
| **David Tolnay** | [github.com/dtolnay](https://github.com/dtolnay) · `serde`, `thiserror`, `anyhow`, `syn` | **API design as a craft.** Rarely blogs — read the code and [case-studies](https://github.com/dtolnay/case-studies) instead | 0, 1 |
| **Amanieu d'Antras** | [github.com/Amanieu](https://github.com/Amanieu) · `hashbrown`, `parking_lot` | Wrote the hash table that became `std`'s. Data structures taken to the limit | 3 |
| **Carl Lerche** | [github.com/carllerche](https://github.com/carllerche) · `tokio`, `bytes`, `tower` | Abstraction design for async. `bytes` is a ROADMAP row you will implement against | 3, 9 |
| **Armin Ronacher** | [lucumr.pocoo.org](https://lucumr.pocoo.org/) · `insta`, `similar` | Systems and API design, plus **snapshot testing you should probably adopt** | 1, 6 |

**Aggregators worth one subscription:** [This Week in Rust](https://this-week-in-rust.org/) ·
[Rust Blog](https://blog.rust-lang.org/) · [Inside Rust](https://blog.rust-lang.org/inside-rust/) ·
[/r/rust](https://reddit.com/r/rust) for release notes, [Rust Users Forum](https://users.rust-lang.org/)
for "why does this not compile".

---

## Editors, ropes, document models

| Person / org | Where | Search when | Phase |
|---|---|---|---|
| **Raph Levien** | [raphlinus.github.io](https://raphlinus.github.io/) · built xi-editor, Druid, Vello | Ropes, text layout, editor architecture, 2D rendering. **His xi retrospective is Phase 3's most valuable post-build read** | 3, 16 |
| **Ink & Switch** | [inkandswitch.com](https://www.inkandswitch.com/) | Local-first, Peritext, editorial workflows over CRDTs. **The research lab whose output this project is closest to** | 3, 14 |
| **Martin Kleppmann** | [martin.kleppmann.com](https://martin.kleppmann.com/) · DDIA, Peritext, [Cambridge course](https://www.cl.cam.ac.uk/teaching/2223/ConcDisSys/) | CRDTs, consistency, distributed systems teaching. **The single most load-bearing author for this project** | 3, 6, 14 |
| **Joseph Gentle** | [josephg.com/blog](https://josephg.com/blog/) · diamond-types | Sequence CRDT *performance*, with measurements. The counterweight to CRDT papers that ignore constants | 3 |
| **Zed team** | [zed.dev/blog](https://zed.dev/blog) | Rope/SumTree, GPU text rendering, and a production Rust editor's engineering decisions | 3, 16 |
| **Marijn Haverbeke** | [marijnhaverbeke.nl](https://marijnhaverbeke.nl/blog/) · ProseMirror, CodeMirror | Block-editor architecture, transforms, and the problems `contenteditable` creates. **Read even though you are not using ProseMirror** | 16 |

---

## Distributed systems

| Person / org | Where | Search when | Phase |
|---|---|---|---|
| **Kyle Kingsbury** (aphyr) | [aphyr.com](https://aphyr.com/) · [jepsen.io](https://jepsen.io/) | Consistency models, and how systems violate their own claims. **[jepsen.io/consistency](https://jepsen.io/consistency) is a permanent bookmark** | 3, 10 |
| **Marc Brooker** | [brooker.co.za/blog](https://brooker.co.za/blog/) · AWS | **The operational half nobody teaches.** Timeouts, retries, jitter, hedging, load shedding, queueing theory | 9, 10, 12 |
| **Murat Demirbas** | [muratbuffalo.blogspot.com](http://muratbuffalo.blogspot.com/) | Triaging a paper before committing to it. He reviews everything and says whether it is worth your time | 3, 10 |
| **Phil Eaton** | [notes.eatonphil.com](https://notes.eatonphil.com/) | Deterministic simulation testing, database internals, and building things from scratch to learn | 3, 6 |
| **Tyler Neely** | [sled.rs](https://sled.rs/) · [blog](https://tylerneely.com/) | Lock-free Rust, simulation testing, and strong opinions about correctness that are usually right | 3, 5 |
| **TigerBeetle** | [tigerbeetle.com/blog](https://tigerbeetle.com/blog) · [VSR docs](https://github.com/tigerbeetle/tigerbeetle/blob/main/docs/internals/vsr.md) | A system designed around deterministic simulation from day one. Their engineering writing is unusually good | 3, 10 |
| **Leslie Lamport** | [lamport.azurewebsites.net](https://lamport.azurewebsites.net/) | The primary sources. Time/clocks, distributed snapshots, Paxos. Everything else is a footnote | 3 |
| **Andy Pavlo** | [CMU 15-445](https://15445.courses.cs.cmu.edu/) / [15-721](https://15721.courses.cs.cmu.edu/) · [ottertune blog](https://ottertune.com/blog/) | Storage engines, indexes, **MVCC**, query execution. The 15-721 MVCC lectures are Phase 6's core | 1, 6 |
| **Alex Petrov** | [Database Internals](https://www.databass.dev/) · [blog](https://www.databass.dev/) | You own the book. Part II is Phase 10 nearly chapter for chapter | 1, 3, 10 |
| **Dan Luu** | [danluu.com](https://danluu.com/) | Measurement, why systems fail in unpredicted ways, and healthy scepticism about engineering folklore | 12 |

---

## Algorithms & data structures

| Person / org | Where | Search when | Phase |
|---|---|---|---|
| **cp-algorithms** | [cp-algorithms.com](https://cp-algorithms.com/) | **The reference implementation of any classical algorithm.** DSU, Dijkstra, MST, topological sort, dynamic connectivity. Terse and correct | 4, 7, 8, 21 |
| **Steven Skiena** | [algorist.com](https://www.algorist.com/) · you own the book · [lectures](https://www3.cs.stonybrook.edu/~skiena/373/videos/) | **Part II's catalog is a lookup table** — "I have this problem, what is it called and is it solved?" | 4, 7, 21 |
| **Stanford MMDS** | [mmds.org](http://www.mmds.org/) — Leskovec, Rajaraman, Ullman | LSH, link analysis, social graphs. **Ch. 3 on LSH is the best treatment anywhere**, free | 21 |
| **Daniel Lemire** | [lemire.me/blog](https://lemire.me/blog/) | Roaring bitmaps, fast parsing, SIMD, and integer compression. Measures everything | 7 |
| **Mike McCandless** | [blog.mikemccandless.com](https://blog.mikemccandless.com/) | Lucene internals for over a decade. Search it for anything index-related | 7 |
| **Paul Masurel** | [fulmicoton.com](https://fulmicoton.com/) · wrote Tantivy · [Quickwit blog](https://quickwit.io/blog) | The search engine you will adopt, explained by its author | 7 |
| **Erik Demaine** | [MIT 6.851 Advanced Data Structures](https://courses.csail.mit.edu/6.851/) | Persistent data structures, link-cut trees, and the theory behind Phase 6 and 21. Free lectures | 6, 21 |

---

## Cloud, infrastructure, operations

| Person / org | Where | Search when | Phase |
|---|---|---|---|
| **Google SRE** | [sre.google/books](https://sre.google/books/) | SLOs, alerting, postmortems, release engineering. **Free, and the SLO chapter is mandatory for Phase 12** | 12 |
| **AWS Builders' Library** | [aws.amazon.com/builders-library](https://aws.amazon.com/builders-library/) | Timeouts, retries, load shedding, health checks. **Vendor-agnostic despite the name**, and better than most books | 9, 12 |
| **Gil Tene** | [talks](https://www.youtube.com/watch?v=lJ8ydIuPFeU) | Latency measurement. *How NOT to Measure Latency* is the one talk to watch before benchmarking anything | 3, 12 |
| **Brendan Gregg** | [brendangregg.com](https://www.brendangregg.com/) | Flame graphs, systems performance methodology, and how to read a profile | 3, 12 |
| **Google Cloud Blog — Infrastructure** | [cloud.google.com/blog](https://cloud.google.com/blog/products/infrastructure) | GKE and Cloud Run changes that affect you. Low signal-to-noise; skim release notes instead | 11, 12 |
| **Kelsey Hightower** | [github.com/kelseyhightower](https://github.com/kelseyhightower) | Kubernetes fundamentals without the marketing. *Kubernetes the Hard Way* if you want the control plane | 12 |

---

## Security

| Person / org | Where | Search when | Phase |
|---|---|---|---|
| **OWASP Cheat Sheet Series** | [cheatsheetseries.owasp.org](https://cheatsheetseries.owasp.org/) | **Read rather than decide.** Password storage, auth, session management, authorization. Every item has a known right answer | 2, 13 |
| **Bytecode Alliance** | [bytecodealliance.org](https://bytecodealliance.org/articles) · [Wasmtime docs](https://docs.wasmtime.dev/) | WASM sandboxing, the component model, and the security boundary you will depend on | 18 |
| **Filippo Valsorda** | [words.filippo.io](https://words.filippo.io/) | Cryptography engineering, and why you should not build any. Read before touching a primitive | 2 |
| **Thomas Ptacek / latacora** | [latacora.micro.blog](https://www.latacora.com/blog/) | "Cryptographic Right Answers" — the shortest path to not getting crypto wrong | 2 |

---

## The one-line version

If you subscribe to **five** things and no more:

1. [matklad](https://matklad.github.io/) — highest hit rate for this specific project
2. [Alice Ryhl](https://ryhl.io/blog/) — async Rust, correct
3. [Marc Brooker](https://brooker.co.za/blog/) — the operational half
4. [Ink & Switch](https://www.inkandswitch.com/) — the research closest to what you are building
5. [This Week in Rust](https://this-week-in-rust.org/) — everything else, filtered

**Several of the best Rust authors barely blog** — dtolnay, Amanieu, and Carl Lerche communicate
almost entirely through code and API design. For them, [`codebases.md` §3](codebases.md) is the
subscription: read the crate, not a feed.

---

## Anti-recommendation

Skip: Medium listicles, "Rust vs Go" anything, tutorials that build a toy without naming what
they simplified, YouTube channels that read documentation aloud, and any blog post whose code
does not compile. **This project's failure mode is breadth, not depth** — every feed you add is
an hour you did not spend on Phase 3.