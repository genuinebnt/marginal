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
  Body, Inspector, Label, Readout, Rule, Screen, StatusBar, SubBar, SubItem,
  TopBar, TopicChip, num,
} from "../shell/Chrome";
import { ReadingProgress } from "../shell/ReadingProgress";

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

  const headings = collab.blocks.filter((b) => b.kind.tag === "heading");

  const words = useMemo(
    () => collab.blocks.reduce((n, b) => n + b.text.trim().split(/\s+/).filter(Boolean).length, 0),
    [collab.blocks],
  );
  const minutes = Math.max(1, Math.round(words / WORDS_PER_MINUTE));
  const minutesLeft = Math.max(0, Math.round((minutes * (100 - pct)) / 100));

  // Progress is read here as well as drawn, because the rail reports it as a
  // number — one measurement, two consumers, rather than two that can drift.
  useEffect(() => {
    const el = canvasRef.current;
    if (!el) return;
    const measure = () => {
      const max = el.scrollHeight - el.clientHeight;
      setPct(max <= 0 ? 100 : Math.round((el.scrollTop / max) * 100));
    };
    measure();
    el.addEventListener("scroll", measure, { passive: true });
    return () => el.removeEventListener("scroll", measure);
  }, [page]);

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
            <Readout k="READ" v={`${pct}%`} tone={pct >= 100 ? "#3FCFA8" : undefined} />
            <Readout k="LEFT" v={`~${minutesLeft} min`} />
          </>
        }
      />

      <SubBar>
        <SubItem>WIDTH</SubItem>
        {(["S", "M", "L"] as Width[]).map((w) => (
          <SubItem key={w} on={width === w} onClick={() => setWidth(w)}>{w}</SubItem>
        ))}
        <div style={{ width: 1, height: 18, background: "rgba(255,255,255,.09)", margin: "0 8px" }} />
        <SubItem>TYPE</SubItem>
        {(["SERIF", "SANS"] as Face[]).map((f) => (
          <SubItem key={f} on={face === f} onClick={() => setFace(f)}>{f}</SubItem>
        ))}
        <div style={{ flex: 1 }} />
        <SubItem tone="#585550">width and type are yours, not the document's</SubItem>
      </SubBar>

      <Body>
        <div className="rail">
          <div className="rail-h">OUTLINE<div /><span style={{ color: "#585550" }}>{headings.length}</span></div>
          <div style={{ display: "flex", flexDirection: "column", gap: 1, padding: "0 6px" }}>
            <div className="oi oi-h1 oi-on">
              <span className="oi-m">H1</span>
              <span className="oi-t">{page?.title ?? "…"}</span>
            </div>
            {headings.map((b) => {
              const level = (b.kind as { level?: number }).level ?? 1;
              return (
                <div
                  key={b.id}
                  className={`oi oi-h${level}`}
                  onClick={() => document.querySelector(`[data-block-id="${b.id}"]`)
                    ?.scrollIntoView({ behavior: "smooth", block: "center" })}
                >
                  <span className="oi-m">H{level}</span>
                  <span className="oi-t">{b.text || "(empty)"}</span>
                </div>
              );
            })}
          </div>

          <div style={{ flex: 1, minHeight: 0 }} />

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
          <ReadingProgress target={canvasRef} />
          <article style={{ width: MEASURE[width], maxWidth: "calc(100% - 64px)" }}>
            <div className="mono" style={{
              fontSize: 9, fontWeight: 600, letterSpacing: ".2em", color: "#E8873C", marginBottom: 9,
            }}>
              {page?.topic?.name.toUpperCase() ?? "UNTOPICED"}
            </div>
            <h1 className="h1" style={{ fontSize: 40, lineHeight: 1.14, margin: "0 0 8px" }}>
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
            { id: "backlinks", label: <>BACKLINKS {backlinks.length > 0 && <span style={{ color: "#E8873C" }}>{backlinks.length}</span>}</> },
          ]}
          active="backlinks"
        >
          <Label>BACKLINKS · {num(backlinks.length)}</Label>
          {backlinks.length === 0 && (
            <div style={{ fontSize: 11.5, color: "#585550", lineHeight: 1.6 }}>
              Nothing links here yet.
            </div>
          )}
          {backlinks.map((b) => (
            <div
              key={b.source_page_id}
              style={{ fontSize: 12.5, color: "#D2CFC8", cursor: "pointer" }}
              onClick={() => navigate(`/read/${b.source_page_id}`)}
            >
              {b.source_title}
            </div>
          ))}

          <Rule />
          <Label>TOPIC · ONE PER PAGE</Label>
          <div className="tgrow">
            {page?.topic
              ? <TopicChip name={page.topic.name} colorKey={page.topic.color_key} />
              : <span className="chip">UNTOPICED</span>}
          </div>
          <div style={{ fontSize: 11, lineHeight: 1.6, color: "#585550" }}>
            Owned, singular, indexed. Changing it moves this page's node between clusters on{" "}
            <span className="mono" style={{ color: "#8C8880" }}>/graph</span> and re-scopes which
            pages it sits near.
          </div>

          <Rule />
          <Label>TAGS · {num((page?.tags ?? []).length)}</Label>
          <div className="tgrow">
            {(page?.tags ?? []).map((t) => <span key={t} className="tg">{t}</span>)}
            {(page?.tags ?? []).length === 0 && (
              <span style={{ fontSize: 11.5, color: "#585550" }}>None.</span>
            )}
          </div>

          <Rule />
          <Label>SIDENOTES</Label>
          <div style={{ fontSize: 11, lineHeight: 1.6, color: "#585550" }}>
            A footnote is a block kind, and whether it renders in the margin or inline is a
            rendering choice over the same tree — driven by available width, never by an edit.
            Nothing in this corpus defines one yet.
          </div>
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
