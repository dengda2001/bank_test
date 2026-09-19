// P0-3 says amounts render truncated into plausible-looking wrong numbers. That
// claim drives a P0 rating, so measure it in the DOM rather than trusting a
// screenshot reading. Also re-checks the 26x16 touch target on the dashboard.
import { chromium } from 'playwright';
import { launchOptions } from './launch.mjs';

const BASE = process.env.AUDIT_BASE || 'http://localhost:8081';
const USER = process.env.AUDIT_USER;
const PASS = process.env.AUDIT_PASS;

const browser = await chromium.launch(launchOptions());
const ctx = await browser.newContext({ viewport: { width: 375, height: 667 }, deviceScaleFactor: 2 });
const page = await ctx.newPage();
await page.goto(BASE + '/', { waitUntil: 'domcontentloaded' });
await page.fill('#username', USER);
await page.fill('#password', PASS);
await Promise.all([page.waitForNavigation({ waitUntil: 'domcontentloaded' }).catch(() => {}), page.click('button[type=submit]')]);

// --- /billing: how much of each column is actually inside the viewport? ------
await page.goto(BASE + '/billing', { waitUntil: 'networkidle' });
const billing = await page.evaluate(() => {
  const wrap = document.querySelector('.table-wrap');
  const table = wrap.querySelector('table');
  const wr = wrap.getBoundingClientRect();
  const ths = [...table.querySelectorAll('thead th')];
  const row = table.querySelector('tbody tr');
  const tds = row ? [...row.querySelectorAll('td')] : [];
  const report = (cells, isHeader) =>
    cells.map((c, i) => {
      const r = c.getBoundingClientRect();
      const visibleLeft = Math.max(r.left, wr.left);
      const visibleRight = Math.min(r.right, wr.right);
      const visibleW = Math.max(0, visibleRight - visibleLeft);
      const fullW = r.width;
      // Text that overflows a clipping container is silently cut mid-glyph.
      const text = (c.textContent || '').trim().replace(/\s+/g, ' ');
      const clipped = c.scrollWidth > c.clientWidth + 1;
      return {
        col: i,
        label: (ths[i] ? ths[i].textContent : '').trim().slice(0, 12),
        fullW: Math.round(fullW),
        visibleW: Math.round(visibleW),
        fraction: fullW ? +(visibleW / fullW).toFixed(2) : 0,
        // 0 = entirely off-screen, 1 = entirely on-screen, in between = sliced
        sliced: visibleW > 0 && visibleW < fullW - 1,
        selfClipped: clipped,
        text: text.slice(0, 30),
      };
    });
  return { wrapWidth: Math.round(wr.width), headers: report(ths, true), cells: report(tds, false) };
});
console.log('=== /billing @375, scroll=0 (wrapWidth ' + billing.wrapWidth + ') ===');
for (const c of billing.cells) {
  const state = c.visibleW === 0 ? 'OFF-SCREEN' : c.sliced ? '*** SLICED ***' : 'ok';
  console.log(`  col${c.col} ${c.label.padEnd(12)} full=${String(c.fullW).padStart(4)} visible=${String(c.visibleW).padStart(4)} ${state}  "${c.text}"`);
}

// --- /rent-dashboard: the smallest touch target in the app ------------------
await page.goto(BASE + '/rent-dashboard', { waitUntil: 'networkidle' });
const dash = await page.evaluate(() => {
  const out = [];
  for (const a of document.querySelectorAll('a')) {
    const r = a.getBoundingClientRect();
    if (r.width === 0 || r.height === 0) continue;
    if (r.width < 44 || r.height < 44) {
      out.push({ text: (a.textContent || '').trim().replace(/\s+/g, ' ').slice(0, 20), w: Math.round(r.width), h: Math.round(r.height), href: (a.getAttribute('href') || '').slice(0, 24) });
    }
  }
  out.sort((x, y) => x.w * x.h - y.w * y.h);
  return out.slice(0, 6);
});
console.log('\n=== /rent-dashboard smallest links ===');
for (const t of dash) console.log(`  ${String(t.w + 'x' + t.h).padStart(8)}  "${t.text}"  -> ${t.href}`);

await browser.close();
