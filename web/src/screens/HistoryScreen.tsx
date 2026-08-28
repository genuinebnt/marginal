import { useEffect, useMemo, useState } from "react";
import { Link, useParams } from "react-router-dom";
import { useAuth } from "../auth/AuthContext";
import { useCollabPage } from "../collab/useCollabPage";
import { getTrace, getPalimpsest, describeOp, type TraceStep, type PalimpsestChar } from "../api/history";

const ACTOR_COLORS = ["var(--teal)", "var(--violet)", "var(--ai)", "var(--cat-4)", "var(--cat-5)"];
function colorFor(actorId: string, order: string[]): string {
  const i = order.indexOf(actorId);
  return ACTOR_COLORS[i % ACTOR_COLORS.length];
}

/**
 * docs/ui-mockups/v2/index.html § 17 HISTORY, made real (v2.4.0): the scrubber walks a
 * real replay (GET .../trace, internal/session.Trace, unchanged from
 * trace.html's own data source), "Restore this version"/"Undo my last
 * edit" send real WS messages (Session.RestoreTo/Undo), and the
 * palimpsest panel reads one block's real tombstoned character history
 * (GET .../palimpsest, internal/palimpsest.Build). Snapshot tick marks
 * and rebuild-timing telemetry from the mockup are dropped — this repo
 * has no snapshot system (RFC-002: replay from scratch, always) and no
 * such instrumentation, and a decorative substitute would be exactly the
 * "not real" the rest of this repo refuses to ship.
 */
