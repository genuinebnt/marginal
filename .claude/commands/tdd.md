Write TDD-style test cases for the feature or function described. I will write the production code to make them pass. Do not write the production implementation.

---

## Instructions

1. Read the feature description from the conversation context (or the argument passed to this command).
2. Identify the correct test harness for the layer being tested:
   - **Repository / DB layer:** `#[sqlx::test]` — real Postgres DB per test, no Docker needed
   - **Async service / use-case layer:** `#[tokio::test]`
   - **Pure domain logic:** `#[test]` (synchronous, no runtime)
   - **HTTP handler / integration:** `axum::test` with `TestContext` from `libs/test-utils`
3. Write test cases that:
   - Cover the **happy path** first
   - Cover **error cases** (invalid input, not found, permission denied, duplicate)
   - Cover **edge cases** specific to this feature (empty collections, boundary values, concurrent access if relevant)
4. Each test must:
   - Have a descriptive name following the pattern `test_<what>_<condition>_<expected_outcome>`
   - Assert the exact shape of the return value, not just `is_ok()`
   - Use `expect("reason")` rather than `unwrap()` so failures are readable
5. If the feature involves a new repository trait, write the tests against the **trait**, not the concrete Postgres impl — this keeps tests decoupled from the DB implementation detail.

---

## Output Format

Group tests by scenario:

```
// --- Happy path ---
#[test_harness]
async fn test_... { ... }

// --- Error cases ---
#[test_harness]
async fn test_... { ... }

// --- Edge cases ---
#[test_harness]
async fn test_... { ... }
```

Include any necessary fixture builders or helper functions above the test cases. Use `todo!()` as a placeholder in any production function signature you need to reference — do not implement it.

After the test block, add a one-line summary of what I need to implement to make each group pass.

---

## Marginal-specific conventions

- IDs are newtypes from `libs/domain` — use `UserId::new()`, `PageId::new()`, etc.
- Errors use `thiserror` enums — assert on the specific variant, not just `is_err()`
- `TestContext` from `libs/test-utils` wires `PgPool`, `RedisPool`, and `NatsClient` for full integration tests
- Repository traits live in `libs/domain` — the test imports the trait, not the Postgres impl directly
