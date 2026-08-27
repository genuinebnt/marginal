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

import type { LayoutEdge, LayoutNode, LayoutParams, Site, Rect, TerritoryResult } from "./types";

interface WasmResult {
  value: string | null;
  error: string | null;
}

interface GraphWasmExports {
  graphSeedPositions(reqJson: string): WasmResult;
  graphLayoutTick(reqJson: string): WasmResult;
  graphTerritory(reqJson: string): WasmResult;
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
