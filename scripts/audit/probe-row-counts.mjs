// How much real data each list page has in the audit fixture: a page that
// renders zero rows cannot demonstrate that its filterbar filters anything.
// Usage: AUDIT_BASE=... AUDIT_USER=... AUDIT_PASS=... node scripts/audit/probe-row-counts.mjs
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
    const desktop = document.querySelectorAll('table tbody tr').length;
    const mobile = document.querySelectorAll('article[data-row], .object-mobile-list article, .collection-mobile-list article, .tenancy-mobile-list article, .expense-mobile-list article, .cash-receipt-mobile-list article, .transaction-route-mobile-list article').length;
    const empty = [...document.querySelectorAll('.empty')].map((n) => n.textContent.trim()).filter(Boolean);
    return { desktop, mobile, empty };
  });
  console.log(`${label.padEnd(6)} ${url.padEnd(36)} desktopRows=${String(info.desktop).padStart(3)} mobileCards=${String(info.mobile).padStart(3)} empty=${JSON.stringify(info.empty)}`);
}

await browser.close();
