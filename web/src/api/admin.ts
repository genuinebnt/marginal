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
import { GATEWAY_URL } from "./config";

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
