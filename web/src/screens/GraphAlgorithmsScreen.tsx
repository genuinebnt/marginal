/**
 * docs/ui-mockups/v2/index.html § 08 GRAPH ALGORITHMS, ported.
 *
 * Every number here is computed in Go (services/graphalgo) and read off
 * GraphService — connected components by flood fill, cycles by three-colour
 * DFS, BFS shortest path and its frontier widths, forward reachability, and
 * the Betti numbers. TypeScript draws them and nothing more; there is no
 * second implementation of any of it on this side.
 *
 * The lens strip selects which result is painted over the same layout rather
 * than swapping layouts, which is the mockup's own arrangement: the graph is
 * one thing, and these are five questions asked of it.
 */
import { useEffect, useMemo, useState } from "react";
import { useAuth } from "../auth/AuthContext";
import {
  analyzeGraph, getLinkGraph, graphNeighborhood,
  type GraphAnalysis, type GraphNeighborhood, type LinkGraph,
} from "../api/graph";
import { useForceLayout } from "../graph-core/useForceLayout";
import { useStagedReveal } from "../graph-core/useStagedReveal";
import {
  Body, Inspector, Label, Readout, Rule, Screen, StatusBar, SubBar, SubItem, TopBar, num,
} from "../shell/Chrome";

const W = 1104;
const H = 722;

type Lens =
  | "path" | "components" | "cycles" | "reach" | "blast" | "topology"
  | "scc" | "topo" | "nearest";

/**
 * The nine algorithms this screen paints, in the order § 08's strip lists
 * them, with the three that answer a question none of the others do appended.
 *
 * Each is a DIFFERENT question, and the point of putting them on one canvas
 * is that they disagree about the same graph:
 *
 *   COMPONENTS   can I get there at all, ignoring direction
 *   SCC          can I get there AND back, following links as written
 *   CYCLES       show me one loop, as a path
 *   TOPO SORT    an order in which nothing precedes what it links to
 *   NEAREST      the ranked ring around one page, by hop distance
 *   BLAST RADIUS what a cascading delete here would take with it
 */
const LENSES: Array<{ id: Lens; label: string }> = [
  { id: "path", label: "BFS · SHORTEST PATH" },
  { id: "nearest", label: "NEAREST" },
  { id: "components", label: "COMPONENTS" },
  { id: "scc", label: "SCC · TARJAN" },
  { id: "cycles", label: "CYCLES · 3-COLOUR DFS" },
  { id: "topo", label: "TOPO SORT · KAHN" },
  { id: "reach", label: "REACHABILITY" },
  { id: "blast", label: "BLAST RADIUS" },
  { id: "topology", label: "TOPOLOGY" },
];

/** Component colours. Not the topic ramp — these index a partition, and
 *  reusing topic hues would imply a page's component says what it is about. */
/** Ids ordered by hop distance, nearest first, ties on id so the reveal
 *  is the same every time — a wavefront that shuffles between runs is a
 *  wavefront nobody can read. */
function byDistance(dist: Record<string, number> | undefined): string[] {
  if (!dist) return [];
  return Object.entries(dist)
    .sort((a, b) => a[1] - b[1] || a[0].localeCompare(b[0]))
    .map(([id]) => id);
}

/**
 * What each lens's reveal is actually showing, in one line.
 *
 * Per-lens rather than shared: the whole complaint these animations answer
 * is that every lens looked the same, and a caption that says "N reached"
 * under all nine is the same defect in words.
 */
interface GestureCtx {
  source: string | null;
  target: string | null;
  hood: GraphNeighborhood | null;
  reveal: number;
  total: number;
  visited: number;
}

const LENS_GESTURE: Record<Lens, (c: GestureCtx) => string> = {
  path: (c) => {
    if (!c.source) return "click the page you are starting from";
    if (!c.target) return "now click where you want to get to";
    if (c.hood && !c.hood.path_exists) return "no route — these two are in different components";
    const hops = Math.max(0, (c.hood?.shortest_path?.length ?? 1) - 1);
    return `${hops} hop${hops === 1 ? "" : "s"} · BFS, undirected`;
  },
  nearest: (c) => `${c.reveal} of ${c.total} revealed · ranked outward by hops`,
  components: (c) => `${c.reveal} of ${c.total} filled · flood fill, one component at a time`,
  scc: (c) => c.total === 0
    ? "no multi-page loops — every component is a singleton"
    : `${c.reveal} of ${c.total} · Tarjan, only the real citation loops`,
  cycles: (c) => c.total === 0
    ? "no cycle — the link graph is acyclic"
    : `${c.reveal} of ${c.total} · the walk returns to where it began`,
  topo: (c) => `${c.reveal} of ${c.total} placed · Kahn peels a layer at a time`,
  reach: (c) => `${c.reveal} of ${c.total} downstream · directed, outbound links only`,
  blast: (c) => `${c.reveal} of ${c.total} would go with it · forward reachability`,
  topology: () => "Betti numbers are a property of the whole graph — nothing to walk",
};

