/**
 * docs/ui-mockups/v2/index.html § 23 SPACES & ROLES, ported.
 *
 * Who is in which space, and what a role actually permits — the screen for
 * `ADR-013`'s permission boundary.
 *
 * THREE ROLES, NOT THE MOCKUP'S FIVE. It showed OWNER / EDITOR / COMMENTER /
 * PROPOSER / READER, and two of those gate capabilities this repo does not
 * have: comments are `v3.2.0` and the assistant is `v4.4.0`. A permission
 * for something nobody can do is a control that cannot be checked, which is
 * exactly what the uidiff gate's second half exists to catch — so the mockup
 * was corrected rather than this screen built to it.
 *
 * Every figure is fetched. The capability table is the one thing here that
 * is written down rather than read from the server, and it says so — there
 * is no "what may this role do" endpoint, because the answer lives in two
 * enforcement points (`internal/spaces.Role.AtLeast` for management,
 * `roles.Role.CanWrite` for ops) and inventing a third place to state it
 * would be inventing a third thing to disagree.
 */
import { useCallback, useEffect, useMemo, useState } from "react";
import { useAuth } from "../auth/AuthContext";
import {
  CAPABILITIES, createSpace, grantRole, listMembers, listSpaces, revokeRole,
  type Member, type Space,
} from "../api/spaces";
import { ApiError } from "../api/http";
import {
  Body, Inspector, Label, Main, Readout, Rule, Screen, StatusBar, TopBar, num,
} from "../shell/Chrome";

const ROLES: Member["role"][] = ["viewer", "editor", "admin"];

