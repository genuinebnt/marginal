# ADR-006 — gRPC for Selected Service Pairs

**Date:** 2026-08-06
**Status:** Superseded by [ADR-007](ADR-007-grpc-as-east-west-default.md) on 2026-08-07
**Related:** ADR-001 (service boundaries)
**Deciders:** @genuinebasilnt

> **Superseded.** gRPC is now the transport for *all* synchronous east-west calls, not
> four selected pairs. The protocol-boundary reasoning below (browsers cannot speak gRPC;
> JWT verification stays local) survives unchanged in ADR-007 — only the "HTTP for simple
> pairs" decision was reversed. Kept for the record.

---

## Context

The default inter-service transport is HTTP (Axum) for synchronous request/response and NATS JetStream for asynchronous domain events. That covers most communication well.

Four pairs do not fit plain HTTP, and between them they exercise **all four gRPC modes** — which is the pedagogical reason to include gRPC at all.

---

## Decision

Use **gRPC (tonic + prost)** for these four pairs only. Everything else stays HTTP + NATS.

| Pair | Mode | Why |
|---|---|---|
| `api-gateway` → `auth-service` | **Unary** | Token introspection and key rotation. Binary protobuf over multiplexed HTTP/2 |
| `document-service` ↔ `collaboration-service` | **Bidirectional streaming** | Ops flow both ways continuously for a session's lifetime (minutes to hours). Modelling this as independent HTTP requests would need polling or SSE |
| `collaboration-service` → `diagnostics-service` | **Server streaming** | Collaboration opens one long-lived call per document; diagnostics streams results back as analysis completes. Incremental results must arrive as they are computed, not batched at the end |
| `collaboration-service` → `history-service` | **Client streaming** | Stream batched ops for snapshotting with gRPC flow control providing back-pressure |

All `.proto` definitions live in `libs/proto` with `tonic-build` codegen in `build.rs`. Services that participate add `libs/proto` as a path dependency; the rest do not depend on it at all.

### Why not gRPC everywhere

- Most pairs are simple, low-frequency request/response where HTTP is idiomatic and trivially observable with `curl` and Axum middleware
- NATS JetStream already handles async fan-out; replacing it with gRPC server-streaming would be a regression
- Four pairs keeps proto management and tonic boilerplate proportional to the benefit

### Diagnostics streaming is the interesting one

`diagnostics-service` is **degradable** (ADR-001) — if it is unavailable, editing continues without squiggles. Server streaming makes that concrete: the stream simply ends or fails to open, and the client renders no diagnostics. There is no request to retry, no queue to drain, no user-visible error. Graceful degradation as a transport property rather than a code path.

---

## Consequences

### JWT verification is local — this is not a per-request RPC

Tokens are signed **RS256**, which is asymmetric. The gateway holds the public key and verifies signatures **locally with no network call**, checking a Redis blocklist for revocation.

A per-request `ValidateToken` RPC would add a hop to the critical path of every request and make `auth-service` a hard availability dependency for all traffic. The unary RPC exists for introspection and key rotation, which are low-frequency.

**Do not "optimise" toward calling `auth-service` per request.** Removing that hop is the largest latency win available, and it is already the design.

### gRPC is east-west only — the browser never speaks it

Browsers cannot use gRPC: it requires control over HTTP/2 trailers, which `fetch`/`XHR` do not expose. gRPC-Web and Connect exist as workarounds but need a translating proxy, and gRPC-Web cannot do bidirectional streaming.

**The api-gateway is the protocol boundary:**

```
   browser                     api-gateway                internal
   ───────                     ───────────                ────────
   HTTPS / REST   ──────────▶  │              │  ──gRPC──▶  auth
   WSS (collab)   ──────────▶  │ translation  │  ──gRPC──▶  collaboration
                               │   boundary   │  ──HTTP──▶  document, search
                               └──────────────┘
   HTTPS ─── direct to S3/MinIO (presigned PUT) ───▶  bypasses all services
```

North-south is REST and WebSocket. gRPC is confined inside the cluster.

### Added to the workspace

`tonic`, `prost`, `tonic-build` (dev-dependency in `libs/proto`), and a `build.rs` running `tonic_build::compile_protos`.

---

## Resources

| Resource | For |
|---|---|
| [tonic](https://github.com/hyperium/tonic) | Streaming patterns, interceptors |
| [proto3 language guide](https://protobuf.dev/programming-guides/proto3/) | Schema design |
| [gRPC core concepts](https://grpc.io/docs/what-is-grpc/core-concepts/) | The four RPC modes |
