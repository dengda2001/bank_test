// Quantitative mobile metrics that a visual review cannot produce: touch target
// sizes, sub-12px text, and how far a table must be scrolled to reach each column.
import { chromium } from 'playwright';
import fs from 'node:fs';

const BASE = process.env.AUDIT_BASE || 'http://localhost:8081';
const USER = process.env.AUDIT_USER;
const PASS = process.env.AUDIT_PASS;
const W = 375, H = 667;

const browser = await chromium.launch({
  executablePath: '/tmp/mobile-audit/chrome-linux64/chrome',
  args: ['--no-sandbox', '--disable-dev-shm-usage'],
});
const ctx = await browser.newContext({ viewport: { width: W, height: H }, deviceScaleFactor: 2 });
const page = await ctx.newPage();

await page.goto(BASE + '/', { waitUntil: 'domcontentloaded' });
await page.fill('#username', USER);
await page.fill('#password', PASS);
await Promise.all([page.waitForNavigation({ waitUntil: 'domcontentloaded' }).catch(() => {}), page.click('button[type=submit]')]);

// --- find a cash receipt so the void page can be audited at all ---------------
await page.goto(BASE + '/rent-dashboard', { waitUntil: 'networkidle' });
const rowCount = await page.locator('.rent-row, tbody tr').count();
let receiptId = null;
for (let i = 0; i < Math.min(rowCount, 12) && !receiptId; i++) {
  const row = page.locator('tbody tr').nth(i);
  const toggle = row.locator('[data-rent-toggle], .rent-toggle');
  if (await toggle.count()) await toggle.first().click().catch(() => {});
  else await row.click().catch(() => {});
  await page.waitForTimeout(200);
  receiptId = await page.evaluate(() => {
    const a = document.querySelector('a[href*="cash-receipts/void"]');
    return a ? new URL(a.href).searchParams.get('receipt_id') : null;
  });
}
console.log('cash receipt id discovered:', receiptId);
// Also try every tenant detail page's history table.
if (!receiptId) {
  const ids = await page.$$eval('a[href^="/tenants/"]', (a) => a.map((e) => e.getAttribute('href')).filter((h) => /^\/tenants\/\d+$/.test(h)));
  for (const h of [...new Set(ids)]) {
    await page.goto(BASE + h, { waitUntil: 'networkidle' });
    receiptId = await page.evaluate(() => {
      const a = document.querySelector('a[href*="cash-receipts/void"]');
      return a ? new URL(a.href).searchParams.get('receipt_id') : null;
    });
    if (receiptId) { console.log('found on', h, '->', receiptId); break; }
  }
}

// --- measurements -------------------------------------------------------------
const measure = () => {
  const small = [];
  const tiny = [];
  for (const el of document.querySelectorAll('a, button, input, select, textarea, [role=button]')) {
    const r = el.getBoundingClientRect();
    if (r.width === 0 || r.height === 0) continue;
    const st = getComputedStyle(el);
    if (st.visibility === 'hidden') continue;
    const label = (el.textContent || el.getAttribute('aria-label') || el.name || el.id || '').trim().replace(/\s+/g, ' ').slice(0, 28);
    if (r.width < 44 || r.height < 44) {
      small.push({ tag: el.tagName.toLowerCase(), label, w: Math.round(r.width), h: Math.round(r.height) });
    }
    const fs = parseFloat(st.fontSize);
    if (fs < 12) tiny.push({ tag: el.tagName.toLowerCase(), label, fontSize: fs });
  }
  const tables = [];
  for (const wrap of document.querySelectorAll('.table-wrap')) {
    const t = wrap.querySelector('table');
    if (!t) continue;
    const headers = [...t.querySelectorAll('thead th')].map((th, i) => {
      const r = th.getBoundingClientRect();
      // How far the reader must scroll right before this column is fully visible.
      const scrollToSee = Math.max(0, Math.round(r.right + wrap.scrollLeft - (wrap.getBoundingClientRect().left + wrap.clientWidth)));
      return { col: (th.textContent || '').trim().slice(0, 12), scrollToSee };
    });
    tables.push({
      minWidth: getComputedStyle(t).minWidth,
      tableWidth: Math.round(t.getBoundingClientRect().width),
      wrapWidth: wrap.clientWidth,
      scrollRange: Math.round(wrap.scrollWidth - wrap.clientWidth),
      headers,
    });
  }
  const fs = {};
  for (const el of document.querySelectorAll('body *')) {
    const r = el.getBoundingClientRect();
    if (r.width === 0 || r.height === 0) continue;
    const v = parseFloat(getComputedStyle(el).fontSize);
    fs[v] = (fs[v] || 0) + 1;
  }
  return { smallTargets: small, tinyText: tiny, tables, fontHistogram: fs };
};

const routes = [
  ['tenants', '/tenants'],
  ['tenant-detail', null],
  ['rent-dashboard', '/rent-dashboard'],
  ['billing', '/billing'],
  ['expenses', '/expenses'],
  ['cash-receipt-new', '/cash-receipts/new'],
  ['cash-receipt-void', receiptId ? `/cash-receipts/void?receipt_id=${receiptId}` : null],
];

const out = {};
for (const [name, url] of routes) {
  let target = url;
  if (!target) {
    const id = await page.evaluate(async (b) => {
      const r = await fetch(b + '/tenants');
      return ((await r.text()).match(/\/tenants\/(\d+)"/) || [])[1];
    });
    target = `/tenants/${id || 6}`;
  }
  await page.goto(BASE + target, { waitUntil: 'networkidle' });
  const m = await page.evaluate(measure);
  out[name] = m;
  const uniq = (a) => [...new Map(a.map((x) => [x.tag + x.label + x.w + x.h, x])).values()];
  console.log(
    `${name.padEnd(18)} 小目标=${uniq(m.smallTargets).length} <12px文本=${uniq(m.tinyText).length} ` +
    `表格滚动范围=${m.tables.map((t) => t.scrollRange).join(',') || '-'}`,
  );
}

fs.writeFileSync('/tmp/mobile-audit/out/metrics.json', JSON.stringify(out, null, 2));
await browser.close();
console.log('\nwrote /tmp/mobile-audit/out/metrics.json');