export function SpacesScreen() {
  const { session } = useAuth();
  const actorId = session?.actorId ?? null;

  const [spaces, setSpaces] = useState<Space[]>([]);
  const [selected, setSelected] = useState<string | null>(null);
  const [members, setMembers] = useState<Member[]>([]);
  const [memberError, setMemberError] = useState<string | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [insTab, setInsTab] = useState<"roles" | "reach">("roles");
  const [busy, setBusy] = useState(false);

  const load = useCallback(() => {
    if (!actorId) return;
    listSpaces(actorId)
      .then((r) => {
        setSpaces(r.spaces);
        setSelected((cur) => cur ?? r.spaces[0]?.id ?? null);
        setErr(null);
      })
      .catch((e) => setErr(String(e)));
  }, [actorId]);

  useEffect(load, [load]);

  const current = spaces.find((s) => s.id === selected) ?? null;

  useEffect(() => {
    if (!actorId || !selected) { setMembers([]); return; }
    listMembers(actorId, selected)
      .then((r) => { setMembers(r.members); setMemberError(null); })
      .catch((e) => {
        setMembers([]);
        // 403 means "you are in this space but not an admin" — a real
        // state with a real explanation, not an error to swallow. 404 for
        // a space you are not in cannot happen here: this list only shows
        // spaces you ARE in.
        setMemberError(e instanceof ApiError && e.status === 403
          ? "Only an admin of this space can see its roster."
          : String(e));
      });
  }, [actorId, selected, busy]);

  const change = useCallback(async (userID: string, role: Member["role"] | "remove") => {
    if (!actorId || !selected) return;
    setBusy(true);
    try {
      if (role === "remove") await revokeRole(actorId, selected, userID);
      else await grantRole(actorId, selected, userID, role);
      setMemberError(null);
      load();
    } catch (e) {
      // The last-admin rule arrives as 409. Saying which rule refused is
      // the whole point — "failed" would leave someone retrying forever.
      setMemberError(e instanceof ApiError && e.status === 409
        ? "A space must keep at least one admin — promote someone else first."
        : String(e));
    } finally {
      setBusy(false);
    }
  }, [actorId, selected, load]);

  const addSpace = useCallback(async () => {
    if (!actorId) return;
    const name = window.prompt("Name the new space:");
    if (!name) return;
    try {
      await createSpace(actorId, name);
      load();
    } catch (e) {
      setErr(String(e));
    }
  }, [actorId, load]);

  const totalMembers = useMemo(
    () => spaces.reduce((n, s) => n + s.members, 0), [spaces],
  );

  return (
    <Screen>
      <TopBar
        crumb={<>workspace / <b>spaces</b></>}
        readouts={
          <>
            <Readout k="SPACES" v={num(spaces.length)} />
            <Readout k="MEMBERSHIPS" v={num(totalMembers)} />
          </>
        }
        right={
          <div className="btn" style={{ cursor: "pointer" }} onClick={addSpace}>
            NEW SPACE
            <div className="brk-tl" />
            <div className="brk-br" />
          </div>
        }
      />

      <Body>
        <div className="rail">
          <div className="rail-h">
            SPACES<div /><span style={{ color: "#585550" }}>{num(spaces.length)}</span>
          </div>
          {spaces.map((s) => (
            <div
              key={s.id}
              className={`tr${s.id === selected ? " tr-on" : ""}`}
              style={{ cursor: "pointer" }}
              onClick={() => setSelected(s.id)}
            >
              <span className="tr-bars"><span className="tr-tick" /><span className="tr-bar" /></span>
              <span className="tr-t">{s.name}</span>
              <span className="tr-n" style={{ marginLeft: "auto" }}>{num(s.members)}</span>
            </div>
          ))}
          {spaces.length === 0 && !err && (
            <div style={{ padding: "10px 14px", fontSize: 11.5, lineHeight: 1.6, color: "#585550" }}>
              You are in no space yet, so there is nothing here and no page is
              visible to you either. That is the filter working, not a failure.
            </div>
          )}

          <div style={{ flex: 1, minHeight: 0 }} />
          <div className="wal">
          <Label>VISIBILITY</Label>
          <div style={{ fontSize: 11.5, lineHeight: 1.6, color: "#6E6A63", marginTop: 8 }}>
            A page is visible <b style={{ color: "#8C8880", fontWeight: 500 }}>by membership</b>,
            never by a flag on the page. A page belongs to exactly one space and takes that
            space's permissions — which is what makes the check one lookup instead of a walk
            up the tree.
          </div>
          </div>
        </div>

        <Main>
          {err && <div style={{ fontSize: 12, color: "#E0A34E" }}>◌ {err}</div>}

          {current && (
            <>
              <h1 className="h1" style={{ fontSize: 22 }}>{current.name}</h1>
              <div className="mono" style={{ fontSize: 11, color: "#585550", marginTop: 4 }}>
                {num(current.members)} member{current.members === 1 ? "" : "s"} ·
                you are <b style={{ color: "#E8873C", fontWeight: 600 }}>{current.your_role}</b>
                {current.is_default && " · the default space"}
              </div>

              <Label style={{ margin: "26px 0 12px", display: "block" }}>MEMBERS</Label>
              {memberError && (
                <div style={{ fontSize: 11.5, lineHeight: 1.6, color: "#E0A34E", marginBottom: 12 }}>
                  ◌ {memberError}
                </div>
              )}
              {members.map((m) => (
                <div key={m.user_id} className="row" style={{ padding: "11px 0" }}>
                  <div className={m.user_id === actorId ? "av av-you" : "av av-them"}
                       style={{ width: 22, height: 22, fontSize: 8.5 }}>
                    {initials(m.display_name || m.email)}
                  </div>
                  <span style={{ flex: 1, fontSize: 13, color: "#D2CFC8" }}>
                    {m.display_name || m.email}
                    {m.user_id === actorId && (
                      <span className="mono" style={{ fontSize: 9.5, color: "#585550", marginLeft: 7 }}>you</span>
                    )}
                  </span>
                  {/* Every role is a real control, and so is removal — a
                      segmented group with decorative options is the exact
                      defect the gate's second half looks for. */}
                  <div style={{ display: "flex", gap: 5 }}>
                    {ROLES.map((r) => (
                      <span
                        key={r}
                        className={`chip${m.role === r ? " chip-e" : ""}`}
                        style={{ cursor: busy ? "wait" : "pointer" }}
                        onClick={() => m.role !== r && change(m.user_id, r)}
                      >
                        {r.toUpperCase()}
                      </span>
                    ))}
                    <span
                      className="chip chip-a"
                      style={{ cursor: busy ? "wait" : "pointer" }}
                      onClick={() => change(m.user_id, "remove")}
                    >
                      REMOVE
                    </span>
                  </div>
                </div>
              ))}

              <Label style={{ margin: "26px 0 12px", display: "block" }}>WHAT EACH ROLE MAY DO</Label>
              <div style={{ display: "flex", flexDirection: "column", gap: 1 }}>
                <div style={{ display: "flex", gap: 2 }}>
                  <div style={{ width: 190 }} />
                  {["ADMIN", "EDITOR", "VIEWER"].map((r) => (
                    <div key={r} className="mono" style={{
                      flex: 1, textAlign: "center", fontSize: 8.5,
                      fontWeight: 600, letterSpacing: ".14em", color: "#585550",
                    }}>{r}</div>
                  ))}
                </div>
                {CAPABILITIES.map((c) => (
                  <div key={c.label} style={{ display: "flex", gap: 2, alignItems: "center" }}>
                    <div style={{ width: 190, fontSize: 12, color: "#9B968D" }}>{c.label}</div>
                    {[c.admin, c.editor, c.viewer].map((ok, i) => (
                      <div key={i} style={{
                        flex: 1, textAlign: "center", fontSize: 11,
                        color: ok ? "#3FCFA8" : "#4B4842",
                      }}>{ok ? "✓" : "—"}</div>
                    ))}
                  </div>
                ))}
              </div>
              <div style={{ marginTop: 12, fontSize: 11.5, lineHeight: 1.6, color: "#6E6A63" }}>
                Three roles, not five. The mockup showed COMMENTER and PROPOSER, which gate
                capabilities this repo does not have — comments are <b style={{ color: "#8C8880", fontWeight: 500 }}>v3.2.0</b> and
                the assistant <b style={{ color: "#8C8880", fontWeight: 500 }}>v4.4.0</b> — and a permission for
                something nobody can do is a control that cannot be checked.
              </div>
            </>
          )}
        </Main>

        <Inspector
          tabs={[{ id: "roles", label: "ROLES" }, { id: "reach", label: "REACH" }]}
          active={insTab}
          onSelect={(id) => setInsTab(id as "roles" | "reach")}
        >
          {insTab === "reach" ? (
            <div style={{ display: "flex", flexDirection: "column", gap: 12 }}>
              <Label>WHERE A ROLE IS ENFORCED</Label>
              <div style={{ fontSize: 11.5, lineHeight: 1.7, color: "#8C8880" }}>
                Reads and writes are checked in different places, on purpose.
                <br /><br />
                <b style={{ color: "#C3BFB7", fontWeight: 500 }}>Reads</b> are filtered by
                document-service against its own projection of these memberships — in SQL,
                before the LIMIT, because filtering afterwards returns fewer rows than asked
                for and the shortfall looks exactly like "there were no more".
                <br /><br />
                <b style={{ color: "#C3BFB7", fontWeight: 500 }}>Writes</b> go through
                collaboration-service's <span className="mono">can_apply</span> — RFC-002 §5's
                one chokepoint. The role is resolved once when you join a page, not per
                keystroke.
              </div>
              <Rule />
              <Label>WHAT THAT COSTS</Label>
              <div style={{ fontSize: 11.5, lineHeight: 1.7, color: "#8C8880" }}>
                A role changed while someone is connected takes effect when their connection
                closes, or within five minutes, whichever comes first. Saying "revocation is
                instant" would be a claim this architecture cannot honour.
              </div>
            </div>
          ) : (
            <div style={{ display: "flex", flexDirection: "column", gap: 12 }}>
              <Label>YOUR ROLE, PER SPACE</Label>
              {spaces.map((s) => (
                <div key={s.id} style={{ display: "flex", alignItems: "center", gap: 9 }}>
                  <span style={{ flex: 1, fontSize: 12, color: "#D2CFC8" }}>{s.name}</span>
                  <span className={`chip${s.your_role === "admin" ? " chip-e" : ""}`}>
                    {s.your_role.toUpperCase()}
                  </span>
                </div>
              ))}
              <Rule />
              <Label>THE ONE RULE WITH NO UNDO</Label>
              <div style={{ fontSize: 11.5, lineHeight: 1.7, color: "#8C8880" }}>
                A space must keep at least one admin. The last one cannot be removed
                <i> or demoted</i> — demoting yourself leaves exactly the same
                unadministrable space, and it is the easier of the two to do by accident.
              </div>
            </div>
          )}
        </Inspector>
      </Body>

      <StatusBar
        route="/spaces"
        mechanism="auth-service owns memberships · document-service filters reads · can_apply gates writes"
        state={current ? `${current.name} — you are ${current.your_role}` : "no space selected"}
        healthy={!err}
      />
    </Screen>
  );
}

function initials(name: string): string {
  const parts = name.trim().split(/[\s@.]+/).filter(Boolean);
  return ((parts[0]?.[0] ?? "?") + (parts[1]?.[0] ?? "")).toUpperCase();
}
