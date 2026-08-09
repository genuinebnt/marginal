Review the current branch changes as a strict senior Rust engineer. Follow agents.md strict reviewer mode exactly.

---

## 1. Naming Conventions

- `snake_case` — files, variables, functions, modules
- `CamelCase` — structs, enums, traits
- `SCREAMING_SNAKE_CASE` — constants
- Meaningful lifetime names (`'src`, `'conn`) not single letters unless trivially short
- Flag every deviation and explain why it breaks idiom

---

## 2. Architecture Compliance

- `libs/domain` must have zero external dependencies — flag any `use sqlx`, `use redis`, `use axum` etc.
- Infrastructure layer implements domain traits; domain never imports infrastructure
- New external dependencies must be behind a trait — flag any direct concrete usage in domain or presentation
- Config changes must follow single `config.yaml` + env var override pattern (see `docs/architecture/CLOUD_PORTABILITY.md`)
- Every new service dependency must have a local Docker equivalent in `docker-compose.yml`

---

## 3. Rust Idioms

- Manual loops where an iterator adapter (`.map()`, `.filter()`, `.fold()`) would read more clearly
- Unnecessary `.clone()` — flag each one; explain whether it can be eliminated
- `String` parameters where `&str` or `impl AsRef<str>` suffices
- `Box<dyn Error>` in domain code — should be a typed `thiserror` error enum
- `match` blocks that only map errors — should use `.map_err()` or a `From` impl
- Missing `From`/`Into`/`TryFrom` impls where conversions are repeated manually

---

## 4. Performance

- Allocations in hot paths (inside loops, per-request, per-message)
- `Arc<Mutex<T>>` on read-heavy state — suggest `Arc<RwLock<T>>` or a channel
- `.clone()` on large types passed into async tasks — suggest `Arc` wrapping
- Repeated `.contains()` on `Vec` — suggest `HashSet` for O(1) lookup
- Missing `#[inline]` on small, frequently called trait methods

---

## 5. Security

- All sqlx queries must use parameterised form (`query!`, `query_as!`) — flag any string interpolation
- Secret comparisons must be constant-time — flag any `==` on tokens, passwords, or API keys
- New unauthenticated routes must have rate-limit middleware
- RBAC: every handler accessing workspace or page data must check membership and role first
- No secrets or PII in `tracing` span fields or log output

---

## 6. Tests

- New handlers need integration tests using `#[sqlx::test]` against a real database
- Async tests use `#[tokio::test]`
- Edge cases covered: empty input, invalid IDs, permission boundaries
- No `unwrap()` outside of tests; inside tests use `expect("reason")`

---

## 7. Documentation (agents.md requirement)

Flag if any of these need updating and are not updated in this branch:

- `docs/planning/FEATURE_LIST.md` — new feature or sub-task added?
- `docs/architecture/DATA_MODEL.md` — schema or ER diagram changed?
- `docs/api/` — new or modified endpoint (request/response/status codes)?
- `docs/architecture/adr/` — major architectural decision made?

---

Report findings grouped by severity: **blocking**, **recommended**, **minor**. For each finding include the file path and line number.
