/**
 * docs/ui-mockups/v2/index.html § 03 REGISTER and § 03b LOG IN, ported.
 *
 * One component for both, as the mockup itself is: the two screens differ
 * only in the eyebrow, the heading, the standfirst, and whether a display
 * name is asked for. The right-hand 420px column is identical across both.
 *
 * Pre-auth, so the top bar is `bare` — no tabs, no utility cluster
 * (DESIGN_GUIDELINES.md §6.1: a stranger has no session to show).
 *
 * Inputs are real form controls here, where the mockup draws static divs.
 * That is the one place a port must diverge: the mockup shows a filled
 * field because it cannot be typed into, and copying that literally would
 * ship a login screen nobody can log in with. Everything visual about them
 * matches the drawn field — same ground, border, mono face, padding.
 */
import { useState, type SyntheticEvent } from "react";
import { useNavigate } from "react-router-dom";
import { useAuth } from "../auth/AuthContext";
import { ApiError } from "../api/http";
import { Body, Label, Screen, StatusBar, TopBar } from "../shell/Chrome";
import { ph, PlaceholderNote } from "../shell/placeholder";

type Mode = "signin" | "register";

/** The drawn field, made typable. Visual properties are the mockup's. */
function Field({
  value, onChange, type = "text", placeholder, accent,
}: {
  value: string;
  onChange: (v: string) => void;
  type?: string;
  placeholder?: string;
  accent?: boolean;
}) {
  return (
    <input
      value={value}
      type={type}
      placeholder={placeholder}
      onChange={(e) => onChange(e.target.value)}
      style={{
        width: "100%",
        padding: "9px 12px",
        background: "#141617",
        border: `1px solid ${accent ? "rgba(232,135,60,.3)" : "rgba(255,255,255,.09)"}`,
        font: "400 13px 'IBM Plex Mono',monospace",
        color: "#E4E2DC",
        outline: "none",
      }}
    />
  );
}

