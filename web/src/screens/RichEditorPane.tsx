import { useEffect, useRef, useState, type DragEvent, type ElementType, type FormEvent, type KeyboardEvent, type ReactNode } from "react";
import type { Page } from "../api/pages";
import type { Diagnostic } from "../api/diagnostics";
import type { CollabPage, BlockView } from "../collab/useCollabPage";
import type { BlockKind } from "../collab/types";
import { KIND_LABELS, KIND_ORDER, kindFromKey, keyOf, type BlockKindKey } from "../collab/blockKind";
import { addMark, isFullyMarked, removeMark, renderMarkedHTML, shiftMarksForEdit, type Mark, type MarkKind } from "../collab/marks";

// Lists are two blocks (a List container plus its first ListItem child),
// not a single in-place conversion the way every other kind is — see
// chooseKind's own comment for why this table exists.
const LIST_KEYS: Partial<Record<BlockKindKey, "bulleted" | "numbered" | "todo">> = {
  bulleted_list: "bulleted",
  numbered_list: "numbered",
  todo_list: "todo",
};

const CALLOUT_TONE_COLOR: Record<string, string> = {
  note: "var(--blue, #3b82f6)",
  info: "var(--blue, #3b82f6)",
  tip: "var(--green, #22c55e)",
  warn: "var(--warn, #d97706)",
  danger: "var(--red, #dc2626)",
  success: "var(--green, #22c55e)",
};

const EMPTY_DIAGNOSTICS: Diagnostic[] = [];
const REPLACE_DEBOUNCE_MS = 400;
const BLOCK_ID_ATTR = "data-block-id";

/** A short, human-distinguishable 2-character tag for an actor id — never
 * a leading substring of it. Actor ids are UUIDv7s, which front-load a
 * millisecond timestamp (RFC 9562 §5.7): two accounts registered within
 * the same session share leading hex digits, so `id.slice(0, 2)` — the
 * form this used to be — collides for exactly the case that matters most
 * (two people testing presence/cursors together, created minutes apart).
 * Hashing the whole id instead spreads its actual entropy (concentrated
 * in its later, random bits) across both displayed characters. */
function actorTag(actorId: string): string {
  let hash = 0;
  for (let i = 0; i < actorId.length; i++) {
    hash = (hash * 31 + actorId.charCodeAt(i)) | 0;
  }
  const alphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789";
  const a = alphabet[Math.abs(hash) % alphabet.length];
  const b = alphabet[Math.abs(hash >> 5) % alphabet.length];
  return a + b;
}

/**
 * The rich block editor — paragraphs, headings, quotes, code blocks, and
 * dividers, each its own block-tree node and its own live collaborative
 * rope (RFC-001/RFC-002, internal/pageop). Replaces the older flat
 * single-textarea EditorPane now that collaboration-service reconciles
 * block structure and character-level text into one system
 * (docs/porting/PROGRESS.md).
 *
 * Text blocks (paragraph/heading/quote) render as real contentEditable
 * elements of the matching tag (h1/h2/h3/p/blockquote) — free typography
 * from design-system.css's .doc rules, and Enter always means "new block"
 * (a block is the unit of text here, not a line — RFC-001 §1), so these
 * never need a literal newline of their own. code_block is the one kind
 * that legitimately holds multiple lines within itself, so it's a plain
 * <textarea> instead — sidesteps contentEditable's notoriously unreliable
 * newline behavior entirely, at the cost of not being a "real" DOM code
 * block the way the others are real DOM headings (and, structurally, why
 * code blocks never get marks or a bubble menu — editor.html's own rule:
 * "the bubble menu is SUPPRESSED inside code").
 *
 * Marks (bold/italic/strike/code/link) apply over a text selection via
 * the bubble menu, and are carried by SetBlockContent — the only op that
 * can move Content.Marks at all (internal/doctext's live rope has no mark
 * storage of its own; RFC-001). The real design tradeoff this creates:
 * **once a block has any mark, every future edit to it — including plain
 * typing — routes through SetBlockContent instead of the fast anchor-based
 * Text ops**, because Text ops have no way to carry marks and would
 * silently strand them. An unmarked block keeps full real-time
 * character-level CRDT merging exactly as before; a marked block trades
 * that for whole-block last-write-wins (SetBlockContent's own Prev
 * precondition already requires an exact match, so two people editing the
 * very same marked block at the same instant occasionally get one edit
 * rejected rather than corrupted — a real, accepted cost, not a silent
 * bug). Marks survive ordinary typing via shiftMarksForEdit's plain
 * prefix/suffix diff, not a real text-transform — a mark spanning exactly
 * the edited region can be dropped rather than guessed at; see that
 * function's own doc comment.
 */
