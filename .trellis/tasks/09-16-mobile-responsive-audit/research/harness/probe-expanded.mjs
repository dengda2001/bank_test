// The other half of the /billing narrow-screen question: what happens once the
// 处理 summary is tapped? The action cell then holds a real form, which is far
// wider than the collapsed cell, and a frozen cell that grows leftward can eat the
// whole window.
import { chromium } from 'playwright';

const BASE = process.env.AUDIT_BASE || 'http://localhost:8081';
const W = Number(process.env.PROBE_W || 375);
const browser = await chromium.launch({
  executablePath: '/tmp/mobile-audit/chrome-linux64/chrome',
  args: ['--no-sandbox', '--disable-dev-shm-usage'],
});
const ctx = await browser.newContext({ viewport: { width: W, height: 900 } });
const page = await ctx.newPage();
await page.goto(BASE + '/', { waitUntil: 'domcontentloaded' });
await page.fill('#username', process.env.AUDIT_USER);
await page.fill('#password', process.env.AUDIT_PASS);
await Promise.all([page.waitForNavigation({ waitUntil: 'domcontentloaded' }).catch(() => {}), page.click('button[type=submit]')]);
await page.goto(BASE + '/billing', { waitUntil: 'networkidle', timeout: 30000 });

const snap = () => page.evaluate(() => {
  const wrap = document.querySelector('.table-wrap');
  const tr = wrap.querySelector('tbody > tr');
  const cs = [...tr.children].map((td) => {
    const b = td.getBoundingClientRect();
    const cls = (td.className || '').split(/\s+/).filter((c) => c.startsWith('txn-col')).join('');
    return { cls, left: Math.round(b.left), right: Math.round(b.right), w: Math.round(b.width) };
  });
  const wb = wrap.getBoundingClientRect();
  return { visLeft: Math.round(wb.left), visRight: Math.round(wb.right), table: Math.round(wrap.querySelector('table').getBoundingClientRect().width), cells: cs };
});

const collapsed = await snap();
console.log(`\n===== ${W}px 折叠 =====`);
console.log(`可视 ${collapsed.visLeft}..${collapsed.visRight}, 表格 ${collapsed.table}px`);
for (const c of collapsed.cells) console.log(`  ${c.cls.padEnd(14)} ${String(c.w).padStart(4)}px  ${c.left}..${c.right}`);

await page.click('.txn-col-action > .txn-action > summary');
await page.waitForTimeout(300);
const expanded = await snap();
console.log(`\n===== ${W}px 展开后 =====`);
console.log(`可视 ${expanded.visLeft}..${expanded.visRight}, 表格 ${expanded.table}px`);
for (const c of expanded.cells) console.log(`  ${c.cls.padEnd(14)} ${String(c.w).padStart(4)}px  ${c.left}..${c.right}`);

const payer = expanded.cells.find((c) => c.cls === 'txn-col-payer');
const action = expanded.cells.find((c) => c.cls === 'txn-col-action');
console.log(`\n付款人列可见宽度: ${payer ? Math.max(0, Math.min(payer.right, action.left) - Math.max(payer.left, expanded.visLeft)) : 'n/a'}px`);
await browser.close();
