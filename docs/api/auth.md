# API — Auth

**Status:** Implemented in Go (`services/auth-service`) — all six RPCs,
account lockout, refresh rotation with reuse detection, JWKS. See
`docs/porting/PROGRESS.md` for what's still deferred (key rotation
tooling, per-IP rate limiting, the gateway/cookie boundary).
**Owners:** `auth-service` (gRPC `AuthService` + the one public HTTP route,
JWKS) · `api-gateway` (REST translation, deferred — out of this repo's scope)
**Related:** `docs/architecture/lld/auth-service.md` — the full design this
contract implements, written for the original Rust track but normative
regardless of language; `DATA_MODEL.md` §3 (schema); `ADR-007` (gRPC
east-west); `docs/architecture/adr/ADR-001` (self-hosted, invitation-only
after bootstrap, no public sign-up)

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

**`Register` is also the bootstrap claim** (LLD §7) — there is no separate
RPC. The first successful call, on an instance with zero users, creates the
sole administrator under a `pg_advisory_xact_lock`-guarded transaction.
Every call after that fails `FAILED_PRECONDITION` ("instance already
claimed") — there is no invitation flow in this repo's scope (RBAC/invites
are ADR-009, deferred past this track), so **`Register` is single-use for
the lifetime of an instance**, not a general sign-up endpoint.

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
| Instance already claimed | `FAILED_PRECONDITION` | `"instance already claimed"` | `warn` |
| Database / hashing failure | `INTERNAL` | `"internal error"`, no detail | `error` |

**The two `UNAUTHENTICATED` credential messages must be byte-identical, at
indistinguishable latency.** A different message, code, or a measurably
different response time is a user-enumeration oracle. This repo does not
yet implement per-IP or per-account rate limiting (`RESOURCE_EXHAUSTED`,
LLD §12) — deferred; see `docs/porting/PROGRESS.md`.

---

## 2. Gaps the LLD left open, decided here

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

## 3. Deferred, not built in this repo yet

- Per-IP rate limiting (LLD §12 calls this the gateway's job, Phase 9) —
  no gateway exists in this repo's scope. Per-account lockout (this
  service's own job) **is** implemented — see table above.
- Key rotation tooling (LLD §6's five-step sequence) — the `KeyStore`
  abstraction supports multiple verification keys, but nothing yet drives
  an actual rotation; one signing key exists for the life of the process.
- `api-gateway` and its cookie/CSRF handling (LLD §12) — refresh tokens are
  returned directly in `TokenPair` for now, not set as an `HttpOnly`
  cookie, since there's no gateway/browser boundary in this repo yet.