export function AuthPage() {
  const [mode, setMode] = useState<Mode>("signin");
  const { login, register } = useAuth();
  const navigate = useNavigate();

  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [displayName, setDisplayName] = useState("");
  const [keepOpen, setKeepOpen] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  const signin = mode === "signin";

  async function handleSubmit(e: SyntheticEvent) {
    e.preventDefault();
    setError(null);
    setBusy(true);
    try {
      if (signin) await login(email, password);
      else await register(email, password, displayName);
      navigate("/pages");
    } catch (err) {
      setError(err instanceof ApiError ? err.message : "Something went wrong. Try again.");
    } finally {
      setBusy(false);
    }
  }

  return (
    <Screen>
      <TopBar
        bare
        crumb={<>account / <b>{signin ? "log in" : "register"}</b></>}
        right={
          <>
            <span style={{ fontSize: 12, color: "#9B968D" }}>
              {signin ? "New here?" : "Have an account?"}
            </span>
            <span
              className="chip chip-e"
              style={{ cursor: "pointer" }}
              onClick={() => { setMode(signin ? "register" : "signin"); setError(null); }}
            >
              {signin ? "REGISTER" : "LOG IN"}
            </span>
          </>
        }
      />

      <Body>
        <div style={{
          flex: 1, minWidth: 0, display: "flex", alignItems: "center",
          justifyContent: "center", borderRight: "1px solid rgba(255,255,255,.07)",
        }}>
          <form onSubmit={handleSubmit} style={{ width: 380 }}>
            <div className="mono" style={{
              fontSize: 9.5, letterSpacing: ".2em", color: "#E8873C", marginBottom: 14,
            }}>
              {signin ? "LOG IN" : "REGISTER"}
            </div>
            <h1 className="h1" style={{ fontSize: 28, marginBottom: 10 }}>
              {signin ? "Welcome back" : "Make an account"}
            </h1>
            <p style={{ fontSize: 13, color: "#8C8880", lineHeight: 1.65, margin: "0 0 28px" }}>
              {signin
                ? "Your undo stack, reading position and presence colour are waiting where you left them."
                : "A page is created empty and untitled. Naming it later is normal — the id was never the name."}
            </p>

            {!signin && (
              <>
                <Label>DISPLAY NAME</Label>
                <div style={{ marginBottom: 16 }}>
                  <Field value={displayName} onChange={setDisplayName} placeholder="Ada Lovelace" />
                </div>
              </>
            )}

            <Label>EMAIL</Label>
            <div style={{ marginBottom: 16 }}>
              <Field value={email} onChange={setEmail} type="email" placeholder="you@example.com" />
            </div>

            <div style={{ display: "flex", alignItems: "baseline", marginBottom: 7 }}>
              <Label>PASSPHRASE</Label>
              <span className="mono" style={{ marginLeft: "auto", fontSize: 10, color: "#E8873C" }}>
                forgot?
              </span>
            </div>
            <div style={{ marginBottom: 20 }}>
              <Field value={password} onChange={setPassword} type="password" placeholder="••••••••••" accent />
            </div>

            <div
              style={{ display: "flex", alignItems: "center", gap: 10, marginBottom: 22, cursor: "pointer" }}
              onClick={() => setKeepOpen(!keepOpen)}
            >
              <div style={{
                width: 13, height: 13,
                border: `1px solid ${keepOpen ? "rgba(232,135,60,.5)" : "rgba(255,255,255,.18)"}`,
                background: keepOpen ? "rgba(232,135,60,.15)" : "transparent",
                display: "flex", alignItems: "center", justifyContent: "center",
                fontSize: 9, color: "#E8873C",
              }}>
                {keepOpen ? "✓" : ""}
              </div>
              <span style={{ fontSize: 12, color: "#8C8880" }}>Keep this session open</span>
            </div>

            {error && (
              <div style={{
                display: "flex", gap: 9, padding: "9px 11px", marginBottom: 16,
                border: "1px solid rgba(224,163,78,.3)", background: "rgba(224,163,78,.06)",
                fontSize: 11.5, lineHeight: 1.55, color: "#9B968D",
              }}>
                <span style={{ color: "#E0A34E" }}>◌</span>
                <span>{error}</span>
              </div>
            )}

            <button
              type="submit"
              disabled={busy}
              style={{
                width: "100%", padding: "11px 20px", background: "#E8873C", color: "#12100D",
                font: "600 11.5px 'IBM Plex Mono',monospace", letterSpacing: ".1em",
                textAlign: "center", border: "none",
                cursor: busy ? "default" : "pointer", opacity: busy ? 0.6 : 1,
              }}
            >
              {busy ? "…" : signin ? "LOG IN" : "CREATE ACCOUNT"}
            </button>

            <div style={{ display: "flex", alignItems: "center", gap: 12, margin: "18px 0" }}>
              <div style={{ flex: 1, height: 1, background: "rgba(255,255,255,.09)" }} />
              <span className="mono" style={{ fontSize: 9.5, color: "#4B4842" }}>OR</span>
              <div style={{ flex: 1, height: 1, background: "rgba(255,255,255,.09)" }} />
            </div>
            {/* Drawn in the mockup, and no OAuth exists — so they are marked
                rather than wired to nothing or quietly deleted (§9.4). */}
            <div style={{ display: "flex", gap: 10, opacity: 0.45 }}>
              <div className="btn" style={{ flex: 1, textAlign: "center", padding: "9px 0" }}>
                GOOGLE<div className="brk-tl" /><div className="brk-br" />
              </div>
              <div className="btn" style={{ flex: 1, textAlign: "center", padding: "9px 0" }}>
                GITHUB<div className="brk-tl" /><div className="brk-br" />
              </div>
            </div>
          </form>
        </div>

        <div style={{
          width: 420, flex: "none", boxSizing: "border-box",
          background: "#0F1012", display: "flex", flexDirection: "column",
        }}>
          <div style={{ padding: "34px 40px", borderBottom: "1px solid rgba(255,255,255,.07)" }}>
            <Label>WHERE YOU LEFT OFF</Label>
            <PlaceholderNote>resume needs per-user caret state — no endpoint yet</PlaceholderNote>
            <div className="fx" style={{
              display: "flex", alignItems: "baseline", gap: 12, padding: "10px 0",
              borderBottom: "1px solid rgba(255,255,255,.07)",
            }}>
              <span style={{ fontFamily: "Spectral,serif", fontSize: 14.5, color: "#E4E2DC", flex: 1 }}>
                {ph("Sync protocol notes")}
              </span>
              <span className="mono" style={{ fontSize: 10, color: "#585550" }}>{ph("4 min ago")}</span>
            </div>
            <div className="fx" style={{
              display: "flex", alignItems: "baseline", gap: 12, padding: "10px 0", animationDelay: ".08s",
            }}>
              <span style={{ fontFamily: "Spectral,serif", fontSize: 14.5, color: "#9B968D", flex: 1 }}>
                {ph("Block model")}
              </span>
              <span className="mono" style={{ fontSize: 10, color: "#585550" }}>{ph("yesterday")}</span>
            </div>
          </div>

          <div style={{ padding: "26px 40px", display: "flex", flexDirection: "column", gap: 13 }}>
            <Label>WHILE YOU WERE AWAY</Label>
            <PlaceholderNote>activity feed needs the notification stream</PlaceholderNote>
            {[
              { hue: "#A98CE8", ping: true, text: ph("Ada rewrote the intro"), meta: ph("14 ops · 2h ago") },
              { hue: "#7D9EC9", ping: false, text: ph("Assistant proposed 2 ops"), meta: ph("awaiting review") },
              { hue: "rgba(255,255,255,.18)", ping: false, text: ph("3 checks closed"), meta: ph("Monday") },
            ].map((r, i) => (
              <div key={i} style={{ display: "flex", gap: 12 }}>
                <div style={{ display: "flex", flexDirection: "column", alignItems: "center", width: 8 }}>
                  <div className={r.ping ? "dot" : undefined}
                       style={{ width: 7, height: 7, background: r.hue }}>
                    {r.ping && <div className="ping" style={{ background: "rgba(169,140,232,.5)" }} />}
                  </div>
                  {i < 2 && <div style={{ flex: 1, width: 1, background: "rgba(255,255,255,.1)" }} />}
                </div>
                <div style={{ paddingBottom: i < 2 ? 14 : 0 }}>
                  <div style={{ fontSize: 12.5, color: i === 2 ? "#8C8880" : "#D2CFC8" }}>{r.text}</div>
                  <div className="mono" style={{ fontSize: 10, color: "#585550", marginTop: 2 }}>{r.meta}</div>
                </div>
              </div>
            ))}
          </div>

          <div className="wal" style={{ borderTop: "1px solid rgba(255,255,255,.07)" }}>
            <Label>SESSION</Label>
            <span className="mono" style={{ fontSize: 10.5, color: "#8C8880" }}>
              a page open is a live socket, not a tab
            </span>
          </div>
        </div>
      </Body>

      <StatusBar
        route={signin ? "/login" : "/register"}
        mechanism="bearer token · rotating refresh"
        state={error ? "sign-in failed" : "ready"}
        healthy={!error}
      />
    </Screen>
  );
}

export default AuthPage;
