/**
 * docs/ui-mockups/v2/index.html § 07 GRAPH, ported.
 *
 * Ported, not redrawn. The markup below was extracted from the mockup and
 * converted to JSX mechanically; the class names, the panel geometry, the
 * readout order, and the copy are the mockup's own.
 *
 * Every value on this screen is now computed. Betweenness and modularity come
 * from graphalgo over gRPC; the territory polygons are real convex hulls; the
 * layout, its alpha and its tick count are the simulation's own state; the
 * tag filter, topic mix and territory share are joins over /pages. No
 * placeholders remain.
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
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { analyzeGraph, getLinkGraph, type GraphAnalysis, type LinkGraph } from "../api/graph";
import { getTopics, type Topic } from "../api/topics";
import { listPages } from "../api/pages";
import { useAuth } from "../auth/AuthContext";
import { useForceLayout } from "../graph-core/useForceLayout";
import { hulls as computeHulls, type Hull } from "../graph-core/wasm";
import {
  Body, Inspector, Label, Readout, Rule, Screen, StatusBar, SubBar, SubItem,
  TopBar, TopicChip, TOPIC_HEX, num,
} from "../shell/Chrome";

const W = 1104;
const H = 754;

export function GraphScreen() {
  const { session } = useAuth();
  const actorId = session?.actorId ?? null;
  const [graph, setGraph] = useState<LinkGraph | null>(null);
  const [analysis, setAnalysis] = useState<GraphAnalysis | null>(null);
  const [topics, setTopics] = useState<Topic[]>([]);
  // Per-node topic is joined client-side from /pages rather than served on
  // /graph: GraphService owns edges, not page metadata, and widening its
  // response to carry classification would make the graph endpoint a second
  // source of truth for something /pages already answers.
  const [topicOf, setTopicOf] = useState<Map<string, string>>(new Map());
  // Tags per page, for the filter. Same join as topicOf and from the same
  // /pages response — the graph endpoint owns edges, not page metadata.
  const [tagsOf, setTagsOf] = useState<Map<string, string[]>>(new Map());
  const [tagFilter, setTagFilter] = useState<string | null>(null);
  const [minDegree, setMinDegree] = useState(0);
  const [selected, setSelected] = useState<string | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const svgRef = useRef<SVGSVGElement | null>(null);
  // Territory polygons — one convex hull per topic over the settled
  // positions, computed in Go (graphalgo.Territories) via wasm. Recomputed
  // when the layout changes, not per frame: a hull over a still-moving
  // simulation is a shape nobody can read.
  const [territories, setTerritories] = useState<Hull[]>([]);

  useEffect(() => {
    if (!actorId) return;
    getLinkGraph(actorId).then(setGraph).catch((e) => setErr(String(e)));
    analyzeGraph(actorId).then(setAnalysis).catch(() => setAnalysis(null));
    getTopics(actorId).then((r) => setTopics(r.topics)).catch(() => {});
    listPages(actorId)
      .then((r) => {
        setTopicOf(new Map(r.pages.map((p) => [p.id, p.topic?.color_key ?? ""])));
        setTagsOf(new Map(r.pages.map((p) => [p.id, p.tags ?? []])));
      })
      .catch(() => {});
  }, [actorId]);

  const nodeIds = useMemo(() => graph?.nodes.map((n) => n.id) ?? [], [graph]);
  const edges = useMemo(
    () => graph?.edges.map((e) => ({ from: e.from_page, to: e.to_page })) ?? [],
    [graph],
  );
  const { nodes, startDrag, dragTo, endDrag, alpha, ticks, cooled } = useForceLayout(nodeIds, edges, W, H);

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

  /** Tags ranked by how many of THIS graph's nodes carry them. */
  const topTags = useMemo(() => {
    const m = new Map<string, number>();
    nodeIds.forEach((id) => (tagsOf.get(id) ?? []).forEach((t) => m.set(t, (m.get(t) ?? 0) + 1)));
    return [...m].sort((a, b) => b[1] - a[1] || a[0].localeCompare(b[0])).slice(0, 6);
  }, [nodeIds, tagsOf]);

  /**
   * Filters DIM rather than remove. Pulling nodes out would re-run the
   * simulation and rearrange everything still on screen, so a filter would
   * destroy the spatial memory it exists to help you use.
   */
  const dimmed = useCallback((id: string) => {
    const d = degree.get(id);
    const deg = (d?.in ?? 0) + (d?.out ?? 0);
    if (deg < minDegree) return true;
    if (tagFilter && !(tagsOf.get(id) ?? []).includes(tagFilter)) return true;
    return false;
  }, [degree, minDegree, tagFilter, tagsOf]);

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
    const padX = 150, padY = 74;
    const s = Math.min((W - padX * 2) / Math.max(maxX - minX, 1),
                       (H - padY * 2) / Math.max(maxY - minY, 1));
    return { s, dx: padX - minX * s, dy: padY - minY * s };
  }, [nodes]);

  useEffect(() => {
    if (nodes.length === 0 || topicOf.size === 0) { setTerritories([]); return; }
    const pts = nodes.map((n) => ({
      group: topicOf.get(n.id) ?? "",
      x: n.x * fit.s + fit.dx,
      y: n.y * fit.s + fit.dy,
    }));
    let cancelled = false;
    computeHulls(pts).then((h) => { if (!cancelled) setTerritories(h); }).catch(() => setTerritories([]));
    return () => { cancelled = true; };
  }, [nodes, topicOf, fit]);

  /** The selected node's neighbours, grouped by topic — declared against
   *  emergent, which is the disagreement this panel exists to surface. */
  const neighbourMix = useMemo(() => {
    const m = new Map<string, number>();
    neighbours.forEach((nb) => {
      const key = topicOf.get(nb.id) ?? "";
      m.set(key, (m.get(key) ?? 0) + 1);
    });
    return [...m].sort((a, b) => b[1] - a[1]);
  }, [neighbours, topicOf]);

  const offTopic = useMemo(
    () => neighbours.filter((nb) => (topicOf.get(nb.id) ?? "") !== selKey).length,
    [neighbours, topicOf, selKey],
  );

  /**
   * How much of the canvas the selected node's TOPIC covers — the shoelace
   * area of its hull over the viewport, as a percentage.
   *
   * Of the topic, not of the node: a single point has no area, and "this
   * page occupies 0% of the plane" would be true and useless. The question
   * the panel is asking is how much ground the thing it belongs to holds.
   */
  const territoryShare = useMemo(() => {
    const hull = territories.find((h) => h.group === selKey);
    if (!hull || hull.points.length < 3) return null;
    let a = 0;
    for (let i = 0; i < hull.points.length; i++) {
      const p = hull.points[i], q = hull.points[(i + 1) % hull.points.length];
      a += p.x * q.y - q.x * p.y;
    }
    return (Math.abs(a) / 2 / (W * H)) * 100;
  }, [territories, selKey]);

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
            <Readout k="ALPHA" v={alpha.toFixed(3)} tone={cooled ? "#3FCFA8" : "#E0A34E"} />
          </>
        }
      />

      <SubBar>
        <SubItem on>FORCE</SubItem>
        <SubItem>TERRITORY · VORONOI</SubItem>
        <SubItem>DELAUNAY DUAL</SubItem>
        <div style={{ flex: 1 }} />
        <SubItem tone={cooled ? "#3FCFA8" : "#E0A34E"}>
          {cooled ? "SIMULATION COOLED" : "SIMULATION RUNNING"}
        </SubItem>
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
            {/* Territories behind everything. Colour is the topic's own, at
                the wash alpha the design system uses for coloured regions —
                a fill any stronger competes with the nodes it contains. */}
            <g>
              {territories.map((h) => {
                if (h.points.length < 3) return null;
                const hex = TOPIC_HEX[h.group] ?? "#6E6A63";
                const d = h.points.map((p, i) => `${i === 0 ? "M" : "L"}${p.x} ${p.y}`).join(" ") + " Z";
                return <path key={h.group} d={d} fill={`${hex}0A`} stroke={`${hex}2E`} strokeWidth="1" />;
              })}
            </g>

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
                            opacity={dimmed(n.id) ? 0.16 : isSel ? 1 : deg > 2 ? 1 : 0.62} />
                    <text x={n.x + r + 8} y={n.y + (isSel ? -4 : 4)}
                          fontFamily={isSel ? "Spectral" : "Archivo"}
                          fontSize={isSel ? 15 : 11.5}
                          fill={dimmed(n.id) ? "#3A3833" : isSel ? "#EFEDE7" : deg > 2 ? "#8C8880" : "#6E6A63"}>
                      {byId.get(n.id)?.title ?? ""}
                    </text>
                    {isSel && (
                      <text x={n.x + r + 8} y={n.y + 13} fontFamily="IBM Plex Mono" fontSize="9.5" fill="#8C8880">
                        degree {deg} · betweenness {(analysis?.betweenness[n.id] ?? 0).toFixed(3)}
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
              {topTags.length === 0 && (
                <span className="mono" style={{ fontSize: 9.5, color: "#585550" }}>no tags yet</span>
              )}
              {topTags.map(([t, n]) => (
                <span
                  key={t}
                  className={`tg${tagFilter === t ? " tg-on" : ""}`}
                  style={{ cursor: "pointer" }}
                  onClick={() => setTagFilter(tagFilter === t ? null : t)}
                >
                  {t}<span style={{ color: "#585550", marginLeft: 5, fontSize: 9 }}>{n}</span>
                </span>
              ))}
            </div>
            <div style={{ display: "flex", alignItems: "center", gap: 9, marginTop: 4 }}>
              <span className="rd-k">MIN DEGREE</span>
              <input
                type="range" min={0} max={6} value={minDegree}
                onChange={(e) => setMinDegree(Number(e.target.value))}
                style={{ width: 90, accentColor: "#E8873C" }}
              />
              <span className="mono" style={{ fontSize: 10, color: "#8C8880" }}>{minDegree}</span>
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
            {(tagsOf.get(sel ?? "") ?? []).map((t) => (
              <span key={t} className="tg">{t}</span>
            ))}
          </div>
          <div className="mono" style={{ fontSize: 10.5, lineHeight: 1.95, color: "#8C8880" }}>
            degree&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;{(selDeg?.in ?? 0) + (selDeg?.out ?? 0)}<br />
            in / out&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;{selDeg?.in ?? 0} / {selDeg?.out ?? 0}<br />
            topic&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;{selKey || "none"}<br />
            territory&nbsp;&nbsp;{territoryShare === null ? "—" : `${territoryShare.toFixed(1)}% of plane`}
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
          {neighbourMix.length === 0 ? (
            <span style={{ fontSize: 11.5, color: "#585550" }}>No neighbours to mix.</span>
          ) : (
            <>
              <div style={{ display: "flex", height: 7, gap: 1 }}>
                {neighbourMix.map(([key, n]) => (
                  <div key={key} style={{ flex: n, background: TOPIC_HEX[key] ?? "#6E6A63" }}
                       title={`${n} ${key || "untopiced"}`} />
                ))}
              </div>
              <div style={{ display: "flex", gap: 12, flexWrap: "wrap", font: "400 10px 'IBM Plex Mono',monospace", color: "#585550" }}>
                {neighbourMix.map(([key, n]) => (
                  <span key={key}>
                    <span style={{ color: TOPIC_HEX[key] ?? "#6E6A63" }}>■</span> {n} {key || "untopiced"}
                  </span>
                ))}
              </div>
              {offTopic > 0 && (
                <div style={{
                  display: "flex", gap: 9, padding: "9px 11px",
                  border: "1px solid rgba(224,163,78,.3)", background: "rgba(224,163,78,.06)",
                }}>
                  <span style={{ color: "#E0A34E", fontSize: 11 }}>◌</span>
                  <div style={{ flex: 1, fontSize: 11.5, lineHeight: 1.55, color: "#9B968D" }}>
                    {offTopic} of {neighbours.length} neighbours sit outside{" "}
                    <span style={{ color: TOPIC_HEX[selKey] ?? "#6E6A63" }}>{selKey || "no topic"}</span>.
                    Either this page spans two topics, or one of them is mis-topiced — the graph
                    can show the disagreement, not settle it.
                  </div>
                </div>
              )}
            </>
          )}
          <Rule />
          <Label>SIMULATION</Label>
          <div className="mono" style={{ fontSize: 10.5, lineHeight: 1.9, color: "#8C8880" }}>
            alpha&nbsp;&nbsp;&nbsp;{alpha.toFixed(3)}{" "}
            <span style={{ color: cooled ? "#3FCFA8" : "#E0A34E" }}>{cooled ? "cooled" : "running"}</span><br />
            ticks&nbsp;&nbsp;&nbsp;{num(ticks)}<br />
            seed&nbsp;&nbsp;&nbsp;&nbsp;0x5EED<br />
            modularity&nbsp;{(analysis?.modularity_by_topic ?? 0).toFixed(2)} <span style={{ color: "#585550" }}>by topic</span><br />
            &nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;{(analysis?.modularity_by_component ?? 0).toFixed(2)} <span style={{ color: "#585550" }}>by component</span>
          </div>
        </Inspector>
      </Body>

      <StatusBar
        route="/graph"
        mechanism="force sim in wasm"
        // Must agree with the sub-bar: reporting "settled" while the strip
        // says RUNNING is the screen contradicting itself about the one thing
        // it is watching.
        state={
          err ? "graph unavailable"
            : cooled ? `${nodes.length} nodes settled`
            : `${nodes.length} nodes · cooling, α ${alpha.toFixed(3)}`
        }
        healthy={!err && cooled}
      />
    </Screen>
  );
}

export default GraphScreen;
