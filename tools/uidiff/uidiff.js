#!/usr/bin/env node
/**
 * uidiff — compares a running screen against its mockup, property by property.
 *
 * Screenshots hide whole missing regions: several rounds of eyeballing the
 * editor "looked fine" while its inspector was not rendering at all. This
 * loads both documents, walks them in parallel, and reports every element
 * that differs in structure OR in the properties the design system fixes.
 *
 * Text is compared only for CHROME (labels, readout keys, button captions) —
 * body content legitimately differs, since the app has real pages and the
 * mockup has samples. Comparing all text would report a difference on every
 * node and tell you nothing.
 *
 *   node tools/uidiff/uidiff.js <mockupScreenNumber> <appPath> [pageTitle]
 *   node tools/uidiff/uidiff.js 04 /pages "Sync protocol notes"
 *   node tools/uidiff/uidiff.js 05 "/read/{id}" "Sync protocol notes"
 *
 * appPath may carry a {id} placeholder for screens whose route is not under
 * /pages. Requires the stack up (docker compose up) and the dev server
 * running, plus `npm install` in tools/ for playwright-core.
 */
const path = require('node:path');
const WebSocket = require(path.join(__dirname, '..', 'node_modules', 'ws'));
// Resolved from THIS file rather than a hardcoded home directory or the
// caller's cwd, so the tool works in any checkout and from any working
// directory. Running the gate from web/ used to die with MODULE_NOT_FOUND.
const REPO = path.resolve(__dirname, '..', '..');
const { chromium } = require(path.join(REPO, 'tools', 'node_modules', 'playwright-core'));
const MOCKUP = 'file://' + path.join(REPO, 'docs', 'ui-mockups', 'v2', 'index.html');
const APP = 'http://localhost:5173';
const GW = 'http://localhost:8000';

/** The properties the design system actually fixes. Anything not here is
 *  either derived or deliberately free. */
const PROPS = [
  'fontFamily', 'fontSize', 'fontWeight', 'letterSpacing', 'lineHeight',
  'color', 'backgroundColor',
  'paddingTop', 'paddingRight', 'paddingBottom', 'paddingLeft',
  'marginTop', 'marginBottom',
  'gap', 'display', 'flexDirection', 'alignItems', 'justifyContent',
  'borderRadius', 'borderTopWidth', 'borderLeftWidth', 'borderTopColor',
  'opacity', 'textDecorationLine', 'textTransform',
];

/* Width and height are deliberately NOT compared.
 *
 * A text element's box is a function of its text, and the app's content
 * differs from the mockup's by design — the mockup says "Sync protocol
 * notes", the app says whatever the page is called. Comparing measured boxes
 * therefore compares CONTENT, not design, and reports a difference on every
 * element that renders a real string.
 *
 * Verified rather than assumed: a run flagged the nav tab as 26x12 in the
 * mockup against 49x26 in the app, and measuring the same element directly
 * gave 49x26 on BOTH. What the geometry check below covers instead is the
 * chrome the guidelines actually fix — bar height, rail and inspector width —
 * which are constants, not consequences of content. */

/**
 * Utility classes: they set ONE property and carry no role.
 *
 * `.mono` says "monospace" and nothing about what the element is, so it is
 * applied to dozens of unrelated nodes that each add their own inline size
 * and colour. Pairing the 8th `.mono` in the mockup against the 8th in the
 * app compares a topic count against a status hint and calls the difference
 * a defect — it accounted for nearly every property diff on § 10b.
 *
 * They are still checked for PRESENCE (a missing `.mono` is a missing
 * element); only their properties are skipped, because those belong to
 * whatever role class or inline style sits alongside.
 */
const UTILITY = new Set(['mono', 'lbl', 'tgrow', 'row', 'dot']);