export function HistoryScreen() {
  const { id: pageId } = useParams();
  const { session, logout } = useAuth();
  if (!session) throw new Error("HistoryScreen requires an authenticated session");
  const { actorId } = session;
  const collab = useCollabPage(pageId ?? "", actorId);

  const [steps, setSteps] = useState<TraceStep[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [scrub, setScrub] = useState(0);
  const [actorFilter, setActorFilter] = useState<string | "all">("all");
  const [restoring, setRestoring] = useState(false);

  const [palBlockId, setPalBlockId] = useState<string | null>(null);
  const [palChars, setPalChars] = useState<PalimpsestChar[] | null>(null);
  const [palReveal, setPalReveal] = useState(false);

  useEffect(() => {
    if (!pageId) return;
    let cancelled = false;
    getTrace(pageId)
      .then((r) => {
        if (cancelled) return;
        setSteps(r.steps);
        setScrub(r.steps.length > 0 ? r.steps.length - 1 : 0);
      })
      .catch(() => {
        if (!cancelled) setError("Couldn't load this page's op log.");
      });
    return () => {
      cancelled = true;
    };
  }, [pageId, restoring]); // re-fetch after a restore completes, so the scrubber reflects the new tip

  const actorOrder = useMemo(() => {
    const seen: string[] = [];
    for (const s of steps ?? []) if (!seen.includes(s.op.actor_id)) seen.push(s.op.actor_id);
    return seen;
  }, [steps]);

  const current = steps?.[scrub] ?? null;
  const isTip = !!steps && scrub === steps.length - 1;

  // The same "pop this actor's own most recent undo_group" rule
  // Session.Undo itself applies (RFC-002 §3), computed here only to
  // preview which op(s) a click would revert — the actual undo still
  // goes through collab.undo(), authoritative server-side.
  const willUndoIds = useMemo(() => {
    if (!steps) return new Set<string>();
    for (let i = steps.length - 1; i >= 0; i--) {
      if (steps[i].op.actor_id !== actorId) continue;
      const group = steps[i].op.undo_group;
      if (!group) return new Set([steps[i].op.id]);
      return new Set(steps.filter((s) => s.op.undo_group === group).map((s) => s.op.id));
    }
    return new Set<string>();
  }, [steps, actorId]);

  useEffect(() => {
    setPalChars(null);
    if (!current || !palBlockId) return;
    if (!current.after.blocks.some((b) => b.id === palBlockId)) return;
    if (!pageId) return;
    let cancelled = false;
    getPalimpsest(pageId, palBlockId).then((r) => {
      if (!cancelled) setPalChars(r.chars);
    });
    return () => {
      cancelled = true;
    };
  }, [pageId, palBlockId, current]);

  async function handleRestore() {
    if (!steps || scrub >= steps.length - 1) return;
    setRestoring(true);
    collab.restoreTo(scrub);
    setTimeout(() => setRestoring(false), 500);
  }

  if (error) {
    return <div className="app"><div className="note" style={{ margin: 24, maxWidth: "none" }}>{error}</div></div>;
  }

  const palBlock = current?.after.blocks.find((b) => b.id === palBlockId);

  return (
    <div className="app">
      <header className="topbar">
        <Link to="/pages" className="brand" style={{ textDecoration: "none" }}>
          <span className="mark"></span>Marginal
        </Link>
        <nav className="nav">
          {pageId && <Link to={`/pages/${pageId}`}>Editor</Link>}
          <Link to={`/pages/${pageId}/history`} aria-current="page">History</Link>
          <Link to={`/pages/${pageId}/trace`}>Op trace</Link>
          {pageId && <Link to={`/pages/${pageId}/diff`}>Compare</Link>}
        </nav>
        <div className="crumb">Product · <b>{current?.after.title || "Untitled"}</b></div>
        <div className="spacer"></div>
        <button className="btn" onClick={() => collab.undo()} title="Reverts your own most recent gesture — never a peer's">
          Undo my last edit
        </button>
        <button
          className="btn primary"
          disabled={!steps || isTip || restoring}
          onClick={handleRestore}
          title="Sends a real 'restore' message — repeated undo, applied live"
        >
          {restoring ? "Restoring…" : "Restore this version"}
        </button>
        <button className="btn" onClick={logout}>Sign out</button>
      </header>

      {!steps ? (
        <div className="muted" style={{ padding: 24 }}>Loading…</div>
      ) : steps.length === 0 ? (
        <div className="note" style={{ margin: 24, maxWidth: "none" }}>This page has no confirmed ops yet.</div>
      ) : (
        <>
          <div className="scrub">
            <div className="scrub-head">
              <span className="scrub-when">{new Date(current!.op.created_at).toLocaleString()}</span>
              <span className="pill violet">op {scrub + 1} of {steps.length}</span>
              <span className="muted">{isTip ? "current" : "historical — read only"}</span>
              <span className="spacer"></span>
            </div>
            <div className="track">
              <div className="ticks">
                {steps.map((s, i) => (
                  <div
                    key={s.op.id}
                    className="tick"
                    style={{ left: `${(i / Math.max(1, steps.length - 1)) * 100}%`, background: colorFor(s.op.actor_id, actorOrder) }}
                  />
                ))}
              </div>
              <input
                type="range"
                min={0}
                max={steps.length - 1}
                value={scrub}
                onChange={(e) => setScrub(Number(e.target.value))}
                aria-label="Scrub through revisions"
              />
            </div>
            <div className="legend">
              {actorOrder.map((a) => (
                <span key={a}>
                  <i style={{ background: colorFor(a, actorOrder) }}></i>
                  {a === actorId ? "You" : a.slice(0, 8)}
                </span>
              ))}
            </div>
          </div>

          <div className="body-row">
            <main className="canvas">
              <article className="doc standard">
                <h1>{current?.after.title || "Untitled"}</h1>
                <div className="dek">As of op {scrub + 1}</div>
                {current?.after.blocks.map((b) => (
                  <p
                    key={b.id}
                    onClick={() => setPalBlockId(b.id)}
                    style={{ cursor: "pointer", background: palBlockId === b.id ? "var(--hover)" : undefined, borderRadius: 4 }}
                    title="Click to inspect this block's palimpsest"
                  >
                    {b.text || <span className="muted">(empty)</span>}
                  </p>
                ))}

                {palBlockId && (
                  <div className="pal-wrap">
                    <div className="pal-head">
                      <span className="label">Palimpsest{palBlock ? ` — ${(palBlock.text || "").slice(0, 24) || "empty block"}` : ""}</span>
                      <span className="pill violet">v{scrub}</span>
                      <span className="spacer"></span>
                      <button className="pal-toggle" aria-pressed={palReveal} onClick={() => setPalReveal((v) => !v)}>
                        Palimpsest
                      </button>
                    </div>
                    {!palChars ? (
                      <div className="muted">Loading…</div>
                    ) : (
                      <>
                        <div className="pal-text">
                          {palChars
                            .filter((c) => palReveal || c.delete_step == null)
                            .map((c, i) => {
                              const isDead = c.delete_step != null;
                              const age = isDead ? scrub - c.delete_step! : 0;
                              const opacity = isDead ? Math.max(0.25, 1 - age / Math.max(1, steps.length)) : 1;
                              return isDead ? (
                                <span
                                  key={i}
                                  className="ghost"
                                  style={{ color: colorFor(c.delete_actor ?? "", actorOrder), opacity }}
                                  title={`deleted at step ${c.delete_step}`}
                                >
                                  {String.fromCodePoint(c.rune)}
                                </span>
                              ) : (
                                <span key={i} title={`inserted at step ${c.insert_step}`}>{String.fromCodePoint(c.rune)}</span>
                              );
                            })}
                        </div>
                        <div className="pal-meta">
                          <span>chars stored <b>{palChars.length}</b></span>
                          <span>live <b>{palChars.filter((c) => c.delete_step == null).length}</b></span>
                          <span>tombstoned <b>{palChars.filter((c) => c.delete_step != null).length}</b></span>
                          <span>versions addressable <b>{steps.length}</b></span>
                        </div>
                      </>
                    )}
                  </div>
                )}
              </article>

              <div className="note" style={{ maxWidth: "none" }}>
                <b>Really real.</b> The scrubber walks an actual replay of this page's confirmed op log
                (<code>internal/session.Trace</code>), "Restore this version" sends a real{" "}
                <code>restore</code> message (repeated undo, never a snapshot swap), and the palimpsest
                panel above reads one block's real tombstoned character array — a delete sets a version
                stamp, it never removes.
              </div>
            </main>

            <aside className="rail right">
              <div className="rail-head">
                <span className="label">Op stream</span>
                <span className="spacer"></span>
                <span className="muted mono">{steps.length}</span>
              </div>
              <div className="panel-body">
                <div className="filters" role="group" aria-label="Filter by actor">
                  <button aria-pressed={actorFilter === "all"} onClick={() => setActorFilter("all")}>All</button>
                  {actorOrder.map((a) => (
                    <button key={a} aria-pressed={actorFilter === a} onClick={() => setActorFilter(a)}>
                      {a === actorId ? "You" : a.slice(0, 8)}
                    </button>
                  ))}
                </div>

                <div>
                  {steps.map((s, i) => {
                    const { kind, detail } = describeOp(s.op.op);
                    const dim = actorFilter !== "all" && s.op.actor_id !== actorFilter;
                    const willUndo = willUndoIds.has(s.op.id);
                    return (
                      <div
                        key={s.op.id}
                        className={`op ${willUndo ? "will-undo" : ""} ${dim ? "dim" : ""}`.trim()}
                        onClick={() => setScrub(i)}
                        title={willUndo ? "Undo my last edit would revert this" : undefined}
                      >
                        <span className="who" style={{ background: colorFor(s.op.actor_id, actorOrder) }}></span>
                        <span><span className="kind">{kind}</span> {detail}</span>
                        <span className="at">{new Date(s.op.created_at).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" })}</span>
                      </div>
                    );
                  })}
                </div>

                <div className="panel-section">
                  <div className="panel-h">Undo scope</div>
                  <div className="note" style={{ margin: 0, maxWidth: "none" }}>
                    Undo inverts <b>your</b> last op, not the document's — every other actor's
                    interleaved edits stay untouched, because every op carries its own inverse.
                  </div>
                </div>
              </div>
            </aside>
          </div>
        </>
      )}
    </div>
  );
}
