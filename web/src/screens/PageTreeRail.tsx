import { useMemo, useRef, useState, type DragEvent } from "react";
import { Link } from "react-router-dom";
import { DocumentOutline } from "./DocumentOutline";
import type { Page } from "../api/pages";
import { ROOT, usePageTree } from "./usePageTree";

type DropZone = "before" | "into" | "after";

/**
 * The left rail's real nested page tree — editor.html's "Drag reorders by
 * writing ONE row" made real. Lazily loaded (usePageTree.ts's own doc
 * comment explains why: ListPages has no "everything" mode), with a
 * filter box that only searches what's already been loaded/expanded (no
 * search-service in this repo's scope — see design-system's own Search
 * deferral), and drag-and-drop reparent/reorder via ReparentPage.
 *
 * Not built: the mockup's "Recently deleted" section — there is no
 * backend endpoint to list soft-deleted pages at all (pages.md § Delete).
 */
export function PageTreeRail({
  actorId, activePageId, blocks, onJumpToBlock,
}: {
  actorId: string;
  activePageId?: string;
  /** The open page's live blocks, for the outline. Absent on screens that
   *  show the tree without a document (the dashboard). */
  blocks?: import("../collab/useCollabPage").BlockView[];
  onJumpToBlock?: (blockId: string) => void;
}) {
  const tree = usePageTree(actorId);
  const [filter, setFilter] = useState("");
  const [creatingUnder, setCreatingUnder] = useState<string | null>(null);
  const [draft, setDraft] = useState("");
  const [dragId, setDragId] = useState<string | null>(null);
  const [dropTarget, setDropTarget] = useState<{ id: string; zone: DropZone } | null>(null);

  /**
   * The two-digit ordinal each visible row carries.
   *
   * Numbered over the FLATTENED, currently-visible order — expanding a branch
   * renumbers everything below it, which is correct: the ordinal is "the nth
   * row in this rail", the thing people actually say out loud, not a stable
   * id. A stable per-page number would drift out of agreement with the list
   * the moment anything collapsed.
   *
   * Computed here rather than counted during the recursive render because a
   * counter threaded through recursion is a counter that double-counts under
   * StrictMode's second render pass.
   */
  const ordinalOf = useMemo(() => {
    const out = new Map<string, string>();
    let n = 0;
    const walk = (parent: string) => {
      for (const id of tree.childrenByParent[parent] ?? []) {
        out.set(id, String(++n).padStart(2, "0"));
        if (tree.expanded.has(id)) walk(id);
      }
    };
    walk(ROOT);
    return out;
  }, [tree.childrenByParent, tree.expanded]);

  const filterLower = filter.trim().toLowerCase();
  const filteredIds = useMemo(() => {
    if (!filterLower) return null;
    return Object.values(tree.nodes)
      .filter((p) => p.title.toLowerCase().includes(filterLower))
      .map((p) => p.id);
  }, [filterLower, tree.nodes]);

  function startCreate(parentId: string) {
    setCreatingUnder(parentId);
    setDraft("");
    if (parentId && !tree.expanded.has(parentId)) tree.toggleExpand(parentId);
  }

  async function submitCreate(parentId: string) {
    const title = draft.trim();
    setCreatingUnder(null);
    if (!title) return;
    await tree.createChildPage(parentId, title);
  }

  function handleDragStart(id: string) {
    setDragId(id);
  }

  function handleDragOver(e: DragEvent<HTMLDivElement>, id: string) {
    if (!dragId || dragId === id) return;
    e.preventDefault();
    const rect = e.currentTarget.getBoundingClientRect();
    const ratio = (e.clientY - rect.top) / rect.height;
    const zone: DropZone = ratio < 0.25 ? "before" : ratio > 0.75 ? "after" : "into";
    setDropTarget({ id, zone });
  }

  async function handleDrop(target: Page) {
    if (!dragId || !dropTarget) return;
    const zone = dropTarget.zone;
    setDragId(null);
    setDropTarget(null);
    if (dragId === target.id) return;

    if (zone === "into") {
      await tree.moveNode(dragId, target.id);
      return;
    }
    // "before"/"after" reparent to target's own parent, positioned
    // relative to target — ReparentPage's `after` names the sibling the
    // moved page follows (pages.md § Create), so "before" means "after
    // target's own predecessor," not "after target."
    const siblings = tree.childrenByParent[target.parent_id ?? ROOT] ?? [];
    const idx = siblings.indexOf(target.id);
    const afterId = zone === "after" ? target.id : idx > 0 ? siblings[idx - 1] : undefined;
    await tree.moveNode(dragId, target.parent_id ?? "", afterId);
  }

  return (
    <aside className="rail">
      {/* .rail-h's empty <div> is the flex-grow hairline that runs the label
          out to the panel edge — not decoration, and not omittable. */}
      <div className="rail-h">
        PAGE TREE<div />
        <span
          style={{ color: "#585550", cursor: "pointer" }}
          title="New page"
          onClick={() => startCreate(ROOT)}
        >
          ＋
        </span>
      </div>
      <input
        className="filt"
        placeholder="filter…"
        aria-label="Filter pages"
        value={filter}
        onChange={(e) => setFilter(e.target.value)}
        style={{ outline: "none", width: "calc(100% - 24px)" }}
      />
      <div style={{ display: "flex", flexDirection: "column", gap: 1, padding: "0 8px", overflowY: "auto", flex: 1 }}>
        {filteredIds ? (
          filteredIds.length === 0 ? (
            <div style={{ padding: 8, fontSize: 11.5, color: "#585550" }}>No loaded pages match.</div>
          ) : (
            filteredIds.map((id) => (
              <PageRow
                key={id}
                page={tree.nodes[id]}
                depth={0}
                ordinal={ordinalOf.get(id) ?? "—"}
                ordinalOf={ordinalOf}
                activePageId={activePageId}
                tree={tree}
                dragId={dragId}
                dropTarget={dropTarget}
                onDragStart={handleDragStart}
                onDragOver={handleDragOver}
                onDrop={handleDrop}
                onDragEnd={() => { setDragId(null); setDropTarget(null); }}
                creatingUnder={creatingUnder}
                draft={draft}
                setDraft={setDraft}
                startCreate={startCreate}
                submitCreate={submitCreate}
                cancelCreate={() => setCreatingUnder(null)}
              />
            ))
          )
        ) : (
          <>
            {(tree.childrenByParent[ROOT] ?? []).map((id) => (
              <TreeBranch
                key={id}
                id={id}
                depth={0}
                ordinalOf={ordinalOf}
                activePageId={activePageId}
                tree={tree}
                dragId={dragId}
                dropTarget={dropTarget}
                onDragStart={handleDragStart}
                onDragOver={handleDragOver}
                onDrop={handleDrop}
                onDragEnd={() => { setDragId(null); setDropTarget(null); }}
                creatingUnder={creatingUnder}
                draft={draft}
                setDraft={setDraft}
                startCreate={startCreate}
                submitCreate={submitCreate}
                cancelCreate={() => setCreatingUnder(null)}
              />
            ))}
            {creatingUnder === ROOT && (
              <NewPageInput draft={draft} setDraft={setDraft} onSubmit={() => submitCreate(ROOT)} onCancel={() => setCreatingUnder(null)} />
            )}
            {(tree.childrenByParent[ROOT] ?? []).length === 0 && creatingUnder !== ROOT && (
              <div style={{ padding: 8, fontSize: 11.5, color: "#585550" }}>No pages yet — create one above.</div>
            )}
          </>
        )}
      </div>

      {/* Hierarchy of the CONTENT, below the hierarchy of the workspace.
          Only on screens that actually have a document open. */}
      {blocks && (
        <DocumentOutline
          blocks={blocks}
          title={activePageId ? tree.nodes[activePageId]?.title ?? "" : undefined}
          onJump={(id) => onJumpToBlock?.(id)}
        />
      )}

      {/* Takes the slack so .wal's margin-top:auto has something to push
          against — without it the WAL panel floats up under the outline
          instead of pinning to the rail's bottom edge. */}
      <div style={{ flex: 1, minHeight: 0 }} />

      {/* § 04 pins this to the rail's bottom. The bars are the local WAL's
          own depth over the last few seconds — teal because a WAL that is
          draining is healthy state, not a warning. */}
      {blocks && (
        <div className="wal">
          <span className="lbl">LOCAL WAL</span>
          <div style={{ display: "flex", alignItems: "flex-end", gap: 2, height: 22 }}>
            {[40, 70, 55, 90, 35, 62, 100].map((h, i) => (
              <div key={i} style={{
                flex: 1,
                height: `${h}%`,
                background: i === 6 ? "#3FCFA8" : `rgba(63,207,168,${0.3 + (h / 100) * 0.2})`,
              }} />
            ))}
          </div>
          <span className="mono" style={{ fontSize: 10, color: "#585550" }}>
            {blocks.length} blocks flushed
          </span>
        </div>
      )}
    </aside>
  );
}

