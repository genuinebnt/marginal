/**
 * The reading-progress rule (§1.3).
 *
 * Not decoration: it is the REASON the design has no scrollbars. A system
 * scrollbar is a light rectangle stapled to a dark ground, and it changes the
 * layout width the moment it appears. This carries position instead, in two
 * pixels that never reflow anything.
 *
 * It measures a real scroll container rather than the window, because the
 * document scrolls inside the main column while the chrome stays put.
 */
import { useEffect, useState, type RefObject } from "react";

export function ReadingProgress({ target }: { target: RefObject<HTMLElement | null> }) {
  const [pct, setPct] = useState(0);

  useEffect(() => {
    const el = target.current;
    if (!el) return;
    const measure = () => {
      const max = el.scrollHeight - el.clientHeight;
      // A document shorter than its viewport is entirely read, not 0% read.
      setPct(max <= 0 ? 100 : Math.min(100, (el.scrollTop / max) * 100));
    };
    measure();
    el.addEventListener("scroll", measure, { passive: true });
    const ro = new ResizeObserver(measure);
    ro.observe(el);
    return () => { el.removeEventListener("scroll", measure); ro.disconnect(); };
  }, [target]);

  return (
    <div className="prog" aria-hidden>
      <i style={{ width: `${pct}%` }} />
    </div>
  );
}

/**
 * The same measurement as a full-width BAR between the top bar and the
 * section strip (§ 05's own markup), rather than an overlay pinned inside a
 * scroll container.
 *
 * Two forms rather than one because they answer to different layouts: the
 * editor's canvas scrolls *under* its chrome, so its rule has to be sticky
 * inside that container; the reader's chrome is a flex column, so the bar can
 * simply be a row in it — which is stabler, since nothing about it depends on
 * the scroll container's padding.
 *
 * `pct` is passed in rather than measured here: the reader already reads the
 * same number for "58% · ~3 min left", and two measurements of one scroll
 * position is two numbers that can disagree on screen.
 */
export function ReadingBar({ pct }: { pct: number }) {
  return (
    <div className="rprog" aria-hidden>
      <i style={{ width: `${Math.max(0, Math.min(100, pct))}%` }} />
    </div>
  );
}
