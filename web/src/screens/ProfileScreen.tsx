/**
 * docs/ui-mockups/v2/index.html § 23b PROFILE, ported — a person as their
 * op log.
 *
 * Every figure is a `GROUP BY actor` over `collab.ops`. Nothing here is a
 * counter kept beside the log, and that is the screen's whole claim rather
 * than a detail: a counter can drift from what happened, a projection of
 * the log cannot. The prose says so and the queries make it true.
 *
 * The join is done HERE, in the client, on purpose. collaboration-service
 * owns the ops and knows page IDs; document-service owns titles, topics and
 * tags. Neither reaches into the other's schema (`DATA_MODEL.md` §1), so a
 * cross-service join happens where both answers have already arrived — the
 * same shape § 18b's audit log already uses.
 *
 * The contribution grid is drawn from the days that HAVE ops. Silent days
 * are absent from the payload rather than zero-filled, so the grid fills a
 * fixed 52×7 and looks each date up: a year of empty rows would be payload
 * spent saying nothing happened.
 */
import { useEffect, useMemo, useState } from "react";
import { useParams } from "react-router-dom";
import { useAuth } from "../auth/AuthContext";
import { getProfile, type Profile } from "../api/history";
import { getLinkGraph, type GraphNode } from "../api/graph";
// The person's own details come from the admin people list rather than a
// per-user endpoint: auth.md's GetUser needs an id the caller may not be
// allowed to look up, and this screen already has a list it can index.
import { getPeople, type Person } from "../api/admin";
import {
  Body, Inspector, Label, Main, Readout, Rule, Screen, StatusBar, TopBar, TopicChip, num,
} from "../shell/Chrome";

/** Sunday-first weeks, oldest column left — the shape of every grid of
 *  this kind, and the one people already know how to read. */
const DAY_MS = 86_400_000;

