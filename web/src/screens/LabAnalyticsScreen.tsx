/**
 * docs/ui-mockups/v2/index.html § 12 ANALYTICS, ported — and editable.
 *
 * Three sketches over one stream: HyperLogLog for distinct readers, Count-Min
 * for heavy pages, a t-digest for session length. Each is shown beside the
 * EXACT answer and the gap between them, because a sketch that hides its error
 * is indistinguishable from a wrong number.
 *
 * The stream is a textarea, like § 11's buffer and for the same reason. It is
 * also the fastest way to see what a sketch actually is: duplicate a line and
 * watch the cardinality estimate refuse to move, which is the entire argument
 * in one gesture. Paste a few hundred distinct actors and watch the estimate
 * drift outside 64 registers' error bound, which is the other half of it.
 *
 * Every number comes from one `sketch.Analyze` call in Go (wasm) — including
 * the exact answers it is measured against. Computing the truth in TypeScript
 * and the estimate in Go would make this a comparison of two languages rather
 * than of a sketch against its own input.
 */
import { useEffect, useMemo, useState } from "react";
import { analyze, type Report } from "../sketch-core/wasm";
import {
  Body, Label, Main, Readout, Rule, Screen, StatusBar, TopBar, TOPIC_HEX, num,
} from "../shell/Chrome";

const SAMPLE = [
  "ana, sync-protocol-notes, protocol, 252000, crdt rope",
  "bo, block-model, storage, 41000, blocks",
  "ana, anchors-vs-offsets, protocol, 123000, anchors crdt",
  "cy, crdt-survey, research, 510000, crdt",
  "bo, operation-model, protocol, 78000, undo",
  "di, bubble-menu, interface, 22000, marks",
  "cy, sync-protocol-notes, protocol, 190000, crdt rope",
  "eve, block-model, storage, 63000, blocks anchors",
  "ana, sync-protocol-notes, protocol, 88000, crdt rope",
  "fin, wal-and-flush, operations, 34000, wal",
  "di, anchors-vs-offsets, protocol, 210000, anchors",
  "eve, crdt-survey, research, 470000, crdt",
  "bo, sync-protocol-notes, protocol, 145000, crdt rope",
  "gil, block-model, storage, 29000, blocks",
  "cy, undo-stack, interface, 51000, undo marks",
  "ana, wal-and-flush, operations, 96000, wal",
].join("\n");

/** ms → the "6 m" / "41 s" form § 12 prints.
 *
 *  "No median" is a real state and prints as an em dash. An empty digest
 *  arrives as 0 rather than NaN — JSON has no NaN, so Go cannot send one —
 *  which makes 0 ambiguous on this side. The caller passes undefined when
 *  the digest is empty, so a genuine 0 ms still prints as 0. */
function dur(ms: number | undefined): string {
  if (ms === undefined || Number.isNaN(ms)) return "—";
  if (ms < 60000) return `${Math.round(ms / 1000)} s`;
  return `${Math.round(ms / 60000)} m`;
}

function pct(n: number): string {
  const sign = n > 0 ? "+" : n < 0 ? "−" : "";
  return `${sign}${Math.abs(n).toFixed(0)}%`;
}

/** Ember at full strength for the tallest register, fading down from there —
 *  the mockup's ramp, driven by the actual register values. */
function registerFill(v: number, max: number): string {
  if (max === 0) return "rgba(232,135,60,.28)";
  if (v === max) return "#E8873C";
  return `rgba(232,135,60,${(0.28 + 0.24 * (v / max)).toFixed(2)})`;
}

const TOPIC_ORDER = ["protocol", "storage", "research", "interface", "operations"];

