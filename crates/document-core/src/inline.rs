#[derive(Debug, Clone, PartialEq, Eq, Hash)]
pub enum MarkKind {
    Bold,
    Italic,
    Code,
    Strike,
    Link(String),
}
