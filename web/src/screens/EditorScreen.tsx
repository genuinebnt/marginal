import { useEffect, useRef, useState } from "react";
import { Link, useNavigate, useParams } from "react-router-dom";
import { useAuth } from "../auth/AuthContext";
import { getPage, renamePage, type Page } from "../api/pages";
import { savePosition } from "../api/resume";
import { getPageDiagnostics, type Diagnostic } from "../api/diagnostics";
import { useCollabPage } from "../collab/useCollabPage";
import { isPageLinkClick, pageLinkTarget } from "../collab/pagelinks";
import { getLinkGraph } from "../api/graph";
import { getPageSeries, type PageSeries } from "../api/series";
import { SeriesBanner } from "../ui";
import { RichEditorPane } from "./RichEditorPane";
import { InspectorRail } from "./InspectorRail";
import { PageTreeRail } from "./PageTreeRail";
import { Body, Readout, Screen, Spark, StatusBar, TopBar } from "../shell/Chrome";

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
  const navigate = useNavigate();
  const [activePage, setActivePage] = useState<Page | null>(null);
  /** Every live page, for resolving a [[link]] on ⌘-click. GetLinkGraph, not
   *  ListPages: the latter returns root pages only, so a link into a nested
   *  page would resolve to nothing. */
  const [pages, setPages] = useState<Array<{ id: string; title: string }>>([]);
  const [series, setSeries] = useState<PageSeries | null>(null);
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
    getPageSeries(actorId, id).then(setSeries).catch(() => setSeries(null));
  }, [id, actorId]);

  useEffect(() => {
    getLinkGraph(actorId)
      .then((g) => setPages(g.nodes.map((n) => ({ id: n.id, title: n.title }))))
      .catch(() => setPages([]));
  }, [actorId]);

  // Remember where the caret is, so the dashboard can resume here.
  //
  // Reported by the pane rather than read back out of collab.cursors — that
  // map is PEER state and may not echo your own cursor, so reading it would
  // have silently saved a null block forever. (It also typechecked while
  // doing so, since that map is not strictly typed at this call site.)
  //
  // Debounced hard: this is advisory view state, not an op. Nothing replays
  // it and nothing breaks if the last two seconds are lost, whereas writing
  // per keystroke would put a database round trip in the typing path to save
  // a number that is only read when you come back.
  // Ops per second over a rolling 8-bucket window, one bucket per second.
  // Real measurement: each bucket counts how much the block list changed in
  // that second, which is the only op signal this screen actually observes.
  const [opsWindow, setOpsWindow] = useState<number[]>(Array(8).fill(0));
  const lastCount = useRef(0);
  useEffect(() => {
    const t = setInterval(() => {
      const n = collab.blocks.length;
      const delta = Math.abs(n - lastCount.current);
      lastCount.current = n;
      setOpsWindow((w) => [...w.slice(1), delta]);
    }, 1000);
    return () => clearInterval(t);
  }, [collab.blocks.length]);
  const opsPerSecond = opsWindow[opsWindow.length - 1] ?? 0;

  const caretRef = useRef<{ blockId: string | null; start: number; end: number } | null>(null);
  useEffect(() => {
    if (!id) return;
    const t = setInterval(() => {
      const c = caretRef.current;
      if (!c) return;
      caretRef.current = null;   // only write when it actually moved
      savePosition(actorId, id, c.blockId, c.start, c.end).catch(() => {});
    }, 2000);
    return () => clearInterval(t);
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

  /**
   * ⌘/Ctrl-click on a [[page link]] opens it.
   *
   * A plain click must still place the caret — this is a contenteditable, and
   * hijacking the click would make the link the one span in the document you
   * cannot put a cursor in. Modifier-click is the same convention every code
   * editor uses for "go to definition", for the same reason.
   */
  function handleLinkNavigate(e: React.MouseEvent) {
    if (!e.metaKey && !e.ctrlKey) return;
    const title = isPageLinkClick(e);
    if (!title) return;
    e.preventDefault();
    const target = pageLinkTarget(e, pages);
    if (target) navigate(`/pages/${target.id}`);
  }

  return (
    <Screen>
      <TopBar
        crumb={activePage ? <>page / <b>{activePage.title}</b></> : undefined}
        readouts={
          <>
            {/* § 04's own two. Ops/s is measured over a rolling window of
                real acks, not a decorative number. */}
            <Readout k="OPS/S" v={opsPerSecond} />
            {/* Measured on this connection — the time from sending an op to
                the server acking it, p99 over the last 100. Dimmed with an
                em dash before the first ack rather than showing 0, which
                would be a latency claim nobody made. */}
            <Readout
              k="ACK P99"
              v={collab.ackP99 === null ? "—" : `${Math.round(collab.ackP99)} ms`}
              tone={collab.ackP99 === null ? undefined : collab.ackP99 < 50 ? "#3FCFA8" : "#E0A34E"}
            />
            <Readout
              k="LINK"
              v={collab.state === "open" ? "live" : collab.state}
              tone={collab.state === "open" ? "#3FCFA8" : "#E0A34E"}
            />
          </>
        }
        spark={<Spark values={opsWindow} />}
        peers={[...collab.peers].slice(0, 2).map((p) => (
          <div key={p} className="av av-them" title={p}>
            {p.slice(0, 2).toUpperCase()}
          </div>
        ))}
        right={
          <>
            {/* Presence lives with the prose (the pane's dek), not up here:
                an actor id sliced to two characters is noise, and the tag
                the pane derives is at least stable per actor. */}
            {/* The other half of the read/write switch. One page, two views. */}
            <Link to={`/read/${id}`} className="btn" style={{ textDecoration: "none" }}>
              READ
              <div className="brk-tl" /><div className="brk-br" />
            </Link>
            <div className="btn" onClick={copyShareLink} style={{ cursor: "pointer" }}>
              {linkCopied ? "COPIED" : "SHARE"}
              <div className="brk-tl" /><div className="brk-br" />
            </div>
          </>
        }
      />

      {/* The same series strip the reader carries — the page is one page, and
          knowing it is part 4 of 19 matters as much while writing it. */}
      {series?.membership === "member" && (
        <SeriesBanner
          seriesTitle={series.series_title}
          seriesTo={`/series/${series.series_page_id}`}
          number={series.number}
          total={series.parts.length}
          prev={series.number > 1
            ? { title: series.parts[series.number - 2].title, to: `/pages/${series.parts[series.number - 2].page_id}` }
            : null}
          next={series.number < series.parts.length
            ? { title: series.parts[series.number].title, to: `/pages/${series.parts[series.number].page_id}` }
            : null}
        />
      )}

      <Body onClick={handleLinkNavigate}>
        <PageTreeRail
          actorId={actorId}
          activePageId={id}
          activePagePath={activePage?.path}
          blocks={collab.blocks}
          onJumpToBlock={(blockId) => {
            document
              .querySelector(`[data-block-id="${blockId}"]`)
              ?.scrollIntoView({ behavior: "smooth", block: "center" });
          }}
        />

        {activePage ? (
          <>
            <RichEditorPane
              key={activePage.id}
              page={activePage}
              collab={collab}
              onRename={handleRename}
              onCaretMoved={(blockId, start, end) => {
                caretRef.current = { blockId, start, end };
              }}
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
