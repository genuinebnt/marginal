// notification-service's own HTTP surface — reached directly, not through
// api-gateway (nothing here is gRPC; see that service's package doc
// comment for why).
import { NOTIFICATIONS_URL } from "./config";
import { apiFetch } from "./http";

export interface Notification {
  id: string;
  kind: string;
  message: string;
  read_at?: string;
  created_at: string;
}

/** The list and the badge come from ONE response, so the bell and the panel
 *  cannot disagree about how many rows are unread. */
export interface NotificationList {
  notifications: Notification[];
  unread: number;
}

export function listNotifications(actorId: string): Promise<NotificationList> {
  return apiFetch(`${NOTIFICATIONS_URL}/notifications`, { actorId });
}

/** Clears one row. Already-read is `{cleared: 0}`, not an error — the two are
 *  the same outcome to a caller, and distinguishing them would confirm the
 *  existence of a row that is not yours. */
export function markNotificationRead(actorId: string, id: string): Promise<{ cleared: number }> {
  return apiFetch(`${NOTIFICATIONS_URL}/notifications/${id}/read`, { method: "POST", actorId });
}

export function markAllNotificationsRead(actorId: string): Promise<{ cleared: number }> {
  return apiFetch(`${NOTIFICATIONS_URL}/notifications/read-all`, { method: "POST", actorId });
}
