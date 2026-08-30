/**
 * docs/ui-mockups/v2/index.html § 18b AUDIT LOG, ported.
 *
 * The section's subtitle is the whole design: "derived from the
 * op log rather than written beside it". Content rows are read
 * out of `collab.ops`; auth rows are read out of `auth.users`
 * and `auth.refresh_tokens`. Nothing here is an event somebody
 * remembered to emit, which is exactly why it cannot disagree
 * with what happened — there is no code path that edits a page
 * without producing the row that says so, because the row IS
 * the op.
 *
 * The two halves come from two services and are merged by
 * timestamp HERE, in the client. That is deliberate:
 * `DATA_MODEL.md` forbids cross-schema joins, and the honest
 * place for a join across a service boundary is the caller that
 * wanted both. Page titles come from a third call for the same
 * reason — `collab.ops` holds page ids, and titles belong to
 * document-service.
 *
 * Three things the mockup claimed that this does not, corrected
 * in the mockup first: there is no prev-hash chain (tamper
 * evidence would need a column on an append-only table and a
 * write-path change, and the panel now says what it would take
 * rather than showing a green tick for it); there is no
 * per-request source, so SOURCE reports the actor kind the op
 * log actually records; and failed sign-ins are absent because
 * nothing records them.
 */
import { useEffect, useMemo, useState } from "react";
import { useNavigate } from "react-router-dom";
import {
  getAudit, getAuthEvents, getPeople,
  type AuditReport, type AuthEvent, type People,
} from "../api/admin";
import { getLinkGraph } from "../api/graph";
import { listTrash } from "../api/trash";
import { useAuth } from "../auth/AuthContext";
import {
  Body, Inspector, Label, Main, Readout, Screen, StatusBar, SubBar,
  SubItem, TopBar, num,
} from "../shell/Chrome";

/** The rail, identical to § 18's — same admin, same map. */
const NAV: Array<{ name: string; to?: string; note?: string }> = [
  { name: "Health", to: "/admin" },
  { name: "Services", note: "a panel on Health" },
  { name: "Queues & outbox", note: "a panel on Health" },
  { name: "People", note: "a panel on Health" },
  { name: "Sessions", note: "a panel on Health" },
  { name: "Storage & quota", note: "needs object storage — not in this repo" },
  { name: "Index & embeddings", note: "v4.4.0" },
  { name: "Backups", note: "no backup system exists yet" },
  { name: "Jobs & sagas", to: "/trash" },
  { name: "Feature flags", note: "v3.5.0" },
  { name: "Audit log" },
  { name: "API keys", note: "v3.1.0 — needs RBAC first, § 18c" },
  { name: "Licence & version", note: "v3.5.0" },
];

type Filter = "all" | "content" | "destructive" | "auth";

const FILTERS: Array<{ id: Filter; label: string }> = [
  { id: "all", label: "ALL" },
  { id: "auth", label: "AUTH" },
  { id: "content", label: "CONTENT" },
  { id: "destructive", label: "DESTRUCTIVE" },
];

/** One merged row, from either source. */
interface Entry {
  id: string;
  at: string;
  actorId: string;
  actorKind: string;
  event: string;
  detail: string;
  targetId: string;
  target: string;
  cls: string;
  source: string;
  /** How many consecutive identical events this row stands for.
   *  1 for an ordinary row. */
  repeat: number;
  /** The earliest of them, when repeat > 1 — so the row still
   *  says what span it covers. */
  since?: string;
}

const CLASS_HUE: Record<string, string> = {
  content: "#7AA8E8",
  destructive: "#E0A34E",
  auth: "#5AC8B4",
};

/** `block:InsertBlock` → `page.block.insert.block` reads badly,
 *  so: tier + the op's own words, lowercased and dotted. Derived
 *  from the kind rather than mapped by hand, so a new op kind
 *  gets a sensible name for free instead of falling off the
 *  screen. */
function eventName(kind: string): string {
  const hasTier = kind.includes(":");
  const tier = hasTier ? kind.split(":")[0] : "block";
  const op = hasTier ? kind.split(":")[1] : kind;
  const words = op.replace(/([a-z])([A-Z])/g, "$1.$2").toLowerCase();
  return `page.${tier}.${words}`;
}

const timeOf = (iso: string) => iso.slice(11, 19);
const dayOf = (iso: string) => iso.slice(0, 10);

