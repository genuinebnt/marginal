// Mirrors services/documentcore/inline.go's Mark/MarkKind/Content model —
// same wire shape ({"tag":"bold"}, {"tag":"link","url":"..."}), same
// AddMark/RemoveMark split-or-merge semantics, ported by hand since this
// is a TS consumer of the same JSON contract, not a shared module
// (RFC-001's own note: "any TypeScript consumer must use the same
// offsets to agree on the same text").
//
// Known, accepted gap: offsets here are JS string indices (UTF-16 code
// units), not the byte offsets documentcore's Go side actually persists.
// Identical for plain ASCII text; a mark boundary can land one code unit
// off for text with multi-byte UTF-8 characters. Not fixed this pass —
// the same class of simplification doctext's own byte/rune conversion
// note already accepts elsewhere in this codebase.

export type MarkKind =
  | { tag: "bold" }
  | { tag: "italic" }
  | { tag: "strike" }
  | { tag: "code" }
  | { tag: "link"; url: string };

export interface Mark {
  kind: MarkKind;
  start: number;
  end: number;
}

function sameKind(a: MarkKind, b: MarkKind): boolean {
  if (a.tag !== b.tag) return false;
  if (a.tag === "link" && b.tag === "link") return a.url === b.url;
  return true;
}

/** Every mark whose range covers offset. */
export function marksAt(marks: Mark[], offset: number): MarkKind[] {
  return marks.filter((m) => m.start <= offset && offset < m.end).map((m) => m.kind);
}

/** True only if [start, end) is covered edge-to-edge by kind — used to
 * decide whether a bubble-menu button toggles on or off. */
export function isFullyMarked(marks: Mark[], kind: MarkKind, start: number, end: number): boolean {
  if (start >= end) return false;
  let cursor = start;
  const runs = marks.filter((m) => sameKind(m.kind, kind)).sort((a, b) => a.start - b.start);
  for (const m of runs) {
    if (m.start > cursor) return false;
    if (m.end > cursor) cursor = m.end;
    if (cursor >= end) return true;
  }
  return cursor >= end;
}

/** Adds kind over [start, end) — documentcore.Content.AddMark, minus
 * canonical merge-adjacent-runs normalisation (a cosmetic difference:
 * two touching same-kind marks instead of one still render identically,
 * since rendering checks "any mark of this kind active," not count). */
export function addMark(marks: Mark[], kind: MarkKind, start: number, end: number): Mark[] {
  if (start >= end) return marks;
  return [...marks, { kind, start, end }];
}

/** Subtracts [start, end) from every mark of kind — trims an edge, splits
 * a mark that fully covers the range into two, drops one fully covered by
 * it, or leaves disjoint marks untouched. Ports documentcore.Content.RemoveMark's
 * exact case split. */
export function removeMark(marks: Mark[], kind: MarkKind, start: number, end: number): Mark[] {
  if (start >= end) return marks;
  const kept: Mark[] = [];
  for (const m of marks) {
    if (!sameKind(m.kind, kind)) {
      kept.push(m);
      continue;
    }
    if (m.end <= start || m.start >= end) {
      kept.push(m); // disjoint
    } else if (m.start >= start && m.end <= end) {
      // fully covered by the removal — dropped
    } else if (m.start < start && m.end > end) {
      kept.push({ kind: m.kind, start: m.start, end: start });
      kept.push({ kind: m.kind, start: end, end: m.end });
    } else if (m.start < start) {
      kept.push({ kind: m.kind, start: m.start, end: start });
    } else {
      kept.push({ kind: m.kind, start: end, end: m.end });
    }
  }
  return kept;
}

/** Shifts/clips marks after text[oldText] became text[newText] — a plain
 * common-prefix/common-suffix diff, not a real text-diff algorithm. Marks
 * entirely within the common prefix or common suffix survive at their
 * (possibly shifted) offsets; any mark overlapping the actually-changed
 * middle region is dropped rather than guessed at. Good enough for the
 * common case (typing before/after a marked run); a mark spanning exactly
 * the edited region not surviving is a real, accepted limitation — full
 * rich-text-editing correctness needs a real diff/transform, out of scope
 * for this pass. */
