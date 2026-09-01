// auth.md §2's REST mapping — register/login/refresh. Matches that doc's
// field names exactly (snake_case on the wire, per auth.md's own examples).
import { GATEWAY_URL } from "./config";
import { apiFetch } from "./http";

export interface TokenPair {
  access_token: string;
  refresh_token: string;
  expires_in: number;
}

export function register(email: string, password: string, displayName: string): Promise<TokenPair> {
  return apiFetch<TokenPair>(`${GATEWAY_URL}/auth/register`, {
    method: "POST",
    body: { email, password, display_name: displayName },
  });
}

export function login(email: string, password: string): Promise<TokenPair> {
  return apiFetch<TokenPair>(`${GATEWAY_URL}/auth/login`, {
    method: "POST",
    body: { email, password },
  });
}

export function refresh(refreshToken: string): Promise<TokenPair> {
  return apiFetch<TokenPair>(`${GATEWAY_URL}/auth/refresh`, {
    method: "POST",
    body: { refresh_token: refreshToken },
  });
}

// decodeActorId reads the JWT's `sub` claim client-side, for the UI's own
// use only — whose avatar is "you", which cursor is yours, which rows in the
// audit log are your own.
//
// It is NOT how a request identifies itself. This value used to be sent as
// the X-Actor-Id header and trusted by every service, which meant the
// browser was choosing who it was; now the token itself goes on the wire and
// the server derives `sub` from a checked signature (ADR-013 §1). The
// difference matters even though both read the same claim: this one is
// decoded without verification, and a value decoded without verification can
// only ever be used for how a screen looks, never for what it is allowed to
// do.
export function decodeActorId(accessToken: string): string {
  const payload = accessToken.split(".")[1];
  const json = atob(payload.replace(/-/g, "+").replace(/_/g, "/"));
  const claims = JSON.parse(json) as { sub: string };
  return claims.sub;
}
