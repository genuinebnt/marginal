Review the current branch changes for idiomatic Rust quality and simplicity. Fix issues directly. Add a one-line comment only where the reason is non-obvious — never explain what the code does.

---

## Iterator Adapters

Replace manual loops where an iterator adapter makes intent clearer:

- `for x in xs { result.push(f(x)) }` → `.map().collect()`
- Manual filter-then-collect → `.filter().collect()`
- Manual accumulation → `.fold()`, `.sum()`, or `.product()`
- `.iter().find()` + `unwrap_or` chains → `.iter().find_map()`

Do not replace loops where the imperative form is genuinely clearer (early returns with side effects, complex multi-step mutation).

---

## Unnecessary Allocations

- `.to_string()` or `.to_owned()` on a value that is immediately consumed → use `&str` or `impl AsRef<str>`
- `format!("{}", x)` where `x` implements `Display` and a borrow suffices
- `.clone()` on a value that is only read in the callee → pass a reference instead
- `vec![x]` constructed and immediately iterated once → use `std::iter::once(x)`

---

## Error Handling

- `unwrap()` or `expect()` outside tests → replace with `?` and the appropriate `thiserror` variant
- `match err { SomeVariant(e) => OtherError(e), ... }` → implement `From<SomeError> for OtherError` and use `?`
- `Box<dyn Error>` as a return type in domain code (`libs/domain`) → define a typed error enum with `thiserror`
- Infrastructure errors in domain return types → they belong in `libs/infra`, not `libs/domain`

---

## Type Conversions

- Repeated struct-to-struct field mapping → implement `From<SourceType> for TargetType`
- `String` parameters that are never mutated → change to `impl Into<String>` or `&str` depending on ownership needs
- Manual `if let Some(x) = opt { f(x) } else { default }` → `.map().unwrap_or()` or `.map_or()`

---

## Module & Function Size

- Functions over ~30 lines → look for a named helper that improves readability
- Files over ~300 lines → suggest a module split with a comment on where the boundary should be
- Structs with more than ~8 fields → consider a sub-struct or builder

---

## Naming (agents.md rules)

- `snake_case` — variables, functions, modules, files
- `CamelCase` — structs, enums, traits, type aliases
- `SCREAMING_SNAKE_CASE` — constants and statics
- Boolean variables and functions that return `bool`: prefer `is_`, `has_`, `can_` prefixes

Fix every deviation in-place.

---

## Performance Quick-Wins

- `Arc<Mutex<T>>` on a value that is read far more than written → `Arc<RwLock<T>>`
- `.clone()` on a large struct passed into an `async` task → wrap in `Arc` and clone the `Arc`
- Repeated `.contains()` on a `Vec` inside a loop → collect into a `HashSet` before the loop
- `HashMap::new()` in a hot path with known capacity → `HashMap::with_capacity(n)`

---

After all fixes: summarize what was changed and why, grouped by category. Keep it to one line per change.
