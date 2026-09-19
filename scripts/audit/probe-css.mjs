import { chromium } from 'playwright';
import { launchOptions } from './launch.mjs';
const BASE = 'http://localhost:8081';
const b = await chromium.launch(launchOptions());
const ctx = await b.newContext({ viewport: { width: 375, height: 900 } });
const p = await ctx.newPage();
await p.goto(BASE + '/', { waitUntil: 'domcontentloaded' });
await p.fill('#username', process.env.AUDIT_USER); await p.fill('#password', process.env.AUDIT_PASS);
await Promise.all([p.waitForNavigation({ waitUntil: 'domcontentloaded' }).catch(()=>{}), p.click('button[type=submit]')]);
// revoke preview (GET) — its CSS block is in the same file as the payer preview's.
await p.goto(BASE + '/billing/revoke?transaction_id=600', { waitUntil: 'networkidle' });
const revoke = await p.evaluate(() => ({
  hasInput44: document.documentElement.innerHTML.includes('button,a,input{min-height:44px}'),
  reasonH: (() => { const e = document.querySelector('input[name=reason]'); return e ? Math.round(e.getBoundingClientRect().height) : null; })(),
}));
console.log('revoke:', JSON.stringify(revoke));
// payer preview (POST) — assert its own block survived escaping.
const html = await p.evaluate(async () => {
  const r = await fetch('/billing/payer/preview', { method: 'POST', credentials: 'same-origin' });
  return r.text();
});
for (const frag of ['input:not([type=checkbox]){min-height:44px}', 'label.muted{display:inline-flex;align-items:center;gap:6px;min-height:44px}']) {
  console.log(`  ${html.includes(frag) ? 'PRESENT' : 'MISSING'}  ${frag}`);
}
await b.close();