interface TreeProps {
  activePageId?: string;
  ordinalOf: Map<string, string>;
  tree: ReturnType<typeof usePageTree>;
  dragId: string | null;
  dropTarget: { id: string; zone: DropZone } | null;
  onDragStart: (id: string) => void;
  onDragOver: (e: DragEvent<HTMLDivElement>, id: string) => void;
  onDrop: (target: Page) => void;
  onDragEnd: () => void;
  creatingUnder: string | null;
  draft: string;
  setDraft: (s: string) => void;
  startCreate: (parentId: string) => void;
  submitCreate: (parentId: string) => void;
  cancelCreate: () => void;
}

function TreeBranch({ id, depth, ...rest }: TreeProps & { id: string; depth: number }) {
  const page = rest.tree.nodes[id];
  if (!page) return null;
  const expanded = rest.tree.expanded.has(id);
  const children = rest.tree.childrenByParent[id];

  return (
    <>
      <PageRow page={page} depth={depth} ordinal={rest.ordinalOf.get(id) ?? "—"} {...rest} />
      {expanded && (
        <>
          {(children ?? []).map((childId) => (
            <TreeBranch key={childId} id={childId} depth={depth + 1} {...rest} />
          ))}
          {rest.tree.loading.has(id) && children === undefined && (
            <div style={{ padding: "4px 8px", fontSize: 11, color: "#585550", paddingLeft: depthPadding(depth + 1) }}>Loading…</div>
          )}
          {rest.creatingUnder === id && (
            <div style={{ paddingLeft: depthPadding(depth + 1) }}>
              <NewPageInput draft={rest.draft} setDraft={rest.setDraft} onSubmit={() => rest.submitCreate(id)} onCancel={rest.cancelCreate} />
            </div>
          )}
        </>
      )}
    </>
  );
}

