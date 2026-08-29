/**
 * docs/ui-mockups/v2/index.html § 05 READER, ported.
 *
 * The same block tree the editor renders, read rather than written. Nothing
 * here is a second rendering path — it reads the same blocks over the same
 * WebSocket, with editing off.
 *
 * The rule that makes the reading tools safe (ADR-009 §9, and RFC-001 §1's
 * own rule about toggle collapse): width, type and scale are VIEW STATE and
 * must never enter the block tree. The failure mode is worse than for a
 * toggle — if font size were model state, changing your own text size would
 * be a collaborative edit that resized the document for everyone on the page.
 */
import { useEffect, useMemo, useRef, useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import { useAuth } from "../auth/AuthContext";
import { getBacklinks, getPage, listPages, type Backlink, type Page } from "../api/pages";
import { useCollabPage } from "../collab/useCollabPage";
import {
  Body, Inspector, Label, Main, Readout, Rule, Screen, StatusBar,
  TopBar, TopicChip, num,
} from "../shell/Chrome";
import { PageCard, RowBars, ViewToggle, readMinutes, type ViewMode } from "../ui";
import { PageTreeRail } from "./PageTreeRail";
import { ReadingBar } from "../shell/ReadingProgress";
import { DocumentOutline, outlineOf } from "./DocumentOutline";
import { ReadBlocks } from "./ReadBlocks";
import { getPageSeries, type PageSeries } from "../api/series";
import { getLinkGraph, graphNeighborhood, type GraphNeighborhood } from "../api/graph";
import { pageLinkTarget, isPageLinkClick, titleSet } from "../collab/pagelinks";
import { ReadingPath, SeriesBanner } from "../ui";

type InspTab = "sidenotes" | "comments" | "backlinks";
type Width = "S" | "M" | "L";
type Face = "SERIF" | "SANS";

const MEASURE: Record<Width, number> = { S: 540, M: 660, L: 780 };

/** Average adult reading speed. Used for "~n min left", nothing else. */
const WORDS_PER_MINUTE = 220;

export function ReaderScreen() {
  const { id } = useParams();
  const { session } = useAuth();
  const actorId = session?.actorId ?? null;
  const navigate = useNavigate();

  const [pages, setPages] = useState<Page[]>([]);
  /**
   * Every live page, as (id, title) — the link resolver's own index.
   *
   * NOT `pages`, which is ListPages and therefore ROOT pages only: resolving
   * against it made every link into a nested page look dangling, and clicking
   * one did nothing. GetLinkGraph is already the "every live page" endpoint,
   * and it is the same set docs.page_links resolves backlinks against — so
   * the click and the graph cannot disagree about whether a link resolves.
   */
  const [allPages, setAllPages] = useState<Array<{ id: string; title: string }>>([]);
  const [page, setPage] = useState<Page | null>(null);
  const [backlinks, setBacklinks] = useState<Backlink[]>([]);
  const [width, setWidth] = useState<Width>("M");
  const [face, setFace] = useState<Face>("SERIF");
  const [pct, setPct] = useState(0);
  const canvasRef = useRef<HTMLElement | null>(null);

  const collab = useCollabPage(id ?? "", actorId ?? "");

  useEffect(() => {
    if (!actorId) return;
    listPages(actorId).then((r) => setPages(r.pages)).catch(() => {});
    getLinkGraph(actorId)
      .then((g) => setAllPages(g.nodes.map((n) => ({ id: n.id, title: n.title }))))
      .catch(() => setAllPages([]));
  }, [actorId]);

  useEffect(() => {
    if (!actorId || !id) { setPage(null); return; }
    getPage(actorId, id).then(setPage).catch(() => setPage(null));
    getBacklinks(actorId, id).then((r) => setBacklinks(r.backlinks)).catch(() => setBacklinks([]));
    getPageSeries(actorId, id).then(setSeries).catch(() => setSeries(null));
    graphNeighborhood(actorId, id).then(setHood).catch(() => setHood(null));
    setLinkNote(null);
  }, [actorId, id]);

  /** Live titles, so a [[link]] to a page nobody has written yet renders as
   *  the unwritten link it is rather than as a broken one. */
  const known = useMemo(() => titleSet(allPages), [allPages]);

  /**
   * A click anywhere in the prose. Page links are a DECORATION over plain
   * text, not stored marks, so resolution happens here rather than at write
   * time — which is what lets a link to a page that does not exist start
   * working the moment someone creates it, with no stored op rewritten.
   */
  function handleProseClick(e: React.MouseEvent) {
    const title = isPageLinkClick(e);
    if (!title) return;
    e.preventDefault();
    const target = pageLinkTarget(e, allPages);
    if (target) { navigate(`/read/${target.id}`); return; }
    setLinkNote(`No page called “${title}” yet.`);
  }

  const outline = useMemo(() => outlineOf(collab.blocks), [collab.blocks]);

  /**
   * Which outline entry you are currently inside.
   *
   * Measured from the rendered blocks rather than tracked as editor state,
   * because in read mode there is no caret to ask. The rule is the one a
   * table of contents has always used: the active entry is the LAST landmark
   * whose top has passed the reading line, not the nearest one — otherwise it
   * flips forward to a heading still below the fold.
   */
  const [activeBlock, setActiveBlock] = useState<string | null>(null);

  /** Which inspector pane. § 05's own three, SIDENOTES first. */
  const [inspTab, setInspTab] = useState<InspTab>("sidenotes");
  /** Grid or list, on the picker. View state, remembered per screen. */
  const [pickView, setPickView] = useState<ViewMode>("grid");
  const [series, setSeries] = useState<PageSeries | null>(null);
  const [hood, setHood] = useState<GraphNeighborhood | null>(null);
  /** What a click on a dangling [[link]] said, so it can say something. */
  const [linkNote, setLinkNote] = useState<string | null>(null);

  /**
   * Tags that share a page with this page's own tags, ranked.
   *
   * A groupBy over the page list this screen already loaded, not a second
   * query and not an algorithm — the same call SearchScreen's facet panel
   * makes, for the same reason: counting co-occurrence is a fold, and pushing
   * a fold to the server buys a round trip and nothing else.
   */
  const coOccurring = useMemo(() => {
    const mine = new Set(page?.tags ?? []);
    if (mine.size === 0) return [] as Array<[string, number]>;
    const counts = new Map<string, number>();
    for (const p of pages) {
      if (p.id === page?.id) continue;
      const tags = p.tags ?? [];
      if (!tags.some((t) => mine.has(t))) continue;
      for (const t of tags) {
        if (mine.has(t)) continue;
        counts.set(t, (counts.get(t) ?? 0) + 1);
      }
    }
    return [...counts].sort((a, b) => b[1] - a[1] || a[0].localeCompare(b[0])).slice(0, 4);
  }, [pages, page]);

  /** How many pages share this page's topic — the "14 pages" § 05 prints. */
  const topicPeers = useMemo(
    () => (page?.topic ? pages.filter((p) => p.topic?.id === page.topic!.id).length : 0),
    [pages, page],
  );

  const words = useMemo(
    () => collab.blocks.reduce((n, b) => n + b.text.trim().split(/\s+/).filter(Boolean).length, 0),
    [collab.blocks],
  );
  const minutes = Math.max(1, Math.round(words / WORDS_PER_MINUTE));
  const minutesLeft = Math.max(0, Math.round((minutes * (100 - pct)) / 100));

  // Progress is read here as well as drawn, because the rail reports it as a
  // number — one measurement, two consumers, rather than two that can drift.
  // The active outline entry falls out of the same scroll event for the same
  // reason: two listeners on one scroll container can disagree by a frame.
  useEffect(() => {
    const el = canvasRef.current;
    if (!el) return;
    const measure = () => {
      const max = el.scrollHeight - el.clientHeight;
      setPct(max <= 0 ? 100 : Math.round((el.scrollTop / max) * 100));

      const line = el.getBoundingClientRect().top + 120;
      let current: string | null = null;
      for (const e of outline) {
        const node = el.querySelector(`[data-block-id="${e.id}"]`);
        if (node && node.getBoundingClientRect().top <= line) current = e.id;
      }
      // null means "above the first landmark" — the title row, which is a
      // real position in the outline rather than a missing one.
      setActiveBlock(current);
    };
    measure();
    el.addEventListener("scroll", measure, { passive: true });
    return () => el.removeEventListener("scroll", measure);
  }, [page, outline]);

  if (!id) {
    return (
      <Screen>
        <TopBar
          crumb={<>read</>}
          readouts={
            <>
              <Readout k="PAGES" v={num(pages.length)} />
              <Readout k="READ TIME" v={`~${num(readMinutes(pages.reduce((n, p) => n + (p.word_count ?? 0), 0)) ?? 0)} min`} />
            </>
          }
        />
        <Body>
          <PageTreeRail actorId={actorId ?? ""} />
          <Main style={{ padding: "28px 34px", overflow: "hidden" }}>
            <div style={{ display: "flex", alignItems: "baseline", gap: 12, marginBottom: 16 }}>
              <h1 className="h1" style={{ fontSize: 22 }}>Read</h1>
              <span className="mono" style={{ fontSize: 11, color: "#585550" }}>
                {num(pages.length)} pages · width and type are yours, never the document's
              </span>
              <div style={{ flex: 1 }} />
              <Link to="/series" className="chip" style={{ textDecoration: "none" }}>SERIES →</Link>
              <ViewToggle mode={pickView} onChange={setPickView} />
            </div>

            <div style={{ flex: 1, minHeight: 0, overflowY: "auto" }}>
              {pages.length === 0 && (
                <div style={{ fontSize: 12.5, lineHeight: 1.7, color: "#585550", maxWidth: 560 }}>
                  Nothing to read yet. A blank workspace is a correct state, not an empty one.
                </div>
              )}
              {pickView === "grid" ? (
                <div style={{ display: "grid", gridTemplateColumns: "repeat(3, 1fr)", gap: 12 }}>
                  {pages.map((p, i) => (
                    <PageCard
                      key={p.id}
                      to={`/read/${p.id}`}
                      title={p.title}
                      topicName={p.topic?.name}
                      colorKey={p.topic?.color_key}
                      delay={Math.min(i, 12) * 0.03}
                      excerpt={(p.tags ?? []).slice(0, 4).join(" · ")}
                      meta={
                        <>
                          <span>{num(p.block_count)} blocks</span>
                          <span>{readMinutes(p.word_count) ? `~${readMinutes(p.word_count)} min` : "empty"}</span>
                        </>
                      }
                    />
                  ))}
                </div>
              ) : (
                <div style={{ display: "flex", flexDirection: "column" }}>
                  {pages.map((p) => (
                    <div key={p.id} className="srow" onClick={() => navigate(`/read/${p.id}`)}>
                      <RowBars colorKey={p.topic?.color_key} status="ok" />
                      <div style={{ flex: 1, minWidth: 0 }}>
                        <div className="srow-t">{p.title}</div>
                        <div className="tgrow" style={{ marginTop: 4 }}>
                          {(p.tags ?? []).slice(0, 5).map((t) => <span key={t} className="tg">{t}</span>)}
                        </div>
                      </div>
                      {p.topic && <TopicChip name={p.topic.name} colorKey={p.topic.color_key} small />}
                      <span className="mono srow-n">
                        {readMinutes(p.word_count) ? `~${readMinutes(p.word_count)} min` : "empty"}
                      </span>
                    </div>
                  ))}
                </div>
              )}
            </div>
          </Main>
        </Body>
        <StatusBar
          route="/read"
          mechanism="view state, never model state"
          state={`${num(pages.length)} pages`}
          healthy
        />
      </Screen>
    );
  }

  return (
    <Screen>
      <TopBar
        crumb={<>read / <b>{page?.title ?? "…"}</b></>}
        right={
          // Read and write are two views of ONE page, and the switch between
          // them belongs where you are rather than three clicks away in a
          // rail. § 05's own status line already claims the tree is untouched
          // by view state; this is that claim made usable.
          <Link to={`/pages/${id}`} className="btn" style={{ textDecoration: "none" }}>
            EDIT
            <div className="brk-tl" /><div className="brk-br" />
          </Link>
        }
        readouts={
          <>
            {/* § 05 puts the reading tools IN the top bar, as two labelled
                chip groups — not in a section strip. They are the only
                controls on the screen, and a strip holding two of them is a
                strip that exists to hold a strip. */}
            <div style={{ display: "flex", alignItems: "center", gap: 7 }}>
              <span className="rd-k">WIDTH</span>
              {(["S", "M", "L"] as Width[]).map((w) => (
                <span
                  key={w}
                  className={`chip${width === w ? " chip-e" : ""}`}
                  style={{ padding: "2px 7px", cursor: "pointer" }}
                  onClick={() => setWidth(w)}
                >
                  {w}
                </span>
              ))}
            </div>
            <div style={{ display: "flex", alignItems: "center", gap: 7 }}>
              <span className="rd-k">TYPE</span>
              {(["SERIF", "SANS"] as Face[]).map((f) => (
                <span
                  key={f}
                  className={`chip${face === f ? " chip-e" : ""}`}
                  style={{ padding: "2px 7px", cursor: "pointer" }}
                  onClick={() => setFace(f)}
                >
                  {f}
                </span>
              ))}
            </div>
          </>
        }
      />

      {/* § 05 draws the reading rule as a full-width bar under the top bar
          rather than as an overlay inside the column. It reflows nothing, and
          it is the reason this design has no scrollbars. */}
      <ReadingBar pct={pct} />

      {/* § 10d's banner, on every part. The single most common thing a reader
          of part 4 wants is part 5, and making them find it in a rail is
          making them find it. */}
      {series?.membership === "member" && (
        <SeriesBanner
          seriesTitle={series.series_title}
          seriesTo={`/series/${series.series_page_id}`}
          number={series.number}
          total={series.parts.length}
          prev={series.number > 1
            ? { title: series.parts[series.number - 2].title, to: `/read/${series.parts[series.number - 2].page_id}` }
            : null}
          next={series.number < series.parts.length
            ? { title: series.parts[series.number].title, to: `/read/${series.parts[series.number].page_id}` }
            : null}
        />
      )}
      {series?.membership === "leader" && (
        <div className="sbanner">
          <span className="lbl">SERIES PAGE</span>
          <span className="sbanner-of">
            This page leads <b>{num(series.parts.length)}</b> parts
          </span>
          <div style={{ flex: 1 }} />
          <Link to={`/series/${series.series_page_id}`} className="sbanner-nav sbanner-nav-next">
            all parts →
          </Link>
          <Link to={`/read/${series.parts[0].page_id}`} className="sbanner-nav sbanner-nav-next">
            start with {series.parts[0].title} →
          </Link>
        </div>
      )}

      <Body>
        <div className="rail">
          {/* The editor's own IN THIS PAGE section, not a second one. Reading
              and writing ask the identical question of the document — "where
              am I in it" — and two lists answering it is two lists that
              drift. */}
          <DocumentOutline
            first
            title={page?.title ?? "…"}
            onJumpTop={() => canvasRef.current?.scrollTo({ top: 0, behavior: "smooth" })}
            blocks={collab.blocks}
            activeId={activeBlock}
            onJump={(id) => document.querySelector(`[data-block-id="${id}"]`)
              ?.scrollIntoView({ behavior: "smooth", block: "start" })}
          />

          <div className="wal">
            <Label>READING</Label>
            <div style={{ display: "flex", alignItems: "center", gap: 9 }}>
              <div style={{ flex: 1, height: 3, background: "rgba(255,255,255,.08)" }}>
                <div style={{ width: `${pct}%`, height: "100%", background: "#E8873C" }} />
              </div>
              <span className="mono" style={{ fontSize: 10.5, color: "#8C8880" }}>{pct}%</span>
            </div>
            <span className="mono" style={{ fontSize: 10.5, color: "#585550" }}>
              ~{minutesLeft} min left · {num(words)} words
            </span>
          </div>
        </div>

        <main className="canvas" ref={canvasRef}>
          <article style={{ width: MEASURE[width], maxWidth: "calc(100% - 64px)" }}>
            <div className="mono" style={{
              fontSize: 9, fontWeight: 600, letterSpacing: ".2em", color: "#E8873C", marginBottom: 9,
            }}>
              {page?.topic?.name.toUpperCase() ?? "UNTOPICED"}
            </div>
            <h1 className="h1" style={{ fontSize: 40, lineHeight: 1.13, margin: "12px 0 10px" }}>
              {page?.title ?? "…"}
            </h1>
            <div className="dek">
              {minutes} min read · {num(collab.blocks.length)} blocks · {num(words)} words
            </div>

            <div className="tgrow" style={{ marginBottom: 26 }}>
              {page?.topic
                ? <TopicChip name={page.topic.name} colorKey={page.topic.color_key} />
                : <span className="chip">UNTOPICED</span>}
              {(page?.tags ?? []).map((t) => <span key={t} className="tg">{t}</span>)}
            </div>

            {/* Read-only: the same blocks, no contenteditable, no ops. The
                typeface is view state and is applied here rather than stored,
                so it can never reach the document. */}
            {linkNote && (
              <div className="mono" style={{
                margin: "0 0 14px", padding: "6px 9px", fontSize: 10,
                border: "1px solid rgba(224,163,78,.28)", background: "rgba(224,163,78,.05)",
                color: "#E0A34E",
              }}>
                ◌ {linkNote} A link to a page you have not written is how you write forward —
                it starts working the moment the page exists.
              </div>
            )}
            <div
              onClick={handleProseClick}
              style={{
                fontFamily: face === "SERIF" ? "Spectral, serif" : "Archivo, system-ui, sans-serif",
                fontSize: face === "SERIF" ? 18 : 16,
                lineHeight: 1.75,
                color: "#D2CFC8",
              }}
            >
              <ReadBlocks blocks={collab.blocks} known={known} />
            </div>
          </article>
        </main>

        <Inspector
          tabs={[
            { id: "sidenotes", label: "SIDENOTES" },
            { id: "comments", label: <>COMMENTS <span style={{ color: "#A98CE8" }}>0</span></> },
            { id: "backlinks", label: "BACKLINKS" },
          ]}
          active={inspTab}
          onSelect={(t) => setInspTab(t as InspTab)}
          width={312}
        >
          {inspTab === "sidenotes" && (
            <>
              <Label>SIDENOTES</Label>
              <div style={{ fontSize: 11, lineHeight: 1.6, color: "#585550" }}>
                A footnote is a block kind, and whether it renders in the margin or inline is a
                rendering choice over the same tree — driven by available width, never by an
                edit. Nothing in this corpus defines one yet.
              </div>

              <Rule />
              <Label>BACKLINKS · {num(backlinks.length)}</Label>
              <div className="mono" style={{ fontSize: 11, lineHeight: 1.9, color: "#8C8880" }}>
                {backlinks.length === 0 && <span style={{ color: "#585550" }}>Nothing links here yet.</span>}
                {backlinks.map((b) => (
                  <div
                    key={b.from_page}
                    style={{ cursor: "pointer", color: b.from_page_deleted ? "#585550" : undefined }}
                    onClick={() => navigate(`/read/${b.from_page}`)}
                  >
                    {b.from_page_title}
                  </div>
                ))}
              </div>

              <Rule />
              <Label>TOPIC · ONE PER PAGE</Label>
              <div className="tgrow">
                {page?.topic
                  ? <TopicChip name={page.topic.name} colorKey={page.topic.color_key} />
                  : <span className="chip">UNTOPICED</span>}
                <span className="mono" style={{ fontSize: 10, color: "#585550", marginLeft: "auto" }}>
                  {num(topicPeers)} pages
                </span>
              </div>
              <div style={{ fontSize: 11, lineHeight: 1.6, color: "#585550" }}>
                Owned, singular, indexed. Changing it moves this page's node between clusters on{" "}
                <span className="mono" style={{ color: "#8C8880" }}>/graph</span> and re-scopes
                which pages it sits near.
              </div>

              <Label style={{ marginTop: 2 }}>TAGS · {num((page?.tags ?? []).length)}</Label>
              <div className="tgrow">
                {(page?.tags ?? []).map((t) => <span key={t} className="tg">{t}</span>)}
                {(page?.tags ?? []).length === 0 && (
                  <span style={{ fontSize: 11.5, color: "#585550" }}>None.</span>
                )}
              </div>

              <Rule />
              {/* "Read these, in this order", over the real dependency layers —
                  everything that reaches this page by following links FORWARD.
                  Not the shortest path, which answers how two pages are
                  connected, and not similar tags, which give you neighbours
                  rather than prerequisites: only the arrows carry order. */}
              <Label>READ THESE FIRST · {num(Math.max((hood?.reading_path.length ?? 1) - 1, 0))}</Label>
              <ReadingPath
                steps={hood?.reading_path ?? []}
                hrefFor={(pid) => `/read/${pid}`}
              />

              <Rule />
              <Label>CO-OCCURRING TAGS</Label>
              <div style={{ display: "flex", flexDirection: "column", gap: 7 }}>
                {coOccurring.length === 0 && (
                  <span style={{ fontSize: 11.5, color: "#585550" }}>
                    None — this page's tags appear nowhere else.
                  </span>
                )}
                {coOccurring.map(([tag, n]) => (
                  <div key={tag} style={{ display: "flex", alignItems: "center", gap: 9 }}>
                    <span className="tg" style={{ width: 82, justifyContent: "flex-start" }}>{tag}</span>
                    <div style={{ flex: 1, height: 3, background: "rgba(255,255,255,.06)" }}>
                      <div style={{
                        width: `${Math.round((n / coOccurring[0][1]) * 100)}%`,
                        height: "100%",
                        background: "#7AA8E8",
                      }} />
                    </div>
                    <span className="mono" style={{ fontSize: 9.5, color: "#585550", width: 16, textAlign: "right" }}>
                      {n}
                    </span>
                  </div>
                ))}
              </div>
            </>
          )}

          {inspTab === "comments" && (
            <div style={{ fontSize: 11.5, lineHeight: 1.7, color: "#585550" }}>
              Comments are not built. A comment anchors to a range the way a mark does, and the
              anchor has to survive every edit under it — which is a second use of RFC-002's
              anchor machinery, not a text field beside the page. Drawn and reporting zero rather
              than hidden, so the gap is visible.
            </div>
          )}

          {inspTab === "backlinks" && (
            <>
              <Label>BACKLINKS · {num(backlinks.length)}</Label>
              {backlinks.length === 0 && (
                <div style={{ fontSize: 11.5, color: "#585550", lineHeight: 1.6 }}>
                  Nothing links here yet. This is the reverse index over docs.page_links, not a
                  text search — a page appears only once its [[link]] actually resolved.
                </div>
              )}
              {backlinks.map((b) => (
                <div
                  key={b.from_page}
                  style={{ fontSize: 12.5, color: b.from_page_deleted ? "#585550" : "#D2CFC8", cursor: "pointer" }}
                  onClick={() => navigate(`/read/${b.from_page}`)}
                >
                  {b.from_page_title}
                  {b.from_page_deleted && (
                    <span className="mono" style={{ fontSize: 9, color: "#E0A34E", marginLeft: 6 }}>DELETING</span>
                  )}
                </div>
              ))}
            </>
          )}
        </Inspector>
      </Body>

      <StatusBar
        route={`/read/${id}`}
        mechanism="width and type are view state, never model state"
        state={`${pct}% read · ~${minutesLeft} min left`}
        healthy
      />
    </Screen>
  );
}

export default ReaderScreen;
