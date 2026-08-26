# Benchmarks

Baselines recorded here as each module lands, for comparison once the Rust
port exists. Reproduce with:

```
cd services/document-service && go test ./internal/... -bench=. -benchmem -run=^$
```

## `document-core` — 2026-08-26

Apple M4 Pro, `go test ./internal/... -bench=. -benchmem -run=^$ -benchtime=100x`:

| Benchmark | ns/op | B/op | allocs/op |
|---|---|---|---|
| `PageApplyInsertBlock` | 771.7 | 369 | 3 |
| `PageApplySetBlockContent` | 427.1 | 192 | 3 |
| `HistoryUndoRedo` (one undo + one redo) | 389.6 | 384 | 6 |

No Rust numbers to compare against yet — the port happens after the MVP
ships (`ADR-011`). These exist so that comparison has a Go-side baseline
already measured under identical conditions when the time comes.

## `collaboration-service`'s `rope` — 2026-08-26

Apple M4 Pro, `go test ./internal/rope/... -bench=. -benchmem -run=^$ -benchtime=200x`,
inserting one character into the middle of a ~900KB document:

| Benchmark | ns/op | B/op | allocs/op |
|---|---|---|---|
| `RopeInsertMiddle` | 112.9 | 192 | 3 |
| `NaiveStringInsertMiddle` (slice + concat) | 79,167 | 901,151 | 1 |
| `RopeStringManySequentialInserts` (1000 appends, amortized) | ~61/insert | ~42.8K/insert | ~2.5/insert |

**~700x** faster than the naive string for a single middle-insert on a
document this size — the whole point of using a rope instead of `string`
slicing, and the concrete number this project can compare against once
the Rust port has its own rope.
