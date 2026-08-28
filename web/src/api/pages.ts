// pages.md §2's REST mapping.
import { GATEWAY_URL } from "./config";
import { apiFetch } from "./http";

export interface Page {
  id: string;
  created_by: string;
  title: string;
  parent_id: string | null;
  path: string;
  sort_key: string;
  lifecycle_state: string;
  created_at: string;
  updated_at: string;
  deleted_at?: string;
  /** v2.7.0 — absent when untopiced, a real reported state. */
  topic?: PageTopic | null;
  tags?: string[];
}

export function createPage(actorId: string, title: string, parentId?: string, after?: string): Promise<Page> {
  return apiFetch<Page>(`${GATEWAY_URL}/pages`, {
    method: "POST",
    body: { title, parent_id: parentId, after },
    actorId,
  });
}

// parentId omitted lists root pages — ListPages is a filter, not a
// subtree walk (pages.md § List: "direct children only"), so a full tree
// is built by calling this once per expanded node, not once for
// everything (usePageTree.ts).
export interface PageTopic {
  id: string;
  name: string;
  color_key: string;
}

export function listPages(actorId: string, parentId?: string): Promise<{ pages: Page[]; next_cursor?: string }> {
  const url = new URL(`${GATEWAY_URL}/pages`);
  if (parentId) url.searchParams.set("parent_id", parentId);
  return apiFetch(url.toString(), { actorId });
}

export function getPage(actorId: string, id: string): Promise<Page> {
  return apiFetch<Page>(`${GATEWAY_URL}/pages/${id}`, { actorId });
}

export function renamePage(actorId: string, id: string, title: string): Promise<Page> {
  return apiFetch<Page>(`${GATEWAY_URL}/pages/${id}/title`, { method: "PATCH", body: { title }, actorId });
}

/** parentId "" promotes to a root page — ReparentPage's own contract
 * (pages.md): the key must always be present, never omitted, since an
 * absent key there means "leave the parent alone," which this client
 * never wants (it always calls this to actually change something). */
export function reparentPage(actorId: string, id: string, parentId: string, after?: string): Promise<Page> {
  return apiFetch<Page>(`${GATEWAY_URL}/pages/${id}/parent`, {
    method: "PATCH",
    body: { parent_id: parentId, after },
    actorId,
  });
}

export function deletePage(actorId: string, id: string): Promise<void> {
  return apiFetch(`${GATEWAY_URL}/pages/${id}`, { method: "DELETE", actorId });
}

export interface Backlink {
  from_page: string;
  from_page_title: string;
  from_page_deleted: boolean;
  target_title: string;
}

export function getBacklinks(actorId: string, id: string): Promise<{ backlinks: Backlink[] }> {
  return apiFetch(`${GATEWAY_URL}/pages/${id}/backlinks`, { actorId });
}
