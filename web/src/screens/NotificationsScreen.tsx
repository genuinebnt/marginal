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
import { useCallback, useEffect, useMemo, useState } from "react";
import { useInbox } from "../notifications/NotificationsContext";
import { useNavigate } from "react-router-dom";
import { useAuth } from "../auth/AuthContext";
import { replyToThread } from "../api/comments";
import { listInvitations, respondToInvitation, type Invitation } from "../api/spaces";
import { listDanglingLinks, type DanglingLink } from "../api/graph";
import { createPage } from "../api/pages";
import type { Notification } from "../api/notifications";
import type { InvitePointer } from "../api/notifications";
import { useMentionContext } from "../notifications/mentions";
import { KIND_TONE, ago } from "../notifications/NotificationsPanel";
import {
  Body, Inspector, Label, Main, Readout, Rule, Screen, StatusBar, SubBar,
  SubItem, TopBar, num,
} from "../shell/Chrome";

/** The five facets § 20 names, and the stored kind each accepts. An empty
 *  `kinds` means "everything"; `null` means "nothing produces this yet". */
const FACETS: Array<{ id: string; label: string; kinds: string[] | null }> = [
  { id: "all", label: "ALL", kinds: [] },
  // Real since v3.3.0 — an @handle in a comment (docs/api/notifications.md).
  { id: "mentions", label: "MENTIONS", kinds: ["mention"] },
  { id: "proposals", label: "PROPOSALS", kinds: null },
  // Real since v3.3.0, and unlike every other facet these rows are
  // DERIVED rather than stored — see `checks` below.
  { id: "checks", label: "CHECKS", kinds: ["check"] },
  // Real since v3.3.0 — a space invitation (docs/api/spaces.md § 5).
  { id: "invites", label: "INVITES", kinds: ["invite"] },
];

