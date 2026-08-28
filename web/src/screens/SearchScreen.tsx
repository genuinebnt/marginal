import { useEffect, useMemo, useState, type ReactNode } from "react";
import { Link, useNavigate } from "react-router-dom";
import { useAuth } from "../auth/AuthContext";
import { search, suggestTitles, type SearchHit, type TitleSuggestion } from "../api/search";

/**
 * docs/ui-mockups/search.html, made real (v2.5.0): real full-text search
 * (GET /search, Postgres FTS over docs.pages/docs.blocks' own tsvector
 * columns) and real fuzzy "did you mean" (GET /search/suggest,
 * internal/bktree's BK-tree, unchanged) — nothing here re-derives either
 * algorithm in TypeScript (ADR-012). The "kind" filter (page title vs.
 * block mention) is real, computed from each hit's own block_id
 * presence; a "scope" filter isn't — this repo has one flat workspace,
 * so there's nothing yet for it to filter by (an honest omission, not a
 * cosmetic stub the mockup's own static version could get away with).
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

  useEffect(() => {
    const q = query.trim();
    if (!q) {
      setHits(null);
      setSuggestions([]);
      setError(null);
      return;
    }
    let cancelled = false;
    const timer = setTimeout(() => {
      search(actorId, q)
        .then((r) => {
          if (cancelled) return;
          setHits(r.hits);
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
          <Link to="/search" aria-current="page">Search</Link>
          <Link to="/graph">Graph</Link>
        </nav>
        <div className="spacer"></div>
        <button className="btn" onClick={logout}>Sign out</button>
      </header>

      <main className="canvas" style={{ maxWidth: "44rem", margin: "0 auto", padding: "0 24px 40px", width: "100%" }}>
        <div className="searchbar" style={{ padding: "26px 0 6px" }}>
          <input
            autoFocus
            value={query}
            onChange={(e) => setQuery(e.target.value)}
            placeholder="Search this workspace…"
            style={{
              width: "100%", padding: "13px 16px", fontFamily: "var(--display)", fontSize: 22,
              background: "var(--bg-raised)", color: "var(--ink)", border: "1px solid var(--rule)",
              borderRadius: "var(--radius-lg)",
            }}
          />
        </div>

        {hits && hits.length > 0 && (
          <div style={{ display: "flex", gap: 8, margin: "12px 0" }}>
            {(["all", "page", "block"] as const).map((k) => (
              <button
                key={k}
                className={`pill ${kindFilter === k ? "violet" : ""}`}
                onClick={() => setKindFilter(k)}
                style={{ cursor: "pointer" }}
              >
                {k === "all" ? "All" : k === "page" ? "Title matches" : "Block mentions"}
              </button>
            ))}
          </div>
        )}

        {error && <div className="note" style={{ marginTop: 16 }}>{error}</div>}

        {query.trim() && hits && hits.length === 0 && (
          <div style={{ marginTop: 20 }}>
            <div className="muted">No results for "{query}".</div>
            {suggestions.length > 0 && (
              <div className="note" style={{ marginTop: 12, maxWidth: "none" }}>
                Did you mean:{" "}
                {suggestions.map((s, i) => (
                  <span key={s.page_id}>
                    {i > 0 && ", "}
                    <a onClick={() => navigate(`/pages/${s.page_id}`)} style={{ cursor: "pointer" }}>
                      {s.title}
                    </a>
                  </span>
                ))}
                ?
              </div>
            )}
          </div>
        )}

        <div style={{ marginTop: 8 }}>
          {filtered?.map((h, i) => (
            <div
              key={`${h.page_id}-${h.block_id ?? "title"}-${i}`}
              className="result"
              onClick={() => navigate(`/pages/${h.page_id}`)}
              style={{
                display: "block", padding: "13px 4px", borderBottom: "1px solid var(--rule)", cursor: "pointer",
              }}
            >
              <div style={{ fontFamily: "var(--display)", fontSize: 17, fontWeight: 560 }}>{h.page_title || "Untitled"}</div>
              {h.snippet && (
                <div style={{ marginTop: 4, fontFamily: "var(--serif)", fontSize: 14, color: "var(--ink-soft)" }}>
                  {renderSnippet(h.snippet)}
                </div>
              )}
              <div style={{ marginTop: 6 }}>
                <span className="pill">{h.block_id ? "Block" : "Page"}</span>
              </div>
            </div>
          ))}
        </div>

        <div className="note" style={{ marginTop: 24, maxWidth: "none" }}>
          <b>Really real.</b> Every result above comes from Postgres full-text search
          (<code>websearch_to_tsquery</code> + <code>ts_rank</code>) over this workspace's actual pages
          and blocks — the snippet is <code>ts_headline</code>'s own output, the reason it matched, not
          a guess. "Did you mean" is a real BK-tree query over page titles, which has its own rebuild
          cadence (every 30s) and may lag a page just renamed — search is full text plus the link
          graph, never vectors.
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
