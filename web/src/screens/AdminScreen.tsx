/**
 * docs/ui-mockups/v2/index.html § 18 ADMIN, ported.
 *
 * Every number here is fetched, and each comes from a different
 * place on purpose:
 *
 *   - SERVICES is a live probe the gateway performs per request.
 *     "up" means one thing only — this gateway asked and got an
 *     answer in time. It does NOT mean the service's database is
 *     reachable, and the panel says so rather than borrowing the
 *     credibility of the word "healthy" without earning it.
 *   - PEOPLE and its session count come from auth-service.
 *   - The queue numbers come from collaboration-service directly,
 *     the same convention its other instance-fact endpoints
 *     follow.
 *
 * "Sessions" means three different things in this system —
 * refresh tokens not yet revoked, pages with a live rope, and
 * connected editors — so no readout here is labelled just
 * SESSIONS. Two of the three are shown and each says which it is.
 *
 * The rail's dimmed rows are the admin surfaces this repo has not
 * built. They are drawn rather than omitted for the same reason
 * the Lab hub lists its unbuilt screens: a menu of only what
 * exists cannot be read as a map of what the section is for.
 */
import { useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import {
  getCollabStats, getHealth, getPeople,
  type CollabStats, type HealthReport, type People,
} from "../api/admin";
import { useAuth } from "../auth/AuthContext";
import {
  Body, Label, Main, Readout, Screen, StatusBar, TopBar, num,
} from "../shell/Chrome";

/** Rail rows. `to` present = built and reachable. */
const NAV: Array<{ name: string; to?: string; note?: string }> = [
  { name: "Health" },
  // The next four are panels on THIS page, not separate views —
  // bright, because the data is right there, and not links,
  // because there is nowhere else to go.
  { name: "Services" },
  { name: "Queues & outbox" },
  { name: "People" },
  { name: "Sessions" },
  { name: "Storage & quota", note: "needs object storage — not in this repo" },
  { name: "Index & embeddings", note: "v4.4.0" },
  { name: "Backups", note: "no backup system exists yet" },
  // The one row that goes somewhere real.
  { name: "Jobs & sagas", to: "/trash" },
  { name: "Feature flags", note: "v3.5.0" },
  { name: "Audit log", note: "v3.5.0 — derived from the op log, § 18b" },
  { name: "API keys", note: "v3.1.0 — needs RBAC first, § 18c" },
  { name: "Licence & version" },
];

function bytes(n: number): string {
  if (n >= 1e9) return `${(n / 1e9).toFixed(1)} GB`;
  if (n >= 1e6) return `${(n / 1e6).toFixed(1)} MB`;
  if (n >= 1e3) return `${(n / 1e3).toFixed(0)} kB`;
  return `${n} B`;
}

/** Seconds → the coarse form this screen wants. Hours matter
 *  here; milliseconds do not. */
function since(sec: number): string {
  if (sec >= 86400) return `${(sec / 86400).toFixed(1)} d`;
  if (sec >= 3600) return `${(sec / 3600).toFixed(1)} h`;
  if (sec >= 60) return `${Math.round(sec / 60)} m`;
  return `${sec.toFixed(1)} s`;
}

const STATUS_HUE: Record<string, string> = {
  up: "#3FCFA8", down: "#E0A34E", timeout: "#E0A34E",
};

export function AdminScreen() {
  const navigate = useNavigate();
  const { session } = useAuth();
  const actorId = session?.actorId ?? null;

  const [health, setHealth] = useState<HealthReport | null>(null);
  const [people, setPeople] = useState<People | null>(null);
  const [stats, setStats] = useState<CollabStats | null>(null);
  const [errs, setErrs] = useState<string[]>([]);
  const [active, setActive] = useState("Health");

  useEffect(() => {
    const fail = (what: string) => (e: unknown) =>
      setErrs((prev) => [...prev, `${what}: ${String(e)}`]);
    // Three independent fetches, and one failing must not blank
    // the other two — a health screen that goes dark because one
    // of its sources is down is the least useful moment for it
    // to do that.
    getHealth().then(setHealth).catch(fail("health"));
    getPeople(actorId).then(setPeople).catch(fail("people"));
    getCollabStats().then(setStats).catch(fail("queues"));
  }, [actorId]);

  const perHour = stats?.ops_per_hour ?? [];
  const maxHour = Math.max(1, ...perHour);
  const quiet = perHour.length > 0 && perHour.every((n) => n === 0);

  const degraded = health != null && health.up < health.total;
  const stale = (stats?.outbox_oldest_seconds ?? 0) > 30;

  return (
    <Screen>
      <TopBar
        // No nav tabs: admin sits outside the five-tab set, and
        // the gear in the utility cluster is how you get here.
        noTabs
        crumb={<>workspace / <b>admin</b></>}
        readouts={
          <>
            <Readout
              k="OUTBOX DEPTH"
              v={num(stats?.outbox_depth ?? 0)}
              tone={stale ? "#E0A34E" : "#3FCFA8"}
            />
            <Readout
              k="OP-LOG LAG"
              v={stats ? since(stats.lag_seconds) : "—"}
              tone={undefined}
            />
          </>
        }
      />

      <Body>
        <div className="rail" style={{ width: 212 }}>
          <div className="rail-h">ADMIN<div /></div>
          <div style={{ display: "flex", flexDirection: "column", padding: "0 8px", gap: 1 }}>
            {NAV.map((n) => {
              const row = (
                <>
                  {n.name === active && <i />}
                  {n.name}
                  {n.to && <span className="tr-n" style={{ marginLeft: "auto" }}>→</span>}
                </>
              );
              return (
                <div
                  key={n.name}
                  className={`tr${n.name === active ? " tr-on" : ""}`}
                  style={n.note ? { opacity: 0.45, cursor: "default" } : { cursor: "pointer" }}
                  title={n.note}
                  onClick={() => {
                    if (n.to) navigate(n.to);
                    else if (!n.note) setActive(n.name);
                  }}
                >
                  {row}
                </div>
              );
            })}
          </div>
          <div className="wal">
            <Label>BUILD</Label>
            <span className="mono" style={{ fontSize: 10.5, color: "#8C8880" }}>
              v2.6.0 · self-hosted
            </span>
          </div>
        </div>

        <Main style={{ padding: "26px 32px", overflow: "hidden", gap: 22 }}>
          <div style={{ display: "flex", gap: 40, flexWrap: "wrap" }}>
            <Readout k="OUTBOX DEPTH" v={num(stats?.outbox_depth ?? 0)} size={24} />
            <Readout k="OP-LOG LAG" v={stats ? since(stats.lag_seconds) : "—"} size={24} />
            <Readout k="SIGNED IN" v={num(people?.active_sessions ?? 0)} size={24} />
            <Readout k="OPEN PAGES" v={num(stats?.open_sessions ?? 0)} size={24} />
            <Readout k="PAGES" v={num(stats?.pages ?? 0)} size={24} />
            <Readout k="DB SIZE" v={stats ? bytes(stats.database_bytes) : "—"} size={24} />
          </div>

          {/* Ops per hour, last fourteen. Quiet hours are drawn as
              the zeroes they are — a sparkline that omits them
              draws a busy day where there was a gap. */}
          <div>
            <div style={{ display: "flex", alignItems: "flex-end", gap: 3, height: 56 }}>
              {(perHour.length ? perHour : Array<number>(14).fill(0)).map((n, i) => (
                <div
                  key={i}
                  title={`${n} ops`}
                  style={{
                    flex: 1,
                    height: `${Math.max(2, (n / maxHour) * 100)}%`,
                    background: n === maxHour && n > 0
                      ? "#E8873C"
                      : `rgba(232,135,60,${(0.28 + 0.12 * (n / maxHour)).toFixed(2)})`,
                    transition: "height .25s cubic-bezier(.2,.7,.3,1)",
                  }}
                />
              ))}
            </div>
            <div className="mono" style={{ fontSize: 9.5, color: "#585550", marginTop: 6 }}>
              accepted ops per hour · last 14 h
              {quiet && (
                <span style={{ color: "#6E6A63" }}>
                  {" "}— nothing in that window; the last op was{" "}
                  {stats ? since(stats.lag_seconds) : "—"} ago
                </span>
              )}
            </div>
          </div>

          <div style={{ display: "flex", gap: 30, flex: 1, minHeight: 0 }}>
            <div style={{ flex: 1, minWidth: 0, overflowY: "auto" }}>
              <Label style={{ marginBottom: 12, display: "block" }}>
                SERVICES{health ? ` · ${health.up} of ${health.total}` : ""}
              </Label>
              <div style={{ display: "flex", flexDirection: "column" }}>
                {(health?.services ?? []).map((s, i, all) => (
                  <div
                    key={s.name}
                    className="row"
                    style={{ padding: "9px 0", ...(i === all.length - 1 ? { borderBottom: 0 } : {}) }}
                    title={s.url}
                  >
                    <div style={{
                      width: 6, height: 6,
                      background: STATUS_HUE[s.status] ?? "#E0A34E",
                    }} />
                    <span style={{ flex: 1, fontSize: 12.5, color: "#D2CFC8" }}>{s.name}</span>
                    <span className="mono" style={{ fontSize: 10.5, color: "#8C8880" }}>
                      {s.detail ? s.detail : `${s.latency_ms} ms`}
                    </span>
                    <span className="mono" style={{
                      fontSize: 10.5, color: STATUS_HUE[s.status] ?? "#E0A34E",
                    }}>
                      {s.status}
                    </span>
                  </div>
                ))}
                {!health && !errs.length && (
                  <div className="mono" style={{ fontSize: 11, color: "#585550" }}>probing…</div>
                )}
              </div>
              <div style={{ fontSize: 11, lineHeight: 1.6, color: "#585550", marginTop: 12 }}>
                A probe, not a report. <span className="mono">up</span> means this gateway
                asked and got an answer inside two seconds — it says nothing about whether
                that service can reach its own database. There is no registry to ask for
                more, and claiming more would be borrowing a word.
              </div>
            </div>

            <div style={{ width: 330, flex: "none", overflowY: "auto" }}>
              <Label style={{ marginBottom: 12, display: "block" }}>
                PEOPLE{people ? ` · ${people.people.length}` : ""}
              </Label>
              <div style={{ display: "flex", flexDirection: "column" }}>
                {(people?.people ?? []).map((p, i, all) => (
                  <div
                    key={p.id}
                    className="row"
                    style={{ padding: "9px 0", ...(i === all.length - 1 ? { borderBottom: 0 } : {}) }}
                    title={`${p.email} · ${p.cursor_color}`}
                  >
                    <div
                      className={`av ${p.id === actorId ? "av-you" : "av-them"}`}
                      style={{ width: 20, height: 20, fontSize: 8 }}
                    >
                      {p.display_name.slice(0, 2).toUpperCase()}
                    </div>
                    <span style={{ flex: 1, fontSize: 12.5, color: "#D2CFC8" }}>
                      {p.display_name}
                    </span>
                    <span className="mono" style={{ fontSize: 10.5, color: "#8C8880" }}>
                      {p.id === actorId ? "you" : "member"}
                    </span>
                  </div>
                ))}
                {!people && !errs.length && (
                  <div className="mono" style={{ fontSize: 11, color: "#585550" }}>loading…</div>
                )}
              </div>
              <div style={{ fontSize: 11, lineHeight: 1.6, color: "#585550", marginTop: 10 }}>
                Everyone is a member. Roles arrive with RBAC (<span className="mono">v3.1.0</span>),
                and until then this list is readable by any signed-in actor — an open admin
                surface, said out loud rather than implied shut.
              </div>

              <Label style={{ margin: "20px 0 10px", display: "block" }}>BACKUPS</Label>
              <div className="mono" style={{ fontSize: 10.5, lineHeight: 1.9, color: "#585550" }}>
                none — no backup system exists in this repo.<br />
                what would restore: <span style={{ color: "#8C8880" }}>collab.ops</span>, which
                is the source of truth;<br />
                everything else is a projection replay rebuilds.
              </div>
            </div>
          </div>

          {errs.length > 0 && (
            <div className="mono" style={{ fontSize: 11, color: "#E0A34E" }}>
              {errs.join(" · ")}
            </div>
          )}
        </Main>
      </Body>

      <StatusBar
        route="/admin"
        mechanism="outbox depth and op-log lag are the two that matter"
        state={degraded
          ? `${(health?.total ?? 0) - (health?.up ?? 0)} of ${health?.total} services not answering`
          : stale
            ? `outbox oldest ${since(stats?.outbox_oldest_seconds ?? 0)} — the poller may be stopped`
            : "every service answered, outbox drained"}
        healthy={!degraded && !stale}
      />
    </Screen>
  );
}
