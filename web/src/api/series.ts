// docs/api/pages.md §2's series routes (v2.9.0).
//
// A SERIES IS A PAGE WITH CHILDREN — no series table, no `series_name`
// column, no second ordering. The page tree already says "these belong
// together, in this order", and dragging a row in the rail therefore
// reorders the series, for free.
import { apiFetch } from "./http";
import { GATEWAY_URL } from "./config";
import type { PageTopic } from "./pages";

export interface SeriesPart {
  page_id: string;
  title: string;
  /** 1-based, as printed ("Part 3 of 19"). 0 is never a valid part number. */
  number: number;
  word_count: number;
  topic?: PageTopic | null;
  tags: string[];
}

/** Three states, because they need different words on screen. */
export type Membership = "none" | "member" | "leader";

export interface PageSeries {
  membership: Membership;
  series_page_id: string;
  series_title: string;
  parts: SeriesPart[];
  /** 1-based position of the requested page; 0 when leader or none. */
  number: number;
}

export interface SeriesSummary {
  series_page_id: string;
  title: string;
  topic?: PageTopic | null;
  part_count: number;
  /** The whole series — the series page plus every part. */
  word_count: number;
  parts: SeriesPart[];
}

export function listSeries(actorId: string): Promise<{ series: SeriesSummary[] }> {
  return apiFetch(`${GATEWAY_URL}/series`, { actorId });
}

export function getPageSeries(actorId: string, pageId: string): Promise<PageSeries> {
  return apiFetch(`${GATEWAY_URL}/pages/${pageId}/series`, { actorId });
}
