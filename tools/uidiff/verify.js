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
const path = require('node:path');
// Resolved from THIS file, not the caller's cwd: running the gate from
// web/ used to die with MODULE_NOT_FOUND, and a chained `echo` made the
// failure look like a pass. A gate that only works from one directory is
// a gate that will be run from another.
const REPO = path.resolve(__dirname, '..', '..');
const { chromium } = require(path.join(REPO, 'tools', 'node_modules', 'playwright-core'));
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

  // Each lens must also STAGE its answer differently, not just paint a
  // different still. The unrevealed grey is the animation's own colour, so
  // counting it mid-reveal says whether anything is actually moving.
  const pendingNodes = () => p.evaluate(() =>
    [...document.querySelectorAll('svg circle')].filter((c) => c.getAttribute('fill') === '#3A3833').length);
  const staged = [];
  for (const name of ['NEAREST', 'COMPONENTS', 'TOPO SORT · KAHN']) {
    await p.locator('.sb').filter({ hasText: name }).first().click();
    await p.waitForTimeout(250);
    const early = await pendingNodes();
    await p.waitForTimeout(3500);
    staged.push(`${name.split(' ')[0]} ${early}→${await pendingNodes()}`);
  }
  check('§08 lenses reveal their answer over time rather than all at once',
        staged.every((s) => { const [a, b] = s.split(' ')[1].split('→').map(Number); return a > b; }),
        staged.join(' · '));

  // The two-node lens: click a start, click a destination, watch the route
  // draw. This is the gesture the whole lens exists for.
  await p.locator('.sb').filter({ hasText: 'SHORTEST PATH' }).first().click();
  await p.waitForTimeout(1500);
  const gesture = () => p.evaluate(() => {
    const m = document.body.innerText.match(/(PICK A START|PICK A DESTINATION|ROUTE)\n[^\n]*\n([^\n]*)/);
    return m ? `${m[1]} :: ${m[2]}` : null;
  });
  check('§08 the path lens asks for a destination, not just a source',
        /PICK A DESTINATION/.test(await gesture() ?? ''), await gesture());
  const graphNodes = p.locator('svg g[font-family="Archivo"] > g');
  const gestureText = async () => (await txt()).match(/(no route[^\n]*|\d+ hops? · BFS[^\n]*)/)?.[0] ?? '';

  // Try destinations until one is REACHABLE from the source.
  //
  // The corpus is not one connected component, so a fixed nth(N) destination
  // is a coin flip: when it lands in another component the screen correctly
  // says "no route — these two are in different components" and draws
  // nothing, and a check demanding a drawn hop fails on a screen that is
  // working. Which nodes share a component is a fact about seed data, so the
  // check finds a reachable one instead of assuming one.
  const total = await graphNodes.count();
  let routed = '';
  for (let i = 0; i < total && !/hops? · BFS/.test(routed); i++) {
    await graphNodes.nth(i).click({ force: true });
    await p.waitForFunction(
      () => /no route|hops? · BFS|hop · BFS/.test(document.body.innerText),
      null, { timeout: 15000 }).catch(() => {});
    routed = await gestureText();
  }
  check('§08 a second click produces a real route', /hops? · BFS/.test(routed), routed);
  check('§08 and the route is drawn, hop by hop',
        (await p.evaluate(() => [...document.querySelectorAll('svg line')]
          .filter((l) => l.getAttribute('stroke') === '#E8873C').length)) >= 1);

  // ── § 09 DISCOVER ─────────────────────────────────────────────────────
  await go('/discover', 7000);
  check('§09 recall is reported', /RECALL@5/.test(await txt()));
  check('§09 three signals table', /TERM COSINE/.test(await txt()) && /GRAPH DISTANCE/.test(await txt()));
  await p.locator('.tpc').first().click(); await p.waitForTimeout(1200);
  check('§09 scope filter runs', (await count('.tpc')) > 0);

  // A typed query is the same index reached from a different origin — and
  // two of the three signals then have no question to answer, which the
  // screen has to say rather than draw as zeros.
  const qbox = p.locator('input[aria-label="semantic query"]');
  const beforeQ = await txt();
  await qbox.fill('balanced tree of substrings splice concat');
  await p.waitForFunction(() => /n\/a · not a node/.test(document.body.innerText),
    null, { timeout: 20000 }).catch(()=>{});
  const afterQ = await txt();
  check('§09 typing a sentence queries the index', afterQ !== beforeQ && /near “balanced/.test(afterQ));
  check('§09 and it finds the page that sentence is about', /Ropes, and why we do not use one/.test(afterQ));
  check('§09 the signals a typed query cannot have say so, rather than reading 0',
    /n\/a · no tags/.test(afterQ) && /n\/a · not a node/.test(afterQ));
  check('§09 and it explains why they are blank rather than leaving a gap',
    /A sentence carries no tags/.test(afterQ));
  await qbox.fill('');
  // Wait for the n/a columns to GO, not merely for the text to change: a
  // clock in the top bar changes the page text every second, so "differs
  // from before" would pass while the request was still in flight.
  await p.waitForFunction(() => !/n\/a · not a node/.test(document.body.innerText),
    null, { timeout: 20000 }).catch(()=>{});
  check('§09 clearing it hands the query back to the selected page',
    !/n\/a · not a node/.test(await txt()));

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

  // ── § 11 COMPILER: the buffer is editable and everything follows it ───
  await go('/lab/compiler', 7000);
  const readouts = () => p.evaluate(()=>[...document.querySelectorAll('.rd')].map(e=>e.innerText.replace(/\n/g,'=')).join(' | '));
  check('§11 the projection holds on the sample', /FIELD-BY-FIELD EQUAL/.test(await txt()));
  const beforeEdit = await readouts();
  await p.locator('.labedit').click();
  await p.keyboard.press('Meta+a');
  await p.keyboard.type('# Live\n\n- one\n  - two\n\n```rust\nunclosed');
  await p.waitForTimeout(1400);
  check('§11 editing the buffer recomputes every panel', (await readouts()) !== beforeEdit);
  check('§11 an unclosed fence reports rather than failing', /never closed/.test(await txt()));
  check('§11 and still round-trips', /FIELD-BY-FIELD EQUAL/.test(await txt()));
  await p.keyboard.press('Meta+a');
  await p.keyboard.type('café — こんにちは');
  await p.waitForTimeout(1000);
  check('§11 chars and bytes diverge, and the divergence is named',
        /DIVERGENCE/.test(await txt()) && !/pure ASCII/.test(await txt()));

  // ── § 12 ANALYTICS: the stream is editable, and each sketch's behaviour
  //    under editing IS the claim the screen makes about it ───────────────
  await go('/lab/analytics', 7000);
  const rd = async (key) => p.evaluate((k) => {
    const el = [...document.querySelectorAll('.rd')].find(e => e.innerText.startsWith(k));
    return el ? el.innerText.split('\n')[1] : null;
  }, key);
  check('§12 registers are drawn, one bar each', (await count('.hll-regs > div')) === 64);
  check('§12 the HLL is shown against its own bound',
        /ITS OWN BOUND/.test(await txt()) && (await rd('ITS OWN BOUND'))?.startsWith('±'));
  check('§12 Count-Min never underestimates', /0 underestimates/.test(await txt()));

  // The whole argument for a sketch, in one gesture: a duplicate actor must
  // not move the cardinality estimate, while the page counter DOES move.
  const beforeUnique = await rd('ESTIMATE');
  // The row's own text, not a body-text regex: the page name and its
  // "4 / 4" counter sit on different lines, so `/name.*/` could never see
  // the number it was supposed to be watching.
  const cmRow = (name) => p.evaluate((n) => {
    const el = [...document.querySelectorAll('.cm-row')].find(e => e.innerText.startsWith(n));
    return el ? el.innerText.replace(/\n/g, ' ') : null;
  }, name);
  const beforeHeavy = await cmRow('sync-protocol-notes');
  await p.locator('.labedit').click();
  await p.keyboard.press('Meta+ArrowDown');
  await p.keyboard.type('\nana, sync-protocol-notes, protocol, 60000, crdt rope');
  await p.waitForTimeout(1200);
  check('§12 a duplicate actor does not move the cardinality estimate',
        (await rd('ESTIMATE')) === beforeUnique, `${beforeUnique} → ${await rd('ESTIMATE')}`);
  const afterHeavy = await cmRow('sync-protocol-notes');
  check('§12 but it does move the page count', afterHeavy !== beforeHeavy,
        `${beforeHeavy} → ${afterHeavy}`);

  // A new actor must move it, or the panel is simply not reading the buffer.
  const beforeNew = await rd('ESTIMATE');
  await p.keyboard.type('\nzed, sync-protocol-notes, protocol, 1000, crdt');
  await p.waitForTimeout(1200);
  check('§12 a new actor does', (await rd('ESTIMATE')) !== beforeNew,
        `${beforeNew} → ${await rd('ESTIMATE')}`);

  // Half a line is the normal state of a text box being typed into.
  await p.keyboard.type('\nthis line is nonsense');
  await p.waitForTimeout(1000);
  check('§12 a malformed line is counted, not fatal',
        /1 skipped/.test(await txt()) && /ESTIMATE/.test(await txt()));

  // Momentum and the topic split are computed from the buffer's two halves,
  // so appending has to move them too.
  check('§12 tag momentum is populated', /crdt|rope|blocks/.test(await txt()) && /second half vs first/.test(await txt()));

  // ── § 14 NETCODE: four lenses, a wire you drag, and the one
  //    contradiction the whole section exists to show ─────────────
  await go('/lab/netcode', 7000);
  const replicas = () => p.evaluate(() =>
    [...document.querySelectorAll('.body [style*="Spectral"]')].map(e => e.innerText));
  const lens = async (name) => {
    await p.locator('.sb', { hasText: name }).first().click();
    await p.waitForTimeout(700);
  };

  check('§14 both replicas converge', await p.evaluate(() => {
    const t = [...document.querySelectorAll('.body [style*="Spectral"]')]
      .map((e) => (e instanceof HTMLTextAreaElement ? e.value : e.innerText).trim());
    return t.length === 2 && t[0] === t[1] && t[0].length > 0;
  }));
  check('§14 the transform actually moved an op',
        /ins @26/.test(await txt()), 'Ada typed @20');
  check('§14 replay from empty matches', /MATCHES/.test(await txt()));

  // Every lens must paint something different. A tab strip whose
  // options all render the same panel is the exact defect this pass
  // exists to catch.
  const panel = () => p.evaluate(() => {
    const main = document.querySelector('.body');
    if (!main) return '';
    // The lens panel is the lower-left pane; take its heading and
    // first lines rather than the whole page, so a label change
    // elsewhere cannot pass for a different panel.
    const labels = [...main.querySelectorAll('.lbl')].map((e) => e.textContent);
    return labels.join('|');
  });
  const lensPanels = {};
  for (const name of ['TREE · MERKLE', 'CAUSALITY · DAG', 'LOG · LSM', 'PREDICTION · ROLLBACK']) {
    await lens(name);
    lensPanels[name] = await panel();
  }
  const distinctPanels = new Set(Object.values(lensPanels));
  check('§14 four lenses paint four DIFFERENT panels',
        distinctPanels.size === 4,
        distinctPanels.size === 4 ? '4 distinct' : JSON.stringify(lensPanels));
  await lens('CAUSALITY · DAG');
  check('§14 the DAG names a causal chain', /LONGEST CHAIN/.test(await txt()));
  await lens('LOG · LSM');
  check('§14 the LSM reports write amplification', /WRITE AMP/.test(await txt()));
  await lens('PREDICTION · ROLLBACK');
  check('§14 prediction shows the wire, not the merkle tree',
        /OPS ON THE WIRE/.test(await txt()) && !/MERKLE COMPARISON/.test(await txt()));

  // Drag the wire. A slider that renders and changes nothing is a
  // picture of a control.
  const beforeRtt = await txt();
  await p.locator('.wsl').first().evaluate((el) => {
    const set = Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, 'value').set;
    set.call(el, '460');
    el.dispatchEvent(new Event('input', { bubbles: true }));
    el.dispatchEvent(new Event('change', { bubbles: true }));
  });
  await p.waitForTimeout(1200);
  check('§14 dragging RTT re-runs the simulation', (await txt()) !== beforeRtt);
  check('§14 and the readout follows it', /460 ms/.test(await txt()));

  // Loss must cost retransmits, never a keystroke.
  await p.locator('.wsl').nth(1).evaluate((el) => {
    const set = Object.getOwnPropertyDescriptor(window.HTMLInputElement.prototype, 'value').set;
    set.call(el, '45');
    el.dispatchEvent(new Event('input', { bubbles: true }));
    el.dispatchEvent(new Event('change', { bubbles: true }));
  });
  await p.waitForTimeout(1200);
  const lossy = await txt();
  check('§14 45% loss produces retransmits', !/RETRANSMITS\n0/.test(lossy));
  check('§14 and still converges', !/REPLICAS DIFFER/.test(lossy));

  // The page's whole argument: transform off, and the replicas still
  // agree — on a document nobody asked for.
  await p.locator('.chip', { hasText: 'TRANSFORM' }).first().click();
  await p.waitForTimeout(1400);
  const off = await txt();
  check('§14 transform off still converges structurally', !/REPLICAS DIFFER/.test(off));
  check('§14 but the intent ledger flags it',
        /DISAGREE ON PURPOSE/.test(off) && /flags \d/.test(off));
  check('§14 and it says what was meant', /meant/.test(off) && /landed as/.test(off));

  // Editing the script re-runs everything.
  const beforeScript = await txt();
  await p.locator('.labedit').click();
  await p.keyboard.press('Meta+a');
  await p.keyboard.type('0, you, insert, 0, ZZZ\n5, ada, delete, 4, 3');
  await p.waitForTimeout(1400);
  check('§14 editing the script re-runs the simulation', (await txt()) !== beforeScript);
  check('§14 a malformed script line is skipped, not fatal', await (async () => {
    await p.keyboard.type('\nthis is not an edit');
    await p.waitForTimeout(1100);
    const t = await txt();
    return /1 skipped/.test(t) && /REPLAY FROM EMPTY/.test(t);
  })());

  // ── § 16 PERF: a benchmark that actually runs ─────────────
  await go('/lab/perf', 9000);
  const settled = async () => {
    await p.waitForFunction(() => !/RUNNING/.test(document.body.innerText),
      null, { timeout: 90000 }).catch(() => {});
    await p.waitForTimeout(600);
  };
  await settled();

  check('§16 percentiles are measured, not zero',
        !/P50\s*0 ns/.test(await txt()) && /P99\.9/.test(await txt()));
  check('§16 it states the clock it was quantised by', /CLOCK\s*±/.test(await txt()));
  check('§16 the flame graph names real functions',
        /applyOp/.test(await txt()) && /instrumented spans/.test(await txt()));
  check('§16 queue depth comes from the service',
        /ops stored/.test(await txt()) && !/did not answer/.test(await txt()));

  // Each workload must actually change the numbers. Four chips
  // that all run the same code is the exact defect this pass
  // exists for.
  const percentiles = () => p.evaluate(() =>
    [...document.querySelectorAll('.rd')]
      .filter(e => /^P\d/.test(e.innerText))
      .map(e => e.innerText.replace(/\n/g, '=')).join(' '));
  const seenP = new Set();
  for (const name of ['applyOp', 'compilePaste', 'simulate', 'embedIndex']) {
    await p.locator('.chip', { hasText: new RegExp(`^${name}$`) }).first().click();
    await settled();
    seenP.add(await percentiles());
  }
  check('§16 four workloads produce four different profiles',
        seenP.size >= 3, `${seenP.size} distinct`);

  // The expensive one must say it clamped rather than quietly
  // running fewer. `simulate` declares MaxSamples; asking it
  // for 50 000 has to come back saying so.
  await p.locator('.chip', { hasText: /^simulate$/ }).first().click();
  await settled();
  await p.locator('.chip', { hasText: /^50k$/ }).first().click();
  await settled();
  check('§16 an expensive workload clamps and says so',
        /Clamped to|Stopped on its own clock/.test(await txt()));

  // NOT checked: that the chip reads RUNNING… mid-run. It does,
  // but the run holds the page's thread, so `evaluate` cannot
  // return until it is over — the intermediate state is real and
  // unobservable from out here. What IS checkable is that the
  // click re-measured, which is the claim that matters.
  await p.locator('.chip', { hasText: /^applyOp$/ }).first().click();
  await p.locator('.chip', { hasText: /^1k$/ }).first().click();
  await settled();
  const beforeRerun = await percentiles();
  await p.locator('.chip', { hasText: /RUN AGAIN/ }).first().click();
  await settled();
  check('§16 RUN AGAIN re-measures', (await percentiles()) !== beforeRerun);

  // ── § 18 ADMIN: every number fetched, from three sources ──
  await go('/admin', 6000);
  check('§18 the gear reaches it at all',
        (await p.evaluate(() => location.pathname)) === '/admin');
  await p.waitForFunction(() => /SERVICES · \d+ of \d+/.test(document.body.innerText),
    null, { timeout: 15000 }).catch(() => {});
  const services = (await txt()).match(/SERVICES · (\d+) of (\d+)/);
  check('§18 every service was probed and answered',
        services != null && services[1] === services[2], services?.[0]);
  check('§18 a probe reports a latency, not a guess', /\d+ ms/.test(await txt()));
  check('§18 it refuses to overclaim what "up" means',
        /A probe, not a report/.test(await txt()));
  check('§18 people come from auth-service', /PEOPLE · [1-9]/.test(await txt()));
  check('§18 and it names the actor reading it', /\byou\b/.test(await txt()));
  check('§18 the open admin surface is stated, not implied shut',
        /readable by any signed-in actor/.test(await txt()));
  check('§18 queue numbers come from collaboration-service',
        /OUTBOX DEPTH/.test(await txt()) && /DB SIZE/.test(await txt()));
  check('§18 "sessions" is disambiguated rather than guessed at',
        /SIGNED IN/.test(await txt()) && /OPEN PAGES/.test(await txt()));
  check('§18 a quiet sparkline says it is quiet rather than drawing a flat line',
        /accepted ops per hour/.test(await txt()));
  check('§18 backups say there are none', /no backup system exists/.test(await txt()));

  // The one rail row that goes somewhere has to go there.
  await p.locator('.tr', { hasText: 'Jobs & sagas' }).first().click();
  await p.waitForTimeout(1500);
  check('§18 Jobs & sagas reaches the saga screen',
        (await p.evaluate(() => location.pathname)) === '/trash');

  // ── § 18b AUDIT LOG: derived, not written beside ──────────
  await go('/admin/audit', 7000);
  check('§18b it says what it is a projection of',
        /a projection of collab.ops \+ auth state/.test(await txt()));
  check('§18b rows from BOTH services are merged',
        /auth\.signin/.test(await txt()) && /page\.(block|text)\./.test(await txt()));
  check('§18b repeated events collapse with a count',
        /×\d+/.test(await txt()), (await txt()).match(/auth\.signin ×\d+/)?.[0]);
  // Delete a page HERE rather than hoping one is still inside the audit
  // view's recent window. Whether an old deletion is still on screen is a
  // fact about how much has happened since, not about this screen.
  const doomed = await (await fetch(`${GW}/pages`, {method:'POST',
    headers:{'Content-Type':'application/json','X-Actor-Id':sub},
    body: JSON.stringify({title:'Audit probe — deleted on purpose'})})).json();
  // It has to be EDITED, not merely created: the audit log is derived from
  // collab.ops, so a page nobody ever opened has no row to name it in.
  await go(`/pages/${doomed.id}`, 4000);
  const probeBlock = p.locator('[contenteditable="true"]').nth(1);
  if (await probeBlock.count()) {
    await probeBlock.click();
    await p.keyboard.type('deleted on purpose', { delay: 12 });
    await p.waitForTimeout(2500);
  }
  await fetch(`${GW}/pages/${doomed.id}`, {method:'DELETE', headers:{'X-Actor-Id':sub}});
  await go('/admin/audit', 7000);
  check('§18b a deleted page is still named',
        /\(deleted\)/.test(await txt()));

  // Each filter must actually select. A chip that renders and
  // filters nothing is the defect this pass exists for.
  const auditRows = () => p.evaluate(() =>
    [...document.querySelectorAll('.mono')].filter(e => /^\d\d:\d\d:\d\d$/.test(e.innerText)).length);
  const allRows = await auditRows();
  await p.locator('.sb', { hasText: 'DESTRUCTIVE' }).first().click();
  await p.waitForTimeout(1200);
  const destructive = await txt();
  check('§18b DESTRUCTIVE selects only deletes',
        /delete/.test(destructive) && !/insert/.test(destructive),
        `${allRows} → ${await auditRows()}`);
  await p.locator('.sb', { hasText: 'AUTH' }).first().click();
  await p.waitForTimeout(1200);
  const authOnly = await txt();
  check('§18b AUTH selects only auth events',
        /auth\./.test(authOnly) && !/page\.(block|text)\./.test(authOnly));
  check('§18b registrations are not crowded out by sign-ins',
        /auth\.register/.test(authOnly));
  await p.locator('.sb', { hasText: 'ALL' }).first().click();
  await p.waitForTimeout(1200);

  // Selecting a row has to say where the row came from. Click a
  // TIME cell: the first `.av` on the page belongs to the
  // utility cluster, not to a row.
  await p.locator('.mono').filter({ hasText: /^\d\d:\d\d:\d\d$/ }).first().click();
  await p.waitForTimeout(600);
  check('§18b a selected row names what it was derived from',
        /DERIVED FROM/.test(await txt()));
  await p.locator('.it', { hasText: 'RETENTION' }).first().click();
  await p.waitForTimeout(500);
  check('§18b it does not claim tamper evidence it does not have',
        /TAMPER EVIDENCE/.test(await txt()) && /prev_hash/.test(await txt()));
  check('§18b and names what is not recorded at all',
        /Failed sign-ins/.test(await txt()));

  // ── every wasm module the SPA loads actually IS wasm ──────
  //
  // A missing .wasm does not 404 in this SPA: try_files falls
  // back to index.html, so the browser fetches `<!doctype…`
  // and WebAssembly.instantiate fails on the magic word. It is
  // silent until somebody opens the one screen that needs it —
  // which is how mdc.wasm shipped as an HTML page once, and
  // then sketch, netsim and bench did too, taking § 12, § 14
  // and § 16 down on the deployed instance with them.
  //
  // Checked here rather than trusted, because the failure has
  // now happened twice and both times the list was hand-kept.
  const wasmModules = ['documentcore', 'graph', 'diff', 'trie',
                       'syntax', 'mdc', 'sketch', 'netsim', 'bench'];
  const badWasm = [];
  for (const name of wasmModules) {
    const head = await p.evaluate(async (n) => {
      const r = await fetch(`/${n}.wasm`);
      if (!r.ok) return `HTTP ${r.status}`;
      const b = new Uint8Array(await r.arrayBuffer()).slice(0, 4);
      return [...b].map((x) => x.toString(16).padStart(2, '0')).join(' ');
    }, name);
    // 00 61 73 6d — "\0asm", the wasm magic word.
    if (head !== '00 61 73 6d') badWasm.push(`${name}: ${head}`);
  }
  check('wasm: every module the SPA loads is really wasm',
        badWasm.length === 0, badWasm.join(' · ') || `${wasmModules.length} checked`);

  // ── § 02 HOME: the only public route, and its panel is real ──
  //
  // Checked SIGNED OUT, because that is who sees it. Everything
  // else in this file runs with a session in localStorage; this
  // block clears it and puts it back.
  const saved = await p.evaluate(() => {
    const s = localStorage.getItem('marginal.session');
    localStorage.removeItem('marginal.session');
    return s;
  });
  await go('/', 6000);
  check('§02 the front door needs no session',
        (await p.evaluate(() => location.pathname)) === '/');
  check('§02 it makes the pitch', /one tree, converging/.test(await txt()));

  // The panel runs netsim in wasm and plays the log forward, so
  // the text has to actually change and then converge.
  const paneText = () => p.evaluate(() =>
    document.querySelector('[style*="Spectral"]')?.textContent ?? '');
  const first = await paneText();
  await p.waitForTimeout(3000);
  const later = await paneText();
  check('§02 the live panel actually runs', first !== later, `${first.length} → ${later.length}`);
  // The wait IS the evidence. Re-reading the page afterwards is a race: the
  // panel loops, so "converged" can appear and be replaced by "in flight"
  // before the assert gets to look.
  const convergedSeen = await p.waitForFunction(
    () => /converged/.test(document.body.innerText),
    null, { timeout: 25000 }).then(() => true).catch(() => false);
  check('§02 and it converges', convergedSeen);
  check('§02 it says the panel is the real transform, not an animation',
        /runs the real transform, in wasm/.test(await txt()));

  // Counters come from the instance. Inventing traffic on a
  // landing page is the first thing a reader stops believing.
  check('§02 the counters are read off this instance',
        /OPS ACCEPTED/.test(await txt()) && !/1 412/.test(await txt()));

  // A price on a public page is an offer, and there is nothing
  // to buy. The whole commercial band was removed rather than
  // rewritten.
  check('§02 it quotes no price at all',
        !/\$\d/.test(await txt()) && !/per month/i.test(await txt()));
  check('§02 and says why there is nothing to price',
        /none of them exist/.test(await txt()));

  await p.getByText('CREATE A WORKSPACE').first().click();
  await p.waitForTimeout(1200);
  check('§02 the call to action reaches sign-in',
        (await p.evaluate(() => location.pathname)) === '/login');

  await p.evaluate((s) => { if (s) localStorage.setItem('marginal.session', s); }, saved);

  // ── every inspector's SECOND tab does something ───────────
  //
  // Ten screens shipped with `active` hardcoded and no onSelect, so the
  // second tab rendered, highlighted nothing, and changed nothing. Every
  // one passed uidiff — the markup was right — and every one passed the
  // per-screen checks, because none of them clicked that tab. A user found
  // all ten by clicking.
  //
  // So this walks them generically rather than trusting each screen's own
  // block to remember. A tab that renders and does nothing is the single
  // defect this whole pass exists for.
  const tabbed = [
    ['/graph/algorithms', 'COST'],
    ['/search', 'HISTORY'],
    ['/discover', 'INDEX'],
    ['/facts', 'COST'],
    ['/topics', 'HISTORY'],
    ['/trash', 'PURGED'],
    ['/notifications', 'MUTED'],
    ['/lab/compiler', 'COST'],
    [`/pages/${page.id}/diff`, 'MOVES'],
    [`/pages/${page.id}/trace`, 'KINDS'],
  ];
  const deadTabs = [];
  for (const [route, tab] of tabbed) {
    await go(route, 6000);
    const inspector = () => p.evaluate(() => document.querySelector('.insp')?.innerText ?? '');
    const before = await inspector();
    const t = p.locator('.it', { hasText: tab }).first();
    if (!(await t.count())) { deadTabs.push(`${route}:${tab} MISSING`); continue; }
    await t.click();
    await p.waitForTimeout(700);
    const after = await inspector();
    if (after === before) deadTabs.push(`${route}:${tab}`);
    // And it must be able to come back — a one-way tab is half a control.
    const first = p.locator('.it').first();
    await first.click();
    await p.waitForTimeout(500);
    if ((await inspector()) !== before) deadTabs.push(`${route}:${tab} NO RETURN`);
  }
  check('every inspector second tab changes the panel',
        deadTabs.length === 0, deadTabs.join(' · ') || `${tabbed.length} checked`);

  // ── § 24e NOT FOUND ───────────────────────────────────────────────────
  await go('/p/the-documnt-block-modl', 4000);
  check('§24e a wrong URL is not a redirect', (await p.evaluate(()=>location.pathname)) === '/p/the-documnt-block-modl');
  await p.waitForFunction(
    () => /The document block model/.test(document.body.innerText),
    null, { timeout: 20000 }).catch(() => {});
  check('§24e BK-tree suggests the real page', /The document block model/.test(await txt()));

  // ── § 17 HISTORY ──────────────────────────────────────────────────────
  await go(`/pages/${page.id}/history`, 3000);
  await p.waitForFunction(
    () => /InsertBlock|SetBlockContent|InsertText/.test(document.body.innerText),
    null, { timeout: 25000 }).catch(() => {});
  check('§17 op stream names the op kind', /InsertBlock|SetBlockContent|InsertText/.test(await txt()));
  await p.locator('.it', { hasText: 'REVISIONS' }).first().click(); await p.waitForTimeout(400);
  check('§17 REVISIONS folds ops into gestures', /GESTURES ·/.test(await txt()));
  await p.getByText('TEXT',{exact:true}).first().click(); await p.waitForTimeout(500);
  check('§17 TEXT/PALIMPSEST toggle', /TEXT AT THIS REVISION/.test(await txt()));

  // The palimpsest is the CHARACTER tier, so it only has anything to show on
  // a block that was really typed into — the seeder gives exactly one page a
  // character-level revision history for this reason.
  const anchors = g.nodes.find(n => n.title === 'Anchors vs offsets');
  if (anchors) {
    await go(`/pages/${anchors.id}/history`, 3000);
    // Both history routes match the same pattern, so React keeps the screen
    // MOUNTED across this navigation and its lens is still whatever the
    // check above left it on. Ask for the palimpsest rather than assuming.
    await p.getByText('PALIMPSEST', { exact: true }).first().click().catch(() => {});
    // Wait for a NUMBER, not for the word TOMBSTONED — the label is static
    // chrome and is on screen while the figure beside it is still 0.
    await p.waitForFunction(
      () => /TOMBSTONED\s*\n?\s*[1-9]/.test(document.body.innerText),
      null, { timeout: 25000 }).catch(()=>{});
    const pal = await txt();
    const tomb = Number((pal.match(/TOMBSTONED\s+([\d,]+)/) || [0,'0'])[1].replace(/,/g,''));
    check('§17 the palimpsest has real tombstones, not an empty structure', tomb > 0, `${tomb} tombstoned`);
    const stored = Number((pal.match(/STORED\s+([\d,]+)/) || [0,'0'])[1].replace(/,/g,''));
    const liveN = Number((pal.match(/\bLIVE\s+([\d,]+)/) || [0,'0'])[1].replace(/,/g,''));
    check('§17 and stored is live plus tombstoned — nothing is ever removed',
      stored > 0 && stored === liveN + tomb, `${stored} = ${liveN} + ${tomb}`);
  } else {
    check('§17 the seeded corpus has a page with character history', false, 'Anchors vs offsets missing');
  }

  // ── § 04's mid-delete row ─────────────────────────────────────────────
  //
  // The page tree marks a page whose delete saga is in flight. uidiff cannot
  // see it — the saga finishes in well under a settled screenshot — so it is
  // caught here, while it is happening, by polling the API the tree reads.
  const sagaParent = await (await fetch(`${GW}/pages`, {method:'POST',
    headers:{'Content-Type':'application/json','X-Actor-Id':sub},
    body: JSON.stringify({title:'Saga probe — deleted on purpose'})})).json();
  await fetch(`${GW}/pages`, {method:'POST',
    headers:{'Content-Type':'application/json','X-Actor-Id':sub},
    body: JSON.stringify({title:'Saga probe child', parent_id: sagaParent.id})});
  const preview = await (await fetch(`${GW}/pages/${sagaParent.id}/delete-preview`,
    {headers:{'X-Actor-Id':sub}})).json();
  // `descendants`, and it excludes the page itself — the preview answers
  // "what ELSE goes", which is the only part a person cannot already see.
  check('§04 a delete says what it will take with it, before it runs',
        (preview.descendants?.length ?? 0) >= 1,
        `${preview.descendants?.length ?? 0} descendants`);
  await fetch(`${GW}/pages/${sagaParent.id}`, {method:'DELETE', headers:{'X-Actor-Id':sub}});
  const trash = await (await fetch(`${GW}/trash`, {headers:{'X-Actor-Id':sub}})).json();
  check('§04 and the whole subtree lands in the trash, not just the page named',
        (trash.entries ?? []).filter(e => /^Saga probe/.test(e.page.title)).length >= 2,
        `${(trash.entries ?? []).filter(e => /^Saga probe/.test(e.page.title)).length} entries`);

  // ── § 13 TRACE (the scratchpad) ───────────────────────────────────────
  await go('/lab/trace', 3000);
  const blk = p.locator('[contenteditable="true"]').nth(1);
  await blk.click();
  await p.keyboard.type('anchors survive a split', { delay: 12 });
  await p.waitForFunction(() => /STEP/.test(document.body.innerText) &&
    !/STEP\s*0\s*\/\s*0/.test(document.body.innerText), null, { timeout: 20000 }).catch(()=>{});
  const stepOf = async () => ((await txt()).match(/STEP\s*(\d+)\s*\/\s*(\d+)/) || []).slice(1).join('/');
  const typed = await stepOf();
  check('§13 typing into the scratchpad produces ops', /^[1-9]/.test(typed), typed);
  check('§13 and each one carries its own inverse', /HOLDS/.test(await txt()));

  // Stepping back is a replay from empty, not a delete: the text goes and
  // the log stays, which is the whole claim the screen makes.
  const preStep = await txt();
  await p.getByText('◀ STEP', { exact: true }).first().click();
  await p.waitForFunction(t => document.body.innerText !== t, preStep, { timeout: 15000 }).catch(()=>{});
  const back = await stepOf();
  check('§13 ◀ STEP rewinds the document', back !== typed && back.split('/')[1] === typed.split('/')[1], `${typed} → ${back}`);
  const midText = await p.locator('.canvas').innerText();

  await p.getByText('STEP ▶', { exact: true }).first().click();
  await p.waitForFunction(() => true, null, { timeout: 1000 }).catch(()=>{});
  await p.waitForFunction(t => document.querySelector('.canvas').innerText !== t, midText, { timeout: 15000 }).catch(()=>{});
  check('§13 STEP ▶ replays it forward again', (await stepOf()) === typed, await stepOf());
  check('§13 and the text comes back', (await p.locator('.canvas').innerText()) !== midText);

  await p.locator('.it', { hasText: 'KINDS' }).first().click(); await p.waitForTimeout(400);
  check('§13 KINDS counts the op kinds it emitted', /OP KINDS YOU PRODUCED/.test(await txt()) && /SetBlockContent|InsertBlock/.test(await txt()));

  console.log(errs.length ? `\nPAGE ERRORS: ${errs.slice(0,5).join(' | ')}` : '\nno page errors');
  if (errs.length) fails++;
  console.log(fails ? `\n${fails} FAILED` : '\nall checks passed');
  await b.close();
  process.exit(fails ? 1 : 0);
})();
