/**
 * docs/ui-mockups/v2/index.html § 15 DIFF, ported.
 *
 * The rendered matrix IS the computed table. Both come from
 * marginal/textdiff in Go (compiled to wasm): the LCS table, its traceback,
 * and the edit script are one call, and the ember path is exactly the cells
 * Go visited — never re-derived here.
 *
 * The screen's own argument, kept: it shows the O(n·m) table AND argues
 * against using it. Exposing the cost is the point — a diff view that hides
 * its quadratic table teaches nothing about why Myers exists.
 */
import { useCallback, useEffect, useMemo, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { useAuth } from "../auth/AuthContext";
import { getDiff, getTrace, type Move, type Snapshot } from "../api/history";
import { listPages, type Page } from "../api/pages";
import { diffTokens, tokenizeChars, tokenizeWords, type DiffResult } from "../diff-core/wasm";
import {
  Body, Inspector, Label, Readout, Rule, Screen, StatusBar, TopBar, num,
} from "../shell/Chrome";

type Granularity = "word" | "char";

export function DiffScreen() {
  const { id } = useParams();
  const { session } = useAuth();
  const actorId = session?.actorId ?? null;
  const navigate = useNavigate();

  const [pages, setPages] = useState<Page[]>([]);
  const [total, setTotal] = useState(0);
  const [from, setFrom] = useState(0);
  const [to, setTo] = useState(0);
  const [before, setBefore] = useState<Snapshot | null>(null);
  const [after, setAfter] = useState<Snapshot | null>(null);
  const [moves, setMoves] = useState<Move[]>([]);
  const [gran, setGran] = useState<Granularity>("word");
  const [result, setResult] = useState<DiffResult | null>(null);
  const [err, setErr] = useState<string | null>(null);

  useEffect(() => {
    if (!actorId) return;
    listPages(actorId).then((r) => setPages(r.pages)).catch(() => {});
  }, [actorId]);

  // Revision bounds come from the op log's own length.
  useEffect(() => {
    if (!id) return;
    getTrace(id)
      .then((r) => {
        // from/to are step INDICES into the trace, so the last valid one is
        // length-1. Passing the count reads as "one past the end" and the
        // endpoint rejects it — which it should.
        const last = Math.max(r.steps.length - 1, 0);
        setTotal(r.steps.length);
        setFrom(Math.max(0, last - 6));
        setTo(last);
      })
      .catch((e) => setErr(String(e.message ?? e)));
  }, [id]);

  const load = useCallback(() => {
    if (!id || to === 0) return;
    getDiff(id, from, to)
      .then((r) => { setBefore(r.before); setAfter(r.after); setMoves(r.moves); setErr(null); })
      .catch((e) => setErr(String(e.message ?? e)));
  }, [id, from, to]);

  useEffect(load, [load]);

  /** Flatten each side to one string — the diff is over prose, not structure;
   *  structural change is what `moves` reports separately. */
  const beforeText = useMemo(() => (before?.blocks ?? []).map((b) => b.text).join(" "), [before]);
  const afterText = useMemo(() => (after?.blocks ?? []).map((b) => b.text).join(" "), [after]);

  useEffect(() => {
    if (!beforeText && !afterText) { setResult(null); return; }
    const tok = gran === "word" ? tokenizeWords : tokenizeChars;
    // Tokenising is the only non-Go step here, and only because splitting a
    // string is not diffing it. The table, traceback and script are all Go.
    let cancelled = false;
    diffTokens(tok(beforeText), tok(afterText))
      .then((r) => { if (!cancelled) setResult(r); })
      .catch(() => setResult(null));
    return () => { cancelled = true; };
  }, [beforeText, afterText, gran]);

  const onPath = useMemo(() => {
    const s = new Set<string>();
    (result?.path ?? []).forEach((c) => s.add(`${c.i},${c.j}`));
    return s;
  }, [result]);

  const rows = result?.table.length ?? 0;
  const cols = result?.table[0]?.length ?? 0;
  // A full quadratic table is unreadable past a certain size and pointless to
  // paint — the claim is that the matrix IS the computation, not that every
  // cell must be on screen.
  const CAP = 26;
  const showTable = rows > 0 && rows <= CAP && cols <= CAP;

  if (!id) {
    return (
      <Screen>
        <TopBar crumb={<>lab / <b>diff</b></>} />
        <Body>
          <div className="rail">
            <div className="rail-h">PICK A PAGE<div /></div>
            <div style={{ display: "flex", flexDirection: "column", gap: 1, padding: "0 8px", overflowY: "auto" }}>
              {pages.map((p) => (
                <div key={p.id} className="tr" style={{ cursor: "pointer" }}
                     onClick={() => navigate(`/pages/${p.id}/diff`)}>
                  <span className="tr-t">{p.title}</span>
                </div>
              ))}
            </div>
          </div>
          <div style={{ flex: 1, display: "grid", placeItems: "center", padding: 40 }}>
            <div style={{ maxWidth: 520, fontSize: 12.5, lineHeight: 1.7, color: "#585550" }}>
              A diff is between two revisions of one page, so it needs a page. Both sides are
              replayed from the op log — neither is a stored snapshot.
            </div>
          </div>
        </Body>
        <StatusBar route="/lab/diff" mechanism="LCS in Go, via wasm" state="no page selected" healthy />
      </Screen>
    );
  }

  return (
    <Screen>
      <TopBar
        crumb={<>lab / <b>diff</b> · rev {num(from)} → {num(to)}</>}
        readouts={
          <>
            <Readout k="GRANULARITY" v={gran.toUpperCase()} />
            <Readout k="TABLE" v={rows ? `${num(rows)} × ${num(cols)}` : "—"} />
            <Readout k="COST" v="O(n·m)" tone="#E0A34E" />
          </>
        }
        right={
          <div style={{ display: "flex", gap: 6 }}>
            <span className={`chip${gran === "word" ? " chip-e" : ""}`}
                  style={{ cursor: "pointer" }} onClick={() => setGran("word")}>WORD</span>
            <span className={`chip${gran === "char" ? " chip-e" : ""}`}
                  style={{ cursor: "pointer" }} onClick={() => setGran("char")}>CHAR</span>
          </div>
        }
      />

      <Body>
        <div style={{ flex: 1, minWidth: 0, display: "flex", flexDirection: "column", overflow: "hidden" }}>
          <div style={{ display: "flex", flex: 1, minHeight: 0 }}>
            <div style={{ flex: 1, padding: "22px 26px", borderRight: "1px solid rgba(255,255,255,.07)", minWidth: 0, overflowY: "auto" }}>
              <div style={{ display: "flex", alignItems: "baseline", gap: 10, marginBottom: 14 }}>
                <Label>REV {num(from)}</Label>
                <span className="mono" style={{ fontSize: 10, color: "#585550" }}>
                  {before ? `${before.blocks.length} blocks` : "—"}
                </span>
              </div>
              <div style={{ fontFamily: "Spectral,serif", fontSize: 15.5, lineHeight: 1.75, color: "#8C8880" }}>
                {(result?.ops ?? []).filter((o) => o.kind !== "insert").map((o, i) => (
                  <span
                    key={i}
                    style={o.kind === "delete"
                      ? { background: "rgba(224,163,78,.14)", color: "#E0A34E", textDecoration: "line-through" }
                      : undefined}
                  >
                    {o.token}{gran === "word" ? " " : ""}
                  </span>
                ))}
                {!result && <span style={{ color: "#585550", fontSize: 12.5 }}>Nothing at this revision.</span>}
              </div>
            </div>

            <div style={{ flex: 1, padding: "22px 26px", minWidth: 0, overflowY: "auto" }}>
              <div style={{ display: "flex", alignItems: "baseline", gap: 10, marginBottom: 14 }}>
                <Label>REV {num(to)}</Label>
                <span className="mono" style={{ fontSize: 10, color: "#585550" }}>
                  {after ? `${after.blocks.length} blocks` : "—"}
                </span>
              </div>
              <div style={{ fontFamily: "Spectral,serif", fontSize: 15.5, lineHeight: 1.75, color: "#D2CFC8" }}>
                {(result?.ops ?? []).filter((o) => o.kind !== "delete").map((o, i) => (
                  <span
                    key={i}
                    style={o.kind === "insert"
                      ? { background: "rgba(63,207,168,.14)", color: "#3FCFA8" }
                      : undefined}
                  >
                    {o.token}{gran === "word" ? " " : ""}
                  </span>
                ))}
              </div>
            </div>
          </div>

          <div style={{ borderTop: "1px solid rgba(255,255,255,.07)", padding: "20px 26px", maxHeight: 340, overflowY: "auto" }}>
            <div style={{ display: "flex", alignItems: "baseline", gap: 12, marginBottom: 14 }}>
              <Label>LCS TABLE — THE RENDERED MATRIX IS THE COMPUTED TABLE</Label>
              <span className="mono" style={{ fontSize: 10, color: "#585550" }}>traceback in ember</span>
            </div>

            {!showTable && rows > 0 && (
              <div style={{ fontSize: 11.5, color: "#585550", lineHeight: 1.6, maxWidth: 620 }}>
                {num(rows)} × {num(cols)} = {num(rows * cols)} cells. The table is computed in
                full — that is the cost being demonstrated — but past {CAP} tokens a side it is
                not legible, so it is not painted. Switch to a narrower revision range to see it.
              </div>
            )}

            {showTable && (
              <div style={{ display: "flex", flexDirection: "column", gap: 2 }}>
                {result!.table.map((row, i) => (
                  <div key={i} style={{ display: "flex", gap: 2 }}>
                    {row.map((v, j) => {
                      const lit = onPath.has(`${i},${j}`);
                      return (
                        <div
                          key={j}
                          style={{
                            width: 20, height: 16, flex: "none",
                            display: "flex", alignItems: "center", justifyContent: "center",
                            font: "400 9px 'IBM Plex Mono',monospace",
                            background: lit ? "rgba(232,135,60,.18)" : "rgba(255,255,255,.03)",
                            color: lit ? "#E8873C" : v === 0 ? "#3A3833" : "#6E6A63",
                          }}
                        >
                          {v}
                        </div>
                      );
                    })}
                  </div>
                ))}
              </div>
            )}
          </div>
        </div>

        <Inspector
          tabs={[{ id: "script", label: "EDIT SCRIPT" }, { id: "moves", label: "MOVES" }]}
          active="script"
        >
          <Label>REVISION RANGE</Label>
          <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
            <input
              type="range" min={0} max={Math.max(total - 1, 0)} value={from}
              onChange={(e) => setFrom(Math.min(Number(e.target.value), to))}
              style={{ flex: 1, accentColor: "#E8873C" }}
            />
            <span className="mono" style={{ fontSize: 10, color: "#8C8880", width: 28, textAlign: "right" }}>{from}</span>
          </div>
          <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
            <input
              type="range" min={0} max={Math.max(total - 1, 0)} value={to}
              onChange={(e) => setTo(Math.max(Number(e.target.value), from))}
              style={{ flex: 1, accentColor: "#E8873C" }}
            />
            <span className="mono" style={{ fontSize: 10, color: "#8C8880", width: 28, textAlign: "right" }}>{to}</span>
          </div>

          <Rule />
          <Label>EDIT SCRIPT</Label>
          <div className="mono" style={{ fontSize: 10.5, lineHeight: 1.9, color: "#8C8880" }}>
            equal&nbsp;&nbsp;&nbsp;{num((result?.ops ?? []).filter((o) => o.kind === "equal").length)}<br />
            <span style={{ color: "#E0A34E" }}>delete&nbsp;&nbsp;{num((result?.ops ?? []).filter((o) => o.kind === "delete").length)}</span><br />
            <span style={{ color: "#3FCFA8" }}>insert&nbsp;&nbsp;{num((result?.ops ?? []).filter((o) => o.kind === "insert").length)}</span><br />
            visited&nbsp;{num(result?.path.length ?? 0)}
          </div>

          <Rule />
          <Label>BLOCK MOVES · {num(moves.length)}</Label>
          {moves.length === 0 ? (
            <div style={{ fontSize: 11, lineHeight: 1.6, color: "#585550" }}>
              No blocks moved. This is a filter over the op log's own MoveBlock ops, not a
              heuristic over the text — a move either happened or it did not.
            </div>
          ) : (
            <div style={{ display: "flex", flexDirection: "column", gap: 6 }}>
              {moves.map((m, i) => (
                <div key={i} className="mono" style={{ fontSize: 10, color: "#8C8880" }}>
                  {m.block_id.slice(0, 6)} · step {m.step}
                </div>
              ))}
            </div>
          )}

          <Rule />
          <div style={{ fontSize: 11, lineHeight: 1.6, color: "#585550" }}>
            The full O(n·m) table is computed and shown, then argued against. A diff view that
            hides its quadratic cost teaches nothing about why Myers exists — so the cost is a
            readout rather than a footnote.
          </div>
        </Inspector>
      </Body>

      <StatusBar
        route={`/pages/${id}/diff`}
        mechanism="LCS table and traceback in Go, via wasm"
        state={
          err ? "diff unavailable"
            : rows === 0 ? "nothing to compare"
            : `${num(rows * cols)} cells · ${num(result?.path.length ?? 0)} on the path`
        }
        healthy={!err}
      />
    </Screen>
  );
}

export default DiffScreen;
