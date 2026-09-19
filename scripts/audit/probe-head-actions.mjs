// AC 1 / AC 5-11 of 09-19-list-pages-alignment, read off the live DOM instead of
// the rendered template string: does each list page actually show a page-head
// primary action, and do its rows actually carry the row action the parent PRD
// asks for?
// Usage: AUDIT_BASE=... AUDIT_USER=... AUDIT_PASS=... node scripts/audit/probe-head-actions.mjs
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
  ['/cash-receipts?period=2026-08', '现金补录'],
  ['/expenses?period=2026-09', '费用支出'],
  ['/bank', '银行设置'],
];

const browser = await chromium.launch(launchOptions());
const ctx = await browser.newContext({ viewport: { width: 1440, height: 900 } });
const page = await ctx.newPage();
await page.goto(BASE + '/', { waitUntil: 'domcontentloaded' });
await page.fill('#username', USER);
await page.fill('#password', PASS);
await Promise.all([
  page.waitForNavigation({ waitUntil: 'domcontentloaded' }).catch(() => {}),
  page.click('button[type=submit]'),
]);

for (const [url, label] of PAGES) {
  await page.goto(BASE + url, { waitUntil: 'networkidle', timeout: 30000 });
  const info = await page.evaluate(() => {
    const heads = [...document.querySelectorAll('main header.topbar, main header.collection-toolbar, main > header')];
    const head = heads[0];
    const headActions = head
      ? [...head.querySelectorAll('a.btn, button.btn')].filter((b) => b.offsetParent !== null)
          .map((b) => `${b.tagName.toLowerCase()}:${b.textContent.trim()}`)
      : [];
    const body = document.body.textContent;
    const rowActions = [...document.querySelectorAll('table tbody tr a, table tbody tr button')]
      .filter((b) => b.offsetParent !== null).map((b) => b.textContent.trim());
    const unique = [...new Set(rowActions)];
    return {
      headActions,
      rowActionKinds: unique,
      demoNote: /演示数据/.test(body) ? body.match(/演示数据[^·<]*·\s*[A-Z]{3}/)?.[0] ?? '演示数据…' : null,
      hasViewDetails: /查看详情/.test(body),
    };
  });
  console.log(`${label.padEnd(6)} ${url}
   headActions = ${JSON.stringify(info.headActions)}
   rowActions  = ${JSON.stringify(info.rowActionKinds)} 查看详情=${info.hasViewDetails} demoNote=${JSON.stringify(info.demoNote)}`);
}

await browser.close();
