// docs/api/pages.md §2's trash routes — v2.6.0's delete saga, made visible.
import { apiFetch } from "./http";
import { GATEWAY_URL } from "./config";
import type { Page } from "./pages";

export interface SagaProgress {
  steps_done: string[];
  steps_left: string[];
  /** > 1 means the saga resumed after a crash — the difference between a
   *  slow delete and an unstable one. */
  attempts: number;
  last_error?: string;
  /** Steps with no backing store at this repo's scope. Sent so the UI draws
   *  them as "no store yet" rather than as work performed. */
  not_applicable: string[];
}

export interface TrashEntry {
  page: Page;
  purge_at: string;
  /** Absent once the saga finishes — which is how a caller tells "deleted,
   *  restorable" from "deleting, mid-saga" without re-reading
   *  lifecycle_state. */
  progress?: SagaProgress;
}

export interface DeletePreview {
  descendants: Page[];
  /** Pages OUTSIDE the subtree whose [[links]] point into it. Those links do
   *  not break — they DANGLE, a state the graph already models. */
  referrers: Page[];
  block_count: number;
}

export function listTrash(actorId: string): Promise<{ entries: TrashEntry[]; total: number }> {
  return apiFetch(`${GATEWAY_URL}/trash`, { actorId });
}

export function previewDelete(actorId: string, pageId: string): Promise<DeletePreview> {
  return apiFetch(`${GATEWAY_URL}/pages/${pageId}/delete-preview`, { actorId });
}

export function restorePage(actorId: string, pageId: string): Promise<Page> {
  return apiFetch(`${GATEWAY_URL}/pages/${pageId}/restore`, { method: "POST", actorId });
}
