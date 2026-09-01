/**
 * docs/ui-mockups/v2/index.html § 04 EDITOR's inspector, ported.
 *
 * Rebuilt from the mockup's own markup rather than carried over from V1.
 * The panel order, labels and copy are § 04's; only the numbers are this
 * instance's.
 *
 * Verified with tools/uidiff rather than by eye: the previous version of this
 * file used V1 class names, so the entire `.insp` region was absent from the
 * rendered page — width 0 — while screenshots looked plausible.
 *
 * Four tabs, which is § 04's set and deliberately not the seven the design
 * lists as candidates: "already too many to show at once" is the mockup's own
 * note, and a strip overflowing its 332px panel reads as a rendering bug.
 */
import { useCallback, useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { getBacklinks, type Backlink, type Page } from "../api/pages";
import {
  listThreads, replyToThread, setThreadResolved, type Thread,
} from "../api/comments";
import type { Diagnostic } from "../api/diagnostics";
import type { CollabPage } from "../collab/useCollabPage";
import { Label, Rule, TopicChip, num } from "../shell/Chrome";

type Tab = "outline" | "checks" | "links" | "presence" | "comments" | "queue";

/** A short, stable two-character tag for an actor — never their id verbatim. */
function actorTag(actorId: string): string {
  let hash = 0;
  for (let i = 0; i < actorId.length; i++) hash = (hash * 31 + actorId.charCodeAt(i)) | 0;
  const a = "ABCDEFGHIJKLMNOPQRSTUVWXYZ";
  return a[Math.abs(hash) % 26] + a[Math.abs(hash >> 5) % 26];
}

/** RFC-003 §2's analyzer count — the denominator "checks passed" reports. */
const ANALYZERS = 9;

export function InspectorRail({
  page, actorId, collab, diagnostics, diagnosticsError, onRefreshDiagnostics,
}: {
  page: Page;
  actorId: string;
  collab: CollabPage;
  diagnostics: Diagnostic[] | null;
  diagnosticsError: string | null;
  onRefreshDiagnostics: () => void;
}) {
  const navigate = useNavigate();
  const [tab, setTab] = useState<Tab>("checks");
  const [threads, setThreads] = useState<Thread[]>([]);
  const [threadsError, setThreadsError] = useState<string | null>(null);

  /** Open threads only. A count including resolved ones would grow forever
 *  and stop meaning "there is something to answer". */
  const openThreads = threads.filter((t) => !t.resolved_at).length;

  const loadThreads = useCallback(() => {
    if (!page?.id) return;
    listThreads(page.id)
      .then((r) => { setThreads(r.threads); setThreadsError(null); })
      .catch((e) => setThreadsError(String(e)));
  }, [page?.id]);

  useEffect(loadThreads, [loadThreads]);

  const replyTo = useCallback(async (threadId: string) => {
    const body = window.prompt("Reply:");
    if (!body) return;
    try { await replyToThread(threadId, body); loadThreads(); }
    catch (e) { setThreadsError(String(e)); }
  }, [loadThreads]);

  const toggleResolved = useCallback(async (t: Thread) => {
    try { await setThreadResolved(t.id, !t.resolved_at); loadThreads(); }
    catch (e) { setThreadsError(String(e)); }
  }, [loadThreads]);
  const [backlinks, setBacklinks] = useState<Backlink[]>([]);

  useEffect(() => {
    getBacklinks(actorId, page.id)
      .then((r) => setBacklinks(r.backlinks))
      .catch(() => setBacklinks([]));
  }, [actorId, page.id]);

  const open = diagnostics ?? [];
  const passed = Math.max(ANALYZERS - open.length, 0);
  const headings = collab.blocks.filter((b) => b.kind.tag === "heading");

  return (
    <div className="insp">
      {/* .insp-t is the mockup's own class. NOT .tabs — that belongs to the
          top-bar nav, and redefining it here put the inspector's padding on
          the nav strip (caught by tools/uidiff, invisible in a screenshot). */}
      <div className="insp-t">
        {/* Counts live ON the tab, per § 04: an inspector that only reports
            two open checks once you have already opened the checks tab is an
            inspector you have to go and interrogate. A zero is not drawn —
            "CHECKS 0" reads as a finding rather than as its absence. */}
        {([
          { id: "outline", label: "OUTLINE" },
          { id: "checks", label: "CHECKS", count: open.length, tone: "#E0A34E" },
          { id: "links", label: "LINKS", count: backlinks.length, tone: "#E8873C" },
          { id: "presence", label: "PRESENCE", count: collab.peers.size, tone: "#A98CE8" },
          // Open threads only. A count that included resolved ones would
          // grow forever and stop meaning "there is something to answer".
          { id: "comments", label: "COMMENTS", count: openThreads, tone: "#7AA8E8" },
          // § 24's own tab, and it appears only when there IS a queue: a
          // permanently-visible "QUEUE 0" is a tab that teaches you to ignore
          // it, which is exactly wrong for the one that matters when it is
          // not zero.
          ...(collab.queued > 0 || collab.state !== "open"
            ? [{ id: "queue" as Tab, label: "QUEUE", count: collab.queued, tone: "#E0A34E" }]
            : []),
        ] as Array<{ id: Tab; label: string; count?: number; tone?: string }>).map((t) => (
          <span
            key={t.id}
            className={`it${t.id === tab ? " it-on" : ""}`}
            style={{ cursor: "pointer" }}
            onClick={() => setTab(t.id)}
          >
            {t.label}
            {t.count ? <span style={{ color: t.tone }}> {t.count}</span> : null}
          </span>
        ))}
      </div>

      <div style={{ padding: 14, display: "flex", flexDirection: "column", gap: 12, overflowY: "auto" }}>
        {tab === "checks" && (
          <>
            <Label>STRUCTURAL</Label>
            {diagnosticsError && (
              <div style={{ fontSize: 11.5, color: "#E0A34E", lineHeight: 1.55 }}>
                ◌ {diagnosticsError}{" "}
                <span style={{ cursor: "pointer", textDecoration: "underline" }} onClick={onRefreshDiagnostics}>
                  retry
                </span>
              </div>
            )}
            {!diagnosticsError && open.length === 0 && (
              <div style={{ fontSize: 12, color: "#8C8880", lineHeight: 1.55 }}>
                <span style={{ color: "#3FCFA8" }}>✓</span> Nothing structural to report.
              </div>
            )}
            {open.map((d, i) => (
              <div key={i} style={{ display: "flex", gap: 9 }}>
                {/* A 2px left rule and a wash, never a full outline (§4.2).
                    Amber throughout — a notebook has no compile step, so a red
                    squiggle on prose reads as an accusation. */}
                <div style={{ width: 2, flex: "none", background: i === 0 ? "#E0A34E" : "rgba(224,163,78,.45)" }} />
                <div>
                  <div style={{ fontSize: 12.5, lineHeight: 1.45, color: "#D2CFC8" }}>{d.message}</div>
                  <div className="mono" style={{ fontSize: 10, color: "#585550", marginTop: 3 }}>
                    {d.block_id ? `block ${d.block_id.slice(0, 4)}` : "page"} · {d.analyzer}
                  </div>
                  {d.analyzer === "DanglingLink" && (
                    <div style={{ marginTop: 7 }}>
                      <span className="chip chip-a">CREATE PAGE</span>
                    </div>
                  )}
                </div>
              </div>
            ))}

            <Rule />
            <div style={{ display: "flex", alignItems: "baseline", gap: 8 }}>
              <Label>CHECKS PASSED</Label>
              <span className="mono" style={{ marginLeft: "auto", fontSize: 10, fontWeight: 500, color: "#8C8880" }}>
                {passed}/{ANALYZERS}
              </span>
            </div>
            <div style={{ display: "flex", gap: 3, height: 6 }}>
              {Array.from({ length: ANALYZERS }, (_, i) => (
                <div key={i} style={{ flex: 1, background: i < passed ? "#3FCFA8" : "#E0A34E" }} />
              ))}
            </div>
          </>
        )}

        {tab === "outline" && (
          <>
            <Label>DOCUMENT STRUCTURE</Label>
            {headings.length === 0 && (
              <div style={{ fontSize: 11.5, color: "#585550", lineHeight: 1.6 }}>
                No headings yet. A flat page has no structure to show, which is a real state
                rather than an empty panel.
              </div>
            )}
            {headings.map((b) => {
              const level = (b.kind as { level?: number }).level ?? 1;
              return (
                <div key={b.id} style={{ display: "flex", gap: 9, paddingLeft: (level - 1) * 12 }}>
                  <span className="mono" style={{ fontSize: 9, color: "#4B4842", width: 16, flex: "none" }}>
                    H{level}
                  </span>
                  <span style={{ fontSize: 12, color: level === 1 ? "#E4E2DC" : "#9B968D" }}>
                    {b.text || "(empty)"}
                  </span>
                </div>
              );
            })}
          </>
        )}

        {tab === "links" && (
          <>
            <Label>BACKLINKS · {num(backlinks.length)}</Label>
            {backlinks.length === 0 && (
              <div style={{ fontSize: 11.5, color: "#585550", lineHeight: 1.6 }}>
                Nothing links here yet. This is the reverse index over docs.page_links, not a text
                search — a page appears only once its [[link]] actually resolved.
              </div>
            )}
            {backlinks.map((b) => (
              <div
                key={b.from_page}
                style={{ fontSize: 12.5, color: b.from_page_deleted ? "#585550" : "#D2CFC8", cursor: "pointer" }}
                onClick={() => navigate(`/pages/${b.from_page}`)}
              >
                {b.from_page_title}
                {b.from_page_deleted && (
                  <span className="mono" style={{ fontSize: 9, color: "#E0A34E", marginLeft: 6 }}>DELETING</span>
                )}
              </div>
            ))}
          </>
        )}

        {tab === "queue" && (
          <>
            <div style={{ display: "flex", alignItems: "baseline", gap: 8 }}>
              <Label>QUEUED · IN ORDER</Label>
              <span className="mono" style={{ marginLeft: "auto", fontSize: 10, color: "#E0A34E" }}>
                {num(collab.queued)}
              </span>
            </div>
            <div style={{ fontSize: 11.5, lineHeight: 1.65, color: "#8C8880" }}>
              Nothing is lost. Ops replay in the order they were written, and the server
              applies each against its own current state — which works because a text op
              names an <b style={{ color: "#C3BFB7", fontWeight: 500 }}>anchor</b>, not an
              offset, and an anchor resolves in any version that still contains its
              neighbour. A delete tombstones rather than removes, so it always does.
            </div>

            <Rule />
            <Label>ON RECONNECT</Label>
            {[
              "Queued ops replay, in order",
              "The server re-sends a snapshot, so the document is its state, not ours",
              "Anything that could not apply comes back as an error frame",
            ].map((line, i) => (
              <div key={i} style={{ display: "flex", gap: 9, alignItems: "baseline" }}>
                <span style={{ color: "#3FCFA8", fontSize: 10 }}>{i + 1}</span>
                <span style={{ fontSize: 11.5, color: "#8C8880" }}>{line}</span>
              </div>
            ))}
            <div style={{ fontSize: 11, lineHeight: 1.6, color: "#585550" }}>
              No conflict dialog appears when the socket returns — but there is no
              operational transform here either, so an op whose anchor a peer deleted while
              you were away is REJECTED rather than merged. Stated, because pretending
              otherwise is the dishonest option.
            </div>

            <Rule />
            <Label>WHAT IS NOT CACHED</Label>
            <div style={{ fontSize: 11.5, lineHeight: 1.65, color: "#8C8880" }}>
              The document itself. A page's blocks arrive in the server's snapshot, so a page
              you have not opened this session is <b style={{ color: "#C3BFB7", fontWeight: 500 }}>unavailable</b>{" "}
              offline rather than stale — nothing here keeps a local copy. The queue protects
              edits you already made; it does not make the workspace readable without a server.
            </div>

            <Rule />
            <Label>WHAT KEEPS WORKING</Label>
            {[["typing", true], ["undo / redo", true], ["reading this page", true],
              ["opening a page you have not opened", false],
              ["search", false], ["the graph", false], ["opening another page", false]].map(
              ([what, ok]) => (
                <div key={what as string} style={{
                  display: "flex", alignItems: "baseline", gap: 8, fontSize: 11.5, color: "#9B968D",
                }}>
                  <span style={{ flex: 1 }}>{what}</span>
                  <span className="mono" style={{ fontSize: 11, color: ok ? "#3FCFA8" : "#585550" }}>
                    {ok ? "yes" : "no"}
                  </span>
                </div>
              ),
            )}
          </>
        )}

        {tab === "comments" && (
          <div style={{ display: "flex", flexDirection: "column", gap: 12 }}>
            {threadsError && (
              <div style={{ fontSize: 11.5, color: "#E0A34E" }}>◌ {threadsError}</div>
            )}
            {threads.length === 0 && !threadsError && (
              <div style={{ fontSize: 11.5, lineHeight: 1.6, color: "#585550" }}>
                No threads on this page. Select a block's handle and use COMMENT to open one —
                a thread is anchored to the text, so it follows the words when somebody edits
                around it.
              </div>
            )}
            {threads.map((t) => (
              <div key={t.id} style={{
                border: "1px solid rgba(255,255,255,.08)", padding: "10px 11px",
                background: t.resolved_at ? "transparent" : "rgba(122,168,232,.04)",
                opacity: t.resolved_at ? 0.65 : 1,
              }}>
                <div className="mono" style={{
                  fontSize: 10, color: t.orphaned ? "#E0A34E" : "#7AA8E8", marginBottom: 6,
                }}>
                  {t.orphaned
                    // Said, not hidden: the words are gone and the remark is
                    // not, and a reader has to be able to tell.
                    ? "the text this was about is gone"
                    : `chars ${t.range?.start ?? 0}–${t.range?.end ?? 0}`}
                  {t.resolved_at && " · resolved"}
                </div>
                <div style={{
                  fontSize: 11.5, lineHeight: 1.5, color: "#8C8880", marginBottom: 8,
                  borderLeft: "2px solid rgba(255,255,255,.12)", paddingLeft: 8,
                }}>
                  {t.quoted || <i>(an empty block)</i>}
                </div>
                {t.comments.map((c) => (
                  <div key={c.id} style={{ marginBottom: 7 }}>
                    <span className="mono" style={{ fontSize: 9.5, color: "#585550" }}>
                      {actorTag(c.author_id)}
                    </span>
                    <div style={{ fontSize: 12, lineHeight: 1.5, color: "#D2CFC8" }}>{c.body}</div>
                  </div>
                ))}
                <div style={{ display: "flex", gap: 6, marginTop: 8 }}>
                  <span className="chip" style={{ cursor: "pointer" }}
                        onClick={() => void replyTo(t.id)}>REPLY</span>
                  <span className="chip" style={{ cursor: "pointer" }}
                        onClick={() => void toggleResolved(t)}>
                    {t.resolved_at ? "REOPEN" : "RESOLVE"}
                  </span>
                </div>
              </div>
            ))}
            <Rule />
            <div style={{ fontSize: 11.5, lineHeight: 1.7, color: "#8C8880" }}>
              A thread is anchored, not positioned: it holds the identity of the characters it
              was opened on, so it follows them when somebody types above. The quote never
              changes — it is what was being discussed, not what the anchors point at now.
              <br /><br />
              Comments are not ops. They are absent from Trace, the palimpsest and the diff on
              purpose: those are views of the op log, and a remark is not a change to the
              document.
            </div>
          </div>
        )}

        {tab === "presence" && (
          <>
            <Label>LIVE IN THIS PAGE</Label>
            <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
              <div style={{ width: 6, height: 6, flex: "none", background: "#3FCFA8" }} />
              <span style={{ fontSize: 12, color: "#D2CFC8" }}>You</span>
              <span className="mono" style={{ marginLeft: "auto", fontSize: 10, color: "#585550" }}>
                {collab.state}
              </span>
            </div>
            {[...collab.peers].map((p) => {
              const c = collab.cursors.get(p);
              return (
                <div key={p} style={{ display: "flex", alignItems: "center", gap: 8 }}>
                  {/* The ping fires on JOIN, never idling — an avatar that
                      pulses forever is a distraction, not presence. */}
                  <div className="dot" style={{ width: 6, height: 6, background: "#A98CE8" }}>
                    <div className="ping" style={{ background: "rgba(169,140,232,.55)" }} />
                  </div>
                  <span style={{ fontSize: 12, color: "#D2CFC8" }}>{actorTag(p)}</span>
                  <span className="mono" style={{ marginLeft: "auto", fontSize: 10, color: "#585550" }}>
                    {c ? `block ${c.blockId.slice(0, 4)}` : "reading"}
                  </span>
                </div>
              );
            })}
            {collab.peers.size === 0 && (
              <div style={{ fontSize: 11.5, color: "#585550", lineHeight: 1.6 }}>
                Nobody else is here. Presence is a real join/leave signal, not inferred from
                whether ops happen to be arriving.
              </div>
            )}
          </>
        )}

        {/* Below the tabs, always — § 04 shows these regardless of which tab
            is open, because they describe the page rather than one view of it. */}
        <Rule />
        <Label>THIS PAGE</Label>
        {[
          { k: "blocks", v: num(collab.blocks.length), tone: "#E4E2DC" },
          { k: "live actors", v: num(collab.peers.size + 1), tone: "#E4E2DC" },
          { k: "unflushed ops", v: collab.state === "open" ? "0" : "—",
            tone: collab.state === "open" ? "#3FCFA8" : "#E0A34E" },
          { k: "connection", v: collab.state,
            tone: collab.state === "open" ? "#3FCFA8" : "#E0A34E" },
        ].map((r) => (
          <div key={r.k} style={{ display: "flex", alignItems: "baseline", gap: 8, fontSize: 11.5, color: "#9B968D" }}>
            <span style={{ flex: 1 }}>{r.k}</span>
            <span className="mono" style={{ fontSize: 11, color: r.tone }}>{r.v}</span>
          </div>
        ))}

        <Rule />
        <Label>TOPIC &amp; TAGS</Label>
        <div className="tgrow">
          {page.topic
            ? <TopicChip name={page.topic.name} colorKey={page.topic.color_key} />
            : <span className="chip">UNTOPICED</span>}
          {(page.tags ?? []).map((t) => <span key={t} className="tg">{t}</span>)}
        </div>
      </div>
    </div>
  );
}
