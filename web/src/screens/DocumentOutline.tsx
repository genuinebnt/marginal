/**
 * "Where am I in this page", in the left rail — after genuine-folio's own
 * EditorOutlineTab (~/projects/genuine-folio/frontend/components/editor).
 *
 * The page tree above answers "where am I in the workspace". This is a
 * different question, so it gets its own section rather than an inspector tab
 * you have to go and find: it is navigation, and navigation belongs on the
 * side you navigate from.
 *
 * Hierarchy is shown by indent AND by an explicit level mark, because indent
 * alone stops being legible past two levels — genuine-folio's H1/H2/H3 chip
 * is doing real work, not decorating.
 *
 * Non-heading landmarks are listed too. A code fence or a callout has no
 * title but is still somewhere you navigate to, so it appears marked by kind
 * rather than by number.
 */
import type { BlockView } from "../collab/useCollabPage";

type Entry =
  | { type: "heading"; id: string; level: number; text: string }
  | { type: "block"; id: string; kind: "code" | "callout" | "aside"; text: string };

const LANDMARK: Record<string, "code" | "callout" | "aside"> = {
  code_block: "code",
  callout: "callout",
  aside: "aside",
};

const MARK: Record<string, string> = { code: "</>", callout: "◌", aside: "▎" };

export function outlineOf(blocks: BlockView[]): Entry[] {
  const out: Entry[] = [];
  for (const b of blocks) {
    const tag = b.kind.tag;
    if (tag === "heading") {
      const level = (b.kind as { level?: number }).level ?? 1;
      // A heading with no text is still structure, but naming it "Untitled"
      // would be inventing content — it is shown as an empty slot instead.
      out.push({ type: "heading", id: b.id, level, text: b.text });
      continue;
    }
    const kind = LANDMARK[tag];
    if (kind) out.push({ type: "block", id: b.id, kind, text: b.text });
  }
  return out;
}

export function DocumentOutline({
  blocks, activeId, onJump, first = false, title, onJumpTop,
}: {
  blocks: BlockView[];
  activeId?: string | null;
  onJump: (blockId: string) => void;
  /** The page's own title, drawn as the outline's H1 row — § 04 and § 05
   *  both lead with it. It is the document's top level even though it is not
   *  a block: an outline that starts at the first H2 is an outline missing
   *  its root. */
  title?: string;
  onJumpTop?: () => void;
  /** True when this is the rail's FIRST section (the reader, § 05), where it
   *  keeps a normal section header and needs no rule above it. In the editor
   *  it follows the page tree, so it is separated from it and its header sits
   *  tight against that rule. */
  first?: boolean;
}) {
  const entries = outlineOf(blocks);

  return (
    <div style={first
      ? undefined
      : { margin: "14px 0 0", padding: "12px 0 0", borderTop: "1px solid rgba(255,255,255,.07)" }}>
      <div className="rail-h" style={first ? undefined : { paddingTop: 0 }}>
        IN THIS PAGE<div /><span style={{ color: "#585550" }}>{entries.length + (title ? 1 : 0)}</span>
      </div>
      <div style={{ display: "flex", flexDirection: "column", gap: 1, padding: "0 6px", maxHeight: 260, overflowY: "auto" }}>
        {title !== undefined && (
          <div
            // `=== null` on purpose, not `== null`: undefined means the
            // screen does not track a position at all (the editor), where
            // null means it does and you are above the first landmark (the
            // reader, at the top). Only the second is "the title is active".
            className={`oi oi-h1${activeId === null ? " oi-on" : ""}`}
            onClick={() => onJumpTop?.()}
            title={title}
          >
            <span className="oi-m">H1</span>
            <span className="oi-t">{title || <span style={{ color: "#4B4842" }}>(untitled)</span>}</span>
          </div>
        )}
        {entries.length === 0 && (
          <div style={{ padding: "4px 9px", fontSize: 11, color: "#585550", lineHeight: 1.55 }}>
            No headings or landmarks yet — a flat page has no structure to show,
            which is a real state rather than an empty panel.
          </div>
        )}
        {entries.map((e) => {
          const cls =
            e.type === "heading"
              ? `oi oi-h${e.level}${e.id === activeId ? " oi-on" : ""}`
              : `oi oi-${e.kind}${e.id === activeId ? " oi-on" : ""}`;
          // Landmarks sit one level in from the heading they follow, matching
          // the mockup: they are contained by a section, not peers of it.
          const style = e.type === "block" ? { paddingLeft: 20 } : undefined;
          return (
            <div key={e.id} className={cls} style={style} onClick={() => onJump(e.id)}
                 title={e.text}>
              {e.type === "heading"
                ? <span className="oi-m">H{e.level}</span>
                : <span className="oi-k">{MARK[e.kind]}</span>}
              <span className="oi-t">
                {e.text.trim() || <span style={{ color: "#4B4842" }}>(empty)</span>}
              </span>
            </div>
          );
        })}
      </div>
    </div>
  );
}
