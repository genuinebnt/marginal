import { useEffect, useRef, useState } from "react";
import { Link, useParams } from "react-router-dom";
import { useAuth } from "../auth/AuthContext";
import { getTrace, describeOp, type TraceStep } from "../api/history";

const ACTOR_COLORS = ["var(--teal)", "var(--violet)", "var(--ai)", "var(--cat-4)", "var(--cat-5)"];
function colorFor(actorId: string, order: string[]): string {
  return ACTOR_COLORS[order.indexOf(actorId) % ACTOR_COLORS.length];
}

function blkClass(kindTag: string): string {
  if (kindTag === "heading") return "blk h2";
  if (kindTag === "code_block") return "blk code";
  if (kindTag === "quote") return "blk quote";
  return "blk";
}

/** The one block this step's op actually touched — pageop.Block ops name
 * the block directly or via their tombstone; pageop.Text ops name it via
 * `block`. Real data read off the op itself, not guessed. */
function touchedBlockId(op: TraceStep["op"]["op"]): string | null {
  if (op.scope === "text") return (op.block as string) ?? null;
  return (op.id as string) ?? (op.block as string) ?? ((op.tombstone as { id?: string } | undefined)?.id ?? null);
}

/**
 * docs/ui-mockups/v2/index.html § 13 TRACE, made real (v2.4.0): every step is a real
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

  if (error) return <div className="app"><div className="note" style={{ margin: 24, maxWidth: "none" }}>{error}</div></div>;
  if (!steps) return <div className="app"><div className="muted" style={{ padding: 24 }}>Loading…</div></div>;

  const actorOrder = [...new Set(steps.map((s) => s.op.actor_id))];
  const currentStep = pos > 0 ? steps[pos - 1] : null;
  const doc = currentStep?.after ?? null;
  const lawHolds = steps.slice(0, pos).every((s) => s.law_holds);
  const touched = currentStep ? touchedBlockId(currentStep.op.op) : null;

  return (
    <div className="app">
      <header className="topbar">
        <Link to="/pages" className="brand" style={{ textDecoration: "none" }}>
          <span className="mark"></span>Marginal
        </Link>
        <nav className="nav">
          {pageId && <Link to={`/pages/${pageId}`}>Editor</Link>}
          {pageId && <Link to={`/pages/${pageId}/history`}>History</Link>}
          <Link to={`/pages/${pageId}/trace`} aria-current="page">Op trace</Link>
        </nav>
        <div className="crumb">Product · <b>Op trace</b></div>
        <div className="spacer"></div>
        <span className={`law ${lawHolds ? "ok" : "fail"}`}>{lawHolds ? "law holds" : "law FAILED"}</span>
        <Link className="btn" to={`/pages/${pageId}/history`}>Scrubber</Link>
        <button className="btn" onClick={logout}>Sign out</button>
      </header>

      <div className="tracebar">
        <button className="btn" disabled={pos === 0} onClick={() => setPos((p) => Math.max(0, p - 1))}>◀ Invert</button>
        <button className="btn" disabled={pos >= steps.length} onClick={() => setPos((p) => Math.min(steps.length, p + 1))}>Apply ▶</button>
        <button className="btn primary" onClick={() => setPlaying((v) => !v)}>{playing ? "Pause" : "Play"}</button>
        <span style={{ width: 8 }}></span>
        <span className="label">Op</span>
        <span className="mono">{pos} / {steps.length}</span>
        <span className="spacer"></span>
        <span className="muted" style={{ fontSize: 12 }}>
          Backwards applies <b style={{ color: "var(--ink)" }}>invert(op)</b> — never a snapshot restore
        </span>
      </div>

      <div className="split3">
        <main className="docpane">
          {doc ? (
            <>
              <div className="blk h1">{doc.title || "Untitled"}</div>
              {doc.blocks.map((b) => (
                <div key={b.id} className={`${blkClass(b.kind.tag)} ${b.id === touched ? "touched" : ""}`.trim()}>
                  {b.text || <span className="muted">(empty)</span>}
                </div>
              ))}
            </>
          ) : (
            <div className="muted">Empty document — no ops applied yet.</div>
          )}

          <div className="note" style={{ maxWidth: "36rem" }}>
            <b>Really real.</b> Every op above is applied and inverted by real Go
            (<code>internal/session</code>), and the badge in the header re-checks{" "}
            <code>apply(invert(op), apply(op, doc)) == doc</code> on every single step. Step
            backwards and you are watching inverses run, not a saved copy being restored.
          </div>
        </main>

        <aside className="oplist">
          {steps.map((s, i) => {
            const { kind, detail } = describeOp(s.op.op);
            return (
              <div
                key={s.op.id}
                className={`oprow ${i + 1 === pos ? "here" : ""} ${i >= pos ? "future" : ""}`.trim()}
                onClick={() => setPos(i + 1)}
              >
                <span className="who" style={{ background: colorFor(s.op.actor_id, actorOrder) }}></span>
                <div>
                  <div><span className="kind">{kind}</span></div>
                  {detail && <div className="arg">{detail}</div>}
                  <div className="arg" style={{ marginTop: 2 }}>
                    {s.law_holds ? "law holds" : "law FAILED"}
                  </div>
                </div>
              </div>
            );
          })}
        </aside>
      </div>
    </div>
  );
}
