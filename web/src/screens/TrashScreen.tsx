/**
 * docs/ui-mockups/v2/index.html § 23c TRASH & RESTORE, ported.
 *
 * The delete saga, visible — because deleting IS a state, not an event. A
 * cascading delete crosses four services; doing it in one transaction is
 * impossible, and doing it without one means a crash leaves half a page
 * deleted. Idempotent steps plus a stored cursor make a crash a pause rather
 * than a corruption, and this is the one screen that says so.
 *
 * Every figure is real: the steps come from `docs.page_deletions.steps_done`,
 * the remaining ones from `pagesaga.Steps` (that slice is the authority on
 * what "finished" means, which is why appending a step reopens completed
 * sagas), and `attempts > 1` is a saga that resumed after a restart.
 *
 * Steps with no backing store at this scope — embeddings, blobs — are drawn
 * as "no store yet" rather than as work performed or quietly dropped. That is
 * the same honesty the mockup applies to routes it has not drawn, and it is
 * the reason they are real steps in the first place: omitting them means the
 * list silently changes shape when v4 lands.
 */
import { useCallback, useEffect, useMemo, useState } from "react";
import { useNavigate } from "react-router-dom";
import { useAuth } from "../auth/AuthContext";
import { listTrash, previewDelete, restorePage, type DeletePreview, type TrashEntry } from "../api/trash";
import {
  Body, Inspector, Label, Main, Readout, Rule, Screen, StatusBar, SubBar,
  SubItem, TopBar, TopicChip, num,
} from "../shell/Chrome";
import { RowBars } from "../ui";

/** The saga's own step names, as a person reads them. */
const STEP_LABEL: Record<string, string> = {
  tree_detached: "tree detached",
  links_rewritten: "links rewritten",
  search_index: "search index",
  sessions_released: "sessions released",
  embeddings_purged: "embeddings",
  blobs_released: "blobs released",
};

function relative(iso: string): string {
  const days = Math.round((Date.now() - new Date(iso).getTime()) / 86400000);
  if (days <= 0) {
    const hours = Math.round((Date.now() - new Date(iso).getTime()) / 3600000);
    return hours <= 0 ? "just now" : `${hours} h ago`;
  }
  return days === 1 ? "yesterday" : `${days} d ago`;
}

function until(iso: string): string {
  const days = Math.round((new Date(iso).getTime() - Date.now()) / 86400000);
  return days <= 0 ? "any moment" : `${days} d`;
}

