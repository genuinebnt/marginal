import { beforeAll, describe, expect, it } from "vitest";
import { layoutTick, seedPositions, territory } from "./wasm";

// Integration tests for the graph-core wasm bridge, not a second
// implementation of graphalgo's logic — every assertion here proves a
// real call through wasm.ts reaches the actual Go graphalgo package and
// gets a correct answer back. The behavior itself (the physics, the
// exact Voronoi partition property) is already pinned by
// services/document-service/internal/graphalgo's own tests; duplicating
// that here would just be testing JSON serialization twice.

beforeAll(async () => {
  // Warm the wasm instance once so the first `it` isn't the one paying
  // for instantiation.
  await seedPositions(["warmup"], 1, 0, 0, 1);
});

describe("graph-core wasm bridge", () => {
  it("seedPositions is deterministic for the same seed, through real Go", async () => {
    const first = await seedPositions(["a", "b", "c"], 20260807, 250, 250, 150);
    const second = await seedPositions(["a", "b", "c"], 20260807, 250, 250, 150);
    expect(second).toEqual(first);
    expect(first).toHaveLength(3);
    expect(first[0].id).toBe("a");
  });

  it("layoutTick settles two connected nodes near the spring length, through real Go", async () => {
    let nodes = [
      { id: "a", x: 0, y: 0, vx: 0, vy: 0 },
      { id: "b", x: 500, y: 0, vx: 0, vy: 0 },
    ];
    const edges = [{ from: "a", to: "b" }];
    let alpha = 1;

    for (let i = 0; i < 3000 && alpha > 0.004; i++) {
      const result = await layoutTick(nodes, edges, 250, 0, alpha, null);
      nodes = result.nodes;
      alpha = result.alpha;
    }

    const dx = nodes[0].x - nodes[1].x;
    const dy = nodes[0].y - nodes[1].y;
    const dist = Math.hypot(dx, dy);
    expect(dist).toBeGreaterThan(55);
    expect(dist).toBeLessThan(85); // graphalgo's own default spring_length is 70
  });

  it("layoutTick never moves the dragged node's position", async () => {
    const nodes = [
      { id: "dragged", x: 123, y: 456, vx: 0, vy: 0 },
      { id: "other", x: 0, y: 0, vx: 0, vy: 0 },
    ];
    const result = await layoutTick(nodes, [{ from: "dragged", to: "other" }], 250, 250, 1, "dragged");
    const dragged = result.nodes.find((n) => n.id === "dragged")!;
    expect(dragged.x).toBe(123);
    expect(dragged.y).toBe(456);
  });

  it("territory computes a Voronoi partition and its Delaunay dual, through real Go", async () => {
    const sites = [
      { id: "a", x: 25, y: 50 },
      { id: "b", x: 75, y: 50 },
    ];
    const bounds = { min_x: 0, min_y: 0, max_x: 100, max_y: 100 };
    const result = await territory(sites, bounds);

    expect(result.cells).toHaveLength(2);
    expect(result.delaunay).toHaveLength(1);
    expect([result.delaunay[0].a, result.delaunay[0].b].sort()).toEqual(["a", "b"]);

    // The shoelace area of each cell, computed here purely to verify the
    // wire data round-tripped correctly — not a reimplementation of
    // PolygonArea, just a JS-side check on plain numbers already computed
    // by Go.
    const area = (poly: { x: number; y: number }[]) => {
      let s = 0;
      for (let i = 0; i < poly.length; i++) {
        const p = poly[i];
        const q = poly[(i + 1) % poly.length];
        s += p.x * q.y - q.x * p.y;
      }
      return Math.abs(s) / 2;
    };
    const total = area(result.cells[0].poly) + area(result.cells[1].poly);
    expect(total).toBeCloseTo(100 * 100, 3);
  });
});
