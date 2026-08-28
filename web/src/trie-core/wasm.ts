// Loads the Go/WASM triewasm module and exposes a typed call — the same
// loader shape as ../graph-core/wasm.ts and ../diff-core/wasm.ts (this
// repo's fourth wasm module, not a fourth bridge design). No prefix-
// search algorithm lives here: the trie itself runs in Go
// (services/document-service/internal/trie, compiled by
// document-service/cmd/triewasm) because `[[` autocomplete needs
// interactive, per-keystroke response (ADR-012) — this file only loads
// the module and forwards a JSON call.

export interface WasmResult {
  value: string | null;
  error: string | null;
}

interface TrieWasmExports {
  triePrefixSearch(reqJson: string): WasmResult;
}

interface GoRuntime {
  importObject: WebAssembly.Imports;
  run(instance: WebAssembly.Instance): Promise<void>;
}

export class TrieCoreError extends Error {
  constructor(message: string) {
    super(message);
    this.name = "TrieCoreError";
  }
}

function unwrap<T>(result: WasmResult): T {
  if (result.error !== null) throw new TrieCoreError(result.error);
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
    const buf = await readFile(new URL("../../public/trie.wasm", import.meta.url));
    return buf.buffer.slice(buf.byteOffset, buf.byteOffset + buf.byteLength);
  }
  const res = await fetch("/trie.wasm");
  return res.arrayBuffer();
}

let loadPromise: Promise<TrieWasmExports> | null = null;

/** Loads and instantiates trie.wasm exactly once per process; later
 * calls reuse the same running instance. */
export function loadTrieCore(): Promise<TrieWasmExports> {
  if (!loadPromise) loadPromise = instantiate();
  return loadPromise;
}

async function instantiate(): Promise<TrieWasmExports> {
  await loadGoRuntime();
  const Go = (globalThis as unknown as { Go: new () => GoRuntime }).Go;
  const go = new Go();

  const bytes = await fetchWasmBytes();
  const { instance } = await WebAssembly.instantiate(bytes, go.importObject);
  void go.run(instance); // never resolves — Go's main() blocks in select{}

  return globalThis as unknown as TrieWasmExports;
}

/** Real trie prefix search (services/trie, unchanged) over the current
 * title list — rebuilt fresh on every call (the module is stateless, see
 * cmd/triewasm's own doc comment on why that's the right tradeoff at
 * this repo's scale), so this can be called on every keystroke. */
export async function prefixSearch(titles: string[], prefix: string): Promise<string[]> {
  const api = await loadTrieCore();
  return unwrap<string[]>(api.triePrefixSearch(JSON.stringify({ titles, prefix })));
}