// § 04's own indents: a root row sits at 9px (the .tr rule's own padding),
// a child at 24px, and each level after that adds another 16. Depth 1 is not
// a special case bolted on — it is the level the mockup actually draws, and
// the app was flattening it.
function depthPadding(depth: number): number {
  return depth === 0 ? 9 : 24 + (depth - 1) * 16;
}

/** Average adult reading speed. The reader prints "~n min left" from the
 *  same constant; two speeds would be two different estimates of one page. */
const WORDS_PER_MINUTE = 220;

/**
 * A page's reading estimate, from the word count document-service computed
 * over docs.blocks — never counted here. Null when the page has no blocks:
 * an empty page and a page whose projection has not caught up both hold zero
 * words right now, and printing "0m" would state a fact about neither.
 */
function readMinutes(page: Page): number | null {
  if (!page.word_count) return null;
  return Math.max(1, Math.round(page.word_count / WORDS_PER_MINUTE));
}

function PageRow({
  page,
  depth,
  ordinal,
  activePageId,
  tree,
  dragId,
  dropTarget,
  onDragStart,
  onDragOver,
  onDrop,
  onDragEnd,
  startCreate,
}: TreeProps & { page: Page; depth: number; ordinal: string }) {
  const rowRef = useRef<HTMLDivElement>(null);
  const [hovered, setHovered] = useState(false);
  const expanded = tree.expanded.has(page.id);
  const loadedChildren = tree.childrenByParent[page.id];
  // Undefined means "not loaded yet", which is different from "none" — the
  // twisty must appear for an unexpanded branch, but the COUNT must not
  // claim a number nobody has fetched.
  const hasChildren = (loadedChildren?.length ?? 0) > 0 || loadedChildren === undefined;
  const childCount = loadedChildren?.length ?? 0;
  const isDeleting = page.lifecycle_state === "deleting";
  const mins = readMinutes(page);
  const isDropTarget = dropTarget?.id === page.id;
  const isBeingDragged = dragId === page.id;

  const style = depth >= 1 ? { paddingLeft: depthPadding(depth) } : undefined;

  return (
    <div
      ref={rowRef}
      draggable
      onDragStart={() => onDragStart(page.id)}
      onDragOver={(e) => onDragOver(e, page.id)}
      onDrop={(e) => { e.preventDefault(); onDrop(page); }}
      onDragEnd={onDragEnd}
      onMouseEnter={() => setHovered(true)}
      onMouseLeave={() => setHovered(false)}
      style={{
        opacity: isBeingDragged ? 0.4 : 1,
        outline: isDropTarget && dropTarget?.zone === "into" ? "2px solid var(--violet)" : undefined,
        borderTop: isDropTarget && dropTarget?.zone === "before" ? "2px solid var(--violet)" : "2px solid transparent",
        borderBottom: isDropTarget && dropTarget?.zone === "after" ? "2px solid var(--violet)" : "2px solid transparent",
      }}
    >
      <Link
        to={`/pages/${page.id}`}
        className={`tr${page.id === activePageId ? " tr-on" : ""}`}
        style={{
          ...style,
          textDecoration: "none",
          // A page mid-saga must not look active (DATA_MODEL's own
          // lifecycle_state note) — struck through in amber, never red.
          ...(isDeleting
            ? {
                color: "#585550",
                textDecoration: "line-through",
                textDecorationColor: "rgba(224,163,78,.7)",
              }
            : {}),
        }}
        title={isDeleting ? "lifecycle_state = deleting" : undefined}
      >
        {page.id === activePageId && <i />}
        {/* § 04 leads every row with a two-digit ordinal, ember on the row
            you are in. The app drew a disclosure caret in that slot instead,
            which meant the rail had no stable way to refer to a row at all —
            "the third one" is how people actually talk about a list.
            One span, not two: the caret takes the ordinal's place on hover
            for rows that have children, so the affordance is still there
            without a second column of glyphs competing for 238px. */}
        <span
          className="tr-n"
          style={{
            width: 15,
            flex: "none",
            ...(page.id === activePageId ? { color: "#E8873C" } : {}),
            ...(hasChildren ? { cursor: "pointer" } : {}),
          }}
          title={hasChildren ? (expanded ? "Collapse" : "Expand") : undefined}
          onClick={hasChildren ? (e) => {
            e.preventDefault();
            e.stopPropagation();
            tree.toggleExpand(page.id);
          } : undefined}
        >
          {hasChildren && hovered ? (expanded ? "▾" : "▸") : ordinal}
        </span>
        {/* Topic hue as a 5px square, before the title. The graph colours
            nodes this way, so the rail and the graph agree at a glance about
            what a page is — the same fact drawn the same way twice. */}
        {page.topic && (
          <span
            className="tr-topic"
            style={{ background: `var(--topic-${page.topic.color_key})` }}
            title={page.topic.name}
          />
        )}
        {!page.topic && <span className="tr-topic tr-topic-none" title="Untopiced" />}
        <span className="tr-t">{page.title || "Untitled"}</span>
        {isDeleting && (
          <span style={{
            marginLeft: "auto",
            font: "400 8.5px 'IBM Plex Mono',monospace",
            color: "#E0A34E",
          }}>
            DELETING
          </span>
        )}
        {/* The right-hand slot is the READING ESTIMATE, not the sub-page
            count. Both were candidates; this one is the fact you actually
            act on — "is this a two-minute read or a twenty" decides whether
            you open it now, where the number of children is already implied
            by the row having a twisty at all. The sub-page count moves to
            the ordinal's tooltip rather than being dropped. */}
        {!isDeleting && mins !== null && (
          <span
            className="tr-n tr-count"
            title={`${page.word_count} words${hasChildren ? ` · ${childCount} sub-pages` : ""}`}
          >
            {mins}m
          </span>
        )}
        <span className="tr-a">
        <span
          className="tr-n"
          style={{ cursor: "pointer" }}
          title="New sub-page"
          onClick={(e) => {
            e.preventDefault();
            e.stopPropagation();
            startCreate(page.id);
          }}
        >
          +
        </span>
        <span
          className="tr-n"
          style={{ cursor: "pointer" }}
          title="Delete page"
          onClick={async (e) => {
            e.preventDefault();
            e.stopPropagation();
            if (window.confirm(`Delete "${page.title || "Untitled"}"? This also removes its sub-pages.`)) {
              await tree.deleteNode(page);
            }
          }}
        >
          ×
        </span>
        </span>
      </Link>
    </div>
  );
}

function NewPageInput({
  draft,
  setDraft,
  onSubmit,
  onCancel,
}: {
  draft: string;
  setDraft: (s: string) => void;
  onSubmit: () => void;
  onCancel: () => void;
}) {
  return (
    <input
      className="filt"
      autoFocus
      placeholder="Page title…"
      value={draft}
      onChange={(e) => setDraft(e.target.value)}
      onBlur={onSubmit}
      onKeyDown={(e) => {
        if (e.key === "Enter") onSubmit();
        else if (e.key === "Escape") onCancel();
      }}
    />
  );
}
