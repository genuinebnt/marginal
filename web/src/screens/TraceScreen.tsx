/**
 * docs/ui-mockups/v2/index.html § 13 TRACE, ported.
 *
 * A debugger for the op log. Every step here is real: the ops are the page's
 * actual collab.ops rows, the inverse beside each one is what
 * documentcore.Op.Invert() returned, and the document shown is the state
 * after applying that op — not a rendered guess.
 *
 * The invertibility law is RE-CHECKED per step by the server rather than
 * asserted here (`law_holds` on each step), which is the whole reason this
 * screen is worth having: `apply(invert(op), apply(op, doc)) == doc` is the
 * property the op model rests on, and a UI that displays it without
 * verifying it would be a decoration.
 *
 * Stepping backwards runs inverses. It never restores a snapshot — the
 * distinction the rail states out loud, and the one that makes per-actor
 * undo possible at all.
 */
import { useEffect, useMemo, useState } from "react";
import { TracePlayground } from "./TracePlayground";
import { useParams } from "react-router-dom";
import { getTrace, describeOp, type TraceStep } from "../api/history";
import {
  Body, Inspector, Label, Readout, Rule, Screen, StatusBar, TopBar, num,
} from "../shell/Chrome";

/** Pretty-prints an op as the mockup's own brace-block, from real fields. */
function OpBody({ op }: { op: Record<string, unknown> }) {
  const entries = Object.entries(op).filter(([k]) => k !== "scope");
  return (
    <div className="mono" style={{ fontSize: 11.5, lineHeight: 1.8, color: "inherit" }}>
      {"{"}
      {entries.map(([k, v]) => (
        <div key={k} style={{ paddingLeft: 14 }}>
          {k}: {typeof v === "object" && v !== null ? JSON.stringify(v).slice(0, 46) : String(v).slice(0, 46)},
        </div>
      ))}
      {"}"}
    </div>
  );
}