const COMPONENT_HUES = ["#3FCFA8", "#7D9EC9", "#A98CE8", "#585550", "#D6A660", "#D07C8A"];

export function GraphAlgorithmsScreen() {
  const { session } = useAuth();
  const actorId = session?.actorId ?? null;

  const [graph, setGraph] = useState<LinkGraph | null>(null);
  const [analysis, setAnalysis] = useState<GraphAnalysis | null>(null);
  const [source, setSource] = useState<string | null>(null);
  /** The second click, on the lenses that connect two nodes. Null on
   *  every other lens — "how far is everything" needs one node, "how are
   *  these two connected" needs two, and the strip says which you are on. */
  const [target, setTarget] = useState<string | null>(null);
  const [hood, setHood] = useState<GraphNeighborhood | null>(null);
  const [lens, setLens] = useState<Lens>("path");
  const [insTab, setInsTab] = useState<"results" | "cost">("results");
  const [err, setErr] = useState<string | null>(null);

  useEffect(() => {
    if (!actorId) return;
    getLinkGraph(actorId).then(setGraph).catch((e) => setErr(String(e)));
    analyzeGraph(actorId).then(setAnalysis).catch(() => setAnalysis(null));
  }, [actorId]);

  useEffect(() => {
    if (!actorId || !source) { setHood(null); return; }
    graphNeighborhood(actorId, source, target ?? undefined)
      .then(setHood).catch(() => setHood(null));
  }, [actorId, source, target]);

  // A target only means something on the path lens, and carrying a stale
  // one across a lens change would leave the previous question's answer
  // half-painted under a different heading.
  useEffect(() => { setTarget(null); }, [lens]);

  /**
   * Land on the most-connected page rather than on nothing.
   *
   * Every source-relative lens (BFS, nearest, blast radius) is blank until a
   * source exists, so an unselected screen is a screen that looks broken. The
   * hub is the least arbitrary default: it is the page the graph itself
   * nominates, and every other node is reachable from it if anything is.
   * Explicit selection wins the moment there is one.
   */
  useEffect(() => {
    // NOT on the path lens. There the first click is the START, and
    // auto-picking a source silently consumes that slot — every click then
    // lands on the destination and the start looks stuck on whichever page
    // the graph nominated. Which is exactly how it behaved.
    if (lens === "path") return;
    if (source || !graph || graph.nodes.length === 0) return;
    const degree = new Map<string, number>();
    graph.edges.forEach((e) => {
      degree.set(e.from_page, (degree.get(e.from_page) ?? 0) + 1);
      degree.set(e.to_page, (degree.get(e.to_page) ?? 0) + 1);
    });
    // Ties break on page id, so the landing node does not move between loads.
    const best = [...graph.nodes]
      .sort((a, b) => (degree.get(b.id) ?? 0) - (degree.get(a.id) ?? 0) || a.id.localeCompare(b.id))[0];
    setSource(best.id);
  }, [graph, source, lens]);

  const nodeIds = useMemo(() => graph?.nodes.map((n) => n.id) ?? [], [graph]);
  const edges = useMemo(
    () => graph?.edges.map((e) => ({ from: e.from_page, to: e.to_page })) ?? [],
    [graph],
  );
  const { nodes } = useForceLayout(nodeIds, edges, W, H);

  const titleOf = useMemo(
    () => new Map(graph?.nodes.map((n) => [n.id, n.title]) ?? []),
    [graph],
  );

  // Same view-space fit the Graph screen uses, and for the same reason:
  // rescaling the simulation's coordinates would change what its next tick
  // measures.
  const fit = useMemo(() => {
    if (nodes.length < 2) return { s: 1, dx: 0, dy: 0 };
    const xs = nodes.map((n) => n.x), ys = nodes.map((n) => n.y);
    const minX = Math.min(...xs), maxX = Math.max(...xs);
    const minY = Math.min(...ys), maxY = Math.max(...ys);
    const padX = 150, padY = 60;
    const s = Math.min((W - padX * 2) / Math.max(maxX - minX, 1),
                       (H - padY * 2) / Math.max(maxY - minY, 1));
    return { s, dx: padX - minX * s, dy: padY - minY * s };
  }, [nodes]);
  const at = (id: string) => {
    const n = nodes.find((v) => v.id === id);
    return n ? { x: n.x * fit.s + fit.dx, y: n.y * fit.s + fit.dy } : undefined;
  };

  /** Component sizes, largest first — the flood fill's own partition. */
  const components = useMemo(() => {
    if (!analysis) return [];
    const sizes = new Map<number, number>();
    Object.values(analysis.component_of).forEach((c) => sizes.set(c, (sizes.get(c) ?? 0) + 1));
    return [...sizes].sort((a, b) => b[1] - a[1]);
  }, [analysis]);

  /** BFS frontier widths from the neighbourhood's own distance map. */
  const frontier = useMemo(() => {
    if (!hood) return [];
    const byLevel = new Map<number, number>();
    Object.values(hood.undirected_distance).forEach((d) => byLevel.set(d, (byLevel.get(d) ?? 0) + 1));
    return [...byLevel].sort((a, b) => a[0] - b[0]);
  }, [hood]);

  /** Edges over the complete graph's edge count — what makes every
   *  traversal here linear in practice rather than quadratic. */
  const density = useMemo(() => {
    const v = graph?.nodes.length ?? 0;
    const e = graph?.edges.length ?? 0;
    return v > 1 ? (2 * e) / (v * (v - 1)) : 0;
  }, [graph]);

  const cycleSet = useMemo(() => new Set(analysis?.cycle ?? []), [analysis]);

  /** How many pages each strongly connected component holds — a singleton is
   *  the normal case, so only >1 gets a hue. */
  const sccSize = useMemo(() => {
    const m = new Map<number, number>();
    Object.values(analysis?.strongly_connected ?? {}).forEach((c) => m.set(c, (m.get(c) ?? 0) + 1));
    return m;
  }, [analysis]);

  /** Dependency level per page, inverted out of `layers`. */
  const layerOf = useMemo(() => {
    const m = new Map<string, number>();
    (analysis?.layers ?? []).forEach((level, i) => level.forEach((id) => m.set(id, i)));
    return m;
  }, [analysis]);

  /** Position in the ranked ring around the source — 0 is nearest. */
  const nearestRank = useMemo(() => {
    const m = new Map<string, number>();
    (hood?.nearest ?? []).forEach((n, i) => m.set(n.page_id, i));
    return m;
  }, [hood]);
  const reachable = useMemo(() => new Set(Object.keys(hood?.forward_reachable ?? {})), [hood]);
  const visited = hood ? Object.keys(hood.undirected_distance).length : 0;
  const total = graph?.nodes.length ?? 0;

  /**
   * The order this lens reveals its answer in.
   *
   * Nothing here computes: every id below came from Go. What differs per
   * lens is the SEQUENCE, and the sequence is the algorithm's shape —
   * a BFS grows a ring at a time, Kahn peels a layer at a time, a flood
   * fill spreads from seeds, a cycle walks round and returns to itself.
   * Painting all of them as one still image is why they looked alike.
   */
  const steps = useMemo<string[]>(() => {
    switch (lens) {
      case "path": {
        // Two-node lens: the route itself, hop by hop. Before a target is
        // picked there is no route to draw, so it falls back to the
        // wavefront — the same BFS, seen as "everything at distance d".
        if (hood?.shortest_path?.length) return hood.shortest_path!.map((p) => p.page_id);
        return byDistance(hood?.undirected_distance);
      }
      // Ring by ring outward: the wavefront, in the order BFS found it.
      case "nearest":
        return (hood?.nearest ?? []).map((n) => n.page_id);
      case "reach":
      case "blast":
        // Directed, so it spreads DOWNSTREAM only — visibly narrower than
        // the undirected wave on the same source, which is the point.
        return byDistance(hood?.forward_reachable);
      case "components": {
        // Flood fill: each component filled in turn, so you see the
        // partition happen rather than arrive.
        if (!analysis) return [];
        const order = [...Object.entries(analysis.component_of)]
          .sort((a, b) => a[1] - b[1] || a[0].localeCompare(b[0]));
        return order.map(([id]) => id);
      }
      case "scc": {
        // Only the real loops. Singletons are the normal case and revealing
        // eighteen of them one at a time would spend the animation saying
        // "nothing here".
        if (!analysis) return [];
        return Object.entries(analysis.strongly_connected)
          .filter(([, c]) => (sccSize.get(c) ?? 1) > 1)
          .sort((a, b) => a[1] - b[1] || a[0].localeCompare(b[0]))
          .map(([id]) => id);
      }
      case "cycles":
        // Walks the loop and closes it — the one sequence that ends where
        // it started, which is what a cycle IS.
        return analysis?.cycle ?? [];
      case "topo":
        // Layer by layer, which is exactly Kahn's peel: everything with no
        // remaining prerequisite, then everything that unblocked.
        return (analysis?.layers ?? []).flat();
      case "topology":
        // No traversal to stage — Betti numbers are a property of the whole
        // graph, not a walk over it. Revealed at once, on purpose.
        return [];
      default:
        return [];
    }
  }, [lens, hood, analysis, sccSize]);

  // Slower for the short sequences, faster for the long ones, so a
  // four-hop path and a forty-node flood take a comparable time to watch.
  const stepMs = steps.length > 24 ? 70 : steps.length > 10 ? 130 : 260;
  const reveal = useStagedReveal(steps.length, stepMs, `${lens}:${source}:${target}`);

  /** How far through the sequence a given node is, or -1 if it is not in it. */
  const stepIndex = useMemo(() => {
    const m = new Map<string, number>();
    steps.forEach((id, i) => { if (!m.has(id)) m.set(id, i); });
    return m;
  }, [steps]);

  /** Has this node been revealed yet? Nodes outside the sequence are always
   *  visible — they are the graph the answer sits in. */
  const revealed = (id: string) => {
    const i = stepIndex.get(id);
    return i === undefined ? null : i < reveal.shown;
  };

  /** Which hue a node takes under the current lens.
   *
   *  A node the sequence has not reached yet keeps the unvisited grey —
   *  that is the whole animation: the colour IS the algorithm's progress,
   *  not a decoration playing beside it. */
  function hueOf(id: string): { fill: string; opacity: number } {
    if (revealed(id) === false) return { fill: "#3A3833", opacity: 0.3 };
    if (lens === "components" && analysis) {
      const c = analysis.component_of[id];
      const orphan = analysis.orphan_components.includes(c);
      return { fill: orphan ? "#585550" : COMPONENT_HUES[c % COMPONENT_HUES.length], opacity: 1 };
    }
    if (lens === "cycles") {
      return cycleSet.has(id)
        ? { fill: "#E0A34E", opacity: 1 }
        : { fill: "#585550", opacity: 0.5 };
    }
    if ((lens === "reach" || lens === "blast") && hood) {
      return reachable.has(id) || id === source
        ? { fill: "#E8873C", opacity: 1 }
        : { fill: "#585550", opacity: 0.4 };
    }
    if (lens === "path" && hood) {
      const d = hood.undirected_distance[id];
      if (d === undefined) return { fill: "#585550", opacity: 0.35 };
      return { fill: "#3FCFA8", opacity: Math.max(0.4, 1 - d / 6) };
    }
    if (lens === "scc" && analysis) {
      const c = analysis.strongly_connected[id];
      if (c === undefined) return { fill: "#585550", opacity: 0.4 };
      // A singleton SCC is the normal case and is deliberately NOT given a
      // hue of its own: colouring all eighteen of them would spend the whole
      // palette saying "nothing here". Only a real citation loop is coloured.
      const looped = (sccSize.get(c) ?? 1) > 1;
      return looped
        ? { fill: COMPONENT_HUES[c % COMPONENT_HUES.length], opacity: 1 }
        : { fill: "#585550", opacity: 0.45 };
    }
    if (lens === "topo" && analysis) {
      const level = layerOf.get(id);
      if (level === undefined) {
        // Unplaced: inside a cycle, or downstream of one. Amber, because
        // this is the one state on this lens that is a finding.
        return { fill: "#E0A34E", opacity: 1 };
      }
      const depth = Math.max(analysis.layers.length - 1, 1);
      // Earlier levels are brighter: level 0 is what you can read first.
      return { fill: "#3FCFA8", opacity: Math.max(0.35, 1 - level / (depth + 1)) };
    }
    if (lens === "nearest" && hood) {
      if (id === source) return { fill: "#E8873C", opacity: 1 };
      const rank = nearestRank.get(id);
      if (rank === undefined) return { fill: "#585550", opacity: 0.3 };
      return { fill: "#3FCFA8", opacity: Math.max(0.35, 1 - rank / 12) };
    }
    return { fill: "#7D9EC9", opacity: 0.9 };
  }

  const b = analysis?.betti;

  return (
    <Screen>
      <TopBar
        crumb={<>graph / <b>algorithms</b></>}
        readouts={
          <>
            <Readout k="VISITED" v={`${num(visited)} / ${num(total)}`} />
            <Readout
              k="FRONTIER"
              v={frontier.length > 0 ? num(Math.max(...frontier.map(([, n]) => n))) : "—"}
              tone="#E8873C"
            />
          </>
        }
      />

      <SubBar>
        {LENSES.map((l) => (
          <SubItem key={l.id} on={lens === l.id} onClick={() => setLens(l.id)}>{l.label}</SubItem>
        ))}
      </SubBar>

      <Body>
        <div style={{ flex: 1, minWidth: 0, position: "relative", overflow: "hidden" }}>
          <svg viewBox={`0 0 ${W} ${H}`} style={{ width: "100%", height: "100%", display: "block" }}>
            <g stroke="rgba(255,255,255,.08)">
              {edges.map((e, i) => {
                const a = at(e.from), z = at(e.to);
                if (!a || !z) return null;
                return <line key={i} x1={a.x} y1={a.y} x2={z.x} y2={z.y} />;
              })}
            </g>

            {/* The cycle, when the three-colour DFS found one — drawn over
                the rest so the loop reads as a loop. */}
            {lens === "cycles" && (analysis?.cycle.length ?? 0) > 1 && (
              <g stroke="#E0A34E" strokeWidth="2">
                {(analysis?.cycle ?? []).map((id, i, arr) => {
                  const a = at(id), z = at(arr[(i + 1) % arr.length]);
                  if (!a || !z) return null;
                  return <line key={i} x1={a.x} y1={a.y} x2={z.x} y2={z.y} />;
                })}
              </g>
            )}

            {/* BFS wavefront: one dashed ring per level, radius from the
                level itself. The animation is the mockup's own. */}
            {lens === "path" && source && at(source) && frontier.length > 1 && (
              <g stroke="rgba(63,207,168,.5)" strokeWidth="1" strokeDasharray="4 4" fill="none">
                {frontier.slice(1, 4).map(([level], i) => (
                  <circle
                    key={level}
                    cx={at(source)!.x}
                    cy={at(source)!.y}
                    r={46 + i * 52}
                    style={{ animation: "wave 3s ease-out infinite", animationDelay: `${i}s` }}
                  />
                ))}
              </g>
            )}

            {/* The route, drawn one hop at a time. This is the lens's
                whole answer: not "these nodes are near" but "this is how
                you get from here to there", which only reads as an answer
                if you can see it form. */}
            {lens === "path" && (hood?.shortest_path?.length ?? 0) > 1 && (
              <g fill="none" strokeLinecap="round">
                {hood!.shortest_path!.slice(0, -1).map((step, i) => {
                  const a = at(step.page_id);
                  const z = at(hood!.shortest_path![i + 1].page_id);
                  if (!a || !z) return null;
                  // Hop i is drawn once the reveal has passed node i+1 —
                  // the edge arrives with the node it lands on.
                  const on = reveal.shown > i + 1;
                  return (
                    <line
                      key={step.page_id}
                      x1={a.x} y1={a.y} x2={z.x} y2={z.y}
                      stroke="#E8873C"
                      strokeWidth={on ? 2.5 : 0}
                      opacity={on ? 1 : 0}
                      style={{ transition: "stroke-width .18s ease, opacity .18s ease" }}
                    />
                  );
                })}
                {/* A pulse travelling the finished route, so a path that
                    has already formed still reads as directional. */}
                {!reveal.running && (
                  <circle r="4" fill="#E8873C">
                    <animateMotion
                      dur={`${Math.max(1.2, hood!.shortest_path!.length * 0.45)}s`}
                      repeatCount="indefinite"
                      path={hood!.shortest_path!
                        .map((st, i) => {
                          const q = at(st.page_id);
                          return q ? `${i === 0 ? "M" : "L"}${q.x},${q.y}` : "";
                        })
                        .join(" ")}
                    />
                  </circle>
                )}
              </g>
            )}

            {/* Kahn's peel: a bar per layer, lighting in order. The
                sequence IS the sort — everything with no remaining
                prerequisite, then everything that unblocked. */}
            {lens === "topo" && (analysis?.layers.length ?? 0) > 0 && (
              <g>
                {(analysis?.layers ?? []).map((level, i) => {
                  const done = level.every((id) => revealed(id) !== false);
                  const xs = level.map((id) => at(id)).filter(Boolean) as Array<{x: number; y: number}>;
                  if (xs.length === 0) return null;
                  const minX = Math.min(...xs.map((q) => q.x)) - 18;
                  const maxX = Math.max(...xs.map((q) => q.x)) + 18;
                  const minY = Math.min(...xs.map((q) => q.y)) - 14;
                  const maxY = Math.max(...xs.map((q) => q.y)) + 14;
                  return (
                    <rect
                      key={i}
                      x={minX} y={minY} width={maxX - minX} height={maxY - minY}
                      fill="none"
                      stroke={done ? "rgba(63,207,168,.35)" : "rgba(255,255,255,.05)"}
                      strokeWidth="1"
                      strokeDasharray="3 4"
                      style={{ transition: "stroke .3s ease" }}
                    />
                  );
                })}
              </g>
            )}

            <g fontFamily="Archivo" fontSize="11.5" fill="#8C8880">
              {nodes.map((raw) => {
                const p = at(raw.id)!;
                const { fill, opacity } = hueOf(raw.id);
                const isSource = raw.id === source;
                return (
                  <g
                    key={raw.id}
                    style={{ cursor: "pointer" }}
                    onClick={() => {
                      // On the two-node lens the first click is where you
                      // are and the second is where you want to get to; a
                      // third starts over, so the gesture never dead-ends.
                      if (lens !== "path") { setSource(raw.id); return; }
                      // Fill the empty slot: start, then destination, then
                      // start over. Clicking either endpoint again clears
                      // back to it, so the gesture never dead-ends.
                      if (!source) { setSource(raw.id); return; }
                      if (raw.id === source) { setSource(null); setTarget(null); return; }
                      if (!target) { setTarget(raw.id); return; }
                      setSource(raw.id);
                      setTarget(null);
                    }}
                  >
                    {isSource && <circle cx={p.x} cy={p.y} r="15" fill="#E8873C" opacity=".2" />}
                    {raw.id === target && (
                      <circle cx={p.x} cy={p.y} r="15" fill="#E8873C" opacity=".2">
                        <animate attributeName="r" values="12;18;12" dur="1.6s" repeatCount="indefinite" />
                      </circle>
                    )}
                    <circle cx={p.x} cy={p.y} r={isSource || raw.id === target ? 7 : 5} fill={fill} opacity={opacity} />
                    <text x={p.x + 12} y={p.y + 4} fill={opacity > 0.6 ? "#8C8880" : "#4B4842"}>
                      {titleOf.get(raw.id)}
                    </text>
                  </g>
                );
              })}
            </g>
          </svg>

          <div style={{
            position: "absolute", right: 22, top: 18, padding: "12px 14px",
            background: "#111214", border: "1px solid rgba(255,255,255,.1)",
            display: "flex", flexDirection: "column", gap: 7, maxWidth: 260,
          }}>
            <Label>
              {lens === "path"
                ? (target ? "ROUTE" : source ? "PICK A DESTINATION" : "PICK A START")
                : source ? "FROM HERE" : "PICK A SOURCE"}
            </Label>
            {lens === "path" ? (
              <div style={{ display: "flex", flexDirection: "column", gap: 4 }}>
                {([["start", source, () => { setSource(null); setTarget(null); }],
                   ["end", target, () => setTarget(null)]] as const).map(([slot, id, clear]) => (
                  <div key={slot} style={{ display: "flex", alignItems: "baseline", gap: 7 }}>
                    <span className="mono" style={{ fontSize: 9, color: "#585550", width: 30 }}>
                      {slot}
                    </span>
                    <span className="mono" style={{
                      flex: 1, fontSize: 11, lineHeight: 1.5,
                      color: id ? "#E4E2DC" : "#4B4842",
                    }}>
                      {id ? titleOf.get(id) : "click a node"}
                    </span>
                    {id && (
                      <span
                        className="mono"
                        style={{ fontSize: 11, color: "#585550", cursor: "pointer" }}
                        onClick={clear}
                        title={`clear ${slot}`}
                      >
                        ×
                      </span>
                    )}
                  </div>
                ))}
              </div>
            ) : (
              <div className="mono" style={{ fontSize: 11, color: "#E4E2DC", lineHeight: 1.6 }}>
                {source ? titleOf.get(source) : "click any node"}
              </div>
            )}
            <span className="mono" style={{ fontSize: 10, color: "#585550" }}>
              {LENS_GESTURE[lens](
                { source, target, hood, reveal: reveal.shown, total: steps.length, visited },
              )}
            </span>
            {reveal.running && steps.length > 0 && (
              <div style={{ height: 2, background: "rgba(255,255,255,.08)" }}>
                <div style={{
                  width: `${(reveal.shown / steps.length) * 100}%`, height: "100%",
                  background: "#E8873C", transition: "width .1s linear",
                }} />
              </div>
            )}
            {!reveal.running && steps.length > 0 && (
              <span
                className="chip"
                style={{ cursor: "pointer", alignSelf: "flex-start" }}
                onClick={reveal.replay}
              >
                REPLAY
              </span>
            )}
          </div>
        </div>

        <Inspector
          tabs={[{ id: "results", label: "RESULTS" }, { id: "cost", label: "COST" }]}
          active={insTab}
          onSelect={(id) => setInsTab(id as "results" | "cost")}
        >
        {insTab === "cost" ? (
          <div style={{ display: "flex", flexDirection: "column", gap: 12 }}>
            <Label>WHAT THIS GRAPH COSTS</Label>
            <div className="mono" style={{ fontSize: 10.5, lineHeight: 1.95, color: "#8C8880" }}>
              nodes&nbsp;&nbsp;&nbsp;&nbsp;{num(graph?.nodes.length ?? 0)}<br />
              edges&nbsp;&nbsp;&nbsp;&nbsp;{num(graph?.edges.length ?? 0)}<br />
              density&nbsp;&nbsp;{density.toFixed(3)}
            </div>
            <div style={{ fontSize: 11, lineHeight: 1.6, color: "#585550" }}>
              Density is edges over the complete graph's edge count. Low is normal for a
              notebook — people cite a handful of pages, not all of them — and it is why
              every traversal below is linear rather than quadratic in practice.
            </div>

            <div style={{ height: 1, background: "rgba(255,255,255,.07)" }} />
            <Label>PER LENS</Label>
            {[
              ["BFS · shortest path", "O(V+E)", "one traversal answers distance AND route"],
              ["Nearest", "O(V+E)", "the same BFS, ranked"],
              ["Components", "O(V+E)", "flood fill, each node visited once"],
              ["SCC · Tarjan", "O(V+E)", "one DFS, no repeated descent"],
              ["Cycles · 3-colour", "O(V+E)", "DFS with a colour per node"],
              ["Topo · Kahn", "O(V+E)", "each edge decrements one in-degree"],
              ["Betti / triangles", "O(V·d²)", "the expensive one — d is degree"],
            ].map(([name, big, why]) => (
              <div key={name} style={{ display: "flex", flexDirection: "column", gap: 2 }}>
                <div style={{ display: "flex", alignItems: "baseline", gap: 8, fontSize: 11.5 }}>
                  <span style={{ flex: 1, color: "#9B968D" }}>{name}</span>
                  <span className="mono" style={{
                    fontSize: 10.5,
                    color: big.includes("d²") ? "#E0A34E" : "#E4E2DC",
                  }}>{big}</span>
                </div>
                <span style={{ fontSize: 10.5, color: "#585550" }}>{why}</span>
              </div>
            ))}

            <div style={{ height: 1, background: "rgba(255,255,255,.07)" }} />
            <Label>WHERE IT RUNS</Label>
            <div style={{ fontSize: 11.5, lineHeight: 1.65, color: "#8C8880" }}>
              All of it in Go, server-side, in{" "}
              <span className="mono" style={{ color: "#9B968D" }}>marginal/graphalgo</span> — one
              call per screen, not one per lens. Switching a lens above costs a repaint,
              not a request: every answer arrived together.
            </div>
            <div style={{ fontSize: 11, lineHeight: 1.6, color: "#585550" }}>
              The exception is the layout, which runs in the browser as wasm — a force
              simulation has to tick against what you are dragging, and a round trip per
              frame is not a thing.
            </div>
          </div>
        ) : (
          <>
          <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
            <Label>COMPONENTS · FLOOD FILL</Label>
            <div style={{ display: "flex", gap: 4, height: 8 }}>
              {components.map(([c, n]) => (
                <div
                  key={c}
                  style={{
                    flex: n,
                    background: analysis?.orphan_components.includes(c)
                      ? "#585550"
                      : COMPONENT_HUES[c % COMPONENT_HUES.length],
                  }}
                />
              ))}
            </div>
            <span style={{ fontSize: 11.5, color: "#8C8880" }}>
              {components.length} component{components.length === 1 ? "" : "s"}
              {components.length > 0 && total > 0 &&
                ` · largest holds ${Math.round((components[0][1] / total) * 100)}%`}
              {(analysis?.orphan_components.length ?? 0) > 0 &&
                ` · ${analysis!.orphan_components.length} orphaned`}
            </span>
          </div>

          <Rule />
          <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
            <Label>CYCLES · 3-COLOUR DFS</Label>
            <div style={{ display: "flex", alignItems: "center", gap: 9, flexWrap: "wrap" }}>
              {(analysis?.cycle.length ?? 0) > 0 ? (
                <>
                  <span className="chip chip-a" style={{ padding: "2px 7px" }}>1 FOUND</span>
                  <span className="mono" style={{ fontSize: 10.5, color: "#8C8880" }}>
                    {analysis!.cycle.map((id) => titleOf.get(id) ?? "?").join(" → ")}
                  </span>
                </>
              ) : (
                <>
                  <span className="chip chip-t" style={{ padding: "2px 7px" }}>NONE</span>
                  <span className="mono" style={{ fontSize: 10.5, color: "#585550" }}>
                    acyclic — a visited set alone could not tell you that
                  </span>
                </>
              )}
            </div>
          </div>

          <Rule />
          {/* Strong connectivity. The interesting row here is usually the
              absence of one: all-singletons means nothing cites in a loop. */}
          <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
            <Label>SCC · TARJAN</Label>
            {(() => {
              const sizes = analysis?.scc_sizes ?? [];
              const loops = sizes.filter((n) => n > 1);
              return (
                <>
                  <div style={{ display: "flex", gap: 4, height: 8 }}>
                    {sizes.slice(0, 12).map((n, i) => (
                      <div key={i} style={{
                        flex: n,
                        background: n > 1 ? COMPONENT_HUES[i % COMPONENT_HUES.length] : "rgba(255,255,255,.09)",
                      }} />
                    ))}
                  </div>
                  <span style={{ fontSize: 11.5, color: "#8C8880" }}>
                    {loops.length === 0
                      ? `${num(sizes.length)} singletons · no page cites another that cites it back`
                      : `${num(loops.length)} citation loop${loops.length === 1 ? "" : "s"} · largest holds ${num(loops[0])} pages`}
                  </span>
                  <span style={{ fontSize: 11, lineHeight: 1.55, color: "#585550" }}>
                    Different from components above: that one ignores which way a link
                    points, this one does not. A component can be one piece and still
                    contain no loop at all.
                  </span>
                </>
              );
            })()}
          </div>

          <Rule />
          {/* Kahn's order, and the shape of it. The layer histogram is the
              part worth reading: it is how deep the workspace goes. */}
          <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
            <Label>TOPOLOGICAL SORT · KAHN</Label>
            <div style={{ display: "flex", alignItems: "flex-end", gap: 5, height: 46 }}>
              {(analysis?.layers ?? []).slice(0, 10).map((level, i, arr) => {
                const max = Math.max(...arr.map((l) => l.length), 1);
                return (
                  <div key={i} style={{
                    flex: 1, display: "flex", flexDirection: "column", alignItems: "center", gap: 4,
                  }}>
                    <div style={{
                      width: "100%",
                      height: `${Math.max(8, (level.length / max) * 100)}%`,
                      background: `rgba(63,207,168,${0.4 + (level.length / max) * 0.6})`,
                    }} />
                    <span className="mono" style={{ fontSize: 8.5, color: "#585550" }}>{i}</span>
                  </div>
                );
              })}
            </div>
            <div className="mono" style={{ fontSize: 10.5, lineHeight: 1.9, color: "#8C8880" }}>
              orderable&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;{num(analysis?.topological_order.length ?? 0)} / {num(total)}<br />
              depth&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;{num(analysis?.layers.length ?? 0)} levels<br />
              <span style={{ color: analysis?.is_dag === false ? "#E0A34E" : "#3FCFA8" }}>
                {analysis?.is_dag === false
                  ? `${num(analysis.unplaced.length)} unplaced — a cycle blocks them`
                  : "acyclic · a full reading order exists"}
              </span>
            </div>
            <span style={{ fontSize: 11, lineHeight: 1.55, color: "#585550" }}>
              Everything in one level can be read in any order, or by two people at
              once. The number of levels is the longest dependency chain there is.
            </span>
          </div>

          <Rule />
          {/* The ranked ring. Near BY LINKS — a different notion from near by
              meaning, and the gap between them is what /discover exists for. */}
          <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
            <Label>NEAREST · BY LINK DISTANCE</Label>
            {!source && (
              <span style={{ fontSize: 11.5, color: "#585550" }}>Pick a source on the canvas.</span>
            )}
            {source && (hood?.nearest.length ?? 0) === 0 && (
              <span style={{ fontSize: 11.5, color: "#585550" }}>
                Nothing links to or from this page — it is an island, not merely far.
              </span>
            )}
            <div style={{ display: "flex", flexDirection: "column", gap: 5 }}>
              {(hood?.nearest ?? []).slice(0, 6).map((n) => (
                <div key={n.page_id} style={{ display: "flex", alignItems: "baseline", gap: 8 }}>
                  <span style={{ flex: 1, fontSize: 12, color: "#D2CFC8", overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
                    {n.title}
                  </span>
                  <span className="mono" style={{ fontSize: 10, color: "#585550" }}>{n.hops} hop{n.hops === 1 ? "" : "s"}</span>
                </div>
              ))}
            </div>
            {(hood?.ring_sizes.length ?? 0) > 1 && (
              <span className="mono" style={{ fontSize: 10, color: "#8C8880" }}>
                {hood!.ring_sizes.join(" → ")} · {num(visited)} reached
              </span>
            )}
          </div>

          <Rule />
          <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
            <Label>BLAST RADIUS</Label>
            <div className="mono" style={{ fontSize: 10.5, lineHeight: 1.9, color: "#8C8880" }}>
              forward reachable&nbsp;&nbsp;{hood ? num(Object.keys(hood.forward_reachable).length) : "—"}<br />
              undirected reach&nbsp;&nbsp;&nbsp;{hood ? num(visited) : "—"}<br />
              diameter&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;{analysis ? num(analysis.diameter) : "—"}
            </div>
            {!source && (
              <span style={{ fontSize: 11, color: "#585550", lineHeight: 1.55 }}>
                Reach is asked from somewhere. Pick a node to make these two real.
              </span>
            )}
          </div>

          <Rule />
          <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
            <Label>TOPOLOGY</Label>
            <div className="mono" style={{ fontSize: 10.5, lineHeight: 1.9, color: "#8C8880" }}>
              β₀ components&nbsp;&nbsp;{b ? num(b.b0) : "—"}<br />
              β₁ independent loops&nbsp;&nbsp;{b ? num(b.b1) : "—"}<br />
              β₁ clique complex&nbsp;&nbsp;{b ? num(b.b1_clique) : "—"}<br />
              triangles filled&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;{b ? num(b.triangles) : "—"}
            </div>
            <div style={{ fontSize: 11, lineHeight: 1.55, color: "#585550" }}>
              {b && b.b1 > b.b1_clique
                ? `Filling every triangle kills ${b.b1 - b.b1_clique} of ${b.b1} loops. What survives is a real hole, not three pages citing each other.`
                : "Filling the triangles changes nothing here — every loop is longer than three pages."}
            </div>
          </div>

          <Rule />
          {/* BFS frontier widths: the wavefront on the canvas, as data. The
              shape is the argument — a frontier that stops growing is a
              graph that stops connecting. */}
          <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
            <Label>FRONTIER BY LEVEL</Label>
            {frontier.length === 0 && (
              <span style={{ fontSize: 11, color: "#585550" }}>Pick a source to run the BFS.</span>
            )}
            {frontier.length > 0 && (
              <div style={{ display: "flex", alignItems: "flex-end", gap: 5, height: 46 }}>
                {frontier.map(([level, n]) => {
                  const max = Math.max(...frontier.map(([, v]) => v));
                  return (
                    <div key={level} style={{
                      flex: 1, display: "flex", flexDirection: "column",
                      alignItems: "center", gap: 4,
                    }}>
                      <div style={{
                        width: "100%",
                        height: `${Math.max((n / max) * 100, 8)}%`,
                        background: level === 0 ? "#3FCFA8" : "rgba(63,207,168,.45)",
                      }} />
                      <span className="mono" style={{ fontSize: 8.5, color: "#585550" }}>{level}</span>
                    </div>
                  );
                })}
              </div>
            )}
          </div>
          </>
        )}
        </Inspector>
      </Body>

      <StatusBar
        route="/graph/algorithms"
        mechanism="graphalgo in Go · never a second implementation here"
        state={err ? "analysis unavailable" : `${num(total)} nodes · ${components.length} components`}
        healthy={!err}
      />
    </Screen>
  );
}

export default GraphAlgorithmsScreen;
