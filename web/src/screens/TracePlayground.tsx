/**
 * § 13 TRACE with no page: the real editor, and the op log it produces.
 *
 * The lab's premise is that these screens show what happens under the
 * hood when someone uses the editor — so the input is the editor, not a
 * script describing one. This is `RichEditorPane`, the same component
 * `/pages/:id` renders, with `useLocalPage` applying its ops to a
 * documentcore page in wasm instead of sending them to a socket.
 *
 * Every op it emits appears in the rail with its inverse, and the
 * invertibility law (RFC-002 §3) is re-checked per op by ACTUALLY
 * applying that inverse and comparing — not printed from a constant.
 *
 * `/pages/:id/trace` still replays a stored page's log. Same screen,
 * same law, two sources: one you type, one the server already has.
 */
import { useState } from "react";
import { RichEditorPane } from "./RichEditorPane";
import { useLocalPage } from "../collab/useLocalPage";
import { useAuth } from "../auth/AuthContext";
import {
  Body, Inspector, Label, Readout, Rule, Screen, StatusBar, TopBar, num,
} from "../shell/Chrome";
import type { Page } from "../api/pages";

const SCRATCH: Page = {
  id: "scratch", title: "Scratch", parent_id: null, created_by: "you",
  path: "scratch", sort_key: "a", lifecycle_state: "active",
  created_at: new Date().toISOString(), updated_at: new Date().toISOString(),
  block_count: 0, word_count: 0,
};

