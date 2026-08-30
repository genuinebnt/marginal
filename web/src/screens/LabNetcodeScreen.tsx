/**
 * docs/ui-mockups/v2/index.html § 14 NETCODE, ported — and driven.
 *
 * Four lenses over one op log: prediction and rollback, a Merkle
 * comparison of the two replicas, the causal DAG, and the log's LSM
 * shape. They are four views of ONE system, not four systems, which is
 * the only reason a disagreement between them means anything.
 *
 * You control the wire. RTT, loss and jitter are sliders, transform is a
 * toggle, and the edit script is a textarea — every one of them re-runs
 * the whole simulation. The argument the page exists to make needs that:
 * turn transform off and the two replicas still agree perfectly, on a
 * document nobody asked for. The structural digest is happy; the intent
 * ledger is not. Two instruments, disagreeing on purpose.
 *
 * Everything below comes from one `netsim.Run` call in Go (wasm) — the
 * transform, the rollback, the Merkle tree, the DAG, the LSM shape and
 * the replay check. This file sets four controls and draws the result.
 *
 * It is a SIMULATION and the status bar says so. `collaboration-service`
 * is the real engine — live ropes, a real WAL, real sockets. What this
 * adds is the one thing a live service cannot: a deterministic,
 * re-runnable 400 ms of a 4%-loss network with every layer visible at
 * once.
 */
import { useEffect, useMemo, useState } from "react";
import { runSim, type Report, type MerkleNode } from "../netsim-core/wasm";
import {
  Body, Inspector, Label, Main, Readout, Rule, Screen, StatusBar, SubBar,
  SubItem, TopBar, num,
} from "../shell/Chrome";

const INITIAL = "A rope is the wrong primitive here.";

// Chosen so every position is legible in the base text: 10 is the
// "t" of "the", 20 is the "p" of "primitive", and 2..7 is "rope ".
// Concurrent by construction — Ada types 40 ms in, a full round
// trip before she could possibly have heard about the insert at 10,
// which is exactly the case the transform exists for.
const SCRIPT = [
  "0, you, insert, 10, quite ",
  "40, ada, insert, 20, addressable ",
  "120, you, delete, 2, 5",
].join("\n");

type Lens = "prediction" | "merkle" | "causality" | "lsm";

const LENS_LABEL: Record<Lens, string> = {
  prediction: "PREDICTION · ROLLBACK",
  merkle: "TREE · MERKLE",
  causality: "CAUSALITY · DAG",
  lsm: "LOG · LSM",
};

/** A slider row. The value is the input's, so dragging it is the
 *  only way it can move — no separate display state to drift. */
function WireSlider({
  k, value, min, max, suffix, onChange,
}: {
  k: string; value: number; min: number; max: number;
  suffix?: string; onChange: (v: number) => void;
}) {
  return (
    <div style={{ display: "flex", alignItems: "center", gap: 9 }}>
      <span className="rd-k" style={{ width: 52 }}>{k}</span>
      <input
        className="wsl"
        type="range"
        min={min}
        max={max}
        value={value}
        aria-label={k}
        onChange={(e) => onChange(Number(e.target.value))}
      />
      <span className="mono" style={{ fontSize: 10, color: "#8C8880" }}>
        {value}{suffix ?? ""}
      </span>
    </div>
  );
}

