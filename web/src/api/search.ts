// docs/api/search.md §2's REST mapping.
import { GATEWAY_URL } from "./config";
import { apiFetch } from "./http";

export interface SearchHit {
  page_id: string;
  page_title: string;
  block_id?: string;
  snippet?: string;
  rank: number;
}

export function search(actorId: string, query: string): Promise<{ hits: SearchHit[] }> {
  const url = new URL(`${GATEWAY_URL}/search`);
  url.searchParams.set("q", query);
  return apiFetch(url.toString(), { actorId });
}

export interface TitleSuggestion {
  page_id: string;
  title: string;
  distance: number;
}

export function suggestTitles(actorId: string, query: string, maxDistance = 2): Promise<{ suggestions: TitleSuggestion[] }> {
  const url = new URL(`${GATEWAY_URL}/search/suggest`);
  url.searchParams.set("q", query);
  url.searchParams.set("max_distance", String(maxDistance));
  return apiFetch(url.toString(), { actorId });
}