export function LabAnalyticsScreen() {
  const [src, setSrc] = useState(SAMPLE);
  const [report, setReport] = useState<Report | null>(null);
  const [err, setErr] = useState<string | null>(null);

  // Recomputed on every keystroke, not debounced — three sketches over a text
  // box are microseconds, and a delay between typing and the panels moving is
  // exactly the feedback this screen exists to give.
  useEffect(() => {
    let cancelled = false;
    analyze(src)
      .then((r) => { if (!cancelled) { setReport(r); setErr(null); } })
      .catch((e) => { if (!cancelled) setErr(String(e)); });
    return () => { cancelled = true; };
  }, [src]);

  const maxRegister = useMemo(
    () => Math.max(0, ...(report?.hll_registers ?? [])),
    [report],
  );
  const maxHeavy = useMemo(
    () => Math.max(1, ...(report?.heavy ?? []).map((h) => h.estimate)),
    [report],
  );

  // Inside its own bound is the structure working; outside it is a finding.
  // The screen colours the error by which of those it is rather than by
  // whether the number looks small.
  const withinBound =
    report !== null && report.hll_error_pct <= report.hll_standard_error * 3;

  const topics = useMemo(() => {
    const rows = report?.by_topic ?? [];
    return [...rows].sort(
      (a, b) => TOPIC_ORDER.indexOf(a.topic) - TOPIC_ORDER.indexOf(b.topic),
    );
  }, [report]);

  // The t-digest's own centroids, drawn as the density they are: x is the
  // running quantile, y the weight at it. Not a decorative curve — moving a
  // long session in the buffer moves this line.
  const curve = useMemo(() => {
    const cs = report?.centroids ?? [];
    if (cs.length < 2) return "";
    const total = cs.reduce((s, c) => s + c.weight, 0);
    const maxW = Math.max(...cs.map((c) => c.weight));
    let seen = 0;
    return cs
      .map((c, i) => {
        seen += c.weight;
        const x = (seen / total) * 360;
        const y = 130 - (c.weight / maxW) * 100;
        return `${i === 0 ? "M" : "L"}${x.toFixed(1)} ${y.toFixed(1)}`;
      })
      .join(" ");
  }, [report]);

  const hasDigest = (report?.centroids.length ?? 0) > 0;
  const quantileX = (q: number) => q * 360;

  return (
    <Screen>
      <TopBar
        crumb={<>lab / <b>analytics</b></>}
        readouts={
          <>
            <Readout k="STREAM" v={`${num(report?.events ?? 0)} events`} />
            <Readout
              k="MEMORY"
              v={`${((report?.total_bytes ?? 0) / 1024).toFixed(1)} KB`}
              tone="#3FCFA8"
            />
          </>
        }
      />

      <Body style={{ flexDirection: "column", padding: "26px 32px", overflow: "hidden" }}>
        <Main>
          <div style={{ display: "flex", gap: 26, flex: 1, minHeight: 0 }}>
            {/* Column one — the two counting sketches. */}
            <div style={{ flex: 1, display: "flex", flexDirection: "column", gap: 22, minWidth: 0 }}>
              <div>
                <div style={{ display: "flex", alignItems: "baseline", gap: 12, marginBottom: 12 }}>
                  <Label>HYPERLOGLOG · UNIQUE READERS</Label>
                  <span className="mono" style={{ fontSize: 10, color: "#585550" }}>
                    {report?.hll_registers.length ?? 64} registers
                  </span>
                </div>
                {/* The registers themselves. Each bar is one 6-bit counter —
                    the leading-zero run of some actor's hash. This IS the
                    data structure, not a chart of its output. */}
                <div
                  className="hll-regs"
                  style={{ display: "flex", alignItems: "flex-end", gap: 2, height: 56, marginBottom: 12 }}
                >
                  {(report?.hll_registers ?? Array(64).fill(0)).map((v, i) => (
                    <div
                      key={i}
                      title={`register ${i} · ${v}`}
                      style={{
                        flex: 1,
                        height: `${maxRegister ? Math.max(6, (v / maxRegister) * 100) : 6}%`,
                        background: registerFill(v, maxRegister),
                        transition: "height .22s cubic-bezier(.2,.7,.3,1), background .22s ease",
                      }}
                    />
                  ))}
                </div>
                <div style={{ display: "flex", gap: 28, flexWrap: "wrap" }}>
                  <Readout k="ESTIMATE" v={num(Math.round(report?.hll_estimate ?? 0))} size={19} />
                  <Readout k="EXACT" v={num(report?.hll_exact ?? 0)} size={19} tone="#6E6A63" />
                  <Readout
                    k="ERROR"
                    v={`${(report?.hll_error_pct ?? 0).toFixed(1)}%`}
                    size={19}
                    tone={withinBound ? "#3FCFA8" : "#E0A34E"}
                  />
                  <Readout
                    k="ITS OWN BOUND"
                    v={`±${(report?.hll_standard_error ?? 0).toFixed(1)}%`}
                    size={19}
                    tone="#6E6A63"
                  />
                </div>
              </div>

              <Rule />

              <div>
                <div style={{ display: "flex", alignItems: "baseline", gap: 12, marginBottom: 12 }}>
                  <Label>COUNT-MIN · HEAVY PAGES</Label>
                  <span className="mono" style={{ fontSize: 10, color: "#585550" }}>
                    {report?.cm_depth ?? 4} × {report?.cm_width ?? 24} table
                  </span>
                </div>
                <div style={{ display: "flex", flexDirection: "column", gap: 5 }}>
                  {(report?.heavy ?? []).slice(0, 6).map((h) => (
                    <div key={h.key} className="cm-row" style={{ display: "flex", gap: 2, alignItems: "center" }}>
                      <div style={{
                        width: 130, fontSize: 11.5, color: "#D2CFC8",
                        overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap",
                      }}>
                        {h.key}
                      </div>
                      <div style={{ flex: 1, height: 14, background: "rgba(255,255,255,.05)" }}>
                        <div style={{
                          width: `${(h.estimate / maxHeavy) * 100}%`,
                          height: "100%",
                          background: h.estimate > h.exact ? "rgba(224,163,78,.75)" : "#E8873C",
                          transition: "width .25s cubic-bezier(.2,.7,.3,1)",
                        }} />
                      </div>
                      <span className="mono" style={{
                        fontSize: 10, color: h.estimate > h.exact ? "#E0A34E" : "#8C8880",
                        width: 82, textAlign: "right",
                      }}>
                        {h.estimate} / {h.exact}
                      </span>
                    </div>
                  ))}
                  {!report?.heavy.length && (
                    <div className="mono" style={{ fontSize: 11, color: "#585550" }}>
                      no parseable events — every line was skipped
                    </div>
                  )}
                </div>
                <div className="mono" style={{ fontSize: 10, color: "#585550", marginTop: 10 }}>
                  sketch / exact · overestimate only, never under —{" "}
                  <span style={{ color: (report?.cm_under_estimates ?? 0) === 0 ? "#3FCFA8" : "#E0A34E" }}>
                    {report?.cm_under_estimates ?? 0} underestimates
                  </span>
                  , {report?.cm_over_estimates ?? 0} over
                </div>
              </div>
            </div>

            {/* Column two — the digest, and why any of this. */}
            <div style={{ width: 380, flex: "none", display: "flex", flexDirection: "column", gap: 18 }}>
              <div>
                <div style={{ display: "flex", alignItems: "baseline", gap: 12, marginBottom: 12 }}>
                  <Label>T-DIGEST · SESSION LENGTH</Label>
                </div>
                <svg viewBox="0 0 360 150" style={{ width: "100%", display: "block" }}>
                  <g stroke="rgba(255,255,255,.07)">
                    <line x1="0" y1="130" x2="360" y2="130" />
                    <line x1="0" y1="90" x2="360" y2="90" />
                    <line x1="0" y1="50" x2="360" y2="50" />
                  </g>
                  {curve && (
                    <path
                      d={curve}
                      fill="none"
                      stroke="#E8873C"
                      strokeWidth="1.6"
                      style={{ transition: "d .25s ease" }}
                    />
                  )}
                  <g stroke="rgba(63,207,168,.55)" strokeDasharray="3 3">
                    {[0.5, 0.95, 0.99].map((q) => (
                      <line key={q} x1={quantileX(q)} y1="20" x2={quantileX(q)} y2="130" />
                    ))}
                  </g>
                  <g fontFamily="IBM Plex Mono" fontSize="9" fill="#6E6A63">
                    <text x={quantileX(0.5) - 12} y="146">p50</text>
                    <text x={quantileX(0.95) - 12} y="146">p95</text>
                    <text x={quantileX(0.99) - 20} y="146">p99</text>
                  </g>
                </svg>
                <div style={{ display: "flex", gap: 20, marginTop: 10, flexWrap: "wrap" }}>
                  <Readout k="P50" v={dur(hasDigest ? report?.p50 : undefined)} />
                  <Readout k="P95" v={dur(hasDigest ? report?.p95 : undefined)} />
                  <Readout k="P99" v={dur(hasDigest ? report?.p99 : undefined)} />
                  <Readout k="CENTROIDS" v={String(report?.centroids.length ?? 0)} tone="#3FCFA8" />
                </div>
                {/* The exact quantiles, beside the estimated ones. Same rule as
                    the other two panels: the comparison is the point. */}
                <div className="mono" style={{ fontSize: 10, color: "#585550", marginTop: 8 }}>
                  exact · {dur(hasDigest ? report?.exact_p50 : undefined)}{" / "}
                  {dur(hasDigest ? report?.exact_p95 : undefined)}{" / "}
                  {dur(hasDigest ? report?.exact_p99 : undefined)}
                  {" · "}
                  {num(report?.tdigest_bytes ?? 0)} B held
                </div>
              </div>

              <Rule />

              <div>
                <Label>WHY SKETCHES</Label>
                <div style={{ fontSize: 12, lineHeight: 1.7, color: "#8C8880", marginTop: 10 }}>
                  No per-person row is stored, so there is nothing to leak and nothing
                  to subpoena. Each sketch is displayed beside its exact answer and its
                  error — a sketch that hides its error is indistinguishable from a
                  wrong number.
                </div>
              </div>

              <div style={{ marginTop: "auto", display: "flex", gap: 20, flexWrap: "wrap" }}>
                <Readout k="RETENTION" v="counters only" />
                <Readout k="PII" v="none" tone="#3FCFA8" />
                <Readout
                  k="VS RAW ROWS"
                  v={`${num(report?.exact_bytes ?? 0)} B`}
                  tone="#6E6A63"
                />
              </div>
            </div>

            {/* Column three — of what, and the input all of it reads. */}
            <div style={{ width: 392, flex: "none", display: "flex", flexDirection: "column", gap: 20, minHeight: 0 }}>
              <div>
                <div style={{ display: "flex", alignItems: "baseline", gap: 10, marginBottom: 12 }}>
                  <Label>READS BY TOPIC</Label>
                  <span className="mono" style={{ fontSize: 10, color: "#585550" }}>
                    one HLL per topic · vs the buffer's first half
                  </span>
                </div>
                <div style={{ display: "flex", height: 9, gap: 1, marginBottom: 11 }}>
                  {topics.map((t) => (
                    <div
                      key={t.topic}
                      style={{
                        flex: Math.max(1, t.exact),
                        background: TOPIC_HEX[t.topic] ?? "#7AA8E8",
                        transition: "flex-grow .25s ease",
                      }}
                    />
                  ))}
                </div>
                <div style={{ display: "flex", flexDirection: "column", gap: 7 }}>
                  {topics.map((t) => (
                    <div key={t.topic} style={{ display: "flex", alignItems: "center", gap: 9 }}>
                      <span style={{
                        width: 6, height: 6,
                        background: TOPIC_HEX[t.topic] ?? "#7AA8E8",
                      }} />
                      <span style={{ flex: 1, fontSize: 12, color: "#D2CFC8" }}>
                        {t.topic.charAt(0).toUpperCase() + t.topic.slice(1)}
                      </span>
                      <span className="mono" style={{ fontSize: 10.5, color: "#8C8880" }}>
                        {Math.round(t.estimate)}
                      </span>
                      <span className="mono" style={{
                        fontSize: 9.5, width: 42, textAlign: "right",
                        color: t.delta_pct > 0 ? "#3FCFA8" : t.delta_pct < 0 ? "#E0A34E" : "#8C8880",
                      }}>
                        {pct(t.delta_pct)}
                      </span>
                    </div>
                  ))}
                  {!topics.length && (
                    <div className="mono" style={{ fontSize: 11, color: "#585550" }}>
                      no topics in the stream — the third field
                    </div>
                  )}
                </div>
              </div>

              <Rule />

              <div>
                <div style={{ display: "flex", alignItems: "baseline", gap: 10, marginBottom: 12 }}>
                  <Label>TAG MOMENTUM</Label>
                  <span className="mono" style={{ fontSize: 10, color: "#585550" }}>
                    reads per tag · second half vs first
                  </span>
                </div>
                <div style={{ display: "flex", flexDirection: "column", gap: 9 }}>
                  {(report?.momentum ?? []).map((m) => {
                    const mag = Math.min(100, Math.abs(m.delta_pct));
                    const up = m.delta_pct > 0;
                    return (
                      <div key={m.tag} style={{ display: "flex", alignItems: "center", gap: 10 }}>
                        <span className="tg" style={{ width: 88, justifyContent: "flex-start" }}>{m.tag}</span>
                        <div style={{ flex: 1, display: "flex", alignItems: "center", height: 14 }}>
                          <div style={{ width: "50%", display: "flex", justifyContent: "flex-end" }}>
                            <div style={{
                              width: up ? "0%" : `${mag}%`, height: 5,
                              background: "rgba(224,163,78,.55)",
                              transition: "width .25s cubic-bezier(.2,.7,.3,1)",
                            }} />
                          </div>
                          <div style={{ width: 1, height: 14, background: "rgba(255,255,255,.14)" }} />
                          <div style={{ width: "50%" }}>
                            <div style={{
                              width: up ? `${mag}%` : "0%", height: 5,
                              background: "#3FCFA8",
                              transition: "width .25s cubic-bezier(.2,.7,.3,1)",
                            }} />
                          </div>
                        </div>
                        <span className="mono" style={{
                          fontSize: 9.5, width: 38, textAlign: "right",
                          color: up ? "#3FCFA8" : m.delta_pct < 0 ? "#E0A34E" : "#8C8880",
                        }}>
                          {pct(m.delta_pct)}
                        </span>
                      </div>
                    );
                  })}
                  {!report?.momentum.length && (
                    <div className="mono" style={{ fontSize: 11, color: "#585550" }}>
                      no tags in the stream — the fifth field, space-separated
                    </div>
                  )}
                </div>
                <div style={{ fontSize: 11, lineHeight: 1.6, color: "#585550", marginTop: 11 }}>
                  Momentum is reads, not writes. A tag can climb while nobody edits it —
                  which is precisely the week to go and check whether what it points at
                  is still true.
                </div>
              </div>

              <Rule />

              <div style={{ flex: 1, minHeight: 0, display: "flex", flexDirection: "column" }}>
                <div style={{ display: "flex", alignItems: "baseline", gap: 10, marginBottom: 11 }}>
                  <Label>STREAM · EDITABLE</Label>
                  <span className="mono" style={{ fontSize: 10, color: "#585550" }}>
                    the one input all three sketches read
                  </span>
                </div>
                <textarea
                  className="labedit"
                  style={{ flex: 1, minHeight: 90, fontSize: 10.5, lineHeight: 1.8 }}
                  value={src}
                  spellCheck={false}
                  onChange={(e) => setSrc(e.target.value)}
                  aria-label="Event stream"
                />
                <div className="mono" style={{ fontSize: 10, color: "#585550", marginTop: 8 }}>
                  actor, page, topic, ms, tags ·{" "}
                  <span style={{ color: (report?.skipped ?? 0) > 0 ? "#E0A34E" : "#585550" }}>
                    {report?.skipped ?? 0} skipped
                  </span>
                  , not fatal
                </div>
                <div style={{
                  marginTop: "auto", paddingTop: 14, display: "flex", gap: 20, flexWrap: "wrap",
                  borderTop: "1px solid rgba(255,255,255,.07)",
                }}>
                  <Readout k="EVENTS" v={num(report?.events ?? 0)} />
                  <Readout
                    k="SKETCH MEMORY"
                    v={`${((report?.total_bytes ?? 0) / 1024).toFixed(1)} KB`}
                    tone="#3FCFA8"
                  />
                  <Readout k="ROWS STORED" v="0" tone="#3FCFA8" />
                </div>
              </div>
            </div>
          </div>

          {err && (
            <div className="mono" style={{ fontSize: 11, color: "#E0A34E", marginTop: 10 }}>
              {err}
            </div>
          )}
        </Main>
      </Body>

      <StatusBar
        route="/lab/analytics"
        mechanism="HLL · Count-Min · t-digest over one editable stream"
        state={`${((report?.total_bytes ?? 0) / 1024).toFixed(1)} KB holds all three`}
      />
    </Screen>
  );
}
