use crate::{
    block::{Block, BlockId},
    operation::Op,
    page::PageError::BlockNotFound,
};

#[derive(Debug, Clone, Copy, PartialEq, Eq, Default)]
pub struct PageId(uuid::Uuid);

impl PageId {
    pub fn new(id: uuid::Uuid) -> Self {
        Self(id)
    }
}

#[derive(Debug, Clone, PartialEq, Eq)]
pub struct Page {
    id: PageId,
    title: String,
    blocks: Vec<Block>,
}

impl Page {
    pub fn new(id: PageId, title: impl Into<String>) -> Self {
        Self {
            id,
            title: title.into(),
            blocks: vec![],
        }
    }

    pub fn blocks(&self) -> &[Block] {
        &self.blocks
    }

    pub fn title(&self) -> &str {
        &self.title
    }

    pub fn apply(&mut self, op: &Op) -> Result<(), PageError> {
        match op {
            Op::InsertBlock { after, block } => {
                if let Some(id) = after {
                    let index = self
                        .blocks
                        .iter()
                        .position(|pos| pos.id() == *id)
                        .ok_or(BlockNotFound(*id))?;
                    self.blocks.insert(index + 1, block.clone());
                } else {
                    self.blocks.insert(0, block.clone());
                }
            }
            Op::DeleteBlock { block, .. } => {
                match self.blocks.iter().position(|pos| pos.id() == block.id()) {
                    Some(index) => self.blocks.remove(index),
                    None => return Err(PageError::BlockNotFound(block.id())),
                };
            }
            Op::UpdateBlockKind { id, new_kind, .. } => {
                let index = self
                    .blocks
                    .iter()
                    .position(|pos| pos.id() == *id)
                    .ok_or(BlockNotFound(*id))?;
                let block = self.blocks.get_mut(index).unwrap();
                block.update_kind(new_kind.clone());
            }
            Op::UpdateBlockContent {
                id, new_content, ..
            } => {
                let index = self
                    .blocks
                    .iter()
                    .position(|pos| pos.id() == *id)
                    .ok_or(BlockNotFound(*id))?;
                let block = self.blocks.get_mut(index).unwrap();
                block.update_content(new_content.clone());
            }
        };

        Ok(())
    }
}

#[derive(Debug, thiserror::Error)]
pub enum PageError {
    #[error("Block not found: {0:?}")]
    BlockNotFound(BlockId),
}

#[cfg(test)]
mod tests {
    use uuid::Uuid;

    use super::*;
    use crate::block::{Block, BlockId, BlockKind};
    use crate::inline::Span;

    const FIXTURE_LEN: u64 = 20;

    fn empty_page() -> Page {
        Page::new(PageId::new(Uuid::from_u128(1)), "Title")
    }

    fn paragraph(id: u64) -> Block {
        Block::new(BlockId::new(id), BlockKind::Paragraph, vec![])
    }

    fn page_with_blocks(n: u64) -> Page {
        let mut page = empty_page();
        for i in 0..n {
            let kind = match i % 3 {
                0 => BlockKind::Paragraph,
                1 => BlockKind::Heading { level: 1 },
                _ => BlockKind::CodeBlock {
                    language: "rust".to_string(),
                },
            };
            let op = Op::InsertBlock {
                after: (i > 0).then(|| BlockId::new(i - 1)),
                block: Block::new(BlockId::new(i), kind, vec![]),
            };
            page.apply(&op).expect("fixture insert must apply");
        }
        page
    }

    fn block_in(page: &Page, id: u64) -> Block {
        page.blocks()
            .iter()
            .find(|b| b.id() == BlockId::new(id))
            .expect("fixture block must exist")
            .clone()
    }

    #[track_caller]
    fn assert_round_trips(page: &mut Page, op: Op) {
        let before = page.clone();

        page.apply(&op).expect("op must apply");
        assert_ne!(*page, before, "op left the page unchanged");

        page.apply(&op.invert()).expect("inverse must apply");
        assert_eq!(*page, before, "inverse did not restore the page");
    }

    #[test]
    fn apply_insert_block_at_start() {
        let mut page = empty_page();
        let op = Op::InsertBlock {
            after: None,
            block: paragraph(1),
        };

        page.apply(&op).unwrap();

        assert_eq!(page.blocks().len(), 1);
        assert_eq!(page.blocks()[0].id(), BlockId::new(1));
    }

    #[test]
    fn apply_insert_block_after_existing() {
        let mut page = empty_page();
        page.apply(&Op::InsertBlock {
            after: None,
            block: paragraph(1),
        })
        .unwrap();

        let op = Op::InsertBlock {
            after: Some(BlockId::new(1)),
            block: paragraph(2),
        };

        page.apply(&op).unwrap();

        assert_eq!(page.blocks().len(), 2);
        assert_eq!(page.blocks()[1].id(), BlockId::new(2));
    }

    #[test]
    fn apply_delete_block_removes_it() {
        let mut page = page_with_blocks(FIXTURE_LEN);
        let op = Op::DeleteBlock {
            after: Some(BlockId::new(10)),
            block: block_in(&page, 11),
        };

        page.apply(&op).unwrap();

        assert_eq!(page.blocks().len() as u64, FIXTURE_LEN - 1);
        assert!(page.blocks().iter().all(|b| b.id() != BlockId::new(11)));
    }

    #[test]
    fn apply_returns_error_if_block_not_found() {
        let mut page = empty_page();
        let op = Op::DeleteBlock {
            after: None,
            block: paragraph(1),
        };

        assert!(matches!(page.apply(&op), Err(PageError::BlockNotFound(_))));
    }

    #[test]
    fn insert_then_invert_restores_empty_page() {
        let mut page = empty_page();

        assert_round_trips(
            &mut page,
            Op::InsertBlock {
                after: None,
                block: paragraph(1),
            },
        );

        assert!(page.blocks().is_empty());
    }

    #[test]
    fn insert_in_middle_then_invert_restores_original_order() {
        let mut page = page_with_blocks(FIXTURE_LEN);

        assert_round_trips(
            &mut page,
            Op::InsertBlock {
                after: Some(BlockId::new(10)),
                block: paragraph(99),
            },
        );
    }

    #[test]
    fn delete_then_invert_restores_block_at_same_index() {
        let mut page = page_with_blocks(FIXTURE_LEN);
        let target = block_in(&page, 11);

        assert_round_trips(
            &mut page,
            Op::DeleteBlock {
                after: Some(BlockId::new(10)),
                block: target,
            },
        );
    }

    #[test]
    fn update_kind_then_invert_restores_old_kind() {
        let mut page = page_with_blocks(FIXTURE_LEN);
        let target = block_in(&page, 5);

        assert_round_trips(
            &mut page,
            Op::UpdateBlockKind {
                id: target.id(),
                old_kind: target.kind().clone(),
                new_kind: BlockKind::Heading { level: 3 },
            },
        );
    }

    #[test]
    fn update_content_then_invert_restores_old_content() {
        let mut page = page_with_blocks(FIXTURE_LEN);
        let target = block_in(&page, 5);

        assert_round_trips(
            &mut page,
            Op::UpdateBlockContent {
                id: target.id(),
                old_content: target.content().to_vec(),
                new_content: vec![Span::new("hello").with_bold()],
            },
        );
    }
}
