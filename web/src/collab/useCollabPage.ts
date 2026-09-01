import { useCallback, useEffect, useRef, useState } from "react";
import { accessToken } from "../api/http";
import { COLLAB_URL } from "../api/config";
import type { Mark } from "./marks";
import type { AnchorRange, BlockKind, ClientMessage, PageOp, ServerMessage } from "./types";
import type { CompiledOp } from "../mdc-core/wasm";

export type ConnectionState = "connecting" | "open" | "closed";

export interface BlockView {
  id: string;
  /** nil (null) means top-level — RFC-001 §1's containment, mirroring
   * documentcore.Block.Parent. blocks is kept in the same depth-first
   * order documentcore.Page.Blocks requires: a container immediately
   * precedes all its descendants, then the next sibling. */
  parent: string | null;
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
  /** Inserts a fresh, empty block of kind as parent's child (nil = top-
   * level), immediately after afterId (or as parent's first child — or
   * the document start, if parent is also nil — if afterId is null).
   * Returns the new block's client-generated id so the caller can focus
   * it once it appears. */
  insertBlock: (afterId: string | null, kind: BlockKind, parent?: string | null) => string;
  /** Paste: a compiled batch of InsertBlocks, one undo group, ids rewritten
   *  to UUIDs. Returns the last top-level block inserted. */
  insertCompiled: (ops: CompiledOp[], afterId: string | null) => string | null;
  /** Removes a block entirely. Refuses (no-op) if it's the last block —
   * a page always has at least one block in this editor. The backend
   * itself refuses a non-empty container (ContainerNotEmptyError) — this
   * client doesn't pre-check that, so deleting one is a silent no-op
   * today (the "error" server frame is only logged, not surfaced as UI) —
   * a known, accepted gap, not a correctness issue: the op is rejected,
   * not silently corrupting the page. */
  deleteBlock: (blockId: string) => void;
  /** Changes a block's kind in place (paragraph ↔ heading ↔ quote ↔
   * code_block ↔ divider ↔ …) — does not touch its content. */
  setBlockKind: (blockId: string, kind: BlockKind) => void;
  /** Moves blockId to immediately after afterId (null = parent's first
   * child) — drag-to-reorder, MoveBlock (RFC-002 §2). parent is
   * OMITTED, not null, when the caller only wants a same-level reorder —
   * it then defaults to the block's own current parent, so existing
   * drag-and-drop (which never names a parent) can't accidentally
   * reparent a nested block to top-level. Passing parent explicitly (even
   * `null` for "make top-level") is how a caller opts into reparenting;
   * no UI in this editor does that yet — a stated gap, not a silent one. */
  moveBlock: (blockId: string, afterId: string | null, parent?: string | null) => void;
  /** Reverts this actor's own most recently recorded gesture (RFC-002 §3:
   * "undo pops the newest undo_group belonging to this actor, never the
   * newest op") — never another peer's edits. A no-op, not an error, when
   * there's nothing to undo. The server drives the resulting inverse
   * op(s) through the same "ack" path an ordinary edit takes
   * (docs/api/collaboration.md §2.1), so this hook needs no separate
   * reducer for it — see the "ack" case below. */
  undo: () => void;
  /** Re-applies the gesture undo most recently reverted. Cleared the
   * moment any new op commits for this actor — the same "a new edit
   * invalidates redo" rule every editor holds itself to. */
  redo: () => void;
  /** Restores the live document to its state as of right after step
   * toStep of GET .../trace's own "steps" array (docs/api/
   * collaboration.md §2.2) — history.html's "restore to a point," real:
   * repeated undo server-side, never a snapshot swap. Same "ack" path as
   * undo/redo, no separate reducer needed here either. */
  restoreTo: (toStep: number) => void;
  /** Round-trip p99 in ms over the last ACK_WINDOW acks, or null before the
   *  first one. § 04's ACK P99, measured on this connection. */
  ackP99: number | null;
  /** Ops written while the socket was down, waiting to replay in order. */
  queued: number;
  /** Reconnect attempts since the last successful open; 0 while connected. */
  attempt: number;
  /** Unix ms of the next scheduled reconnect, or null when not waiting. */
  retryAt: number | null;
  /** Reconnect now rather than waiting out the backoff. */
  retryNow: () => void;
}

