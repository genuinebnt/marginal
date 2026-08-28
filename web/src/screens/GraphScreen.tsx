/**
 * docs/ui-mockups/v2/index.html § 07 GRAPH, ported.
 *
 * Ported, not redrawn. The markup below was extracted from the mockup and
 * converted to JSX mechanically; the class names, the panel geometry, the
 * readout order, and the copy are the mockup's own. Real data replaces the
 * mockup's constants where an endpoint exists, and `ph(...)` marks every
 * value that is still the mockup's — see shell/placeholder.tsx.
 *
 * The layout is real: every tick calls graphalgo.LayoutTick through wasm
 * (graph-core/useForceLayout), never a second physics implementation in TS.
 *
 * The screen's own argument, kept from the mockup because it is the reason
 * topic is a column rather than a derived label: colour is the page's
 * TOPIC, not the simulation's cluster index. The hull a node lands inside
 * is emergent (where links pulled it); the hue is declared (what the page
 * says it is about). When they disagree, that disagreement is the finding.
 */
import { useEffect, useMemo, useRef, useState } from "react";
import { getLinkGraph, type LinkGraph } from "../api/graph";
import { getTopics, type Topic } from "../api/topics";
import { listPages } from "../api/pages";
import { useAuth } from "../auth/AuthContext";
import { useForceLayout } from "../graph-core/useForceLayout";
import {
  Body, Inspector, Label, Readout, Rule, Screen, StatusBar, SubBar, SubItem,
  TopBar, TopicChip, TOPIC_HEX, num,
} from "../shell/Chrome";
import { ph, PlaceholderNote } from "../shell/placeholder";

const W = 1104;
const H = 754;

