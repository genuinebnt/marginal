# Porting Progress Log

Read this first in any new or compacted session — it's the record of what's
actually done, not a summary to re-derive from memory. Append short entries
as work lands. Don't let this balloon into a second `rust/TASKS.md`.

---

## 2026-08-26 — Pivot to Go+TS MVP first

Decided (see `ADR-011`): build the Track 1 MVP (Documents → Auth →
Collaboration) completely in Go + TypeScript, Claude writing it directly,
before any Rust work. The Rust hand-port happens later, in a new separate
repo. Reasoning: ADR-005's three objections to this shape (deletes DSA
objective, GC has nothing to port for the hardest Rust content, reading a
port is slower than designing one) are accepted as real costs, traded for a
complete, demo-quality product on a nearer timeline.

Repo reorg done: `crates/document-core` and the root Cargo workspace deleted
(not archived — RFC-001/RFC-002/DATA_MODEL.md are the spec now, not the
deleted draft, which had known-wrong shapes its own open-decisions list
already flagged). Rust-mentor-mode docs (`.agents/agents.md`,
`docs/learning/`, `docs/planning/TASKS.md`) moved to `rust/` as a doc-only
waypoint. New top-level `.agents/agents.md` written for direct Go/TS
implementation.

Scope for the MVP: standalone skeleton code areas for all three Track 1
services (`document-service`, `auth-service`, `collaboration-service`) now;
real logic lands one at a time, starting with `document-service`'s
`document-core`. Complexity budget is feature depth, not route/service
count — see `.agents/agents.md` §3.

**Next:** scaffold `go/` and `web/` workspaces (Phase B), then implement
`document-core` in both (Phase C) — see `PORTING_GUIDE.md`.
