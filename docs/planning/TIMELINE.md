# Marginal — Timeline

**Status:** Estimate. Not a commitment, not a schedule, and not a thing to feel behind.
**Basis:** ~2 hours a day, ~12 effective hours a week.
**Related:** `ROADMAP.md` (what gets built and why) · `ADR-002` (Rust depth is the objective)

`ROADMAP.md` answers *what* and *in what order*. This answers *roughly when*, and it exists
for one reason: to make the two handoffs visible early enough to plan around.

---

## 1. How to use a plan you are not going to follow

Every estimate here will be wrong. The useful question is not "am I behind" but **"which
assumption broke"** — so §2 lists them explicitly. When a band slips, find the assumption
that failed and re-derive from there rather than sliding everything right by a month.

Two rules that make the slip cheap:

- **Nothing here reorders `ROADMAP.md` § Execution Order.** Dependencies are dependencies.
  If a band slips, the *order* is unchanged; only the dates move.
- **Depth is not the variable.** ADR-002 makes Rust depth the objective, so the way to
  absorb a slip is to cut scope from a later track, never to rush a phase into being
  shallow. A hurried CRDT teaches nothing and demos worse than no CRDT.

---

## 2. Assumptions

Each of these is falsifiable, and each one failing has a different fix.

| # | Assumption | If it breaks |
|---|---|---|
| 1 | ~12 effective hours a week | Everything stretches proportionally. Divide, do not panic |
| 2 | A 2-hour session yields ~1.5 hours of work on hard code | Longer sessions on fewer days beats short daily ones for the CRDT |
| 3 | Fractional indexing takes ~16h, not ~8h | It is the first genuinely subtle thing. See § Risks |
| 4 | The frontend never blocks the backend | Holds by construction — see §3 |
| 5 | Phase 3 is ~130h of Rust regardless of help | The floor. Nothing parallelises it away |
| 6 | The handoffs in §6 land roughly on time | Each one stalls the frontend completely, not partially |

---

## 3. Division of labour

Agreed 2026-08-07. Recorded in ADR-005 § Amendment, because it changes who writes what.

| | Owner |
|---|---|
| All Rust — services, `libs/`, **and the `wasm32` editor core** | You |
| The TypeScript SPA in `web/` — shell, routing, panels, DOM plumbing, API client | Me |

**The line does not move.** `agents.md` forbids reimplementing the document model,
diagnostics, or highlighting in JavaScript. Where the SPA needs a document operation and the
`wasm-bindgen` binding does not exist yet, the call is **stubbed**, never written in
TypeScript. Crossing that line once quietly deletes Phase 3.

The consequence for this timeline: frontend work is **off the critical path**. It proceeds
in parallel, is not limited to 2h/day, and finishes ahead of the backend it talks to.

---

## 4. The bands

Weeks are relative to starting `domain.rs`, not calendar dates.

| Weeks | You — Rust | Me — TypeScript | Coupling |
|---|---|---|---|
| **1–3** | `domain.rs`: newtypes, `TryFrom`, **fractional indexing**, LTREE paths. Activate `domain.rs` and `fractional_index.rs` | `web/` scaffold, design tokens from `mockup.css`, app shell, both themes | none |
| **4** | **Thin gateway (~6h)** — Phase 0's ~150-line proxy + JWT verification | Mock API against `docs/api/pages.md` §2, then swap to the real one | ⚠ handoff |
| **4–9** | `libs/proto`, migration 0002, `pages` repo + gRPC, `tree`, `blocks`, outbox write | Page tree with drag-reorder, inspector panels, reader chrome, search, admin | parallel |
| **9–10** | Cloud Run deploy, Terraform, Secret Manager, budget alert | Empty states, error handling, loading, polish | parallel |
| **11–13** | Auth: Argon2id, RS256, refresh rotation, Redis blocklist, first-run claim | Sign-in, first-run, session handling, protected routes | parallel |
| **14** | **`wasm-bindgen` API surface (~10h)** — signatures only | Editor shell built against the stubs | ⚠ handoff |
| **14–26** | Rope, op ISA, CRDT convergence, WAL, WebSocket session loop, `loom` + Miri + fuzz | `contenteditable` plumbing, presence, live cursors, offline queue | coupled |

---

## 5. Three milestones that matter

### Week 10 — demoable

A deployed web application: page tree, editing, search, admin console, on a real URL.
Not "a gRPC service you can `curl`".

This is the number the division of labour actually changed. Building the SPA serially would
have put this at week 24.

### Week 12 — interview-ready

Add auth and the deploy is a system rather than an endpoint. Combined with the ADRs, RFCs,
and a test suite written before the implementation, this is a complete and honest story —
**including the CRDT, which you describe from RFC-002 as designed and in progress.**

An interviewer spends ten minutes. They ask: does it run, is the code good, can you defend a
hard decision. All three are answerable at week 12.

### Week 26 — Track 1 complete

Live multiplayer. The demo. Treat it as a stretch goal that makes the story spectacular if
it lands and costs nothing if it does not, because week 12 already stands on its own.

---

## 6. The two handoffs

Everything else is independent. These two are not.

| Handoff | ~When | Size | Cost of slipping |
|---|---|---|---|
| **Thin gateway** | Week 4 | ~6h | The SPA stays on mocks. Recoverable, but every screen built after it is unverified against a real response |
| **`wasm-bindgen` signatures** | Week 14 | ~10h | The editor shell cannot be finished. **Total stall on the hardest frontend work** |

The second is the one to protect. It is *signatures only* — not the rope, not the ops, just
the exported surface: build a document from blocks, apply a keystroke and get ops back, apply
a remote op, query selection, produce render state. Deciding that shape is design work and it
is yours, because it determines what is Rust and what is DOM.

---

## 7. What would blow this

**Fractional indexing, weeks 1–3.** The first genuinely subtle problem, and the
`COLLATE "C"` trap means a wrong answer looks correct locally and silently reorders pages
in production. Budget 16h. If it takes 30, that is normal and not a signal about the rest.

**CRDT convergence, weeks 14–26.** The hardest debugging in the project. A month here is a
month on everything after it. This is the single largest source of variance, and it is why
week 12 rather than week 26 is the milestone worth planning around.

**Scope.** ADR-009 brought eight phases into scope with guard rails saying none of them start
before Track 1 ships. Those guard rails are what keep this document from being fiction.

---

## 8. When to redo this

Re-estimate after **Phase 1 ships**, not before. At that point you have a real measurement —
how long a phase actually took at your real cadence — and every number here can be scaled by
one honest ratio instead of guessed again.

Do not re-estimate weekly. The estimate is not the work.
