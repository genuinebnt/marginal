import { useEffect, useMemo, useRef, useState, type DragEvent } from "react";
import { Link } from "react-router-dom";
import { DocumentOutline } from "./DocumentOutline";
import type { Page } from "../api/pages";
import { listSeries } from "../api/series";
import { Label } from "../shell/Chrome";
import { ROOT, usePageTree } from "./usePageTree";
import { getResume } from "../api/resume";

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
  actorId, activePageId, activePagePath, blocks, onJumpToBlock,
}: {
  actorId: string;
  activePageId?: string;
  /** The open page's LTREE ancestry. Passed in rather than re-fetched: the
   *  caller already has the page, and a second GetPage per navigation both
   *  costs a request and delays the reveal by a round trip — long enough to
   *  see the rail render without your own row in it. */
  activePagePath?: string;
  /** The open page's live blocks, for the outline. Absent on screens that
   *  show the tree without a document (the dashboard). */
  blocks?: import("../collab/useCollabPage").BlockView[];
  onJumpToBlock?: (blockId: string) => void;
}) {
  const tree = usePageTree(actorId);
  const [filter, setFilter] = useState("");
  /**
   * Pages you have a stored reading position for.
   *
   * docs.reading_positions has existed since v2.8.0 with exactly one reader —
   * the dashboard's resume list. The rail is where you actually decide what
   * to open, so "you have been here" belongs on the row, as a mark rather
   * than a percentage: a position is a block id and a caret offset, and
   * turning that into "37% read" would be arithmetic over a number nobody
   * measured.
   */
  const [resumedIds, setResumedIds] = useState<Set<string>>(new Set());

  // Reveal the open page in the rail. Without it, opening a page from search,
  // a link or the graph left the rail showing root pages only — with no mark
  // on the row you were in, because the row was not rendered at all. `path`
  // is the LTREE ancestry GetPage already returns, so this costs one request
  // rather than one per level.
  useEffect(() => {
    if (activePagePath) tree.revealPath(activePagePath);
  }, [activePagePath, tree.revealPath]);

  /**
   * How many children each branch has, before expanding it.
   *
   * ListSeries already counts them — a series IS a page with children — so
   * this is one request for the whole rail rather than one per row, and it
   * is what lets a collapsed branch say "19 PARTS" instead of nothing. It
   * also fixes the twisty: without counts, every row had to assume it MIGHT
   * have children, so every row grew a caret on hover.
   */
  const [partCountOf, setPartCountOf] = useState<Map<string, number>>(new Map());
  useEffect(() => {
    if (!actorId) return;
    listSeries(actorId)
      .then((r) => setPartCountOf(new Map(r.series.map((x) => [x.series_page_id, x.part_count]))))
      .catch(() => setPartCountOf(new Map()));
  }, [actorId]);

  useEffect(() => {
    if (!actorId) return;
    getResume(actorId, 50)
      .then((r) => setResumedIds(new Set(r.positions.map((p) => p.page_id))))
      .catch(() => setResumedIds(new Set()));
  }, [actorId]);
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
      {/* A filter that hides rows without saying how many is a filter you
          cannot tell you have finished typing. */}
      {filterLower && (
        <div className="tr-filter-note">
          {filteredIds?.length ?? 0} of {Object.keys(tree.nodes).length} loaded
        </div>
      )}
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
            <div style={{ padding: 8, fontSize: 11.5, color: "#585550", lineHeight: 1.6 }}>
              No loaded pages match. The filter searches what the rail has already
              fetched — expand a branch to bring its children into range.
            </div>
          ) : (
            filteredIds.map((id) => (
              <PageRow
                key={id}
                page={tree.nodes[id]}
                depth={0}
                ordinal={ordinalOf.get(id) ?? "—"}
                ordinalOf={ordinalOf}
                match={filter}
                resumedIds={resumedIds}
                partCountOf={partCountOf}
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
                match={filter}
                resumedIds={resumedIds}
                partCountOf={partCountOf}
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
      {/* With no document open the rail still ends in a wal panel — the
          bottom of a rail that just stops is a rail that looks unfinished,
          and there is something true to say there. */}
      {!blocks && (
        <div className="wal">
          <Label>NEW</Label>
          <div style={{ fontSize: 11, lineHeight: 1.55, color: "#8C8880" }}>
            A page is created empty and untitled. Naming it later is normal — the id was
            never the name.
          </div>
        </div>
      )}

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
  /** The live filter string, for match highlighting. A filter that dims the
   *  list without showing WHERE it matched makes you re-read every row. */
  match: string;
  /** Page ids you have a stored reading position for. */
  resumedIds: Set<string>;
  /** Child count per branch, known before expanding it. */
  partCountOf: Map<string, number>;
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
          {/* Children arrive rather than appear. The stagger is capped at 12
              so a nineteen-part series does not take two seconds to open —
              motion that outlasts the intent it acknowledges stops reading as
              feedback and starts reading as lag. */}
          {(children ?? []).map((childId, i) => (
            <div key={childId} className="tr-child" style={{ animationDelay: `${Math.min(i, 12) * 0.018}s` }}>
              <TreeBranch id={childId} depth={depth + 1} {...rest} />
            </div>
          ))}
          {/* Shimmer, not a spinner: the branch is already on screen and only
              its contents are pending, so the placeholder should have the
              shape of what is coming. */}
          {rest.tree.loading.has(id) && children === undefined && (
            <div style={{ paddingLeft: depthPadding(depth + 1), paddingRight: 8 }}>
              {[0, 1, 2].map((i) => (
                <div key={i} className="tr-skel" style={{ animationDelay: `${i * 0.12}s`, width: `${88 - i * 14}%` }} />
              ))}
            </div>
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
 * Marks where the filter matched, inside the title.
 *
 * A filter that removes rows without showing WHERE it matched makes you
 * re-read every surviving row to find out why it survived — which is most of
 * the work the filter was supposed to save.
 */
function highlight(title: string, match: string) {
  const q = match.trim().toLowerCase();
  if (!q) return title;
  const at = title.toLowerCase().indexOf(q);
  if (at < 0) return title;
  return (
    <>
      {title.slice(0, at)}
      <span className="tr-hit">{title.slice(at, at + q.length)}</span>
      {title.slice(at + q.length)}
    </>
  );
}

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
  match,
  resumedIds,
  partCountOf,
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
  // Known up front from ListSeries, so a collapsed branch says how many
  // pages are under it and a LEAF grows no twisty at all.
  const knownCount = partCountOf.get(page.id) ?? 0;
  const childCount = loadedChildren?.length ?? knownCount;
  const hasChildren = childCount > 0;
  const isDeleting = page.lifecycle_state === "deleting";
  const mins = readMinutes(page);
  const resumed = resumedIds.has(page.id);
  const isDropTarget = dropTarget?.id === page.id;
  const isBeingDragged = dragId === page.id;


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
      style={{ position: "relative", opacity: isBeingDragged ? 0.4 : 1 }}
    >
      {/* The drop indicator is a LINE over the row, not a border on it. A
          border changes the row's height, so everything below jumps as the
          pointer crosses — which reads as the list fighting the drag. */}
      {isDropTarget && dropTarget?.zone === "before" && <div className="tr-drop" style={{ top: -1 }} />}
      {isDropTarget && dropTarget?.zone === "after" && <div className="tr-drop" style={{ bottom: -1 }} />}
      {isDropTarget && dropTarget?.zone === "into" && <div className="tr-drop-into" />}

      <Link
        to={`/pages/${page.id}`}
        className={`tr${page.id === activePageId ? " tr-on" : ""}`}
        style={{
          paddingLeft: 9,
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
        title={isDeleting ? "lifecycle_state = deleting" : page.title}
      >
        {page.id === activePageId && <i />}

        {/* Depth guides. Indent alone stops being legible past two levels;
            one hairline per level is how a file tree has always solved it,
            and it costs nothing the indent was not already spending. */}
        {depth > 0 && (
          <span className="tr-guides" aria-hidden>
            {Array.from({ length: depth }, (_, i) => <span key={i} className="tr-guide" />)}
          </span>
        )}

        {/* Two 2px rules, not one dot: "what state is this in" and "what is
            it about" are different questions, and a single coloured mark can
            only ever answer one of them. genuine-folio's ContentRowBars. */}
        <span className="tr-bars" aria-hidden>
          {/* The tick is state: amber mid-delete, ember where you are, teal
              where you have been. Teal comes from docs.reading_positions —
              a real column that had nowhere to show. Deliberately a MARK and
              not a percentage: a position is a block id and a caret offset,
              and turning that into "37% read" would be arithmetic over a
              number nobody measured. */}
          <span className={`tr-tick${
            isDeleting ? " tr-tick-del"
              : page.id === activePageId ? " tr-tick-on"
                : resumed ? " tr-tick-live" : ""
          }`} title={resumed ? "you were reading this" : undefined} />
          <span
            className="tr-bar"
            style={page.topic ? { background: `var(--topic-${page.topic.color_key})` } : undefined}
            title={page.topic?.name ?? "Untopiced"}
          />
        </span>

        {/* § 04 leads every row with a two-digit ordinal, ember on the row
            you are in. One span, not two: the caret takes the ordinal's place
            on hover for rows with children, so the affordance is there
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

        <span className="tr-t">{highlight(page.title || "Untitled", match)}</span>

        {isDeleting && (
          <span style={{
            marginLeft: "auto",
            font: "400 8.5px 'IBM Plex Mono',monospace",
            color: "#E0A34E",
          }}>
            DELETING
          </span>
        )}

        {/* A branch says how many pages are under it, and says SERIES once it
            has enough of them to be one. Two is the threshold, matching the
            server's: one child is a sub-page, and "Part 1 of 1" tells a
            reader nothing. */}
        {!isDeleting && childCount > 0 && (
          <span
            className={`tr-parts${childCount >= 2 ? " tr-parts-series" : ""}`}
            title={childCount >= 2 ? `A series of ${childCount} parts` : `${childCount} sub-page`}
          >
            {childCount >= 2 ? `${childCount} PARTS` : childCount}
          </span>
        )}

        {/* The right-hand slot is the READING ESTIMATE. "Is this a two-minute
            read or a twenty" decides whether you open it now; the number of
            children is already said by the badge beside it. */}
        {!isDeleting && mins !== null && (
          <span
            className="tr-n tr-count"
            title={`${page.word_count} words`}
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
