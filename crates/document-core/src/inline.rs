use std::cmp::Ordering;

#[derive(Debug, Clone, PartialEq, Eq, Hash)]
pub enum MarkKind {
    Bold,
    Italic,
    Code,
    Strike,
    Link(String),
}

#[derive(Debug, Clone, PartialEq, Eq, Hash)]
pub struct Mark {
    kind: MarkKind,
    start: usize,
    end: usize,
}

impl Mark {
    pub fn new(kind: MarkKind, start: usize, end: usize) -> Self {
        Self { kind, start, end }
    }
}

#[derive(Debug, Clone, PartialEq, Eq, Hash)]
pub struct Content {
    text: String,
    marks: Vec<Mark>,
}

impl Content {
    pub fn plain(text: impl Into<String>) -> Self {
        Self {
            text: text.into(),
            marks: vec![],
        }
    }

    pub fn text(&self) -> &str {
        &self.text
    }

    pub fn marks(&self) -> &[Mark] {
        &self.marks
    }

    pub fn add_mark(
        &mut self,
        kind: MarkKind,
        start: usize,
        end: usize,
    ) -> Result<(), ContentError> {
        match start.cmp(&end) {
            Ordering::Equal => return Ok(()),
            Ordering::Less => {
                let mark = Mark::new(kind, start, end);
                self.marks.push(mark);
            }
            Ordering::Greater => return Err(ContentError::InvertedRange { start, end }),
        }
        Ok(())
    }

    pub fn remove_mark(
        &mut self,
        kind: MarkKind,
        start: usize,
        end: usize,
    ) -> Result<(), ContentError> {
        todo!()
    }

    fn normalise(&mut self) {
        todo!()
    }

    // pub fn marks_at(&self, offset: usize) -> impl Iterator<Item = &MarkKind> {
    //     todo!()
    // }
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub enum ContentError {
    NotCharBoundary { offset: usize },
    OutOfBounds { offset: usize, len: usize },
    InvertedRange { start: usize, end: usize },
}

#[cfg(test)]
mod tests {
    use super::*;

    fn bold(start: usize, end: usize) -> Mark {
        Mark::new(MarkKind::Bold, start, end)
    }

    fn accented() -> Content {
        Content::plain("héllo")
    }

    #[test]
    fn plain_text_has_no_marks() {
        let c = Content::plain("hello");
        assert_eq!(c.text(), "hello");
        assert!(c.marks().is_empty());
    }

    #[test]
    fn add_mark_stores_it() {
        let mut c = Content::plain("hello");
        c.add_mark(MarkKind::Bold, 0, 5).unwrap();
        assert_eq!(c.marks(), &[bold(0, 5)]);
    }

    #[test]
    fn add_mark_drops_zero_width() {
        let mut c = Content::plain("hello");
        c.add_mark(MarkKind::Bold, 2, 2).unwrap();
        assert!(c.marks().is_empty(), "a zero-width mark must not be stored");
    }

    #[test]
    fn add_mark_rejects_inverted_range() {
        let mut c = Content::plain("hello");
        assert!(matches!(
            c.add_mark(MarkKind::Bold, 4, 1),
            Err(ContentError::InvertedRange { start: 4, end: 1 })
        ));
        assert!(c.marks().is_empty(), "a rejected mark must not be stored");
    }

    #[test]
    fn add_mark_rejects_non_char_boundary() {
        let mut c = accented();
        assert!(matches!(
            c.add_mark(MarkKind::Bold, 0, 2),
            Err(ContentError::NotCharBoundary { offset: 2 })
        ));
    }

    #[test]
    fn add_mark_accepts_boundaries_around_a_multibyte_char() {
        let mut c = accented();
        c.add_mark(MarkKind::Bold, 1, 3).unwrap();
        assert_eq!(c.marks(), &[bold(1, 3)]);
    }

    #[test]
    fn add_mark_rejects_out_of_bounds() {
        let mut c = accented(); // len == 6
        assert!(matches!(
            c.add_mark(MarkKind::Bold, 0, 99),
            Err(ContentError::OutOfBounds { offset: 99, len: 6 })
        ));
    }

    // ── invariant 4: same kind never overlaps and never touches ────────────

    #[test]
    fn same_kind_overlapping_coalesce() {
        let mut c = Content::plain("hello world");
        c.add_mark(MarkKind::Bold, 0, 5).unwrap();
        c.add_mark(MarkKind::Bold, 3, 8).unwrap();
        assert_eq!(c.marks(), &[bold(0, 8)]);
    }

    #[test]
    fn same_kind_touching_coalesce() {
        let mut c = Content::plain("hello world");
        c.add_mark(MarkKind::Bold, 0, 3).unwrap();
        c.add_mark(MarkKind::Bold, 3, 5).unwrap();
        assert_eq!(c.marks(), &[bold(0, 5)], "adjacent runs are one run");
    }

