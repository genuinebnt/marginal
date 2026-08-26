# Track 1 — MVP · Phases 0, 1, 2, 3

`1 Documents → 2 Auth → 3 Collaboration` · 🏁 *log in, write a page, edit live with someone*

**This track carries most of the Rust depth in the whole project.** Phase 3 alone reaches
thirteen of the rare concepts in `ROADMAP.md` § Rust, DSA & Concepts Map. Budget accordingly:
Phases 0–2 are weeks, Phase 3 is months.

Cross-references into [`00-foundations.md`](00-foundations.md) are not repeated here.

---

# Phase 0 — Foundation *(pulled, not pushed)*

**Not a step.** Foundation work is pulled in by the first service that needs it. The reading is
correspondingly small — you are deciding *layout*, not building infrastructure.

**What you must be able to decide alone at the end:** where a new type goes, when a `crates/` crate
is justified, and what a feature flag may and may not do.

## Before you build

### Mandatory

| Resource | Type | The decision it unlocks |
|---|---|---|
| matklad — [**Large Rust Workspaces**](https://matklad.github.io/2021/08/22/large-rust-workspaces.html) | blog | Flat workspace with many small crates, or few large ones. Twelve minutes and it decides your `Cargo.toml` topology for the whole project. Read against `PROJECT_STRUCTURE.md` §5 |
| *Rust for Rustaceans* Ch. **Project Structure** | owned | Features, workspaces, the `dep:` syntax, and why **Cargo features must be additive**. The feature-flag rules in `ROADMAP.md` come straight from here |
| [Cargo Book — Features](https://doc.rust-lang.org/cargo/reference/features.html) + [Resolver](https://doc.rust-lang.org/cargo/reference/resolver.html) | docs | You set `resolver = "3"`. This is what it bought — feature unification rules and MSRV-aware selection. Know it before a dependency conflict teaches you |
| *Rust for Rustaceans* Ch. **Macros** | owned | Whether `define_id!` is `macro_rules!` or a proc macro. Read the declarative half now; the `syn`/`quote` half when you actually need a derive |
| [Rust API Guidelines](https://rust-lang.github.io/api-guidelines/) — the [checklist](https://rust-lang.github.io/api-guidelines/checklist.html) | free | Naming, common traits, and the interoperability items. Use as a literal checklist on `crates/domain` |

### Optional

| Resource | Type | Why |
|---|---|---|
| matklad — [Rust in 100 kLOC](https://matklad.github.io/2021/09/05/Rust100k.html) | blog | What a large Rust codebase actually feels like from the inside. Useful calibration on compile times and crate boundaries |
| [Rust Design Patterns](https://rust-unofficial.github.io/patterns/) — *Newtype*, *RAII guards* | free book | The two patterns Phase 1 uses most |
| [`config` crate docs](https://docs.rs/config/) | docs | Already wired up. Read only if you want to change the layering |

## After it works

| Resource | Why after |
|---|---|
| matklad — [Basic Things](https://matklad.github.io/2024/03/22/basic-things.html) | A checklist of project hygiene most projects skip. Reads as obvious *after* you have felt the absence of two of them |

---

# Phase 1 — Documents · `document-service`

**Build against:** [`lld/document-service.md`](../architecture/lld/document-service.md) for the
service, and **[`lld/document-core.md`](../architecture/lld/document-core.md) for the editor core** — Phase 1
is both, and the parser lives entirely in the second.

**Where you are now.** Nothing is written — not the startup path, not the tests. The LLD specifies the design; the layout is yours.

**What you must be able to decide alone at the end:** how a tree is stored in a relational
database, what a newtype owes its caller, how ordering survives concurrent reorders, and where
the transaction boundary sits.

## Before you build

### Mandatory

| Resource | Type | The decision it unlocks |
|---|---|---|
| [§2 Data modelling](00-foundations.md#2-data-modelling--how-to-arrive-at-a-schema) — the whole mandatory list | mixed | The schema. Do not skip; this is the phase it exists for |
| *Rust for Rustaceans* Ch. **Designing Interfaces** | owned | `PageId`, `SortKey`, `MaterialisedPath`. What `Borrow` vs `AsRef` vs `Deref` mean for a newtype, and why `Borrow` is the strict one |
| *Rust for Rustaceans* Ch. **Error Handling** | owned | The `AppError` → `ApiError` split, and why a database message must never reach a client |
| [**Implementing Fractional Indexing**](https://observablehq.com/@dgreensp/implementing-fractional-indexing) — Greenspan | notebook | `key_between`. Executable, so you can poke the midpoint algorithm before writing it. **The single most important link in this phase** |
| [Figma — Realtime Editing of Ordered Sequences](https://www.figma.com/blog/realtime-editing-of-ordered-sequences/) | blog | *Why* fractional indexing rather than integer positions: reorder writes one row. Ten minutes |
| [LTREE docs](https://www.postgresql.org/docs/current/ltree.html) | docs | `<@` subtree queries, and the **label character restrictions** that force the `p018f2b…` prefix (`lld/document-service.md` §12) |
| *Rust for Rustaceans* Ch. **Testing** | owned | Before you write the repo tests. Integration vs unit, and why mocking Postgres tests your beliefs about Postgres |
| [`#[sqlx::test]`](https://docs.rs/sqlx/latest/sqlx/attr.test.html) | docs | An isolated database per test, dropped after. This is how `CLOUD_PORTABILITY.md` §4's "never mock infrastructure" is actually achieved |
| [Postgres collation](https://www.postgresql.org/docs/current/collation.html) — `COLLATE "C"` | docs | **The trap.** Your Rust `Ord` on `SortKey` must match Postgres byte order, and the default collation is not byte order. `lld/document-service.md` §12 |
| **DDIA** Ch. *Transactions* — the isolation-levels section | owned | The outbox write shares a transaction with the op. You need to know what "shares a transaction" guarantees |

### Optional

| Resource | Type | Why |
|---|---|---|
| **Crafting Interpreters** Ch. **Scanning** | owned | Input rules are a lexer. Mandatory by day 9 of §1's plan; optional if you defer input rules to Phase 16 |
| [CommonMark spec](https://spec.commonmark.org/) §4 | spec | The reference definition of the markdown you are approximating |
| [SQL Antipatterns](https://pragprog.com/titles/bksqla/sql-antipatterns/) Ch. *Naive Trees* | book | Four tree-storage strategies compared. Read to defend LTREE against alternatives |
| [`memchr`](https://docs.rs/memchr/) docs + [BurntSushi on SIMD substring search](https://github.com/BurntSushi/memchr) | docs/repo | Delimiter scanning in the input rules. The one place SIMD is free |
| [**`bat`**](https://github.com/sharkdp/bat) — `src/assets.rs` | repo | **Only when you reach code-block highlighting.** `syntect` + `two-face` with a grammar allowlist and lazy loading — RFC-001 §7's requirement, already solved. Read it rather than rediscovering the bundle-size problem |
| [Postgres `EXPLAIN`](https://www.postgresql.org/docs/current/using-explain.html) | docs | For the first slow query, not before |
| [`utoipa`](https://docs.rs/utoipa/) | docs | The annotations are mandatory per `docs/api/README.md`; the crate docs are optional until one confuses you |

## After it works

### Mandatory

| Resource | Why after |
|---|---|
| [`cargo-mutants`](https://mutants.rs/) — run it | You wrote the tests before the code. **This answers whether they were any good** — surviving mutants are tests you thought you had |
| [The Rust Performance Book](https://nnethercote.github.io/perf-book/) — *Benchmarking* + *Profiling* | Before you optimise anything. Also: measure `dyn PageRepo` vs monomorphised, which is a `ROADMAP.md` § language-surface row |
| **The Art of PostgreSQL** — Ch. on data modelling | Now it reads as a review of your schema rather than a list of opinions |

### Optional

| Resource | Why |
|---|---|
| [CMU 15-445](https://15445.courses.cs.cmu.edu/) — index lectures | The inside of the B-tree you have been using |
| [`cargo-fuzz` book](https://rust-fuzz.github.io/book/) | The paste sanitiser is attacker-facing. Fuzz it once and you will keep fuzzing |

---

# Phase 2 — Auth · `auth-service`

Small phase, high stakes. **The reading is almost entirely "what not to invent."**

**Build against:** [`lld/auth-service.md`](../architecture/lld/auth-service.md) — module map, rotation algorithm, error mapping, and eight traps.

**What you must be able to decide alone at the end:** how to store a password, why refresh
rotation detects theft, and where a token is verified.

## Before you build

### Mandatory

| Resource | Type | The decision it unlocks |
|---|---|---|
| [**OWASP Password Storage Cheat Sheet**](https://cheatsheetseries.owasp.org/cheatsheets/Password_Storage_Cheat_Sheet.html) | reference | Argon2id, and the parameters. **Read this rather than deciding.** The PHC string format is why parameters upgrade without a migration (`DATA_MODEL.md` §3) |
| [**OWASP Authentication Cheat Sheet**](https://cheatsheetseries.owasp.org/cheatsheets/Authentication_Cheat_Sheet.html) | reference | Rate limiting, generic error messages, timing-safe comparison. Every item is a decision with a known right answer |
| [**OWASP Session Management Cheat Sheet**](https://cheatsheetseries.owasp.org/cheatsheets/Session_Management_Cheat_Sheet.html) | reference | Token lifetime, storage, and rotation. Directly shapes `auth.refresh_tokens` |
| [RFC 9106 — Argon2](https://www.rfc-editor.org/rfc/rfc9106.html) §4 *Parameter Choice* | RFC | The actual parameter guidance. Two pages, and it is the citation you want when someone asks why `m`, `t`, `p` are what they are |
| [RFC 7519 — JWT](https://www.rfc-editor.org/rfc/rfc7519) §4 registered claims, §11 security considerations | RFC | What a JWT is and is not. §11 is the part people skip. RS256 + local verification at the gateway depends on understanding this |
| [`argon2` crate docs](https://docs.rs/argon2/latest/argon2/) | docs | The PHC encoding in practice |
| **Refresh token rotation** — [OAuth 2.0 Security BCP](https://datatracker.ietf.org/doc/html/draft-ietf-oauth-security-topics) § refresh tokens | draft RFC | Why reuse of a revoked token means theft, and why you revoke the **whole chain**. This is the `parent_id` column in `auth.refresh_tokens` |

### Optional

| Resource | Type | Why |
|---|---|---|
| [RFC 6749 — OAuth 2.0](https://datatracker.ietf.org/doc/html/rfc6749) | RFC | You are not building OAuth. Read §1 and §4.1 for vocabulary only, so you can say precisely what you did *not* build |
| [`jsonwebtoken` crate](https://docs.rs/jsonwebtoken/) | docs | The verification path. Read the `Validation` struct carefully — its defaults matter |
| [Timing attacks — `subtle` crate](https://docs.rs/subtle/) | docs | Constant-time comparison as a type-level discipline rather than a habit |

## After it works

| Resource | Why after |
|---|---|
| Run `/project:security-review` | It is in the skill workflow for exactly this phase |
| [`cargo-audit`](https://github.com/rustsec/rustsec/tree/main/cargo-audit) | Auth pulls crypto dependencies. Advisory checking earns its place here first |
| [Troy Hunt — Pwned Passwords k-anonymity](https://www.troyhunt.com/ive-just-launched-pwned-passwords-version-2/) | If you ever add a breached-password check, this is the design that does not leak the password |

---

# Phase 3 — Collaboration · `collaboration-service`

**The hardest and richest phase in the project.** Rope, CRDT, WAL, lock-free, `unsafe`, hand-written
`Future`, vector clocks, zero-copy. If you only ever finish Phases 1 and 3, you have an unusual
résumé.

**Build against:** [`lld/collaboration-service.md`](../architecture/lld/collaboration-service.md) — the densest §9 in the project (21 algorithms with invariants) and ten traps in §12.

**Read the prerequisites in the order given.** They build, and the order matters more here than
anywhere else.

**What you must be able to decide alone at the end:** which sequence CRDT, what an anchor is, why
your `Ordering` choices are correct, when the WAL is durable, and what happens to an op when a
`select!` branch loses.

## Before you build — part A: the model

### Mandatory, in order

| # | Resource | Type | The decision it unlocks |
|---|---|---|---|
| 1 | [§5 Distributed systems ladder](00-foundations.md#5-distributed-systems-theory--the-ladder) rungs 1–4 | mixed | Do this first. Everything below assumes causality and logical clocks |
| 2 | [**Local-First Software**](https://www.inkandswitch.com/local-first/) — Ink & Switch | essay | The philosophy Marginal implements. Seven ideals; you satisfy some and not others, and you should know which |
| 3 | Kleppmann — [**CRDTs and the Quest for Distributed Consistency**](https://www.youtube.com/watch?v=B5NULPSiOGw) | talk (45 min) | The clearest explanation of *why* CRDTs converge. Watch before reading any CRDT paper |
| 4 | [**Peritext**](https://www.inkandswitch.com/peritext/) — Litt, Lim, Kleppmann, van Hardenberg | paper + prose | **The paper this project's document model is based on.** Formatting spans anchored to stable character ids, converging deterministically. RFC-001 §9's anchor decision comes from here — read it before implementing anchors |
| 5 | Joseph Gentle — [**CRDTs go brrr**](https://josephg.com/blog/crdts-go-brrr/) | blog | The performance reality of sequence CRDTs, with measurements. Also the best explanation of why the naive representation is unusable |
| 6 | [Automerge](https://github.com/automerge/automerge) — read the **storage format** docs | repo | Columnar encoding of an op log by people who did it in Rust. Directly relevant to your `collab.ops` payload design |
| 7 | **DDIA** Ch. *Consistency and Consensus* — total order broadcast section | owned | Why one owner per page gives you linearizable writes without consensus |

### Optional

| Resource | Type | Why |
|---|---|---|
| Bartosz Sypytkowski — [CRDT series](https://www.bartoszsypytkowski.com/the-state-of-a-state-based-crdts/) | blog series | State-based vs op-based, and the whole zoo. Good breadth if Peritext felt narrow |
| [Yjs internals — YATA](https://github.com/yjs/yjs/blob/main/INTERNALS.md) | repo docs | The other major sequence CRDT. Know the alternative you did not pick |
| [Loro](https://github.com/loro-dev/loro) — Fugue + movable trees | repo | Rust, current, and implements the 2023 Fugue algorithm with provable non-interleaving. The most directly comparable codebase to what you are building |
| [Figma — How multiplayer works](https://www.figma.com/blog/how-figmas-multiplayer-technology-works/) | blog | A production system that is *not* a CRDT, and why they chose that. Useful contrast |

## Before you build — part B: the rope

### Mandatory

| Resource | Type | The decision it unlocks |
|---|---|---|
| [**Rope & SumTree**](https://zed.dev/blog/zed-decoded-rope-sumtree) — Zed | blog | **Start here.** How a production editor's rope actually works, including the summary/dimension abstraction that makes anchor→offset an O(log n) lookup. This is the design you want |
| [`ropey` source](https://github.com/cessen/ropey) — read `tree/node.rs` and the `Metric` handling | repo | The best-documented Rust rope. Read it before writing yours — not to copy, but so your differences are deliberate |
| [Learn Rust With Entirely Too Many Linked Lists](https://rust-unofficial.github.io/too-many-lists/) | free book | If you have not done this: do it now, before the rope. It is the cheapest way to learn where ownership and `unsafe` collide |
| *Rust for Rustaceans* Ch. **Unsafe Code** | owned | The contract discipline: type invariant on the struct, `// SAFETY:` per block, API where misuse is impossible |
| [**Rustonomicon**](https://doc.rust-lang.org/nomicon/) — *Working with Unsafe*, *Subtyping*, *Aliasing*, *Uninitialized Memory* | free book | The four chapters the rope actually needs. Not the whole book |
| [Miri README](https://github.com/rust-lang/miri) + Ralf Jung — [Stacked Borrows](https://www.ralfj.de/blog/2018/08/07/stacked-borrows.html) / [Tree Borrows](https://www.ralfj.de/blog/2023/06/02/tree-borrows.html) | docs/blog | What Miri is checking and why your `unsafe` might be UB even though it works. Tree Borrows is the current model |

### Optional

| Resource | Type | Why |
|---|---|---|
| [`crop`](https://github.com/nomad/crop) | repo | A second rope to compare against `ropey`. Smaller, easier to read end to end |
| [xi-rope `Metric` trait](https://github.com/xi-editor/xi-editor/tree/master/rust/rope) | repo | Where the metric abstraction originated |
| [Boehm, Atkinson, Plass — *Ropes: an Alternative to Strings*](https://www.cs.tufts.edu/comp/150FP/archive/hans-boehm/ropes.pdf) | paper (1995) | The original. Short. Read for provenance, not technique |

## Before you build — part C: concurrency, durability, async

### Mandatory, in order

| # | Resource | Type | The decision it unlocks |
|---|---|---|---|
| 1 | **Rust Atomics and Locks** Ch. 1–3 | ✅ [free](https://marabos.nl/atomics/) | **Ch. 3 *Memory Ordering* is the one to read twice.** `SeqCst` vs `AcqRel` vs `Relaxed` for the op sequence generator |
| 2 | **Rust Atomics and Locks** Ch. 4–6 | ✅ | Spin lock, channels, `Arc`. Ch. 6 builds `Arc` including the weak-count subtleties |
| 3 | [**KAIST cs431**](https://github.com/kaist-cp/cs431) — lectures + the **crossbeam epoch** assignment | owned course | **The best structured treatment of epoch-based reclamation anywhere.** You are using `crossbeam::ArrayQueue` and epoch reclamation; this is where you learn what they guarantee |
| 4 | [`loom` docs](https://docs.rs/loom) + [the loom intro post](https://tokio.rs/blog/2019-10-scheduler) | docs/blog | Model checking interleavings. An `Ordering` that has not been loom-tested is a guess |
| 5 | *Rust for Rustaceans* Ch. **Asynchronous Programming** | owned | Then the hand-written `Future`. `poll`, `Pin`, `Waker` |
| 6 | Alice Ryhl — [**Actors with Tokio**](https://ryhl.io/blog/actors-with-tokio/) + [**Async: What is blocking?**](https://ryhl.io/blog/async-what-is-blocking/) | blog | The doc-actor *is* an actor. These two posts are the correct design, from a Tokio maintainer |
| 7 | [**Cancellation safety**](https://docs.rs/tokio/latest/tokio/macro.select.html#cancellation-safety) — tokio docs, then [sunshowers on cancelling async Rust](https://sunshowers.io/posts/cancelling-async-rust/) | docs/blog | **The async Rust footgun.** A WebSocket read cancelled between "op received" and "op applied" loses the op silently. Read both |
| 8 | [Postgres WAL intro](https://www.postgresql.org/docs/current/wal-intro.html) then **Database Internals** Ch. *Recovery* | docs/owned | What a WAL is and what recovery owes. Then: `flush()` is not `sync_data()`, and acknowledging before `sync_data` is a lie |
| 9 | [**Gossip Glomers**](https://fly.io/dist-sys/) challenges 3a–3e and 4 | owned exercises | Broadcast and a CRDT counter, built. Do these *before* the real thing — they are the same problems at 1% of the size |

### Optional

| Resource | Type | Why |
|---|---|---|
| Jon Gjengset — [Crust of Rust: `Pin` and `Unpin`](https://www.youtube.com/watch?v=DkMwYxfSYNQ) + [atomics](https://www.youtube.com/watch?v=rMGWeSjctlY) | video | If the book chapters left `Pin` fuzzy |
| [`tokio-console`](https://github.com/tokio-rs/console) | repo | For the first stalled flush task |
| [`rkyv`](https://rkyv.org/) | docs | Zero-copy decode. Read the **validation** section — the wire is attacker-controlled |
| [`bytes` crate](https://docs.rs/bytes/) | docs | Refcounted buffers: one allocation, N atomic increments, not N copies |
| Kleppmann — [lecture notes](https://www.cl.cam.ac.uk/teaching/2223/ConcDisSys/dist-sys-notes.pdf) §5–7 | notes | Replication and broadcast protocols, formally |
| [Chandy–Lamport snapshots](https://lamport.azurewebsites.net/pubs/chandy.pdf) | paper | Conceptual for Phase 6. Beautiful and short |

## Before you build — part D: testing the untestable

### Mandatory

| Resource | Type | The decision it unlocks |
|---|---|---|
| [**What's the big deal about Deterministic Simulation Testing?**](https://notes.eatonphil.com/2024-08-20-deterministic-simulation-testing.html) — Phil Eaton | blog | Why simulation beats chaos at this scale: chaos finds bugs in production, simulation finds them in CI, and only one reproduces |
| [**turmoil**](https://github.com/tokio-rs/turmoil) — README + examples | repo | Deterministic partitions against real code. Written by the Tokio team for exactly this |
| [**sled simulation guide**](https://sled.rs/simulation.html) — Tyler Neely | guide | "Jepsen-proof engineering" in Rust. Practical and opinionated |
| [`proptest` book](https://altsysrq.github.io/proptest-book/) — shrinking chapter | book | The invertibility law and convergence are properties. Shrinking is what makes a failure readable |

### Optional

| Resource | Type | Why |
|---|---|---|
| [awesome-deterministic-simulation-testing](https://github.com/ivanyu/awesome-deterministic-simulation-testing) | list | The whole ecosystem, curated |
| [FoundationDB testing talk](https://www.youtube.com/watch?v=4fFDFbi3toc) | video | Where DST came from. The famous one |
| [TigerBeetle VSR docs](https://github.com/tigerbeetle/tigerbeetle/blob/main/docs/internals/vsr.md) | repo | A current system designed around simulation from day one |
| [Jepsen — Linearizability checking](https://jepsen.io/consistency/models/linearizable) | reference | Before writing your own history checker |

## After it works

### Mandatory

| Resource | Why after |
|---|---|
| Raph Levien — [**xi-editor retrospective**](https://raphlinus.github.io/xi/2020/06/27/xi-retrospective.html) | **The most valuable single thing to read after Phase 3.** An honest post-mortem of an editor built on a rope, a CRDT, and async plugins — nearly your architecture. He explains which parts he regrets. It is useless before you have made the same choices and devastating after |
| Run Miri + `loom` + `cargo-fuzz` on what you built | The three tools that check what tests cannot. If any of them fails, the phase is not done |
| **Rust Atomics and Locks** Ch. 7 *Understanding the Processor* | Now that you have written atomics, this explains what they compiled to. Cache lines, `lock cmpxchg`, false sharing |

### Optional

| Resource | Why |
|---|---|
| [Zed's rope/SumTree post](https://zed.dev/blog/zed-decoded-rope-sumtree) — reread | Different post-build. First read for design; second for the details you skipped |
| [Kleppmann — Making CRDTs Byzantine fault tolerant](https://martin.kleppmann.com/papers/bft-crdt-papoc22.pdf) | Where the research went next. Not in scope; good to know it exists |
| [MIT 6.5840](https://pdos.csail.mit.edu/6.824/schedule.html) — Linearizability + Raft papers | You now have the context to read them properly. See [`papers.md`](papers.md) |
