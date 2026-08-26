import { randomUUID } from "node:crypto";
import { beforeAll, describe, expect, it } from "vitest";
import { History } from "./history";
import { bold, paragraph, plainContent } from "./types";
import { DocumentCoreError, applyOp, invertOp, newPage } from "./wasm";

// These are integration tests for the WASM bridge, not a second
// implementation of document-core's logic — every assertion here is
// proving that a real call through wasm.ts reaches the actual Go
// documentcore package and gets a correct answer back. The behavior itself
// (mark coalescing, op preconditions, invertibility) is already pinned by
// services/document-service/internal/documentcore's tests and the shared
// testdata/document-core/marks.json vectors; duplicating that here would
// just be testing JSON serialization twice.

beforeAll(async () => {
  // Warm the wasm instance once so the first `it` isn't the one paying for
  // instantiation — matters more for readable timings than correctness.
  await newPage(randomUUID(), "warmup");
});

describe("wasm bridge", () => {
  it("creates a page and inserts a block through real Go logic", async () => {
    const pageId = randomUUID();
    const page = await newPage(pageId, "Title");
    expect(page.id).toBe(pageId);
    expect(page.blocks).toEqual([]);

    const blockId = randomUUID();
    const next = await applyOp(page, {
      type: "InsertBlock",
      id: blockId,
      after: null,
      kind: paragraph(),
      content: plainContent("hello"),
    });

    expect(next.blocks).toHaveLength(1);
    expect(next.blocks[0].id).toBe(blockId);
    expect(next.blocks[0].content.text).toBe("hello");
  });

  it("propagates a documentcore precondition error as DocumentCoreError", async () => {
    const page = await newPage(randomUUID(), "Title");

    await expect(
      applyOp(page, {
        type: "DeleteBlock",
        tombstone: { id: randomUUID(), kind: paragraph(), content: plainContent("") },
        after: null,
      }),
    ).rejects.toBeInstanceOf(DocumentCoreError);
  });

  it("inverts an op through Go and the invert round-trips", async () => {
    const blockId = randomUUID();
    const op = {
      type: "InsertBlock" as const,
      id: blockId,
      after: null,
      kind: paragraph(),
      content: plainContent("x"),
    };

    const inverted = await invertOp(op);
    expect(inverted.type).toBe("DeleteBlock");

    const restored = await invertOp(inverted);
    expect(restored).toEqual(op);
  });

  it("History.undo/redo round-trip a content change via real Go apply+invert", async () => {
    let page = await newPage(randomUUID(), "Title");
    const blockId = randomUUID();
    page = await applyOp(page, {
      type: "InsertBlock",
      id: blockId,
      after: null,
      kind: paragraph(),
      content: plainContent(""),
    });

    const history = new History(8);
    const before = page.blocks[0].content;

    const retype = {
      type: "SetBlockContent" as const,
      block: blockId,
      prev: before,
      content: { text: "hello", marks: [{ kind: bold(), start: 0, end: 5 }] },
    };
    page = await applyOp(page, retype);
    history.record(retype);
    expect(page.blocks[0].content.text).toBe("hello");

    page = await history.undo(page);
    expect(page.blocks[0].content).toEqual(before);

    page = await history.redo(page);
    expect(page.blocks[0].content.text).toBe("hello");
  });
});
