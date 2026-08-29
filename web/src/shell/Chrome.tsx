/**
 * The chrome every in-app screen shares, ported from
 * docs/ui-mockups/v2/index.html — the .bar / .sub / .body / .status
 * structure DESIGN_GUIDELINES.md §1.1 and §6 specify.
 *
 * These are components rather than copied markup for exactly one reason:
 * the guidelines require the top-bar utility cluster to appear in a FIXED
 * ORDER on every in-app screen (§6.1), and 25 hand-written copies is 25
 * chances to reorder it. A control that relocates between routes is one you
 * have to re-find.
 *
 * Everything else about them is verbatim. Screens compose these and add
 * their own main column; they never restyle what is here.
 */
import { Link, useLocation } from "react-router-dom";
import type { ReactNode } from "react";

/** The six primary destinations, in the mockup's own order. */
const TABS: Array<{ label: string; to: string }> = [
  { label: "Write", to: "/pages" },
  { label: "Read", to: "/read" },
  { label: "Search", to: "/search" },
  { label: "Graph", to: "/graph" },
  { label: "History", to: "/history" },
  { label: "Lab", to: "/lab" },
];

/**
 * One readout in the top bar — a label over a value, always mono.
 * `tone` colours the value only; the key stays --ink-8 so a row of readouts
 * still scans as one group.
 */
/**
 * § 04's op-rate sparkline. Bars are drawn from an array, never authored —
 * the design system's own rule for charts (§6.7) — and the endpoint is
 * emphasised because it is the only value anyone reads off a sparkline.
 */
export function Spark({ values }: { values: number[] }) {
  const max = Math.max(...values, 1);
  return (
    <div className="spark">
      {values.map((v, i) => (
        <div
          key={i}
          style={{
            width: 3,
            height: Math.max(Math.round((v / max) * 15), 2),
            background: i === values.length - 1 ? "#E8873C" : `rgba(232,135,60,${0.35 + (v / max) * 0.15})`,
          }}
        />
      ))}
    </div>
  );
}

export function Readout({
  k, v, tone, size,
}: { k: string; v: ReactNode; tone?: string; size?: number }) {
  return (
    <div className="rd">
      <span className="rd-k">{k}</span>
      {/* size lands ON .rd-v, as the mockup applies it. Wrapping the value in
          an inner span looks identical and is not: the class itself then
          reports the default, which is what a style audit reads. */}
      <span
        className="rd-v"
        style={{ ...(tone ? { color: tone } : {}), ...(size ? { fontSize: size } : {}) }}
      >
        {v}
      </span>
    </div>
  );
}

export function Rule() {
  return <div style={{ height: 1, background: "rgba(255,255,255,.07)" }} />;
}

export function VRule() {
  return <div className="vr" />;
}

/** §2.3 — thousands are separated by a thin space, never a comma. */
export function num(n: number): string {
  return n.toLocaleString("en-US").replace(/,/g, " ");
}

/**
 * The utility cluster: clock → ⌘K → bell → admin → you. Fixed order, every
 * in-app screen. Pre-auth and meta screens deliberately omit it — a stranger
 * has no session to show — which is why it is opt-in via TopBar's `bare`.
 */
function Utility({ now, unread, peers }: { now: Date; unread: number; peers?: ReactNode }) {
  const time = now.toLocaleTimeString("en-GB", { hour: "2-digit", minute: "2-digit" });
  const date = now
    .toLocaleDateString("en-GB", { weekday: "short", day: "2-digit", month: "short" })
    .toUpperCase()
    .replace(",", "");
  return (
    <>
      <div className="clk">
        <b>{time}</b>
        <span>{date}</span>
      </div>
      <span className="kbd">⌘K</span>
      <Link to="/notifications" className="icb" style={{ textDecoration: "none" }}>
        ◎{unread > 0 && <div className="bdg">{unread}</div>}
      </Link>
      <Link to="/admin" className="icb" style={{ textDecoration: "none" }}>⚙</Link>
      <div className="av av-you">GN</div>
      {peers}
    </>
  );
}

export function TopBar({
  crumb,
  readouts,
  now = new Date(),
  unread = 0,
  bare = false,
  right,
  spark,
  peers,
}: {
  crumb?: ReactNode;
  readouts?: ReactNode;
  now?: Date;
  unread?: number;
  bare?: boolean;
  /** Trailing content for screens whose bar ends in something other than
   *  the utility cluster — the pre-auth screens' register/log-in switch.
   *  A prop rather than an overlay: .bar carries z-index 7, so anything
   *  positioned over it from outside loses. */
  right?: ReactNode;
  /** § 04's sparkline, between the readouts and the divider. */
  spark?: ReactNode;
  /** Presence avatars, which sit AFTER your own in the cluster. */
  peers?: ReactNode;
}) {
  const { pathname } = useLocation();
  return (
    <div className="bar">
      <span className="wm">
        m<span style={{ color: "#E8873C" }}>/</span>arginal
      </span>
      {!bare && (
        <div className="tabs">
          {TABS.map((t) => (
            <Link
              key={t.to}
              to={t.to}
              className={`tb${pathname.startsWith(t.to) ? " tb-on" : ""}`}
              style={{ textDecoration: "none" }}
            >
              {t.label}
            </Link>
          ))}
        </div>
      )}
      {crumb && <span className="crumb">{crumb}</span>}
      <div style={{ flex: 1 }} />
      {readouts}
      {spark}
      {right}
      {!bare && (
        <>
          <VRule />
          <Utility now={now} unread={unread} peers={peers} />
        </>
      )}
    </div>
  );
}