export function RichEditorPane({
  page,
  collab,
  onRename,
  diagnostics = [],
}: {
  page: Page;
  collab: CollabPage;
  onRename: (title: string) => void;
  /** RFC-003 §2's own diagnostics for this page (v2.3.0), keyed to a
   * block by `block_id` — rendered as editor.html's own LEFT GUTTER
   * marker (dotted amber, never a red squiggle), never re-derived here.
   * Optional so every existing caller/test keeps compiling unchanged. */
  diagnostics?: Diagnostic[];
}) {
  const { state, ready, blocks, peers, cursors, setCursor, setBlockText, setBlockContent, insertBlock, deleteBlock, setBlockKind, moveBlock } = collab;
  const pendingFocusId = useRef<string | null>(null);
  const rowRefs = useRef<Map<string, HTMLElement>>(new Map());
  const createdFirstBlock = useRef(false);
  const [kindMenu, setKindMenu] = useState<
    { mode: "convert"; blockId: string; textBeforeChoice: string; top: number; left: number }
    | { mode: "insert"; afterId: string; top: number; left: number }
    | { mode: "handle"; blockId: string; top: number; left: number }
    | null
  >(null);
  const [dragId, setDragId] = useState<string | null>(null);
  const [dropTarget, setDropTarget] = useState<{ id: string; zone: "before" | "after" } | null>(null);
  const [bubble, setBubble] = useState<{ blockId: string; start: number; end: number; top: number; left: number } | null>(null);
  // Toggle collapse is view state, not model state (RFC-001 §1) — a
  // client-local Set, never sent over the wire; collapsing/expanding a
  // Toggle does not touch collab at all.
  const [collapsedToggles, setCollapsedToggles] = useState<Set<string>>(new Set());
  const [peerCarets, setPeerCarets] = useState<Map<string, { rects: DOMRect[]; caretRect: DOMRect }>>(new Map());

  const diagnosticsByBlock = new Map<string, Diagnostic[]>();
  for (const d of diagnostics) {
    if (!d.block_id) continue;
    const list = diagnosticsByBlock.get(d.block_id) ?? [];
    list.push(d);
    diagnosticsByBlock.set(d.block_id, list);
  }

  // Recomputes every peer's on-screen caret/selection whenever their
  // reported offsets or this page's own block text changes — DOM
  // measurement (getBoundingClientRect) has to run after commit, so this
  // is an effect, not a render-time computation. Deliberately does not
  // recompute on scroll/resize alone (no listener for either): a peer's
  // caret can drift a little between their own next move and a window
  // resize, the same "good enough for a demo, not scroll-independently
  // live" tradeoff already accepted elsewhere in this file. code_block
  // peers are skipped entirely — <textarea> selection has no
  // getClientRects() equivalent; a peer editing code shows in the header
  // avatar list (they're still "present") but not with an inline caret,
  // a real, named gap rather than a faked position.
  useEffect(() => {
    const next = new Map<string, { rects: DOMRect[]; caretRect: DOMRect }>();
    for (const [actorId, cursor] of cursors) {
      const el = rowRefs.current.get(cursor.blockId);
      if (!el || el instanceof HTMLTextAreaElement) continue;
      const range = offsetsToRange(el, cursor.start, cursor.end);
      if (!range) continue;
      // getClientRects() on a collapsed range can still return one
      // zero-width rect in some browsers rather than an empty list — a
      // plain caret (start === end) is never a selection to highlight.
      const rects = cursor.start === cursor.end ? [] : Array.from(range.getClientRects());
      const caretRange = range.cloneRange();
      caretRange.collapse(false);
      next.set(actorId, { rects, caretRect: caretRange.getBoundingClientRect() });
    }
    setPeerCarets(next);
  }, [cursors, blocks]);

  useEffect(() => {
    const id = pendingFocusId.current;
    if (id && blocks.some((b) => b.id === id)) {
      pendingFocusId.current = null;
      const el = rowRefs.current.get(id);
      el?.focus();
    }
  }, [blocks]);

  // A brand-new (or freshly reset) page has zero blocks — there's nothing
  // to click into and no keyboard entry point, since Enter/Backspace only
  // exist inside an already-rendered block. `ready` (not state === "open")
  // is the correct gate: the socket opens strictly before the server's
  // first "snapshot" frame arrives, so blocks.length === 0 is also
  // (briefly) true for every page load, empty or not — checking `ready`
  // is what tells "genuinely empty" apart from "haven't heard yet."
  useEffect(() => {
    if (ready && blocks.length === 0 && !createdFirstBlock.current) {
      createdFirstBlock.current = true;
      const id = insertBlock(null, { tag: "paragraph" });
      pendingFocusId.current = id;
    }
  }, [ready, blocks, insertBlock]);

  // The bubble menu (raised on a real, non-collapsed selection) and this
  // client's own live cursor report (raised on ANY caret move, collapsed
  // or not — a plain caret is what most typing looks like) share one
  // listener: both need the same "which .editable block, what offsets"
  // resolution, and selectionchange already fires on every caret move
  // regardless of which triggered it. Never fires inside code — those
  // aren't .editable at all (a plain <textarea>, tracked separately by
  // CodeBlockField's own onSelect), so this simply never matches there.
  // Each EditableTextBlock stamps its own block id via BLOCK_ID_ATTR so
  // the selection can be mapped back to a block without a second ref
  // table.
  useEffect(() => {
    function onSelectionChange() {
      const sel = window.getSelection();
      if (!sel || sel.rangeCount === 0) {
        setBubble(null);
        setCursor(null, 0, 0);
        return;
      }
      const range = sel.getRangeAt(0);
      const anchor = range.startContainer instanceof Element ? range.startContainer : range.startContainer.parentElement;
      const editableEl = anchor?.closest(`[${BLOCK_ID_ATTR}]`) as HTMLElement | null;
      if (!editableEl || !editableEl.contains(range.endContainer)) {
        setBubble(null);
        setCursor(null, 0, 0);
        return;
      }
      const blockId = editableEl.getAttribute(BLOCK_ID_ATTR)!;
      const offsets = rangeToTextOffsets(editableEl, range);
      if (!offsets) {
        setBubble(null);
        setCursor(null, 0, 0);
        return;
      }
      setCursor(blockId, offsets.start, offsets.end);
      if (sel.isCollapsed || offsets.start === offsets.end) {
        setBubble(null);
        return;
      }
      const rect = range.getBoundingClientRect();
      setBubble({ blockId, start: offsets.start, end: offsets.end, top: rect.top - 44, left: rect.left + rect.width / 2 });
    }
    document.addEventListener("selectionchange", onSelectionChange);
    return () => document.removeEventListener("selectionchange", onSelectionChange);
  }, [setCursor]);

  // byId/depthOf/isHiddenByCollapsedToggle/listContextOf/visibleBlocks:
  // RFC-001 §1's containment rendered — blocks stays one flat,
  // depth-first-ordered array (mirroring documentcore.Page.Blocks
  // exactly), so nesting is a render-time derivation from each block's
  // own `parent`, never a second tree structure kept in sync by hand.
  const byId = new Map(blocks.map((b) => [b.id, b]));

  function depthOf(id: string): number {
    let depth = 0;
    let cur = byId.get(id)?.parent ?? null;
    while (cur !== null) {
      depth++;
      cur = byId.get(cur)?.parent ?? null;
    }
    return depth;
  }

  function isHiddenByCollapsedToggle(id: string): boolean {
    let cur = byId.get(id)?.parent ?? null;
    while (cur !== null) {
      const parent = byId.get(cur);
      if (parent && parent.kind.tag === "toggle" && collapsedToggles.has(parent.id)) return true;
      cur = parent?.parent ?? null;
    }
    return false;
  }

  // A List block itself never gets a visible row — only its ListItem
  // children do, each looking up its own list_kind/1-based index here.
  function listContextOf(b: BlockView): { kind: "bulleted" | "numbered" | "todo"; index: number } | null {
    if (b.parent === null) return null;
    const parent = byId.get(b.parent);
    if (!parent || parent.kind.tag !== "list") return null;
    const siblings = blocks.filter((x) => x.parent === b.parent);
    return { kind: parent.kind.list_kind, index: siblings.findIndex((x) => x.id === b.id) + 1 };
  }

  const visibleBlocks = blocks.filter((b) => b.kind.tag !== "list" && !isHiddenByCollapsedToggle(b.id));

  function handleEnter(afterId: string) {
    // Enter continues a ListItem as a sibling under the same List, and
    // stays under whatever non-list container (Toggle/Callout/Aside/
    // Quote) the current block is already in — never resets to a
    // top-level paragraph out from under an in-progress nested edit.
    const current = byId.get(afterId);
    const kind: BlockKind = current?.kind.tag === "list_item" ? { tag: "list_item", checked: false } : { tag: "paragraph" };
    const id = insertBlock(afterId, kind, current?.parent ?? null);
    pendingFocusId.current = id;
  }

  function handleBackspaceEmpty(blockId: string) {
    const idx = blocks.findIndex((b) => b.id === blockId);
    const prev = idx > 0 ? blocks[idx - 1] : null;
    deleteBlock(blockId);
    if (prev) pendingFocusId.current = prev.id;
  }

  // A block with marks routes every text change through setBlockContent
  // (shifting marks to follow the edit) instead of the fast anchor-based
  // setBlockText — see the module doc comment for why.
  function handleChangeText(block: BlockView, newText: string) {
    if (block.marks.length === 0) {
      setBlockText(block.id, newText);
    } else {
      setBlockContent(block.id, newText, shiftMarksForEdit(block.marks, block.text, newText));
    }
  }

  // editor.html's own trigger: typing "/" at the end of a block's text
  // opens a floating kind picker, positioned below the block (not at the
  // caret — computing a caret's screen position inside contentEditable is
  // its own can of worms; anchoring to the block's own rect is simpler
  // and just as usable). currentText is the live DOM value at the moment
  // "/" was typed, not blocks state — text sync is debounced
  // (REPLACE_DEBOUNCE_MS), so blocks wouldn't have this "/" yet.
  function handleSlashTrigger(blockId: string, el: HTMLElement, currentText: string) {
    const rect = el.getBoundingClientRect();
    setKindMenu({ mode: "convert", blockId, textBeforeChoice: currentText, top: rect.bottom + 6, left: rect.left });
  }

  // The insert-element bar: a persistent "+" per block (distinct from "/",
  // which converts the CURRENT block) that opens the same kind picker to
  // insert a brand-new block right after this one — a directly-discoverable
  // way to add any block kind without relying on Enter-then-convert.
  function handleInsertTrigger(afterId: string, el: HTMLElement) {
    const rect = el.getBoundingClientRect();
    setKindMenu({ mode: "insert", afterId, top: rect.bottom + 6, left: rect.left });
  }

  // The drag handle doubles as a click target (Notion's own convention):
  // dragging it reorders the block (existing onDragStart/onDragEnd), while
  // a plain click — no pointer movement, so the browser never fires
  // dragstart — opens the same kind-picker popup "/" and "+" already use,
  // with a Delete row appended. Keeps the block row itself free of a
  // permanently-visible <select>/"×" (the "make it like notion" ask), and
  // was also the actual cause of a real bug: that always-rendered toolbar
  // was wider than its -84px gutter offset, so it bled ~60px onto the
  // block's own text and ate clicks meant for typing.
  function handleHandleClick(blockId: string, el: HTMLElement) {
    const rect = el.getBoundingClientRect();
    setKindMenu({ mode: "handle", blockId, top: rect.bottom + 6, left: rect.left });
  }

  function chooseKind(kind: BlockKindKey) {
    if (!kindMenu) return;
    const listKind = LIST_KEYS[kind];
    if (listKind) {
      // A list is a container plus its first item, not a single in-place
      // conversion — always inserted fresh, right after the block that
      // triggered whichever menu mode is open, regardless of mode.
      const afterId = kindMenu.mode === "insert" ? kindMenu.afterId : kindMenu.blockId;
      const parent = byId.get(afterId)?.parent ?? null;
      setKindMenu(null);
      const listId = insertBlock(afterId, { tag: "list", list_kind: listKind }, parent);
      const itemId = insertBlock(null, { tag: "list_item", checked: false }, listId);
      pendingFocusId.current = itemId;
      return;
    }
    if (kindMenu.mode === "convert") {
      const { blockId, textBeforeChoice } = kindMenu;
      setKindMenu(null);
      if (textBeforeChoice.endsWith("/")) {
        setBlockText(blockId, textBeforeChoice.slice(0, -1));
      }
      setBlockKind(blockId, kindFromKey(kind));
    } else if (kindMenu.mode === "insert") {
      const { afterId } = kindMenu;
      setKindMenu(null);
      const parent = byId.get(afterId)?.parent ?? null;
      const id = insertBlock(afterId, kindFromKey(kind), parent);
      pendingFocusId.current = id;
    } else {
      const { blockId } = kindMenu;
      setKindMenu(null);
      setBlockKind(blockId, kindFromKey(kind));
    }
  }

  function handleDeleteFromMenu() {
    if (!kindMenu || kindMenu.mode !== "handle") return;
    const { blockId } = kindMenu;
    setKindMenu(null);
    deleteBlock(blockId);
  }

  function handleDragOver(e: DragEvent<HTMLDivElement>, id: string) {
    if (!dragId || dragId === id) return;
    e.preventDefault();
    const rect = e.currentTarget.getBoundingClientRect();
    const zone = e.clientY - rect.top < rect.height / 2 ? "before" : "after";
    setDropTarget({ id, zone });
  }

  function handleDrop(targetId: string) {
    if (!dragId || !dropTarget) return;
    const zone = dropTarget.zone;
    const draggedId = dragId;
    setDragId(null);
    setDropTarget(null);
    if (draggedId === targetId) return;
    const idx = blocks.findIndex((b) => b.id === targetId);
    const afterId = zone === "after" ? targetId : idx > 0 ? blocks[idx - 1].id : null;
    moveBlock(draggedId, afterId);
  }

  function toggleMark(kind: MarkKind) {
    if (!bubble) return;
    const block = blocks.find((b) => b.id === bubble.blockId);
    if (!block) return;
    const marks = isFullyMarked(block.marks, kind, bubble.start, bubble.end)
      ? removeMark(block.marks, kind, bubble.start, bubble.end)
      : addMark(block.marks, kind, bubble.start, bubble.end);
    setBlockContent(block.id, block.text, marks);
  }

  return (
    <main className="canvas">
      <article className="doc standard">
        <PageTitle title={page.title} onRename={onRename} />
        <div className="dek" style={{ display: "flex", alignItems: "center", gap: 8 }}>
          <span className={`state${state !== "open" ? " offline" : ""}`}>
            <span className="dot"></span>
            {state === "open" ? "Synced" : state === "connecting" ? "Connecting…" : "Disconnected"}
          </span>
          {peers.size > 0 && (
            <div className="avatars">
              {[...peers].map((p) => (
                <div className="avatar peer" key={p} title={p}>
                  {actorTag(p)}
                </div>
              ))}
            </div>
          )}
        </div>

        {visibleBlocks.map((b) => (
          <BlockRow
            key={b.id}
            block={b}
            depth={depthOf(b.id)}
            listContext={listContextOf(b)}
            diagnostics={diagnosticsByBlock.get(b.id) ?? EMPTY_DIAGNOSTICS}
            collapsed={collapsedToggles.has(b.id)}
            onToggleCollapse={() =>
              setCollapsedToggles((prev) => {
                const next = new Set(prev);
                if (next.has(b.id)) next.delete(b.id);
                else next.add(b.id);
                return next;
              })
            }
            onToggleChecked={() => {
              if (b.kind.tag === "list_item") setBlockKind(b.id, { tag: "list_item", checked: !b.kind.checked });
            }}
            disabled={state !== "open"}
            registerRef={(el) => {
              if (el) rowRefs.current.set(b.id, el);
              else rowRefs.current.delete(b.id);
            }}
            onChangeText={(text) => handleChangeText(b, text)}
            onEnter={() => handleEnter(b.id)}
            onBackspaceEmpty={() => handleBackspaceEmpty(b.id)}
            onSlashTrigger={(el, currentText) => handleSlashTrigger(b.id, el, currentText)}
            onInsertTrigger={(el) => handleInsertTrigger(b.id, el)}
            onHandleClick={(el) => handleHandleClick(b.id, el)}
            onCursorChange={(start, end) => (start === -1 ? setCursor(null, 0, 0) : setCursor(b.id, start, end))}
            dragging={dragId === b.id}
            dropZone={dropTarget?.id === b.id ? dropTarget.zone : null}
            onDragStart={() => setDragId(b.id)}
            onDragOver={(e) => handleDragOver(e, b.id)}
            onDrop={() => handleDrop(b.id)}
            onDragEnd={() => { setDragId(null); setDropTarget(null); }}
          />
        ))}

        {kindMenu && (
          <>
            <div style={{ position: "fixed", inset: 0, zIndex: 29 }} onClick={() => setKindMenu(null)} />
            <div className="slash" style={{ top: kindMenu.top, left: kindMenu.left }}>
              {KIND_ORDER.map((k) => (
                <div key={k} className="palette-item" onClick={() => chooseKind(k)}>
                  <span className="lead mono muted">{k === "heading1" ? "H1" : k === "heading2" ? "H2" : k === "heading3" ? "H3" : k === "code_block" ? "<>" : k === "divider" ? "—" : k === "quote" ? "❝" : "¶"}</span>
                  {KIND_LABELS[k]}
                </div>
              ))}
              {kindMenu.mode === "handle" && (
                <>
                  <div className="menu-divider" />
                  <div className="palette-item" onClick={handleDeleteFromMenu}>
                    <span className="lead mono muted">×</span>
                    Delete
                  </div>
                </>
              )}
            </div>
          </>
        )}

        {bubble && (
          <div className="bubble open" style={{ top: bubble.top, left: bubble.left, transform: "translateX(-50%)" }}>
            <button title="Bold" onMouseDown={(e) => e.preventDefault()} onClick={() => toggleMark({ tag: "bold" })}><b>B</b></button>
            <button title="Italic" onMouseDown={(e) => e.preventDefault()} onClick={() => toggleMark({ tag: "italic" })}><i>I</i></button>
            <button title="Strikethrough" onMouseDown={(e) => e.preventDefault()} onClick={() => toggleMark({ tag: "strike" })}><s>S</s></button>
            <button title="Inline code" style={{ fontFamily: "var(--mono)", fontSize: 12 }} onMouseDown={(e) => e.preventDefault()} onClick={() => toggleMark({ tag: "code" })}>&lt;&gt;</button>
            <span className="sep"></span>
            <button
              title="Link"
              onMouseDown={(e) => e.preventDefault()}
              onClick={() => {
                const url = window.prompt("Link URL:");
                if (url) toggleMark({ tag: "link", url });
              }}
            >
              🔗
            </button>
          </div>
        )}

        {[...peerCarets].map(([actorId, { rects, caretRect }]) => (
          <div key={actorId}>
            {rects.map((r, i) => (
              <div
                key={i}
                className="peer-selection"
                style={{ top: r.top, left: r.left, width: r.width, height: r.height }}
              />
            ))}
            <div className="peer-caret" style={{ top: caretRect.top, left: caretRect.left, height: caretRect.height || 20 }}>
              <span className="peer-caret-tag">{actorTag(actorId)}</span>
            </div>
          </div>
        ))}

        <div className="note">
          Every block is its own live document — open this same page in a second tab (or ask
          someone else to open its "Copy link") to see block edits sync in real time. Select text
          for formatting; hover a block's left margin for its drag handle and "+", or click the
          handle to change its kind or delete it.
        </div>
      </article>
    </main>
  );
}

