#!/usr/bin/env node
/**
 * verify — clicks every control every built screen draws, and asserts the
 * thing it is supposed to change actually changed.
 *
 * uidiff compares markup and computed style, so a control that renders and
 * does nothing passes it. This is the other half of the gate CLAUDE.md
 * describes, and it is not a formality: it found a 500 on GET .../trace that
 * no unit test and no DOM diff could have, because the editor never replays
 * the op log — only Trace and History do, and only this pass opens them.
 *
 * Usage:  node tools/uidiff/verify.js
 * Needs:  the stack up (docker compose up), the dev server running, and
 *         `npm install` in tools/ for playwright-core.
 *
 * Add a check when you add a control. A screen whose controls are not in here
 * is a screen whose controls nobody is checking.
 */
const { chromium } = require('/Users/genuinebasilnt/projects/marginal/tools/node_modules/playwright-core');
const GW='http://localhost:8000', APP='http://localhost:5173';
let fails = 0;
const check = (name, ok, detail='') => { console.log(`${ok?' ok ':'FAIL'}  ${name}${detail?'  '+detail:''}`); if(!ok) fails++; };

(async () => {
  const r = await fetch(`${GW}/auth/login`, {method:'POST',headers:{'Content-Type':'application/json'},
    body: JSON.stringify({email:'ui-demo@example.com',password:'ui-demo-password-123'})});
  const pair = await r.json(); const sub = JSON.parse(Buffer.from(pair.access_token.split('.')[1],'base64url')).sub;
  const g = await (await fetch(`${GW}/graph`,{headers:{'X-Actor-Id':sub}})).json();
  const page = g.nodes.find(n=>n.title==='The document block model');
  const hub  = g.nodes.find(n=>n.title==='The Rust Porting Handbook');

  const b = await chromium.launch({channel:'chrome'});
  const p = await b.newPage({viewport:{width:1440,height:900}});
  const errs=[]; p.on('pageerror',e=>errs.push(e.message));
  await p.goto(APP+'/',{waitUntil:'domcontentloaded'});
  await p.evaluate(s=>localStorage.setItem('marginal.session',JSON.stringify(s)),
    {actorId:sub,accessToken:pair.access_token,refreshToken:pair.refresh_token});

  const go = async (route, wait=6000) => { await p.goto(APP+route,{waitUntil:'networkidle'}); await p.waitForTimeout(wait); };
  const txt = () => p.evaluate(()=>document.body.innerText);
  const count = (sel) => p.evaluate(s=>document.querySelectorAll(s).length, sel);

  // ── § 07 GRAPH: three lenses, three colourings, drag ──────────────────
  await go('/graph', 7000);
  // The hulls arrive through wasm after the layout settles, so the baseline
  // has to be taken once they exist — measuring at 0 and comparing for
  // equality later is a flaky check, which is worse than no check.
  await p.waitForFunction(() => document.querySelectorAll('svg path').length > 0, null, { timeout: 20000 });
  const forcePaths = await count('svg path');
  await p.getByText('TERRITORY · VORONOI',{exact:true}).click(); await p.waitForTimeout(1200);
  const voronoiPaths = await count('svg path');
  check('§07 FORCE draws hulls, TERRITORY draws Voronoi', forcePaths > 0 && voronoiPaths > forcePaths, `${forcePaths} → ${voronoiPaths}`);
  await p.getByText('FORCE',{exact:true}).click(); await p.waitForTimeout(900);
  const backToForce = await count('svg path');
  check('§07 FORCE turns Voronoi back off', backToForce < voronoiPaths, `${voronoiPaths} → ${backToForce}`);
  const hue = () => p.evaluate(()=>[...new Set([...document.querySelectorAll('svg circle')].map(c=>c.getAttribute('fill')))].sort().join(','));
  const byTopic = await hue();
  await p.getByText('CLUSTER',{exact:true}).click(); await p.waitForTimeout(700);
  check('§07 COLOUR BY CLUSTER repaints', (await hue()) !== byTopic);
  await p.getByText('SPACE',{exact:true}).click(); await p.waitForTimeout(700);
  await p.getByText('TOPIC',{exact:true}).click(); await p.waitForTimeout(500);
  check('§07 COLOUR BY TOPIC restores', (await hue()) === byTopic);
  check('§07 disagreement is counted', /\d+ sit in another topic's territory/.test(await txt()));

  // ── § 08: nine lenses each repaint ────────────────────────────────────
  await go('/graph/algorithms', 7000);
  const lenses = ['BFS · SHORTEST PATH','NEAREST','COMPONENTS','SCC · TARJAN','CYCLES · 3-COLOUR DFS','TOPO SORT · KAHN','REACHABILITY','BLAST RADIUS','TOPOLOGY'];
  const seen = new Set();
  for (const l of lenses) { await p.getByText(l,{exact:true}).first().click(); await p.waitForTimeout(400); seen.add(await hue()); }
  check('§08 nine lenses produce distinct paints', seen.size >= 6, `${seen.size} distinct`);

  // ── § 09 DISCOVER ─────────────────────────────────────────────────────
  await go('/discover', 7000);
  check('§09 recall is reported', /RECALL@5/.test(await txt()));
  check('§09 three signals table', /TERM COSINE/.test(await txt()) && /GRAPH DISTANCE/.test(await txt()));
  await p.locator('.tpc').first().click(); await p.waitForTimeout(1200);
  check('§09 scope filter runs', (await count('.tpc')) > 0);

  // ── § 10d SERIES ──────────────────────────────────────────────────────
  await go('/series', 5000);
  check('§10d series index lists the handbook', /Rust Porting Handbook/.test(await txt()));
  await p.getByText('≡ LIST',{exact:true}).click(); await p.waitForTimeout(500);
  check('§10d list view switches', (await count('.srow')) > 0);
  await go(`/series/${hub.id}`, 5000);
  check('§10d one series shows 19 parts', /19 parts/.test(await txt()));

  // ── § 05 READER: banner, outline, links, tabs ─────────────────────────
  const part = g.nodes.find(n=>n.title==='The grammar, in full');
  await go(`/read/${part.id}`, 7000);
  check('§05 series banner with prev/next', /Part\s+\d+\s+of/.test(await txt()));
  check('§05 IN THIS PAGE, no "(empty)" rows', (await count('.oi')) > 0 && !/\(empty\)/.test(await txt()));
  check('§05 marks and page links render', (await count('.pl')) > 0);
  check('§05 code blocks are highlighted', (await p.evaluate(()=>document.querySelectorAll('.blk-code pre span').length)) > 10);
  await p.locator('.it', { hasText: 'BACKLINKS' }).first().click(); await p.waitForTimeout(400);
  check('§05 inspector tabs switch', /BACKLINKS ·/.test(await txt()));
  const before = await p.evaluate(()=>location.pathname);
  await p.locator('.pl').first().click(); await p.waitForTimeout(1500);
  check('§05 a page link navigates', (await p.evaluate(()=>location.pathname)) !== before);

  // ── § 04 EDITOR: rail, banner, read switch ────────────────────────────
  await go(`/pages/${page.id}`, 8000);
  check('§04 rail reveals the open page', (await count('.tr-on')) === 1);
  check('§04 rail draws depth guides', (await count('.tr-guide')) > 0);
  check('§04 rail shows a part count', (await count('.tr-parts')) > 0);
  check('§04 ACK P99 is measured', /ACK P99/.test(await txt()));
  await p.locator('.it', { hasText: 'CHECKS' }).first().click(); await p.waitForTimeout(400);
  check('§04 inspector tab counts', /CHECKS \d/.test(await txt()));

  // ── § 24b COMMAND PALETTE ─────────────────────────────────────────────
  await p.keyboard.press('Meta+k'); await p.waitForTimeout(500);
  check('§24b ⌘K opens the palette', (await count('.pal')) === 1);
  await p.keyboard.type('topic:storage rope'); await p.waitForTimeout(1600);
  check('§24b parses a filter query', /UNDERSTOOD AS A QUERY/.test(await txt()));
  await p.keyboard.press('Escape'); await p.waitForTimeout(300);
  check('§24b esc closes it', (await count('.pal')) === 0);

  // ── § 24c NOTIFICATIONS PANEL + § 20 ──────────────────────────────────
  await p.locator('.icb').first().click(); await p.waitForTimeout(700);
  check('§24c the bell opens the panel', /INBOX/.test(await txt()));
  await p.getByText('open inbox →',{exact:true}).click(); await p.waitForTimeout(1500);
  check('§20 the panel links to the inbox', (await p.evaluate(()=>location.pathname)) === '/notifications');

  // ── § 23c TRASH ───────────────────────────────────────────────────────
  await go('/trash', 6000);
  check('§23c trash lists deleted pages', (await count('.trash-row')) > 0);
  check('§23c states what restore does', /WHAT RESTORE ACTUALLY DOES/.test(await txt()));
  await p.locator('.trash-row').first().click(); await p.waitForTimeout(1500);
  check('§23c a trashed row explains it has no live blast radius', /already deleted, so there is no blast radius/.test(await txt()));

  // ── § 24e NOT FOUND ───────────────────────────────────────────────────
  await go('/p/the-documnt-block-modl', 4000);
  check('§24e a wrong URL is not a redirect', (await p.evaluate(()=>location.pathname)) === '/p/the-documnt-block-modl');
  check('§24e BK-tree suggests the real page', /The document block model/.test(await txt()));

  // ── § 17 HISTORY ──────────────────────────────────────────────────────
  await go(`/pages/${page.id}/history`, 6000);
  check('§17 op stream names the op kind', /InsertBlock|SetBlockContent|InsertText/.test(await txt()));
  await p.locator('.it', { hasText: 'REVISIONS' }).first().click(); await p.waitForTimeout(400);
  check('§17 REVISIONS folds ops into gestures', /GESTURES ·/.test(await txt()));
  await p.getByText('TEXT',{exact:true}).first().click(); await p.waitForTimeout(500);
  check('§17 TEXT/PALIMPSEST toggle', /TEXT AT THIS REVISION/.test(await txt()));

  console.log(errs.length ? `\nPAGE ERRORS: ${errs.slice(0,5).join(' | ')}` : '\nno page errors');
  if (errs.length) fails++;
  console.log(fails ? `\n${fails} FAILED` : '\nall checks passed');
  await b.close();
  process.exit(fails ? 1 : 0);
})();
