// Loads the Go/WASM diffwasm module and exposes a typed call — the same
// loader shape as ../graph-core/wasm.ts (this repo's third wasm module,
// not a third bridge design). No diff algorithm lives here: the LCS
// dynamic-programming table and its traceback both run in Go
// (services/textdiff, compiled by services/document-service/cmd/diffwasm)
// because diff.html's own "token granularity switching (word ↔
// character), recomputed live" needs interactive, client-side response
// to a toggle — the same reasoning graph-core/wasm.ts's own doc comment
// gives for the force layout and Voronoi/Delaunay (ADR-012). Tokenizing
// text into words or characters before calling in is this file's only
// job — textdiff itself never sees a whole document, only whichever
// token list the caller already split.

export type DiffKind = "equal" | "delete" | "insert";

export interface DiffOp {
  kind: DiffKind;
  token: string;
}

export interface Coord {
  i: number;
  j: number;
}

export interface DiffResult {
  table: number[][];
  ops: DiffOp[];
  /** Every (i, j) cell the real Go traceback walked, corner to origin —
   * diff.html's own DP-matrix "outlined path," drawn from what Go
   * actually visited, never re-derived here. */
  path: Coord[];
}

interface WasmResult {
  value: string | null;
  error: string | null;
}

interface DiffWasmExports {
  textDiff(reqJson: string): WasmResult;
}

interface GoRuntime {
  importObject: WebAssembly.Imports;
  run(instance: WebAssembly.Instance): Promise<void>;
}

export class DiffCoreError extends Error {
  constructor(message: string) {
    super(message);
    this.name = "DiffCoreError";
  }
}

function unwrap<T>(result: WasmResult): T {
  if (result.error !== null) throw new DiffCoreError(result.error);
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
    const buf = await readFile(new URL("../../public/diff.wasm", import.meta.url));
    return buf.buffer.slice(buf.byteOffset, buf.byteOffset + buf.byteLength);
  }
  const res = await fetch("/diff.wasm");
  return res.arrayBuffer();
}

let loadPromise: Promise<DiffWasmExports> | null = null;

/** Loads and instantiates diff.wasm exactly once per process; later
 * calls reuse the same running instance. */
export function loadDiffCore(): Promise<DiffWasmExports> {
  if (!loadPromise) loadPromise = instantiate();
  return loadPromise;
}

async function instantiate(): Promise<DiffWasmExports> {
  await loadGoRuntime();
  const Go = (globalThis as unknown as { Go: new () => GoRuntime }).Go;
  const go = new Go();

  const bytes = await fetchWasmBytes();
  const { instance } = await WebAssembly.instantiate(bytes, go.importObject);
  void go.run(instance); // never resolves — Go's main() blocks in select{}

  return globalThis as unknown as DiffWasmExports;
}

/** Real LCS diff (services/textdiff, unchanged) between two already-
 * tokenized sequences — word tokens (split on whitespace) or character
 * tokens (one entry per rune), the caller's choice, recomputed on every
 * call so toggling granularity in the UI is just calling this again
 * with a different tokenization. */
export async function diffTokens(a: string[], b: string[]): Promise<DiffResult> {
  const api = await loadDiffCore();
  return unwrap<DiffResult>(api.textDiff(JSON.stringify({ a, b })));
}

/** Splits on runs of whitespace, keeping the whitespace itself as its
 * own token (so re-joining ops' tokens with "" reconstructs the original
 * string exactly) — diff.html's own "word" granularity. */
export function tokenizeWords(s: string): string[] {
  return s.match(/\s+|\S+/g) ?? [];
}

/** One token per rune — diff.html's own "character" granularity. */
export function tokenizeChars(s: string): string[] {
  return Array.from(s);
}
