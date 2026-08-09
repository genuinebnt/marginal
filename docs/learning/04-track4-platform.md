# Track 4 — Knowledge platform · Phases 13, 14, 15, 16, 20

`13 Identity/RBAC → 14 Comments → 15 Notifications → 16 Full editor → 20 Settings/admin`

> **Gated on the 🏁.** ADR-009 § Guard Rails is binding: nothing in this track starts before the
> MVP ships. If you are reading this before Phase 3 is done, you are reading the wrong file.

These phases are **broad rather than deep** — they add product surface and a handful of genuinely
new Rust (typestate, variance, WASM, IME). Budget less reading per phase than Track 1.

---

# Phase 13 — Identity, Spaces & RBAC · `auth-service`

**One new Rust idea and one large design question.** The Rust is typestate; the design is *which
authorization model*, and that question has a canonical answer paper.

**What you must be able to decide alone at the end:** RBAC vs ABAC vs ReBAC for your product,
whether permissions are checked at the edge or the chokepoint, and what `PhantomData`'s variance
buys you.

## Before you build

### Mandatory

| Resource | Type | The decision it unlocks |
|---|---|---|
| Google — [**Zanzibar: Google's Consistent, Global Authorization System**](https://research.google/pubs/zanzibar-googles-consistent-global-authorization-system/) | paper | **The paper on this subject.** Relationship-based authorization as tuples, and why "who can see this" is a graph reachability question. You will not build Zanzibar; you will decide consciously not to, and know what you gave up |
| [**SpiceDB / OpenFGA** concepts docs](https://authzed.com/docs/spicedb/concepts/schema) | docs | Zanzibar's ideas in a usable form. Read the schema language — it is the clearest statement of ReBAC modelling that exists |
| [NIST RBAC model](https://csrc.nist.gov/projects/role-based-access-control) — the [core RBAC paper](https://csrc.nist.gov/CSRC/media/Publications/conference-paper/1992/10/13/role-based-access-controls/documents/ferraiolo-kuhn-92.pdf) | paper | Roles, role hierarchies, separation of duty. The vocabulary everyone uses loosely |
| [**Typestate in Rust**](https://cliffle.com/blog/rust-typestate/) — Cliff Biffle | blog | **The clearest explanation.** `Op<Unchecked>` → `Op<Authorized>` as a compile-time guarantee that `can_apply` ran. Short |
| *Rust for Rustaceans* Ch. **Types** — the variance section | owned | Why `PhantomData<fn(T)>` and `PhantomData<T>` are different, and why the marker's variance matters for a typestate |
| [Rustonomicon — **Subtyping and Variance**](https://doc.rust-lang.org/nomicon/subtyping.html) | free book | The reference table. Read it *with* your typestate open |
| Jon Gjengset — [Crust of Rust: **Subtyping and Variance**](https://www.youtube.com/watch?v=iVYWDIW71jk) | video | If the Nomicon table did not click. It usually does not on first read |

### Optional

| Resource | Type | Why |
|---|---|---|
| [Airbnb's Himeji](https://medium.com/airbnb-engineering/himeji-a-scalable-centralized-system-for-authorization-at-airbnb-341664924574) / [Carta's authz post](https://medium.com/building-carta/authz-cartas-highly-scalable-permissions-system-782a7f2c840f) | blog | Two production ReBAC systems, sized closer to yours than Google's |
| [Oso — Authorization Academy](https://www.osohq.com/academy) | course | Free, well-structured, vendor-neutral enough. Good if the papers feel abstract |
| [`sealed` trait pattern](https://rust-lang.github.io/api-guidelines/future-proofing.html#sealed-traits-protect-against-downstream-implementations-c-sealed) | guidelines | The analyzer trait is sealed so a plugin cannot implement it. Same mechanism, different purpose |
| [OWASP Authorization Cheat Sheet](https://cheatsheetseries.owasp.org/cheatsheets/Authorization_Cheat_Sheet.html) | reference | The failure modes: IDOR, missing function-level checks, fail-open |

## After it works

| Resource | Why after |
|---|---|
| Run `/project:security-review` | Mandated by CLAUDE.md for any auth boundary. This phase is the largest one |
| Reread Zanzibar §3 (data model) with your schema in hand | The comparison only means something once you have something to compare |

---

# Phase 14 — Comments, Reactions & Mentions

**The phase that tests RFC-001 §9's anchor decision.** Comments are *not* ops — they are anchored
aggregates. Reactions are PN-Counters.

**What you must be able to decide alone at the end:** why comments stayed out of the op ISA, what
happens to a comment whose anchored text is deleted, and why a counter needs to be a CRDT at all.

## Before you build

### Mandatory

| Resource | Type | The decision it unlocks |
|---|---|---|
| RFC-001 **§9 Anchor Representation** + ADR-009 §on comments | project docs | **Read your own decision first.** `Resolved::Detached` exists for this phase; confirm it is sufficient before building on it |
| [**Peritext**](https://www.inkandswitch.com/peritext/) — reread the *anchors* and *marks* sections | paper | You read it in Phase 3 for marks. Comments are the third consumer of the same mechanism, and the section on what happens to a span whose text is deleted is now the relevant part |
| Shapiro et al. — [**A comprehensive study of Convergent and Commutative Replicated Data Types**](https://inria.hal.science/inria-00555588/document) §3 (counters) | paper | **PN-Counter, properly.** Why an increment-only counter converges and a plain integer does not. Read only the counter section; the paper is 50 pages |
| Bartosz Sypytkowski — [CRDT counters](https://www.bartoszsypytkowski.com/) | blog | The same thing in 10 minutes, with code |
| [Ink & Switch — **Upwelling**](https://www.inkandswitch.com/upwelling/) or [**Patchwork**](https://www.inkandswitch.com/patchwork/) | essay | Editorial workflows over a CRDT document — comments, suggestions, review. The product design thinking for exactly this phase |

### Optional

| Resource | Type | Why |
|---|---|---|
| [Google Docs' comment anchoring](https://dl.acm.org/doi/10.1145/3311957) — or any paper on *sticky positions* | paper | The general problem: a stable reference into mutable text. Yours is solved by item ids; know the alternatives |
| [`ui-mockups/reader.html`](../ui-mockups/reader.html) — comment thread and sidenotes | mockup | The UI contract you are implementing |
| [Notification/mention parsing](https://spec.commonmark.org/) — CommonMark inlines | spec | `@mention` is another inline mark. Same pipeline as `[[link]]` |

## After it works

| Resource | Why after |
|---|---|
| Test: delete the text a comment anchors to, then reload | Not a resource. `Detached { nearest_live }` either renders sensibly or the anchor design was wrong. This is the test that validates an eleven-phase-old decision |

---

# Phase 15 — Notifications · `notification-service`

**Small, bursty, degradable.** One data structure (min-heap), one hard idea (fan-out under
back-pressure), and one product rule (a lost notification costs nothing).

**What you must be able to decide alone at the end:** why `BinaryHeap` + `Reverse`, how to batch a
digest without losing events, and what "degradable" means in code rather than in prose.

## Before you build

### Mandatory

| Resource | Type | The decision it unlocks |
|---|---|---|
| [`BinaryHeap`](https://doc.rust-lang.org/std/collections/struct.BinaryHeap.html) + [`Reverse`](https://doc.rust-lang.org/std/cmp/struct.Reverse.html) docs | docs | The standard library ships a **max**-heap only. Scheduled digests are a min-heap, so `Reverse` is the whole trick |
| Skiena — Ch. *Data Structures*, priority queue section | owned | Heaps formally, and when a heap beats a sorted structure |
| [**AWS Builders' Library — Avoiding fallback in distributed systems**](https://aws.amazon.com/builders-library/avoiding-fallback-in-distributed-systems/) | article | Degradability done wrong is worse than failure. This is the argument |
| Alice Ryhl — [Actors with Tokio](https://ryhl.io/blog/actors-with-tokio/) — the **back-pressure and bounded channels** section | blog | Bursty fan-out with a bounded channel. What to do when the channel is full is a product decision, and for notifications the answer is *drop* |
| [NATS JetStream — consumers](https://docs.nats.io/nats-concepts/jetstream/consumers) | docs | Ack policies, max deliver, and the DLQ. Read before choosing a consumer type |

### Optional

| Resource | Type | Why |
|---|---|---|
| [`tokio::time::Interval` + `DelayQueue`](https://docs.rs/tokio-util/latest/tokio_util/time/delay_queue/struct.DelayQueue.html) | docs | The scheduling primitive if you would rather not hand-roll the heap. Hand-rolling is the learning |
| [Fan-out on write vs read](https://www.infoq.com/presentations/Twitter-Timeline-Scalability/) | blog | The Twitter timeline problem. Your fan-out is small; the framing is still the right one |
| [Web Push protocol](https://datatracker.ietf.org/doc/html/rfc8030) | RFC | Only if browser push is in scope. It probably is not |

## After it works

| Resource | Why after |
|---|---|
| Load-test the fan-out until the bounded channel fills | Not a resource. Confirm that a full channel drops notifications rather than blocking the doc-actor — *bulkhead isolation*, which is a `ROADMAP.md` row |

---

# Phase 16 — The Full Editor

**The phase where the browser stops being a rendering target and becomes a platform.** WASM,
selection, IME, accessibility, and text layout. This is the hardest *frontend* phase and most of
the difficulty is not Rust.

**What you must be able to decide alone at the end:** what crosses the wasm boundary and what
does not, why `contenteditable` is a trap, how IME composition interacts with an op log, and what
a screen reader needs from a block editor.

## Before you build

### Mandatory

| Resource | Type | The decision it unlocks |
|---|---|---|
| [**wasm-bindgen guide**](https://rustwasm.github.io/docs/wasm-bindgen/) — *Passing data*, *`js_sys`/`web_sys`* | book | What crosses the boundary cheaply and what copies. Your rope lives in wasm memory and the DOM does not |
| [**The `wasm-pack` book**](https://rustwasm.github.io/docs/wasm-pack/) + [Rust and WebAssembly book](https://rustwasm.github.io/docs/book/) | book | Build pipeline, and the size/performance chapters |
| [**Why ContentEditable is Terrible**](https://medium.engineering/why-contenteditable-is-terrible-122d8a40e480) — Medium Engineering | blog | **Mandatory.** The canonical write-up of why every serious editor stops trusting `contenteditable` and manages its own model. You already made this choice; this is why it was right |
| [**Text editing hates you too**](https://lord.io/text-editing-hates-you-too/) — Lord | blog | Grapheme clusters, bidi, selection, IME, and the ways naive cursor movement is wrong. Short, funny, and every paragraph is a bug you would otherwise ship |
| [**IME composition events**](https://developer.mozilla.org/en-US/docs/Web/API/Element/compositionstart_event) — MDN | docs | **The one people forget.** During composition the DOM text is provisional. Emitting an op per composition update corrupts the log; you must commit on `compositionend` |
| [Unicode **UAX #29** — Text Segmentation](https://unicode.org/reports/tr29/) + [`unicode-segmentation`](https://docs.rs/unicode-segmentation/) | spec/docs | Grapheme cluster boundaries. Cursor movement is by *grapheme*, marks are stored by *byte*, and conflating them breaks on any emoji |
| [**ARIA Authoring Practices** — textbox + toolbar patterns](https://www.w3.org/WAI/ARIA/apg/patterns/) | spec | A custom editor is invisible to assistive tech unless you say what it is. Read the `textbox`, `toolbar`, and `menu` patterns |
| [ProseMirror guide](https://prosemirror.net/docs/guide/) — *Documents*, *Transforms*, *Commands* | docs | **Read this even though you are not using it.** The best-documented block-editor architecture in existence, and its transform/step model is the closest published thing to your op model |

### Optional

| Resource | Type | Why |
|---|---|---|
| [Lexical](https://lexical.dev/docs/intro) / [Tiptap](https://tiptap.dev/docs) architecture docs | docs | Two more editor architectures. Skim for the state/view split |
| [Zed's editor rendering](https://zed.dev/blog/videogame) | blog | Text rendering as a GPU problem. Not your problem, but the framing is excellent |
| [Raph Levien on text layout](https://raphlinus.github.io/text/2020/10/26/text-layout.html) | blog | Shaping, line breaking, and why text layout is harder than it looks |
| [`twiggy`](https://github.com/rustwasm/twiggy) | tool | WASM bundle size profiling. `perf.html` draws a treemap of this; something has to produce it |
| [`syntect`](https://docs.rs/syntect/) + [`two-face`](https://docs.rs/two-face/), and [**`bat`'s `src/assets.rs`**](https://github.com/sharkdp/bat) | docs/repo | Code-block highlighting compiled to wasm. Watch the bundle budget — `two-face` bundles all of `bat`'s grammars, and **`bat` is where the allowlist-and-lazy-load answer already exists** |
| [Accessibility of rich text editors](https://www.tpgi.com/) — TPGi articles | blog | If you want to do the a11y work properly rather than minimally |

## After it works

| Resource | Why after |
|---|---|
| Test with VoiceOver or NVDA, and with an IME (Japanese or Korean) | Not a resource. Both will find bugs no unit test does. Do this before calling the editor done |
| [Rust Performance Book](https://nnethercote.github.io/perf-book/) — *Build Configuration* for wasm size | `opt-level = "z"`, LTO, `wasm-opt`. The bundle is a product constraint |

---

# Phase 20 — Instance Settings & Admin

**Almost no new theory.** The interesting part is the three-tier config model and feature flags,
both of which are decisions rather than algorithms.

**What you must be able to decide alone at the end:** which settings are startup-only vs runtime,
what a feature flag may never do, and how to change config without a restart safely.

## Before you build

### Mandatory

| Resource | Type | The decision it unlocks |
|---|---|---|
| [**Twelve-Factor App** — Config](https://12factor.net/config) | article | Old and still correct: config in the environment, no config files in the image. Read the *Config* and *Backing services* factors |
| [`arc-swap` docs](https://docs.rs/arc-swap/) | docs | Runtime config reload without a lock on the read path. Read the *Performance* and *ArcSwapAny* sections — this is the mechanism for the runtime tier |
| Martin Fowler — [**Feature Toggles**](https://martinfowler.com/articles/feature-toggles.html) | article | **The taxonomy that matters:** release toggles, experiment toggles, ops toggles, permission toggles. They have different lifetimes and mixing them is how a flag becomes permanent |
| `ROADMAP.md` § *The three tiers* + `ADR-009` §9 | project docs | Your own decision. Confirm the startup/runtime split before building the UI for it |

### Optional

| Resource | Type | Why |
|---|---|---|
| [OpenFeature spec](https://openfeature.dev/specification/) | spec | The vendor-neutral flag model. Overkill for a self-hosted instance; good vocabulary |
| [`config` crate layering](https://docs.rs/config/) | docs | Already in use. Read if you change the precedence order |
| [Cloud Run / GKE config reload patterns](https://cloud.google.com/run/docs/configuring/environment-variables) | docs | How a restart-free change actually reaches a pod |

## After it works

| Resource | Why after |
|---|---|
| Audit every flag for an owner and a removal date | Not a resource. Fowler's article says flags are debt; this is when you start paying it |