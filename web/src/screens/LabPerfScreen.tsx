/**
 * docs/ui-mockups/v2/index.html § 16 PERF, ported — and run,
 * here, now.
 *
 * The mockup says "measured on your machine" and "benchmark ran
 * locally", and this is what makes those true: the four workloads
 * are real Go paths from this repo — the editor's `Page.Apply`,
 * the paste handler's `mdc.Compile`, § 14's `netsim.Run`, the
 * search index's vector build — compiled to wasm and timed where
 * you are sitting. A benchmark of code nothing else calls measures
 * the benchmark.
 *
 * The loop, the clock, the log-spaced histogram, the percentiles
 * and the call tree are all `marginal/bench` in Go. Timing in
 * TypeScript and running the work in Go would measure the bridge.
 *
 * Two things it refuses to pretend:
 *
 *   - The flame graph is walked from INSTRUMENTED SPANS, not
 *     sampled. There is no sampling profiler in wasm, and drawing
 *     one anyway from invented stacks is exactly the dishonesty
 *     this screen exists against. Every frame is a function
 *     somebody named.
 *   - Percentiles are quantised by whatever clock the host gave
 *     us, so the observed resolution is printed beside them.
 *
 * QUEUE DEPTH is the one panel that is not local: it reads
 * `GET /collab/stats` — collaboration-service's real outbox depth
 * and op-log lag, reached directly, the same convention its other
 * debug endpoints follow.
 */
import { useCallback, useEffect, useMemo, useState } from "react";
import {
  dur, listWorkloads, runBench,
  type BenchResult, type WorkloadInfo,
} from "../bench-core/wasm";
import { getCollabStats, type CollabStats } from "../api/history";
import {
  Body, Label, Readout, Rule, Screen, StatusBar, TopBar, num,
} from "../shell/Chrome";

const SAMPLE_CHOICES = [1000, 10000, 50000];

/** Whether the run gathered enough samples for this quantile to
 *  be an observation rather than the maximum renamed. */
function supports(r: BenchResult | null, q: number): boolean {
  return r != null && r.supported_quantile >= q;
}

/** Bundle sizes come from the built assets, so the treemap is the
 *  real shipped weight rather than a drawn one. Vite writes them
 *  at build time; in dev there is no manifest, and the panel says
 *  so instead of inventing numbers. */
interface Chunk { name: string; bytes: number }

const CHUNK_COLOUR = [
  { fill: "rgba(232,135,60,.3)", border: "rgba(232,135,60,.5)", text: "#EFEDE7" },
  { fill: "rgba(125,158,201,.22)", border: "rgba(125,158,201,.4)", text: "#D2CFC8" },
  { fill: "rgba(169,140,232,.2)", border: "rgba(169,140,232,.35)", text: "#D2CFC8" },
  { fill: "rgba(255,255,255,.05)", border: "rgba(255,255,255,.09)", text: "#9B968D" },
  { fill: "rgba(255,255,255,.04)", border: "rgba(255,255,255,.08)", text: "#8C8880" },
];

