/**
 * docs/ui-mockups/v2/index.html § 24c NOTIFICATIONS PANEL, ported.
 *
 * Anchored under the bell rather than centred, which is the section's own
 * argument: a panel that drops from its trigger tells you what opened it
 * without a title bar saying so.
 *
 * What is real here and what is not, stated rather than implied. This repo
 * produces two notification kinds: `welcome` (auth.user_registered) and,
 * since `v3.3.0`, `mention` (collab.comment_mentioned — an @handle in a
 * comment; docs/api/notifications.md). Assistant proposals and stale-fact
 * alerts each still need a feature that does not exist yet, so their tabs
 * are drawn and report zero rather than being hidden: § 9.4's rule that a
 * nav listing more than exists must mark which is which. The alternative — inventing rows so
 * the panel looks busy — is the one thing this panel must not do, since the
 * whole claim of the screen is that an inbox empties by acting on real work.
 */
import { Link } from "react-router-dom";
import { useMemo, useState } from "react";
import { useAuth } from "../auth/AuthContext";
import { useMentionContext, type MentionContext } from "./mentions";
import { useInbox } from "./NotificationsContext";
import type { Notification } from "../api/notifications";

/**
 * The tabs § 24c draws, and which stored `kind` each one accepts.
 *
 * `null` means "no producer in this repo yet" — the tab still renders, still
 * counts (to zero), and says so when selected.
 */
const TABS: Array<{ id: string; label: string; tone: string; kinds: string[] | null }> = [
  { id: "needs", label: "NEEDS YOU", tone: "#E8873C", kinds: null },
  { id: "mentions", label: "MENTIONS", tone: "#A98CE8", kinds: ["mention"] },
  { id: "checks", label: "CHECKS", tone: "#E0A34E", kinds: null },
  { id: "all", label: "ALL", tone: "#8C8880", kinds: [] },
];

/** Relative time, to the granularity § 24c actually prints. */
export function ago(iso: string): string {
  const secs = Math.max(0, (Date.now() - new Date(iso).getTime()) / 1000);
  if (secs < 90) return "just now";
  const mins = Math.round(secs / 60);
  if (mins < 60) return `${mins} min ago`;
  const hours = Math.round(mins / 60);
  if (hours < 24) return `${hours}h ago`;
  const days = Math.round(hours / 24);
  return days === 1 ? "yesterday" : `${days} days ago`;
}

/** Which accent a stored kind takes. One place, so the panel and § 20 agree. */
export const KIND_TONE: Record<string, string> = {
  welcome: "#3FCFA8",
  mention: "#A98CE8",
  proposal: "#7D9EC9",
  check: "#E0A34E",
  invite: "#3FCFA8",
};

export function NotificationRow({
  n, onRead, context,
}: {
  n: Notification;
  onRead: (id: string) => void;
  /** Resolved at read time for a pointer-shaped kind. Absent for `welcome`,
   *  whose whole content is its message. */
  context?: MentionContext;
}) {
  const tone = KIND_TONE[n.kind] ?? "#585550";
  const read = Boolean(n.read_at);
  return (
    <div style={{
      display: "flex", gap: 11, padding: "12px 14px",
      borderLeft: `2px solid ${read ? "rgba(255,255,255,.09)" : tone}`,
      background: read ? "transparent" : `${tone}0D`,
      opacity: read ? 0.72 : 1,
    }}>
      <span style={{ color: tone, fontSize: 13, width: 22, textAlign: "center" }}>◎</span>
      <div style={{ flex: 1 }}>
        {context ? (
          <>
            <div style={{ fontSize: 12.5, lineHeight: 1.5, color: "#D2CFC8" }}>
              <b style={{ fontWeight: 500 }}>{context.actorName}</b> mentioned you on{" "}
              <span style={{ color: "#E8873C" }}>{context.pageTitle}</span>
            </div>
            <div style={{
              fontSize: 11.5, lineHeight: 1.5, marginTop: 5,
              color: context.orphaned ? "#E0A34E" : "#8C8880",
            }}>
              {context.orphaned
                ? "the text this was written about has since been deleted"
                : context.body}
            </div>
          </>
        ) : (
          <div style={{ fontSize: 12.5, lineHeight: 1.5, color: "#D2CFC8" }}>{n.message}</div>
        )}
        <div className="mono" style={{ fontSize: 10, color: "#585550", marginTop: 5 }}>
          {n.kind} · {ago(n.created_at)}{read ? " · read" : ""}
        </div>
        {!read && (
          <div style={{ display: "flex", gap: 7, marginTop: 9 }}>
            <span
              className="chip chip-e"
              style={{ padding: "2px 9px", cursor: "pointer" }}
              onClick={() => onRead(n.id)}
            >
              MARK READ
            </span>
          </div>
        )}
      </div>
    </div>
  );
}

