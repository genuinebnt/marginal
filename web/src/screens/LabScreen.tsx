/**
 * docs/ui-mockups/v2/index.html § 10c LAB INDEX, ported.
 *
 * The hub for the screens where the thing being described is running rather
 * than illustrated. § 10c lists six; three exist here, and this page says
 * which — per DESIGN_GUIDELINES §9.4, a nav that lists more destinations than
 * exist must mark which is which rather than quietly omitting the difference.
 *
 * The section's own rule, kept verbatim because it is the reason the section
 * exists: if a screen states a number, the page computed it. Nothing here is
 * a screenshot of a result — which is why every one of them can be wrong in
 * public.
 */
import { useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { useAuth } from "../auth/AuthContext";
import { listPages, type Page } from "../api/pages";
import {
  Body, Label, Readout, Screen, StatusBar, TopBar, num,
} from "../shell/Chrome";
import { undrawn } from "../shell/placeholder";

interface Entry {
  name: string;
  route: string;
  /** Absent when the screen is not built — rendered dimmed, with no link. */
  to?: (pageId: string) => string;
  blurb: string;
  facts: string[];
}

const ENTRIES: Entry[] = [
  {
    name: "Trace",
    route: "/lab/trace",
    to: (p) => `/pages/${p}/trace`,
    blurb: "Step and invert a real op log. The invertibility law is re-checked every step by replaying, never asserted.",
    facts: ["LAW RE-CHECKED", "PER STEP"],
  },
  {
    name: "Diff",
    route: "/lab/diff",
    to: (p) => `/pages/${p}/diff`,
    blurb: "LCS by the full DP table, with the traceback outlined. Also the argument against it: O(n·m) is fine for a block and absurd for a document.",
    facts: ["O(n·m)", "MYERS IS O(nd)"],
  },
  {
    name: "History",
    route: "/lab/history",
    to: (p) => `/pages/${p}/history`,
    blurb: "One tombstoned array. Every revision is the filter ins ≤ v < del over it, so scrubbing costs nothing to keep.",
    facts: ["0 COPIES"],
  },
  {
    name: "Compiler",
    route: "/lab/compiler",
    blurb: "Paste as a pipeline — buffer → tokens → AST → tree → ops. Replaying the emitted ops must equal the tree it built directly.",
    facts: ["NOT BUILT"],
  },
  {
    name: "Netcode",
    route: "/lab/netcode",
    blurb: "One editor, four lenses — prediction, rollback, transform, log. Replay from empty must match the incremental view.",
    facts: ["NOT BUILT"],
  },
  {
    name: "Perf",
    route: "/lab/perf",
    blurb: "Percentiles, queue depth, bundle treemap, flame graph — measured on this machine, now, not quoted from a README.",
    facts: ["NOT BUILT"],
  },
];

export function LabScreen() {
  const { session } = useAuth();
  const actorId = session?.actorId ?? null;
  const navigate = useNavigate();
  const [pages, setPages] = useState<Page[]>([]);

  useEffect(() => {
    if (!actorId) return;
    listPages(actorId).then((r) => setPages(r.pages)).catch(() => {});
  }, [actorId]);

  // The lab screens are per page, because an op log is. Without one there is
  // nothing to step through, so the first page stands in as the default.
  const target = pages[0]?.id ?? null;
  const built = ENTRIES.filter((e) => e.to).length;

  return (
    <Screen>
      <TopBar
        crumb={<>lab / <b>index</b></>}
        readouts={
          <>
            <Readout k="SCREENS" v={num(ENTRIES.length)} />
            <Readout k="BUILT" v={num(built)} tone={built === ENTRIES.length ? "#3FCFA8" : "#E0A34E"} />
          </>
        }
      />

      <Body>
        <div style={{ flex: 1, minWidth: 0, padding: "34px 40px", overflowY: "auto" }}>
          <h1 className="h1" style={{ fontSize: 27, marginBottom: 8 }}>The lab</h1>
          <div style={{ fontSize: 13, color: "#8C8880", lineHeight: 1.6, maxWidth: 640, marginBottom: 26 }}>
            Screens where the thing being described is running rather than illustrated. Every
            number on them is computed from real input — if an implementation is correct the
            figure appears, and if it is wrong the figure is wrong in a way you can see.
          </div>

          <div style={{ display: "grid", gridTemplateColumns: "1fr 1fr", gap: 11 }}>
            {ENTRIES.map((e) => {
              const live = Boolean(e.to && target);
              return (
                <div
                  key={e.name}
                  onClick={() => live && navigate(e.to!(target!))}
                  style={{
                    border: "1px solid rgba(255,255,255,.09)",
                    padding: "14px 16px",
                    cursor: live ? "pointer" : "default",
                    ...(live ? {} : undrawn),
                  }}
                >
                  <div style={{ display: "flex", alignItems: "baseline", gap: 9, marginBottom: 7 }}>
                    <span style={{ fontFamily: "Spectral,serif", fontSize: 16, color: "#EFEDE7" }}>
                      {e.name}
                    </span>
                    <span className="mono" style={{ fontSize: 9.5, color: "#585550" }}>{e.route}</span>
                    <div style={{ flex: 1 }} />
                    {live && <span className="mono" style={{ fontSize: 9.5, color: "#E8873C" }}>→</span>}
                  </div>
                  <div style={{ fontSize: 12, lineHeight: 1.6, color: "#8C8880", marginBottom: 9 }}>
                    {e.blurb}
                  </div>
                  <div className="tgrow">
                    {e.facts.map((f) => (
                      <span key={f} className={`chip${f === "NOT BUILT" ? " chip-a" : ""}`}
                            style={{ padding: "2px 7px" }}>
                        {f}
                      </span>
                    ))}
                  </div>
                </div>
              );
            })}
          </div>

          <div style={{
            marginTop: 26, paddingTop: 22, borderTop: "1px solid rgba(255,255,255,.07)",
            display: "grid", gridTemplateColumns: "1fr 1fr 1.1fr", gap: 26,
          }}>
            <div>
              <Label>THE RULE FOR THIS SECTION</Label>
              <div style={{ fontSize: 11.5, lineHeight: 1.7, color: "#8C8880" }}>
                If a screen states a number, the page computed it. Nothing here is a screenshot of
                a result, and nothing is a hard-coded figure dressed as one — which is why every
                one of them can be wrong in public.
              </div>
            </div>
            <div>
              <Label>WHY IT IS SHIPPED, NOT HIDDEN</Label>
              <div style={{ fontSize: 11.5, lineHeight: 1.7, color: "#8C8880" }}>
                These are the parts of the product that are hard to believe from a description. A
                claim about convergence is cheap; a page where you can break it is not.
              </div>
            </div>
            <div>
              <Label>WHAT IS NOT BUILT</Label>
              <div style={{ fontSize: 11.5, lineHeight: 1.7, color: "#585550" }}>
                Three of six. Compiler, Netcode and Perf are dimmed above rather than omitted,
                because a hub that lists only what exists cannot be read as a map of what the
                section is for.
              </div>
            </div>
          </div>
        </div>
      </Body>

      <StatusBar
        route="/lab"
        mechanism="every figure computed, none authored"
        state={`${built} of ${ENTRIES.length} screens built`}
        healthy={built === ENTRIES.length}
      />
    </Screen>
  );
}

export default LabScreen;
