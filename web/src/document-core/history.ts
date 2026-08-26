import type { Op, Page } from "./types";
import { applyOp, invertOp } from "./wasm";

/**
 * A per-actor undo/redo stack over Ops. This class is bookkeeping only —
 * two arrays and push/pop — not a second implementation of undo/redo
 * semantics: every apply and invert call goes through wasm.ts into the same
 * Go documentcore that document-service uses. If applyOp/invertOp throw,
 * the throw happens before this class's stacks are mutated, so a failed
 * undo/redo leaves them exactly as they were.
 */
export class History {
  private undoStack: Op[] = [];
  private redoStack: Op[] = [];
  private readonly maxDepth: number;

  constructor(maxDepth: number) {
    this.maxDepth = maxDepth;
  }

  get undoDepth(): number {
    return this.undoStack.length;
  }

  get redoDepth(): number {
    return this.redoStack.length;
  }

  /** Pushes op onto the undo stack and clears redo. Evicts the oldest
   * recorded op if maxDepth is exceeded. */
  record(op: Op): void {
    this.undoStack.push(op);
    this.redoStack = [];
    if (this.undoStack.length > this.maxDepth) this.undoStack.shift();
  }

  /** Applies the inverse of the most recently recorded op and returns the
   * resulting page. Returns page unchanged if the undo stack is empty. */
  async undo(page: Page): Promise<Page> {
    if (this.undoStack.length === 0) return page;
    const undoOp = this.undoStack[this.undoStack.length - 1];
    const redoOp = await invertOp(undoOp);

    const next = await applyOp(page, redoOp);
    this.undoStack.pop();
    this.redoStack.push(redoOp);
    return next;
  }

  /** Re-applies the most recently undone op and returns the resulting
   * page. Returns page unchanged if the redo stack is empty. */
  async redo(page: Page): Promise<Page> {
    if (this.redoStack.length === 0) return page;
    const redoOp = this.redoStack[this.redoStack.length - 1];
    const undoOp = await invertOp(redoOp);

    const next = await applyOp(page, undoOp);
    this.redoStack.pop();
    this.undoStack.push(undoOp);
    return next;
  }
}
