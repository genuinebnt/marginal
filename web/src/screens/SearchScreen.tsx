/**
 * docs/ui-mockups/v2/index.html § 06 SEARCH, ported.
 *
 * Real: the query itself (Postgres FTS via internal/search — rank and
 * snippet both come from the server, never recomputed here), "did you mean"
 * (a genuine BK-tree over titles, and the distances shown are the ones it
 * returned), and the topic/tag facets.
 *
 * The facet counts are computed from the hits the server returned — a
 * groupBy for display, not a second search. Selecting a facet filters the
 * already-returned hits rather than re-querying: at this scale a round trip
 * to narrow ten results would be slower and no more correct.
 *
 * The screen's own admission, kept from the mockup: the index has its own
 * cadence and may lag. A search UI that implies its index is transactional
 * is lying about a thing the user can catch it on.
 */
import { useCallback, useEffect, useMemo, useState } from "react";
import { useNavigate } from "react-router-dom";
import { useAuth } from "../auth/AuthContext";
import { search, suggestTitles, type SearchHit, type TitleSuggestion } from "../api/search";
import { listPages, type Page } from "../api/pages";
import { getTopics, type Topic } from "../api/topics";
import {
  Body, Label, Readout, Rule, Screen, StatusBar, TopBar, TopicChip, TOPIC_HEX, num,
} from "../shell/Chrome";
import { ph, PlaceholderNote, undrawn } from "../shell/placeholder";

/**
 * Renders ts_headline's own <b> markers as the mockup's ember bold.
 *
 * The server's markers are used rather than re-highlighting the raw query
 * client-side, because only the server knows what actually matched: FTS
 * stems, so a search for "step" legitimately highlights "steps", and
 * "running" matches "run". A client-side regex over the raw query would
 * miss every one of those and highlight nothing — while also rendering
 * these tags as literal text, which is the bug this replaced.
 *
 * Split on the marker pair rather than parsing HTML: ts_headline emits only
 * <b>/</b> (its StartSel/StopSel defaults) and the surrounding text is
 * already escaped, so there is no markup here to interpret and nothing to
 * gain from dangerouslySetInnerHTML.
 */
function Snippet({ text }: { text: string }) {
  const parts = text.split(/<\/?b>/);
  return (
    <>
      {parts.map((p, i) =>
        // Odd indices are the spans that sat between <b> and </b>.
        i % 2 === 1
          ? <span key={i} style={{ color: "#E8873C", fontWeight: 600 }}>{p}</span>
          : <span key={i}>{p}</span>,
      )}
    </>
  );
}

