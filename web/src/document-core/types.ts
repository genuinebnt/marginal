// Wire types for documentcore's JSON boundary. Pure type declarations plus
// object-literal builders — no algorithms, no validation. All of that
// (mark coalescing, op preconditions, heading-level validation, ...) lives
// in Go, compiled to wasm (see wasm.ts) — this file exists so views get
// type safety on the same shapes the Go side actually serializes
// (services/document-service/internal/documentcore's `json:"..."` tags),
// the same way a generated OpenAPI client's types would.

export type PageId = string;
export type BlockId = string;

export type MarkKind =
  | { tag: "bold" }
  | { tag: "italic" }
  | { tag: "strike" }
  | { tag: "code" }
  | { tag: "link"; url: string }
  | { tag: "pagelink"; page: PageId };

export const bold = (): MarkKind => ({ tag: "bold" });
export const italic = (): MarkKind => ({ tag: "italic" });
export const strike = (): MarkKind => ({ tag: "strike" });
export const code = (): MarkKind => ({ tag: "code" });
export const link = (url: string): MarkKind => ({ tag: "link", url });
export const pageLink = (page: PageId): MarkKind => ({ tag: "pagelink", page });

export interface Mark {
  kind: MarkKind;
  start: number; // UTF-8 byte offset — see documentcore's inline.go doc comment
  end: number;
}

export interface Content {
  text: string;
  marks: Mark[];
}

export const plainContent = (text: string): Content => ({ text, marks: [] });

// documentcore now also implements List/ListItem/Toggle/Image/Callout/
// Aside (RFC-001 §1) — not added here, since this module isn't wired to
// any screen yet (this file's own header comment) and nothing exercises
// them through the wasm boundary; add them when/if that changes, same
// convention as everything else in this file staying a direct mirror of
// whatever documentcore's JSON actually is at the time.
export type BlockKind =
  | { tag: "paragraph" }
  | { tag: "heading"; level: 1 | 2 | 3 }
  | { tag: "quote" }
  | { tag: "code_block"; language: string }
  | { tag: "divider" };

export const paragraph = (): BlockKind => ({ tag: "paragraph" });
export const quote = (): BlockKind => ({ tag: "quote" });
export const divider = (): BlockKind => ({ tag: "divider" });
export const codeBlock = (language: string): BlockKind => ({ tag: "code_block", language });
// Heading level validation (1..=3) happens in Go when applyOp runs the op
// through Page.Apply — an invalid level here is only caught there, not
// client-side. That's deliberate: the check has exactly one owner.
export const heading = (level: 1 | 2 | 3): BlockKind => ({ tag: "heading", level });

export interface Block {
  id: BlockId;
  parent: BlockId | null;
  kind: BlockKind;
  content: Content;
}

export interface Page {
  id: PageId;
  title: string;
  blocks: Block[];
}

/** Mirrors documentcore's Op union exactly — field names match RFC-002 §2
 * and RFC-001 §1's containment (Parent/FromParent/ToParent). This module
 * isn't wired to any screen yet (see this file's own header comment) —
 * kept in sync with documentcore's real wire shape anyway, the same
 * reason wasm.test.ts's own literals are, so a later hookup doesn't
 * inherit a silently-stale type. */
export type Op =
  | { type: "InsertBlock"; id: BlockId; parent: BlockId | null; after: BlockId | null; kind: BlockKind; content: Content }
  | { type: "DeleteBlock"; tombstone: Block; after: BlockId | null }
  | { type: "SetBlockKind"; id: BlockId; from: BlockKind; to: BlockKind }
  | { type: "SetBlockContent"; block: BlockId; prev: Content; content: Content }
  | { type: "SetTitle"; page: PageId; from: string; to: string }
  | { type: "MoveBlock"; id: BlockId; from_parent: BlockId | null; from: BlockId | null; to_parent: BlockId | null; to: BlockId | null };
