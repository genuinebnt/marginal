# API — Pages

**Status:** Implemented in Go (`services/document-service/internal/pages`) —
all six page-metadata RPCs, including ReparentPage's transactional subtree LTREE
rewrite, plus two more that read tables belonging to `internal/blockproj`'s own
projection of `collab.ops_flushed`, not page metadata: `ListBacklinks`
(`docs.page_links` — see `docs/porting/PROGRESS.md`'s backlinks entries) and
`ListBlocks` (`docs.blocks` — v2.3.0's diagnostics-service, the one caller;
no REST mapping exists for it, since it's an east-west call, never a browser
one, per ADR-007). Both live on this service only because those tables live
in this service's own database.

**`DeletePage` starts a saga (`v2.6.0`); it is not a soft delete.** This
reverses what this file said through `v2.5.0` — that a saga couldn't
coordinate with participants that don't exist, so none was attempted. The
first half is still true: `ARCHITECTURE.md` §5's full version names
`search-service`, `diagnostics-service`, and `history-service`, and the
first two are out of scope while the third is stateless with nothing to
invalidate. The conclusion was wrong, though, because it overlooked the
participant that *does* exist: `collaboration-service` holds a live rope
and an op log over the page being deleted, and purging rows out from under
it is the exact failure a saga prevents. See `ARCHITECTURE.md` §5 § *What
this actually is at this repo's scope* for the reduced-but-real flow.

The observable consequence for a caller: `DeletePage` returns as soon as
the page is marked `deleting`, **not** when the delete is finished. A page
is `deleting` while the saga runs, then `deleted` and restorable until its
purge window elapses. `GetPage` keeps answering for both states —
`lifecycle_state` is how a caller tells them apart, which is the whole
reason the field is on the wire rather than inferred from `deleted_at`.
**Owners:** `document-service` (gRPC `PageService`) · `api-gateway` (REST translation)
**Related:** ADR-007 (gRPC east-west) · `docs/architecture/lld/document-service.md` · `DATA_MODEL.md` §4

Pages have **two contracts**, and they are not the same document:

```
   browser  ──REST/JSON──▶  api-gateway  ──gRPC/protobuf──▶  document-service
            §2 below                      §1 below
```

§1 is the real service contract — protobuf in `services/document-service/proto/document.proto`, the
schema `document-service` is built and tested against. §2 is the gateway's REST
projection of it, and it is what the generated TypeScript client is built from
(`README.md` in this directory).

Semantics — idempotency, ordering, pagination — belong to §1 and are inherited by §2. A
rule stated once here applies to both.

---

## 1. `PageService` — the gRPC contract

```protobuf
service PageService {
  rpc CreatePage   (CreatePageRequest)   returns (Page);
  rpc GetPage      (GetPageRequest)      returns (Page);
  rpc ListPages    (ListPagesRequest)    returns (ListPagesResponse);
  rpc RenamePage   (RenamePageRequest)   returns (Page);
  rpc ReparentPage (ReparentPageRequest) returns (Page);
  rpc DeletePage   (DeletePageRequest)   returns (google.protobuf.Empty);
  rpc ListBacklinks(ListBacklinksRequest) returns (ListBacklinksResponse);
  rpc ListBlocks(ListBlocksRequest) returns (ListBlocksResponse);

  // v2.6.0 — the delete saga, made observable and reversible.
  rpc PreviewDelete(PreviewDeleteRequest) returns (PreviewDeleteResponse);
  rpc ListTrash    (ListTrashRequest)     returns (ListTrashResponse);
  rpc RestorePage  (RestorePageRequest)   returns (Page);
}
```

```protobuf
// What a delete would actually take with it. Reads the same forward
// reachability GraphService.BlastRadius already computes (v2.2.0) rather
// than a second traversal — a delete's blast radius and a link graph's
// are the same question asked by two screens.
message PreviewDeleteRequest  { string id = 1; }
message PreviewDeleteResponse {
  repeated Page descendants  = 1;  // cascade targets, the LTREE subtree
  repeated Page referrers    = 2;  // pages whose [[links]] would dangle
  int32         block_count  = 3;
}

message ListTrashRequest  { int32 limit = 1; int32 offset = 2; }
message ListTrashResponse {
  repeated TrashEntry entries = 1;
  int32               total   = 2;
}
message TrashEntry {
  Page                      page       = 1;
  google.protobuf.Timestamp purge_at   = 2;  // derived: deleted_at + window
  // Present only while lifecycle_state == 'deleting'. Absent once the
  // saga completes, because a finished saga has no progress to report.
  optional SagaProgress     progress   = 3;
}
message SagaProgress {
  repeated string steps_done = 1;  // in completion order
  repeated string steps_left = 2;  // the code's step list minus the above
  int32           attempts   = 3;  // > 1 means it resumed after a crash
  optional string last_error = 4;
}

message RestorePageRequest { string id = 1; }
```

**`RestorePage` refuses a page still in `deleting`.** Restoring mid-saga
would race the sweeper: a step that has already run (sessions closed, index
rows dropped) would have to be undone, which is exactly the rollback
forward-only compensation exists to avoid. The saga is short; the caller
waits for `deleted` and then restores. `FAILED_PRECONDITION`, not
`INVALID_ARGUMENT` — the request is well-formed, the state is wrong.

Rename and reparent are separate RPCs, not one `UpdatePage` with a field mask. They have
different authorization and, from Phase 3, compile to different ops — `SetTitle` versus
`MoveBlock` (RFC-002 §2). A single mutable update would collapse that distinction and
make the op compiler guess.

### The complete file

`services/document-service/proto/document.proto` — the whole Phase 1 surface:

```protobuf
syntax = "proto3";

package marginal.document.v1;

import "google/protobuf/timestamp.proto";
import "google/protobuf/empty.proto";

service PageService {
  rpc CreatePage   (CreatePageRequest)   returns (Page);
  rpc GetPage      (GetPageRequest)      returns (Page);
  rpc ListPages    (ListPagesRequest)    returns (ListPagesResponse);
  rpc RenamePage   (RenamePageRequest)   returns (Page);
  rpc ReparentPage (ReparentPageRequest) returns (Page);
  rpc DeletePage   (DeletePageRequest)   returns (google.protobuf.Empty);
  rpc ListBacklinks(ListBacklinksRequest) returns (ListBacklinksResponse);
  rpc ListBlocks(ListBlocksRequest) returns (ListBlocksResponse);
}

enum LifecycleState {
  LIFECYCLE_STATE_UNSPECIFIED = 0;
  LIFECYCLE_STATE_ACTIVE      = 1;
  LIFECYCLE_STATE_DELETING    = 2;
  LIFECYCLE_STATE_DELETED     = 3;
}

message Page {
  string id                            = 1;
  string created_by                    = 2;
  string title                         = 3;
  optional string parent_id            = 4;
  string path                          = 5;
  string sort_key                      = 6;
  LifecycleState lifecycle_state       = 7;
  google.protobuf.Timestamp created_at = 8;
  google.protobuf.Timestamp updated_at = 9;
  optional google.protobuf.Timestamp deleted_at = 10;
}

message CreatePageRequest   { string title = 1; optional string parent_id = 2; optional string after = 3; }
message GetPageRequest      { string id = 1; }
message ListPagesRequest    { optional string parent_id = 1; optional string after = 2; optional int32 limit = 3; }
message ListPagesResponse   { repeated Page pages = 1; optional string next_cursor = 2; }
message RenamePageRequest   { string id = 1; string title = 2; }
message ReparentPageRequest { string id = 1; optional string parent_id = 2; optional string after = 3; }
message DeletePageRequest   { string id = 1; }

message ListBacklinksRequest  { string page_id = 1; }
message ListBacklinksResponse { repeated Backlink backlinks = 1; }
message Backlink {
  string from_page          = 1;
  string from_page_title    = 2;
  bool from_page_deleted    = 3;
  string target_title       = 4;
}

message ListBlocksRequest  { string page_id = 1; }
message ListBlocksResponse { repeated Block blocks = 1; }
message Block {
  string id                 = 1;
  optional string parent_id = 2;
  string kind_json          = 3; // documentcore.BlockKind's own JSON shape
  string content_json       = 4; // documentcore.Content's own JSON shape
}
```

The enum's zero value is `UNSPECIFIED`, not `ACTIVE`. proto3 cannot distinguish an unset
enum from its zero value, so making `ACTIVE` zero would silently turn a serialisation bug
into a live page.

### Request messages

```protobuf
message CreatePageRequest   { string title = 1; optional string parent_id = 2; optional string after = 3; }
message GetPageRequest      { string id = 1; }
message ListPagesRequest    { optional string parent_id = 1; optional string after = 2; optional int32 limit = 3; }
message ListPagesResponse   { repeated Page pages = 1; optional string next_cursor = 2; }
message RenamePageRequest   { string id = 1; string title = 2; }
message ReparentPageRequest { string id = 1; optional string parent_id = 2; optional string after = 3; }
message DeletePageRequest   { string id = 1; }
message ListBacklinksRequest  { string page_id = 1; }
message ListBacklinksResponse { repeated Backlink backlinks = 1; }
message Backlink {
  string from_page = 1;
  string from_page_title = 2;
  bool from_page_deleted = 3;
  string target_title = 4;
}
message ListBlocksRequest  { string page_id = 1; }
message ListBlocksResponse { repeated Block blocks = 1; }
message Block {
  string id = 1;
  optional string parent_id = 2;
  string kind_json = 3;
  string content_json = 4;
}
```

`ReparentPageRequest.parent_id` being **absent** means "leave the parent alone"; promoting
a page to a root is `parent_id` present and empty. `optional` on a proto3 scalar gives
explicit field presence, which is what makes that distinction expressible at all.

The generated Go package is `marginal/document-service/genproto/documentv1` — NOT
under `internal/`, so `api-gateway` (a separate module) can import the client stub
across module boundaries (`services/document-service/scripts/gen-proto.sh` regenerates it).

### The `Page` message

```protobuf
message Page {
  string id               = 1;   // UUIDv7
  string created_by       = 2;
  string title            = 3;
  optional string parent_id = 4;
  string path             = 5;   // materialised LTREE ancestry
  string sort_key         = 6;   // opaque, lexicographic
  LifecycleState lifecycle_state = 7;
  google.protobuf.Timestamp created_at = 8;
  google.protobuf.Timestamp updated_at = 9;
  optional google.protobuf.Timestamp deleted_at = 10;
}
```

Field numbers are **never renumbered and never reused**, and messages only ever grow.
Same discipline as `content_version` in `DATA_MODEL.md` — an old reader must survive a
new writer, forever, because `history-service` replays events written by every prior
release.

`path` is exposed so the gateway can render breadcrumbs without a second call. It is not
an address — never construct a request from it. `sort_key` is opaque: order by it, never
parse it.

### Metadata, not fields

| Key | Set by | Purpose |
|---|---|---|
| `actor-id` | gateway, after RS256 verification | Authorship and `can_apply` |
| `grpc-timeout` | gateway | Deadline, propagated to Postgres statement timeout |
| `traceparent` | gateway | W3C trace context |

`created_by` is never taken from a request field. A client that could name its own author
could forge authorship — the gateway is the only component that knows who the caller is.

**No `api-gateway` exists in this repo's scope yet** — `document-service` reads
`actor-id` directly off incoming gRPC metadata as a stand-in (`internal/pages/api.go`'s
`actorID`), rejecting its absence with `UNAUTHENTICATED`. This is temporary scaffolding,
not the real trust boundary; replace it, don't build on it, once a gateway exists.

**No page read or write is scoped by `created_by` — reversed 2026-08-26** (see
`docs/porting/PROGRESS.md`). An earlier revision filtered every query on
`created_by = <actor>` (user story A-04's "only I can read or change my pages"),
which directly contradicted the "shared workspace, not multi-tenant" model
`Register`'s own reversal already established (`docs/api/auth.md`): a second
registered user's `actor-id` never matches the first user's `created_by`, so
every one of that first user's pages silently `NOT_FOUND`ed for them — a page
list stuck empty and a shared link stuck loading forever, found via live testing
with a second real account, not by inspection. `GetPage`/`ListPages`/`RenamePage`/
`DeletePage`/`ReparentPage`/`ListBacklinks` now filter only on `id`/`parent_id`/
`deleted_at` (`internal/pagerepo/queries.sql`) — every page on the instance is
visible to and editable by every authenticated actor. `created_by` is still
recorded on `CreatePage` (who authored a page) and still comes only from
`actor-id` metadata, never a request field (forgeable authorship is still not
allowed) — it's just no longer an access filter. `CreatePage`'s `parent_id`/
`after` and `ReparentPage`'s new parent resolve the same way: naming *any*
existing page succeeds, not just one the caller made themselves. `actorID(ctx)`
is still called on every RPC — the caller must still be authenticated, just not
the resource's own author. Locked in by `TestPagesAreVisibleToEveryActor`/
`TestCreateCanNestUnderAnyExistingPage` in `internal/pages/repo_integration_test.go`
(these replace the old `TestPagesAreScopedToTheirOwner`/
`TestCreateCannotNestUnderAnotherActorsPage`, which asserted the now-reversed
behavior). User story A-04 no longer holds as written — see `docs/porting/PROGRESS.md`.

### Status codes

| Situation | gRPC status |
|---|---|
| Page, parent, or anchor does not exist | `NOT_FOUND` |
| Title empty, oversized, or containing control characters | `INVALID_ARGUMENT` |
| Reparent under self or own descendant | `FAILED_PRECONDITION` |
| Anchor is not a child of the named parent | `FAILED_PRECONDITION` |
| Any database failure | `INTERNAL` |
| Deadline exceeded | `DEADLINE_EXCEEDED` |

`FAILED_PRECONDITION` rather than `ABORTED` is deliberate: per gRPC's own guidance,
`ABORTED` invites a retry and these failures are client bugs that will fail identically
on retry. `INTERNAL` never carries detail — the cause goes to the log inside the request
span, correlated by trace id.

---

## 2. Gateway REST mapping

| Method | Path | RPC |
|---|---|---|
| `POST` | `/pages` | `CreatePage` |
| `GET` | `/pages` | `ListPages` |
| `GET` | `/pages/{id}` | `GetPage` |
| `PATCH` | `/pages/{id}/title` | `RenamePage` |
| `PATCH` | `/pages/{id}/parent` | `ReparentPage` |
| `DELETE` | `/pages/{id}` | `DeletePage` |
| `GET` | `/pages/{id}/backlinks` | `ListBacklinks` |
| `GET` | `/pages/{id}/delete-preview` | `PreviewDelete` |
| `GET` | `/trash` | `ListTrash` |
| `POST` | `/pages/{id}/restore` | `RestorePage` |

`/trash` is top-level rather than `/pages/trash`: it lists pages that are
no longer in the tree, and nesting it under the collection they left would
make `GET /pages/{id}` and `GET /pages/trash` compete for the same route
shape with `trash` as a reserved id.

`POST /pages/{id}/restore` rather than `PATCH /pages/{id}` with a
lifecycle field — restore is an operation with its own precondition
(§1: it fails on a page still `deleting`), not a field assignment, and a
`PATCH` that silently no-ops on the wrong state is the shape that hides
that.

```json
{
  "id": "018f2b1c-0000-7000-8000-000000000000",
  "created_by": "018f2b1c-0000-7000-8000-000000000001",
  "title": "Architecture",
  "parent_id": null,
  "path": "p018f2b1c00007000",
  "sort_key": "a1",
  "lifecycle_state": "active",
  "created_at": "2026-08-07T00:38:16.454433Z",
  "updated_at": "2026-08-07T00:38:16.454433Z"
}
```

`deleted_at` is omitted while the page is active rather than sent as `null`. Timestamps
are RFC 3339 UTC — `google.protobuf.Timestamp` on the wire, formatted at the gateway.

### Status translation

| gRPC | HTTP | `error` code | Retryable |
|---|---|---|---|
| `UNAUTHENTICATED` | 401 | `unauthenticated` | No — missing/invalid `actor-id` (§ temporary scaffolding above) |
| `INVALID_ARGUMENT` | 422 | `validation_failed` | No — fix the request |
| `NOT_FOUND` | 404 | `not_found` | No |
| `FAILED_PRECONDITION` | 409 | `conflict` | No — contradicts current state |
| `INTERNAL`, `UNKNOWN` | 500 | `internal_error` | Yes, with backoff |
| `UNAVAILABLE` | 503 | `unavailable` | Yes, with backoff |
| `DEADLINE_EXCEEDED` | 504 | `timeout` | Yes, with backoff |

One error shape, so the client has exactly one branch:

```json
{ "error": "not_found", "message": "page not found" }
```

Every response carries `x-request-id`; it appears in the matching log span on both sides
of the gateway.

---

## 3. Semantics

### Create

`after` names the sibling the new page follows; omitted means append. `parent_id` omitted
means a root page.

**Not idempotent.** Two identical calls create two pages — duplicate titles are legal and
surface as an RFC-003 `DuplicateTitle` diagnostic, never a constraint violation. Clients
needing retry safety should generate the id client-side; that is a future contract change,
not current behaviour.

**Ordering guarantee:** the new page's `sort_key` sorts strictly between its neighbours
and **no sibling row is rewritten**. That is the entire point of a fractional index.

### List

`ListPagesRequest { parent_id?, after?, limit? }` → `ListPagesResponse { pages, next_cursor }`.

- `parent_id` omitted lists root pages. It is a **filter, not a subtree walk** — direct
  children only.
- Always ordered by `sort_key` ascending.
- `after` is the previous response's `next_cursor` — a `sort_key`, not an offset. Keyset
  pagination means an insertion during paging cannot duplicate or skip a row.
- `next_cursor` is unset on the final page.
- Soft-deleted pages never appear.

### Get

`NOT_FOUND` if the page does not exist **or has been soft-deleted** — the contract does
not distinguish the two.

### Rename

`updated_at` advances, `created_at` does not. `NOT_FOUND` on a deleted page — a tombstone
is not editable.

### Reparent

`parent_id` and `after` are independently optional:

| Set | Effect |
|---|---|
| `parent_id` only | Move to a new parent, append |
| `after` only | Reorder within the current parent |
| both | Move and position in one transaction |
| `parent_id` explicitly null | Promote to a root page |

**Transactional over the subtree.** Every descendant's `path` is rewritten in the same
transaction; a concurrent reader sees all old paths or all new ones, never a mixture.

### Delete

Soft, and **cascades to the subtree** — descendants disappear with the parent. The
cascade itself is purely a `document-service`-internal LTREE operation (same
`path <@ ...` pattern `RewriteDescendantPaths` uses for reparenting) and doesn't
need any other service, so it's implemented in full here.

**What's genuinely out of scope:** `ARCHITECTURE.md` §5's full choreographed saga —
waiting for `collab.page_released`/`search.page_purged`/etc. acks before a final
hard-delete — coordinates with `collaboration-service`, `search-service`,
`diagnostics-service`, and `history-service`. Only the first of those exists in this
repo. `lifecycle_state` stays `deleting` forever here (never progresses to
`deleted`), since nothing in this repo's scope is positioned to be the thing that
completes that transition. That's a real, structural gap for a production
deployment, not a silently missing feature.

**Idempotent.** Deleting an already-deleted (or already-`deleting`) page succeeds
without changing anything — cascading again over descendants that are already
`deleting` is a no-op, not an error.

The row survives. `lifecycle_state` moves `active → deleting`; only the saga's final step
sets `deleted`.

---

## 4. What is not on this surface

`/health` and `/health/ready` stay plain HTTP on a separate port — Kubernetes probes are
HTTP, and that Axum listener never grows a business endpoint (ADR-007).

Transport-level failures come from the gateway, not from `PageService`: a body over
`max_body_bytes` is 413, and a request over the gateway's timeout is 504. Neither uses the
`ApiError` shape.
