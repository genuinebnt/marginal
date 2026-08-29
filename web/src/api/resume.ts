// docs/api/pages.md §2's resume routes (v2.8.0).
//
// Resume is not a recent-files list. Recent files are derivable from
// updated_at and say only THAT a page changed; this says where you were in
// it, which nothing in the tree records.
import { apiFetch } from "./http";
import { GATEWAY_URL } from "./config";
import type { PageTopic } from "./pages";

export interface ReadingPosition {
  page_id: string;
  page_title: string;
  /** Absent when the page was opened but never clicked into — still a
   *  position worth resuming to. */
  block_id?: string | null;
  caret_start: number;
  caret_end: number;
  updated_at: string;
  topic?: PageTopic | null;
}

export function getResume(actorId: string, limit = 6): Promise<{ positions: ReadingPosition[] }> {
  return apiFetch(`${GATEWAY_URL}/resume?limit=${limit}`, { actorId });
}

export function savePosition(
  actorId: string,
  pageId: string,
  blockId: string | null,
  caretStart: number,
  caretEnd: number,
) {
  return apiFetch(`${GATEWAY_URL}/pages/${pageId}/position`, {
    method: "PUT",
    actorId,
    body: { block_id: blockId, caret_start: caretStart, caret_end: caretEnd },
  });
}
