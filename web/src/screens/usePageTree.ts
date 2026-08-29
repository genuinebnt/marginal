import { useCallback, useEffect, useRef, useState } from "react";
import { createPage, deletePage, listPages, renamePage as renamePageApi, reparentPage, type Page } from "../api/pages";

// The empty string is the root sentinel — ListPages' own convention
// (parent_id omitted from the request = root pages), reused here as a map
// key since "" can never collide with a real page id.
const ROOT = "";

export interface PageTree {
  /** Every page fetched so far, by id — a page can be in this map without
   * being in any currently-*visible* list, if its parent hasn't been
   * expanded. */
  nodes: Record<string, Page>;
  /** parent id (or ROOT) → ordered child ids, undefined if never fetched. */
  childrenByParent: Record<string, string[] | undefined>;
  expanded: Set<string>;
  /** Expands every branch on the way to a page, from its `path`. */
  revealPath: (path: string) => void;
  loading: Set<string>;
  toggleExpand: (id: string) => void;
  createChildPage: (parentId: string, title: string) => Promise<Page>;
  renamePage: (id: string, title: string) => Promise<void>;
  deleteNode: (page: Page) => Promise<void>;
  /** Reparents id to be a child of newParentId, positioned right after
   * afterId (undefined = at the start of newParentId's children). */
  moveNode: (id: string, newParentId: string, afterId?: string) => Promise<void>;
}

/**
 * The left rail's page tree — lazily loaded, one ListPages call per
 * expanded node, because ListPages is a filter (direct children of one
 * parent), not a subtree walk (pages.md § List). There is no way to fetch
 * "everything" in one call, and no way to list soft-deleted pages at all
 * (pages.md § Delete: soft-deleted pages never appear in ListPages, and
 * there's no separate endpoint for them) — so unlike editor.html's mockup,
 * this tree has no "Recently deleted" section. That's a real backend gap,
 * not an oversight here.
 */
