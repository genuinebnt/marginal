# Spaces & Roles API — `SpaceService`

`ADR-013` made real: **a space is the permission boundary**, a membership
binds one user to one space with one role, and a page belongs to exactly one
space. Owned by `auth-service`, because a membership is a statement about
what a *user* may do — the same reason roles are not a `docs` concern.

`DATA_MODEL.md` § `auth` Schema has the tables; this is the contract over
them.

---

## 0. What a role means

Three, and only three (`ADR-013` §2):

| Role | May |
|---|---|
| `viewer` | read pages in the space |
| `editor` | everything `viewer` may, plus emit any op |
| `admin` | everything `editor` may, plus manage membership and delete the space |

**Read and write are enforced in different places, on purpose.**
`document-service` filters reads against its own `docs.space_members`
projection; `collaboration-service` gates writes in `can_apply`, resolving
the role from this service at join (`ADR-013` §3, §4). There is no third
path, and the gateway enforces nothing — it authenticates and forwards.

**A space always has at least one admin.** The last admin cannot be demoted
or removed; the attempt is `FAILED_PRECONDITION`. This is checked in the
transaction rather than by a constraint, because it is a claim about the set
of remaining rows (`DATA_MODEL.md` says why a trigger is worse).

---

## 1. `SpaceService` — the gRPC contract

```protobuf
service SpaceService {
  // Every space the CALLER is a member of, with the caller's own role in
  // each. Never "every space that exists": that is an admin question, and
  // answering it here would leak the existence of spaces you are not in.
  rpc ListSpaces   (ListSpacesRequest)   returns (ListSpacesResponse);
  rpc CreateSpace  (CreateSpaceRequest)  returns (Space);
  // The caller must be an admin of space_id. Listing members of a space you
  // are not in is NOT_FOUND, not PERMISSION_DENIED — see § 3.
  rpc ListMembers  (ListMembersRequest)  returns (ListMembersResponse);
  rpc GrantRole    (GrantRoleRequest)    returns (Membership);
  rpc RevokeRole   (RevokeRoleRequest)   returns (google.protobuf.Empty);
  // The whole membership set, for document-service's periodic reconcile
  // (DATA_MODEL.md § docs.space_members). Not a user-facing call.
  rpc ListAllMemberships (ListAllMembershipsRequest) returns (ListAllMembershipsResponse);
}
```

`GrantRole` is an upsert: granting a role to someone who already has one in
that space changes it. That is deliberate — "change Ada from editor to
viewer" and "add Ada as a viewer" are the same intent expressed twice, and
two RPCs for it would mean a client has to know which case it is in.

Both mutations publish (`DATA_MODEL.md` §10, topics already reserved):

| RPC | Event |
|---|---|
| `GrantRole` | `auth.role_granted` |
| `RevokeRole` | `auth.role_revoked` |

Published through `auth-service`'s existing outbox, in the same transaction
as the row — the write and its announcement cannot disagree.

---

## 2. REST mapping (`api-gateway`)

| Method | Path | RPC |
|---|---|---|
| `GET` | `/spaces` | `ListSpaces` |
| `POST` | `/spaces` | `CreateSpace` |
| `GET` | `/spaces/{id}/members` | `ListMembers` |
| `PUT` | `/spaces/{id}/members/{userId}` | `GrantRole` |
| `DELETE` | `/spaces/{id}/members/{userId}` | `RevokeRole` |

```json
// GET /spaces
{
  "spaces": [
    { "id": "...", "name": "Showcase", "is_default": true, "your_role": "editor", "members": 4 }
  ]
}
```

`your_role` rather than `role`: the field answers "what may *I* do here",
and a name that could be read as "the space's role" invites a client to
cache it against the wrong key.

---

## 3. A space you are not in is `404`, not `403`

`403` on a resource you cannot see tells you it exists. For a space that is
a real leak — space names are chosen by people and often say what a team is
working on — so `ListMembers`, `GrantRole` and `RevokeRole` all answer
`NOT_FOUND` when the caller is not a member, and `PERMISSION_DENIED` only
when they *are* a member without the rank.

The distinction is deliberate and worth keeping consistent: **`403` means
"you may not", which is only safe to say once "it exists" is already known
to you.**

---

## 4. What this does not cover

- ~~**Invitations.**~~ Decided and built (`v3.3.0`) — see § 5 below.
- **Per-page overrides.** Deliberately out — a page that can disagree with
  its space turns "who can read this" into a tree walk with inheritance.
- **API keys** (`§ 18c`). A second credential kind, with its own lifetime
  and its own scoping question.

---

## 5. Invitations (`v3.3.0`)

`§ 20` decides the flow this contract previously deferred: an admin invites
somebody who is already an account on this instance, and the invitation sits
**pending** until they accept or decline it. It arrives as a notification
(`docs/api/notifications.md`), which is the only place it is surfaced.

**An invitation is not a membership.** It is its own table, and
`auth.memberships` is unchanged — so a role check that does not know
invitations exist behaves exactly as it did before they did. That is the
whole reason for the split: a `accepted_at` column on the membership would
be a nullable field on the hot path of every authorization decision, and
forgetting it once grants access to somebody who never accepted.

### `InviteMember(space_id, user_id, role)` → `Invitation`

Admin of that space only — the same check `GrantRole` makes, in the same
place. `ALREADY_EXISTS` if they are already a member (an invitation is not a
way to change somebody's role) or already have a pending invitation to that
space.

Publishes `auth.member_invited`.

The response carries ids and the role; `space_name` and `invited_by_name`
are empty on it. They are joined columns, and the surface that renders an
invitation as a sentence (`§ 20`) reads `ListInvitations`, which has them.
Stated rather than silently half-filled.

### `RespondToInvitation(invitation_id, accept)` → `Invitation`

Only the person invited may answer, and only once; a second answer is
`FAILED_PRECONDITION` rather than a silent no-op, because the two outcomes
differ and the caller should be able to tell which happened.

**Accepting grants**, in one transaction with marking the invitation
answered, through the same `GrantRole` path an admin would take — so
`auth.role_granted` is published exactly as it already is, and
`document-service`'s projection needs no new consumer.

**Declining records the refusal** rather than deleting the row. A deleted
invitation cannot be told apart from one that was never sent.

### `ListInvitations()` → pending invitations for the caller

Scoped to the caller by their token, never by a parameter.
