// Shared fetch wrapper — every REST call in this app goes through here so
// there is exactly one place that knows pages.md/auth.md §2's one error
// shape ({ error, message }) and how a request proves who is making it.
//
// That proof is now a bearer token (ADR-013 §1). It used to be the
// X-Actor-Id header: a user id this code read out of the token itself and
// then sent as a plain string, which any client could change to any other
// user's id. The gateway ignores that header entirely now, so sending it
// would be sending nothing.
export class ApiError extends Error {
  status: number;
  code: string;

  constructor(status: number, code: string, message: string) {
    super(message);
    this.name = "ApiError";
    this.status = status;
    this.code = code;
  }
}

export interface RequestOptions {
  method?: string;
  body?: unknown;
  /**
   * Who is asking.
   *
   * Kept because most callers genuinely have it and several endpoints use it
   * in a URL — but it is NOT how the request authenticates any more, and it
   * is no longer sent as a header. The token is the credential.
   */
  actorId?: string | null;
}

/**
 * Where the access token comes from.
 *
 * A provider rather than a parameter: there are roughly forty call sites and
 * threading a token through every one of them would put the same value in
 * forty places that can each get it wrong. AuthContext registers this once,
 * and reads it fresh on every call so a refresh takes effect immediately
 * rather than at the next reload.
 */
let provideToken: () => string | null = () => null;

export function setAccessTokenProvider(provider: () => string | null) {
  provideToken = provider;
}

/**
 * The current access token, for the one caller that cannot go through
 * apiFetch: the WebSocket, which carries its credential in the subprotocol
 * list rather than a header (ADR-013 §1).
 */
export function accessToken(): string | null {
  return provideToken();
}

export async function apiFetch<T>(url: string, opts: RequestOptions = {}): Promise<T> {
  const headers: Record<string, string> = { "Content-Type": "application/json" };
  const token = provideToken();
  if (token) headers["Authorization"] = `Bearer ${token}`;

  const res = await fetch(url, {
    method: opts.method ?? "GET",
    headers,
    body: opts.body !== undefined ? JSON.stringify(opts.body) : undefined,
  });

  if (res.status === 204) return undefined as T;

  const text = await res.text();
  const data = text ? JSON.parse(text) : undefined;

  if (!res.ok) {
    const errBody = data as { error?: string; message?: string } | undefined;
    throw new ApiError(res.status, errBody?.error ?? "unknown", errBody?.message ?? res.statusText);
  }
  return data as T;
}