export function LabNetcodeScreen() {
  const [script, setScript] = useState(SCRIPT);
  const [rtt, setRtt] = useState(180);
  const [loss, setLoss] = useState(4);
  const [jitter, setJitter] = useState(40);
  const [transform, setTransform] = useState(true);
  const [seed, setSeed] = useState(7);
  const [lens, setLens] = useState<Lens>("prediction");
  const [insTab, setInsTab] = useState<"wire" | "invariants">("wire");
  const [report, setReport] = useState<Report | null>(null);
  const [err, setErr] = useState<string | null>(null);

  // Re-run on every control change. The whole simulation is a few
  // thousand ticks of integer arithmetic — far cheaper than the
  // render it triggers — so debouncing it would only add lag between
  // dragging a slider and seeing what it did.
  useEffect(() => {
    let cancelled = false;
    runSim({
      script,
      wire: { rtt_ms: rtt, loss_pct: loss, jitter_ms: jitter, seed },
      transform,
      initial: INITIAL,
    })
      .then((r) => { if (!cancelled) { setReport(r); setErr(null); } })
      .catch((e) => { if (!cancelled) setErr(String(e)); });
    return () => { cancelled = true; };
  }, [script, rtt, loss, jitter, transform, seed]);

  const you = report?.replicas.find((r) => r.actor === "you") ?? report?.replicas[0];
  const them = report?.replicas.find((r) => r !== you) ?? null;
  const violations = report?.intent_violations ?? [];

  // The tree, laid out by depth so the drawing is the structure —
  // an SVG whose node positions came from anywhere else would be a
  // picture of a Merkle tree rather than this one.
  const merkleLayout = useMemo(() => {
    const nodes = report?.merkle.nodes ?? [];
    if (!nodes.length) return [] as Array<MerkleNode & { x: number; y: number; i: number }>;
    const maxDepth = Math.max(...nodes.map((n) => n.depth));
    const perDepth = new Map<number, number>();
    for (const n of nodes) perDepth.set(n.depth, (perDepth.get(n.depth) ?? 0) + 1);
    const seen = new Map<number, number>();
    return nodes.map((n, i) => {
      const idx = seen.get(n.depth) ?? 0;
      seen.set(n.depth, idx + 1);
      const count = perDepth.get(n.depth) ?? 1;
      return {
        ...n, i,
        x: ((idx + 0.5) / count) * 300,
        y: 20 + (n.depth / Math.max(1, maxDepth)) * 105,
      };
    });
  }, [report]);

  const dagLayout = useMemo(() => {
    const nodes = report?.causality.nodes ?? [];
    if (!nodes.length) return [];
    const maxDepth = Math.max(1, ...nodes.map((n) => n.depth));
    const perDepth = new Map<number, number>();
    for (const n of nodes) perDepth.set(n.depth, (perDepth.get(n.depth) ?? 0) + 1);
    const seen = new Map<number, number>();
    return nodes.map((n) => {
      const idx = seen.get(n.depth) ?? 0;
      seen.set(n.depth, idx + 1);
      const count = perDepth.get(n.depth) ?? 1;
      return {
        ...n,
        x: 24 + (n.depth / maxDepth) * 250,
        y: ((idx + 0.5) / count) * 130 + 10,
      };
    });
  }, [report]);

  const maxLevelOps = Math.max(1, ...(report?.lsm.levels ?? []).map((l) => l.ops));

  return (
    <Screen>
      <TopBar
        crumb={<>lab / <b>netcode</b></>}
        // Ada is in the document, so she is in the presence cluster. The
        // second replica pane below is HER — a screen that draws someone's
        // replica and not their avatar reads as a mock-up of two people.
        peers={<div className="av av-them">AD</div>}
        readouts={
          <>
            <Readout k="RTT" v={`${rtt} ms`} />
            <Readout k="LOSS" v={`${loss}%`} tone={loss > 0 ? "#E0A34E" : undefined} />
            <Readout
              k="TRANSFORM"
              v={transform ? "ON" : "OFF"}
              tone={transform ? "#3FCFA8" : "#E0A34E"}
            />
          </>
        }
      />

      <SubBar>
        {(Object.keys(LENS_LABEL) as Lens[]).map((l) => (
          <SubItem key={l} on={lens === l} onClick={() => setLens(l)}>
            {LENS_LABEL[l]}
          </SubItem>
        ))}
        <div style={{ flex: 1 }} />
        <SubItem tone={violations.length ? "#E0A34E" : "#585550"}>
          {violations.length
            ? "2 INSTRUMENTS DISAGREE ON PURPOSE"
            : "BOTH INSTRUMENTS AGREE"}
        </SubItem>
      </SubBar>

      <Body>
        <Main style={{ overflow: "hidden" }}>
          {/* Top half — the two replicas, always shown: they are what
              every lens below is a lens ON. */}
          <div style={{
            display: "flex", flex: 1, minHeight: 0,
            borderBottom: "1px solid rgba(255,255,255,.07)",
          }}>
            <div style={{
              flex: 1, padding: "20px 24px", minWidth: 0,
              borderRight: "1px solid rgba(255,255,255,.07)",
            }}>
              <div style={{ display: "flex", alignItems: "center", gap: 9, marginBottom: 14 }}>
                <div style={{ width: 6, height: 6, background: "#3FCFA8" }} />
                <Label>YOUR REPLICA · PREDICTED</Label>
              </div>
              <div style={{
                fontFamily: "Spectral, serif", fontSize: 16, lineHeight: 1.7,
                color: "#D2CFC8", wordBreak: "break-word",
              }}>
                {you?.text ?? INITIAL}
              </div>
              <div className="mono" style={{
                fontSize: 10.5, lineHeight: 1.9, color: "#8C8880", marginTop: 16,
              }}>
                predicted&nbsp;&nbsp;{you?.predicted ?? 0} ops<br />
                confirmed&nbsp;&nbsp;{you?.confirmed ?? 0} ops<br />
                rolled back&nbsp;&nbsp;
                <span style={{ color: (you?.rolled_back ?? 0) > 0 ? "#E0A34E" : "#8C8880" }}>
                  {you?.rolled_back ?? 0} ops
                </span>
              </div>
            </div>

            <div style={{ flex: 1, padding: "20px 24px", minWidth: 0 }}>
              <div style={{ display: "flex", alignItems: "center", gap: 9, marginBottom: 14 }}>
                <div style={{ width: 6, height: 6, background: "#A98CE8" }} />
                <Label>{(them?.actor ?? "ADA").toUpperCase()}'S REPLICA · CONFIRMED</Label>
              </div>
              <div style={{
                fontFamily: "Spectral, serif", fontSize: 16, lineHeight: 1.7,
                color: "#D2CFC8", wordBreak: "break-word",
              }}>
                {them?.text ?? report?.server_text ?? INITIAL}
              </div>
              <div className="mono" style={{
                fontSize: 10.5, lineHeight: 1.9, color: "#8C8880", marginTop: 16,
              }}>
                transformed&nbsp;&nbsp;{transform ? report?.log.length ?? 0 : 0} ops<br />
                server rev&nbsp;&nbsp;{num(report?.log.length ?? 0)}<br />
                divergence&nbsp;&nbsp;
                <span style={{ color: report?.converged ? "#3FCFA8" : "#E0A34E" }}>
                  {report?.converged ? "none" : "REPLICAS DIFFER"}
                </span>
              </div>
            </div>
          </div>

          {/* Bottom half — the selected lens, and the log beside it. */}
          <div style={{ display: "flex", flex: 1, minHeight: 0 }}>
            <div style={{
              flex: 1, padding: "20px 24px", minWidth: 0, overflow: "hidden",
              borderRight: "1px solid rgba(255,255,255,.07)",
            }}>
              {lens === "merkle" || lens === "prediction" ? (
                <>
                  <Label style={{ marginBottom: 14, display: "block" }}>
                    TREE · MERKLE COMPARISON
                  </Label>
                  <svg viewBox="0 0 300 150" style={{ width: "100%", display: "block" }}>
                    <g stroke="rgba(255,255,255,.12)">
                      {merkleLayout.map((n) =>
                        (n.children ?? []).map((c) => {
                          const child = merkleLayout[c];
                          if (!child) return null;
                          return (
                            <line key={`${n.i}-${c}`}
                              x1={n.x} y1={n.y} x2={child.x} y2={child.y} />
                          );
                        }))}
                    </g>
                    {merkleLayout.map((n) => (
                      <circle
                        key={n.i}
                        cx={n.x} cy={n.y}
                        r={n.depth === 0 ? 7 : n.children ? 6 : 5}
                        fill={n.equal ? "#3FCFA8" : "#E0A34E"}
                      >
                        <title>{`${n.id} · ${n.hash} vs ${n.other_hash}`}</title>
                      </circle>
                    ))}
                    <g fontFamily="IBM Plex Mono" fontSize="9" fill="#E0A34E">
                      {merkleLayout.filter((n) => n.divergence).map((n) => (
                        <text key={`d${n.i}`} x={n.x + 10} y={n.y + 4}>
                          {n.depth === 0 ? "root ≠" : n.id}
                        </text>
                      ))}
                    </g>
                  </svg>
                  <div style={{
                    fontSize: 11.5, color: "#8C8880", marginTop: 10, lineHeight: 1.55,
                  }}>
                    {report?.merkle.equal
                      ? `Equal at the root — one hash settled ${report.merkle.nodes.length} nodes. Agreement is the cheap case.`
                      : "The first amber node is the divergence point — subtree hashes, predicted against confirmed."}
                  </div>
                  <div style={{ display: "flex", gap: 20, marginTop: 12, flexWrap: "wrap" }}>
                    <Readout k="NODES COMPARED" v={String(report?.merkle.compared_nodes ?? 0)} />
                    <Readout k="OF" v={String(report?.merkle.nodes.length ?? 0)} tone="#6E6A63" />
                    <Readout k="LEAF" v={`${report?.merkle.leaf_bytes ?? 0} B`} tone="#6E6A63" />
                  </div>
                </>
              ) : lens === "causality" ? (
                <>
                  <Label style={{ marginBottom: 14, display: "block" }}>
                    CAUSALITY · OP DAG
                  </Label>
                  <svg viewBox="0 0 300 150" style={{ width: "100%", display: "block" }}>
                    <g stroke="rgba(255,255,255,.14)">
                      {dagLayout.map((n) =>
                        (n.deps ?? []).map((d) => {
                          const from = dagLayout.find((m) => m.id === d);
                          if (!from) return null;
                          return (
                            <line key={`${n.id}-${d}`}
                              x1={from.x} y1={from.y} x2={n.x} y2={n.y}
                              stroke={n.on_longest && from.on_longest
                                ? "rgba(232,135,60,.55)" : "rgba(255,255,255,.14)"} />
                          );
                        }))}
                    </g>
                    {dagLayout.map((n) => (
                      <circle key={n.id} cx={n.x} cy={n.y} r={n.on_longest ? 6 : 4.5}
                        fill={n.actor === "you" ? "#3FCFA8" : "#A98CE8"}
                        stroke={n.on_longest ? "#E8873C" : "none"} strokeWidth="1.4">
                        <title>{n.label}</title>
                      </circle>
                    ))}
                  </svg>
                  <div style={{
                    fontSize: 11.5, color: "#8C8880", marginTop: 10, lineHeight: 1.55,
                  }}>
                    The ember path is the longest causal chain — the round trips this
                    session could not have avoided. Everything off it happened
                    concurrently, which is exactly the work the transform had to do.
                  </div>
                  <div style={{ display: "flex", gap: 20, marginTop: 12, flexWrap: "wrap" }}>
                    <Readout k="LONGEST CHAIN" v={`${report?.causality.longest_chain ?? 0} ops`} />
                    <Readout k="CONCURRENT" v={`${report?.causality.concurrent ?? 0} ops`}
                      tone={(report?.causality.concurrent ?? 0) > 0 ? "#E0A34E" : undefined} />
                    <Readout k="WIDTH" v={String(report?.causality.width ?? 0)} tone="#6E6A63" />
                  </div>
                </>
              ) : (
                <>
                  <Label style={{ marginBottom: 14, display: "block" }}>LOG · LSM SHAPE</Label>
                  <div style={{ display: "flex", flexDirection: "column", gap: 6 }}>
                    {(report?.lsm.levels ?? []).map((l) => (
                      <div key={l.name} style={{ display: "flex", alignItems: "center", gap: 10 }}>
                        <span className="mono" style={{ fontSize: 9.5, color: "#585550", width: 52 }}>
                          {l.name}
                        </span>
                        {l.compacting ? (
                          <div style={{ flex: 1, height: 12, background: "rgba(255,255,255,.07)" }} />
                        ) : l.files > 1 ? (
                          <div style={{ flex: 1, height: 12, display: "flex", gap: 2 }}>
                            {Array.from({ length: l.files }).map((_, i) => (
                              <div key={i} style={{
                                flex: 1, background: "rgba(232,135,60,.32)",
                                transition: "background .2s ease",
                              }} />
                            ))}
                          </div>
                        ) : (
                          <div style={{ flex: 1, height: 12, background: "rgba(255,255,255,.07)" }}>
                            <div style={{
                              width: `${(l.ops / maxLevelOps) * 100}%`, height: "100%",
                              background: "rgba(232,135,60,.5)",
                              transition: "width .25s cubic-bezier(.2,.7,.3,1)",
                            }} />
                          </div>
                        )}
                        <span className="mono" style={{ fontSize: 9.5, color: "#8C8880" }}>
                          {l.compacting ? "compacting"
                            : l.name === "memtable" ? `${l.ops} ops` : `${l.files} files`}
                        </span>
                      </div>
                    ))}
                  </div>
                  <div style={{
                    fontSize: 11.5, color: "#8C8880", marginTop: 12, lineHeight: 1.55,
                  }}>
                    A model of the shape, not this repo's storage — <span className="mono">collab.ops</span>{" "}
                    is one Postgres table. The op log has an LSM's exact access pattern
                    (append-only writes, ordered reads, periodic compaction), and the
                    write amplification is what that shape costs.
                  </div>
                  <div style={{ display: "flex", gap: 20, marginTop: 12, flexWrap: "wrap" }}>
                    <Readout k="WRITE AMP"
                      v={`${(report?.lsm.write_amplification ?? 0).toFixed(2)}×`}
                      tone={(report?.lsm.write_amplification ?? 0) > 1 ? "#E0A34E" : undefined} />
                    <Readout k="MEMTABLE" v={`${report?.lsm.memtable_cap ?? 0} ops`} tone="#6E6A63" />
                    <Readout k="FANOUT" v={String(report?.lsm.fanout ?? 0)} tone="#6E6A63" />
                  </div>
                </>
              )}
            </div>

            <div style={{ flex: 1, padding: "20px 24px", minWidth: 0, overflow: "hidden",
              display: "flex", flexDirection: "column" }}>
              <Label style={{ marginBottom: 12, display: "block" }}>
                {lens === "prediction" ? "OPS IN FLIGHT" : "CONFIRMED LOG"}
              </Label>
              <div className="mono" style={{
                fontSize: 10, lineHeight: 1.85, color: "#8C8880",
                overflowY: "auto", flex: 1, minHeight: 0,
              }}>
                {(report?.log ?? []).map((op) => (
                  <div key={op.id} style={{ display: "flex", gap: 8 }}>
                    <span style={{ color: "#4B4842", width: 22 }}>{op.id}</span>
                    <span style={{ color: op.actor === "you" ? "#3FCFA8" : "#A98CE8" }}>
                      {op.actor}
                    </span>
                    <span style={{ flex: 1, color: "#8C8880" }}>
                      {op.kind === "insert"
                        ? `ins @${op.pos} ${JSON.stringify(op.text ?? "")}`
                        : `del @${op.pos} ×${op.len ?? 0}`}
                    </span>
                    <span style={{ color: "#4B4842" }}>base {op.base}</span>
                  </div>
                ))}
                {!report?.log.length && (
                  <div style={{ color: "#585550" }}>
                    no ops — the script has no parseable lines
                  </div>
                )}
              </div>
              <div style={{ marginTop: 14, display: "flex", gap: 20, flexWrap: "wrap" }}>
                <Readout k="REPLAY FROM EMPTY"
                  v={report?.replay_matches ? "MATCHES" : "DIFFERS"}
                  tone={report?.replay_matches ? "#3FCFA8" : "#E0A34E"} />
                <Readout k="CAUSAL CHAIN" v={`${report?.causality.longest_chain ?? 0} ops`} />
                <Readout k="RETRANSMITS" v={String(report?.retransmits ?? 0)}
                  tone={(report?.retransmits ?? 0) > 0 ? "#E0A34E" : undefined} />
              </div>
            </div>
          </div>

          {err && (
            <div className="mono" style={{ fontSize: 11, color: "#E0A34E", padding: "8px 24px" }}>
              {err}
            </div>
          )}
        </Main>

        <Inspector
          tabs={[{ id: "wire", label: "WIRE" }, { id: "invariants", label: "INVARIANTS" }]}
          active={insTab}
          onSelect={(id) => setInsTab(id as "wire" | "invariants")}
        >
          {insTab === "wire" ? (
            <>
              <Label>YOU CONTROL THE WIRE</Label>
              <div style={{ display: "flex", flexDirection: "column", gap: 9 }}>
                <WireSlider k="RTT" value={rtt} min={0} max={500} onChange={setRtt} />
                <WireSlider k="LOSS" value={loss} min={0} max={50} suffix="%" onChange={setLoss} />
                <WireSlider k="JITTER" value={jitter} min={0} max={200} onChange={setJitter} />
              </div>
              <div style={{ display: "flex", gap: 6 }}>
                <span
                  className={`chip${transform ? " chip-e" : ""}`}
                  style={{ cursor: "pointer" }}
                  onClick={() => setTransform((t) => !t)}
                >
                  TRANSFORM {transform ? "ON" : "OFF"}
                </span>
                {/* A different seed is a different bad network, same
                    settings — which is the only way to tell "this run
                    was unlucky" from "this design is wrong". */}
                <span className="chip" style={{ cursor: "pointer" }}
                  onClick={() => setSeed((s) => s + 1)}>
                  RESEED
                </span>
              </div>

              <Rule />
              <Label>THE SCRIPT · EDITABLE</Label>
              <textarea
                className="labedit"
                rows={5}
                style={{ fontSize: 10, lineHeight: 1.75 }}
                value={script}
                spellCheck={false}
                onChange={(e) => setScript(e.target.value)}
                aria-label="Edit script"
              />
              <div className="mono" style={{ fontSize: 9.5, color: "#585550" }}>
                tick, actor, insert|delete, pos, text-or-length
                {(report?.skipped ?? 0) > 0 && (
                  <span style={{ color: "#E0A34E" }}> · {report?.skipped} skipped</span>
                )}
              </div>

              <Rule />
              <Label>TRANSFORM {transform ? "ON" : "OFF"}</Label>
              <div style={{
                border: `1px solid ${transform ? "rgba(63,207,168,.3)" : "rgba(224,163,78,.35)"}`,
                background: transform ? "rgba(63,207,168,.05)" : "rgba(224,163,78,.06)",
                padding: "11px 12px", fontSize: 11.5, lineHeight: 1.6, color: "#D2CFC8",
              }}>
                {transform
                  ? "Every replica agrees and the document says what its authors meant. Turn it off and watch only half of that survive."
                  : "Every replica agrees perfectly and the document is quietly wrong. That contradiction is the page's argument."}
              </div>

              <Rule />
              <Label>TWO INSTRUMENTS</Label>
              <div className="mono" style={{ fontSize: 10.5, lineHeight: 1.9, color: "#8C8880" }}>
                structural digest&nbsp;&nbsp;
                <span style={{ color: report?.converged ? "#3FCFA8" : "#E0A34E" }}>
                  {report?.converged ? "agrees" : "differs"}
                </span>
                <br />
                intent ledger&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;
                <span style={{ color: violations.length ? "#E0A34E" : "#3FCFA8" }}>
                  {violations.length ? `flags ${violations.length}` : "clean"}
                </span>
              </div>
              {violations.length > 0 && (
                <div className="mono" style={{
                  fontSize: 10, lineHeight: 1.7, color: "#8C8880",
                  borderLeft: "2px solid rgba(224,163,78,.5)", paddingLeft: 9,
                }}>
                  {violations.slice(0, 3).map((v) => (
                    <div key={v.op_id}>
                      <span style={{ color: "#E0A34E" }}>{v.op_id}</span> meant{" "}
                      <span style={{ color: "#D2CFC8" }}>{v.meant}</span>, landed as{" "}
                      <span style={{ color: "#D2CFC8" }}>{v.got}</span>
                    </div>
                  ))}
                </div>
              )}
            </>
          ) : (
            <>
              <Label>THE FOUR LENSES</Label>
              {[
                ["prediction", "local echo, 0 ms"],
                ["rollback", "on server disagree"],
                ["transform", "text tier here"],
                ["log", "the source of truth"],
              ].map(([k, v]) => (
                <div key={k} style={{
                  display: "flex", alignItems: "baseline", gap: 8,
                  fontSize: 11.5, color: "#9B968D",
                }}>
                  <span style={{ flex: 1 }}>{k}</span>
                  <span className="mono" style={{ fontSize: 11, color: "#E4E2DC" }}>{v}</span>
                </div>
              ))}
              <div style={{ fontSize: 11, lineHeight: 1.6, color: "#585550" }}>
                All four read one op log. They are four views of one system, not four
                systems — which is the only reason a disagreement between them means
                something.
              </div>

              <Rule />
              <Label>RECONCILIATION</Label>
              {[
                ["predicted ops in flight", String(you?.pending ?? 0), "#E4E2DC"],
                ["rolled back this session", String(you?.rolled_back ?? 0),
                  (you?.rolled_back ?? 0) > 0 ? "#E0A34E" : "#E4E2DC"],
                ["replay matches incremental", report?.replay_matches ? "yes" : "no",
                  report?.replay_matches ? "#3FCFA8" : "#E0A34E"],
                ["packets lost", String(report?.lost ?? 0),
                  (report?.lost ?? 0) > 0 ? "#E0A34E" : "#E4E2DC"],
                ["simulated ms", String(report?.ticks ?? 0), "#E4E2DC"],
              ].map(([k, v, tone]) => (
                <div key={k} style={{
                  display: "flex", alignItems: "baseline", gap: 8,
                  fontSize: 11.5, color: "#9B968D",
                }}>
                  <span style={{ flex: 1 }}>{k}</span>
                  <span className="mono" style={{ fontSize: 11, color: tone }}>{v}</span>
                </div>
              ))}
              <div style={{ fontSize: 11, lineHeight: 1.6, color: "#585550" }}>
                Replaying from empty must equal the incrementally-maintained view. That
                equality is checked on every run, not asserted in a comment.
              </div>

              <Rule />
              <Label>WHAT THIS IS NOT</Label>
              <div style={{ fontSize: 11, lineHeight: 1.6, color: "#585550" }}>
                A simulation, deterministic from a seed. The live engine is{" "}
                <span className="mono" style={{ color: "#8C8880" }}>collaboration-service</span> —
                real ropes, a real WAL, real sockets, and both op tiers. This one models
                the character tier, which is where transform is legible.
              </div>
            </>
          )}
        </Inspector>
      </Body>

      <StatusBar
        route="/lab/netcode"
        mechanism="OT over the text tier · deterministic from a seed"
        state={report?.replay_matches
          ? "log replayed from empty matches the incremental view"
          : "replay does not match — the log is not the source of truth here"}
        healthy={report?.replay_matches !== false}
      />
    </Screen>
  );
}
