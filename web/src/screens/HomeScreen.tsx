/**
 * docs/ui-mockups/v2/index.html § 02 HOME, ported — the public
 * front door, and the only route in this app that does not
 * require a session.
 *
 * Two things here are real rather than drawn, because the page's
 * central claim is one this codebase can actually demonstrate:
 *
 *   - LIVE SESSION runs `marginal/netsim` in wasm: two replicas,
 *     a 180 ms wire, concurrent edits, and the transform that
 *     makes them converge. Same engine § 14 drives. It is a
 *     SIMULATION and says so — a landing page claiming a live
 *     multiplayer session while showing a canned animation is
 *     the exact thing this one is arguing against.
 *   - The counters come from `GET /collab/stats` on the running
 *     instance. Unauthenticated, which that endpoint already is.
 *
 * The pricing row states what is actually purchasable, which
 * today is nothing: self-hosting is real and free, the cloud
 * offering is designed (`ADR-008`/`ADR-010`, Terraform written)
 * but has never been applied against a project, and there is no
 * enterprise anything. Numbers were removed rather than kept as
 * placeholders — a price on a public page is an offer.
 */
import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { runSim, type Report } from "../netsim-core/wasm";
import { getCollabStats, type CollabStats } from "../api/history";
import { num } from "../shell/Chrome";

/** The scenario the panel plays: two people, one sentence, edits
 *  that genuinely need a transform to converge. Ada types 40 ms
 *  in — a full round trip before she could have heard about the
 *  first edit, which is what makes them concurrent. */
const SCRIPT = [
  "0, you, insert, 10, quite ",
  "40, ada, insert, 20, addressable ",
].join("\n");

const INITIAL = "A rope is the wrong primitive here.";

