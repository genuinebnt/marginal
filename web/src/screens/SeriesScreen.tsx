/**
 * docs/ui-mockups/v2/index.html § 10d SERIES, ported.
 *
 * A SERIES IS A PAGE WITH CHILDREN. There is no series table and no
 * `series_name` column: the page tree already says "these belong together, in
 * this order", and `sort_key` already orders them — so dragging a row in the
 * rail reorders the series, by construction rather than by a second write.
 *
 * The shape is adapted from genuine-folio's /series and /series/:slug: an
 * index of series as cards, and one series as an ordered parts list with the
 * same grid/list toggle every other list-shaped screen here uses. What is
 * NOT adapted is its CSS — that belongs to a different design system, and
 * importing it would fork this one's palette.
 */
import { useEffect, useMemo, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { useAuth } from "../auth/AuthContext";
import { listSeries, type SeriesSummary } from "../api/series";
import {
  Body, Label, Main, Readout, Rule, Screen, StatusBar, SubBar, SubItem,
  TopBar, TopicChip, num,
} from "../shell/Chrome";
import { PageCard, RowBars, ViewToggle, readMinutes, type ViewMode } from "../ui";

export function SeriesScreen() {
  const { id } = useParams();
  const navigate = useNavigate();
  const { session } = useAuth();
  const actorId = session?.actorId ?? null;

  const [series, setSeries] = useState<SeriesSummary[]>([]);
  const [view, setView] = useState<ViewMode>("grid");
  const [err, setErr] = useState<string | null>(null);

  useEffect(() => {
    if (!actorId) return;
    listSeries(actorId).then((r) => setSeries(r.series)).catch((e) => setErr(String(e)));
  }, [actorId]);

  const one = useMemo(
    () => (id ? series.find((s) => s.series_page_id === id) ?? null : null),
    [id, series],
  );

  const totalParts = series.reduce((n, s) => n + s.part_count, 0);
  const totalWords = series.reduce((n, s) => n + s.word_count, 0);

  return (
    <Screen>
      <TopBar
        crumb={one ? <>series / <b>{one.title}</b></> : <>series</>}
        readouts={
          <>
            <Readout k="SERIES" v={num(series.length)} />
            <Readout k="PARTS" v={num(one ? one.part_count : totalParts)} />
            <Readout
              k="READ TIME"
              v={`~${num(readMinutes(one ? one.word_count : totalWords) ?? 0)} min`}
            />
          </>
        }
      />

      <SubBar>
        <SubItem on={!id} onClick={() => navigate("/series")}>ALL SERIES</SubItem>
        {series.map((s) => (
          <SubItem key={s.series_page_id} on={s.series_page_id === id}
                   onClick={() => navigate(`/series/${s.series_page_id}`)}>
            {s.title.toUpperCase()}
          </SubItem>
        ))}
        <div style={{ flex: 1 }} />
        <SubItem>a series is a page with children — reorder it in the rail</SubItem>
      </SubBar>

      <Body>
        <Main style={{ padding: "24px 32px", overflow: "hidden" }}>
          <div style={{ display: "flex", alignItems: "baseline", gap: 12, marginBottom: 16 }}>
            <h1 className="h1" style={{ fontSize: 22 }}>
              {one ? one.title : "Series"}
            </h1>
            <span className="mono" style={{ fontSize: 11, color: "#585550" }}>
              {one
                ? `${num(one.part_count)} parts · ~${num(readMinutes(one.word_count) ?? 0)} min · ordered by the page tree`
                : `${num(series.length)} in this workspace · a series is a page with children`}
            </span>
            <div style={{ flex: 1 }} />
            <ViewToggle mode={view} onChange={setView} />
          </div>

          {err && <div style={{ fontSize: 12, color: "#E0A34E" }}>◌ {err}</div>}

          {!err && series.length === 0 && (
            <div style={{ fontSize: 12.5, lineHeight: 1.7, color: "#585550", maxWidth: 620 }}>
              No series yet. Nest a page under another in the rail and the parent becomes
              one — there is nothing else to set up, because there is no series table to
              write to. Two children is the threshold: one child is a sub-page, and
              “Part 1 of 1” tells a reader nothing.
            </div>
          )}

          <div style={{ flex: 1, minHeight: 0, overflowY: "auto" }}>
            {/* THE INDEX — every series as a card, with a first/last preview.
                A full ordered part list does not fit a card, so the card shows
                the ends and the count; click through for the middle. */}
            {!one && view === "grid" && (
              <div style={{ display: "grid", gridTemplateColumns: "repeat(3, 1fr)", gap: 12 }}>
                {series.map((s, i) => (
                  <PageCard
                    key={s.series_page_id}
                    to={`/series/${s.series_page_id}`}
                    title={s.title}
                    topicName={s.topic?.name}
                    colorKey={s.topic?.color_key}
                    delay={i * 0.04}
                    excerpt={
                      <>
                        <b style={{ color: "#9B968D" }}>01</b> {s.parts[0]?.title}
                        <br />
                        <b style={{ color: "#9B968D" }}>
                          {String(s.part_count).padStart(2, "0")}
                        </b>{" "}
                        {s.parts[s.parts.length - 1]?.title}
                      </>
                    }
                    meta={
                      <>
                        <span>{num(s.part_count)} parts</span>
                        <span>~{num(readMinutes(s.word_count) ?? 0)} min</span>
                      </>
                    }
                  />
                ))}
              </div>
            )}

            {!one && view === "list" && (
              <div style={{ display: "flex", flexDirection: "column" }}>
                {series.map((s) => (
                  <div
                    key={s.series_page_id}
                    className="srow"
                    onClick={() => navigate(`/series/${s.series_page_id}`)}
                  >
                    <RowBars colorKey={s.topic?.color_key} status="ok" />
                    <div style={{ flex: 1, minWidth: 0 }}>
                      <div className="srow-t">{s.title}</div>
                      <div className="mono srow-m">
                        {s.parts.slice(0, 3).map((p) => p.title).join(" · ")}
                        {s.part_count > 3 ? " …" : ""}
                      </div>
                    </div>
                    {s.topic && <TopicChip name={s.topic.name} colorKey={s.topic.color_key} small />}
                    <span className="mono srow-n">{num(s.part_count)} parts</span>
                    <span className="mono srow-n">~{num(readMinutes(s.word_count) ?? 0)} min</span>
                  </div>
                ))}
              </div>
            )}

            {/* ONE SERIES — the ordered parts. Numbered, because the order is
                the answer; a bulleted list of a series is a list that has
                thrown away the one thing it knows. */}
            {one && view === "grid" && (
              <div style={{ display: "grid", gridTemplateColumns: "repeat(3, 1fr)", gap: 12 }}>
                {one.parts.map((p, i) => (
                  <PageCard
                    key={p.page_id}
                    to={`/read/${p.page_id}`}
                    title={p.title}
                    topicName={p.topic?.name}
                    colorKey={p.topic?.color_key}
                    delay={Math.min(i, 12) * 0.03}
                    excerpt={p.tags.slice(0, 4).join(" · ")}
                    meta={
                      <>
                        <span style={{ color: "#E8873C" }}>
                          PART {String(p.number).padStart(2, "0")}
                        </span>
                        <span>~{num(readMinutes(p.word_count) ?? 0)} min</span>
                      </>
                    }
                  />
                ))}
              </div>
            )}

            {one && view === "list" && (
              <div style={{ display: "flex", flexDirection: "column" }}>
                {one.parts.map((p) => (
                  <div key={p.page_id} className="srow" onClick={() => navigate(`/read/${p.page_id}`)}>
                    <RowBars colorKey={p.topic?.color_key} status="ok" />
                    <span className="mono srow-p">{String(p.number).padStart(2, "0")}</span>
                    <div style={{ flex: 1, minWidth: 0 }}>
                      <div className="srow-t">{p.title}</div>
                      <div className="tgrow" style={{ marginTop: 4 }}>
                        {p.tags.slice(0, 4).map((t) => <span key={t} className="tg">{t}</span>)}
                      </div>
                    </div>
                    {p.topic && <TopicChip name={p.topic.name} colorKey={p.topic.color_key} small />}
                    <span className="mono srow-n">~{num(readMinutes(p.word_count) ?? 0)} min</span>
                  </div>
                ))}
              </div>
            )}
          </div>

          {one && (
            <div style={{
              marginTop: "auto", paddingTop: 20, borderTop: "1px solid rgba(255,255,255,.07)",
              display: "flex", gap: 26, alignItems: "flex-start",
            }}>
              <div style={{ flex: 1 }}>
                <Label style={{ marginBottom: 10, display: "block" }}>HOW THE ORDER IS DECIDED</Label>
                <div style={{ fontSize: 11.5, lineHeight: 1.7, color: "#8C8880", maxWidth: 560 }}>
                  By <span className="mono" style={{ color: "#C3BFB7" }}>sort_key</span> — the same
                  fractional index the page tree orders by, so dragging a part in the rail
                  reorders the series and there is no second write to keep in step. The parts
                  are children of{" "}
                  <span
                    className="mono"
                    style={{ color: "#E8873C", cursor: "pointer" }}
                    onClick={() => navigate(`/read/${one.series_page_id}`)}
                  >
                    {one.title}
                  </span>
                  , and that is the whole storage model.
                </div>
              </div>
              <Readout k="PARTS" v={num(one.part_count)} size={15} />
              <Readout k="WORDS" v={num(one.word_count)} size={15} />
              <Readout k="TOPICS SPANNED"
                       v={num(new Set(one.parts.map((p) => p.topic?.name).filter(Boolean)).size)}
                       size={15} />
            </div>
          )}
        </Main>
      </Body>

      <StatusBar
        route={one ? `/series/${one.series_page_id}` : "/series"}
        mechanism="a series is a page with children · ordered by sort_key"
        state={one ? `${num(one.part_count)} parts` : `${num(series.length)} series`}
        healthy
      />
      <Rule />
    </Screen>
  );
}

export default SeriesScreen;
