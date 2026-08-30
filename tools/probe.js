const { chromium } = require('playwright-core');
const GW='http://localhost:8000', APP='http://localhost:5173';
(async () => {
  const r = await fetch(`${GW}/auth/login`, { method:'POST', headers:{'Content-Type':'application/json'},
    body: JSON.stringify({email:'ui-demo@example.com',password:'ui-demo-password-123'})});
  const pair = await r.json();
  const sub = JSON.parse(Buffer.from(pair.access_token.split('.')[1],'base64url')).sub;
  const b = await chromium.launch({ channel: 'chrome' });
  const p = await b.newPage({viewport:{width:1440,height:900}});
  p.on('pageerror', e => console.log('  PAGEERROR', e.message.slice(0,160)));
  await p.goto(APP+'/', {waitUntil:'domcontentloaded'});
  await p.evaluate(s => localStorage.setItem('marginal.session', JSON.stringify(s)),
    {actorId:sub, accessToken:pair.access_token, refreshToken:pair.refresh_token});
  for (const route of ['/lab/netcode','/lab/analytics','/lab/compiler','/lab/perf']) {
    await p.goto(APP+route, {waitUntil:'networkidle'});
    await p.waitForTimeout(6000);
    const tas = p.locator('textarea');
    const n = await tas.count();
    let typed = 'n/a';
    if (n > 0) {
      const box = await tas.first().boundingBox();
      const before = await tas.first().inputValue();
      await tas.first().click({force:true});
      await p.keyboard.type('XYZ');
      await p.waitForTimeout(600);
      const after = await tas.first().inputValue();
      typed = (after !== before) ? 'TYPES OK' : 'CANNOT TYPE';
      console.log(`${route.padEnd(18)} textareas=${n} ${typed} box=${box?Math.round(box.y)+'x'+Math.round(box.height):'NOT VISIBLE'}`);
    } else {
      console.log(`${route.padEnd(18)} textareas=0  (sliders/chips only?)`);
    }
  }
  await b.close();
})();