export function TrashScreen() {
  const { session } = useAuth();
  const actorId = session?.actorId ?? null;
  const navigate = useNavigate();

  const [entries, setEntries] = useState<TrashEntry[]>([]);
  const [total, setTotal] = useState(0);
  const [selected, setSelected] = useState<string | null>(null);
  const [preview, setPreview] = useState<DeletePreview | null>(null);
  const [note, setNote] = useState<string | null>(null);
  const [err, setErr] = useState<string | null>(null);

  const load = useCallback(() => {
    if (!actorId) return;
    listTrash(actorId)
      .then((r) => { setEntries(r.entries); setTotal(r.total); setErr(null); })
      .catch((e) => setErr(String(e)));
  }, [actorId]);

  useEffect(load, [load]);

  // A saga in flight is state that moves without you. Polling it is the
  // honest thing to do — there is no event stream from document-service, and
  // a screen about progress that never updates is a screenshot.
  const inFlight = useMemo(() => entries.filter((e) => e.progress), [entries]);
  useEffect(() => {
    if (inFlight.length === 0) return;
    const t = setInterval(load, 4000);
    return () => clearInterval(t);
  }, [inFlight.length, load]);

  // The blast radius panel is about a LIVE page, so it can only be shown for
  // one that still exists. A deleted page's descendants have gone with it.
  useEffect(() => {
    if (!actorId || !selected) { setPreview(null); return; }
    previewDelete(actorId, selected).then(setPreview).catch(() => setPreview(null));
  }, [actorId, selected]);

  const deleting = entries.filter((e) => e.page.lifecycle_state === "deleting").length;
  const deleted = entries.length - deleting;
  const soonest = entries.length
    ? entries.reduce((a, b) => (a.purge_at < b.purge_at ? a : b)).purge_at
    : null;

  async function restore(id: string, title: string) {
    if (!actorId) return;
    try {
      await restorePage(actorId, id);
      setNote(`Restored “${title}” — its op log was never deleted, so nothing was rebuilt from a backup.`);
      load();
    } catch (e) {
      setNote(String(e));
    }
  }

  return (
    <Screen>
      <TopBar
        crumb={<>workspace / <b>trash</b></>}
        readouts={
          <>
            <Readout k="DELETING" v={num(deleting)} tone={deleting ? "#E0A34E" : undefined} />
            <Readout k="DELETED" v={num(deleted)} />
            <Readout k="PURGE IN" v={soonest ? until(soonest) : "—"} />
          </>
        }
      />

      <SubBar>
        <SubItem on>IN TRASH · {num(total)}</SubItem>
        <SubItem tone="#585550">PURGED · not counted</SubItem>
        <SubItem tone="#585550">ORPHANED BLOBS · no object store yet</SubItem>
        <div style={{ flex: 1 }} />
        {inFlight.some((e) => (e.progress?.attempts ?? 0) > 1) && (
          <SubItem tone="#E0A34E">
            {num(inFlight.filter((e) => (e.progress?.attempts ?? 0) > 1).length)} saga resuming after restart
          </SubItem>
        )}
      </SubBar>

      <Body>
        <Main style={{ padding: "26px 32px", overflow: "hidden" }}>
          {err && <div style={{ fontSize: 12, color: "#E0A34E", marginBottom: 14 }}>◌ {err}</div>}

          {/* The saga in progress, drawn as the state machine it is. This is
              the one screen that shows a delete is not instantaneous, which is
              the only honest thing to show given the cascade crosses services. */}
          {inFlight.map((e) => (
            <SagaCard key={e.page.id} entry={e} />
          ))}

          {inFlight.length === 0 && entries.length > 0 && (
            <div className="mono" style={{
              fontSize: 10, color: "#3FCFA8", marginBottom: 20,
              padding: "7px 10px", border: "1px solid rgba(63,207,168,.25)",
              background: "rgba(63,207,168,.05)",
            }}>
              ● no saga in flight — every delete below finished its steps
            </div>
          )}

          <Label style={{ marginBottom: 12, display: "block" }}>
            IN TRASH · RESTORABLE UNTIL PURGE
          </Label>

          {note && (
            <div className="mono" style={{ fontSize: 10, color: "#3FCFA8", marginBottom: 10 }}>✓ {note}</div>
          )}

          <div style={{ flex: 1, minHeight: 0, overflowY: "auto" }}>
            {entries.length === 0 && (
              <div style={{ fontSize: 12.5, lineHeight: 1.7, color: "#585550", maxWidth: 560 }}>
                Nothing in the trash. Deleting a page puts it here for 30 days — a window
                derived from <span className="mono" style={{ color: "#8C8880" }}>deleted_at</span> at
                read time, so changing it moves every pending purge without a backfill.
              </div>
            )}
            {entries.map((e) => {
              const busy = Boolean(e.progress);
              return (
                <div
                  key={e.page.id}
                  className="row trash-row"
                  onClick={() => setSelected(e.page.id)}
                >
                  <RowBars colorKey={e.page.topic?.color_key} status={busy ? "warn" : "muted"} />
                  <span style={{ flex: 1, minWidth: 0, fontSize: 13, color: busy ? "#6E6A63" : "#D2CFC8", overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap" }}>
                    {e.page.title || "Untitled"}
                    {busy && <span className="mono" style={{ fontSize: 10, color: "#E0A34E" }}> · in progress</span>}
                  </span>
                  {e.page.topic
                    ? <TopicChip name={e.page.topic.name} colorKey={e.page.topic.color_key} small />
                    : <span className="tpc" style={{ padding: "1px 6px", borderColor: "rgba(255,255,255,.12)", color: "#585550" }}><i style={{ background: "#4B4842" }} />NONE</span>}
                  <span className="mono" style={{ fontSize: 10, color: "#8C8880", width: 92, textAlign: "right" }}>
                    {e.page.deleted_at ? `deleted ${relative(e.page.deleted_at)}` : "—"}
                  </span>
                  <span className="mono" style={{ fontSize: 10, color: "#585550", width: 64, textAlign: "right" }}>
                    {busy ? "—" : `purge ${until(e.purge_at)}`}
                  </span>
                  <span
                    className={busy ? "chip" : "chip chip-t"}
                    style={{ padding: "2px 8px", cursor: busy ? "default" : "pointer", ...(busy ? { color: "#4B4842", borderColor: "rgba(255,255,255,.08)" } : {}) }}
                    title={busy ? "The saga has not finished — restoring mid-flight would race it" : "Flip lifecycle_state back to active"}
                    onClick={(ev) => { ev.stopPropagation(); if (!busy) void restore(e.page.id, e.page.title); }}
                  >
                    RESTORE
                  </span>
                </div>
              );
            })}
          </div>

          {total > entries.length && (
            <div className="mono" style={{ fontSize: 10, color: "#585550", marginTop: 12 }}>
              {num(total - entries.length)} more
              {soonest ? ` · oldest purges in ${until(soonest)}` : ""}
            </div>
          )}

          <div style={{
            marginTop: "auto", display: "grid", gridTemplateColumns: "1fr 1fr", gap: 26,
            paddingTop: 22, borderTop: "1px solid rgba(255,255,255,.07)",
          }}>
            <div>
              <Label style={{ marginBottom: 10, display: "block" }}>WHAT RESTORE ACTUALLY DOES</Label>
              <div style={{ fontSize: 11.5, lineHeight: 1.7, color: "#8C8880" }}>
                Flips <span className="mono" style={{ color: "#9B968D" }}>lifecycle_state</span> back
                to <span className="mono" style={{ color: "#3FCFA8" }}>active</span> and lets the
                projections re-derive — the links index and the FTS vectors rebuild from the op
                log. It does not resurrect an op log, because the op log was never deleted:
                deleting a page removes it from the tree, never from history.
              </div>
            </div>
            <div>
              <Label style={{ marginBottom: 10, display: "block" }}>WHAT PURGE DOES</Label>
              <div style={{ fontSize: 11.5, lineHeight: 1.7, color: "#8C8880" }}>
                Drops the rows, keeping the page id so an inbound link can still say{" "}
                <i>this existed and is gone</i> rather than <i>this never existed</i>. The
                distinction costs one row and saves an argument.
              </div>
            </div>
          </div>
        </Main>

        <Inspector
          tabs={[{ id: "blast", label: "BLAST RADIUS" }, { id: "purged", label: "PURGED" }]}
          active="blast"
          width={302}
        >
          {!selected && (
            <div style={{ fontSize: 11.5, lineHeight: 1.65, color: "#585550" }}>
              Pick a row. The blast radius is computed for a LIVE page — the subtree it would
              take and the links that would dangle — so a page already in the trash has none
              left to report.
            </div>
          )}
          {selected && preview && (
            <>
              <div className="mono" style={{ fontSize: 10.5, lineHeight: 1.95, color: "#8C8880" }}>
                descendants&nbsp;&nbsp;&nbsp;&nbsp;{num(preview.descendants.length)}<br />
                inbound links&nbsp;&nbsp;&nbsp;{num(preview.referrers.length)}<br />
                blocks&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;{num(preview.block_count)}<br />
                blobs held&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;<span style={{ color: "#585550" }}>no object store yet</span>
              </div>

              <Rule />
              <Label>{num(preview.referrers.length)} PAGES LINK HERE</Label>
              <div style={{ fontSize: 12, lineHeight: 1.85, color: "#9B968D" }}>
                {preview.referrers.slice(0, 4).map((p) => (
                  <div key={p.id} style={{ cursor: "pointer" }} onClick={() => navigate(`/read/${p.id}`)}>
                    {p.title}
                  </div>
                ))}
                {preview.referrers.length > 4 && (
                  <span style={{ color: "#585550" }}>{num(preview.referrers.length - 4)} more</span>
                )}
                {preview.referrers.length === 0 && (
                  <span style={{ color: "#585550" }}>Nothing links here.</span>
                )}
              </div>

              {preview.referrers.length > 0 && (
                <div style={{
                  display: "flex", gap: 9, padding: "10px 12px",
                  border: "1px solid rgba(224,163,78,.3)", background: "rgba(224,163,78,.06)",
                }}>
                  <span style={{ color: "#E0A34E", fontSize: 11 }}>◌</span>
                  <div style={{ flex: 1, fontSize: 11.5, lineHeight: 1.55, color: "#9B968D" }}>
                    Those {num(preview.referrers.length)} links become{" "}
                    <b style={{ color: "#E0A34E", fontWeight: 500 }}>dangling</b>, not broken. They
                    render dotted amber and offer to create the page — the same state a link
                    typed before its page exists is already in.
                  </div>
                </div>
              )}
            </>
          )}

          <Rule />
          <Label>WHY A SAGA</Label>
          <div style={{ fontSize: 11.5, lineHeight: 1.65, color: "#8C8880" }}>
            A cascading delete crosses four services. Doing it in one transaction is impossible;
            doing it without one means a crash leaves half a page deleted. Idempotent steps plus
            a stored cursor mean a crash is a pause, not a corruption.
          </div>

          <Rule />
          <Label>WHY THE PROGRESS IS ITS OWN TABLE</Label>
          <div style={{ fontSize: 11.5, lineHeight: 1.65, color: "#8C8880" }}>
            <span className="mono" style={{ color: "#9B968D" }}>lifecycle_state</span> says what a
            page IS; it structurally cannot say how far a delete got. Progress belongs to the
            operation — it has its own retry count, and once the page is purged the row is
            history rather than state.
          </div>
        </Inspector>
      </Body>

      <StatusBar
        route="/trash"
        mechanism="six idempotent steps · resumed from a stored cursor"
        state={deleting ? `${num(deleting)} saga in flight` : `${num(total)} restorable`}
        healthy={deleting === 0}
      />
    </Screen>
  );
}

/** One in-flight saga, as the state machine it is. */
function SagaCard({ entry }: { entry: TrashEntry }) {
  const p = entry.progress!;
  const steps = [...p.steps_done, ...p.steps_left];
  const na = new Set(p.not_applicable);
  const current = p.steps_left[0];

  return (
    <div style={{
      border: "1px solid rgba(224,163,78,.3)", background: "rgba(224,163,78,.05)",
      padding: "16px 18px", marginBottom: 26,
    }}>
      <div style={{ display: "flex", alignItems: "baseline", gap: 11, marginBottom: 14 }}>
        <span className="chip chip-a">DELETING</span>
        <span style={{ fontFamily: "Spectral,serif", fontSize: 17, color: "#EFEDE7" }}>
          {entry.page.title || "Untitled"}
        </span>
        <span className="mono" style={{ fontSize: 10.5, color: "#8C8880" }}>
          {num(p.steps_done.length)} of {num(steps.length)} steps
          {p.attempts > 1 ? ` · resumed ${p.attempts - 1}×` : " · first attempt"}
        </span>
      </div>

      <div style={{ display: "flex", alignItems: "center", gap: 0 }}>
        {steps.map((s, i) => {
          const done = p.steps_done.includes(s);
          const running = s === current;
          return (
            <div key={s} style={{ display: "contents" }}>
              {i > 0 && <div style={{ width: 14 }} />}
              <div style={{ flex: 1, display: "flex", flexDirection: "column", gap: 6 }}>
                <div style={{
                  height: 4,
                  background: done ? "#3FCFA8" : running ? "#E8873C" : "rgba(255,255,255,.07)",
                  animation: running ? "shimmer 1.4s ease-in-out infinite" : undefined,
                }} />
                <span className="mono" style={{
                  fontSize: 9,
                  color: done ? "#3FCFA8" : running ? "#E8873C" : "#585550",
                }}>
                  {done ? "✓ " : running ? "◌ " : ""}{STEP_LABEL[s] ?? s}
                  {na.has(s) && !done && <span style={{ color: "#4B4842" }}> · no store</span>}
                </span>
              </div>
            </div>
          );
        })}
      </div>

      <div style={{ fontSize: 11.5, lineHeight: 1.6, color: "#9B968D", marginTop: 14 }}>
        {p.attempts > 1
          ? <>The process died mid-delete and <b style={{ color: "#C3BFB7", fontWeight: 500 }}>resumed at step {p.steps_done.length + 1}</b>, not from the start. Each step is idempotent, so re-running one costs nothing and skipping one is impossible.</>
          : <>Each step is idempotent and the cursor is stored, so a crash here is a pause rather than a corruption — the sweeper resumes at the first step not yet recorded.</>}
        {p.last_error && (
          <span style={{ color: "#E0A34E" }}> Last error: {p.last_error}</span>
        )}
      </div>
    </div>
  );
}

export default TrashScreen;
