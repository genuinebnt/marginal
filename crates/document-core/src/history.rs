use crate::{
    operation::Op,
    page::{Page, PageError},
};

#[derive(Debug)]
pub struct History {
    pub undo: Vec<Op>,
    pub redo: Vec<Op>,
    pub max_depth: usize,
}

impl History {
    pub fn new(max_depth: usize) -> Self {
        Self {
            undo: vec![],
            redo: vec![],
            max_depth,
        }
    }

    pub fn record(&mut self, op: Op) {
        self.undo.push(op);
        self.redo.clear();

        if self.undo.len() > self.max_depth {
            self.undo.remove(0);
        }
    }

    pub fn undo(&mut self, page: &mut Page) -> Result<(), PageError> {
        if let Some(undo_op) = self.undo.pop() {
            let redo_op = undo_op.invert();
            self.redo.push(redo_op.clone());
            page.apply(&redo_op).inspect_err(|err| {
                self.undo.push(undo_op);
                self.redo.pop();
            })?;
        }

        Ok(())
    }

    pub fn redo(&mut self, page: &mut Page) -> Result<(), PageError> {
        if let Some(redo_op) = self.redo.pop() {
            let undo_op = redo_op.invert();
            self.undo.push(undo_op.clone());
            page.apply(&undo_op).inspect_err(|_err| {
                self.redo.push(redo_op);
                self.undo.pop();
            })?;
        }

        Ok(())
    }
}

#[cfg(test)]
mod tests {
    use super::*;
    use crate::block::{Block, BlockId, BlockKind};
    use crate::inline::Span;
    use crate::page::PageId;

    fn paragraph(id: u64) -> Block {
        Block::new(BlockId::new(id), BlockKind::Paragraph, vec![])
    }

    /// A page holding one paragraph, block 1, inserted outside the history.
    fn page_with_one_block() -> Page {
        let mut page = Page::new(PageId::new(), "Title");
        page.apply(&Op::InsertBlock {
            after: None,
            block: paragraph(1),
        })
        .expect("fixture insert must apply");
        page
    }

    fn retype_block_1() -> Op {
        Op::UpdateBlockContent {
            id: BlockId::new(1),
            old_content: vec![],
            new_content: vec![Span::new("hello").with_bold()],
        }
    }

    /// Apply `op` and record it, the way a caller is expected to pair them.
    fn do_op(history: &mut History, page: &mut Page, op: Op) {
        page.apply(&op).expect("op must apply");
        history.record(op);
    }

    /// `redo` recovers the forward op by inverting twice, so `invert` must be an
    /// involution. Nothing in the type system enforces that; this test does.
    #[test]
    fn invert_is_an_involution() {
        let ops = [
            Op::InsertBlock {
                after: None,
                block: paragraph(1),
            },
            Op::DeleteBlock {
                after: Some(BlockId::new(1)),
                block: paragraph(2),
            },
            Op::UpdateBlockKind {
                id: BlockId::new(1),
                old_kind: BlockKind::Paragraph,
                new_kind: BlockKind::Heading { level: 3 },
            },
            retype_block_1(),
        ];

        for op in ops {
            assert_eq!(op.invert().invert(), op, "invert is not an involution");
        }
    }

    #[test]
    fn undo_restores_the_pre_op_page() {
        let mut page = page_with_one_block();
        let mut history = History::new(8);
        let before = page.clone();

        do_op(&mut history, &mut page, retype_block_1());
        assert_ne!(page, before, "op left the page unchanged");

        history.undo(&mut page).expect("undo must apply");
        assert_eq!(page, before, "undo did not restore the page");
    }

    #[test]
    fn redo_restores_the_post_op_page() {
        let mut page = page_with_one_block();
        let mut history = History::new(8);

        do_op(&mut history, &mut page, retype_block_1());
        let after = page.clone();

        history.undo(&mut page).expect("undo must apply");
        history.redo(&mut page).expect("redo must apply");

        assert_eq!(page, after, "redo did not restore the post-op page");
    }

