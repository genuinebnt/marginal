# LLD — `auth-service`

**Owns:** `auth` schema — users, refresh tokens, and later roles and preferences
**Transport:** gRPC `AuthService` (ADR-007). HTTP exists only for Kubernetes probes and the JWKS endpoint.
**Depends on:** PostgreSQL 18, Redis (revocation blocklist). No dependency on `document-service`.
**Related:** `DATA_MODEL.md` §3 (schema) · `ADR-007` (gRPC east-west) · `ui-mockups/signin.html` (the two screens) · `docs/learning/01-track1-mvp.md` § Phase 2 (reading list)

**Small service, high stakes.** Roughly 900 lines of Rust. Almost every decision has a known-correct
answer published by OWASP or an RFC, and the work is looking them up rather than inventing.

> **The rule for this phase: do not invent anything.** If a decision here is not traceable to
> OWASP, an RFC, or a cited paper, it is probably wrong. `learning/01-track1-mvp.md` § Phase 2 lists
> the six documents that contain every answer.

---

## 1. Scope — what is hand-written here

The startup path is **copied** from `document-service` — by then it exists — rather than designed
again. Re-deriving it teaches nothing on `ROADMAP.md` § Rust, DSA & Concepts Map.

| Copy from `document-service` | Designed for this service |
|---|---|
| `main.rs`, `lib.rs` (`serve`, pool, drain, both listeners) | `libs/proto` — `auth.proto` |
| `telemetry.rs` | `domain.rs` — `Email`, `Password`, `PasswordHash`, `UserId`, `Jti` |
| `routes.rs` (probe router + middleware) | `users/` slice |
| `health.rs` | `sessions/` slice — issue, refresh, revoke |
| `error.rs` shape (`AppError` → `ApiError`) | `keys/` — RS256 keypair, JWKS endpoint |
| `config.rs` pattern | `bootstrap/` — the first-run claim |

Copying rather than extracting is correct here: `PROJECT_STRUCTURE.md` §5 says duplicate on the
second use and extract on the third. This is the second use. **`libs/infra` gets extracted when
`collaboration-service` needs the same code — not before.**

---

## 2. Module map

```
services/auth-service/
├── config.yaml                  # port 8006, argon2 params, token lifetimes. NO secrets
├── migrations/
│   ├── 0001_users.sql           # auth.users            — DATA_MODEL.md §3
│   └── 0002_refresh_tokens.sql  # auth.refresh_tokens   — DATA_MODEL.md §3
└── src/
    ├── main.rs  lib.rs  config.rs  telemetry.rs  routes.rs  state.rs  error.rs   # copied
    ├── domain.rs                # newtypes + validation. Zero I/O, unit-testable
    ├── users/
    │   ├── mod.rs
    │   ├── model.rs             # User, NewUser, Credentials
    │   ├── repo.rs              # trait UserRepo + PostgresUserRepo (same file)
    │   └── grpc.rs              # Register, Authenticate, GetUser
    ├── sessions/
    │   ├── mod.rs
    │   ├── model.rs             # AccessToken, RefreshToken, TokenPair, Claims
    │   ├── repo.rs              # trait RefreshTokenRepo + Postgres impl
    │   ├── rotation.rs          # THE interesting file — reuse detection
    │   └── grpc.rs              # Refresh, Revoke, RevokeAll
    ├── keys/
    │   ├── mod.rs               # trait KeyStore + impls; RS256 keypair, `kid` selection
    │   └── jwks.rs              # GET /.well-known/jwks.json — the one public HTTP route
    └── bootstrap/
        └── mod.rs               # first-run claim: an instance with no users
```

**Why `sessions/` and not `tokens/`.** The slice owns the *lifecycle* of a login, not a data type.
`rotation.rs` earns its own file because it is the only place in this service with a non-obvious
algorithm, and `PROJECT_STRUCTURE.md` §5.3 allows a file when logic genuinely exists.

---

## 3. `domain.rs`

Zero external dependencies beyond `secrecy` and the hashing crate. Every type validates on
construction, so an invalid value cannot exist further in.

