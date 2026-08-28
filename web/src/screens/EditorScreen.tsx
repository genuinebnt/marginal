import { useEffect, useState } from "react";
import { useParams } from "react-router-dom";
import { useAuth } from "../auth/AuthContext";
import { getPage, renamePage, type Page } from "../api/pages";
import { getPageDiagnostics, type Diagnostic } from "../api/diagnostics";
import { useCollabPage } from "../collab/useCollabPage";
import { RichEditorPane } from "./RichEditorPane";
import { InspectorRail } from "./InspectorRail";
import { PageTreeRail } from "./PageTreeRail";
import { Body, Readout, Screen, StatusBar, TopBar } from "../shell/Chrome";

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
  const { session } = useAuth();
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

  // Live peers exclude you, so "2 actors live" counts everyone in the room.
  const live = collab.peers.size + 1;
  const unflushed = collab.state === "open" ? 0 : null;

  return (
    <Screen>
      <TopBar
        crumb={activePage ? <>page / <b>{activePage.title}</b></> : undefined}
        readouts={
          <>
            <Readout k="BLOCKS" v={collab.blocks.length} />
            <Readout
              k="LINK"
              v={collab.state === "open" ? "live" : collab.state}
              tone={collab.state === "open" ? "#3FCFA8" : "#E0A34E"}
            />
          </>
        }
        right={
          <>
            {/* Presence lives with the prose (the pane's dek), not up here:
                an actor id sliced to two characters is noise, and the tag
                the pane derives is at least stable per actor. */}
            <div className="btn" onClick={copyShareLink} style={{ cursor: "pointer" }}>
              {linkCopied ? "COPIED" : "SHARE"}
              <div className="brk-tl" /><div className="brk-br" />
            </div>
          </>
        }
      />

      <Body>
        <PageTreeRail actorId={actorId} activePageId={id} />

        {activePage ? (
          <>
            <RichEditorPane
              key={activePage.id}
              page={activePage}
              collab={collab}
              onRename={handleRename}
              diagnostics={diagnostics ?? undefined}
              actorId={actorId}
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
          <div style={{ flex: 1, display: "grid", placeItems: "center", color: "#585550", fontSize: 12 }}>
            Loading…
          </div>
        )}
      </Body>

      <StatusBar
        route={`/pages/${id ?? ""}`}
        mechanism="every change is an op · anchors, never offsets"
        state={
          collab.state === "open"
            ? `${live} live · ${unflushed ?? 0} unflushed`
            : `connection ${collab.state}`
        }
        healthy={collab.state === "open"}
      />
    </Screen>
  );
}