/**
 * The three deliberate departures web/src/styles/mockup.css documents, and
 * one consequence of them.
 *
 * `.scan` is a scanline overlay with no product meaning and `.tag` is the
 * design doc's own caption strip, so neither ships. `.sc` is a fixed 1440x860
 * artboard in the mockup and a viewport in the app, which is where its
 * margin-bottom goes.
 *
 * Reported every run, these are three lines of known noise on every screen —
 * and a report with known noise in it is a report that stops being read. They
 * are suppressed HERE, once, with the reason, rather than mentally skipped
 * forty times.
 */
const IGNORED_MISSING = new Set([
  'div.scan', 'div.tag',
  // The page tree's mid-delete row. PageTreeRail really renders it
  // (lifecycle_state === 'deleting'), and § 23c's saga stages are what it
  // belongs to — but at this scale the delete saga finishes in under 200ms,
  // so no settled screenshot of the tree can ever contain one. Measured, not
  // assumed: a page with a child went from DELETE to gone before the first
  // poll came back.
  //
  // A transient is not this tool's subject. uidiff compares a screen at
  // rest; verify.js catches this one while it is happening, which is the
  // only way to catch it at all.
  'span.tr-tick.tr-tick-del',
]);

/** Text properties, which an element with no text inherits and never uses. */
const INHERITED_TEXT_PROPS = new Set([
  'color', 'fontFamily', 'fontSize', 'fontWeight', 'letterSpacing', 'lineHeight',
  'textDecorationLine', 'textTransform', 'borderTopColor',
]);
const IGNORED_PROPS = {
  'div.sc': new Set(['marginBottom']),
  // .wal is pinned by margin-top:auto, so its computed margin is the slack
  // left over by the content above it — a measurement of the corpus, not of
  // the design.
  'div.wal': new Set(['marginTop']),
  // .ping is a keyframe animation. Its opacity is whatever phase the pulse
  // happened to be in when the page was measured — a clock reading, not a
  // style.
  'div.ping': new Set(['opacity']),

  // The page tree's rows are CONTENT, and these properties are functions of
  // WHICH pages exist rather than of the design system.
  //
  // This is the same rule the tool already applies to text (see the header:
  // body content legitimately differs, so only chrome text is compared).
  // A row's bar takes its topic's colour; its title goes bold when it is the
  // open page; its number goes ember when it is the active one. Pairing the
  // mockup's fourth row against the app's fourth row therefore compares one
  // corpus's topics against another's and calls every difference a defect —
  // it was 26 of § 04's property diffs, and not one of them was about the
  // design.
  //
  // What the design DOES fix about a row — its padding, gap, font size and
  // family — is still compared.
  'span.tr-bar': new Set(['backgroundColor']),
  // The outline's rows are content for the same reason: which entry is
  // active depends on where the caret is, and a landmark's icon colour is
  // its KIND (code blue, callout amber, aside grey) — so pairing the third
  // entry against the third entry compares one page's structure with
  // another's.
  'span.oi-t': new Set(['color', 'borderTopColor']),
  'span.oi-k': new Set(['color', 'borderTopColor']),
  // A peer's caret is a 2px bar that renders no text of its own; it carries
  // its name in a child with its own explicit font. Its inherited text
  // properties are whatever it happens to be positioned over — the mockup
  // typed it inside the prose, the app overlays it — and mean nothing
  // either way.
  // display included: a real caret is an OVERLAY positioned from a measured
  // rect, so it is blockified; the mockup types one into the prose, where it
  // is inline. Same element, and the app's is the only version that can sit
  // where a peer's caret actually is.
  'span.peer-caret': new Set(['fontFamily', 'fontSize', 'lineHeight', 'color', 'borderTopColor', 'display']),
  // A readout's value takes a TONE from what it is reporting (green for
  // healthy, amber for degraded). That is the number's meaning, not the
  // design's.
  'span.rd-v': new Set(['color', 'borderTopColor']),
  'span.tr-t': new Set(['fontWeight', 'color', 'borderTopColor']),
  'span.tr-n': new Set(['color', 'borderTopColor']),
  // paddingLeft included: a row's indent is its DEPTH in the tree, which is
  // a fact about where the open page sits rather than about the design.
  'div.tr': new Set(['color', 'borderTopColor', 'textDecorationLine', 'paddingLeft']),
};

