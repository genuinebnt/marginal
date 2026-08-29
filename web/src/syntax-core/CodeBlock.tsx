/**
 * A highlighted code block, read-only.
 *
 * The tokens come from Go (marginal/syntax, via wasm). This renders them and
 * nothing else — which is why it can be used by the reader, the published
 * page and the lab screens without any of them agreeing on a highlighter.
 *
 * It renders the PLAIN source first and swaps in the highlighted version when
 * the module resolves. Not a spinner: a code block that is briefly uncoloured
 * is readable, and a code block that is briefly absent is a layout jump.
 */
import { useEffect, useState } from "react";
import { highlight, type Token } from "./wasm";

/** Kind → hue. The lexer emits a KIND and never a colour (the palette belongs
 *  to the design system), so this is the one place the two meet.
 *
 *  The hues are deliberately drawn from the categorical ramp rather than the
 *  semantic four: amber already means "diagnostic" and teal "you", and a
 *  keyword borrowing either would make a code block argue with the status bar. */
const HUE: Record<Token["kind"], string> = {
  plain: "#B3AFA7",
  keyword: "#C48AE0",
  type: "#7AA8E8",
  string: "#5AC8B4",
  number: "#D6A660",
  comment: "#6E6A63",
  func: "#E8873C",
  punct: "#8C8880",
};

export function CodeBlock({
  language, source, className,
}: { language?: string; source: string; className?: string }) {
  const [tokens, setTokens] = useState<Token[] | null>(null);

  useEffect(() => {
    let cancelled = false;
    highlight(language ?? "", source)
      .then((t) => { if (!cancelled) setTokens(t); })
      // A highlighter failing must never cost you the code. Plain text is the
      // fallback, and it is the same text.
      .catch(() => { if (!cancelled) setTokens(null); });
    return () => { cancelled = true; };
  }, [language, source]);

  return (
    <div className={`blk-code${className ? ` ${className}` : ""}`}>
      <div className="blk-code-h">
        <span className="mono lang">{(language || "plain text").toUpperCase()}</span>
        <span className="mono blk-code-n">{source.split("\n").length} lines</span>
      </div>
      <pre>
        {tokens === null
          ? source
          : tokens.map((t, i) => (
              <span key={i} style={{ color: HUE[t.kind] ?? HUE.plain }}>{t.text}</span>
            ))}
      </pre>
    </div>
  );
}
