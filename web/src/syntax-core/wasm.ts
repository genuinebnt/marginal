// Loads cmd/syntaxwasm and exposes the code-block highlighter — the same
// JSON-bridge shape as document-core/, graph-core/, diff-core/ and trie-core/.
//
// No algorithm lives here. The lexer is marginal/syntax in Go (CLAUDE.md's
// rule: TypeScript draws what Go computed, never a second implementation),
// and this file only marshals across the boundary.
//
// Tokens carry TEXT rather than offsets, deliberately: offsets would be byte
// offsets on the Go side and UTF-16 indices here — the exact mismatch already
// documented as an open gap for marks — and a highlighter has no reason to
// inherit it. Concatenating every token reproduces the source exactly, which
// is the property the Go tests pin.

export type TokenKind =
  | "plain" | "keyword" | "type" | "string" | "number" | "comment" | "func" | "punct";

export interface Token {
  kind: TokenKind;
  text: string;
}

interface WasmResult {
  value: string | null;
  error: string | null;
}

interface SyntaxWasmExports {
  syntaxHighlight(reqJson: string): WasmResult;
  syntaxLanguages(): WasmResult;
}

interface GoRuntime {
  importObject: WebAssembly.Imports;
  run(instance: WebAssembly.Instance): Promise<void>;
}

export class SyntaxCoreError extends Error {
  constructor(message: string) {
    super(message);
    this.name = "SyntaxCoreError";
  }
}

function unwrap<T>(result: WasmResult): T {
  if (result.error !== null) throw new SyntaxCoreError(result.error);
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
    const buf = await readFile(new URL("../../public/syntax.wasm", import.meta.url));
    return buf.buffer.slice(buf.byteOffset, buf.byteOffset + buf.byteLength);
  }
  const res = await fetch("/syntax.wasm");
  return res.arrayBuffer();
}

let loadPromise: Promise<SyntaxWasmExports> | null = null;

export function loadSyntaxCore(): Promise<SyntaxWasmExports> {
  if (!loadPromise) loadPromise = instantiate();
  return loadPromise;
}

async function instantiate(): Promise<SyntaxWasmExports> {
  await loadGoRuntime();
  const Go = (globalThis as unknown as { Go: new () => GoRuntime }).Go;
  const go = new Go();
  const bytes = await fetchWasmBytes();
  const { instance } = await WebAssembly.instantiate(bytes, go.importObject);
  void go.run(instance); // never resolves — Go's main() blocks in select{}
  return globalThis as unknown as SyntaxWasmExports;
}

export async function highlight(lang: string, src: string): Promise<Token[]> {
  const api = await loadSyntaxCore();
  return unwrap<{ tokens: Token[] }>(api.syntaxHighlight(JSON.stringify({ lang, src }))).tokens;
}

export async function languages(): Promise<string[]> {
  const api = await loadSyntaxCore();
  return unwrap<string[]>(api.syntaxLanguages());
}
