import { useEffect, useState } from "react";
import { Link } from "react-router-dom";
import { getBacklinks, type Backlink, type Page } from "../api/pages";
import type { Diagnostic } from "../api/diagnostics";
import type { CollabPage } from "../collab/useCollabPage";
import { keyOf } from "../collab/blockKind";

type Tab = "outline" | "checks" | "links" | "comments" | "people" | "history";

// § 04's four, in its order. The mockup's own note applies: seven candidate
// tabs is already too many to show at once, and a strip that overflows its
// own 296px panel reads as a rendering bug. Comments and History are still
// reachable — Comments has no service in scope, and History is its own
// route — so dropping them from the strip loses nothing but the clipping.
const TABS: { id: Tab; label: string }[] = [
  { id: "outline", label: "OUTLINE" },
  { id: "checks", label: "CHECKS" },
  { id: "links", label: "LINKS" },
  { id: "people", label: "PRESENCE" },
];

/**
 * The right-side inspector — editor.html's tab rail. Outline, People, and
 * Backlinks are real: Outline/People compute straight from this page's
 * own live state (the block list, and real presence — who's actually
 * connected right now, not who has happened to edit);
 * Backlinks reads document-service's docs.page_links
 * (internal/blockproj's projection) via GET /pages/{id}/backlinks.
 * Checks is real too (v2.3.0) — every RFC-003 §2 analyzer, run fresh by
 * diagnostics-service and read via GET /pages/{id}/diagnostics; nothing
 * here re-derives a diagnostic in TypeScript (ADR-012). History is real
 * too (v2.4.0) — this tab is a launcher into HistoryScreen/TraceScreen/
 * DiffScreen (each its own full screen, matching Graph/Facts' own
 * precedent), not a second, cramped copy of them. Comments still needs
 * a service this repo doesn't have in scope (Track 2) — that tab says
 * so plainly rather than showing invented data.
 */
export function InspectorRail({
  page,
  actorId,
  collab,
  diagnostics,
  diagnosticsError,
  onRefreshDiagnostics,
}: {
  page: Page;
  actorId: string;
  collab: CollabPage;
  /** Lifted to EditorScreen (v2.3.0) — the same diagnostics run also
   * drives RichEditorPane's left-gutter markers, so it's fetched once,
   * not once per consumer (the same reasoning EditorScreen's own doc
   * comment already gives for lifting useCollabPage itself). */
  diagnostics: Diagnostic[] | null;
  diagnosticsError: string | null;
  onRefreshDiagnostics: () => void;
}) {
  const [tab, setTab] = useState<Tab>("outline");

  return (
    <aside className="rail right">
      <div className="tabs" role="tablist">
        {TABS.map((t) => (
          <button
            key={t.id}
            className="tab"
            role="tab"
            aria-selected={tab === t.id}
            onClick={() => setTab(t.id)}
          >
            {t.label}
          </button>
        ))}
      </div>
      <div className="panel-body">
        {tab === "outline" && <OutlinePanel page={page} collab={collab} />}
        {tab === "people" && <PeoplePanel collab={collab} />}
        {tab === "checks" && (
          <ChecksPanel diagnostics={diagnostics} error={diagnosticsError} onRefresh={onRefreshDiagnostics} />
        )}
        {tab === "links" && <BacklinksPanel page={page} actorId={actorId} />}
        {tab === "comments" && <NotBuiltPanel what="Comments" service="a comments feature (Track 2)" />}
        {tab === "history" && <HistoryPanel page={page} />}
      </div>
    </aside>
  );
}

function OutlinePanel({ page, collab }: { page: Page; collab: CollabPage }) {
  const headings = collab.blocks
    .map((b) => ({ b, key: keyOf(b.kind) }))
    .filter((x) => x.key === "heading1" || x.key === "heading2" || x.key === "heading3");

  return (
    <section className="tabpanel">
      <div className="panel-h">Document structure</div>
      <div className="row">
        <span className="lead">h1</span>
        {page.title || "Untitled"}
      </div>
      {headings.length === 0 && (
        <div className="muted" style={{ padding: "8px 0", fontSize: 12.5 }}>No headings yet.</div>
      )}
      {headings.map(({ b, key }) => (
        <div className="row" key={b.id} style={{ paddingLeft: key === "heading1" ? 0 : 14 }}>
          <span className="lead">{key === "heading1" ? "h1" : key === "heading2" ? "h2" : "h3"}</span>
          {b.text || "Untitled heading"}
        </div>
      ))}
    </section>
  );
}