/**
 * The page's own title — a real contentEditable h1, not a plain heading,
 * so the title has always been rename-able server-side (RenamePage;
 * usePageTree.renamePage) but never had any UI actually wired to it. Same
 * debounce/blur-flush/focus-guard shape as EditableTextBlock, minus marks
 * and minus multi-block concerns: Enter blurs (a title is one line, not a
 * block sequence) rather than inserting a new block, and an emptied title
 * is never sent (validateTitle rejects empty server-side anyway) — it
 * just reverts to the last confirmed title once the field blurs, since
 * the sync effect only skips the DOM write while focused.
 */
function PageTitle({ title, onRename }: { title: string; onRename: (title: string) => void }) {
  const ref = useRef<HTMLHeadingElement | null>(null);
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const pendingRef = useRef<string | null>(null);
  const onRenameRef = useRef(onRename);
  useEffect(() => {
    onRenameRef.current = onRename;
  });

  useEffect(() => {
    if (ref.current === document.activeElement) return;
    if (ref.current && ref.current.textContent !== title) {
      ref.current.textContent = title;
    }
  }, [title]);

  function flushPending() {
    if (debounceRef.current && pendingRef.current !== null) {
      clearTimeout(debounceRef.current);
      const value = pendingRef.current;
      pendingRef.current = null;
      if (value.trim() !== "") onRenameRef.current(value);
    }
  }

  useEffect(() => {
    return flushPending;
  }, []);

  function handleInput(e: FormEvent<HTMLHeadingElement>) {
    const value = e.currentTarget.textContent ?? "";
    pendingRef.current = value;
    if (debounceRef.current) clearTimeout(debounceRef.current);
    debounceRef.current = setTimeout(() => {
      pendingRef.current = null;
      if (value.trim() !== "") onRenameRef.current(value);
    }, REPLACE_DEBOUNCE_MS);
  }

  return (
    <h1
      ref={ref}
      style={{ fontFamily: "var(--display)", fontWeight: 560 }}
      className="editable"
      contentEditable
      suppressContentEditableWarning
      onInput={handleInput}
      onBlur={flushPending}
      onKeyDown={(e) => {
        if (e.key === "Enter") {
          e.preventDefault();
          e.currentTarget.blur();
        }
      }}
      data-placeholder="Untitled"
    />
  );
}

