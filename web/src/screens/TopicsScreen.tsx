/**
 * docs/ui-mockups/v2/index.html § 10b TOPICS & TAGS, ported.
 *
 * The screen that makes the two-axis model visible, and its argument is the
 * reason both axes exist:
 *
 *   A TOPIC is singular, owned, indexed — one per page, a real column. It
 *   clusters the graph and scopes similarity search.
 *   A TAG is free-form and many. It facets search, never boosts rank, and
 *   never picks a hue.
 *
 * The tag cloud is sized by count and NOT coloured, deliberately: colour is
 * reserved for the topic, and a cloud that also carried hue would be two
 * encodings fighting for the same glance.
 *
 * Everything here is backed by real endpoints — /topics, /tags, /pages. The
 * per-topic groupings are computed client-side from those responses, which
 * is view aggregation (a groupBy over data the server returned) rather than
 * a second implementation of anything: no ranking, no scoring, no traversal
 * happens in TypeScript.
 */
import { useCallback, useEffect, useMemo, useState } from "react";
import { useNavigate } from "react-router-dom";
import { useAuth } from "../auth/AuthContext";
import { listPages, type Page } from "../api/pages";
import { getTagFacets, getTopics, setPageTopic, type TagFacet, type Topic } from "../api/topics";
import {
  Body, Inspector, Label, Readout, Rule, Screen, StatusBar, SubBar, SubItem,
  TopBar, TopicChip, TOPIC_HEX, num,
} from "../shell/Chrome";

/**
 * § 10b's own tag-cloud tiers, in rank order. Discrete rather than
 * interpolated: five legible sizes read as a ranking; a smooth ramp reads as
 * noise, and every chip lands between two of the mockup's steps.
 */
const TAG_TIERS = [
  { size: 14,   pad: "4px 11px", color: "#E4E2DC", border: "rgba(255,255,255,.2)", opacity: 1 },
  { size: 12.5, pad: "3px 10px", color: "#D2CFC8", border: undefined, opacity: 1 },
  { size: 12,   pad: "3px 9px",  color: "#C3BFB7", border: undefined, opacity: 1 },
  { size: 11.5, pad: "2px 7px",  color: undefined, border: undefined, opacity: 1 },
  { size: 11,   pad: "2px 7px",  color: undefined, border: undefined, opacity: 0.75 },
  { size: 11,   pad: "2px 7px",  color: undefined, border: undefined, opacity: 0.55 },
] as const;

/** The untopiced pseudo-topic. A real, selectable state — not a filter. */
const NONE = "__none__";

