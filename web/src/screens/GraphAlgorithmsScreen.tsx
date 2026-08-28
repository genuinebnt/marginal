import { useEffect, useMemo, useRef, useState } from "react";
import { Link } from "react-router-dom";
import { useAuth } from "../auth/AuthContext";
import { getLinkGraph, analyzeGraph, graphNeighborhood, type LinkGraph, type GraphAnalysis, type GraphNeighborhood } from "../api/graph";
import { useForceLayout } from "../graph-core/useForceLayout";

const COMPONENT_COLORS = ["#1F8A75", "#7A5AC2", "#B8791E", "#4F6D9A", "#C2547A", "#5A9BC2"];

/**
 * docs/ui-mockups/v2/index.html § 08 GRAPH ALGORITHMS, made real: every metric here is
 * internal/graphalgo's own output (docs/api/graph.md's AnalyzeGraph/
 * GraphNeighborhood), never recomputed in TypeScript. Click a page to
 * pick it as BFS's source — link-distance rings and the forward-only
 * blast radius are both real graphalgo.BFS/ForwardReachable results,
 * just colored here.
 */
export function GraphAlgorithmsScreen() {
  const { session, logout } = useAuth();
  if (!session) throw new Error("GraphAlgorithmsScreen requires an authenticated session");
  const { actorId } = session;

  const [graph, setGraph] = useState<LinkGraph | null>(null);
  const [analysis, setAnalysis] = useState<GraphAnalysis | null>(null);
  const [source, setSource] = useState<string | null>(null);
  const [neighborhood, setNeighborhood] = useState<GraphNeighborhood | null>(null);
  const [lens, setLens] = useState<"components" | "cycle" | "wavefront" | "blast">("components");

  const stageRef = useRef<HTMLDivElement>(null);
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const [size, setSize] = useState({ width: 800, height: 600 });

  useEffect(() => {
    getLinkGraph(actorId).then(setGraph).catch(() => setGraph({ nodes: [], edges: [] }));
    analyzeGraph(actorId).then(setAnalysis).catch(() => setAnalysis(null));
  }, [actorId]);

  useEffect(() => {
    if (!source) {
      setNeighborhood(null);
      return;
    }
    graphNeighborhood(actorId, source).then(setNeighborhood).catch(() => setNeighborhood(null));
  }, [actorId, source]);

  useEffect(() => {
    const stage = stageRef.current;
    if (!stage) return;
    const observer = new ResizeObserver(() => setSize({ width: stage.clientWidth, height: stage.clientHeight }));
    observer.observe(stage);
    return () => observer.disconnect();
  }, []);

  const nodeIds = useMemo(() => graph?.nodes.map((n) => n.id) ?? [], [graph]);
  const layoutEdges = useMemo(() => graph?.edges.map((e) => ({ from: e.from_page, to: e.to_page })) ?? [], [graph]);
  const { nodes: positions, startDrag, dragTo, endDrag } = useForceLayout(nodeIds, layoutEdges, size.width, size.height);

  const titleById = useMemo(() => {
    const m = new Map<string, string>();
    for (const n of graph?.nodes ?? []) m.set(n.id, n.title);
    return m;
  }, [graph]);

  const cycleEdgeSet = useMemo(() => {
    const set = new Set<string>();
    const cyc = analysis?.cycle ?? [];
    for (let i = 0; i + 1 < cyc.length; i++) set.add(`${cyc[i]}:${cyc[i + 1]}`);
    return set;
  }, [analysis]);

  const orphanNodeIds = useMemo(() => {
    if (!analysis) return new Set<string>();
    const orphanComponents = new Set(analysis.orphan_components);
    const set = new Set<string>();
    for (const [id, comp] of Object.entries(analysis.component_of)) {
      if (orphanComponents.has(comp)) set.add(id);
    }
    return set;
  }, [analysis]);

  useEffect(() => {
    const canvas = canvasRef.current;
    if (!canvas) return;
    const dpr = window.devicePixelRatio || 1;
    canvas.width = size.width * dpr;
    canvas.height = size.height * dpr;
    canvas.style.width = `${size.width}px`;
    canvas.style.height = `${size.height}px`;
    const ctx = canvas.getContext("2d");
    if (!ctx) return;
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    ctx.clearRect(0, 0, size.width, size.height);

    const css = getComputedStyle(document.documentElement);
    const ink = css.getPropertyValue("--ink").trim() || "#1a1a1a";
    const rule = css.getPropertyValue("--rule").trim() || "#ddd";
    const amber = css.getPropertyValue("--amber").trim() || "#B8791E";

    // edges
    for (const e of graph?.edges ?? []) {
      const a = positions.find((p) => p.id === e.from_page);
      const b = positions.find((p) => p.id === e.to_page);
      if (!a || !b) continue;
      const onCycle = lens === "cycle" && cycleEdgeSet.has(`${e.from_page}:${e.to_page}`);
      ctx.strokeStyle = onCycle ? amber : rule;
      ctx.lineWidth = onCycle ? 2.5 : 1;
      ctx.beginPath();
      ctx.moveTo(a.x, a.y);
      ctx.lineTo(b.x, b.y);
      ctx.stroke();
    }

    for (const p of positions) {
      let fill = "#999";
      if (lens === "components" && analysis) {
        const comp = analysis.component_of[p.id] ?? 0;
        fill = orphanNodeIds.has(p.id) ? "#999" : COMPONENT_COLORS[comp % COMPONENT_COLORS.length];
      } else if (lens === "wavefront" && neighborhood) {
        const d = neighborhood.undirected_distance[p.id];
        fill = d === undefined ? "#ccc" : `hsl(${170 - Math.min(d, 8) * 18}, 55%, 45%)`;
      } else if (lens === "blast" && neighborhood) {
        fill = neighborhood.forward_reachable[p.id] !== undefined ? "#B8791E" : "#ccc";
      } else if (lens === "cycle") {
        fill = (analysis?.cycle ?? []).includes(p.id) ? amber : "#999";
      }

      ctx.beginPath();
      ctx.arc(p.x, p.y, p.id === source ? 8 : 5.5, 0, Math.PI * 2);
      ctx.fillStyle = fill;
      ctx.fill();
      if (p.id === source) {
        ctx.lineWidth = 2;
        ctx.strokeStyle = ink;
        ctx.stroke();
      }
    }
  }, [positions, graph, analysis, neighborhood, lens, source, cycleEdgeSet, orphanNodeIds, size]);

  function nodeAt(x: number, y: number): string | null {
    for (let i = positions.length - 1; i >= 0; i--) {
      const p = positions[i];
      if ((x - p.x) ** 2 + (y - p.y) ** 2 <= 100) return p.id;
    }
    return null;
  }
  const draggingRef = useRef<string | null>(null);
  function toLocal(e: React.MouseEvent) {
    const rect = canvasRef.current!.getBoundingClientRect();
    return { x: e.clientX - rect.left, y: e.clientY - rect.top };
  }

  const mostLinked = useMemo(() => {
    const degree = new Map<string, number>();
    for (const e of graph?.edges ?? []) {
      degree.set(e.from_page, (degree.get(e.from_page) ?? 0) + 1);
      degree.set(e.to_page, (degree.get(e.to_page) ?? 0) + 1);
    }
    return [...degree.entries()].sort((a, b) => b[1] - a[1]).slice(0, 5);
  }, [graph]);

  const frontierLevels = useMemo(() => {
    if (!neighborhood) return [];
    const byDist = new Map<number, number>();
    for (const d of Object.values(neighborhood.undirected_distance)) byDist.set(d, (byDist.get(d) ?? 0) + 1);
    return [...byDist.entries()].sort((a, b) => a[0] - b[0]);
  }, [neighborhood]);
  const maxFrontier = Math.max(1, ...frontierLevels.map(([, n]) => n));

  const readout = (() => {
    if (lens === "components" && analysis) {
      return { algo: "Components · flood fill", text: `${analysis.betti.b0} component${analysis.betti.b0 === 1 ? "" : "s"}, ${orphanNodeIds.size} orphaned page${orphanNodeIds.size === 1 ? "" : "s"}.` };
    }
    if (lens === "cycle" && analysis) {
      return {
        algo: "DetectCycle · three-colour DFS",
        text: analysis.cycle.length > 0 ? `A real cycle exists — ${analysis.cycle.length} pages, highlighted amber.` : "This graph is acyclic — no cycle exists.",
      };
    }
    if (lens === "wavefront") {
      if (!source) return { algo: "BFS · frontier by level", text: "Click a page to pick it as BFS's source." };
      return {
        algo: "BFS · frontier by level",
        text: `From ${titleById.get(source) || "this page"}: ${frontierLevels.length} level${frontierLevels.length === 1 ? "" : "s"}, ${neighborhood ? Object.keys(neighborhood.undirected_distance).length : 0} of ${graph?.nodes.length ?? 0} pages reached.`,
        cost: frontierLevels.length > 0 ? `frontier widths ${frontierLevels.map(([, n]) => n).join(" → ")}` : undefined,
      };
    }
    if (lens === "blast") {
      if (!source) return { algo: "ForwardReachable · directed BFS", text: "Click a page to pick it as the blast-radius source." };
      const reached = neighborhood ? Object.keys(neighborhood.forward_reachable).length : 0;
      return { algo: "ForwardReachable · directed BFS", text: `${reached} page${reached === 1 ? "" : "s"} reachable from ${titleById.get(source) || "this page"} by following outbound links only.` };
    }
    return null;
  })();

  return (
    <div className="app">
      <header className="topbar">
        <Link to="/pages" className="brand" style={{ textDecoration: "none" }}>
          <span className="mark"></span>Marginal
        </Link>
        <nav className="nav">
          <Link to="/graph">Graph</Link>
          <Link to="/graph/algorithms" aria-current="page">Algorithms</Link>
          <Link to="/facts">Facts</Link>
        </nav>
        <div className="spacer"></div>
        <button className="btn" onClick={logout}>Sign out</button>
      </header>

      <div className="body-row">
        <main className="stage" ref={stageRef}>
          <canvas
            ref={canvasRef}
            style={{ display: "block" }}
            onMouseDown={(e) => {
              const { x, y } = toLocal(e);
              const id = nodeAt(x, y);
              if (id) {
                draggingRef.current = id;
                startDrag(id);
              }
            }}
            onMouseMove={(e) => {
              if (!draggingRef.current) return;
              const { x, y } = toLocal(e);
              dragTo(draggingRef.current, x, y);
            }}
            onMouseUp={() => {
              const was = draggingRef.current;
              draggingRef.current = null;
              if (was) endDrag();
            }}
            onClick={(e) => {
              if (draggingRef.current) return;
              const { x, y } = toLocal(e);
              const id = nodeAt(x, y);
              setSource(id === source ? null : id);
            }}
          />

          <div className="modes" role="group" aria-label="Overlay">
            {(["components", "cycle", "wavefront", "blast"] as const).map((l) => (
              <button key={l} className="mode" aria-pressed={lens === l} onClick={() => setLens(l)}>
                {l}
              </button>
            ))}
          </div>

          {lens === "wavefront" && frontierLevels.length > 0 && (
            <div className="levels">
              {frontierLevels.map(([d, n]) => (
                <div key={d} className="lv on">
                  <div className="bar" style={{ height: `${(n / maxFrontier) * 48}px` }}></div>
                  <span className="n">{d}</span>
                </div>
              ))}
            </div>
          )}

          {readout && (
            <div className="readout">
              <span className="algo">{readout.algo}</span>
              {readout.text}
              {readout.cost && <span className="cost">{readout.cost}</span>}
            </div>
          )}
        </main>

        <aside className="rail right">
          <div className="rail-head"><span className="label">Graph</span></div>
          <div className="panel-body">
            <div className="panel-h">Shape</div>
            <div className="metric"><span className="k">Pages</span><span className="v">{graph?.nodes.length ?? "—"}</span></div>
            <div className="metric"><span className="k">Links</span><span className="v">{graph?.edges.length ?? "—"}</span></div>
            <div className="metric"><span className="k">Components</span><span className="v">{analysis?.betti.b0 ?? "—"}</span></div>
            <div className="metric"><span className="k">Cycles</span><span className="v">{analysis?.cycle.length ? 1 : 0}</span></div>
            <div className="metric"><span className="k">Orphaned</span><span className="v">{orphanNodeIds.size}</span></div>
            <div className="metric"><span className="k">Diameter</span><span className="v">{analysis?.diameter ?? "—"}</span></div>

            {analysis && (
              <div className="panel-section">
                <div className="panel-h">Topology (β numbers)</div>
                <div className="metric"><span className="k">β₀ · components</span><span className="v">{analysis.betti.b0}</span></div>
                <div className="metric"><span className="k">β₁ · cycle rank</span><span className="v">{analysis.betti.b1}</span></div>
                <div className="metric"><span className="k">β₁ · clique complex</span><span className="v">{analysis.betti.b1_clique}</span></div>
                <div className="metric"><span className="k">β₂ · voids</span><span className="v">{analysis.betti.b2}</span></div>
                <div className="metric"><span className="k">Triangles filled</span><span className="v">{analysis.betti.triangles}</span></div>
                <div className="note" style={{ margin: "9px 0 0", maxWidth: "none" }}>
                  E − V + β₀ counts every independent loop, so a mutually-citing triangle scores as a
                  hole; filling triangles removes {analysis.betti.b1 - analysis.betti.b1_clique} of{" "}
                  {analysis.betti.b1} loops, leaving {analysis.betti.b1_clique} that survive.
                  {analysis.betti.b2 > 0 &&
                    ` β₂ = ${analysis.betti.b2}: a fully-filled shape with an interior nothing fills.`}
                </div>
              </div>
            )}

            <div className="panel-section">
              <div className="panel-h">Most linked to</div>
              {mostLinked.map(([id, n]) => (
                <div className="row" key={id}>
                  <span className="lead">{n}</span>
                  {titleById.get(id) || "Untitled"}
                </div>
              ))}
              {mostLinked.length === 0 && <div className="muted" style={{ fontSize: 12.5, padding: "4px 0" }}>No links yet.</div>}
            </div>

            <div className="panel-section">
              <div className="panel-h">Selected page</div>
              {source ? (
                <>
                  <div className="row"><span className="lead">●</span>{titleById.get(source) || "Untitled"}</div>
                  <div className="note" style={{ margin: "9px 0 0", maxWidth: "none" }}>
                    "Wavefront"/"Blast" above colour every other page by its real BFS distance from
                    here.
                  </div>
                </>
              ) : (
                <div className="muted" style={{ fontSize: 12.5, padding: "4px 0" }}>Click a page to pick it as BFS's source.</div>
              )}
            </div>
          </div>
        </aside>
      </div>
    </div>
  );
}
