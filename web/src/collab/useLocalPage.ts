/**
 * A CollabPage backed by documentcore in wasm, with no socket.
 *
 * `RichEditorPane` takes `collab: CollabPage` as a prop, not the hook —
 * so it depends on the interface, and `useCollabPage` (WebSocket to
 * collaboration-service) is only one implementation of it. This is the
 * other one: the same fourteen methods, applying ops to a local page
 * through the same Go core, and keeping every op it emitted.
 *
 * That is what makes the lab screens honest. They put the REAL editor on
 * screen — slash menu, marks, drag-reorder, the lot — and then show what
 * your typing actually produced. Not a demonstration that resembles the
 * editor: the editor, wired to a different sink.
 *
 * What it deliberately does NOT do:
 *
 *   - No network, no persistence. Reloading loses it, which is correct
 *     for a scratchpad and is said on screen.
 *   - No peers, no cursors. There is nobody else here; § 14 is where
 *     concurrency is demonstrated, with a real transform behind it.
 *   - No character tier. documentcore is the BLOCK tier; InsertText /
 *     DeleteText and their tombstones live in collaboration-service and
 *     have no wasm build, so a palimpsest cannot be driven from here.
 *     Stated rather than faked.
 */
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { applyOp, invertOp, newPage } from "../document-core/wasm";
import type {
  Block, BlockKind as CoreBlockKind, Content as CoreContent, Op, Page as CorePage,
} from "../document-core/types";
import type { CompiledOp } from "../mdc-core/wasm";
import type { Mark } from "./marks";
import type { BlockView, CollabPage, PeerCursor } from "./useCollabPage";

/** One entry of the op stream this page produced. */
export interface LocalOp {
  /** Monotonic, 1-based — what the trace scrubber indexes. */
  seq: number;
  /** The op as documentcore encodes it. */
  op: Op;
  /** The page immediately after it applied, for the scrubber. */
  after: CorePage;
  /** The op that undoes it, from the Go core. */
  inverse: Op;
  /**
   * RFC-002 §3, CHECKED rather than asserted: the inverse is actually
   * applied to the page this op produced, and the result compared with
   * the page from before it. § 13 prints this per step, and a screen
   * that printed it without running it would be making the same empty
   * claim as an audit log with a green tick and no hash chain.
   */
  lawHolds: boolean;
}

/** Compares what the law is about: the block sequence, each one's kind
 *  and text. Ids included — a reinserted block keeping its identity is
 *  exactly what makes an inverse an inverse. */
function samePage(a: CorePage, b: CorePage): boolean {
  const x = a.blocks ?? [], y = b.blocks ?? [];
  if (x.length !== y.length) return false;
  return x.every((bx, i) => {
    const by = y[i];
    return bx.id === by.id
      && JSON.stringify(bx.kind) === JSON.stringify(by.kind)
      && (bx.content?.text ?? "") === (by.content?.text ?? "");
  });
}

export interface LocalPage extends CollabPage {
  /** Everything this editor has emitted, oldest first. */
  ops: LocalOp[];
  /** Wipe back to an empty page. */
  reset: () => void;
  /** Set when an op was rejected by the core — a real answer about the
   *  model, so it is surfaced rather than swallowed. */
  rejected: string | null;
}

const uuid = () =>
  (globalThis.crypto?.randomUUID?.() ??
    `${Date.now().toString(16)}-${Math.random().toString(16).slice(2)}`);

/** documentcore's Page → the BlockView[] RichEditorPane renders. */
function toBlocks(page: CorePage | null): BlockView[] {
  return (page?.blocks ?? []).map((b: Block) => ({
    id: b.id,
    parent: b.parent ?? null,
    kind: b.kind as unknown as BlockView["kind"],
    text: b.content?.text ?? "",
    marks: (b.content?.marks ?? []) as unknown as Mark[],
  }));
}

