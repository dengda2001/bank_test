// The audit's offender probe reports elements whose right edge passes the viewport,
// which includes two classes of false positive: content inside an overflow-x:auto
// scroller (it scrolls in its own box, not the page) and the off-canvas drawer
// (translateX(-100%), i.e. off-screen *left*). This walks each offender's ancestor
// chain and classifies it, so "offenders=25" becomes a statement about what they
// actually are. Page overflow is asserted separately — it is the number that matters.
import { chromium } from 'playwright';
import { launchOptions } from './launch.mjs';
import fs from 'node:fs';

const BASE = process.env.AUDIT_BASE || 'http://localhost:8081';
const report = JSON.parse(fs.readFileSync(process.env.REPORT || '/tmp/mobile-audit/out/report.json', 'utf8'));

const browser = await chromium.launch(launchOptions());

// Group the recorded offenders by page+viewport so each page is visited once.
const groups = new Map();
for (const p of report.pages) {
  if (!p.result.offenders.length) continue;
  const k = p.viewport + '|' + p.url;
  if (!groups.has(k)) groups.set(k, { viewport: p.viewport, url: p.url, page: p.page, selectors: [] });
  groups.get(k).selectors.push(...p.result.offenders.map((o) => o.selector));
}

for (const [k, g] of groups) {
  const [w, h] = g.viewport.split('x').map(Number);
  const ctx = await browser.newContext({ viewport: { width: w, height: h } });
  const page = await ctx.newPage();
  await page.goto(BASE + '/', { waitUntil: 'domcontentloaded' });
  await page.fill('#username', process.env.AUDIT_USER);
  await page.fill('#password', process.env.AUDIT_PASS);
  await Promise.all([page.waitForNavigation({ waitUntil: 'domcontentloaded' }).catch(() => {}), page.click('button[type=submit]')]);
  await page.goto(BASE + g.url, { waitUntil: 'networkidle', timeout: 30000 });

  const out = await page.evaluate((sels) => {
    const seen = new Map();
    for (const el of document.querySelectorAll('*')) {
      const b = el.getBoundingClientRect();
      if (b.width === 0 || b.height === 0) continue;
      const sel = el.tagName.toLowerCase() + (typeof el.className === 'string' && el.className.trim() ? '.' + el.className.trim().split(/\s+/).join('.') : '');
      if (!sels.includes(sel)) continue;
      // Classify by walking the ancestors.
      let cls = 'PAGE-LEVEL?';
      for (let n = el; n && n !== document.documentElement; n = n.parentElement) {
        const cs = getComputedStyle(n);
        if ((cs.overflowX === 'auto' || cs.overflowX === 'scroll') && n.scrollWidth > n.clientWidth) { cls = 'in scroller ' + (n.className || n.tagName); break; }
        if (cs.position === 'fixed' && (cs.transform || '').includes('matrix') && n.getBoundingClientRect().right <= 0) { cls = 'off-canvas ' + (n.className || n.tagName); break; }
        if ((cs.transform || '').includes('matrix') && n.getBoundingClientRect().right <= 0) { cls = 'off-canvas ' + (n.className || n.tagName); break; }
      }
      seen.set(sel, { sel, cls, n: (seen.get(sel)?.n || 0) + 1 });
    }
    return { pageOverflow: document.documentElement.scrollWidth - document.documentElement.clientWidth, rows: [...seen.values()] };
  }, g.selectors);

  const buckets = new Map();
  for (const r of out.rows) buckets.set(r.cls, (buckets.get(r.cls) || 0) + r.n);
  console.log(`\n== ${g.page} ${g.viewport}  页面溢出=${out.pageOverflow}`);
  for (const [cls, n] of buckets) console.log(`   ${String(n).padStart(3)} x  ${cls}`);
  const unexplained = out.rows.filter((r) => r.cls === 'PAGE-LEVEL?');
  if (unexplained.length) console.log('   未能归类:', unexplained.map((r) => r.sel).join(', '));
  if (out.pageOverflow > 0) console.log('   ** 页面级横向溢出 **');
  await ctx.close();
}

await browser.close();
