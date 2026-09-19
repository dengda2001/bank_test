// The design.md acceptance list has two items nothing else measures: content must
// start at y<=120 on every page at 375px, and the P2 page heights. Also re-settles
// P0-2's exact claim — that no scroll position showed the name and the action cell
// together — since that is what the frozen-column decision was made for.
import { chromium } from 'playwright';
import { launchOptions } from './launch.mjs';
const BASE = process.env.AUDIT_BASE || 'http://localhost:8081';
const W = 375;
const b = await chromium.launch(launchOptions());
const ctx = await b.newContext({ viewport: { width: W, height: 667 } });
const p = await ctx.newPage();
await p.goto(BASE + '/', { waitUntil: 'domcontentloaded' });
await p.fill('#username', process.env.AUDIT_USER); await p.fill('#password', process.env.AUDIT_PASS);
await Promise.all([p.waitForNavigation({ waitUntil: 'domcontentloaded' }).catch(()=>{}), p.click('button[type=submit]')]);
const id = await p.evaluate(async () => (await (await fetch('/tenants',{credentials:'same-origin'})).text()).match(/\/tenants\/(\d+)/)?.[1]);
const rid = await p.evaluate(async () => {
  const h = await (await fetch('/cash-receipts/new',{credentials:'same-origin'})).text();
  return (h.match(/cash-receipts\/void\?receipt_id=(\d+)/) || [])[1];
});
const pages = [['/tenants','/tenants'],['tenant-detail',`/tenants/${id}`],['/rent-dashboard','/rent-dashboard'],['/billing','/billing'],['/expenses','/expenses'],['/cash-receipts/new','/cash-receipts/new'],['cash-receipt-void', rid?`/cash-receipts/void?receipt_id=${rid}`:null]];
console.log(`\n===== ${W}px 首屏内容起点 / 整页高度 =====`);
for (const [name, url] of pages) {
  if (!url) { console.log(`  ${name.padEnd(18)} SKIP`); continue; }
  await p.goto(BASE + url, { waitUntil: 'networkidle', timeout: 30000 });
  const r = await p.evaluate(() => {
    const cands = [...document.querySelectorAll('main.content > *')].filter(e => e.getBoundingClientRect().height > 0);
    const first = cands[0];
    const t = document.querySelector('main.content > section .table-wrap > table');
    const row = t && t.querySelector(':scope > tbody > tr');
    let cols = null;
    if (row) {
      const wrap = t.closest('.table-wrap');
      const wb = wrap.getBoundingClientRect();
      cols = [...row.children].map((td) => { const x = td.getBoundingClientRect(); return [Math.round(x.left), Math.round(x.right)]; });
      return { y: first ? Math.round(first.getBoundingClientRect().top) : null, h: document.documentElement.scrollHeight, vis: [Math.round(wb.left), Math.round(wb.right)], cols, scrollMax: wrap.scrollWidth - wrap.clientWidth };
    }
    return { y: first ? Math.round(first.getBoundingClientRect().top) : null, h: document.documentElement.scrollHeight };
  });
  let extra = '';
  if (r.cols) {
    const overlaps = (a, b) => Math.min(a[1], b[1]) - Math.max(a[0], b[0]) > 0;
    // name = first cell, action = last cell
    extra = `  姓名列与操作列同屏: scroll=0 ${overlaps(r.cols[0], r.vis) && overlaps(r.cols[r.cols.length-1], r.vis)}`;
  }
  console.log(`  ${name.padEnd(18)} 内容起点 y=${String(r.y).padStart(4)}  整页 ${String(r.h).padStart(5)}px${extra}`);
}
await b.close();
