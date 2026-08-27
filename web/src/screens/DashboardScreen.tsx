import { useEffect, useState, type SyntheticEvent } from "react";
import { Link, useNavigate } from "react-router-dom";
import { useAuth } from "../auth/AuthContext";
import { createPage, listPages, type Page } from "../api/pages";
import { listNotifications, type Notification } from "../api/notifications";

/**
 * The landing screen after login — a Google-Docs/Sheets-style "pick or
 * start a document" grid, deliberately separate from the editor screen
 * (`EditorScreen`). The editor already carries a rail and (once the rich
 * block editor lands) an inspector on the other side; cramming "create a
 * new page" into that same three-column layout would bury the single
 * most common action new session behind two side panels. This screen has
 * exactly one job: get you into a page, new or existing.
 */
export function DashboardScreen() {
  const { session, logout } = useAuth();
  const navigate = useNavigate();
  const [pages, setPages] = useState<Page[]>([]);
  const [notifications, setNotifications] = useState<Notification[]>([]);
  const [showNotifications, setShowNotifications] = useState(false);
  const [creating, setCreating] = useState(false);

  if (!session) throw new Error("DashboardScreen requires an authenticated session");
  const { actorId } = session;

  useEffect(() => {
    // No parentId: root pages only, deliberately — sub-pages (real
    // nesting now, via EditorScreen's PageTreeRail) are reached by
    // drilling into a root page's tree, not listed flat here.
    listPages(actorId).then((r) => setPages(r.pages)).catch(() => setPages([]));
  }, [actorId]);

  useEffect(() => {
    listNotifications(actorId).then((r) => setNotifications(r.notifications)).catch(() => setNotifications([]));
  }, [actorId]);

  async function handleCreateBlank(e: SyntheticEvent) {
    e.preventDefault();
    if (creating) return;
    setCreating(true);
    try {
      const page = await createPage(actorId, "Untitled");
      navigate(`/pages/${page.id}`);
    } finally {
      setCreating(false);
    }
  }

  const sorted = [...pages].sort((a, b) => (a.updated_at < b.updated_at ? 1 : -1));

  return (
    <div className="app">
      <header className="topbar">
        <span className="brand">
          <span className="mark"></span>Marginal
        </span>
        <div className="spacer"></div>
        <div style={{ position: "relative" }}>
          <button className="icon-btn" title="Notifications" onClick={() => setShowNotifications((s) => !s)}>
            🔔{notifications.length > 0 && <span className="badge">{notifications.length}</span>}
          </button>
          {showNotifications && (
            <div className="float" style={{ position: "absolute", right: 0, top: 40, width: 300, maxHeight: 360, overflowY: "auto", padding: 10, zIndex: 30 }}>
              <div className="panel-h">Notifications</div>
              {notifications.length === 0 && <div className="muted" style={{ padding: 8 }}>Nothing yet.</div>}
              {notifications.map((n) => (
                <div className="row" key={n.id} style={{ cursor: "default" }}>
                  <div>
                    <div>{n.message}</div>
                    <div className="muted" style={{ fontSize: 11 }}>{new Date(n.created_at).toLocaleString()}</div>
                  </div>
                </div>
              ))}
            </div>
          )}
        </div>
        <Link className="btn" to="/graph">Graph</Link>
        <button className="btn" onClick={logout}>Sign out</button>
      </header>

      <main className="canvas" style={{ maxWidth: 980, margin: "0 auto", width: "100%", padding: "32px 24px" }}>
        <h1 style={{ fontFamily: "var(--display)", fontWeight: 560, marginBottom: 4 }}>Your pages</h1>
        <p className="muted" style={{ marginTop: 0, marginBottom: 24 }}>
          Start a blank page, or open one you already have. Anyone who opens the same page's link and signs
          in edits it live with you — see the editor's "Copy link" button.
        </p>

        <div className="grid c3" style={{ marginBottom: 32 }}>
          <a href="#" className="card link" onClick={handleCreateBlank} aria-disabled={creating}>
            <div className="glyph teal" aria-hidden>+</div>
            <div className="card-h">{creating ? "Creating…" : "Blank page"}</div>
            <p>Start with an empty page and give it a title as you write.</p>
          </a>

          {sorted.map((p) => (
            <a key={p.id} href={`/pages/${p.id}`} className="card link" onClick={(e) => { e.preventDefault(); navigate(`/pages/${p.id}`); }}>
              <div className="glyph amber" aria-hidden>📄</div>
              <div className="card-h">{p.title || "Untitled"}</div>
              <p>Updated {new Date(p.updated_at).toLocaleString()}</p>
            </a>
          ))}
        </div>

        {sorted.length === 0 && (
          <div className="muted">No pages yet — start with "Blank page" above.</div>
        )}
      </main>
    </div>
  );
}