export function LabPerfScreen() {
  const [workloads, setWorkloads] = useState<WorkloadInfo[]>([]);
  const [workload, setWorkload] = useState("applyOp");
  const [samples, setSamples] = useState(10000);
  const [result, setResult] = useState<BenchResult | null>(null);
  const [running, setRunning] = useState(false);
  const [err, setErr] = useState<string | null>(null);
  const [stats, setStats] = useState<CollabStats | null>(null);
  const [statsErr, setStatsErr] = useState<string | null>(null);

  useEffect(() => {
    listWorkloads().then(setWorkloads).catch((e) => setErr(String(e)));
  }, []);

  const run = useCallback((name: string, n: number) => {
    setRunning(true);
    setErr(null);
    // A frame before the loop starts, so RUN AGAIN visibly
    // becomes RUNNING rather than freezing on the old numbers:
    // wasm runs on this thread, and a benchmark that looks
    // identical while it works reads as a dead button.
    requestAnimationFrame(() => {
      runBench(name, n)
        .then(setResult)
        .catch((e) => setErr(String(e)))
        .finally(() => setRunning(false));
    });
  }, []);

  useEffect(() => { run(workload, samples); }, [workload, samples, run]);

  useEffect(() => {
    getCollabStats()
      .then((s) => { setStats(s); setStatsErr(null); })
      .catch((e) => setStatsErr(String(e)));
  }, []);

  const maxBucket = Math.max(1, ...(result?.buckets ?? []).map((b) => b.count));
  const note = workloads.find((w) => w.name === workload)?.note ?? result?.note ?? "";

  const axis = useMemo(() => {
    const bs = result?.buckets ?? [];
    if (!bs.length) return [] as string[];
    return [0, Math.floor(bs.length / 3), Math.floor((bs.length * 2) / 3), bs.length - 1]
      .map((i) => bs[i]?.label ?? "");
  }, [result]);

  const clamped = result != null && result.samples < samples;
  const budgeted = result?.budgeted === true;

  return (
    <Screen>
      <TopBar
        crumb={<>lab / <b>perf</b></>}
        readouts={
          <>
            <Readout k="WORKLOAD" v={workload} />
            <Readout
              k="SAMPLES"
              v={num(result?.ran ?? 0)}
              tone={clamped || budgeted ? "#E0A34E" : undefined}
            />
          </>
        }
        right={
          <>
            <div className="vr" />
            <span
              className={`chip${running ? "" : " chip-e"}`}
              style={{ cursor: running ? "default" : "pointer" }}
              onClick={() => { if (!running) run(workload, samples); }}
            >
              {running ? "RUNNING…" : "RUN AGAIN"}
            </span>
          </>
        }
      />

      <Body style={{ flexDirection: "column", padding: "24px 30px", overflow: "hidden" }}>
        <div style={{ display: "flex", gap: 26, flex: 1, minHeight: 0 }}>
          <div style={{ flex: 1.2, display: "flex", flexDirection: "column", gap: 20, minWidth: 0 }}>
            <div>
              <div style={{ display: "flex", alignItems: "baseline", gap: 12, marginBottom: 12 }}>
                <Label>ACK LATENCY · LOG-SPACED BUCKETS</Label>
                <span className="mono" style={{ fontSize: 10, color: "#585550" }}>
                  measured on your machine
                </span>
              </div>

              {/* The workload picker. Four real code paths, and the
                  one you pick is the one that runs. */}
              <div style={{ display: "flex", gap: 6, marginBottom: 12, flexWrap: "wrap" }}>
                {workloads.map((w) => (
                  <span
                    key={w.name}
                    className={`chip${w.name === workload ? " chip-e" : ""}`}
                    style={{ cursor: "pointer" }}
                    onClick={() => setWorkload(w.name)}
                  >
                    {w.name}
                  </span>
                ))}
                <div style={{ flex: 1 }} />
                {SAMPLE_CHOICES.map((n) => (
                  <span
                    key={n}
                    className={`chip${n === samples ? " chip-e" : ""}`}
                    style={{ cursor: "pointer" }}
                    onClick={() => setSamples(n)}
                  >
                    {n >= 1000 ? `${n / 1000}k` : n}
                  </span>
                ))}
              </div>

              <div style={{ display: "flex", alignItems: "flex-end", gap: 2, height: 104 }}>
                {(result?.buckets ?? Array.from({ length: 12 })).map((b, i) => {
                  const count = b && typeof b === "object" && "count" in b ? b.count : 0;
                  const tail = i >= 10;
                  return (
                    <div
                      key={i}
                      title={b && "label" in b ? `${b.label} · ${count}` : undefined}
                      style={{
                        flex: 1,
                        height: `${Math.max(2, (count / maxBucket) * 100)}%`,
                        background: count === maxBucket
                          ? "#E8873C"
                          : tail && count > 0
                            ? "rgba(224,163,78,.5)"
                            : `rgba(232,135,60,${(0.2 + 0.25 * (count / maxBucket)).toFixed(2)})`,
                        transition: "height .25s cubic-bezier(.2,.7,.3,1)",
                      }}
                    />
                  );
                })}
              </div>
              <div style={{ display: "flex", justifyContent: "space-between", marginTop: 7 }}>
                {axis.map((a, i) => (
                  <span key={i} className="mono" style={{ fontSize: 9, color: "#4B4842" }}>{a}</span>
                ))}
              </div>

              <div style={{ display: "flex", gap: 26, marginTop: 14, flexWrap: "wrap" }}>
                {/* A quantile the sample count cannot support is
                    greyed rather than dropped: its absence is
                    itself information, and printing it in the
                    same ink as a real one would be the lie. */}
                <Readout k="P50" v={dur(result?.p50_ns ?? 0)} size={16}
                  tone={supports(result, 0.5) ? undefined : "#4B4842"} />
                <Readout k="P95" v={dur(result?.p95_ns ?? 0)} size={16}
                  tone={supports(result, 0.95) ? undefined : "#4B4842"} />
                <Readout k="P99" v={dur(result?.p99_ns ?? 0)} size={16}
                  tone={supports(result, 0.99) ? "#3FCFA8" : "#4B4842"} />
                <Readout k="P99.9" v={dur(result?.p999_ns ?? 0)} size={16}
                  tone={supports(result, 0.999) ? "#E0A34E" : "#4B4842"} />
                <Readout
                  k="CLOCK"
                  v={`±${dur(result?.clock_resolution_ns ?? 0)}`}
                  size={16}
                  tone="#6E6A63"
                />
                <Readout
                  k="BATCH"
                  v={`×${num(result?.batch_size ?? 1)}`}
                  size={16}
                  tone={(result?.batch_size ?? 1) > 1 ? "#E0A34E" : "#6E6A63"}
                />
              </div>
              <div style={{ fontSize: 11, lineHeight: 1.6, color: "#585550", marginTop: 8 }}>
                {note}
                {(result?.batch_size ?? 1) > 1 && (
                  <span>
                    {" "}One iteration is faster than this host's clock
                    ({dur(result?.clock_resolution_ns ?? 0)}), so each sample is
                    the mean of {num(result?.batch_size ?? 1)} of them — which
                    narrows the spread. The p99.9 below is a tail of batch
                    means, not of single calls.
                  </span>
                )}
                {result != null && result.supported_quantile < 0.999 && (
                  <span>
                    {" "}Only {num(result.ran)} samples fit, so anything above
                    p{(result.supported_quantile * 100).toFixed(
                      result.supported_quantile < 0.99 ? 0 : 1)} is greyed —
                    it would be the maximum wearing a percentile's name.
                  </span>
                )}
                {budgeted && (
                  <span style={{ color: "#E0A34E" }}>
                    {" "}Stopped on its own clock after {num(result?.ran ?? 0)} of{" "}
                    {num(result?.samples ?? 0)} — wasm runs on this page's thread,
                    so a run that overruns freezes the tab rather than taking longer.
                  </span>
                )}
                {clamped && (
                  <span style={{ color: "#E0A34E" }}>
                    {" "}Clamped to {num(result?.samples ?? 0)} — one iteration here is
                    milliseconds, not microseconds.
                  </span>
                )}
              </div>
            </div>

            <Rule />

            <div style={{ flex: 1, minHeight: 0 }}>
              <div style={{ display: "flex", alignItems: "baseline", gap: 12, marginBottom: 12 }}>
                <Label>FLAME · WALKED FROM A CALL TREE</Label>
                <span className="mono" style={{ fontSize: 10, color: "#585550" }}>
                  instrumented spans, not sampled
                </span>
              </div>
              <div style={{ display: "flex", flexDirection: "column", gap: 2 }}>
                {[0, 1, 2, 3].map((depth) => {
                  const row = (result?.frames ?? []).filter((f) => f.depth === depth);
                  if (!row.length) return null;
                  const claimed = row.reduce((s, f) => s + f.fraction, 0);
                  return (
                    <div key={depth} style={{ height: 16, display: "flex", gap: 2 }}>
                      {row.map((f) => (
                        <div
                          key={f.name}
                          title={`${f.name} · total ${dur(f.total_ns)} · self ${dur(f.self_ns)} · ${f.calls} calls`}
                          style={{
                            flex: Math.max(0.02, f.fraction),
                            background: depth === 0
                              ? "rgba(232,135,60,.5)"
                              : `rgba(232,135,60,${(0.38 - depth * 0.08).toFixed(2)})`,
                            display: "flex", alignItems: "center", paddingLeft: 7,
                            font: "400 9px 'IBM Plex Mono', monospace",
                            color: depth < 2 ? "#0E0F10" : "#8C8880",
                            overflow: "hidden", whiteSpace: "nowrap",
                            transition: "flex-grow .25s cubic-bezier(.2,.7,.3,1)",
                          }}
                        >
                          {f.name}
                        </div>
                      ))}
                      {claimed < 0.98 && (
                        <div style={{ flex: 1 - claimed, background: "rgba(255,255,255,.03)" }} />
                      )}
                    </div>
                  );
                })}
              </div>
            </div>
          </div>

          <div style={{ width: 400, flex: "none", display: "flex", flexDirection: "column", gap: 18 }}>
            <div>
              <div style={{ display: "flex", alignItems: "baseline", gap: 10, marginBottom: 12 }}>
                <Label>QUEUE DEPTH</Label>
                <span className="mono" style={{ fontSize: 10, color: "#585550" }}>
                  this instance, live
                </span>
              </div>
              {statsErr ? (
                <div className="mono" style={{ fontSize: 10.5, color: "#E0A34E", lineHeight: 1.7 }}>
                  collaboration-service did not answer — {statsErr}
                </div>
              ) : (
                <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
                  {[
                    {
                      k: "outbox", v: stats?.outbox_depth ?? 0,
                      // Depth against a drain the poller can plausibly
                      // clear in one pass; a full bar is a backlog, not
                      // an arbitrary maximum.
                      frac: Math.min(1, (stats?.outbox_depth ?? 0) / 64),
                      label: String(stats?.outbox_depth ?? 0),
                      warn: (stats?.outbox_oldest_seconds ?? 0) > 30,
                    },
                    {
                      k: "op-log lag", v: stats?.lag_seconds ?? 0,
                      frac: Math.min(1, (stats?.lag_seconds ?? 0) / 600),
                      label: `${Math.round(stats?.lag_seconds ?? 0)}s`,
                      warn: false,
                    },
                    {
                      k: "ops stored", v: stats?.ops ?? 0,
                      frac: Math.min(1, (stats?.ops ?? 0) / 5000),
                      label: num(stats?.ops ?? 0),
                      warn: false,
                    },
                  ].map((row) => (
                    <div key={row.k} style={{ display: "flex", alignItems: "center", gap: 10 }}>
                      <span className="mono" style={{ fontSize: 10, color: "#585550", width: 66 }}>
                        {row.k}
                      </span>
                      <div style={{ flex: 1, height: 10, background: "rgba(255,255,255,.05)" }}>
                        <div style={{
                          width: `${row.frac * 100}%`, height: "100%",
                          background: row.warn ? "#E0A34E" : "#3FCFA8",
                          transition: "width .3s ease",
                        }} />
                      </div>
                      <span className="mono" style={{
                        fontSize: 10, color: "#8C8880", width: 34, textAlign: "right",
                      }}>
                        {row.label}
                      </span>
                    </div>
                  ))}
                </div>
              )}
              <div style={{ fontSize: 11, lineHeight: 1.6, color: "#585550", marginTop: 10 }}>
                Depth and age, not depth alone — 400 events draining in 200 ms and
                three whose oldest has waited four minutes are opposite conditions,
                and the count calls the second one healthy.
              </div>
            </div>

            <Rule />

            <div style={{ flex: 1, minHeight: 0 }}>
              <div style={{ display: "flex", alignItems: "baseline", gap: 10, marginBottom: 12 }}>
                <Label>BUNDLE · SQUARIFIED TREEMAP</Label>
                <span className="mono" style={{ fontSize: 10, color: "#585550" }}>
                  the wasm this page actually fetched
                </span>
              </div>
              {/* The wasm modules ARE the bundle's weight here, and
                  their sizes are known without a manifest: they are
                  fetched by this very page. Drawn from what actually
                  loaded rather than from a build that has not run. */}
              <WasmTreemap />
              <div style={{ display: "flex", gap: 24, marginTop: 12, flexWrap: "wrap" }}>
                <Readout k="RUN COST" v={dur(result?.total_ns ?? 0)} size={15} />
                <Readout k="MEAN" v={dur(result?.mean_ns ?? 0)} size={15} tone="#6E6A63" />
              </div>
            </div>
          </div>
        </div>

        {err && (
          <div className="mono" style={{ fontSize: 11, color: "#E0A34E", marginTop: 10 }}>
            {err}
          </div>
        )}
      </Body>

      <StatusBar
        route="/lab/perf"
        // The freeze is real and stated rather than hidden: wasm
        // runs on this page's own thread, so the tab stops
        // painting for the length of a run. Bounded at two
        // seconds, which is why it is bounded at all.
        mechanism={`benchmark ran locally · ${num(result?.ran ?? 0)} samples · the tab stops painting while it runs`}
        state={result
          ? `p99.9 ${dur(result.p999_ns)} — measured here, not quoted`
          : "running…"}
        healthy={!running}
      />
    </Screen>
  );
}

