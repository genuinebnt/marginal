/**
 * docs/ui-mockups/v2/index.html § 09 DISCOVER, ported.
 *
 * "What is near this page by MEANING" — a third notion of nearness beside
 * § 07's near-in-space and § 08's near-by-links, and the screen exists
 * because the three disagree. The row worth looking at is always the one
 * where they do: high cosine, no shared tags, unreachable by link is prose
 * similarity finding something the graph and the tags both missed, which is
 * the only reason to run an index at all.
 *
 * WHAT THE VECTORS ARE, said on the screen as well as here. Not neural
 * embeddings — there is no model in this repo. They are hashed, IDF-weighted
 * term frequencies (marginal/semantic), which capture LEXICAL similarity: two
 * pages using the same uncommon words score high, and "rope" and "cord" are
 * unrelated to them. The mockup's own caption said "384-d embeddings" and has
 * been corrected rather than quietly lived with, because the screen's whole
 * posture is that its numbers can be checked.
 *
 * The index is a real HNSW (marginal/semantic.Index) — layers, greedy
 * descent, heuristic neighbour pruning — and every query runs a brute-force
 * scan beside it to report recall. Speed without recall is half a result.
 */
import { useCallback, useEffect, useMemo, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { useAuth } from "../auth/AuthContext";
import { listPages, type Page } from "../api/pages";
import { discoverNear, type NearResponse } from "../api/discover";
import {
  Body, Inspector, Label, Readout, Rule, Screen, StatusBar, TopBar, TopicChip,
  num,
} from "../shell/Chrome";

const K = 5;

export function DiscoverScreen() {
  const { id } = useParams();
  const navigate = useNavigate();
  const { session } = useAuth();
  const actorId = session?.actorId ?? null;

  const [pages, setPages] = useState<Page[]>([]);
  const [near, setNear] = useState<NearResponse | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [insTab, setInsTab] = useState<"hnsw" | "index">("hnsw");
  const [scope, setScope] = useState<string[]>([]);
  const [mustTags, setMustTags] = useState<string[]>([]);
  const [view, setView] = useState<"grid" | "list">("grid");
  /** § 09 offers one sort beside the two views. Sort order is view state:
   *  it changes what you see, never what the index was built from. */
  const [recentFirst, setRecentFirst] = useState(false);

  useEffect(() => {
    if (!actorId) return;
    listPages(actorId).then((r) => setPages(r.pages)).catch((e) => setErr(String(e)));
  }, [actorId]);

  // Land on a page rather than on nothing: every panel here is relative to a
  // source, so an unselected screen is a screen that looks broken.
  const source = id ?? pages[0]?.id ?? null;
  const sourcePage = pages.find((p) => p.id === source) ?? null;

  useEffect(() => {
    if (!actorId || !source) { setNear(null); return; }
    discoverNear(actorId, source, { k: K, topics: scope, tags: mustTags })
      .then((r) => { setNear(r); setErr(null); })
      .catch((e) => { setNear(null); setErr(String(e)); });
  }, [actorId, source, scope, mustTags]);

  const toggleScope = useCallback((t: string) => {
    setScope((cur) => (cur.includes(t) ? cur.filter((x) => x !== t) : [...cur, t]));
  }, []);

  /**
   * The POSTS list, in the order the sort chip asks for. Sorted here rather
   * than re-fetched: the order pages are drawn in has nothing to do with the
   * index, and re-querying to reorder a list would imply it did.
   */
  const ordered = useMemo(() => {
    if (!recentFirst) return pages;
    return [...pages].sort((a, b) => b.updated_at.localeCompare(a.updated_at));
  }, [pages, recentFirst]);

  /** Tags on the source page — the only ones a MUST HAVE constraint can
   *  usefully name, since requiring a tag the source lacks asks for pages
   *  near it that share nothing with it. */
  const sourceTags = sourcePage?.tags ?? [];

  const stats = near?.stats;
  const maxCosine = useMemo(
    () => Math.max(...(near?.neighbours ?? []).map((n) => n.cosine), 0.0001),
    [near],
  );

  return (
    <Screen>
      <TopBar
        crumb={<>discover / <b>near {sourcePage?.title ?? "…"}</b></>}
        readouts={
          <>
            <Readout
              k="RECALL@5"
              v={stats ? stats.recall_at_k.toFixed(2) : "—"}
              tone={stats && stats.recall_at_k >= 1 ? "#3FCFA8" : "#E0A34E"}
            />
            <Readout k="HOPS" v={stats ? num(stats.hops) : "—"} />
            <Readout k="EXACT SCAN" v={stats ? num(stats.exact_comparisons) : "—"} tone="#6E6A63" />
          </>
        }
      />

      <Body>
        {/* POSTS — every page, as a card. Clicking one re-queries the index. */}
        <div style={{
          width: 392, flex: "none", boxSizing: "border-box", background: "#0F1012",
          borderRight: "1px solid rgba(255,255,255,.07)", display: "flex", flexDirection: "column",
        }}>
          <div className="rail-h">
            POSTS<div /><span style={{ color: "#585550" }}>{num(pages.length)}</span>
          </div>
          <div style={{ padding: "0 14px 12px", display: "flex", gap: 6 }}>
            {(["grid", "list"] as const).map((v) => (
              <span
                key={v}
                className={`chip${view === v ? " chip-e" : ""}`}
                style={{ cursor: "pointer" }}
                onClick={() => setView(v)}
              >
                {v.toUpperCase()}
              </span>
            ))}
            <span style={{ flex: 1 }} />
            <span
              className={`chip${recentFirst ? " chip-e" : ""}`}
              style={{ cursor: "pointer" }}
              onClick={() => setRecentFirst((r) => !r)}
            >
              RECENT
            </span>
          </div>
          <div style={{
            padding: "0 14px", flex: 1, minHeight: 0, overflowY: "auto",
            display: view === "grid" ? "grid" : "flex",
            gridTemplateColumns: view === "grid" ? "1fr 1fr" : undefined,
            flexDirection: view === "list" ? "column" : undefined,
            gap: 10,
          }}>
            {ordered.map((p, i) => {
              const selected = p.id === source;
              return (
                <div
                  key={p.id}
                  className={selected ? undefined : "fx"}
                  onClick={() => navigate(`/discover/${p.id}`)}
                  style={{
                    position: "relative", cursor: "pointer", padding: 12, boxSizing: "border-box",
                    height: view === "grid" ? 104 : undefined,
                    display: "flex", flexDirection: "column",
                    border: selected ? "1px solid rgba(232,135,60,.5)" : "1px solid rgba(255,255,255,.08)",
                    background: selected ? "rgba(232,135,60,.07)" : undefined,
                    animationDelay: `${Math.min(i, 8) * 0.04}s`,
                  }}
                >
                  {selected && <><div className="brk-tl" /><div className="brk-br" /></>}
                  <span className="mono" style={{
                    fontSize: 8.5, letterSpacing: ".16em",
                    color: selected ? "#E8873C" : p.topic ? "#4B4842" : "#E0A34E",
                  }}>
                    {selected ? "SELECTED" : (p.topic?.name ?? "UNTOPICED").toUpperCase()}
                  </span>
                  <div style={{
                    fontFamily: "Spectral, serif", fontSize: 14, lineHeight: 1.3,
                    color: selected ? "#EFEDE7" : "#D2CFC8", marginTop: 7,
                  }}>
                    {p.title}
                  </div>
                  {selected ? (
                    <div style={{ marginTop: "auto", display: "flex", alignItems: "center", gap: 6 }}>
                      {/* The pulse marks the page the index is being queried
                          FROM — the one thing on this rail that is changing
                          while you look at it. */}
                      <div className="dot" style={{ width: 5, height: 5, background: "#A98CE8" }}>
                        <div className="ping" style={{ background: "rgba(169,140,232,.5)" }} />
                      </div>
                      <span className="mono" style={{ fontSize: 9, color: "#585550" }}>querying</span>
                    </div>
                  ) : (
                    <div style={{ marginTop: "auto" }} className="mono">
                      <span style={{ fontSize: 9, color: "#585550" }}>
                        {p.word_count ? `${num(p.word_count)} words` : "empty"}
                      </span>
                    </div>
                  )}
                </div>
              );
            })}
          </div>
          <div className="wal">
            <Label>CLICK A POST</Label>
            <span style={{ fontSize: 11, color: "#585550", lineHeight: 1.5 }}>
              The middle column re-queries the index; the right column shows the descent
              that answered it.
            </span>
          </div>
        </div>

        <div style={{
          flex: 1, minWidth: 0, padding: "28px 34px", overflow: "hidden",
          display: "flex", flexDirection: "column",
        }}>
          <div style={{ display: "flex", alignItems: "baseline", gap: 12, marginBottom: 14 }}>
            <h1 className="h1" style={{ fontSize: 22 }}>Semantically near</h1>
            <span className="mono" style={{ fontSize: 11, color: "#585550" }}>
              cosine over {stats?.dimensions ?? 256}-d hashed TF-IDF · greedy descent from layer{" "}
              {Math.max((stats?.layers ?? 1) - 1, 0)}
            </span>
          </div>

          {/* SCOPE — a PRE-filter on the index, not a post-filter on the
              results. Post-filtering asks for k=5, throws three away and ships
              two, and recall collapses exactly when the filter is narrow. */}
          <div style={{
            display: "flex", alignItems: "center", gap: 9, marginBottom: 20,
            padding: "9px 12px", background: "#131415", border: "1px solid rgba(255,255,255,.07)",
            flexWrap: "wrap",
          }}>
            <Label>SCOPE</Label>
            {(near?.topics ?? []).map((t) => {
              const on = scope.includes(t);
              const key = pages.find((p) => p.topic?.name === t)?.topic?.color_key ?? "protocol";
              return on
                ? <span key={t} onClick={() => toggleScope(t)} style={{ cursor: "pointer" }}>
                    <TopicChip name={t} colorKey={key} />
                  </span>
                : <span
                    key={t}
                    className="tpc"
                    style={{ borderColor: "rgba(255,255,255,.12)", color: "#6E6A63", cursor: "pointer" }}
                    onClick={() => toggleScope(t)}
                  >
                    <i style={{ background: "#4B4842" }} />
                    {t.toUpperCase()}
                  </span>;
            })}
            <span
              className="chip"
              style={{ padding: "2px 8px", borderStyle: "dashed", color: "#585550", cursor: "pointer" }}
              onClick={() => setScope([])}
            >
              ALL {num(near?.topics.length ?? 0)}
            </span>
            <div style={{ width: 1, height: 15, background: "rgba(255,255,255,.09)" }} />
            <Label>MUST HAVE TAG</Label>
            {sourceTags.length === 0 && (
              <span className="mono" style={{ fontSize: 10, color: "#585550" }}>
                this page has no tags
              </span>
            )}
            {sourceTags.map((t) => (
              <span
                key={t}
                className={`tg${mustTags.includes(t) ? " tg-on" : ""}`}
                style={{ cursor: "pointer" }}
                onClick={() => setMustTags((cur) =>
                  cur.includes(t) ? cur.filter((x) => x !== t) : [...cur, t])}
              >
                {t}
              </span>
            ))}
            <div style={{ flex: 1 }} />
            <span className="mono" style={{ fontSize: 10, color: "#585550" }}>
              filter applied <b style={{ color: "#3FCFA8", fontWeight: 500 }}>during</b> descent, never after
            </span>
          </div>

          <div style={{ flex: 1, minHeight: 0, overflowY: "auto" }}>
            {err && (
              <div style={{ fontSize: 12, color: "#E0A34E", padding: "12px 0" }}>◌ {err}</div>
            )}
            {near && near.neighbours.length === 0 && (
              <div style={{ fontSize: 12.5, lineHeight: 1.7, color: "#585550", maxWidth: 560 }}>
                Nothing passed the filter. {num(stats?.candidates ?? 0)} of{" "}
                {num(stats?.corpus ?? 0)} pages were eligible — narrow the scope less, or
                drop a required tag. The search itself did not fail: it returned every
                page there was.
              </div>
            )}
            <div style={{ display: "flex", flexDirection: "column" }}>
              {(near?.neighbours ?? []).map((n, i, arr) => (
                <div
                  key={n.page_id}
                  onClick={() => navigate(`/discover/${n.page_id}`)}
                  style={{
                    display: "flex", alignItems: "center", gap: 16, padding: "14px 0",
                    cursor: "pointer",
                    borderBottom: i === arr.length - 1 ? undefined : "1px solid rgba(255,255,255,.07)",
                  }}
                >
                  <span className="mono" style={{
                    fontSize: 10, width: 44,
                    color: n.cosine >= maxCosine * 0.85 ? "#E8873C" : "#9B968D",
                  }}>
                    {n.cosine.toFixed(2)}
                  </span>
                  <div style={{ flex: 1, minWidth: 0 }}>
                    <div style={{ fontFamily: "Spectral, serif", fontSize: 16, color: "#EFEDE7" }}>
                      {n.title}
                    </div>
                    <div style={{
                      fontSize: 12, color: "#8C8880", marginTop: 3,
                      overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap",
                    }}>
                      {n.excerpt}
                    </div>
                    <div className="tgrow" style={{ marginTop: 7 }}>
                      {n.topic_name
                        ? <TopicChip name={n.topic_name} colorKey={n.topic_color_key} small />
                        : <span className="chip">UNTOPICED</span>}
                      {n.tags.slice(0, 3).map((t) => <span key={t} className="tg">{t}</span>)}
                      <span className="mono" style={{ fontSize: 9.5, color: "#4B4842", marginLeft: 4 }}>
                        {n.shared_tags} shared tag{n.shared_tags === 1 ? "" : "s"}
                      </span>
                    </div>
                  </div>
                  <div style={{ width: 120, height: 3, background: "rgba(255,255,255,.09)" }}>
                    <div style={{
                      width: `${Math.round((n.cosine / maxCosine) * 100)}%`,
                      height: "100%",
                      background: `rgba(232,135,60,${0.4 + (n.cosine / maxCosine) * 0.6})`,
                    }} />
                  </div>
                </div>
              ))}
            </div>

            {/* Three signals, kept separate and never blended into one number:
                a blended score is unarguable, and an unarguable score is one
                you cannot debug when it puts the wrong page first. */}
            {(near?.neighbours.length ?? 0) > 0 && (
              <div style={{ marginTop: 24, paddingTop: 20, borderTop: "1px solid rgba(255,255,255,.07)" }}>
                <Label style={{ marginBottom: 12, display: "block" }}>
                  WHY EACH ONE SURFACED · THREE SIGNALS, NOT ONE BLENDED SCORE
                </Label>
                <div style={{
                  display: "grid", gridTemplateColumns: "150px 1fr 1fr 1fr",
                  gap: "9px 14px", alignItems: "center", fontSize: 11.5,
                }}>
                  <span className="mono" style={{ fontSize: 9, letterSpacing: ".14em", color: "#585550" }} />
                  <span className="mono" style={{ fontSize: 9, letterSpacing: ".14em", color: "#585550" }}>TERM COSINE</span>
                  <span className="mono" style={{ fontSize: 9, letterSpacing: ".14em", color: "#585550" }}>TAG OVERLAP · JACCARD</span>
                  <span className="mono" style={{ fontSize: 9, letterSpacing: ".14em", color: "#585550" }}>GRAPH DISTANCE</span>

                  {(near?.neighbours ?? []).map((n) => (
                    <Signals key={n.page_id} n={n} maxCosine={maxCosine} />
                  ))}
                </div>
                {(() => {
                  // The finding, computed rather than asserted: the row with
                  // real prose similarity that neither the tags nor the graph
                  // would ever have surfaced.
                  const odd = (near?.neighbours ?? []).find(
                    (n) => n.shared_tags === 0 && n.hops < 0,
                  );
                  if (!odd) return null;
                  return (
                    <div style={{
                      display: "flex", gap: 9, marginTop: 14, padding: "9px 12px",
                      border: "1px solid rgba(125,158,201,.28)", background: "rgba(125,158,201,.05)",
                    }}>
                      <span style={{ color: "#7D9EC9", fontSize: 11 }}>✦</span>
                      <div style={{ flex: 1, fontSize: 11.5, lineHeight: 1.55, color: "#9B968D" }}>
                        <b style={{ color: "#C3BFB7", fontWeight: 500 }}>{odd.title}</b> is the
                        interesting row: cosine {odd.cosine.toFixed(2)}, zero shared tags,
                        unreachable by link. Prose similarity found something the graph and the
                        tags both missed — which is the only reason to run an index at all.
                      </div>
                    </div>
                  );
                })()}
              </div>
            )}
          </div>

          <div style={{
            marginTop: "auto", display: "flex", gap: 26, paddingTop: 22,
            borderTop: "1px solid rgba(255,255,255,.07)",
          }}>
            <Readout k="DISTANCE COMPS" v={num(stats?.comparisons ?? 0)} size={15} />
            <Readout k="EXACT WOULD COST" v={num(stats?.exact_comparisons ?? 0)} size={15} tone="#6E6A63" />
            <Readout
              k="RECALL@5 VS BRUTE FORCE"
              v={`${Math.round((stats?.recall_at_k ?? 0) * K)} / ${K}`}
              size={15}
              tone={(stats?.recall_at_k ?? 0) >= 1 ? "#3FCFA8" : "#E0A34E"}
            />
            <Readout
              k="FILTERED CANDIDATES"
              v={<>{num(stats?.candidates ?? 0)} <span style={{ fontSize: 10, color: "#585550" }}>of {num(stats?.corpus ?? 0)}</span></>}
              size={15}
            />
          </div>
        </div>

        <Inspector
          tabs={[{ id: "hnsw", label: "HNSW" }, { id: "index", label: "INDEX" }]}
          active={insTab}
          onSelect={(id) => setInsTab(id as "hnsw" | "index")}
          width={352}
        >
        {insTab === "index" ? (
          <div style={{ display: "flex", flexDirection: "column", gap: 12 }}>
            <Label>THE INDEX</Label>
            <div className="mono" style={{ fontSize: 10.5, lineHeight: 1.95, color: "#8C8880" }}>
              vectors&nbsp;&nbsp;&nbsp;&nbsp;{stats?.corpus ?? 0}<br />
              dimensions&nbsp;256<br />
              layers&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;{stats?.layers ?? 0}<br />
              per layer&nbsp;&nbsp;{(stats?.layer_sizes ?? []).join(" · ") || "—"}
            </div>

            <div style={{ height: 1, background: "rgba(255,255,255,.07)" }} />
            <Label>WHY ONLY L0 AND L1</Label>
            <div style={{ fontSize: 11.5, lineHeight: 1.65, color: "#8C8880" }}>
              Layer assignment is geometric: P(level ≥ l) = exp(−l/m<sub>L</sub>) with
              m<sub>L</sub> = 1/ln(M) and M = 16. So P(≥ 1) = 6.3% and P(≥ 2) = 0.39% —
              over {stats?.corpus ?? 0} vectors that is{" "}
              <span className="mono" style={{ color: "#E4E2DC" }}>
                {(((stats?.corpus ?? 0) * 0.0039)).toFixed(2)}
              </span>{" "}
              expected nodes at L2.
            </div>
            <div style={{ fontSize: 11.5, lineHeight: 1.65, color: "#8C8880" }}>
              Two layers is not the index failing to build — it is the overwhelmingly
              likely outcome at this size. An L2 node becomes more likely than not at
              around 250 pages; the upper layers only do real work in the thousands.
            </div>
            <div style={{ fontSize: 11, lineHeight: 1.6, color: "#585550" }}>
              Upper layers exist so the greedy descent can skip distance, and that only
              pays once there is distance to skip. At this size the descent is close to a
              scan — which is exactly why recall is measured against a brute-force scan on
              every query instead of asserted.
            </div>

            <div style={{ height: 1, background: "rgba(255,255,255,.07)" }} />
            <Label>NOT AN EMBEDDING MODEL</Label>
            <div style={{ fontSize: 11.5, lineHeight: 1.65, color: "#8C8880" }}>
              Hashed IDF-weighted term vectors — lexical, not semantic. There is no model
              in this repository. A page about the same idea in different words scores far
              away here, and that gap is the honest finding rather than a defect.
            </div>
          </div>
        ) : (
          <>
          <Label>LAYERS · GREEDY DESCENT</Label>
          <LayerTower sizes={stats?.layer_sizes ?? []} />

          <Rule />
          <Label>PARAMETERS</Label>
          <div className="mono" style={{ fontSize: 10.5, lineHeight: 1.95, color: "#8C8880" }}>
            M&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;{stats?.m ?? 16}<br />
            Mmax0&nbsp;&nbsp;&nbsp;&nbsp;{(stats?.m ?? 16) * 2}<br />
            efSearch&nbsp;&nbsp;{stats?.ef_search ?? 64}<br />
            pruning&nbsp;&nbsp;&nbsp;heuristic
          </div>

          <Rule />
          <div style={{ fontSize: 11.5, color: "#8C8880", lineHeight: 1.65 }}>
            Speed without recall is half a result, so the exact answer is computed beside
            every query — which is why this screen can be caught being wrong.
          </div>

          <Rule />
          <Label>WHAT THE VECTORS ARE</Label>
          <div style={{ fontSize: 11.5, lineHeight: 1.65, color: "#8C8880" }}>
            Hashed, IDF-weighted term frequencies at {stats?.dimensions ?? 256} dimensions —
            not neural embeddings; there is no model in this repo. They capture{" "}
            <b style={{ color: "#C3BFB7", fontWeight: 500 }}>lexical</b> similarity: two pages
            using the same uncommon words score high, and “rope” and “cord” are unrelated to
            them. Swapping in real embeddings changes one function and nothing else — the
            index does not know where its vectors came from.
          </div>

          <Rule />
          <Label>THIS PAGE'S HEAVIEST TERMS</Label>
          <div className="tgrow">
            {(stats?.top_terms ?? []).map((t) => <span key={t} className="tg">{t}</span>)}
            {(stats?.top_terms.length ?? 0) === 0 && (
              <span style={{ fontSize: 11.5, color: "#585550" }}>No indexable prose yet.</span>
            )}
          </div>

          <Rule />
          <Label>FILTER RIDES THE DESCENT</Label>
          <div style={{ fontSize: 11.5, lineHeight: 1.65, color: "#8C8880" }}>
            Topic and tag constraints are applied{" "}
            <b style={{ color: "#C3BFB7", fontWeight: 500 }}>during</b> greedy descent, not to
            the result set. Post-filtering asks for k={K}, throws three away and ships two —
            and recall collapses exactly when the filter is narrow, which is when someone is
            relying on it.
          </div>
          </>
        )}
        </Inspector>
      </Body>

      <StatusBar
        route={`/discover/${source ?? ""}`}
        mechanism="HNSW · hashed TF-IDF · exact scan beside every query"
        state={stats ? `recall@${K} ${stats.recall_at_k.toFixed(2)} · ${num(stats.comparisons)} of ${num(stats.exact_comparisons)} comparisons` : "no query yet"}
        healthy={(stats?.recall_at_k ?? 1) >= 1}
      />
    </Screen>
  );
}

/** One row of the three-signal table. */
function Signals({
  n, maxCosine,
}: { n: import("../api/discover").SemanticNeighbour; maxCosine: number }) {
  return (
    <>
      <span style={{ color: "#D2CFC8", overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
        {n.title}
      </span>
      <div style={{ display: "flex", alignItems: "center", gap: 7 }}>
        <div style={{ flex: 1, height: 4, background: "rgba(255,255,255,.06)" }}>
          <div style={{
            width: `${Math.round((n.cosine / maxCosine) * 100)}%`,
            height: "100%",
            background: `rgba(232,135,60,${0.5 + (n.cosine / maxCosine) * 0.5})`,
          }} />
        </div>
        <span className="mono" style={{ fontSize: 9.5, color: "#8C8880" }}>{n.cosine.toFixed(2)}</span>
      </div>
      <div style={{ display: "flex", alignItems: "center", gap: 7 }}>
        <div style={{ flex: 1, height: 4, background: "rgba(255,255,255,.06)" }}>
          <div style={{ width: `${Math.round(n.tag_jaccard * 100)}%`, height: "100%", background: "#7AA8E8" }} />
        </div>
        <span className="mono" style={{ fontSize: 9.5, color: n.shared_tags ? "#8C8880" : "#4B4842" }}>
          {n.shared_tags}/{n.tags.length || 0}
        </span>
      </div>
      <span className="mono" style={{ fontSize: 9.5, color: n.hops < 0 ? "#E0A34E" : "#8C8880" }}>
        {n.hops < 0 ? "unreachable" : `${n.hops} hop${n.hops === 1 ? "" : "s"}`}
      </span>
    </>
  );
}

/**
 * The HNSW tower, drawn from the index's OWN layer sizes.
 *
 * Not the mockup's fixed four layers: a 20-page corpus does not build four,
 * and drawing them anyway would be the one dishonest thing on a screen whose
 * whole argument is that its figures are computed. The ember path is the
 * descent — one element per layer, ending at layer 0.
 */
function LayerTower({ sizes }: { sizes: number[] }) {
  if (sizes.length === 0) {
    return <span style={{ fontSize: 11.5, color: "#585550" }}>No index built yet.</span>;
  }
  const H = 210, W = 320;
  const rows = [...sizes].reverse(); // top layer first, as drawn
  const gap = rows.length > 1 ? (H - 40) / (rows.length - 1) : 0;
  const y = (i: number) => 20 + i * gap;
  const max = Math.max(...sizes, 1);

  return (
    <svg viewBox={`0 0 ${W} ${H}`} style={{ width: "100%", display: "block" }}>
      <g fontFamily="IBM Plex Mono" fontSize="9" fill="#4B4842">
        {rows.map((_, i) => (
          <text key={i} x="0" y={y(i) + 4}>L{rows.length - 1 - i}</text>
        ))}
      </g>
      <g stroke="rgba(255,255,255,.1)">
        {rows.map((_, i) => <line key={i} x1="28" y1={y(i)} x2="300" y2={y(i)} />)}
      </g>
      <g fill="#585550">
        {rows.map((n, i) => {
          // One dot per element, capped so a wide layer 0 stays a row of
          // dots rather than a solid bar.
          const count = Math.min(n, 14);
          const r = Math.max(2, 4.5 - (n / max) * 2);
          return Array.from({ length: count }, (_, j) => (
            <circle key={`${i}-${j}`} cx={40 + (j * 260) / Math.max(count - 1, 1)} cy={y(i)} r={r} />
          ));
        })}
      </g>
      {/* The descent: enters at the top layer, ends at layer 0. */}
      <g stroke="#E8873C" strokeWidth="1.5" fill="none">
        <path d={rows.map((_, i) => `${i === 0 ? "M" : "L"}${120 + i * 24} ${y(i)}`).join(" ")} />
      </g>
      <g fill="#E8873C">
        {rows.map((_, i) => <circle key={i} cx={120 + i * 24} cy={y(i)} r={4.5 - i * 0.3} />)}
      </g>
      <g fontFamily="IBM Plex Mono" fontSize="9" fill="#585550">
        {rows.map((n, i) => <text key={i} x="304" y={y(i) + 4}>{n}</text>)}
      </g>
    </svg>
  );
}

export default DiscoverScreen;
