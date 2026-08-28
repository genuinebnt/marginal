// The seed corpus: real technical writing across compilers, Rust, data
// structures, databases and systems programming — chosen so the link graph
// has genuine cross-topic edges rather than a star around one hub, and so
// every block kind in RFC-001's grammar appears somewhere.
//
// Block shorthand:
//   ['p',    text]                     paragraph
//   ['h1'|'h2'|'h3', text]             heading
//   ['quote', text]                    quote (a container; text is its first child)
//   ['code', lang, text]               code block
//   ['div']                            divider
//   ['ul'|'ol'|'todo', [items]]        list; todo items may be ['x', text] for checked
//   ['toggle', summary, [children]]    collapsible
//   ['callout', tone, text]            callout
//   ['aside', text]                    margin note
//   ['img', caption]                   image placeholder (no asset pipeline yet)

module.exports = [
  // ── compilers ───────────────────────────────────────────────────────────
  {
    title: 'Front end, end to end',
    topic: 'interface', tags: ['compilers', 'parsing', 'ast'],
    blocks: [
      ['p', 'A compiler front end is four passes that each throw information away. The trick is knowing which information, and when you will want it back.'],
      ['h2', 'The pipeline'],
      ['ol', ['Lexing — bytes to tokens, discarding whitespace and comments',
              'Parsing — tokens to a tree, discarding token positions unless you kept them',
              'Lowering — tree to IR, discarding syntax',
              'Emission — IR to output, discarding names']],
      ['p', 'Every arrow is lossy. See [[Lexing is a state machine]] for why the first one is the cheapest, and [[Anchors vs offsets]] for what happens when you throw away positions you needed.'],
      ['callout', 'warn', 'A parser that cannot advance past a malformed token will hang. Emit a recovery token and keep going — the diagnostic is more useful than the halt.'],
      ['code', 'rust', 'enum Token {\n    Ident(Symbol),\n    Int(u64),\n    Punct(char),\n    Eof,\n}'],
      ['quote', 'The AST is not the parse tree. Conflating them is the single most common way a front end becomes untestable.'],
      ['h2', 'Why the IR exists'],
      ['p', 'An IR is a commitment to forget syntax. Once lowered, the optimiser cannot accidentally depend on whether the author wrote a for loop or a while loop, which is exactly the freedom that makes optimisation sound.'],
      ['toggle', 'What we do not do here', [
        ['p', 'No SSA, no register allocation, no instruction selection. Those belong to a back end, and this notebook has none.'],
      ]],
    ],
  },
  {
    title: 'Lexing is a state machine',
    topic: 'interface', tags: ['compilers', 'parsing'],
    blocks: [
      ['p', 'A lexer is a DFA with an output tape. That framing is not an analogy — it is how you should implement it, because it tells you the cost up front: one pass, one byte at a time, no backtracking.'],
      ['code', 'go', 'for {\n    switch l.state {\n    case stateStart:\n        // one byte decides the next state\n    }\n}'],
      ['p', 'Input rules in an editor are the same shape, run backwards. [[Input rules are a bounded scan]] makes the case that a bounded lookbehind is the whole difference between typing being cheap and typing being a parse.'],
      ['todo', [['x', 'Handle unterminated strings without hanging'],
                ['x', 'Emit positions for diagnostics'],
                'Interned symbols instead of String allocations',
                'Byte offsets, not rune offsets, in the token stream']],
    ],
  },
  {
    title: 'Input rules are a bounded scan',
    topic: 'interface', tags: ['compilers', 'editor'],
    blocks: [
      ['p', 'When you type a space after "## ", something has to decide that this is now a heading. The naive implementation reparses the block. The correct one reads at most forty-eight bytes backwards and stops.'],
      ['aside', 'Bytes read is the only figure on this page that does not grow with the document.'],
      ['p', 'The bounded scan is not an optimisation of the parser. It is a different algorithm that happens to answer one question the parser could also answer, far more slowly.'],
      ['div'],
      ['p', 'Contrast with [[Front end, end to end]], where the full pipeline runs because paste genuinely needs it.'],
    ],
  },

  // ── rust ────────────────────────────────────────────────────────────────
  {
    title: 'Ownership is a cost model',
    topic: 'protocol', tags: ['rust', 'ownership', 'memory'],
    blocks: [
      ['p', 'The borrow checker is usually explained as a safety mechanism. It is more useful to read it as a cost model that happens to be enforced: every move is a memcpy you can see, every borrow is a pointer you did not have to refcount.'],
      ['h2', 'The three questions'],
      ['ul', ['Who owns this value?',
              'How long does the borrow live?',
              'Is anyone else looking at it right now?']],
      ['p', 'A design that cannot answer all three is not a Rust design yet. That is true whether or not the compiler is involved — see [[Arenas and the lifetime trick]].'],
      ['code', 'rust', 'fn longest<\'a>(a: &\'a str, b: &\'a str) -> &\'a str {\n    if a.len() > b.len() { a } else { b }\n}'],
      ['callout', 'success', 'A lifetime annotation is never a runtime cost. It is a claim the compiler checks and then erases entirely.'],
    ],
  },
  {
    title: 'Arenas and the lifetime trick',
    topic: 'protocol', tags: ['rust', 'memory', 'arena'],
    blocks: [
      ['p', 'An arena turns N frees into one. The Rust-specific part is that a single lifetime parameter can then stand for "lives as long as the arena", which collapses a graph of ownership questions into one.'],
      ['code', 'rust', "struct Ast<'a> {\n    nodes: &'a Arena<Node<'a>>,\n}"],
      ['p', 'This is the pattern behind almost every fast compiler written in Rust. [[Front end, end to end]] allocates one node per syntax construct and never frees an individual one.'],
      ['quote', 'Bump allocation is not clever. It is the absence of cleverness, which is why it is fast.'],
      ['toggle', 'When an arena is wrong', [
        ['p', 'When lifetimes are genuinely heterogeneous, an arena forces the longest one on everything. A cache is the usual counterexample.'],
      ]],
    ],
  },
  {
    title: 'Send, Sync, and what they are not',
    topic: 'protocol', tags: ['rust', 'concurrency'],
    blocks: [
      ['p', 'Send means the value can move to another thread. Sync means a shared reference can. Neither says anything about whether doing so is a good idea.'],
      ['ul', ['Send + !Sync — a Cell: safe to move, unsafe to share',
              '!Send + Sync — rare, and usually a thread-affine handle',
              '!Send + !Sync — a raw pointer, until you prove otherwise']],
      ['p', 'The op log in [[The operation log is the truth]] is Send but deliberately not Sync: one writer, many readers of the projection.'],
    ],
  },

  // ── data structures ─────────────────────────────────────────────────────
  {
    title: 'Ropes, and why we do not use one',
    topic: 'storage', tags: ['datastructures', 'rope', 'text'],
    blocks: [
      ['p', 'A rope is a balanced tree of string fragments: concatenation and split become logarithmic instead of linear. For a text editor holding one enormous buffer, that is exactly the right trade.'],
      ['p', 'It is the wrong primitive for a block editor. What we want is a tree of addressable nodes, each one a unit of intent — see [[The block tree is the document]].'],
      ['code', 'go', 'type Rope struct {\n    left, right *Rope\n    weight      int\n    frag        []byte\n}'],
      ['callout', 'info', 'We still keep a rope per block. The rope is right at the scale of a paragraph and wrong at the scale of a document.'],
      ['h2', 'Cost'],
      ['ul', ['concat — O(1) amortised',
              'split — O(log n)',
              'index — O(log n), which is worse than a flat buffer and the whole reason not to use one for short text']],
    ],
  },
  {
    title: 'BK-trees for did-you-mean',
    topic: 'research', tags: ['datastructures', 'search', 'metric'],
    blocks: [
      ['p', 'A BK-tree indexes a metric space. Given the triangle inequality, a query at distance d can prune every subtree whose edge label is outside [d-k, d+k] — which for typo correction is almost all of them.'],
      ['code', 'go', 'func (t *Tree) Query(q string, max int) []Match {\n    // triangle inequality does the pruning\n}'],
      ['p', 'It is the second use of one algorithm, not a second algorithm: the same tree answers "did you mean" in [[Full-text search is not a vector]] and near-duplicate tag detection.'],
      ['aside', 'Levenshtein is a metric. Jaro-Winkler is not, which is why it cannot go in this tree.'],
    ],
  },
  {
    title: 'LSM trees earn their write amplification',
    topic: 'storage', tags: ['datastructures', 'lsm', 'databases'],
    blocks: [
      ['p', 'An LSM trades read cost for write cost, and then spends engineering effort buying the read cost back with bloom filters and compaction policy.'],
      ['ol', ['Writes land in a memtable — sorted, in memory',
              'The memtable flushes to an immutable sorted run',
              'Runs compact into larger levels as they age',
              'Reads check every level until they hit']],
      ['p', 'The op log in [[The operation log is the truth]] has the same shape and none of the pressure: it is append-only and never compacted, because nothing ever updates an op.'],
      ['quote', 'Write amplification is not a bug in an LSM. It is the price, and the design is the argument that the price is worth paying.'],
    ],
  },

  // ── databases ───────────────────────────────────────────────────────────
  {
    title: 'The operation log is the truth',
    topic: 'protocol', tags: ['databases', 'event-sourcing', 'crdt'],
    blocks: [
      ['h2', 'One rule'],
      ['p', 'Block rows are a projection. If replay cannot reproduce them, the projection is wrong — never the log.'],
      ['callout', 'tip', 'Every op is invertible, designed in from the start. Undo is then a consequence rather than a project.'],
      ['code', 'go', 'type Op interface {\n    Invert() Op\n    isOp()\n}'],
      ['p', 'This is what lets [[Per-actor undo without a stack]] work at all, and what [[LSM trees earn their write amplification]] would call a level that never compacts.'],
      ['todo', [['x', 'Every op invertible'],
                ['x', 'Replay reproduces the projection'],
                ['x', 'Anchors, never offsets'],
                'Snapshots for faster cold replay']],
    ],
  },
  {
    title: 'Isolation levels are a menu, not a ladder',
    topic: 'storage', tags: ['databases', 'transactions', 'mvcc'],
    blocks: [
      ['p', 'Serializable is not "read committed but more". They permit different anomalies, and picking one is picking which anomaly you will debug at three in the morning.'],
      ['ul', ['Read committed — no dirty reads; non-repeatable reads are fine',
              'Repeatable read — snapshot per transaction; write skew is fine',
              'Serializable — some execution order exists; you pay for it in aborts']],
      ['p', 'MVCC makes readers cheap by making them read the past. See [[The operation log is the truth]] for the version of this idea where the past is the only thing stored.'],
      ['aside', 'Postgres calls its repeatable read "snapshot isolation". The SQL standard does not.'],
    ],
  },
  {
    title: 'Full-text search is not a vector',
    topic: 'research', tags: ['databases', 'search', 'fts'],
    blocks: [
      ['p', 'Lexical search asks whether these tokens appear. Semantic search asks whether this means the same thing. Conflating them produces a system that is bad at both.'],
      ['code', 'sql', "SELECT ts_rank(search_vector, q), ts_headline(body, q)\nFROM docs.blocks, websearch_to_tsquery('english', $1) q\nWHERE search_vector @@ q;"],
      ['p', 'Stemming is why the highlight has to come from the server: a query for "step" matches "steps", and no client-side regex over the raw query will find that.'],
      ['p', 'Fuzzy title matching is a different index again — [[BK-trees for did-you-mean]].'],
    ],
  },

  // ── systems programming ─────────────────────────────────────────────────
  {
    title: 'Write-ahead logging, minimally',
    topic: 'operations', tags: ['systems', 'wal', 'durability'],
    blocks: [
      ['p', 'A WAL is one idea: write your intent somewhere durable before you act on it. Everything else — checkpointing, truncation, group commit — is amortising the cost of that one idea.'],
      ['ol', ['Append the record',
              'fsync (this is the expensive line)',
              'Apply the change',
              'Acknowledge']],
      ['callout', 'warn', 'Step 2 is where durability claims are usually quietly dropped. An fsync that is batched is fine. An fsync that is skipped is a different product.'],
      ['p', 'Recovery replays from the last checkpoint. That is the same replay [[The operation log is the truth]] does, with a different name and a much shorter horizon.'],
    ],
  },
  {
    title: 'Memory ordering is not intuition',
    topic: 'operations', tags: ['systems', 'concurrency', 'atomics'],
    blocks: [
      ['p', 'Relaxed, Acquire, Release, SeqCst. The names suggest a ladder of strength, and the model is not a ladder — it is a set of guarantees about which reorderings are visible to whom.'],
      ['code', 'rust', 'flag.store(true, Ordering::Release);\n// ...\nif flag.load(Ordering::Acquire) {\n    // everything before the Release is visible here\n}'],
      ['quote', 'If you cannot name the pairing — which Release this Acquire synchronises with — you have not chosen an ordering, you have guessed one.'],
      ['p', 'See [[Send, Sync, and what they are not]] for the type-level half of the same problem.'],
    ],
  },
  {
    title: 'Backpressure or bust',
    topic: 'operations', tags: ['systems', 'queues', 'reliability'],
    blocks: [
      ['p', 'An unbounded queue is not a queue. It is a memory leak with a scheduling policy, and it converts a throughput problem into an outage.'],
      ['ul', ['Bounded, block — the producer feels it',
              'Bounded, drop — someone must be told what was lost',
              'Unbounded — the failure moves somewhere you cannot see it']],
      ['p', 'The outbox in [[Write-ahead logging, minimally]] is bounded by the database, which is the same discipline arrived at from the other side.'],
      ['div'],
      ['aside', 'Latency under load is the metric. Throughput at saturation tells you almost nothing.'],
    ],
  },

  // ── the product itself ──────────────────────────────────────────────────
  {
    title: 'The block tree is the document',
    topic: 'interface', tags: ['editor', 'blocks', 'crdt'],
    blocks: [
      ['h2', 'Grammar'],
      ['p', 'A page is a list of blocks. A block is an id and a kind. Some kinds hold spans, some hold other blocks, and one holds nothing at all.'],
      ['ul', ['Paragraph, Heading, Quote — spans',
              'List, Toggle, Callout — children',
              'Code — raw text, no marks, ever',
              'Divider — nothing']],
      ['callout', 'info', 'Code has no Spans in the grammar, which is why the bubble menu is unreachable inside a code block rather than merely ignored there.'],
      ['p', 'Nesting is materialised the same way pages-within-pages already are, one level deeper. [[Anchors vs offsets]] covers how a position inside all this survives a concurrent edit.'],
      ['img', 'The block tree, as drawn in RFC-001 §1'],
    ],
  },
  {
    title: 'Anchors vs offsets',
    topic: 'protocol', tags: ['crdt', 'anchors', 'editor'],
    blocks: [
      ['p', 'An integer offset is invalidated by any concurrent edit before it. An anchor names an item, so it survives one. This is the single most important correctness detail in the operation model.'],
      ['code', 'go', 'type Anchor struct {\n    Item ItemID // {Actor, Counter}\n    Bias Bias    // Before | After\n}'],
      ['callout', 'warn', 'The actor tag in an ItemID is the server process, not the editing user. It orders inserts; it does not record authorship.'],
      ['p', 'Which is also why that tag must be stable across restarts — a regenerated one produces different ItemIDs for the same historical inserts, and every replay after it fails.'],
      ['p', 'See [[The operation log is the truth]] and [[Ropes, and why we do not use one]].'],
    ],
  },
  {
    title: 'Per-actor undo without a stack',
    topic: 'protocol', tags: ['crdt', 'undo', 'editor'],
    blocks: [
      ['p', 'A shared undo stack is wrong the moment two people edit one page: your undo would revert their work. Per-actor undo filters the log by actor and inverts what it finds.'],
      ['ol', ['Find this actor\'s most recent undo group',
              'Invert each op in it, newest first',
              'Apply the inverses as new ops',
              'The log grows; nothing is ever removed']],
      ['quote', 'Undo that rewrites history is not undo. It is a second, hidden edit that nobody can audit.'],
      ['p', 'The group is why one paste undoes as one action. See [[The operation log is the truth]].'],
    ],
  },
];
