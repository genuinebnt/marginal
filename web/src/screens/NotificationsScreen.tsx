/**
 * docs/ui-mockups/v2/index.html § 20 NOTIFICATIONS, ported.
 *
 * The same inbox § 24c's panel shows, full screen — one source of truth
 * (NotificationsContext), so the badge, the panel and this list cannot
 * disagree about what is unread.
 *
 * The section's own claim is that an inbox is cleared BY ACTING, and the
 * evidence for it is the footer's three bars. Those are computed over the
 * real rows in this inbox, not authored: `resolved by acting` is not
 * measurable yet (no notification in this repo carries an action that
 * resolves it — see the panel's doc comment for why), so it reports 0 and
 * says so, rather than showing 84% because the mockup did.
 *
 * The five sub-bar facets are drawn even where nothing produces them. A nav
 * that lists more than exists must mark which is which (§ 9.4); silently
 * dropping four facets would make the inbox look complete instead of
 * one-fifth built.
 */
import { useMemo, useState } from "react";
import { useInbox } from "../notifications/NotificationsContext";
import { KIND_TONE, ago } from "../notifications/NotificationsPanel";
import {
  Body, Inspector, Label, Main, Readout, Rule, Screen, StatusBar, SubBar,
  SubItem, TopBar, num,
} from "../shell/Chrome";

/** The five facets § 20 names, and the stored kind each accepts. An empty
 *  `kinds` means "everything"; `null` means "nothing produces this yet". */
const FACETS: Array<{ id: string; label: string; kinds: string[] | null }> = [
  { id: "all", label: "ALL", kinds: [] },
  { id: "mentions", label: "MENTIONS", kinds: null },
  { id: "proposals", label: "PROPOSALS", kinds: null },
  { id: "checks", label: "CHECKS", kinds: null },
  { id: "invites", label: "INVITES", kinds: null },
];

