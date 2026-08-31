/**
 * § 16's benchmark, off the main thread.
 *
 * The benchmark's own budget is two seconds of wall clock, and Go compiled
 * to wasm runs on whichever thread instantiated it. On the main thread that
 * is two seconds during which the tab cannot paint, scroll, or answer a
 * click — while a screen whose subject is latency claims to be measuring
 * something. Here it is two seconds of a worker nobody is looking at.
 *
 * The budget still matters, and for the reason it always did: a run has to
 * end at a stated bound so the numbers say which bound they hit. What
 * changes is only who waits.
 *
 * The worker holds its own wasm instance. That is not a cost worth avoiding
 * — one instance per thread is how wasm works — but it does mean the module
 * is fetched again here, from the browser's cache, which is where the
 * immutable wasm URLs earn their keep.
 */
import { instantiateBench, type BenchWasmExports, type WasmResult } from "./runtime";
import { wasmExecURL, wasmURL } from "../wasm-url";

let api: Promise<BenchWasmExports> | null = null;
const load = () => (api ??= instantiateBench(
  async () => (await fetch(wasmExecURL())).text(),
  async () => (await fetch(wasmURL("bench"))).arrayBuffer(),
));

export interface BenchRequest {
  id: number;
  kind: "list" | "run";
  workload?: string;
  samples?: number;
}

self.onmessage = async (e: MessageEvent<BenchRequest>) => {
  const { id, kind, workload, samples } = e.data;
  let result: WasmResult;
  try {
    const bench = await load();
    result = kind === "list"
      ? bench.benchList()
      : bench.benchRun(JSON.stringify({ workload, samples }));
  } catch (err) {
    // A failure to load is still an answer, and a caller waiting on a
    // message that never arrives has no way to say so.
    result = { value: null, error: err instanceof Error ? err.message : String(err) };
  }
  (self as unknown as Worker).postMessage({ id, result });
};
