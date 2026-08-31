/**
 * docs/ui-mockups/v2/index.html § 11 COMPILER, ported — and editable.
 *
 * Paste and import as a pipeline: buffer → tokens → AST/block tree → ops,
 * with the ops replayed into a SECOND tree and compared field by field. The
 * header says HOLDS because that comparison happened on this input, not
 * because anyone is confident.
 *
 * The buffer is a textarea, and that is the point of the whole lab set: each
 * of these screens claims its figures are computed rather than quoted, and the
 * only way to make the claim checkable is to let you change the input and
 * watch every panel move. Type a broken fence and watch the diagnostic
 * appear; type an emoji and watch CHARS and BYTES part company.
 *
 * Everything below is drawn from one `mdc.Compile` call in Go (wasm). There
 * is no second lexer in TypeScript, and the ops shown are the exact ops the
 * editor's paste handler sends — the same function, not a demonstration of
 * one.
 */
import { useEffect, useMemo, useRef, useState } from "react";
import { compile, type CompileResult } from "../mdc-core/wasm";
import {
  Body, Inspector, Label, Main, Readout, Rule, Screen, StatusBar, SubBar,
  SubItem, TopBar, num,
} from "../shell/Chrome";

const SAMPLE = `## Why not a rope
A rope is the wrong primitive — a
café of fragments.

- anchors survive a split
  - integers do not
- [x] tombstones keep resolving

> The AST is not the parse tree.

\`\`\`rust
enum Op { InsertText { at: Anchor } }
\`\`\`

See [[Anchors, and why offsets die]] and **the operation model**.
`;

type Stage = "buffer" | "tokens" | "ast" | "tree" | "ops";

/** Token kind → hue. Structural openers ember, content plain, blanks dim. */
const TOKEN_HUE: Record<string, string> = {
  ATX_HEADING: "#E8873C", FENCE_OPEN: "#E8873C", FENCE_CLOSE: "#E8873C",
  DIVIDER: "#E8873C", QUOTE: "#C48AE0",
  BULLET: "#5AC8B4", ORDERED: "#5AC8B4", TODO: "#5AC8B4",
  PARAGRAPH: "#9B968D", CODE_TEXT: "#7AA8E8", BLANK: "#4B4842",
};