    /// Undo and redo must be exact inverses however many times they alternate —
    /// no drift, no accumulation.
    #[test]
    fn undo_redo_cycles_converge() {
        let mut page = page_with_one_block();
        let mut history = History::new(8);
        let before = page.clone();

        do_op(&mut history, &mut page, retype_block_1());
        let after = page.clone();

        for i in 0..10 {
            history.undo(&mut page).expect("undo must apply");
            assert_eq!(page, before, "drifted from pre-op state on cycle {i}");

            history.redo(&mut page).expect("redo must apply");
            assert_eq!(page, after, "drifted from post-op state on cycle {i}");
        }
    }

    #[test]
    fn recording_a_new_op_clears_the_redo_stack() {
        let mut page = page_with_one_block();
        let mut history = History::new(8);

        do_op(&mut history, &mut page, retype_block_1());
        history.undo(&mut page).expect("undo must apply");
        assert_eq!(history.redo.len(), 1, "undo should have filled redo");

        do_op(
            &mut history,
            &mut page,
            Op::UpdateBlockKind {
                id: BlockId::new(1),
                old_kind: BlockKind::Paragraph,
                new_kind: BlockKind::Heading { level: 2 },
            },
        );

        assert!(
            history.redo.is_empty(),
            "a new op must invalidate the redo branch"
        );
    }

    #[test]
    fn undo_on_empty_history_leaves_the_page_alone() {
        let mut page = page_with_one_block();
        let mut history = History::new(8);
        let before = page.clone();

        history
            .undo(&mut page)
            .expect("undo on empty must not error");
        history
            .redo(&mut page)
            .expect("redo on empty must not error");

        assert_eq!(page, before);
        assert!(history.undo.is_empty());
        assert!(history.redo.is_empty());
    }

    #[test]
    fn max_depth_evicts_the_oldest_op() {
        let mut page = page_with_one_block();
        let mut history = History::new(2);

        for level in 1..=4u8 {
            do_op(
                &mut history,
                &mut page,
                Op::UpdateBlockKind {
                    id: BlockId::new(1),
                    old_kind: BlockKind::Paragraph,
                    new_kind: BlockKind::Heading { level },
                },
            );
        }

        assert_eq!(history.undo.len(), 2, "undo stack must respect max_depth");

        // Draining past the retained depth must stop cleanly, not panic.
        for _ in 0..5 {
            history.undo(&mut page).expect("undo must not error");
        }
    }

    /// If the inverse cannot apply, the undo must not have happened: the op stays
    /// on the undo stack and the redo stack is untouched. Today the op is popped
    /// and pushed to redo *before* `apply` runs, so a failure leaves both stacks
    /// describing edits that were never made.
    #[test]
    fn failed_undo_leaves_both_stacks_unchanged() {
        let mut page = page_with_one_block();
        let mut history = History::new(8);

        do_op(&mut history, &mut page, retype_block_1());

        // The block the recorded op refers to disappears behind history's back.
        page.apply(&Op::DeleteBlock {
            after: None,
            block: page.blocks()[0].clone(),
        })
        .expect("delete must apply");

        let undo_depth = history.undo.len();
        let redo_depth = history.redo.len();

        assert!(
            history.undo(&mut page).is_err(),
            "undo must report that the inverse could not apply"
        );
        assert_eq!(
            history.undo.len(),
            undo_depth,
            "a failed undo must not consume the op"
        );
        assert_eq!(
            history.redo.len(),
            redo_depth,
            "a failed undo must not push onto redo"
        );
    }

    #[test]
    fn failed_redo_leaves_both_stacks_unchanged() {
        let mut page = page_with_one_block();
        let mut history = History::new(8);

        do_op(&mut history, &mut page, retype_block_1());
        history.undo(&mut page).expect("undo must apply");

        page.apply(&Op::DeleteBlock {
            after: None,
            block: page.blocks()[0].clone(),
        })
        .expect("delete must apply");

        let undo_depth = history.undo.len();
        let redo_depth = history.redo.len();

        assert!(
            history.redo(&mut page).is_err(),
            "redo must report that the op could not apply"
        );
        assert_eq!(
            history.undo.len(),
            undo_depth,
            "a failed redo must not push onto undo"
        );
        assert_eq!(
            history.redo.len(),
            redo_depth,
            "a failed redo must not consume the op"
        );
    }
}