export function GraphScreen() {
  const { session } = useAuth();
  const actorId = session?.actorId ?? null;
  const [graph, setGraph] = useState<LinkGraph | null>(null);
  const [topics, setTopics] = useState<Topic[]>([]);
  // Per-node topic is joined client-side from /pages rather than served on
  // /graph: GraphService owns edges, not page metadata, and widening its
  // response to carry classification would make the graph endpoint a second
  // source of truth for something /pages already answers.
  const [topicOf, setTopicOf] = useState<Map<string, string>>(new Map());
  const [selected, setSelected] = useState<string | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const svgRef = useRef<SVGSVGElement | null>(null);

  useEffect(() => {
    if (!actorId) return;
    getLinkGraph(actorId).then(setGraph).catch((e) => setErr(String(e)));
    getTopics(actorId).then((r) => setTopics(r.topics)).catch(() => {});
    listPages(actorId)
      .then((r) => setTopicOf(new Map(r.pages.map((p) => [p.id, p.topic?.color_key ?? ""]))))
      .catch(() => {});
  }, [actorId]);

  const nodeIds = useMemo(() => graph?.nodes.map((n) => n.id) ?? [], [graph]);
  const edges = useMemo(
    () => graph?.edges.map((e) => ({ from: e.from_page, to: e.to_page })) ?? [],
    [graph],
  );
  const { nodes, startDrag, dragTo, endDrag } = useForceLayout(nodeIds, edges, W, H);

  const byId = useMemo(() => {
    const m = new Map<string, { title: string }>();
    graph?.nodes.forEach((n) => m.set(n.id, { title: n.title }));
    return m;
  }, [graph]);

  // Degree from the real edge list — the inspector's headline number, so it
  // is computed rather than carried over from the mockup.
  const degree = useMemo(() => {
    const d = new Map<string, { in: number; out: number }>();
    nodeIds.forEach((id) => d.set(id, { in: 0, out: 0 }));
    edges.forEach((e) => {
      const f = d.get(e.from); if (f) f.out++;
      const t = d.get(e.to); if (t) t.in++;
    });
    return d;
  }, [nodeIds, edges]);

  const sel = selected ?? nodes[0]?.id ?? null;
  const selTitle = sel ? byId.get(sel)?.title ?? "—" : "—";
  const selDeg = sel ? degree.get(sel) : undefined;
  const selKey = sel ? topicOf.get(sel) ?? "" : "";

  const neighbours = useMemo(() => {
    if (!sel) return [];
    const out = new Map<string, "in" | "out" | "both">();
    edges.forEach((e) => {
      if (e.from === sel) out.set(e.to, out.get(e.to) === "in" ? "both" : "out");
      if (e.to === sel) out.set(e.from, out.get(e.from) === "out" ? "both" : "in");
    });
    return [...out].map(([id, dir]) => ({ id, dir, title: byId.get(id)?.title ?? id }));
  }, [sel, edges, byId]);

  // The simulation settles into whatever area its forces imply, which at 25
  // nodes is a fraction of the canvas. Fit-to-viewport is a VIEW transform,
  // applied on render and never written back: rescaling the simulation's own
  // coordinates would change the distances its next tick reads, and the
  // layout would fight itself. The mockup's graph fills its frame, so this is
  // what makes the port match without touching the physics.
  const fit = useMemo(() => {
    if (nodes.length < 2) return { s: 1, dx: 0, dy: 0 };
    const xs = nodes.map((n) => n.x), ys = nodes.map((n) => n.y);
    const minX = Math.min(...xs), maxX = Math.max(...xs);
    const minY = Math.min(...ys), maxY = Math.max(...ys);
    // Padding leaves room for labels, which extend right of every node.
    const padX = 150, padY = 60;
    const s = Math.min((W - padX * 2) / Math.max(maxX - minX, 1),
                       (H - padY * 2) / Math.max(maxY - minY, 1));
    return { s, dx: padX - minX * s, dy: padY - minY * s };
  }, [nodes]);

  const pos = (id: string) => {
    const n = nodes.find((v) => v.id === id);
    return n ? { x: n.x * fit.s + fit.dx, y: n.y * fit.s + fit.dy } : undefined;
  };

  function svgPoint(ev: React.MouseEvent): { x: number; y: number } {
    const r = svgRef.current!.getBoundingClientRect();
    const vx = ((ev.clientX - r.left) / r.width) * W;
    const vy = ((ev.clientY - r.top) / r.height) * H;
    // Back out the fit transform: the simulation only understands its own
    // coordinates, so a drag reported in view space would jump.
    return { x: (vx - fit.dx) / fit.s, y: (vy - fit.dy) / fit.s };
  }

  return (
    <Screen>
      <TopBar
        readouts={
          <>
            <Readout k="NODES" v={num(graph?.nodes.length ?? 0)} />
            <Readout k="EDGES" v={num(graph?.edges.length ?? 0)} />
            <Readout k="ALPHA" v={ph("0.001")} tone="#3FCFA8" />
          </>
        }
      />

      <SubBar>
        <SubItem on>FORCE</SubItem>
        <SubItem>TERRITORY · VORONOI</SubItem>
        <SubItem>DELAUNAY DUAL</SubItem>
        <div style={{ flex: 1 }} />
        <SubItem tone="#3FCFA8">SIMULATION COOLED</SubItem>
      </SubBar>

      <Body>
        <div style={{ flex: 1, minWidth: 0, position: "relative", overflow: "hidden" }}>
          <svg
            ref={svgRef}
            viewBox={`0 0 ${W} ${H}`}
            style={{ width: "100%", height: "100%", display: "block" }}
            onMouseMove={(e) => dragTo(svgPoint(e).x, svgPoint(e).y)}
            onMouseUp={endDrag}
            onMouseLeave={endDrag}
          >
            {/* Edges under nodes, at the mockup's own alpha. */}
            <g stroke="rgba(255,255,255,.09)" strokeWidth="1">
              {edges.map((e, i) => {
                const a = pos(e.from), b = pos(e.to);
                if (!a || !b) return null;
                return <line key={i} x1={a.x} y1={a.y} x2={b.x} y2={b.y} />;
              })}
            </g>

            {/* The selected node's own edges, ember — "neighbourhood" in the
                legend below. Drawn as a second pass so they sit above the
                rest rather than depending on edge order. */}
            <g stroke="#E8873C" strokeWidth="1.6" opacity=".75">
              {edges.map((e, i) => {
                if (e.from !== sel && e.to !== sel) return null;
                const a = pos(e.from), b = pos(e.to);
                if (!a || !b) return null;
                return <line key={`s${i}`} x1={a.x} y1={a.y} x2={b.x} y2={b.y} />;
              })}
            </g>

            <g fontFamily="Archivo" fontSize="11.5" fill="#8C8880">
              {nodes.map((raw) => {
                const p = pos(raw.id)!;
                const n = { id: raw.id, x: p.x, y: p.y };
                const isSel = n.id === sel;
                const d = degree.get(n.id);
                const deg = (d?.in ?? 0) + (d?.out ?? 0);
                // Radius tracks degree the way the mockup's does — a hub is
                // visibly a hub without a legend explaining it.
                const r = isSel ? 8 : Math.max(4, Math.min(6, 4 + deg / 4));
                // Untopiced nodes draw in --ink-7 rather than a topic hue.
                // Not a fallback colour: "no topic" is a real state, and
                // giving it one of the five would make it indistinguishable
                // from a page that genuinely is that topic.
                const key = topicOf.get(n.id) ?? "";
                const hex = TOPIC_HEX[key] ?? "#6E6A63";
                return (
                  <g key={n.id} onMouseDown={() => { setSelected(n.id); startDrag(n.id); }}
                     style={{ cursor: "pointer" }}>
                    {isSel && (
                      <>
                        <circle cx={n.x} cy={n.y} r="17" fill="none" stroke={hex} strokeWidth="1.5" opacity=".55" />
                        <circle cx={n.x} cy={n.y} r="15" fill="#E8873C" opacity=".2" />
                      </>
                    )}
                    <circle cx={n.x} cy={n.y} r={r} fill={isSel ? "#E8873C" : hex}
                            opacity={isSel ? 1 : deg > 2 ? 1 : 0.62} />
                    <text x={n.x + r + 8} y={n.y + (isSel ? -4 : 4)}
                          fontFamily={isSel ? "Spectral" : "Archivo"}
                          fontSize={isSel ? 15 : 11.5}
                          fill={isSel ? "#EFEDE7" : deg > 2 ? "#8C8880" : "#6E6A63"}>
                      {byId.get(n.id)?.title ?? ""}
                    </text>
                    {isSel && (
                      <text x={n.x + r + 8} y={n.y + 13} fontFamily="IBM Plex Mono" fontSize="9.5" fill="#8C8880">
                        degree {deg} · betweenness {ph("0.31")}
                      </text>
                    )}
                  </g>
                );
              })}
            </g>
          </svg>

          {/* Colour is the page's TOPIC, not its space and not the force
              simulation's own cluster index: the hull a node lands inside is
              emergent (where links pulled it), the hue is declared (what the
              page says it is about). When the two disagree, that disagreement
              is the finding. */}
          <div style={{
            position: "absolute", left: 20, top: 16, display: "flex", flexDirection: "column",
            gap: 9, padding: "12px 14px", background: "#111214",
            border: "1px solid rgba(255,255,255,.1)", width: 212,
          }}>
            <div style={{ display: "flex", alignItems: "baseline", gap: 8 }}>
              <Label>COLOUR BY</Label>
              <span className="mono" style={{ fontSize: 9, color: "#4B4842", marginLeft: "auto" }}>
                topic ≠ cluster
              </span>
            </div>
            <div style={{ display: "flex", gap: 0 }}>
              <span className="tb tb-on" style={{ padding: "4px 9px", fontSize: 10 }}>TOPIC</span>
              <span className="tb" style={{ padding: "4px 9px", fontSize: 10 }}>CLUSTER</span>
              <span className="tb" style={{ padding: "4px 9px", fontSize: 10 }}>SPACE</span>
            </div>
            <Rule />
            <Label>TOPICS · {topics.length}</Label>
            <div style={{ display: "flex", flexDirection: "column", gap: 5 }}>
              {topics.map((t, i) => (
                <div key={t.id} style={{ display: "flex", alignItems: "center", gap: 8, ...(i > 2 ? { opacity: 0.5 } : {}) }}>
                  <span style={{ width: 7, height: 7, background: TOPIC_HEX[t.color_key] }} />
                  <span style={{ flex: 1, fontSize: 11.5, color: i > 2 ? "#9B968D" : "#E4E2DC" }}>{t.name}</span>
                  <span className="mono" style={{ fontSize: 9.5, color: i > 2 ? "#585550" : "#8C8880" }}>
                    {t.page_count ?? 0}
                  </span>
                </div>
              ))}
            </div>
            <Rule />
            <Label>TAG FILTER</Label>
            <div className="tgrow">
              <span className="tg tg-on">{ph("crdt")}</span>
              <span className="tg">{ph("anchors")}</span>
              <span className="tg">{ph("rope")}</span>
            </div>
            <div style={{ display: "flex", alignItems: "center", gap: 9, marginTop: 4 }}>
              <span className="rd-k">MIN DEGREE</span>
              <div style={{ width: 90, height: 2, background: "rgba(255,255,255,.12)" }}>
                <div style={{ width: "34%", height: "100%", background: "#E8873C" }} />
              </div>
              <span className="mono" style={{ fontSize: 10, color: "#8C8880" }}>{ph(2)}</span>
            </div>
          </div>

          <div style={{
            position: "absolute", left: 20, bottom: 18, padding: "10px 14px",
            background: "#111214", border: "1px solid rgba(255,255,255,.1)",
            display: "flex", gap: 18, fontSize: 11, color: "#8C8880",
          }}>
            <span><span style={{ color: "#E8873C" }}>━</span> neighbourhood</span>
            <span>drag reheats · settles again</span>
            <span className="mono" style={{ color: "#585550" }}>60fps only while moving</span>
          </div>
        </div>

        <Inspector
          tabs={[{ id: "sel", label: "SELECTED" }, { id: "topics", label: "TOPICS" }, { id: "clusters", label: "CLUSTERS" }]}
          active="sel"
        >
          <div style={{ fontFamily: "Spectral,serif", fontSize: 16, color: "#EFEDE7" }}>{selTitle}</div>
          <div className="tgrow">
            {selKey ? (
              <TopicChip name={topics.find((t) => t.color_key === selKey)?.name ?? selKey} colorKey={selKey} />
            ) : (
              <span className="chip">UNTOPICED</span>
            )}
            <span className="tg">{ph("crdt")}</span>
            <span className="tg">{ph("lamport")}</span>
          </div>
          <div className="mono" style={{ fontSize: 10.5, lineHeight: 1.95, color: "#8C8880" }}>
            degree&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;{(selDeg?.in ?? 0) + (selDeg?.out ?? 0)}<br />
            in / out&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;{selDeg?.in ?? 0} / {selDeg?.out ?? 0}<br />
            topic&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;{selKey || "none"}<br />
            territory&nbsp;&nbsp;{ph("9.4% of plane")}
          </div>
          <Rule />
          <Label>NEIGHBOURS</Label>
          <div style={{ display: "flex", flexDirection: "column", gap: 7, fontSize: 12, color: "#9B968D" }}>
            {neighbours.length === 0 && (
              <span style={{ color: "#585550", fontSize: 11.5 }}>
                No links either way — an orphan, which the graph shows rather than hides.
              </span>
            )}
            {neighbours.map((n) => (
              <div key={n.id} style={{ display: "flex", cursor: "pointer" }} onClick={() => setSelected(n.id)}>
                <span style={{ flex: 1, color: "#D2CFC8" }}>{n.title}</span>
                <span className="mono" style={{ fontSize: 10, color: "#585550" }}>{n.dir}</span>
              </div>
            ))}
          </div>
          <Rule />
          {/* The declared/emergent disagreement, counted. This is the whole
              reason topic is a column and not a derived label: if it were
              derived from the clustering it could never contradict it, and a
              contradiction is the only thing here worth acting on. */}
          <Label>TOPIC MIX OF NEIGHBOURS</Label>
          <PlaceholderNote>topic mix needs per-node topics on the graph endpoint</PlaceholderNote>
          <div style={{ display: "flex", height: 7, gap: 1 }}>
            <div style={{ flex: 5, background: "#7AA8E8" }} title="Protocol" />
            <div style={{ flex: 3, background: "#C48AE0" }} title="Storage" />
            <div style={{ flex: 1, background: "#D07C8A" }} title="Research" />
          </div>
          <div style={{ display: "flex", gap: 12, font: "400 10px 'IBM Plex Mono',monospace", color: "#585550" }}>
            <span><span style={{ color: "#7AA8E8" }}>■</span> {ph(5)} protocol</span>
            <span><span style={{ color: "#C48AE0" }}>■</span> {ph(3)} storage</span>
            <span><span style={{ color: "#D07C8A" }}>■</span> {ph(1)} research</span>
          </div>
          <Rule />
          <Label>SIMULATION</Label>
          <div className="mono" style={{ fontSize: 10.5, lineHeight: 1.9, color: "#8C8880" }}>
            alpha&nbsp;&nbsp;&nbsp;{ph("0.001")} <span style={{ color: "#3FCFA8" }}>cooled</span><br />
            ticks&nbsp;&nbsp;&nbsp;{ph(412)}<br />
            seed&nbsp;&nbsp;&nbsp;&nbsp;0x5EED<br />
            modularity&nbsp;{ph("0.61")} <span style={{ color: "#585550" }}>by topic</span><br />
            &nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;{ph("0.68")} <span style={{ color: "#585550" }}>by cluster</span>
          </div>
        </Inspector>
      </Body>

      <StatusBar
        route="/graph"
        mechanism="force sim in wasm"
        state={err ? "graph unavailable" : `${nodes.length} nodes settled`}
        healthy={!err}
      />
    </Screen>
  );
}

export default GraphScreen;
