// Reseeds the workspace with real content, through the real pipeline.
//
// Pages and classification go over REST; every BLOCK is an op sent on
// collaboration-service's WebSocket, exactly as the editor sends it. Nothing
// here writes to a database — so the op log, the projection, the link graph,
// the FTS index and the fact DAG all get built the way they would be by a
// person typing, and a seeding bug is a real bug.
//
// Usage:
//   node tools/seed/seed.js              base corpus only  (clears first)
//   node tools/seed/seed.js handbook     the Rust porting series only
//   node tools/seed/seed.js all          both              (clears first)
//   node tools/seed/seed.js handbook --append   add without clearing
const WebSocket = require('ws');
const { randomUUID } = require('crypto');

const args = process.argv.slice(2);
const which = args.find((a) => !a.startsWith('--')) ?? 'base';
const append = args.includes('--append');

const CORPUS = {
  base:     () => require('./content'),
  handbook: () => require('./handbook'),
  all:      () => [...require('./content'), ...require('./handbook')],
};
if (!CORPUS[which]) {
  console.error(`unknown corpus "${which}" — expected base | handbook | all`);
  process.exit(2);
}
const CONTENT = CORPUS[which]();

const GW = 'http://localhost:8000';
const COLLAB = 'ws://localhost:8002';

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

async function login() {
  const r = await fetch(`${GW}/auth/login`, {
    method: 'POST', headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ email: 'ui-demo@example.com', password: 'ui-demo-password-123' }),
  });
  const pair = await r.json();
  const sub = JSON.parse(Buffer.from(pair.access_token.split('.')[1], 'base64url')).sub;
  return sub;
}

const KIND = {
  p: { tag: 'paragraph' },
  h1: { tag: 'heading', level: 1 },
  h2: { tag: 'heading', level: 2 },
  h3: { tag: 'heading', level: 3 },
  quote: { tag: 'quote' },
  div: { tag: 'divider' },
  toggle: { tag: 'toggle' },
  ul: { tag: 'list', list_kind: 'bulleted' },
  ol: { tag: 'list', list_kind: 'numbered' },
  todo: { tag: 'list', list_kind: 'todo' },
  aside: { tag: 'aside' },
};

/** One page's blocks, sent as ops over a single live session. */
async function seedPage(actorId, pageId, blocks) {
  const ws = new WebSocket(`${COLLAB}/collab/pages/${pageId}?actor_id=${actorId}`);
  const order = [];            // top-level block ids, in order
  let ready = false;

  await new Promise((resolve, reject) => {
    ws.on('message', (raw) => {
      const m = JSON.parse(raw.toString());
      if (m.type === 'snapshot') {
        (m.blocks || []).forEach((b) => { if (!b.parent) order.push(b.id); });
        ready = true;
        resolve();
      }
    });
    ws.on('error', reject);
    setTimeout(() => (ready ? resolve() : reject(new Error('no snapshot'))), 8000);
  });

  const send = (op) => ws.send(JSON.stringify({ type: 'op', op }));

  /** Insert one block and return its id. `parent` null = top level. */
  function insert(kind, after, parent, text) {
    const id = randomUUID();
    send({ scope: 'block', type: 'InsertBlock', id, parent: parent ?? null,
           after: after ?? null, kind, content: { text: text ?? '' } });
    return id;
  }

  let last = order[order.length - 1] ?? null;

  for (const spec of blocks) {
    const [k] = spec;

    if (k === 'div') { last = insert(KIND.div, last, null, ''); await sleep(45); continue; }

    if (k === 'code') {
      const [, lang, text] = spec;
      last = insert({ tag: 'code_block', language: lang }, last, null, text);
      await sleep(45); continue;
    }

    if (k === 'img') {
      const [, caption] = spec;
      // No asset pipeline yet, so FileID is the zero uuid — the block kind is
      // real and renders as a labelled placeholder rather than being skipped.
      last = insert({ tag: 'image', file_id: '00000000-0000-0000-0000-000000000000' },
                    last, null, caption);
      await sleep(45); continue;
    }

    if (k === 'callout') {
      const [, tone, text] = spec;
      const c = insert({ tag: 'callout', tone }, last, null, '');
      insert(KIND.p, null, c, text);
      last = c; await sleep(70); continue;
    }

    if (k === 'quote' || k === 'aside') {
      const [, text] = spec;
      const c = insert(KIND[k], last, null, '');
      insert(KIND.p, null, c, text);
      last = c; await sleep(70); continue;
    }

    if (k === 'toggle') {
      const [, summary, children] = spec;
      const c = insert(KIND.toggle, last, null, summary);
      let inner = null;
      for (const [ck, ctext] of children) {
        inner = insert(KIND[ck] ?? KIND.p, inner, c, ctext);
        await sleep(40);
      }
      last = c; await sleep(60); continue;
    }

    if (k === 'ul' || k === 'ol' || k === 'todo') {
      const [, items] = spec;
      const list = insert(KIND[k], last, null, '');
      let prev = null;
      for (const item of items) {
        const checked = Array.isArray(item) && item[0] === 'x';
        const text = Array.isArray(item) ? item[1] : item;
        prev = insert({ tag: 'list_item', checked }, prev, list, text);
        await sleep(40);
      }
      last = list; await sleep(60); continue;
    }

    // paragraph / headings
    const [, text] = spec;
    last = insert(KIND[k] ?? KIND.p, last, null, text);
    await sleep(45);
  }

  await sleep(900);   // let the flush pipeline drain before dropping the socket
  ws.close();
  await sleep(120);
}