export function SearchScreen() {
  const { session } = useAuth();
  const actorId = session?.actorId ?? null;
  const navigate = useNavigate();

  const [query, setQuery] = useState("");
  const [hits, setHits] = useState<SearchHit[]>([]);
  const [suggestions, setSuggestions] = useState<TitleSuggestion[]>([]);
  const [pages, setPages] = useState<Page[]>([]);
  const [topics, setTopics] = useState<Topic[]>([]);
  const [topicFilter, setTopicFilter] = useState<string | null>(null);
  const [tagFilter, setTagFilter] = useState<string[]>([]);
  const [elapsed, setElapsed] = useState<number | null>(null);
  const [err, setErr] = useState<string | null>(null);

  useEffect(() => {
    if (!actorId) return;
    listPages(actorId).then((r) => setPages(r.pages)).catch(() => {});
    getTopics(actorId).then((r) => setTopics(r.topics)).catch(() => {});
  }, [actorId]);

  const run = useCallback((q: string) => {
    if (!actorId || !q.trim()) { setHits([]); setSuggestions([]); setElapsed(null); return; }
    const started = performance.now();
    search(actorId, q)
      .then((r) => { setHits(r.hits); setElapsed(Math.round(performance.now() - started)); setErr(null); })
      .catch((e) => setErr(String(e)));
    // Fuzzy titles run alongside rather than only on zero results: the
    // mockup shows them always, and a near-miss title is worth offering even
    // when full-text found something.
    suggestTitles(actorId, q).then((r) => setSuggestions(r.suggestions)).catch(() => {});
  }, [actorId]);

  // Debounced, so a query runs per pause rather than per keystroke.
  useEffect(() => {
    const t = setTimeout(() => run(query), 220);
    return () => clearTimeout(t);
  }, [query, run]);

  const pageOf = useMemo(() => new Map(pages.map((p) => [p.id, p])), [pages]);

  /** Facet counts over the returned hits — a groupBy, not a second search. */
  const topicCounts = useMemo(() => {
    const m = new Map<string, number>();
    hits.forEach((h) => {
      const key = pageOf.get(h.page_id)?.topic?.color_key;
      if (key) m.set(key, (m.get(key) ?? 0) + 1);
    });
    return m;
  }, [hits, pageOf]);

  const tagCounts = useMemo(() => {
    const m = new Map<string, number>();
    hits.forEach((h) => (pageOf.get(h.page_id)?.tags ?? []).forEach((t) => m.set(t, (m.get(t) ?? 0) + 1)));
    return [...m].sort((a, b) => b[1] - a[1] || a[0].localeCompare(b[0]));
  }, [hits, pageOf]);

  /** Topic is single-select (a page has exactly one); tags are AND. */
  const shown = useMemo(() => hits.filter((h) => {
    const p = pageOf.get(h.page_id);
    if (topicFilter && p?.topic?.color_key !== topicFilter) return false;
    if (tagFilter.length > 0 && !tagFilter.every((t) => (p?.tags ?? []).includes(t))) return false;
    return true;
  }), [hits, pageOf, topicFilter, tagFilter]);

  const toggleTag = (t: string) =>
    setTagFilter((cur) => (cur.includes(t) ? cur.filter((x) => x !== t) : [...cur, t]));

  return (
    <Screen>
      <TopBar
        readouts={
          <>
            <Readout k="INDEX LAG" v={ph("—")} tone="#E0A34E" />
            <Readout k="QUERY" v={elapsed === null ? "—" : `${elapsed} ms`} />
          </>
        }
        right={
          <input
            value={query}
            autoFocus
            placeholder="search…"
            onChange={(e) => setQuery(e.target.value)}
            style={{
              flex: 1, minWidth: 260, maxWidth: 420, padding: "6px 11px",
              background: "#141617", border: "1px solid rgba(232,135,60,.3)",
              font: "400 12.5px 'IBM Plex Mono',monospace", color: "#E4E2DC", outline: "none",
            }}
          />
        }
      />

      <Body>
        <div className="rail">
          <div className="rail-h">FACETS<div /></div>
          <div style={{ padding: "0 14px", display: "flex", flexDirection: "column", gap: 16, overflowY: "auto" }}>
            {/* Topic facet: single-select, because a page has exactly one
                topic. Counts are over the current hits, so picking one
                narrows the tag facet below. */}
            <div>
              <Label>TOPIC · SINGLE-SELECT</Label>
              <div style={{ marginTop: 8, display: "flex", flexDirection: "column", gap: 6 }}>
                {topics.map((t) => {
                  const n = topicCounts.get(t.color_key) ?? 0;
                  const on = topicFilter === t.color_key;
                  return (
                    <div
                      key={t.id}
                      onClick={() => setTopicFilter(on ? null : t.color_key)}
                      style={{
                        display: "flex", alignItems: "center", gap: 8,
                        cursor: n > 0 ? "pointer" : "default", opacity: n > 0 ? 1 : 0.45,
                      }}
                    >
                      <span style={{ width: 5, height: 5, background: TOPIC_HEX[t.color_key] }} />
                      <span style={{ flex: 1, fontSize: 12, color: on ? "#E4E2DC" : "#9B968D" }}>{t.name}</span>
                      <span className="mono" style={{ fontSize: 10, color: on ? "#E8873C" : "#585550" }}>{n}</span>
                    </div>
                  );
                })}
              </div>
            </div>

            <Rule />
            {/* Tag facet: multi-select AND, so the count in the header is an
                intersection rather than a union. */}
            <div>
              <Label>TAGS · MULTI · AND</Label>
              <div className="tgrow" style={{ marginTop: 9 }}>
                {tagCounts.slice(0, 12).map(([t, n]) => (
                  <span
                    key={t}
                    className={`tg${tagFilter.includes(t) ? " tg-on" : ""}`}
                    style={{ cursor: "pointer" }}
                    onClick={() => toggleTag(t)}
                  >
                    {t}<span style={{ color: "#585550", marginLeft: 5, fontSize: 9 }}>{n}</span>
                  </span>
                ))}
                {tagCounts.length === 0 && (
                  <span className="mono" style={{ fontSize: 9.5, color: "#585550" }}>
                    no tags on these results
                  </span>
                )}
                {tagCounts.length > 0 && (
                  <span className="mono" style={{ fontSize: 9.5, color: "#585550", width: "100%", marginTop: 3 }}>
                    {tagFilter.length} of {tagCounts.length} selected
                  </span>
                )}
              </div>
            </div>

            <Rule />
            <div style={undrawn}>
              <Label>BLOCK KIND</Label>
              <div className="mono" style={{ marginTop: 8, fontSize: 10.5, color: "#585550", lineHeight: 1.5 }}>
                needs block kind on the hit
              </div>
            </div>

            <Rule />
            <div>
              <Label>DID YOU MEAN</Label>
              <div className="mono" style={{ marginTop: 8, fontSize: 11, lineHeight: 1.9, color: "#8C8880" }}>
                {suggestions.length === 0 && (
                  <span style={{ color: "#585550" }}>{query.trim() ? "no near titles" : "—"}</span>
                )}
                {suggestions.map((s) => (
                  <div key={s.page_id} style={{ cursor: "pointer" }} onClick={() => navigate(`/pages/${s.page_id}`)}>
                    {s.title} <span style={{ color: "#4B4842" }}>· d={s.distance}</span>
                  </div>
                ))}
              </div>
              <div style={{ marginTop: 8, fontSize: 10.5, color: "#585550", lineHeight: 1.5 }}>
                BK-tree over {num(pages.length)} titles — triangle inequality prunes the rest
              </div>
            </div>
          </div>

          <div className="wal">
            <Label>INDEX CADENCE</Label>
            <PlaceholderNote>lag needs the projector's own clock</PlaceholderNote>
            <span style={{ fontSize: 10.5, color: "#585550" }}>the index has its own clock</span>
          </div>
        </div>

        <div style={{ flex: 1, minWidth: 0, padding: "26px 34px", overflow: "hidden", display: "flex", flexDirection: "column" }}>
          <div style={{ display: "flex", alignItems: "baseline", gap: 12, marginBottom: 22 }}>
            <span className="mono" style={{ fontSize: 11, color: "#8C8880" }}>
              {num(shown.length)} result{shown.length === 1 ? "" : "s"}
            </span>
            <span className="mono" style={{ fontSize: 11, color: "#585550" }}>
              {elapsed !== null && `· ${elapsed} ms`}
              {shown.length !== hits.length && ` · ${hits.length - shown.length} filtered out`}
            </span>
            <div style={{ flex: 1 }} />
            <span className="chip chip-e">RELEVANCE</span>
            <span className="chip" style={undrawn}>RECENT</span>
          </div>

          <div style={{ display: "flex", flexDirection: "column", overflowY: "auto" }}>
            {query.trim() === "" && (
              <div style={{ fontSize: 12.5, color: "#585550", lineHeight: 1.7, maxWidth: 560 }}>
                Full text across every page, plus fuzzy title matching for when you nearly
                remember the name. The index is built from the op log on its own schedule, so a
                page edited a moment ago may not be here yet — which this screen says out loud
                rather than implying otherwise.
              </div>
            )}
            {query.trim() !== "" && shown.length === 0 && (
              <div style={{ fontSize: 12.5, color: "#585550", lineHeight: 1.7 }}>
                Nothing matched{tagFilter.length > 0 || topicFilter ? " with these facets applied" : ""}.
                {suggestions.length > 0 && " A near title is offered in the rail."}
              </div>
            )}

            {shown.map((h, i) => {
              const p = pageOf.get(h.page_id);
              const first = i === 0;
              return (
                <div
                  key={`${h.page_id}-${h.block_id ?? i}`}
                  onClick={() => navigate(`/pages/${h.page_id}`)}
                  style={{
                    padding: first ? "16px 0 16px 16px" : "16px 0 16px 16px",
                    borderBottom: "1px solid rgba(255,255,255,.07)",
                    borderLeft: first ? "2px solid #E8873C" : undefined,
                    background: first ? "rgba(232,135,60,.04)" : undefined,
                    cursor: "pointer",
                  }}
                >
                  <div style={{ display: "flex", alignItems: "baseline", gap: 10 }}>
                    <span style={{ fontFamily: "Spectral,serif", fontSize: 17, color: first ? "#EFEDE7" : "#E4E2DC" }}>
                      {h.page_title}
                    </span>
                    <span style={{ flex: 1 }} />
                    <span className="mono" style={{ fontSize: 10, color: "#585550" }}>
                      score {h.rank.toFixed(2)}
                    </span>
                  </div>
                  {h.snippet && (
                    <div style={{ fontSize: 13, color: "#8C8880", lineHeight: 1.6, marginTop: 6 }}>
                      <Snippet text={h.snippet} />
                    </div>
                  )}
                  <div className="tgrow" style={{ marginTop: 9 }}>
                    {p?.topic && <TopicChip name={p.topic.name} colorKey={p.topic.color_key} />}
                    {(p?.tags ?? []).map((t) => (
                      <span key={t} className={`tg${tagFilter.includes(t) ? " tg-on" : ""}`}>{t}</span>
                    ))}
                    <span className="mono" style={{ fontSize: 10, color: "#585550", marginLeft: "auto" }}>
                      {h.block_id ? `block ${h.block_id.slice(0, 4)}` : "title match"}
                    </span>
                  </div>
                </div>
              );
            })}
          </div>
        </div>
      </Body>

      <StatusBar
        route="/search"
        mechanism="postgres FTS · BK-tree titles"
        state={err ? "search unavailable" : "index may trail the tree"}
        healthy={false}
      />
    </Screen>
  );
}

export default SearchScreen;