export function TraceScreen() {
  const { id } = useParams();

  const [steps, setSteps] = useState<TraceStep[]>([]);
  const [at, setAt] = useState(0);
  const [err, setErr] = useState<string | null>(null);
  const [insTab, setInsTab] = useState<"law" | "kinds">("law");

  useEffect(() => {
    if (!id) return;
    setSteps([]); setErr(null);
    getTrace(id)
      .then((r) => { setSteps(r.steps); setAt(Math.max(r.steps.length - 1, 0)); })
      .catch((e) => setErr(String(e.message ?? e)));
  }, [id]);

  const step = steps[at] ?? null;
  const doc = step?.after ?? null;

  const chars = useMemo(
    () => (doc?.blocks ?? []).reduce((n, b) => n + b.text.length, 0),
    [doc],
  );
  const marks = useMemo(
    () => (doc?.blocks ?? []).reduce((n, b) => n + (b.marks?.length ?? 0), 0),
    [doc],
  );

  // A single failing step makes the whole trace suspect, so the readout
  // reports the trace, not just the step you happen to be looking at.
  const allHold = steps.every((s) => s.law_holds);

  /** Op kinds in this log, commonest first — the KINDS tab's histogram.
   *  Over the whole replayed log rather than the step you are on, because
   *  "what is in this document's history" is not a per-step question. */
  const kindCounts = useMemo(() => {
    const m = new Map<string, number>();
    for (const s of steps) {
      const k = describeOp(s.op.op).kind;
      m.set(k, (m.get(k) ?? 0) + 1);
    }
    return [...m].sort((a, b) => b[1] - a[1]);
  }, [steps]);

  if (!id) {
    return <TracePlayground />;
  }

  return (
    <Screen>
      <TopBar
        crumb={<>lab / <b>trace</b></>}
        readouts={
          <>
            <Readout k="STEP" v={`${num(steps.length === 0 ? 0 : at + 1)} / ${num(steps.length)}`} />
            <Readout
              k="INVERTIBILITY"
              v={steps.length === 0 ? "—" : allHold ? "HOLDS" : "FAILS"}
              tone={allHold ? "#3FCFA8" : "#E0A34E"}
            />
          </>
        }
        right={
          <div style={{ display: "flex", gap: 6 }}>
            <span
              className="chip"
              style={{ cursor: at > 0 ? "pointer" : "default", opacity: at > 0 ? 1 : 0.4 }}
              onClick={() => setAt((v) => Math.max(0, v - 1))}
              title="Steps back by APPLYING THE INVERSE — never by restoring a snapshot"
            >
              ◀ STEP
            </span>
            <span
              className="chip chip-e"
              style={{ cursor: at < steps.length - 1 ? "pointer" : "default", opacity: at < steps.length - 1 ? 1 : 0.4 }}
              onClick={() => setAt((v) => Math.min(steps.length - 1, v + 1))}
            >
              STEP ▶
            </span>
          </div>
        }
      />

      <Body>
        <div className="rail" style={{ width: 330 }}>
          <div className="rail-h">
            OP LOG<div /><span style={{ color: "#585550" }}>{steps.length}</span>
          </div>
          <div style={{ display: "flex", flexDirection: "column", padding: "0 8px", gap: 1, overflowY: "auto", flex: 1 }}>
            {steps.length === 0 && !err && (
              <div style={{ padding: 10, fontSize: 11.5, color: "#585550", lineHeight: 1.6 }}>
                This page has no ops yet. An empty log is a real state — the page was
                created and never edited.
              </div>
            )}
            {steps.map((s, i) => {
              const { kind, detail } = describeOp(s.op.op);
              const current = i === at;
              return (
                <div
                  key={s.op.id}
                  onClick={() => setAt(i)}
                  style={{
                    position: "relative",
                    padding: current ? "8px 10px 8px 24px" : "8px 10px",
                    background: current ? "#181A1B" : undefined,
                    display: "flex", gap: 9, alignItems: "baseline", cursor: "pointer",
                  }}
                >
                  {current && (
                    <div style={{ position: "absolute", left: 0, top: 4, bottom: 4, width: 2, background: "#E8873C" }} />
                  )}
                  <span className="mono" style={{ fontSize: 9, color: current ? "#E8873C" : "#4B4842" }}>
                    {i + 1}
                  </span>
                  <span className="mono" style={{
                    fontSize: 10.5,
                    color: current ? "#E4E2DC" : i < at ? "#6E6A63" : "#585550",
                  }}>
                    {kind} {detail}
                  </span>
                  {!s.law_holds && (
                    <span className="mono" style={{ marginLeft: "auto", fontSize: 9, color: "#E0A34E" }}>
                      ✕
                    </span>
                  )}
                </div>
              );
            })}
          </div>
          <div className="wal">
            <Label>STEPPING BACK RUNS INVERSES</Label>
            <span style={{ fontSize: 11, color: "#585550", lineHeight: 1.55 }}>
              It never restores a snapshot.
            </span>
          </div>
        </div>

        <div style={{ flex: 1, minWidth: 0, display: "flex", flexDirection: "column", overflow: "hidden" }}>
          <div style={{
            flex: 1, padding: "30px 40px", borderBottom: "1px solid rgba(255,255,255,.07)",
            minHeight: 0, overflowY: "auto",
          }}>
            <Label>
              DOCUMENT AT STEP {steps.length === 0 ? 0 : at + 1}
            </Label>
            {err && (
              <div style={{ fontSize: 12.5, color: "#E0A34E", lineHeight: 1.7 }}>
                ◌ {err}
              </div>
            )}
            <div style={{ fontFamily: "Spectral,serif", fontSize: 19, lineHeight: 1.7, color: "#D2CFC8" }}>
              {(doc?.blocks ?? []).map((b) => (
                <div key={b.id} style={{ marginBottom: 10 }}>
                  {b.text || <span style={{ color: "#4B4842" }}>(empty {b.kind.tag})</span>}
                </div>
              ))}
              {doc && doc.blocks.length === 0 && (
                <span style={{ color: "#4B4842", fontSize: 15 }}>(no blocks at this step)</span>
              )}
            </div>
            {doc && (
              <div style={{ marginTop: 22, display: "flex", gap: 26 }}>
                <Readout k="BLOCKS" v={num(doc.blocks.length)} />
                <Readout k="CHARS" v={num(chars)} />
                <Readout k="MARKS" v={num(marks)} />
              </div>
            )}
          </div>

          <div style={{ padding: "22px 40px" }}>
            <Label>THIS OP AND ITS INVERSE</Label>
            {step ? (
              <div style={{ display: "flex", gap: 16 }}>
                <div style={{
                  flex: 1, border: "1px solid rgba(232,135,60,.3)",
                  background: "rgba(232,135,60,.05)", padding: "13px 15px", color: "#E4E2DC",
                }}>
                  <div className="mono" style={{ fontSize: 9, letterSpacing: ".16em", color: "#E8873C", marginBottom: 8 }}>
                    APPLIED
                  </div>
                  <OpBody op={step.op.op as Record<string, unknown>} />
                </div>
                <div style={{
                  flex: 1, border: "1px solid rgba(255,255,255,.09)",
                  padding: "13px 15px", color: "#9B968D",
                }}>
                  <div className="mono" style={{ fontSize: 9, letterSpacing: ".16em", color: "#6E6A63", marginBottom: 8 }}>
                    INVERSE
                  </div>
                  <OpBody op={step.inverse as Record<string, unknown>} />
                </div>
              </div>
            ) : (
              <span style={{ fontSize: 11.5, color: "#585550" }}>Nothing to step through.</span>
            )}
          </div>
        </div>

        <Inspector
          tabs={[{ id: "law", label: "LAW" }, { id: "kinds", label: "KINDS" }]}
          active={insTab}
          onSelect={(id) => setInsTab(id as "law" | "kinds")}
        >
        {insTab === "kinds" ? (
          <div style={{ display: "flex", flexDirection: "column", gap: 12 }}>
            <Label>OP KINDS IN THIS LOG</Label>
            {kindCounts.length === 0 ? (
              <div style={{ fontSize: 11.5, color: "#8C8880" }}>No ops replayed yet.</div>
            ) : kindCounts.map(([kind, n]) => (
              <div key={kind} style={{ display: "flex", alignItems: "center", gap: 9 }}>
                <span className="mono" style={{ flex: 1, fontSize: 10.5, color: "#D2CFC8" }}>{kind}</span>
                <div style={{ width: 64, height: 4, background: "rgba(255,255,255,.06)" }}>
                  <div style={{
                    width: `${(n / kindCounts[0][1]) * 100}%`, height: "100%",
                    background: /Delete/.test(kind) ? "#E0A34E" : "#7AA8E8",
                  }} />
                </div>
                <span className="mono" style={{ fontSize: 9.5, color: "#8C8880", width: 30, textAlign: "right" }}>{n}</span>
              </div>
            ))}
            <div style={{ height: 1, background: "rgba(255,255,255,.07)" }} />
            <Label>TWO TIERS, ONE LOG</Label>
            <div style={{ fontSize: 11.5, lineHeight: 1.65, color: "#8C8880" }}>
              Kinds are prefixed <span className="mono" style={{ color: "#9B968D" }}>block:</span>{" "}
              or <span className="mono" style={{ color: "#9B968D" }}>text:</span> — RFC-002 §2's
              two ISA tiers. Structure and characters share one log, one flush pipeline and
              one broadcast, which is why replay reproduces both together rather than
              reconciling them afterwards.
            </div>
          </div>
        ) : (
          <>
          <Label>RE-CHECKED THIS STEP</Label>
          <div style={{
            border: `1px solid ${step?.law_holds === false ? "rgba(224,163,78,.4)" : "rgba(63,207,168,.35)"}`,
            background: step?.law_holds === false ? "rgba(224,163,78,.06)" : "rgba(63,207,168,.06)",
            padding: "12px 13px",
          }}>
            <div className="mono" style={{ fontSize: 11, lineHeight: 1.7, color: "#D2CFC8" }}>
              apply(invert(op),<br />&nbsp;&nbsp;apply(op, doc))<br />&nbsp;&nbsp;== doc
            </div>
            <div style={{ display: "flex", alignItems: "center", gap: 8, marginTop: 9 }}>
              <span className={`chip ${step?.law_holds === false ? "chip-a" : "chip-t"}`}>
                {step ? (step.law_holds ? "HOLDS" : "FAILS") : "—"}
              </span>
              <span className="mono" style={{ fontSize: 10, color: "#585550" }}>
                step {steps.length === 0 ? 0 : at + 1} of {steps.length}
              </span>
            </div>
          </div>
          <div style={{ fontSize: 11, lineHeight: 1.6, color: "#585550" }}>
            Verified server-side per step, not asserted here. A screen that displayed this
            law without running it would be a decoration.
          </div>

          <Rule />
          <Label>ACTOR</Label>
          {step && (
            <div className="mono" style={{ fontSize: 10.5, lineHeight: 1.9, color: "#8C8880" }}>
              actor&nbsp;&nbsp;&nbsp;&nbsp;{step.op.actor_id.slice(0, 8)}<br />
              kind&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;{step.op.actor_kind}<br />
              group&nbsp;&nbsp;&nbsp;&nbsp;{step.op.undo_group?.slice(0, 8) ?? "—"}<br />
              at&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;{new Date(step.op.created_at).toLocaleTimeString("en-GB")}
            </div>
          )}
          <div style={{ fontSize: 11, lineHeight: 1.6, color: "#585550" }}>
            undo_group is why one paste undoes as one action rather than forty. NULL
            degrades to a group of one, which is why it could be added later and was not.
          </div>
          </>
        )}
        </Inspector>
      </Body>

      <StatusBar
        route={`/pages/${id}/trace`}
        mechanism="apply and invert run for real, per step"
        state={
          err ? "op log unavailable"
            : steps.length === 0 ? "no ops on this page"
            : allHold ? `${num(steps.length)} ops · law holds throughout`
            : "law fails on at least one step"
        }
        healthy={!err && allHold}
      />
    </Screen>
  );
}

export default TraceScreen;
