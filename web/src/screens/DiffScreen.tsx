import { useEffect, useMemo, useState } from "react";
import { Link, useParams } from "react-router-dom";
import { useAuth } from "../auth/AuthContext";
import { getTrace, getDiff, type TraceStep, type Move } from "../api/history";
import { diffTokens, tokenizeChars, tokenizeWords, type DiffResult } from "../diff-core/wasm";

type Granularity = "char" | "word";
type View = "text" | "matrix" | "ops";

/**
 * docs/ui-mockups/diff.html, made real (v2.4.0): before/after are two
 * real revisions (GET .../diff, backed by internal/session.Trace),
 * block moves/adds/removes are real (a MoveBlock filter over the
 * confirmed log, plus a plain before/after set difference — no
 * heuristic), and the LCS diff itself — the DP table, its traceback, and
 * the traceback PATH the matrix view outlines — all run in real Go
 * compiled to wasm (services/textdiff via document-service/cmd/diffwasm),
 * recomputed live when the granularity toggle changes. Nothing here
 * re-derives the algorithm in TypeScript (ADR-012); this file tokenizes
 * text and renders what Go already computed.
 */
export function DiffScreen() {
  const { id: pageId } = useParams();
  const { logout } = useAuth();

  const [steps, setSteps] = useState<TraceStep[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [from, setFrom] = useState(0);
  const [to, setTo] = useState(0);
  const [blockId, setBlockId] = useState<string | null>(null);
  const [granularity, setGranularity] = useState<Granularity>("word");
  const [view, setView] = useState<View>("text");
  const [moves, setMoves] = useState<Move[]>([]);
  const [diff, setDiff] = useState<DiffResult | null>(null);

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

  const beforeStep = steps?.[from];
  const afterStep = steps?.[to];
  const beforeText = beforeStep?.after.blocks.find((b) => b.id === blockId)?.text ?? "";
  const afterText = afterStep?.after.blocks.find((b) => b.id === blockId)?.text ?? "";

  useEffect(() => {
    const tokenize = granularity === "word" ? tokenizeWords : tokenizeChars;
    diffTokens(tokenize(beforeText), tokenize(afterText))
      .then(setDiff)
      .catch(() => setDiff(null));
  }, [beforeText, afterText, granularity]);

  const blockRows = useMemo(() => {
    if (!beforeStep || !afterStep) return [];
    const movedIds = new Set(moves.map((m) => m.block_id));
    const beforeIds = new Set(beforeStep.after.blocks.map((b) => b.id));
    const afterIds = new Set(afterStep.after.blocks.map((b) => b.id));
    const rows: { id: string; text: string; kind: "unchanged" | "moved" | "added" | "removed"; op: string }[] = [];
    for (const b of afterStep.after.blocks) {
      if (!beforeIds.has(b.id)) {
        rows.push({ id: b.id, text: b.text, kind: "added", op: "InsertBlock" });
      } else if (movedIds.has(b.id)) {
        rows.push({ id: b.id, text: b.text, kind: "moved", op: "MoveBlock" });
      } else {
        rows.push({ id: b.id, text: b.text, kind: "unchanged", op: "unchanged" });
      }
    }
    for (const b of beforeStep.after.blocks) {
      if (!afterIds.has(b.id)) rows.push({ id: b.id, text: b.text, kind: "removed", op: "DeleteBlock" });
    }
    return rows;
  }, [beforeStep, afterStep, moves]);

  const insertions = diff?.ops.filter((op) => op.kind === "insert").length ?? 0;
  const deletions = diff?.ops.filter((op) => op.kind === "delete").length ?? 0;
  const lcsLength = diff ? diff.table[diff.table.length - 1][diff.table[0].length - 1] : 0;
  const onPath = useMemo(() => new Set(diff?.path.map((c) => `${c.i},${c.j}`) ?? []), [diff]);
  const aTokens = useMemo(() => (granularity === "word" ? tokenizeWords(beforeText) : tokenizeChars(beforeText)), [beforeText, granularity]);
  const bTokens = useMemo(() => (granularity === "word" ? tokenizeWords(afterText) : tokenizeChars(afterText)), [afterText, granularity]);

  if (error) return <div className="app"><div className="note" style={{ margin: 24, maxWidth: "none" }}>{error}</div></div>;
  if (!steps) return <div className="app"><div className="muted" style={{ padding: 24 }}>Loading…</div></div>;

  const blocksInScope = steps[to]?.after.blocks ?? [];

  return (
    <div className="app">
      <header className="topbar">
        <Link to="/pages" className="brand" style={{ textDecoration: "none" }}>
          <span className="mark"></span>Marginal
        </Link>
        <nav className="nav">
          {pageId && <Link to={`/pages/${pageId}`}>Editor</Link>}
          {pageId && <Link to={`/pages/${pageId}/history`}>History</Link>}
          {pageId && <Link to={`/pages/${pageId}/trace`}>Op trace</Link>}
          <Link to={`/pages/${pageId}/diff`} aria-current="page">Diff</Link>
        </nav>
        <div className="crumb">Product · <b>{afterStep?.after.title || "Untitled"}</b></div>
        <div className="spacer"></div>
        {pageId && <Link className="btn" to={`/pages/${pageId}/history`}>Back to scrubber</Link>}
        <button className="btn" onClick={logout}>Sign out</button>
      </header>

      <div className="diffbar">
        <span className="label">Granularity</span>
        <div className="seg2">
          <button aria-pressed={granularity === "char"} onClick={() => setGranularity("char")}>Character</button>
          <button aria-pressed={granularity === "word"} onClick={() => setGranularity("word")}>Word</button>
        </div>
        <span style={{ width: 10 }}></span>
        <span className="label">Show</span>
        <div className="seg2">
          <button aria-pressed={view === "text"} onClick={() => setView("text")}>Text</button>
          <button aria-pressed={view === "matrix"} onClick={() => setView("matrix")}>DP table</button>
          <button aria-pressed={view === "ops"} onClick={() => setView("ops")}>Operations</button>
        </div>
        <span style={{ width: 10 }}></span>
        <select value={blockId ?? ""} onChange={(e) => setBlockId(e.target.value)}>
          {blocksInScope.map((b) => (
            <option key={b.id} value={b.id}>{(b.text || "(empty block)").slice(0, 40)}</option>
          ))}
        </select>
        <label className="muted" style={{ fontSize: 12, marginLeft: 8 }}>
          From <input type="range" min={0} max={steps.length - 1} value={from} onChange={(e) => setFrom(Math.min(Number(e.target.value), to))} /> op {from}
        </label>
        <label className="muted" style={{ fontSize: 12 }}>
          To <input type="range" min={0} max={steps.length - 1} value={to} onChange={(e) => setTo(Math.max(Number(e.target.value), from))} /> op {to}
        </label>
        <span className="spacer"></span>
        <span className="muted mono">{insertions} ins · {deletions} del</span>
      </div>

      <div className="body-row">
        <main className="canvas">
          {view === "text" && (
            <>
              <div className="cols">
                <div className="col">
                  <div className="col-h"><span className="t">op {from}</span><span className="pill">{new Date(beforeStep!.op.created_at).toLocaleString()}</span></div>
                  <div className="prose-d">
                    {diff?.ops.filter((op) => op.kind !== "insert").map((op, i) => (
                      <span key={i} className={op.kind === "delete" ? "del" : undefined}>
                        {op.token}{granularity === "word" ? " " : ""}
                      </span>
                    ))}
                  </div>
                </div>
                <div className="col">
                  <div className="col-h"><span className="t">op {to}</span><span className="pill teal">{to === steps.length - 1 ? "current" : new Date(afterStep!.op.created_at).toLocaleString()}</span></div>
                  <div className="prose-d">
                    {diff?.ops.filter((op) => op.kind !== "delete").map((op, i) => (
                      <span key={i} className={op.kind === "insert" ? "ins" : undefined}>
                        {op.token}{granularity === "word" ? " " : ""}
                      </span>
                    ))}
                  </div>
                </div>
              </div>

              <div style={{ padding: "0 24px 26px" }}>
                <div className="panel-h" style={{ marginTop: 22 }}>Block structure</div>
                <div className="blocks">
                  {blockRows.map((r) => (
                    <div key={r.id} className={`bl ${r.kind !== "unchanged" ? r.kind : ""}`.trim()}>
                      {r.kind === "moved" && <span className="arrow">↕</span>}
                      <span>{r.text || "(empty block)"}</span>
                      <span className="op">{r.op}</span>
                    </div>
                  ))}
                </div>

                <div className="note" style={{ maxWidth: "none" }}>
                  <b>A moved block reads as MOVED, not delete + insert.</b> <code>MoveBlock</code>{" "}
                  carries <code>from</code> as well as <code>to</code>, so the log already knows —
                  this is a filter over the confirmed op log (<code>GET /collab/pages/id/diff</code>),
                  never a heuristic guessing that two blocks are "the same."
                </div>
              </div>
            </>
          )}

          {view === "matrix" && diff && (
            <div style={{ padding: "22px 24px 30px" }}>
              <div className="panel-h">LCS table — every cell computed by real Go, traceback path outlined</div>
              <div className="matrix">
                <table className="dp">
                  <thead>
                    <tr>
                      <th></th>
                      <th></th>
                      {bTokens.map((tok, j) => <th key={j}>{tok}</th>)}
                    </tr>
                  </thead>
                  <tbody>
                    {diff.table.map((row, i) => (
                      <tr key={i}>
                        <th>{i === 0 ? "" : aTokens[i - 1]}</th>
                        {row.map((cell, j) => {
                          const isMatch = i > 0 && j > 0 && aTokens[i - 1] === bTokens[j - 1];
                          const isOn = onPath.has(`${i},${j}`);
                          return (
                            <td key={j} className={`${isOn ? "on" : ""} ${isMatch ? "match" : ""}`.trim()}>
                              {cell}
                            </td>
                          );
                        })}
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
              <div className="note" style={{ maxWidth: "none" }}>
                <b>This is the algorithm, not a picture of it.</b> Every cell above is{" "}
                <code>services/textdiff.LCSTable</code>'s own real output; the outlined path is{" "}
                <code>TracebackWithPath</code>'s own real traceback, not re-derived here. It's also
                the argument against using it at document scale: the table is <b>O(n·m)</b> cells —
                fine for one block, ruinous for a whole document, which is why history should use
                Myers' <b>O(nd)</b> instead.
              </div>
            </div>
          )}

          {view === "ops" && diff && (
            <div style={{ padding: "22px 24px 30px" }}>
              <div className="panel-h">The edit script, as operations</div>
              <div className="blocks">
                {diff.ops.map((op, i) => (
                  <div key={i} className={`bl ${op.kind === "insert" ? "added" : op.kind === "delete" ? "removed" : ""}`.trim()}>
                    <span>{op.token}</span>
                    <span className="op">{op.kind}</span>
                  </div>
                ))}
              </div>
              <div className="note" style={{ maxWidth: "none" }}>
                Derived from the real traceback above. In the real system these are read from the op
                log rather than inferred — this view exists so a human can read a range of ops, not
                so the system has to work out what changed.
              </div>
            </div>
          )}
        </main>

        <aside className="rail right">
          <div className="rail-head"><span className="label">This diff</span></div>
          <div className="panel-body">
            <div className="metric"><span>Tokens compared</span><span className="v">{aTokens.length + bTokens.length}</span></div>
            <div className="metric"><span>Common subsequence</span><span className="v">{lcsLength}</span></div>
            <div className="metric"><span>Insertions</span><span className="v" style={{ color: "var(--teal)" }}>{insertions}</span></div>
            <div className="metric"><span>Deletions</span><span className="v" style={{ color: "var(--amber)" }}>{deletions}</span></div>
            <div className="metric"><span>DP cells filled</span><span className="v">{(aTokens.length + 1) * (bTokens.length + 1)}</span></div>

            <div className="panel-section">
              <div className="panel-h">Block moves</div>
              {moves.length === 0 && <div className="muted" style={{ padding: "8px 0", fontSize: 12.5 }}>No blocks moved between op {from} and op {to}.</div>}
              {moves.map((m, i) => (
                <div className="row" key={i}>
                  <span className="lead">↕</span>
                  block {m.block_id.slice(0, 8)}
                  <span className="muted" style={{ marginLeft: "auto", fontSize: 11 }}>step {m.step}</span>
                </div>
              ))}
            </div>
          </div>
        </aside>
      </div>
    </div>
  );
}
