import { useCallback, useEffect, useRef, useState } from "react";
import { COLLAB_URL } from "../api/config";
import type { Mark } from "./marks";
import type { AnchorRange, BlockKind, ClientMessage, PageOp, ServerMessage } from "./types";

export type ConnectionState = "connecting" | "open" | "closed";

export interface BlockView {
  id: string;
  kind: BlockKind;
  text: string;
  marks: Mark[];
}

/** One peer's current caret/selection — see docs/api/collaboration.md's
 * "cursor" frame. start === end is a plain caret, not a selection.
 * Offsets are rune offsets into the block's live text, same unit as
 * InsertText/DeleteText — not yet UTF-16-safe for multi-byte text
 * client-side (RichEditorPane builds these from JS string indices), the
 * same accepted simplification marks.ts's own doc comment already notes. */
export interface PeerCursor {
  blockId: string;
  start: number;
  end: number;
}

export interface CollabPage {
  state: ConnectionState;
  /** True once the initial "snapshot" frame has actually been processed —
   * distinct from state === "open", which only means the socket handshake
   * finished. blocks is meaningless (always []) until this is true, since
   * the socket opens strictly before the server's first frame arrives; a
   * caller that wants to tell "genuinely empty page" apart from "snapshot
   * hasn't arrived yet" must check this, not blocks.length alone. */
  ready: boolean;
  blocks: BlockView[];
  /** Actor ids currently connected to this page, other than our own —
   * real presence (docs/api/collaboration.md's "presence" frame and the
   * snapshot's "present" list), not a heuristic inferred from who has
   * edited: seeded from the snapshot, updated on every join/leave. */
  peers: Set<string>;
  /** Other peers' current caret/selection, keyed by actor id — absent
   * means that peer isn't focused in any block right now. Seeded from the
   * snapshot's "cursors" list, updated on every "cursor" frame, and
   * cleared for a peer who leaves (belt-and-suspenders: the server itself
   * clears a departing actor's cursor too, on unsubscribe). */
  cursors: Map<string, PeerCursor>;
  /** Reports this client's own current caret/selection, or clears it
   * (blockId null) when focus leaves every block. Fire-and-forget, no
   * ack — RichEditorPane calls this from the same selectionchange
   * listener that already drives the bubble menu. */
  setCursor: (blockId: string | null, start: number, end: number) => void;
  /** Replaces one block's whole text — the same "replace everything"
   * strategy docs/api/collaboration.md documents, now scoped per block.
   * A block with any marks routes through setBlockContent instead (see
   * RichEditorPane) — this only ever touches text, never marks. */
  setBlockText: (blockId: string, newText: string) => void;
  /** Replaces one block's whole text AND marks atomically, via
   * SetBlockContent — the only op that can carry marks at all (RFC-001;
   * internal/doctext's live rope has no mark storage of its own). A block
   * that has ever had a mark applied uses this for every edit from then
   * on, not just mark changes — see RichEditorPane's own doc comment for
   * why (marks would silently desync from a block's live text otherwise). */
  setBlockContent: (blockId: string, newText: string, newMarks: Mark[]) => void;
  /** Inserts a fresh, empty block of kind right after afterId (or at the
   * document start if afterId is null), returning the new block's
   * client-generated id so the caller can focus it once it appears. */
  insertBlock: (afterId: string | null, kind: BlockKind) => string;
  /** Removes a block entirely. Refuses (no-op) if it's the last block —
   * a page always has at least one block in this editor. */
  deleteBlock: (blockId: string) => void;
  /** Changes a block's kind in place (paragraph ↔ heading ↔ quote ↔
   * code_block ↔ divider) — does not touch its content. */
  setBlockKind: (blockId: string, kind: BlockKind) => void;
  /** Moves blockId to immediately after afterId (null = the document
   * start) — drag-to-reorder, MoveBlock (RFC-002 §2). */
  moveBlock: (blockId: string, afterId: string | null) => void;
}

interface liveBlock {
  kind: BlockKind;
  text: string;
  marks: Mark[];
  boundaries: AnchorRange | null;
}

