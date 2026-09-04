// docs/api/spaces.md §2's REST mapping — v3.1.0's permission boundary.
import { GATEWAY_URL } from "./config";
import { apiFetch } from "./http";

export interface Space {
  id: string;
  name: string;
  is_default: boolean;
  created_by: string;
  /** What YOU may do here — not "the space's role". A field called `role`
   *  invites caching one person's answer under a key that reads as
   *  everyone's (docs/api/spaces.md §2). */
  your_role: "viewer" | "editor" | "admin";
  members: number;
}

export interface Member {
  user_id: string;
  space_id: string;
  role: "viewer" | "editor" | "admin";
  display_name: string;
  email: string;
}

export function listSpaces(actorId: string): Promise<{ spaces: Space[] }> {
  return apiFetch(`${GATEWAY_URL}/spaces`, { actorId });
}

export function createSpace(actorId: string, name: string): Promise<Space> {
  return apiFetch(`${GATEWAY_URL}/spaces`, { method: "POST", body: { name }, actorId });
}

/** Admin-only. A space you are not in answers 404, never 403 — a 403 would
 *  confirm it exists (docs/api/spaces.md §3). */
export function listMembers(actorId: string, spaceId: string): Promise<{ members: Member[] }> {
  return apiFetch(`${GATEWAY_URL}/spaces/${spaceId}/members`, { actorId });
}

export function grantRole(
  actorId: string, spaceId: string, userId: string, role: Member["role"],
): Promise<Member> {
  return apiFetch(`${GATEWAY_URL}/spaces/${spaceId}/members/${userId}`, {
    method: "PUT", body: { role }, actorId,
  });
}

export function revokeRole(actorId: string, spaceId: string, userId: string): Promise<void> {
  return apiFetch(`${GATEWAY_URL}/spaces/${spaceId}/members/${userId}`, {
    method: "DELETE", actorId,
  });
}

/** What each role may do — the table § 23 draws.
 *
 *  Three, not five: the mockup showed COMMENTER and PROPOSER, which gate
 *  capabilities this repo does not have (comments are v3.2.0, the assistant
 *  v4.4.0). A permission for something nobody can do is a control that
 *  cannot be checked, so the mockup was corrected rather than the screen
 *  built to it (ADR-013 §2).
 *
 *  Kept beside the API client because it must agree with what the server
 *  enforces — internal/spaces.Role.AtLeast and roles.Role.CanWrite are the
 *  two places that decide, and this table only reports them. */
export const CAPABILITIES: { label: string; admin: boolean; editor: boolean; viewer: boolean }[] = [
  { label: "Read pages", admin: true, editor: true, viewer: true },
  { label: "Emit ops", admin: true, editor: true, viewer: false },
  { label: "Move or delete pages", admin: true, editor: true, viewer: false },
  { label: "Change membership", admin: true, editor: false, viewer: false },
  { label: "Delete the space", admin: true, editor: false, viewer: false },
];

/** docs/api/spaces.md § 5. An invitation is NOT a membership — it grants
 *  nothing until it is accepted, which is why it has its own type here as
 *  well as its own table there. */
export interface Invitation {
  id: string;
  space_id: string;
  space_name: string;
  user_id: string;
  role: "viewer" | "editor" | "admin";
  invited_by: string;
  invited_by_name: string;
  created_at: string;
  responded_at?: string;
  accepted: boolean;
}

export function inviteMember(
  actorId: string, spaceId: string, userId: string, role: string,
): Promise<Invitation> {
  return apiFetch(`${GATEWAY_URL}/spaces/${spaceId}/invitations`, {
    method: "POST", actorId, body: { user_id: userId, role },
  });
}

/** The caller's own pending invitations. No user id parameter — the token
 *  says who is asking, and a parameter would be a way to ask about
 *  somebody else. */
export function listInvitations(actorId: string): Promise<{ invitations: Invitation[] }> {
  return apiFetch(`${GATEWAY_URL}/invitations`, { actorId });
}

export function respondToInvitation(
  actorId: string, id: string, accept: boolean,
): Promise<Invitation> {
  return apiFetch(`${GATEWAY_URL}/invitations/${id}/respond`, {
    method: "POST", actorId, body: { accept },
  });
}