export function AuditScreen() {
  const navigate = useNavigate();
  const { session } = useAuth();
  const actorId = session?.actorId ?? null;

  const [filter, setFilter] = useState<Filter>("all");
  const [audit, setAudit] = useState<AuditReport | null>(null);
  const [authEvents, setAuthEvents] = useState<AuthEvent[]>([]);
  const [people, setPeople] = useState<People | null>(null);
  /** Page id → title. Deleted pages are in here too, prefixed —
   *  an audit log that cannot name a deleted page fails at
   *  exactly the moment naming matters most. */
  const [titles, setTitles] = useState<Map<string, string>>(new Map());
  const [selected, setSelected] = useState<Entry | null>(null);
  const [insTab, setInsTab] = useState<"selected" | "retention">("selected");
  const [errs, setErrs] = useState<string[]>([]);

  const note = (what: string) => (e: unknown) =>
    setErrs((prev) => (prev.some((p) => p.startsWith(what)) ? prev : [...prev, `${what}: ${String(e)}`]));

  useEffect(() => {
    getAudit(filter === "auth" ? "all" : filter).then(setAudit).catch(note("ops"));
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [filter]);

  useEffect(() => {
    getAuthEvents(actorId).then((r) => setAuthEvents(r.events)).catch(note("auth"));
    getPeople(actorId).then(setPeople).catch(note("people"));
    // Titles come from the graph, which is every page rather than
    // one parent's children — the same lesson the graph colouring
    // learned when a ListPages join left half the corpus unnamed.
    if (actorId) {
      // The live graph and the trash, because the log outlives
      // the pages in it. The graph is every page rather than one
      // parent's children — the same lesson the graph colouring
      // learned when a ListPages join left half the corpus
      // unnamed.
      Promise.all([getLinkGraph(actorId), listTrash(actorId)])
        .then(([g, t]) => {
          const m = new Map(g.nodes.map((n) => [n.id, n.title]));
          for (const e of t.entries) m.set(e.page.id, `${e.page.title} (deleted)`);
          setTitles(m);
        })
        .catch(note("titles"));
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [actorId]);

  const nameOf = useMemo(() => {
    const m = new Map((people?.people ?? []).map((p) => [p.id, p.display_name]));
    return (id: string) => m.get(id) ?? "unknown";
  }, [people]);

  const entries = useMemo<Entry[]>(() => {
    const out: Entry[] = [];
    if (filter !== "auth") {
      for (const r of audit?.rows ?? []) {
        out.push({
          id: r.id, at: r.created_at, actorId: r.actor_id, actorKind: r.actor_kind,
          event: eventName(r.kind),
          detail: r.undo_group ? "part of one gesture" : "",
          targetId: r.page_id,
          target: titles.get(r.page_id) ?? r.page_id.slice(0, 8),
          cls: r.class,
          // The op log records an actor KIND, not a request
          // source. Reporting what it has beats inventing "web".
          source: r.actor_kind,
          repeat: 1,
        });
      }
    }
    if (filter === "all" || filter === "auth") {
      for (const e of authEvents) {
        out.push({
          id: e.id, at: e.at, actorId: e.user_id, actorKind: "user",
          event: e.kind,
          detail: e.kind === "auth.signin" ? "refresh token issued"
            : e.kind === "auth.signout" ? "token revoked"
            : "account created",
          targetId: "", target: nameOf(e.user_id),
          cls: "auth", source: "user", repeat: 1,
        });
      }
    }
    // Merged by time, the only ordering the two sources share —
    // neither service's sequence means anything to the other.
    out.sort((a, b) => (a.at < b.at ? 1 : a.at > b.at ? -1 : 0));

    // Consecutive identical events by the same actor on the same
    // target collapse into one row with a count.
    //
    // Not cosmetic: this instance had 300+ sign-ins from one
    // account, and an unfolded list showed nothing else at all.
    // A log where one repeated event drowns everything is a log
    // nobody reads — and the collapse loses nothing, since the
    // row still carries how many and how far back. It is the
    // same idea `undo_group` already applies to ops: one
    // gesture, one line.
    const folded: Entry[] = [];
    for (const e of out) {
      const prev = folded[folded.length - 1];
      if (prev && prev.event === e.event && prev.actorId === e.actorId
          && prev.targetId === e.targetId && prev.cls === e.cls) {
        prev.repeat++;
        prev.since = e.at;
        continue;
      }
      folded.push({ ...e });
    }
    return folded.slice(0, 60);
  }, [audit, authEvents, filter, titles, nameOf]);

  const counts = audit?.counts ?? {};
  const classRows = [
    { name: "Content ops", key: "content", n: counts.content ?? 0, hue: "#7AA8E8" },
    { name: "Auth", key: "auth", n: authEvents.length, hue: "#5AC8B4" },
    { name: "Destructive", key: "destructive", n: counts.destructive ?? 0, hue: "#E0A34E" },
  ];
  const maxClass = Math.max(1, ...classRows.map((c) => c.n));

  /** CSV of exactly what is on screen. No summarisation — an
   *  export that quietly aggregates is one nobody can check. */
  function exportCsv() {
    const head = ["time", "actor", "actor_kind", "event", "count", "detail", "target", "class", "source"];
    const lines = entries.map((e) => [
      e.at, nameOf(e.actorId), e.actorKind, e.event, e.repeat, e.detail, e.target, e.cls, e.source,
    ].map((c) => `"${String(c).replace(/"/g, '""')}"`).join(","));
    const blob = new Blob([[head.join(","), ...lines].join("\n")], { type: "text/csv" });
    const url = URL.createObjectURL(blob);
    const a = document.createElement("a");
    a.href = url;
    a.download = `marginal-audit-${filter}.csv`;
    a.click();
    URL.revokeObjectURL(url);
  }

  return (
    <Screen>
      <TopBar
        noTabs
        crumb={<>admin / <b>audit log</b></>}
        readouts={
          <>
            <Readout k="EVENTS" v={num((audit?.total ?? 0) + authEvents.length)} />
            <Readout k="RETENTION" v="forever" />
          </>
        }
      />

      <SubBar>
        {FILTERS.map((f) => (
          <SubItem key={f.id} on={filter === f.id} onClick={() => setFilter(f.id)}>
            {f.label}
          </SubItem>
        ))}
        <div style={{ flex: 1 }} />
        <SubItem tone="#585550">append-only · no delete, for anyone</SubItem>
      </SubBar>

      <Body>
        <div className="rail" style={{ width: 236 }}>
          <div className="rail-h">ADMIN<div /></div>
          <div style={{ display: "flex", flexDirection: "column", padding: "0 8px", gap: 1 }}>
            {NAV.map((n) => (
              <div
                key={n.name}
                className={`tr${n.name === "Audit log" ? " tr-on" : ""}`}
                style={n.note ? { opacity: 0.45, cursor: "default" } : { cursor: "pointer" }}
                title={n.note}
                onClick={() => { if (n.to) navigate(n.to); }}
              >
                {n.name === "Audit log" && <i />}
                {n.name}
                {n.to && <span className="tr-n" style={{ marginLeft: "auto" }}>→</span>}
              </div>
            ))}
          </div>
          <div className="wal">
            <Label>→ = A SCREEN EXISTS</Label>
            <div style={{ fontSize: 11, lineHeight: 1.55, color: "#8C8880" }}>
              Dimmed entries are real routes with no screen drawn yet. Marked rather than
              hidden — a nav that quietly omits what is unfinished is how a design doc
              starts lying.
            </div>
          </div>
        </div>

        <Main style={{ padding: "22px 30px", overflow: "hidden" }}>
          <div style={{ display: "flex", alignItems: "baseline", gap: 12, marginBottom: 16 }}>
            <h1 className="h1" style={{ fontSize: 20 }}>Audit log</h1>
            <span className="mono" style={{ fontSize: 11, color: "#585550" }}>
              a projection of collab.ops + auth state · never a second write path
            </span>
            <div style={{ flex: 1 }} />
            <span className="chip" style={{ cursor: "pointer" }} onClick={exportCsv}>
              EXPORT CSV
            </span>
          </div>

          <div style={{ overflowY: "auto", flex: 1, minHeight: 0 }}>
            <div style={{
              display: "grid",
              gridTemplateColumns: "74px 96px 1fr 132px 92px",
              alignItems: "center", fontSize: 11.5,
            }}>
              {["TIME", "ACTOR", "EVENT", "TARGET", "SOURCE"].map((h, i) => (
                <span
                  key={h}
                  className="mono"
                  style={{
                    fontSize: 8.5, letterSpacing: ".12em", color: "#585550",
                    paddingBottom: 9, ...(i === 4 ? { textAlign: "right" as const } : {}),
                  }}
                >
                  {h}
                </span>
              ))}

              {entries.map((e) => {
                const cell = {
                  borderTop: "1px solid rgba(255,255,255,.07)",
                  padding: "9px 0",
                  cursor: "pointer",
                  background: selected?.id === e.id ? "rgba(255,255,255,.03)" : undefined,
                };
                return (
                  <div key={e.id} style={{ display: "contents" }} onClick={() => setSelected(e)}>
                    <span className="mono" style={{ ...cell, fontSize: 10, color: "#585550" }}
                      title={dayOf(e.at)}>
                      {timeOf(e.at)}
                    </span>
                    <div style={{ ...cell, display: "flex", alignItems: "center", gap: 7 }}>
                      <div
                        className={`av ${e.actorId === actorId ? "av-you" : "av-them"}`}
                        style={{ width: 17, height: 17, fontSize: 7 }}
                      >
                        {nameOf(e.actorId).slice(0, 2).toUpperCase()}
                      </div>
                      <span style={{ color: "#9B968D", fontSize: 11 }}>{nameOf(e.actorId)}</span>
                    </div>
                    <div style={cell}>
                      <span className="mono" style={{
                        fontSize: 10.5, color: CLASS_HUE[e.cls] ?? "#8C8880",
                      }}>
                        {e.event}
                      </span>{" "}
                      {e.repeat > 1 && (
                        <span className="mono" style={{ fontSize: 10, color: "#E4E2DC" }}>
                          ×{e.repeat}{" "}
                        </span>
                      )}
                      <span style={{ color: "#8C8880" }}>
                        {e.repeat > 1 && e.since
                          ? `${e.detail || "repeated"}, back to ${timeOf(e.since)}`
                          : e.detail}
                      </span>
                    </div>
                    <span style={{ ...cell, color: "#8C8880", fontSize: 11 }}>{e.target}</span>
                    <span className="mono" style={{
                      ...cell, textAlign: "right", fontSize: 10, color: "#585550",
                    }}>
                      {e.source}
                    </span>
                  </div>
                );
              })}
            </div>
            {!entries.length && (
              <div className="mono" style={{ fontSize: 11, color: "#585550", paddingTop: 12 }}>
                {errs.length ? errs.join(" · ") : "nothing in this class"}
              </div>
            )}
          </div>

          <div className="mono" style={{ fontSize: 10, color: "#585550", marginTop: 12 }}>
            {num(Math.max(0, (audit?.total ?? 0) + authEvents.length - entries.length))} more ·
            every row IS the op or the auth row it was read from, not a copy of it
          </div>

          <div style={{
            marginTop: "auto", paddingTop: 20,
            borderTop: "1px solid rgba(255,255,255,.07)",
            display: "grid", gridTemplateColumns: "1fr 1fr 1.2fr", gap: 26,
          }}>
            <div>
              <Label style={{ marginBottom: 11, display: "block" }}>EVENTS BY CLASS</Label>
              <div style={{ display: "flex", flexDirection: "column", gap: 7 }}>
                {classRows.map((c) => (
                  <div key={c.key} style={{ display: "flex", alignItems: "center", gap: 9 }}>
                    <span style={{
                      flex: 1, fontSize: 11.5,
                      color: c.key === "destructive" ? "#E0A34E" : "#D2CFC8",
                    }}>
                      {c.name}
                    </span>
                    <div style={{ width: 64, height: 4, background: "rgba(255,255,255,.06)" }}>
                      <div style={{
                        width: `${(c.n / maxClass) * 100}%`, height: "100%", background: c.hue,
                        transition: "width .25s ease",
                      }} />
                    </div>
                    <span className="mono" style={{
                      fontSize: 9.5, color: "#8C8880", width: 44, textAlign: "right",
                    }}>
                      {num(c.n)}
                    </span>
                  </div>
                ))}
              </div>
            </div>
            <div>
              <Label style={{ marginBottom: 11, display: "block" }}>WHY IT CANNOT DRIFT</Label>
              <div style={{ fontSize: 11.5, lineHeight: 1.7, color: "#8C8880" }}>
                Content rows are read from{" "}
                <span className="mono" style={{ color: "#9B968D" }}>collab.ops</span>, not written
                alongside it. There is no code path that edits a page without producing the row
                that says so, because the row <i>is</i> the op.
              </div>
            </div>
            <div>
              <Label style={{ marginBottom: 11, display: "block" }}>WHAT IS NOT IN HERE</Label>
              <div style={{ fontSize: 11.5, lineHeight: 1.7, color: "#8C8880" }}>
                Reads. Deliberately: a log of who opened what is a surveillance feature, and
                once it exists someone will ask for a report from it. Analytics answers “how
                many” with sketches that cannot name a person, which is the version worth
                keeping.
              </div>
            </div>
          </div>
        </Main>

        <Inspector
          width={290}
          tabs={[{ id: "selected", label: "SELECTED" }, { id: "retention", label: "RETENTION" }]}
          active={insTab}
          onSelect={(id) => setInsTab(id as "selected" | "retention")}
        >
          {insTab === "selected" ? (
            selected ? (
              <>
                <div className="mono" style={{ fontSize: 11, color: "#E4E2DC" }}>
                  {selected.event}
                </div>
                <div className="mono" style={{ fontSize: 10.5, lineHeight: 1.95, color: "#8C8880" }}>
                  at&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;{selected.at.slice(0, 23).replace("T", " ")}<br />
                  actor&nbsp;&nbsp;{nameOf(selected.actorId)}<br />
                  kind&nbsp;&nbsp;&nbsp;{selected.actorKind}<br />
                  class&nbsp;&nbsp;{selected.cls}<br />
                  target&nbsp;{selected.target}
                </div>
                <div style={{ height: 1, background: "rgba(255,255,255,.07)" }} />
                <Label>DERIVED FROM</Label>
                <div style={{ fontSize: 11.5, lineHeight: 1.6, color: "#8C8880" }}>
                  {selected.cls === "auth"
                    ? "A row in auth.users or auth.refresh_tokens. Not an event someone chose to emit — the row is the state, and the log reads it."
                    : <>The op itself, <span className="mono" style={{ color: "#9B968D" }}>
                        {selected.id.slice(0, 8)}
                      </span> in collab.ops. Reading it is the audit; there is nothing else to
                      keep in sync.</>}
                </div>
                {selected.targetId && (
                  <>
                    <div style={{ height: 1, background: "rgba(255,255,255,.07)" }} />
                    <span
                      className="chip"
                      style={{ cursor: "pointer" }}
                      onClick={() => navigate(`/pages/${selected.targetId}/history`)}
                    >
                      OPEN ITS HISTORY
                    </span>
                  </>
                )}
              </>
            ) : (
              <div style={{ fontSize: 11.5, lineHeight: 1.6, color: "#585550" }}>
                Pick a row. Each one names the op or auth row it was read from, and content
                rows link to that page's own history.
              </div>
            )
          ) : (
            <>
              <Label>RETENTION</Label>
              <div style={{ fontSize: 11.5, lineHeight: 1.65, color: "#8C8880" }}>
                Forever, and not as a policy choice: <span className="mono">collab.ops</span> is
                append-only because it is the source of truth. Deleting from it would delete
                the document, not a record of it.
              </div>
              <div style={{ height: 1, background: "rgba(255,255,255,.07)" }} />
              <Label>TAMPER EVIDENCE</Label>
              {/* The mockup claimed a verified prev-hash chain.
                  There isn't one, and printing "verified ✓" about
                  a chain that does not exist is the single worst
                  thing an audit screen can do. */}
              <div style={{ fontSize: 11.5, lineHeight: 1.65, color: "#8C8880" }}>
                <span style={{ color: "#E0A34E" }}>None.</span> Rows are ordered by{" "}
                <span className="mono">seq</span> and the table takes no UPDATE or DELETE from
                this codebase — but somebody holding the database could still edit it
                undetectably.
              </div>
              <div style={{ fontSize: 11, lineHeight: 1.6, color: "#585550" }}>
                What it would take: a <span className="mono">prev_hash</span> column carrying
                the hash of the row before it, written on the accept path. Then deleting a row
                anywhere breaks the chain from that point on. That is a migration and a change
                to the write path, not a screen — so it is named here rather than drawn as a
                green tick.
              </div>
              <div style={{ height: 1, background: "rgba(255,255,255,.07)" }} />
              <Label>EXPORT</Label>
              <div style={{ fontSize: 11.5, lineHeight: 1.65, color: "#8C8880" }}>
                CSV of exactly the rows on screen. Same data, no summarisation — a compliance
                export that quietly aggregates is one nobody can check.
              </div>
              <div style={{ height: 1, background: "rgba(255,255,255,.07)" }} />
              <Label>NOT RECORDED</Label>
              <div style={{ fontSize: 11.5, lineHeight: 1.65, color: "#8C8880" }}>
                Failed sign-ins, and the request source. Nothing writes either, so the columns
                that would show them show what the log does have — the actor kind — rather than
                an invented “web”.
              </div>
            </>
          )}
        </Inspector>
      </Body>

      <StatusBar
        route="/admin/audit"
        mechanism="append-only · derived, never written beside"
        state={`${num(entries.length)} rows · ${num(counts.destructive ?? 0)} destructive in the whole log`}
      />
    </Screen>
  );
}