/**
 * One page's live collaborative session, block-aware — the rich editor's
 * data layer, built directly on internal/pageop's wire protocol (both
 * ISA tiers: structural block ops and per-block character ops). Unlike
 * useCollabSocket (the earlier single-implicit-block compatibility
 * shim this hook replaces), every block on the page is tracked and
 * editable.
 *
 * Ordering and identity live in two refs kept in lockstep: `order` (block
 * ids, document order) and `live` (id → current kind/text/boundaries).
 * Structural ops (insert/delete/kind-change) apply directly against
 * `order`; text ops only ever touch one block's entry in `live`. Both are
 * driven from the server's own frames (snapshot/ack/broadcast) — there is
 * no separate optimistic local model to reconcile, the same choice
 * useCollabSocket made and for the same reason: every op round-trips in
 * well under a keystroke's worth of local dev latency.
 */
export function useCollabPage(pageId: string, actorId: string): CollabPage {
  const [state, setState] = useState<ConnectionState>("connecting");
  const [ready, setReady] = useState(false);
  const [blocks, setBlocks] = useState<BlockView[]>([]);
  const [peers, setPeers] = useState<Set<string>>(new Set());
  const [cursors, setCursors] = useState<Map<string, PeerCursor>>(new Map());

  const orderRef = useRef<string[]>([]);
  const liveRef = useRef<Map<string, liveBlock>>(new Map());
  const socketRef = useRef<WebSocket | null>(null);
  // How many of THIS block's own text-scope ops are still awaiting their
  // ack — see setBlockText and the "ack" case below. Absent/0 means no
  // batch is in flight.
  const pendingTextAcksRef = useRef<Map<string, number>>(new Map());

  const publish = useCallback(() => {
    setBlocks(orderRef.current.map((id) => {
      const b = liveRef.current.get(id)!;
      return { id, kind: b.kind, text: b.text, marks: b.marks };
    }));
  }, []);

  useEffect(() => {
    setState("connecting");
    setReady(false);
    orderRef.current = [];
    liveRef.current = new Map();
    pendingTextAcksRef.current = new Map();
    setBlocks([]);
    setPeers(new Set());
    setCursors(new Map());

    const url = new URL(`${COLLAB_URL}/collab/pages/${pageId}`);
    url.protocol = url.protocol === "https:" ? "wss:" : "ws:";
    // The browser WebSocket API has no mechanism to set custom headers on
    // the upgrade request, so the actor id rides as a query param instead
    // of the X-Actor-Id header pages.md/auth.md use — collaboration.md §1
    // documents both forms; the header exists for non-browser callers.
    url.searchParams.set("actor_id", actorId);
    const ws = new WebSocket(url);
    socketRef.current = ws;

    ws.onopen = () => setState("open");
    ws.onclose = () => setState("closed");
    ws.onerror = () => setState("closed");

    function indexOf(id: string): number {
      return orderRef.current.indexOf(id);
    }
    function insertAfter(id: string, after: string | null) {
      const idx = after === null ? 0 : indexOf(after) + 1;
      orderRef.current.splice(idx, 0, id);
    }

    function applyStructural(op: PageOp & { scope: "block" }) {
      switch (op.type) {
        case "InsertBlock":
          if (!liveRef.current.has(op.id)) {
            liveRef.current.set(op.id, { kind: op.kind, text: op.content.text, marks: op.content.marks ?? [], boundaries: null });
            insertAfter(op.id, op.after);
          }
          break;
        case "DeleteBlock": {
          const id = op.tombstone.id;
          orderRef.current = orderRef.current.filter((x) => x !== id);
          liveRef.current.delete(id);
          break;
        }
        case "SetBlockKind": {
          const b = liveRef.current.get(op.id);
          if (b) b.kind = op.to;
          break;
        }
        case "SetBlockContent": {
          const b = liveRef.current.get(op.block);
          if (b) {
            b.text = op.content.text;
            b.marks = op.content.marks ?? [];
            b.boundaries = null; // the backend reseeds a fresh rope on SetBlockContent — old boundaries no longer name anything
          }
          break;
        }
        case "MoveBlock": {
          orderRef.current = orderRef.current.filter((x) => x !== op.id);
          insertAfter(op.id, op.to);
          break;
        }
        case "SetTitle":
          break; // not surfaced by this editor yet — see docs/porting/PROGRESS.md
      }
    }

    ws.onmessage = (event) => {
      const msg = JSON.parse(event.data as string) as ServerMessage;
      switch (msg.type) {
        case "snapshot": {
          orderRef.current = msg.snapshot.blocks.map((b) => b.id);
          liveRef.current = new Map(
            msg.snapshot.blocks.map((b) => [b.id, { kind: b.kind, text: b.text, marks: b.marks ?? [], boundaries: b.boundaries ?? null }]),
          );
          publish();
          setPeers(new Set(msg.present ?? []));
          setCursors(new Map(
            (msg.cursors ?? [])
              .filter((c): c is typeof c & { block_id: string } => c.block_id !== null)
              .map((c) => [c.actor_id, { blockId: c.block_id, start: c.start, end: c.end }]),
          ));
          setReady(true);
          break;
        }
        // ack confirms MY OWN just-submitted op; broadcast is someone
        // ELSE's. Both used to publish() unconditionally and identically —
        // which caused a real, reproduced bug for a "text"-scope ack
        // specifically: setBlockText's delete-then-insert "replace this
        // block's whole text" strategy (docs/api/collaboration.md) sends
        // TWO ops once a block already has prior boundaries, and each got
        // its own ack, published separately. The DeleteText half's ack —
        // correctly, per the naive "anything but InsertText means empty"
        // reducer below, since the block genuinely IS empty server-side
        // for that instant mid-replace — got published on its own,
        // re-rendering the block's contentEditable as empty for the
        // moment before the InsertText ack arrived and refilled it.
        // EditableTextBlock's caret-preserving sync effect saves the
        // selection's offsets before a content write and restores them
        // after, but an emptied element has no text nodes left to
        // reattach a saved offset to, so the restore silently no-ops and
        // the caret lands at the browser's default — offset 0. Found
        // live: "cursor now reset to start," specifically on the SECOND
        // edit to a block (the first InsertText has no prior boundaries
        // yet, so setBlockText sends only one op and this gap never
        // opens). Fixed by coalescing: pendingTextAcksRef tracks how many
        // acks setBlockText's current batch (1 or 2 ops) is still owed,
        // and only the LAST one of the batch triggers publish() — so
        // `blocks` (and anything reading it, like InspectorRail's
        // Outline) still ends up correct, just never exposed to the
        // transient all-empty half-state in between.
        case "ack": {
          const op = msg.op.op;
          if (op.scope === "block") {
            applyStructural(op);
            publish();
          } else {
            const b = liveRef.current.get(op.block);
            if (b) {
              b.boundaries = msg.boundaries ?? null;
              b.text = op.op.type === "InsertText" ? op.op.text : "";
            }
            const remaining = (pendingTextAcksRef.current.get(op.block) ?? 1) - 1;
            if (remaining <= 0) {
              pendingTextAcksRef.current.delete(op.block);
              publish();
            } else {
              pendingTextAcksRef.current.set(op.block, remaining);
            }
          }
          break;
        }
        case "broadcast": {
          const op = msg.op.op;
          if (op.scope === "block") {
            applyStructural(op);
          } else {
            const b = liveRef.current.get(op.block);
            if (b) {
              b.boundaries = msg.boundaries ?? null;
              b.text = op.op.type === "InsertText" ? op.op.text : "";
            }
          }
          publish();
          break;
        }
        case "presence": {
          setPeers((prev) => {
            const next = new Set(prev);
            if (msg.joined) next.add(msg.actor_id);
            else next.delete(msg.actor_id);
            return next;
          });
          if (!msg.joined) {
            // Belt-and-suspenders: the server already broadcasts a
            // clearing "cursor" frame when an actor's last connection
            // closes, but dropping it here too means a stale caret can
            // never survive a missed/reordered frame.
            setCursors((prev) => {
              if (!prev.has(msg.actor_id)) return prev;
              const next = new Map(prev);
              next.delete(msg.actor_id);
              return next;
            });
          }
          break;
        }
        case "cursor": {
          setCursors((prev) => {
            const next = new Map(prev);
            if (msg.cursor.block_id === null) {
              next.delete(msg.cursor.actor_id);
            } else {
              next.set(msg.cursor.actor_id, { blockId: msg.cursor.block_id, start: msg.cursor.start, end: msg.cursor.end });
            }
            return next;
          });
          break;
        }
        case "error":
          console.error("collab error:", msg.message);
          break;
      }
    };

    return () => ws.close();
  }, [pageId, actorId, publish]);

  const send = useCallback((msg: ClientMessage) => {
    socketRef.current?.send(JSON.stringify(msg));
  }, []);

  const setCursor = useCallback(
    (blockId: string | null, start: number, end: number) => {
      send({ type: "cursor", cursor: { block_id: blockId, start, end } });
    },
    [send],
  );

  const setBlockText = useCallback(
    (blockId: string, newText: string) => {
      const b = liveRef.current.get(blockId);
      if (!b) return;
      let opsSent = 0;
      if (b.boundaries) {
        send({ type: "op", op: { scope: "text", block: blockId, op: { type: "DeleteText", range: b.boundaries, text: "" } } });
        opsSent++;
      }
      if (newText.length > 0) {
        send({ type: "op", op: { scope: "text", block: blockId, op: { type: "InsertText", at: null, text: newText } } });
        opsSent++;
      }
      // Registers how many acks this batch owes — see the "ack" case's
      // own doc comment for why this matters (coalescing a delete+insert
      // replace into one visible update instead of two).
      if (opsSent > 0) {
        pendingTextAcksRef.current.set(blockId, (pendingTextAcksRef.current.get(blockId) ?? 0) + opsSent);
      }
    },
    [send],
  );

  const setBlockContent = useCallback(
    (blockId: string, newText: string, newMarks: Mark[]) => {
      const b = liveRef.current.get(blockId);
      if (!b) return;
      send({
        type: "op",
        op: {
          scope: "block",
          type: "SetBlockContent",
          block: blockId,
          prev: { text: b.text, marks: b.marks },
          content: { text: newText, marks: newMarks },
        },
      });
    },
    [send],
  );

  const insertBlock = useCallback(
    (afterId: string | null, kind: BlockKind) => {
      const id = crypto.randomUUID();
      send({
        type: "op",
        op: { scope: "block", type: "InsertBlock", id, after: afterId, kind, content: { text: "" } },
      });
      return id;
    },
    [send],
  );

  const deleteBlock = useCallback(
    (blockId: string) => {
      if (orderRef.current.length <= 1) return; // always keep at least one block
      const b = liveRef.current.get(blockId);
      if (!b) return;
      const idx = orderRef.current.indexOf(blockId);
      const after = idx > 0 ? orderRef.current[idx - 1] : null;
      send({
        type: "op",
        op: { scope: "block", type: "DeleteBlock", tombstone: { id: blockId, kind: b.kind, content: { text: b.text, marks: b.marks } }, after },
      });
    },
    [send],
  );

  const setBlockKind = useCallback(
    (blockId: string, kind: BlockKind) => {
      const b = liveRef.current.get(blockId);
      if (!b) return;
      send({ type: "op", op: { scope: "block", type: "SetBlockKind", id: blockId, from: b.kind, to: kind } });
    },
    [send],
  );

  const moveBlock = useCallback(
    (blockId: string, afterId: string | null) => {
      if (blockId === afterId) return;
      const idx = orderRef.current.indexOf(blockId);
      if (idx === -1) return;
      const from = idx > 0 ? orderRef.current[idx - 1] : null;
      if (from === afterId) return; // already there
      send({ type: "op", op: { scope: "block", type: "MoveBlock", id: blockId, from, to: afterId } });
    },
    [send],
  );

  return { state, ready, blocks, peers, cursors, setCursor, setBlockText, setBlockContent, insertBlock, deleteBlock, setBlockKind, moveBlock };
}
