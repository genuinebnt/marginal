# Porting Guide — how the future Rust port should approach this codebase

This is written now, while the Go+TS MVP is being built, so the approach is
decided once rather than reconstructed later. The actual port happens in a
new, separate repo (`ADR-011`) — this guide is what that repo's first
sessions should read.

## Order of operations, per module

1. **Read the spec first, not the Go/TS code.** `RFC-001`, `RFC-002`,
   `DATA_MODEL.md`, and `docs/api/` are language-agnostic and didn't change
   for this build — they're still the actual spec.
2. **Read the golden test vectors** (`testdata/<module>/*.json`) — this is
   the behavior contract, expressed as data. A Rust implementation that
   passes every vector for a module is behaviorally equivalent to the Go/TS
   one, regardless of internal shape.
3. **Read the Go/TS implementation third**, as a worked reference — not a
   template to transliterate. Watch for `// PORT-NOTE:` comments; each one
   marks a place where Go/TS leaned on garbage collection to avoid a
   decision Rust will force (an allocation pattern, an ownership choice, a
   lifetime). Those are exactly the spots ADR-005 originally worried a Go
   reference would have "nothing to port for" — they're the real Rust work
   in each module.
4. **Write the Rust idiomatically**, not shaped to match Go's structure.
   Small interfaces become traits where Rust's type system wants them, not
   because Go had an interface there. `(T, error)` becomes `Result<T, E>`
   with a real `thiserror` taxonomy, not a mechanical rename.
5. **Run the same golden vectors against the Rust implementation** before
   trusting it's equivalent.

## What ports cleanly vs. what doesn't

**Ports directly:** service/module boundaries (`ARCHITECTURE.md`,
`DATA_MODEL.md`), the op ISA and its invertibility law, the gRPC/OpenAPI
contracts, the golden test vectors, the database schema.

**Needs real Rust design work, not a mechanical port** (flagged inline with
`// PORT-NOTE:` as they come up): anything currently relying on GC for
memory management the way `crossbeam-epoch`/arenas/`MaybeUninit`/
`repr(align)` would in Rust; `Arc<RwLock<T>>` vs. channel-based ownership
choices where Go just shares a pointer across goroutines.

## Status

Empty until Phase C lands — updated as `document-core` and later modules
are implemented. See `docs/porting/PROGRESS.md` for the current state.
