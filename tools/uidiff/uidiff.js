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
const { chromium } = require('playwright-core');

const MOCKUP = 'file:///Users/genuinebasilnt/projects/marginal/docs/ui-mockups/v2/index.html';
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
const IGNORED_MISSING = new Set(['div.scan', 'div.tag']);
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
    const topic = p.locator('.tpc').first();
    if (await topic.count()) await topic.click();
    await p.waitForTimeout(700);
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
    await p.waitForTimeout(1500);
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
    const graph = await (await fetch(`${GW}/graph`, { headers: { 'X-Actor-Id': sub } })).json();
    const p = graph.nodes.find(x => x.title === pageTitle);
    if (!p) console.error(`no page titled "${pageTitle}" — diffing ${appPath} as given`);
    if (p) route = appPath.includes('{id}') ? appPath.replace('{id}', p.id) : `/pages/${p.id}`;
  }

  const a = await b.newPage({ viewport: { width: 1440, height: 900 } });
  await a.goto(APP + '/', { waitUntil: 'domcontentloaded' });
  await a.evaluate(s => localStorage.setItem('marginal.session', JSON.stringify(s)),
    { actorId: sub, accessToken: pair.access_token, refreshToken: pair.refresh_token });
  await a.goto(APP + route, { waitUntil: 'networkidle' });
  await a.evaluate('document.fonts.ready');
  // 3.5s was enough when the rail listed 18 flat pages. It is not enough now
  // that the editor also resolves a series, a diagnostics pass, a link graph
  // and a reveal-to-active-page — and a diff taken mid-load reports "missing"
  // for everything that had not arrived yet, which is the most misleading
  // failure this tool can produce.
  await a.waitForTimeout(7000);
  if (SEED[screen]) {
    // Put the screen into the state its mockup depicts, so what remains
    // missing afterwards is absent rather than merely unshown.
    try { await SEED[screen](a); } catch (e) { console.error('seed failed:', e.message); }
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

  for (const [sig, els] of M) {
    if (IGNORED_MISSING.has(sig)) continue;
    const hit = Anorm.get(norm(sig)) || subsetMatch(sig);
    if (!hit) { missing.push(sig); continue; }
    // Rule 2: pair up occurrence-by-occurrence, and only as far as both
    // sides go. A count difference is content, not design.
    const cls = sig.split('.').slice(1);
    if (cls.length === 1 && UTILITY.has(cls[0])) continue;   // presence only
    const n = Math.min(els.length, hit.length);
    for (let i = 0; i < n; i++) {
      const me = els[i], ae = hit[i];
      const skip = IGNORED_PROPS[norm(sig)] || IGNORED_PROPS[sig];
      for (const p of PROPS) {
        if (skip && skip.has(p)) continue;
        if (me.style[p] !== ae.style[p]) {
          propDiffs.push(`${sig}[${i}]  ${p}: ${me.style[p]}  ->  ${ae.style[p]}`);
        }
      }
      if (me.text && ae.text && me.text !== ae.text && /^[A-Z0-9 ·⌘/&—+]+$/.test(me.text)) {
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

  await b.close();
  process.exit(missing.length ? 1 : 0);
}
main();
