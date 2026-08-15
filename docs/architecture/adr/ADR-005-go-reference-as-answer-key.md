# ADR-005 — Go Reference Implementations as an Answer Key

**Date:** 2026-08-06
**Status:** Accepted
**Related:** ADR-002 (Rust depth primary)
**Deciders:** @genuinebasilnt

---

## Context

Every feature previously required solving three problems at once: *what should this do* (business logic), *what algorithm fits* (DSA), and *how is this expressed in Rust* (ownership, lifetimes, trait objects, async). Fighting all three simultaneously made progress slow and made it unclear which one was actually the blocker.

The original proposal was to have an AI write the whole product in another language and port it to Rust, removing the first two problems.

That was rejected in its original form for two reasons.

**It deletes a stated goal.** DSA is an explicit objective and the DSA map is the largest section of the roadmap. Porting an AI-written BK-tree teaches the *shape* of a BK-tree, not why the triangle inequality permits subtree pruning.

**The return curve is inverted.** A large part of the interesting work exists *because Rust has no garbage collector*:

| Roadmap item | Go / TypeScript equivalent |
|---|---|
| `crossbeam-epoch` epoch-based reclamation | none — GC handles it |
| `typed-arena` / `slotmap` for the op log | none — GC handles it |
| `MaybeUninit<[Node; N]>` in the rope | none |
| `#[repr(C)]` / `#[repr(align(N))]` | none |
| `Arc<RwLock<T>>` vs channel ownership choices | none |

A garbage-collected reference has nothing to port for any of these. It saves the least effort exactly where the work is most ambitious.

A third, practical objection: reading tens of thousands of lines of a design you did not make is slower than writing a smaller amount of one you did.

---

## Decision

Adopt a **per-feature Go reference, revealed as an answer key** rather than as a worked example.

The governing distinction: **business logic may be outsourced; algorithms may not.**

### Five stages, in order

| Stage | Owner | Output |
|---|---|---|
| 1. Design | @genuinebasilnt | `DATA_MODEL.md`, `docs/api/`, ADR/RFC if warranted |
| 2. Specification | AI | Failing Rust test suite; invariants stated in prose |
| 3. Orchestration reference | AI | Go implementation of saga sequencing, NATS choreography, retry/backoff → `reference/` |
| 4. Implementation | @genuinebasilnt | The Rust code |
| 5. Answer key | AI | Go reference for DSA items — **written only after stage 4 is attempted** |

**Stage 5 is the load-bearing inversion.** The reference for any DSA item is not written until an attempt exists, which makes premature reading impossible rather than merely discouraged.

### Stage 3 is orchestration only

Per ADR-002, **handlers and repositories are not plumbing.** They are where `async_trait`, `Arc<dyn Trait>` vs `impl Trait`, sqlx type mapping, extractor lifetimes, and `From` error-conversion chains are learned. Handing those over in Go removes real Rust practice.

Stage 3 covers: saga step sequencing, NATS choreography, retry and backoff policy. Nothing else.

### Why Go

| Go | Rust |
|---|---|
| explicit `if err != nil` | `Result<T, E>` and `?` |
| interfaces | traits |
| goroutines + channels | tokio tasks + `mpsc` |
| structs, no inheritance | structs, no inheritance |

TypeScript was rejected — GC plus exceptions plus structural typing turn the port into a rewrite. OCaml/F# match the type system better but teach nothing about the concurrency model. Prose plus pseudocode remains the fallback where a runnable reference adds nothing.

---

## Consequences

### Repository layout

```
reference/          Go reference implementations
```

Tracked, **not** in the Cargo workspace, never deployed. Scaffolding — delete per-feature once the Rust passes its tests, or keep for diffing.

### Mentor rules are unchanged

`.agents/agents.md` already permitted illustrative code in other languages while forbidding ready-to-paste Rust. This scales that permission from snippet to feature and adds the stage-5 ordering constraint. **The prohibition on AI-written Rust stands** — "I give up" remains the only route to a Rust solution.

### What this cannot help with

The GC-dependent items above have no reference by construction. Arena allocation, epoch reclamation, `MaybeUninit`, and `repr(align)` are unaided work supported only by prose, tests, and links. Say so plainly rather than producing a misleading equivalent.

### Risk accepted

A Go reference biases the port toward Go idioms — `interface{}`-flavoured trait objects where generics belong, channel-passing where ownership transfer is simpler, stringly-typed errors instead of `thiserror` variants. Stage 2 tests and `/project:simplify` are the mitigation; strict review must call out Go-shaped Rust by name.

---

## The SPA is outsourced, the editor core is not

**Added 2026-08-07.**

This ADR governs who writes what. One boundary moved: **the TypeScript SPA in `web/` is
written for me; all Rust remains mine, including the `wasm32` editor core.**

| | Owner |
|---|---|
| Services, `crates/`, **and the editor core compiled to `wasm32`** | Me |
| `web/` — shell, routing, panels, DOM plumbing, API client, styling | Outsourced |

**Why this does not weaken ADR-002.** Rust depth is the primary objective, and the SPA is
TypeScript. Outsourcing it removes ~80–120h of work that teaches no Rust, and it moves the
frontend off the critical path — it proceeds in parallel rather than after the backend
(`TIMELINE.md` §3).

**The line that must not move.** `agents.md` already forbids reimplementing the document
model, diagnostics, or syntax highlighting in JavaScript. That rule now has teeth it did not
need before: where the SPA needs a document operation and the `wasm-bindgen` binding does not
exist yet, **the call is stubbed, never written in TypeScript.**

The failure mode is quiet and terminal. A selection helper written in TS "just for now"
becomes the document model, and Phase 3 — the rope, the CRDT, the WAL, the whole demo —
stops having a reason to exist. Core Principle 1 protects Rust *algorithms*; this protects
Rust *ownership of the domain*, which is the same rule pointed at a different axis.

**The consequence to plan around:** the `wasm-bindgen` API surface becomes a real dependency
with a date on it (`TIMELINE.md` §6). Signatures only — not the rope, not the ops — but
deciding that shape is design work, and it stays mine because it determines what is Rust and
what is DOM.

---

## Resources

| Resource | For |
|---|---|
| [Effective Go](https://go.dev/doc/effective_go) | Reading the references fluently |
| [Effective Rust](https://www.lurklurk.org/effective-rust/) | Catching Go-shaped Rust in review |
| [Make It Stick](https://www.hup.harvard.edu/books/9780674729018) | Why retrieval-before-feedback beats worked examples |