/** The wasm modules this app actually ships, sized by the bytes
 *  the browser fetched. Real weight, no build step needed —
 *  and the honest version of a bundle treemap for a page that
 *  loads five Go modules. */
function WasmTreemap() {
  const [sizes, setSizes] = useState<Chunk[]>([]);

  useEffect(() => {
    const names = ["documentcore", "graph", "diff", "trie", "syntax", "mdc", "netsim", "bench"];
    Promise.all(names.map(async (n) => {
      try {
        const r = await fetch(`/${n}.wasm`, { method: "HEAD" });
        const len = Number(r.headers.get("content-length") ?? 0);
        return { name: n, bytes: r.ok ? len : 0 };
      } catch {
        return { name: n, bytes: 0 };
      }
    })).then((all) => setSizes(all.filter((c) => c.bytes > 0)
      .sort((a, b) => b.bytes - a.bytes)));
  }, []);

  const total = sizes.reduce((s, c) => s + c.bytes, 0);
  if (!sizes.length) {
    return (
      <div className="mono" style={{ fontSize: 10.5, color: "#585550", lineHeight: 1.7 }}>
        no wasm modules answered a HEAD — nothing to weigh
      </div>
    );
  }

  const left = sizes.slice(0, 2);
  const right = sizes.slice(2);
  return (
    <>
      <div style={{ display: "flex", gap: 3, height: 190 }}>
        <div style={{ flex: 1.7, display: "flex", flexDirection: "column", gap: 3 }}>
          {left.map((c, i) => (
            <TreemapCell key={c.name} chunk={c} i={i} />
          ))}
        </div>
        <div style={{ flex: 1, display: "flex", flexDirection: "column", gap: 3 }}>
          {right.map((c, i) => (
            <TreemapCell key={c.name} chunk={c} i={i + 2} />
          ))}
        </div>
      </div>
      <div style={{ display: "flex", gap: 24, marginTop: 12, flexWrap: "wrap" }}>
        <Readout k="TOTAL WASM" v={`${Math.round(total / 1024)} kB`} size={15} />
        <Readout
          k="BUDGET"
          v="4 MB"
          size={15}
          tone={total < 4 * 1024 * 1024 ? "#3FCFA8" : "#E0A34E"}
        />
      </div>
    </>
  );
}

function TreemapCell({ chunk, i }: { chunk: Chunk; i: number }) {
  const c = CHUNK_COLOUR[Math.min(i, CHUNK_COLOUR.length - 1)];
  return (
    <div style={{
      flex: Math.max(0.5, chunk.bytes / 1024 / 512),
      background: c.fill, border: `1px solid ${c.border}`,
      display: "flex", alignItems: "flex-end", padding: 7,
      font: "400 9px 'IBM Plex Mono', monospace", color: c.text,
      overflow: "hidden", whiteSpace: "nowrap",
    }}>
      {chunk.name} {Math.round(chunk.bytes / 1024)} kB
    </div>
  );
}
