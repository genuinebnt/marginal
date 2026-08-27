// Wire types for cmd/graphwasm — field-for-field mirrors of
// internal/graphalgo's own JSON-tagged Go structs. No algorithm lives
// here; this file only names the shapes wasm.ts marshals across the
// wasm boundary.

export interface Point {
  x: number;
  y: number;
}

/** A page's live simulation state — position and velocity, updated one
 * layoutTick call at a time. */
export interface LayoutNode {
  id: string;
  x: number;
  y: number;
  vx: number;
  vy: number;
}

export interface LayoutParams {
  repel: number;
  spring_length: number;
  spring_k: number;
  center: number;
  damp: number;
}

export interface LayoutEdge {
  from: string;
  to: string;
}

/** One page's position for the Voronoi/Delaunay territory view. */
export interface Site {
  id: string;
  x: number;
  y: number;
}

export interface Rect {
  min_x: number;
  min_y: number;
  max_x: number;
  max_y: number;
}

export interface VoronoiCell {
  site: Site;
  poly: Point[];
}

export interface DelaunayPair {
  a: string;
  b: string;
}

export interface TerritoryResult {
  cells: VoronoiCell[];
  delaunay: DelaunayPair[];
}