/** Chrome text worth comparing — short, uppercase-ish, structural. */
const CHROME_SELECTORS = '.lbl,.rd-k,.tb,.sb,.it,.chip,.btn,.kbd,.tpc,.wm';

/**
 * Per-screen setup, run before the app is measured.
 *
 * Without this the diff cannot tell "not built" from "not shown yet": a
 * search screen with no query has no result chips, and reporting those as
 * MISSING is reporting an empty text box. Each entry puts the screen into
 * the state its mockup depicts, so anything still missing afterwards is
 * genuinely absent.
 *
 * Keep these to interactions a person would perform. Reaching into React
 * state would prove the markup can render, not that it does.
 */
const SEED = {
  '06': async (p) => {                       // SEARCH — the mockup shows hits
    await p.keyboard.type('rope');
    await p.waitForTimeout(1600);
  },
  '14': async (p) => {                     // NETCODE — transform is OFF
    // The section argues from the transform-off case: both replicas
    // agree perfectly on a document nobody asked for. A fresh load
    // starts with it ON (the safe default), so the diff has to put
    // the screen into the state the mockup depicts.
    await p.waitForTimeout(2000);
    const chip = p.locator('.chip', { hasText: 'TRANSFORM' }).first();
    if (await chip.count()) await chip.click();
    await p.waitForTimeout(1200);
  },
  '16': async (p) => {                     // PERF — the run has to finish
    // The benchmark starts on load and runs on this thread. A
    // diff taken mid-run compares a screen whose RUN AGAIN
    // chip currently reads RUNNING… — which is correct
    // behaviour and a false positive.
    await p.waitForFunction(
      () => !/RUNNING/.test(document.body.innerText),
      null, { timeout: 60000 }).catch(() => {});
    await p.waitForTimeout(1200);
  },
  '13': async (p) => {                     // TRACE — a log needs ops in it
    // /lab/trace is a scratchpad: it starts empty, and the mockup
    // depicts a session in progress. So type, exactly as a reader
    // would, and let the editor produce the log the diff compares.
    const block = p.locator('[contenteditable="true"]').nth(1);
    if (await block.count()) {
      await block.click();
      await p.keyboard.type('A rope is the wrong primitive', { delay: 15 });
      await p.waitForTimeout(1500);
      await p.keyboard.press('Enter');
      await p.keyboard.type('anchors survive a split', { delay: 15 });
      await p.waitForTimeout(2000);
    }
  },
  '04': async (p, ctx) => {                  // EDITOR — a peer, a caret, a landmark
    await p.waitForTimeout(1200);

    // § 04 depicts a page being edited WITH someone. Presence is real —
    // join/leave over the WebSocket, not an op-broadcast heuristic — so the
    // only way to reach the depicted state is for someone to actually be
    // here. A second socket joins as its own actor and parks a cursor,
    // which is what puts a dot, a ping and a caret on screen.
    if (ctx?.pageId && ctx?.peerActor) {
      const ws = new WebSocket(`ws://localhost:8002/collab/pages/${ctx.pageId}`, ['bearer', ctx.peerToken]);
      ctx.sockets.push(ws);
      await new Promise((resolve) => {
        ws.on('message', (raw) => {
          const m = JSON.parse(raw.toString());
          if (m.type !== 'snapshot') return;
          const blocks = (m.snapshot?.blocks ?? m.blocks ?? []).filter((b) => !b.parent);
          const b = blocks.find((x) => (x.text ?? '').length > 8) ?? blocks[0];
          // The payload is NESTED (wsapi.clientMessage.Cursor), not flat.
          // Sent flat it parses as a cursor with no block, which the server
          // reads as "blurred out of every block" — so the peer showed as
          // present and "reading", and no caret was drawn.
          if (b) ws.send(JSON.stringify({ type: 'cursor', cursor: { block_id: b.id, start: 4, end: 4 } }));
          resolve();
        });
        ws.on('error', resolve);
        setTimeout(resolve, 6000);
      });
      await p.waitForTimeout(1800);
      if (process.env.UIDIFF_DEBUG) console.error('peer socket state:', ws.readyState);
    } else if (process.env.UIDIFF_DEBUG) { console.error('no peer/page ctx', ctx.pageId, ctx.peerActor); }

    // The inspector's PRESENCE tab, where LIVE IN THIS PAGE is.
    await p.locator('.it', { hasText: 'PRESENCE' }).first().click().catch(() => {});
    await p.waitForTimeout(700);

    // A landmark is ACTIVE when the caret is inside it, so put the caret in
    // one — clicking the outline row jumps the view but does not move the
    // caret, and "where the view is" is the reader's question, not this
    // screen's.
    const heads = p.locator('.canvas h2[contenteditable="true"]');
    if (await heads.count()) {
      await heads.first().click();
      await p.waitForTimeout(900);
    }
  },
  '07': async (p) => {                       // GRAPH — a node is selected
    await p.waitForTimeout(2500);            // let the layout settle first
    const n = p.locator('svg circle').nth(3);
    if (await n.count()) await n.click({ force: true });
    await p.waitForTimeout(900);
  },
  '08': async (p) => {                       // ALGORITHMS — a source is picked
    await p.waitForTimeout(2500);
    const n = p.locator('svg circle').nth(2);
    if (await n.count()) await n.click({ force: true });
    await p.waitForTimeout(1200);
  },
  '05': async (p) => {                       // READER — mid-document, as § 05 draws it
    const c = p.locator('main.canvas');
    if (await c.count()) await c.evaluate((el) => { el.scrollTop = el.scrollHeight * 0.4; });
    await p.waitForTimeout(600);
  },
  '09': async (p) => {                       // DISCOVER — a scope and a tag are chosen
    await p.waitForTimeout(1200);
    // The mockup depicts a query in the box. Typing one is also the only way
    // the n/a columns a typed query produces get compared at all.
    const q = p.locator('input[aria-label="semantic query"]');
    if (await q.count()) {
      await q.fill('balanced tree of substrings');
      await p.waitForTimeout(1400);
      await q.fill('');
      await p.waitForTimeout(900);
    }
    // PROTOCOL by name, not "the first chip": the mockup depicts that scope
    // specifically, and which topic sorts first is a fact about the seed
    // data rather than about this screen.
    const topic = p.locator('.tpc', { hasText: 'PROTOCOL' }).first();
    if (await topic.count()) await topic.click();
    await p.waitForTimeout(900);
    const tag = p.locator('.tg').first();
    if (await tag.count()) await tag.click();
    await p.waitForTimeout(900);
  },
  '10': async (p) => {                       // FACTS — a definition is selected
    const d = p.locator('.tr').first();
    if (await d.count()) await d.click();
    await p.waitForTimeout(1200);
  },
  '17': async (p) => {                       // HISTORY — a block with characters
    await p.waitForTimeout(1800);
    // The mockup depicts a revision selected part-way back, which is the only
    // state where RESTORE is live — at head there is nothing to revert to and
    // the chip is correctly dimmed. Scrub back to reach the depicted state.
    const revs = p.locator('[title^="rev "]');
    const n = await revs.count();
    if (n > 2) await revs.nth(Math.max(0, n - 3)).click();
    await p.waitForTimeout(900);
  },
  '24b': async (p) => {                      // COMMAND PALETTE — ⌘K, with a query typed
    await p.keyboard.press('Meta+k');
    await p.waitForTimeout(400);
    await p.keyboard.type('rope');
    await p.waitForTimeout(1400);
  },
  '24c': async (p) => {                      // NOTIFICATIONS PANEL — the bell, opened
    const bell = p.locator('.icb').first();
    if (await bell.count()) await bell.click();
    await p.waitForTimeout(500);
  },
};

