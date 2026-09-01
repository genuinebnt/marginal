import { createContext, useCallback, useContext, useMemo, useState, type ReactNode } from "react";
import { decodeActorId, login as apiLogin, register as apiRegister, type TokenPair } from "../api/auth";
import { setAccessTokenProvider } from "../api/http";

interface Session {
  actorId: string;
  accessToken: string;
  refreshToken: string;
}

interface AuthContextValue {
  session: Session | null;
  login: (email: string, password: string) => Promise<void>;
  register: (email: string, password: string, displayName: string) => Promise<void>;
  logout: () => void;
}

const AuthContext = createContext<AuthContextValue | null>(null);

const STORAGE_KEY = "marginal.session";

function loadSession(): Session | null {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    return raw ? (JSON.parse(raw) as Session) : null;
  } catch {
    // Per-viewer convenience only (Artifact/browser-storage conventions
    // this repo follows elsewhere) — a corrupt or inaccessible value just
    // means "not signed in," never a crash.
    return null;
  }
}

function toSession(pair: TokenPair): Session {
  return { actorId: decodeActorId(pair.access_token), accessToken: pair.access_token, refreshToken: pair.refresh_token };
}

/**
 * The token every REST call proves itself with (ADR-013 §1).
 *
 * Registered at module scope, not in an effect: a request can be in flight
 * before any component has mounted, and a provider that is only wired up
 * after the first render would send that one unauthenticated. It reads a
 * mutable ref rather than closing over a value so a re-login takes effect on
 * the very next call.
 */
let currentSession: Session | null = loadSession();
setAccessTokenProvider(() => currentSession?.accessToken ?? null);

export function AuthProvider({ children }: { children: ReactNode }) {
  const [session, setSession] = useState<Session | null>(currentSession);

  const persist = useCallback((s: Session | null) => {
    currentSession = s;
    setSession(s);
    if (s) localStorage.setItem(STORAGE_KEY, JSON.stringify(s));
    else localStorage.removeItem(STORAGE_KEY);
  }, []);

  const login = useCallback(
    async (email: string, password: string) => {
      persist(toSession(await apiLogin(email, password)));
    },
    [persist],
  );

  const register = useCallback(
    async (email: string, password: string, displayName: string) => {
      persist(toSession(await apiRegister(email, password, displayName)));
    },
    [persist],
  );

  const logout = useCallback(() => persist(null), [persist]);

  const value = useMemo(() => ({ session, login, register, logout }), [session, login, register, logout]);
  return <AuthContext.Provider value={value}>{children}</AuthContext.Provider>;
}

export function useAuth(): AuthContextValue {
  const ctx = useContext(AuthContext);
  if (!ctx) throw new Error("useAuth must be used within AuthProvider");
  return ctx;
}