function PeoplePanel({ collab }: { collab: CollabPage }) {
  return (
    <section className="tabpanel">
      <div className="panel-h">On this page</div>
      <div className="row">
        <span className="lead" style={{ color: "var(--teal)" }}>●</span>
        You
        <span className="muted" style={{ marginLeft: "auto" }}>here</span>
      </div>
      {[...collab.peers].map((p) => (
        <div className="row" key={p}>
          <span className="lead" style={{ color: "var(--violet)" }}>●</span>
          {p.slice(0, 8)}
          <span className="muted" style={{ marginLeft: "auto" }}>here</span>
        </div>
      ))}
      {collab.peers.size === 0 && (
        <div className="muted" style={{ padding: "8px 0", fontSize: 12.5 }}>
          Nobody else is here right now — "Copy link" from the toolbar and open it as a second person to see them here live.
        </div>
      )}
    </section>
  );
}

// RFC-003 §2's analyzer table, for the "Passed" section below — an
// analyzer diagnostics-service didn't report anything for on this page
// is real "passed" information (cross-referencing the fixed registry
// against what an actual analysis run actually returned), not a guess.
const ANALYZERS: { id: string; label: string }[] = [
  { id: "DanglingPageLink", label: "Every [[page link]] resolves" },
  { id: "AmbiguousPageLink", label: "No [[page link]] matches more than one title" },
  { id: "SelfLink", label: "No page links to itself" },
  { id: "LinkCycle", label: "Not part of a link cycle" },
  { id: "HeadingSkip", label: "No heading level is skipped" },
  { id: "EmptyCodeBlock", label: "Every code block has content" },
  { id: "DuplicateTitle", label: "Title is unique in this workspace" },
  { id: "OrphanPage", label: "Page is reachable from another page" },
  { id: "BrokenImage", label: "Every image reference is set" },
];

/**
 * Real diagnostics (v2.3.0) — every RFC-003 §2 analyzer, run fresh by
 * diagnostics-service and read via GET /pages/{id}/diagnostics. Severity
 * drives presentation (RFC-003 §2): "warning" gets the solid stripe,
 * "hint"/"info" the faint one — never red, nothing here is a compile
 * error. The Passed section lists analyzers this run reported nothing
 * for, so a clean page still shows real work having happened. The fetch
 * itself lives in EditorScreen (see InspectorRail's own prop doc) since
 * RichEditorPane's gutter markers need the same result.
 */
function ChecksPanel({
  diagnostics,
  error,
  onRefresh,
}: {
  diagnostics: Diagnostic[] | null;
  error: string | null;
  onRefresh: () => void;
}) {
  const found = new Set(diagnostics?.map((d) => d.analyzer));
  const passed = ANALYZERS.filter((a) => diagnostics && !found.has(a.id));

  return (
    <section className="tabpanel">
      <div className="panel-h" style={{ display: "flex", alignItems: "center" }}>
        Checks{diagnostics ? ` · ${diagnostics.length}` : ""}
        <button className="icon-btn" style={{ marginLeft: "auto" }} title="Re-run" onClick={onRefresh}>
          ↻
        </button>
      </div>
      {error && <div className="muted" style={{ padding: "8px 0", fontSize: 12.5 }}>{error}</div>}
      {diagnostics?.map((d, i) => (
        <div className={`check ${d.severity === "warning" ? "warn" : "hint"}`} key={`${d.analyzer}-${i}`}>
          <span className="stripe"></span>
          <div>
            <div className="msg">{d.message}</div>
            <div className="meta">{d.analyzer}{d.block_id ? ` · block ${d.block_id.slice(0, 8)}` : ""}</div>
          </div>
        </div>
      ))}
      {passed.length > 0 && (
        <div className="panel-section">
          <div className="panel-h">Passed · {passed.length}</div>
          {passed.map((a) => (
            <div className="check ok" key={a.id}>
              <span className="stripe"></span>
              <div className="msg">{a.label}</div>
            </div>
          ))}
        </div>
      )}
    </section>
  );
}

