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
      <PageRow page={page} depth={depth} {...rest} />
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

function depthPadding(depth: number): number {
  return depth === 0 ? 8 : 22 + (depth - 1) * 16;
}

function PageRow({
  page,
  depth,
  activePageId,
  tree,
  dragId,
  dropTarget,
  onDragStart,
  onDragOver,
  onDrop,
  onDragEnd,
  startCreate,
}: TreeProps & { page: Page; depth: number }) {
  const rowRef = useRef<HTMLDivElement>(null);
  const expanded = tree.expanded.has(page.id);
  const loadedChildren = tree.childrenByParent[page.id];
  // Undefined means "not loaded yet", which is different from "none" — the
  // twisty must appear for an unexpanded branch, but the COUNT must not
  // claim a number nobody has fetched.
  const hasChildren = (loadedChildren?.length ?? 0) > 0 || loadedChildren === undefined;
  const childCount = loadedChildren?.length ?? 0;
  const isDeleting = page.lifecycle_state === "deleting";
  const isDropTarget = dropTarget?.id === page.id;
  const isBeingDragged = dragId === page.id;

  const style = depth >= 2 ? { paddingLeft: depthPadding(depth) } : undefined;

  return (
    <div
      ref={rowRef}
      draggable
      onDragStart={() => onDragStart(page.id)}
      onDragOver={(e) => onDragOver(e, page.id)}
      onDrop={(e) => { e.preventDefault(); onDrop(page); }}
      onDragEnd={onDragEnd}
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
        {hasChildren ? (
          <span
            className="tr-n"
            style={{ cursor: "pointer", width: 9 }}
            onClick={(e) => {
              e.preventDefault();
              e.stopPropagation();
              tree.toggleExpand(page.id);
            }}
          >
            {expanded ? "▾" : "▸"}
          </span>
        ) : (
          <span className="tr-n" style={{ width: 9 }} />
        )}
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
        {!isDeleting && hasChildren && (
          <span className="tr-n tr-count" title={`${childCount} sub-pages`}>{childCount}</span>
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
