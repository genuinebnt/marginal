// collaboration-service's own plain HTTP endpoints (docs/api/
// collaboration.md §5-§7) — reached directly, never through api-gateway,
// same convention its WebSocket already uses (a persistent connection
// isn't a request/response resource, and these read-only debug/history
// endpoints were never given the api-gateway's REST-shim treatment
// either). Unlike pages.md/auth.md/graph.md's REST layer, these return a
// plain-text body on error (net/http.Error, not the {error, message}
// JSON shape ../api/http.ts's apiFetch expects) — collabFetch handles
// that directly instead of reusing apiFetch and mismatching its contract.
import { accessToken } from "./http";
import { COLLAB_URL } from "./config";

export class CollabHttpError extends Error {
  status: number;
  constructor(status: number, message: string) {
    super(message);
    this.name = "CollabHttpError";
    this.status = status;
  }
}

async function collabFetch<T>(url: string): Promise<T> {
  // collaboration-service verifies the same token the socket does — these
  // endpoints return a page's whole op log, so they are not public
  // (ADR-013 §1). /collab/stats is public and goes through here too;
  // sending a token it does not need costs nothing.
  const token = accessToken();
  const res = await fetch(url, token ? { headers: { Authorization: `Bearer ${token}` } } : undefined);
  if (!res.ok) {
    const text = await res.text();
    throw new CollabHttpError(res.status, text.trim() || res.statusText);
  }
  return res.json() as Promise<T>;
}

export interface Op {
  scope: "block" | "text";
  [key: string]: unknown;
}

export interface LoggedOp {
  id: string;
  page_id: string;
  actor_id: string;
  actor_kind: string;
  created_at: string;
  undo_group?: string | null;
  op: Op;
}

export interface BlockSnapshot {
  id: string;
  parent: string | null;
  kind: { tag: string; [key: string]: unknown };
  text: string;
  marks?: unknown[];
}

export interface Snapshot {
  page_id: string;
  title: string;
  blocks: BlockSnapshot[];
}

export interface TraceStep {
  op: LoggedOp;
  inverse: Op;
  law_holds: boolean;
  after: Snapshot;
}

export function getTrace(pageId: string): Promise<{ steps: TraceStep[] }> {
  return collabFetch(`${COLLAB_URL}/collab/pages/${pageId}/trace`);
}

/** GET /collab/stats — this instance's own queue depths, for
 *  § 16's QUEUE DEPTH panel. Measured, not drawn.
 *
 *  Two numbers per queue on purpose: a depth of 400 draining in
 *  200 ms and a depth of 3 whose oldest row has waited four
 *  minutes are opposite conditions, and the count alone calls the
 *  second one healthy. */
export interface CollabStats {
  outbox_depth: number;
  outbox_oldest_seconds: number;
  ops: number;
  pages: number;
  /** Time since the newest op. On an idle instance this is large
   *  and fine — the screen labels it rather than colouring it. */
  lag_seconds: number;
  /** Pages with a live rope in memory. NOT people signed in
   *  (auth-service's number) and not editors connected. Three
   *  meanings of "sessions"; each surface says which it shows. */
  open_sessions: number;
  /** This service's own database. Database-per-service means an
   *  instance-wide "DB size" is a number nobody owns. */
  database_bytes: number;
  /** Last 14 hours, oldest first, quiet hours present as zero —
   *  a sparkline that omits them draws a busy day where there
   *  was a gap. Always an array, never null. */
  ops_per_hour: number[];
}

export function getCollabStats(): Promise<CollabStats> {
  return collabFetch<CollabStats>(`${COLLAB_URL}/collab/stats`);
}

export interface PalimpsestChar {
  rune: number; // a Go rune, i.e. a Unicode code point — String.fromCodePoint to render
  insert_step: number;
  insert_actor: string;
  delete_step?: number;
  delete_actor?: string;
}

export function getPalimpsest(pageId: string, blockId: string): Promise<{ chars: PalimpsestChar[]; current_step: number }> {
  return collabFetch(`${COLLAB_URL}/collab/pages/${pageId}/blocks/${blockId}/palimpsest`);
}

export interface Move {
  block_id: string;
  from_parent: string | null;
  from: string | null;
  to_parent: string | null;
  to: string | null;
  step: number;
}

export function getDiff(pageId: string, from: number, to: number): Promise<{ before: Snapshot; after: Snapshot; moves: Move[] }> {
  return collabFetch(`${COLLAB_URL}/collab/pages/${pageId}/diff?from=${from}&to=${to}`);
}

/** A short, human-readable label for one op — used by both HistoryScreen's
 * op stream and TraceScreen's op list, the same two "which op is this"
 * readouts trace.html's own oprow and history.html's own .op share. Real
 * data only: the op's own type plus whatever text it actually carries,
 * truncated for display, never a fabricated description. */
export function describeOp(op: Op): { kind: string; detail: string } {
  if (op.scope === "block") {
    const type = String(op.type ?? "Op");
    if (type === "InsertBlock" || type === "SetBlockContent") {
      const text = (op.content as { text?: string } | undefined)?.text ?? "";
      return { kind: type, detail: text ? `"${text.slice(0, 40)}"` : type === "InsertBlock" ? "(empty block)" : "" };
    }
    if (type === "SetTitle") return { kind: type, detail: `→ "${op.to ?? ""}"` };
    return { kind: type, detail: "" };
  }
  const inner = op.op as { type?: string; text?: string } | undefined;
  const type = inner?.type ?? "text edit";
  return { kind: type, detail: inner?.text ? `"${inner.text.slice(0, 40)}"` : "" };
}

/** § 23b PROFILE — a person as their op log.
 *
 *  Titles, topics and tags are NOT in this payload: they live in
 *  document-service's schema and collaboration-service does not reach
 *  across it. The screen joins them from the link graph it already
 *  fetches, the same way § 18b's audit rows get their titles. */
export interface Profile {
  actor_id: string;
  ops: number;
  pages: number;
  /** One entry per day the actor wrote anything. Silent days are ABSENT,
   *  not zero — the grid is drawn client-side and looks each date up. */
  daily: { day: string; ops: number }[];
  top_pages: { page_id: string; ops: number; last_touched: string }[];
  recent: { id: string; page_id: string; kind: string; seq: number; created_at: string }[];
  /** Who else has ops on the pages this person touched — pages in common,
   *  not ops in common. */
  most_edited_with: { actor_id: string; pages: number }[];
  /** The window every figure covers, stated rather than implied. */
  weeks: number;
}

export function getProfile(actorId: string): Promise<Profile> {
  return collabFetch(`${COLLAB_URL}/collab/people/${actorId}/profile`);
}