/** How many recent acks the latency percentile is taken over. 100 is one
 *  nearest-rank bucket per percent, so p99 is a real sample rather than an
 *  interpolation between two. */
const ACK_WINDOW = 100;

interface liveBlock {
  parent: string | null;
  kind: BlockKind;
  text: string;
  marks: Mark[];
  boundaries: AnchorRange | null;
}

// Module-level, not closures: applyStructural (inside the connection
// effect) and the public insertBlock/deleteBlock/moveBlock callbacks
// (outside it) both need these, and they're pure functions of
// (order, live) — no reason to duplicate them per call site. They mirror
// documentcore's own page.go (predecessorOf/isDescendant/subtreeEnd/
// insertIndexAfterInParent) exactly: this client's local order must stay
// in the same depth-first order the backend maintains, or a MoveBlock
// (which relocates a whole subtree, not just one block — see MoveBlock's
// own doc comment in collab/types.ts) would desync this copy from the
// real one.
function parentOf(live: Map<string, liveBlock>, id: string): string | null {
  return live.get(id)?.parent ?? null;
}

/** The sibling immediately before id — the nearest earlier block sharing
 * id's own parent, mirroring documentcore's predecessorOf. */
function predecessorOf(order: string[], live: Map<string, liveBlock>, id: string): string | null {
  const parent = parentOf(live, id);
  const idx = order.indexOf(id);
  for (let j = idx - 1; j >= 0; j--) {
    if (parentOf(live, order[j]) === parent) return order[j];
  }
  return null;
}

function isDescendant(live: Map<string, liveBlock>, id: string, ancestorId: string): boolean {
  let cur = parentOf(live, id);
  while (cur !== null) {
    if (cur === ancestorId) return true;
    cur = parentOf(live, cur);
  }
  return false;
}

/** The index one past id's last descendant in order's depth-first
 * ordering (i.e. right after id itself, if it has none). */
function subtreeEndIndex(order: string[], live: Map<string, liveBlock>, id: string): number {
  let j = order.indexOf(id) + 1;
  while (j < order.length && isDescendant(live, order[j], id)) j++;
  return j;
}

/** Resolves (parent, after) to an insertion index into order — nil after
 * means "parent's first child" (or the document start, if parent is also
 * nil); a non-nil after goes after all of after's own descendants
 * (subtreeEndIndex), preserving depth-first order. */
