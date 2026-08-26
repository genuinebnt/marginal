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

// decodeActorId reads the JWT's `sub` claim client-side — this app never
// verifies the token itself (that's auth-service's job via its RS256
// public key; a browser has no business re-deriving trust it was already
// handed), it only needs the user id to attach as the X-Actor-Id header
// on every other request per the repo-wide temporary auth stand-in
// (pages.md § Actor identity).
export function decodeActorId(accessToken: string): string {
  const payload = accessToken.split(".")[1];
  const json = atob(payload.replace(/-/g, "+").replace(/_/g, "/"));
  const claims = JSON.parse(json) as { sub: string };
  return claims.sub;
}
