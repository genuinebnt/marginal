import { useEffect, useMemo, useState } from "react";
import { Link, useParams } from "react-router-dom";
import { useAuth } from "../auth/AuthContext";
import { useCollabPage } from "../collab/useCollabPage";
import { getTrace, getPalimpsest, type TraceStep, type PalimpsestChar } from "../api/history";

const ACTOR_COLORS = ["#1F8A75", "#7A5AC2", "#B8791E", "#4F6D9A", "#C2547A"];
function colorFor(actorId: string, order: string[]): string {
  const i = order.indexOf(actorId);
  return ACTOR_COLORS[i % ACTOR_COLORS.length];
}

/**
 * docs/ui-mockups/history.html, made real (v2.4.0): the scrubber walks
 * a real replay (GET .../trace, internal/session.Trace, unchanged from
 * trace.html's own data source), "Restore this version" sends a real
 * "restore" WS message (Session.RestoreTo — repeated undo, never a
 * snapshot swap), and the palimpsest panel reads one block's real
 * tombstoned character history (GET .../palimpsest,
 * internal/palimpsest.Build) — nothing here is simulated the way the
 * mockup's own client-side model was. The op stream's actor filter and
 * "current" line are plain presentation over that same real data.
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
  const [actorFilter, setActorFilter] = useState<Set<string> | null>(null);
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
    // The ack/broadcast round trip updates collab's own live state; this
    // screen's own scrubber re-fetches GET .../trace once the restore
    // has had a moment to flush, rather than trying to reconstruct the
    // new tip purely from WS acks.
    setTimeout(() => setRestoring(false), 500);
  }

  if (error) {
    return <div className="app"><div className="note" style={{ margin: 24 }}>{error}</div></div>;
  }

  return (
    <div className="app">
      <header className="topbar">
        <Link to="/pages" className="brand" style={{ textDecoration: "none" }}>
          <span className="mark"></span>Marginal
        </Link>
        <nav className="nav">
          <Link to={`/pages/${pageId}/history`} aria-current="page">History</Link>
          <Link to={`/pages/${pageId}/trace`}>Trace</Link>
          {pageId && <Link to={`/pages/${pageId}/diff`}>Diff</Link>}
          {pageId && <Link to={`/pages/${pageId}`}>Editor</Link>}
        </nav>
        <div className="crumb">Product · <b>History</b></div>
        <div className="spacer"></div>
        <button
          className="btn primary"
          disabled={!steps || scrub >= steps.length - 1 || restoring}
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
        <div className="note" style={{ margin: 24 }}>This page has no confirmed ops yet.</div>
      ) : (
        <div className="body-row">
          <main className="canvas" style={{ flex: 1, padding: 24, overflowY: "auto" }}>
            <div style={{ display: "flex", alignItems: "center", gap: 10, marginBottom: 12 }}>
              <span className="pill violet">op {scrub + 1} of {steps.length}</span>
              <span className="muted">{scrub === steps.length - 1 ? "current" : "viewing a past revision"}</span>
            </div>
            <input
              type="range"
              min={0}
              max={steps.length - 1}
              value={scrub}
              onChange={(e) => setScrub(Number(e.target.value))}
              style={{ width: "100%" }}
              aria-label="Scrub through revisions"
            />

            <article className="doc standard" style={{ marginTop: 20 }}>
              <h1>{current?.after.title || "Untitled"}</h1>
              {current?.after.blocks.map((b) => (
                <p
                  key={b.id}
                  onClick={() => setPalBlockId(b.id)}
                  style={{ cursor: "pointer", background: palBlockId === b.id ? "var(--hover)" : undefined, borderRadius: 4, padding: "2px 4px" }}
                  title="Click to inspect this block's palimpsest"
                >
                  {b.text || <span className="muted">(empty)</span>}
                </p>
              ))}
            </article>

            {palBlockId && (
              <div className="panel-section" style={{ marginTop: 20, maxWidth: "40rem" }}>
                <div className="panel-h" style={{ display: "flex", alignItems: "center", gap: 8 }}>
                  Palimpsest
                  <button className="btn" style={{ marginLeft: "auto" }} aria-pressed={palReveal} onClick={() => setPalReveal((v) => !v)}>
                    {palReveal ? "Hide tombstones" : "Reveal tombstones"}
                  </button>
                </div>
                {!palChars ? (
                  <div className="muted">Loading…</div>
                ) : (
                  <>
                    <div style={{ fontFamily: "var(--mono)", fontSize: 14, lineHeight: 1.8, wordBreak: "break-word" }}>
                      {palChars
                        .filter((c) => palReveal || c.delete_step == null)
                        .map((c, i) => {
                          const isDead = c.delete_step != null;
                          const age = isDead ? scrub - c.delete_step! : 0;
                          const opacity = isDead ? Math.max(0.25, 1 - age / Math.max(1, steps.length)) : 1;
                          return (
                            <span
                              key={i}
                              style={{
                                textDecoration: isDead ? "line-through" : undefined,
                                color: isDead ? colorFor(c.delete_actor ?? "", actorOrder) : undefined,
                                opacity,
                              }}
                              title={isDead ? `deleted at step ${c.delete_step}` : `inserted at step ${c.insert_step}`}
                            >
                              {String.fromCodePoint(c.rune)}
                            </span>
                          );
                        })}
                    </div>
                    <div className="fmeta" style={{ marginTop: 10, display: "flex", gap: 14, fontSize: 12, color: "var(--ink-faint)" }}>
                      <span>chars stored <b>{palChars.length}</b></span>
                      <span>live <b>{palChars.filter((c) => c.delete_step == null).length}</b></span>
                      <span>tombstoned <b>{palChars.filter((c) => c.delete_step != null).length}</b></span>
                      <span>versions addressable <b>{steps.length}</b></span>
                    </div>
                  </>
                )}
              </div>
            )}
          </main>

          <aside className="rail right">
            <div className="rail-head" style={{ display: "flex", alignItems: "center" }}>
              <span className="label">Op stream</span>
              <span className="spacer"></span>
              <span className="muted mono">{steps.length}</span>
            </div>
            <div className="panel-body">
              <div style={{ display: "flex", flexWrap: "wrap", gap: 6, marginBottom: 10 }}>
                {actorOrder.map((a) => {
                  const active = !actorFilter || actorFilter.has(a);
                  return (
                    <button
                      key={a}
                      className="pill"
                      style={{ borderColor: colorFor(a, actorOrder), color: active ? colorFor(a, actorOrder) : "var(--ink-faint)", cursor: "pointer" }}
                      onClick={() =>
                        setActorFilter((prev) => {
                          const next = new Set(prev ?? actorOrder);
                          if (next.has(a)) next.delete(a);
                          else next.add(a);
                          return next.size === actorOrder.length ? null : next;
                        })
                      }
                    >
                      {a.slice(0, 8)}
                    </button>
                  );
                })}
              </div>
              {steps.map((s, i) => {
                if (actorFilter && !actorFilter.has(s.op.actor_id)) return null;
                return (
                  <div
                    key={s.op.id}
                    className="row"
                    style={{ cursor: "pointer", background: i === scrub ? "var(--hover)" : undefined }}
                    onClick={() => setScrub(i)}
                  >
                    <span className="lead" style={{ color: colorFor(s.op.actor_id, actorOrder) }}>●</span>
                    {s.op.op.scope === "block" ? String(s.op.op.type) : `${s.op.op.type ?? "text"}`}
                    <span className="muted" style={{ marginLeft: "auto", fontSize: 11 }}>{i}</span>
                  </div>
                );
              })}
            </div>
          </aside>
        </div>
      )}
    </div>
  );
}