```rust
/// A syntactically valid email, lowercased and trimmed. Uniqueness is the
/// database's job, not this type's.
pub struct Email(String);
impl TryFrom<&str> for Email { type Error = DomainError; }

/// A plaintext password, in memory only. Wraps `secrecy::SecretString` so it
/// is not `Debug`-printable and is zeroed on drop.
/// NEVER derives Debug, Clone, Serialize, or Display.
pub struct Password(SecretString);
impl Password {
    /// Length and composition policy. OWASP: minimum 8, maximum >= 64 —
    /// a low maximum is a bug, because it prevents passphrases.
    pub fn parse(raw: &str) -> Result<Self, DomainError>;
}

/// A PHC-format string: `$argon2id$v=19$m=…,t=…,p=…$salt$hash`.
/// Carries algorithm, parameters, and salt together — which is why parameters
/// can be upgraded without a schema migration (DATA_MODEL.md §3).
pub struct PasswordHash(String);

pub struct UserId(Uuid);        // UUIDv7, generated in Rust — see document-service §12
pub struct DisplayName(String);
pub struct CursorColor(String); // assigned at signup so collaborators are distinguishable

/// The JWT ID claim. The blocklist key is derived from it, so it must be
/// unguessable — generate it, never derive it from the user id.
pub struct Jti(Uuid);
```

**`Password` must never be loggable.** The `secrecy` wrapper is not decoration: a `tracing`
field, a `#[derive(Debug)]` on an enclosing struct, or a panic message would otherwise put a
plaintext password in a log aggregator. That is the single highest-consequence mistake available
in this service.

---

## 4. `users/` slice

### `repo.rs` — `trait UserRepo` + `struct PostgresUserRepo`

Trait and impl in the same file (`PROJECT_STRUCTURE.md` §4).

```rust
#[async_trait]
pub trait UserRepo: Send + Sync {
    async fn insert(&self, tx: &mut Transaction<'_, Postgres>, user: NewUser) -> Result<User, AppError>;
    async fn find_by_email(&self, email: &Email) -> Result<Option<User>, AppError>;
    async fn find_by_id(&self, id: UserId) -> Result<Option<User>, AppError>;
    async fn count(&self) -> Result<i64, AppError>;   // bootstrap only
}
```

**`insert` takes a transaction**, for the same reason `document-service` does: registration and
the first refresh-token issue must commit together, or a user exists who cannot log in.

### `grpc.rs`

| RPC | Notes |
|---|---|
| `Register` | Gated by `bootstrap` state — see §7. Not open to the internet |
| `Authenticate` | Email + password → `TokenPair`. **Constant-time on the failure path**, §9 |
| `GetUser` | By id, for **off-hot-path** lookups only. Display name and cursor colour are needed on every page load and every presence update, so consumers **materialise them locally from `auth.user_updated` events** rather than calling here — `DATA_MODEL.md` §1 § Where a "join" happens |

`Authenticate` returns the same error for *unknown email* and *wrong password*, and takes
approximately the same time in both cases. §9 and §12 both cover why.

---

## 5. `sessions/` slice — the rotation chain

### The model

```rust
/// Short-lived, RS256-signed, NEVER stored server-side. The gateway verifies
/// it locally against the cached public key — no per-request RPC (ADR-007).
pub struct Claims {
    sub: UserId,
    jti: Jti,
    exp: i64,           // seconds since epoch
    iat: i64,
    nbf: i64,
    // `kid` lives in the JWT *header*, not here — see keys/
}

/// Long-lived and opaque. The database stores SHA-256 of it, never the token:
/// a database leak must not yield usable credentials (DATA_MODEL.md §3).
pub struct RefreshToken(SecretString);
```

### `repo.rs`

```rust
#[async_trait]
pub trait RefreshTokenRepo: Send + Sync {
    async fn insert(&self, tx: &mut Transaction<'_, Postgres>, row: NewRefreshToken) -> Result<(), AppError>;
    async fn find_active_by_hash(&self, hash: &[u8; 32]) -> Result<Option<StoredToken>, AppError>;
    /// Walks `parent_id` to the root and revokes every descendant. One statement,
    /// a recursive CTE — not a loop in Rust.
    async fn revoke_family(&self, tx: &mut Transaction<'_, Postgres>, any_id: Uuid) -> Result<u64, AppError>;
    async fn revoke_all_for_user(&self, tx: &mut Transaction<'_, Postgres>, user: UserId) -> Result<u64, AppError>;
}
```

### `rotation.rs` — the only real algorithm here

The rule, from the OAuth 2.0 Security BCP:

> **Every refresh consumes the old token and issues a new one. Presenting a token that has
> already been consumed means it was stolen — so revoke the entire chain, not just that token.**

