import { useEffect, useMemo, useState } from "react";
import { Link } from "react-router-dom";
import { useAuth } from "../auth/AuthContext";
import { getFacts, getStaleReferences, type FactsGraph, type FactReference } from "../api/diagnostics";
import { getPage, type Page } from "../api/pages";

/**
 * docs/ui-mockups/facts.html, made real (v2.3.0): the dependency DAG, the
 * cycle check, and the duplicate-name detection are diagnostics-service's
 * own `internal/facts` (graphalgo.DetectCycle/ForwardReachable, unchanged
 * — the same algorithms graph-algorithms.html already runs, applied to
 * fact names instead of pages), read via GET /facts. This file draws what
 * Go already computed and fetches GET /facts/{name}/stale on demand — it
 * never re-derives the DAG or the propagation itself (ADR-012).
 *
 * The mockup lets you type `{{name}}` inline and see the graph react
 * instantly, because its whole page is a client-side simulation. Here
 * there's a real page tree behind every definition and reference, so
 * editing happens in the real editor (open the page, add or change a
 * `{{define name = value}}` block or a `{{name}}` reference) — this
 * screen is the read side: what the fact graph looks like right now, and
 * which references go stale if a given definition changes.
 */
export function FactsScreen() {
  const { session, logout } = useAuth();
  if (!session) throw new Error("FactsScreen requires an authenticated session");
  const { actorId } = session;

  const [facts, setFacts] = useState<FactsGraph | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [pagesById, setPagesById] = useState<Map<string, Page>>(new Map());
  const [selected, setSelected] = useState<string | null>(null);
  const [stale, setStale] = useState<FactReference[] | null>(null);
  const [refreshKey, setRefreshKey] = useState(0);

  useEffect(() => {
    let cancelled = false;
    setFacts(null);
    setError(null);
    getFacts(actorId)
      .then((f) => {
        if (!cancelled) setFacts(f);
      })
      .catch(() => {
        if (!cancelled) setError("Couldn't load the fact graph.");
      });
    return () => {
      cancelled = true;
    };
  }, [actorId, refreshKey]);

  // Resolve every page id this graph mentions to a real title, one
  // GetPage call each — the set is small at this repo's demo scale, and
  // this is the read path every other screen already uses (pages.ts).
  useEffect(() => {
    if (!facts) return;
    const ids = new Set<string>();
    for (const d of facts.definitions) ids.add(d.page_id);
    for (const r of facts.references) ids.add(r.page_id);
    const missing = [...ids].filter((id) => !pagesById.has(id));
    if (missing.length === 0) return;
    let cancelled = false;
    Promise.all(missing.map((id) => getPage(actorId, id).catch(() => null))).then((pages) => {
      if (cancelled) return;
      setPagesById((prev) => {
        const next = new Map(prev);
        for (const p of pages) if (p) next.set(p.id, p);
        return next;
      });
    });
    return () => {
      cancelled = true;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [facts, actorId]);

  useEffect(() => {
    if (!selected) {
      setStale(null);
      return;
    }
    let cancelled = false;
    getStaleReferences(actorId, selected).then((refs) => {
      if (!cancelled) setStale(refs);
    });
    return () => {
      cancelled = true;
    };
  }, [actorId, selected]);

  const duplicateNames = useMemo(() => new Set(facts?.duplicates.map((d) => d.name) ?? []), [facts]);
  const cycleNames = useMemo(() => new Set(facts?.cycle ?? []), [facts]);
  const staleKeys = useMemo(() => new Set(stale?.map((r) => `${r.page_id}:${r.block_id}`) ?? []), [stale]);

  const referencesByName = useMemo(() => {
    const m = new Map<string, FactReference[]>();
    for (const r of facts?.references ?? []) {
      const list = m.get(r.name) ?? [];
      list.push(r);
      m.set(r.name, list);
    }
    return m;
  }, [facts]);

  const titleOf = (pageId: string) => pagesById.get(pageId)?.title || "Untitled";

  return (
    <div className="app">
      <header className="topbar">
        <Link to="/pages" className="brand" style={{ textDecoration: "none" }}>
          <span className="mark"></span>Marginal
        </Link>
        <nav className="nav">
          <Link to="/graph">Graph</Link>
          <Link to="/graph/algorithms">Algorithms</Link>
          <Link to="/facts" aria-current="page">Facts</Link>
        </nav>
        <div className="crumb">Workspace · <b>Facts</b></div>
        <div className="spacer"></div>
        <span className={`pill ${cycleNames.size ? "amber" : "teal"}`}>
          {cycleNames.size ? `${cycleNames.size} in a cycle` : "no cycles"}
        </span>
        <button className="btn" onClick={() => setRefreshKey((k) => k + 1)}>Re-run</button>
        <button className="btn" onClick={logout}>Sign out</button>
      </header>

      <div className="fsplit" style={{ flex: 1, display: "grid", gridTemplateColumns: "1fr 340px", minHeight: 0 }}>
        <main className="pane" style={{ overflowY: "auto", padding: "26px" }}>
          {error && <div className="note">{error}</div>}
          {!facts && !error && <div className="muted">Loading the fact graph…</div>}

          {facts && (
            <>
              <div className="label" style={{ marginBottom: 9 }}>Definitions · {facts.definitions.length}</div>
              {facts.definitions.length === 0 && (
                <div className="note" style={{ maxWidth: "38rem" }}>
                  No facts defined yet. Write <code>{"{{define name = value}}"}</code> as a
                  block's entire text on any page to define one, then reference it elsewhere
                  with <code>{"{{name}}"}</code>.
                </div>
              )}
              {facts.definitions.map((d) => {
                const uses = referencesByName.get(d.name)?.length ?? 0;
                const isDup = duplicateNames.has(d.name);
                const inCycle = cycleNames.has(d.name);
                return (
                  <div
                    key={`${d.name}-${d.block_id}`}
                    className="fact-card"
                    style={{
                      border: "1px solid var(--rule)",
                      borderLeft: `2px solid ${isDup || inCycle ? "var(--amber)" : "var(--teal)"}`,
                      borderRadius: "var(--radius-lg)",
                      background: "var(--bg-raised)",
                      padding: "13px 15px",
                      marginBottom: 12,
                    }}
                  >
                    <div style={{ fontFamily: "var(--mono)", fontSize: 11.5, color: isDup || inCycle ? "var(--amber)" : "var(--teal)" }}>
                      {"{{" + d.name + "}}"}
                      {isDup && <span className="pill amber" style={{ marginLeft: 6 }}>defined twice</span>}
                      {inCycle && <span className="pill amber" style={{ marginLeft: 6 }}>in a cycle</span>}
                    </div>
                    <div style={{ marginTop: 7, fontFamily: "var(--display)", fontSize: 19, fontWeight: 560 }}>{d.value}</div>
                    <div className="fmeta" style={{ marginTop: 7, fontSize: 11.5, color: "var(--ink-faint)", display: "flex", alignItems: "center", gap: 10 }}>
                      <Link to={`/pages/${d.page_id}`}>{titleOf(d.page_id)}</Link>
                      <span>{uses} reference{uses === 1 ? "" : "s"} downstream</span>
                      <button className="btn" style={{ marginLeft: "auto" }} onClick={() => setSelected(d.name)}>
                        {selected === d.name ? "Checking…" : "Check downstream"}
                      </button>
                    </div>
                  </div>
                );
              })}

              {facts.duplicates.length > 0 && (
                <div className="panel-section" style={{ marginTop: 20 }}>
                  <div className="label" style={{ marginBottom: 9 }}>Duplicate names · {facts.duplicates.length}</div>
                  <div className="note" style={{ maxWidth: "38rem" }}>
                    A hash-lookup collision, not a satisfiability problem — each of these
                    names has more than one definition, so none of them resolve.
                  </div>
                  {facts.duplicates.map((group) => (
                    <div key={group.name} className="note" style={{ maxWidth: "38rem" }}>
                      <b>{"{{" + group.name + "}}"}</b> is defined {group.definitions.length} times:{" "}
                      {group.definitions.map((d, i) => (
                        <span key={`${d.page_id}-${d.block_id}`}>
                          {i > 0 && ", "}
                          <Link to={`/pages/${d.page_id}`}>{titleOf(d.page_id)}</Link> = {d.value}
                        </span>
                      ))}
                    </div>
                  ))}
                </div>
              )}

              <div className="label" style={{ margin: "24px 0 9px" }}>References · {facts.references.length}</div>
              {facts.references.length === 0 && (
                <div className="muted" style={{ padding: "8px 0", fontSize: 12.5 }}>
                  No page references a fact yet.
                </div>
              )}
              {facts.references.map((r, i) => {
                const isStale = staleKeys.has(`${r.page_id}:${r.block_id}`);
                return (
                  <div
                    key={`${r.name}-${r.page_id}-${i}`}
                    className={`doc-card ${isStale ? "stale" : ""}`}
                    style={{
                      border: "1px solid var(--rule)",
                      borderLeft: `2px solid ${isStale ? "var(--amber)" : "var(--rule-strong)"}`,
                      borderRadius: "var(--radius-lg)",
                      background: isStale ? "var(--amber-soft)" : "var(--bg-raised)",
                      padding: "13px 15px",
                      marginBottom: 10,
                    }}
                  >
                    <Link to={`/pages/${r.page_id}`} style={{ fontFamily: "var(--display)", fontSize: 15, fontWeight: 560 }}>
                      {titleOf(r.page_id)}
                    </Link>
                    <div style={{ marginTop: 6, fontSize: 13, color: "var(--ink-soft)" }}>
                      references <span className="ref" style={{ fontFamily: "var(--mono)" }}>{"{{" + r.name + "}}"}</span>
                    </div>
                    {isStale && <span className="pill amber" style={{ marginTop: 9, display: "inline-block" }}>stale — reviewed change upstream</span>}
                  </div>
                );
              })}
            </>
          )}
        </main>

        <aside className="rail right" style={{ width: "auto" }}>
          <div className="rail-head"><span className="label">Invalidation</span></div>
          <div className="panel-body">
            <div className="metric2" style={{ display: "flex", alignItems: "baseline", gap: 8, padding: "6px 0", borderBottom: "1px solid var(--rule)", fontSize: 12.5 }}>
              <span>Definitions</span><span className="v" style={{ marginLeft: "auto", fontFamily: "var(--mono)" }}>{facts?.definitions.length ?? "—"}</span>
            </div>
            <div className="metric2" style={{ display: "flex", alignItems: "baseline", gap: 8, padding: "6px 0", borderBottom: "1px solid var(--rule)", fontSize: 12.5 }}>
              <span>References</span><span className="v" style={{ marginLeft: "auto", fontFamily: "var(--mono)" }}>{facts?.references.length ?? "—"}</span>
            </div>
            <div className="metric2" style={{ display: "flex", alignItems: "baseline", gap: 8, padding: "6px 0", borderBottom: "1px solid var(--rule)", fontSize: 12.5 }}>
              <span>Duplicate names</span><span className="v" style={{ marginLeft: "auto", fontFamily: "var(--mono)", color: duplicateNames.size ? "var(--amber)" : undefined }}>{facts?.duplicates.length ?? "—"}</span>
            </div>
            <div className="metric2" style={{ display: "flex", alignItems: "baseline", gap: 8, padding: "6px 0", fontSize: 12.5 }}>
              <span>Cycle</span><span className="v" style={{ marginLeft: "auto", fontFamily: "var(--mono)", color: cycleNames.size ? "var(--amber)" : undefined }}>{facts?.cycle.length ? facts.cycle.join(" → ") : "none"}</span>
            </div>

            {selected && (
              <div className="panel-section" style={{ marginTop: 20 }}>
                <div className="panel-h">Downstream of {"{{" + selected + "}}"}</div>
                <div className="note" style={{ margin: 0, maxWidth: "none" }}>
                  {stale === null
                    ? "Walking the dependency DAG…"
                    : stale.length === 0
                      ? "Nothing downstream — safe to change without a stale reference anywhere."
                      : `${stale.length} reference${stale.length === 1 ? "" : "s"} would go stale, highlighted amber on the left.`}
                </div>
              </div>
            )}

            <div className="panel-section" style={{ marginTop: 20 }}>
              <div className="panel-h">Why not 2-SAT</div>
              <div className="note" style={{ margin: 0, maxWidth: "none" }}>
                With explicit <code>{"{{name}}"}</code> facts, a contradiction is two
                definitions of one key — a hash lookup, not a satisfiability problem
                (<code>ROADMAP.md</code> § Fact dependency graph).
              </div>
            </div>
          </div>
        </aside>
      </div>
    </div>
  );
}
