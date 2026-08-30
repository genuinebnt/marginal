// Loads cmd/benchwasm and exposes § 16's benchmark — the same
// JSON-bridge shape as every other wasm module here.
//
// No algorithm lives here, and no timing either: the loop, the
// clock, the histogram, the percentiles and the call tree are all
// marginal/bench in Go. Timing in TypeScript and running the work
// in Go would measure the bridge.

export interface Bucket {
  lo_ns: number;
  hi_ns: number;
  count: number;
  label: string;
}

export interface Frame {
  name: string;
  depth: number;
  /** Excludes children — the number a flame graph is actually about. */
  self_ns: number;
  total_ns: number;
  fraction: number;
  calls: number;
}

export interface BenchResult {
  workload: string;
  samples: number;
  /** What actually ran: an expensive workload clamps the request,
   *  and the wall-clock budget can stop it earlier still. */
  ran: number;
  /** True when the run stopped on the clock rather than on the
   *  sample count. wasm holds the page's thread, so a run that
   *  overruns freezes the tab — the bound is part of the screen
   *  working, and which bound was hit is part of the numbers
   *  meaning anything. */
  budgeted: boolean;
  buckets: Bucket[];
  p50_ns: number;
  p95_ns: number;
  p99_ns: number;
  p999_ns: number;
  min_ns: number;
  max_ns: number;
  mean_ns: number;
  /** The smallest interval this host's clock will report. Every
   *  number above is quantised by it, so the screen says it. */
  clock_resolution_ns: number;
  /** How many iterations each timed sample covers. Greater than 1
   *  whenever one iteration is faster than the clock can see —
   *  the normal case in a browser, where performance.now() is
   *  deliberately coarsened. Each sample is then a MEAN over
   *  that many iterations, which narrows the distribution: a
   *  p99.9 over batch means is not a tail latency, and the
   *  screen says so rather than quietly implying otherwise. */
  batch_size: number;
  /** The highest quantile this sample count can support. A
   *  p99.9 over 29 samples is the maximum wearing a
   *  percentile's name; anything above this is greyed. */
  supported_quantile: number;
  total_ns: number;
  frames: Frame[];
  note: string;
}

export interface WorkloadInfo {
  name: string;
  note: string;
  max_samples: number;
}

interface WasmResult { value: string | null; error: string | null }
interface BenchWasmExports {
  benchRun(reqJson: string): WasmResult;
  benchList(): WasmResult;
}
interface GoRuntime { importObject: WebAssembly.Imports; run(i: WebAssembly.Instance): Promise<void> }

export class BenchCoreError extends Error {
  constructor(message: string) {
    super(message);
    this.name = "BenchCoreError";
  }
}

function unwrap<T>(result: WasmResult): T {
  if (result.error !== null) throw new BenchCoreError(result.error);
  return JSON.parse(result.value as string) as T;
}

const isNode = typeof process !== "undefined" && process.versions?.node != null;

async function loadGoRuntime(): Promise<void> {
  if (typeof (globalThis as { Go?: unknown }).Go !== "undefined") return;
  const source = isNode
    ? await (await import("node:fs/promises")).readFile(
        new URL("../../public/wasm_exec.js", import.meta.url), "utf-8")
    : await (await fetch("/wasm_exec.js")).text();
  new Function(source)();
}

async function fetchWasmBytes(): Promise<ArrayBuffer> {
  if (isNode) {
    const { readFile } = await import("node:fs/promises");
    const buf = await readFile(new URL("../../public/bench.wasm", import.meta.url));
    return buf.buffer.slice(buf.byteOffset, buf.byteOffset + buf.byteLength);
  }
  const res = await fetch("/bench.wasm");
  return res.arrayBuffer();
}

let loadPromise: Promise<BenchWasmExports> | null = null;

export function loadBenchCore(): Promise<BenchWasmExports> {
  if (!loadPromise) loadPromise = instantiate();
  return loadPromise;
}

async function instantiate(): Promise<BenchWasmExports> {
  await loadGoRuntime();
  const Go = (globalThis as unknown as { Go: new () => GoRuntime }).Go;
  const go = new Go();
  const bytes = await fetchWasmBytes();
  const { instance } = await WebAssembly.instantiate(bytes, go.importObject);
  void go.run(instance); // never resolves — Go's main() blocks in select{}
  return globalThis as unknown as BenchWasmExports;
}

export async function listWorkloads(): Promise<WorkloadInfo[]> {
  const api = await loadBenchCore();
  return unwrap<{ workloads: WorkloadInfo[] }>(api.benchList()).workloads;
}

export async function runBench(workload: string, samples: number): Promise<BenchResult> {
  const api = await loadBenchCore();
  return unwrap<BenchResult>(api.benchRun(JSON.stringify({ workload, samples })));
}

/** ns → the "2 ms" / "480 µs" form § 16 prints. Mirrors bench.Duration
 *  so the axis labels and the readouts agree; the Go side formats what
 *  it sends in `label`, this formats what the screen derives. */
export function dur(ns: number): string {
  if (!Number.isFinite(ns)) return "—";
  if (ns >= 1e9) return `${(ns / 1e9).toFixed(2)} s`;
  if (ns >= 1e6) return `${(ns / 1e6).toFixed(1)} ms`;
  if (ns >= 1e3) return `${(ns / 1e3).toFixed(1)} µs`;
  // Sub-nanosecond keeps a decimal: after dividing a batch total
  // by its size, 0.4 ns is a real number and "0 ns" reads as
  // "not measured".
  if (ns >= 10) return `${Math.round(ns)} ns`;
  return `${ns.toFixed(2)} ns`;
}