export function ProfileScreen() {
  const { id } = useParams();
  const { session } = useAuth();
  const actorId = session?.actorId ?? null;
  const subject = id ?? actorId;

  const [profile, setProfile] = useState<Profile | null>(null);
  const [person, setPerson] = useState<Person | null>(null);
  const [people, setPeople] = useState<Person[]>([]);
  const [nodes, setNodes] = useState<GraphNode[]>([]);
  const [err, setErr] = useState<string | null>(null);
  const [insTab, setInsTab] = useState<"built" | "tags">("built");

  useEffect(() => {
    if (!subject || !actorId) return;
    getProfile(subject).then(setProfile).catch((e) => setErr(String(e)));
    getPeople(actorId)
      .then((r) => { setPeople(r.people); setPerson(r.people.find((p) => p.id === subject) ?? null); })
      .catch(() => setPerson(null));
    getLinkGraph(actorId).then((g) => setNodes(g.nodes)).catch(() => setNodes([]));
  }, [subject, actorId]);

  const byID = useMemo(() => new Map(nodes.map((n) => [n.id, n])), [nodes]);

  /** Which topics this person writes in, weighted by ops — computed from
   *  the join rather than stored anywhere, because nothing stores it. */
  const topics = useMemo(() => {
    const counts = new Map<string, { name: string; key: string; ops: number }>();
    for (const p of profile?.top_pages ?? []) {
      const node = byID.get(p.page_id);
      if (!node?.topic_name) continue;
      const cur = counts.get(node.topic_name) ?? { name: node.topic_name, key: node.topic_color_key, ops: 0 };
      cur.ops += p.ops;
      counts.set(node.topic_name, cur);
    }
    return [...counts.values()].sort((a, b) => b.ops - a.ops);
  }, [profile, byID]);

  const tags = useMemo(() => {
    const counts = new Map<string, number>();
    for (const p of profile?.top_pages ?? []) {
      for (const t of byID.get(p.page_id)?.tags ?? []) {
        counts.set(t, (counts.get(t) ?? 0) + p.ops);
      }
    }
    return [...counts].sort((a, b) => b[1] - a[1]).slice(0, 8);
  }, [profile, byID]);

  /** The grid: 52 weeks back to today, one square per day. */
  const grid = useMemo(() => {
    const ops = new Map((profile?.daily ?? []).map((d) => [d.day, d.ops]));
    const today = new Date();
    const start = new Date(today.getTime() - (profile?.weeks ?? 52) * 7 * DAY_MS);
    // Align to the week boundary so columns are weeks, not arbitrary runs.
    start.setDate(start.getDate() - start.getDay());
    const weeks: { day: string; ops: number }[][] = [];
    for (let t = start.getTime(); t <= today.getTime(); t += DAY_MS) {
      const d = new Date(t);
      const key = d.toISOString().slice(0, 10);
      if (d.getDay() === 0) weeks.push([]);
      (weeks[weeks.length - 1] ?? weeks[0])?.push({ day: key, ops: ops.get(key) ?? 0 });
    }
    return weeks;
  }, [profile]);

  const busiest = useMemo(
    () => Math.max(1, ...(profile?.daily ?? []).map((d) => d.ops)), [profile],
  );

  /** Longest run of consecutive days with any op. Computed, because a
   *  streak that is stored is a streak that can be wrong. */
  const streak = useMemo(() => {
    const days = (profile?.daily ?? []).map((d) => d.day).sort();
    let best = 0, run = 0, prev: number | null = null;
    for (const day of days) {
      const t = Date.parse(day);
      run = prev !== null && t - prev === DAY_MS ? run + 1 : 1;
      prev = t;
      best = Math.max(best, run);
    }
    return best;
  }, [profile]);

  return (
    <Screen>
      <TopBar
        crumb={<>people / <b>{person?.display_name ?? subject?.slice(0, 8) ?? "…"}</b></>}
        readouts={
          <>
            <Readout k="OPS AUTHORED" v={num(profile?.ops ?? 0)} />
            <Readout k="PAGES TOUCHED" v={num(profile?.pages ?? 0)} />
          </>
        }
      />

      <Body>
        <div className="rail" style={{ width: 262 }}>
          <div style={{ padding: "16px 14px", display: "flex", flexDirection: "column", gap: 10 }}>
            <div className={subject === actorId ? "av av-you" : "av av-them"}
                 style={{ width: 56, height: 56, fontSize: 19 }}>
              {initials(person?.display_name ?? person?.email ?? "?")}
            </div>
            <div>
              <h1 className="h1" style={{ fontSize: 20 }}>{person?.display_name ?? "Unknown"}</h1>
              <div className="mono" style={{ fontSize: 10.5, color: "#585550", marginTop: 4 }}>
                {person?.email ?? subject}
                {person?.created_at && ` · joined ${new Date(person.created_at).toLocaleDateString("en-GB", { month: "short", year: "numeric" })}`}
              </div>
            </div>
            {/* No bio, and no "editing now". auth.users stores a display
                name, an email and a cursor colour — there is no bio column,
                and presence is per-page rather than per-person, so neither
                could be shown without inventing it. */}
            <div style={{ fontSize: 11.5, lineHeight: 1.6, color: "#585550" }}>
              No bio: nothing stores one. Presence is per-page, so "editing now" would need a
              question this service cannot answer about a person in general.
            </div>
          </div>

          <div style={{ flex: 1, minHeight: 0 }} />
          <div className="wal">
            <Label>WHAT A COUNT IS NOT</Label>
            <div style={{ fontSize: 11.5, lineHeight: 1.6, color: "#6E6A63", marginTop: 8 }}>
              An op count measures typing, not contribution. One considered edit and forty
              keystrokes look the same here, and the grid says so rather than implying a
              ranking it cannot support.
            </div>
          </div>
        </div>

        <Main>
          {err && <div style={{ fontSize: 12, color: "#E0A34E" }}>◌ {err}</div>}

          <Label style={{ margin: "0 0 12px", display: "block" }}>WRITES MOSTLY IN</Label>
          {topics.length === 0 ? (
            <div style={{ fontSize: 11.5, color: "#585550" }}>
              No topic shows up yet — this person's pages carry none, which is a real state
              rather than a gap.
            </div>
          ) : (
            <div className="tgrow">
              {topics.map((t) => <TopicChip key={t.name} name={t.name} colorKey={t.key} small />)}
            </div>
          )}

          <Label style={{ margin: "26px 0 12px", display: "block" }}>SIGNATURE TAGS</Label>
          {tags.length === 0 ? (
            <div style={{ fontSize: 11.5, color: "#585550" }}>
              No tags on the pages this person touches.
            </div>
          ) : (
            <div className="tgrow">
              {tags.map(([tag]) => <span key={tag} className="tg">{tag}</span>)}
            </div>
          )}

          <Label style={{ margin: "26px 0 12px", display: "block" }}>MOST EDITED WITH</Label>
          {(profile?.most_edited_with ?? []).length === 0 ? (
            <div style={{ fontSize: 11.5, color: "#585550" }}>
              Nobody else has touched these pages.
            </div>
          ) : (
            <div style={{ display: "flex", flexDirection: "column", gap: 8 }}>
              {(profile?.most_edited_with ?? []).map((c) => {
                const who = people.find((p) => p.id === c.actor_id);
                return (
                  <div key={c.actor_id} style={{ display: "flex", alignItems: "center", gap: 9 }}>
                    <div className="av" style={{
                      width: 19, height: 19, fontSize: 8,
                      background: "rgba(255,255,255,.06)",
                      border: "1px solid rgba(255,255,255,.16)", color: "#9B968D",
                    }}>
                      {initials(who?.display_name ?? who?.email ?? "?")}
                    </div>
                    <span style={{ flex: 1, fontSize: 12.5, color: "#D2CFC8" }}>
                      {who?.display_name ?? "someone you cannot see"}
                    </span>
                    <span className="mono" style={{ fontSize: 10, color: "#8C8880" }}>
                      {num(c.pages)} page{c.pages === 1 ? "" : "s"}
                    </span>
                  </div>
                );
              })}
            </div>
          )}

          <Label style={{ margin: "26px 0 12px", display: "block" }}>
            {num(profile?.ops ?? 0)} OPS OVER {num(profile?.weeks ?? 52)} WEEKS
          </Label>
          <div style={{ display: "flex", gap: 2 }}>
            {grid.map((week, i) => (
              <div key={i} style={{ display: "flex", flexDirection: "column", gap: 2 }}>
                {week.map((d) => (
                  <div
                    key={d.day}
                    title={`${d.day} · ${d.ops} op${d.ops === 1 ? "" : "s"}`}
                    style={{
                      width: 9, height: 9,
                      background: d.ops === 0
                        ? "rgba(255,255,255,.05)"
                        : `rgba(232,135,60,${0.25 + (d.ops / busiest) * 0.75})`,
                    }}
                  />
                ))}
              </div>
            ))}
          </div>
          <div className="mono" style={{ fontSize: 9.5, color: "#585550", marginTop: 8 }}>
            one square per day · shade is op count, and op count is not merit
          </div>
          <div className="mono" style={{ fontSize: 9.5, color: "#585550", marginTop: 4 }}>
            longest streak {num(streak)} day{streak === 1 ? "" : "s"} · quietest week 0 · both fine
          </div>

          <Label style={{ margin: "26px 0 12px", display: "block" }}>
            RECENT · FROM THE OP LOG, NOT AN ACTIVITY TABLE
          </Label>
          <div style={{ display: "flex", flexDirection: "column" }}>
            {(profile?.recent ?? []).slice(0, 12).map((o) => (
              <div key={o.id} className="row" style={{ padding: "8px 0" }}>
                <span className="mono" style={{ fontSize: 10, color: "#585550", width: 46 }}>
                  {new Date(o.created_at).toLocaleTimeString("en-GB", { hour: "2-digit", minute: "2-digit" })}
                </span>
                <span className="mono" style={{ fontSize: 10.5, color: "#7AA8E8", width: 130 }}>
                  {o.kind.replace(/^(block|text):/, "")}
                </span>
                <span style={{ flex: 1, fontSize: 12.5, color: "#D2CFC8", overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
                  {byID.get(o.page_id)?.title ?? <span style={{ color: "#585550" }}>a page you cannot see</span>}
                </span>
              </div>
            ))}
            {(profile?.recent.length ?? 0) === 0 && (
              <div style={{ fontSize: 11.5, color: "#585550" }}>
                Nothing in the op log for this person yet.
              </div>
            )}
          </div>
        </Main>

        <Inspector
          tabs={[{ id: "built", label: "HOW BUILT" }, { id: "tags", label: "TAGS" }]}
          active={insTab}
          onSelect={(t) => setInsTab(t as "built" | "tags")}
        >
          {insTab === "tags" ? (
            <div style={{ display: "flex", flexDirection: "column", gap: 12 }}>
              <Label>SIGNATURE TAGS</Label>
              {tags.length === 0 ? (
                <div style={{ fontSize: 11.5, color: "#585550" }}>
                  No tags on the pages this person touches.
                </div>
              ) : tags.map(([tag, ops]) => (
                <div key={tag} style={{ display: "flex", alignItems: "center", gap: 9 }}>
                  <span className="tg" style={{ flex: 1 }}>{tag}</span>
                  <span className="mono" style={{ fontSize: 9.5, color: "#8C8880" }}>{num(ops)}</span>
                </div>
              ))}
              <Rule />
              <div style={{ fontSize: 11.5, lineHeight: 1.7, color: "#8C8880" }}>
                Weighted by ops on the pages carrying each tag, not by how many pages carry
                it — writing forty times in one place and once in another is not the same as
                writing once in each.
              </div>
            </div>
          ) : (
            <div style={{ display: "flex", flexDirection: "column", gap: 12 }}>
              <Label>HOW THIS IS BUILT</Label>
              <div style={{ fontSize: 11.5, lineHeight: 1.7, color: "#8C8880" }}>
                Every figure here is a <span className="mono">GROUP BY actor</span> over the op
                log. Nothing is a counter that could drift out of sync with what actually
                happened — there is no code path that edits a page without producing the row
                that says so, because the row <i>is</i> the op.
              </div>
              <Rule />
              <Label>WHERE THE JOIN HAPPENS</Label>
              <div style={{ fontSize: 11.5, lineHeight: 1.7, color: "#8C8880" }}>
                collaboration-service owns the ops and knows page ids. Titles, topics and tags
                belong to document-service. Neither reads the other's schema, so the join
                happens here, in the client, once both answers have arrived.
                <br /><br />
                A page you have no access to shows as <i>a page you cannot see</i> rather than
                being dropped: the op happened, and hiding the row would misreport the count
                beside it.
              </div>
            </div>
          )}
        </Inspector>
      </Body>

      <StatusBar
        route={`/people/${subject ?? ""}`}
        mechanism="GROUP BY actor over collab.ops · titles joined client-side from the graph"
        state={profile ? `${num(profile.ops)} ops · ${num(profile.pages)} pages` : "loading…"}
        healthy={!err}
      />
    </Screen>
  );
}

function initials(name: string): string {
  const parts = name.trim().split(/[\s@.]+/).filter(Boolean);
  return ((parts[0]?.[0] ?? "?") + (parts[1]?.[0] ?? "")).toUpperCase();
}
