// docs/api/pages.md §2's classification routes (v2.7.0).
import { apiFetch } from "./http";
import { GATEWAY_URL } from "./config";

export interface Topic {
  id: string;
  name: string;
  /** A key into the design system's categorical ramp, never a hex value —
   *  the palette is the frontend's to own (DESIGN_GUIDELINES.md §3.4). */
  color_key: string;
  page_count?: number;
}

export interface TopicList {
  topics: Topic[];
  /** Pages with no topic. A real, reported state — not a gap to hide. */
  untopiced_pages: number;
}

export interface TagFacet {
  tag: string;
  page_count: number;
  /** Distinct topics this tag's pages span. A tag spanning several is
   *  naming a technique rather than a subject (ui-mockups § 10b). */
  topics_spanned: number;
}

export function getTopics(actorId: string): Promise<TopicList> {
  return apiFetch<TopicList>(`${GATEWAY_URL}/topics`, { actorId });
}

export function getTagFacets(actorId: string, limit = 40): Promise<{ facets: TagFacet[] }> {
  return apiFetch<{ facets: TagFacet[] }>(`${GATEWAY_URL}/tags?limit=${limit}`, { actorId });
}

/** null clears the assignment back to untopiced. */
export function setPageTopic(actorId: string, pageId: string, topicId: string | null) {
  return apiFetch(`${GATEWAY_URL}/pages/${pageId}/topic`, {
    method: "PUT", actorId, body: { topic_id: topicId },
  });
}

export function addPageTag(actorId: string, pageId: string, tag: string) {
  return apiFetch(`${GATEWAY_URL}/pages/${pageId}/tags`, {
    method: "POST", actorId, body: { tag },
  });
}

export function removePageTag(actorId: string, pageId: string, tag: string) {
  return apiFetch(`${GATEWAY_URL}/pages/${pageId}/tags/${encodeURIComponent(tag)}`, {
    method: "DELETE", actorId,
  });
}
