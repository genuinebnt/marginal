import { wasmExecURL, wasmURL } from "../wasm-url";
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
   *  sample count. The bound exists so a run ends somewhere
   *  stated, and which bound it hit is part of the numbers
   *  meaning anything. (It used to also be what kept the tab
   *  alive: the benchmark ran on the main thread. It runs in a
   *  Web Worker now, so overrunning costs a worker's time, not
   *  the page's.) */
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

import { instantiateBench, type BenchWasmExports, type WasmResult } from "./runtime";
import type { BenchRequest } from "./worker";

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

/**
 * In the browser the benchmark runs in a Web Worker; under Node (the tests)
 * it runs right here.
 *
 * The reason is § 16's whole subject. Go compiled to wasm runs on the thread
 * that instantiated it, and the benchmark's budget is two seconds — two
 * seconds in which a main-thread run cannot paint, scroll or answer a click,
 * on a screen about latency. A worker costs one more instance of the module
 * (fetched from cache) and buys back the tab.
 *
 * Node keeps the direct path because a test has no worker and no tab, and
 * routing it through one would test the plumbing rather than the benchmark.
 */
let workerPort: Worker | null = null;
let nextId = 1;
const pending = new Map<number, (r: WasmResult) => void>();

function worker(): Worker {
  if (!workerPort) {
    workerPort = new Worker(new URL("./worker.ts", import.meta.url), { type: "module" });
    workerPort.onmessage = (e: MessageEvent<{ id: number; result: WasmResult }>) => {
      pending.get(e.data.id)?.(e.data.result);
      pending.delete(e.data.id);
    };
  }
  return workerPort;
}

function ask(req: Omit<BenchRequest, "id">): Promise<WasmResult> {
  const id = nextId++;
  return new Promise((resolve) => {
    pending.set(id, resolve);
    worker().postMessage({ ...req, id });
  });
}

let loadPromise: Promise<BenchWasmExports> | null = null;

/** The in-process instance — Node only. */
export function loadBenchCore(): Promise<BenchWasmExports> {
  if (!loadPromise) {
    loadPromise = instantiateBench(
      async () => isNode
        ? (await import("node:fs/promises")).readFile(
            new URL("../../public/wasm_exec.js", import.meta.url), "utf-8")
        : (await fetch(wasmExecURL())).text(),
      async () => {
        if (isNode) {
          const { readFile } = await import("node:fs/promises");
          const buf = await readFile(new URL("../../public/bench.wasm", import.meta.url));
          return buf.buffer.slice(buf.byteOffset, buf.byteOffset + buf.byteLength);
        }
        return (await fetch(wasmURL("bench"))).arrayBuffer();
      },
    );
  }
  return loadPromise;
}

export async function listWorkloads(): Promise<WorkloadInfo[]> {
  if (isNode) return unwrap<{ workloads: WorkloadInfo[] }>((await loadBenchCore()).benchList()).workloads;
  return unwrap<{ workloads: WorkloadInfo[] }>(await ask({ kind: "list" })).workloads;
}

export async function runBench(workload: string, samples: number): Promise<BenchResult> {
  if (isNode) {
    return unwrap<BenchResult>((await loadBenchCore()).benchRun(JSON.stringify({ workload, samples })));
  }
  return unwrap<BenchResult>(await ask({ kind: "run", workload, samples }));
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
