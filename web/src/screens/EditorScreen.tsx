import { useEffect, useState } from "react";
import { Link, useParams } from "react-router-dom";
import { useAuth } from "../auth/AuthContext";
import { getPage, renamePage, type Page } from "../api/pages";
import { getPageDiagnostics, type Diagnostic } from "../api/diagnostics";
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
  const [diagnostics, setDiagnostics] = useState<Diagnostic[] | null>(null);
  const [diagnosticsError, setDiagnosticsError] = useState<string | null>(null);
  const [diagnosticsRefreshKey, setDiagnosticsRefreshKey] = useState(0);

  if (!session) throw new Error("EditorScreen requires an authenticated session");
  const { actorId } = session;
  const collab = useCollabPage(id ?? "", actorId);

  useEffect(() => {
    if (!id) return;
    setActivePage(null);
    getPage(actorId, id).then(setActivePage).catch(() => setActivePage(null));
  }, [id, actorId]);

  // Fetched once here (v2.3.0), not once per consumer: RichEditorPane's
  // left-gutter markers and InspectorRail's Checks tab are two views of
  // the exact same AnalyzePage run, not two separate diagnostic passes.
  useEffect(() => {
    if (!id) return;
    let cancelled = false;
    setDiagnostics(null);
    setDiagnosticsError(null);
    getPageDiagnostics(actorId, id)
      .then((r) => {
        if (!cancelled) setDiagnostics(r.diagnostics);
      })
      .catch(() => {
        if (!cancelled) setDiagnosticsError("Couldn't run diagnostics.");
      });
    return () => {
      cancelled = true;
    };
  }, [id, actorId, diagnosticsRefreshKey]);

  // ⌘Z/Ctrl+Z and ⌘⇧Z/Ctrl+Y — real per-actor undo/redo (v2.1.0), not the
  // browser's own native contenteditable undo. Captured at the document
  // level (capture: true, before the focused contenteditable's own native
  // handling) and preventDefault'd unconditionally on a match: the browser's
  // native undo operates purely on local DOM history and has no idea this
  // page is a collaborative session, so letting it run instead would desync
  // the contenteditable's text from what the server (and every other actor)
  // thinks this block contains. RFC-002 §3's own undo/redo lives entirely
  // server-side (internal/session.Undo/Redo) — this just asks for it.
  useEffect(() => {
    function onKeyDown(e: KeyboardEvent) {
      const mod = e.metaKey || e.ctrlKey;
      if (!mod || e.key.toLowerCase() !== "z") return;
      e.preventDefault();
      if (e.shiftKey) collab.redo();
      else collab.undo();
    }
    document.addEventListener("keydown", onKeyDown, { capture: true });
    return () => document.removeEventListener("keydown", onKeyDown, { capture: true });
  }, [collab]);

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
        <button className="btn" onClick={collab.undo} title="Undo your last edit (⌘Z / Ctrl+Z)">
          ↶ Undo
        </button>
        <button className="btn" onClick={collab.redo} title="Redo (⌘⇧Z / Ctrl+Shift+Z)">
          ↷ Redo
        </button>
        <button className="btn" onClick={copyShareLink} title="Copy this page's link so someone else can join">
          {linkCopied ? "Link copied" : "Copy link"}
        </button>
        <button className="btn" onClick={logout}>Sign out</button>
      </header>

      <div className="body-row">
        <PageTreeRail actorId={actorId} activePageId={id} />

        {activePage ? (
          <>
            <RichEditorPane
              key={activePage.id}
              page={activePage}
              collab={collab}
              onRename={handleRename}
              diagnostics={diagnostics ?? undefined}
            />
            <InspectorRail
              page={activePage}
              actorId={actorId}
              collab={collab}
              diagnostics={diagnostics}
              diagnosticsError={diagnosticsError}
              onRefreshDiagnostics={() => setDiagnosticsRefreshKey((k) => k + 1)}
            />
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
