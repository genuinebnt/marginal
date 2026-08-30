# API — Auth

**Status:** Implemented in Go (`services/auth-service`) — all six RPCs,
account lockout, refresh rotation with reuse detection, JWKS. See
`docs/porting/PROGRESS.md` for what's still deferred (key rotation
tooling, per-IP rate limiting, the gateway/cookie boundary).
**Owners:** `auth-service` (gRPC `AuthService` + the one public HTTP route,
JWKS) · `api-gateway` (REST translation, deferred — out of this repo's scope)
**Related:** `docs/architecture/lld/auth-service.md` — the full design this
contract implements, written for the original Rust track but normative
regardless of language, **except §7's bootstrap-claim model — reversed,
see below and `docs/porting/PROGRESS.md`**; `DATA_MODEL.md` §3 (schema);
`ADR-007` (gRPC east-west). `docs/architecture/adr/ADR-001`'s
"self-hosted, invitation-only, no public sign-up" clause no longer holds
for `Register` specifically — everything else in that ADR (no
multi-tenancy, one shared workspace) is unchanged.

This doc fills the gaps the LLD explicitly left open (exact RPC/message
shapes, token lifetimes, Argon2id parameters) and translates its Rust-shaped
decisions (types, error taxonomy) into the Go equivalents this repo
actually uses. Nothing here contradicts the LLD — where they'd conflict,
the LLD's *invariants* win; only the language-specific shape differs.

---

## 1. `AuthService` — the gRPC contract

```protobuf
syntax = "proto3";
package marginal.auth.v1;

service AuthService {
  rpc Register     (RegisterRequest)     returns (TokenPair);
  rpc Authenticate  (AuthenticateRequest) returns (TokenPair);
  rpc GetUser       (GetUserRequest)      returns (User);
  rpc Refresh        (RefreshRequest)     returns (TokenPair);
  rpc Revoke          (RevokeRequest)     returns (google.protobuf.Empty);
  rpc RevokeAll       (RevokeAllRequest)  returns (google.protobuf.Empty);
}

message User {
  string id           = 1; // UUIDv7
  string email        = 2;
  string display_name = 3;
  string cursor_color = 4;
  google.protobuf.Timestamp created_at = 5;
}

message TokenPair {
  string access_token  = 1; // RS256 JWT
  string refresh_token = 2; // opaque, random — never a JWT
  int64 expires_in     = 3; // access token lifetime in seconds, from issuance
}

message RegisterRequest {
  string email        = 1;
  string password     = 2;
  string display_name = 3;
}
message AuthenticateRequest { string email = 1; string password = 2; }
message GetUserRequest      { string id = 1; }
message RefreshRequest      { string refresh_token = 1; }
message RevokeRequest       {
  string refresh_token        = 1;
  optional string access_token = 2; // present for a full logout — see below
}
message RevokeAllRequest    {} // the caller's own sessions only — see § actor identity
```

**`Register` is ordinary, repeatable signup** — reversed from LLD §7's
original bootstrap-claim design (there, the first successful call created
a sole administrator and every later call failed `FAILED_PRECONDITION`).
Real multi-user use (several distinct people, each with their own
account, editing together — the actual Track 1 🏁) needs a way for more
than one person to obtain an account without an operator running a
side-channel tool for each one; there is still no invitation/RBAC flow
(ADR-009, deferred past this track) to gate it with, so the pragmatic
choice is what this heading says: `Register` just registers. Every
account has identical standing — there is no "administrator" role
distinguishing the first account from the rest, and no per-workspace
isolation either (`ADR-001`'s "not multi-tenant" still holds: every
account shares the one page space). Uniqueness is still enforced —
`AlreadyExists` if the email is already registered (see the status table
below).

**`Revoke`'s `access_token` is optional but matters.** Revoking the refresh
token chain stops *future* refreshes; it does nothing to the access token
the client is currently holding, which stays valid for up to its own
(short) remaining lifetime unless it's also blocklisted (LLD §12: "Revoking
on the blocklist is not revoking the refresh token... logout must do
both"). A caller that has the access token on hand (a real logout) should
send it so its `jti` gets blocklisted immediately; omitting it still
revokes the refresh chain, which is enough for cleaning up a stale session
from another device that isn't presenting an access token at all.

**`RevokeAll` takes no fields.** Its target is always the calling user's own
sessions — same principle as `document-service`'s `created_by`: a field
naming *whose* sessions to revoke would let a caller revoke a stranger's
session. See § Actor identity below for how "calling user" is determined
in this repo (no gateway yet).

**`GetUser` is off-hot-path only** (LLD §4) — display name and cursor color
belong on every page load and presence update, so real consumers should
materialize them from `auth.user_updated` events (not built in this repo's
scope) rather than calling this per-request. It exists for occasional
lookups, not the hot path.

### Actor identity (temporary, until a gateway exists)

Same stand-in `document-service` uses (`docs/api/pages.md`): `Refresh` and
`Revoke` authenticate via the token itself (presenting a valid refresh
token/its hash *is* the credential). `RevokeAll` reads the caller's user id
from `actor-id` gRPC metadata, rejecting its absence with `UNAUTHENTICATED`.
This is scaffolding, not the real trust boundary — replace it, don't build
on it, once `api-gateway` exists and issues verified access tokens as the
actual source of `actor-id`.

### Status codes (LLD §8 — reproduced here as the authoritative table)

| Situation | gRPC status | Message the client sees | Logged as |
|---|---|---|---|
| Unknown email | `UNAUTHENTICATED` | `"invalid credentials"` | `info` — no user id, it doesn't exist |
| Wrong password | `UNAUTHENTICATED` | `"invalid credentials"` — **byte-identical to the above** | `info` with user id |
| Malformed email / weak password on register | `INVALID_ARGUMENT` | the specific rule that failed | not logged — caller's fault |
| Expired refresh token | `UNAUTHENTICATED` | `"session expired"` | `info` |
| **Refresh token reuse** | `UNAUTHENTICATED` | `"session expired"` — attacker learns nothing | **`warn`**, with user id and family id |
| Email already registered | `ALREADY_EXISTS` | `"email already registered"` | not logged — caller's fault |
| Database / hashing failure | `INTERNAL` | `"internal error"`, no detail | `error` |

**The two `UNAUTHENTICATED` credential messages must be byte-identical, at
indistinguishable latency.** A different message, code, or a measurably
different response time is a user-enumeration oracle. This repo does not
yet implement per-IP or per-account rate limiting (`RESOURCE_EXHAUSTED`,
LLD §12) — deferred; see `docs/porting/PROGRESS.md`.

---

## 2. Gateway REST mapping

Same status as `pages.md`'s §2 — the projection a browser actually calls,
implemented as a thin REST↔gRPC shim since no full `api-gateway` exists in
this repo's scope (`ADR-011`). Semantics (idempotency, the actor-identity
stand-in, the two-`UNAUTHENTICATED`-messages rule) belong to §1 and are
inherited here, not restated.

