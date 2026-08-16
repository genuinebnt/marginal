use crate::{
    block::{Block, BlockId, BlockKind},
    inline::Content,
};

#[derive(Debug, Clone, PartialEq, Eq, Hash)]
pub enum Op {
    InsertBlock {
        after: Option<BlockId>,
        block: Block,
    },

    DeleteBlock {
        after: Option<BlockId>,
        block: Block,
    },

    UpdateBlockKind {
        id: BlockId,
        old_kind: BlockKind,
        new_kind: BlockKind,
    },

    UpdateBlockContent {
        id: BlockId,
        old_content: Content,
        new_content: Content,
    },
}

impl Op {
    pub fn invert(&self) -> Op {
        match self {
            Self::InsertBlock { after, block } => Self::DeleteBlock {
                after: *after,
                block: block.clone(),
            },
            Self::DeleteBlock { after, block } => Self::InsertBlock {
                after: *after,
                block: block.clone(),
            },
            Self::UpdateBlockContent {
                id,
                old_content,
                new_content,
            } => Self::UpdateBlockContent {
                id: *id,
                old_content: new_content.clone(),
                new_content: old_content.clone(),
            },

            Self::UpdateBlockKind {
                id,
                old_kind,
                new_kind,
            } => Self::UpdateBlockKind {
                id: *id,
                old_kind: new_kind.clone(),
                new_kind: old_kind.clone(),
            },
        }
    }
}
