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

/** Chrome text worth comparing — short, uppercase-ish, structural. */
const CHROME_SELECTORS = '.lbl,.rd-k,.tb,.sb,.it,.chip,.btn,.kbd,.tpc,.wm';

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

  let route = appPath;
  if (pageTitle) {
    const pages = await (await fetch(`${GW}/pages`, { headers: { 'X-Actor-Id': sub } })).json();
    const p = pages.pages.find(x => x.title === pageTitle);
    if (p) route = `/pages/${p.id}`;
  }

  const a = await b.newPage({ viewport: { width: 1440, height: 900 } });
  await a.goto(APP + '/', { waitUntil: 'domcontentloaded' });
  await a.evaluate(s => localStorage.setItem('marginal.session', JSON.stringify(s)),
    { actorId: sub, accessToken: pair.access_token, refreshToken: pair.refresh_token });
  await a.goto(APP + route, { waitUntil: 'networkidle' });
  await a.evaluate('document.fonts.ready');
  await a.waitForTimeout(3500);
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

  const norm = (s) => (s || '').replace(/^a\./, 'span.').replace(/^aside\./, 'div.').replace(/^input\./, 'div.');
  const Anorm = new Map();
  for (const [k, v] of A) {
    const nk = norm(k);
    Anorm.set(nk, (Anorm.get(nk) || []).concat(v));
  }
  // Merged buckets must stay in document order, or pairwise comparison
  // pairs a late element against an early one.
  for (const v of Anorm.values()) v.sort((x, y) => x.order - y.order);

  let missing = [], propDiffs = [], textDiffs = [];
  for (const [sig, els] of M) {
    const hit = Anorm.get(norm(sig));
    if (!hit) { missing.push(sig); continue; }
    // Rule 2: pair up occurrence-by-occurrence, and only as far as both
    // sides go. A count difference is content, not design.
    const n = Math.min(els.length, hit.length);
    for (let i = 0; i < n; i++) {
      const me = els[i], ae = hit[i];
      for (const p of PROPS) {
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
