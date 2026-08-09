# Foundations — read alongside everything

Two kinds of thing live here:

1. **§1 Start here** — a concrete ten-day order for someone who has the repo open and does not
   know what to read first. Follow it literally.
2. **§2–§6** — the four theory areas the early phases depend on (data modelling, API design,
   compiler theory, distributed systems theory) plus the Rust spine and tooling. These are
   *reference sections*: each phase file links into them rather than repeating them.

> Books you own are cited **by chapter title, never by number** — chapter numbers move between
> editions and titles do not.

---

## 1. Start here — the first ten days

You are at **Phase 1, `document-service`**, with **no code written**. Everything below is yours,
starting from an empty workspace. This is the order that gets you from *no idea where to start* to *writing
`libs/domain` with opinions you can defend.*

Each day ends in a **deliverable that is not code** — a decision written down, or an existing doc
you have read critically enough to disagree with. That is deliberate: the code is the easy part
once the decisions are yours.

| Day | Read | Then decide | Deliverable |
|---|---|---|---|
| **1** | *Rust for Rustaceans* Ch. **Project Structure** · matklad [Large Rust Workspaces](https://matklad.github.io/2021/08/22/large-rust-workspaces.html) · then `PROJECT_STRUCTURE.md` | Whether the `libs/` + `services/` split and the inline→duplicate→extract rule are right | Three sentences on where you disagree with `PROJECT_STRUCTURE.md`, or why you don't |
| **2** | §2 below — **Data modelling**, mandatory rows only | Whether `docs.pages` and `docs.blocks` are correctly shaped | Read `DATA_MODEL.md` §4 and justify *every* column out loud. Any you cannot justify is a bug — in the doc or in your understanding |
| **3** | §2 continued — indexes and trees. [use-the-index-luke](https://use-the-index-luke.com/) Ch. 1–3 · [LTREE docs](https://www.postgresql.org/docs/current/ltree.html) · [Don't Do This](https://wiki.postgresql.org/wiki/Don't_Do_This) | Adjacency list + materialised path, or something else | Write down *why* `path LTREE` and `parent_id` are both stored when either alone is lossy-but-workable |
| **4** | §3 below — **API design**. Google AIP-121, 122, 131–135, 193 · [protobuf Do's and Don'ts](https://protobuf.dev/best-practices/dos-donts/) | Whether `docs/api/pages.md`'s `PageService` is well-shaped | Find at least one thing in `pages.md` that violates an AIP, and decide whether the AIP or the doc wins |
| **5** | *Rust for Rustaceans* Ch. **Designing Interfaces** · [Rust API Guidelines](https://rust-lang.github.io/api-guidelines/) checklist | How `PageId`, `SortKey`, `MaterialisedPath` should feel to use | The signatures for `libs/domain` — types and `impl` blocks, `todo!()` bodies |
| **6** | **Write code.** Domain newtypes + `TryFrom` validation | — | **Write the failing tests first** (`agents.md` § stage 1), then make them pass |
| **7** | §2 § *Fractional indexing* — [Figma](https://www.figma.com/blog/realtime-editing-of-ordered-sequences/) then [David Greenspan's notebook](https://observablehq.com/@dgreensp/implementing-fractional-indexing) | The alphabet, and what happens when keys grow | `SortKey::key_between` — 11 more tests pass |
| **8** | *Rust for Rustaceans* Ch. **Testing** · [`#[sqlx::test]` docs](https://docs.rs/sqlx/latest/sqlx/attr.test.html) | How integration tests get a database | `PageRepo` trait signature + the first failing repo test against real Postgres |
| **9–10** | §4 below — **Compiler theory**, mandatory rows. *Crafting Interpreters* Ch. **Scanning** · [CommonMark spec](https://spec.commonmark.org/) §4 (leaf blocks) skim | What an input rule is, precisely | The input-rule scanner's signature, and a written rule for when `## ` becomes a heading |

**After day 10 you are inside Phase 1's build order** (`lld/document-service.md` §11) and this
document stops being a schedule and becomes a per-phase reference.

### If you read only three things this month

1. matklad — [Large Rust Workspaces](https://matklad.github.io/2021/08/22/large-rust-workspaces.html). Twelve minutes. Prevents a month of churn.
2. *Rust for Rustaceans* Ch. **Designing Interfaces**. The newtype and `TryFrom` discipline the whole domain layer rests on.
3. [Google AIP-121 Resource-oriented design](https://google.aip.dev/121). Your gRPC contract either follows a convention or invents one badly.

---

## 2. Data modelling — how to arrive at a schema

You do not have a data-modelling gap so much as a *method* gap: how to go from "pages contain
blocks" to a set of tables you can defend. The method is four questions, in order.

> **The method.** ① What are the entities, and what is the *one* thing each owns? ② What is the
> access pattern — what queries must be fast, and which are allowed to be slow? ③ What must be
> true always (invariant), and can the database express it? ④ What will change later, and what
> does that force to be additive?
>
> `DATA_MODEL.md` is the answer to those four questions for Marginal. Your job on day 2 is to
> re-derive it and find where you'd answer differently.

### Mandatory

| Resource | Type | Why this one, and what it decides |
|---|---|---|
| **DDIA** Ch. *Data Models and Query Languages* | owned | The relational/document/graph trilemma. Marginal is **all three at once** — relational pages, JSONB document content, a graph of `[[links]]` — and this chapter is why that is a choice rather than a mess |
| **DDIA** Ch. *Encoding and Evolution* | owned | Decides `content_version` and `encoding_version`. Read it before you write a migration, because it is the argument for why **additive-only** is not laziness |
| [**PostgreSQL: Don't Do This**](https://wiki.postgresql.org/wiki/Don't_Do_This) | wiki | 20 minutes, saves a schema. `timestamp` without a zone, `char(n)`, `serial`, table inheritance, `money`. Every item is a decision you might otherwise make wrong once |
| [**use-the-index-luke**](https://use-the-index-luke.com/) Ch. 1–3 | free book | Markus Winand. **The** resource on indexes. Ch. 1–3 gets you concatenated-index column order, which is exactly the `(parent_id, sort_key)` question in `docs.pages` |
| [**LTREE**](https://www.postgresql.org/docs/current/ltree.html) + [GiST index](https://www.postgresql.org/docs/current/gist.html) docs | docs | You are using it. `<@`, `~`, label restrictions — the last of which is the hyphen trap in `lld/document-service.md` §12 |
| [**Indexing JSONB in Postgres**](https://www.crunchydata.com/blog/indexing-jsonb-in-postgres) | blog | `jsonb_path_ops` vs default GIN, and when neither helps. `docs.blocks.content` is JSONB with a GIN index and you should know what that index can and cannot answer |
| **Database Internals** Part I — Ch. *B-Tree Basics* → *Implementing B-Trees* | owned | What an index physically is. You cannot reason about index choice from the outside; this is the inside |

### Optional

| Resource | Type | Why |
|---|---|---|
| [CMU **15-445**](https://15445.courses.cs.cmu.edu/) lectures on storage + indexes | owned course | Pavlo. If Database Internals Part I felt thin, this is the same material with a lecturer |
| **SQL Antipatterns** — Karwin, Ch. *Naive Trees* | book | The canonical comparison of adjacency list / path enumeration / nested sets / closure table. If you want to *defend* the LTREE choice against three alternatives, this is the chapter |
| [Fractional indexing — Figma](https://www.figma.com/blog/realtime-editing-of-ordered-sequences/) | blog | Where the term comes from. Short |
| [Implementing Fractional Indexing](https://observablehq.com/@dgreensp/implementing-fractional-indexing) | notebook | David Greenspan. The actual midpoint algorithm, executable. **Read this before writing `key_between`** — it is Phase 1 mandatory, listed optional here only because it belongs to one day |
| [How Figma's multiplayer technology works](https://www.figma.com/blog/how-figmas-multiplayer-technology-works/) | blog | A whole collaborative data model explained by someone who shipped it. The best single overview of the problem space you are entering |
| **Domain Modeling Made Functional** — Wlaschin | book | Making illegal states unrepresentable. Written for F# and applies wholesale to Rust newtypes — this is the *why* behind `PageId(Uuid)` instead of `Uuid` |
| [Postgres **row estimates** / `EXPLAIN`](https://www.postgresql.org/docs/current/using-explain.html) | docs | You will need this the first time a query is slow. Not before |

### Post-build (after Phase 1 works)

| Resource | Why after |
|---|---|
| **The Art of PostgreSQL** — Fontaine, Ch. on data modelling + *Concurrency* | Reads as a set of opinions until you have your own to compare. Then it reads as a review of your schema |
| [Postgres 18 release notes](https://www.postgresql.org/docs/18/release-18.html) | You depend on `uuidv7()`. Know what else arrived, and which of it Cloud SQL lags on |

---

## 3. API design — how to arrive at a contract

Marginal has **two** contracts and they are designed differently: gRPC east-west (ADR-007) and
REST at the gateway. The mistake to avoid is designing one and mechanically translating.

> **The method.** ① Name the resources, not the actions. ② Use standard methods before inventing
> custom ones. ③ Decide the error taxonomy *once*, centrally. ④ Design for the change you know is
> coming — pagination, partial response, idempotency. ⑤ Only then map to transport.

### Mandatory

| Resource | Type | Why this one, and what it decides |
|---|---|---|
| [**Google AIP-121** Resource-oriented design](https://google.aip.dev/121) | spec | The single most useful thing you can read about API design. Resources over RPCs, and why. Twenty minutes |
| [**AIP-122** Resource names](https://google.aip.dev/122) | spec | `pages/{page}/blocks/{block}` versus ad-hoc ids. Decides your proto message shapes |
| [**AIP-131–135** Standard methods](https://google.aip.dev/131) | spec | Get, List, Create, Update, Delete — the exact semantics each owes. **Your `PageService` has six RPCs; this says what five of them must mean** |
| [**AIP-193** Errors](https://google.aip.dev/193) | spec | The error model. Pairs with `docs/api/pages.md` §2's status translation, and with `lld/document-service.md` §8 |
| [**AIP-158** Pagination](https://google.aip.dev/158) | spec | Page tokens, not offsets. You will need this for `ListPages` and it is much cheaper to design in |
| [**Protobuf Do's and Don'ts**](https://protobuf.dev/best-practices/dos-donts/) | docs | Field numbering, `reserved`, never renumber, enum zero value. **The `LifecycleState.UNSPECIFIED = 0` rule in `pages.md` comes from here** |
| [**proto3 language guide**](https://protobuf.dev/programming-guides/proto3/) | docs | Read the sections on defaults, `optional` presence, and `oneof`. Field presence in proto3 is a genuine footgun |
| **Microservice Patterns** — Richardson, Ch. *Interprocess communication in a microservice architecture* | owned | The IPC chapter. Sync vs async, API versioning, message formats. Written for exactly your topology |

### Optional

| Resource | Type | Why |
|---|---|---|
| [**AIP-180** Backwards compatibility](https://google.aip.dev/180) | spec | What you may and may not change later. Read before the first breaking change tempts you |
| [Zalando RESTful API Guidelines](https://opensource.zalando.com/restful-api-guidelines/) | guidelines | The best public REST guideline set. Use for the **gateway's** REST surface, where AIPs are less directly applicable |
| [tonic docs](https://docs.rs/tonic/latest/tonic/) + [tonic examples](https://github.com/hyperium/tonic/tree/master/examples) | docs | The four RPC modes in practice. Streaming server/client/bidi is where ADR-007's claim gets cashed |
| [gRPC status codes](https://grpc.io/docs/guides/status-codes/) | docs | The canonical list with intended meanings. `FAILED_PRECONDITION` vs `ABORTED` vs `UNAVAILABLE` is a distinction `pages.md` §3 already leans on |
| [OpenAPI 3.1 spec](https://spec.openapis.org/oas/latest.html) | spec | You generate this from `utoipa`. Skim so the generated output is legible rather than magic |
| [Stripe API design talk — *Move Fast, Don't Break Your API*](https://www.youtube.com/results?search_query=stripe+api+versioning+talk) | video | Versioning in a product people depend on. Optional but it is the industry's best-argued position |

### Post-build

| Resource | Why after |
|---|---|
| [`cargo-semver-checks`](https://github.com/obi1kenobi/cargo-semver-checks) | Once `libs/proto` has consumers, this catches the break you would otherwise ship |
| [Buf — protobuf lint & breaking-change detection](https://buf.build/docs/breaking/overview) | The CI answer to "did I break the wire format". Worth adopting once the proto stabilises |

---

## 4. Compiler theory — for the document model, ops, and diagnostics

**Marginal is a compiler that happens to look like a notebook.** The editor front end lexes and
parses (Phase 1, 16), the op log is a bytecode ISA (Phase 3, RFC-002), and diagnostics are
incremental semantic analysis (Phase 4, RFC-003). `ui-mockups/compiler.html` runs the front end
so you can see the stages.

> **The method.** Source → tokens → tree → lowered form → analysis. Each arrow is a separate
> program with its own tests. The editor difference from a batch compiler: **it must be
> incremental and it must never reject its input** — there is no compile step and nothing is
> ever "broken".

### Mandatory

| Resource | Type | Why this one, and what it decides |
|---|---|---|
| **Crafting Interpreters** Ch. **Scanning** | owned/[free](https://craftinginterpreters.com/scanning.html) | A hand-written lexer with no generator. Exactly the shape of the input-rule scanner and the block lexer |
| **Crafting Interpreters** Ch. **Representing Code** + **Parsing Expressions** | owned | Recursive descent, and why an AST is a separate thing from the token stream. This is `compiler.html`'s stages 2–3 |
| **Crafting Interpreters** Part II Ch. **Chunks of Bytecode** + **A Virtual Machine** | owned | **Read this to understand the op log.** An op is an instruction, `apply` is the interpreter loop, and `invert` is the thing a normal VM does not have. RFC-002's ISA framing comes from here |
| [**CommonMark spec**](https://spec.commonmark.org/) §4 *Leaf blocks* + §6 *Inlines* | spec | You are parsing a markdown subset. Read how the reference spec defines the thing you are approximating, and where you deliberately differ |
| matklad — [**Resilient LL Parsing Tutorial**](https://matklad.github.io/2023/05/21/resilient-ll-parsing-tutorial.html) | blog | **The one that matters most for an editor.** A parser for an IDE must produce a tree for *broken* input, because the input is broken most of the time — halfway through a keystroke. Batch-compiler parsing does not teach this |

### Optional

| Resource | Type | Why |
|---|---|---|
| matklad — [Simple but Powerful Pratt Parsing](https://matklad.github.io/2020/04/13/simple-but-powerful-pratt-parsing.html) + **Crafting Interpreters Ch. *Compiling Expressions*** | blog / owned | **Marginal will never force you to write a Pratt parser** — the inline grammar has no precedence, only nesting, and a formula language is out of scope (ADR-001). Read these if you want the technique for a DSL later; `codecrafters-interpreter-rust` is where you would already have written one |
| [KAIST **cs420** compilers](https://github.com/kaist-cp/cs420) | owned course | If you want the formal treatment — IR design, dataflow analysis, optimisation. More than Phase 1 needs and exactly right before Phase 4 |
| [**tree-sitter**](https://tree-sitter.github.io/tree-sitter/) — docs + [the design writeup](https://tree-sitter.github.io/tree-sitter/#underlying-research) | docs | Incremental GLR parsing for editors. The industrial answer to the problem RFC-001 solves differently. Know the alternative you did not pick |
| matklad — [Explaining rust-analyzer](https://www.youtube.com/playlist?list=PLhb66M_x9UmrqXhUuAaLwO_UT8XORQV8m) | video series | Red-green trees, incrementality, IDE architecture, by the person who built it. Slow but unmatched |
| [pulldown-cmark source](https://github.com/pulldown-cmark/pulldown-cmark) | repo | A fast CommonMark parser in Rust. Read the block/inline split before designing yours |
| [Dragon book](https://en.wikipedia.org/wiki/Compilers:_Principles,_Techniques,_and_Tools) Ch. 3 (lexing) | book | Only if you want the theory of regular languages → DFA behind your scanner. Skippable |

### If the real goal is writing parsers and DSLs

Worth naming, because it changes what "compiler theory" means for you.

**There are two things called the *middle end* and this project teaches only one of them.**

| | Compiler middle end | Editor / language-server middle end |
|---|---|---|
| Job | Make the program faster | Answer questions quickly, repeatedly, **on broken input** |
| Core ideas | SSA, dataflow lattices, inlining, loop transforms | Name resolution, incremental invalidation, error tolerance, spans |
| Optimises | Throughput of a batch run | Latency of the 400th keystroke |
| In Marginal | **Absent** | **Phase 4** |

For a DSL you want the second. Nobody designing a language starts with SSA — and note that **SSA
is middle-end, not back-end**; the true back end is instruction selection, register allocation and
ABI. Most of what people call "LLVM stuff" is middle end.

**What Marginal covers for language work:** hand-written lexing (three of them — block, inline,
and a bounded cursor scanner) · recursive descent · **error-tolerant parsing** · grammar design ·
AST and lowering passes · name resolution · **incremental invalidation** · diagnostics with spans
and quick fixes.

**Calibrate what that is and is not.** The grammar is *small* — a markdown subset with no
precedence and no ambiguity — so this is real recursive descent but not hard grammar work:

| Axis | Marginal | A typical language compiler |
|---|---|---|
| Grammar complexity | **Low** | High |
| **Error tolerance** | **High** — must never reject its input | Usually low |
| **Incrementality** | **High** (Phase 4) | Usually none |
| Performance under continuous editing | **High** | Irrelevant |

That trade is favourable for DSL work: grammar complexity is the part you can look up, and error
recovery is the part that decides whether anyone can stand using your language.

**The two gaps, and how to close them:**

| Gap | Shortest useful path |
|---|---|
| **Type checking** — no inference, no unification, no HM anywhere in this project | Start with **bidirectional typing**, not full Hindley–Milner: it is what most real languages implement and it is far easier to get right. [Dunfield & Krishnaswami — *Bidirectional Typing*](https://arxiv.org/abs/1908.05839) is the survey; [thunderseethe's type-inference series](https://thunderseethe.dev/posts/type-inference/) is the readable Rust-flavoured version. [TAPL](https://www.cis.upenn.edu/~bcpierce/tapl/) is the reference if you want depth |
| **Evaluation** — Marginal emits ops, never a program you run | You have already done this: `codecrafters-interpreter-rust`. [Crafting Interpreters Part II](https://craftinginterpreters.com/a-bytecode-virtual-machine.html) is the book — bytecode VM, closures, GC |

**Rust tooling worth knowing when you build a DSL for real** (none of it needed for Marginal, which
hand-writes everything on purpose):

| Crate | For |
|---|---|
| [`logos`](https://github.com/maciejhirsz/logos) | Derive-based lexer. Fast, and the generated DFA is readable |
| [`chumsky`](https://github.com/zesterer/chumsky) | Parser combinators **with first-class error recovery** — the rare one |
| [`winnow`](https://github.com/winnow-rs/winnow) / [`lalrpop`](https://github.com/lalrpop/lalrpop) | The combinator and the LR-generator alternatives |
| [`ariadne`](https://github.com/zesterer/ariadne) · [`miette`](https://github.com/zkat/miette) · [`codespan`](https://github.com/brendanzab/codespan) | **Beautiful diagnostics.** Error-message quality is most of a DSL's felt usability and these do the hard part |
| [`rowan`](https://github.com/rust-analyzer/rowan) / [`cstree`](https://github.com/domenicquirl/cstree) | Lossless syntax trees — red/green, what rust-analyzer uses |
| [`salsa`](https://salsa-rs.github.io/salsa/) | Incrementality. Same crate Phase 4 studies |
| [LSP spec](https://microsoft.github.io/language-server-protocol/) | If anyone will *edit* your DSL, it needs a server. Phase 4's diagnostics model already matches it |

### Post-build

| Resource | Why after |
|---|---|
| Raph Levien — [xi-editor retrospective](https://raphlinus.github.io/xi/2020/06/27/xi-retrospective.html) | **Read this after Phase 3, not before.** An honest post-mortem of an editor built on a rope + CRDT + async plugin model — which is close to Marginal's design. He explains what he would not do again. It is only useful once you have made the same choices |
| [Salsa architecture](https://salsa-rs.github.io/salsa/) | Phase 4's engine. Listed properly there |

---

## 5. Distributed systems theory — the ladder

You own most of this. The gap is *order* — distributed systems reading is notoriously easy to
do in a sequence that leaves you with vocabulary and no judgement.

> **The ladder.** ① Why is this hard at all (partial failure, no global clock). ② What can be
> ordered (Lamport, vector clocks, causality). ③ What is impossible (FLP, CAP, Two Generals).
> ④ What consistency models exist and how they relate. ⑤ How real systems choose. ⑥ How you
> would *test* any of it.
>
> Do not start at ⑤. Everyone starts at ⑤ — reading about Raft before understanding why ordering
> is hard is how people end up quoting CAP incorrectly for years.

### Mandatory — in this order

| # | Resource | Type | What rung it is |
|---|---|---|---|
| 1 | **DDIA** Ch. *The Trouble with Distributed Systems* | owned | ① Partial failure, unreliable clocks, unbounded delay. **Start here, always** |
| 2 | Kleppmann — [Distributed Systems lecture notes](https://www.cl.cam.ac.uk/teaching/2223/ConcDisSys/dist-sys-notes.pdf) §2–4 + [video course](https://www.youtube.com/playlist?list=PLeKd45zvjcDFUEv_ohr_HdUFe97RItdiB) | free notes/video | ②③ Cambridge course, ~8 hours total, free. The cleanest treatment of logical clocks and causality that exists. **The single best free distributed-systems resource** |
| 3 | Lamport — [Time, Clocks, and the Ordering of Events](https://lamport.azurewebsites.net/pubs/time-clocks.pdf) | paper | ② The original. Short, readable, and everything after it is a footnote. Marginal's op log needs *vector* clocks; this is why Lamport timestamps alone are not enough |
| 4 | **DDIA** Ch. *Consistency and Consensus* | owned | ④ Linearizability, causality, total order broadcast, consensus. The chapter that ties the ladder together |
| 5 | aphyr — [Strong consistency models](https://aphyr.com/posts/313-strong-consistency-models) + [jepsen.io/consistency](https://jepsen.io/consistency) | blog/reference | ④ The hierarchy as a *picture*. Bookmark the second one permanently — it is the map you will re-check for years |
| 6 | Kleppmann — [Please stop calling databases CP or AP](https://martin.kleppmann.com/2015/05/11/please-stop-calling-databases-cp-or-ap.html) | blog | ③ Why the CAP framing you have absorbed from blog posts is wrong. **Read before writing anything that claims CP** — `ROADMAP.md` claims exactly that for ops |
| 7 | [**Gossip Glomers**](https://fly.io/dist-sys/) challenges 1, 2, 3a–3b, 4 | owned exercises | ⑤⑥ Stop reading, start building. Challenge 4 is a CRDT counter and it will teach you more about convergence than three papers |

### Optional

| Resource | Type | Why |
|---|---|---|
| [Distributed Systems for Fun and Profit](http://book.mixu.net/distsys/) — Takada | free book | A two-hour survey. Good if you want the whole map before the detail. Some of it is dated |
| **Database Internals** Part II Ch. *Failure Detection* + *Anti-Entropy and Dissemination* | owned | ⑤ Reads better at Phase 10 than now, but it is where phi-accrual and Merkle anti-entropy live, and both are on your roadmap |
| [MIT **6.5840** paper list](https://pdos.csail.mit.edu/6.824/schedule.html) | owned course | ⑤ The canon. See [`papers.md`](papers.md) for which paper serves which phase — reading them in course order is *not* optimal for this project |
| Murat Demirbas — [metadata blog](http://muratbuffalo.blogspot.com/) | blog | Paper reviews by someone who reads everything. Best way to triage a paper before committing to it |
| Marc Brooker — [blog](https://brooker.co.za/blog/) | blog | ⑤ The operational half nobody teaches: timeouts, retries, backoff, load shedding, why your p99 is the sum of everyone's tails |
| [AWS Builders' Library — Timeouts, retries and backoff with jitter](https://aws.amazon.com/builders-library/timeouts-retries-and-backoff-with-jitter/) | article | ⑤ Twenty minutes, permanently changes how you write a retry. Phase 3 and 9 both need it |
| [Two Generals](https://en.wikipedia.org/wiki/Two_Generals%27_Problem) + [FLP impossibility](https://groups.csail.mit.edu/tds/papers/Lynch/jacm85.pdf) | wiki/paper | ③ Why exactly-once delivery is not a feature you can request. `ARCHITECTURE.md` relies on this |

### Post-build

| Resource | Why after |
|---|---|
| [Jepsen analyses](https://jepsen.io/analyses) | Pick any database you have used. Reading how a real system violated its own claims is the argument for testing yours. Best read *after* you have written a claim of your own |
| [awesome-deterministic-simulation-testing](https://github.com/ivanyu/awesome-deterministic-simulation-testing) + [Phil Eaton on DST](https://notes.eatonphil.com/2024-08-20-deterministic-simulation-testing.html) | ⑥ How to test the untestable. Phase 3 mandatory; here as the pointer |
| [sled simulation guide](https://sled.rs/simulation.html) | Tyler Neely on "jepsen-proof engineering" in Rust specifically |

---

## 6. The Rust spine

Read these *across* phases, not before them. Each is cited again by the phase that forces it.

### Mandatory, in rough order of when you will need it

| Resource | Owned? | When it becomes urgent |
|---|---|---|
| *Rust for Rustaceans* Ch. **Foundations**, **Types**, **Designing Interfaces** | ✅ | Now — Phase 1's newtypes |
| [Rust API Guidelines](https://rust-lang.github.io/api-guidelines/) | free | Now — as a checklist, not a read-through |
| *Rust for Rustaceans* Ch. **Error Handling** | ✅ | Phase 1 — `AppError`/`ApiError` and the `thiserror`/`anyhow` split |
| *Rust for Rustaceans* Ch. **Testing** | ✅ | Phase 1 — before the repo tests |
| *Rust for Rustaceans* Ch. **Asynchronous Programming** | ✅ | Phase 3 |
| [Async Rust book](https://rust-lang.github.io/async-book/) + Alice Ryhl [Actors with Tokio](https://ryhl.io/blog/actors-with-tokio/), [Async: What is blocking?](https://ryhl.io/blog/async-what-is-blocking/) | free | Phase 3 — the doc-actor is an actor |
| **Rust Atomics and Locks** Ch. 1–3, then 4–6 | ✅ [free](https://marabos.nl/atomics/) | Phase 3 — Ch. 3 *Memory Ordering* twice |
| *Rust for Rustaceans* Ch. **Unsafe Code** + [Rustonomicon](https://doc.rust-lang.org/nomicon/) | ✅/free | Phase 3 — the rope's leaves |
| *Rust for Rustaceans* Ch. **Concurrency** | ✅ | Phase 3, 5 |
| *Rust for Rustaceans* Ch. **Macros** | ✅ | Phase 0 — `define_id!` and the derive macros |

### Optional but high value

| Resource | Why |
|---|---|
| [Rust Design Patterns](https://rust-unofficial.github.io/patterns/) | Idiom lookup. Newtype, RAII guards, `Deref` polymorphism (and why to avoid it) |
| Jon Gjengset — [Crust of Rust](https://www.youtube.com/playlist?list=PLqbS7AVVErFiWDOAVrPt7aYmnuuOLYvOa) | Watch the episode matching your current confusion. `Pin`, variance, atomics, channels, iterators, lifetime annotations |
| [The Rust Performance Book](https://nnethercote.github.io/perf-book/) | Nethercote. Read the *Profiling* and *Benchmarking* chapters before your first optimisation, not after |
| [Learn Rust With Entirely Too Many Linked Lists](https://rust-unofficial.github.io/too-many-lists/) | The best hands-on introduction to why `unsafe` and ownership fight. Do it before the rope |
| [Rust Atomics and Locks](https://marabos.nl/atomics/) Ch. 7 *Understanding the Processor* | Cache lines, false sharing, what `lock cmpxchg` compiles to. Directly behind `#[repr(align(64))]` |
| [std lib source](https://doc.rust-lang.org/std/) | Read `VecDeque`, `BTreeMap`, `Vec::dedup_by`. The stdlib is the best-reviewed Rust you have access to |

---

## 7. Tooling — set up once, use forever

| Tool | Docs | When |
|---|---|---|
| `clippy` | [lint list](https://rust-lang.github.io/rust-clippy/master/) | Day 1. Read the *pedantic* group once so you know what you are opting out of |
| `rustfmt` | [config](https://rust-lang.github.io/rustfmt/) | Day 1. Commit a `rustfmt.toml` and stop discussing it |
| `proptest` | [book](https://altsysrq.github.io/proptest-book/) | Phase 1 — `key_between` is a property, not an example |
| **`insta`** | [docs](https://docs.rs/insta/latest/insta/) | **Phase 1 — adopt it.** Block trees, ASTs, op lists and diagnostics output are large structured values; asserting on them by hand is how test suites rot. `cargo insta review` makes an intentional change a one-key confirmation |
| `cargo-fuzz` | [book](https://rust-fuzz.github.io/book/) | Phase 1 — the paste sanitiser is attacker-facing |
| `cargo-mutants` | [mutants.rs](https://mutants.rs/) | After Phase 1 — you write the tests before the code, so this is the question that follows: *are they any good?* |
| Miri | [README](https://github.com/rust-lang/miri) | Phase 3 — the moment you write `unsafe` |
| `loom` | [docs](https://docs.rs/loom) | Phase 3 — the moment you write an atomic |
| `criterion` | [book](https://bheisler.github.io/criterion.rs/book/) | Phase 1, 3, 7 — "it got faster" without an interval is a guess |
| `tokio-console` | [repo](https://github.com/tokio-rs/console) | Phase 3 — a task that never yields is invisible otherwise |
| `cargo-deny` | [book](https://embarkstudios.github.io/cargo-deny/) | Phase 11 — licences and advisories |
| `turmoil` | [repo](https://github.com/tokio-rs/turmoil) | Phase 3, 10 — deterministic partitions |
| `sqlx` offline | [`cargo sqlx prepare`](https://github.com/launchbadge/sqlx/blob/main/sqlx-cli/README.md) | Phase 1 — commit `.sqlx/`, or CI cannot compile your queries |

> **Set up `clippy`, `rustfmt`, and the `wasm32` CI gate on day one.** All three are cheap now
> and expensive to retrofit — the third because three later phases depend on `libs/doc` staying
> browser-clean, and prose does not fail a build (`ROADMAP.md` § The wasm32 rule needs a gate).