(async () => {
  const actorId = await login();
  const H = { 'X-Actor-Id': actorId, 'Content-Type': 'application/json' };

  // 1. Clear what is there, unless asked to append. DELETE is idempotent and
  //    cascades to descendants.
  if (append) {
    console.log('appending — nothing cleared');
  } else {
    const existing = await (await fetch(`${GW}/pages`, { headers: H })).json();
    for (const p of existing.pages) {
      await fetch(`${GW}/pages/${p.id}`, { method: 'DELETE', headers: H });
    }
    console.log(`cleared ${existing.pages.length} pages`);
  }

  // 2. Topics, so classification can be assigned by name.
  const topics = await (await fetch(`${GW}/topics`, { headers: H })).json();
  const topicId = Object.fromEntries(topics.topics.map((t) => [t.color_key, t.id]));

  // 3. Create, classify, then fill.
  //    `parent` names an earlier page by TITLE. Nesting is seeded rather than
  //    left flat because a page tree of 38 roots exercises none of the rail —
  //    no indent, no twisty, no lazy child load — and those are the parts most
  //    likely to be quietly broken.
  const idByTitle = new Map();
  for (const page of CONTENT) {
    const parentId = page.parent ? idByTitle.get(page.parent) : undefined;
    if (page.parent && !parentId) {
      throw new Error(`"${page.title}" names parent "${page.parent}", which is not seeded before it`);
    }
    const created = await (await fetch(`${GW}/pages`, {
      method: 'POST', headers: H,
      body: JSON.stringify({ title: page.title, parent_id: parentId }),
    })).json();
    idByTitle.set(page.title, created.id);

    await fetch(`${GW}/pages/${created.id}/topic`, {
      method: 'PUT', headers: H, body: JSON.stringify({ topic_id: topicId[page.topic] }),
    });
    for (const tag of page.tags) {
      await fetch(`${GW}/pages/${created.id}/tags`, {
        method: 'POST', headers: H, body: JSON.stringify({ tag }),
      });
    }

    await seedPage(actorId, created.id, page.blocks);
    console.log(`  ${page.title} — ${page.blocks.length} blocks`);
  }

  console.log(`seeded ${CONTENT.length} pages (${which})`);
})().catch((e) => { console.error('FAILED:', e.message); process.exit(1); });