export function NotificationsScreen() {
  const inbox = useInbox();
  const { session } = useAuth();
  const actorId = session?.actorId ?? null;
  const navigate = useNavigate();
  // Every word of a mention row is fetched here, now — the notification
  // itself carries only ids. See docs/api/notifications.md § 1.
  const mentions = useMentionContext(actorId, inbox.items);
  const [busy, setBusy] = useState<string | null>(null);
  // The caller's PENDING invitations, which is what makes an invite row
  // answerable: the notification carries the invitation's id, and this says
  // whether it is still open and what it is for. An invitation that has
  // been answered elsewhere therefore reads as answered here, rather than
  // offering two buttons that would both fail.
  const [invites, setInvites] = useState<Invitation[]>([]);
  const loadInvites = useCallback(() => {
    if (!actorId) return;
    listInvitations(actorId).then((r) => setInvites(r.invitations)).catch(() => setInvites([]));
  }, [actorId]);
  useEffect(loadInvites, [loadInvites]);

  /** § 20's CHECKS rows: `[[links]]` to pages nobody has written.
   *
   *  Computed on every load, never stored as notifications. A stored check
   *  would go stale the moment somebody created the page — the row would
   *  sit there asserting something that has stopped being true, which is
   *  worse than not showing it. Deriving it means CREATE PAGE makes the
   *  row disappear because the check passes now, not because anything
   *  cleared it. That is § 20's "acting on an item clears it" with no
   *  clearing machinery at all. */
  const [checks, setChecks] = useState<DanglingLink[]>([]);
  const [ignored, setIgnored] = useState<string[]>(() => {
    // Per-viewer, and only per-viewer. An ignored check is a statement
    // about what one person wants to see, not about the workspace — so it
    // lives in this browser and the panel says so.
    try { return JSON.parse(localStorage.getItem("marginal.ignoredChecks") ?? "[]"); }
    catch { return []; }
  });
  const loadChecks = useCallback(() => {
    if (!actorId) return;
    listDanglingLinks(actorId).then((r) => setChecks(r.links)).catch(() => setChecks([]));
  }, [actorId]);
  useEffect(loadChecks, [loadChecks]);

  const ignoreCheck = (title: string) => {
    const next = [...ignored, title];
    setIgnored(next);
    try { localStorage.setItem("marginal.ignoredChecks", JSON.stringify(next)); } catch { /* private mode */ }
  };
  const createMissingPage = async (title: string) => {
    if (!actorId) return;
    setBusy(title);
    try {
      await createPage(actorId, title);
      // Re-derived, not assumed: the row goes because the check passes.
      loadChecks();
    } finally {
      setBusy(null);
    }
  };
  const openChecks = useMemo(
    () => checks.filter((c) => !ignored.includes(c.target_title)),
    [checks, ignored],
  );
  const [facet, setFacet] = useState("all");
  const [insTab, setInsTab] = useState<"delivery" | "muted">("delivery");

  /** Derived rows, given the shape the list renders. They carry no id
   *  from the server because they are not stored anywhere — the target
   *  title is what identifies one. */
  const checkRows: Notification[] = useMemo(
    () => openChecks.map((c) => ({
      id: `check:${c.target_title}`,
      kind: "check",
      message: "",
      // A check has no moment it happened — it is a condition that is
      // true right now. Giving it `Date.now()` would be a made-up
      // timestamp that also sorts it above things that really did just
      // happen, which is why these are appended rather than merged by
      // time. The row says "checked just now, not stored" for the same
      // reason.
      created_at: "",
    })),
    [openChecks],
  );
  const allItems = useMemo(() => [...inbox.items, ...checkRows], [inbox.items, checkRows]);

  const active = FACETS.find((f) => f.id === facet)!;
  const shown = active.kinds === null
    ? []
    : active.kinds.length === 0
      ? allItems
      : allItems.filter((n) => active.kinds!.includes(n.kind));

  /** ACCEPT and DECLINE are the two halves of § 20's "decision" row: the
   *  act resolves the item, so the row clears without anybody marking it
   *  read. Both are real — accepting grants the role, declining records
   *  the refusal — which is why the list is reloaded afterwards rather
   *  than assumed. */
  const answerInvite = async (id: string, invitationId: string, accept: boolean) => {
    if (!actorId) return;
    setBusy(id);
    try {
      await respondToInvitation(actorId, invitationId, accept);
      await inbox.markRead(id);
      loadInvites();
    } finally {
      setBusy(null);
    }
  };

  /** § 20: "acting on an item clears it". A reply IS the act — the row is
   *  marked read because the thing it was asking for happened, not because
   *  somebody acknowledged seeing it. */
  const replyAndClear = async (id: string, threadId: string) => {
    const body = window.prompt("Reply in this thread");
    if (!body) return;
    setBusy(id);
    try {
      await replyToThread(threadId, body);
      await inbox.markRead(id);
    } finally {
      setBusy(null);
    }
  };

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
              ? allItems.length
              : allItems.filter((x) => f.kinds!.includes(x.kind)).length;
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
              const m = mentions.get(n.id);
              const chk = n.kind === "check"
                ? openChecks.find((c) => `check:${c.target_title}` === n.id)
                : undefined;
              const inv = n.kind === "invite"
                ? invites.find((i) => i.id === (n.pointer as InvitePointer | undefined)?.invitation_id)
                : undefined;
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
                    {chk ? (
                      <>
                        <div style={{ fontSize: 13.5, color: "#E4E2DC" }}>
                          A check you opened is still unresolved
                        </div>
                        <div style={{ fontSize: 12.5, color: "#8C8880", marginTop: 6, lineHeight: 1.5 }}>
                          Link to a page that does not exist yet —{" "}
                          <span style={{ color: "#E8873C" }}>[[{chk.target_title}]]</span>
                        </div>
                        <div className="mono" style={{ fontSize: 10, color: "#585550", marginTop: 7 }}>
                          in {chk.from_page_title} · checked just now, not stored
                        </div>
                      </>
                    ) : inv ? (
                      <>
                        <div style={{ fontSize: 13.5, color: read ? "#9B968D" : "#E4E2DC" }}>
                          <b style={{ fontWeight: 500 }}>{inv.invited_by_name}</b> invited you to{" "}
                          <span style={{ color: read ? "#9B968D" : "#3FCFA8" }}>{inv.space_name}</span>
                        </div>
                        <div className="mono" style={{ fontSize: 10, color: "#585550", marginTop: 7 }}>
                          {inv.role} role · {ago(n.created_at)}
                        </div>
                      </>
                    ) : n.kind === "invite" ? (
                      // The notification is here and the invitation is not
                      // in the pending list — it was answered somewhere
                      // else. Said, rather than drawing two buttons that
                      // would both fail.
                      <div style={{ fontSize: 12.5, color: "#8C8880" }}>
                        An invitation you have already answered.
                      </div>
                    ) : m ? (
                      <>
                        <div style={{ fontSize: 13.5, color: read ? "#9B968D" : "#E4E2DC" }}>
                          <b style={{ fontWeight: 500 }}>{m.actorName}</b> mentioned you in{" "}
                          <span style={{ color: read ? "#9B968D" : "#E8873C" }}>{m.pageTitle}</span>
                        </div>
                        {m.orphaned ? (
                          // The anchor no longer resolves. Said outright,
                          // because the alternative is quoting a ghost.
                          <div style={{ fontSize: 12, color: "#E0A34E", marginTop: 6, lineHeight: 1.5 }}>
                            the text this was written about has since been deleted
                          </div>
                        ) : (
                          <div style={{
                            fontSize: 12.5, lineHeight: 1.55, color: "#8C8880", marginTop: 6,
                            borderLeft: "2px solid rgba(255,255,255,.12)", paddingLeft: 9,
                          }}>
                            {m.body || <i>(the comment is no longer readable)</i>}
                          </div>
                        )}
                        <div className="mono" style={{ fontSize: 10, color: "#585550", marginTop: 7 }}>
                          block {m.blockId.slice(0, 4)}
                          {m.range && ` · chars ${m.range.start}\u2013${m.range.end}`}
                          {" \u00b7 "}{ago(n.created_at)}
                          {read && " \u00b7 cleared"}
                        </div>
                      </>
                    ) : (
                      <>
                        <div style={{ fontSize: 13.5, color: read ? "#9B968D" : "#E4E2DC" }}>{n.message}</div>
                        <div className="mono" style={{ fontSize: 10, color: "#585550", marginTop: 7 }}>
                          {n.kind} · {ago(n.created_at)}
                          {read && " · cleared"}
                        </div>
                      </>
                    )}
                  </div>
                  <div style={{ display: "flex", flexDirection: "column", gap: 6, alignItems: "flex-end" }}>
                    {chk && (
                      <>
                        <span
                          className="chip chip-a"
                          style={{ cursor: busy === chk.target_title ? "wait" : "pointer" }}
                          onClick={() => void createMissingPage(chk.target_title)}
                        >
                          CREATE PAGE
                        </span>
                        <span
                          className="chip"
                          style={{ cursor: "pointer" }}
                          onClick={() => ignoreCheck(chk.target_title)}
                        >
                          IGNORE
                        </span>
                      </>
                    )}
                    {inv && (
                      <>
                        <span
                          className="chip chip-t"
                          style={{ cursor: busy === n.id ? "wait" : "pointer" }}
                          onClick={() => void answerInvite(n.id, inv.id, true)}
                        >
                          ACCEPT
                        </span>
                        <span
                          className="chip"
                          style={{ cursor: busy === n.id ? "wait" : "pointer" }}
                          onClick={() => void answerInvite(n.id, inv.id, false)}
                        >
                          DECLINE
                        </span>
                      </>
                    )}
                    {m && (
                      <>
                        {/* § 20's design in one line: "acting on an item
                            clears it — there is no read button per row."
                            So REPLY posts a real reply AND clears the row;
                            it is not a second way to navigate. */}
                        <span
                          className="chip chip-e"
                          style={{ cursor: busy === n.id ? "wait" : "pointer" }}
                          onClick={() => void replyAndClear(n.id, m.threadId)}
                        >
                          REPLY
                        </span>
                        <span
                          className="chip"
                          style={{ cursor: "pointer" }}
                          onClick={() => navigate(`/pages/${m.pageId}`)}
                        >
                          OPEN BLOCK
                        </span>
                      </>
                    )}
                    {!read && !m && !inv && !chk && (
                      <span
                        className="chip chip-e"
                        style={{ cursor: "pointer" }}
                        onClick={() => inbox.markRead(n.id)}
                      >
                        MARK READ
                      </span>
                    )}
                  </div>
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