```
   refresh(presented) →
     hash = sha256(presented)
     row  = find_active_by_hash(hash)

     ├─ None, and no row with that hash exists at all
     │     → Unauthenticated. Nothing to revoke; this is a bad token, not a theft signal
     │
     ├─ Some(row) where row.revoked_at IS NOT NULL      ← REUSE DETECTED
     │     → revoke_family(row.id); log at WARN with user_id
     │     → Unauthenticated
     │     Both the attacker and the victim are now logged out. That is correct:
     │     you cannot tell which one presented it, and the safe action is the same.
     │
     └─ Some(row), active and unexpired
           → in ONE transaction: revoke(row), insert(new with parent_id = row.id)
           → issue a fresh access token
```

**Reuse detection requires keeping revoked rows.** A `DELETE` on rotation would make the theft
signal indistinguishable from a bad token. Rows are swept by a job long after expiry, not on use.

---

## 6. `keys/` — RS256 and rotation

Asymmetric on purpose: the gateway verifies with a **public** key it caches, so a compromised
gateway cannot mint tokens. HS256 would require sharing the signing secret with every verifier.

```rust
#[async_trait]
pub trait KeyStore: Send + Sync {
    fn signing_key(&self) -> (&EncodingKey, &str);        // key + its `kid`
    fn verification_keys(&self) -> &[(String, DecodingKey)]; // ALL valid kids
}
```

**Rotation is why `verification_keys` is plural.** During a rotation two keys are valid: tokens
signed with the old `kid` must keep verifying until they expire. The sequence is

```
   1. publish new public key in JWKS   (verifiers now accept both)
   2. wait > access-token lifetime
   3. switch signing to the new key
   4. wait > access-token lifetime
   5. remove the old public key from JWKS
```

Steps 1 and 3 in the wrong order signs tokens nobody can verify. That ordering is the same
"consumers deploy before producers" rule as `ROADMAP.md` Phase 11.

`jwks.rs` serves `GET /.well-known/jwks.json` — the **only** public HTTP route on this service.
Everything else is gRPC.

---

## 7. `bootstrap/` — the first-run claim

`ui-mockups/signin.html` asserts that a fresh instance's first screen is **not** a login. An
instance with zero users offers to create the first administrator; after that, registration is
invitation-only (ADR-001 — self-hosted, not a public sign-up).

**This is a race, and it must be treated as one.** Two requests arriving at an empty instance
must not both succeed.

```
   claim_instance(email, password) →
     BEGIN
       SELECT pg_advisory_xact_lock(BOOTSTRAP_LOCK_KEY);
       IF (SELECT count(*) FROM auth.users) > 0 THEN
           → FailedPrecondition("instance already claimed")
       END IF;
       insert user; insert refresh token;
     COMMIT
```

A transaction-scoped advisory lock rather than a table lock: it releases on commit **or** on
crash, and it does not block reads. `count(*) = 0` checked *outside* a lock is a TOCTOU bug that
grants a second administrator to whoever arrives during the window.

---

## 8. Error mapping

Security changes the usual rules: **the client learns less than the log does, deliberately.**

| Domain condition | gRPC status | Client sees | Logged as |
|---|---|---|---|
| Unknown email | `UNAUTHENTICATED` | `"invalid credentials"` | `info` — no user id, it does not exist |
| Wrong password | `UNAUTHENTICATED` | `"invalid credentials"` — **byte-identical to the above** | `info` with `user_id` |
| Malformed email / weak password on register | `INVALID_ARGUMENT` | the specific rule that failed | not logged — caller's fault |
| Expired refresh token | `UNAUTHENTICATED` | `"session expired"` | `info` |
| **Refresh token reuse** | `UNAUTHENTICATED` | `"session expired"` — *the attacker learns nothing* | **`warn`** with `user_id` and family id |
| Rate limited | `RESOURCE_EXHAUSTED` | `"too many attempts"` | `info` |
| Instance already claimed | `FAILED_PRECONDITION` | `"instance already claimed"` | `warn` |
| Database / hashing failure | `INTERNAL` | `"internal error"`, no detail | `error`, detail on the span |

**The two `UNAUTHENTICATED` credential messages must be identical strings**, not merely similar.
A different message, a different error code, or a measurably different latency is a user-enumeration
oracle — an attacker learns which addresses have accounts.

---

## 9. Algorithms — named, not written

