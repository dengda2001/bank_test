// Enumerate every control on a page with its measured box, so a sub-44px control
// can be traced to the exact rule that wins over the shared one.
import { chromium } from 'playwright';

const BASE = process.env.AUDIT_BASE || 'http://localhost:8081';
const b = await chromium.launch({ executablePath: '/tmp/mobile-audit/chrome-linux64/chrome', args: ['--no-sandbox', '--disable-dev-shm-usage'] });
const ctx = await b.newContext({ viewport: { width: 375, height: 800 } });
const p = await ctx.newPage();
await p.goto(BASE + '/', { waitUntil: 'domcontentloaded' });
await p.fill('#username', process.env.AUDIT_USER);
await p.fill('#password', process.env.AUDIT_PASS);
await Promise.all([p.waitForNavigation({ waitUntil: 'domcontentloaded' }).catch(() => {}), p.click('button[type=submit]')]);

await p.goto(BASE + '/tenants', { waitUntil: 'networkidle' });
const tid = await p.evaluate(() => (document.body.innerHTML.match(/\/tenants\/(\d+)/) || [])[1]);
console.error(`discovered tenant id = ${tid}`);

for (const url of (process.env.PROBE_URLS || `/tenants/${tid},/tenants`).split(',')) {
  await p.goto(BASE + url, { waitUntil: 'networkidle', timeout: 30000 });
  console.log(`\n===== ${url} =====`);
  const rows = await p.evaluate(() => [...document.querySelectorAll('input, select, textarea, a.btn, button.btn, a.month-nav')]
    .map((e) => {
      const r = e.getBoundingClientRect();
      const cls = typeof e.className === 'string' ? e.className : '';
      return { s: `${e.tagName.toLowerCase()} type=${e.getAttribute('type') || '-'} class="${cls}" name=${e.name || '-'}`, w: Math.round(r.width), h: Math.round(r.height) };
    })
    .filter((x) => x.w > 0 && x.h > 0)
    .map((x) => `${String(x.w).padStart(4)}x${String(x.h).padStart(3)}  ${x.s}`));
  console.log(rows.join('\n'));
}
await b.close();
