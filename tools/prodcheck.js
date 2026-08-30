// Clean-browser check against production — no cache, no stored session.
// Answers one question per report: real bug, or stale bundle?
const { chromium } = require('playwright-core');
const O = 'https://marginal.genuinebasil.dev';
const [email, password] = [process.env.SEED_EMAIL, process.env.SEED_PASSWORD];
(async () => {
  const r = await fetch(`${O}/api/auth/login`, {method:'POST',headers:{'Content-Type':'application/json'},
    body: JSON.stringify({email, password})});
  if (!r.ok) { console.log('LOGIN FAILED', r.status, (await r.text()).slice(0,120)); process.exit(1); }
  const pair = await r.json();
  const sub = JSON.parse(Buffer.from(pair.access_token.split('.')[1],'base64url')).sub;
  const b = await chromium.launch({channel:'chrome'});
  const ctx = await b.newContext({viewport:{width:1440,height:900}, bypassCSP:true});
  const p = await ctx.newPage();
  p.on('pageerror', e => console.log('  PAGEERROR:', e.message.slice(0,140)));
  await p.goto(O+'/', {waitUntil:'domcontentloaded'});
  await p.evaluate(s => localStorage.setItem('marginal.session', JSON.stringify(s)),
    {actorId:sub, accessToken:pair.access_token, refreshToken:pair.refresh_token});

  const txt = () => p.evaluate(()=>document.body.innerText);
  const go = async (route, ms=7000) => { await p.goto(O+route,{waitUntil:'networkidle'}); await p.waitForTimeout(ms); };

  // 1. wordmark links home
  await go('/lab');
  const wm = await p.evaluate(()=>{const e=document.querySelector('.wm'); return e?e.tagName+':'+(e.getAttribute('href')||'none'):'MISSING';});
  console.log('1 wordmark      :', wm);

  // 2. compiler stage chips
  await go('/lab/compiler');
  const before = (await txt()).length;
  await p.locator('.sb').filter({hasText:'AST'}).first().click(); await p.waitForTimeout(900);
  console.log('2 compiler AST  :', (await txt()).length !== before ? 'CHANGES' : 'DEAD');

  // 3. graph COST tab
  await go('/graph/algorithms');
  const g0 = (await txt()).slice(-500);
  await p.locator('.it').filter({hasText:'COST'}).first().click(); await p.waitForTimeout(700);
  console.log('3 graph COST    :', (await txt()).slice(-500) !== g0 ? 'CHANGES' : 'DEAD');

  // 4. graph animation (unrevealed grey exists?)
  await p.locator('.sb').filter({hasText:'COMPONENTS'}).first().click(); await p.waitForTimeout(300);
  const grey = await p.evaluate(()=>[...document.querySelectorAll('svg circle')].filter(c=>c.getAttribute('fill')==='#3A3833').length);
  console.log('4 graph anim    :', grey>0 ? `ANIMATING (${grey} pending)` : 'NOT ANIMATING');

  // 5. search HISTORY tab
  await go('/search');
  const s0 = (await txt()).slice(-400);
  await p.locator('.it').filter({hasText:'HISTORY'}).first().click(); await p.waitForTimeout(700);
  console.log('5 search HISTORY:', (await txt()).slice(-400) !== s0 ? 'CHANGES' : 'DEAD');

  // 6. discover INDEX tab + layers
  await go('/discover');
  const d0 = (await txt()).slice(-400);
  await p.locator('.it').filter({hasText:'INDEX'}).first().click(); await p.waitForTimeout(700);
  console.log('6 discover INDEX:', (await txt()).slice(-400) !== d0 ? 'CHANGES' : 'DEAD');
  const layers = (await txt()).match(/L\d/g);
  console.log('  hnsw layers   :', layers ? [...new Set(layers)].join(',') : 'none shown');

  // 7. netcode typing
  await go('/lab/netcode');
  const ta = p.locator('textarea').first();
  if (await ta.count()) {
    const v0 = await ta.inputValue(); await ta.click(); await p.keyboard.type('ZZ'); await p.waitForTimeout(700);
    console.log('7 netcode type  :', (await ta.inputValue())!==v0 ? 'TYPES OK' : 'CANNOT TYPE');
  } else console.log('7 netcode type  : NO TEXTAREA');
  await b.close();
})();