export function usePageTree(actorId: string): PageTree {
  const [nodes, setNodes] = useState<Record<string, Page>>({});
  const [childrenByParent, setChildrenByParent] = useState<Record<string, string[] | undefined>>({});
  const [expanded, setExpanded] = useState<Set<string>>(new Set());
  const [loading, setLoading] = useState<Set<string>>(new Set());
  const inFlight = useRef<Set<string>>(new Set());

  const fetchChildren = useCallback(
    async (parentId: string) => {
      if (inFlight.current.has(parentId)) return;
      inFlight.current.add(parentId);
      setLoading((prev) => new Set(prev).add(parentId));
      try {
        const { pages } = await listPages(actorId, parentId || undefined);
        setNodes((prev) => {
          const next = { ...prev };
          for (const p of pages) next[p.id] = p;
          return next;
        });
        setChildrenByParent((prev) => ({ ...prev, [parentId]: pages.map((p) => p.id) }));
      } finally {
        inFlight.current.delete(parentId);
        setLoading((prev) => {
          const next = new Set(prev);
          next.delete(parentId);
          return next;
        });
      }
    },
    [actorId],
  );

  useEffect(() => {
    setNodes({});
    setChildrenByParent({});
    setExpanded(new Set());
    void fetchChildren(ROOT);
    // fetchChildren intentionally excluded — it's stable per actorId, and
    // including it would re-run this on every render since it's a new
    // function identity whenever nodes/childrenByParent change.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [actorId]);

  /**
   * Expanding is a STATE change; fetching is what that state implies.
   *
   * Both toggleExpand and revealPath used to fetch inline, and both were
   * wrong in the same way: a state updater must be pure, and React calls
   * them twice under StrictMode — so a fetch inside one fires twice, and a
   * fetch before one races the instance the result belongs to. That is what
   * left a branch marked expanded whose children never arrived: `expanded`
   * held the id, `childrenByParent` did not, and the branch rendered as an
   * empty row with no error anywhere.
   *
   * So the effect below is the single fetch trigger: every expanded id whose
   * children are unknown gets loaded, once, whoever asked for it and however
   * many times React re-runs the render that asked.
   */
  useEffect(() => {
    for (const id of expanded) {
      if (childrenByParent[id] === undefined) void fetchChildren(id);
    }
  }, [expanded, childrenByParent, fetchChildren]);

  const toggleExpand = useCallback((id: string) => {
    setExpanded((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }, []);

  /**
   * Opens every branch on the way to `id`, so the page you are looking at is
   * visible in the rail rather than hidden three collapsed levels down.
   *
   * The ancestor chain comes from `path` — the materialised LTREE ancestry
   * `GetPage` already returns — so this is ONE request, not one per level.
   * The labels are `p` + the uuid with its dashes removed (docs.pages's own
   * encoding: an LTREE label may not contain a hyphen), and decoding them
   * here is why the rail does not need a "walk up the tree" endpoint.
   */
  const revealPath = useCallback(
    (path: string) => {
      const ancestors = path
        .split(".")
        .map(labelToId)
        .filter((v): v is string => v !== null);
      // The last label is the page itself; opening it would expand the page
      // you are ON, which is not what "reveal" means.
      const branches = ancestors.slice(0, -1);
      if (branches.length === 0) return;
      setExpanded((prev) => {
        const next = new Set(prev);
        for (const a of branches) next.add(a);
        return next;
      });
    },
    [],
  );

  const createChildPage = useCallback(
    async (parentId: string, title: string) => {
      const page = await createPage(actorId, title, parentId || undefined);
      setNodes((prev) => ({ ...prev, [page.id]: page }));
      setChildrenByParent((prev) => ({ ...prev, [parentId]: [...(prev[parentId] ?? []), page.id] }));
      if (parentId) setExpanded((prev) => new Set(prev).add(parentId));
      return page;
    },
    [actorId],
  );

  const renamePageInTree = useCallback(
    async (id: string, title: string) => {
      const page = await renamePageApi(actorId, id, title);
      setNodes((prev) => ({ ...prev, [id]: page }));
    },
    [actorId],
  );

  const deleteNode = useCallback(
    async (page: Page) => {
      await deletePage(actorId, page.id);
      const parentKey = page.parent_id ?? ROOT;
      setChildrenByParent((prev) => ({ ...prev, [parentKey]: (prev[parentKey] ?? []).filter((id) => id !== page.id) }));
      setNodes((prev) => {
        const next = { ...prev };
        delete next[page.id];
        return next;
      });
    },
    [actorId],
  );

  const moveNode = useCallback(
    async (id: string, newParentId: string, afterId?: string) => {
      const moving = nodes[id];
      if (!moving) return;
      const oldParentKey = moving.parent_id ?? ROOT;
      const page = await reparentPage(actorId, id, newParentId, afterId);
      setNodes((prev) => ({ ...prev, [id]: page }));
      // Refresh both the source and destination sibling lists from the
      // server rather than guessing the new order client-side — sort_key
      // recomputation is document-service's own job (internal/sortkey).
      await fetchChildren(oldParentKey);
      await fetchChildren(newParentId || ROOT);
      if (newParentId) setExpanded((prev) => new Set(prev).add(newParentId));
    },
    [actorId, nodes, fetchChildren],
  );

  return {
    revealPath,
    nodes,
    childrenByParent,
    expanded,
    loading,
    toggleExpand,
    createChildPage,
    renamePage: renamePageInTree,
    deleteNode,
    moveNode,
  };
}

export { ROOT };

/**
 * `p01a04c1a67cd7c3b9c3eeecffdd5c460` → `01a04c1a-67cd-7c3b-9c3e-eecffdd5c460`.
 *
 * docs.pages.path is an LTREE, whose labels may not contain a hyphen, so
 * every page id is stored with its dashes removed and a `p` in front (an
 * LTREE label may not start with a digit either). Returns null rather than a
 * malformed id for anything that does not match: `path` is documented as a
 * cache of ancestry and never as an address, and guessing at a label that
 * does not decode would turn a display concern into a bad request.
 */
function labelToId(label: string): string | null {
  const hex = label.startsWith("p") ? label.slice(1) : label;
  if (!/^[0-9a-f]{32}$/i.test(hex)) return null;
  return [hex.slice(0, 8), hex.slice(8, 12), hex.slice(12, 16), hex.slice(16, 20), hex.slice(20)]
    .join("-")
    .toLowerCase();
}
