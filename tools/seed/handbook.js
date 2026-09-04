// The Rust Porting Handbook, as a seedable series.
//
// docs/porting/RUST_PORTING_HANDBOOK.md is the source. This module PARSES it
// rather than restating it, for the obvious reason: two copies of a 2,000-line
// document diverge within a week, and the one on screen would be the stale one.
//
// It produces one page per Part, plus a hub page, in the same block shorthand
// content.js uses — so the series is seeded through the identical pipeline
// (REST for the page, one op per block over the WebSocket), and the op log,
// the projection, the link graph, the FTS index and the semantic index all get
// built the way a person typing would build them.
//
// Markdown → block-kind mapping, and where it is lossy on purpose:
//
//   ## / ###          h2 / h3
//   ```lang           code block
//   > quote           quote
//   ---               divider
//   - / 1.            bulleted / numbered list
//   | a | b |         a bulleted list of "a — b" rows, plus an aside saying so.
//                     RFC-001 §10 has no Table kind and it is gated on an ADR
//                     (CLAUDE.md § Out of Scope), so a table CANNOT be seeded
//                     faithfully. Flattening it and saying so beats either
//                     inventing a block kind or dropping the content.
//   **Invariant …**   callout, tone "info"   — the load-bearing claims
//   **Before:** …     callout, tone "tip"    — prerequisites
//   **After:** …      callout, tone "note"   — how real projects solved it
//   **DSA.** …        callout, tone "info"
//   > **What this is** callout, tone "note"
//
// Inline `code`, **bold** and *italic* are stripped to plain text: the seeder
// sends InsertBlock with a Content that has no Marks, and leaving the markers
// in would render literal asterisks. [[Page links]] are preserved verbatim —
// they are the whole reason the series has a link graph.
const fs = require('fs');
const path = require('path');

const SRC = path.join(__dirname, '../../docs/porting/RUST_PORTING_HANDBOOK.md');

/** Title, topic and tags per Part. Authored here rather than derived: a
 *  topic is an owned classification (docs.topics), not something a heading
 *  can be parsed into. */
