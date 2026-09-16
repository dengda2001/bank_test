// Two questions the P1 work left open, answered by measurement:
//
//  1. The named cramped columns: how many *text lines* does the cell really take
//     now? Counting line boxes via Range rects rather than height/line-height,
//     which miscounts the moment a cell contains a <br> or a sub-line.
//  2. /billing at 375px: with the action column pinned (88px) and the amount
//     column frozen at its left edge, is the payer name simultaneously readable
//     with them, or does it scroll out of the window? If it scrolls out, the
//     overlay geometry needs a decision.
import { chromium } from 'playwright';

const BASE = process.env.AUDIT_BASE || 'http://localhost:8081';
const W = Number(process.env.PROBE_W || 375);
const browser = await chromium.launch({
  executablePath: '/tmp/mobile-audit/chrome-linux64/chrome',
  args: ['--no-sandbox', '--disable-dev-shm-usage'],
});
const ctx = await browser.newContext({ viewport: { width: W, height: 800 } });
const page = await ctx.newPage();
await page.goto(BASE + '/', { waitUntil: 'domcontentloaded' });
await page.fill('#username', process.env.AUDIT_USER);
await page.fill('#password', process.env.AUDIT_PASS);
await Promise.all([page.waitForNavigation({ waitUntil: 'domcontentloaded' }).catch(() => {}), page.click('button[type=submit]')]);

// Count real rendered line boxes of an element's text.
const LINE_COUNTER = `(el) => {
  const texts = [];
  const walk = (n) => { for (const c of n.childNodes) { if (c.nodeType === 3 && c.textContent.trim()) texts.push(c); else if (c.nodeType === 1 && c.tagName !== 'BR') walk(c); } };
  walk(el);
  const tops = new Set();
  for (const t of texts) {
    const r = document.createRange(); r.selectNodeContents(t);
    for (const b of r.getClientRects()) if (b.width > 0 && b.height > 0) tops.add(Math.round(b.top));
  }
  return tops.size;
}`;

console.log(`===== viewport ${W} =====`);

for (const url of ['/tenants', '/expenses']) {
  await page.goto(BASE + url, { waitUntil: 'networkidle', timeout: 30000 });
  const out = await page.evaluate((counterSrc) => {
    const countLines = eval(counterSrc);
    // Scope to the page's own list table: /tenants nests a wrapped history table
    // inside each expanded row, and a bare descendant selector walks into it.
    const table = document.querySelector('main.content > section .table-wrap > table');
    const row = table.querySelector(':scope > tbody > tr');
    const cells = [...row.children];
    return cells.map((td, i) => ({ i: i + 1, w: Math.round(td.getBoundingClientRect().width), lines: countLines(td), text: (td.textContent || '').trim().replace(/\s+/g, ' ').slice(0, 60) }));
  }, LINE_COUNTER);
  console.log(`\n-- ${url} 首行各列`);
  for (const c of out) console.log(`   第${c.i}列 ${String(c.w).padStart(4)}px ${String(c.lines).padStart(2)} 行  "${c.text}"`);
}

// /billing overlay geometry
await page.goto(BASE + '/billing', { waitUntil: 'networkidle', timeout: 30000 });
const bill = await page.evaluate(() => {
  const wrap = document.querySelector('.table-wrap');
  const table = wrap.querySelector('table');
  const rows = [...table.querySelectorAll('tbody > tr')].slice(0, 3);
  const wrapBox = wrap.getBoundingClientRect();
  const vis = { left: wrapBox.left, right: wrapBox.right, width: Math.round(wrapBox.width) };
  const out = [];
  for (const tr of rows) {
    const cells = [...tr.children].map((td) => {
      const b = td.getBoundingClientRect();
      const cs = getComputedStyle(td);
      return {
        cls: (td.className || '').split(/\s+/).filter((c) => c.startsWith('txn-col')).join('.'),
        left: Math.round(b.left), right: Math.round(b.right), w: Math.round(b.width),
        pos: cs.position,
        text: (td.textContent || '').trim().replace(/\s+/g, ' ').slice(0, 24),
      };
    });
    out.push(cells);
  }
  return { vis, scrollW: Math.round(table.getBoundingClientRect().width), scrollLeftMax: wrap.scrollWidth - wrap.clientWidth, rows: out };
});
console.log(`\n-- /billing 横向滚动容器 可视 ${bill.vis.width}px, 表格 ${bill.scrollW}px, 可滚 ${bill.scrollLeftMax}px`);
for (const [ri, cells] of bill.rows.entries()) {
  console.log(`   行 ${ri + 1}:`);
  for (const c of cells) {
    const fullyVisible = c.left >= bill.vis.left - 1 && c.right <= bill.vis.right + 1;
    console.log(`     ${(c.cls || '(无 txn-col 类)').padEnd(26)} ${String(c.w).padStart(4)}px 位置 ${String(c.left).padStart(5)}..${String(c.right).padStart(5)} pos=${c.pos.padEnd(8)} ${fullyVisible ? '完整可见' : '被裁切'}  "${c.text}"`);
  }
}
await browser.close();
