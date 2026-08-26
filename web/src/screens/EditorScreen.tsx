import { useEffect, useState } from "react";
import { Link, useParams } from "react-router-dom";
import { useAuth } from "../auth/AuthContext";
import { getPage, renamePage, type Page } from "../api/pages";
import { useCollabPage } from "../collab/useCollabPage";
import { RichEditorPane } from "./RichEditorPane";
import { InspectorRail } from "./InspectorRail";
import { PageTreeRail } from "./PageTreeRail";

/**
 * The editor screen for one page: the real nested page tree (rail), the
 * rich block-editing canvas (RichEditorPane), and the inspector
 * (InspectorRail) — editor.html's three-column layout. Root-level page
 * creation also happens on DashboardScreen (`/pages`); the tree rail adds
 * sub-page creation, rename-by-drag-reparent, and delete.
 *
 * useCollabPage is called once, here, and its result handed to both
 * RichEditorPane and InspectorRail — each opening its own WebSocket to
 * the same page would double-connect for no reason and could observe
 * each other's own broadcasts inconsistently (each connection gets its
 * own subscriber id).
 */
export function EditorScreen() {
  const { id } = useParams();
  const { session, logout } = useAuth();
  const [activePage, setActivePage] = useState<Page | null>(null);
  const [linkCopied, setLinkCopied] = useState(false);

  if (!session) throw new Error("EditorScreen requires an authenticated session");
  const { actorId } = session;
  const collab = useCollabPage(id ?? "", actorId);

  useEffect(() => {
    if (!id) return;
    setActivePage(null);
    getPage(actorId, id).then(setActivePage).catch(() => setActivePage(null));
  }, [id, actorId]);

  function handleRename(title: string) {
    if (!id) return;
    renamePage(actorId, id, title)
      .then(setActivePage)
      .catch(() => {});
  }

  async function copyShareLink() {
    try {
      await navigator.clipboard.writeText(window.location.href);
    } catch {
      // Clipboard API can be unavailable (insecure context, permissions) —
      // fall back to a prompt so the link is still recoverable by hand.
      window.prompt("Copy this link:", window.location.href);
    }
    setLinkCopied(true);
    setTimeout(() => setLinkCopied(false), 2000);
  }

  return (
    <div className="app">
      <header className="topbar">
        <Link to="/pages" className="brand" style={{ textDecoration: "none" }}>
          <span className="mark"></span>Marginal
        </Link>
        <div className="spacer"></div>
        <button className="btn" onClick={copyShareLink} title="Copy this page's link so someone else can join">
          {linkCopied ? "Link copied" : "Copy link"}
        </button>
        <button className="btn" onClick={logout}>Sign out</button>
      </header>

      <div className="body-row">
        <PageTreeRail actorId={actorId} activePageId={id} />

        {activePage ? (
          <>
            <RichEditorPane key={activePage.id} page={activePage} collab={collab} onRename={handleRename} />
            <InspectorRail page={activePage} actorId={actorId} collab={collab} />
          </>
        ) : (
          <main className="canvas" style={{ display: "grid", placeItems: "center" }}>
            <div className="muted">Loading…</div>
          </main>
        )}
      </div>
    </div>
  );
}
