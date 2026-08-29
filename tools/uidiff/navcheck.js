// Every nav destination must resolve to its own screen, not the catch-all.
// §9.4: a nav that lists more routes than exist is a design lying about its
// own coverage — and this is the check that keeps it honest.
const { chromium } = require('playwright-core');
(async () => {
  const r = await fetch('http://localhost:8000/auth/login', { method:'POST',
    headers:{'Content-Type':'application/json'},
    body: JSON.stringify({email:'ui-demo@example.com',password:'ui-demo-password-123'})});
  const pair = await r.json();
  const sub = JSON.parse(Buffer.from(pair.access_token.split('.')[1],'base64url')).sub;
  const b = await chromium.launch({ channel:'chrome' });
  const p = await b.newPage({ viewport:{width:1440,height:900} });
  await p.goto('http://localhost:5173/', { waitUntil:'domcontentloaded' });
  await p.evaluate(s=>localStorage.setItem('marginal.session',JSON.stringify(s)),
    {actorId:sub,accessToken:pair.access_token,refreshToken:pair.refresh_token});
  for (const route of ['/pages','/read','/search','/graph','/history','/lab','/topics','/facts','/graph/algorithms']) {
    await p.goto('http://localhost:5173' + route, { waitUntil:'networkidle' });
    await p.waitForTimeout(1200);
    const landed = await p.evaluate(() => location.pathname);
    const status = await p.evaluate(() => {
      const s = document.querySelector('.status span');
      return s ? s.textContent.trim() : '(no status bar)';
    });
    const ok = landed === route;
    console.log(`${ok ? ' ok ' : 'FALL'}  ${route.padEnd(20)} -> ${landed.padEnd(20)} ${status}`);
  }
  await b.close();
})();
