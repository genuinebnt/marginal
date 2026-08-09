Run a security review of the current branch changes against Marginal's threat model. For each finding, show how the attack occurs, then link to a prevention resource (per agents.md mentor rules — do not just list findings).

---

## Auth & Sessions

- **Passwords:** Argon2id only — flag any MD5, SHA-*, or bcrypt usage; link to Argon2id parameter tuning guide
- **JWT:** RS256 asymmetric signing; verify `exp`, `iat`, `sub` claims are validated on every decode; HS256 is a blocker
- **Refresh tokens:** Single-use with family revocation on reuse detection; rotating without revocation is a blocker
- **Token blocklist:** Redis entries must have TTL matching token expiry; missing TTL = tokens live forever after logout
- **OAuth2:** PKCE required on every flow — flag implicit grants or auth-code flows without `code_challenge`

---

## Timing Attacks

- Password comparison must use `argon2::verify_password` (constant-time internally) — flag any `==` or `!=` on password strings
- Token/API key comparison must use `subtle::ConstantTimeEq` — flag any standard equality on secret bytes
- Login and register error messages must be identical for "user not found" vs "wrong password" — username enumeration is a medium severity finding

---

## RBAC & Permissions

- Every handler that reads or writes workspace-scoped data must check workspace membership and role first
- Page-level permission checks must go through the full resolution chain (page → parent → workspace) — shortcuts are a blocker
- Guest tokens must be scoped to a specific page, single-use-expiry enforced, and checked against the permission resolution chain
- Permission escalation: a user must not be able to grant themselves or others a role higher than their own

---

## SQL & Data

- All queries must use sqlx parameterised form (`query!`, `query_as!`, `query_scalar!`) — any string interpolation into SQL is a blocker
- LTREE path inputs from user-supplied data must be validated (alphanumeric + dots only) before use in queries
- Mass assignment: raw JSON deserialized directly into an INSERT or UPDATE struct is a blocker — domain types must validate first
- `JSONB` content stored from user input must be validated against a known schema before persistence

---

## File Handling

- Presigned URL generation: bucket and object key must be validated against the workspace scope — cross-workspace access is a blocker
- Path traversal: file keys must reject `../`, URL-encoded variants (`%2e%2e`), and null bytes
- Content-type must be validated server-side against an allowlist — do not trust the `Content-Type` header from the client

---

## Rate Limiting

- All unauthenticated endpoints must be behind the Tower rate-limit layer
- Auth endpoints (login, register, token refresh, OAuth callback) need per-IP limits with a stricter budget than general API routes
- Rate limit state must be in Redis (not in-process memory) so it holds across restarts and multiple instances

---

## Secrets & Config

- No secrets in `config.yaml`, git history, or `tracing` span fields
- `.env` file must not be staged — verify `.gitignore` covers it
- `tracing` spans must not log: passwords, tokens, API keys, full email addresses, or any PII field
- `Settings` struct deserialization must fail fast on missing required secrets at startup — no silent fallbacks to empty strings

---

Report each finding with:
- **Severity:** critical / high / medium / low
- **Location:** file path and line number
- **Attack vector:** one sentence on how this gets exploited
- **Resource:** a link to the relevant prevention docs or CVE
