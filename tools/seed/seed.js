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
// The same service over plain HTTP — comments are REST, the op stream is not.
const COLLAB_HTTP = (process.env.SEED_COLLAB_URL || 'ws://localhost:8002').replace(/^ws/, 'http');
// notification-service is reached DIRECTLY, like collaboration-service — it is
// not behind the gateway. Its own variable, not the gateway's URL with the
// port swapped: that works on localhost and resolves to the wrong CONTAINER
// on a compose network, where each service is its own host.
const NOTIFY = process.env.SEED_NOTIFY_URL || 'http://localhost:8007';
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
  return pair.access_token;
}

/**
 * Make `target` an editor of the default space.
 *
 * Since v3.1.0 a NEW registration joins as a viewer (ADR-013), so the
 * second seeded actor cannot write the character history it exists to
 * produce — its ops would be refused by can_apply, and § 17's palimpsest
 * would quietly go back to having one deleter.
 *
 * Best-effort: this only works when the seeding account is an admin of the
 * default space, which it is on a database this seeder bootstrapped (the
 * first registration becomes its admin) and may not be otherwise. A
 * failure is logged rather than fatal — one actor is still a history.
 */
async function makeEditor(token, targetToken) {
  const sub = (t) => JSON.parse(Buffer.from(t.split('.')[1], 'base64url')).sub;
  const DEFAULT_SPACE = '00000000-0000-7000-8000-00000000d0c5';
  const r = await fetch(`${GW}/spaces/${DEFAULT_SPACE}/members/${sub(targetToken)}`, {
    method: 'PUT',
    headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' },
    body: JSON.stringify({ role: 'editor' }),
  });
  if (!r.ok) console.log(`  (second actor stays a viewer: ${r.status} — needs an admin token)`);
  return r.ok;
}

/**
 * A mention and an invitation, addressed to the seeding account.
 *
 * Best-effort throughout: a failure here logs and continues, because a
 * corpus without an inbox is still a corpus. Every step goes through the
 * public API.
 */
async function seedNotifications(token, idByTitle) {
  const sub = (t) => JSON.parse(Buffer.from(t.split('.')[1], 'base64url')).sub;
  const jf = (u, o) => fetch(u, o).then((r) => r.json()).catch(() => null);
  const me = sub(token);
  const H = { 'Content-Type': 'application/json', Authorization: `Bearer ${token}` };

  // Everything already in the inbox is marked read first, so the rows this
  // seeds are the only UNREAD ones. Notifications are not cleared with the
  // pages — there is no delete endpoint and there should not be one, since
  // an inbox that can be emptied from outside is not a record of anything —
  // so without this a re-seeded demo shows forty unread mentions from
  // corpora that no longer exist.
  await fetch(`${NOTIFY}/notifications/read-all`, {
    method: 'POST', headers: { Authorization: `Bearer ${token}` },
  }).catch(() => {});

  const other = await secondActor();
  if (!other) { console.log('  notifications: SKIPPED (no second actor)'); return; }
  const PH = { 'Content-Type': 'application/json', Authorization: `Bearer ${other}` };

  // The handle somebody would type for the seeding account: their display
  // name with the spaces removed (docs/api/notifications.md § 4).
  const DEFAULT_SPACE = '00000000-0000-7000-8000-00000000d0c5';
  const members = await jf(`${GW}/spaces/${DEFAULT_SPACE}/members`, { headers: H });
  const mine = (members?.members ?? []).find((m) => m.user_id === me);
  if (!mine) { console.log('  notifications: SKIPPED (cannot read own display name)'); return; }
  const handle = `@${mine.display_name.replace(/ /g, '')}`;

  // A MENTION. It needs a thread, and a thread needs a block with text in
  // it — a comment anchors to characters, so an empty block has nothing to
  // anchor to.
  //   Block ids come over the WebSocket, not REST: docs.blocks has no REST
  //   projection, because a block's live state belongs to the session that
  //   holds it. Same snapshot reviseBlockText above reads.
  const pageId = idByTitle.get('Anchors vs offsets') ?? [...idByTitle.values()][0];
  const block = await firstTextBlock(token, pageId);
  if (block) {
    const opened = await fetch(`${COLLAB_HTTP}/collab/pages/${pageId}/comments`, {
      method: 'POST', headers: PH,
      body: JSON.stringify({
        block_id: block.id, anchor_start: null, anchor_end: null,
        quoted: (block.text ?? '').slice(0, 60),
        body: `${handle} does the tiebreak still hold if two actors share a Lamport stamp?`,
      }),
    });
    console.log(`  mention: ${opened.ok ? 'from ' + (await secondActorName(other)) : 'FAILED ' + opened.status}`);
  } else {
    console.log('  mention: SKIPPED (no block with text)');
  }

  // An INVITATION, to a space the seeding account is not in — one it is
  // already in would be refused, correctly.
  const theirs = await jf(`${GW}/spaces`, { headers: PH });
  let space = (theirs?.spaces ?? []).find((x) => x.name === 'Research' && x.your_role === 'admin');
  if (!space) {
    space = await jf(`${GW}/spaces`, {
      method: 'POST', headers: PH, body: JSON.stringify({ name: 'Research' }),
    });
  }
  if (space?.id) {
    const r = await fetch(`${GW}/spaces/${space.id}/invitations`, {
      method: 'POST', headers: PH, body: JSON.stringify({ user_id: me, role: 'editor' }),
    });
    // 409 means one is already pending, which is the state we wanted.
    console.log(`  invitation: ${r.ok || r.status === 409 ? 'pending to Research' : 'FAILED ' + r.status}`);
  }

  // A DELETED page, so § 23c's trash and § 18b's audit have one.
  //
  // A LEAF, and not a series parent. The first version took the last page
  // in the list, which was "The Rust Porting Handbook" — the parent of a
  // 19-part series, whose deletion cascades to every part and takes § 10d
  // with it. Which page it is still does not matter; that it has nothing
  // hanging off it does.
  // Leafness comes from CONTENT, not from GET /pages — that route lists
  // ROOTS only, so every row it returns looks parentless and the first
  // version of this happily deleted the 19-part handbook.
  // A ROOT leaf: no children of its own, and no parent either. The second
  // version picked a leaf with a parent and happened to take the last part
  // of the 19-part handbook series with it, so § 10d then counted 18.
  // Deleting a page that belongs to a series changes what that series is.
  const parents = new Set(CONTENT.map((c) => c.parent).filter(Boolean));
  const hasParent = new Set(CONTENT.filter((c) => c.parent).map((c) => c.title));
  const victimTitle = [...idByTitle.keys()].reverse()
    .find((t) => !parents.has(t) && !hasParent.has(t));
  const victimId = victimTitle && idByTitle.get(victimTitle);
  if (victimId) {
    const r = await fetch(`${GW}/pages/${victimId}`, { method: 'DELETE', headers: H });
    console.log(`  trashed: ${r.ok ? victimTitle : 'FAILED ' + r.status}`);
  }
}

