import { beforeAll, describe, expect, it } from "vitest";
import { prefixSearch } from "./wasm";

// Integration tests for the trie-core wasm bridge, not a second
// implementation of trie's logic — every assertion here proves a real
// call through wasm.ts reaches the actual Go trie package and gets a
// correct answer back. The algorithm itself is already pinned by
// services/document-service/internal/trie's own tests; duplicating that
// here would just be testing JSON serialization twice.

beforeAll(async () => {
  await prefixSearch(["warmup"], "warm");
});

describe("trie-core wasm bridge", () => {
  it("finds every title starting with the prefix, through real Go", async () => {
    const titles = ["Architecture notes", "Architecture decisions", "Rollout plan"];
    const matches = await prefixSearch(titles, "Arch");
    expect(matches.sort()).toEqual(["Architecture decisions", "Architecture notes"]);
  });

  it("is case-insensitive, through real Go", async () => {
    const matches = await prefixSearch(["Architecture notes"], "arch");
    expect(matches).toEqual(["Architecture notes"]);
  });

  it("returns an empty array, not null, for no matches", async () => {
    const matches = await prefixSearch(["Rollout plan"], "zzz");
    expect(matches).toEqual([]);
  });
});
