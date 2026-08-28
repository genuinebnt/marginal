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
 * Real: the page tree, page counts, topic counts, untopiced count, and
 * page creation. Placeholder: caret positions (resume needs per-user view
 * state stored server-side, which has no endpoint), the change feed (needs
 * a read over collab.ops since last session), and the "needs you" queue
 * (needs notifications + the facts DAG).
 */
import { useCallback, useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { useAuth } from "../auth/AuthContext";
import { createPage, listPages, type Page } from "../api/pages";
import { getTopics, type TopicList } from "../api/topics";
import {
  Body, Label, Readout, Screen, StatusBar, TopBar, TopicChip, num,
} from "../shell/Chrome";
import { ph, PlaceholderNote, undrawn } from "../shell/placeholder";

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
  const [busy, setBusy] = useState(false);
  const [err, setErr] = useState<string | null>(null);

  const load = useCallback(() => {
    if (!actorId) return;
    listPages(actorId).then((r) => setPages(r.pages)).catch((e) => setErr(String(e)));
    getTopics(actorId).then(setTopics).catch(() => {});
  }, [actorId]);

  useEffect(load, [load]);

  async function newPage() {
    if (!actorId) return;
    setBusy(true);
    try {
      // Created empty and untitled, exactly as the rail's own note says —
      // naming it later is normal, because the id was never the name.
      const p = await createPage(actorId, { title: "Untitled" });
      navigate(`/pages/${p.id}`);
    } catch (e) {
      setErr(String(e));
    } finally {
      setBusy(false);
    }
  }

  // "Resume" is drawn from the two most recently updated pages. The caret
  // is the part that is still placeholder — updated_at is a real signal,
  // a stored caret position is not one we have.
  const resume = [...pages]
    .sort((a, b) => (a.updated_at < b.updated_at ? 1 : -1))
    .slice(0, 2);

  const untopiced = topics?.untopiced_pages ?? 0;
  const now = new Date();

  return (
    <Screen>
      <TopBar
        crumb={<>workspace / <b>marginal</b></>}
        readouts={
          <>
            <Readout k="PAGES" v={num(pages.length)} />
            <Readout k="LIVE NOW" v={ph(0)} tone="#3FCFA8" />
          </>
        }
      />

      <Body>
        <div className="rail">
          <div className="rail-h">
            PAGE TREE<div /><span style={{ color: "#585550" }}>{pages.length}</span>
          </div>
          <div className="filt">filter…</div>
          <div style={{ display: "flex", flexDirection: "column", gap: 1, padding: "0 8px", overflowY: "auto" }}>
            {pages.map((p, i) => (
              <div
                key={p.id}
                className="tr"
                style={{ cursor: "pointer" }}
                onClick={() => navigate(`/pages/${p.id}`)}
              >
                <span className="tr-n">{String(i + 1).padStart(2, "0")}</span>
                {p.title}
                {p.topic && (
                  <span style={{
                    marginLeft: "auto", width: 5, height: 5,
                    background: `var(--topic-${p.topic.color_key})`,
                  }} />
                )}
              </div>
            ))}
          </div>
          <div className="wal">
            <Label>NEW</Label>
            <div style={{ fontSize: 11, lineHeight: 1.55, color: "#8C8880" }}>
              A page is created empty and untitled. Naming it later is normal — the id was never the name.
            </div>
          </div>
        </div>

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
          <Label style={{ display: "block", marginBottom: 12 }}>RESUME</Label>
          <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 11, marginBottom: 28 }}>
            {resume.map((p, i) => (
              <div
                key={p.id}
                onClick={() => navigate(`/pages/${p.id}`)}
                style={{
                  border: i === 0 ? "1px solid rgba(232,135,60,.35)" : "1px solid rgba(255,255,255,.09)",
                  background: i === 0 ? "rgba(232,135,60,.04)" : undefined,
                  padding: "14px 16px", cursor: "pointer",
                }}
              >
                <div style={{ display: "flex", alignItems: "baseline", gap: 9, marginBottom: 7 }}>
                  <span style={{ fontFamily: "Spectral,serif", fontSize: 16, color: "#EFEDE7" }}>
                    {p.title}
                  </span>
                  <div style={{ flex: 1 }} />
                </div>
                <div className="tgrow" style={{ marginBottom: 9 }}>
                  {p.topic
                    ? <TopicChip name={p.topic.name} colorKey={p.topic.color_key} />
                    : <span className="chip">UNTOPICED</span>}
                  {(p.tags ?? []).slice(0, 2).map((t) => <span key={t} className="tg">{t}</span>)}
                </div>
                <div className="mono" style={{ fontSize: 10.5, color: "#585550" }}>
                  {ph("caret at block —")} · {new Date(p.updated_at).toLocaleString("en-GB", {
                    day: "2-digit", month: "short", hour: "2-digit", minute: "2-digit",
                  })}
                </div>
              </div>
            ))}
          </div>

          <div style={{ display: "grid", gridTemplateColumns: "1.35fr 1fr", gap: 32, flex: 1, minHeight: 0 }}>
            <div style={{ display: "flex", flexDirection: "column", minHeight: 0 }}>
              <div style={{ display: "flex", alignItems: "baseline", gap: 10, marginBottom: 12 }}>
                <Label>CHANGED SINCE YOU LAST LOOKED</Label>
                <span className="mono" style={{ fontSize: 10, color: "#585550" }}>by op, not by mtime</span>
              </div>
              <PlaceholderNote>needs a read over collab.ops since your last session</PlaceholderNote>
              <div style={{ display: "flex", flexDirection: "column", ...undrawn }}>
                {[
                  { av: "AD", cls: "av-them", title: ph("Anchors vs offsets"), meta: ph("+2 blocks · −1"), when: ph("2 h ago"), tone: "#585550" },
                  { av: "✦", cls: "av-ai", title: ph("Block model"), meta: ph("2 ops proposed"), when: ph("4 h ago"), tone: "#7D9EC9" },
                  { av: "RK", cls: "", title: ph("CRDT survey"), meta: ph("retitled"), when: ph("yesterday"), tone: "#585550" },
                ].map((r, i) => (
                  <div key={i} className="row" style={{ padding: "10px 0" }}>
                    <div className={`av ${r.cls}`} style={{ width: 18, height: 18, fontSize: 7 }}>{r.av}</div>
                    <span style={{ flex: 1, fontSize: 12.5, color: "#D2CFC8" }}>{r.title}</span>
                    <span className="mono" style={{ fontSize: 10, color: r.tone }}>{r.meta}</span>
                    <span className="mono" style={{ fontSize: 10, color: "#585550", width: 58, textAlign: "right" }}>
                      {r.when}
                    </span>
                  </div>
                ))}
              </div>
              <div className="mono" style={{ fontSize: 10, color: "#585550", marginTop: 11 }}>
                read from collab.ops since your last session ended — a page edited and reverted
                shows nothing, because nothing changed
              </div>

              <div style={{ marginTop: 22 }}>
                <Label style={{ display: "block", marginBottom: 11 }}>START</Label>
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
                <Label style={{ display: "block", marginBottom: 11 }}>
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
                <Label style={{ display: "block", marginBottom: 11 }}>WORKSPACE</Label>
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
                <Label style={{ display: "block", marginBottom: 10 }}>WHY NO FEED</Label>
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
