import { wasmExecURL, wasmURL } from "../wasm-url";
// Loads cmd/mdcwasm and exposes the paste-and-import pipeline — the same
// JSON-bridge shape as document-core/, graph-core/, diff-core/, trie-core/
// and syntax-core/.
//
// No algorithm lives here. The lexer, parser, lowering, emission and the
// round-trip check are marginal/mdc in Go; this marshals across the boundary.

export interface Token {
  kind: string;
  start: number;
  end: number;
  text?: string;
  level?: number;
  indent?: number;
  lang?: string;
  checked?: boolean;
}

export interface MarkKind { tag: string; url?: string; page?: string }
export interface Mark { kind: MarkKind; start: number; end: number }
export interface Content { text: string; marks: Mark[] }

export interface BlockKind {
  tag: string;
  level?: number;
  language?: string;
  list_kind?: string;
  checked?: boolean;
}

export interface Block {
  id: string;
  parent?: string;
  kind: BlockKind;
  content: Content;
}

/** One emitted op, already in `internal/pageop`'s wire shape — so paste can
 *  send what the compiler produced without translating it. */
export interface CompiledOp {
  scope: "block";
  type: "InsertBlock";
  id: string;
  parent: string | null;
  after: string | null;
  kind: BlockKind;
  content: Content;
}

export interface Diagnostic { message: string; line: number }

export interface Stats {
  chars: number;
  bytes: number;
  divergences: string[];
  tokens: number;
  blocks: number;
  ops: number;
  lex_ns: number;
  parse_ns: number;
  emit_ns: number;
  replay_ns: number;
}

export interface CompileResult {
  tokens: Token[];
  tree: { blocks: Block[] };
  ops: CompiledOp[];
  /** The tree rebuilt from `ops`. `holds` is whether it matched. */
  replayed: { blocks: Block[] };
  holds: boolean;
  mismatch?: string;
  diagnostics: Diagnostic[];
  stats: Stats;
}

interface WasmResult { value: string | null; error: string | null }
interface MdcWasmExports { mdcCompile(reqJson: string): WasmResult }
interface GoRuntime { importObject: WebAssembly.Imports; run(i: WebAssembly.Instance): Promise<void> }

export class MdcCoreError extends Error {
  constructor(message: string) {
    super(message);
    this.name = "MdcCoreError";
  }
}

function unwrap<T>(result: WasmResult): T {
  if (result.error !== null) throw new MdcCoreError(result.error);
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
    const buf = await readFile(new URL("../../public/mdc.wasm", import.meta.url));
    return buf.buffer.slice(buf.byteOffset, buf.byteOffset + buf.byteLength);
  }
  const res = await fetch(wasmURL("mdc"));
  return res.arrayBuffer();
}

let loadPromise: Promise<MdcWasmExports> | null = null;

export function loadMdcCore(): Promise<MdcWasmExports> {
  if (!loadPromise) loadPromise = instantiate();
  return loadPromise;
}

async function instantiate(): Promise<MdcWasmExports> {
  await loadGoRuntime();
  const Go = (globalThis as unknown as { Go: new () => GoRuntime }).Go;
  const go = new Go();
  const bytes = await fetchWasmBytes();
  const { instance } = await WebAssembly.instantiate(bytes, go.importObject);
  void go.run(instance); // never resolves — Go's main() blocks in select{}
  return globalThis as unknown as MdcWasmExports;
}

export async function compile(src: string): Promise<CompileResult> {
  const api = await loadMdcCore();
  return unwrap<CompileResult>(api.mdcCompile(JSON.stringify({ src })));
}
