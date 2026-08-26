// Loads the Go/WASM documentcore module and exposes typed calls. This is
// the entire "logic" boundary from the TS side — everything below this
// file just marshals JSON across it. No document-core algorithm is
// reimplemented here; see services/document-service/cmd/wasm/main.go and
// internal/documentcore for what's on the other side.
//
// Works identically in the browser (fetches /documentcore.wasm, served
// from web/public/ by Vite) and under Vitest/Node (reads the same file
// from disk) — same loader code, no environment-specific test double.

import type { Op, Page } from "./types";

interface WasmResult {
  value: string | null;
  error: string | null;
}

interface DocumentCoreExports {
  documentcoreNewPage(reqJson: string): WasmResult;
  documentcoreApplyOp(pageJson: string, opJson: string): WasmResult;
  documentcoreInvertOp(opJson: string): WasmResult;
}

interface GoRuntime {
  importObject: WebAssembly.Imports;
  run(instance: WebAssembly.Instance): Promise<void>;
}

export class DocumentCoreError extends Error {
  constructor(message: string) {
    super(message);
    this.name = "DocumentCoreError";
  }
}

function unwrap<T>(result: WasmResult): T {
  if (result.error !== null) throw new DocumentCoreError(result.error);
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
    const buf = await readFile(new URL("../../public/documentcore.wasm", import.meta.url));
    return buf.buffer.slice(buf.byteOffset, buf.byteOffset + buf.byteLength);
  }
  const res = await fetch("/documentcore.wasm");
  return res.arrayBuffer();
}

let loadPromise: Promise<DocumentCoreExports> | null = null;

/** Loads and instantiates documentcore.wasm exactly once per process;
 * later calls reuse the same running instance. */
export function loadDocumentCore(): Promise<DocumentCoreExports> {
  if (!loadPromise) loadPromise = instantiate();
  return loadPromise;
}

async function instantiate(): Promise<DocumentCoreExports> {
  await loadGoRuntime();
  const Go = (globalThis as unknown as { Go: new () => GoRuntime }).Go;
  const go = new Go();

  const bytes = await fetchWasmBytes();
  const { instance } = await WebAssembly.instantiate(bytes, go.importObject);
  void go.run(instance); // never resolves — Go's main() blocks in select{}

  return globalThis as unknown as DocumentCoreExports;
}

export async function newPage(id: string, title: string): Promise<Page> {
  const api = await loadDocumentCore();
  return unwrap<Page>(api.documentcoreNewPage(JSON.stringify({ id, title })));
}

export async function applyOp(page: Page, op: Op): Promise<Page> {
  const api = await loadDocumentCore();
  return unwrap<Page>(api.documentcoreApplyOp(JSON.stringify(page), JSON.stringify(op)));
}

export async function invertOp(op: Op): Promise<Op> {
  const api = await loadDocumentCore();
  return unwrap<Op>(api.documentcoreInvertOp(JSON.stringify(op)));
}