export function HomeScreen() {
  const [sim, setSim] = useState<Report | null>(null);
  const [stats, setStats] = useState<CollabStats | null>(null);
  /** How many ops of the run have "arrived" — the panel plays
   *  the log forward so you watch it converge rather than being
   *  shown the answer. */
  const [step, setStep] = useState(0);

  useEffect(() => {
    runSim({
      script: SCRIPT,
      wire: { rtt_ms: 180, loss_pct: 0, jitter_ms: 40, seed: 3 },
      transform: true,
      initial: INITIAL,
    }).then(setSim).catch(() => setSim(null));
    // The stats endpoint needs no session, which is what makes
    // real numbers possible on a page nobody has signed in to.
    getCollabStats().then(setStats).catch(() => setStats(null));
  }, []);

  useEffect(() => {
    if (!sim) return;
    const total = sim.log.length;
    setStep(0);
    const t = setInterval(() => {
      setStep((s) => (s >= total ? 0 : s + 1));
    }, 1400);
    return () => clearInterval(t);
  }, [sim]);

  // The document as of `step` ops — replayed from the initial
  // text through the confirmed log, which is the same thing
  // history replay does.
  const shown = (() => {
    if (!sim) return INITIAL;
    let text = INITIAL;
    for (const op of sim.log.slice(0, step)) {
      const pos = Math.max(0, Math.min(op.pos, text.length));
      text = op.kind === "insert"
        ? text.slice(0, pos) + (op.text ?? "") + text.slice(pos)
        : text.slice(0, pos) + text.slice(pos + (op.len ?? 0));
    }
    return text;
  })();

  const current = sim?.log[step - 1] ?? null;
  const converged = sim?.converged === true && step === (sim?.log.length ?? 0);

  return (
    <div className="sc">
      <div className="scan" />
      <div className="bar">
        <Link to="/" className="wm">m<span style={{ color: "#E8873C" }}>/</span>arginal</Link>
        <div style={{ flex: 1 }} />
        {/* Only links that go somewhere. The mockup's Docs and
            Pricing had nowhere to point, and a nav item that
            does nothing is the defect the verification pass
            exists to catch. */}
        <a href="https://github.com/genuinebnt/marginal" className="home-nav">Source</a>
        <a href="https://github.com/genuinebnt/marginal#readme" className="home-nav">Self-hosting</a>
        <div className="vr" />
        <Link to="/login" className="home-nav">Sign in</Link>
        <Link to="/login" className="btn" style={{ textDecoration: "none" }}>
          START<div className="brk-tl" /><div className="brk-br" />
        </Link>
      </div>

      <div className="body" style={{ flexDirection: "column", overflow: "hidden" }}>
        <div style={{ padding: "76px 90px 0", display: "flex", gap: 70 }}>
          <div style={{ flex: 1, minWidth: 0 }}>
            <div className="mono" style={{
              fontSize: 9.5, letterSpacing: ".2em", color: "#E8873C", marginBottom: 18,
            }}>
              A COLLABORATIVE NOTEBOOK · SELF-HOSTED OR NOT
            </div>
            <h1 className="h1" style={{ fontSize: 60, lineHeight: 1.06, marginBottom: 22 }}>
              Two people editing<br />one tree, converging.
            </h1>
            <p style={{
              fontSize: 16.5, lineHeight: 1.7, color: "#9B968D",
              maxWidth: 540, margin: "0 0 30px",
            }}>
              Every edit is an operation addressed by anchor, not offset. Every operation is
              invertible. There is no conflict dialog, because there is nothing to resolve —
              and no compile step, so nothing in your notebook is ever "broken".
            </p>
            <div style={{ display: "flex", gap: 12, alignItems: "center" }}>
              <Link
                to="/login"
                style={{
                  padding: "11px 22px", background: "#E8873C", color: "#12100D",
                  font: "600 12px 'IBM Plex Mono', monospace", letterSpacing: ".1em",
                  textDecoration: "none",
                }}
              >
                CREATE A WORKSPACE
              </Link>
              <a
                href="https://github.com/genuinebnt/marginal#readme"
                className="btn"
                style={{ padding: "11px 20px", textDecoration: "none" }}
              >
                RUN IT YOURSELF<div className="brk-tl" /><div className="brk-br" />
              </a>
            </div>
            {/* Counters from the instance you are looking at.
                Empty is empty — a landing page that invents
                traffic is the first thing a reader stops
                believing. */}
            <div style={{ display: "flex", gap: 34, marginTop: 52, flexWrap: "wrap" }}>
              <div className="rd">
                <span className="rd-k">OPS ACCEPTED</span>
                <span className="rd-v" style={{ fontSize: 17, color: "#3FCFA8" }}>
                  {num(stats?.ops ?? 0)}
                </span>
              </div>
              <div className="rd">
                <span className="rd-k">PAGES</span>
                <span className="rd-v" style={{ fontSize: 17 }}>{num(stats?.pages ?? 0)}</span>
              </div>
              <div className="rd">
                <span className="rd-k">CONFLICT DIALOGS</span>
                <span className="rd-v" style={{ fontSize: 17 }}>0</span>
              </div>
            </div>
          </div>

          <div style={{
            width: 430, flex: "none", border: "1px solid rgba(255,255,255,.09)",
            background: "#111214", alignSelf: "flex-start",
          }}>
            <div style={{
              display: "flex", alignItems: "center", gap: 8, padding: "8px 12px",
              borderBottom: "1px solid rgba(255,255,255,.07)",
            }}>
              <span className="mono" style={{
                fontSize: 9, letterSpacing: ".14em", color: "#6E6A63",
              }}>
                LIVE SESSION
              </span>
              <div style={{ flex: 1 }} />
              <div style={{ display: "flex", gap: 4 }}>
                <div className="av av-you" style={{ width: 18, height: 18, fontSize: 8 }}>GN</div>
                <div className="av av-them" style={{ width: 18, height: 18, fontSize: 8 }}>AD</div>
              </div>
            </div>
            <div style={{
              padding: 18, fontFamily: "Spectral, serif", fontSize: 14.5,
              lineHeight: 1.7, color: "#D2CFC8", minHeight: 92,
            }}>
              {shown}
              <span style={{
                display: "inline-block", width: 2, height: "1.05em",
                background: current?.actor === "ada" ? "#A98CE8" : "#3FCFA8",
                verticalAlign: "-.16em", margin: "0 1px",
              }} />
              {current && (
                <span style={{
                  font: "600 8px Archivo, sans-serif", color: "#0E0F10",
                  background: current.actor === "ada" ? "#A98CE8" : "#3FCFA8",
                  padding: "1px 4px", verticalAlign: ".5em", marginLeft: 2,
                }}>
                  {current.actor.toUpperCase()}
                </span>
              )}
            </div>
            <div style={{
              borderTop: "1px solid rgba(255,255,255,.07)", padding: "9px 12px",
              display: "flex", alignItems: "center", gap: 12,
            }}>
              <span className="mono" style={{ fontSize: 9.5, color: "#585550" }}>
                {current
                  ? `${current.kind === "insert" ? "InsertText" : "DeleteText"} ${current.id} @${current.pos}`
                  : "waiting for the first op"}
              </span>
              <div style={{ flex: 1 }} />
              <span className="mono" style={{
                fontSize: 9.5, color: converged ? "#3FCFA8" : "#585550",
              }}>
                {converged ? "converged" : "in flight"}
              </span>
            </div>
          </div>
        </div>

        {/* No pricing band. There is no paid tier, no cloud plan
            and no enterprise edition — not as a pricing strategy
            but because none of them exist, and a table quoting
            prices for them is a claim the product cannot honour.
            On a publicly deployed instance it is a claim
            strangers read. */}
        <div style={{
          marginTop: "auto", display: "flex",
          borderTop: "1px solid rgba(255,255,255,.07)",
        }}>
          <div style={{ flex: 1, padding: "26px 30px" }}>
            <div className="lbl" style={{ marginBottom: 10, color: "#E8873C" }}>
              SELF-HOSTED, AND THAT IS THE WHOLE OFFER
            </div>
            <div style={{
              fontSize: 12.5, color: "#8C8880", lineHeight: 1.7, maxWidth: 760,
            }}>
              There is no paid tier, no cloud plan and no enterprise edition — not as a
              pricing strategy, but because none of them exist. What exists is this:
              compose, Postgres, your machine, every feature.{" "}
              <span style={{ color: "#9B968D" }}>
                ADR-001 · self-hosting is feature-identical.
              </span>
            </div>
          </div>
        </div>
      </div>

      <div className="status">
        <span>marginal</span>
        <span>the panel above runs the real transform, in wasm — not an animation</span>
        <div style={{ flex: 1 }} />
        <span>ADR-001 · self-hosting is feature-identical</span>
      </div>
    </div>
  );
}
