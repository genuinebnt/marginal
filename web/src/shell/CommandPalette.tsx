/**
 * docs/ui-mockups/v2/index.html § 24b COMMAND PALETTE, ported.
 *
 * ⌘K — one input that resolves to a page, an action, or a query. The chip
 * has been drawn in every top bar since the first screen and did nothing;
 * this is the thing it was drawing.
 *
 * THE SECTION'S OWN ARGUMENT, which decides the shape: sources are LABELLED
 * and ranked against each other rather than concatenated. A palette that puts
 * every page above every action is a page-switcher wearing a palette's
 * clothes. So: best match, then actions (verbs first, always), then pages by
 * rank, then the query reading, then recents.
 *
 * And the query is PARSED, never guessed at. `topic:storage tag:rope` is
 * offered as a filter expression you can run with a second, explicit
 * keystroke — the palette never silently decides you meant a filter.
 */
import {
  useCallback, useEffect, useMemo, useRef, useState, type ReactNode,
} from "react";
import { useLocation, useNavigate } from "react-router-dom";
import { useAuth } from "../auth/AuthContext";
import { search, type SearchHit } from "../api/search";
import { getLinkGraph } from "../api/graph";
import { createPage } from "../api/pages";

/** One row. `run` is what ↵ does; everything else is presentation. */
interface Item {
  id: string;
  icon: string;
  iconColor?: string;
  label: ReactNode;
  meta?: ReactNode;
  run: () => void | Promise<void>;
  /** Drawn dimmed and not runnable — a destination this repo has not built.
   *  § 9.4: mark what is missing rather than omitting it. */
  disabled?: boolean;
}

interface Section {
  title: string;
  hint: string;
  items: Item[];
}

/**
 * The filter grammar the palette understands, and nothing more.
 *
 * Deliberately tiny: `topic:`, `tag:` and bare words. Every token it does not
 * recognise stays a bare word rather than becoming an error, because a
 * palette that rejects your typing is a palette you stop opening.
 */
export function parseQuery(q: string): { topics: string[]; tags: string[]; words: string[] } {
  const topics: string[] = [], tags: string[] = [], words: string[] = [];
  for (const tok of q.split(/\s+/).filter(Boolean)) {
    const m = /^(topic|tag):(.+)$/i.exec(tok);
    if (!m) { words.push(tok); continue; }
    (m[1].toLowerCase() === "topic" ? topics : tags).push(m[2].toLowerCase());
  }
  return { topics, tags, words };
}

/** Where the palette has been, this session. Module-level rather than
 *  localStorage: § 24b says "this session", and a recents list that outlives
 *  the session is a history, which is a different feature. */
const recents: Array<{ id: string; title: string; at: number }> = [];

export function rememberVisit(id: string, title: string) {
  const at = Date.now();
  const i = recents.findIndex((r) => r.id === id);
  if (i >= 0) recents.splice(i, 1);
  recents.unshift({ id, title, at });
  recents.length = Math.min(recents.length, 8);
}

function ago(at: number): string {
  const mins = Math.round((Date.now() - at) / 60000);
  if (mins < 1) return "just now";
  if (mins < 60) return `${mins} min ago`;
  return `${Math.round(mins / 60)}h ago`;
}

