import { useEffect, useMemo, useState } from "react";
import { Link, useParams } from "react-router-dom";
import { useAuth } from "../auth/AuthContext";
import { getTrace, getDiff, type TraceStep, type Move } from "../api/history";
import { diffTokens, tokenizeChars, tokenizeWords, type DiffResult } from "../diff-core/wasm";

/**
 * docs/ui-mockups/diff.html, made real (v2.4.0): before/after are two
 * real revisions (GET .../diff, backed by internal/session.Trace),
 * moves are real MoveBlock ops the op log already recorded (no
 * heuristic), and the LCS diff itself — the DP table plus its traceback
 * — runs in real Go compiled to wasm (services/textdiff via
 * document-service/cmd/diffwasm), recomputed live when the granularity
 * toggle changes. Nothing here re-derives the algorithm in TypeScript
 * (ADR-012); this file tokenizes text and renders what Go already
 * computed.
 */
export function DiffScreen() {
  const { id: pageId } = useParams();
  const { logout } = useAuth();

  const [steps, setSteps] = useState<TraceStep[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [from, setFrom] = useState(0);
  const [to, setTo] = useState(0);
  const [blockId, setBlockId] = useState<string | null>(null);
  const [granularity, setGranularity] = useState<"word" | "char">("word");
  const [moves, setMoves] = useState<Move[]>([]);
  const [diff, setDiff] = useState<DiffResult | null>(null);
  const [showTable, setShowTable] = useState(false);

  useEffect(() => {
    if (!pageId) return;
    getTrace(pageId)
      .then((r) => {
        setSteps(r.steps);
        if (r.steps.length > 1) {
          setFrom(0);
          setTo(r.steps.length - 1);
        }
      })
      .catch(() => setError("Couldn't load this page's op log."));
  }, [pageId]);

  useEffect(() => {
    if (!pageId || !steps || from > to) return;
    let cancelled = false;
    getDiff(pageId, from, to).then((r) => {
      if (cancelled) return;
      setMoves(r.moves);
      if (!blockId) {
        const firstShared = r.after.blocks.find((b) => r.before.blocks.some((bb) => bb.id === b.id)) ?? r.after.blocks[0];
        if (firstShared) setBlockId(firstShared.id);
      }
    });
    return () => {
      cancelled = true;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [pageId, from, to, steps]);

  const beforeText = useMemo(() => {
    if (!steps || from >= steps.length) return "";
    return steps[from].after.blocks.find((b) => b.id === blockId)?.text ?? "";
  }, [steps, from, blockId]);
  const afterText = useMemo(() => {
    if (!steps || to >= steps.length) return "";
    return steps[to].after.blocks.find((b) => b.id === blockId)?.text ?? "";
  }, [steps, to, blockId]);

  useEffect(() => {
    const tokenize = granularity === "word" ? tokenizeWords : tokenizeChars;
    diffTokens(tokenize(beforeText), tokenize(afterText))
      .then(setDiff)
      .catch(() => setDiff(null));
  }, [beforeText, afterText, granularity]);

  if (error) return <div className="app"><div className="note" style={{ margin: 24 }}>{error}</div></div>;
  if (!steps) return <div className="app"><div className="muted" style={{ padding: 24 }}>Loading…</div></div>;

  const blocksInScope = steps[to]?.after.blocks ?? [];

  return (
    <div className="app">
      <header className="topbar">
        <Link to="/pages" className="brand" style={{ textDecoration: "none" }}>
          <span className="mark"></span>Marginal
        </Link>
        <nav className="nav">
          {pageId && <Link to={`/pages/${pageId}/history`}>History</Link>}
          {pageId && <Link to={`/pages/${pageId}/trace`}>Trace</Link>}
          <Link to={`/pages/${pageId}/diff`} aria-current="page">Diff</Link>
        </nav>
        <div className="crumb">Product · <b>Diff</b></div>
        <div className="spacer"></div>
        <button className="btn" onClick={logout}>Sign out</button>
      </header>

      <div className="diffbar" style={{ display: "flex", alignItems: "center", gap: 10, padding: "10px 20px", flexWrap: "wrap" }}>
        <label className="muted" style={{ fontSize: 12 }}>
          From <input type="range" min={0} max={steps.length - 1} value={from} onChange={(e) => setFrom(Math.min(Number(e.target.value), to))} />
          op {from}
        </label>
        <label className="muted" style={{ fontSize: 12 }}>
          To <input type="range" min={0} max={steps.length - 1} value={to} onChange={(e) => setTo(Math.max(Number(e.target.value), from))} />
          op {to}
        </label>
        <select value={blockId ?? ""} onChange={(e) => setBlockId(e.target.value)}>
          {blocksInScope.map((b) => (
            <option key={b.id} value={b.id}>{(b.text || "(empty block)").slice(0, 40)}</option>
          ))}
        </select>
        <div className="seg2">
          <button className={`btn ${granularity === "word" ? "primary" : ""}`} onClick={() => setGranularity("word")}>Word</button>
          <button className={`btn ${granularity === "char" ? "primary" : ""}`} onClick={() => setGranularity("char")}>Character</button>
        </div>
        <button className="btn" onClick={() => setShowTable((v) => !v)}>{showTable ? "Hide DP table" : "Show DP table"}</button>
      </div>

      <div className="fsplit" style={{ display: "grid", gridTemplateColumns: "1fr 320px", flex: 1, minHeight: 0 }}>
        <main className="pane" style={{ padding: 24, overflowY: "auto" }}>
          <div className="label" style={{ marginBottom: 9 }}>Rendered diff</div>
          <p style={{ fontFamily: "var(--serif)", fontSize: 15, lineHeight: 1.7, maxWidth: "42rem" }}>
            {diff?.ops.map((op, i) => {
              if (op.kind === "equal") return <span key={i}>{op.token}</span>;
              if (op.kind === "delete") return <span key={i} className="diff-del" style={{ background: "color-mix(in srgb, var(--amber) 20%, transparent)", textDecoration: "line-through" }}>{op.token}</span>;
              return <span key={i} className="diff-add" style={{ background: "color-mix(in srgb, var(--teal) 20%, transparent)" }}>{op.token}</span>;
            })}
          </p>

          {showTable && diff && (
            <div style={{ overflowX: "auto", marginTop: 20 }}>
              <div className="label" style={{ marginBottom: 9 }}>DP table (real LCS values, from services/textdiff)</div>
              <table style={{ borderCollapse: "collapse", fontFamily: "var(--mono)", fontSize: 11 }}>
                <tbody>
                  {diff.table.map((row, i) => (
                    <tr key={i}>
                      {row.map((cell, j) => (
                        <td key={j} style={{ border: "1px solid var(--rule)", padding: "2px 6px", textAlign: "center" }}>{cell}</td>
                      ))}
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
          )}

          <div className="note" style={{ maxWidth: "36rem", marginTop: 20 }}>
            <b>Really real.</b> The LCS table and its traceback both run in Go, compiled to wasm — this
            page never recomputes the algorithm, only tokenizes text and draws what came back.
          </div>
        </main>

        <aside className="rail right">
          <div className="rail-head"><span className="label">Block moves</span></div>
          <div className="panel-body">
            {moves.length === 0 && <div className="muted" style={{ padding: "8px 0", fontSize: 12.5 }}>No blocks moved between op {from} and op {to}.</div>}
            {moves.map((m, i) => (
              <div className="row" key={i}>
                <span className="lead">↕</span>
                block {m.block_id.slice(0, 8)}
                <span className="muted" style={{ marginLeft: "auto", fontSize: 11 }}>step {m.step}</span>
              </div>
            ))}
            <div className="note" style={{ margin: "12px 0 0", maxWidth: "none" }}>
              A moved block reads as MOVED here — MoveBlock carries <code>from</code> and{" "}
              <code>to</code>, so this is a filter over the op log, not a flat-text diff guessing at
              a delete+insert pair.
            </div>
          </div>
        </aside>
      </div>
    </div>
  );
}
