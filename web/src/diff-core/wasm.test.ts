import { beforeAll, describe, expect, it } from "vitest";
import { diffTokens, tokenizeChars, tokenizeWords } from "./wasm";

// Integration tests for the diff-core wasm bridge, not a second
// implementation of textdiff's logic — every assertion here proves a
// real call through wasm.ts reaches the actual Go textdiff package and
// gets a correct answer back. The algorithm itself (LCS correctness,
// the reconstruction law) is already pinned by services/textdiff's own
// tests; duplicating that here would just be testing JSON serialization
// twice.

beforeAll(async () => {
  // Warm the wasm instance once so the first `it` isn't the one paying
  // for instantiation.
  await diffTokens(["warmup"], ["warmup"]);
});

describe("diff-core wasm bridge", () => {
  it("identical token sequences are all equal, through real Go", async () => {
    const tokens = tokenizeWords("the quick brown fox");
    const result = await diffTokens(tokens, tokens);
    expect(result.ops.every((op) => op.kind === "equal")).toBe(true);
    expect(result.ops.map((op) => op.token)).toEqual(tokens);
  });

  it("finds a minimal word-level edit between similar sentences, through real Go", async () => {
    const a = tokenizeWords("we hold sync acknowledgement under a tight budget");
    const b = tokenizeWords("we hold sync acknowledgement under a strict budget");
    const result = await diffTokens(a, b);

    const changed = result.ops.filter((op) => op.kind !== "equal");
    expect(changed).toEqual([
      { kind: "delete", token: "tight" },
      { kind: "insert", token: "strict" },
    ]);
  });

  it("recomputes at character granularity — the same toggle diff.html exposes", async () => {
    const a = tokenizeChars("cat");
    const b = tokenizeChars("cot");
    const result = await diffTokens(a, b);

    const changed = result.ops.filter((op) => op.kind !== "equal");
    expect(changed).toEqual([
      { kind: "delete", token: "a" },
      { kind: "insert", token: "o" },
    ]);
  });

  it("the DP table's bottom-right corner is the LCS length, through real Go", async () => {
    const a = ["a", "b", "c", "b", "d", "a", "b"];
    const b = ["b", "d", "c", "a", "b", "a"];
    const result = await diffTokens(a, b);
    expect(result.table[a.length][b.length]).toBe(4);
  });

  it("the traceback path starts at the table's corner and ends at the origin, through real Go", async () => {
    const a = tokenizeWords("we hold sync acknowledgement under a tight budget");
    const b = tokenizeWords("we hold sync acknowledgement under a strict budget");
    const result = await diffTokens(a, b);
    expect(result.path[0]).toEqual({ i: a.length, j: b.length });
    expect(result.path[result.path.length - 1]).toEqual({ i: 0, j: 0 });
  });
});
