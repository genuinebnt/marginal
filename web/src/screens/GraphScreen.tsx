import { useEffect, useMemo, useRef, useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import { useAuth } from "../auth/AuthContext";
import { getLinkGraph, type LinkGraph } from "../api/graph";
import { useForceLayout } from "../graph-core/useForceLayout";
import { territory } from "../graph-core/wasm";
import type { TerritoryResult } from "../graph-core/types";

/**
 * docs/ui-mockups/graph.html, made real: the actual [[link]] graph
 * (docs/api/graph.md's GetLinkGraph), laid out by a real seeded
 * force-directed simulation (graphalgo.LayoutTick, compiled to wasm —
 * ../graph-core/useForceLayout), with a "Territory" mode switching to
 * the exact Voronoi diagram over the same live positions
 * (graphalgo.Voronoi/Delaunay, same wasm module). No layout or geometry
 * algorithm is reimplemented here — this file only draws what Go already
 * computed and forwards mouse events to it (ADR-012).
 */
export function GraphScreen() {
  const { session, logout } = useAuth();
  const navigate = useNavigate();
  if (!session) throw new Error("GraphScreen requires an authenticated session");
  const { actorId } = session;

  const [graph, setGraph] = useState<LinkGraph | null>(null);
  const [view, setView] = useState<"nodes" | "territory">("nodes");
  const [territoryData, setTerritoryData] = useState<TerritoryResult | null>(null);
  const [hovered, setHovered] = useState<string | null>(null);

  const stageRef = useRef<HTMLDivElement>(null);
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const [size, setSize] = useState({ width: 800, height: 600 });

  useEffect(() => {
    getLinkGraph(actorId).then(setGraph).catch(() => setGraph({ nodes: [], edges: [] }));
  }, [actorId]);

  useEffect(() => {
    const stage = stageRef.current;
    if (!stage) return;
    const observer = new ResizeObserver(() => {
      setSize({ width: stage.clientWidth, height: stage.clientHeight });
    });
    observer.observe(stage);
    return () => observer.disconnect();
  }, []);

  const nodeIds = useMemo(() => graph?.nodes.map((n) => n.id) ?? [], [graph]);
  const layoutEdges = useMemo(
    () => graph?.edges.map((e) => ({ from: e.from_page, to: e.to_page })) ?? [],
    [graph],
  );
  const { nodes: positions, startDrag, dragTo, endDrag } = useForceLayout(
    nodeIds,
    layoutEdges,
    size.width,
    size.height,
  );

  const titleById = useMemo(() => {
    const m = new Map<string, string>();
    for (const n of graph?.nodes ?? []) m.set(n.id, n.title);
    return m;
  }, [graph]);
  const isRootById = useMemo(() => {
    const m = new Map<string, boolean>();
    for (const n of graph?.nodes ?? []) m.set(n.id, n.is_root);
    return m;
  }, [graph]);
  const degreeById = useMemo(() => {
    const m = new Map<string, number>();
    for (const e of graph?.edges ?? []) {
      m.set(e.from_page, (m.get(e.from_page) ?? 0) + 1);
      m.set(e.to_page, (m.get(e.to_page) ?? 0) + 1);
    }
    return m;
  }, [graph]);

  // Territory mode recomputes the exact Voronoi diagram (real Go, via
  // wasm) over whatever the current live positions are — so dragging a
  // node in Territory view visibly reshapes the cells around it.
  useEffect(() => {
    if (view !== "territory" || positions.length === 0) return;
    let cancelled = false;
    void territory(
      positions.map((p) => ({ id: p.id, x: p.x, y: p.y })),
      { min_x: 0, min_y: 0, max_x: size.width, max_y: size.height },
    ).then((result) => {
      if (!cancelled) setTerritoryData(result);
    });
    return () => {
      cancelled = true;
    };
  }, [view, positions, size.width, size.height]);

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
    const teal = css.getPropertyValue("--teal").trim() || "#1F8A75";
    const violet = css.getPropertyValue("--violet").trim() || "#7A5AC2";

    if (view === "territory" && territoryData) {
      for (const cell of territoryData.cells) {
        if (cell.poly.length < 3) continue;
        ctx.beginPath();
        ctx.moveTo(cell.poly[0].x, cell.poly[0].y);
        for (const p of cell.poly.slice(1)) ctx.lineTo(p.x, p.y);
        ctx.closePath();
        ctx.strokeStyle = rule;
        ctx.lineWidth = 1;
        ctx.stroke();
      }
      ctx.strokeStyle = violet;
      ctx.globalAlpha = 0.5;
      for (const pair of territoryData.delaunay) {
        const a = positions.find((p) => p.id === pair.a);
        const b = positions.find((p) => p.id === pair.b);
        if (!a || !b) continue;
        ctx.beginPath();
        ctx.moveTo(a.x, a.y);
        ctx.lineTo(b.x, b.y);
        ctx.stroke();
      }
      ctx.globalAlpha = 1;
    } else {
      ctx.strokeStyle = rule;
      ctx.lineWidth = 1;
      for (const e of graph?.edges ?? []) {
        const a = positions.find((p) => p.id === e.from_page);
        const b = positions.find((p) => p.id === e.to_page);
        if (!a || !b) continue;
        ctx.beginPath();
        ctx.moveTo(a.x, a.y);
        ctx.lineTo(b.x, b.y);
        ctx.stroke();
      }
    }

    for (const p of positions) {
      const r = 4.5 + Math.min(degreeById.get(p.id) ?? 0, 8) * 1.15;
      ctx.beginPath();
      ctx.arc(p.x, p.y, r, 0, Math.PI * 2);
      ctx.fillStyle = isRootById.get(p.id) ? teal : violet;
      ctx.globalAlpha = p.id === hovered ? 1 : 0.85;
      ctx.fill();
      if (isRootById.get(p.id)) {
        ctx.lineWidth = 2;
        ctx.strokeStyle = ink;
        ctx.stroke();
      }
      ctx.globalAlpha = 1;

      if (p.id === hovered) {
        ctx.fillStyle = ink;
        ctx.font = "12px sans-serif";
        ctx.fillText(titleById.get(p.id) ?? "", p.x + r + 6, p.y + 4);
      }
    }
  }, [positions, view, territoryData, graph, hovered, size, degreeById, isRootById, titleById]);

  function nodeAt(x: number, y: number): string | null {
    for (let i = positions.length - 1; i >= 0; i--) {
      const p = positions[i];
      const r = 4.5 + Math.min(degreeById.get(p.id) ?? 0, 8) * 1.15 + 5;
      if ((x - p.x) ** 2 + (y - p.y) ** 2 <= r * r) return p.id;
    }
    return null;
  }

  const draggingRef = useRef<string | null>(null);

  function toLocal(e: React.MouseEvent): { x: number; y: number } {
    const rect = canvasRef.current!.getBoundingClientRect();
    return { x: e.clientX - rect.left, y: e.clientY - rect.top };
  }

  return (
    <div className="app">
      <header className="topbar">
        <Link to="/pages" className="brand" style={{ textDecoration: "none" }}>
          <span className="mark"></span>Marginal
        </Link>
        <nav className="nav">
          <Link to="/graph" aria-current="page">Graph</Link>
          <Link to="/graph/algorithms">Algorithms</Link>
        </nav>
        <div className="spacer"></div>
        <button
          className="btn"
          onClick={() => setView(view === "nodes" ? "territory" : "nodes")}
          title="Toggle between the force layout and the exact Voronoi territory view"
        >
          {view === "nodes" ? "Territory" : "Nodes"}
        </button>
        <button className="btn" onClick={logout}>Sign out</button>
      </header>

      <div className="body-row">
        <main className="canvas" style={{ flex: 1, position: "relative" }} ref={stageRef}>
          <canvas
            ref={canvasRef}
            style={{ display: "block", cursor: hovered ? "pointer" : "default" }}
            onMouseDown={(e) => {
              const { x, y } = toLocal(e);
              const id = nodeAt(x, y);
              if (id) {
                draggingRef.current = id;
                startDrag(id);
              }
            }}
            onMouseMove={(e) => {
              const { x, y } = toLocal(e);
              if (draggingRef.current) {
                dragTo(draggingRef.current, x, y);
              } else {
                setHovered(nodeAt(x, y));
              }
            }}
            onMouseUp={() => {
              const wasDragging = draggingRef.current;
              draggingRef.current = null;
              if (wasDragging) endDrag();
            }}
            onClick={(e) => {
              if (draggingRef.current) return;
              const { x, y } = toLocal(e);
              const id = nodeAt(x, y);
              if (id) navigate(`/pages/${id}`);
            }}
          />
          <div
            className="note"
            style={{ position: "absolute", top: 12, left: 12, margin: 0, maxWidth: "20rem" }}
          >
            <b>{graph?.nodes.length ?? 0}</b> pages · <b>{graph?.edges.length ?? 0}</b> links.{" "}
            {view === "territory"
              ? "Cell area measures the layout, never orphan status — that's the Algorithms view."
              : "Teal rings are root pages. Drag a node; click to open its page."}
          </div>
        </main>
      </div>
    </div>
  );
}