/** Walks root's text nodes in document order to convert a DOM Range into
 * plain-text character offsets — the inverse of renderMarkedHTML's own
 * segment splitting. Only resolves ranges whose start/end containers are
 * themselves text nodes (always true for a real, non-empty text
 * selection, which is the only time this is called). */
function rangeToTextOffsets(root: HTMLElement, range: Range): { start: number; end: number } | null {
  const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT);
  let offset = 0;
  let start = -1;
  let end = -1;
  let node: Node | null;
  while ((node = walker.nextNode())) {
    const len = node.textContent?.length ?? 0;
    if (node === range.startContainer) start = offset + range.startOffset;
    if (node === range.endContainer) end = offset + range.endOffset;
    offset += len;
  }
  if (start === -1 || end === -1) return null;
  return { start: Math.min(start, end), end: Math.max(start, end) };
}

/** The inverse of rangeToTextOffsets: builds a DOM Range spanning
 * [start, end) plain-text character offsets inside root — what a peer's
 * cursor report (character offsets over the wire) needs to become a
 * screen position. Returns null if root's actual text is shorter than
 * start (the block was edited out from under a stale cursor report; the
 * caller should just skip rendering that peer's caret until their next
 * update, not guess). */
function offsetsToRange(root: HTMLElement, start: number, end: number): Range | null {
  const walker = document.createTreeWalker(root, NodeFilter.SHOW_TEXT);
  let acc = 0;
  let startNode: Node | null = null;
  let startOffset = 0;
  let endNode: Node | null = null;
  let endOffset = 0;
  let node: Node | null;
  while ((node = walker.nextNode())) {
    const len = node.textContent?.length ?? 0;
    if (startNode === null && start <= acc + len) {
      startNode = node;
      startOffset = start - acc;
    }
    if (endNode === null && end <= acc + len) {
      endNode = node;
      endOffset = end - acc;
    }
    acc += len;
  }
  if (!startNode || !endNode) return null;
  const range = document.createRange();
  range.setStart(startNode, startOffset);
  range.setEnd(endNode, endOffset);
  return range;
}