export function CommandPalette({ onClose }: { onClose: () => void }) {
  const { session } = useAuth();
  const actorId = session?.actorId ?? null;
  const navigate = useNavigate();
  const { pathname } = useLocation();
  const inputRef = useRef<HTMLInputElement>(null);

  const [q, setQ] = useState("");
  const [pages, setPages] = useState<Array<{ id: string; title: string }>>([]);
  const [hits, setHits] = useState<SearchHit[]>([]);
  const [ms, setMs] = useState<number | null>(null);
  const [cursor, setCursor] = useState(0);

  useEffect(() => { inputRef.current?.focus(); }, []);

  useEffect(() => {
    if (!actorId) return;
    getLinkGraph(actorId)
      .then((g) => setPages(g.nodes.map((n) => ({ id: n.id, title: n.title }))))
      .catch(() => setPages([]));
  }, [actorId]);

  // Debounced, because this is a keystroke path. 140ms is under the point
  // where the list feels like it is lagging behind the caret.
  useEffect(() => {
    const words = parseQuery(q).words.join(" ").trim();
    if (!actorId || words.length < 2) { setHits([]); setMs(null); return; }
    const started = performance.now();
    const t = setTimeout(() => {
      search(actorId, words)
        .then((r) => { setHits(r.hits); setMs(Math.round(performance.now() - started)); })
        .catch(() => { setHits([]); setMs(null); });
    }, 140);
    return () => clearTimeout(t);
  }, [q, actorId]);

  const parsed = useMemo(() => parseQuery(q), [q]);
  const words = parsed.words.join(" ").trim();
  const openPage = useCallback((id: string) => { navigate(`/read/${id}`); onClose(); }, [navigate, onClose]);

  const sections: Section[] = useMemo(() => {
    const out: Section[] = [];
    const lower = words.toLowerCase();

    // BEST MATCH — an exact title, which is the one case where a page
    // legitimately outranks every verb.
    const exact = lower ? pages.find((p) => p.title.toLowerCase() === lower) : undefined;
    if (exact) {
      out.push({
        title: "BEST MATCH", hint: "exact title",
        items: [{
          id: `exact:${exact.id}`, icon: "◆", label: exact.title,
          meta: "page · ↵ open", run: () => openPage(exact.id),
        }],
      });
    }

    // ACTIONS — verbs first, always. Only real ones; a verb the app cannot
    // perform is drawn dimmed rather than offered.
    const actions: Item[] = [];
    if (words) {
      actions.push({
        id: "create", icon: "✎",
        label: <>Create page “{words}”</>, meta: "⇧↵",
        run: async () => {
          if (!actorId) return;
          const p = await createPage(actorId, words);
          navigate(`/pages/${p.id}`);
          onClose();
        },
      });
      actions.push({
        id: "search", icon: "⌕", iconColor: "#5AC8B4",
        label: <>Search for “{words}”</>, meta: `${hits.length} hits · ⌥↵`,
        run: () => { navigate(`/search?q=${encodeURIComponent(words)}`); onClose(); },
      });
    }
    for (const [label, to, icon] of [
      ["Go to the graph", "/graph", "◉"],
      ["Go to series", "/series", "◫"],
      ["Go to discover", "/discover", "✧"],
      ["Go to the inbox", "/notifications", "◎"],
    ] as const) {
      if (!words || label.toLowerCase().includes(lower)) {
        actions.push({ id: to, icon, label, meta: to, run: () => { navigate(to); onClose(); } });
      }
    }
    if (/^\/pages\//.test(pathname)) {
      const id = pathname.split("/")[2];
      actions.push({
        id: "history", icon: "◷", label: "Show this page's history", meta: "history",
        run: () => { navigate(`/pages/${id}/history`); onClose(); },
      });
    }
    if (actions.length) out.push({ title: "ACTIONS", hint: "verbs first, always", items: actions });

    // PAGES — real FTS rank, not a title substring match.
    if (hits.length) {
      out.push({
        title: `PAGES · ${hits.length}`, hint: "Postgres FTS rank",
        items: hits.slice(0, 6).map((h) => ({
          id: `hit:${h.page_id}:${h.block_id ?? ""}`,
          icon: "·",
          label: h.page_title,
          meta: h.rank.toFixed(2),
          run: () => openPage(h.page_id),
        })),
      });
    }

    // UNDERSTOOD AS A QUERY — parsed, never guessed at. Running it is always
    // an explicit second keystroke.
    if (parsed.topics.length || parsed.tags.length) {
      const expr = [
        ...parsed.topics.map((t) => `topic:${t}`),
        ...parsed.tags.map((t) => `tag:${t}`),
        ...parsed.words,
      ].join(" ");
      out.push({
        title: "UNDERSTOOD AS A QUERY", hint: "parsed, not guessed",
        items: [{
          id: "query", icon: "⌕", iconColor: "#5AC8B4",
          label: <span className="mono" style={{ fontSize: 11.5, color: "#C3BFB7" }}>{expr}</span>,
          meta: "⌥↵",
          run: () => {
            const u = new URLSearchParams();
            if (parsed.words.length) u.set("q", parsed.words.join(" "));
            if (parsed.tags.length) u.set("tags", parsed.tags.join(","));
            if (parsed.topics.length) u.set("topics", parsed.topics.join(","));
            navigate(`/search?${u}`);
            onClose();
          },
        }, {
          id: "ask", icon: "✦", iconColor: "#7D9EC9",
          label: <>Ask: “{words || q}”</>,
          meta: "no assistant in this repo",
          run: () => {},
          disabled: true,
        }],
      });
    }

    if (!words && recents.length) {
      out.push({
        title: "RECENT", hint: "this session",
        items: recents.map((r) => ({
          id: `recent:${r.id}`, icon: "↺", label: r.title, meta: ago(r.at),
          run: () => openPage(r.id),
        })),
      });
    }
    return out;
  }, [words, q, pages, hits, parsed, actorId, navigate, onClose, openPage, pathname]);

  const flat = useMemo(() => sections.flatMap((s) => s.items.filter((i) => !i.disabled)), [sections]);
  useEffect(() => { setCursor(0); }, [q]);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") { onClose(); return; }
      if (e.key === "ArrowDown") { e.preventDefault(); setCursor((c) => Math.min(c + 1, flat.length - 1)); }
      if (e.key === "ArrowUp") { e.preventDefault(); setCursor((c) => Math.max(c - 1, 0)); }
      if (e.key === "Enter") {
        e.preventDefault();
        void flat[cursor]?.run();
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [flat, cursor, onClose]);

  let index = -1;
  return (
    <>
      {/* The screen behind stays exactly where it was: the palette is an
          overlay, not a route, so escaping returns you to your caret. */}
      <div className="scrim" onClick={onClose} />
      <div className="pal">
        <div className="pal-q">
          <span style={{ color: "#E8873C", fontSize: 14 }}>⌘</span>
          <input
            ref={inputRef}
            value={q}
            onChange={(e) => setQ(e.target.value)}
            placeholder="page, action, or topic: tag: query…"
            aria-label="Command palette"
          />
          <span className="mono" style={{ fontSize: 10, color: "#585550" }}>
            {sections.length} source{sections.length === 1 ? "" : "s"}
            {ms !== null ? ` · ${ms} ms` : ""}
          </span>
          <span className="kbd">esc</span>
        </div>

        <div style={{ maxHeight: 520, overflowY: "auto" }}>
          {sections.length === 0 && (
            <div style={{ padding: "18px 16px", fontSize: 12, lineHeight: 1.7, color: "#585550" }}>
              Type a page title, a verb, or a filter like{" "}
              <span className="mono" style={{ color: "#8C8880" }}>topic:storage tag:crdt</span>.
            </div>
          )}
          {sections.map((s) => (
            <div key={s.title}>
              <div className="pal-s">{s.title}<div /><span style={{ color: "#4B4842" }}>{s.hint}</span></div>
              {s.items.map((it) => {
                if (!it.disabled) index++;
                const on = !it.disabled && index === cursor;
                return (
                  <div
                    key={it.id}
                    className={`pi${on ? " pi-on" : ""}`}
                    style={it.disabled ? { opacity: 0.45 } : { cursor: "pointer" }}
                    onMouseEnter={() => { if (!it.disabled) setCursor(index); }}
                    onClick={() => { if (!it.disabled) void it.run(); }}
                  >
                    <span className="pi-i" style={it.iconColor ? { color: it.iconColor } : undefined}>{it.icon}</span>
                    <span style={{ flex: 1, minWidth: 0, overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
                      {it.label}
                    </span>
                    {it.meta && <span className="pi-m">{it.meta}</span>}
                  </div>
                );
              })}
            </div>
          ))}
        </div>

        <div style={{
          display: "flex", alignItems: "center", gap: 14, padding: "9px 16px",
          borderTop: "1px solid rgba(255,255,255,.08)", background: "#0F1011",
        }}>
          <span className="mono" style={{ fontSize: 10, color: "#585550" }}>
            <span className="kbd" style={{ padding: "1px 5px" }}>↑↓</span> move
          </span>
          <span className="mono" style={{ fontSize: 10, color: "#585550" }}>
            <span className="kbd" style={{ padding: "1px 5px" }}>↵</span> open
          </span>
          <div style={{ flex: 1 }} />
          <span className="mono" style={{ fontSize: 10, color: "#585550" }}>
            type <span style={{ color: "#8C8880" }}>topic:</span> or{" "}
            <span style={{ color: "#8C8880" }}>tag:</span> to filter
          </span>
        </div>
      </div>
    </>
  );
}
