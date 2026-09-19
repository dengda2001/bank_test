// Independent measurement of 09-19-list-pages-alignment AC 12:
// ten list pages at 1024 / 1366 / 1440 / 1920 must have
// documentElement.scrollWidth === documentElement.clientWidth.
//
// Reports the worst overflow and names the widest element so a failure is
// actionable rather than just a boolean.
//
// Usage: AUDIT_BASE=... AUDIT_USER=... AUDIT_PASS=... node scripts/audit/probe-four-widths.mjs
import { chromium } from 'playwright';
import { launchOptions } from './launch.mjs';

const BASE = (process.env.AUDIT_BASE || 'http://127.0.0.1:18094').replace(/\/+$/, '');
const USER = process.env.AUDIT_USER;
const PASS = process.env.AUDIT_PASS;

const PAGES = [
  ['/properties?period=2026-09&status=all', '房产管理'],
  ['/rooms?period=2026-09&status=all', '房间管理'],
  ['/tenants', '租客管理'],
  ['/tenancies?period=2026-09&status=all', '租约管理'],
  ['/bills?period=2026-09', '应收账单'],
  ['/transactions?scope=pending', '流水匹配'],
  ['/dunning?period=2026-09', '催收任务'],
  ['/cash-receipts?period=2026-09', '现金补录'],
  ['/expenses?period=2026-09', '费用支出'],
  ['/bank', '银行设置'],
];
const WIDTHS = [1024, 1366, 1440, 1920];

const MEASURE = `(() => {
  const doc = document.documentElement;
  const overflow = doc.scrollWidth - doc.clientWidth;
  let worst = null;
  if (overflow > 0) {
    for (const node of document.querySelectorAll('body *')) {
      if (node.offsetParent === null && node.tagName !== 'BODY') continue;
      const r = node.getBoundingClientRect();
      if (r.right - doc.clientWidth > 0.5) {
        const excess = r.right - doc.clientWidth;
        if (!worst || excess > worst.excess) {
          worst = { excess: Math.round(excess), tag: node.tagName.toLowerCase(),
                    cls: (node.className && String(node.className).slice(0, 60)) || '' };
        }
      }
    }
  }
  return { scrollWidth: doc.scrollWidth, clientWidth: doc.clientWidth, overflow, worst, title: document.title };
})()`;

const browser = await chromium.launch(launchOptions());
const results = [];
let failures = 0;

for (const width of WIDTHS) {
  const ctx = await browser.newContext({ viewport: { width, height: 900 } });
  const page = await ctx.newPage();
  await page.goto(BASE + '/', { waitUntil: 'domcontentloaded' });
  await page.fill('#username', USER);
  await page.fill('#password', PASS);
  await Promise.all([
    page.waitForNavigation({ waitUntil: 'domcontentloaded' }).catch(() => {}),
    page.click('button[type=submit]'),
  ]);
  for (const [url, label] of PAGES) {
    const resp = await page.goto(BASE + url, { waitUntil: 'networkidle', timeout: 30000 });
    await page.waitForTimeout(250);
    const m = await page.evaluate(MEASURE);
    const ok = m.scrollWidth === m.clientWidth;
    if (!ok) failures += 1;
    results.push({ width, url, label, status: resp?.status(), ...m, ok });
    console.log(
      `${ok ? 'OK  ' : 'FAIL'} ${String(width).padStart(4)} ${url.padEnd(36)} ` +
      `scroll=${m.scrollWidth} client=${m.clientWidth} overflow=${m.overflow} status=${resp?.status()}` +
      (m.worst ? ` widest=${m.worst.tag}.${m.worst.cls} +${m.worst.excess}px` : '')
    );
  }
  await ctx.close();
}

await browser.close();
console.log(`\ntotal=${results.length} failures=${failures}`);
