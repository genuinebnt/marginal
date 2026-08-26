# Open Questions — product decisions, independent of language

Carried forward from `rust/TASKS.md`'s open-decisions table. Some of that
table is already resolved by simply following the RFCs/DATA_MODEL directly
in the Go implementation (see below); these are the ones still genuinely
open.

| # | Question | Status |
|---|---|---|
| Soft or hard delete for pages | Blocks the page-delete step of the MVP (auth/collaboration don't need it yet). Still open. | Open |
| Anchor representation (`RFC-001` §9) | Needed once character-granular ops (Phase 3 / collaboration-service) land — block-granular Phase 1 ops don't touch it. Still open. | Open |

## Resolved by fiat (not re-litigated)

These were open in the Rust attempt because the deleted code disagreed with
the RFCs/DATA_MODEL. The Go implementation just follows the spec, so
there's nothing left to decide:

- `BlockId`/`PageId` are `Uuid` (v7 where generated), never a bare integer —
  `DATA_MODEL.md` already specifies this; the deleted Rust draft's `u64` was
  the thing that was wrong, not the spec.
- `Op` variant names and field shapes follow `RFC-002` §2's ISA exactly
  (`DeleteBlock{id, tombstone}`, `SetBlockKind{from,to}`,
  `SetBlockContent{block,content,prev}`), not the deleted draft's
  `UpdateBlockKind`/`UpdateBlockContent` naming.
- `Heading{level}` is validated at construction (rejects out-of-range
  levels) — no bare `u8` escape hatch.
