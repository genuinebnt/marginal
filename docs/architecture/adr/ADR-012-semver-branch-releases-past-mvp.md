# ADR-012 — SemVer Branch Releases Past the MVP; Rust Port Happens Major-by-Major

**Date:** 2026-08-27
**Status:** Accepted
**Amends:** ADR-011 (§ Decision: "Extra features (Tracks 2–6) are built on top only
after that port, in the new repo's own time" — reversed below)
**Related:** ADR-001 (scope), ADR-009 (knowledge-platform expansion), `ROADMAP.md`
**Deciders:** @genuinebasilnt

---

## Context

`v1.0.0` (Track 1 — Documents → Auth → Collaboration, the 🏁) is done, tagged as its
own branch, and demoable end to end. ADR-011 planned to stop building product features
here the moment that happened: hand-port the MVP to Rust in a new, separate repo, and
build Tracks 2–6 only after that port, in that future repo's own time.

That plan assumed the MVP was *the* deliverable worth porting, once, in full. The actual
goal is narrower and more useful: **`v1.0.0`'s own size and shape — three substantial,
independently-complete phases (Documents, Auth, Collaboration) — is exactly the unit of
work a single Go→Rust porting pass can absorb.** Continuing to build past the MVP is
worth doing in this repo, in Go+TS, precisely *because* each further major version can be
cut to that same size and completeness — a self-contained, MVP-sized chunk, portable on
its own — rather than accumulating into one undifferentiated pile that only makes sense
to port as a single, much larger effort at the very end. Versioning by major/minor here
is a porting-granularity decision as much as a release-planning one.

## Decision

**Marginal keeps building past `v1.0.0`, in this repo, in Go+TS, following SemVer.**

- **Major version (`vX.0.0`)** — a milestone: a themed group of features that together
  cross-load into "the product is qualitatively different now" (`v1` = "a demo exists
  at all"; `v2`/`v3`/`v4` below are each their own such claim).
- **Minor version (`vX.Y.0`)** — one feature, built completely: backend and UI, real
  end-to-end, usable from a browser the same way `v1.0.0`'s three phases already are.
  Nothing ships half-wired — no feature lands with a backend and no screen, or a screen
  with a stubbed backend, except where a doc already states that gap on purpose (the
  same standard `v1.0.0`'s own `InspectorRail` tabs hold themselves to). This bar exists
  for a second reason beyond "shippable": **the TypeScript/HTML/CSS frontend is not part
  of what gets ported to Rust, and never will be — it's the permanent visual verification
  harness.** Only the Go services get replaced, one major version at a time; the same
  browser UI is pointed at whichever backend (the Go original or the in-progress Rust
  port) so the user can compare behavior side by side. A minor with a half-built UI gives
  the future porting pass nothing to visually check itself against.
- **The acceptance bar for `v2`–`v4` is the full `docs/ui-mockups/` set, not just the
  notebook-editing screens.** Eleven of those seventeen mockups run a real algorithm
  client-side today (force-directed layout + exact Voronoi, graph BFS/DFS/flood-fill,
  HNSW, HyperLogLog/Count-Min/t-digest, LCS DP, a dependency DAG, op apply/invert,
  OT+Merkle+DAG+LSM views) — `docs/planning/RELEASES.md` maps every one of them onto a
  minor via `ROADMAP.md` § Mockup Coverage's existing phase assignments, so none of them
  are silently dropped just because they read as "algorithm demo" rather than "product
  feature."
- **Business logic behind those algorithms is Go, the same rule `documentcore` already
  follows, not a second implementation in TypeScript.** Graph algorithms, HNSW, the
  sketches, the LCS table, the dependency DAG, Merkle comparison, the LSM-shaped log —
  each computed in Go (server-side, or compiled to wasm via the same `GOOS=js
  GOARCH=wasm` boundary when it must run against live client-side state), with
  TypeScript only drawing what Go computed. This is what gives the eventual Rust port
  real learning weight per `ADR-011`: this is the algorithmic depth that gets hand-ported,
  major by major, while the view layer never moves. Reimplementing one of these
  algorithms twice — once in Go, once in TS "for the demo" — does the porting work
  backwards and is a review finding, not a style nit.
- **Patch version (`vX.Y.Z`)** — *not* a required cadence. Bumped only if a real issue
  surfaces after a minor has shipped that doesn't warrant a new minor of its own; most
  fixes just land inside whichever minor is currently active. There is no obligation to
  produce patches, and no minor is required to have one.

**Branching:** each minor gets its own branch, cut from the previous minor once that one
merged to `master` (or from `v1.0.0` for the first one). The branch is the unit of work
for one feature; it merges back to `master` — and `master` gets tagged — only once that
feature is complete end-to-end per the minor-version bar above.

**Release notes, kept in two places:** the merge commit message for a minor states what
shipped and, briefly, why (the same "why, not just what" bar this repo's other commit
messages already hold); `docs/planning/RELEASES.md` (new, this ADR) is the running,
skimmable table of every version, its theme, and its status — the roadmap `ROADMAP.md`'s
own Track 2–6 tables fed into, but re-cut into shippable, browser-usable slices instead
of Rust/DSA-density-ordered phases.

**Scope for `v2`–`v4`:** drawn from `ROADMAP.md`'s Tracks 2–5 (Track 6, cloud hardening,
was never "a track at the end" per that doc's own words — it stays continuous,
patch-level work woven into whichever minor needs it, not its own minor). `docs/planning/RELEASES.md`
has the full breakdown, targeting the *full* RFC-001 §10 grammar, not a subset of it —
the one carve-out (`Table`/`CommTable` and the cross-page query kinds) gets its own
minor (`v4.5.0`) gated on writing the ADR that resolves cross-page aggregation ownership,
rather than being left permanently unscoped. `CLAUDE.md`'s remaining "Still Out" items —
a formula language, a spatial canvas, mobile apps — are **not** in `v2`–`v4`; they still
need their own ADR before they're scoped at all, per that document's own rule, unchanged
by this one.

**The Rust port happens major version by major version, not once at the very end.**
ADR-011's plan was a single porting pass, right after the MVP. The actual intent is
narrower and recurring: each major version here (`v1`, then `v2`, `v3`, `v4`) is sized
and scoped to be its own complete, self-contained porting unit — the same size class as
the MVP itself — so the user can port `v1.0.0`'s branch to Rust in one pass, then come
back and continue this repo's Go+TS work on `v2`, port that increment once it ships, and
so on. This is *why* a major version's minors need to add up to roughly MVP-scale
completeness rather than being an arbitrary bucket — a major that's too thin isn't worth
its own porting pass, and one that's too sprawling stops being one. Everything ADR-011
said about *how* each porting pass should work — a separate repo, design docs
(RFCs/`DATA_MODEL.md`) unchanged and authoritative, golden JSON test vectors and recorded
benchmarks as the concrete artifacts a hand-port checks itself against — still holds,
applied per major version instead of once. ADR-002 (Rust depth as primary objective)
stays suspended for this repo's Go+TS branches; it re-applies in full to each porting
pass in the future Rust repo, the same as ADR-011 already said.

## Why this doesn't repeat what almost killed the original 13-service scope

ADR-009 named the real risk plainly: an ordering-free feature list is what stalled the
first attempt at this scope. The guard here is the same kind of guard, adapted to
branches instead of phases: **one minor is one branch, done completely, merged, before
the next starts.** There is no "half of three features in flight," because a minor isn't
declared done — isn't merged, isn't tagged — until its UI is real and its backend is
real, the same bar `v1.0.0` already had to clear three times over (Documents, Auth,
Collaboration) before it could call itself done.

## Consequences

- `docs/planning/RELEASES.md` is the new source of truth for "what version are we on,
  what's next" — read alongside `docs/porting/PROGRESS.md` (which stays the session-level
  "what actually landed" log; `RELEASES.md` is the version-level plan the sessions work
  against).
- `CLAUDE.md`'s "Objective & Order" and "Out of Scope" sections need updating to stop
  saying Tracks 2–6 are deferred to a future repo — they are `v2`–`v4` of this one now.
  `ROADMAP.md` gets a pointer at its own top, the same way it already points to `ADR-011`.
- Every future minor's own branch does the same documentation-rules check `CLAUDE.md`
  already requires before code: `DATA_MODEL.md`, `docs/api/`, the RFCs, and an ADR if
  the feature itself is architecturally significant enough to need one (most won't;
  `v2`–`v4`'s features were mostly already designed in `ROADMAP.md`'s Track 2–5
  descriptions, just not yet re-cut into this repo's Go+TS shape).

## Resources

Same as `ADR-011`'s: this decision changes *when* the port happens and *what* gets
built here first, not how the port itself should be approached once it starts.
