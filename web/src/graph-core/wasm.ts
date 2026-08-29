// Loads the Go/WASM graphwasm module and exposes typed calls — the same
// loader shape as ../document-core/wasm.ts (this repo's second wasm
// module, not a second bridge design). No algorithm lives here: the
// force-directed layout and the exact Voronoi/Delaunay territory view
// both run in Go (services/document-service/internal/graphalgo,
// compiled by cmd/graphwasm) because they need interactive, client-side
// 60fps response to dragging — a network round trip per animation frame
// would be far too slow. Every one-shot algorithm (components, cycles,
// BFS, diameter, Betti) is server-side only, reached over
// docs/api/graph.md instead; this file has no business duplicating those.

import type { DelaunayPair, LayoutEdge, LayoutNode, LayoutParams, Site, Rect, TerritoryResult } from "./types";

interface WasmResult {
  value: string | null;
  error: string | null;
}

interface GraphWasmExports {
  graphSeedPositions(reqJson: string): WasmResult;
  graphLayoutTick(reqJson: string): WasmResult;
  graphTerritory(reqJson: string): WasmResult;
  graphHulls(reqJson: string): WasmResult;
  graphSpatialMajority(reqJson: string): WasmResult;
}

interface GoRuntime {
  importObject: WebAssembly.Imports;
  run(instance: WebAssembly.Instance): Promise<void>;
}

export class GraphCoreError extends Error {
  constructor(message: string) {
    super(message);
    this.name = "GraphCoreError";
  }
}

function unwrap<T>(result: WasmResult): T {
  if (result.error !== null) throw new GraphCoreError(result.error);
  return JSON.parse(result.value as string) as T;
}

const isNode = typeof process !== "undefined" && process.versions?.node != null;

async function loadGoRuntime(): Promise<void> {
  if (typeof (globalThis as { Go?: unknown }).Go !== "undefined") return;

  const source = isNode
    ? await (
        await import("node:fs/promises")
      ).readFile(new URL("../../public/wasm_exec.js", import.meta.url), "utf-8")
    : await (await fetch("/wasm_exec.js")).text();

  // wasm_exec.js is a classic (non-module) script that assigns
  // `globalThis.Go = class {...}` — running it via `new Function` executes
  // it with access to the real global object, same as a <script> tag would.
  new Function(source)();
}

async function fetchWasmBytes(): Promise<ArrayBuffer> {
  if (isNode) {
    const { readFile } = await import("node:fs/promises");
    const buf = await readFile(new URL("../../public/graph.wasm", import.meta.url));
    return buf.buffer.slice(buf.byteOffset, buf.byteOffset + buf.byteLength);
  }
  const res = await fetch("/graph.wasm");
  return res.arrayBuffer();
}

let loadPromise: Promise<GraphWasmExports> | null = null;

/** Loads and instantiates graph.wasm exactly once per process; later
 * calls reuse the same running instance. */
export function loadGraphCore(): Promise<GraphWasmExports> {
  if (!loadPromise) loadPromise = instantiate();
  return loadPromise;
}

async function instantiate(): Promise<GraphWasmExports> {
  await loadGoRuntime();
  const Go = (globalThis as unknown as { Go: new () => GoRuntime }).Go;
  const go = new Go();

  const bytes = await fetchWasmBytes();
  const { instance } = await WebAssembly.instantiate(bytes, go.importObject);
  void go.run(instance); // never resolves — Go's main() blocks in select{}

  return globalThis as unknown as GraphWasmExports;
}

export async function seedPositions(
  nodeIds: string[],
  seed: number,
  centerX: number,
  centerY: number,
  spread: number,
): Promise<LayoutNode[]> {
  const api = await loadGraphCore();
  return unwrap<LayoutNode[]>(
    api.graphSeedPositions(
      JSON.stringify({ node_ids: nodeIds, seed, center_x: centerX, center_y: centerY, spread }),
    ),
  );
}

export interface LayoutTickResult {
  nodes: LayoutNode[];
  alpha: number;
}

/** Advances the simulation by one step. params omitted uses
 * graphalgo.DefaultLayoutParams(); draggedId (if set) is left exactly
 * where the caller put it, never moved by the tick. */
export async function layoutTick(
  nodes: LayoutNode[],
  edges: LayoutEdge[],
  centerX: number,
  centerY: number,
  alpha: number,
  draggedId: string | null,
  params?: LayoutParams,
): Promise<LayoutTickResult> {
  const api = await loadGraphCore();
  return unwrap<LayoutTickResult>(
    api.graphLayoutTick(
      JSON.stringify({
        nodes,
        edges,
        params,
        center_x: centerX,
        center_y: centerY,
        alpha,
        dragged_id: draggedId ?? undefined,
      }),
    ),
  );
}

export async function territory(sites: Site[], bounds: Rect): Promise<TerritoryResult> {
  const api = await loadGraphCore();
  return unwrap<TerritoryResult>(api.graphTerritory(JSON.stringify({ sites, bounds })));
}

/** One settled node position, tagged with the group it belongs to. */
export interface HullPoint {
  group: string;
  x: number;
  y: number;
}

export interface Hull {
  group: string;
  points: Array<{ x: number; y: number }>;
}

/**
 * Convex hull per group — § 07's background territory polygons.
 *
 * Deliberately not `territory()` above, which is Voronoi: Voronoi partitions
 * the WHOLE plane between sites, so it answers "which page is nearest to this
 * pixel" and would hand empty space to whichever page happens to border it.
 * A hull covers only where a topic's pages actually are, and topics may
 * overlap — which is itself the finding, since two overlapping hulls are two
 * topics whose pages are interleaved.
 *
 * The geometry is graphalgo.Territories in Go; this is only the bridge.
 */
export async function hulls(points: HullPoint[], pad = 26): Promise<Hull[]> {
  const api = await loadGraphCore();
  const out = unwrap<{ hulls: Hull[] }>(api.graphHulls(JSON.stringify({ points, pad })));
  return out.hulls;
}

/**
 * § 07's SPACE lens: colour a node by what its spatial NEIGHBOURS are about,
 * not by what it declares.
 *
 * `adjacent` is the Delaunay dual `territory()` already returned — cells that
 * share a border — so this is a vote over spatial adjacency rather than over
 * citation. The three lenses on that screen exist precisely because they can
 * disagree, and the vote is graphalgo.NeighbourMajority in Go; this is only
 * the bridge.
 *
 * A node with no label and no labelled neighbour is ABSENT from the result
 * rather than mapped to "": untopiced is a state the caller draws in its own
 * hue, not a gap to fill with a guess.
 */
export async function spatialMajority(
  adjacent: DelaunayPair[],
  label: Record<string, string>,
): Promise<Record<string, string>> {
  const api = await loadGraphCore();
  const out = unwrap<{ majority: Record<string, string> }>(
    api.graphSpatialMajority(JSON.stringify({ adjacent, label })),
  );
  return out.majority;
}
