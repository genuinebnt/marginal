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

// Default to the local docker-compose ports; override to seed a deployed
// instance, where the services sit behind the gateway's namespaced prefixes:
//   SEED_GATEWAY_URL=https://marginal.genuinebasil.dev/api \
//   SEED_COLLAB_URL=wss://marginal.genuinebasil.dev/ws \
//   SEED_EMAIL=... SEED_PASSWORD=... node tools/seed/seed.js all
const GW = process.env.SEED_GATEWAY_URL || 'http://localhost:8000';
const COLLAB = process.env.SEED_COLLAB_URL || 'ws://localhost:8002';
const SEED_EMAIL = process.env.SEED_EMAIL || 'ui-demo@example.com';
const SEED_PASSWORD = process.env.SEED_PASSWORD || 'ui-demo-password-123';

const sleep = (ms) => new Promise((r) => setTimeout(r, ms));

/**
 * A second seeded account, so § 17's palimpsest attributes its tombstoned
 * runs to two different people. One deleter on every run demonstrates
 * nothing about attribution — it looks like the column is hardcoded.
 *
 * Register-or-login: the workspace is rebuilt nightly but accounts are not,
 * so this has to be idempotent across runs.
 */
async function secondActor() {
  const body = JSON.stringify({
    email: 'ui-demo-editor@example.com',
    password: 'ui-demo-password-123',
    display_name: 'Wren Halloway',
  });
  const H = { 'Content-Type': 'application/json' };
  let r = await fetch(`${GW}/auth/register`, { method: 'POST', headers: H, body });
  let pair = await r.json();
  if (!pair.access_token) {
    r = await fetch(`${GW}/auth/login`, {
      method: 'POST', headers: H,
      body: JSON.stringify({ email: 'ui-demo-editor@example.com', password: 'ui-demo-password-123' }),
    });
    pair = await r.json();
  }
  if (!pair.access_token) return null;   // not fatal: one actor is still a history
  return JSON.parse(Buffer.from(pair.access_token.split('.')[1], 'base64url')).sub;
}

async function login() {
  const r = await fetch(`${GW}/auth/login`, {
    method: 'POST', headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ email: SEED_EMAIL, password: SEED_PASSWORD }),
  });
  const pair = await r.json();
  if (!pair.access_token) {
    throw new Error(`login failed for ${SEED_EMAIL} at ${GW}: ${JSON.stringify(pair)}`);
  }
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

/**
 * Rewrite one block's text CHARACTER by character, several times over.
 *
 * Everything above seeds through the BLOCK tier: InsertBlock carries the
 * finished text, and a block that is written once and never edited has no
 * character history at all. § 17's palimpsest — the tombstoned character
 * array, who deleted each run and when — was therefore correct and empty on
 * every seeded page, which reads exactly like a broken panel.
 *
 * So one page gets a real revision history, made the way the editor makes
 * one: `DeleteText` over the block's current boundaries followed by an
 * `InsertText`, which is precisely what `useCollabPage.setBlockText` sends
 * when you retype a sentence. The deleted runes become tombstones because
 * they really were deleted.
 *
 * Two actors, not one — a palimpsest whose every run names the same deleter
 * demonstrates nothing about attribution.
 */
async function reviseBlockText(actorId, pageId, blockIndex, revisions) {
  const ws = new WebSocket(`${COLLAB}/collab/pages/${pageId}?actor_id=${actorId}`);
  let blockId = null;
  let boundaries = null;

  await new Promise((resolve, reject) => {
    ws.on('message', (raw) => {
      const m = JSON.parse(raw.toString());
      if (m.type === 'snapshot') {
        const blocks = (m.snapshot?.blocks ?? m.blocks ?? []).filter((b) => !b.parent);
        const b = blocks[blockIndex];
        if (b) { blockId = b.id; boundaries = b.boundaries ?? null; }
        resolve();
      }
      // The server hands back the block's new anchor range with every text
      // ack. Without it the next delete has nothing valid to name — an
      // Anchor a client invented is not one the server ever issued.
      if ((m.type === 'ack' || m.type === 'broadcast') && m.boundaries) {
        boundaries = m.boundaries;
      }
      if (m.type === 'ack' && !m.boundaries) {
        boundaries = null;   // the block was emptied; nothing to delete next
      }
      if (m.type === 'error') console.error('  revise:', m.message);
    });
    ws.on('error', reject);
    setTimeout(resolve, 8000);
  });

  if (!blockId) { ws.close(); return false; }

  const send = (op, group) =>
    ws.send(JSON.stringify({ type: 'op', op, undo_group: group ?? undefined }));

  for (const text of revisions) {
    const group = randomUUID();
    if (boundaries) {
      send({ scope: 'text', block: blockId, op: { type: 'DeleteText', range: boundaries, text: '' } }, group);
      await sleep(260);
    }
    send({ scope: 'text', block: blockId, op: { type: 'InsertText', at: null, text } }, group);
    await sleep(320);
  }

  await sleep(900);
  ws.close();
  await sleep(120);
  return true;
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

  // 4. One page gets a real CHARACTER-level revision history.
  //
  //    Everything above is block-tier: text arrives finished, inside the
  //    InsertBlock that created the block. That leaves § 17's palimpsest —
  //    the tombstoned character array — correct and empty on every page,
  //    which reads exactly like a panel that does not work.
  //
  //    "Anchors vs offsets" is the page, because its own argument is that a
  //    reference survives an edit that an offset does not, and the panel
  //    below it is that argument running.
  const anchorsId = idByTitle.get('Anchors vs offsets');
  if (anchorsId) {
    const other = await secondActor();
    const drafts = [
      'An offset is a number. An anchor is a reference to a thing.',
      'An offset is a number counted from the start. An anchor names an item.',
      'An offset is a number counted from the start of something. An anchor is a reference to a thing, so it survives an edit that invalidates the number.',
    ];
    //   The two actors alternate, so no run in the palimpsest is attributable
    //   to the same person as the one before it.
    let ok = await reviseBlockText(actorId, anchorsId, 0, drafts.slice(0, 2));
    if (ok && other) ok = await reviseBlockText(other, anchorsId, 0, drafts.slice(2));
    console.log(`  character history: ${ok ? 'Anchors vs offsets' : 'SKIPPED (block not found)'}`);
  }

  // 5. A few reading positions, so `resume` on the dashboard has something
  //    true to show. Written through the real endpoint, like everything else
  //    here — a position is view state stored per user, and seeding it any
  //    other way would be seeding a table this app does not otherwise write.
  const live = await (await fetch(`${GW}/graph`, { headers: H })).json();
  const resumable = live.nodes.slice(0, 4);
  for (const n of resumable) {
    await fetch(`${GW}/pages/${n.id}/position`, {
      method: 'PUT', headers: H,
      body: JSON.stringify({ block_id: null, caret_start: 0, caret_end: 0 }),
    });
  }
  console.log(`  resume positions: ${resumable.length}`);

  console.log(`seeded ${CONTENT.length} pages (${which})`);
})().catch((e) => { console.error('FAILED:', e.message); process.exit(1); });
