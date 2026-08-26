import { useEffect, useState } from "react";
import { getBacklinks, type Backlink, type Page } from "../api/pages";
import type { CollabPage } from "../collab/useCollabPage";
import { keyOf } from "../collab/blockKind";

type Tab = "outline" | "checks" | "links" | "comments" | "people" | "history";

const TABS: { id: Tab; label: string }[] = [
  { id: "outline", label: "Outline" },
  { id: "checks", label: "Checks" },
  { id: "links", label: "Backlinks" },
  { id: "comments", label: "Comments" },
  { id: "history", label: "History" },
  { id: "people", label: "People" },
];

/**
 * The right-side inspector — editor.html's tab rail. Outline, People, and
 * Backlinks are real: Outline/People compute straight from this page's
 * own live state (the block list, and real presence — who's actually
 * connected right now, not who has happened to edit);
 * Backlinks reads document-service's docs.page_links
 * (internal/blockproj's projection) via GET /pages/{id}/backlinks.
 * Checks/Comments/History need services this repo doesn't have in scope
 * (`diagnostics-service`, a comments feature, `history-service` —
 * `ADR-011` defers all of them past the Rust port) — those tabs say so
 * plainly rather than showing invented data.
 */
export function InspectorRail({ page, actorId, collab }: { page: Page; actorId: string; collab: CollabPage }) {
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
        {tab === "checks" && <NotBuiltPanel what="Diagnostics" service="diagnostics-service" />}
        {tab === "links" && <BacklinksPanel page={page} actorId={actorId} />}
        {tab === "comments" && <NotBuiltPanel what="Comments" service="a comments feature (Track 2)" />}
        {tab === "history" && <NotBuiltPanel what="Version history" service="history-service" />}
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
