/**
 * Loading cmd/benchwasm, shared by the two places that do it.
 *
 * § 16 runs in a Web Worker (see worker.ts) so a two-second benchmark does
 * not freeze the tab it is measuring. The Node path in wasm.ts loads the
 * same module directly, because a test has no worker and no tab. Both need
 * exactly this, so it lives here rather than twice.
 */
export interface WasmResult { value: string | null; error: string | null }

export interface BenchWasmExports {
  benchRun(reqJson: string): WasmResult;
  benchList(): WasmResult;
}

interface GoRuntime { importObject: WebAssembly.Imports; run(i: WebAssembly.Instance): Promise<void> }

export async function instantiateBench(
  execSource: () => Promise<string>,
  bytes: () => Promise<ArrayBuffer>,
): Promise<BenchWasmExports> {
  if (typeof (globalThis as { Go?: unknown }).Go === "undefined") {
    new Function(await execSource())();
  }
  const Go = (globalThis as unknown as { Go: new () => GoRuntime }).Go;
  const go = new Go();
  const { instance } = await WebAssembly.instantiate(await bytes(), go.importObject);
  void go.run(instance); // never resolves — Go's main() blocks in select{}
  return globalThis as unknown as BenchWasmExports;
}
