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

#[derive(Debug, Clone, PartialEq, Hash)]
pub struct Content {
    text: String,
    marks: Vec<Mark>,
}

pub enum ContentError {
    NotCharBoundary { offset: usize },
    OutOfBounds { offset: usize, len: usize },
    InvertedRange { start: usize, end: usize },
}
