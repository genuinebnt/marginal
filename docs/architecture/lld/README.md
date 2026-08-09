# Low-Level Design

One document per service, written **before** the code and specific enough that implementation is
typing rather than deciding. `document-service.md` is the reference; every other LLD follows its
shape.

**One exception: [`libs-doc.md`](libs-doc.md).** `libs/doc` is a crate rather than a deployable, but
it has **three consumers across four phases** — which is precisely the shape that falls between
per-service documents and ends up unspecified. It got its own LLD because it did.

> **An LLD says what must be built, never what has been built.** Types, invariants, error mappings,
> algorithm contracts, build order, and the traps that will bite — all forward-looking. It carries no
> status, no progress, and no checkboxes; that is what `ROADMAP.md` § Status is for, and duplicating
> it here guarantees the two disagree.
>
> It also contains no implementation bodies. §9 of every LLD is titled *"Algorithms — named, not
> written"* and that title is the rule (`agents.md` § stage 2).

---

## The template

Deviating is fine when a service genuinely differs; renumbering is not, because tests and other
docs cite these section numbers.

| § | Title | Contains |
|---|---|---|
| — | Front matter | Owns · Transport · Depends on · Related docs |
| **1** | Scope — what is hand-written here | A two-column table. The left column is scaffolding and off-limits; the right is the design. Every right-hand row must teach something on `ROADMAP.md` § Rust, DSA & Concepts Map, or it should be a dependency rather than hand-written code |
| **2** | Module map | Feature-first slices per `PROJECT_STRUCTURE.md` §2, drawn as a tree with one line per file explaining what it owns |
| **3–7** | Per-module contracts | Types, trait signatures, and the invariant each protects. Signatures only — no bodies |
| **8** | Error mapping | Domain error → transport status → what is logged and at which level. **A database message must never reach a client** |
| **9** | Algorithms — named, not written | A table: algorithm · invariant that must hold · reference. This is where the depth goes, and it is deliberately not code |
| **10** | Test map | Which test file covers what, and what must exist before each compiles |
| **11** | Build order | Bottom-up, each step making the next step's tests compile. Plus **11.1 The cloud increment** — the phase is not done when tests pass, it is done when it is deployed (`CLOUD_ROADMAP.md` §2) |
| **12** | Implementation notes — the things that will bite | The traps. Version skew, collation, type gaps, footguns. **Written from research, not from having hit them** |

### What makes §12 worth writing

`document-service.md` §12 records seven traps — `COLLATE "C"`, LTREE label restrictions, sqlx
having no LTREE type, `IS NOT DISTINCT FROM` for nullable parents, `cargo sqlx prepare`, the
transaction boundary belonging to the outbox, and never omitting the id on insert.

None of those are discoverable from the happy path, and each costs an afternoon. **A §12 that is
empty means the design has not been thought through, not that the service is simple.**

---

## Index

| Phase | Service | Document |
|---|---|---|
| **1, 3** | **`libs/doc`** — *a crate, not a service* | [`libs-doc.md`](libs-doc.md) |
| **1** | `document-service` | [`document-service.md`](document-service.md) |
| **2** | `auth-service` | [`auth-service.md`](auth-service.md) |
| **3** | `collaboration-service` | [`collaboration-service.md`](collaboration-service.md) |
| 4 | `diagnostics-service` | Write at the start of Phase 4 |
| 5 | undo/redo | **Extends `collaboration-service.md` §7** — no new document |
| 6 | `history-service` | Write at the start of Phase 6 |
| 7 | `search-service` | Write at the start of Phase 7 |
| 8 | page-delete saga | Spans four services — a cross-service design, not an LLD. See `ARCHITECTURE.md` §5 |
| 9 | `api-gateway` | Write at the start of Phase 9 |
| 10 | session routing | **Extends `collaboration-service.md` and the gateway LLD** — no new document |
| 11–12 | infrastructure | Not a service. See `CLOUD_ROADMAP.md` |
| 15, 17, 18, 19 | notification · publishing · plugin · assistant | Gated on the 🏁 (ADR-009 § Guard Rails) |

### Why most of these are written later, on purpose

ADR-009 § Guard Rails is binding: **nothing in Tracks 4–5 starts before the MVP ships.** Writing
their LLDs now would be designing against requirements that the MVP will change, and a stale LLD is
worse than no LLD — it looks authoritative. They get written in the week before their phase starts,
which is also when the reading list in [`docs/learning/`](../../learning/README.md) says to start.

The same rule applies to Phases 4, 6, 7 and 9: **the LLD is the first deliverable of the phase, not
a prerequisite written a year early.** Phase 3's exists ahead of time only because it is the
acknowledged cliff — the phase where "what do I even build" was a real question.

---

## Maintenance rule

An LLD is the contract the tests are written against, so it moves before the code does.

1. **Schema change** → update the LLD *and* `DATA_MODEL.md`. They must not disagree; `DATA_MODEL.md` is authoritative for DDL, the LLD is authoritative for how the service uses it.
2. **A new algorithm** → §9 gets a row, with its invariant. A row with no invariant is a wish.
3. **A trap you hit that §12 did not predict** → add it. §12 is the only section that legitimately grows during implementation.
4. **A phase is renumbered, split, or cut** → the LLD follows, and the index above changes with it. Same rule as `docs/learning/` and `ROADMAP.md` § Mockup Coverage.

> **If the code and the LLD disagree, that is a bug in one of them — decide which, then fix that
> one.** Silently letting the code win is how a design document becomes archaeology.