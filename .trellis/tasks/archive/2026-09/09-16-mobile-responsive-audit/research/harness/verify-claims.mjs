// Independently check the two claims that decide the remediation design:
//  1. On /tenants, once the table is scrolled fully right to reach 操作, is the
//     row's tenant NAME still on screen? If not, a sticky first column is the fix.
//  2. On /expenses, is the 类别 column really narrow enough to break CJK per-glyph?
// Both came from screenshot reading, which can be wrong; measure the DOM instead.
import { chromium } from 'playwright';

const BASE = process.env.AUDIT_BASE || 'http://localhost:8081';
const USER = process.env.AUDIT_USER;
const PASS = process.env.AUDIT_PASS;

const browser = await chromium.launch({
  executablePath: '/tmp/mobile-audit/chrome-linux64/chrome',
  args: ['--no-sandbox', '--disable-dev-shm-usage'],
});
const ctx = await browser.newContext({ viewport: { width: 375, height: 667 }, deviceScaleFactor: 2 });
const page = await ctx.newPage();
await page.goto(BASE + '/', { waitUntil: 'domcontentloaded' });
await page.fill('#username', USER);
await page.fill('#password', PASS);
await Promise.all([page.waitForNavigation({ waitUntil: 'domcontentloaded' }).catch(() => {}), page.click('button[type=submit]')]);

// --- claim 1: /tenants row identity survives scrolling to the actions --------
await page.goto(BASE + '/tenants', { waitUntil: 'networkidle' });
const tenants = await page.evaluate(() => {
  const wrap = document.querySelector('.table-wrap');
  const vis = () => {
    const wr = wrap.getBoundingClientRect();
    return { left: wr.left, right: wr.right };
  };
  const atZero = [...wrap.querySelectorAll('tbody tr')].map((tr) => {
    const tds = [...tr.querySelectorAll('td')].map((td, i) => {
      const r = td.getBoundingClientRect();
      const w = vis();
      return { col: i, text: (td.textContent || '').trim().replace(/\s+/g, ' ').slice(0, 14), left: Math.round(r.left), right: Math.round(r.right), fullyVisible: r.left >= w.left - 1 && r.right <= w.right + 1 };
    });
    return tds;
  })[0];
  const headers = [...wrap.querySelectorAll('thead th')].map((th) => (th.textContent || '').trim());

  wrap.scrollLeft = wrap.scrollWidth; // all the way right
  const atMax = [...wrap.querySelectorAll('tbody tr')].map((tr) => {
    const tds = [...tr.querySelectorAll('td')].map((td, i) => {
      const r = td.getBoundingClientRect();
      const w = vis();
      return { col: i, text: (td.textContent || '').trim().replace(/\s+/g, ' ').slice(0, 14), left: Math.round(r.left), right: Math.round(r.right), fullyVisible: r.left >= w.left - 1 && r.right <= w.right + 1 };
    });
    return tds;
  })[0];
  return {
    scrollRange: Math.round(wrap.scrollWidth - wrap.clientWidth),
    wrapWidth: Math.round(wrap.clientWidth),
    headers,
    atZero,
    atMax,
    nameVisibleAtMax: atMax[0] ? atMax[0].fullyVisible : null,
    actionsVisibleAtZero: atZero.length ? atZero[atZero.length - 1].fullyVisible : null,
  };
});
console.log('=== /tenants ===');
console.log('headers:', tenants.headers.join(' | '));
console.log('wrapWidth', tenants.wrapWidth, 'scrollRange', tenants.scrollRange);
console.log('at scroll 0   :', tenants.atZero.map((c) => `${c.col}:${c.fullyVisible ? 'vis' : 'HID'}`).join(' '));
console.log('at max scroll :', tenants.atMax.map((c) => `${c.col}:${c.fullyVisible ? 'vis' : 'HID'}`).join(' '));
console.log('name column fully visible at max scroll?', tenants.nameVisibleAtMax);
console.log('actions column visible at scroll 0?', tenants.actionsVisibleAtZero);

// --- claim 2: /expenses 类别 column width and per-glyph breaking -------------
await page.goto(BASE + '/expenses', { waitUntil: 'networkidle' });
const expenses = await page.evaluate(() => {
  const wrap = document.querySelector('.table-wrap');
  if (!wrap) return { error: 'no .table-wrap' };
  const table = wrap.querySelector('table');
  const ths = [...table.querySelectorAll('thead th')];
  const firstRow = table.querySelector('tbody tr');
  const cells = firstRow ? [...firstRow.querySelectorAll('td')] : [];
  const lines = (el) => {
    // Count rendered line boxes via client rects of the text.
    const r = document.createRange();
    r.selectNodeContents(el);
    return r.getClientRects().length;
  };
  return {
    tableMinWidth: getComputedStyle(table).minWidth,
    tableWidth: Math.round(table.getBoundingClientRect().width),
    wrapWidth: Math.round(wrap.clientWidth),
    scrollRange: Math.round(wrap.scrollWidth - wrap.clientWidth),
    headers: ths.map((th) => ({ text: (th.textContent || '').trim(), w: Math.round(th.getBoundingClientRect().width) })),
    cells: cells.map((td, i) => ({ col: i, text: (td.textContent || '').trim().replace(/\s+/g, ' ').slice(0, 16), w: Math.round(td.getBoundingClientRect().width), lineBoxes: lines(td) })),
  };
});
console.log('\n=== /expenses ===');
console.log('table minWidth', expenses.tableMinWidth, 'tableWidth', expenses.tableWidth, 'wrapWidth', expenses.wrapWidth, 'scrollRange', expenses.scrollRange);
console.log('headers:', expenses.headers.map((h) => `${h.text}=${h.w}px`).join(' '));
console.log('cells  :', expenses.cells.map((c) => `c${c.col}(${c.text})=${c.w}px/${c.lineBoxes}行`).join(' '));

await browser.close();
