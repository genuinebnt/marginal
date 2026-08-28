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
import {
  Body, Inspector, Label, Readout, Rule, Screen, StatusBar, SubBar, SubItem, TopBar, num,
} from "../shell/Chrome";

const W = 1104;
const H = 722;

type Lens = "path" | "components" | "cycles" | "reach" | "blast" | "topology";

const LENSES: Array<{ id: Lens; label: string }> = [
  { id: "path", label: "BFS · SHORTEST PATH" },
  { id: "components", label: "COMPONENTS" },
  { id: "cycles", label: "CYCLES · 3-COLOUR DFS" },
  { id: "reach", label: "REACHABILITY" },
  { id: "blast", label: "BLAST RADIUS" },
  { id: "topology", label: "TOPOLOGY" },
];

/** Component colours. Not the topic ramp — these index a partition, and
 *  reusing topic hues would imply a page's component says what it is about. */
const COMPONENT_HUES = ["#3FCFA8", "#7D9EC9", "#A98CE8", "#585550", "#D6A660", "#D07C8A"];

export function GraphAlgorithmsScreen() {
  const { session } = useAuth();
  const actorId = session?.actorId ?? null;

  const [graph, setGraph] = useState<LinkGraph | null>(null);
  const [analysis, setAnalysis] = useState<GraphAnalysis | null>(null);
  const [source, setSource] = useState<string | null>(null);
  const [hood, setHood] = useState<GraphNeighborhood | null>(null);
  const [lens, setLens] = useState<Lens>("components");
  const [err, setErr] = useState<string | null>(null);

  useEffect(() => {
    if (!actorId) return;
    getLinkGraph(actorId).then(setGraph).catch((e) => setErr(String(e)));
    analyzeGraph(actorId).then(setAnalysis).catch(() => setAnalysis(null));
  }, [actorId]);

  useEffect(() => {
    if (!actorId || !source) { setHood(null); return; }
    graphNeighborhood(actorId, source).then(setHood).catch(() => setHood(null));
  }, [actorId, source]);

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

  const cycleSet = useMemo(() => new Set(analysis?.cycle ?? []), [analysis]);
  const reachable = useMemo(() => new Set(Object.keys(hood?.forward_reachable ?? {})), [hood]);
  const visited = hood ? Object.keys(hood.undirected_distance).length : 0;
  const total = graph?.nodes.length ?? 0;

  /** Which hue a node takes under the current lens. */
  function hueOf(id: string): { fill: string; opacity: number } {
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

            <g fontFamily="Archivo" fontSize="11.5" fill="#8C8880">
              {nodes.map((raw) => {
                const p = at(raw.id)!;
                const { fill, opacity } = hueOf(raw.id);
                const isSource = raw.id === source;
                return (
                  <g key={raw.id} style={{ cursor: "pointer" }} onClick={() => setSource(raw.id)}>
                    {isSource && <circle cx={p.x} cy={p.y} r="15" fill="#E8873C" opacity=".2" />}
                    <circle cx={p.x} cy={p.y} r={isSource ? 7 : 5} fill={fill} opacity={opacity} />
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
            <Label>{source ? "FROM HERE" : "PICK A SOURCE"}</Label>
            <div className="mono" style={{ fontSize: 11, color: "#E4E2DC", lineHeight: 1.6 }}>
              {source ? titleOf.get(source) : "click any node"}
            </div>
            <span className="mono" style={{ fontSize: 10, color: "#585550" }}>
              {source
                ? `${visited} reached · BFS from one source`
                : "every metric below is over the whole graph"}
            </span>
          </div>
        </div>

        <Inspector
          tabs={[{ id: "results", label: "RESULTS" }, { id: "cost", label: "COST" }]}
          active="results"
        >
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
