// docs/api/comments.md — collaboration-service's own HTTP routes, reached
// directly like /trace and /palimpsest (nothing here is gRPC).
import { accessToken } from "./http";
import { COLLAB_URL } from "./config";

export interface Comment {
  id: string;
  thread_id: string;
  author_id: string;
  body: string;
  edited_at?: string;
  created_at: string;
}

export interface Thread {
  id: string;
  block_id: string;
  /** What was being discussed, captured when the thread was opened and
   *  never updated. Not a cache of what the anchors resolve to. */
  quoted: string;
  resolved_at: string | null;
  created_by: string;
  created_at: string;
  /** Where the anchors point RIGHT NOW. null when they no longer resolve. */
  range: { start: number; end: number } | null;
  /** The text this thread was about is gone. The thread is not — deleting
   *  a remark because somebody edited a sentence is worse than an untidy
   *  list. */
  orphaned: boolean;
  comments: Comment[];
}

async function collab<T>(url: string, init?: RequestInit): Promise<T> {
  const token = accessToken();
  const res = await fetch(url, {
    ...init,
    headers: {
      "Content-Type": "application/json",
      ...(token ? { Authorization: `Bearer ${token}` } : {}),
      ...(init?.headers ?? {}),
    },
  });
  if (!res.ok) throw new Error((await res.text()).trim() || res.statusText);
  if (res.status === 204) return undefined as T;
  return res.json() as Promise<T>;
}

export function listThreads(pageId: string): Promise<{ threads: Thread[] }> {
  return collab(`${COLLAB_URL}/collab/pages/${pageId}/comments`);
}

/** Opens a thread. The anchors must be ones the SERVER issued — a block's
 *  `boundaries` from a snapshot or an ack — never offsets computed here:
 *  an offset drifts to the wrong words the moment somebody types above it. */
export function openThread(
  pageId: string,
  body: {
    block_id: string;
    anchor_start: unknown;
    anchor_end: unknown;
    quoted: string;
    body: string;
  },
): Promise<Comment> {
  return collab(`${COLLAB_URL}/collab/pages/${pageId}/comments`, {
    method: "POST", body: JSON.stringify(body),
  });
}

export function replyToThread(threadId: string, body: string): Promise<Comment> {
  return collab(`${COLLAB_URL}/collab/threads/${threadId}/comments`, {
    method: "POST", body: JSON.stringify({ body }),
  });
}

export function setThreadResolved(threadId: string, resolved: boolean): Promise<void> {
  return collab(`${COLLAB_URL}/collab/threads/${threadId}/${resolved ? "resolve" : "reopen"}`, {
    method: "POST",
  });
}