| Method | Path | RPC |
|---|---|---|
| `POST` | `/auth/register` | `Register` |
| `POST` | `/auth/login` | `Authenticate` |
| `GET` | `/auth/users/{id}` | `GetUser` |
| `POST` | `/auth/refresh` | `Refresh` |
| `POST` | `/auth/revoke` | `Revoke` |
| `POST` | `/auth/revoke-all` | `RevokeAll` |
| `GET` | `/admin/people` | `ListPeople` |
| `GET` | `/admin/auth-events` | `ListAuthEvents` |

`/admin/people` is under `/admin` rather than `/auth` because it
is a workspace view, not an authentication operation — § 18
ADMIN's PEOPLE panel and its SIGNED IN readout, which arrive in
one response because the screen shows them together and two
round trips would let them disagree on screen.

```json
{
  "people": [
    { "id": "01a048f4-…", "email": "ui-demo@example.com",
      "display_name": "Genuine", "cursor_color": "#1971C2",
      "created_at": "2026-08-28T15:19:35.278136Z" }
  ],
  "active_sessions": 324
}
```

`active_sessions` counts refresh tokens that are neither revoked
nor expired — "signed in somewhere". It is deliberately **not** a
count of live WebSocket connections, which is
`collaboration-service`'s number, nor of pages with a live rope,
which is a third. "Sessions" means three things in this system;
every surface that shows one says which.

Unpaginated and unfiltered: a self-hosted instance's people list
is short by construction, and a cursor API nobody can exercise is
worse than none. No `password_hash` is selected — a column that
never leaves the repository layer cannot leak from a screen.

**Not authorization-gated, and that is a gap rather than a
design.** RBAC is `v3.1.0` (`RELEASES.md`), so until it exists
every authenticated actor can read this list. § 18 states that on
screen rather than implying an admin surface that is actually
open.

### `/admin/auth-events`

§ 18b AUDIT LOG's auth half, derived the same way the content
half is: **nothing here is emitted.** A user row *is* the
registration, a refresh-token row *is* a sign-in, and its
`revoked_at` *is* a sign-out. None of it can disagree with what
happened, because none of it is a second copy of anything.

```json
{
  "events": [
    { "id": "7553b303-…", "kind": "auth.signin",
      "user_id": "01a048f4-…", "at": "2026-08-30T14:20:04.247Z" }
  ]
}
```

**`limit` applies to sign-ins and sign-outs only — registrations
are always all of them.** An account being created is the most
audit-worthy auth event there is, and a shared limit lets a few
hundred routine sign-ins push every registration off the end,
leaving the log showing only the noise. So the returned count
can exceed `limit` by the number of people, which on a
self-hosted instance is a number that fits.

**Failed sign-in attempts are absent because nothing records
them.** § 18b says so on screen rather than letting an empty
column read as "there were none". So is the request source: the
op log records an actor *kind*, not where a request came from,
and the SOURCE column shows the former rather than inventing the
latter.

`/auth/revoke-all`'s actor identity comes from the same `X-Actor-Id` header
stand-in `pages.md`'s gateway shim reads (§ Actor identity above) — there
is no session cookie or verified-token source to read it from instead
until a real `api-gateway` exists.

