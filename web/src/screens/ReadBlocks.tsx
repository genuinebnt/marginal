/**
 * The block tree, rendered for READING.
 *
 * Why this exists as its own component rather than as a branch inside
 * ReaderScreen: the reader was rendering only the blocks with no parent, and
 * every container kind in RFC-001's grammar — quote, callout, aside, toggle,
 * list — keeps its content in CHILD blocks. So a quote rendered as an empty
 * paragraph, a callout rendered as nothing, and an aside's whole text was
 * silently dropped. On the seeded corpus that was most of the argument on
 * most of the pages, and it was invisible: the page looked short rather than
 * broken.
 *
 * It is a second RENDERER, not a second model. Marks and page links go
 * through `renderMarkedHTML`, the same function the editor uses, so the two
 * views cannot disagree about what a page says. What differs is only what
 * read mode does not need: no contenteditable, no caret, no ops.
 */
import type { ReactNode } from "react";
import type { BlockView } from "../collab/useCollabPage";
import { renderMarkedHTML } from "../collab/marks";

const CALLOUT_ICON: Record<string, string> = {
  warn: "◌", danger: "◌", success: "✓", info: "✦", note: "✦", tip: "◆",
};

/** Marks and page links, as HTML. Escaped by renderMarkedHTML before any
 *  markup is added, so there is nothing here to inject through. */
function Inline({ b, known }: { b: BlockView; known: Set<string> }) {
  return <span dangerouslySetInnerHTML={{ __html: renderMarkedHTML(b.text, b.marks ?? [], known) }} />;
}

export function ReadBlocks({
  blocks, known,
}: { blocks: BlockView[]; known: Set<string> }) {
  const childrenOf = new Map<string | null, BlockView[]>();
  for (const b of blocks) {
    const key = b.parent ?? null;
    const list = childrenOf.get(key);
    if (list) list.push(b);
    else childrenOf.set(key, [b]);
  }

  function render(b: BlockView, indexInParent: number, siblings: BlockView[]): ReactNode {
    const tag = b.kind.tag;
    const kids = childrenOf.get(b.id) ?? [];
    const inner = kids.map((c, i) => render(c, i, kids));

    if (tag === "divider") return <hr key={b.id} className="block-divider" />;

    if (tag === "heading") {
      const level = (b.kind as { level?: number }).level ?? 1;
      const size = level === 1 ? 27 : level === 2 ? 22 : 18;
      return (
        <div key={b.id} data-block-id={b.id} style={{
          fontFamily: "Spectral, serif", fontWeight: 500, fontSize: size,
          letterSpacing: "-.015em", color: "#EFEDE7", margin: "26px 0 12px",
        }}>
          <Inline b={b} known={known} />
        </div>
      );
    }

    if (tag === "code_block") {
      return (
        <div key={b.id} data-block-id={b.id} className="blk-code" style={{ margin: "0 0 16px" }}>
          <div className="blk-code-h">
            <span className="mono lang">
              {((b.kind as { language?: string }).language || "plain text").toUpperCase()}
            </span>
          </div>
          <pre>{b.text}</pre>
        </div>
      );
    }

    if (tag === "quote") {
      return (
        <blockquote key={b.id} data-block-id={b.id} className="blk-quote">
          {b.text && <Inline b={b} known={known} />}
          {inner}
        </blockquote>
      );
    }

    if (tag === "callout") {
      const tone = (b.kind as { tone?: string }).tone ?? "warn";
      return (
        <div key={b.id} data-block-id={b.id} className={`blk-callout tone-${tone}`}>
          <span className="ic">{CALLOUT_ICON[tone] ?? "◌"}</span>
          <div className="block-children">
            {b.text && <Inline b={b} known={known} />}
            {inner}
          </div>
        </div>
      );
    }

    if (tag === "aside") {
      return (
        <div key={b.id} data-block-id={b.id} className="blk-aside">
          {b.text && <Inline b={b} known={known} />}
          {inner}
        </div>
      );
    }

    if (tag === "toggle") {
      // Always open in read mode, and that is a decision rather than a
      // shortcut: collapse state is VIEW state (RFC-001 §1), so it must not
      // enter the tree — and a reader who cannot see a section has no way to
      // know it was there. Writing is where you fold things away.
      return (
        <div key={b.id} data-block-id={b.id} className="blk-toggle">
          <div className="blk-toggle-summary">
            <span className="tw">▾</span>
            <div style={{ flex: 1 }}><Inline b={b} known={known} /></div>
          </div>
          <div className="block-children">{inner}</div>
        </div>
      );
    }

    if (tag === "list") {
      return <div key={b.id} data-block-id={b.id} style={{ margin: "0 0 16px" }}>{inner}</div>;
    }

    if (tag === "list_item") {
      const parent = blocks.find((x) => x.id === b.parent);
      const listKind = parent?.kind.tag === "list"
        ? (parent.kind as { list_kind?: string }).list_kind ?? "bulleted"
        : "bulleted";
      const checked = Boolean((b.kind as { checked?: boolean }).checked);
      // Numbering counts only the ITEMS among the siblings, so a stray block
      // in a list would not silently shift every number after it.
      const n = siblings.slice(0, indexInParent + 1).filter((x) => x.kind.tag === "list_item").length;
      return (
        <div key={b.id} data-block-id={b.id} className={`li-row${checked ? " done" : ""}`}>
          {listKind === "numbered" && <span className="li-marker num mono">{n}</span>}
          {listKind === "todo" && (
            <span className="li-marker">
              <span className={`li-check${checked ? " on" : ""}`}>{checked ? "✓" : ""}</span>
            </span>
          )}
          {listKind === "bulleted" && <span className="li-marker">•</span>}
          <div className="li-body">
            <Inline b={b} known={known} />
            {inner}
          </div>
        </div>
      );
    }

    return (
      <p key={b.id} data-block-id={b.id} style={{ margin: "0 0 16px" }}>
        <Inline b={b} known={known} />
        {inner}
      </p>
    );
  }

  const roots = childrenOf.get(null) ?? [];
  return <>{roots.map((b, i) => render(b, i, roots))}</>;
}
