# ADR-007 — gRPC as the East-West Default

**Date:** 2026-08-07
**Status:** Accepted
**Related:** ADR-001 (service boundaries) · ADR-002 (Rust depth) · ADR-004 (SPA)
**Deciders:** @genuinebasilnt

---

## Context

The obvious economy is to confine gRPC to the pairs that need streaming — four of them
exercise all four RPC modes — and leave `api-gateway → document-service`,
`→ search-service` and `→ history-service` on plain HTTP, on the grounds that simple
request/response is idiomatic over HTTP and trivially `curl`-able.

That optimises for the smallest amount of transport machinery. This project's
primary objectives are Rust depth and a **genuinely scalable microservice architecture**
(ADR-002, ADR-001), and under those objectives a mixed east-west transport is a liability
rather than an economy:

- Two internal transports means two sets of timeouts, retries, tracing propagation, and
  error mappings — the cross-cutting concerns get solved twice and drift.
- HTTP + JSON between services has no schema. The contract that protects the TypeScript
  client (`docs/api/README.md`) protects nothing east-west, where the same class of
  rename-breaks-consumer bug exists.
- The interesting distributed-systems work — deadline propagation, load balancing across
  replicas, connection pooling, interceptors carrying trace context — is uniform gRPC
  machinery. Applying it to four of seven pairs teaches it partially.

---

## Decision

**gRPC (tonic + prost) is the transport for all synchronous service-to-service calls.
REST and WebSocket exist only at the edge, between the browser and `api-gateway`.**

```
   browser                     api-gateway                 internal
   ───────                     ───────────                 ────────
   HTTPS / REST   ──────────▶  │              │  ──gRPC──▶  auth
   WSS (collab)   ──────────▶  │  protocol    │  ──gRPC──▶  document
                               │  boundary    │  ──gRPC──▶  search · history
                               └──────────────┘  ──WSS───▶  collaboration
   HTTPS ─── direct to S3/MinIO (presigned PUT) ───▶  bypasses all services
```

Three things are deliberately **not** gRPC:

| Exception | Transport | Why |
|---|---|---|
| Browser ↔ `api-gateway` | REST + WSS | Browsers cannot speak gRPC — it needs HTTP/2 trailer access that `fetch` does not expose. gRPC-Web needs a translating proxy and cannot do bidirectional streaming |
| Asynchronous domain events | NATS JetStream | Fan-out with durable consumers. Replacing it with server streaming would be a regression |
| `/health`, `/health/ready` | HTTP | Kubernetes probes are HTTP. Every service keeps a tiny Axum server for probes and nothing else |

Client → gateway remains REST because that is the contract the generated TypeScript
client is built from. The gateway is the translation boundary in both directions.

### RPC modes still map to real needs

A uniform transport does not cost the pedagogical spread — each of the four modes is still
demanded by a real pair, and there are simply more unary examples:

| Pair | Mode |
|---|---|
| `api-gateway` → `auth-service` | Unary — introspection, key rotation |
| `api-gateway` → `document-service` | Unary — page CRUD, tree queries |
| `api-gateway` → `search-service` | Unary, and server streaming for incremental results |
| `api-gateway` → `history-service` | Unary — snapshot list, replay to version |
| `document-service` ↔ `collaboration-service` | **Bidirectional streaming** — ops both ways for a session's life |
| `collaboration-service` → `diagnostics-service` | **Server streaming** — results as analysis completes |
| `collaboration-service` → `history-service` | **Client streaming** — batched ops with flow-control back-pressure |

### JWT verification stays local

Worth stating here because it is the largest latency decision in the system: tokens are RS256, the gateway holds the public key and verifies **with no
network call**, checking a Redis blocklist for revocation. The unary RPC to `auth-service`
is for introspection and key rotation only.

**Do not "optimise" toward a per-request `ValidateToken` call.** It would put
`auth-service` on the critical path of every request.

---

## Consequences

### `crates/proto` becomes a dependency of every service

`.proto` files live in `crates/proto` with `tonic-build` codegen in `build.rs`, and all seven
services depend on it. Proto files are organised per owning service (`document.proto`,
`auth.proto`, …), not one god file.

Protobuf field numbering discipline now matters everywhere: fields are never renumbered
and never reused, and every message is extended additively. Same rule as
`content_version` in `DATA_MODEL.md` — old readers must survive new writers.

### Losing `curl` is a real cost, and the mitigation is tooling

`grpcurl` with server reflection replaces `curl` for manual poking. Reflection is enabled
in local and staging builds and disabled in production, where it would expose the full
service surface to anything that can reach the port.

### Each service runs two listeners

One tonic server for gRPC, one small Axum server for `/health` and `/health/ready`. They
bind different ports (`grpc_port`, `http_port` in `config.yaml`). The Axum server never
grows a business endpoint — if a route needs authentication, it belongs on the gRPC
surface behind the gateway.

### Testing does not need the gateway

A tonic service is tested in-process: build the service, serve it over an in-memory
duplex transport, drive it with a generated client. No ports, no gateway, no Docker —
the same shape as driving an Axum router with `oneshot`. Phase 1 remains fully testable
before Phase 9 exists.

### Cross-cutting concerns move into interceptors

Trace context propagation, deadline enforcement, and actor identity (`X-Actor-Id` becomes
a gRPC metadata key) become tonic interceptors written once, rather than Tower layers
duplicated per service in two transport flavours.

### What this invalidates

- `docs/api/pages.md` described REST **on `document-service`**. The REST contract belongs
  to `api-gateway`; `document-service` exposes `PageService`. That document must be split.
- The pending HTTP test suite for `document-service` becomes tonic client tests.
- `ARCHITECTURE.md` §1's service map and §3's load-a-page sequence both show HTTP hops
  that are now gRPC.

---

## Alternatives considered

**gRPC only for the pairs that need streaming**, HTTP for the rest. Cheapest in machinery,
and the four-mode coverage is just as elegant. Rejected because a mixed transport is not what
a scalable microservice architecture looks like, and the project exists to build one.

**gRPC everywhere including the browser, via gRPC-Web.** Rejected: needs a translating
proxy, cannot do bidirectional streaming, and would force the editor's op stream onto a
worse transport than the WebSocket it already needs.

**REST everywhere with an OpenAPI contract east-west.** Rejected: it solves the schema
problem but not streaming, and the op flow between document and collaboration genuinely
needs bidirectional streaming.

---

## Resources

| Resource | For |
|---|---|
| [tonic](https://github.com/hyperium/tonic) | Streaming, interceptors, in-process testing |
| [proto3 language guide](https://protobuf.dev/programming-guides/proto3/) | Schema design and evolution rules |
| [gRPC core concepts](https://grpc.io/docs/what-is-grpc/core-concepts/) | The four RPC modes |
| [grpcurl](https://github.com/fullstorydev/grpcurl) | The `curl` replacement |
| [gRPC deadlines](https://grpc.io/blog/deadlines/) | Why deadline propagation beats per-hop timeouts |
