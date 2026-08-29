/**
 * docs/ui-mockups/v2/index.html § 24 OFFLINE / RECONNECT — the strip.
 *
 * § 24 calls itself "the state every route can enter", and the reason it gets
 * a whole screen is that the honest answer to a dropped socket is not an
 * error dialog: nothing is lost, the ops are queued in order, and the only
 * things that stop working are the ones that need the server.
 *
 * The countdown is shown rather than hidden. A banner that says "retrying"
 * and nothing else is one you cannot tell from a hung one, and the difference
 * between those two is the entire question a person has while looking at it.
 */
import { useEffect, useState } from "react";

export function OfflineBanner({
  state, queued, attempt, retryAt, onRetry,
}: {
  state: "connecting" | "open" | "closed";
  queued: number;
  attempt: number;
  retryAt: number | null;
  onRetry: () => void;
}) {
  // A countdown has to tick. One interval, only while there is something to
  // count down to.
  const [, force] = useState(0);
  useEffect(() => {
    if (retryAt === null) return;
    const t = setInterval(() => force((n) => n + 1), 500);
    return () => clearInterval(t);
  }, [retryAt]);

  if (state === "open") return null;

  const seconds = retryAt === null ? null : Math.max(0, Math.ceil((retryAt - Date.now()) / 1000));

  return (
    <div className="offline">
      <div className="offline-spin" />
      <span style={{ fontSize: 12, color: "#E0A34E" }}>
        {state === "connecting" && attempt === 0
          ? "Connecting…"
          : "Offline — your edits are being kept locally"}
      </span>
      <span className="mono" style={{ fontSize: 10.5, color: "#8C8880" }}>
        {queued > 0 ? `${queued} op${queued === 1 ? "" : "s"} queued` : "nothing queued"}
        {seconds !== null && ` · retrying in ${seconds} s`}
        {attempt > 0 && ` · attempt ${attempt + 1}`}
      </span>
      <div style={{ flex: 1 }} />
      <span className="chip chip-a" style={{ cursor: "pointer" }} onClick={onRetry}>
        RETRY NOW
      </span>
    </div>
  );
}
