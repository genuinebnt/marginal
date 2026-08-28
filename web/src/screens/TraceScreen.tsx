import { useEffect, useRef, useState } from "react";
import { Link, useParams } from "react-router-dom";
import { useAuth } from "../auth/AuthContext";
import { getTrace, type TraceStep } from "../api/history";

/**
 * docs/ui-mockups/trace.html, made real (v2.4.0): every step is a real
 * confirmed op from this page's own log, replayed and law-checked by
 * internal/session.Trace (GET /collab/pages/{id}/trace) — the badge
 * re-checks `apply(invert(op), apply(op, doc)) == doc` for real, per
 * step, not asserted the way the mockup's own fixed nine-op sequence
 * was. "Apply ▶"/"◀ Invert" just move the scrubber across
 * already-computed steps (`after` is the whole document, precomputed —
 * ADR-012's "the client draws what Go already computed").
 */
export function TraceScreen() {
  const { id: pageId } = useParams();
  const { logout } = useAuth();

  const [steps, setSteps] = useState<TraceStep[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [pos, setPos] = useState(0); // 0 = before any op; N = right after steps[N-1]
  const [playing, setPlaying] = useState(false);
  const timerRef = useRef<number | null>(null);

  useEffect(() => {
    if (!pageId) return;
    getTrace(pageId)
      .then((r) => setSteps(r.steps))
      .catch(() => setError("Couldn't load this page's op log."));
  }, [pageId]);

  useEffect(() => {
    if (!playing || !steps) return;
    if (pos >= steps.length) {
      setPlaying(false);
      return;
    }
    timerRef.current = window.setTimeout(() => setPos((p) => p + 1), 700);
    return () => {
      if (timerRef.current) window.clearTimeout(timerRef.current);
    };
  }, [playing, pos, steps]);

  if (error) return <div className="app"><div className="note" style={{ margin: 24 }}>{error}</div></div>;
  if (!steps) return <div className="app"><div className="muted" style={{ padding: 24 }}>Loading…</div></div>;

  const currentStep = pos > 0 ? steps[pos - 1] : null;
  const doc = currentStep?.after ?? null;
  const lawHolds = steps.slice(0, pos).every((s) => s.law_holds);

  return (
    <div className="app">
      <header className="topbar">
        <Link to="/pages" className="brand" style={{ textDecoration: "none" }}>
          <span className="mark"></span>Marginal
        </Link>
        <nav className="nav">
          {pageId && <Link to={`/pages/${pageId}/history`}>History</Link>}
          <Link to={`/pages/${pageId}/trace`} aria-current="page">Trace</Link>
          {pageId && <Link to={`/pages/${pageId}/diff`}>Diff</Link>}
        </nav>
        <div className="crumb">Product · <b>Op trace</b></div>
        <div className="spacer"></div>
        <span className={`pill ${lawHolds ? "teal" : "amber"}`}>{lawHolds ? "law holds" : "law FAILED"}</span>
        <button className="btn" onClick={logout}>Sign out</button>
      </header>

      <div className="fbar" style={{ display: "flex", alignItems: "center", gap: 10, padding: "10px 20px" }}>
        <button className="btn" disabled={pos === 0} onClick={() => setPos((p) => Math.max(0, p - 1))}>◀ Invert</button>
        <button className="btn" disabled={pos >= steps.length} onClick={() => setPos((p) => Math.min(steps.length, p + 1))}>Apply ▶</button>
        <button className="btn primary" onClick={() => setPlaying((v) => !v)}>{playing ? "Pause" : "Play"}</button>
        <span className="label">Op</span>
        <span className="mono">{pos} / {steps.length}</span>
        <span className="spacer"></span>
        <span className="muted" style={{ fontSize: 12 }}>
          Backwards applies <b>invert(op)</b> — never a snapshot restore
        </span>
      </div>

      <div className="split3" style={{ display: "grid", gridTemplateColumns: "1fr 320px", flex: 1, minHeight: 0 }}>
        <main className="docpane" style={{ padding: 24, overflowY: "auto" }}>
          {doc ? (
            <article className="doc standard">
              <h1>{doc.title || "Untitled"}</h1>
              {doc.blocks.map((b) => (
                <p key={b.id}>{b.text || <span className="muted">(empty)</span>}</p>
              ))}
            </article>
          ) : (
            <div className="muted">Empty document — no ops applied yet.</div>
          )}

          {currentStep && (
            <div className="note" style={{ maxWidth: "36rem", marginTop: 20 }}>
              <b>Step {pos}.</b> {currentStep.op.op.scope === "block" ? String(currentStep.op.op.type) : "a character edit"}, by{" "}
              <code>{currentStep.op.actor_id.slice(0, 8)}</code>. Invertibility law:{" "}
              <b style={{ color: currentStep.law_holds ? "var(--teal)" : "var(--amber)" }}>
                {currentStep.law_holds ? "holds" : "FAILED"}
              </b>
              .
            </div>
          )}
        </main>

        <aside className="oplist rail right" style={{ overflowY: "auto" }}>
          <div className="rail-head"><span className="label">Op log</span></div>
          <div className="panel-body">
            {steps.map((s, i) => (
              <div
                key={s.op.id}
                className="row"
                style={{ cursor: "pointer", background: i + 1 === pos ? "var(--hover)" : undefined }}
                onClick={() => setPos(i + 1)}
              >
                <span className="lead" style={{ color: s.law_holds ? "var(--teal)" : "var(--amber)" }}>
                  {s.law_holds ? "✓" : "!"}
                </span>
                {s.op.op.scope === "block" ? String(s.op.op.type) : "text edit"}
                <span className="muted" style={{ marginLeft: "auto", fontSize: 11 }}>{i}</span>
              </div>
            ))}
          </div>
        </aside>
      </div>
    </div>
  );
}