const META = [
  { n: 0,  title: 'The shape of the port',                     topic: 'operations', tags: ['rust', 'porting', 'architecture'] },
  { n: 1,  title: 'Crate layout and the workspace',            topic: 'operations', tags: ['rust', 'porting', 'cargo'] },
  { n: 2,  title: 'The data model, in Rust types',             topic: 'storage',    tags: ['rust', 'postgres', 'datamodel'] },
  { n: 3,  title: 'The document block model',                  topic: 'protocol',   tags: ['rust', 'editor', 'datastructures'] },
  { n: 4,  title: 'The grammar, in full',                      topic: 'interface',  tags: ['grammar', 'editor', 'parsing'] },
  { n: 5,  title: 'The parser and the paste pipeline',         topic: 'interface',  tags: ['compilers', 'parsing', 'rust'] },
  { n: 6,  title: 'The operation model in Rust',               topic: 'protocol',   tags: ['crdt', 'op-log', 'rust'] },
  { n: 7,  title: 'Anchors, and why offsets die',              topic: 'protocol',   tags: ['crdt', 'anchors', 'convergence'] },
  { n: 8,  title: 'Collaboration service, the stateful one',   topic: 'protocol',   tags: ['crdt', 'websocket', 'concurrency'] },
  { n: 9,  title: 'Document service: pages, projections, sagas', topic: 'storage',  tags: ['postgres', 'saga', 'projection'] },
  { n: 10, title: 'Graph algorithms, ported',                  topic: 'research',   tags: ['graphs', 'algorithms', 'datastructures'] },
  { n: 11, title: 'Search: FTS, BK-tree, trie',                topic: 'research',   tags: ['search', 'datastructures', 'algorithms'] },
  { n: 12, title: 'Semantics: vectors and HNSW',               topic: 'research',   tags: ['hnsw', 'search', 'algorithms'] },
  { n: 13, title: 'Diff, history, and the palimpsest',         topic: 'storage',    tags: ['diff', 'history', 'datastructures'] },
  { n: 14, title: 'Diagnostics and the fact DAG',              topic: 'interface',  tags: ['diagnostics', 'graphs'] },
  { n: 15, title: 'Async, concurrency, and backpressure',      topic: 'operations', tags: ['rust', 'concurrency', 'tokio'] },
  { n: 16, title: 'Errors, and the Rust idiom table',          topic: 'operations', tags: ['rust', 'errors', 'idiom'] },
  { n: 17, title: 'The wasm boundary',                         topic: 'interface',  tags: ['wasm', 'rust', 'boundary'] },
  { n: 18, title: 'Testing strategy and the order of work',    topic: 'operations',  tags: ['testing', 'porting', 'rust'] },
  { n: 19, title: 'Microservices: the parts that are not the algorithm', topic: 'operations', tags: ['microservices', 'architecture', 'rust'] },
  { n: 20, title: 'gRPC, in Rust',                             topic: 'protocol',   tags: ['grpc', 'tonic', 'rust'] },
  { n: 21, title: 'Persistence: sqlx, transactions, and the projection rule', topic: 'storage', tags: ['postgres', 'sqlx', 'transactions'] },
  { n: 22, title: 'Identity, authorization, and the rule that gets broken quietly', topic: 'operations', tags: ['auth', 'rbac', 'security'] },
  { n: 23, title: 'Security testing: what to actually test',   topic: 'operations', tags: ['security', 'testing'] },
  { n: 24, title: 'The bug catalogue',                         topic: 'operations', tags: ['testing', 'bugs', 'review'] },
  { n: 25, title: 'Sketches: HyperLogLog, Count-Min, t-digest', topic: 'research',  tags: ['sketches', 'algorithms', 'probabilistic'] },
  { n: 26, title: 'The markdown compiler, and the lexer',      topic: 'interface',  tags: ['compilers', 'parsing', 'lexer'] },
  { n: 27, title: 'The network simulator: TP1, Merkle, the causal DAG, LSM', topic: 'protocol', tags: ['crdt', 'ot', 'simulation'] },
  { n: 28, title: 'Benchmarking honestly',                     topic: 'research',   tags: ['benchmark', 'performance'] },
  { n: 29, title: 'Configuration, deployment, and observability', topic: 'operations', tags: ['deployment', 'observability'] },
  { n: 30, title: 'The order of work, with checkpoints',       topic: 'operations', tags: ['porting', 'planning'] },
  { n: 31, title: 'The frontend contract, and how the port is judged', topic: 'interface', tags: ['frontend', 'testing'] },
  { n: 32, title: 'Appendix: the file-by-file map',            topic: 'operations', tags: ['porting', 'reference'] },
];

const HUB = 'The Rust Porting Handbook';

/** Strip inline markdown the block model cannot carry as marks. [[links]] and
 *  the em dashes stay. */
