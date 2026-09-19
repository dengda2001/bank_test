// Why does /billing scroll the page horizontally at 1440 when nothing else does?
// Walk every element and report those whose right edge passes the viewport, with
// their parent's right edge, so the root offender is distinguishable from its
// descendants (a child inside an already-overflowing parent is not the cause).
import { chromium } from 'playwright';
import { launchOptions } from './launch.mjs';

const BASE = process.env.AUDIT_BASE || 'http://localhost:8081';
const W = Number(process.env.PROBE_W || 1440);
const browser = await chromium.launch(launchOptions());
const ctx = await browser.newContext({ viewport: { width: W, height: 900 } });
const page = await ctx.newPage();
await page.goto(BASE + '/', { waitUntil: 'domcontentloaded' });
await page.fill('#username', process.env.AUDIT_USER);
await page.fill('#password', process.env.AUDIT_PASS);
await Promise.all([page.waitForNavigation({ waitUntil: 'domcontentloaded' }).catch(() => {}), page.click('button[type=submit]')]);

for (const url of (process.env.PROBE_URLS || '/billing').split(',')) {
  await page.goto(BASE + url, { waitUntil: 'networkidle', timeout: 30000 });
  const out = await page.evaluate(() => {
    const vw = document.documentElement.clientWidth;
    const lim = vw + 1;
    const rows = [];
    for (const el of document.querySelectorAll('*')) {
      const r = el.getBoundingClientRect();
      if (r.width === 0 || r.height === 0) continue;
      if (r.right <= lim) continue;
      const pr = el.parentElement?.getBoundingClientRect();
      rows.push({
        sel: el.tagName.toLowerCase() + (el.className && typeof el.className === 'string' ? '.' + el.className.trim().split(/\s+/).join('.') : ''),
        right: Math.round(r.right), w: Math.round(r.width),
        parentRight: pr ? Math.round(pr.right) : null,
        parentOverflows: pr ? pr.right > lim : false,
        text: (el.textContent || '').trim().slice(0, 40),
      });
    }
    return { vw, scrollW: document.documentElement.scrollWidth, rows: rows.slice(0, 40) };
  });
  console.log(`\n===== ${url}  vw=${out.vw} scrollW=${out.scrollW} =====`);
  for (const r of out.rows) {
    const root = r.parentOverflows ? '  (child of an overflowing parent)' : '  <== ROOT OFFENDER';
    console.log(`right=${String(r.right).padStart(5)} w=${String(r.w).padStart(5)} parent=${String(r.parentRight).padStart(5)}  ${r.sel}${root}`);
    if (r.text) console.log(`        "${r.text}"`);
  }
}
await browser.close();
