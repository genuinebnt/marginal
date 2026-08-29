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
  describeOp, getPalimpsest, getTrace, type PalimpsestChar, type TraceStep,
} from "../api/history";
import { useCollabPage } from "../collab/useCollabPage";
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

/** A short, stable two-character tag for a peer — never their id verbatim,
 *  which is 36 characters of noise in a 10.5px mono line. */
function actorTag(actorId: string): string {
  let hash = 0;
  for (let i = 0; i < actorId.length; i++) hash = (hash * 31 + actorId.charCodeAt(i)) | 0;
  const A = "ABCDEFGHIJKLMNOPQRSTUVWXYZ";
  return A[Math.abs(hash) % 26] + A[Math.abs(hash >> 5) % 26];
}

/** One label-value row in the inspector. Five of them in a column is the
 *  shape § 17 uses for "as stored", and five hand-written copies is five
 *  chances for the alignment to drift. */
function Stat({ label, value, tone }: { label: string; value: string; tone?: string }) {
  return (
    <div style={{ display: "flex", alignItems: "baseline", gap: 8, fontSize: 11.5, color: "#9B968D" }}>
      <span style={{ flex: 1 }}>{label}</span>
      <span className="mono" style={{ fontSize: 11, color: tone ?? "#E4E2DC" }}>{value}</span>
    </div>
  );
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
  const [inspTab, setInspTab] = useState<"ops" | "revisions">("ops");
  /** Which rendering of the same filtered array the middle column shows. */
  const [lens, setLens] = useState<"TEXT" | "PALIMPSEST">("PALIMPSEST");
  /** What the last action on this screen did, in words. Restore writes ops,
   *  so it is worth reporting rather than leaving the scrubber to imply it. */
  const [action, setAction] = useState<string | null>(null);

  // A live connection, only so RESTORE is a real op rather than a chip that
  // does nothing. Reading history needs no socket — that is HTTP — but
  // restoring is an EDIT, and it goes down the same path every other edit
  // takes so it acks, broadcasts and undoes like one.
  const collab = useCollabPage(id ?? "", actorId ?? "");

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

  /**
   * Revisions grouped into GESTURES.
   *
   * One user action is often several ops — a replace is a DeleteText plus an
   * InsertText, a paste is many — and RFC-002 §3 already ties them together
   * with `undo_group` so one ⌘Z reverts the whole thing. A revision list that
   * shows the ops instead of the gestures is a list where "I typed a word"
   * appears as four rows, which is why the REVISIONS tab folds them.
   *
   * Ops with no group are their own gesture of one, which is that field's own
   * documented meaning rather than a special case invented here.
   */
  const gestures = useMemo(() => {
    const out: Array<{ key: string; steps: TraceStep[]; firstIndex: number }> = [];
    steps.forEach((s, i) => {
      const g = s.op.undo_group ?? null;
      const last = out[out.length - 1];
      if (g && last && last.key === g) {
        last.steps.push(s);
        return;
      }
      out.push({ key: g ?? `solo:${s.op.id}`, steps: [s], firstIndex: i });
    });
    return out;
  }, [steps]);

  const stored = chars?.length ?? 0;
  const live = chars?.filter((c) => c.delete_step === undefined).length ?? 0;
  const tombstoned = stored - live;
  const head = steps[steps.length - 1]?.after ?? null;
  // Restoring to head is a no-op the server already treats as one
  // (collaboration.md §2.2), so the chip says so rather than firing.
  const canRestore = collab.ready && steps.length > 0 && at < steps.length - 1;

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
        right={
          // § 17's own two views of one block. TEXT is what the document says
          // at this revision; PALIMPSEST is what it is STORED as — the same
          // array with the tombstones shown. Two renderings of one filter,
          // never two stored copies, which is the screen's whole claim.
          <div style={{ display: "flex", gap: 6 }}>
            {(["TEXT", "PALIMPSEST"] as const).map((v) => (
              <span
                key={v}
                className={`chip${lens === v ? " chip-e" : ""}`}
                style={{ cursor: "pointer" }}
                onClick={() => setLens(v)}
              >
                {v}
              </span>
            ))}
          </div>
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
              <Label>
                {lens === "PALIMPSEST"
                  ? "PALIMPSEST — TOMBSTONES THE LIVE TEXT IS READ FROM"
                  : "TEXT AT THIS REVISION — THE SAME FILTER, TOMBSTONES HIDDEN"}
              </Label>
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
                  // TEXT is the same filter with the tombstones simply not
                  // drawn — one array, two renderings, never two copies.
                  if (lens === "TEXT" && deletedByNow) return null;
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
          tabs={[
            { id: "ops", label: "OP STREAM" },
            { id: "revisions", label: "REVISIONS" },
          ]}
          active={inspTab}
          onSelect={(t) => setInspTab(t as "ops" | "revisions")}
        >
          {inspTab === "ops" && (
            <>
              <Label>
                {actorFilter ? "OPS BY THIS ACTOR" : "ALL OPS"} · {num(shownSteps.length)}
              </Label>
              {/* One line per op, in the log's own order, coloured by actor —
                  and carrying WHAT the op did, not just when. A stream of
                  timestamps is a stream nobody reads twice. */}
              <div className="mono" style={{ fontSize: 10.5, lineHeight: 1.95, color: "#8C8880" }}>
                {shownSteps.length === 0 && (
                  <span style={{ color: "#585550" }}>No ops yet on this page.</span>
                )}
                {shownSteps.slice(-16).map((s) => {
                  const i = steps.indexOf(s);
                  const { kind, detail } = describeOp(s.op.op);
                  const hue = actorHue(s.op.actor_id, actorId);
                  return (
                    <div
                      key={s.op.id}
                      onClick={() => setAt(i)}
                      title={`rev ${i + 1} · ${new Date(s.op.created_at).toLocaleTimeString("en-GB")}`}
                      style={{
                        cursor: "pointer", display: "flex", gap: 7,
                        color: i === at ? "#E4E2DC" : undefined,
                        background: i === at ? "rgba(232,135,60,.08)" : undefined,
                        boxShadow: i === at ? "inset 2px 0 0 #E8873C" : undefined,
                        paddingLeft: i === at ? 5 : 7,
                      }}
                    >
                      <span style={{ color: hue, flex: "none" }}>
                        {s.op.actor_id === actorId ? "you" : s.op.actor_kind === "assistant" ? "✦" : actorTag(s.op.actor_id)}
                      </span>
                      <span style={{ flex: "none" }}>{kind}</span>
                      <span style={{
                        color: "#585550", overflow: "hidden",
                        textOverflow: "ellipsis", whiteSpace: "nowrap",
                      }}>
                        {detail}
                      </span>
                      {/* The law is re-checked by replaying, every step — an
                          invertibility claim nobody verifies is a comment. */}
                      {!s.law_holds && (
                        <span style={{ marginLeft: "auto", color: "#E0A34E", flex: "none" }}>law ✗</span>
                      )}
                    </div>
                  );
                })}
              </div>

              <Rule />
              <Label>AT THIS REVISION</Label>
              <div style={{ display: "flex", gap: 8 }}>
                <span
                  className="chip chip-e"
                  style={{ cursor: canRestore ? "pointer" : "default", opacity: canRestore ? 1 : 0.45 }}
                  title={canRestore
                    ? `Revert ${steps.length - 1 - at} op(s) by writing their inverses`
                    : "Already at head — nothing to revert"}
                  onClick={() => {
                    if (!canRestore) return;
                    collab.restoreTo(at);
                    setAction(`restored to rev ${at + 1} — ${steps.length - 1 - at} inverse op(s) written`);
                    // The log grew, so re-read it rather than guessing what
                    // the server appended.
                    setTimeout(load, 400);
                  }}
                >
                  RESTORE
                </span>
                <span
                  className="chip"
                  style={{ cursor: "pointer" }}
                  title="Copy the ops between this revision and head as JSON"
                  onClick={() => {
                    const patch = steps.slice(at + 1).map((s) => s.op.op);
                    void navigator.clipboard?.writeText(JSON.stringify(patch, null, 2));
                    setAction(`copied ${patch.length} op(s) as JSON`);
                  }}
                >
                  COPY AS PATCH
                </span>
              </div>
              {action && (
                <div className="mono" style={{ fontSize: 10, color: "#3FCFA8" }}>✓ {action}</div>
              )}
              <div style={{ fontSize: 11.5, lineHeight: 1.65, color: "#8C8880" }}>
                Restoring writes new ops. Nothing in the log is rewritten, and every earlier
                version stays readable — which is also why the restore itself undoes, as one
                gesture, like any other edit.
              </div>

              <Rule />
              <Label>UNDO IS PER ACTOR</Label>
              <div style={{ fontSize: 11.5, lineHeight: 1.65, color: "#8C8880" }}>
                Your ⌘Z pops your stack only — it never inverts someone else's ops, even the
                ones that landed after yours. Possible only because each op carries its own
                inverse rather than the document carrying a snapshot.
              </div>

              <Rule />
              <Label>THIS BLOCK, AS STORED</Label>
              <Stat label="chars ever inserted" value={num(stored)} />
              <Stat label="live now" value={num(live)} />
              <Stat label="tombstoned" value={num(tombstoned)} tone={tombstoned ? "#A98CE8" : undefined} />
              <Stat label="versions addressable" value={num(steps.length)} />
              <Stat label="copies made" value="0" tone="#3FCFA8" />
              <div style={{ fontSize: 11, lineHeight: 1.6, color: "#585550" }}>
                Reading version <span className="mono" style={{ color: "#8C8880" }}>v</span> is the
                filter <span className="mono" style={{ color: "#9B968D" }}>ins ≤ v &lt; del</span>{" "}
                over one array. History costs storage, never time, and never a second copy of
                the text.
              </div>
            </>
          )}

          {inspTab === "revisions" && (
            <>
              <Label>GESTURES · {num(gestures.length)}</Label>
              <div style={{ fontSize: 11, lineHeight: 1.55, color: "#585550" }}>
                One action is often several ops — a replace is a delete and an insert. These
                are folded by <span className="mono" style={{ color: "#8C8880" }}>undo_group</span>,
                the same field one ⌘Z pops, so a word you typed is one row rather than four.
              </div>
              <div style={{ display: "flex", flexDirection: "column", gap: 7 }}>
                {gestures.slice(-18).reverse().map((g) => {
                  const first = g.steps[0];
                  const last = g.steps[g.steps.length - 1];
                  const lastIndex = steps.indexOf(last);
                  const { kind, detail } = describeOp(first.op.op);
                  const inRange = at >= g.firstIndex;
                  return (
                    <div
                      key={g.key}
                      onClick={() => setAt(lastIndex)}
                      style={{
                        display: "flex", gap: 8, alignItems: "baseline", cursor: "pointer",
                        opacity: inRange ? 1 : 0.45,
                      }}
                    >
                      <span style={{
                        width: 5, height: 5, flex: "none",
                        background: actorHue(first.op.actor_id, actorId),
                      }} />
                      <div style={{ flex: 1, minWidth: 0 }}>
                        <div style={{
                          fontSize: 12, color: "#D2CFC8",
                          overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap",
                        }}>
                          {kind}{detail ? ` ${detail}` : ""}
                        </div>
                        <div className="mono" style={{ fontSize: 9.5, color: "#585550" }}>
                          rev {num(g.firstIndex + 1)}
                          {g.steps.length > 1 ? `–${num(lastIndex + 1)} · ${g.steps.length} ops` : ""}
                          {" · "}
                          {new Date(first.op.created_at).toLocaleTimeString("en-GB")}
                        </div>
                      </div>
                    </div>
                  );
                })}
              </div>

              <Rule />
              <Label>BLOCK · PALIMPSEST SOURCE</Label>
              <div style={{ display: "flex", flexDirection: "column", gap: 6 }}>
                {(head?.blocks ?? []).map((b) => (
                  <div
                    key={b.id}
                    onClick={() => setBlockId(b.id)}
                    style={{
                      fontSize: 11.5, cursor: "pointer",
                      color: b.id === blockId ? "#E4E2DC" : "#8C8880",
                      whiteSpace: "nowrap", overflow: "hidden", textOverflow: "ellipsis",
                      boxShadow: b.id === blockId ? "inset 2px 0 0 #E8873C" : undefined,
                      paddingLeft: b.id === blockId ? 7 : 0,
                    }}
                  >
                    {b.text.slice(0, 44) || `(empty ${b.kind.tag})`}
                  </div>
                ))}
              </div>

              <Rule />
              <Label>WHO WROTE THIS PAGE</Label>
              <div style={{ display: "flex", height: 7, gap: 1 }}>
                {actors.map(([a, n]) => (
                  <div key={a} style={{ flex: n, background: actorHue(a, actorId) }} title={`${n} ops`} />
                ))}
                {actors.length === 0 && (
                  <div style={{ flex: 1, background: "rgba(255,255,255,.06)" }} />
                )}
              </div>
              <div style={{ display: "flex", gap: 12, flexWrap: "wrap", font: "400 10px 'IBM Plex Mono',monospace", color: "#585550" }}>
                {actors.map(([a, n]) => (
                  <span key={a}>
                    <span style={{ color: actorHue(a, actorId) }}>■</span>{" "}
                    {num(n)} {a === actorId ? "yours" : actorTag(a)}
                  </span>
                ))}
              </div>
            </>
          )}
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
