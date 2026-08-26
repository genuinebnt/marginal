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

export function listNotifications(actorId: string): Promise<{ notifications: Notification[] }> {
  return apiFetch(`${NOTIFICATIONS_URL}/notifications`, { actorId });
}
