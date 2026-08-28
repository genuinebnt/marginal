// Seeds fact definitions by TYPING THEM INTO THE REAL EDITOR — the same
// path a person takes, so every op goes through collaboration-service and
// lands in the op log. Nothing here writes to a database.
const { chromium } = require('playwright-core');

const DEFS = [
  ['Architecture', [
    '{{define anchor = A position that survives concurrent edits, unlike an integer offset}}',
    '{{define lamport-clock = A counter per actor, compared pairwise then broken by actor id}}',
    'An {{anchor}} is what an op names. Ordering falls back to {{lamport-clock}}.',
  ]],
  ['Runbook', [
    '{{define outbox = A table written in the same transaction as the change it announces}}',
    'On an incident, check {{outbox}} depth first.',
  ]],
  ['Node W', [
    'The rope is addressed by {{anchor}}, never by offset.',
  ]],
];

(async () => {
  const r = await fetch('http://localhost:8000/auth/login', { method:'POST',
    headers:{'Content-Type':'application/json'},
    body: JSON.stringify({email:'ui-demo@example.com',password:'ui-demo-password-123'})});
  const pair = await r.json();
  const sub = JSON.parse(Buffer.from(pair.access_token.split('.')[1],'base64url')).sub;
  const pages = await (await fetch('http://localhost:8000/pages',{headers:{'X-Actor-Id':sub}})).json();

  const b = await chromium.launch({ channel:'chrome' });
  const p = await b.newPage({ viewport:{width:1440,height:860} });
  await p.goto('http://localhost:5173/', { waitUntil:'domcontentloaded' });
  await p.evaluate(s=>localStorage.setItem('marginal.session',JSON.stringify(s)),
    {actorId:sub,accessToken:pair.access_token,refreshToken:pair.refresh_token});

  for (const [title, lines] of DEFS) {
    const page = pages.pages.find(x => x.title === title);
    if (!page) { console.log('missing page', title); continue; }
    await p.goto('http://localhost:5173/pages/' + page.id, { waitUntil:'networkidle' });
    await p.waitForTimeout(2500);
    const blocks = p.locator('.block-row .editable');
    const n = await blocks.count();
    if (n === 0) { console.log('no blocks on', title); continue; }
    await blocks.nth(n - 1).click();
    await p.keyboard.press('End');
    for (const line of lines) {
      await p.keyboard.press('Enter');
      await p.waitForTimeout(350);
      await p.keyboard.type(line, { delay: 4 });
      await p.waitForTimeout(500);
    }
    await p.waitForTimeout(1600);
    console.log('seeded', title, lines.length);
  }
  await b.close();
})();
