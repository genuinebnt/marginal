/**
 * One inbox, shared by everything that draws it.
 *
 * The bell's badge is in the top bar of every in-app screen, the panel drops
 * from that bell, and § 20 is the same list full-screen. Three consumers, and
 * if each fetched for itself they would disagree the moment one of them
 * cleared a row — the badge saying 6 while the list below it shows 5 is the
 * classic notification bug, and it is a state-ownership bug, not a timing one.
 *
 * So the list lives here, once, and `unread` is the server's own COUNT rather
 * than a length over the (limited) list.
 */
import {
  createContext, useCallback, useContext, useEffect, useMemo, useState,
  type ReactNode,
} from "react";
import { useAuth } from "../auth/AuthContext";
import {
  listNotifications, markAllNotificationsRead, markNotificationRead,
  type Notification,
} from "../api/notifications";

interface InboxValue {
  items: Notification[];
  unread: number;
  /** Null until the first load resolves — distinguishes "empty inbox" from
   *  "not loaded", which the panel renders differently. */
  loaded: boolean;
  error: string | null;
  refresh: () => void;
  markRead: (id: string) => Promise<void>;
  markAllRead: () => Promise<void>;
  /** The bell's dropdown. Held here rather than in the bell because the
   *  panel itself renders at the SCREEN level — § 24c anchors it to the
   *  screen, and in document order it belongs after the body, not inside
   *  the top bar. Two components, one piece of state. */
  panelOpen: boolean;
  togglePanel: () => void;
  closePanel: () => void;
}

const Ctx = createContext<InboxValue | null>(null);

/** How often the inbox re-reads itself. notification-service has no socket of
 *  its own (the WebSocket belongs to collaboration-service and is per page),
 *  so this polls — stated plainly rather than dressed as push. */
const POLL_MS = 30_000;

export function NotificationsProvider({ children }: { children: ReactNode }) {
  const { session } = useAuth();
  const actorId = session?.actorId ?? null;
  const [items, setItems] = useState<Notification[]>([]);
  const [unread, setUnread] = useState(0);
  const [loaded, setLoaded] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [panelOpen, setPanelOpen] = useState(false);

  const refresh = useCallback(() => {
    if (!actorId) { setItems([]); setUnread(0); setLoaded(true); return; }
    listNotifications(actorId)
      .then((r) => {
        setItems(r.notifications);
        setUnread(r.unread);
        setError(null);
      })
      .catch((e) => setError(String(e)))
      .finally(() => setLoaded(true));
  }, [actorId]);

  useEffect(() => {
    refresh();
    if (!actorId) return;
    const t = setInterval(refresh, POLL_MS);
    return () => clearInterval(t);
  }, [refresh, actorId]);

  const markRead = useCallback(async (id: string) => {
    if (!actorId) return;
    // Optimistic, then reconciled by the refresh: clearing a row you can see
    // should not wait for a round trip, and the server's own count is what
    // the badge ends up showing either way.
    setItems((cur) => cur.map((n) => (n.id === id ? { ...n, read_at: new Date().toISOString() } : n)));
    setUnread((n) => Math.max(0, n - 1));
    await markNotificationRead(actorId, id).catch(() => {});
    refresh();
  }, [actorId, refresh]);

  const markAllRead = useCallback(async () => {
    if (!actorId) return;
    const now = new Date().toISOString();
    setItems((cur) => cur.map((n) => (n.read_at ? n : { ...n, read_at: now })));
    setUnread(0);
    await markAllNotificationsRead(actorId).catch(() => {});
    refresh();
  }, [actorId, refresh]);

  const togglePanel = useCallback(() => setPanelOpen((p) => !p), []);
  const closePanel = useCallback(() => setPanelOpen(false), []);

  // Escape closes it, from anywhere. A dropdown you can only close by
  // finding its trigger again is a dropdown that traps you.
  useEffect(() => {
    if (!panelOpen) return;
    const onKey = (e: KeyboardEvent) => { if (e.key === "Escape") setPanelOpen(false); };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [panelOpen]);

  const value = useMemo<InboxValue>(
    () => ({ items, unread, loaded, error, refresh, markRead, markAllRead, panelOpen, togglePanel, closePanel }),
    [items, unread, loaded, error, refresh, markRead, markAllRead, panelOpen, togglePanel, closePanel],
  );

  return <Ctx.Provider value={value}>{children}</Ctx.Provider>;
}

/** Safe outside the provider (the pre-auth screens): an empty inbox rather
 *  than a thrown error, because a login screen legitimately has none. */
export function useInbox(): InboxValue {
  return useContext(Ctx) ?? {
    items: [], unread: 0, loaded: true, error: null,
    refresh: () => {}, markRead: async () => {}, markAllRead: async () => {},
    panelOpen: false, togglePanel: () => {}, closePanel: () => {},
  };
}