export function TopicsScreen() {
  const { session } = useAuth();
  const actorId = session?.actorId ?? null;
  const navigate = useNavigate();

  const [topics, setTopics] = useState<Topic[]>([]);
  const [untopiced, setUntopiced] = useState(0);
  const [facets, setFacets] = useState<TagFacet[]>([]);
  const [pages, setPages] = useState<Page[]>([]);
  const [selected, setSelected] = useState<string | null>(null);
  const [err, setErr] = useState<string | null>(null);

  const load = useCallback(() => {
    if (!actorId) return;
    getTopics(actorId).then((r) => { setTopics(r.topics); setUntopiced(r.untopiced_pages); })
      .catch((e) => setErr(String(e)));
    getTagFacets(actorId, 60).then((r) => setFacets(r.facets)).catch(() => {});
    listPages(actorId).then((r) => setPages(r.pages)).catch(() => {});
  }, [actorId]);

  useEffect(load, [load]);

  const sel = selected ?? topics[0]?.id ?? null;
  const selTopic = topics.find((t) => t.id === sel) ?? null;
  const showingNone = sel === NONE;

  /** Pages in the selected topic (or the untopiced ones). */
  const topicPages = useMemo(
    () => pages.filter((p) => (showingNone ? !p.topic : p.topic?.id === sel)),
    [pages, sel, showingNone],
  );

  /** Tag counts within the selection — a groupBy, not a ranking. */
  const tagsInTopic = useMemo(() => {
    const counts = new Map<string, number>();
    topicPages.forEach((p) => (p.tags ?? []).forEach((t) => counts.set(t, (counts.get(t) ?? 0) + 1)));
    return [...counts].sort((a, b) => b[1] - a[1] || a[0].localeCompare(b[0]));
  }, [topicPages]);

  /**
   * Tags in this topic that also appear outside it. The mockup's own point:
   * a tag living in several topics is naming a technique, not a subject —
   * which is exactly the distinction the two axes exist to keep.
   */
  const escaping = useMemo(() => {
    const inTopic = new Set(tagsInTopic.map(([t]) => t));
    return facets
      .filter((f) => inTopic.has(f.tag) && f.topics_spanned > 1)
      .slice(0, 6)
      .map((f) => {
        const here = tagsInTopic.find(([t]) => t === f.tag)?.[1] ?? 0;
        return { ...f, here };
      });
  }, [facets, tagsInTopic]);

  /** Where the selection's tags also live, by topic — the link-out panel. */
  const spread = useMemo(() => {
    const inTopic = new Set(tagsInTopic.map(([t]) => t));
    const byTopic = new Map<string, number>();
    pages.forEach((p) => {
      if (!p.topic || p.topic.id === sel) return;
      if ((p.tags ?? []).some((t) => inTopic.has(t))) {
        byTopic.set(p.topic.color_key, (byTopic.get(p.topic.color_key) ?? 0) + 1);
      }
    });
    const max = Math.max(...byTopic.values(), 1);
    return [...byTopic]
      .sort((a, b) => b[1] - a[1])
      .map(([key, n]) => ({
        key,
        n,
        pct: Math.round((n / max) * 100),
        name: topics.find((t) => t.color_key === key)?.name ?? key,
      }));
  }, [pages, sel, tagsInTopic, topics]);

  const onceOnly = useMemo(() => facets.filter((f) => f.page_count === 1), [facets]);

  /**
   * Tag pairs one edit apart. The mockup credits this to the BK-tree behind
   * "did you mean" — a second USE of that algorithm, not a second
   * implementation. There is no REST surface for it yet, so this is a plain
   * adjacent-pair check over the sorted list: it finds the same
   * single-character slips (op-log/oplog) without claiming to be the metric
   * tree, and it is replaced the moment that endpoint exists.
   */
  const nearDupes = useMemo(() => {
    const names = facets.map((f) => f.tag).sort();
    const out: Array<[string, string]> = [];
    for (let i = 1; i < names.length && out.length < 2; i++) {
      const a = names[i - 1], b = names[i];
      if (Math.abs(a.length - b.length) <= 1 && a.replace(/-/g, "") === b.replace(/-/g, "")) {
        out.push([a, b]);
      }
    }
    return out;
  }, [facets]);
  const pct = pages.length > 0 ? Math.round(((pages.length - untopiced) / pages.length) * 100) : 100;

  async function assign(pageId: string, topicId: string | null) {
    if (!actorId) return;
    await setPageTopic(actorId, pageId, topicId);
    load();
  }

  return (
    <Screen>
      <TopBar
        crumb={<>workspace / <b>topics</b></>}
        readouts={
          <>
            <Readout k="TOPICS" v={num(topics.length)} />
            <Readout k="TAGS" v={num(facets.length)} />
            <Readout k="UNTOPICED" v={num(untopiced)} tone={untopiced > 0 ? "#E0A34E" : undefined} />
          </>
        }
      />

      <SubBar>
        <SubItem on>TOPICS</SubItem>
        <SubItem>TAGS</SubItem>
        <SubItem>CO-OCCURRENCE</SubItem>
        <SubItem tone={untopiced > 0 ? "#E0A34E" : undefined} onClick={() => setSelected(NONE)}>
          UNTOPICED · {untopiced}
        </SubItem>
        <div style={{ flex: 1 }} />
        <SubItem tone="#585550">a topic is a column · a tag is a label</SubItem>
      </SubBar>

      <Body>
        <div className="rail" style={{ width: 250 }}>
          <div className="rail-h">TOPIC<div /><span style={{ color: "#585550" }}>{topics.length}</span></div>
          <div style={{ display: "flex", flexDirection: "column", padding: "0 8px", gap: 1 }}>
            {topics.map((t) => (
              <div
                key={t.id}
                className={`tr${t.id === sel ? " tr-on" : ""}`}
                style={{ cursor: "pointer" }}
                onClick={() => setSelected(t.id)}
              >
                {t.id === sel && <i />}
                <span style={{ width: 6, height: 6, background: TOPIC_HEX[t.color_key], flex: "none" }} />
                {t.name}
                <span style={{ marginLeft: "auto" }} className="tr-n">{t.page_count ?? 0}</span>
              </div>
            ))}
            <div
              className={`tr${showingNone ? " tr-on" : ""}`}
              style={{ color: "#E0A34E", cursor: "pointer" }}
              onClick={() => setSelected(NONE)}
            >
              {showingNone && <i />}
              <span style={{ width: 6, height: 6, border: "1px solid #E0A34E", flex: "none" }} />
              — none —
              <span style={{ marginLeft: "auto", color: "#E0A34E" }} className="tr-n">{untopiced}</span>
            </div>
          </div>

          <div style={{ margin: "16px 12px 0", paddingTop: 14, borderTop: "1px solid rgba(255,255,255,.07)" }}>
            <Label>RENAMING IS SAFE</Label>
            <div style={{ fontSize: 11, lineHeight: 1.6, color: "#8C8880", marginTop: 8 }}>
              A topic is stored by id, not by name. Rename{" "}
              <span className="mono" style={{ color: TOPIC_HEX[selTopic?.color_key ?? "protocol"] }}>
                {selTopic?.name ?? "a topic"}
              </span>{" "}
              and {selTopic?.page_count ?? 0} pages follow — no reindex, no rewrite, and the graph
              keeps its colour.
            </div>
          </div>
          <div className="wal">
            <Label>MERGE</Label>
            <div style={{ fontSize: 11, lineHeight: 1.55, color: "#8C8880" }}>
              Merging two topics is one <span className="mono" style={{ color: "#9B968D" }}>UPDATE</span>{" "}
              and one invalidation. Splitting one is not — it needs a human to say which page went where.
            </div>
          </div>
        </div>

        <div style={{
          flex: 1, minWidth: 0, padding: "26px 30px", overflow: "hidden",
          display: "flex", flexDirection: "column",
        }}>
          <div style={{ display: "flex", alignItems: "baseline", gap: 12, marginBottom: 4 }}>
            {showingNone
              ? <span className="chip chip-a" style={{ padding: "4px 11px" }}>UNTOPICED</span>
              : selTopic && <TopicChip name={selTopic.name} colorKey={selTopic.color_key} />}
            <h1 className="h1" style={{ fontSize: 23 }}>{num(topicPages.length)} pages</h1>
            <span className="mono" style={{ fontSize: 11, color: "#585550" }}>
              {pages.length > 0 ? Math.round((topicPages.length / pages.length) * 100) : 0}% of the
              workspace · {tagsInTopic.length} distinct tags
            </span>
            <div style={{ flex: 1 }} />
            <span className="chip">RENAME</span>
            <span className="chip">MERGE INTO…</span>
          </div>
          <div style={{ fontSize: 12.5, color: "#8C8880", lineHeight: 1.6, marginBottom: 22, maxWidth: 640 }}>
            {showingNone
              ? "These pages carry no topic. They still render, still search, still link — they simply do not colour on the graph."
              : "One owned classification per page. "}
            <span style={{ color: "#585550" }}>Described once here, not restated on every page.</span>
          </div>

          {/* Tag cloud sized by count, NOT coloured. Colour is reserved for
              the topic; a tag cloud that also carries hue is two encodings
              fighting. */}
          <Label>
            TAGS INSIDE THIS TOPIC · {tagsInTopic.length} OF {facets.length}
          </Label>
          <div className="tgrow" style={{ gap: 7, marginBottom: 26, alignItems: "baseline" }}>
            {tagsInTopic.length === 0 && (
              <span style={{ fontSize: 11.5, color: "#585550" }}>
                No tags here yet — a topic with no tags is a subject nobody has described a technique for.
              </span>
            )}
            {tagsInTopic.map(([tag, n], idx) => {
              // § 10b sizes the cloud in DISCRETE tiers, not on a continuous
              // ramp. Interpolating landed every chip between two of the
              // mockup's steps — and tiers are the better design anyway: five
              // legible sizes read as a ranking, where a smooth ramp just
              // reads as noise.
              const t = TAG_TIERS[Math.min(idx, TAG_TIERS.length - 1)];
              return (
                <span
                  key={tag}
                  className="tg"
                  style={{
                    fontSize: t.size,
                    padding: t.pad,
                    color: t.color,
                    borderColor: t.border,
                    opacity: t.opacity,
                    // A tag used once is drawn dashed — provisional rather
                    // than small, since one use is a note, not a facet.
                    borderStyle: n === 1 ? "dashed" : undefined,
                  }}
                >
                  {tag}
                  <span style={{ color: "#585550", marginLeft: 5, fontSize: 9 }}>{n}</span>
                </span>
              );
            })}
          </div>

          <div style={{ display: "grid", gridTemplateColumns: "1.25fr 1fr", gap: 28, flex: 1, minHeight: 0 }}>
            <div style={{ display: "flex", flexDirection: "column", minHeight: 0 }}>
              <Label>PAGES</Label>
              <div style={{ display: "flex", flexDirection: "column", overflowY: "auto" }}>
                {topicPages.map((p, i) => (
                  <div
                    key={p.id}
                    className="row"
                    style={{ padding: "9px 0", cursor: "pointer", borderBottom: i === topicPages.length - 1 ? 0 : undefined }}
                    onClick={() => navigate(`/pages/${p.id}`)}
                  >
                    <span style={{ flex: 1, fontSize: 13, color: i === 0 ? "#EFEDE7" : "#D2CFC8" }}>
                      {p.title}
                    </span>
                    <div className="tgrow">
                      {(p.tags ?? []).slice(0, 2).map((t) => <span key={t} className="tg">{t}</span>)}
                    </div>
                    <span className="mono" style={{ fontSize: 10, color: "#8C8880", width: 34, textAlign: "right" }}>
                      {(p.tags ?? []).length}
                    </span>
                  </div>
                ))}
              </div>
              <div className="mono" style={{ fontSize: 10, color: "#585550", marginTop: 11 }}>
                sorted by the tree's own order, not recency
              </div>
            </div>

            <div style={{ display: "flex", flexDirection: "column", gap: 18, minHeight: 0 }}>
              <div>
                <Label>WHERE THIS TOPIC'S TAGS ALSO LIVE</Label>
                <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
                  {spread.length === 0 && (
                    <span style={{ fontSize: 11.5, color: "#585550" }}>
                      Nothing in common with another topic — self-contained, which is rarer than it sounds.
                    </span>
                  )}
                  {spread.map((s) => (
                    <div key={s.key} style={{ display: "flex", alignItems: "center", gap: 9 }}>
                      <span style={{ width: 6, height: 6, background: TOPIC_HEX[s.key] }} />
                      <span style={{ flex: 1, fontSize: 12, color: "#D2CFC8" }}>{s.name}</span>
                      <div style={{ width: 70, height: 4, background: "rgba(255,255,255,.06)" }}>
                        <div style={{ width: `${s.pct}%`, height: "100%", background: TOPIC_HEX[s.key] }} />
                      </div>
                      <span className="mono" style={{ fontSize: 9.5, color: "#8C8880", width: 26, textAlign: "right" }}>
                        {s.n}
                      </span>
                    </div>
                  ))}
                </div>
              </div>

              <div>
                <Label>TAGS THAT ESCAPE THIS TOPIC</Label>
                <div style={{ display: "flex", flexDirection: "column", gap: 7 }}>
                  {escaping.length === 0 && (
                    <span style={{ fontSize: 11.5, color: "#585550" }}>
                      Every tag here stays here — these name subjects, not techniques.
                    </span>
                  )}
                  {escaping.map((e) => (
                    <div key={e.tag} style={{ display: "flex", alignItems: "center", gap: 9 }}>
                      <span className="tg" style={{ width: 78, justifyContent: "flex-start" }}>{e.tag}</span>
                      <div style={{ flex: 1, display: "flex", height: 5, gap: 1 }}>
                        <div style={{ flex: e.here, background: TOPIC_HEX[selTopic?.color_key ?? "protocol"] }} />
                        <div style={{ flex: Math.max(e.page_count - e.here, 0), background: "rgba(255,255,255,.14)" }} />
                      </div>
                      <span className="mono" style={{ fontSize: 9, color: "#585550" }}>
                        {e.here}/{e.page_count}
                      </span>
                    </div>
                  ))}
                </div>
                <div style={{ fontSize: 11, lineHeight: 1.6, color: "#585550", marginTop: 10 }}>
                  A tag that lives in three topics is doing real work — it names a technique, not a
                  subject. That is the difference the two axes exist to keep.
                </div>
              </div>
            </div>
          </div>
        </div>

        <Inspector
          tabs={[{ id: "hygiene", label: "HYGIENE" }, { id: "history", label: "HISTORY" }]}
          active="hygiene"
          width={300}
        >
          <Label>UNTOPICED · {untopiced}</Label>
          <div style={{ display: "flex", alignItems: "center", gap: 10 }}>
            <div style={{ flex: 1, height: 5, background: "rgba(255,255,255,.06)" }}>
              <div style={{ width: `${pct}%`, height: "100%", background: pct === 100 ? "#3FCFA8" : "#E0A34E" }} />
            </div>
            <span className="mono" style={{ fontSize: 10, color: "#8C8880" }}>{pct}%</span>
          </div>
          <div style={{ fontSize: 11.5, lineHeight: 1.6, color: "#8C8880" }}>
            {untopiced} pages carry no topic. They still render, still search, still link — they
            simply do not colour on the graph and never surface as a topic-scoped neighbour.
          </div>

          {showingNone && topicPages.length > 0 && (
            <>
              <Rule />
              <Label>ASSIGN</Label>
              <div style={{ fontSize: 11, lineHeight: 1.6, color: "#585550" }}>
                Assignment is yours. A wrong topic is worse than none, because none is visibly none.
              </div>
              {topicPages.map((p) => (
                <div key={p.id} style={{ display: "flex", flexDirection: "column", gap: 6 }}>
                  <span style={{ fontSize: 12, color: "#D2CFC8" }}>{p.title}</span>
                  <div className="tgrow">
                    {topics.map((t) => (
                      <span
                        key={t.id}
                        className="tg"
                        style={{ cursor: "pointer", borderColor: `${TOPIC_HEX[t.color_key]}66` }}
                        onClick={() => assign(p.id, t.id)}
                      >
                        {t.name}
                      </span>
                    ))}
                  </div>
                </div>
              ))}
            </>
          )}

          <Rule />
          <Label>TAG HYGIENE</Label>
          <div style={{ display: "flex", flexDirection: "column", gap: 9 }}>
            {onceOnly.length > 0 && (
              <div style={{ display: "flex", gap: 9 }}>
                <span style={{ color: "#E0A34E", fontSize: 10 }}>◌</span>
                <div style={{ flex: 1 }}>
                  <div style={{ fontSize: 12, color: "#D2CFC8" }}>{onceOnly.length} tags used once</div>
                  <div className="mono" style={{ fontSize: 10, color: "#585550", marginTop: 2 }}>
                    {onceOnly.slice(0, 4).map((f) => f.tag).join(" · ")}
                  </div>
                </div>
              </div>
            )}
            {nearDupes.map(([a, b]) => (
              <div key={a + b} style={{ display: "flex", gap: 9 }}>
                <span style={{ color: "#E0A34E", fontSize: 10 }}>◌</span>
                <div style={{ flex: 1 }}>
                  <div style={{ fontSize: 12, color: "#D2CFC8" }}>Near-duplicate pair</div>
                  <div className="mono" style={{ fontSize: 10, color: "#585550", marginTop: 2 }}>
                    {a} · {b} <span style={{ color: "#4B4842" }}>· d=1</span>
                  </div>
                </div>
              </div>
            ))}
            <div style={{ display: "flex", gap: 9 }}>
              <span style={{ color: "#3FCFA8", fontSize: 10 }}>✓</span>
              <div style={{ flex: 1, fontSize: 12, color: "#8C8880" }}>No tag over 40 chars</div>
            </div>
          </div>
          <div style={{ fontSize: 11, lineHeight: 1.6, color: "#585550" }}>
            A tag used once is not yet a facet — it is a note to yourself. Left alone rather than
            swept up: the cost of a stray tag is nearly zero, and deleting someone's vocabulary is not.
          </div>

          <Rule />
          <Label>WHY BOTH</Label>
          <div style={{
            display: "grid", gridTemplateColumns: "auto 1fr", gap: "6px 10px",
            fontSize: 11.5, lineHeight: 1.5, color: "#8C8880",
          }}>
            <span className="mono" style={{ fontSize: 9, letterSpacing: ".1em", color: "#7AA8E8" }}>TOPIC</span>
            <span>one, owned, indexed. Clusters the graph, scopes Discover, colours nothing else.</span>
            <span className="mono" style={{ fontSize: 9, letterSpacing: ".1em", color: "#8C8880" }}>TAG</span>
            <span>many, free, cheap. Facets search, never boosts rank, never picks a hue.</span>
          </div>
          <div style={{ fontSize: 11, lineHeight: 1.6, color: "#585550" }}>
            Collapse them into one field and you get folders — one axis, argued over forever, and a
            page that is genuinely two things has to lie.
          </div>
        </Inspector>
      </Body>

      <StatusBar
        route="/topics"
        mechanism="topic by id · rename is free"
        state={err ? "topics unavailable" : `${untopiced} untopiced · assignment is yours`}
        healthy={!err && untopiced === 0}
      />
    </Screen>
  );
}

export default TopicsScreen;
