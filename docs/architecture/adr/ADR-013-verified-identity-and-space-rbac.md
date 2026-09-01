# ADR-013 — Verified Identity First, Then Space-Scoped RBAC

**Date:** 2026-09-01
**Status:** Accepted
**Related:** RFC-002 §5 (`can_apply`, the one authorization chokepoint), ADR-001
(service boundaries), ADR-007 (gRPC east-west), `docs/api/auth.md`,
`docs/api/collaboration.md`, `RELEASES.md` (`v3.1.0`)
**Deciders:** @genuinebasilnt

---

## Context

`v3.1.0` is "Identity, Spaces & RBAC — users beyond one shared pool, spaces,
roles, invitations, and real permission enforcement inside `can_apply(op,
actor)`". Starting it surfaced a prerequisite that the plan did not name.

**Identity is currently asserted by the client and verified by nobody.** Every
service learns who you are from a value you wrote yourself:

- REST, via `api-gateway`: the `X-Actor-Id` request header
  (`internal/actorctx`'s own doc comment calls it "the temporary actor-identity
  stand-in… until real auth exists").
- The WebSocket, which is **not proxied** and is reached directly: the
  `?actor_id=<uuid>` query parameter (`docs/api/collaboration.md` §1, recorded
  there as "the browser's query-param actor-auth workaround" — a browser cannot
  set headers on a `WebSocket` handshake).

`auth-service` issues real JWTs and the SPA stores them, but **no service
verifies one**. The token is decoded in the browser to read its `sub` claim,
and that claim is then sent as a plain string that any client can change to any
other user's id.

This is fine at the scope it was written for — a single shared demo pool, where
`can_apply` returns `true` unconditionally and there is nothing to escalate
*to*. It stops being fine the moment roles exist: a role check against an
unverified identity is not an authorization system, it is a suggestion. Adding
spaces and roles on top of a claimed identity would produce a screen that
*says* "viewer" while the same person can edit anything by changing one string.

There is a second, structural fact. The two services that must enforce
permissions do not share a request path: `document-service` is reached through
`api-gateway`, and `collaboration-service`'s WebSocket is reached **directly**
by design (a persistent connection is not a request/response resource). Any
scheme where the gateway resolves permissions and passes the answer downstream
is therefore bypassable by connecting to the socket instead — which is exactly
the path that carries every mutation.

## Decision

### 1. Identity is verified from the token, at every entry point, before RBAC exists

`v3.1.0` ships in two parts, and the first is a prerequisite rather than a
feature: **the actor id comes from a verified token's `sub` claim, and from
nowhere else.**

- `api-gateway` verifies the `Authorization: Bearer` token and derives the
  actor id from it. `X-Actor-Id` is **ignored on input**, not merely
  deprioritised — a header that is read "only when the token is absent" is the
  same hole with an extra step.
- `collaboration-service` verifies the token on the WebSocket handshake,
  carried in the **`Sec-WebSocket-Protocol`** header as
  `bearer, <access token>`. `?actor_id=` is removed.

  The browser's `WebSocket` constructor cannot set arbitrary headers, but it
  *can* set this one — `new WebSocket(url, ["bearer", token])` — which is why
  it is the conventional place to put a credential on a socket. The token
  therefore never enters the URL, and so never reaches an access log, a
  `Referer`, or browser history.
- Verification is local: `auth-service` signs, everyone else verifies against
  the public key. No service asks `auth-service` "is this token good" on the
  request path, because that turns every keystroke into a second network hop
  and makes `auth-service` a hard dependency of editing.

Until this lands, **no role check is worth writing**, and `can_apply` stays as
it is: honest about allowing everything, rather than dishonest about enforcing
something.

### 2. The space is the permission boundary; a page belongs to exactly one

Roles do not attach to pages. A **space** owns pages; a **membership** binds
one user to one space with one role. A page's permissions are its space's,
which is what makes the check a single lookup rather than a walk up the page
tree — and what stops "who can read this page" from depending on where somebody
last dragged it.

Three roles, and deliberately only three:

| Role | Can |
|---|---|
| `viewer` | read pages in the space |
| `editor` | everything `viewer` can, plus emit any op |
| `admin` | everything `editor` can, plus manage membership and delete the space |

A fourth role is a product decision, not a technical one, and this repo has no
requirement that needs one. Adding one later is a row in a table and an arm in
a `switch`; adding one now is a guess.

### 3. The role is resolved once per WebSocket join, not once per op

`can_apply` runs on **every op** — every keystroke, in a session that already
holds a rope in memory precisely to avoid per-keystroke I/O. Resolving a role
there would put a database round trip inside the hot path of typing.

So the role is resolved **at join**, from the verified identity, and held on
the subscriber for the life of the connection. `can_apply` then reads a value
that is already in memory, and stays the pure, synchronous, auditable function
RFC-002 §5 describes.

**This trades freshness for latency, and the trade must be stated rather than
discovered.** A role revoked mid-session stays in effect on that connection
until it closes. That window is bounded three ways:

1. `auth-service` publishes `auth.role_revoked` (a topic `DATA_MODEL.md` §10
   already reserves). `collaboration-service` consumes it and **closes the
   affected subscribers**, which forces a rejoin and therefore a re-resolve.
   The window is then a NATS delivery, not a session lifetime.
2. Core NATS has no redelivery (see `internal/notify`'s doc comment), so
   delivery is best-effort. A subscriber therefore also re-resolves on a
   **TTL** — 5 minutes — so a dropped event costs at most that.
3. The window is on **writes to one page by an already-connected session**,
   never on reads, which go through the gateway and are checked per request.

Stating the bound is the point. "Revocation is immediate" would be a claim this
architecture cannot honour, and the screen will say what it actually does.

### 4. `can_apply` stays the only write-authorization path

No second check, anywhere. `document-service` enforces **read** scope on its
own queries (a page outside your spaces is not in your result set — filtered in
SQL, not after), and `collaboration-service` enforces **write** through
`can_apply`. Those are two different questions, not two paths to the same one.

The gateway does **not** enforce. It authenticates — it turns a token into a
verified actor id — and passes that down. Enforcement lives with the service
that owns the data, because the socket proves the gateway cannot be the only
place a request arrives.

## Consequences

**The subprotocol carries the credential, and the first draft of this ADR got
that backwards.** It specified a short-lived ticket minted by `auth-service`
and passed in the query string, and dismissed the subprotocol header as "more
moving parts for the same guarantee". That was wrong on both halves, and is
recorded rather than quietly edited:

- It is *fewer* moving parts, not more. The ticket needs a new RPC, a new
  token kind, a new lifetime, a new claim shape, and an extra round trip
  before every connect. The subprotocol needs none of those — the token
  already exists.
- The guarantee is *better*, not the same. A ticket in a URL is still a
  credential in a URL, bounded to sixty seconds; a subprotocol value is a
  header and never lands in a log at all. Bounding an exposure is worse than
  not having one.

The cost is that `Sec-WebSocket-Protocol` is being used for something that
is not really a subprotocol. That is a well-worn convention rather than an
abuse — it is how browsers are expected to authenticate a socket, precisely
because the constructor takes no headers — and the server echoes back only
the `bearer` element, never the token.

**Every existing client call site changes.** `X-Actor-Id` is how the SPA, the
seeder, `verify.js` and every smoke test currently identify themselves. All of
them move to a bearer token. That is mechanical but broad, and it is the reason
this lands as its own slice before any role work.

**The demo workspace stops being a free-for-all.** Today anyone who registers
can edit or delete the seeded pages, which is why `ops/reseed.timer` rebuilds
the workspace nightly. With spaces, the showcase content lives in a space where
new accounts are `viewer`. The nightly reseed stays — it is still the cheapest
way to undo damage — but it stops being the only thing standing between a
visitor and the corpus.

**`v1.0.0`'s "one shared pool" behaviour is a migration, not a default.**
Existing pages have no space. The migration creates one space, puts every page
in it, and makes every existing user an `editor` of it — preserving exactly
today's behaviour, so the change is observable in the model without being a
behaviour change on day one.

**This is the phase where a bug is a breach.** `RELEASES.md` already says every
change here gets `/security-review` before merge. Recorded again here because
the ADR is what a reviewer reads first.

## What this does not decide

- **API keys** (`§ 18c`) — a second credential kind with its own lifetime and
  scoping. It needs this ADR's identity work to exist first, and then its own
  decision about what a key may do that a session may not.
- **Invitations** — the flow (email? link? admin-adds-by-address?) is a product
  decision that does not change the model above. Membership rows are the same
  either way.
- **Per-page overrides.** Deliberately out. The moment a page can disagree with
  its space, "who can read this" becomes a tree walk with inheritance rules,
  and that is a substantially different system from the one decided here.

## Resources

- DDIA ch. 9 (consistency and consensus) — for why "revocation is immediate" is
  a claim about a distributed system, not about a `switch` statement.
- Zero To Production, ch. 10 (securing an API) — token verification shape,
  and why the signing key and the verifying key are different concerns.
- OWASP ASVS §4 (access control) — the "verify on the server, per request,
  from a trusted source" rule that `X-Actor-Id` violates today.
