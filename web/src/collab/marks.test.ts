import { describe, expect, it } from "vitest";
import { addMark, isFullyMarked, marksAt, removeMark, renderMarkedHTML, shiftMarksForEdit, type Mark } from "./marks";

const bold = { tag: "bold" as const };
const italic = { tag: "italic" as const };

describe("marksAt / isFullyMarked", () => {
  it("finds every mark covering an offset, half-open at End", () => {
    const marks: Mark[] = [{ kind: bold, start: 0, end: 5 }];
    expect(marksAt(marks, 0)).toEqual([bold]);
    expect(marksAt(marks, 4)).toEqual([bold]);
    expect(marksAt(marks, 5)).toEqual([]); // End is exclusive
  });

  it("is fully marked only when the whole range is covered edge to edge", () => {
    const marks: Mark[] = [{ kind: bold, start: 0, end: 3 }, { kind: bold, start: 3, end: 6 }];
    expect(isFullyMarked(marks, bold, 0, 6)).toBe(true); // two touching runs cover it
    expect(isFullyMarked(marks, bold, 0, 7)).toBe(false); // one past the end
    expect(isFullyMarked([{ kind: bold, start: 0, end: 3 }], bold, 1, 2)).toBe(true); // fully inside one run
  });
});

describe("addMark / removeMark", () => {
  it("adds a mark verbatim over the given range", () => {
    const result = addMark([], bold, 2, 5);
    expect(result).toEqual([{ kind: bold, start: 2, end: 5 }]);
  });

  it("is a no-op for a zero-width or inverted range", () => {
    expect(addMark([], bold, 3, 3)).toEqual([]);
    expect(addMark([], bold, 5, 3)).toEqual([]);
  });

  it("removing a range that fully covers a mark drops it", () => {
    const marks: Mark[] = [{ kind: bold, start: 2, end: 5 }];
    expect(removeMark(marks, bold, 0, 10)).toEqual([]);
  });

  it("removing a hole inside a mark splits it in two", () => {
    const marks: Mark[] = [{ kind: bold, start: 0, end: 10 }];
    const result = removeMark(marks, bold, 3, 6);
    expect(result).toEqual([
      { kind: bold, start: 0, end: 3 },
      { kind: bold, start: 6, end: 10 },
    ]);
  });

  it("removing trims a mark's left or right edge", () => {
    expect(removeMark([{ kind: bold, start: 0, end: 10 }], bold, 0, 4)).toEqual([{ kind: bold, start: 4, end: 10 }]);
    expect(removeMark([{ kind: bold, start: 0, end: 10 }], bold, 6, 10)).toEqual([{ kind: bold, start: 0, end: 6 }]);
  });

  it("leaves disjoint marks and other kinds untouched", () => {
    const marks: Mark[] = [{ kind: bold, start: 0, end: 3 }, { kind: italic, start: 0, end: 10 }];
    const result = removeMark(marks, bold, 5, 8);
    expect(result).toEqual(marks);
  });
});

describe("shiftMarksForEdit", () => {
  it("keeps a mark entirely in the untouched prefix, unshifted", () => {
    const marks: Mark[] = [{ kind: bold, start: 0, end: 5 }];
    // "hello world" -> "hello there world": inserted text after the prefix
    const result = shiftMarksForEdit(marks, "hello world", "hello there world");
    expect(result).toEqual([{ kind: bold, start: 0, end: 5 }]);
  });

  it("shifts a mark entirely in the untouched suffix by the length delta", () => {
    const marks: Mark[] = [{ kind: bold, start: 6, end: 11 }]; // "world" in "hello world"
    const result = shiftMarksForEdit(marks, "hello world", "hello there world");
    // "there " (6 chars) inserted before the suffix -> shift by +6
    expect(result).toEqual([{ kind: bold, start: 12, end: 17 }]);
  });

  it("drops a mark that overlaps the actually-changed region", () => {
    const marks: Mark[] = [{ kind: bold, start: 2, end: 8 }]; // overlaps the edit
    const result = shiftMarksForEdit(marks, "hello world", "hey world");
    expect(result).toEqual([]);
  });

  it("is a no-op when the text is unchanged", () => {
    const marks: Mark[] = [{ kind: bold, start: 0, end: 5 }];
    expect(shiftMarksForEdit(marks, "hello", "hello")).toBe(marks);
  });

  it("handles a pure deletion (newText shorter, no insertion)", () => {
    const marks: Mark[] = [{ kind: bold, start: 6, end: 11 }]; // "world"
    const result = shiftMarksForEdit(marks, "hello world", "hello ");
    // "world" itself got deleted — nothing left to shift, the mark's own
    // region was in the changed middle (there's no untouched suffix here)
    expect(result).toEqual([]);
  });
});

describe("renderMarkedHTML", () => {
  it("escapes plain text with no marks", () => {
    expect(renderMarkedHTML("<b>hi</b> & bye", [])).toBe("&lt;b&gt;hi&lt;/b&gt; &amp; bye");
  });

  it("wraps a marked segment and leaves the rest plain", () => {
    const marks: Mark[] = [{ kind: bold, start: 6, end: 11 }];
    expect(renderMarkedHTML("hello world", marks)).toBe("hello <strong>world</strong>");
  });

  it("nests overlapping marks of different kinds, well-formed", () => {
    const marks: Mark[] = [{ kind: bold, start: 0, end: 5 }, { kind: italic, start: 2, end: 8 }];
    // "hello world": [0-2) bold only, [2-5) bold+italic, [5-8) italic only, [8-11) plain
    const html = renderMarkedHTML("hello world", marks);
    expect(html).toBe("<strong>he</strong><strong><em>llo</em></strong><em> wo</em>rld");
  });

  it("escapes text inside a link href and the text content", () => {
    const marks: Mark[] = [{ kind: { tag: "link", url: 'javascript:alert(1)"' }, start: 0, end: 4 }];
    const html = renderMarkedHTML("click", marks);
    expect(html).not.toContain('"><script>');
    expect(html).toContain("&quot;");
  });
});
