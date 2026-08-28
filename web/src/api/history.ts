// collaboration-service's own plain HTTP endpoints (docs/api/
// collaboration.md §5-§7) — reached directly, never through api-gateway,
// same convention its WebSocket already uses (a persistent connection
// isn't a request/response resource, and these read-only debug/history
// endpoints were never given the api-gateway's REST-shim treatment
// either). Unlike pages.md/auth.md/graph.md's REST layer, these return a
// plain-text body on error (net/http.Error, not the {error, message}
// JSON shape ../api/http.ts's apiFetch expects) — collabFetch handles
// that directly instead of reusing apiFetch and mismatching its contract.
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
  const res = await fetch(url);
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
