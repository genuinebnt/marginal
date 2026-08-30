import { wasmExecURL, wasmURL } from "../wasm-url";
// Loads cmd/sketchwasm and exposes § 12's three sketches — the same
// JSON-bridge shape as every other wasm module here.
//
// No algorithm lives here. HyperLogLog, Count-Min and the t-digest are
// marginal/sketch in Go, and so are the EXACT answers they are shown beside:
// computing the truth in TypeScript and the estimate in Go would make the
// comparison a comparison of two languages rather than of a sketch against
// its own input.

export interface Counted {
  key: string;
  estimate: number;
  exact: number;
}

export interface Centroid { mean: number; weight: number }

export interface TopicReaders {
  topic: string;
  estimate: number;
  exact: number;
  /** The buffer's second half against its first, in percent — the only
   *  "vs prior window" a text box actually has. */
  delta_pct: number;
}

/** One TAG MOMENTUM row: reads of pages carrying this tag, late vs early. */
export interface TagMomentum {
  tag: string;
  recent: number;
  prior: number;
  delta_pct: number;
}

export interface Report {
  events: number;
  skipped: number;

  hll_estimate: number;
  hll_exact: number;
  hll_error_pct: number;
  /** The bound the structure itself promises, 1.04/√m as a percentage. An
   *  error inside it is the sketch working; outside it is a finding. */
  hll_standard_error: number;
  hll_registers: number[];
  hll_bytes: number;

  heavy: Counted[];
  cm_depth: number;
  cm_width: number;
  cm_bytes: number;
  cm_over_estimates: number;
  /** Must always be 0 — Count-Min's guarantee is one-sided. Shown, so the
   *  zero means something. */
  cm_under_estimates: number;

  p50?: number; p95?: number; p99?: number;
  exact_p50?: number; exact_p95?: number; exact_p99?: number;
  centroids: Centroid[];
  tdigest_bytes: number;

  by_topic: TopicReaders[];
  momentum: TagMomentum[];

  /** Every sketch added up — the number that does not grow with the stream. */
  total_bytes: number;
  /** What storing the raw stream would have cost. The ratio is the argument. */
  exact_bytes: number;
}

interface WasmResult { value: string | null; error: string | null }
interface SketchWasmExports { sketchAnalyze(reqJson: string): WasmResult }
interface GoRuntime { importObject: WebAssembly.Imports; run(i: WebAssembly.Instance): Promise<void> }

export class SketchCoreError extends Error {
  constructor(message: string) {
    super(message);
    this.name = "SketchCoreError";
  }
}

function unwrap<T>(result: WasmResult): T {
  if (result.error !== null) throw new SketchCoreError(result.error);
  return JSON.parse(result.value as string) as T;
}

const isNode = typeof process !== "undefined" && process.versions?.node != null;

async function loadGoRuntime(): Promise<void> {
  if (typeof (globalThis as { Go?: unknown }).Go !== "undefined") return;
  const source = isNode
    ? await (await import("node:fs/promises")).readFile(
        new URL("../../public/wasm_exec.js", import.meta.url), "utf-8")
    : await (await fetch(wasmExecURL())).text();
  new Function(source)();
}

async function fetchWasmBytes(): Promise<ArrayBuffer> {
  if (isNode) {
    const { readFile } = await import("node:fs/promises");
    const buf = await readFile(new URL("../../public/sketch.wasm", import.meta.url));
    return buf.buffer.slice(buf.byteOffset, buf.byteOffset + buf.byteLength);
  }
  const res = await fetch(wasmURL("sketch"));
  return res.arrayBuffer();
}

let loadPromise: Promise<SketchWasmExports> | null = null;

export function loadSketchCore(): Promise<SketchWasmExports> {
  if (!loadPromise) loadPromise = instantiate();
  return loadPromise;
}

async function instantiate(): Promise<SketchWasmExports> {
  await loadGoRuntime();
  const Go = (globalThis as unknown as { Go: new () => GoRuntime }).Go;
  const go = new Go();
  const bytes = await fetchWasmBytes();
  const { instance } = await WebAssembly.instantiate(bytes, go.importObject);
  void go.run(instance); // never resolves — Go's main() blocks in select{}
  return globalThis as unknown as SketchWasmExports;
}

export async function analyze(stream: string): Promise<Report> {
  const api = await loadSketchCore();
  return unwrap<Report>(api.sketchAnalyze(JSON.stringify({ stream })));
}
