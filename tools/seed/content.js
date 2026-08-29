// The seed corpus: long-form technical writing across compilers, Rust, data
// structures, databases and systems programming.
//
// Two constraints shape it beyond "be interesting". The link graph needs
// genuine cross-topic edges rather than a star around one hub, so articles
// cite each other by title. And every block kind in RFC-001's grammar has to
// appear somewhere, because a corpus that only exercises paragraphs and
// headings will not surface a rendering bug in a toggle or a todo list.
//
// Articles are deliberately long — production length, several screens each —
// so the reader, the outline rail, the scroll position and the block-count
// readouts all have something real to work with.
//
// Block shorthand:
//   ['p',    text]                     paragraph
//   ['h1'|'h2'|'h3', text]             heading
//   ['quote', text]                    quote (a container; text is its first child)
//   ['code', lang, text]               code block
//   ['div']                            divider
//   ['ul'|'ol'|'todo', [items]]        list; todo items may be ['x', text] for checked
//   ['toggle', summary, [children]]    collapsible
//   ['callout', tone, text]            callout — tone: info | tip | warn | success
//   ['aside', text]                    margin note
//   ['img', caption]                   image placeholder (no asset pipeline yet)

module.exports = [
  // ── compilers & the editor front end ────────────────────────────────────
  {
    title: 'Front end, end to end',
    topic: 'interface', tags: ['compilers', 'parsing', 'ast'],
    blocks: [
      ['p', `A compiler front end is four passes that each throw information away. The trick is knowing which information, and when you will want it back. Almost every painful bug in a front end is a pass discarding something a later pass needed, discovered six months after the discard was written and nowhere near it.`],
      ['aside', `Everything here applies to an editor as much as a compiler. This notebook parses markdown-ish input on every keystroke, and the same four passes are in there, just smaller.`],
      ['h2', `The pipeline`],
      ['ol', [`Lexing — bytes to tokens, discarding whitespace and comments`,
              `Parsing — tokens to a tree, discarding token positions unless you deliberately kept them`,
              `Lowering — tree to IR, discarding syntax`,
              `Emission — IR to output, discarding names`]],
      ['p', `Every arrow is lossy, and the losses are the point. A pass that preserved everything would be a copy, and a pipeline of copies optimises nothing. See [[Lexing is a state machine]] for why the first arrow is the cheapest one, and [[Anchors vs offsets]] for what happens when you throw away positions you turn out to need.`],
      ['callout', 'warn', `A parser that cannot advance past a malformed token will hang. Emit a recovery token and keep going — a diagnostic pointing at the right line is worth more than a halt pointing at nothing.`],
      ['h3', `Tokens are a flat tape`],
      ['p', `The token stream wants to be an array, not a linked structure. You will scan it far more often than you mutate it, and every scan is a cache question before it is an algorithm question. A Vec of small Copy tokens with interned identifiers beats a Vec of enums carrying String almost regardless of what the parser then does.`],
      ['code', 'rust', `enum Token {
    Ident(Symbol),   // interned — 4 bytes, not a String
    Int(u64),
    Punct(char),
    Eof,
}

// 16 bytes, Copy, no allocation per token.
struct Spanned {
    tok: Token,
    lo: u32,         // byte offset into the source
    hi: u32,
}`],
      ['p', `Symbol is an index into a side table. The parser compares identifiers by integer equality, the diagnostics renderer looks the string back up once at the end, and the hot loop never touches the heap. That is the whole trick, and it is worth doing on day one because retrofitting interning through a finished parser is miserable.`],
      ['h2', `Why the IR exists`],
      ['p', `An IR is a commitment to forget syntax. Once lowered, the optimiser cannot accidentally depend on whether the author wrote a for loop or a while loop, because that distinction no longer exists in the data. That is not a limitation — it is exactly the freedom that makes the optimisation sound. If the optimiser could see the surface syntax, someone would eventually write a pass that behaved differently for two spellings of the same program, and that pass would be a bug factory.`],
      ['quote', `The AST is not the parse tree. Conflating them is the single most common way a front end becomes untestable, because the parse tree has a shape your grammar happened to produce and the AST has the shape your semantics actually want.`],
      ['h3', `Lowering is where the invariants land`],
      ['p', `The lowering pass is the right place to establish everything downstream gets to assume. Desugar there. Resolve names there. Insert the implicit returns, the coercions, the default arguments. Downstream passes should be able to state their preconditions in one sentence, and lowering is what makes that sentence true.`],
      ['ul', [`Desugaring: for loops become while loops with an explicit iterator`,
              `Name resolution: every identifier gets a binding id, or an error`,
              `Coercions: made explicit as nodes, never left implicit`,
              `Constant folding: only the obviously-safe cases; leave the rest to the optimiser`]],
      ['div'],
      ['h2', `Positions, and the thing everyone gets wrong`],
      ['p', `Keep byte offsets, not line and column. Lines and columns are a rendering concern — they depend on tab width, on whether you count UTF-16 code units or Unicode scalar values, and on decisions the front end has no business making. A byte offset into the original buffer is unambiguous, cheap to store in a u32, and convertible to a line and column at the exact moment a human needs to read one.`],
      ['callout', 'tip', `Store a sorted array of line-start offsets once, after lexing. Converting an offset to a line is then a binary search, and you never need to carry line numbers through the pipeline at all.`],
      ['code', 'rust', `pub struct LineMap { starts: Vec<u32> }

impl LineMap {
    pub fn line_col(&self, offset: u32) -> (u32, u32) {
        // partition_point: index of the first start > offset
        let line = self.starts.partition_point(|&s| s <= offset) - 1;
        (line as u32 + 1, offset - self.starts[line] + 1)
    }
}`],
      ['p', `The same structure serves the editor. When a diagnostic needs to be painted next to a block, the block knows its offset range and the line map turns that into a screen position — no diagnostic ever stores a line number that can go stale when the text above it changes.`],
      ['h2', `Error recovery is a feature, not a fallback`],
      ['p', `A batch compiler can stop at the first error and still be useful. An editor cannot. The document is almost always mid-edit and therefore almost always invalid, so a front end that only produces a tree for valid input produces a tree approximately never. The parser has to return something structured for input that is definitively wrong, and the quality of that something is what the whole editing experience rests on.`],
      ['ol', [`Panic-mode: on error, skip tokens until a synchronising token (a newline, a closing brace)`,
              `Insert-and-continue: pretend the missing token was there, record the fabrication`,
              `Error nodes: put an explicit Error node in the tree so later passes can skip that subtree deliberately`]],
      ['p', `The third is the one worth the effort. An explicit Error node means every later pass has a single obvious branch for "this subtree is broken", rather than each pass inventing its own guess about whether the tree it received is trustworthy.`],
      ['toggle', `What this front end deliberately does not do`, [
        ['p', `No SSA construction, no register allocation, no instruction selection. Those belong to a back end, and this notebook has none — it lowers to a block tree that gets rendered, not to machine code.`],
        ['p', `No incremental reparse of the whole document either. Blocks are the incrementality boundary: one block reparses, the rest are untouched. [[The block tree is the document]] explains why the boundary sits there.`],
      ]],
      ['img', `The four passes, with the discarded information annotated on each arrow`],
      ['h2', `Testing the thing`],
      ['p', `Front ends are unusually testable if you commit to one discipline: every pass takes a value and returns a value, and nothing reads global state. Then a test is a literal input and a literal expected output, and you can snapshot the intermediate representations at every stage.`],
      ['todo', [['x', `Golden tests: source in, pretty-printed AST out, diffed`],
                ['x', `Round-trip: parse then print then parse, assert the second tree equals the first`],
                ['x', `Recovery corpus: a file of deliberately broken inputs that must not panic`],
                `Fuzzing the lexer against a byte-level generator`,
                `Property test: every token span is within bounds and non-overlapping`]],
      ['callout', 'success', `The round-trip test is the highest-value test in a front end. It catches dropped nodes, mangled precedence and lost trivia in one assertion, and it needs no expected output written by hand.`],
      ['p', `Once the round trip holds, adding a syntactic construct becomes mechanical rather than nerve-wracking, which is the actual goal. A front end you are afraid to change is one that stops growing.`],
    ],
  },
  {
    title: 'Lexing is a state machine',
    topic: 'interface', tags: ['compilers', 'parsing'],
    blocks: [
      ['p', `A lexer is a deterministic finite automaton with an output tape. That framing is not an analogy or a teaching device — it is how you should implement it, because it tells you the cost up front: one pass, one byte at a time, no backtracking, no allocation in the common case.`],
      ['aside', `The moment a lexer needs to look ahead an unbounded distance, it has stopped being a lexer and become a parser wearing a lexer's name.`],
      ['h2', `The shape`],
      ['p', `Every lexer worth reading has the same skeleton: a position, a state, and a loop that reads one byte and decides the next state. Everything else is bookkeeping. The temptation is to reach for regular expressions, and for a prototype that is fine, but a hand-written state machine is typically an order of magnitude faster and — more importantly — gives you exact control over error recovery, which regex engines do not.`],
      ['code', 'go', `for {
    switch l.state {
    case stateStart:
        c := l.next()
        switch {
        case c == 0:
            l.emit(EOF); return
        case isSpace(c):
            // stay in stateStart, discard
        case isDigit(c):
            l.state = stateNumber
        case isIdentStart(c):
            l.state = stateIdent
        default:
            l.emit(Punct(c))
        }
    case stateIdent:
        if !isIdentContinue(l.peek()) {
            l.emit(Ident(l.intern()))
            l.state = stateStart
            continue
        }
        l.next()
    }
}`],
      ['p', `Note what is absent: no regex, no substring allocation, no lookahead beyond a single peek. The identifier is interned directly from the byte slice, so the only allocation in the whole loop happens when a genuinely new identifier appears.`],
      ['h3', `One byte, or one character?`],
      ['p', `Bytes. Always bytes. UTF-8 has the property that no multi-byte sequence contains a byte that could be mistaken for ASCII, which means a lexer can scan bytes and treat any byte with the high bit set as "part of an identifier" without decoding anything. Decoding is a rendering concern.`],
      ['callout', 'warn', `If you scan runes instead of bytes, you pay a decode on every character of every token, and your offsets become code-point indices that disagree with everything else in the system. [[Anchors vs offsets]] covers what that disagreement costs later.`],
      ['div'],
      ['h2', `Where lexers actually get hard`],
      ['p', `The state machine is easy. The hard parts are the three places where the lexer needs context it is not supposed to have.`],
      ['ol', [`Nested comments — a DFA cannot count, so you carry an explicit depth counter and accept that this state is not regular`,
              `String interpolation — the string body contains expressions, so the lexer must re-enter expression mode and remember to come back`,
              `Contextual keywords — whether a word is a keyword may depend on the parse, so the lexer emits identifiers and lets the parser decide`]],
      ['p', `The third is the one to internalise: when in doubt, emit the more general token and let the parser specialise. A lexer that tries to be clever about context creates a coupling that makes both halves harder to change.`],
      ['toggle', `The unterminated-string trap`, [
        ['p', `A string that never closes will consume the rest of the file, and every subsequent diagnostic will be nonsense. The fix is to terminate the string at the newline, emit it as an error token, and carry on lexing the next line normally.`],
        ['p', `This costs one comparison per byte in the string scanner and turns a catastrophic cascade into a single localised error. It is the highest-return five lines in the whole lexer.`],
      ]],
      ['h2', `The editor case`],
      ['p', `Input rules in an editor are the same machine run backwards. When someone types a space after "## ", something must decide that the block is now a heading, and the cheapest correct answer is a bounded scan of the bytes immediately behind the caret. [[Input rules are a bounded scan]] argues that bounded is the entire difference between typing being free and typing being a parse.`],
      ['todo', [['x', `Handle unterminated strings without hanging`],
                ['x', `Emit byte offsets for diagnostics`],
                ['x', `Intern symbols instead of allocating a String per identifier`],
                `Fuzz against a byte generator, asserting no panic and no infinite loop`,
                `Benchmark: bytes per second on a 10 MB input, tracked in CI`]],
      ['quote', `A lexer is the only part of a compiler where the constant factor genuinely matters, because it is the only part that touches every byte.`],
      ['callout', 'tip', `Measure in bytes per second, not tokens per second. Tokens per second hides the fact that a comment-heavy file has far fewer tokens per byte, and it is the bytes you are paying for.`],
      ['p', `A hand-written lexer on a modern machine should comfortably exceed a hundred megabytes per second. If yours is an order of magnitude off that, the cause is almost always allocation in the loop or a decode you did not need.`],
    ],
  },
  {
    title: 'Input rules are a bounded scan',
    topic: 'interface', tags: ['compilers', 'editor'],
    blocks: [
      ['p', `When you type a space after "## ", something has to decide that this block is now a heading. The naive implementation reparses the block. The correct one reads at most forty-eight bytes backwards and stops. The difference is not a constant factor — it is the difference between an editor that stays responsive in a long document and one that does not.`],
      ['aside', `Bytes read is the only figure on this page that does not grow with the document. That is the whole design goal, stated as a metric.`],
      ['h2', `Why not just reparse?`],
      ['p', `Because reparsing is O(block length) on every keystroke, and blocks are not bounded. Someone will paste four thousand words into a paragraph, and from then on every character they type costs a scan of four thousand words. It will not be slow enough to file a bug about and it will be slow enough to feel wrong, which is the worst possible failure mode.`],
      ['p', `The bounded scan is not an optimisation of the parser. It is a different algorithm that happens to answer one question the parser could also answer, far more slowly. Keeping that distinction clear is what stops the two implementations from drifting into each other.`],
      ['h3', `What bounded actually means`],
      ['p', `Every input rule has a maximum prefix length, known statically. The longest rule in this editor is the ordered-list marker, which is at most a few digits, a dot and a space. Forty-eight bytes is a generous ceiling that no rule approaches, chosen so the number never has to be revisited.`],
      ['code', 'ts', `const MAX_RULE_BYTES = 48;

export function matchInputRule(text: string, caret: number): Rule | null {
  const from = Math.max(0, caret - MAX_RULE_BYTES);
  const window = text.slice(from, caret);
  for (const rule of RULES) {
    const m = rule.pattern.exec(window);
    // must end exactly at the caret, or it is not an input rule
    if (m && from + m.index + m[0].length === caret) return rule;
  }
  return null;
}`],
      ['callout', 'info', `The anchoring check — that the match ends exactly at the caret — is what stops a rule firing on text the user typed ten minutes ago and has since scrolled past.`],
      ['h2', `The rules themselves`],
      ['ul', [`"# " through "### " — headings, level from the run length`,
              `"- " and "* " — bulleted list`,
              `"1. " — numbered list, the number itself discarded`,
              `"[] " and "[x] " — todo item, checked state from the brackets`,
              `"> " — quote`,
              `"\`\`\` " — code block, the following word taken as the language`,
              `"--- " — divider`]],
      ['p', `Each maps to a block kind in the grammar. That correspondence is deliberate: an input rule that produced something not expressible as a block would be a rule that produces a document the collaboration layer cannot replicate.`],
      ['div'],
      ['h2', `Undo is the hard part`],
      ['p', `An input rule fires on a keystroke, which means undo must treat the rule and the keystroke as one unit. Otherwise the user presses undo, the heading reverts to a paragraph, and the "#" characters are still gone — a state they never typed and cannot get back to except by retyping.`],
      ['callout', 'warn', `Never let an input rule be independently undoable. Coalesce it with the triggering keystroke into a single undo entry, or the undo stack will produce states the user never authored.`],
      ['p', `In a collaborative editor this is sharper still, because undo is per-actor rather than global. [[Per-actor undo without a stack]] works through why the usual stack does not survive concurrent editing, and the coalescing rule here is a precondition for that scheme rather than an independent nicety.`],
      ['toggle', `Rules that fire on paste`, [
        ['p', `Paste is not a keystroke, and running input rules across pasted content is almost always wrong — someone pasting markdown usually wants the markdown converted, and someone pasting code usually does not.`],
        ['p', `The compromise that holds up: run rules on paste only when the paste target is an empty block, and never inside a code block. Both conditions are cheap to check and together they cover the cases people actually complain about.`],
      ]],
      ['img', `The 48-byte window behind the caret, with a matched rule highlighted`],
      ['h2', `Testing`],
      ['todo', [['x', `Every rule fires at the caret and nowhere else`],
                ['x', `No rule fires inside a code block`],
                ['x', `Undo after a rule restores the literal typed characters`],
                `Property test: scan cost is constant regardless of block length`,
                `Paste into a non-empty block leaves the text verbatim`]],
      ['quote', `An input rule is a promise that typing stays free. Any implementation whose cost depends on the document has already broken it.`],
    ],
  },
  // ── Rust ────────────────────────────────────────────────────────────────
  {
    title: 'Ownership is a cost model',
    topic: 'protocol', tags: ['rust', 'ownership', 'memory'],
    blocks: [
      ['h1', `Ownership is a cost model`],
      ['p', `The usual introduction to ownership presents it as a safety mechanism: the borrow checker stops you from using memory after freeing it. That is true and it undersells the thing badly. Ownership is primarily a cost model — a way of making the expensive operations in a program visible in its type signatures — and safety is what falls out of taking that model seriously.`],
      ['aside', `If you find yourself fighting the borrow checker, you are usually fighting a design that was expensive and did not say so.`],
      ['h2', `Signatures as a price list`],
      ['p', `Read these three functions and note that you know their costs without seeing a line of the bodies.`],
      ['code', 'rust', `fn a(s: String)      -> usize   // takes ownership: caller gave up the allocation
fn b(s: &str)        -> usize   // borrows: no allocation, no free, cannot outlive s
fn c(s: &mut String) -> usize   // borrows uniquely: may mutate, may reallocate`],
      ['p', `In a language without ownership these three have the same signature and you find out which one you got by reading the implementation, or by discovering at three in the morning that it retained a reference. The Rust versions are self-documenting in the specific way that matters: they document the costs, not just the types.`],
      ['callout', 'info', `A function taking String rather than &str is announcing that it needs the allocation. If it does not need it, that signature is a tax on every caller, and callers pay it in clones they did not want.`],
      ['h3', `Clone is not a failure`],
      ['p', `There is a strain of Rust writing that treats every clone as a defeat. That is wrong and it produces worse code. A clone of a twelve-byte struct is two moves and a copy; a clone of a String in a cold path is one allocation you will never measure. The clones worth removing are the ones in hot loops and the ones that indicate a confused ownership story. The rest are fine.`],
      ['quote', `Premature borrow-checker appeasement produces the same contorted designs as premature optimisation, for the same reason: it optimises a cost nobody measured.`],
      ['div'],
      ['h2', `Lifetimes are not a duration`],
      ['p', `The single most persistent misreading is that a lifetime describes how long a value lives. It does not. A lifetime is a region of code — a set of program points over which a reference must remain valid. The compiler solves for those regions; it never reasons about wall-clock time or about when the destructor actually runs.`],
      ['p', `This is why the following compiles under non-lexical lifetimes and would not have under the older scope-based rules. The borrow's region ends at its last use, not at the closing brace.`],
      ['code', 'rust', `let mut v = vec![1, 2, 3];
let first = &v[0];
println!("{first}");     // last use of \`first\` — its region ends here
v.push(4);               // so this unique borrow is fine`],
      ['callout', 'tip', `When a lifetime error makes no sense, find the last use of the borrow rather than its declaration. The region ends there, and the error is almost always about a use you forgot was still live.`],
      ['h2', `Where the model stops helping`],
      ['p', `Ownership describes a tree. Plenty of real data is a graph, and for those the model genuinely does not have an answer that is both safe and cheap. The honest options are all compromises, and picking between them is a design decision rather than a lookup.`],
      ['ul', [`Rc / Arc — shared ownership, refcount traffic, cycles leak unless you reach for Weak`,
              `Indices into an arena — cheap and Copy, but you have hand-rolled a pointer with no validity checking`,
              `unsafe with a documented invariant — fastest, and now correctness is your problem, in writing`]],
      ['p', `The arena route is common enough in compilers to be worth its own treatment; [[Arenas and the lifetime trick]] takes it apart, including the part where the borrow checker starts helping again once the arena owns everything.`],
      ['toggle', `Why Rc<RefCell<T>> feels like defeat`, [
        ['p', `Because it moves aliasing checks from compile time to run time, and a panic on double-borrow is a worse failure than a compile error. It is not wrong — it is the same discipline other languages apply by convention, now enforced dynamically.`],
        ['p', `Reach for it when the graph is genuinely dynamic and small. Reach for an arena when it is large or long-lived, and for indices when the graph outlives any borrow you could name.`],
      ]],
      ['h2', `Send and Sync belong to this story`],
      ['p', `Ownership across threads is the same model with one more axis. A type is Send when the ownership can move to another thread, Sync when a shared reference can. These are not extra rules bolted on; they are the thread-level statement of exactly the aliasing discipline the borrow checker already enforces within a thread. [[Send, Sync, and what they are not]] follows that through.`],
      ['todo', [['x', `Take &str in public APIs unless the allocation is genuinely needed`],
                ['x', `Return owned values rather than borrows from constructors`],
                ['x', `Use Cow where callers legitimately split between owned and borrowed`],
                `Audit clones in the collaboration hot loop specifically`,
                `Document the invariant for every unsafe block, in the block`]],
      ['callout', 'success', `The test for a good ownership design: someone can read your public signatures and correctly predict which calls allocate. If they cannot, the design is hiding costs, whatever the borrow checker says about it.`],
    ],
  },
  {
    title: 'Arenas and the lifetime trick',
    topic: 'protocol', tags: ['rust', 'memory', 'arena'],
    blocks: [
      ['p', `An arena is a bump allocator that frees everything at once. Its appeal in a compiler is not primarily speed, though it is fast — it is that the ownership story collapses from a graph into a single owner, and the borrow checker starts cooperating again.`],
      ['h2', `The trick`],
      ['p', `Allocate every node from one arena, and hand out shared references tied to the arena's lifetime. Now every node lives exactly as long as every other node, cycles are free because nothing is refcounted, and the compiler statically prevents any reference escaping the arena's scope.`],
      ['code', 'rust', `struct Arena<T> { chunks: RefCell<Vec<Vec<T>>> }

impl<T> Arena<T> {
    fn alloc(&self, value: T) -> &T {
        // &self, not &mut self — many allocations, no unique borrow
        // ...push into the current chunk, never reallocating a live one
    }
}

// Every node borrows from 'a, so none can outlive the arena.
struct Node<'a> {
    kind: NodeKind,
    children: Vec<&'a Node<'a>>,
}`],
      ['callout', 'warn', `The chunks must never be reallocated while a reference is live, which is why the implementation is a Vec of chunks rather than one growing Vec. Growing a single Vec would move every element and invalidate every reference handed out.`],
      ['aside', `alloc takes &self and still mutates. That is the one genuinely subtle thing here, and it is why the interior mutability is load-bearing rather than incidental.`],
      ['h3', `Why &self and not &mut self`],
      ['p', `If allocation required a unique borrow, you could not hold a reference to one node while allocating the next — which is to say you could not build a tree. Interior mutability is what makes the API usable at all, and the safety argument is that we never hand out a reference into a chunk we might later move.`],
      ['div'],
      ['h2', `What you give up`],
      ['ol', [`No individual free — memory is held until the whole arena drops`,
              `Destructors: a typed arena runs them on drop, an untyped byte arena cannot`,
              `Nothing can outlive the arena, which is the constraint doing all the work and occasionally all the damage`]],
      ['p', `The third is worth sitting with. If a caller wants to keep one node after the arena dies, there is no incremental fix — they must copy it out into owned data. Designing the boundary so that copy is small and obvious is most of the work of using arenas well.`],
      ['quote', `An arena trades the ability to free one thing for the ability to stop thinking about freeing anything.`],
      ['h2', `Indices versus references`],
      ['p', `The alternative to &'a Node is a plain index into a Vec. It gives up compile-time validity checking and gains a great deal in return: the nodes become Copy, serialisable, stable across arena growth, and storable in structures that outlive any particular borrow.`],
      ['code', 'rust', `#[derive(Copy, Clone, PartialEq, Eq, Hash)]
pub struct NodeId(u32);

pub struct Ast { nodes: Vec<Node> }

impl Ast {
    fn get(&self, id: NodeId) -> &Node { &self.nodes[id.0 as usize] }
}`],
      ['callout', 'tip', `Newtype the index. NodeId(u32) and BlockId(u32) are different types even though both are integers, and that distinction has caught more bugs in practice than the arena's lifetime checking ever did.`],
      ['toggle', `When to pick which`, [
        ['p', `References when the structure is built once, read many times, and never leaves the scope — a classic single-pass compiler AST.`],
        ['p', `Indices when anything needs to persist, cross a thread boundary, or be sent over a wire. The block tree in this notebook uses indices for exactly that reason: blocks are replicated between clients, and a reference is not a thing you can replicate.`],
      ]],
      ['p', `That replication requirement is what settles it for the document model. [[The block tree is the document]] walks through the structure that results, and [[The operation log is the truth]] covers why identity has to survive a round trip through the network for any of it to work.`],
      ['todo', [['x', `Newtype every index rather than passing bare u32`],
                ['x', `Arena chunks sized so common documents fit in one`],
                `Benchmark arena versus Box for the parse-heavy path`,
                `Consider a generational index to catch use-after-clear in debug builds`]],
    ],
  },
  {
    title: 'Send, Sync, and what they are not',
    topic: 'protocol', tags: ['rust', 'concurrency'],
    blocks: [
      ['p', `Send and Sync are the two most misexplained traits in Rust, usually because the explanation reaches for threads too early. They are not about threads. They are about aliasing, and threads are simply where aliasing stops being checkable by the borrow checker alone.`],
      ['h2', `The definitions, precisely`],
      ['ul', [`Send — it is safe to move a value of this type to another thread`,
              `Sync — it is safe to share a reference to this type between threads; equivalently, &T is Send`]],
      ['p', `That second equivalence is the whole thing, and it is worth reading twice. Sync is not a separate property. It is Send, applied to the shared-reference type. Once that lands, the derivations stop needing memorisation.`],
      ['aside', `Almost every type is both, automatically. The interesting cases are the handful that are not, and each of them is not for a specific, findable reason.`],
      ['h3', `The exceptions and why`],
      ['code', 'rust', `Rc<T>       // neither: refcount is non-atomic, two threads would race it
RefCell<T>  // Send, not Sync: the borrow flag is non-atomic
Cell<T>     // Send, not Sync: same reason
Mutex<T>    // Send + Sync (if T: Send): that is its entire job
MutexGuard  // not Send: many pthread impls require unlock on the locking thread
*const T    // neither: the compiler knows nothing about what it points to`],
      ['callout', 'info', `RefCell being Send but not Sync is the clearest illustration. Moving it to another thread is fine, because only one thread has it. Sharing it is not, because the borrow flag it uses to detect aliasing is itself racy.`],
      ['h2', `Auto traits, and the negative case`],
      ['p', `Both are auto traits: the compiler implements them for your type if every field implements them. You almost never write the impl. What you occasionally do is opt out, and the opt-out is the interesting direction because it is where you assert something the compiler cannot see.`],
      ['code', 'rust', `use std::marker::PhantomData;

// A handle that is only valid on the thread that created it.
pub struct ThreadBound<T> {
    raw: *mut T,
    _not_send: PhantomData<*const ()>,   // raw pointer => not Send, not Sync
}`],
      ['callout', 'warn', `Writing \`unsafe impl Send for T\` is a claim that you have manually verified what the compiler could not. It belongs next to a comment stating the invariant, and that comment is load-bearing documentation, not decoration.`],
      ['div'],
      ['h2', `Where this bites in practice`],
      ['p', `The error that sends people searching is almost always the same: a future is not Send because something non-Send was held across an await point. The fix is nearly always to shorten the borrow rather than to change the type.`],
      ['code', 'rust', `// Does not compile: the guard is alive across .await
async fn bad(m: &Mutex<Vec<u8>>) {
    let g = m.lock().unwrap();
    send(&g).await;                 // MutexGuard is not Send
}

// Compiles: the lock is released before the await
async fn good(m: &Mutex<Vec<u8>>) {
    let data = { m.lock().unwrap().clone() };
    send(&data).await;
}`],
      ['toggle', `Why the compiler cares where the await is`, [
        ['p', `An async function compiles to a state machine, and everything live across an await becomes a field of that state machine. The executor may resume it on a different thread, so every field must be Send for the future to be Send.`],
        ['p', `That is why the error points at the type rather than the line: the type is what ended up in the generated struct.`],
      ]],
      ['p', `The same discipline appears throughout the collaboration service, where per-page state is owned by one task and reached only through a channel. That is not a Rust-specific pattern — [[Backpressure or bust]] argues it is the right shape regardless of language, and the trait system merely makes the violation a compile error rather than a rare crash.`],
      ['quote', `Send and Sync do not make concurrency safe. They make the unsafe parts spell themselves out in the type, which is a smaller claim and a much more useful one.`],
      ['todo', [['x', `Never hold a non-Send guard across an await`],
                ['x', `Prefer message passing over shared state for per-page data`],
                `Document the invariant on every unsafe impl Send`,
                `Add a compile-time assertion that the public future types are Send`]],
    ],
  },
  // ── data structures ─────────────────────────────────────────────────────
  {
    title: 'Ropes, and why we do not use one',
    topic: 'storage', tags: ['datastructures', 'rope', 'text'],
    blocks: [
      ['p', `A rope is a balanced tree of string fragments that makes insertion into the middle of a large text cheap. It is the canonical answer to "how does a text editor handle a large file", it is a genuinely elegant structure, and this editor does not use one. The reasons are worth writing down, because "use a rope" is received wisdom that stops being right once the document has structure.`],
      ['h2', `What a rope buys`],
      ['p', `Concatenation and splitting in logarithmic time, and — the underrated part — cheap persistent snapshots, because an edit shares almost every node with the version before it.`],
      ['code', 'rust', `enum Rope {
    Leaf(String),
    Node { left: Arc<Rope>, right: Arc<Rope>, weight: usize },
}
// insert = split at index, concat three ways: O(log n)`],
      ['ul', [`Insert and delete at an arbitrary offset: O(log n)`,
              `Concatenation: O(log n)`,
              `Snapshot: O(1) with structural sharing`,
              `Index to position: O(log n) via subtree weights`]],
      ['aside', `The snapshot property is why ropes and undo systems are so often described together.`],
      ['h2', `What it costs`],
      ['p', `Every one of those operations has a pointer chase per level, and the constant factor is significant. For documents under a few tens of kilobytes, a flat String with memmove beats a rope on every operation, because memmove is one of the most heavily optimised routines on the machine and the rope is chasing cache misses.`],
      ['callout', 'warn', `Benchmark before adopting a rope. The crossover is far higher than intuition suggests — typically hundreds of kilobytes in a single contiguous text — and most documents never approach it.`],
      ['div'],
      ['h2', `Why the structure decides it`],
      ['p', `The real argument is not performance. This document is not one long string. It is a tree of blocks, each holding a paragraph's worth of text, and the block boundaries already provide exactly the partitioning a rope would construct.`],
      ['ol', [`Blocks are individually small — a large paragraph is a few kilobytes, where String wins outright`,
              `Insertion between blocks is a tree operation on the block tree, not a text operation`,
              `Collaboration replicates ops on blocks, so the CRDT boundary and the block boundary must coincide`]],
      ['p', `Adding a rope inside each block would mean a tree inside a tree, with the outer tree already doing the job the inner one is for. [[The block tree is the document]] makes the case for the outer structure; once you accept it, the rope has nothing left to do.`],
      ['quote', `Choosing a rope for a block-structured document is solving a problem the document model already solved, one level down and less well.`],
      ['toggle', `When we would revisit this`, [
        ['p', `A code-block kind holding an entire source file would be a single block with hundreds of kilobytes of text, which is precisely the rope's home ground.`],
        ['p', `The likelier fix is to cap block size and split oversized blocks at import, keeping one representation rather than two. Splitting is a document-model change; a rope is a whole second text engine.`],
      ]],
      ['img', `Crossover benchmark: flat String versus rope, insertion cost against document size`],
      ['h2', `The one rope idea worth stealing`],
      ['p', `Weights. A rope stores a subtree size at each node so that offset lookup is a descent rather than a scan. The block tree does the same with cumulative block counts and character counts, which is what makes "jump to character 40,000" cheap without the text ever being contiguous.`],
      ['todo', [['x', `Cumulative character counts cached per subtree`],
                ['x', `Invalidate the cache on the path to the root, not the whole tree`],
                `Cap block size at import and split oversized blocks`,
                `Benchmark: 10 MB import, time to first paint`]],
    ],
  },
  {
    title: 'BK-trees for did-you-mean',
    topic: 'research', tags: ['datastructures', 'search', 'metric'],
    blocks: [
      ['p', `A BK-tree indexes a metric space. Given a dictionary and a misspelled word, it finds every entry within edit distance k while comparing against a small fraction of the dictionary. It is old, simple, and almost perfectly suited to spelling correction — which is the one job it should be given.`],
      ['h2', `The insight`],
      ['p', `Edit distance obeys the triangle inequality. If d(query, node) is 5 and you want candidates within 2, then any child at distance c from that node can only qualify if c lies between 3 and 7. Every other branch is provably empty and never visited.`],
      ['code', 'go', `func (n *Node) Search(q string, k int, out *[]string) {
    d := levenshtein(q, n.Word)
    if d <= k {
        *out = append(*out, n.Word)
    }
    for c := d - k; c <= d+k; c++ {   // the triangle-inequality window
        if child, ok := n.Children[c]; ok {
            child.Search(q, k, out)
        }
    }
}`],
      ['callout', 'info', `The loop bound is the entire algorithm. Everything else is bookkeeping around a tree keyed by integer distance.`],
      ['aside', `Children are keyed by exact distance from the parent, so the map is small and dense — usually fewer than twenty entries.`],
      ['h2', `Building it`],
      ['ol', [`Take the first word as the root`,
              `For each subsequent word, compute its distance to the current node`,
              `If no child exists at that distance, attach it there; otherwise descend into that child and repeat`]],
      ['p', `Insertion order affects balance and nobody bothers to fix that, because the effect is mild and the alternative is complicated. A dictionary inserted in frequency order performs slightly better in practice, for the ordinary reason that common words end up nearer the root.`],
      ['div'],
      ['h2', `What it is bad at`],
      ['ul', [`Large k — the window widens, pruning collapses, and it degenerates toward a linear scan`,
              `Non-metric similarity — anything violating the triangle inequality breaks the pruning proof outright`,
              `Prefix queries — that is a trie's job, not this one`,
              `Deletion — awkward enough that rebuilding is usually the honest answer`]],
      ['callout', 'warn', `Cap k at 2 for interactive use. At k=3 on an English dictionary you are visiting most of the tree and would be better off with a linear scan over a shortlist.`],
      ['h2', `Where it fits here`],
      ['p', `Search in this notebook is three structures with three jobs, and the discipline is refusing to let any of them drift into another's territory.`],
      ['ol', [`A trie for prefix completion as you type`,
              `Full-text search for ranked retrieval over block contents`,
              `A BK-tree for did-you-mean, consulted only when full-text returns nothing`]],
      ['p', `That last condition matters. Running fuzzy matching on every query produces confident nonsense; running it only on zero-result queries produces a suggestion exactly when the user has no other recourse. [[Full-text search is not a vector]] covers the middle structure and why ranking is a separate concern from matching.`],
      ['quote', `The triangle inequality is not a detail of the implementation. It is the only reason the structure works, and any similarity function that lacks it needs a different index.`],
      ['toggle', `Why not a symmetric-delete index?`, [
        ['p', `SymSpell is faster for k ≤ 2 — precomputed deletion variants turn lookup into hashing. It also costs an order of magnitude more memory and only supports the k you built it for.`],
        ['p', `The BK-tree stores each word once, answers any k, and is about forty lines. For a dictionary of tens of thousands of terms, that trade is comfortably right.`],
      ]],
      ['todo', [['x', `Cap k at 2 for interactive queries`],
                ['x', `Only consult the BK-tree on zero-result searches`],
                ['x', `Insert in frequency order`],
                `Benchmark: nodes visited per query, tracked against dictionary growth`,
                `Rebuild on vocabulary change rather than implementing deletion`]],
    ],
  },
  {
    title: 'LSM trees earn their write amplification',
    topic: 'storage', tags: ['datastructures', 'lsm', 'databases'],
    blocks: [
      ['p', `An LSM tree turns random writes into sequential ones by buffering in memory and flushing sorted runs to disk. It then spends the rest of its life merging those runs, and that merging is pure overhead — the same byte rewritten several times before it settles. The bargain only makes sense once you know what the alternative costs.`],
      ['h2', `The bargain`],
      ['ul', [`Write: append to a memtable, plus one WAL append. No disk seek.`,
              `Flush: when the memtable fills, write it out as one sorted, immutable run.`,
              `Read: check the memtable, then each run newest-first, until found.`,
              `Compact: merge runs to bound how many a read must consult.`]],
      ['callout', 'info', `A B-tree writes a full page to update one row. An LSM writes that row once immediately and again on each compaction it survives. Which amplifies more depends entirely on the workload, and neither is universally cheaper.`],
      ['aside', `The WAL append is not optional. Without it, a crash loses the whole memtable. [[Write-ahead logging, minimally]] covers what that append must guarantee.`],
      ['h2', `Reads are the price`],
      ['p', `A point lookup may touch every run. Two mechanisms keep that bounded, and both are essentially mandatory rather than optimisations.`],
      ['ol', [`Bloom filters per run — a negative answer skips the run without a read`,
              `Compaction — merges runs so the number stays logarithmic in the data size`]],
      ['code', 'sql', `-- The shape a read takes, conceptually:
--   memtable        (in RAM, sorted)
--   L0: run, run, run      overlapping key ranges
--   L1: run run run run    disjoint ranges, one run can match
--   L2: ...                ten times larger
-- Bloom filter first, then binary search inside the run.`],
      ['h3', `Levelled versus tiered`],
      ['ul', [`Levelled — each level holds disjoint ranges. Better reads, more write amplification.`,
              `Tiered — runs accumulate then merge wholesale. Better writes, worse reads.`,
              `Most production engines mix them: tiered at L0 where churn is highest, levelled below.`]],
      ['div'],
      ['h2', `Deletes are writes`],
      ['p', `Deleting inserts a tombstone. The key is not gone; it is shadowed by a marker that must outlive every older record for that key, or the delete un-happens when an older run resurfaces. Tombstones can only be dropped by a compaction that provably includes every run holding the key.`],
      ['callout', 'warn', `A delete-heavy workload can grow the store. If your usage is mostly deletion, an LSM is very likely the wrong structure.`],
      ['p', `The document model here has the same shape and the same trap: a deleted block becomes a tombstone in the operation log rather than an erasure, because a concurrent replica may still be referring to it. [[The operation log is the truth]] and [[Per-actor undo without a stack]] both depend on that tombstone surviving.`],
      ['quote', `Every log-structured system eventually discovers that deletion is the hard operation, because deletion is the one thing an append-only structure cannot express directly.`],
      ['toggle', `Choosing between an LSM and a B-tree`, [
        ['p', `LSM when writes dominate, keys arrive out of order, and the storage rewards sequential IO — which describes most ingestion workloads and nearly all time-series.`],
        ['p', `B-tree when reads dominate, especially range scans with predictable latency. Postgres is a B-tree and it is the right default for this application's data precisely because the read pattern is interactive.`],
      ]],
      ['todo', [['x', `Bloom filter on every run`],
                ['x', `Tiered at L0, levelled below`],
                `Measure write amplification per level, not just in aggregate`,
                `Alert on tombstone ratio above a threshold`]],
    ],
  },
  // ── databases ───────────────────────────────────────────────────────────
  {
    title: 'The operation log is the truth',
    topic: 'protocol', tags: ['databases', 'event-sourcing', 'crdt'],
    blocks: [
      ['h1', `The operation log is the truth`],
      ['p', `In this system the current document is not stored. It is derived. The authoritative record is an append-only log of operations, and the thing you read on screen is a projection of that log — a cache that can be thrown away and rebuilt at any time without loss.`],
      ['aside', `Said the other way: if the projection and the log disagree, the log is right by definition and the projection is a bug.`],
      ['h2', `Why invert it`],
      ['ol', [`Concurrent edits merge as operations. Two clients producing two states leaves you arbitrating; two clients producing two operations leaves you ordering.`,
              `History is free. Every past state is a prefix of the log — no snapshot schedule, no separate versioning system.`,
              `Undo becomes an operation rather than a rewind, which is the only formulation that survives concurrency.`,
              `Audit is inherent. Who did what, when, in order, without an audit table.`]],
      ['callout', 'info', `The projection is a cache. Anything that cannot be rebuilt from the log by replay is state you have accidentally made authoritative, and it will eventually diverge.`],
      ['h2', `What an operation has to carry`],
      ['code', 'rust', `struct Op {
    id: OpId,           // (actor, counter) — unique, and orders per actor
    parent: BlockId,    // where it applies
    after: Option<BlockId>,  // sibling ordering, not an array index
    kind: OpKind,       // InsertBlock, DeleteBlock, SetText, ...
    lamport: u64,       // total order across actors, ties broken by actor id
}`],
      ['p', `The critical field is \`after\`. An operation that says "insert at index 3" is meaningless by the time it arrives, because index 3 has moved. An operation that says "insert after block X" still means what it meant when it was written, which is the whole reason it can be applied out of order. [[Anchors vs offsets]] is that argument at length.`],
      ['h3', `Lamport clocks, briefly`],
      ['p', `Each actor keeps a counter, increments it on every local operation, and on receiving a remote one sets its counter to max(local, remote) + 1. This yields a total order consistent with causality: if A caused B, A sorts before B. Concurrent operations get an arbitrary but consistent order, broken by actor id so every replica picks the same winner.`],
      ['callout', 'warn', `Wall-clock timestamps cannot do this job. Clocks skew, and a merge that depends on them will order two operations differently on two machines, which is precisely the divergence the log exists to prevent.`],
      ['div'],
      ['h2', `Replay cost, and the snapshot compromise`],
      ['p', `Replaying from the beginning is O(operations), and the log only grows. A document edited daily for a year is hundreds of thousands of operations, and no one waits for that on open.`],
      ['ul', [`Snapshot the projection every N operations`,
              `On load: take the newest snapshot, replay only the operations after it`,
              `Keep the full log regardless — the snapshot is an optimisation, never a truncation`]],
      ['quote', `The moment you delete log entries because a snapshot exists, the snapshot has become the truth, and you have quietly given up every property you adopted the log for.`],
      ['toggle', `Compaction, when it is genuinely needed`, [
        ['p', `Operations on a block that has been deleted and whose tombstone is older than any live replica's horizon can be dropped. That requires knowing every replica's horizon, which requires every replica to report it.`],
        ['p', `Until there is a real size problem, do not. The log for a large document is a few megabytes and compression handles it. Premature compaction has cost more correctness than it has saved storage.`],
      ]],
      ['h2', `The projection`],
      ['p', `Rebuilt by folding the log. In this service it is materialised into Postgres so that queries — search, the link graph, page listings — are ordinary SQL rather than a replay. Those tables are all derived, and every one of them can be dropped and rebuilt from the log.`],
      ['code', 'sql', `-- Derived. Rebuildable. Never written except by the projector.
CREATE TABLE block_projection (
  page_id   uuid NOT NULL,
  block_id  uuid NOT NULL,
  parent_id uuid,
  sort_key  text NOT NULL,   -- fractional index, not an integer position
  kind      jsonb NOT NULL,
  text      text NOT NULL,
  PRIMARY KEY (page_id, block_id)
);`],
      ['todo', [['x', `Projection tables written only by the projector`],
                ['x', `Lamport ties broken by actor id`],
                ['x', `Snapshots every 500 ops, log retained in full`],
                `Replay-equivalence test: fold the log, compare against the live projection`,
                `Report replica horizons before attempting any compaction`]],
      ['callout', 'success', `The test that matters: drop every projection table, replay the log, and assert the result is byte-identical to what was there. If that passes, the log really is the truth.`],
    ],
  },
  {
    title: 'Isolation levels are a menu, not a ladder',
    topic: 'storage', tags: ['databases', 'transactions', 'mvcc'],
    blocks: [
      ['p', `Isolation levels are usually taught as a ladder from weak to strong, with serialisable at the top and the implication that you would always pick it if you could afford it. That framing hides the useful information, which is that each level permits a specific named set of anomalies, and the question is which of those your application actually cares about.`],
      ['h2', `The anomalies`],
      ['ul', [`Dirty read — reading a value another transaction has not committed`,
              `Non-repeatable read — reading the same row twice and getting different values`,
              `Phantom read — the same query returning a different set of rows`,
              `Write skew — two transactions each read a set, each write disjointly, and together violate an invariant neither broke alone`]],
      ['callout', 'warn', `Write skew is the one to know. It is invisible at Repeatable Read, causes no conflict, and is how "at least one doctor must be on call" ends with none.`],
      ['aside', `The SQL standard defines levels by which anomalies they forbid. Real engines then implement something adjacent and reuse the names, which is the source of most confusion.`],
      ['h2', `What Postgres actually gives you`],
      ['ol', [`Read Committed (default) — each statement sees a snapshot taken at statement start. No dirty reads. Non-repeatable reads are permitted.`,
              `Repeatable Read — one snapshot for the whole transaction. Postgres's implementation also excludes phantoms, which the standard does not require.`,
              `Serialisable — Repeatable Read plus predicate-dependency tracking, aborting transactions that would produce a non-serialisable outcome.`]],
      ['code', 'sql', `-- Write skew: passes at REPEATABLE READ, aborts at SERIALIZABLE
BEGIN ISOLATION LEVEL REPEATABLE READ;
  SELECT count(*) FROM doctors WHERE on_call;   -- both see 2
  UPDATE doctors SET on_call = false WHERE id = 1;
COMMIT;   -- the other transaction does the same for id = 2 — now zero`],
      ['p', `Postgres's Serialisable Snapshot Isolation detects that pattern and aborts one transaction with a serialisation failure. The cost is that your application must be prepared to retry, which is a real requirement and not a formality.`],
      ['callout', 'info', `Under SERIALIZABLE, retry logic is mandatory. A transaction that cannot be safely retried cannot safely run at that level.`],
      ['div'],
      ['h2', `MVCC, and why readers do not block`],
      ['p', `Multi-version concurrency control keeps old row versions rather than locking readers out. A reader takes a snapshot and sees the versions visible as of that moment; a writer creates a new version. Readers never block writers and writers never block readers, which is why Postgres holds up under mixed load.`],
      ['ul', [`Every row carries xmin and xmax — the transactions that created and deleted it`,
              `Visibility is a function of those and the reader's snapshot`,
              `Dead versions are reclaimed by vacuum, which is why long transactions cause bloat`]],
      ['quote', `A transaction left open for an hour is not idle. It is pinning every row version created since it started, and vacuum cannot reclaim any of them.`],
      ['h2', `What this service uses`],
      ['p', `Read Committed almost everywhere, because the operation log already provides the ordering guarantee that would otherwise require stronger isolation. The projection is written by a single projector per page, so the concurrent-writer case that motivates Serialisable does not arise. [[The operation log is the truth]] is doing the work the isolation level would otherwise have to.`],
      ['toggle', `Where we do reach for SERIALIZABLE`, [
        ['p', `The outbox poller, when marking events as dispatched. Two pollers racing would double-send, and the invariant is exactly the read-then-write shape that write skew attacks.`],
        ['p', `Everything else is Read Committed with an explicit unique constraint doing the enforcement, which is cheaper and fails more obviously.`],
      ]],
      ['todo', [['x', `Retry on 40001 in the outbox poller`],
                ['x', `Statement timeout set so a stuck transaction cannot pin versions indefinitely`],
                `Alert on transactions open longer than 60 seconds`,
                `Document the isolation level chosen for each service, and why`]],
    ],
  },
  {
    title: 'Full-text search is not a vector',
    topic: 'research', tags: ['databases', 'search', 'fts'],
    blocks: [
      ['p', `Lexical search and semantic search answer different questions. Full-text search asks which documents contain these terms. Vector search asks which documents are near this meaning. Treating either as a strict upgrade of the other produces a system that is worse at both.`],
      ['h2', `What FTS actually does`],
      ['ol', [`Tokenise — split text into terms`,
              `Normalise — case-fold, strip punctuation, stem or lemmatise`,
              `Index — an inverted index from term to the postings list of documents containing it`,
              `Rank — score matching documents, usually with BM25`]],
      ['code', 'sql', `-- Postgres does all four, and the index is the interesting part
CREATE INDEX block_fts ON block_projection
  USING GIN (to_tsvector('english', text));

SELECT page_id, ts_rank(to_tsvector('english', text), q) AS rank
FROM block_projection, plainto_tsquery('english', $1) q
WHERE to_tsvector('english', text) @@ q
ORDER BY rank DESC
LIMIT 20;`],
      ['callout', 'info', `GIN indexes the terms, not the rows. That is what makes a multi-term query an intersection of postings lists rather than a scan.`],
      ['aside', `Stemming is the step people underestimate. It is why searching "running" finds "run", and why searching a proper noun sometimes does not find itself.`],
      ['h3', `BM25, in one paragraph`],
      ['p', `A term is worth more when it is rare across the corpus and frequent within a document, with diminishing returns on repetition and a penalty for document length. That is the whole model. It has no notion of meaning and it does not need one — it is a statement about statistics, and it has stayed competitive for three decades because those statistics are genuinely informative.`],
      ['div'],
      ['h2', `Where it fails`],
      ['ul', [`Synonyms — "car" does not match "automobile" unless you built a thesaurus`,
              `Paraphrase — no shared terms, no match, regardless of how close the meaning is`,
              `Cross-language — different terms entirely`,
              `Short queries — too little signal for the statistics to work with`]],
      ['p', `These are exactly the cases embeddings handle well, and that is the honest argument for adding them: not as a replacement, but for the queries FTS structurally cannot answer.`],
      ['h2', `Where vectors fail`],
      ['ul', [`Exact terms — searching an error code or an identifier, where you want that literal string and nothing near it`,
              `Negation — "not deprecated" embeds close to "deprecated"`,
              `Explainability — you can show why BM25 ranked a document; you cannot for a cosine distance`,
              `Freshness — new vocabulary needs a re-embed, where an inverted index just indexes it`]],
      ['callout', 'warn', `Searching an exact identifier is the case where a pure-vector system feels broken to users, because it returns things that are merely similar when they wanted the one exact hit.`],
      ['quote', `Users searching a codebase or a notebook usually know a term that is in the document. Optimising away from exact matching optimises against the common case.`],
      ['h2', `The arrangement here`],
      ['p', `FTS is primary. It is fast, exact, explainable, and needs no model. Fuzzy matching handles typos, and only when FTS returns nothing — [[BK-trees for did-you-mean]] covers that path and why it is gated. Embeddings sit behind a separate "related pages" surface where approximate is the point, rather than being mixed into the ranked result list where it would dilute exact hits.`],
      ['toggle', `Hybrid ranking, and why we have not done it`, [
        ['p', `Reciprocal rank fusion merges two ranked lists without needing the scores to be comparable, and it is the standard answer.`],
        ['p', `It is also a tuning problem with no obvious ground truth on a corpus this size. Two clearly-labelled surfaces beat one blended list that nobody can explain, so that is what ships until there is enough usage data to evaluate a blend honestly.`],
      ]],
      ['todo', [['x', `GIN index on the block projection`],
                ['x', `BM25 via ts_rank, weighted by block kind — headings score higher`],
                ['x', `Fuzzy fallback only on zero results`],
                `Query logging, so ranking changes can be evaluated rather than guessed at`,
                `Related-pages surface backed by embeddings, kept separate from search`]],
      ['img', `Inverted index versus vector space, with a query resolving in each`],
    ],
  },
  // ── systems ─────────────────────────────────────────────────────────────
  {
    title: 'Write-ahead logging, minimally',
    topic: 'operations', tags: ['systems', 'wal', 'durability'],
    blocks: [
      ['p', `Write-ahead logging is one rule: before changing anything on disk, record the intent to change it, and make that record durable first. Everything else — checkpointing, recovery, replication — is built on that ordering, and every subtle durability bug is a place the ordering was quietly violated.`],
      ['h2', `The rule, stated exactly`],
      ['callout', 'info', `The log record describing a change must reach stable storage before the changed page does. Not at the same time. Before.`],
      ['p', `If a page reaches disk first and the machine dies, recovery finds a modified page with no record of what modified it and no way to undo it. If the log reaches disk first and the machine dies, recovery finds an intent it can replay. The asymmetry is the whole design.`],
      ['aside', `This is why it is called write-ahead. The log is ahead of the data, always, without exception.`],
      ['h2', `What durable means`],
      ['ol', [`write() moves bytes into the page cache. Not durable.`,
              `fsync() asks the kernel to push them to the device. Usually durable.`,
              `The device may have a volatile write cache of its own, which fsync may or may not flush depending on the device and the mount options.`]],
      ['callout', 'warn', `An fsync that returns an error may have already discarded the dirty pages. On Linux, retrying the fsync can return success while the data is gone — the "fsyncgate" behaviour. Treat an fsync failure as fatal and crash rather than continuing.`],
      ['code', 'go', `func (w *WAL) Append(rec []byte) error {
    if _, err := w.f.Write(framed(rec)); err != nil {
        return err
    }
    if err := w.f.Sync(); err != nil {
        // Do not retry. The dirty pages may already be gone.
        panic(fmt.Sprintf("wal: fsync failed, cannot continue: %v", err))
    }
    return nil
}`],
      ['div'],
      ['h2', `Framing and torn writes`],
      ['p', `A crash mid-append leaves a partial record. Recovery must detect that rather than parse garbage, which means every record carries its own length and a checksum, and recovery stops at the first record that fails either check.`],
      ['code', 'go', `// | length u32 | crc32 u32 | payload ... |
// Recovery reads until a record fails its CRC, then truncates there.
// A partial tail is expected after a crash — it is not corruption.`],
      ['p', `That last comment is the one people get wrong. A torn record at the tail is the normal outcome of a crash, not a sign of a damaged log. Treating it as corruption turns every unclean shutdown into an incident.`],
      ['h2', `Checkpoints`],
      ['p', `Without checkpoints, recovery replays from the beginning of time. A checkpoint records that everything up to some log position is already reflected in the data files, so recovery can start there.`],
      ['ul', [`Too frequent — constant IO for flushes nobody needed`,
              `Too rare — recovery takes minutes, and your availability target is the thing that suffers`,
              `Tune by target recovery time, not by an interval that feels tidy`]],
      ['quote', `Checkpoint frequency is a recovery-time decision wearing a throughput costume.`],
      ['h2', `In this system`],
      ['p', `The collaboration service keeps a local WAL per page so that operations acknowledged to a client survive a restart before they reach Postgres. The log is the operation log, so the WAL is not a separate concept bolted on — it is the same record, made durable earlier. [[The operation log is the truth]] and [[LSM trees earn their write amplification]] both lean on this same append.`],
      ['toggle', `Group commit`, [
        ['p', `fsync costs roughly the same for one record as for a hundred. Batching concurrent appends into one fsync trades a millisecond of latency for a large multiple in throughput.`],
        ['p', `The subtlety is that a caller must not be told its write is durable until the fsync covering it has returned. Getting that acknowledgement boundary wrong is how systems claim durability they do not have.`],
      ]],
      ['todo', [['x', `CRC on every record`],
                ['x', `Truncate at the first bad CRC on recovery`],
                ['x', `Panic on fsync failure rather than retrying`],
                `Group commit with a bounded batch window`,
                `Crash test: kill -9 under load, assert every acknowledged op survives`]],
      ['callout', 'success', `The only durability test worth trusting is kill -9 during writes, repeated, asserting that everything acknowledged is present. Anything gentler tests the happy path.`],
    ],
  },
  {
    title: 'Memory ordering is not intuition',
    topic: 'operations', tags: ['systems', 'concurrency', 'atomics'],
    blocks: [
      ['p', `Every intuition you have about the order of memory operations is a statement about source code, and the machine did not agree to run your source code. The compiler reorders. The processor reorders. The cache hierarchy makes writes visible to different cores at different times. Memory ordering is the vocabulary for constraining that, and it is the one area where reasoning by feel is reliably wrong.`],
      ['aside', `The reason lock-free code is hard is not the algorithms. It is that the failure mode is a test that passes ten million times and fails in production on different hardware.`],
      ['h2', `The orderings`],
      ['ul', [`Relaxed — atomic, but no ordering constraint against any other operation. Counters only.`,
              `Acquire — on a load. No later read or write may be reordered before it.`,
              `Release — on a store. No earlier read or write may be reordered after it.`,
              `AcqRel — both, on read-modify-write operations.`,
              `SeqCst — additionally, one total order that every thread agrees on.`]],
      ['callout', 'info', `Acquire and Release are a pair. A release store publishes; an acquire load that reads that store observes everything sequenced before it. Neither does anything useful alone.`],
      ['code', 'rust', `static READY: AtomicBool = AtomicBool::new(false);
static mut DATA: u64 = 0;

// producer
unsafe { DATA = 42; }
READY.store(true, Ordering::Release);   // publishes the write to DATA

// consumer
if READY.load(Ordering::Acquire) {      // if we see true, we see DATA = 42
    unsafe { assert_eq!(DATA, 42); }
}`],
      ['p', `With Relaxed on both, that assertion can fail on a weakly-ordered machine — ARM, POWER — and will not on x86, which is why "it works on my laptop" is worth nothing here. x86 gives you acquire-release semantics on ordinary loads and stores for free, so the bug is invisible until it runs on a phone or a server you did not test on.`],
      ['callout', 'warn', `Testing concurrency on x86 only tests the strongest common memory model. Run the same code under loom or on ARM before believing it.`],
      ['div'],
      ['h2', `When SeqCst`],
      ['p', `When you are unsure. It is the default in most languages for exactly that reason, and the cost on modern hardware is a fence on the store side, which is real but rarely the bottleneck. Reach for weaker orderings after profiling has told you this is the hot spot, not before.`],
      ['quote', `Relaxed is not a performance setting. It is a claim that no other memory operation depends on this one, and that claim is almost always harder to justify than it looks.`],
      ['h3', `The one honest use of Relaxed`],
      ['code', 'rust', `// A metrics counter. Nothing depends on ordering — only the eventual total.
HITS.fetch_add(1, Ordering::Relaxed);`],
      ['h2', `What this has to do with a notebook`],
      ['p', `Mostly that it argues against needing any of it. The collaboration service gives each page's state to a single owning task and reaches it through a channel, so there is no shared mutable state to order. The atomics that remain are counters. That is a deliberate design choice — [[Backpressure or bust]] makes the case that the channel is the right primitive anyway, and avoiding shared memory is a pleasant consequence.`],
      ['toggle', `Why not just use a Mutex everywhere?`, [
        ['p', `You should, until measurement says otherwise. A Mutex is an acquire on lock and a release on unlock, so it gives you the ordering guarantees automatically and correctly.`],
        ['p', `Lock-free structures buy latency predictability under contention, not throughput. If you are not contended, they buy nothing and cost a great deal of reasoning.`],
      ]],
      ['todo', [['x', `Default to SeqCst; justify anything weaker in a comment`],
                ['x', `Relaxed only for counters nothing reads for control flow`],
                `Run the concurrency tests under loom in CI`,
                `Test on ARM before claiming a lock-free path is correct`]],
    ],
  },
  {
    title: 'Backpressure or bust',
    topic: 'operations', tags: ['systems', 'queues', 'reliability'],
    blocks: [
      ['p', `An unbounded queue is a memory leak with a scheduling policy. It does not fix a throughput mismatch; it hides one, converting a fast, local, obvious failure into a slow, global, confusing one that arrives as an out-of-memory kill twenty minutes later.`],
      ['h2', `Little's Law, and why the queue cannot save you`],
      ['p', `The average number of items in a system equals arrival rate times average time in system. If arrivals exceed service rate, the queue grows without bound — that is arithmetic, not an implementation problem. A bigger buffer buys time, and time is precisely what you do not need when the imbalance is permanent.`],
      ['callout', 'warn', `A bounded queue that fills is telling you something true. An unbounded one that grows is telling you the same thing, later, in a form you cannot act on.`],
      ['aside', `Every unbounded queue in production is a decision to be paged at 3am instead of alerted at 3pm.`],
      ['h2', `The four honest responses`],
      ['ol', [`Block the producer — propagate the pressure upstream to whoever can actually slow down`,
              `Drop — shed load deliberately, choosing which work is expendable`,
              `Fail fast — reject with an explicit error so the caller can retry or degrade`,
              `Scale — add consumers, if the work genuinely parallelises`]],
      ['p', `Every real system does several of these at different layers. What no system can do is none of them, though an unbounded queue will let you believe otherwise for a while.`],
      ['code', 'go', `// Bounded, with an explicit policy at the boundary.
select {
case work <- job:
    // accepted
case <-time.After(50 * time.Millisecond):
    metrics.Shed.Inc()
    return ErrBusy            // fail fast, do not queue forever
}`],
      ['div'],
      ['h2', `WebSockets make this concrete`],
      ['p', `A collaboration session is a socket per client, and clients are not uniform: one is on fibre, another on a phone in a lift. Broadcasting an operation to every session at the rate the fastest client can absorb means the slow client's send buffer grows until something breaks.`],
      ['ul', [`Bound the per-connection outbound buffer`,
              `On overflow, drop the connection rather than the server`,
              `The client reconnects and resyncs from the log, which it can do because the log is the truth`]],
      ['callout', 'tip', `Dropping a slow consumer is acceptable precisely because recovery is cheap. [[The operation log is the truth]] is what makes reconnect-and-replay a complete recovery rather than a data-loss event.`],
      ['quote', `Design the slow-consumer path first. It is the one that decides whether one bad client degrades itself or degrades everyone.`],
      ['h2', `Backpressure inside the process`],
      ['p', `The same rule applies to channels between tasks. A page's owning task receives operations on a bounded channel; when it fills, the sender waits, and that wait propagates back to the socket read loop, which stops reading, which fills the TCP window, which slows the client. That chain is the mechanism — TCP flow control doing the work, provided nothing in the chain quietly buffers without limit.`],
      ['toggle', `Where the chain usually breaks`, [
        ['p', `One unbounded channel anywhere in the path defeats every bound elsewhere, because pressure stops propagating at that point and accumulates there instead.`],
        ['p', `Audit for unbounded constructors specifically. In Go that is make(chan T) with no size in a fan-in position; in Rust it is unbounded_channel. Both are reasonable in places and catastrophic in a hot path.`],
      ]],
      ['todo', [['x', `Every channel bounded, size chosen deliberately`],
                ['x', `Per-connection send buffer capped, overflow closes the connection`],
                ['x', `Shed counter exported as a metric`],
                `Load test with one deliberately slow consumer`,
                `Alert on shed rate, not just on error rate`]],
      ['callout', 'success', `The test: attach a client that reads at one message per second while others read at full speed. Server memory must stay flat and the other clients must be unaffected.`],
    ],
  },
  // ── the editor's own model ──────────────────────────────────────────────
  {
    title: 'The block tree is the document',
    topic: 'interface', tags: ['editor', 'blocks', 'crdt'],
    blocks: [
      ['h1', `The block tree is the document`],
      ['p', `There is no text buffer here. The document is a tree of blocks, each with a kind and its own short text, and every operation names a block rather than a position in a stream. That decision propagates into every other part of the system, which is why it is worth stating first and defending carefully.`],
      ['h2', `The structure`],
      ['code', 'ts', `interface Block {
  id: BlockId;             // uuid, stable for the block's whole life
  parent: BlockId | null;  // null = top level of the page
  sortKey: string;         // fractional index — see below
  kind: BlockKind;         // paragraph | heading | list | code | ...
  text: string;            // this block's own text, not its children's
}`],
      ['p', `Children are found by parent id and ordered by sort key. There is no children array, and that absence is deliberate: an array is a shared mutable structure that two concurrent inserts would both have to modify, which is precisely the conflict the model is built to avoid.`],
      ['aside', `Every field here exists because removing it breaks concurrent editing. There is no ornamentation in this struct.`],
      ['h3', `Fractional indexing`],
      ['p', `The sort key is a string, and between any two strings another string exists. Inserting between "a" and "b" yields "am"; between "am" and "b" yields "an". No renumbering, no coordination, and two clients inserting in the same gap produce different keys that both sort sensibly.`],
      ['code', 'ts', `midpoint("a", "b")   // "am"
midpoint("am", "b")  // "an"
midpoint("a", "am")  // "ag"   — always room, indefinitely`],
      ['callout', 'warn', `Keys grow when insertions repeatedly target the same gap — a client pasting a hundred blocks one at a time in the same place. Cap the length and renormalise the page when the cap is hit, which is a rare, explicit, whole-page operation.`],
      ['div'],
      ['h2', `Why a tree and not a list`],
      ['ol', [`Nesting is real content — list items inside lists, blocks inside toggles and callouts`,
              `Collapse and outline operate on subtrees, and a flat list would need to infer them`,
              `Deleting a container should take its children, which is a tree operation stated once`]],
      ['p', `A flat list with an indent integer is the common alternative and it appears simpler until you delete a block and have to decide what happens to everything indented under it. The tree answers that question by construction.`],
      ['h2', `What it costs`],
      ['ul', [`Rendering needs a traversal, not a loop — mitigated by materialising order in the projection`,
              `Text spanning several blocks is harder: selection, find-and-replace and paste all cross boundaries`,
              `Very large single blocks are still a flat string, with no rope beneath them — [[Ropes, and why we do not use one]] argues that is the right trade`]],
      ['quote', `Choosing blocks over a buffer trades hard text problems for easy tree problems, and pays for it with selection.`],
      ['h3', `Selection is the real tax`],
      ['p', `A selection from the middle of one block to the middle of another is not a range in any single structure. It is a start anchor, an end anchor, and every block between them — which is why anchors have to be identifiers rather than offsets. [[Anchors vs offsets]] is that argument in full.`],
      ['toggle', `Why not ProseMirror's flat model?`, [
        ['p', `ProseMirror uses a flat position space over a tree, which makes selection and transforms elegant and makes every position a number that shifts when anything before it changes.`],
        ['p', `Shifting positions are exactly what breaks under concurrent editing, where an operation may arrive after the positions it referenced have moved. Identifiers do not shift, so identifiers won.`],
      ]],
      ['img', `A page as a block tree, with sort keys shown on the edges`],
      ['todo', [['x', `Fractional sort keys with a length cap`],
                ['x', `Deleting a container tombstones its subtree`],
                ['x', `Projection materialises traversal order for rendering`],
                `Renormalise sort keys when the cap is hit`,
                `Selection model spanning block boundaries, tested against paste`]],
    ],
  },
  {
    title: 'Anchors vs offsets',
    topic: 'protocol', tags: ['crdt', 'anchors', 'editor'],
    blocks: [
      ['p', `An offset is a number counted from the start of something. An anchor is a reference to a thing. In a single-user editor the two are interchangeable and the offset is cheaper. In a collaborative one the offset is wrong, and it is wrong in a way that produces silent corruption rather than an error.`],
      ['h2', `The failure, concretely`],
      ['p', `Two clients edit the same paragraph. A has the caret at offset 10 and types. B, concurrently, inserts five characters at offset 2. When B's operation arrives at A, offset 10 no longer refers to where A was — everything after position 2 has shifted, and A's next keystroke lands five characters from where the user is looking.`],
      ['code', 'ts', `// text: "hello world"
// A: insert "X" at offset 10       -> intends before "d"
// B: insert "beautiful " at 6      -> concurrent

// Naive application, B first:
// "hello beautiful world"
// A's offset 10 now points inside "beautiful" — wrong character entirely.`],
      ['callout', 'warn', `This does not raise an error. It writes the right character in the wrong place, and neither client can tell that anything went wrong. Corruption without a symptom is the worst class of bug in a collaborative system.`],
      ['aside', `Every operational-transform implementation exists to fix exactly this, by rewriting offsets against concurrent operations. It works, and the transform functions are notoriously difficult to get right for every pair of operation types.`],
      ['h2', `The anchor alternative`],
      ['p', `Give every insertable unit an identity that never changes, and refer to identities instead of counting. An operation says "insert after character with id X" and that sentence stays true no matter what else was inserted, deleted or reordered in the meantime.`],
      ['code', 'ts', `interface Anchor {
  blockId: BlockId;
  after: CharId | null;   // null = start of the block
}
// Stable under any concurrent edit: X is X regardless of what moved.`],
      ['ol', [`Every character gets an id — (actor, counter), assigned once`,
              `Deleted characters become tombstones, retained so anchors keep resolving`,
              `Order comes from the ids, never from array position`]],
      ['callout', 'info', `Tombstones are why anchors survive deletion. If a deleted character vanished, an anchor pointing at it would dangle and the operation referencing it would be unapplicable.`],
      ['div'],
      ['h2', `The cost`],
      ['ul', [`Memory: an id per character, though run-length encoding recovers most of it for typed-in-order text`,
              `Tombstones accumulate and can only be collected once every replica has moved past them`,
              `Converting an anchor to a screen position is a lookup, not arithmetic`]],
      ['p', `That last point is the one that shapes the renderer: it must maintain an id-to-position map for the visible region and invalidate it on edit. It is not free, but it is a local cost paid in one place, rather than a correctness hazard spread across every operation type.`],
      ['quote', `Offsets are a coordinate system that other people can move. Anchors are names, and names do not move.`],
      ['h2', `Where offsets are still fine`],
      ['p', `Within a single synchronous operation, and in anything user-visible that is regenerated on read. Diagnostics carry offsets because they are recomputed after each change. The renderer works in offsets because it recomputes each frame. What must never happen is an offset being persisted, sent over the wire, or held across an await — at any of those boundaries it can be invalidated by something it cannot see. [[Per-actor undo without a stack]] runs into exactly this when undo entries outlive the state they were recorded against.`],
      ['toggle', `Why not operational transform?`, [
        ['p', `OT works and it is what Google Docs used. It needs a central server to order operations, and a transform function for every ordered pair of operation kinds — which is quadratic in the number of kinds, and this editor has thirteen.`],
        ['p', `CRDT anchors need no central authority and no pairwise transforms. The cost is tombstones and memory, which is a cost that gets cheaper every year, unlike the cost of proving a growing matrix of transform functions correct.`],
      ]],
      ['todo', [['x', `Character ids assigned at insert, never reused`],
                ['x', `Tombstones retained; anchors resolve through them`],
                ['x', `Run-length encoding for sequentially typed runs`],
                `Id-to-position map maintained for the visible region only`,
                `Tombstone collection once every replica reports its horizon`]],
    ],
  },
  {
    title: 'Per-actor undo without a stack',
    topic: 'protocol', tags: ['crdt', 'undo', 'editor'],
    blocks: [
      ['p', `Undo in a single-user editor is a stack: push the inverse of every change, pop to undo. In a collaborative editor that stack is wrong, because it is global and undo is not. Pressing undo must reverse what you did, not whatever happened most recently.`],
      ['h2', `The problem with a shared stack`],
      ['p', `A types a paragraph. B fixes a typo elsewhere. A presses undo. With a global stack, A removes B's fix — an edit A never made, in a place A may not be looking. The user experience is that undo is dangerous, and users respond by not using it.`],
      ['callout', 'warn', `Undo that can revert someone else's work is worse than no undo, because it silently destroys work the user did not know they were touching.`],
      ['aside', `The rule people expect, stated plainly: undo reverses my last change that has not already been undone. Nothing about anyone else's.`],
      ['h2', `Undo as an operation`],
      ['p', `The scheme that works is to stop treating undo as a rewind and treat it as a new operation that happens to invert an old one. It appends to the log like everything else, it replicates like everything else, and it is itself undoable — which is what redo becomes.`],
      ['code', 'rust', `enum OpKind {
    InsertBlock { .. },
    DeleteBlock { .. },
    SetText { .. },
    Undo { target: OpId },     // inverts a specific op, by id
}`],
      ['ol', [`Find the newest operation by this actor that is not already undone`,
              `Append an Undo naming its id`,
              `Redo is an Undo naming that Undo`]],
      ['callout', 'info', `Because Undo is an ordinary operation, it needs no special handling in replication, persistence or conflict resolution. That is the entire payoff of the reformulation.`],
      ['div'],
      ['h2', `Inverting each kind`],
      ['ul', [`InsertBlock — the inverse tombstones the block; the block is not erased, so anchors into it still resolve`,
              `DeleteBlock — the inverse clears the tombstone; the content was never gone, only hidden`,
              `SetText — the inverse is a character-level diff, expressed as inserts and deletes with ids`]],
      ['p', `Every inverse is expressible in terms the CRDT already has, which is not a coincidence — it is a design constraint that was imposed on the operation set from the start. An operation whose inverse could not be expressed would be an operation that broke undo.`],
      ['h3', `The interaction with anchors`],
      ['p', `Inverting SetText requires naming the characters to restore, and naming them requires their ids, which requires the tombstones. This is the point where [[Anchors vs offsets]] stops being an abstract argument and becomes load-bearing: an offset-based undo entry would refer to positions that concurrent edits have moved, and would restore text in the wrong place.`],
      ['quote', `Undo is the feature that discovers whether your document model is really identity-based. A model that cheats with offsets works until two people use it.`],
      ['h2', `What users actually expect`],
      ['ol', [`Undo reverses my last change, wherever it was`,
              `Redo reapplies it, if nothing has happened since`,
              `Typing after an undo discards the redo chain`,
              `Undo works after a reload, because the log outlived the session`]],
      ['p', `The fourth is the one this design gets for free and a stack cannot: the stack lives in memory and dies with the tab, whereas the log is durable, so undo history survives a reload without any additional persistence work.`],
      ['toggle', `Selective undo, and why we do not offer it`, [
        ['p', `Undoing an arbitrary past operation rather than the most recent one is expressible in this model — the Undo names an id, and any id would do.`],
        ['p', `It is not offered because the result is frequently incoherent: undoing an insert whose text was later edited leaves the edits orphaned. The model permits it; the interface should not, until there is a convincing presentation of what it will do.`],
      ]],
      ['todo', [['x', `Undo is an operation in the log, not a client-side stack`],
                ['x', `Per-actor: only your own operations are candidates`],
                ['x', `Redo is an Undo of an Undo`],
                ['x', `Input rules coalesce with their keystroke into one undo unit`],
                `Undo survives reload — restore the chain from the log on open`,
                `Property test: undo then redo returns the document to a byte-identical state`]],
      ['callout', 'success', `The invariant worth testing above all others: for any operation sequence, applying undo then redo yields exactly the document you started with. It catches asymmetric inverses, which are the bugs that quietly lose text.`],
    ],
  },
];
