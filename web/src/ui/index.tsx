/**
 * Shared list/card primitives, adapted from genuine-folio.
 *
 * That project's writing index, projects grid and series pages are all built
 * from four small pieces — a left-edge status/topic bar pair, a list/grid
 * toggle, a card, and a numbered path list — and reusing the IDEAS here
 * (rather than the CSS, which belongs to a different design system) is what
 * stops Marginal's six list-shaped screens from each inventing their own row.
 *
 * Everything below is drawn in the Instrument language: no radii, mono for
 * every readout, the topic ramp for accents, and the semantic hues reserved
 * for what they already mean (amber diagnostic, teal you, violet another
 * person, slate the assistant).
 */
import { Link } from "react-router-dom";
import type { CSSProperties, ReactNode } from "react";
import { TOPIC_HEX, num } from "../shell/Chrome";

/**
 * The left-edge pair on a list row: a short status tick above a full-height
 * topic bar.
 *
 * genuine-folio's `ContentRowBars`, and the reason to copy it is that it
 * solves a real problem in one glance — "what state is this in" and "what is
 * it about" are two different questions, and answering them with one coloured
 * dot means one of them is always the one you cannot see.
 */
export function RowBars({
  status = "ok", colorKey,
}: { status?: "ok" | "warn" | "muted" | "live"; colorKey?: string | null }) {
  // The SAME classes the page tree uses, not a second set that looks like
  // them. Two vocabularies for one mark is how the rail and the dashboard
  // end up disagreeing about what a topic bar is.
  const tick = { ok: "", warn: " tr-tick-del", live: " tr-tick-live", muted: "" }[status];
  return (
    <span className="tr-bars" aria-hidden>
      <span className={`tr-tick${tick}`} />
      <span
        className="tr-bar"
        style={colorKey ? { background: TOPIC_HEX[colorKey] ?? "#4B4842" } : undefined}
      />
    </span>
  );
}

export type ViewMode = "list" | "grid";

/** genuine-folio's `ViewToggle`, in chips. */
export function ViewToggle({
  mode, onChange,
}: { mode: ViewMode; onChange: (m: ViewMode) => void }) {
  return (
    <div style={{ display: "flex", gap: 6 }}>
      <span
        className={`chip${mode === "list" ? " chip-e" : ""}`}
        style={{ cursor: "pointer" }}
        onClick={() => onChange("list")}
      >
        ≡ LIST
      </span>
      <span
        className={`chip${mode === "grid" ? " chip-e" : ""}`}
        style={{ cursor: "pointer" }}
        onClick={() => onChange("grid")}
      >
        ⊞ GRID
      </span>
    </div>
  );
}

/** Average adult reading speed — the ONE place this constant lives, so the
 *  rail, the reader, a card and a series total cannot disagree about how long
 *  the same page takes. */
export const WORDS_PER_MINUTE = 220;

export function readMinutes(words: number): number | null {
  if (!words) return null;
  return Math.max(1, Math.round(words / WORDS_PER_MINUTE));
}

/**
 * A card in a grid of pages. genuine-folio's `ArticleGridCard`, reduced to
 * what this design system already has: a topic dot, a path line, a title, and
 * the two readouts worth the space.
 */
export function PageCard({
  to, topicName, colorKey, title, excerpt, meta, selected, delay = 0, onClick,
}: {
  to?: string;
  topicName?: string | null;
  colorKey?: string | null;
  title: string;
  excerpt?: ReactNode;
  meta?: ReactNode;
  selected?: boolean;
  delay?: number;
  onClick?: () => void;
}) {
  const accent = colorKey ? TOPIC_HEX[colorKey] ?? "#6E6A63" : "#6E6A63";
  const body = (
    <div
      className={selected ? "card card-on" : "card fx"}
      style={{ "--acc": accent, animationDelay: `${delay}s` } as CSSProperties}
      onClick={onClick}
    >
      {selected && <><div className="brk-tl" /><div className="brk-br" /></>}
      <div className="card-top">
        <span className="card-dot" style={{ background: accent }} />
        <span className="mono card-topic" style={{ color: topicName ? accent : "#E0A34E" }}>
          {(topicName ?? "untopiced").toUpperCase()}
        </span>
      </div>
      <div className="card-title">{title}</div>
      {excerpt && <div className="card-ex">{excerpt}</div>}
      {meta && <div className="card-meta mono">{meta}</div>}
    </div>
  );
  return to ? <Link to={to} style={{ textDecoration: "none" }}>{body}</Link> : body;
}

/**
 * "Series · Part 4 of 19", with the two arrows that matter.
 *
 * genuine-folio's `SeriesBanner`. It sits directly under the chrome on every
 * part, because the single most common thing a reader of part 4 wants is part
 * 5, and making them find it in a rail is making them find it.
 */
export function SeriesBanner({
  seriesTitle, seriesTo, number, total, prev, next,
}: {
  seriesTitle: string;
  seriesTo: string;
  number: number;
  total: number;
  prev?: { title: string; to: string } | null;
  next?: { title: string; to: string } | null;
}) {
  return (
    <div className="sbanner">
      <span className="lbl">SERIES</span>
      <span className="sbanner-of">
        Part <b>{num(number)}</b> of{" "}
        <Link to={seriesTo} className="sbanner-name">{seriesTitle}</Link>
        <span className="mono sbanner-total"> · {num(total)} parts</span>
      </span>
      {/* The progress rule doubles as the position readout: where you are in
          the series is the same fact as how much is left. */}
      <div className="sbanner-rule">
        <i style={{ width: `${Math.round((number / Math.max(total, 1)) * 100)}%` }} />
      </div>
      <div style={{ flex: 1 }} />
      {prev
        ? <Link to={prev.to} className="sbanner-nav" title={prev.title}>← {prev.title}</Link>
        : <span className="sbanner-nav sbanner-nav-off">← start of series</span>}
      {next
        ? <Link to={next.to} className="sbanner-nav sbanner-nav-next" title={next.title}>{next.title} →</Link>
        : <span className="sbanner-nav sbanner-nav-off">end of series</span>}
    </div>
  );
}

/**
 * "Read these, in this order" — genuine-folio's `ReadingPathList`, over
 * Marginal's real dependency layers.
 *
 * Numbered rather than bulleted, because the order IS the answer. The last
 * step is the page you are on: the list is what to read *up to* it, so ending
 * anywhere else would leave you wondering where you were going.
 */
export function ReadingPath({
  steps, hrefFor,
}: {
  steps: Array<{ page_id: string; title: string; depth: number; destination: boolean }>;
  hrefFor: (id: string) => string;
}) {
  if (steps.length <= 1) {
    return (
      <div style={{ fontSize: 11.5, lineHeight: 1.65, color: "#585550" }}>
        Nothing links to this page, so there is no path to it — start here. A page
        reached only through shared tags has neighbours, not prerequisites.
      </div>
    );
  }
  return (
    <ol className="path">
      {steps.map((s, i) => (
        <li key={s.page_id} className={`path-step${s.destination ? " path-dest" : ""}`}>
          <span className="mono path-n">{s.destination ? "→" : String(i + 1).padStart(2, "0")}</span>
          <Link to={hrefFor(s.page_id)} className="path-body">
            <span className="path-title">{s.title}</span>
            <span className="mono path-why">
              {s.destination ? "where you were heading" : `depth ${s.depth}`}
            </span>
          </Link>
        </li>
      ))}
    </ol>
  );
}
