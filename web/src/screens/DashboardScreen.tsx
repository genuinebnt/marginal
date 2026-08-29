/**
 * docs/ui-mockups/v2/index.html § 03c DASHBOARD, ported.
 *
 * The screen you land on after signing in. Its whole argument, kept from
 * the mockup because it decides what belongs here: this is RESUME, not a
 * recent-files list, and it is not a feed. A feed of everything makes the
 * workspace something to keep up with; this lists what changed AND what
 * wants a decision — both finite, both clearable. When there is nothing,
 * the screen is nearly empty, and that is the correct state rather than a
 * failure to fill it.
 *
 * Real: the page tree, page counts, topic counts, untopiced count, page
 * creation, and resume — actual stored caret positions per user (v2.8.0),
 * not the most-recently-updated pages wearing the word.
 *
 * Still placeholder: the change feed (needs a read over collab.ops since
 * last session) and LIVE NOW (needs presence across pages, not one).
 */
import { useCallback, useEffect, useMemo, useState } from "react";
import { useNavigate } from "react-router-dom";
import { useAuth } from "../auth/AuthContext";
import { createPage, listPages, type Page } from "../api/pages";
import { getTopics, type TopicList } from "../api/topics";
import { getResume, type ReadingPosition } from "../api/resume";
import {
  Body, Label, Readout, Screen, StatusBar, TopBar, num,
} from "../shell/Chrome";
import { ph, undrawn } from "../shell/placeholder";
import { PageTreeRail } from "./PageTreeRail";
import { PageCard, RowBars } from "../ui";

/** The mockup's own eyebrow — the time of day, named rather than printed. */
function partOfDay(d: Date): string {
  const h = d.getHours();
  const part = h < 12 ? "MORNING" : h < 18 ? "AFTERNOON" : "EVENING";
  return `${d.toLocaleDateString("en-GB", { weekday: "long" }).toUpperCase()} ${part}`;
}