/** The first top-level block on a page that actually has text in it —
 *  read from the collaboration snapshot, since blocks have no REST route.
 *  A comment anchors to characters, so an empty block cannot carry one. */
async function firstTextBlock(token, pageId) {
  const ws = new WebSocket(`${COLLAB}/collab/pages/${pageId}`, ['bearer', token]);
  let found = null;
  await new Promise((resolve) => {
    ws.on('message', (raw) => {
      const m = JSON.parse(raw.toString());
      if (m.type !== 'snapshot') return;
      const blocks = (m.snapshot?.blocks ?? m.blocks ?? []).filter((b) => !b.parent);
      for (const b of blocks) {
        const text = b.content?.text ?? b.text ?? '';
        if (text.length > 20) { found = { id: b.id, text }; break; }
      }
      resolve();
    });
    ws.on('error', resolve);
    setTimeout(resolve, 8000);
  });
  try { ws.close(); } catch { /* already gone */ }
  return found;
}

async function secondActorName(token) {
  try {
    const claims = JSON.parse(Buffer.from(token.split('.')[1], 'base64url').toString());
    return claims.name ?? 'the second actor';
  } catch { return 'the second actor'; }
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
  return { actorId: sub, token: pair.access_token };
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
async function seedPage(token, pageId, blocks) {
  // The credential is a token in the subprotocol list, not an id in the URL
  // (ADR-013 §1). `ws` sends the array as Sec-WebSocket-Protocol, which is
  // exactly what a browser does.
  const ws = new WebSocket(`${COLLAB}/collab/pages/${pageId}`, ['bearer', token]);
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
async function reviseBlockText(token, pageId, blockIndex, revisions) {
  const ws = new WebSocket(`${COLLAB}/collab/pages/${pageId}`, ['bearer', token]);
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
  const { actorId, token } = await login();
  // A bearer token, not a claimed id (ADR-013 §1). The gateway ignores
  // X-Actor-Id now, so sending it would be sending nothing.
  const H = { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' };

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

    await seedPage(token, created.id, page.blocks);
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
    let ok = await reviseBlockText(token, anchorsId, 0, drafts.slice(0, 2));
    if (ok && other) {
      await makeEditor(token, other);
      ok = await reviseBlockText(other, anchorsId, 0, drafts.slice(2));
    }
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

  // 6. One of every notification kind § 20 can actually produce, so the
  //    inbox and the bell have real rows rather than a welcome and nothing.
  //
  //    Through the real paths, like everything else here: a comment
  //    containing an @handle, and a genuine pending invitation. Writing
  //    rows into notify.notifications directly would seed a table this app
  //    does not otherwise write, and would prove nothing about the two
  //    services that produce them.
  //
  //    The fourth kind — a CHECK — is not seeded at all, because it cannot
  //    be: checks are derived on every read from `[[links]]` whose target
  //    does not exist, and the corpus above already contains those.
  await seedNotifications(token, idByTitle);

  console.log(`seeded ${CONTENT.length} pages (${which})`);
})().catch((e) => { console.error('FAILED:', e.message); process.exit(1); });
