// docs/api/graph.md §2's REST mapping.
import { GATEWAY_URL } from "./config";
import { apiFetch } from "./http";

export interface GraphNode {
  id: string;
  title: string;
  is_root: boolean;
  /**
   * The page's declared topic — empty when untopiced, which is a real state
   * drawn in its own hue rather than one of the five.
   *
   * Carried on the node rather than joined from ListPages, and that is the
   * bug this field fixed: ListPages returns the direct children of ONE
   * parent, so a client joining it against this node set coloured only the
   * root pages and silently drew every nested page as untopiced. With a
   * nested corpus that was half the graph.
   */
  topic_name: string;
  topic_color_key: string;
  /** Free-form tags, for the same reason as the topic above: the tag filter
   *  joined ListPages too, and was therefore filtering over roots only. */
  tags: string[];
}

export interface GraphEdge {
  from_page: string;
  to_page: string;
}

export interface LinkGraph {
  nodes: GraphNode[];
  edges: GraphEdge[];
}

export interface BettiNumbers {
  b0: number;
  b1: number;
  b1_clique: number;
  b2: number;
  chi: number;
  triangles: number;
  rank2: number;
}

export interface GraphAnalysis {
  /** Per-node Brandes centrality over the undirected view, normalised to
   *  [0,1]. Always present, empty rather than null. */
  betweenness: Record<string, number>;
  /** Newman's Q against the DECLARED partition (topics)... */
  modularity_by_topic: number;
  /** ...and against the one the wiring implies. Read together: either alone
   *  is a number with nothing to compare it to. */
  modularity_by_component: number;
  component_of: Record<string, number>;
  orphan_components: number[];
  cycle: string[];
  diameter: number;
  betti: BettiNumbers;
  /**
   * Strong connectivity over the DIRECTED graph (Tarjan).
   *
   * A different question from `component_of`, which is why both ship:
   * `component_of` asks "can I walk between these two pages ignoring
   * direction"; this asks "can I walk there AND back following links as
   * written". A component of size > 1 is a set of pages citing each other in
   * a loop.
   */
  strongly_connected: Record<string, number>;
  /** Those components' sizes, largest first. All ones = no citation loops. */
  scc_sizes: number[];
  /** A reading order in which no page precedes one it links to (Kahn, ties on
   *  page id so it is stable across requests). PARTIAL when `is_dag` is
   *  false — `unplaced` then holds what could not be ordered. */
  topological_order: string[];
  is_dag: boolean;
  unplaced: string[];
  /** `topological_order` grouped into dependency LEVELS. Everything in one
   *  level can be read in any order; the number of levels is the longest
   *  dependency chain in the workspace. */
  layers: string[][];
}

export interface GraphNeighbour {
  page_id: string;
  title: string;
  hops: number;
}

export interface PathStep {
  page_id: string;
  title: string;
  /** The dependency LAYER, not the distance: two pages at the same depth can
   *  be read in either order. */
  depth: number;
  /** Always exactly the last step — the page the path was computed for. */
  destination: boolean;
}

export interface GraphNeighborhood {
  undirected_distance: Record<string, number>;
  forward_reachable: Record<string, number>;
  /** The ranked ring around the source, nearest first. Near BY LINKS —
   *  deliberately a different question from near by meaning, which /discover
   *  answers with cosine distance. The gap between the two answers is the
   *  finding. */
  nearest: GraphNeighbour[];
  /** ring_sizes[d] is how many pages sit exactly d hops out, from d = 0
   *  (the source itself). A frontier that stops growing is a graph that
   *  stops connecting. */
  ring_sizes: number[];
  /** "Read these, in this order" — everything that reaches this page by
   *  following links FORWARD, layered so a page with no prerequisites of its
   *  own comes first, ending at the page itself. A page nothing links to has
   *  a path of one step: itself, which is what "start here" looks like. */
  reading_path: PathStep[];
}

export function getLinkGraph(actorId: string): Promise<LinkGraph> {
  return apiFetch<LinkGraph>(`${GATEWAY_URL}/graph`, { actorId });
}

export function analyzeGraph(actorId: string): Promise<GraphAnalysis> {
  return apiFetch<GraphAnalysis>(`${GATEWAY_URL}/graph/analysis`, { actorId });
}

export function graphNeighborhood(actorId: string, sourcePageId: string): Promise<GraphNeighborhood> {
  return apiFetch<GraphNeighborhood>(`${GATEWAY_URL}/graph/neighborhood/${sourcePageId}`, { actorId });
}
