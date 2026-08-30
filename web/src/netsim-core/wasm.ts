import { wasmExecURL, wasmURL } from "../wasm-url";
// Loads cmd/netsimwasm and exposes § 14's simulation — the same
// JSON-bridge shape as every other wasm module here.
//
// No algorithm lives here. The transform, the rollback, the Merkle
// comparison, the causal DAG and the LSM shape are all
// marginal/netsim in Go. TypeScript sets four controls and draws
// what came back.

export type Kind = "insert" | "delete";

export interface Wire {
  rtt_ms: number;
  loss_pct: number;
  jitter_ms: number;
  /** Makes a bad network reproducible. A dropped packet you cannot
   *  re-run is an anecdote; the same seed twice is evidence. */
  seed: number;
}

export interface Op {
  id: string;
  actor: string;
  kind: Kind;
  pos: number;
  text?: string;
  len?: number;
  base: number;
  deps?: string[];
}

export interface Replica {
  actor: string;
  text: string;
  predicted: number;
  confirmed: number;
  rolled_back: number;
  pending: number;
}

export interface Delivery {
  op_id: string;
  actor: string;
  sent_at: number;
  arrives_at: number;
  attempt: number;
  lost?: boolean;
}

export interface MerkleNode {
  id: string;
  depth: number;
  hash: string;
  other_hash: string;
  equal: boolean;
  /** The highest node that already knows something is wrong — the
   *  answer the tree exists to give. */
  divergence: boolean;
  children?: number[];
  sample?: string;
}

export interface MerkleView {
  nodes: MerkleNode[];
  equal: boolean;
  /** What a real reconciliation would have had to fetch. This is
   *  the number the structure earns, not nodes.length. */
  compared_nodes: number;
  leaf_bytes: number;
}

export interface DAGNode {
  id: string;
  actor: string;
  label: string;
  deps?: string[];
  depth: number;
  on_longest: boolean;
}

export interface DAGView {
  nodes: DAGNode[];
  longest_chain: number;
  concurrent: number;
  width: number;
}

export interface LSMLevel {
  name: string;
  files: number;
  ops: number;
  compacting?: boolean;
}

export interface LSMView {
  levels: LSMLevel[];
  write_amplification: number;
  memtable_cap: number;
  fanout: number;
}

export interface Violation {
  op_id: string;
  actor: string;
  meant: string;
  got: string;
}

export interface Report {
  replicas: Replica[];
  server_text: string;
  converged: boolean;
  log: Op[];
  deliveries: Delivery[];
  lost: number;
  retransmits: number;
  ticks: number;
  merkle: MerkleView;
  causality: DAGView;
  lsm: LSMView;
  /** RFC-002's law, re-checked every run rather than asserted. */
  replay_matches: boolean;
  replay_text?: string;
  /** The second instrument. Structural agreement says the replicas
   *  hold the same bytes; this says whether those bytes are what
   *  anyone asked for. */
  intent_violations: Violation[] | null;
  intent_text?: string;
  skipped: number;
  edits: number;
}

interface WasmResult { value: string | null; error: string | null }
interface NetsimWasmExports { netsimRun(reqJson: string): WasmResult }
interface GoRuntime { importObject: WebAssembly.Imports; run(i: WebAssembly.Instance): Promise<void> }

export class NetsimCoreError extends Error {
  constructor(message: string) {
    super(message);
    this.name = "NetsimCoreError";
  }
}

function unwrap<T>(result: WasmResult): T {
  if (result.error !== null) throw new NetsimCoreError(result.error);
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
    const buf = await readFile(new URL("../../public/netsim.wasm", import.meta.url));
    return buf.buffer.slice(buf.byteOffset, buf.byteOffset + buf.byteLength);
  }
  const res = await fetch(wasmURL("netsim"));
  return res.arrayBuffer();
}

let loadPromise: Promise<NetsimWasmExports> | null = null;

export function loadNetsimCore(): Promise<NetsimWasmExports> {
  if (!loadPromise) loadPromise = instantiate();
  return loadPromise;
}

async function instantiate(): Promise<NetsimWasmExports> {
  await loadGoRuntime();
  const Go = (globalThis as unknown as { Go: new () => GoRuntime }).Go;
  const go = new Go();
  const bytes = await fetchWasmBytes();
  const { instance } = await WebAssembly.instantiate(bytes, go.importObject);
  void go.run(instance); // never resolves — Go's main() blocks in select{}
  return globalThis as unknown as NetsimWasmExports;
}

export async function runSim(args: {
  script: string;
  wire: Wire;
  transform: boolean;
  initial: string;
}): Promise<Report> {
  const api = await loadNetsimCore();
  return unwrap<Report>(api.netsimRun(JSON.stringify(args)));
}
