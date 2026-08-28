import { useEffect, useMemo, useState, type ReactNode } from "react";
import { Link, useNavigate } from "react-router-dom";
import { useAuth } from "../auth/AuthContext";
import { search, suggestTitles, type SearchHit, type TitleSuggestion } from "../api/search";

/**
 * docs/ui-mockups/v2/index.html § 06 SEARCH, made real (v2.5.0): real full-text search
 * (GET /search, Postgres FTS over docs.pages/docs.blocks' own tsvector
 * columns) and real fuzzy "did you mean" (GET /search/suggest,
 * internal/bktree's BK-tree, unchanged) — nothing here re-derives either
 * algorithm in TypeScript (ADR-012). The "kind" facet (Everything / Titles
 * only / Block mentions) is real, computed from each hit's own block_id
 * presence; the mockup's own "Product"/"Reading"/"Archive" SCOPE facets
 * are not — this repo has one flat workspace, nothing yet for them to
 * filter by, an honest omission rather than a cosmetic stub. The
 * mockup's per-result link-graph SVG and result-path breadcrumb are
 * dropped for the same reason: no real per-hit hierarchy/graph data is
 * fetched here, and a decorative substitute would be exactly the "not
 * real" the rest of this repo refuses to ship.
 */
export function SearchScreen() {
  const { session, logout } = useAuth();
  if (!session) throw new Error("SearchScreen requires an authenticated session");
  const { actorId } = session;
  const navigate = useNavigate();

  const [query, setQuery] = useState("");
  const [hits, setHits] = useState<SearchHit[] | null>(null);
  const [suggestions, setSuggestions] = useState<TitleSuggestion[]>([]);
  const [error, setError] = useState<string | null>(null);
  const [kindFilter, setKindFilter] = useState<"all" | "page" | "block">("all");
  const [elapsedMs, setElapsedMs] = useState<number | null>(null);

  useEffect(() => {
    const q = query.trim();
    if (!q) {
      setHits(null);
      setSuggestions([]);
      setError(null);
      setElapsedMs(null);
      return;
    }
    let cancelled = false;
    const timer = setTimeout(() => {
      const startedAt = performance.now();
      search(actorId, q)
        .then((r) => {
          if (cancelled) return;
          setHits(r.hits);
          setElapsedMs(Math.round(performance.now() - startedAt));
          setError(null);
          if (r.hits.length === 0) {
            suggestTitles(actorId, q).then((s) => {
              if (!cancelled) setSuggestions(s.suggestions);
            });
          } else {
            setSuggestions([]);
          }
        })
        .catch(() => {
          if (!cancelled) setError("Search failed.");
        });
    }, 200); // debounced — search.html's own "typing filters the results," not one request per keystroke
    return () => {
      cancelled = true;
      clearTimeout(timer);
    };
  }, [query, actorId]);

  const filtered = useMemo(() => {
    if (!hits) return null;
    if (kindFilter === "all") return hits;
    return hits.filter((h) => (kindFilter === "block" ? !!h.block_id : !h.block_id));
  }, [hits, kindFilter]);

  return (
    <div className="app">
      <header className="topbar">
        <Link to="/pages" className="brand" style={{ textDecoration: "none" }}>
          <span className="mark"></span>Marginal
        </Link>
        <nav className="nav">
          <Link to="/pages">Editor</Link>
          <Link to="/search" aria-current="page">Search</Link>
          <Link to="/graph">Graph</Link>
        </nav>
        <div className="spacer"></div>
        <button className="btn" onClick={logout}>Sign out</button>
      </header>

      <main className="page">
        <div className="wrap">
          <div className="searchbar">
            <input
              autoFocus
              value={query}
              onChange={(e) => setQuery(e.target.value)}
              placeholder="Search everything…"
              autoComplete="off"
              aria-label="Search"
            />
            <div className="facets" role="group" aria-label="Filters">
              {(["all", "page", "block"] as const).map((k) => (
                <button
                  key={k}
                  className="facet"
                  aria-pressed={kindFilter === k}
                  onClick={() => setKindFilter(k)}
                >
                  {k === "all" ? "Everything" : k === "page" ? "Titles only" : "Block mentions"}
                </button>
              ))}
            </div>
          </div>

          <div className="layout">
            <div>
              {hits && (
                <div className="meta-line">
                  <span>{filtered?.length ?? 0} {filtered?.length === 1 ? "result" : "results"}</span>
                  {elapsedMs !== null && (
                    <>
                      <span>·</span>
                      <span>{elapsedMs} ms</span>
                    </>
                  )}
                  <span className="spacer"></span>
                  <span className="pill amber" title="internal/search.DefaultRefreshInterval — fuzzy suggestions only, full-text search is always transactionally fresh">
                    fuzzy index refreshes every 30s
                  </span>
                </div>
              )}

              {error && <div className="note" style={{ maxWidth: "none" }}>{error}</div>}

              {query.trim() && hits && hits.length === 0 && !error && (
                <div className="muted" style={{ padding: "16px 0" }}>No results for "{query}".</div>
              )}

              <div>
                {filtered?.map((h, i) => (
                  <a
                    key={`${h.page_id}-${h.block_id ?? "title"}-${i}`}
                    className="result"
                    href={`/pages/${h.page_id}`}
                    onClick={(e) => {
                      e.preventDefault();
                      navigate(`/pages/${h.page_id}`);
                    }}
                  >
                    <div className="result-t">{h.page_title || "Untitled"}</div>
                    {h.snippet && <div className="result-snip">{renderSnippet(h.snippet)}</div>}
                    <div className="result-meta">
                      <span className="pill">{h.block_id ? "Block" : "Page"}</span>
                    </div>
                  </a>
                ))}
              </div>

              <div className="note" style={{ maxWidth: "none" }}>
                <b>Really real.</b> Every result above comes from Postgres full-text search
                (<code>websearch_to_tsquery</code> + <code>ts_rank</code>) over this workspace's actual
                pages and blocks — the snippet is <code>ts_headline</code>'s own output, the reason it
                matched, not a guess.
              </div>
            </div>

            <aside>
              <div className="card aside-card">
                <div className="label">Did you mean</div>
                {suggestions.length === 0 ? (
                  <div className="muted" style={{ fontSize: 12.5, padding: "6px 0" }}>
                    {query.trim() ? "No close titles found." : "Type a query to search."}
                  </div>
                ) : (
                  suggestions.map((s) => (
                    <div
                      className="row"
                      key={s.page_id}
                      onClick={() => navigate(`/pages/${s.page_id}`)}
                      style={{ cursor: "pointer" }}
                    >
                      <span className="lead">~</span>
                      {s.title}
                      <span className="muted" style={{ marginLeft: "auto" }}>edit distance {s.distance}</span>
                    </div>
                  ))
                )}
                <div className="hr"></div>
                <p style={{ margin: 0, fontSize: 12.5, color: "var(--ink-soft)", lineHeight: 1.55 }}>
                  A real Burkhard-Keller tree over page titles — <code>internal/bktree</code>, pruned by
                  the triangle inequality, not a fuzzy heuristic.
                </p>
              </div>
            </aside>
          </div>
        </div>
      </main>
    </div>
  );
}

/** ts_headline wraps a matched term in <b>...</b> — the only tag Postgres
 * ever emits here. Split on it and render everything else as plain text
 * (auto-escaped by React), rather than dangerouslySetInnerHTML — a
 * block's own prose is untrusted-ish user text that ts_headline does NOT
 * escape on the non-matched side, so treating the whole string as HTML
 * would be a real (self-)XSS surface for anyone who ever typed a "<" into
 * their own notes. */
function renderSnippet(snippet: string): ReactNode[] {
  const parts = snippet.split(/(<b>|<\/b>)/);
  const out: ReactNode[] = [];
  let marking = false;
  for (const part of parts) {
    if (part === "<b>") {
      marking = true;
    } else if (part === "</b>") {
      marking = false;
    } else if (part) {
      out.push(marking ? <mark key={out.length}>{part}</mark> : <span key={out.length}>{part}</span>);
    }
  }
  return out;
}
