# Benchmarks

Baselines recorded here as each module lands, for comparison once the Rust
port exists. Go version and hardware are recorded per entry — both affect
absolute numbers enough to matter for a later comparison.

## `documentcore` — 2026-08-26 (updated during a regression pass)

Apple M4 Pro, Go 1.26.1, `cd services/documentcore && go test ./... -bench=. -benchmem -run=^$ -benchtime=5000x`:

| Benchmark | ns/op | B/op | allocs/op |
|---|---|---|---|
| `PageApplyInsertBlock` | ~910–960 | 112 | 2 |
| `PageApplySetBlockContent` | ~125–130 | 192 | 3 |
| `HistoryUndoRedo` (one undo + one redo) | ~205 | 384 | 6 |

**`PageApplyInsertBlock` was re-measured and its benchmark rewritten this
pass — the old numbers are not comparable to these.** The previous version
inserted a block every iteration and never removed one, so the page it
measured against grew without bound across the whole `b.N` run. Since
`InsertBlock`/`indexOf` are a linear scan over `Page.Blocks` (a deliberate,
simple choice at this repo's scale, not a bug), the reported `ns/op` was
really "total scan-and-insert work across a linearly-growing page, divided
by however many iterations `-benchtime` happened to pick" — not a stable
per-op cost. Concretely: the old benchmark reported ~772–928ns/op at
`-benchtime=100x` (page stays small: ~100 blocks) but ~95,000–127,000ns/op
at `-benchtime=1s` (page grows to ~200,000 blocks) — a ~100x spread that's
the tell. Fixed by pre-populating a realistic, *constant* 200-block page
(matching `PageApplySetBlockContent`'s own convention) and having each timed
iteration insert after a fixed reference block, then immediately apply the
op's own `Invert()` **outside the timer** to undo it before the next
iteration — so the page's size, and therefore the op's true cost, stays
fixed across the whole run regardless of `-benchtime`. Verified stable
across `-benchtime=100x` (~1775ns, noisy — few samples), `5000x`
(~910–960ns), and a full `-benchtime=1s` run (~1.38M iterations, 872.5ns) —
all now the same order of magnitude, not two orders apart.

No Rust numbers to compare against yet — the port happens after the MVP
ships (`ADR-011`). These exist so that comparison has a Go-side baseline
already measured under identical conditions when the time comes.

## `collaboration-service`'s `rope` — 2026-08-26 (re-verified during a regression pass)

Apple M4 Pro, Go 1.26.1, `cd services/collaboration-service && go test ./internal/rope/... -bench=. -benchmem -run=^$ -benchtime=200x`,
inserting one character into the middle of a ~900KB document. `Rope` is
immutable (`Insert`/`Delete` return a new value rather than mutating), so —
unlike `documentcore`'s benchmark above — nothing here was growing
unboundedly; re-verified, not rewritten:

| Benchmark | ns/op | B/op | allocs/op |
|---|---|---|---|
| `RopeInsertMiddle` | ~102–117 | 192 | 3 |
| `NaiveStringInsertMiddle` (slice + concat) | ~66,000–71,700 | 901,150 | 1 |
| `RopeStringManySequentialInserts` (1000 sequential appends from empty, amortized per insert) | ~12,130–12,150 | ~42,825 | ~511 |

**~600–700x** faster than the naive string for a single middle-insert on a
document this size — the whole point of using a rope instead of `string`
slicing, and the concrete number this project can compare against once the
Rust port has its own rope.

**The `RopeStringManySequentialInserts` row above corrects a arithmetic
mistake in the original entry**, found while re-verifying these numbers for
this pass: that benchmark reports one `ns/op`/`allocs/op` per *outer*
iteration (a whole 1000-insert run building a rope from empty), so getting
a genuine "per insert" figure means dividing the raw output by 1000 — the
original entry's `B/op` division was done correctly (~42.8K/insert, matches
exactly) but its `ns/op` and `allocs/op` divisions were not (it claimed
~61ns and ~2.5 allocs per insert; the actual raw output, divided by 1000
the same way, is ~12,130ns and ~511 allocs per insert — roughly 200x off in
both cases, which is what let it go unnoticed: the bytes-per-insert figure,
the one someone would sanity-check against "not much per character," still
looked plausible). The corrected number is unremarked-on scope-wise for
this pass: whether ~511 allocations per insert on a 1000-character rope
is *itself* worth optimizing in `rope`'s implementation is a separate
question this regression pass didn't investigate — this entry only fixes
the measurement and its arithmetic, not the rope's own allocation behavior.