| Algorithm | Invariant that must hold | Reference |
|---|---|---|
| **Argon2id parameter choice** | Verification takes a target wall-clock time on *this* hardware (aim ~100–250 ms), and `m`, `t`, `p` are recorded in the PHC string so a future increase does not need a migration | [RFC 9106 §4](https://www.rfc-editor.org/rfc/rfc9106.html) · [OWASP Password Storage](https://cheatsheetseries.owasp.org/cheatsheets/Password_Storage_Cheat_Sheet.html) |
| **Constant-time credential failure** | Time to reject an *unknown email* is indistinguishable from time to reject a *wrong password*. Achieved by verifying against a **dummy hash** when no user is found — never by an early return | OWASP Authentication · §12 |
| **Timing-safe comparison** | Every comparison of a secret uses a constant-time primitive. A `==` on a token or hash leaks its prefix | [`subtle`](https://docs.rs/subtle/) |
| **Refresh rotation with reuse detection** | A consumed token can never be exchanged again; presenting one revokes the whole chain; a legitimate client is never logged out by its own rotation | OAuth 2.0 Security BCP |
| **Family revocation** | `revoke_family` reaches every descendant of the root in one statement, and is idempotent | Recursive CTE over `parent_id` |
| **Token hashing at rest** | The database never holds a usable credential. SHA-256 is correct here and Argon2 is *wrong* — a 256-bit random token has no entropy problem to solve, and per-refresh Argon2 would be a self-inflicted DoS | `DATA_MODEL.md` §3 |
| **JWT validation** | `exp`, `nbf`, `iat`, issuer, audience and **algorithm** are all checked. `alg` must be pinned to RS256 by the verifier, never read from the token | [RFC 7519 §11](https://www.rfc-editor.org/rfc/rfc7519) |
| **Revocation blocklist** | `jwt:blocklist:{jti}` TTL is **≥ the access token's remaining lifetime**. A shorter TTL un-revokes a token | `DATA_MODEL.md` §6 |
| **Key rotation overlap** | At every instant, every unexpired token's `kid` is present in JWKS | §6 |
| **Bootstrap mutual exclusion** | Exactly one administrator is created no matter how many concurrent claims arrive | `pg_advisory_xact_lock` |

**The `alg` pinning row is the classic JWT vulnerability**: a verifier that trusts the token's own
header accepts `alg: none`, or accepts an HS256 token signed with the *public* key as its secret.
The verifier decides the algorithm; the token does not get a vote.

---

## 10. Test map

```
tests/
├── common/mod.rs        # in-process tonic harness — same shape as document-service
├── domain.rs            # Email/Password/PasswordHash validation, and that Password is not Debug
├── register.rs          # bootstrap claim, the concurrent-claim race, invitation-only after
├── authenticate.rs      # success, wrong password, unknown email, and the TIMING property
├── rotation.rs          # happy rotation, reuse detection, family revocation, expiry
├── jwt.rs               # claim validation, alg pinning, kid selection, rotation overlap
└── revocation.rs        # blocklist TTL >= token lifetime, RevokeAll
```

Two tests deserve naming because they are the ones that catch real bugs:

| Test | What it asserts |
|---|---|
| `unknown_email_and_wrong_password_take_similar_time` | Statistical, not exact — many samples of both paths, then assert the medians are within a tolerance. **It must fail if the dummy-hash verification is removed.** Mark it `#[ignore]` in CI if it proves flaky on shared runners, but keep it runnable locally |
| `reuse_of_a_rotated_token_revokes_the_whole_family` | Rotate three times, replay token #1, then assert **all four** are revoked and a fresh refresh with #4 fails |

Database tests use `#[sqlx::test]`; Redis comes from Testcontainers. Never mock either
(`CLOUD_PORTABILITY.md` §4).

---

## 11. Build order

1. **`domain.rs`** — newtypes, `Password` non-`Debug`, PHC parse/format. Activate `domain.rs`. No database.
2. **`migrations/0001` + `0002`** — `auth.users`, `auth.refresh_tokens` per `DATA_MODEL.md` §3.
3. **`users/repo.rs`** — insert and `find_by_email` against real Postgres.
4. **Argon2 wiring + `Authenticate`** — including the dummy-hash path from the start. Activate `authenticate.rs`.
5. **`keys/`** — keypair loading, `kid`, JWKS. Activate `jwt.rs`.
6. **`sessions/` + `rotation.rs`** — issue, then rotate, then reuse detection. Activate `rotation.rs`.
7. **`bootstrap/`** — the advisory-lock claim. Activate `register.rs`.
8. **Redis blocklist** — `Revoke`, `RevokeAll`. Activate `revocation.rs`.
9. **Gateway integration** — the gateway caches JWKS and verifies locally. Phase 9 formalises it; Phase 2 needs the JWKS endpoint to exist and be correct.
10. Run `/project:security-review`. **Mandated by `CLAUDE.md` for any auth boundary** and this is the largest one in the project.

Step 4 before step 5 is deliberate: password verification is independently testable and does not
need signing keys, so it gives you a working login before JWT complexity arrives.

### 11.1 The cloud increment for this phase

| Terraform resource | Why |
|---|---|
| `google_secret_manager_secret` — RS256 private key, DB password | **Never in `config.yaml`, never in an image, never in a ConfigMap** (`CLOUD_PORTABILITY.md` §3) |
| `google_secret_manager_secret_version` | Rotation without a redeploy |
| `google_redis_instance` (Memorystore) | The blocklist. First Redis dependency in the project |
| **Per-service Postgres roles + grants** | **This phase is where schema isolation stops being a convention.** `auth_svc` granted on `auth` only, `docs_svc` on `docs` only — so `document-service` cannot read a password hash even by accident (`DATA_MODEL.md` §1 § The isolation must be a grant, not a convention). Migrate as owner, run as a restricted login role |
| `google_cloud_run_v2_service` — `auth-service` | Second service deployed |
| IAM binding: `auth-service` SA → `secretAccessor` on those secrets only | Least privilege, per secret rather than per project |

**The private key is the highest-value secret in the system.** It is generated out-of-band and
written to Secret Manager directly — it must never exist in the repository, in Terraform state as
plaintext, or in a CI log.

---

## 12. Implementation notes — the things that will bite

### Argon2 parameters are a latency budget, not a security dial

Higher `m`/`t` is more secure and slower. Verification happens on **every login**, synchronously,
and Argon2 is deliberately memory-hard — so `m = 64 MiB` with `p = 4` means a login spike is a
memory spike. Two consequences:

- **Benchmark on the deployment hardware**, not your laptop. Cloud Run vCPU is not an M-series core.
- **Run it on `spawn_blocking`.** Argon2 is CPU-bound for hundreds of milliseconds; on the async
  runtime it starves every other task on that worker (`ROADMAP.md` § Threads — the rayon/tokio trap
  in a different costume).

### The dummy hash must be a real hash

The constant-time failure path verifies the supplied password against a *pre-computed* Argon2 hash
of a fixed string, using the same parameters as real hashes. Two mistakes to avoid:

- Comparing against a literal that is not a valid PHC string — verification fails fast and the
  timing signal returns.
- Generating it per request — that costs a *hash* rather than a *verify* and the timings diverge
  in the other direction.

Compute it once at startup and hold it in state.

### A short `max_length` on passwords is a bug

OWASP requires supporting at least 64 characters. Argon2 has no practical input-length limit, so a
low cap serves nothing and blocks passphrases. **Do cap it** — somewhere around 128–256 — because
an unbounded input is a cheap DoS against a deliberately expensive function.

### Clock skew will reject valid tokens

`exp` and `nbf` are absolute times compared against *the verifier's* clock. Two pods a few seconds
apart will reject freshly minted tokens as `nbf` in the future. Configure a small leeway (30–60 s)
in the verifier, and never order anything by these timestamps — same reasoning as
`ROADMAP.md` § Distributed systems, *clock skew*.

### `jsonwebtoken`'s `Validation` defaults are a security decision you inherit

Read the struct field by field. Whether `aud` is validated by default, which algorithms are
accepted, and the default leeway are all things the library chose and you are responsible for.
**Construct `Validation` explicitly rather than using `Default`** so the choice is visible in a
review.

### The refresh token belongs in an `HttpOnly` cookie, and that means CSRF

If the SPA holds the refresh token in JavaScript, XSS steals it. If it is an `HttpOnly` cookie, the
browser attaches it automatically — which is CSRF. Pick one and mitigate the other:
`SameSite=Strict` plus a double-submit token for the cookie path. **There is no option without a
trade-off**, and the gateway (Phase 9) is where it lands.

### Revoking on the blocklist is not revoking the refresh token

They are separate: the blocklist stops an *access* token before its `exp`; revoking the family
stops future refreshes. **Logout must do both.** Doing only the second leaves a valid access token
for up to its full lifetime.

### `count(*) = 0` outside a lock is a TOCTOU bug

Covered in §7, repeated here because it is the kind of check that looks obviously correct. Two
concurrent claims on a fresh instance both read zero and both insert an administrator.

### Rate limiting belongs at the gateway, but the counter belongs here

Per-IP limiting at the edge is Phase 9. **Per-account lockout is this service's concern** — an
attacker distributing attempts across many IPs defeats edge limiting entirely. Track failures per
`user_id` in Redis with a backoff, and make sure a locked account returns the *same* generic
message so lockout state is not itself an enumeration oracle.