export function shiftMarksForEdit(marks: Mark[], oldText: string, newText: string): Mark[] {
  if (marks.length === 0 || oldText === newText) return marks;

  let prefix = 0;
  const maxPrefix = Math.min(oldText.length, newText.length);
  while (prefix < maxPrefix && oldText[prefix] === newText[prefix]) prefix++;

  let suffix = 0;
  const maxSuffix = Math.min(oldText.length, newText.length) - prefix;
  while (suffix < maxSuffix && oldText[oldText.length - 1 - suffix] === newText[newText.length - 1 - suffix]) suffix++;

  const oldSuffixStart = oldText.length - suffix;
  const delta = newText.length - oldText.length;

  const out: Mark[] = [];
  for (const m of marks) {
    if (m.end <= prefix) {
      out.push(m); // entirely in the untouched prefix
    } else if (m.start >= oldSuffixStart) {
      out.push({ kind: m.kind, start: m.start + delta, end: m.end + delta }); // entirely in the untouched suffix, shifted
    }
    // otherwise the mark overlaps the changed region — dropped
  }
  return out;
}

const TAG_ORDER: Array<{ tag: MarkKind["tag"]; open: (k: MarkKind) => string; close: string }> = [
  { tag: "bold", open: () => "<strong>", close: "</strong>" },
  { tag: "italic", open: () => "<em>", close: "</em>" },
  { tag: "strike", open: () => "<s>", close: "</s>" },
  { tag: "code", open: () => "<code>", close: "</code>" },
  { tag: "link", open: (k) => `<a href="${escapeAttr(k.tag === "link" ? k.url : "")}" target="_blank" rel="noopener noreferrer">`, close: "</a>" },
];

function escapeHtml(s: string): string {
  return s.replace(/&/g, "&amp;").replace(/</g, "&lt;").replace(/>/g, "&gt;");
}
function escapeAttr(s: string): string {
  return escapeHtml(s).replace(/"/g, "&quot;");
}

/** Renders text+marks as safe, self-contained HTML — always escapes raw
 * text content; the only markup that appears is the fixed tag set above,
 * generated here, never taken from user input verbatim (link href is
 * attribute-escaped). Splits text at every mark boundary (a sweep over
 * sorted boundary points), then wraps each segment in whichever marks
 * are active there, outermost-tag-first per TAG_ORDER so nesting is
 * always well-formed regardless of mark insertion order. */
/**
 * Wraps [[Page Title]] spans so they read as the links they are.
 *
 * A page link is NOT a mark yet — it is plain text that blockproj scans with
 * a regex to build docs.page_links, and making it a real inline mark kind is
 * still open (see CLAUDE.md). Until then the affordance is a render-time
 * decoration over the same text, which keeps this side honest: nothing is
 * stored, and the text a user selects is exactly the text they typed.
 *
 * Applied to ALREADY-ESCAPED output, so there is no markup in the input to
 * confuse the pattern and no way for this to introduce an injection.
 *
 * Known limit: a mark boundary falling inside a [[...]] splits it across two
 * segments and the pattern will not match. It degrades to plain text, which
 * is the same thing it looked like before, so the failure is invisible
 * rather than broken.
 */
function linkify(escaped: string): string {
  return escaped.replace(/\[\[([^\[\]\n]+)\]\]/g, '<span class="pl">[[$1]]</span>');
}

export function renderMarkedHTML(text: string, marks: Mark[]): string {
  if (marks.length === 0) return linkify(escapeHtml(text));

  const boundaries = new Set<number>([0, text.length]);
  for (const m of marks) {
    boundaries.add(Math.max(0, Math.min(m.start, text.length)));
    boundaries.add(Math.max(0, Math.min(m.end, text.length)));
  }
  const points = [...boundaries].sort((a, b) => a - b);

  let html = "";
  for (let i = 0; i < points.length - 1; i++) {
    const segStart = points[i];
    const segEnd = points[i + 1];
    if (segStart === segEnd) continue;
    const active = marksAt(marks, segStart);
    let open = "";
    let close = "";
    for (const def of TAG_ORDER) {
      const k = active.find((a) => a.tag === def.tag);
      if (k) {
        open += def.open(k);
        close = def.close + close;
      }
    }
    html += open + linkify(escapeHtml(text.slice(segStart, segEnd))) + close;
  }
  return html;
}