```json
{
  "access_token": "eyJhbGciOi...",
  "refresh_token": "b64-opaque-random",
  "expires_in": 900
}
```

`GetUser`'s response omits `password_hash` (never leaves `auth-service` at
all, on any transport) and reports timestamps as RFC 3339 UTC, same
convention as `pages.md`.

### Status translation

Same table as `pages.md` §2, plus the one code that table doesn't need but
this one does constantly:

| gRPC | HTTP | `error` code | Retryable |
|---|---|---|---|
| `UNAUTHENTICATED` | 401 | `unauthenticated` | No — bad credentials, an expired/reused refresh token, or a missing actor-id stand-in header. **The response body must be byte-identical for "unknown email" and "wrong password"** (§1's status table) |
| `INVALID_ARGUMENT` | 422 | `validation_failed` | No — fix the request |
| `NOT_FOUND` | 404 | `not_found` | No |
| `FAILED_PRECONDITION` | 409 | `conflict` | No — e.g. "instance already claimed" |
| `INTERNAL`, `UNKNOWN` | 500 | `internal_error` | Yes, with backoff |
| `UNAVAILABLE` | 503 | `unavailable` | Yes, with backoff |
| `DEADLINE_EXCEEDED` | 504 | `timeout` | Yes, with backoff |

Same one error shape as `pages.md`: `{ "error": "unauthenticated", "message": "invalid credentials" }`.

---

## 3. Gaps the LLD left open, decided here

Two kinds of gap: security-engineering parameters (grounded in OWASP/RFCs,
same as the LLD's own rule — not product-specific) and product-facing
behavior (grounded in what a real collaborative document editor — Notion,
Google Docs — actually does, since that's the product this is).

**Security-engineering parameters:**

| Decision | Value | Why |
|---|---|---|
| Argon2id parameters | `m=19456 KiB (19 MiB)`, `t=2`, `p=1`, `keyLen=32` | OWASP Password Storage Cheat Sheet's second-tier recommendation — lighter than the 64 MiB/p=4 first tier, chosen because `CLOUD_ROADMAP.md`'s Cloud Run targets are small, cost-bounded instances where a memory-hard function competing with the request pool under concurrent logins is a real risk. Re-benchmark on real deployment hardware before shipping past a demo (LLD §12) |
| Clock-skew leeway | 60 seconds | LLD §12's suggested 30–60s range, upper end |
| Password length | 8–128 bytes, **no composition rules** | OWASP: enforce length, not character-class rules — composition requirements push users toward predictable patterns and are no longer recommended. A cap exists only to bound Argon2's input cost, never below OWASP's 64-char minimum |
| Blocklist key | `jwt:blocklist:{jti}` | Matches `DATA_MODEL.md` §6 exactly |
| Account lockout | 5 failed attempts → lockout, backoff 1 / 5 / 15 / 30 min (caps at 30) | OWASP Authentication Cheat Sheet's exponential-backoff guidance; counter and lockout state keyed per `user_id` in Redis (LLD §12 — "the counter belongs here", not the gateway). A locked account returns the *same* `"invalid credentials"` used for wrong-password, so lockout state isn't itself an enumeration oracle |
| JWKS response shape | RFC 7517 (JSON Web Key Set) exactly | Not a product decision — it's the standard every JWT verifier expects; inventing a shape here would just break interop for no reason |

**Product-facing behavior (what a document editor does):**

| Decision | Value | Why |
|---|---|---|
| Access token lifetime | 15 minutes (900s) | Short-lived by design (limits the blocklist's exposure window) with silent refresh masking it from the user — the pattern Google Docs/Notion-style apps use so a short token never means a visible re-login |
| Refresh token lifetime | 30 days | Matches the "stay logged in across restarts" story (A-02) the way Notion/Google Workspace do: a trusted browser stays signed in for weeks without prompting, until explicit logout or the rotation chain is broken |
| Cursor color palette | 8 fixed, visually-distinct hues (see `internal/domain`'s `CursorPalette`), assigned at registration by `hash(user_id) mod 8` | Matches the collaborative-cursor convention every multiplayer editor (Google Docs, Figma, Notion) uses: a small fixed palette, not an arbitrary color, so two collaborators' cursors are never confusingly similar. Deterministic on `user_id` rather than random so it's stable across sessions without needing to store a separate column beyond what `DATA_MODEL.md` §3 already has |

## 4. Deferred, not built in this repo yet

- Per-IP rate limiting (LLD §12 calls this the gateway's job, Phase 9) —
  no gateway exists in this repo's scope. Per-account lockout (this
  service's own job) **is** implemented — see table above.
- Key rotation tooling (LLD §6's five-step sequence) — the `KeyStore`
  abstraction supports multiple verification keys, but nothing yet drives
  an actual rotation; one signing key exists for the life of the process.
- `api-gateway` and its cookie/CSRF handling (LLD §12) — refresh tokens are
  returned directly in `TokenPair` for now, not set as an `HttpOnly`
  cookie, since there's no gateway/browser boundary in this repo yet.