function BlockRow({
  block,
  depth,
  listContext,
  diagnostics,
  collapsed,
  onToggleCollapse,
  onToggleChecked,
  disabled,
  registerRef,
  onChangeText,
  onEnter,
  onBackspaceEmpty,
  onSlashTrigger,
  onInsertTrigger,
  onHandleClick,
  onCursorChange,
  dragging,
  dropZone,
  onDragStart,
  onDragOver,
  onDrop,
  onDragEnd,
}: {
  block: BlockView;
  /** Nesting depth (RFC-001 §1's containment) — 0 for a top-level block,
   * derived from the parent chain, never stored. */
  depth: number;
  /** Set only for a ListItem — its parent List's own list_kind, plus
   * this item's 1-based position among its List siblings (for numbered
   * lists). null for every other kind, including List itself (which
   * never gets a row of its own). */
  listContext: { kind: "bulleted" | "numbered" | "todo"; index: number } | null;
  /** This block's own diagnostics (v2.3.0) — never empty-vs-absent
   * ambiguity, RichEditorPane always passes EMPTY_DIAGNOSTICS rather than
   * undefined. */
  diagnostics: Diagnostic[];
  /** Toggle-only view state (RFC-001 §1: not model state) — whether this
   * block's children are currently hidden. Meaningless for any other kind. */
  collapsed: boolean;
  onToggleCollapse: () => void;
  /** ListItem-only — flips Checked via SetBlockKind (Checked lives on
   * BlockKind, not Content — block.go's own field-per-tag pattern). */
  onToggleChecked: () => void;
  disabled: boolean;
  registerRef: (el: HTMLElement | null) => void;
  onChangeText: (text: string) => void;
  onEnter: () => void;
  onBackspaceEmpty: () => void;
  onSlashTrigger: (el: HTMLElement, currentText: string) => void;
  onInsertTrigger: (el: HTMLElement) => void;
  onHandleClick: (el: HTMLElement) => void;
  onCursorChange: (start: number, end: number) => void;
  dragging: boolean;
  dropZone: "before" | "after" | null;
  onDragStart: () => void;
  onDragOver: (e: DragEvent<HTMLDivElement>) => void;
  onDrop: () => void;
  onDragEnd: () => void;
}) {
  const tag = block.kind.tag;
  const kindKey = tag === "list_item" || tag === "toggle" || tag === "callout" || tag === "aside" || tag === "image" ? null : keyOf(block.kind);

  let body: ReactNode;
  if (tag === "divider") {
    body = <hr className="block-divider" />;
  } else if (tag === "code_block") {
    body = (
      <CodeBlockField
        text={block.text}
        disabled={disabled}
        registerRef={registerRef}
        onChangeText={onChangeText}
        onBackspaceEmpty={onBackspaceEmpty}
        onCursorChange={onCursorChange}
      />
    );
  } else if (tag === "image") {
    // No upload/asset pipeline exists yet (RFC-001 §1, §10) — a labeled
    // placeholder standing in for what would otherwise be a broken <img>.
    body = (
      <div style={{ border: "1px dashed var(--border, #999)", borderRadius: 6, padding: "24px 16px", textAlign: "center", color: "var(--muted, #888)" }}>
        🖼 Image {block.kind.tag === "image" && block.kind.file_id ? `(${block.kind.file_id})` : "(no file yet — upload not implemented)"}
      </div>
    );
  } else if (tag === "list_item") {
    const prefix =
      listContext?.kind === "numbered" ? (
        <span className="mono muted" style={{ minWidth: 20, textAlign: "right" }}>{listContext.index}.</span>
      ) : listContext?.kind === "todo" ? (
        <input type="checkbox" checked={block.kind.tag === "list_item" && !!block.kind.checked} onChange={onToggleChecked} disabled={disabled} />
      ) : (
        <span className="muted">•</span>
      );
    body = (
      <div style={{ display: "flex", alignItems: "flex-start", gap: 8 }}>
        {prefix}
        <EditableTextBlock
          blockId={block.id}
          tag="p"
          text={block.text}
          marks={block.marks}
          disabled={disabled}
          registerRef={registerRef}
          onChangeText={onChangeText}
          onEnter={onEnter}
          onBackspaceEmpty={onBackspaceEmpty}
          onSlashTrigger={onSlashTrigger}
        />
      </div>
    );
  } else if (tag === "toggle") {
    body = (
      <div style={{ display: "flex", alignItems: "flex-start", gap: 6 }}>
        <span onClick={onToggleCollapse} title={collapsed ? "Expand" : "Collapse"} style={{ cursor: "pointer", userSelect: "none", marginTop: 2 }}>
          {collapsed ? "▶" : "▼"}
        </span>
        <EditableTextBlock
          blockId={block.id}
          tag="p"
          text={block.text}
          marks={block.marks}
          disabled={disabled}
          registerRef={registerRef}
          onChangeText={onChangeText}
          onEnter={onEnter}
          onBackspaceEmpty={onBackspaceEmpty}
          onSlashTrigger={onSlashTrigger}
        />
      </div>
    );
  } else if (tag === "callout") {
    const tone = block.kind.tag === "callout" ? (block.kind.tone ?? "warn") : "warn";
    body = (
      <div style={{ display: "flex", alignItems: "flex-start", gap: 8, borderLeft: `3px solid ${CALLOUT_TONE_COLOR[tone]}`, background: "rgba(128,128,128,0.06)", borderRadius: 4, padding: "8px 12px" }}>
        <span style={{ marginTop: 2 }}>{(block.kind.tag === "callout" && block.kind.icon) || "💡"}</span>
        <EditableTextBlock
          blockId={block.id}
          tag="p"
          text={block.text}
          marks={block.marks}
          disabled={disabled}
          registerRef={registerRef}
          onChangeText={onChangeText}
          onEnter={onEnter}
          onBackspaceEmpty={onBackspaceEmpty}
          onSlashTrigger={onSlashTrigger}
        />
      </div>
    );
  } else if (tag === "aside") {
    body = (
      <div style={{ display: "flex", alignItems: "flex-start", gap: 8, background: "rgba(128,128,128,0.06)", borderRadius: 4, padding: "8px 12px" }}>
        <span style={{ marginTop: 2 }}>{block.kind.tag === "aside" ? block.kind.emoji : "💬"}</span>
        <EditableTextBlock
          blockId={block.id}
          tag="p"
          text={block.text}
          marks={block.marks}
          disabled={disabled}
          registerRef={registerRef}
          onChangeText={onChangeText}
          onEnter={onEnter}
          onBackspaceEmpty={onBackspaceEmpty}
          onSlashTrigger={onSlashTrigger}
        />
      </div>
    );
  } else {
    body = (
      <EditableTextBlock
        blockId={block.id}
        tag={kindKey === "heading1" ? "h1" : kindKey === "heading2" ? "h2" : kindKey === "heading3" ? "h3" : kindKey === "quote" ? "blockquote" : "p"}
        text={block.text}
        marks={block.marks}
        disabled={disabled}
        registerRef={registerRef}
        onChangeText={onChangeText}
        onEnter={onEnter}
        onBackspaceEmpty={onBackspaceEmpty}
        onSlashTrigger={onSlashTrigger}
      />
    );
  }

  return (
    <div
      className="block-row"
      onDragOver={onDragOver}
      onDrop={(e) => { e.preventDefault(); onDrop(); }}
      style={{
        opacity: dragging ? 0.4 : 1,
        marginLeft: depth * 24,
        borderTop: dropZone === "before" ? "2px solid var(--violet)" : "2px solid transparent",
        borderBottom: dropZone === "after" ? "2px solid var(--violet)" : "2px solid transparent",
      }}
    >
      {diagnostics.length > 0 && (
        <span className="gutter" title={diagnostics.map((d) => `${d.analyzer} — ${d.message}`).join("\n")}>
          ◌
        </span>
      )}
      <div className="block-toolbar">
        <span
          className="icon-btn"
          onClick={(e) => onInsertTrigger(e.currentTarget)}
          title="Insert a block below"
        >
          +
        </span>
        <span
          className="icon-btn"
          draggable
          onDragStart={onDragStart}
          onDragEnd={onDragEnd}
          onClick={(e) => onHandleClick(e.currentTarget)}
          title="Drag to reorder (same level only — see docs/porting/PROGRESS.md), click to change kind or delete"
          style={{ cursor: "grab" }}
        >
          ⠿
        </span>
      </div>

      {body}
    </div>
  );
}

