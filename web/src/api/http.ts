// Shared fetch wrapper — every REST call in this app goes through here so
// there is exactly one place that knows pages.md/auth.md §2's one error
// shape ({ error, message }) and the X-Actor-Id header stand-in
// (docs/api/pages.md's "Actor identity (temporary, until a gateway
// exists)" — reused as-is by every service in this repo, not something
// this frontend invented).
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
  actorId?: string | null;
}

export async function apiFetch<T>(url: string, opts: RequestOptions = {}): Promise<T> {
  const headers: Record<string, string> = { "Content-Type": "application/json" };
  if (opts.actorId) headers["X-Actor-Id"] = opts.actorId;

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
