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
import { useNavigate, useParams } from "react-router-dom";
import { useAuth } from "../auth/AuthContext";
import { getBacklinks, getPage, listPages, type Backlink, type Page } from "../api/pages";
import { useCollabPage } from "../collab/useCollabPage";
import {
  Body, Inspector, Label, Rule, Screen, StatusBar,
  TopBar, TopicChip, num,
} from "../shell/Chrome";
import { ReadingBar } from "../shell/ReadingProgress";
import { DocumentOutline, outlineOf } from "./DocumentOutline";

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
  }, [actorId]);

  useEffect(() => {
    if (!actorId || !id) { setPage(null); return; }
    getPage(actorId, id).then(setPage).catch(() => setPage(null));
    getBacklinks(actorId, id).then((r) => setBacklinks(r.backlinks)).catch(() => setBacklinks([]));
  }, [actorId, id]);

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
        <TopBar crumb={<>read</>} />
        <Body>
          <div className="rail">
            <div className="rail-h">PAGES<div /><span style={{ color: "#585550" }}>{pages.length}</span></div>
            <div style={{ display: "flex", flexDirection: "column", gap: 1, padding: "0 8px", overflowY: "auto" }}>
              {pages.map((p) => (
                <div key={p.id} className="tr" style={{ cursor: "pointer" }}
                     onClick={() => navigate(`/read/${p.id}`)}>
                  {p.topic
                    ? <span className="tr-topic" style={{ background: `var(--topic-${p.topic.color_key})` }} />
                    : <span className="tr-topic tr-topic-none" />}
                  <span className="tr-t">{p.title}</span>
                </div>
              ))}
            </div>
          </div>
          <div style={{ flex: 1, display: "grid", placeItems: "center", padding: 40 }}>
            <div style={{ maxWidth: 520, fontSize: 12.5, lineHeight: 1.7, color: "#585550" }}>
              Reading is per page. Width, typeface and scale are yours and are stored per user —
              they are view state, so changing them never touches the document or anyone else's
              view of it.
            </div>
          </div>
        </Body>
        <StatusBar route="/read" mechanism="view state, never model state" state="no page selected" healthy />
      </Screen>
    );
  }

  return (
    <Screen>
      <TopBar
        crumb={<>read / <b>{page?.title ?? "…"}</b></>}
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
            <div style={{
              fontFamily: face === "SERIF" ? "Spectral, serif" : "Archivo, system-ui, sans-serif",
              fontSize: face === "SERIF" ? 18 : 16,
              lineHeight: 1.75,
              color: "#D2CFC8",
            }}>
              {collab.blocks.filter((b) => !b.parent || b.kind.tag === "list_item").map((b) => {
                const tag = b.kind.tag;
                if (tag === "divider") return <hr key={b.id} className="block-divider" />;
                if (tag === "heading") {
                  const level = (b.kind as { level?: number }).level ?? 1;
                  const size = level === 1 ? 27 : level === 2 ? 22 : 18;
                  return (
                    <div key={b.id} data-block-id={b.id} style={{
                      fontFamily: "Spectral, serif", fontWeight: 500, fontSize: size,
                      letterSpacing: "-.015em", color: "#EFEDE7", margin: "26px 0 12px",
                    }}>
                      {b.text}
                    </div>
                  );
                }
                if (tag === "code_block") {
                  return (
                    <div key={b.id} data-block-id={b.id} className="blk-code" style={{ margin: "0 0 16px" }}>
                      <div className="blk-code-h">
                        <span className="mono lang">
                          {((b.kind as { language?: string }).language || "plain text").toUpperCase()}
                        </span>
                      </div>
                      <pre>{b.text}</pre>
                    </div>
                  );
                }
                if (tag === "list_item") {
                  return (
                    <div key={b.id} data-block-id={b.id} className="li-row">
                      <span className="li-marker">•</span>
                      <div className="li-body">{b.text}</div>
                    </div>
                  );
                }
                return (
                  <p key={b.id} data-block-id={b.id} style={{ margin: "0 0 16px" }}>{b.text}</p>
                );
              })}
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