export function LabCompilerScreen() {
  const [src, setSrc] = useState(SAMPLE);
  const [result, setResult] = useState<CompileResult | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [insTab, setInsTab] = useState<"triggers" | "cost">("triggers");
  const [stage, setStage] = useState<Stage>("tokens");
  /** Bumped on every recompute, to flash the panels that just changed. */
  const [gen, setGen] = useState(0);
  const first = useRef(true);

  // Recompiled on every keystroke. Not debounced: the whole pipeline is
  // sub-millisecond on inputs this size (the stage timings below are measured
  // and say so), and a delay between typing and the panels moving is exactly
  // the feedback this screen exists to give.
  useEffect(() => {
    let cancelled = false;
    compile(src)
      .then((r) => { if (!cancelled) { setResult(r); setErr(null); setGen((g) => g + 1); } })
      .catch((e) => { if (!cancelled) setErr(String(e)); });
    return () => { cancelled = true; };
  }, [src]);

  useEffect(() => { first.current = false; }, []);

  const totalNs = result
    ? result.stats.lex_ns + result.stats.parse_ns + result.stats.emit_ns + result.stats.replay_ns
    : 0;

  const byID = useMemo(() => {
    const m = new Map<string, { depth: number }>();
    if (!result) return m;
    for (const b of result.tree.blocks) {
      const depth = b.parent ? (m.get(b.parent)?.depth ?? 0) + 1 : 0;
      m.set(b.id, { depth });
    }
    return m;
  }, [result]);

  const flash = first.current ? "" : " stage-fresh";

  return (
    <Screen>
      <TopBar
        crumb={<>lab / <b>compiler</b></>}
        readouts={
          <>
            <Readout k="BYTES" v={num(result?.stats.bytes ?? 0)} />
            <Readout
              k="PROJECTION"
              v={result?.holds ? "HOLDS" : "MISMATCH"}
              tone={result?.holds ? "#3FCFA8" : "#E0A34E"}
            />
          </>
        }
      />

      <SubBar>
        {(["buffer", "tokens", "ast", "tree", "ops"] as Stage[]).map((s) => (
          <SubItem key={s} on={stage === s} onClick={() => setStage(s)}>
            {s === "ast" ? "AST" : s === "tree" ? "BLOCK TREE" : s.toUpperCase()}
          </SubItem>
        ))}
        <div style={{ flex: 1 }} />
        <SubItem>
          LIVE · <span style={{ color: "#E8873C" }}>recomputes on every keystroke</span>
        </SubItem>
      </SubBar>

      <Body>
        <Main style={{ overflow: "hidden" }}>
          <div style={{ display: "flex", flex: 1, minHeight: 0 }}>
            {/* BUFFER — editable. Everything to the right is a function of it. */}
            <div style={{
              flex: 1, borderRight: "1px solid rgba(255,255,255,.07)",
              padding: "18px 20px", minWidth: 0, display: "flex", flexDirection: "column",
            }}>
              <Label style={{ marginBottom: 12, display: "block" }}>BUFFER · UTF-8 · EDITABLE</Label>
              <textarea
                className="labedit"
                style={{ flex: 1, minHeight: 0 }}
                value={src}
                spellCheck={false}
                onChange={(e) => setSrc(e.target.value)}
                aria-label="Compiler input"
              />
              <div style={{ marginTop: 16, display: "flex", gap: 20, flexWrap: "wrap" }}>
                <Readout k="CHARS" v={num(result?.stats.chars ?? 0)} />
                <Readout
                  k="BYTES"
                  v={num(result?.stats.bytes ?? 0)}
                  tone={result && result.stats.bytes !== result.stats.chars ? "#E0A34E" : undefined}
                />
                <Readout
                  k="DIVERGENCE"
                  v={result?.stats.divergences.length
                    ? result.stats.divergences.join(" · ")
                    : "none — pure ASCII"}
                />
              </div>
              {(result?.diagnostics.length ?? 0) > 0 && (
                <div style={{ marginTop: 12, display: "flex", flexDirection: "column", gap: 6 }}>
                  {result!.diagnostics.map((d, i) => (
                    <div key={i} className="mono" style={{
                      fontSize: 10, color: "#E0A34E", padding: "6px 9px",
                      border: "1px solid rgba(224,163,78,.28)", background: "rgba(224,163,78,.05)",
                    }}>
                      ◌ line {d.line}: {d.message}
                    </div>
                  ))}
                </div>
              )}
            </div>

            {/* TOKENS */}
            <div key={`tok-${gen}`} className={flash.trim()} style={{ flex: 1, padding: "18px 20px", minWidth: 0, overflowY: "auto" }}>
              <Label style={{ marginBottom: 12, display: "block" }}>
                TOKENS · BLOCK LEXER · {num(result?.stats.tokens ?? 0)}
              </Label>
              <div className="mono" style={{ fontSize: 11, lineHeight: 1.95, color: "#9B968D" }}>
                {(result?.tokens ?? []).map((t, i) => (
                  <div key={i}>
                    <span style={{ color: TOKEN_HUE[t.kind] ?? "#9B968D" }}>{t.kind}</span>
                    {t.level ? ` level=${t.level}` : ""}
                    {t.lang ? ` lang=${t.lang}` : ""}
                    {t.indent ? ` indent=${t.indent}` : ""}
                    {t.checked ? " checked" : ""}{" "}
                    <span style={{ color: "#585550" }}>{t.start}..{t.end}</span>
                  </div>
                ))}
                {(result?.tokens.length ?? 0) === 0 && (
                  <span style={{ color: "#585550" }}>Empty buffer — no tokens, which is a document of zero blocks.</span>
                )}
              </div>
            </div>
          </div>

          <div style={{ borderTop: "1px solid rgba(255,255,255,.07)", display: "flex", flex: 1, minHeight: 0 }}>
            {/* AST → BLOCK TREE */}
            <div key={`tree-${gen}`} className={flash.trim()} style={{
              flex: 1, borderRight: "1px solid rgba(255,255,255,.07)",
              padding: "18px 20px", minWidth: 0, overflowY: "auto",
            }}>
              <Label style={{ marginBottom: 12, display: "block" }}>
                AST → BLOCK TREE · {num(result?.tree.blocks.length ?? 0)}
              </Label>
              <div className="mono" style={{ fontSize: 11, lineHeight: 1.95, color: "#9B968D" }}>
                <div>Root</div>
                {(result?.tree.blocks ?? []).map((b, i, arr) => {
                  const depth = byID.get(b.id)?.depth ?? 0;
                  const last = i === arr.length - 1 || (arr[i + 1] && (byID.get(arr[i + 1].id)?.depth ?? 0) < depth);
                  return (
                    <div key={b.id}>
                      {"│ ".repeat(depth)}{last ? "└ " : "├ "}
                      <span style={{ color: "#C3BFB7" }}>{b.kind.tag}</span>
                      <span style={{ color: "#585550" }}>
                        {b.kind.level ? ` level ${b.kind.level}` : ""}
                        {b.kind.language ? ` · ${b.kind.language}` : ""}
                        {b.kind.list_kind ? ` · ${b.kind.list_kind}` : ""}
                        {b.kind.checked ? " · checked" : ""}
                        {b.kind.tag === "code_block"
                          ? " · marks ✕"
                          : ` · marks ${b.content.marks.length}`}
                      </span>
                    </div>
                  );
                })}
              </div>
              <div style={{ marginTop: 14, fontSize: 11, color: "#585550", lineHeight: 1.6 }}>
                Code has no marks — the bubble menu must be unreachable there, not merely
                ignored. Add <span className="mono">**bold**</span> inside the fence above and
                the mark count stays ✕.
              </div>
            </div>

            {/* OPS + the round trip */}
            <div key={`ops-${gen}`} className={flash.trim()} style={{ flex: 1, padding: "18px 20px", minWidth: 0, overflowY: "auto" }}>
              <Label style={{ marginBottom: 12, display: "block" }}>
                OPS EMITTED · REPLAYED INTO A SECOND TREE
              </Label>
              <div className="mono" style={{ fontSize: 11, lineHeight: 1.95, color: "#9B968D" }}>
                {(result?.ops ?? []).map((o) => (
                  <div key={o.id}>
                    <span style={{ color: "#C3BFB7" }}>{o.type}</span>{" "}
                    {o.kind.tag}
                    <span style={{ color: "#585550" }}>
                      {" "}· {o.parent ? `in ${o.parent}` : "top"}
                      {o.after ? ` after ${o.after}` : " first"}
                    </span>
                  </div>
                ))}
                {(result?.ops.length ?? 0) === 0 && (
                  <span style={{ color: "#585550" }}>No ops — nothing to build.</span>
                )}
              </div>
              <div style={{ marginTop: 14, display: "flex", alignItems: "center", gap: 10, flexWrap: "wrap" }}>
                <span className={result?.holds ? "chip chip-t" : "chip chip-a"}>
                  {result?.holds ? "FIELD-BY-FIELD EQUAL" : "MISMATCH"}
                </span>
                <span className="mono" style={{ fontSize: 10, color: "#585550" }}>
                  {num(result?.stats.ops ?? 0)} ops ·{" "}
                  {result?.holds ? "0 mismatches" : result?.mismatch}
                </span>
              </div>
            </div>
          </div>
        </Main>

        <Inspector
          tabs={[{ id: "triggers", label: "PIPELINE" }, { id: "cost", label: "COST" }]}
          active={insTab}
          onSelect={(id) => setInsTab(id as "triggers" | "cost")}
        >
        {insTab === "cost" ? (
          <div style={{ display: "flex", flexDirection: "column", gap: 12 }}>
            <Label>MEASURED, THIS KEYSTROKE</Label>
            <div className="mono" style={{ fontSize: 10.5, lineHeight: 1.95, color: "#8C8880" }}>
              lex&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;{((result?.stats.lex_ns ?? 0) / 1000).toFixed(1)} µs<br />
              parse&nbsp;&nbsp;&nbsp;{((result?.stats.parse_ns ?? 0) / 1000).toFixed(1)} µs<br />
              emit&nbsp;&nbsp;&nbsp;&nbsp;{((result?.stats.emit_ns ?? 0) / 1000).toFixed(1)} µs<br />
              replay&nbsp;&nbsp;{((result?.stats.replay_ns ?? 0) / 1000).toFixed(1)} µs<br />
              total&nbsp;&nbsp;&nbsp;{(totalNs / 1000).toFixed(1)} µs
            </div>
            <div style={{ fontSize: 11, lineHeight: 1.6, color: "#585550" }}>
              Replay is the projection check — the emitted ops applied to a second, empty
              tree and compared field by field. It is pure overhead in production and is
              here because a pipeline that claims to round-trip should be made to prove it
              on every input, not on a fixture.
            </div>
            <div style={{ height: 1, background: "rgba(255,255,255,.07)" }} />
            <Label>COMPLEXITY</Label>
            <div style={{ fontSize: 11.5, lineHeight: 1.65, color: "#8C8880" }}>
              Linear in input bytes, and deliberately so: the lexer does a bounded backward
              scan with a hard 48-byte lookbehind and no recursion, so there is no input
              that makes it quadratic. That bound is what lets it run on every keystroke.
            </div>
          </div>
        ) : (
          <>
          <Label>THE PROPERTY UNDER TEST</Label>
          <div style={{ fontSize: 11.5, lineHeight: 1.65, color: "#8C8880" }}>
            For any input, replaying the emitted ops into an empty tree must equal the tree the
            pipeline produced directly. It is checked on <b style={{ color: "#C3BFB7", fontWeight: 500 }}>this</b>{" "}
            input, on every keystroke — the header says HOLDS because a comparison happened,
            not because anyone is confident.
          </div>
          <div style={{ fontSize: 11, lineHeight: 1.6, color: "#585550" }}>
            Also pinned by a property test over 3 000 randomised documents, which is what
            catches an emission order where a child precedes its parent — no hand-written
            example reliably produces one.
          </div>

          <Rule />
          <Label>STAGE COST · MEASURED ON THIS INPUT</Label>
          <div className="mono" style={{ fontSize: 10.5, lineHeight: 1.95, color: "#8C8880" }}>
            lex&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;{ms(result?.stats.lex_ns)}<br />
            parse&nbsp;&nbsp;&nbsp;&nbsp;{ms(result?.stats.parse_ns)}<br />
            emit&nbsp;&nbsp;&nbsp;&nbsp;&nbsp;{ms(result?.stats.emit_ns)}<br />
            replay&nbsp;&nbsp;&nbsp;{ms(result?.stats.replay_ns)}<br />
            <span style={{ color: "#C3BFB7" }}>total&nbsp;&nbsp;&nbsp;&nbsp;{ms(totalNs)}</span>
          </div>
          <div style={{ fontSize: 11, lineHeight: 1.6, color: "#585550" }}>
            Replay costs about as much as the parse, and that is the price of the guarantee:
            the pipeline does its work twice so it can check itself once.
          </div>

          <Rule />
          <Label>WHAT EACH PASS THROWS AWAY</Label>
          <div style={{ fontSize: 11.5, lineHeight: 1.7, color: "#8C8880" }}>
            <b style={{ color: "#C3BFB7", fontWeight: 500 }}>lex</b> keeps positions, drops
            nothing else. <b style={{ color: "#C3BFB7", fontWeight: 500 }}>parse</b> drops
            positions and produces an AST, not a parse tree — no node exists to record that a
            delimiter was seen. <b style={{ color: "#C3BFB7", fontWeight: 500 }}>lower</b> drops
            syntax, so nothing downstream can depend on whether you wrote{" "}
            <span className="mono">*x*</span> or <span className="mono">_x_</span>.{" "}
            <b style={{ color: "#C3BFB7", fontWeight: 500 }}>emit</b> drops the tree shape.
          </div>

          <Rule />
          <Label>THIS IS THE PASTE HANDLER</Label>
          <div style={{ fontSize: 11.5, lineHeight: 1.65, color: "#8C8880" }}>
            The ops above are the exact ops the editor sends when you paste — the same
            function, not a demonstration of one. Paste is not a special path: it is a batch of
            ordinary <span className="mono">InsertBlock</span>s under one undo group, so one ⌘Z
            takes the whole thing back.
          </div>

          {err && (
            <>
              <Rule />
              <div style={{ fontSize: 11.5, color: "#E0A34E" }}>◌ {err}</div>
            </>
          )}
          </>
        )}
        </Inspector>
      </Body>

      <StatusBar
        route="/lab/compiler"
        mechanism="lex · parse · lower · emit · replay-and-compare"
        state={result
          ? result.holds
            ? `projection holds · ${num(result.stats.ops)} ops · ${ms(totalNs)}`
            : `MISMATCH · ${result.mismatch}`
          : "compiling…"}
        healthy={result?.holds ?? true}
      />
    </Screen>
  );
}

/** Nanoseconds as milliseconds, at the precision the number deserves. */
function ms(ns?: number): string {
  if (ns === undefined) return "—";
  return `${(ns / 1e6).toFixed(ns < 1e6 ? 3 : 2)} ms`;
}

export default LabCompilerScreen;
