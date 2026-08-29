/**
 * `[[Page Title]]` — from a rendered span back to a page you can open.
 *
 * The link is a decoration over plain text, not a stored mark (see
 * marks.ts's own note, and the Rust handbook's Part 3 for the decision this
 * defers). So resolution happens at CLICK time, against the page list the
 * screen already has, rather than at write time: a link to a page that does
 * not exist yet is a real, useful state — it is how you write forward — and
 * it must start working the moment someone creates the page, without
 * rewriting a single stored op.
 */
/** The only shape either function needs. Deliberately not `Page`: the caller
 *  with the RIGHT set (every live page) gets it from GetLinkGraph, whose
 *  nodes carry an id and a title and nothing else — and demanding a full Page
 *  would have forced a cast at every call site. */
export interface TitledPage {
  id: string;
  title: string;
}

/** Lowercased live titles, for rendering a dangling link differently. */
export function titleSet(pages: TitledPage[]): Set<string> {
  return new Set(pages.map((p) => p.title.trim().toLowerCase()));
}

/**
 * The page a click landed on, or null.
 *
 * Reads `data-title` off the nearest `.pl` ancestor. Matching is
 * case-insensitive and trimmed, which is the same normalisation
 * `blockproj`'s backlink scan uses server-side — two different answers to
 * "does this link resolve" would mean the graph and the click disagree.
 */
export function pageLinkTarget(e: { target: EventTarget | null }, pages: TitledPage[]): TitledPage | null {
  const el = (e.target as HTMLElement | null)?.closest?.(".pl") as HTMLElement | null;
  if (!el) return null;
  const title = (el.dataset.title ?? el.textContent?.replace(/^\[\[|\]\]$/g, "") ?? "").trim().toLowerCase();
  if (!title) return null;
  return pages.find((p) => p.title.trim().toLowerCase() === title) ?? null;
}

/** True when the click landed on a page link at all — resolved or not. So a
 *  dangling link can say "no such page yet" instead of doing nothing, which
 *  is the difference between a broken link and an unwritten one. */
export function isPageLinkClick(e: { target: EventTarget | null }): string | null {
  const el = (e.target as HTMLElement | null)?.closest?.(".pl") as HTMLElement | null;
  if (!el) return null;
  return (el.dataset.title ?? "").trim() || null;
}