export function NotificationsScreen() {
  const inbox = useInbox();
  const [facet, setFacet] = useState("all");
  const [insTab, setInsTab] = useState<"delivery" | "muted">("delivery");

  const active = FACETS.find((f) => f.id === facet)!;
  const shown = active.kinds === null
    ? []
    : active.kinds.length === 0
      ? inbox.items
      : inbox.items.filter((n) => active.kinds!.includes(n.kind));

  /** How this inbox actually emptied, over its own rows. */
  const emptying = useMemo(() => {
    const total = inbox.items.length;
    const read = inbox.items.filter((n) => n.read_at).length;
    const unread = total - read;
    const pct = (n: number) => (total === 0 ? 0 : Math.round((n / total) * 100));
    return [
      // Nothing in this repo resolves a notification by acting on it yet —
      // reported as zero rather than omitted, because the zero is the finding.
      { label: "Resolved by acting", n: 0, pct: 0, hue: "#3FCFA8" },
      { label: "Marked read", n: read, pct: pct(read), hue: "rgba(255,255,255,.25)" },
      { label: "Still unread", n: unread, pct: pct(unread), hue: "#E0A34E" },
    ];
  }, [inbox.items]);

  /** By source — the real kinds present, counted. */
  const bySource = useMemo(() => {
    const m = new Map<string, number>();
    inbox.items.forEach((n) => m.set(n.kind, (m.get(n.kind) ?? 0) + 1));
    return [...m].sort((a, b) => b[1] - a[1]);
  }, [inbox.items]);

  return (
    <Screen>
      <TopBar
        crumb={<>you / <b>inbox</b></>}
        readouts={<Readout k="UNREAD" v={num(inbox.unread)} tone={inbox.unread ? "#E8873C" : "#3FCFA8"} />}
        right={
          <span
            className="chip"
            style={{ cursor: inbox.unread ? "pointer" : "default", opacity: inbox.unread ? 1 : 0.5 }}
            onClick={() => inbox.unread && inbox.markAllRead()}
          >
            MARK ALL READ
          </span>
        }
      />

      <SubBar>
        {FACETS.map((f) => {
          const n = f.kinds === null
            ? 0
            : f.kinds.length === 0
              ? inbox.items.length
              : inbox.items.filter((x) => f.kinds!.includes(x.kind)).length;
          return (
            <SubItem key={f.id} on={facet === f.id} onClick={() => setFacet(f.id)}>
              {f.label} · {n}
            </SubItem>
          );
        })}
        <div style={{ flex: 1 }} />
        <SubItem>
          acting on an item clears it — there is no “read” button per row
        </SubItem>
      </SubBar>

      <Body>
        <Main style={{ padding: "22px 32px", overflow: "hidden" }}>
          <div style={{ flex: 1, minHeight: 0, overflowY: "auto" }}>
            {active.kinds === null && (
              <div style={{ padding: "18px 0", fontSize: 12.5, lineHeight: 1.7, color: "#585550", maxWidth: 620 }}>
                Nothing produces this facet yet. {active.label} needs a feature this repo has
                not built — comments for a mention, an assistant for a proposal, spaces for an
                invite, and a subscriber that turns a diagnostics finding into a durable row for
                a check. The facet is drawn and reports zero rather than being hidden, so the
                gap is visible instead of implied.
              </div>
            )}
            {active.kinds !== null && inbox.loaded && shown.length === 0 && (
              <div style={{ padding: "18px 0", fontSize: 12.5, lineHeight: 1.7, color: "#585550", maxWidth: 620 }}>
                Nothing here. When there is nothing, this screen is nearly empty — that is the
                correct state rather than a failure to fill it.
              </div>
            )}
            {shown.map((n, i) => {
              const tone = KIND_TONE[n.kind] ?? "#585550";
              const read = Boolean(n.read_at);
              return (
                <div
                  key={n.id}
                  className="fx"
                  style={{
                    display: "flex", gap: 14, padding: "16px 0",
                    borderBottom: i === shown.length - 1 ? "0" : "1px solid rgba(255,255,255,.07)",
                    animationDelay: `${i * 0.06}s`,
                  }}
                >
                  <div
                    className={read ? undefined : "dot"}
                    style={{ width: 7, height: 7, background: read ? "rgba(255,255,255,.12)" : tone, marginTop: 5 }}
                  >
                    {!read && <div className="ping" style={{ background: `${tone}80` }} />}
                  </div>
                  <div style={{ flex: 1 }}>
                    <div style={{ fontSize: 13.5, color: read ? "#9B968D" : "#E4E2DC" }}>{n.message}</div>
                    <div className="mono" style={{ fontSize: 10, color: "#585550", marginTop: 7 }}>
                      {n.kind} · {ago(n.created_at)}
                      {read && " · cleared"}
                    </div>
                  </div>
                  {!read && (
                    <div style={{ display: "flex", flexDirection: "column", gap: 6, alignItems: "flex-end" }}>
                      <span
                        className="chip chip-e"
                        style={{ cursor: "pointer" }}
                        onClick={() => inbox.markRead(n.id)}
                      >
                        MARK READ
                      </span>
                    </div>
                  )}
                </div>
              );
            })}
          </div>

          {/* Cleared-by-acting is the claim; this is the evidence for it. */}
          <div style={{
            marginTop: "auto", paddingTop: 20, borderTop: "1px solid rgba(255,255,255,.07)",
            display: "grid", gridTemplateColumns: "1fr 1fr 1.1fr", gap: 26,
          }}>
            <div>
              <Label style={{ marginBottom: 11, display: "block" }}>HOW THIS INBOX EMPTIES</Label>
              <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
                {emptying.map((row) => (
                  <div key={row.label} style={{ display: "flex", alignItems: "center", gap: 9 }}>
                    <span style={{ flex: 1, fontSize: 11.5, color: "#D2CFC8" }}>{row.label}</span>
                    <div style={{ width: 66, height: 5, background: "rgba(255,255,255,.06)" }}>
                      <div style={{ width: `${row.pct}%`, height: "100%", background: row.hue }} />
                    </div>
                    <span className="mono" style={{ fontSize: 9.5, color: "#8C8880", width: 30, textAlign: "right" }}>
                      {row.pct}%
                    </span>
                  </div>
                ))}
              </div>
              <div style={{ fontSize: 11, lineHeight: 1.6, color: "#585550", marginTop: 11 }}>
                “Resolved by acting” is 0 because no notification here carries an action that
                resolves it. That is the number to watch: anything routinely cleared by marking
                it read should not have been a notification.
              </div>
            </div>
            <div>
              <Label style={{ marginBottom: 11, display: "block" }}>BY SOURCE</Label>
              <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
                {bySource.length === 0 && (
                  <span style={{ fontSize: 11.5, color: "#585550" }}>No rows yet.</span>
                )}
                {bySource.map(([kind, n]) => (
                  <div key={kind} style={{ display: "flex", alignItems: "center", gap: 9 }}>
                    <div style={{ width: 6, height: 6, background: KIND_TONE[kind] ?? "#585550" }} />
                    <span style={{ flex: 1, fontSize: 11.5, color: "#D2CFC8" }}>{kind}</span>
                    <span className="mono" style={{ fontSize: 10.5, color: "#8C8880" }}>{num(n)}</span>
                  </div>
                ))}
              </div>
            </div>
            <div>
              <Label style={{ marginBottom: 11, display: "block" }}>WHAT NEVER NOTIFIES</Label>
              <div style={{ fontSize: 11.5, lineHeight: 1.7, color: "#8C8880" }}>
                Your own ops. Presence. Anything in a muted space. Someone editing a page you
                happen to have open — you can already see them doing it, and a notification for
                something visible is noise wearing a badge.
              </div>
            </div>
          </div>
        </Main>

        <Inspector
          tabs={[{ id: "delivery", label: "DELIVERY" }, { id: "muted", label: "MUTED" }]}
          active={insTab}
          onSelect={(id) => setInsTab(id as "delivery" | "muted")}
        >
        {insTab === "muted" ? (
          <div style={{ display: "flex", flexDirection: "column", gap: 12 }}>
            <Label>MUTED</Label>
            <div style={{ fontSize: 11.5, lineHeight: 1.65, color: "#8C8880" }}>
              <span style={{ color: "#E0A34E" }}>Muting does not exist.</span> There is one
              notification topic in this repo —{" "}
              <span className="mono" style={{ color: "#9B968D" }}>auth.user_registered</span>{" "}
              — and nothing to mute it against: no per-user preferences store, and no
              second topic to prefer over it.
            </div>
            <div style={{ fontSize: 11, lineHeight: 1.6, color: "#585550" }}>
              Every other notification-worthy event needs a feature that is not built:
              mentions and comments (<span className="mono">v3.2.0</span>), sharing and
              roles (<span className="mono">v3.1.0</span>). Preferences arrive with them
              (<span className="mono">v3.3.0</span>) — a mute list before there is anything
              to mute would be a control with nothing behind it.
            </div>
          </div>
        ) : (
          <>
          <Label>HOW THESE ARRIVE</Label>
          <div style={{ display: "flex", alignItems: "center", gap: 10, fontSize: 12, color: "#8C8880" }}>
            <span style={{ color: "#3FCFA8" }}>✓</span>In-app, polled every 30 s
          </div>
          <div style={{ display: "flex", alignItems: "center", gap: 10, fontSize: 12, color: "#585550" }}>
            <span style={{ color: "#4B4842" }}>○</span>Email digest — not built
          </div>
          <div style={{ display: "flex", alignItems: "center", gap: 10, fontSize: 12, color: "#585550" }}>
            <span style={{ color: "#4B4842" }}>○</span>Push — not built
          </div>
          <div style={{ fontSize: 11, lineHeight: 1.65, color: "#585550" }}>
            Polling, not push, and said plainly: notification-service has no socket of its own —
            the WebSocket in this system belongs to collaboration-service and is scoped to one
            page.
          </div>

          <Rule />
          <Label>RULE</Label>
          <div style={{ fontSize: 11.5, lineHeight: 1.65, color: "#8C8880" }}>
            A notification is a pointer to an anchor, never a copy of the text. Open it a week
            later and it still lands on the right words, wherever they moved — and if they were
            deleted, it says so instead of quoting a ghost.
          </div>

          <Rule />
          <Label>THIS INBOX</Label>
          <div style={{ display: "flex", alignItems: "baseline", gap: 8, fontSize: 11.5, color: "#9B968D" }}>
            <span style={{ flex: 1 }}>rows</span>
            <span className="mono" style={{ fontSize: 11, color: "#E4E2DC" }}>{num(inbox.items.length)}</span>
          </div>
          <div style={{ display: "flex", alignItems: "baseline", gap: 8, fontSize: 11.5, color: "#9B968D" }}>
            <span style={{ flex: 1 }}>unread</span>
            <span className="mono" style={{ fontSize: 11, color: inbox.unread ? "#E8873C" : "#3FCFA8" }}>
              {num(inbox.unread)}
            </span>
          </div>
          <div style={{ display: "flex", alignItems: "baseline", gap: 8, fontSize: 11.5, color: "#9B968D" }}>
            <span style={{ flex: 1 }}>kinds produced</span>
            <span className="mono" style={{ fontSize: 11, color: "#E4E2DC" }}>{num(bySource.length)} of 5</span>
          </div>

          <Rule />
          <Label>WHY AN ANCHOR, NOT A COPY</Label>
          <div style={{ fontSize: 11.5, lineHeight: 1.65, color: "#8C8880" }}>
            Storing the quoted text would freeze it. Storing the anchor means opening a mention a
            week later lands on the right words even after everything around them was rewritten.
          </div>
          </>
        )}
        </Inspector>
      </Body>

      <StatusBar
        route="/notifications"
        mechanism="one inbox · cleared by acting"
        state={inbox.unread ? `${inbox.unread} unread` : "nothing unread"}
        healthy={inbox.unread === 0}
      />
    </Screen>
  );
}

export default NotificationsScreen;