export function NotificationsPanel({ onClose }: { onClose: () => void }) {
  const inbox = useInbox();
  const { session } = useAuth();
  // Resolved only while the panel is open — this component mounts on the
  // bell's click, so a closed panel costs nothing.
  const mentions = useMentionContext(session?.actorId ?? null, inbox.items);
  const [tab, setTab] = useState("all");

  const counts = useMemo(() => {
    const m: Record<string, number> = {};
    for (const t of TABS) {
      m[t.id] = t.kinds === null
        ? 0
        : t.kinds.length === 0
          ? inbox.items.filter((n) => !n.read_at).length
          : inbox.items.filter((n) => !n.read_at && t.kinds!.includes(n.kind)).length;
    }
    return m;
  }, [inbox.items]);

  const active = TABS.find((t) => t.id === tab)!;
  const shown = active.kinds === null
    ? []
    : active.kinds.length === 0
      ? inbox.items
      : inbox.items.filter((n) => active.kinds!.includes(n.kind));

  return (
    <div
      style={{
        position: "absolute", right: 76, top: 52, width: 412, background: "#131415",
        border: "1px solid rgba(255,255,255,.14)",
        boxShadow: "0 26px 70px -18px rgba(0,0,0,.95)", zIndex: 21,
        display: "flex", flexDirection: "column", maxHeight: 660,
      }}
      // The bell toggles it; clicking inside must not close it, and the
      // top bar behind it must not receive the click either.
      onClick={(e) => e.stopPropagation()}
    >
      <div style={{
        display: "flex", alignItems: "center", gap: 10, padding: "12px 14px",
        borderBottom: "1px solid rgba(255,255,255,.08)",
      }}>
        <span className="lbl">INBOX</span>
        <span className="chip chip-e" style={{ padding: "2px 7px" }}>{inbox.unread} UNREAD</span>
        <div style={{ flex: 1 }} />
        <span
          className="mono"
          style={{ fontSize: 10, color: inbox.unread ? "#8C8880" : "#585550", cursor: "pointer" }}
          onClick={() => inbox.markAllRead()}
        >
          mark all read
        </span>
      </div>

      <div style={{
        display: "flex", gap: 0, padding: "0 14px",
        borderBottom: "1px solid rgba(255,255,255,.08)",
      }}>
        {TABS.map((t) => (
          <span
            key={t.id}
            className={`it${t.id === tab ? " it-on" : ""}`}
            style={{ padding: "8px 8px 9px", cursor: "pointer" }}
            onClick={() => setTab(t.id)}
          >
            {t.label}{" "}
            {t.id !== "all" && <span style={{ color: t.tone }}>{counts[t.id]}</span>}
          </span>
        ))}
      </div>

      <div style={{ padding: "4px 0", overflowY: "auto" }}>
        {active.kinds === null && (
          <div style={{ padding: "16px 14px", fontSize: 11.5, lineHeight: 1.65, color: "#585550" }}>
            No producer for this yet. {active.label === "MENTIONS"
              ? "A mention needs comments, which are not built."
              : active.label === "CHECKS"
                ? "diagnostics-service computes checks per request; nothing subscribes to them and turns one into an inbox row."
                : "An item lands here when it carries a decision — an assistant proposal or a space invite. Neither exists yet."}{" "}
            Drawn rather than hidden, so the gap is visible instead of implied.
          </div>
        )}
        {active.kinds !== null && !inbox.loaded && (
          <div style={{ padding: "16px 14px", fontSize: 11.5, color: "#585550" }}>Loading…</div>
        )}
        {active.kinds !== null && inbox.loaded && shown.length === 0 && (
          <div style={{ padding: "16px 14px", fontSize: 11.5, lineHeight: 1.65, color: "#585550" }}>
            Nothing here. An empty inbox is the correct state, not a failure to fill one.
          </div>
        )}
        {inbox.error && (
          <div style={{ padding: "16px 14px", fontSize: 11.5, color: "#E0A34E" }}>{inbox.error}</div>
        )}
        {shown.map((n) => (
          <NotificationRow key={n.id} n={n} onRead={inbox.markRead} context={mentions.get(n.id)} />
        ))}
      </div>

      <div style={{
        marginTop: "auto", padding: "10px 14px", borderTop: "1px solid rgba(255,255,255,.08)",
        background: "#0F1011", display: "flex", alignItems: "center", gap: 10,
      }}>
        <span className="mono" style={{ fontSize: 10, color: "#585550" }}>
          every item is a pointer to an anchor
        </span>
        <div style={{ flex: 1 }} />
        <Link
          to="/notifications"
          className="mono"
          style={{ fontSize: 10, color: "#E8873C", textDecoration: "none" }}
          onClick={onClose}
        >
          open inbox →
        </Link>
      </div>
    </div>
  );
}
