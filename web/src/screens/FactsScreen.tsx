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
 * instantly, its "Introduce a cycle"/"Duplicate a definition" buttons
 * mutating a hard-coded JS object — its whole page is a client-side
 * simulation. Here there's a real page tree behind every definition and
 * reference, so those specific buttons have no honest equivalent (there's
 * no way to "introduce a cycle" without actually writing ops to a real
 * page) and are dropped rather than faked; editing happens in the real
 * editor (open the page, add or change a `{{define name = value}}` block
 * or a `{{name}}` reference) — this screen is the read side: what the
 * fact graph looks like right now, and which references go stale if a
 * given definition changes.
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
        <button className="btn" onClick={logout}>Sign out</button>
      </header>

      <div className="fbar">
        <span className="label">Try it</span>
        <span className="muted" style={{ fontSize: 12 }}>
          Open a page and write <code>{"{{define name = value}}"}</code> as a block's whole text to
          define a fact, then <code>{"{{name}}"}</code> anywhere else to reference it — every
          definition change here is a real edit through the real editor, not a client-side toy.
        </span>
        <span className="spacer"></span>
        <button className="btn" onClick={() => setRefreshKey((k) => k + 1)}>↻ Refresh</button>
      </div>

      <div className="fsplit">
        <main className="pane">
          {error && <div className="note" style={{ maxWidth: "none" }}>{error}</div>}
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
                  <div key={`${d.name}-${d.block_id}`} className={`fact-card ${isDup || inCycle ? "warn" : ""}`.trim()}>
                    <div className="fname">
                      {"{{" + d.name + "}}"}
                      {isDup && <span className="pill amber" style={{ marginLeft: 6 }}>defined twice</span>}
                      {inCycle && <span className="pill amber" style={{ marginLeft: 6 }}>in a cycle</span>}
                    </div>
                    <div className="fval">{d.value}</div>
                    <div className="fmeta">
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

              <div className="label" style={{ margin: "24px 0 9px" }}>Pages that reference them · {facts.references.length}</div>
              {facts.references.length === 0 && (
                <div className="muted" style={{ padding: "8px 0", fontSize: 12.5 }}>
                  No page references a fact yet.
                </div>
              )}
              {facts.references.map((r, i) => {
                const isStale = staleKeys.has(`${r.page_id}:${r.block_id}`);
                return (
                  <div key={`${r.name}-${r.page_id}-${i}`} className={`doc-card ${isStale ? "stale" : ""}`.trim()}>
                    <Link to={`/pages/${r.page_id}`} className="dtitle">{titleOf(r.page_id)}</Link>
                    <div className="dbody">
                      references <span className="ref">{"{{" + r.name + "}}"}</span>
                    </div>
                    <div className="dfoot">
                      {isStale ? (
                        <span className="pill amber">stale — reviewed change upstream</span>
                      ) : (
                        <span className="pill teal">up to date</span>
                      )}
                    </div>
                  </div>
                );
              })}

              <div className="note" style={{ maxWidth: "38rem" }}>
                <b>Really real.</b> The dependency DAG, the topological dirty propagation, the cycle
                check, and the duplicate detection all run in Go (<code>internal/facts</code>). Note
                what is <i>not</i> happening: nothing tries to read a number out of prose — the edge
                exists because someone wrote <code>{"{{p99-latency}}"}</code>, which is why this is
                exact instead of approximately right.
              </div>
            </>
          )}
        </main>

        <aside className="rail right" style={{ width: "auto" }}>
          <div className="rail-head"><span className="label">Invalidation</span></div>
          <div className="panel-body">
            <div className="metric2"><span>Definitions</span><span className="v">{facts?.definitions.length ?? "—"}</span></div>
            <div className="metric2"><span>References</span><span className="v">{facts?.references.length ?? "—"}</span></div>
            <div className="metric2"><span>Duplicate names</span><span className={`v ${duplicateNames.size ? "warn" : ""}`.trim()}>{facts?.duplicates.length ?? "—"}</span></div>
            <div className="metric2"><span>Cycle</span><span className={`v ${cycleNames.size ? "warn" : ""}`.trim()}>{facts?.cycle.length ? facts.cycle.join(" → ") : "none"}</span></div>

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
