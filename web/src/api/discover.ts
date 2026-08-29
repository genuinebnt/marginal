// docs/api/discover.md §2's REST mapping — § 09 DISCOVER's one route.
import { GATEWAY_URL } from "./config";
import { apiFetch } from "./http";

export interface SemanticNeighbour {
  page_id: string;
  title: string;
  /** The first ~120 characters of the INDEXED body — the same string the
   *  vector was built from, so what you read is what was scored. */
  excerpt: string;
  topic_name: string;
  topic_color_key: string;
  tags: string[];
  /** The three signals, deliberately never blended into one number. */
  cosine: number;
  shared_tags: number;
  tag_jaccard: number;
  /** -1 is UNREACHABLE, not "very far" — the two are different findings. */
  hops: number;
}

export interface DiscoverStats {
  comparisons: number;
  exact_comparisons: number;
  hops: number;
  layers: number;
  recall_at_k: number;
  candidates: number;
  layer_sizes: number[];
  corpus: number;
  top_terms: string[];
  m: number;
  ef_search: number;
  dimensions: number;
}

export interface NearResponse {
  neighbours: SemanticNeighbour[];
  stats: DiscoverStats;
  topics: string[];
}

export function discoverNear(
  actorId: string,
  pageId: string,
  opts: { k?: number; topics?: string[]; tags?: string[] } = {},
): Promise<NearResponse> {
  const url = new URL(`${GATEWAY_URL}/discover/${pageId}`);
  if (opts.k) url.searchParams.set("k", String(opts.k));
  if (opts.topics?.length) url.searchParams.set("topics", opts.topics.join(","));
  if (opts.tags?.length) url.searchParams.set("tags", opts.tags.join(","));
  return apiFetch<NearResponse>(url.toString(), { actorId });
}