/**
 * Real backlinks — internal/blockproj's projection of collab.ops_flushed
 * (docs/porting/PROGRESS.md), read via GET /pages/{id}/backlinks. Fetched
 * on mount/page-change and again on a manual refresh — not live-reactive
 * to just-typed [[links]], since materialization runs async (the outbox
 * poller + NATS, roughly sub-second in practice, not instant).
 */
function BacklinksPanel({ page, actorId }: { page: Page; actorId: string }) {
  const [backlinks, setBacklinks] = useState<Backlink[] | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [refreshKey, setRefreshKey] = useState(0);

  useEffect(() => {
    let cancelled = false;
    setBacklinks(null);
    setError(null);
    getBacklinks(actorId, page.id)
      .then((r) => {
        if (!cancelled) setBacklinks(r.backlinks);
      })
      .catch(() => {
        if (!cancelled) setError("Couldn't load backlinks.");
      });
    return () => {
      cancelled = true;
    };
  }, [page.id, actorId, refreshKey]);

  return (
    <section className="tabpanel">
      <div className="panel-h" style={{ display: "flex", alignItems: "center" }}>
        Linked from{backlinks ? ` · ${backlinks.length}` : ""}
        <button className="icon-btn" style={{ marginLeft: "auto" }} title="Refresh" onClick={() => setRefreshKey((k) => k + 1)}>
          ↻
        </button>
      </div>
      {error && <div className="muted" style={{ padding: "8px 0", fontSize: 12.5 }}>{error}</div>}
      {backlinks && backlinks.length === 0 && !error && (
        <div className="muted" style={{ padding: "8px 0", fontSize: 12.5 }}>
          No other page links here yet — write <code>[[{page.title || "this page's title"}]]</code> on another page.
        </div>
      )}
      {backlinks?.map((b, i) => (
        <div className="row" key={`${b.from_page}-${i}`}>
          <span className="lead">→</span>
          {b.from_page_title || "Untitled"}
          {b.from_page_deleted && <span className="pill" style={{ marginLeft: "auto" }}>deleted</span>}
        </div>
      ))}
    </section>
  );
}

/**
 * Real history (v2.4.0) — a launcher, not a cramped re-implementation:
 * the scrubber/restore/palimpsest, the op-log debugger, and the revision
 * diff each need enough screen real estate that they're their own
 * screens (HistoryScreen/TraceScreen/DiffScreen), same reasoning
 * Graph/Facts already followed rather than trying to fit a force layout
 * or a fact DAG into this rail.
 */
function HistoryPanel({ page }: { page: Page }) {
  return (
    <section className="tabpanel">
      <div className="panel-h">History</div>
      <Link className="row" to={`/pages/${page.id}/history`}>
        <span className="lead">⏱</span>
        Scrubber &amp; restore
      </Link>
      <Link className="row" to={`/pages/${page.id}/trace`}>
        <span className="lead">◉</span>
        Op-log debugger
      </Link>
      <Link className="row" to={`/pages/${page.id}/diff`}>
        <span className="lead">±</span>
        Revision diff
      </Link>
    </section>
  );
}

function NotBuiltPanel({ what, service }: { what: string; service: string }) {
  return (
    <section className="tabpanel">
      <div className="panel-h">{what}</div>
      <div className="muted" style={{ fontSize: 12.5, lineHeight: 1.6 }}>
        Not built — {what.toLowerCase()} needs {service}, which is out of scope for this repo
        (<code>ADR-011</code>: deferred past the Rust port). This tab exists so the chrome matches{" "}
        <code>editor.html</code>, not to fake data.
      </div>
    </section>
  );
}