function insertIndexAfterInParent(order: string[], live: Map<string, liveBlock>, parent: string | null, after: string | null): number {
  if (after === null) {
    if (parent === null) return 0;
    const parentIdx = order.indexOf(parent);
    return parentIdx === -1 ? order.length : parentIdx + 1;
  }
  return subtreeEndIndex(order, live, after);
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

  /**
   * Round-trip latency: the time between sending an op and the server acking
   * it, in milliseconds. § 04's ACK P99 readout, measured rather than quoted.
   *
   * Matched in ORDER, not by id, because the wire has no correlation id: this
   * connection's own acks come back in the order its ops were sent
   * (docs/api/collaboration.md §2), so a FIFO of send timestamps pairs
   * correctly. That holds for ordinary edits and breaks for undo/redo/restore,
   * which ack once per reverted op — `send` empties the queue there rather
   * than letting every later sample pair against the wrong send.
   *
   * A rolling window, not an all-time percentile: the number worth reading is
   * "how is this connection behaving now", and an all-time p99 stops moving
   * after a few minutes of use.
   */
  const inFlightRef = useRef<number[]>([]);
  const samplesRef = useRef<number[]>([]);
  const [ackP99, setAckP99] = useState<number | null>(null);

  /**
   * The offline queue — § 24 OFFLINE / RECONNECT, made real.
   *
   * Before this, `send` was `socketRef.current?.send(...)`: with the socket
   * gone, the `?.` swallowed the op and the edit was simply lost. Nothing on
   * screen said so, which is the worst version of this failure — the text was
   * in the DOM and nowhere else.
   *
   * Queued ops replay IN ORDER on reconnect, and that is sound for the same
   * reason a delayed op is sound: text ops carry anchors, not offsets, and an
   * anchor resolves in any version that still contains its neighbour — which
   * it does, because a delete tombstones rather than removes (RFC-002, and
   * the palimpsest is the same fact from the other side). What is NOT covered:
   * an op whose anchor neighbour a peer deleted while you were away. The
   * server rejects that with an error frame, which is surfaced rather than
   * swallowed — there is no operational transform here and pretending
   * otherwise would be the dishonest option.
   */
  const queueRef = useRef<ClientMessage[]>([]);
  const [queued, setQueued] = useState(0);
  /** Reconnect attempts since the last successful open. 0 while connected. */
  const [attempt, setAttempt] = useState(0);
  /** Unix ms of the next scheduled reconnect, or null when not waiting. */
  const [retryAt, setRetryAt] = useState<number | null>(null);
  const retryTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  // How many of THIS block's own text-scope ops are still awaiting their
  // ack — see setBlockText and the "ack" case below. Absent/0 means no
  // batch is in flight.
  const pendingTextAcksRef = useRef<Map<string, number>>(new Map());

  const publish = useCallback(() => {
    setBlocks(orderRef.current.map((id) => {
      const b = liveRef.current.get(id)!;
      return { id, parent: b.parent, kind: b.kind, text: b.text, marks: b.marks };
    }));
  }, []);

  // Resetting the document is a PAGE change, not a reconnect. Blanking the
  // block list every time the socket blinks would flash an empty page at
  // someone whose text is sitting safely in the queue.
  useEffect(() => {
    setReady(false);
    orderRef.current = [];
    liveRef.current = new Map();
    pendingTextAcksRef.current = new Map();
    queueRef.current = [];
    setQueued(0);
    setBlocks([]);
    setPeers(new Set());
    setCursors(new Map());
  }, [pageId, actorId]);

  useEffect(() => {
    setState("connecting");

    const url = new URL(`${COLLAB_URL}/collab/pages/${pageId}`);
    url.protocol = url.protocol === "https:" ? "wss:" : "ws:";
    // The credential rides the SUBPROTOCOL list, which is the one header a
    // browser's WebSocket constructor can set (ADR-013 §1). It used to be an
    // ?actor_id= query parameter — an identity the client asserted and
    // nobody checked, in a URL, where it would reach every access log. The
    // server verifies the token and echoes back "bearer" alone.
    const token = accessToken();
    const ws = new WebSocket(url, token ? ["bearer", token] : undefined);
    socketRef.current = ws;

    ws.onopen = () => {
      setState("open");
      setAttempt(0);
      setRetryAt(null);
      // Replay in order. The server applies each against its own current
      // state, exactly as it would have when they were sent.
      const pending = queueRef.current;
      queueRef.current = [];
      setQueued(0);
      for (const msg of pending) {
        if (msg.type === "op") inFlightRef.current.push(performance.now());
        ws.send(JSON.stringify(msg));
      }
    };

    /**
     * Exponential backoff, capped, with the delay reported rather than
     * hidden: a screen that says "retrying" and nothing else is a screen you
     * cannot tell from a hung one. Capped at 15s because this is a person
     * waiting, not a batch job — and reset to 0 on a successful open, so a
     * flaky connection does not inherit yesterday's backoff.
     */
    const scheduleReconnect = () => {
      if (retryTimer.current) return;
      const delay = Math.min(1000 * 2 ** attempt, 15000);
      setRetryAt(Date.now() + delay);
      retryTimer.current = setTimeout(() => {
        retryTimer.current = null;
        setAttempt((a) => a + 1);
      }, delay);
    };

    ws.onclose = () => { setState("closed"); scheduleReconnect(); };
    ws.onerror = () => { setState("closed"); scheduleReconnect(); };

    function applyStructural(op: PageOp & { scope: "block" }) {
      switch (op.type) {
        case "InsertBlock":
          if (!liveRef.current.has(op.id)) {
            liveRef.current.set(op.id, { parent: op.parent, kind: op.kind, text: op.content.text, marks: op.content.marks ?? [], boundaries: null });
            const idx = insertIndexAfterInParent(orderRef.current, liveRef.current, op.parent, op.after);
            orderRef.current.splice(idx, 0, op.id);
          }
          break;
        case "DeleteBlock": {
          // The backend only ever allows deleting an empty container or a
          // leaf (ContainerNotEmptyError otherwise), so this never needs
          // to remove more than the one id — unlike MoveBlock below.
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
          // Relocates the whole subtree rooted at op.id as one contiguous
          // unit, preserving depth-first order — the same reason
          // documentcore.Page.Apply's own MoveBlock case does. Only the
          // subtree's own root gets reparented; every descendant already
          // has the right Parent (each other) and needs no change.
          const idx = orderRef.current.indexOf(op.id);
          if (idx === -1) break;
          const end = subtreeEndIndex(orderRef.current, liveRef.current, op.id);
          const subtree = orderRef.current.slice(idx, end);
          orderRef.current = [...orderRef.current.slice(0, idx), ...orderRef.current.slice(end)];
          const insertIdx = insertIndexAfterInParent(orderRef.current, liveRef.current, op.to_parent, op.to);
          orderRef.current.splice(insertIdx, 0, ...subtree);
          const root = liveRef.current.get(op.id);
          if (root) root.parent = op.to_parent;
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
            msg.snapshot.blocks.map((b) => [b.id, { parent: b.parent, kind: b.kind, text: b.text, marks: b.marks ?? [], boundaries: b.boundaries ?? null }]),
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
          const sentAt = inFlightRef.current.shift();
          if (sentAt !== undefined) {
            const samples = samplesRef.current;
            samples.push(performance.now() - sentAt);
            if (samples.length > ACK_WINDOW) samples.shift();
            // Nearest-rank p99 over the window. With fewer than 100 samples
            // that is the slowest one, which is the honest answer to "what is
            // the worst this connection has done lately" at small n.
            const sorted = [...samples].sort((a, b) => a - b);
            const idx = Math.min(sorted.length - 1, Math.ceil(sorted.length * 0.99) - 1);
            setAckP99(sorted[Math.max(0, idx)]);
          }
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

    return () => {
      // Clear the handlers before closing: onclose would otherwise fire
      // during teardown and schedule a reconnect for a socket nobody wants.
      ws.onclose = null;
      ws.onerror = null;
      ws.close();
    };
  }, [pageId, actorId, publish, attempt]);

  // A reconnect timer must not outlive the hook.
  useEffect(() => () => {
    if (retryTimer.current) clearTimeout(retryTimer.current);
    retryTimer.current = null;
  }, []);

  /** Try now instead of waiting out the backoff — § 24's RETRY NOW. */
  const retryNow = useCallback(() => {
    if (retryTimer.current) {
      clearTimeout(retryTimer.current);
      retryTimer.current = null;
    }
    setRetryAt(null);
    setAttempt((a) => a + 1);
  }, []);

  const send = useCallback((msg: ClientMessage) => {
    if (msg.type === "op") {
      // One timestamp per op sent, matched against acks in order — see
      // ackLatencyRef's own comment for why in-order is sound here and where
      // it stops being.
      inFlightRef.current.push(performance.now());
    } else if (msg.type === "undo" || msg.type === "redo" || msg.type === "restore") {
      // These ack once per op the server actually reverted, which this side
      // cannot predict. Rather than mismatch every later sample, the queue is
      // abandoned: a few edits go unmeasured after an undo, which is a
      // smaller lie than a p99 measured against the wrong send.
      inFlightRef.current.length = 0;
    }
    const ws = socketRef.current;
    if (ws && ws.readyState === WebSocket.OPEN) {
      ws.send(JSON.stringify(msg));
      return;
    }
    // Offline. The op is kept, in order, and replayed on reconnect. A cursor
    // frame is NOT: it is a position that will be stale by the time the
    // socket returns, and replaying it would move a caret nobody moved.
    if (msg.type === "cursor") return;
    // The latency sample cannot be paired against an ack that has not been
    // asked for yet — it is re-timestamped at flush.
    if (msg.type === "op") inFlightRef.current.pop();
    queueRef.current.push(msg);
    setQueued(queueRef.current.length);
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
      // One user-visible edit ("replace this block's text") can be two
      // ops on the wire (delete the old text, insert the new) — grouped
      // under one undo_group so a single Ctrl+Z reverts both, not just
      // the InsertText half (RFC-002 §3's "undo pops the newest
      // undo_group... without grouping, ⌘Z undoes one twentieth of a
      // paste and the document looks corrupted"). Undefined (not
      // generated) when only one op is actually sent below — a real
      // group of one needs no id at all.
      const undoGroup = b.boundaries && newText.length > 0 ? crypto.randomUUID() : undefined;
      let opsSent = 0;
      if (b.boundaries) {
        send({ type: "op", op: { scope: "text", block: blockId, op: { type: "DeleteText", range: b.boundaries, text: "" } }, undo_group: undoGroup });
        opsSent++;
      }
      if (newText.length > 0) {
        send({ type: "op", op: { scope: "text", block: blockId, op: { type: "InsertText", at: null, text: newText } }, undo_group: undoGroup });
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
    (afterId: string | null, kind: BlockKind, parent: string | null = null) => {
      const id = crypto.randomUUID();
      send({
        type: "op",
        op: { scope: "block", type: "InsertBlock", id, parent, after: afterId, kind, content: { text: "" } },
      });
      return id;
    },
    [send],
  );

  /**
   * Paste: a compiled batch of ordinary InsertBlocks.
   *
   * Paste is NOT a special path. `marginal/mdc` turns the pasted text into
   * exactly the ops the editor would have sent had you typed it, and they go
   * down the same wire under ONE undo group — so a 200-block paste is one ⌘Z,
   * which is the behaviour people actually report when it is missing.
   *
   * The compiler's own block ids are readable placeholders (`b0`, `b1`) so
   * § 11 can print them; they are rewritten to UUIDs here, because a block id
   * has to be unique across a workspace and `b0` is not. The mapping is
   * applied to `parent` and `after` too — an op naming an id that was
   * rewritten and a parent that was not is the bug this loop exists to
   * avoid.
   *
   * Returns the id of the last top-level block inserted, so the caller can
   * put the caret after it.
   */
  const insertCompiled = useCallback(
    (ops: CompiledOp[], afterId: string | null): string | null => {
      if (ops.length === 0) return null;
      const undoGroup = crypto.randomUUID();
      const idOf = new Map<string, string>();
      for (const op of ops) idOf.set(op.id, crypto.randomUUID());

      let lastTop: string | null = null;
      let previousTop = afterId;
      for (const op of ops) {
        const id = idOf.get(op.id)!;
        const parent = op.parent ? idOf.get(op.parent) ?? null : null;
        // A top-level block follows the last one this paste inserted; the
        // first follows wherever the caret was. A nested block's `after` is
        // always inside the batch, so the map answers it.
        const after = op.after
          ? idOf.get(op.after) ?? null
          : parent === null
            ? previousTop
            : null;
        if (parent === null) {
          previousTop = id;
          lastTop = id;
        }
        send({
          type: "op",
          undo_group: undoGroup,
          op: {
            scope: "block", type: "InsertBlock", id, parent, after,
            kind: op.kind as BlockKind,
            content: { text: op.content.text, marks: op.content.marks as Mark[] },
          },
        });
      }
      return lastTop;
    },
    [send],
  );

  const deleteBlock = useCallback(
    (blockId: string) => {
      if (orderRef.current.length <= 1) return; // always keep at least one block
      const b = liveRef.current.get(blockId);
      if (!b) return;
      const after = predecessorOf(orderRef.current, liveRef.current, blockId);
      send({
        type: "op",
        op: { scope: "block", type: "DeleteBlock", tombstone: { id: blockId, parent: b.parent, kind: b.kind, content: { text: b.text, marks: b.marks } }, after },
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
    (blockId: string, afterId: string | null, parent?: string | null) => {
      if (blockId === afterId) return;
      const b = liveRef.current.get(blockId);
      if (!b) return;
      // Omitted parent means "keep the block's own current parent" — a
      // plain reorder, never a reparent — see this hook's own moveBlock
      // doc comment for why that matters for existing drag-and-drop.
      const toParent = parent === undefined ? b.parent : parent;
      const from = predecessorOf(orderRef.current, liveRef.current, blockId);
      if (b.parent === toParent && from === afterId) return; // already there
      send({ type: "op", op: { scope: "block", type: "MoveBlock", id: blockId, from_parent: b.parent, from, to_parent: toParent, to: afterId } });
    },
    [send],
  );

  const undo = useCallback(() => send({ type: "undo" }), [send]);
  const redo = useCallback(() => send({ type: "redo" }), [send]);
  const restoreTo = useCallback((toStep: number) => send({ type: "restore", to_step: toStep }), [send]);

  return { state, ready, blocks, peers, cursors, ackP99, queued, attempt, retryAt, retryNow, setCursor, insertCompiled, setBlockText, setBlockContent, insertBlock, deleteBlock, setBlockKind, moveBlock, undo, redo, restoreTo };
}
