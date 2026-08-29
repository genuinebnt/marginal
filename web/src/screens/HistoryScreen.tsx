/**
 * docs/ui-mockups/v2/index.html § 17 HISTORY, ported.
 *
 * A scrubber over the real op log, and the palimpsest: one block's actual
 * tombstoned character array, where every version is the filter
 * `ins <= v < del` over ONE array rather than a stored snapshot per revision.
 * That is the screen's central claim, and it is why STORED and LIVE are shown
 * side by side — the gap between them is the tombstones, and 0 copies is the
 * point.
 *
 * Restore is repeated undo through Trace's own precomputed inverses, never a
 * snapshot swap, which is what makes it a normal edit that itself undoes.
 */
import { useCallback, useEffect, useMemo, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { useAuth } from "../auth/AuthContext";
import {
  getPalimpsest, getTrace, type PalimpsestChar, type TraceStep,
} from "../api/history";
import { listPages, type Page } from "../api/pages";
import {
  Body, Inspector, Label, Readout, Rule, Screen, StatusBar, TopBar, num,
} from "../shell/Chrome";

/** Actor colours: you are teal, a peer violet, the assistant slate (§3.3). */
function actorHue(actorId: string, you: string | null): string {
  if (actorId === you) return "#3FCFA8";
  if (actorId.startsWith("assistant")) return "#7D9EC9";
  return "#A98CE8";
}

export function HistoryScreen() {
  const { id } = useParams();
  const { session } = useAuth();
  const actorId = session?.actorId ?? null;
  const navigate = useNavigate();

  const [pages, setPages] = useState<Page[]>([]);
  const [steps, setSteps] = useState<TraceStep[]>([]);
  const [at, setAt] = useState(0);
  const [blockId, setBlockId] = useState<string | null>(null);
  const [chars, setChars] = useState<PalimpsestChar[] | null>(null);
  const [actorFilter, setActorFilter] = useState<string | null>(null);
  const [err, setErr] = useState<string | null>(null);

  useEffect(() => {
    if (!actorId) return;
    listPages(actorId).then((r) => setPages(r.pages)).catch(() => {});
  }, [actorId]);

  const load = useCallback(() => {
    if (!id) return;
    getTrace(id)
      .then((r) => { setSteps(r.steps); setAt(Math.max(r.steps.length - 1, 0)); setErr(null); })
      .catch((e) => setErr(String(e.message ?? e)));
  }, [id]);

  useEffect(load, [load]);

  // The palimpsest is per BLOCK, not per page — a tombstoned array belongs to
  // one block's text. Defaults to the first block that has any.
  useEffect(() => {
    const head = steps[steps.length - 1]?.after;
    if (!head) return;
    const first = head.blocks.find((b) => b.text.length > 0) ?? head.blocks[0];
    if (first && !blockId) setBlockId(first.id);
  }, [steps, blockId]);

  useEffect(() => {
    if (!id || !blockId) { setChars(null); return; }
    getPalimpsest(id, blockId).then((r) => setChars(r.chars)).catch(() => setChars(null));
  }, [id, blockId]);

  /** Op counts per actor — the rail, and the scrubber's tick colours. */
  const actors = useMemo(() => {
    const m = new Map<string, number>();
    steps.forEach((s) => m.set(s.op.actor_id, (m.get(s.op.actor_id) ?? 0) + 1));
    return [...m].sort((a, b) => b[1] - a[1]);
  }, [steps]);

  const shownSteps = useMemo(
    () => (actorFilter ? steps.filter((s) => s.op.actor_id === actorFilter) : steps),
    [steps, actorFilter],
  );

  const stored = chars?.length ?? 0;
  const live = chars?.filter((c) => c.delete_step === undefined).length ?? 0;
  const tombstoned = stored - live;
  const head = steps[steps.length - 1]?.after ?? null;

  if (!id) {
    return (
      <Screen>
        <TopBar crumb={<>history</>} />
        <Body>
          <div className="rail">
            <div className="rail-h">PICK A PAGE<div /></div>
            <div style={{ display: "flex", flexDirection: "column", gap: 1, padding: "0 8px", overflowY: "auto" }}>
              {pages.map((p) => (
                <div key={p.id} className="tr" style={{ cursor: "pointer" }}
                     onClick={() => navigate(`/pages/${p.id}/history`)}>
                  <span className="tr-t">{p.title}</span>
                </div>
              ))}
            </div>
          </div>
          <div style={{ flex: 1, display: "grid", placeItems: "center", padding: 40 }}>
            <div style={{ maxWidth: 520, fontSize: 12.5, lineHeight: 1.7, color: "#585550" }}>
              History is per page, because an op log is. Every revision is a filter over one
              stored array rather than a snapshot, so scrubbing costs nothing to keep.
            </div>
          </div>
        </Body>
        <StatusBar route="/history" mechanism="op-log replay" state="no page selected" healthy />
      </Screen>
    );
  }

  return (
    <Screen>
      <TopBar
        crumb={<>page / <b>{head?.title ?? "…"}</b></>}
        readouts={
          <>
            <Readout k="STORED" v={`${num(stored)} chars`} />
            <Readout k="LIVE AT HEAD" v={num(live)} tone="#3FCFA8" />
            <Readout k="COPIES" v="0" />
          </>
        }
      />

      <Body>
        <div className="rail" style={{ width: 250 }}>
          <div className="rail-h">ACTORS<div /><span style={{ color: "#585550" }}>{actors.length}</span></div>
          <div style={{ display: "flex", flexDirection: "column", padding: "0 8px", gap: 1 }}>
            <div
              className={`tr${actorFilter === null ? " tr-on" : ""}`}
              style={{ cursor: "pointer" }}
              onClick={() => setActorFilter(null)}
            >
              {actorFilter === null && <i />}
              <div style={{ width: 6, height: 6, background: "#E8873C", flex: "none" }} />
              <span className="tr-t">All actors</span>
              <span className="tr-n">{num(steps.length)}</span>
            </div>
            {actors.map(([a, n]) => (
              <div
                key={a}
                className={`tr${actorFilter === a ? " tr-on" : ""}`}
                style={{ cursor: "pointer" }}
                onClick={() => setActorFilter(a)}
                title={a}
              >
                {actorFilter === a && <i />}
                <div style={{ width: 6, height: 6, background: actorHue(a, actorId), flex: "none" }} />
                <span className="tr-t">{a === actorId ? "You" : a.slice(0, 8)}</span>
                <span className="tr-n">{num(n)}</span>
              </div>
            ))}
          </div>
          <div className="wal">
            <Label>PER-ACTOR UNDO</Label>
            <div className="mono" style={{ fontSize: 10.5, lineHeight: 1.8, color: "#8C8880" }}>
              yours&nbsp;&nbsp;&nbsp;&nbsp;{num(actors.find(([a]) => a === actorId)?.[1] ?? 0)} ops<br />
              others&nbsp;&nbsp;&nbsp;not yours to pop
            </div>
          </div>
        </div>

        <div style={{ flex: 1, minWidth: 0, display: "flex", flexDirection: "column", overflow: "hidden" }}>
          <div style={{ padding: "24px 34px 18px", borderBottom: "1px solid rgba(255,255,255,.07)" }}>
            <Label>SCRUBBER · TICKS COLOURED BY ACTOR</Label>
            <div
              style={{ position: "relative", height: 34, cursor: steps.length ? "pointer" : "default" }}
              onClick={(e) => {
                if (!steps.length) return;
                const r = e.currentTarget.getBoundingClientRect();
                const pct = (e.clientX - r.left) / r.width;
                setAt(Math.max(0, Math.min(steps.length - 1, Math.round(pct * (steps.length - 1)))));
              }}
            >
              <div style={{ position: "absolute", left: 0, right: 0, top: 16, height: 2, background: "rgba(255,255,255,.09)" }} />
              <div style={{
                position: "absolute", left: 0, top: 16, height: 2, background: "#E8873C",
                width: steps.length > 1 ? `${(at / (steps.length - 1)) * 100}%` : "0%",
              }} />
              {steps.map((s, i) => {
                const pct = steps.length > 1 ? (i / (steps.length - 1)) * 100 : 0;
                const current = i === at;
                const dimmed = actorFilter !== null && s.op.actor_id !== actorFilter;
                return (
                  <div
                    key={s.op.id}
                    title={`rev ${i + 1}`}
                    style={{
                      position: "absolute", left: `${pct}%`,
                      top: current ? 4 : 8,
                      width: current ? 3 : 2,
                      height: current ? 26 : 18,
                      background: current
                        ? "#E8873C"
                        : dimmed
                          ? "rgba(255,255,255,.12)"
                          : actorHue(s.op.actor_id, actorId),
                    }}
                  />
                );
              })}
            </div>
            <div style={{ display: "flex", justifyContent: "space-between" }}>
              <span className="mono" style={{ fontSize: 9.5, color: "#4B4842" }}>rev 1</span>
              <span className="mono" style={{ fontSize: 9.5, color: "#E8873C" }}>
                rev {num(steps.length ? at + 1 : 0)} · viewing
              </span>
              <span className="mono" style={{ fontSize: 9.5, color: "#4B4842" }}>
                rev {num(steps.length)} head
              </span>
            </div>
          </div>

          <div style={{ padding: "26px 34px", flex: 1, minHeight: 0, overflowY: "auto" }}>
            <div style={{ display: "flex", alignItems: "baseline", gap: 12, marginBottom: 16 }}>
              <Label>PALIMPSEST — TOMBSTONES THE LIVE TEXT IS READ FROM</Label>
            </div>

            {chars === null && (
              <div style={{ fontSize: 12.5, color: "#585550", lineHeight: 1.7 }}>
                {err ? `◌ ${err}` : "Select a block to read its character history."}
              </div>
            )}

            {chars !== null && chars.length === 0 && (
              <div style={{ fontSize: 12.5, color: "#585550", lineHeight: 1.7, maxWidth: 560 }}>
                No character history. This block arrived with its text already in the
                <span className="mono" style={{ color: "#8C8880" }}> InsertBlock </span>
                op — pasted or seeded — so there is nothing to tombstone. Type into it and
                every keystroke becomes an insertion this array remembers.
              </div>
            )}

            {chars !== null && chars.length > 0 && (
              <div style={{ fontFamily: "Spectral,serif", fontSize: 18, lineHeight: 1.9, color: "#D2CFC8" }}>
                {chars.map((c, i) => {
                  const gone = c.delete_step !== undefined;
                  // A character inserted after the scrub point has not been
                  // typed yet at this revision, so it is not shown at all —
                  // that is the `ins <= v` half of the filter.
                  if (c.insert_step > at) return null;
                  const deletedByNow = gone && (c.delete_step as number) <= at;
                  return (
                    <span
                      key={i}
                      style={
                        deletedByNow
                          ? { color: "#585550", textDecoration: "line-through", textDecorationColor: "rgba(224,163,78,.5)" }
                          : gone
                            ? { background: "rgba(232,135,60,.12)" }
                            : undefined
                      }
                      title={deletedByNow ? `deleted at rev ${(c.delete_step as number) + 1}` : undefined}
                    >
                      {String.fromCodePoint(c.rune)}
                    </span>
                  );
                })}
              </div>
            )}

            <div style={{ marginTop: 24, display: "flex", gap: 28 }}>
              <Readout k="STORED" v={num(stored)} size={15} />
              <Readout k="LIVE" v={num(live)} size={15} />
              <Readout k="TOMBSTONED" v={num(tombstoned)} size={15}
                       tone={tombstoned > 0 ? "#E0A34E" : undefined} />
              <Readout k="FILTER AT REV" v={<>ins ≤ v &lt; del</>} size={15} />
            </div>
          </div>
        </div>

        <Inspector
          tabs={[{ id: "ops", label: "OP STREAM" }, { id: "blocks", label: "BLOCKS" }]}
          active="ops"
        >
          <Label>
            {actorFilter ? "OPS BY THIS ACTOR" : "ALL OPS"} · {num(shownSteps.length)}
          </Label>
          <div style={{ display: "flex", flexDirection: "column", gap: 6 }}>
            {shownSteps.slice(-14).map((s) => {
              const i = steps.indexOf(s);
              return (
                <div
                  key={s.op.id}
                  onClick={() => setAt(i)}
                  style={{ display: "flex", gap: 8, alignItems: "baseline", cursor: "pointer" }}
                >
                  <span style={{ width: 5, height: 5, flex: "none", background: actorHue(s.op.actor_id, actorId) }} />
                  <span className="mono" style={{ fontSize: 10, color: i === at ? "#E4E2DC" : "#8C8880" }}>
                    rev {i + 1}
                  </span>
                  <span className="mono" style={{ marginLeft: "auto", fontSize: 9.5, color: "#585550" }}>
                    {new Date(s.op.created_at).toLocaleTimeString("en-GB")}
                  </span>
                </div>
              );
            })}
          </div>

          <Rule />
          <Label>BLOCK</Label>
          <div style={{ display: "flex", flexDirection: "column", gap: 6 }}>
            {(head?.blocks ?? []).map((b) => (
              <div
                key={b.id}
                onClick={() => setBlockId(b.id)}
                style={{
                  fontSize: 11.5, cursor: "pointer",
                  color: b.id === blockId ? "#E4E2DC" : "#8C8880",
                  whiteSpace: "nowrap", overflow: "hidden", textOverflow: "ellipsis",
                }}
              >
                {b.text.slice(0, 44) || `(empty ${b.kind.tag})`}
              </div>
            ))}
          </div>

          <Rule />
          <div style={{ fontSize: 11, lineHeight: 1.6, color: "#585550" }}>
            A delete writes a version stamp and never removes, so every revision is the filter
            <span className="mono" style={{ color: "#8C8880" }}> ins ≤ v &lt; del </span>
            over one array. That is why COPIES reads 0 — scrubbing costs nothing to keep because
            nothing is kept.
          </div>
        </Inspector>
      </Body>

      <StatusBar
        route={`/pages/${id}/history`}
        mechanism="one tombstoned array · every revision is a filter"
        state={
          err ? "op log unavailable"
            : steps.length === 0 ? "no ops on this page"
            : `${num(steps.length)} revisions · ${num(tombstoned)} tombstoned`
        }
        healthy={!err}
      />
    </Screen>
  );
}

export default HistoryScreen;
