// Wire types for docs/api/collaboration.md — kept in sync with
// services/collaboration-service/internal/{anchor,ops,oplog,wsapi,pageop}'s
// own json tags by hand (this repo has no protobuf/OpenAPI generator for
// the WebSocket contract, unlike pages.md/auth.md's REST surface), and
// with services/documentcore's own op/kind JSON shapes for the block tier.

import type { Mark } from "./marks";

export interface ItemId {
  actor: string;
  counter: number;
}

export type Bias = "before" | "after";

export interface Anchor {
  item: ItemId;
  bias: Bias;
}

export interface AnchorRange {
  start: Anchor;
  end: Anchor;
}

// Character-granular ops (internal/ops) — unchanged from before pageop
// existed; scoped to one block's own live rope now instead of a
// whole-page one.
export type TextOp =
  | { type: "InsertText"; at: Anchor | null; text: string }
  | { type: "DeleteText"; range: AnchorRange; text: string };

// Block-granular ops (documentcore.Op) — RFC-002 §2's structural tier.
// Content/BlockKind match documentcore's own JSON shapes exactly
// (block.go's blockKindJSON, page.go's Content). List/ListItem/Toggle/
// Image/Callout/Aside are RFC-001 §1's containment additions — see
// block.go's own blockKindJSON doc comment for which field is
// meaningful on which tag.
export type BlockKind =
  | { tag: "paragraph" }
  | { tag: "heading"; level: number }
  | { tag: "quote" }
  | { tag: "code_block"; language: string }
  | { tag: "divider" }
  | { tag: "list"; list_kind: "bulleted" | "numbered" | "todo" }
  | { tag: "list_item"; checked?: boolean }
  | { tag: "toggle" }
  | { tag: "image"; file_id: string }
  | { tag: "callout"; tone?: "note" | "info" | "tip" | "warn" | "danger" | "success"; icon?: string }
  | { tag: "aside"; emoji: string };

export interface Content {
  text: string;
  marks?: Mark[];
}

// Tombstone/InsertBlock's block-identity fields match documentcore.Block
// (block.go) — parent included, so DeleteBlock's own precondition check
// (ParentMismatchError) has what it needs, the same reasoning Tombstone
// already carries kind/content for Invert().
export type BlockOp =
  | { type: "InsertBlock"; id: string; parent: string | null; after: string | null; kind: BlockKind; content: Content }
  | { type: "DeleteBlock"; tombstone: { id: string; parent: string | null; kind: BlockKind; content: Content }; after: string | null }
  | { type: "SetBlockKind"; id: string; from: BlockKind; to: BlockKind }
  | { type: "SetBlockContent"; block: string; prev: Content; content: Content }
  | { type: "SetTitle"; page: string; from: string; to: string }
  | { type: "MoveBlock"; id: string; from_parent: string | null; from: string | null; to_parent: string | null; to: string | null };

// pageop.Op's wire union (internal/pageop) — "scope" tells the two tiers
// apart. A Block envelope merges BlockOp's own tagged fields directly (no
// extra nesting level); a Text envelope nests TextOp under "op" alongside
// which block it applies to.
export type PageOp = ({ scope: "block" } & BlockOp) | { scope: "text"; block: string; op: TextOp };

export interface LoggedOp {
  id: string;
  version: number;
  page_id: string;
  actor_id: string;
  actor_kind: string;
  undo_group?: string;
  vector_clock: Record<string, number>;
  op: PageOp;
  created_at: string;
}

// session.BlockSnapshot/Snapshot — the "snapshot" frame's payload.
export interface BlockSnapshot {
  id: string;
  parent: string | null;
  kind: BlockKind;
  text: string;
  marks?: Mark[];
  boundaries?: AnchorRange;
}

export interface PageSnapshot {
  page_id: string;
  title: string;
  blocks: BlockSnapshot[];
}

// session.CursorEvent's wire shape (wsapi's cursorWire) — one actor's
// current caret/selection. block_id null means "not focused in any block
// right now" (they blurred everywhere); start/end are meaningless then
// and typically omitted. start/end are rune offsets into that block's
// live text (the same unit InsertText/DeleteText already use), not
// UTF-16 code units — see useCollabPage's own note on where this
// simplifies for multi-byte text, matching marks.ts's existing one.
export interface CursorWire {
  actor_id: string;
  block_id: string | null;
  start: number;
  end: number;
}

export type ServerMessage =
  | { type: "snapshot"; snapshot: PageSnapshot; present?: string[]; cursors?: CursorWire[] }
  | { type: "ack"; op: LoggedOp; boundaries?: AnchorRange }
  | { type: "broadcast"; op: LoggedOp; boundaries?: AnchorRange }
  | { type: "presence"; actor_id: string; joined: boolean }
  | { type: "cursor"; cursor: CursorWire }
  | { type: "error"; message: string };

export type ClientMessage =
  // undo_group is optional even here — omit it (or send undefined) for a
  // single-op edit, RFC-002 §3's "a group of one." Set it to the same
  // client-generated id on every op belonging to one gesture (a paste, an
  // input rule's multi-op conversion) so a later "undo" reverts the whole
  // gesture in one step — see docs/api/collaboration.md §2.
  | { type: "op"; op: PageOp; undo_group?: string }
  | { type: "cursor"; cursor: { block_id: string | null; start: number; end: number } }
  // No payload — undo/redo apply to the sender's own actor id, scoped to
  // this page (docs/api/collaboration.md §2.1). The server acks one frame
  // per op the action actually committed, each indistinguishable on the
  // wire from an ordinary "ack"/"broadcast" — see useCollabPage, which
  // needs no special-case handling for them at all.
  | { type: "undo" }
  | { type: "redo" }
  // Restore the live document to a past point in its own confirmed op log
  // (docs/api/collaboration.md §2.2). Repeated undo rather than a
  // restore-from-backup: the server acks one frame per reverted step, all
  // of them one new undo group for the requester.
  | { type: "restore"; to_step: number };