function plain(s) {
  return s
    .replace(/`([^`]*)`/g, '$1')
    // Non-greedy, and bold BEFORE italic: **a *b* c** contains an asterisk,
    // so a [^*]+ body never matches it and the markers survive to the page.
    .replace(/\*\*(.+?)\*\*/g, '$1')
    .replace(/\*([^*\s][^*]*?)\*/g, '$1')
    .replace(/\[([^\]]+)\]\((?!\[)[^)]*\)/g, '$1')   // [text](url) → text, never [[x]]
    .replace(/\s+/g, ' ')
    .trim();
}

/** A markdown table becomes a bulleted list of "col1 — rest", because
 *  RFC-001 has no Table kind and inventing one here would be a block kind
 *  nothing else in the system can read. */
function tableToList(rows) {
  const cells = (r) => r.split('|').slice(1, -1).map((c) => plain(c));
  const body = rows.filter((r) => !/^\|[\s|:-]+\|$/.test(r.trim()));
  const head = cells(body[0]);
  return body.slice(1).map((r) => {
    const c = cells(r);
    if (c.length <= 1) return c[0] || '';
    const rest = c.slice(1).map((v, i) => (head[i + 1] ? `${head[i + 1]}: ${v}` : v))
                          .filter(Boolean).join(' · ');
    return `${c[0]} — ${rest}`;
  }).filter(Boolean);
}

const TONE = [
  [/^Invariant\b|^I\d|^[A-Z]\d+\.\d+\b/, 'info'],
  [/^Before:/,  'tip'],
  [/^After:/,   'note'],
  [/^DSA\./,    'info'],
  [/^Recommendation:/, 'tip'],
  [/^Known gap|^Carried-over bug|^PORT-NOTE|^Note the/, 'warn'],
];

function toneFor(text) {
  for (const [re, tone] of TONE) if (re.test(text)) return tone;
  return null;
}

/** One Part's markdown body → the block shorthand. */
function blocksFor(md) {
  const lines = md.split('\n');
  const out = [];
  let i = 0;

  const flushPara = (buf) => {
    if (!buf.length) return;
    const text = plain(buf.join(' '));
    if (!text) return;
    const tone = toneFor(text);
    if (tone) out.push(['callout', tone, text]);
    else out.push(['p', text]);
  };

  let para = [];
  while (i < lines.length) {
    const line = lines[i];

    if (/^```/.test(line)) {
      flushPara(para); para = [];
      const lang = (line.slice(3).split(',')[0] || 'text').trim() || 'text';
      const body = [];
      i++;
      while (i < lines.length && !/^```/.test(lines[i])) body.push(lines[i++]);
      i++;
      out.push(['code', lang === 'rust,ignore' ? 'rust' : lang, body.join('\n')]);
      continue;
    }

    if (/^\|/.test(line)) {
      flushPara(para); para = [];
      const rows = [];
      while (i < lines.length && /^\|/.test(lines[i])) rows.push(lines[i++]);
      const items = tableToList(rows);
      if (items.length) {
        out.push(['aside', 'A table in the source. Flattened to a list here — RFC-001 has no Table block kind, and it is gated on an ADR rather than merely unbuilt.']);
        out.push(['ul', items]);
      }
      continue;
    }

    if (/^#{2,3}\s/.test(line)) {
      flushPara(para); para = [];
      const level = line.startsWith('###') ? 'h3' : 'h2';
      out.push([level, plain(line.replace(/^#+\s*/, ''))]);
      i++; continue;
    }

    if (/^---\s*$/.test(line)) {
      flushPara(para); para = [];
      out.push(['div']);
      i++; continue;
    }

    if (/^>\s?/.test(line)) {
      flushPara(para); para = [];
      const buf = [];
      while (i < lines.length && /^>\s?/.test(lines[i])) buf.push(lines[i++].replace(/^>\s?/, ''));
      const text = plain(buf.join(' '));
      if (text) {
        const tone = toneFor(text);
        out.push(tone ? ['callout', tone, text] : ['quote', text]);
      }
      continue;
    }

    if (/^\s*(\d+)\.\s/.test(line) || /^\s*[-*]\s/.test(line)) {
      flushPara(para); para = [];
      const numbered = /^\s*\d+\.\s/.test(line);
      const items = [];
      while (i < lines.length && (/^\s*(\d+)\.\s/.test(lines[i]) || /^\s*[-*]\s/.test(lines[i]) || /^\s{2,}\S/.test(lines[i]))) {
        if (/^\s{2,}\S/.test(lines[i]) && items.length) {
          items[items.length - 1] += ' ' + plain(lines[i]);   // a wrapped item
        } else {
          items.push(plain(lines[i].replace(/^\s*(\d+\.|[-*])\s*/, '')));
        }
        i++;
      }
      // plain() again over the JOINED item: a `code span` wrapped across two
      // source lines is only a matched pair once the lines are one string.
      const kept = items.map(plain).filter(Boolean);
      if (kept.length) out.push([numbered ? 'ol' : 'ul', kept]);
      continue;
    }

    if (!line.trim()) { flushPara(para); para = []; i++; continue; }

    para.push(line);
    i++;
  }
  flushPara(para);
  return out;
}

/** Cross-links, so the series has a real graph rather than a star. Each part
 *  points back at what it depends on and forward at the next one — the
 *  handbook's own Contents table, made navigable. */
const DEPENDS = {
  1: [0], 2: [1], 3: [2], 4: [3], 5: [4], 6: [3], 7: [6], 8: [6, 7],
  9: [2, 6], 10: [1], 11: [1], 12: [10], 13: [6], 14: [3, 10],
  15: [8], 16: [], 17: [3, 10], 18: [],
};

function seriesPages() {
  const md = fs.readFileSync(SRC, 'utf8');
  // Split on the Part headings. The preface before Part 0 becomes the hub.
  const parts = md.split(/\n# Part (\d+) — /);
  const preface = parts[0];
  const pages = [];

  const titleOf = (n) => META.find((m) => m.n === n).title;

  pages.push({
    title: HUB,
    topic: 'operations',
    tags: ['rust', 'porting', 'handbook'],
    blocks: [
      ['p', `A module-by-module scaffold for hand-porting Marginals Go backend to Rust. ${META.length} parts, each self-contained enough to be worked in isolation, ordered so the invariants land before the code that has to hold them. Read Part 30 for the order to actually do it in.`],
      ['callout', 'info', 'This is not a translation of the Go code and contains no finished Rust implementations. Types, signatures, invariants, algorithms in pseudocode, and the test list that proves each one — the format .agents/agents.md §2 specifies, applied to every module at once so the shape of the whole port is visible before the first cargo new.'],
      ['h2', 'The series'],
      ['ol', META.map((m) => `Part ${m.n} — [[${m.title}]]`)],
      ['h2', 'Where to start'],
      ['p', 'Read [[The shape of the port]] once, then keep this open. The order of work is at the end, in [[Testing strategy and the order of work]] — bottom-up, because every step is verifiable against the running Go system, and because attempting [[Collaboration service, the stateful one]] first is the classic way to spend three weeks debugging a document-core bug through a WebSocket.'],
      ['quote', 'The op log is the source of truth; everything else is a projection. A bug in a projection is recoverable; a bug in the log is not.'],
      ['h2', 'The acceptance bar'],
      ['p', 'Not "the tests pass". The screen it feeds renders identically — tools/uidiff against the mockup, missing 0, on the Rust backend. That is why the TypeScript frontend is never ported: it is the oracle. See [[Testing strategy and the order of work]].'],
    ],
  });

  for (let k = 1; k < parts.length; k += 2) {
    const n = Number(parts[k]);
    const body = parts[k + 1];
    const meta = META.find((m) => m.n === n);
    if (!meta) {
      // Loud, because silent was wrong. The handbook grew from 19 parts to
      // 33 and every new one was dropped here without a word — the site
      // kept serving the old series and looked, correctly, like nothing had
      // changed. A seeder that quietly ignores its own source is worse than
      // one that fails.
      console.warn(`  handbook: Part ${n} has no META entry — SKIPPED. Add one.`);
      continue;
    }

    // Drop the part's own title line, which became the page title.
    const md2 = body.split('\n').slice(1).join('\n');
    const blocks = blocksFor(md2);

    const deps = (DEPENDS[n] || []).map((d) => `[[${titleOf(d)}]]`);
    const next = META.find((m) => m.n === n + 1);
    const nav = [];
    if (deps.length) nav.push(`Depends on ${deps.join(' and ')}.`);
    if (next) nav.push(`Next: [[${next.title}]].`);
    nav.push(`Part ${n} of the [[${HUB}]].`);

    blocks.unshift(['aside', nav.join(' ')]);
    blocks.push(['div']);
    blocks.push(['p', `Part ${n} of the [[${HUB}]].${next ? ` Continue with [[${next.title}]].` : ' This is the last part.'}`]);

    // Every part is a CHILD of the hub. The series is a book, and a book's
    // chapters are not eighteen siblings of the book.
    pages.push({ title: meta.title, parent: HUB, topic: meta.topic, tags: meta.tags, blocks });
  }

  // Preface trivia kept out of the corpus on purpose: it is navigation, and
  // the hub already carries it.
  void preface;
  return pages;
}

module.exports = seriesPages();
