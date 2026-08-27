import type { BlockKind } from "./types";

// A flat string key per documentcore.BlockKind variant — what a <select>
// can bind to directly, since BlockKind itself is a tagged object (and
// "heading" needs its level folded into the key to be one option per
// heading level; "list" needs its list_kind folded in the same way).
// list_item/toggle/callout/aside/image are RFC-001 §1's containment
// additions — see block.go's blockKindJSON for which field is
// meaningful on which tag.
export type BlockKindKey =
  | "paragraph"
  | "heading1"
  | "heading2"
  | "heading3"
  | "quote"
  | "code_block"
  | "divider"
  | "bulleted_list"
  | "numbered_list"
  | "todo_list"
  | "toggle"
  | "callout"
  | "aside"
  | "image";

/** Container tags (RFC-001 §1) — kindFromKey never produces one of these
 * as a fresh insert on its own; RichEditorPane's insertContainer handles
 * the container+first-child compound insert these kinds need instead. */
export const CONTAINER_KEYS: ReadonlySet<BlockKindKey> = new Set(["bulleted_list", "numbered_list", "todo_list", "toggle", "callout", "aside"]);

export function keyOf(kind: BlockKind): BlockKindKey {
  switch (kind.tag) {
    case "paragraph":
      return "paragraph";
    case "heading":
      return `heading${kind.level}` as BlockKindKey;
    case "quote":
      return "quote";
    case "code_block":
      return "code_block";
    case "divider":
      return "divider";
    case "list":
      return kind.list_kind === "bulleted" ? "bulleted_list" : kind.list_kind === "numbered" ? "numbered_list" : "todo_list";
    case "list_item":
      return "paragraph"; // ListItem's own key is never looked up directly — see BlockRow's own kind switch
    case "toggle":
      return "toggle";
    case "callout":
      return "callout";
    case "aside":
      return "aside";
    case "image":
      return "image";
  }
}

export function kindFromKey(key: BlockKindKey): BlockKind {
  switch (key) {
    case "paragraph":
      return { tag: "paragraph" };
    case "heading1":
      return { tag: "heading", level: 1 };
    case "heading2":
      return { tag: "heading", level: 2 };
    case "heading3":
      return { tag: "heading", level: 3 };
    case "quote":
      return { tag: "quote" };
    case "code_block":
      return { tag: "code_block", language: "" };
    case "divider":
      return { tag: "divider" };
    case "bulleted_list":
      return { tag: "list", list_kind: "bulleted" };
    case "numbered_list":
      return { tag: "list", list_kind: "numbered" };
    case "todo_list":
      return { tag: "list", list_kind: "todo" };
    case "toggle":
      return { tag: "toggle" };
    case "callout":
      return { tag: "callout", tone: "warn" };
    case "aside":
      return { tag: "aside", emoji: "💬" };
    case "image":
      // documentcore.UnmarshalJSON parses file_id as a real UUID (Image
      // ::= FileId Caption?, RFC-001 §1) — it has no "empty" representation,
      // unlike Language/Emoji's own bare "" defaults, so a fresh Image
      // insert needs a real (if placeholder — no upload pipeline exists
      // yet) id or the op fails server-side validation.
      return { tag: "image", file_id: crypto.randomUUID() };
  }
}

export const KIND_LABELS: Record<BlockKindKey, string> = {
  paragraph: "Text",
  heading1: "Heading 1",
  heading2: "Heading 2",
  heading3: "Heading 3",
  quote: "Quote",
  code_block: "Code",
  divider: "Divider",
  bulleted_list: "Bulleted list",
  numbered_list: "Numbered list",
  todo_list: "To-do list",
  toggle: "Toggle",
  callout: "Callout",
  aside: "Aside",
  image: "Image",
};

export const KIND_ORDER: BlockKindKey[] = [
  "paragraph",
  "heading1",
  "heading2",
  "heading3",
  "quote",
  "code_block",
  "divider",
  "bulleted_list",
  "numbered_list",
  "todo_list",
  "toggle",
  "callout",
  "aside",
  "image",
];
