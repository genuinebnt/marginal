use crate::inline::Content;

#[derive(Debug, Clone, Copy, PartialEq, Eq, Hash)]
pub struct BlockId(u64);

impl BlockId {
    pub fn new(id: u64) -> Self {
        Self(id)
    }
}

#[derive(Debug, Clone, PartialEq, Eq, Hash)]
pub enum BlockKind {
    Paragraph,
    Heading { level: u8 },
    CodeBlock { language: String },
}

#[derive(Debug, Clone, PartialEq, Eq, Hash)]
pub struct Block {
    id: BlockId,
    kind: BlockKind,
    content: Content,
}

impl Block {
    pub fn new(id: BlockId, kind: BlockKind, content: Content) -> Self {
        Self { id, kind, content }
    }

    pub fn id(&self) -> BlockId {
        self.id
    }

    pub fn kind(&self) -> &BlockKind {
        &self.kind
    }

    pub fn content(&self) -> &Content {
        &self.content
    }

    pub fn update_kind(&mut self, kind: BlockKind) {
        self.kind = kind;
    }

    pub fn update_content(&mut self, content: Content) {
        self.content = content;
    }
}