function EditableTextBlock({
  blockId,
  tag,
  text,
  marks,
  disabled,
  registerRef,
  onChangeText,
  onEnter,
  onBackspaceEmpty,
  onSlashTrigger,
}: {
  blockId: string;
  tag: "h1" | "h2" | "h3" | "p" | "blockquote";
  text: string;
  marks: Mark[];
  disabled: boolean;
  registerRef: (el: HTMLElement | null) => void;
  onChangeText: (text: string) => void;
  onEnter: () => void;
  onBackspaceEmpty: () => void;
  onSlashTrigger: (el: HTMLElement, currentText: string) => void;
}) {
  const ref = useRef<HTMLElement | null>(null);
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const pendingValueRef = useRef<string | null>(null);
  const onChangeTextRef = useRef(onChangeText);
  useEffect(() => {
    onChangeTextRef.current = onChangeText;
  });

  // Flushes a still-pending debounced edit immediately, bypassing the
  // REPLACE_DEBOUNCE_MS wait. Wired to onBlur, not just unmount: switching
  // pages (clicking a different page in the left rail) resets
  // useCollabPage's whole session — closes the old socket, empties
  // liveRef/orderRef, points socketRef at the new page — synchronously,
  // in the SAME effect pass that reacts to the pageId change. That reset
  // finishes before this block's own unmount cleanup ever runs (its
  // parent's blocks array only goes empty, unmounting this component, on
  // the render *after* the reset), so by the time an unmount-only flush
  // fires, send() would already be talking to the wrong page — a silent
  // no-op, not a late-but-safe send. A DOM blur, though, fires
  // synchronously the moment focus moves to whatever was clicked —
  // *before* React processes the click that actually changes pages — so
  // flushing there reaches the old session while it's still live. Found
  // live as "backlinks not working": someone typed [[Page Title]] and
  // clicked away inside the 400ms debounce window, so the op never
  // reached the server at all — not a bug in the backlinks feature
  // itself. The unmount case is kept too, as a fallback for a block
  // disappearing without a prior blur (e.g. deleted out from under an
  // unfocused edit within the same page, where the session is still
  // valid).
  function flushPending() {
    if (debounceRef.current && pendingValueRef.current !== null) {
      clearTimeout(debounceRef.current);
      const value = pendingValueRef.current;
      pendingValueRef.current = null;
      onChangeTextRef.current(value);
    }
  }

  useEffect(() => {
    return flushPending;
  }, []);

  // Renders marks as real inline HTML (renderMarkedHTML always escapes raw
  // text; the only markup it ever produces is its own fixed tag set) —
  // ALWAYS applied, never skipped while focused. An earlier version
  // skipped the whole write whenever this block had focus (to protect the
  // caret from resetting to offset 0 on innerHTML reassignment), but that
  // meant a peer who simply clicked into a block someone else was editing
  // never saw the other person's edits arrive at all — not even after
  // blurring, since blurring alone doesn't re-run an effect keyed on
  // [text, marks]; only a *future* edit would happen to unstick it. Found
  // live: "editing same doc parallel[ly] not syncing." The correct fix
  // for the original cursor-jump bug is to preserve the caret/selection
  // across the write, not to skip the write — save this block's own
  // offsets before reassigning innerHTML, restore them after, and only
  // while still focused (a blurred block has nothing local to protect).
  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    const html = renderMarkedHTML(text, marks);
    if (el.innerHTML === html) return;

    const focused = el === document.activeElement;
    const sel = focused ? window.getSelection() : null;
    const savedOffsets = sel && sel.rangeCount > 0 ? rangeToTextOffsets(el, sel.getRangeAt(0)) : null;

    el.innerHTML = html;

    if (focused && savedOffsets && sel) {
      const restored = offsetsToRange(el, savedOffsets.start, savedOffsets.end);
      if (restored) {
        sel.removeAllRanges();
        sel.addRange(restored);
      }
    }
    // tag is a real dependency, not just text/marks: converting a
    // block's kind (SetBlockKind) changes the rendered `<Tag>` (e.g.
    // p -> h1) without changing text/marks at all. React treats that as
    // a host-element type change at this position and swaps in a fresh,
    // empty DOM node — but if text/marks are the only deps, this effect
    // never re-runs to populate it (nothing it depends on changed), so
    // the fresh element stays empty until some *other* prop change
    // happens to fire it. Found live: "converting to another block hides
    // txt" — the text was never lost server-side (a reload always showed
    // it correctly), only this effect's own dependency array was
    // incomplete for the DOM-identity change a tag swap causes.
  }, [text, marks, tag]);

  function handleInput(e: FormEvent<HTMLElement>) {
    const value = e.currentTarget.textContent ?? "";
    pendingValueRef.current = value;
    if (value.endsWith("/")) {
      onSlashTrigger(e.currentTarget, value);
    }
    if (debounceRef.current) clearTimeout(debounceRef.current);
    debounceRef.current = setTimeout(() => {
      pendingValueRef.current = null;
      onChangeText(value);
    }, REPLACE_DEBOUNCE_MS);
  }

  function handleKeyDown(e: KeyboardEvent<HTMLElement>) {
    if (e.key === "Enter") {
      e.preventDefault();
      onEnter();
    } else if (e.key === "Backspace" && (e.currentTarget.textContent ?? "") === "") {
      e.preventDefault();
      onBackspaceEmpty();
    }
  }

  const Tag = tag as ElementType;
  return (
    <Tag
      ref={(el: HTMLElement | null) => {
        ref.current = el;
        registerRef(el);
      }}
      className="editable"
      contentEditable={!disabled}
      suppressContentEditableWarning
      onInput={handleInput}
      onKeyDown={handleKeyDown}
      onBlur={flushPending}
      data-placeholder={tag === "p" ? "Type something…" : undefined}
      {...{ [BLOCK_ID_ATTR]: blockId }}
    />
  );
}