/** The 32px section strip. Items are plain text; `on` gets the ember underline. */
export function SubBar({ children }: { children: ReactNode }) {
  return <div className="sub">{children}</div>;
}

export function SubItem({
  children, on, tone, onClick,
}: { children: ReactNode; on?: boolean; tone?: string; onClick?: () => void }) {
  return (
    <span
      className={`sb${on ? " sb-on" : ""}`}
      style={{ ...(tone ? { color: tone } : {}), ...(onClick ? { cursor: "pointer" } : {}) }}
      onClick={onClick}
    >
      {children}
    </span>
  );
}

/**
 * The status bar. §6.4's contract, and it is a contract rather than a
 * decoration: `route` is where you are, `mechanism` is how the screen works
 * in a few words, and `state` is the screen's honest current state —
 * including when that state is bad. A UI that admits its index lags is more
 * trustworthy than one implying a transaction it does not have.
 */
export function StatusBar({
  route, mechanism, state, healthy = true,
}: { route: string; mechanism?: ReactNode; state?: ReactNode; healthy?: boolean }) {
  return (
    <div className="status">
      <span>{route}</span>
      {mechanism && <span>{mechanism}</span>}
      <div style={{ flex: 1 }} />
      {state && (
        <span style={{ color: healthy ? "#3FCFA8" : "#E0A34E" }}>
          {healthy ? "●" : "◌"} {state}
        </span>
      )}
    </div>
  );
}

/** The screen frame. `.sc` is the viewport here, not the mockup's artboard. */
export function Screen({ children }: { children: ReactNode }) {
  return <div className="sc">{children}</div>;
}

/** §1.1 — min-height:0 is load-bearing; without it flex children refuse to shrink. */
export function Body({ children }: { children: ReactNode }) {
  return <div className="body">{children}</div>;
}

/** The main column. min-width:0 for the same reason Body needs min-height:0. */
export function Main({
  children, style,
}: { children: ReactNode; style?: React.CSSProperties }) {
  return <div style={{ flex: 1, minWidth: 0, display: "flex", flexDirection: "column", ...style }}>{children}</div>;
}

/**
 * A section label. Always uppercase in the markup rather than via
 * text-transform, matching the mockup — the string in the source is the
 * string on screen.
 */
export function Label({ children, style }: { children: ReactNode; style?: React.CSSProperties }) {
  return <span className="lbl" style={style}>{children}</span>;
}

/** The right-hand inspector, with its own tab strip. */
export function Inspector({
  tabs, active, onSelect, children, width,
}: {
  tabs: Array<{ id: string; label: ReactNode }>;
  active: string;
  onSelect?: (id: string) => void;
  children: ReactNode;
  width?: number;
}) {
  return (
    <div className="insp" style={width ? { width } : undefined}>
      <div className="insp-t">
        {tabs.map((t) => (
          <span
            key={t.id}
            className={`it${t.id === active ? " it-on" : ""}`}
            style={onSelect ? { cursor: "pointer" } : undefined}
            onClick={() => onSelect?.(t.id)}
          >
            {t.label}
          </span>
        ))}
      </div>
      <div style={{ padding: 14, display: "flex", flexDirection: "column", gap: 12, overflowY: "auto" }}>
        {children}
      </div>
    </div>
  );
}

/**
 * The topic chip. `<i/>` is the 5px colour square and is required — the CSS
 * has nothing to size without it. colorKey maps to the categorical ramp,
 * which is deliberately disjoint from the semantic hues (§3.3/§3.4).
 */
const TOPIC_CLASS: Record<string, string> = {
  protocol: "tpc-proto",
  storage: "tpc-store",
  interface: "tpc-ui",
  operations: "tpc-ops",
  research: "tpc-rsch",
};

export function TopicChip({ name, colorKey }: { name: string; colorKey: string }) {
  return (
    <span className={`tpc ${TOPIC_CLASS[colorKey] ?? "tpc-proto"}`}>
      <i />
      {name.toUpperCase()}
    </span>
  );
}

/** A tag. The `#` is drawn by CSS ::before — never type it. */
export function Tag({
  children, on, onClick,
}: { children: ReactNode; on?: boolean; onClick?: () => void }) {
  return (
    <span
      className={`tg${on ? " tg-on" : ""}`}
      style={onClick ? { cursor: "pointer" } : undefined}
      onClick={onClick}
    >
      {children}
    </span>
  );
}

export const TOPIC_HEX: Record<string, string> = {
  protocol: "#7AA8E8",
  storage: "#C48AE0",
  interface: "#5AC8B4",
  operations: "#D6A660",
  research: "#D07C8A",
};
