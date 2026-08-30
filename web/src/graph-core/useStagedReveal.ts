/**
 * Reveals an already-computed result one step at a time.
 *
 * The distinction matters: nothing here computes anything. Every
 * lens on § 08 gets its answer from Go — BFS, Tarjan, Kahn, the
 * flood fill — and this only decides how much of that answer is
 * on screen yet. An animation that recomputed as it played would
 * be a second implementation of the algorithm, drawn.
 *
 * Why animate at all: a static paint of "these nodes are green"
 * shows the RESULT and hides the PROCESS, and the process is
 * what distinguishes one algorithm from another. Every lens
 * painting the same still image is why they all looked alike.
 */
import { useEffect, useRef, useState } from "react";

export interface StagedReveal {
  /** How many steps are revealed — 0..total. */
  shown: number;
  /** True while steps are still being revealed. */
  running: boolean;
  /** Restart from zero. Called when the lens or the source changes. */
  replay: () => void;
}

/**
 * @param total  how many steps there are
 * @param msPerStep  delay between reveals
 * @param key  changing this restarts the reveal — the lens, the source,
 *             anything whose change makes the old animation meaningless
 */
export function useStagedReveal(total: number, msPerStep: number, key: string): StagedReveal {
  const [shown, setShown] = useState(0);
  const [nonce, setNonce] = useState(0);
  const timer = useRef<number | null>(null);

  useEffect(() => {
    setShown(0);
    if (total <= 0) return;

    // A step at a time rather than one long CSS transition,
    // because the steps are not interchangeable: step 4 of a BFS
    // is a different fact from step 5, and a reader watching for
    // where a frontier stalls needs to see each one land.
    let i = 0;
    timer.current = window.setInterval(() => {
      i += 1;
      setShown(i);
      if (i >= total && timer.current !== null) {
        window.clearInterval(timer.current);
        timer.current = null;
      }
    }, msPerStep);

    return () => {
      if (timer.current !== null) {
        window.clearInterval(timer.current);
        timer.current = null;
      }
    };
  }, [total, msPerStep, key, nonce]);

  return { shown, running: shown < total, replay: () => setNonce((n) => n + 1) };
}
