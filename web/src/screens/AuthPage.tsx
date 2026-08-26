import { useState, type SyntheticEvent } from "react";
import { useNavigate } from "react-router-dom";
import { useAuth } from "../auth/AuthContext";
import { ApiError } from "../api/http";

type Mode = "signin" | "register";

// Ordinary multi-user sign in/register — like Google Docs: separate
// people, each with their own account, sharing one workspace (no
// multi-tenancy yet — docs/api/auth.md). Register used to be a one-time
// "claim this instance" bootstrap step (docs/porting/PROGRESS.md records
// why that changed); it's now a normal, repeatable signup.
export function AuthPage() {
  const [mode, setMode] = useState<Mode>("signin");
  const { login, register } = useAuth();
  const navigate = useNavigate();

  const [email, setEmail] = useState("");
  const [password, setPassword] = useState("");
  const [displayName, setDisplayName] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  async function handleSubmit(e: SyntheticEvent) {
    e.preventDefault();
    setError(null);
    setBusy(true);
    try {
      if (mode === "signin") {
        await login(email, password);
      } else {
        await register(email, password, displayName);
      }
      navigate("/pages");
    } catch (err) {
      if (err instanceof ApiError) {
        setError(err.message);
      } else {
        setError("Something went wrong. Try again.");
      }
    } finally {
      setBusy(false);
    }
  }

  return (
    <div className="app">
      <header className="topbar">
        <span className="brand">
          <span className="mark"></span>Marginal
        </span>
      </header>

      <div className="auth-shell">
        <div style={{ width: "min(400px,100%)" }}>
          <div className="switcher" role="group" aria-label="State" style={{ display: "flex", gap: 6, justifyContent: "center", marginBottom: 22 }}>
            <button type="button" aria-pressed={mode === "signin"} onClick={() => setMode("signin")} className="btn" style={mode === "signin" ? { background: "var(--ink)", color: "var(--bg)" } : {}}>
              Sign in
            </button>
            <button type="button" aria-pressed={mode === "register"} onClick={() => setMode("register")} className="btn" style={mode === "register" ? { background: "var(--ink)", color: "var(--bg)" } : {}}>
              Register
            </button>
          </div>

          <form className="auth reveal" onSubmit={handleSubmit}>
            {mode === "signin" ? (
              <>
                <span className="eyebrow">Sign in</span>
                <h1 style={{ fontFamily: "var(--display)", fontWeight: 560, fontSize: 24, letterSpacing: "-.018em", margin: "14px 0 5px" }}>Welcome back</h1>
                <div className="muted" style={{ fontSize: 13, lineHeight: 1.55 }}>Sign in with your email and password.</div>

                <label className="field-l" htmlFor="email">Email</label>
                <input className="input" id="email" type="email" autoComplete="username" value={email} onChange={(e) => setEmail(e.target.value)} required />

                <label className="field-l" htmlFor="pw">Password</label>
                <input className="input" id="pw" type="password" autoComplete="current-password" value={password} onChange={(e) => setPassword(e.target.value)} required />
              </>
            ) : (
              <>
                <span className="eyebrow" style={{ color: "var(--teal)" }}>New here</span>
                <h1 style={{ fontFamily: "var(--display)", fontWeight: 560, fontSize: 24, letterSpacing: "-.018em", margin: "14px 0 5px" }}>Create your account</h1>
                <div className="muted" style={{ fontSize: 13, lineHeight: 1.55 }}>
                  Anyone can register — every account is equal, there's no admin role. You'll land
                  on the same shared pages as everyone else on this instance.
                </div>

                <label className="field-l" htmlFor="name">Your name</label>
                <input className="input" id="name" placeholder="Ada Kovács" value={displayName} onChange={(e) => setDisplayName(e.target.value)} required />

                <label className="field-l" htmlFor="email2">Email</label>
                <input className="input" id="email2" type="email" value={email} onChange={(e) => setEmail(e.target.value)} required />

                <label className="field-l" htmlFor="pw2">Password</label>
                <input className="input" id="pw2" type="password" placeholder="At least 8 characters" value={password} onChange={(e) => setPassword(e.target.value)} required minLength={8} />
                <div className="hint">Hashed with Argon2id.</div>
              </>
            )}

            {error && (
              <div className="err show" style={{ display: "flex", gap: 9, marginTop: 16, padding: "10px 12px", border: "1px solid var(--amber-line)", borderLeft: "2px solid var(--amber)", borderRadius: "var(--radius)", background: "var(--amber-soft)", fontSize: 12.5, color: "var(--ink)", lineHeight: 1.5 }}>
                <span>◌</span>
                <span>{error}</span>
              </div>
            )}

            <button type="submit" className="btn primary block" disabled={busy}>
              {busy ? "Working…" : mode === "signin" ? "Sign in" : "Create account"}
            </button>
          </form>
        </div>
      </div>
    </div>
  );
}
