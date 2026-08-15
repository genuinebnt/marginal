# ADR-002 — Rust Depth as the Primary Objective

**Date:** 2026-08-06
**Status:** Accepted
**Related:** ADR-001 (scope), ADR-005 (Go reference), ADR-004 (React SPA)
**Deciders:** @genuinebasilnt

---

## Context

Marginal is a learning project. The goals are Rust, microservice architecture, system design, distributed systems, backend, security, cloud/IaC, DSA, DevOps, and data modelling.

Those goals conflict when sequencing work. The earlier roadmap was ordered by job-market coverage, which put the richest Rust phase behind roughly a dozen phases of CRUD that repeated Rust already learned. Progress stalled there.

The objective is now explicitly ranked: **really good Rust learning is primary.** Everything else remains a goal, but Rust depth wins any tie.

---

## Decision

### 1. Order work by Rust density, not by feature dependency alone

Every phase carries a density label, and the roadmap is sequenced accordingly — subject to the MVP constraint below.

**●●● deep** · **●●○ real but familiar** · **●○○ repeats earlier work** · **○○○ no Rust**

### 2. But ship an MVP first

Depth-first ordering taken literally produces subsystems nobody can look at. A working product is the substrate the other goals live on: microservices, cloud, and distributed systems only exist in a running system.

So the order is **MVP → novel subsystems → distributed systems → cloud**, with the MVP track ending on the *deepest* phase (collaboration/CRDT) so shipping early costs no depth.

### 3. Push work toward Rust when there is a real choice

Where a feature could live on either side of a boundary, it goes in Rust:

- **The editor core is Rust compiled to `wasm32`** — document model, rope, position-anchored marks, selection mapping, transform application. React is a view that receives model state and emits intents (ADR-004).
- Encryption, diagnostics analysis, and search indexing stay in Rust rather than being reimplemented in TypeScript for convenience.

**Rejected:** canvas-based text layout. It forfeits screen-reader support, browser find-in-page, and native selection conventions, then reimplements each — and teaches text layout, which is not a goal.

### 4. `unsafe` and concurrent code require tooling, not just tests

Absent from the earlier plan entirely, and not optional:

| Tool | Where | Why |
|---|---|---|
| **[`loom`](https://docs.rs/loom)** | Collaboration | Model-checks thread interleavings. Lock-free code with a hand-chosen `Ordering` that has not been loom-tested is code that happens to work on one machine |
| **[Miri](https://github.com/rust-lang/miri)** | Collaboration, editor core | UB detection under `MaybeUninit`, `repr(align)`, epoch reclamation |
| **[`cargo-fuzz`](https://rust-fuzz.github.io/book/)** | Input rules, paste, diagnostics | Stronger than `proptest` for never-panic properties over adversarial input |
| **[`syn`](https://docs.rs/syn) + [`quote`](https://docs.rs/quote)** | Foundation | `#[derive(...)]` proc macros are a distinct skill from `macro_rules!` |
| **Hand-written `Future`/`Stream`** | Collaboration | `poll`, `Pin`, `Waker` by hand. The op-buffer flush is the natural home |

Keep `unsafe` data structures in `crates/` with pure unit tests so they stay Miri-reachable — Miri cannot run testcontainers.

> **Read [Rust Atomics and Locks (Mara Bos)](https://marabos.nl/atomics/) before the collaboration phase.** Memory ordering is the one topic where guessing produces code that passes every test and is still wrong. `SeqCst` everywhere is not a plan.

---

## Consequences

### What this costs

**Feature breadth.** The MVP has four services, not seven. Search, history, and diagnostics arrive after live editing works.

**Job-market breadth is no longer the sequencing input.** The earlier roadmap was derived from a skills-gap analysis; that document has been removed because it produced the ordering that stalled. Breadth still arrives — it is simply not what decides what comes next.

### What this protects

Handlers and repositories are **not** plumbing to be outsourced. They are where `async_trait`, `Arc<dyn Trait>` vs `impl Trait`, sqlx type mapping, extractor lifetimes, and `From` error-conversion chains are actually learned. ADR-005's Go reference is narrowed accordingly.

### Interleaving is allowed

Some work has no infrastructure dependencies and can be picked up whenever momentum stalls elsewhere — the diagnostics analyzers, the rope, and the input-rule scanner are all pure-library work testable with `cargo test` alone.

---

## Alternatives Considered

| Alternative | Why not |
|---|---|
| Order by feature dependency only | Produces the CRUD-first sequence that stalled |
| Pure depth-first, no MVP | Builds subsystems nobody can see; the product never becomes real, so the cloud and microservice goals never get exercised |
| Order by job-market coverage | This is what the removed skills analysis did |
| Abandon the product, do pure Rust exercises | Loses microservices, distributed systems, and cloud — which need a real system to exist in |