function CodeBlockField({
  text,
  disabled,
  registerRef,
  onChangeText,
  onBackspaceEmpty,
  onCursorChange,
}: {
  text: string;
  disabled: boolean;
  registerRef: (el: HTMLElement | null) => void;
  onChangeText: (text: string) => void;
  onBackspaceEmpty: () => void;
  onCursorChange: (start: number, end: number) => void;
}) {
  const [draft, setDraft] = useState(text);
  const taRef = useRef<HTMLTextAreaElement | null>(null);
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const pendingValueRef = useRef<string | null>(null);
  const restoreSelectionRef = useRef<{ start: number; end: number } | null>(null);
  const onChangeTextRef = useRef(onChangeText);
  useEffect(() => {
    onChangeTextRef.current = onChangeText;
  });

  // Always applies an incoming text change, never skips it while focused
  // — same fix, same reason, as EditableTextBlock's own sync effect:
  // skipping the write whenever this block had focus meant a peer who'd
  // simply clicked into a code block someone else was editing never saw
  // their changes arrive, not even after blurring. Selection is preserved
  // across the update instead: this effect (keyed on the incoming `text`)
  // captures the current native selection before handing off to
  // `setDraft`; the effect below (keyed on `draft`, so it runs after
  // React commits the new value) restores it, but only immediately after
  // an external sync — a local edit via handleChange leaves
  // restoreSelectionRef null, so it's a no-op there and the browser's own
  // post-input cursor placement is left alone.
  useEffect(() => {
    const el = taRef.current;
    if (el && el === document.activeElement) {
      restoreSelectionRef.current = { start: el.selectionStart ?? 0, end: el.selectionEnd ?? 0 };
    }
    setDraft(text);
  }, [text]);

  useEffect(() => {
    const saved = restoreSelectionRef.current;
    if (!saved) return;
    restoreSelectionRef.current = null;
    const el = taRef.current;
    if (el && el === document.activeElement) {
      el.setSelectionRange(saved.start, saved.end);
    }
  }, [draft]);

  // Flushes a still-pending debounced edit immediately — wired to onBlur,
  // not just unmount; see EditableTextBlock's own comment for why blur is
  // the one that actually reaches a still-live session on a page switch.
  function flushPending() {
    if (debounceRef.current && pendingValueRef.current !== null) {
      clearTimeout(debounceRef.current);
      const value = pendingValueRef.current;
      pendingValueRef.current = null;
      onChangeTextRef.current(value);
    }
  }

  useEffect(() => {
    return flushPending;
  }, []);

  function handleChange(value: string) {
    setDraft(value);
    pendingValueRef.current = value;
    if (debounceRef.current) clearTimeout(debounceRef.current);
    debounceRef.current = setTimeout(() => {
      pendingValueRef.current = null;
      onChangeText(value);
    }, REPLACE_DEBOUNCE_MS);
  }

  // <textarea> selection isn't part of window.getSelection() at all, so
  // RichEditorPane's shared selectionchange listener (EditableTextBlock's
  // mechanism) never sees a code block's caret — reported directly here
  // instead, on the same browser events that already fire for a caret
  // move (select) or a fresh click/keystroke landing in a new spot.
  function reportCursor(e: { currentTarget: HTMLTextAreaElement }) {
    onCursorChange(e.currentTarget.selectionStart, e.currentTarget.selectionEnd);
  }

  return (
    <pre>
      <textarea
        ref={(el: HTMLTextAreaElement | null) => {
          taRef.current = el;
          registerRef(el);
        }}
        className="code-editable"
        value={draft}
        disabled={disabled}
        placeholder="code…"
        onChange={(e) => { handleChange(e.target.value); reportCursor(e); }}
        onSelect={reportCursor}
        onClick={reportCursor}
        onBlur={() => { flushPending(); onCursorChange(-1, -1); }}
        onKeyDown={(e) => {
          if (e.key === "Backspace" && draft === "") {
            e.preventDefault();
            onBackspaceEmpty();
          }
        }}
        spellCheck={false}
      />
    </pre>
  );
}
