import type { BlockKind } from "./types";

// A flat string key per documentcore.BlockKind variant — what a <select>
// can bind to directly, since BlockKind itself is a tagged object (and
// "heading" needs its level folded into the key to be one option per
// heading level).
export type BlockKindKey = "paragraph" | "heading1" | "heading2" | "heading3" | "quote" | "code_block" | "divider";

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
};

export const KIND_ORDER: BlockKindKey[] = ["paragraph", "heading1", "heading2", "heading3", "quote", "code_block", "divider"];