const EXTRACT = (props, chromeSel) => `(root) => {
  const out = [];
  const walk = (n, path) => {
    if (n.nodeType !== 1) return;
    const cls = [...n.classList].sort().join('.');
    const key = path + '>' + n.tagName.toLowerCase() + (cls ? '.' + cls : '');
    const cs = getComputedStyle(n);
    const style = {};
    for (const p of ${JSON.stringify(props)}) style[p] = cs[p];
    const rect = n.getBoundingClientRect();
    out.push({
      order: out.length,
      key,
      style,
      w: Math.round(rect.width),
      h: Math.round(rect.height),
      text: n.matches('${chromeSel}') ? (n.textContent || '').trim().slice(0, 40) : null,
      // Whether this element renders any text at all. An empty span — a rule,
      // a bar, a tick — inherits colour and font from wherever it happens to
      // sit, and comparing those compares its ANCESTRY rather than its design.
      empty: (n.textContent || '').trim() === '',
    });
    let i = 0;
    for (const c of n.children) walk(c, key + '[' + i++ + ']');
  };
  walk(root, '');
  return out;
}`;

async function main() {
  const [screen = '04', appPath = '/pages', pageTitle] = process.argv.slice(2);
  const b = await chromium.launch({ channel: 'chrome' });

  const m = await b.newPage({ viewport: { width: 1440, height: 900 } });
  await m.goto(MOCKUP);
  // Widths are meaningless until the webfonts settle: measuring while
  // Archivo is still loading made every element differ by tens of pixels.
  await m.evaluate('document.fonts.ready');
  await m.waitForTimeout(1500);
  const mockup = await m.evaluate(`(() => {
    const t = [...document.querySelectorAll('.tag')].find(x => x.textContent.trim().startsWith('${screen}'));
    if (!t) return null;
    const extract = ${EXTRACT(PROPS, CHROME_SELECTORS)};
    return extract(t.nextElementSibling);
  })()`);
  if (!mockup) { console.error(`no mockup screen ${screen}`); process.exit(1); }

  // Authenticate, then resolve the route.
  const r = await fetch(`${GW}/auth/login`, { method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({ email: 'ui-demo@example.com', password: 'ui-demo-password-123' }) });
  const pair = await r.json();
  const sub = JSON.parse(Buffer.from(pair.access_token.split('.')[1], 'base64url')).sub;

  // A path may name where the id goes ("/read/{id}"); without a placeholder
  // the id is assumed to be a page under /pages, which is where most of the
  // per-page screens live.
  let route = appPath;
  if (pageTitle) {
    // /graph, not /pages: ListPages returns ROOT pages only, so once the
    // corpus had nested pages every title inside a series resolved to
    // nothing and the diff silently ran against the wrong route.
    const graph = await (await fetch(`${GW}/graph`, { headers: { Authorization: `Bearer ${pair.access_token}` } })).json();
    const p = graph.nodes.find(x => x.title === pageTitle);
    if (!p) {
      const near = graph.nodes
        .map(x => x.title)
        .filter(t => t.toLowerCase().includes(pageTitle.toLowerCase().split(' ')[0]))
        .slice(0, 5);
      console.error(`no page titled "${pageTitle}".`);
      console.error(`Diffing ${appPath} instead would compare the wrong screen and report`);
      console.error(`every element of the real one as missing, so this is fatal rather than a warning.`);
      if (near.length) console.error(`\nDid you mean:\n  ${near.join('\n  ')}`);
      else console.error(`\nNothing similar in the ${graph.nodes.length}-page corpus.`);
      await b.close();
      process.exit(2);
    }
    route = appPath.includes('{id}') ? appPath.replace('{id}', p.id) : `/pages/${p.id}`;
  }

  const a = await b.newPage({ viewport: { width: 1440, height: 900 } });
  // Before any page script runs, on every navigation — see verify.js's own
  // note: setting it after a goto races the app's redirect to /login.
  await a.addInitScript((s) => {
    try { localStorage.setItem('marginal.session', JSON.stringify(s)); } catch { /* private mode */ }
  }, { actorId: sub, accessToken: pair.access_token, refreshToken: pair.refresh_token });
  await a.goto(APP + '/', { waitUntil: 'domcontentloaded' });
  await a.goto(APP + route, { waitUntil: 'networkidle' });
  await a.evaluate('document.fonts.ready');
  // 3.5s was enough when the rail listed 18 flat pages. It is not enough now
  // that the editor also resolves a series, a diagnostics pass, a link graph
  // and a reveal-to-active-page — and a diff taken mid-load reports "missing"
  // for everything that had not arrived yet, which is the most misleading
  // failure this tool can produce.
  await a.waitForTimeout(7000);
  // What a seed may need beyond the page itself: the page it is looking at,
  // and a SECOND actor. Some depicted states are not reachable by one person
  // — presence is real join/leave over the WebSocket, so "somebody else is
  // here" requires somebody else to be here.
  const ctx = { pageId: route.match(/[0-9a-f-]{36}/)?.[0] ?? null, peerActor: null, peerToken: null, sockets: [] };
  if (SEED[screen]) {
    if (/\bpeerActor\b/.test(String(SEED[screen]))) {
      const peer = await fetch(`${GW}/auth/register`, {
        method: 'POST', headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ email: 'uidiff-peer@example.com', password: 'uidiff-peer-123456', display_name: 'Ada Devereux' }),
      }).then(r => r.json()).catch(() => ({}));
      const token = peer.access_token ?? (await fetch(`${GW}/auth/login`, {
        method: 'POST', headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ email: 'uidiff-peer@example.com', password: 'uidiff-peer-123456' }),
      }).then(r => r.json()).catch(() => ({}))).access_token;
      if (token) { ctx.peerActor = JSON.parse(Buffer.from(token.split('.')[1], 'base64url')).sub; ctx.peerToken = token; }
      if (process.env.UIDIFF_DEBUG) console.error('seed peer:', ctx.peerActor, 'page:', ctx.pageId);
    }
    // Put the screen into the state its mockup depicts, so what remains
    // missing afterwards is absent rather than merely unshown.
    try { await SEED[screen](a, ctx); } catch (e) { console.error('seed failed:', e.message); }
  }
  const app = await a.evaluate(`(() => {
    const extract = ${EXTRACT(PROPS, CHROME_SELECTORS)};
    return extract(document.querySelector('.sc') || document.body);
  })()`);

  // Index by class signature rather than exact path — the app nests
  // differently in places for real reasons (a router Link where the mockup
  // has a span), and a path-exact match would report those as total misses.
  //
  // Two rules keep this honest, both learned from false positives:
  //
  // 1. Elements with NO class are skipped. Their signature is a bare `div`,
  //    which matches thousands of unrelated nodes — it reported the search
  //    input against an arbitrary layout wrapper and called the fonts wrong.
  // 2. Occurrences are compared PAIRWISE in document order, not first-to-
  //    first. `.tb` is both the nav tab and the graph's colour-by chip; a
  //    first-to-first match compared the nav against the chip and reported
  //    the chip's smaller padding as a defect.
  const sigOf = (e) => e.key.split('>').pop();
  const group = (list) => {
    const g = new Map();
    for (const e of list) {
      const s = sigOf(e);
      if (!s.includes('.')) continue;   // rule 1: no class, no signal
      if (!g.has(s)) g.set(s, []);
      g.get(s).push(e);
    }
    return g;
  };
  const M = group(mockup), A = group(app);

  /**
   * Generic containers compare by CLASS, not by tag.
   *
   * This design system is class-driven: `.lbl` is a label wherever it
   * appears, and the mockup writes it as a span in one place and a div in
   * another. The app renders `.icb` as a router <a> because it navigates.
   * None of that is a design difference, and treating it as one buried the
   * real gaps under tag noise.
   *
   * Non-generic tags keep theirs — a <pre> is not a <div>, and a heading
   * level is a real distinction.
   */
  const GENERIC = /^(div|span|a|aside|input|button|section|article|nav|main|li|ul|ol)\./;
  const norm = (s) => (s || '').replace(GENERIC, 'box.');
  const Anorm = new Map();
  for (const [k, v] of A) {
    const nk = norm(k);
    Anorm.set(nk, (Anorm.get(nk) || []).concat(v));
  }
  // Merged buckets must stay in document order, or pairwise comparison
  // pairs a late element against an early one.
  for (const v of Anorm.values()) v.sort((x, y) => x.order - y.order);

  let missing = [], propDiffs = [], textDiffs = [];
  /**
   * An app element may carry MORE classes than the mockup's counterpart, and
   * that is not a difference worth reporting.
   *
   * The editor's title is `class="h1 editable page-title"`: `h1` is the
   * mockup's type rule, and the other two are what makes it a live
   * contenteditable rather than a drawing of one. An exact class-set match
   * called that a missing `h1.h1` — the element was right there, doing more.
   *
   * So: exact match first (it pairs in document order and is what most
   * elements hit), then a subset fallback — same tag, and every class the
   * mockup names present. Reported once, so a superset match still pairs its
   * properties and text like any other.
   */
  const subsetMatch = (sig) => {
    const [tag, ...want] = norm(sig).split('.');
    const out = [];
    for (const [k, v] of Anorm) {
      const [atag, ...have] = k.split('.');
      if (atag !== tag) continue;
      if (!want.every((c) => have.includes(c))) continue;
      out.push(...v);
    }
    return out.length ? out.sort((x, y) => x.order - y.order) : null;
  };

  /**
   * Pair occurrences WITHIN a parent, not by flat index across the screen.
   *
   * Rule 2 pairs a bucket occurrence-by-occurrence in document order, which
   * is right until one of those occurrences is a data-driven list. § 05 draws
   * `.tg` in three roles — a page's own tags, a related card's tags, and the
   * co-occurrence rows, which are fixed-width and left-aligned. The mockup's
   * page has four own-tags; a real seeded page has two, and every chip after
   * it shifted by two, so a co-occurrence chip paired against a plain one and
   * reported `justify-content: normal -> flex-start` on four elements. The
   * screen was correct and matched the mockup element for element; only the
   * corpus was a different length, which rule 2 already calls content rather
   * than design.
   *
   * So group both sides by parent signature first and pair inside each group.
   * Elements whose parent has no counterpart group fall back to flat document
   * order, which is exactly today's behaviour — this only refines the pairing
   * where a refinement is available, so the nesting differences rule 2 was
   * written for still pair as they did.
   */
  const parentOf = (e) => {
    const seg = e.key.split('>');
    // The positional [n] must come OFF. A key segment is `div.bar[9]`, and
    // the mockup and the app number the same container differently whenever
    // either side has one element more above it — so keeping the index made
    // `div.bar[9]` on one side the SAME group as an unrelated `div.bar[9]`
    // on the other, and paired § 04's READ against SHARE. Measured, not
    // reasoned: the two keys were printed side by side.
    return norm((seg[seg.length - 2] || '').replace(/\[\d+\]/g, ''));
  };
  const byParent = (list) => {
    const g = new Map();
    for (const e of list) {
      const k = parentOf(e);
      if (!g.has(k)) g.set(k, []);
      g.get(k).push(e);
    }
    return g;
  };
  function pairUp(els, hit) {
    const mp = byParent(els), ap = byParent(hit);
    const pairs = [], mLeft = [], aLeft = [];
    for (const [k, mine] of mp) {
      const theirs = ap.get(k);
      if (!theirs) { mLeft.push(...mine); continue; }
      const n = Math.min(mine.length, theirs.length);
      for (let i = 0; i < n; i++) pairs.push([mine[i], theirs[i]]);
    }
    for (const [k, theirs] of ap) if (!mp.has(k)) aLeft.push(...theirs);
    mLeft.sort((x, y) => x.order - y.order);
    aLeft.sort((x, y) => x.order - y.order);
    for (let i = 0; i < Math.min(mLeft.length, aLeft.length); i++) {
      pairs.push([mLeft[i], aLeft[i]]);
    }
    // Report in document order, so an index in the output still reads down
    // the screen the way somebody looking at it would.
    pairs.sort((x, y) => x[0].order - y[0].order);
    return pairs.map(([m, a], i) => [i, m, a]);
  }

  for (const [sig, els] of M) {
    if (IGNORED_MISSING.has(sig)) continue;
    const hit = Anorm.get(norm(sig)) || subsetMatch(sig);
    if (!hit) { missing.push(sig); continue; }
    // Rule 2: pair up occurrence-by-occurrence, and only as far as both
    // sides go. A count difference is content, not design.
    const cls = sig.split('.').slice(1);
    if (cls.length === 1 && UTILITY.has(cls[0])) continue;   // presence only
    for (const [i, me, ae] of pairUp(els, hit)) {
      // Also matched on the tag + FIRST class, so a rule written for
      // `div.tr` covers `div.tr.tr-on` — a row does not stop being content
      // because it is the selected one.
      const base = sig.replace(/^([a-z]+\.[^.]+).*$/, '$1');
      const skip = IGNORED_PROPS[norm(sig)] || IGNORED_PROPS[sig] || IGNORED_PROPS[base];
      // An element with no text of its own: its inherited text properties are
      // a fact about its ancestors, not about it. Reported, they were the
      // single largest source of noise on the list screens — 45 diffs on one
      // screen, every one of them a 2px bar inheriting a font size it never
      // uses.
      const inherited = me.empty && ae.empty ? INHERITED_TEXT_PROPS : null;
      for (const p of PROPS) {
        if (skip && skip.has(p)) continue;
        if (inherited && inherited.has(p)) continue;
        if (me.style[p] !== ae.style[p]) {
          propDiffs.push(`${sig}[${i}]  ${p}: ${me.style[p]}  ->  ${ae.style[p]}`);
        }
      }
      // Trailing counts are stripped before comparing. A tab reading
      // "CHECKS" where the mockup says "PROBLEMS" is a real finding; one
      // reading "CHECKS 1" where the mockup says "CHECKS 2" is a report
      // that the two corpora differ, which is the premise of the whole
      // tool rather than a defect in the screen.
      const label = (t) => (t || '').replace(/\s+\d+$/, '').trim();
      if (me.text && ae.text && label(me.text) !== label(ae.text) && /^[A-Z0-9 ·⌘/&—+]+$/.test(me.text)) {
        textDiffs.push(`${sig}[${i}]  "${me.text}"  ->  "${ae.text}"`);
      }
    }
  }
  const extra = [...A.keys()].filter(k => !M.has(k) && !M.has(norm(k)));

  const cap = Number(process.env.UIDIFF_CAP || 25);
  console.log(`\n=== § ${screen} vs ${route} ===`);
  console.log(`missing ${missing.length} · property diffs ${propDiffs.length} · chrome text diffs ${textDiffs.length} · extra ${extra.length}`);
  if (missing.length) console.log('\nMISSING (in mockup, not in app):\n  ' + missing.slice(0, cap).join('\n  '));
  if (textDiffs.length) console.log('\nCHROME TEXT:\n  ' + textDiffs.slice(0, cap).join('\n  '));
  if (propDiffs.length) console.log('\nPROPERTIES:\n  ' + propDiffs.slice(0, cap).join('\n  '));
  if (extra.length) console.log('\nEXTRA (in app, not in mockup):\n  ' + extra.slice(0, cap).join('\n  '));

  // Leave nobody connected: a socket held open keeps the peer "present"
  // for the next run, which would make a later diff pass for the wrong
  // reason.
  for (const ws of ctx.sockets) { try { ws.close(); } catch { /* already gone */ } }
  await b.close();
  process.exit(missing.length ? 1 : 0);
}
main();
