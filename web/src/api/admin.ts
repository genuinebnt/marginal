// § 18 ADMIN's three data sources.
//
// They come from three different places on purpose, and the
// screen says which is which: service health is a live probe the
// gateway performs per request, people come from auth-service,
// and the queue numbers come from collaboration-service directly
// (the same convention its other instance-fact endpoints follow —
// the gateway maps resource contracts, and none of these is a
// resource).
import { apiFetch } from "./http";
import { COLLAB_URL, GATEWAY_URL } from "./config";

export interface ServiceHealth {
  name: string;
  url: string;
  /** "up" | "down" | "timeout" — three values because they need
   *  three different next actions: nothing, read the logs, look
   *  at the network. */
  status: string;
  latency_ms: number;
  detail?: string;
  role: string;
}

export interface HealthReport {
  services: ServiceHealth[];
  up: number;
  total: number;
  checked_at: string;
}

export interface Person {
  id: string;
  email: string;
  display_name: string;
  cursor_color: string;
  created_at: string;
}

export interface People {
  people: Person[];
  /** Refresh tokens neither revoked nor expired — signed in
   *  somewhere. Not live WebSocket connections. */
  active_sessions: number;
}

export function getHealth(): Promise<HealthReport> {
  return apiFetch<HealthReport>(`${GATEWAY_URL}/admin/health`);
}

export function getPeople(actorId: string | null): Promise<People> {
  return apiFetch<People>(`${GATEWAY_URL}/admin/people`, { actorId });
}

// The queue numbers live in ./history — that module already owns
// collaboration-service's direct endpoints and their plain-text
// error shape, and a second copy here drifted from it within the
// hour.
export { getCollabStats, type CollabStats } from "./history";

/** One § 18b row from the op log. No payload: an audit row says
 *  who did what to which page, and the text somebody typed is
 *  the document's business. */
export interface AuditRow {
  id: string;
  seq: number;
  page_id: string;
  actor_id: string;
  actor_kind: string;
  kind: string;
  /** "content" | "destructive" — what the filter chips select. */
  class: string;
  /** Ties a row to the one gesture that produced it, so a
   *  keystroke that emitted three ops reads as one action. */
  undo_group?: string;
  created_at: string;
}

export interface AuditReport {
  rows: AuditRow[];
  /** Over the whole log, not the page returned — "by class"
   *  would answer a different question otherwise. */
  counts: Record<string, number>;
  total: number;
  kinds: Array<{ kind: string; class: string; n: number }>;
}

export interface AuthEvent {
  id: string;
  /** auth.register | auth.signin | auth.signout */
  kind: string;
  user_id: string;
  at: string;
}

/** collaboration-service directly — the content half. */
export async function getAudit(cls: string, limit = 120): Promise<AuditReport> {
  const url = new URL(`${COLLAB_URL}/collab/audit`);
  url.searchParams.set("limit", String(limit));
  if (cls && cls !== "all") url.searchParams.set("class", cls);
  const res = await fetch(url);
  if (!res.ok) throw new Error((await res.text()).trim() || res.statusText);
  return res.json() as Promise<AuditReport>;
}

/** auth-service through the gateway — the auth half. The two are
 *  merged by timestamp in the client, which is where a join
 *  across service boundaries belongs. */
export function getAuthEvents(actorId: string | null, limit = 60): Promise<{ events: AuthEvent[] }> {
  return apiFetch<{ events: AuthEvent[] }>(
    `${GATEWAY_URL}/admin/auth-events?limit=${limit}`, { actorId });
}
