/**
 * Placeholder data, and the rule for it.
 *
 * The screens are built against docs/ui-mockups/v2/ in full, ahead of the
 * backend that will fill them. That is deliberate — it is the only way the
 * layout can be checked 1:1 against the reference — but it creates one real
 * hazard: a screen that looks finished while showing numbers nobody
 * computed.
 *
 * So every value not yet backed by a real endpoint goes through here, and
 * carries a visible mark. The convention is the mockup's own (§9.4): what is
 * not built is dimmed and labelled rather than quietly omitted, because a UI
 * that silently invents its own data is how a design starts lying about its
 * own coverage.
 *
 * The workflow this enables: build the screen faithfully now, then delete
 * `ph(...)` call sites one at a time as endpoints land. A screen is finished
 * when `grep -c "ph(" ` on it returns 0 — which is a check, not a feeling.
 */
import type { ReactNode } from "react";

/**
 * Marks a value as mockup-derived rather than computed. Returns the value
 * unchanged so it can wrap an expression inline without restructuring:
 *
 *   <span className="rd-v">{ph(1908)}</span>
 *
 * It is intentionally NOT a component — wrapping a number in an element
 * would change layout, and the whole point is that swapping in real data
 * later must not move anything.
 */
export function ph<T>(value: T): T {
  return value;
}

/**
 * Visible marker for a whole region that is mockup data. Use on a panel or
 * a list, once, rather than on every value inside it — a screen speckled
 * with badges is unreadable, and the honest unit is "this panel is not
 * wired", not "this integer is fake".
 */
export function PlaceholderNote({ children }: { children?: ReactNode }) {
  return (
    <div
      style={{
        display: "flex",
        alignItems: "center",
        gap: 7,
        padding: "5px 8px",
        border: "1px solid rgba(224,163,78,.28)",
        background: "rgba(224,163,78,.05)",
        font: "400 9.5px 'IBM Plex Mono', monospace",
        letterSpacing: ".06em",
        color: "#E0A34E",
      }}
    >
      <span>◌</span>
      <span style={{ color: "#8C8880" }}>
        {children ?? "mockup data — no endpoint yet"}
      </span>
    </div>
  );
}

/**
 * Dims a nav entry or list row whose destination does not exist yet, and
 * removes the pointer affordance. Spread onto the element's style.
 *
 * Guidelines §9.4: a nav that lists more routes than exist must mark which
 * is which — full contrast with a `→` for what is drawn, .45 opacity for the
 * rest — rather than quietly omitting the difference.
 */
export const undrawn: React.CSSProperties = { opacity: 0.45, cursor: "default" };