export function useLocalPage(): LocalPage {
  const [page, setPage] = useState<CorePage | null>(null);
  const [ops, setOps] = useState<LocalOp[]>([]);
  const [rejected, setRejected] = useState<string | null>(null);
  const pageRef = useRef<CorePage | null>(null);
  const seq = useRef(0);

  /**
   * The seed block's id, held so a replay can rebuild the SAME empty page
   * the log was recorded against.
   *
   * The seed is not in the op stream (see start), so a replay that skipped
   * it would hand the editor a page with no blocks — and the editor, which
   * needs somewhere to put a caret, would insert one and that insert WOULD
   * be recorded. Stepping back grew the log by one instead of rewinding it.
   * A replay has to start from the same empty page, block id included.
   */
  const seedId = useRef(uuid());

  const emptyPage = useCallback(async () => {
    const fresh = await newPage(uuid(), "Scratch");
    return applyOp(fresh, {
      type: "InsertBlock", id: seedId.current,
      parent: null, after: null,
      kind: { tag: "paragraph" },
      content: { text: "", marks: [] },
    });
  }, []);

  const start = useCallback(() => {
    (async () => {
      const seeded = await emptyPage();
      pageRef.current = seeded;
      setPage(seeded);
      setOps([]);
      seq.current = 0;
    })().catch((e) => setRejected(String(e)));
  }, [emptyPage]);

  useEffect(() => { start(); }, [start]);

  /**
   * Apply one op through the Go core and record it.
   *
   * Every mutation below funnels through here, which is the point: there
   * is exactly one path from "the editor did something" to "the page
   * changed", and it goes through documentcore. A rejection is kept and
   * shown — the core refusing an op is information about the model, not
   * an error to hide.
   */
  const emit = useCallback(async (op: Op) => {
    const current = pageRef.current;
    if (!current) return;
    try {
      const next = await applyOp(current, op);

      // The law, run rather than claimed: undo the op on the page it
      // produced and see whether we are back where we started. Two extra
      // wasm calls per op, which is cheap next to being able to say the
      // check happened.
      const inverse = await invertOp(op);
      let lawHolds = false;
      try {
        lawHolds = samePage(await applyOp(next, inverse), current);
      } catch {
        lawHolds = false;
      }

      pageRef.current = next;
      seq.current += 1;
      setPage(next);
      setOps((prev) => [...prev, { seq: seq.current, op, after: next, inverse, lawHolds }]);
      setRejected(null);
    } catch (e) {
      setRejected(String(e instanceof Error ? e.message : e));
    }
  }, []);

  const blocks = useMemo(() => toBlocks(page), [page]);

  const blockById = useCallback(
    (id: string) => (pageRef.current?.blocks ?? []).find((b: Block) => b.id === id),
    [],
  );

  const api = useMemo<LocalPage>(() => ({
    state: "open",
    // A scratchpad has no space and no roles — there is nobody to be a
    // viewer to. "editor" rather than "" because the alternative would
    // have the editor render read-only on a page that is nothing but
    // yours, which would be a permission model invented for a page that
    // has none.
    role: "editor",
    canWrite: true,
    denied: rejected,
    ready: page !== null,
    blocks,
    peers: new Set<string>(),
    cursors: new Map<string, PeerCursor>(),
    ops,
    rejected,
    reset: start,

    setCursor: () => {},

    setBlockText: (blockId, newText) => {
      const b = blockById(blockId);
      if (!b) return;
      void emit({
        type: "SetBlockContent", block: blockId,
        prev: b.content, content: { text: newText, marks: [] },
      });
    },

    setBlockContent: (blockId, content) => {
      const b = blockById(blockId);
      if (!b) return;
      void emit({
        type: "SetBlockContent", block: blockId,
        prev: b.content, content: content as unknown as CoreContent,
      });
    },

    insertBlock: (afterId, kind, parent) => {
      // The id is generated HERE and returned, because RichEditorPane
      // uses it to move the caret into the new block. Returning void
      // would leave the caret where it was and read as "insert did
      // nothing".
      const id = uuid();
      void emit({
        type: "InsertBlock", id,
        parent: parent ?? null, after: afterId ?? null,
        kind: kind as unknown as CoreBlockKind,
        content: { text: "", marks: [] },
      });
      return id;
    },

    insertCompiled: (compiled: CompiledOp[], afterId: string | null) => {
      // Paste: the ops mdc already produced, replayed one at a time so
      // the stream shows what a paste really is — a batch of ordinary
      // ops, not a special case.
      void (async () => {
        let prev = afterId ?? null;
        for (const op of compiled) {
          const id = uuid();
          await emit({
            type: "InsertBlock", id,
            parent: null, after: prev,
            kind: (op as { kind: unknown }).kind as CoreBlockKind,
            content: (op as { content: unknown }).content as CoreContent,
          });
          prev = id;
        }
      })();
      return afterId;
    },

    deleteBlock: (blockId) => {
      const b = blockById(blockId);
      if (!b) return;
      const idx = (pageRef.current?.blocks ?? []).findIndex((x: Block) => x.id === blockId);
      const after = idx > 0 ? (pageRef.current?.blocks ?? [])[idx - 1].id : null;
      void emit({ type: "DeleteBlock", tombstone: b, after });
    },

    setBlockKind: (blockId, kind) => {
      const b = blockById(blockId);
      if (!b) return;
      void emit({
        type: "SetBlockKind", id: blockId,
        from: b.kind, to: kind as unknown as CoreBlockKind,
      });
    },

    moveBlock: (blockId, after, parent) => {
      const idx = (pageRef.current?.blocks ?? []).findIndex((x: Block) => x.id === blockId);
      const from = idx > 0 ? (pageRef.current?.blocks ?? [])[idx - 1].id : null;
      void emit({
        type: "MoveBlock", id: blockId,
        from, from_parent: null, to: after ?? null, to_parent: parent ?? null,
      });
    },

    undo: () => {},
    redo: () => {},

    /**
     * Rebuild the page from empty through the first `toStep` ops.
     *
     * Not a stub and not a snapshot: it replays, which is the same thing
     * the server does and the same thing the trace scrubber shows. The
     * op stream is kept intact so scrubbing back and forward is
     * non-destructive — the log is the truth, the page is a projection
     * of it.
     */
    restoreTo: (toStep: number) => {
      void (async () => {
        let acc = await emptyPage();
        for (const entry of ops.slice(0, Math.max(0, toStep))) {
          acc = await applyOp(acc, entry.op);
        }
        pageRef.current = acc;
        setPage(acc);
      })();
    },

    // Wire state, for a page with no wire.
    ackP99: null,
    queued: 0,
    attempt: 0,
    retryAt: null,
    retryNow: () => {},
  }), [blocks, ops, page, rejected, start, emit, blockById, emptyPage]);

  return api;
}