export function TracePlayground() {
  const { session } = useAuth();
  const local = useLocalPage();
  const [title, setTitle] = useState("Scratch");
  const [insTab, setInsTab] = useState<"law" | "kinds">("law");
  /**
   * Which step the document is showing, or null for "the latest".
   *
   * Stepping back does not delete anything: it replays the log from
   * empty through the first n ops, which is what replay IS. The log is
   * the truth and the document is a projection of it, so you can step
   * back and forward without losing what you typed.
   */
  const [at, setAt] = useState<number | null>(null);

  const total = local.ops.length;
  const step = at ?? total;
  const goto = (n: number) => {
    const clamped = Math.max(0, Math.min(total, n));
    setAt(clamped === total ? null : clamped);
    local.restoreTo(clamped);
  };

  const allHold = local.ops.every((o) => o.lawHolds);

  /** Op kinds in this log, commonest first — the KINDS tab. */
  const kindCounts = (() => {
    const m = new Map<string, number>();
    for (const o of local.ops) m.set(o.op.type, (m.get(o.op.type) ?? 0) + 1);
    return [...m].sort((a, b) => b[1] - a[1]);
  })();

  return (
    <Screen>
      <TopBar
        crumb={<>lab / <b>trace</b> · type, and watch the ops</>}
        readouts={
          <>
            <Readout k="STEP" v={`${num(step)} / ${num(total)}`} />
            <Readout
              k="INVERTIBILITY"
              v={local.ops.length === 0 ? "—" : allHold ? "HOLDS" : "FAILS"}
              tone={local.ops.length === 0 ? "#6E6A63" : allHold ? "#3FCFA8" : "#E0A34E"}
            />
          </>
        }
        right={
          <div style={{ display: "flex", gap: 6 }}>
            <span className="chip" style={{ cursor: "pointer" }} onClick={() => goto(step - 1)}>
              ◀ STEP
            </span>
            <span className="chip chip-e" style={{ cursor: "pointer" }} onClick={() => goto(step + 1)}>
              STEP ▶
            </span>
            {/* The law's verdict where you can see it without opening the
                inspector — and it says BROKEN when it is broken, which is the
                only reason a green chip is worth anything. */}
            <span className={allHold ? "chip chip-t" : "chip chip-a"}>
              {total === 0 ? "NO OPS" : allHold ? "HOLDS" : "BROKEN"}
            </span>
          </div>
        }
      />

      <Body>
        <div className="rail" style={{ width: 330 }}>
          <div className="rail-h">
            OP LOG<div /><span style={{ color: "#585550" }}>{local.ops.length}</span>
          </div>
          <div style={{ display: "flex", flexDirection: "column", padding: "0 8px", gap: 1, overflowY: "auto" }}>
            {local.ops.length === 0 && (
              <div style={{ padding: "10px", fontSize: 11.5, lineHeight: 1.6, color: "#585550" }}>
                Empty. Type in the editor — every change produces an op, and each one
                lands here with its inverse.
              </div>
            )}
            {local.ops.slice().reverse().map((o) => (
              <div key={o.seq} style={{ padding: "8px 10px", display: "flex", gap: 9, alignItems: "baseline" }}>
                <span className="mono" style={{ fontSize: 9, color: "#4B4842" }}>{o.seq}</span>
                <span className="mono" style={{
                  flex: 1, fontSize: 10.5,
                  color: /Delete/.test(o.op.type) ? "#E0A34E" : "#8C8880",
                }}>
                  {o.op.type}
                </span>
                <span className="mono" style={{
                  fontSize: 9, color: o.lawHolds ? "#3FCFA8" : "#E0A34E",
                }}>
                  {o.lawHolds ? "↔" : "✗"}
                </span>
              </div>
            ))}
          </div>
          <div className="wal">
            <Label>↔ MEANS THE INVERSE WAS RUN</Label>
            <span style={{ fontSize: 11, color: "#585550", lineHeight: 1.55 }}>
              Applied to the page the op produced, and the result compared. Not a
              constant.
            </span>
          </div>
        </div>

        <div style={{ flex: 1, minWidth: 0, display: "flex", flexDirection: "column", overflow: "hidden" }}>
          <RichEditorPane
            page={{ ...SCRATCH, title }}
            collab={local}
            onRename={setTitle}
            actorId={session?.actorId ?? undefined}
          />
        </div>

        <Inspector
          width={290}
          tabs={[{ id: "law", label: "LAW" }, { id: "kinds", label: "KINDS" }]}
          active={insTab}
          onSelect={(id) => setInsTab(id as "law" | "kinds")}
        >
        {insTab === "kinds" ? (
          <div style={{ display: "flex", flexDirection: "column", gap: 12 }}>
            <Label>OP KINDS YOU PRODUCED</Label>
            {kindCounts.length === 0 ? (
              <div style={{ fontSize: 11.5, color: "#585550" }}>
                Nothing yet. Type, and the kinds appear as the editor emits them.
              </div>
            ) : kindCounts.map(([kind, n]) => (
              <div key={kind} style={{ display: "flex", alignItems: "center", gap: 9 }}>
                <span className="mono" style={{ flex: 1, fontSize: 10.5, color: "#D2CFC8" }}>{kind}</span>
                <div style={{ width: 64, height: 4, background: "rgba(255,255,255,.06)" }}>
                  <div style={{
                    width: `${(n / kindCounts[0][1]) * 100}%`, height: "100%",
                    background: /Delete/.test(kind) ? "#E0A34E" : "#7AA8E8",
                  }} />
                </div>
                <span className="mono" style={{ fontSize: 9.5, color: "#8C8880", width: 26, textAlign: "right" }}>{n}</span>
              </div>
            ))}
            <div style={{ height: 1, background: "rgba(255,255,255,.07)" }} />
            <Label>ONE TIER ONLY</Label>
            <div style={{ fontSize: 11.5, lineHeight: 1.65, color: "#8C8880" }}>
              These are all BLOCK ops — documentcore's tier. Character ops
              (<span className="mono">InsertText</span>, <span className="mono">DeleteText</span>)
              are collaboration-service's, and a scratchpad has no socket to reach it,
              which is also why there are no tombstones here.
            </div>
          </div>
        ) : (
          <>
          <Label>INVERTIBILITY</Label>
          <div style={{ fontSize: 11.5, lineHeight: 1.65, color: "#8C8880" }}>
            RFC-002 §3: every op must be invertible, designed in from the start rather
            than discovered in the undo phase. Each row in the log had its inverse
            applied to the page it produced; <span className="mono">↔</span> means the
            page came back.
          </div>
          <div style={{ fontSize: 11, lineHeight: 1.6, color: "#585550" }}>
            It holds for everything here, and there is a stronger reason than luck:
            the core REFUSES an op whose recorded prior value does not match what the
            block actually holds. An op that could break the law cannot be applied in
            the first place.
          </div>

          {local.rejected && (
            <>
              <Rule />
              <Label>REFUSED BY THE CORE</Label>
              <div className="mono" style={{ fontSize: 10.5, color: "#E0A34E", lineHeight: 1.6 }}>
                {local.rejected}
              </div>
            </>
          )}

          <Rule />
          <Label>SAME EDITOR, DIFFERENT SINK</Label>
          <div style={{ fontSize: 11.5, lineHeight: 1.65, color: "#8C8880" }}>
            This is <span className="mono" style={{ color: "#9B968D" }}>RichEditorPane</span>,
            the component <span className="mono">/pages/:id</span> uses, unmodified. It takes
            a <span className="mono">CollabPage</span>; the one behind it here applies ops to
            documentcore in wasm rather than sending them to collaboration-service.
          </div>
          <div style={{ fontSize: 11, lineHeight: 1.6, color: "#585550" }}>
            So the log is not a demonstration of what the editor would emit. It is what
            it emitted. Nothing is saved.
          </div>

          <Rule />
          <Label>NOT HERE</Label>
          <div style={{ fontSize: 11, lineHeight: 1.6, color: "#585550" }}>
            Character tombstones: those are the text tier, which lives in
            collaboration-service and has no wasm build. Open a real page's history for
            those.
          </div>

          <Rule />
          <span className="chip" style={{ cursor: "pointer", alignSelf: "flex-start" }}
                onClick={local.reset}>
            CLEAR
          </span>
          </>
        )}
        </Inspector>
      </Body>

      <StatusBar
        route="/lab/trace"
        mechanism="RichEditorPane over documentcore in wasm · the law re-checked per op"
        state={local.ops.length === 0
          ? "type something"
          : allHold
            ? `${num(local.ops.length)} ops, every inverse ran and restored the page`
            : "an inverse did not restore the page"}
        healthy={allHold}
      />
    </Screen>
  );
}
