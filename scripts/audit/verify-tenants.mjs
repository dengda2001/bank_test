// Verification pass for the tenants list page: row heights, column rects, and
// what the right-hand columns look like once the table is scrolled horizontally.
import { chromium } from 'playwright';
import { launchOptions } from './launch.mjs';
import fs from 'node:fs';

const BASE = process.env.AUDIT_BASE || 'http://localhost:8081';
const USER = process.env.AUDIT_USER || 'rentops-demo';
const PASS = process.env.AUDIT_PASS || 'RentDemo#2026';
const W = 375, H = 667;

const browser = await chromium.launch(launchOptions());
const ctx = await browser.newContext({ viewport: { width: W, height: H }, deviceScaleFactor: 2 });
const page = await ctx.newPage();

await page.goto(BASE + '/', { waitUntil: 'domcontentloaded' });
if (await page.locator('#username').count()) {
  await page.fill('#username', USER);
  await page.fill('#password', PASS);
  await Promise.all([
    page.waitForNavigation({ waitUntil: 'domcontentloaded' }).catch(() => {}),
    page.click('button[type=submit]'),
  ]);
}
console.log('logged in as', USER);

await page.goto(BASE + '/tenants', { waitUntil: 'networkidle' });

const info = await page.evaluate(() => {
  const wrap = document.querySelector('.table-wrap');
  const table = wrap.querySelector('table');
  const heads = [...table.querySelectorAll('thead th')].map((th) => {
    const r = th.getBoundingClientRect();
    return { col: th.textContent.trim(), left: Math.round(r.left), right: Math.round(r.right), w: Math.round(r.width) };
  });
  const rows = [...table.querySelectorAll('tbody tr.tenant-row')].map((tr) => Math.round(tr.getBoundingClientRect().height));
  const cellWrap = [...table.querySelectorAll('tbody tr.tenant-row')].slice(0, 2).map((tr) =>
    [...tr.children].map((td) => ({ w: Math.round(td.getBoundingClientRect().width), text: td.textContent.trim().replace(/\s+/g, ' ').slice(0, 40) })),
  );
  const actions = [...table.querySelectorAll('.row-actions .btn')].slice(0, 2).map((a) => {
    const r = a.getBoundingClientRect();
    return { label: a.textContent.trim(), w: Math.round(r.width), h: Math.round(r.height), left: Math.round(r.left) };
  });
  const h1 = document.querySelector('h1');
  const list = document.querySelector('#tenant-list-title');
  return {
    wrapWidth: wrap.clientWidth,
    scrollRange: wrap.scrollWidth - wrap.clientWidth,
    tableWidth: Math.round(table.getBoundingClientRect().width),
    heads,
    rowHeights: rows,
    avgRowHeight: Math.round(rows.reduce((a, b) => a + b, 0) / rows.length),
    firstRowCells: cellWrap,
    actionButtons: actions,
    docHeight: document.documentElement.scrollHeight,
    yListTop: Math.round(list.getBoundingClientRect().top + window.scrollY),
    yFirstRow: Math.round(table.querySelector('tbody tr').getBoundingClientRect().top + window.scrollY),
    navTop: Math.round(document.querySelector('.sidebar').getBoundingClientRect().top),
    navBottom: Math.round(document.querySelector('.sidebar').getBoundingClientRect().bottom),
  };
});
console.log(JSON.stringify(info, null, 2));

// What the table looks like once swiped to the right edge.
await page.evaluate(() => {
  const wrap = document.querySelector('.table-wrap');
  wrap.scrollLeft = wrap.scrollWidth - wrap.clientWidth;
});
await page.evaluate(() => window.scrollTo(0, 900));
await page.waitForTimeout(400);
await page.screenshot({ path: '/tmp/mobile-audit/out/verify-tenants-scrolled-right.png' });
await page.evaluate(() => window.scrollTo(0, 1700));
await page.waitForTimeout(300);
await page.screenshot({ path: '/tmp/mobile-audit/out/verify-tenants-scrolled-right-2.png' });

// Row expanded, scrolled right: which history columns become visible.
await page.goto(BASE + '/tenants', { waitUntil: 'networkidle' });
await page.locator('.tenant-row').first().click();
await page.waitForTimeout(400);
const hist = await page.evaluate(() => {
  const wrap = document.querySelector('.table-wrap');
  const t = document.querySelector('.tenant-history-table');
  const row = document.querySelector('.tenant-history-row');
  const inner = t.closest('.table-wrap');
  return {
    histTableWidth: Math.round(t.getBoundingClientRect().width),
    histMinWidth: getComputedStyle(t).minWidth,
    innerWrapWidth: inner.clientWidth,
    outerWrapWidth: wrap.clientWidth,
    outerScrollRange: wrap.scrollWidth - wrap.clientWidth,
    innerScrollRange: inner.scrollWidth - inner.clientWidth,
    historyRowHeight: Math.round(row.getBoundingClientRect().height),
    monthRows: t.querySelectorAll('tbody tr.tenant-month-row').length,
    headCols: [...t.querySelectorAll('thead th')].map((th) => {
      const r = th.getBoundingClientRect();
      return { col: th.textContent.trim(), left: Math.round(r.left), right: Math.round(r.right) };
    }),
    captionText: document.querySelector('.tenant-history-head .tiny') ? document.querySelector('.tenant-history-head .tiny').getBoundingClientRect().left : null,
  };
});
console.log('HISTORY', JSON.stringify(hist, null, 2));
await page.evaluate(() => {
  const wrap = document.querySelector('.table-wrap');
  wrap.scrollLeft = wrap.scrollWidth - wrap.clientWidth;
  window.scrollTo(0, 900);
});
await page.waitForTimeout(400);
await page.screenshot({ path: '/tmp/mobile-audit/out/verify-expanded-scrolled-right.png' });

await browser.close();