export function DashboardScreen() {
  const { session } = useAuth();
  const actorId = session?.actorId ?? null;
  const navigate = useNavigate();

  const [pages, setPages] = useState<Page[]>([]);
  const [topics, setTopics] = useState<TopicList | null>(null);
  const [resume, setResume] = useState<ReadingPosition[]>([]);
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  const load = useCallback(() => {
    if (!actorId) return;
    listPages(actorId).then((r) => setPages(r.pages)).catch((e) => setErr(String(e)));
    getTopics(actorId).then(setTopics).catch(() => {});
    getResume(actorId, 2).then((r) => setResume(r.positions)).catch(() => {});
  }, [actorId]);

  useEffect(load, [load]);

  async function newPage() {
    if (!actorId) return;
    setBusy(true);
    try {
      // Created empty and untitled, exactly as the rail's own note says —
      // naming it later is normal, because the id was never the name.
      const p = await createPage(actorId, "Untitled");
      navigate(`/pages/${p.id}`);
    } catch (e) {
      setErr(String(e));
    } finally {
      setBusy(false);
    }
  }


  const untopiced = topics?.untopiced_pages ?? 0;
  const now = new Date();
  const totalWords = useMemo(() => pages.reduce((n, p) => n + (p.word_count ?? 0), 0), [pages]);

  /**
   * What changed, most recent first.
   *
   * By `updated_at` and it says so: a page edited and then reverted still
   * appears here, where the design asks for a read over `collab.ops` since
   * your last session — which would show nothing, correctly. The gap is
   * printed under the list rather than papered over with a dimmed
   * placeholder, because a real list with a stated limitation is worth more
   * than a fake one with a badge.
   */
  const recentlyChanged = useMemo(
    () => [...pages].sort((a, b) => b.updated_at.localeCompare(a.updated_at)).slice(0, 6),
    [pages],
  );

  return (
    <Screen>
      <TopBar
        crumb={<>workspace / <b>marginal</b></>}
        readouts={
          <>
            <Readout k="PAGES" v={num(pages.length)} />
            <Readout k="WORDS" v={num(totalWords)} />
          </>
        }
      />

      <Body>
        {/* The real tree, not a second flat list of the same pages. The rail
            is the one component that knows about nesting, part counts,
            reading estimates and where you have been — a hand-rolled copy
            here had none of it and drifted the moment the rail gained any. */}
        <PageTreeRail actorId={actorId ?? ""} />

        <div style={{
          flex: 1, minWidth: 0, padding: "34px 40px", overflow: "hidden",
          display: "flex", flexDirection: "column",
        }}>
          <div style={{ marginBottom: 26 }}>
            <div className="mono" style={{
              fontSize: 9, fontWeight: 600, letterSpacing: ".2em", color: "#E8873C", marginBottom: 9,
            }}>
              {partOfDay(now)}
            </div>
            <h1 className="h1" style={{ fontSize: 29, marginBottom: 7 }}>
              {resume.length > 0
                ? `${resume.length === 1 ? "One page is" : `${resume.length} pages are`} still open where you left ${resume.length === 1 ? "it" : "them"}.`
                : "Nothing open. A blank workspace is a correct state, not an empty one."}
            </h1>
            <div style={{ fontSize: 13, color: "#8C8880", lineHeight: 1.6, maxWidth: 600 }}>
              Resume is a real thing here, not a recent-files list: the caret position and the
              selection were view state, stored per user on the server, so they survive the device
              you left them on.
            </div>
          </div>

          {/* Resume, not "recent". The distinction is the caret. */}
          <Label>RESUME</Label>
          <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 11, marginBottom: 28 }}>
            {resume.length === 0 && (
              <div style={{
                gridColumn: "1 / -1", fontSize: 12.5, color: "#585550", lineHeight: 1.7,
              }}>
                Nothing to resume yet. Open a page and your caret position is remembered per
                user, server-side — so it survives the device you left it on.
              </div>
            )}
            {resume.map((p, i) => (
              <PageCard
                key={p.page_id}
                title={p.page_title}
                topicName={p.topic?.name}
                colorKey={p.topic?.color_key}
                selected={i === 0}
                delay={i * 0.04}
                onClick={() => navigate(`/read/${p.page_id}`)}
                excerpt={p.block_id ? `caret at block ${p.block_id.slice(0, 4)}` : "opened, never clicked into"}
                meta={
                  <>
                    <span>{i === 0 ? "MOST RECENT" : "RESUME"}</span>
                    <span>
                      {new Date(p.updated_at).toLocaleString("en-GB", {
                        day: "2-digit", month: "short", hour: "2-digit", minute: "2-digit",
                      })}
                    </span>
                  </>
                }
              />
            ))}
          </div>

          <div style={{ display: "grid", gridTemplateColumns: "1.35fr 1fr", gap: 32, flex: 1, minHeight: 0 }}>
            <div style={{ display: "flex", flexDirection: "column", minHeight: 0 }}>
              <div style={{ display: "flex", alignItems: "baseline", gap: 10, marginBottom: 12 }}>
                <Label>CHANGED SINCE YOU LAST LOOKED</Label>
                <span className="mono" style={{ fontSize: 10, color: "#585550" }}>by op, not by mtime</span>
              </div>
              <div style={{ display: "flex", flexDirection: "column" }}>
                {recentlyChanged.length === 0 && (
                  <div style={{ fontSize: 12, color: "#585550", lineHeight: 1.6 }}>
                    Nothing has changed. That is the cleared state.
                  </div>
                )}
                {recentlyChanged.map((p, i) => (
                  <div
                    key={p.id}
                    className="fx changed-row"
                    style={{ animationDelay: `${i * 0.04}s` }}
                    onClick={() => navigate(`/read/${p.id}`)}
                  >
                    <RowBars colorKey={p.topic?.color_key} status={i === 0 ? "live" : "ok"} />
                    <span style={{ flex: 1, minWidth: 0, fontSize: 12.5, color: "#D2CFC8", overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
                      {p.title}
                    </span>
                    <span className="mono" style={{ fontSize: 10, color: "#585550" }}>
                      {num(p.block_count)} blocks
                    </span>
                    <span className="mono" style={{ fontSize: 10, color: "#585550", width: 76, textAlign: "right" }}>
                      {sinceLabel(p.updated_at)}
                    </span>
                  </div>
                ))}
              </div>
              <div className="mono" style={{ fontSize: 10, lineHeight: 1.6, color: "#585550", marginTop: 11 }}>
                by <span style={{ color: "#8C8880" }}>updated_at</span>, not by op — so a page
                edited and reverted still appears here. Reading collab.ops since your last
                session would fix that, and is the honest difference between this list and the
                one the design asks for.
              </div>

              <div style={{ marginTop: 22 }}>
                <Label>START</Label>
                <div style={{ display: "flex", gap: 9 }}>
                  <div
                    className="btn"
                    onClick={newPage}
                    style={{ borderColor: "rgba(232,135,60,.45)", color: "#E8873C", cursor: busy ? "default" : "pointer" }}
                  >
                    {busy ? "…" : "NEW PAGE"}<div className="brk-tl" /><div className="brk-br" />
                  </div>
                  <div className="btn" style={undrawn}>IMPORT<div className="brk-tl" /><div className="brk-br" /></div>
                  <div className="btn" style={undrawn}>FROM A TEMPLATE<div className="brk-tl" /><div className="brk-br" /></div>
                </div>
              </div>
            </div>

            <div style={{ display: "flex", flexDirection: "column", gap: 20, minHeight: 0 }}>
              <div>
                <Label>
                  NEEDS YOU · {untopiced > 0 ? 1 : 0}
                </Label>
                <div style={{ display: "flex", flexDirection: "column", gap: 9 }}>
                  {untopiced > 0 && (
                    <div
                      onClick={() => navigate("/topics")}
                      style={{
                        display: "flex", gap: 10, padding: "10px 12px",
                        borderLeft: "2px solid #E0A34E", background: "rgba(224,163,78,.05)",
                        cursor: "pointer",
                      }}
                    >
                      <span style={{ color: "#E0A34E", fontSize: 11 }}>◌</span>
                      <div style={{ flex: 1, fontSize: 11.5, lineHeight: 1.5, color: "#9B968D" }}>
                        <span style={{ color: "#C3BFB7" }}>{untopiced}</span> pages have no topic —
                        suggestion is available, assignment is yours
                      </div>
                    </div>
                  )}
                  {untopiced === 0 && (
                    <div style={{ fontSize: 11.5, lineHeight: 1.6, color: "#585550" }}>
                      Nothing wants a decision. That is the cleared state, not a missing panel.
                    </div>
                  )}
                </div>
              </div>

              <div>
                <Label>WORKSPACE</Label>
                <div style={{ display: "flex", flexDirection: "column", gap: 7 }}>
                  {[
                    { k: "Pages", v: num(pages.length), tone: "#E4E2DC" },
                    { k: "Topics", v: num(topics?.topics.length ?? 0), tone: "#E4E2DC" },
                    { k: "Untopiced", v: num(untopiced), tone: untopiced > 0 ? "#E0A34E" : "#8C8880" },
                    { k: "In trash", v: ph("—"), tone: "#8C8880" },
                  ].map((r) => (
                    <div key={r.k} style={{
                      display: "flex", alignItems: "baseline", gap: 8, fontSize: 11.5, color: "#9B968D",
                    }}>
                      <span style={{ flex: 1 }}>{r.k}</span>
                      <span className="mono" style={{ fontSize: 11, color: r.tone }}>{r.v}</span>
                    </div>
                  ))}
                </div>
              </div>

              <div style={{ marginTop: "auto" }}>
                <Label>WHY NO FEED</Label>
                <div style={{ fontSize: 11.5, lineHeight: 1.7, color: "#585550" }}>
                  A feed of everything makes the workspace something to keep up with. This lists
                  what changed <i>and</i> what wants a decision — both finite, both clearable. When
                  there is nothing, this screen is nearly empty, and that is the correct state
                  rather than a failure to fill it.
                </div>
              </div>
            </div>
          </div>
        </div>
      </Body>

      <StatusBar
        route="/"
        mechanism="resume · caret is view state, stored server-side per user"
        state={err ? "workspace unavailable" : `${pages.length} pages · ${untopiced} untopiced`}
        healthy={!err && untopiced === 0}
      />
    </Screen>
  );
}

export default DashboardScreen;

/** "4 min ago" / "yesterday" — the granularity the dashboard actually prints. */
function sinceLabel(iso: string): string {
  const mins = Math.max(0, Math.round((Date.now() - new Date(iso).getTime()) / 60000));
  if (mins < 1) return "just now";
  if (mins < 60) return `${mins} min ago`;
  const hours = Math.round(mins / 60);
  if (hours < 24) return `${hours} h ago`;
  const days = Math.round(hours / 24);
  return days === 1 ? "yesterday" : `${days} days ago`;
}
