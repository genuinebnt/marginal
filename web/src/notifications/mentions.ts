/**
 * Resolving a mention at read time — docs/api/notifications.md § 1.
 *
 * The notification carries ids and nothing else, so every word the inbox
 * shows is fetched here, now: the page's title as it is today, the person's
 * current display name, and the comment itself with its anchor resolved
 * against the live rope.
 *
 * That is the whole point of storing a pointer. It also means this hook has
 * a real failure mode a copy would not have — the page may be gone, the
 * thread may be orphaned — and both are reported rather than smoothed over.
 * An inbox that quietly renders a stale quote is worse than one that says
 * the words are gone.
 */
import { useEffect, useState } from "react";
import { getPage } from "../api/pages";
import { listThreads, type Thread } from "../api/comments";
import { listSpaces, listMembers } from "../api/spaces";
import type { MentionPointer, Notification } from "../api/notifications";

export interface MentionContext {
  pageTitle: string;
  actorName: string;
  /** The comment's own text. Not a quotation of the page — the page's words
   *  live behind the anchor, and `orphaned` says whether they still do. */
  body: string;
  blockId: string;
  orphaned: boolean;
  range: { start: number; end: number } | null;
  threadId: string;
  pageId: string;
  /** The page is unreadable or gone. Said, not hidden. */
  missing: boolean;
}

/** Narrowed on `kind`, not on which fields are present: two pointer shapes
 *  now share one field name each way, and "has a page_id" would quietly
 *  become the wrong test the moment a third kind does too. */
function isMention(n: Notification): n is Notification & { pointer: MentionPointer } {
  return n.kind === "mention" && n.pointer !== undefined;
}

/** A short, stable tag for a person whose name we could not resolve. Two
 *  letters from the id, the same scheme the editor's rail uses — better than
 *  "Unknown", which reads as a claim about who they are. */
export function initials(id: string): string {
  let hash = 0;
  for (let i = 0; i < id.length; i++) hash = (hash * 31 + id.charCodeAt(i)) | 0;
  const a = "ABCDEFGHIJKLMNOPQRSTUVWXYZ";
  return a[Math.abs(hash) % 26] + a[Math.abs(hash >> 5) % 26];
}

export function useMentionContext(
  actorId: string | null,
  items: Notification[],
): Map<string, MentionContext> {
  const [ctx, setCtx] = useState<Map<string, MentionContext>>(new Map());
  // The pages to resolve, as a stable string — so this re-runs when a NEW
  // mention arrives rather than on every re-render of the same list.
  const pageKey = [...new Set(items.filter(isMention).map((n) => n.pointer.page_id))]
    .sort().join(",");

  useEffect(() => {
    if (!actorId || !pageKey) { setCtx(new Map()); return; }
    let live = true;
    void (async () => {
      const pageIds = pageKey.split(",");

      // Names come from the spaces the READER is in. Somebody who mentioned
      // you shares a space with you — that is enforced on the write side —
      // so this resolves every actor who could legitimately appear here,
      // and nobody who could not.
      const names = new Map<string, string>();
      try {
        const { spaces } = await listSpaces(actorId);
        const memberLists = await Promise.all(
          spaces.map((s) => listMembers(actorId, s.id).catch(() => ({ members: [] }))),
        );
        for (const { members } of memberLists) {
          for (const m of members) names.set(m.user_id, m.display_name);
        }
      } catch {
        // A name we cannot resolve becomes initials, not a blank row.
      }

      const titles = new Map<string, string | null>();
      const threads = new Map<string, Thread[]>();
      await Promise.all(pageIds.map(async (id) => {
        const [page, list] = await Promise.all([
          getPage(actorId, id).then((p) => p.title).catch(() => null),
          listThreads(id).then((r) => r.threads).catch(() => [] as Thread[]),
        ]);
        titles.set(id, page);
        threads.set(id, list);
      }));

      if (!live) return;
      const next = new Map<string, MentionContext>();
      for (const n of items.filter(isMention)) {
        const p = n.pointer;
        const t = (threads.get(p.page_id) ?? []).find((x) => x.id === p.thread_id);
        const comment = t?.comments.find((c) => c.id === p.comment_id);
        const title = titles.get(p.page_id);
        next.set(n.id, {
          pageTitle: title ?? "a page you can no longer open",
          actorName: names.get(p.actor_id) ?? initials(p.actor_id),
          body: comment?.body ?? "",
          blockId: p.block_id,
          orphaned: t?.orphaned ?? false,
          range: t?.range ?? null,
          threadId: p.thread_id,
          pageId: p.page_id,
          missing: title === null,
        });
      }
      setCtx(next);
    })();
    return () => { live = false; };
    // items is intentionally absent: pageKey already changes whenever the
    // set of mentioned pages does, and depending on the array itself would
    // re-fetch every render.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [actorId, pageKey]);

  return ctx;
}
