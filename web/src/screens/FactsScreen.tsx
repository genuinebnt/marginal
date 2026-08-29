/**
 * docs/ui-mockups/v2/index.html § 10 FACTS, ported.
 *
 * A second graph, and a different one from the link graph: its nodes are
 * DEFINITIONS, not pages. Everything shown is computed by
 * diagnostics-service's internal/facts — the dependency DAG, cycle rejection
 * by three-colour DFS (the same graphalgo.DetectCycle the Graph Explorer
 * uses, reused rather than reimplemented), duplicate detection by hash
 * collision, and forward reachability for "what goes stale when this
 * changes".
 *
 * The screen's argument, kept from the mockup: the interesting number is
 * nodes VISITED against nodes that exist. That ratio is the entire case for
 * incremental invalidation over full recompute — if editing one definition
 * walked all of them, there would be no reason to build a DAG at all.
 */
import { useEffect, useMemo, useState } from "react";
import { useNavigate } from "react-router-dom";
import { useAuth } from "../auth/AuthContext";
import {
  getFacts, getStaleReferences, type FactReference, type FactsGraph,
} from "../api/diagnostics";
import {
  Body, Inspector, Label, Readout, Rule, Screen, StatusBar, SubBar, SubItem, TopBar, num,
} from "../shell/Chrome";

