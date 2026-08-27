// docs/api/graph.md §2's REST mapping.
import { GATEWAY_URL } from "./config";
import { apiFetch } from "./http";

export interface GraphNode {
  id: string;
  title: string;
  is_root: boolean;
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
  component_of: Record<string, number>;
  orphan_components: number[];
  cycle: string[];
  diameter: number;
  betti: BettiNumbers;
}

export interface GraphNeighborhood {
  undirected_distance: Record<string, number>;
  forward_reachable: Record<string, number>;
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
