/**
 * docs/ui-mockups/v2/index.html § 24e, the NOT FOUND half.
 *
 * § 24e is one artboard showing two states side by side — a new workspace and
 * a wrong URL — because they are the same design problem: a screen with
 * nothing on it still has to say what to do. In the app they are two
 * different situations reached by two different routes, so the first-run half
 * lives on the dashboard (where a workspace with no pages actually lands) and
 * this is the wrong-URL half.
 *
 * The screen's own claim, and the reason it is not a generic 404: a missing
 * page here is the SAME state a [[link]] typed before its page exists is
 * already in — dangling, not an error. So the page offers the two actions
 * that resolve it rather than an apology, and "did you mean" is the real
 * BK-tree over page titles, not a substring scan.
 */
import { useEffect, useState } from "react";
import { useLocation, useNavigate } from "react-router-dom";
import { useAuth } from "../auth/AuthContext";
import { suggestTitles, type TitleSuggestion } from "../api/search";
import { createPage } from "../api/pages";
import { Body, Label, Screen, StatusBar, TopBar } from "../shell/Chrome";

/** The last path segment, de-slugged — the closest thing to "what did they
 *  mean" that a URL carries. */
function guessFromPath(pathname: string): string {
  const last = pathname.split("/").filter(Boolean).pop() ?? "";
  return decodeURIComponent(last).replace(/[-_]+/g, " ").trim();
}

export function NotFoundScreen() {
  const { pathname } = useLocation();
  const navigate = useNavigate();
  const { session } = useAuth();
  const actorId = session?.actorId ?? null;

  const guess = guessFromPath(pathname);
  const [suggestions, setSuggestions] = useState<TitleSuggestion[]>([]);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    if (!actorId || guess.length < 2) return;
    // A real BK-tree over page titles (internal/bktree), reached through
    // /search/suggest — the same index the editor's "did you mean" uses. A
    // substring scan here would answer a different question and would miss
    // exactly the case this screen exists for: a typo.
    suggestTitles(actorId, guess, 5).then((r) => setSuggestions(r.suggestions)).catch(() => setSuggestions([]));
  }, [actorId, guess]);

  async function create() {
    if (!actorId || !guess) return;
    setBusy(true);
    try {
      const p = await createPage(actorId, guess);
      navigate(`/pages/${p.id}`);
    } finally {
      setBusy(false);
    }
  }

  return (
    <Screen>
      <TopBar crumb={<>workspace / <b>not found</b></>} />
      <Body>
        <div style={{ flex: 1, minWidth: 0, display: "flex", flexDirection: "column" }}>
          <div style={{
            padding: "12px 20px", borderBottom: "1px solid rgba(255,255,255,.07)",
            background: "#0F1012",
          }}>
            <Label>NOT FOUND · <span style={{ color: "#E0A34E" }}>{pathname}</span></Label>
          </div>
          <div style={{ flex: 1, display: "flex", alignItems: "center", justifyContent: "center", padding: 40 }}>
            <div style={{ width: 440 }}>
              <div className="mono" style={{
                fontSize: 9, fontWeight: 600, letterSpacing: ".2em", color: "#E0A34E", marginBottom: 12,
              }}>
                ◌ NO PAGE WITH THIS NAME
              </div>
              <h1 className="h1" style={{ fontSize: 26, lineHeight: 1.2, marginBottom: 12 }}>
                Nothing is broken. This page does not exist yet.
              </h1>
              <div style={{ fontSize: 13, lineHeight: 1.7, color: "#8C8880", marginBottom: 22 }}>
                A missing page here is the same state a{" "}
                <span className="mono" style={{ color: "#8C8880" }}>[[link]]</span> typed before
                its page exists is already in — dangling, not an error. The fix is to create it
                or to go to the one you meant.
              </div>

              <Label style={{ marginBottom: 11, display: "block" }}>
                DID YOU MEAN · BK-TREE OVER PAGE TITLES
              </Label>
              <div style={{ display: "flex", flexDirection: "column", gap: 8, marginBottom: 20 }}>
                {suggestions.length === 0 && (
                  <div style={{ fontSize: 12, lineHeight: 1.6, color: "#585550" }}>
                    Nothing within edit distance. The tree searched and came back empty, which
                    is a different answer from not having looked.
                  </div>
                )}
                {suggestions.map((s, i) => (
                    <div
                      key={s.page_id}
                      className="fx"
                      style={{
                        display: "flex", alignItems: "center", gap: 11, padding: "11px 13px",
                        cursor: "pointer", animationDelay: `${i * 0.05}s`,
                        border: i === 0 ? "1px solid rgba(232,135,60,.3)" : "1px solid rgba(255,255,255,.08)",
                        background: i === 0 ? "rgba(232,135,60,.04)" : undefined,
                      }}
                      onClick={() => navigate(`/read/${s.page_id}`)}
                    >
                      <span style={{ flex: 1, fontSize: 13, color: i === 0 ? "#EFEDE7" : "#D2CFC8" }}>
                        {s.title}
                      </span>
                      <span className="mono" style={{ fontSize: 9.5, color: "#585550" }}>d={s.distance}</span>
                    </div>
                ))}
              </div>

              <div style={{ display: "flex", gap: 9, marginBottom: 20 }}>
                <div
                  className="btn"
                  style={{
                    borderColor: "rgba(232,135,60,.45)", color: "#E8873C",
                    cursor: guess ? "pointer" : "default", opacity: guess ? 1 : 0.4,
                  }}
                  onClick={() => guess && void create()}
                >
                  {busy ? "…" : "CREATE THIS PAGE"}
                  <div className="brk-tl" /><div className="brk-br" />
                </div>
                <div
                  className="btn"
                  style={{ cursor: "pointer" }}
                  onClick={() => navigate(guess ? `/search?q=${encodeURIComponent(guess)}` : "/search")}
                >
                  SEARCH INSTEAD<div className="brk-tl" /><div className="brk-br" />
                </div>
              </div>

              <div style={{
                paddingTop: 16, borderTop: "1px solid rgba(255,255,255,.07)",
                fontSize: 11.5, lineHeight: 1.65, color: "#585550",
              }}>
                If it existed and was deleted, the trash says so instead — a purge keeps the page
                id precisely so “this is gone” and “this never was” stay different answers.
              </div>
            </div>
          </div>
        </div>
      </Body>
      <StatusBar
        route={pathname}
        mechanism="a missing page is dangling, never an error"
        state="offers the action, not an apology"
        healthy={false}
      />
    </Screen>
  );
}

export default NotFoundScreen;