export function FactsScreen() {
  const { session } = useAuth();
  const actorId = session?.actorId ?? null;
  const navigate = useNavigate();

  const [facts, setFacts] = useState<FactsGraph | null>(null);
  const [selected, setSelected] = useState<string | null>(null);
  const [stale, setStale] = useState<FactReference[] | null>(null);
  const [filter, setFilter] = useState("");
  const [err, setErr] = useState<string | null>(null);

  useEffect(() => {
    if (!actorId) return;
    getFacts(actorId).then(setFacts).catch((e) => setErr(String(e)));
  }, [actorId]);

  const defs = facts?.definitions ?? [];
  // The effective selection falls back to the first definition, so the
  // screen shows something on load. The stale query must key off THAT, not
  // off `selected` — keying off the raw state meant the default selection
  // never ran its query and the panel read 0 for a term with two referrers.
  const sel = selected ?? defs[0]?.name ?? null;

  useEffect(() => {
    if (!actorId || !sel) { setStale(null); return; }
    getStaleReferences(actorId, sel).then(setStale).catch(() => setStale(null));
  }, [actorId, sel]);
  const selDef = defs.find((d) => d.name === sel) ?? null;

  const shown = useMemo(
    () => defs.filter((d) => d.name.toLowerCase().includes(filter.toLowerCase())),
    [defs, filter],
  );

  /** Which definitions are referenced at all — an unused one is a real find. */
  const referenced = useMemo(
    () => new Set((facts?.references ?? []).map((r) => r.name)),
    [facts],
  );

  const dupeNames = useMemo(
    () => new Set((facts?.duplicates ?? []).map((d) => d.name)),
    [facts],
  );

  const visited = stale?.length ?? 0;
  const exist = defs.length;
  const neverWalked = exist > 0 ? Math.round(((exist - visited) / exist) * 100) : 0;

  return (
    <Screen>
      <TopBar
        crumb={<>lab / <b>facts</b></>}
        readouts={
          <>
            <Readout
              k="VISITED"
              v={`${num(visited)} / ${num(exist)}`}
              tone={visited > 0 ? "#E0A34E" : undefined}
            />
            <Readout
              k="CYCLES"
              v={num(facts?.cycle.length ?? 0)}
              tone={(facts?.cycle.length ?? 0) === 0 ? "#3FCFA8" : "#E0A34E"}
            />
          </>
        }
      />

      <SubBar>
        <SubItem on>DEFINITIONS</SubItem>
        <SubItem>DEPENDENCY DAG</SubItem>
        <SubItem>DIRTY PROPAGATION</SubItem>
        <div style={{ flex: 1 }} />
        <SubItem tone="#585550">edit a definition and only what is downstream is marked</SubItem>
      </SubBar>

      <Body>
        <div className="rail" style={{ width: 262 }}>
          <div className="rail-h">
            DEFINED TERMS<div /><span style={{ color: "#585550" }}>{exist}</span>
          </div>
          <input
            className="filt"
            placeholder="filter…"
            value={filter}
            onChange={(e) => setFilter(e.target.value)}
            style={{ outline: "none", width: "calc(100% - 24px)" }}
          />
          <div style={{ display: "flex", flexDirection: "column", gap: 1, padding: "0 8px", overflowY: "auto", flex: 1 }}>
            {shown.length === 0 && (
              <div style={{ padding: 8, fontSize: 11.5, color: "#585550" }}>
                {exist === 0
                  ? "No definitions yet. A fact is a {{name}} defined once and transcluded elsewhere."
                  : "Nothing matches."}
              </div>
            )}
            {shown.map((d) => {
              const unused = !referenced.has(d.name);
              const dupe = dupeNames.has(d.name);
              return (
                <div
                  key={d.name}
                  className={`tr${d.name === sel ? " tr-on" : ""}`}
                  style={{ cursor: "pointer", ...(unused ? { color: "#585550" } : {}), ...(dupe ? { color: "#E0A34E" } : {}) }}
                  onClick={() => setSelected(d.name)}
                >
                  {d.name === sel && <i />}
                  {d.name}
                  {dupe && (
                    <span style={{ marginLeft: "auto", font: "400 8.5px 'IBM Plex Mono',monospace" }}>
                      DUPLICATE
                    </span>
                  )}
                  {!dupe && unused && (
                    <span style={{
                      marginLeft: "auto",
                      font: "400 8.5px 'IBM Plex Mono',monospace",
                      color: "#4B4842",
                    }}>
                      UNUSED
                    </span>
                  )}
                </div>
              );
            })}
          </div>
          <div className="wal">
            <Label>LAST EDIT COST</Label>
            <div className="mono" style={{ fontSize: 10.5, lineHeight: 1.8, color: "#8C8880" }}>
              visited&nbsp;&nbsp;{num(visited)}<br />
              exist&nbsp;&nbsp;&nbsp;&nbsp;{num(exist)}<br />
              <span style={{ color: neverWalked > 50 ? "#3FCFA8" : "#E0A34E" }}>
                {neverWalked}% never walked
              </span>
            </div>
          </div>
        </div>

        <div style={{ flex: 1, minWidth: 0, display: "flex", flexDirection: "column", overflow: "hidden" }}>
          <div style={{ padding: "26px 34px", borderBottom: "1px solid rgba(255,255,255,.07)" }}>
            <div style={{ display: "flex", alignItems: "baseline", gap: 12, marginBottom: 12 }}>
              <span className="mono" style={{
                fontSize: 9, fontWeight: 600, letterSpacing: ".2em", color: "#E8873C",
              }}>
                DEFINITION
              </span>
              <h1 className="h1" style={{ fontSize: 23 }}>
                {sel ? `{{${sel}}}` : "no definitions"}
              </h1>
              <div style={{ flex: 1 }} />
              {selDef && (
                <span
                  className="chip"
                  style={{ cursor: "pointer" }}
                  onClick={() => navigate(`/read/${selDef.page_id}`)}
                >
                  OPEN SOURCE PAGE
                </span>
              )}
            </div>
            {selDef ? (
              <div style={{
                fontFamily: "Spectral,serif", fontSize: 17, lineHeight: 1.68, color: "#D2CFC8",
                borderLeft: "2px solid #E8873C", background: "rgba(232,135,60,.05)",
                padding: "13px 17px",
              }}>
                {selDef.value}
              </div>
            ) : (
              <div style={{ fontSize: 12.5, color: "#585550", lineHeight: 1.7, maxWidth: 620 }}>
                A fact is a named definition written once and transcluded wherever it is
                needed. Nothing defines one yet, so the DAG has no nodes — which is a real
                empty state, not a failure to load.
              </div>
            )}
          </div>

          <div style={{ padding: "22px 34px", flex: 1, minHeight: 0, display: "flex", flexDirection: "column", gap: 18 }}>
            <div>
              <Label>
                WHAT GOES STALE IF THIS CHANGES · {num(visited)}
              </Label>
              {visited === 0 && (
                <div style={{ fontSize: 11.5, color: "#585550", lineHeight: 1.6 }}>
                  Nothing transcludes this yet. Editing it walks no edges at all — which is the
                  cheapest possible edit, and the reason the cost is measured rather than assumed.
                </div>
              )}
              <div style={{ display: "flex", flexDirection: "column" }}>
                {(stale ?? []).map((r, i) => (
                  <div
                    key={`${r.page_id}-${r.block_id}-${i}`}
                    className="row"
                    style={{ padding: "9px 0", cursor: "pointer" }}
                    onClick={() => navigate(`/read/${r.page_id}`)}
                  >
                    <span style={{ color: "#E0A34E", fontSize: 10 }}>◌</span>
                    <span style={{ flex: 1, fontSize: 12.5, color: "#D2CFC8" }}>
                      transcludes {`{{${r.name}}}`}
                    </span>
                    <span className="mono" style={{ fontSize: 10, color: "#585550" }}>
                      block {r.block_id.slice(0, 4)}
                    </span>
                  </div>
                ))}
              </div>
            </div>

            <div style={{ marginTop: "auto", paddingTop: 22, borderTop: "1px solid rgba(255,255,255,.07)",
                          display: "grid", gridTemplateColumns: "1fr 1fr 1.1fr", gap: 26 }}>
              <div>
                <Label>CYCLES</Label>
                {(facts?.cycle.length ?? 0) === 0 ? (
                  <div style={{ fontSize: 11.5, color: "#8C8880", lineHeight: 1.6 }}>
                    <span style={{ color: "#3FCFA8" }}>✓</span> None. Three-colour DFS —
                    a visited set alone answers "seen before", not "on the current path".
                  </div>
                ) : (
                  <div className="mono" style={{ fontSize: 10.5, color: "#E0A34E", lineHeight: 1.8 }}>
                    {facts!.cycle.join(" → ")}
                  </div>
                )}
              </div>
              <div>
                <Label>DUPLICATES</Label>
                {(facts?.duplicates.length ?? 0) === 0 ? (
                  <div style={{ fontSize: 11.5, color: "#8C8880", lineHeight: 1.6 }}>
                    <span style={{ color: "#3FCFA8" }}>✓</span> Every name defined once.
                  </div>
                ) : (
                  <div style={{ display: "flex", flexDirection: "column", gap: 6 }}>
                    {facts!.duplicates.map((d) => (
                      <div key={d.name} style={{ fontSize: 11.5, color: "#E0A34E" }}>
                        {`{{${d.name}}}`}
                        <span className="mono" style={{ color: "#585550", marginLeft: 6, fontSize: 10 }}>
                          ×{d.definitions.length}
                        </span>
                      </div>
                    ))}
                  </div>
                )}
              </div>
              <div>
                <Label>WHY A DAG AT ALL</Label>
                <div style={{ fontSize: 11.5, lineHeight: 1.7, color: "#8C8880" }}>
                  The counter is nodes <i>visited</i> against nodes that exist. If editing one
                  definition walked all of them, the graph would be costing more than it saves —
                  the ratio is the whole argument for incremental invalidation, so it is measured
                  rather than asserted.
                </div>
              </div>
            </div>
          </div>
        </div>

        <Inspector
          tabs={[{ id: "refs", label: "REFERENCES" }, { id: "cost", label: "COST" }]}
          active="refs"
        >
          <Label>DEFINED IN</Label>
          {selDef ? (
            <div
              style={{ fontSize: 12, color: "#D2CFC8", cursor: "pointer" }}
              onClick={() => navigate(`/read/${selDef.page_id}`)}
            >
              block {selDef.block_id.slice(0, 8)}
            </div>
          ) : (
            <span style={{ fontSize: 11.5, color: "#585550" }}>—</span>
          )}

          <Rule />
          <Label>ALL REFERENCES · {num(facts?.references.length ?? 0)}</Label>
          <div style={{ display: "flex", flexDirection: "column", gap: 7 }}>
            {(facts?.references ?? []).slice(0, 12).map((r, i) => (
              <div
                key={i}
                style={{ display: "flex", gap: 8, fontSize: 11.5, cursor: "pointer" }}
                onClick={() => navigate(`/read/${r.page_id}`)}
              >
                <span style={{ flex: 1, color: r.name === sel ? "#E4E2DC" : "#8C8880" }}>
                  {`{{${r.name}}}`}
                </span>
                <span className="mono" style={{ fontSize: 9.5, color: "#585550" }}>
                  {r.block_id.slice(0, 4)}
                </span>
              </div>
            ))}
            {(facts?.references.length ?? 0) === 0 && (
              <span style={{ fontSize: 11.5, color: "#585550", lineHeight: 1.6 }}>
                No transclusions anywhere yet.
              </span>
            )}
          </div>

          <Rule />
          <Label>UNUSED · {num(defs.filter((d) => !referenced.has(d.name)).length)}</Label>
          <div style={{ fontSize: 11, lineHeight: 1.6, color: "#585550" }}>
            A definition nothing transcludes still costs nothing to keep — it is a note, not
            dead code. Reported so it can be found, not so it can be swept up.
          </div>
        </Inspector>
      </Body>

      <StatusBar
        route="/facts"
        mechanism="dependency DAG · topological dirty propagation"
        state={
          err
            ? "facts unavailable"
            : `${num(exist)} defined · ${neverWalked}% never walked`
        }
        healthy={!err && (facts?.cycle.length ?? 0) === 0}
      />
    </Screen>
  );
}

export default FactsScreen;
