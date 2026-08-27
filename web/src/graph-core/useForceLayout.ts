import { useCallback, useEffect, useRef, useState } from "react";
import { layoutTick, seedPositions } from "./wasm";
import type { LayoutEdge, LayoutNode } from "./types";

/**
 * Drives graph.html's own force-directed simulation for real: every tick
 * is a real call into graphalgo.LayoutTick (compiled to wasm,
 * ../graph-core/wasm.ts), never a second, TypeScript-side physics
 * implementation. This hook only owns the animation loop (requestAnimationFrame,
 * alpha bookkeeping, drag state) and renders nothing itself — screens
 * read `nodes` and draw however they need to.
 *
 * "Cools to a stop and reheats on drag" (graph.html's own claim): the
 * loop stops scheduling itself once alpha decays below AlphaMin and
 * nothing is being dragged, and starts again (via `reheat`) exactly the
 * way graph.html's own `reheat()` does.
 */
export function useForceLayout(nodeIds: string[], edges: LayoutEdge[], width: number, height: number) {
  const [nodes, setNodes] = useState<LayoutNode[]>([]);
  const nodesRef = useRef<LayoutNode[]>([]);
  const alphaRef = useRef(1);
  const draggedRef = useRef<string | null>(null);
  const runningRef = useRef(false);
  const rafRef = useRef<number | null>(null);
  const edgesRef = useRef(edges);
  edgesRef.current = edges;
  const seededRef = useRef<string | null>(null);

  const ALPHA_MIN = 0.004;

  const loop = useCallback(() => {
    runningRef.current = true;
    const step = async () => {
      if (!runningRef.current) return;
      const result = await layoutTick(
        nodesRef.current,
        edgesRef.current,
        width / 2,
        height / 2,
        alphaRef.current,
        draggedRef.current,
      );
      nodesRef.current = result.nodes;
      alphaRef.current = result.alpha;
      setNodes(result.nodes);

      if (alphaRef.current > ALPHA_MIN || draggedRef.current) {
        rafRef.current = requestAnimationFrame(() => void step());
      } else {
        runningRef.current = false;
      }
    };
    void step();
  }, [width, height]);

  const reheat = useCallback(
    (amount = 0.55) => {
      alphaRef.current = Math.max(alphaRef.current, amount);
      if (!runningRef.current) loop();
    },
    [loop],
  );

  // Seed once per distinct node-id set (a real page graph doesn't change
  // shape mid-session under normal use) — re-seeding on every render
  // would restart the simulation's own settle-to-a-stop progress for no
  // reason.
  useEffect(() => {
    const key = [...nodeIds].sort().join(",");
    if (key === seededRef.current || nodeIds.length === 0) return;
    seededRef.current = key;
    let cancelled = false;
    void seedPositions(nodeIds, 20260807, width / 2, height / 2, Math.min(width, height) / 3).then((seeded) => {
      if (cancelled) return;
      nodesRef.current = seeded;
      setNodes(seeded);
      alphaRef.current = 1;
      loop();
    });
    return () => {
      cancelled = true;
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [nodeIds.join(","), width, height]);

  useEffect(() => {
    return () => {
      runningRef.current = false;
      if (rafRef.current !== null) cancelAnimationFrame(rafRef.current);
    };
  }, []);

  const startDrag = useCallback(
    (id: string) => {
      draggedRef.current = id;
      reheat(0.3);
    },
    [reheat],
  );

  const dragTo = useCallback((id: string, x: number, y: number) => {
    const n = nodesRef.current.find((n) => n.id === id);
    if (!n) return;
    n.x = x;
    n.y = y;
    setNodes([...nodesRef.current]);
  }, []);

  const endDrag = useCallback(() => {
    draggedRef.current = null;
    reheat(0.2);
  }, [reheat]);

  return { nodes, startDrag, dragTo, endDrag, reheat };
}