    #[test]
    fn same_kind_disjoint_stay_separate() {
        let mut c = Content::plain("hello world");
        c.add_mark(MarkKind::Bold, 0, 3).unwrap();
        c.add_mark(MarkKind::Bold, 6, 9).unwrap();
        assert_eq!(c.marks(), &[bold(0, 3), bold(6, 9)]);
    }

    #[test]
    fn adding_a_covered_mark_changes_nothing() {
        let mut c = Content::plain("hello world");
        c.add_mark(MarkKind::Bold, 0, 8).unwrap();
        let before = c.clone();
        c.add_mark(MarkKind::Bold, 2, 5).unwrap();
        assert_eq!(c, before, "normalise must be idempotent");
    }

    // ── invariant 5: different kinds are independent ───────────────────────

    #[test]
    fn different_kinds_overlap_freely() {
        let mut c = Content::plain("hello world");
        c.add_mark(MarkKind::Bold, 0, 5).unwrap();
        c.add_mark(MarkKind::Italic, 2, 7).unwrap();
        assert_eq!(c.marks().len(), 2, "bold and italic must not merge");
    }

    #[test]
    fn links_with_different_urls_do_not_coalesce() {
        let mut c = Content::plain("hello world");
        c.add_mark(MarkKind::Link("https://a.example".into()), 0, 5)
            .unwrap();
        c.add_mark(MarkKind::Link("https://b.example".into()), 5, 9)
            .unwrap();
        assert_eq!(
            c.marks().len(),
            2,
            "the URL is part of the kind — touching links with different targets are two marks"
        );
    }

    // ── removal ────────────────────────────────────────────────────────────

    #[test]
    fn remove_mark_splits_a_covering_mark() {
        let mut c = Content::plain("hello world");
        c.add_mark(MarkKind::Bold, 0, 10).unwrap();
        c.remove_mark(MarkKind::Bold, 3, 6).unwrap();
        assert_eq!(c.marks(), &[bold(0, 3), bold(6, 10)]);
    }

    #[test]
    fn remove_mark_trims_a_leading_overlap() {
        let mut c = Content::plain("hello world");
        c.add_mark(MarkKind::Bold, 4, 10).unwrap();
        c.remove_mark(MarkKind::Bold, 0, 6).unwrap();
        assert_eq!(c.marks(), &[bold(6, 10)]);
    }

    #[test]
    fn remove_mark_on_unmarked_range_is_a_noop() {
        let mut c = Content::plain("hello world");
        c.add_mark(MarkKind::Bold, 0, 3).unwrap();
        let before = c.clone();
        c.remove_mark(MarkKind::Bold, 6, 9).unwrap();
        assert_eq!(c, before);
    }

    #[test]
    fn remove_mark_ignores_other_kinds() {
        let mut c = Content::plain("hello world");
        c.add_mark(MarkKind::Bold, 0, 5).unwrap();
        c.add_mark(MarkKind::Italic, 0, 5).unwrap();
        c.remove_mark(MarkKind::Bold, 0, 5).unwrap();
        assert_eq!(c.marks(), &[Mark::new(MarkKind::Italic, 0, 5)]);
    }

    // ── invariant 6: canonical order, and querying ─────────────────────────

    #[test]
    fn marks_are_sorted_by_start() {
        let mut c = Content::plain("hello world");
        c.add_mark(MarkKind::Bold, 6, 9).unwrap();
        c.add_mark(MarkKind::Bold, 0, 3).unwrap();
        assert_eq!(c.marks(), &[bold(0, 3), bold(6, 9)]);
    }

    // #[test]
    // fn marks_at_returns_every_kind_covering_offset() {
    //     let mut c = Content::plain("hello world");
    //     c.add_mark(MarkKind::Bold, 0, 5).unwrap();
    //     c.add_mark(MarkKind::Italic, 2, 7).unwrap();

    //     let at_3: Vec<_> = c.marks_at(3).collect();
    //     assert_eq!(at_3.len(), 2, "offset 3 is inside both");

    //     let at_6: Vec<_> = c.marks_at(6).collect();
    //     assert_eq!(
    //         at_6,
    //         vec![&MarkKind::Italic],
    //         "offset 6 is inside italic only"
    //     );

    //     assert_eq!(c.marks_at(9).count(), 0, "offset 9 is inside neither");
    // }

    // #[test]
    // fn marks_at_excludes_the_end_offset() {
    //     let mut c = Content::plain("hello");
    //     c.add_mark(MarkKind::Bold, 0, 3).unwrap();
    //     assert_eq!(
    //         c.marks_at(3).count(),
    //         0,
    //         "ranges are half-open: end is not covered"
    //     );
    // }
}
