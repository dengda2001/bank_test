// Complements probe-list-filters.mjs: a search box that "changes nothing" is
// ambiguous, because the term may simply match every row. This drives each
// filterbar search with a term that cannot match anything and with a term that
// must match a subset, so the control is proven both ways.
// Usage: AUDIT_BASE=... AUDIT_USER=... AUDIT_PASS=... node scripts/audit/probe-search-negatives.mjs
import { chromium } from 'playwright';
import { launchOptions } from './launch.mjs';

const BASE = (process.env.AUDIT_BASE || 'http://127.0.0.1:18094').replace(/\/+$/, '');
const USER = process.env.AUDIT_USER;
const PASS = process.env.AUDIT_PASS;

const CASES = [
  ['/properties?period=2026-09&status=all', 'form.object-list-filters input[name="search"]', 'Aurora'],
  ['/rooms?period=2026-09&status=all', 'form.object-list-filters input[name="search"]', 'A-01'],
  ['/tenancies?period=2026-09&status=all', 'form.tenancy-filters input[name="search"]', 'A-01'],
  ['/bills?period=2026-09', 'form.bills-filters input[name="search"]', 'LI'],
  ['/cash-receipts?period=2026-08', 'form.cash-receipt-filters input[name="search"]', 'cash'],
  ['/expenses?period=2026-09', 'form.expense-filters input[name="search"]', 'repair'],
  ['/transactions?scope=pending', 'form.transaction-route-quickfilter input[name="payer"]', 'CHEN'],
];

const COUNT = `document.querySelectorAll('table tbody tr').length`;

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

async function rowsWith(url, selector, term) {
  await page.goto(BASE + url, { waitUntil: 'networkidle', timeout: 30000 });
  const base = await page.evaluate(COUNT);
  await page.fill(selector, term);
  await page.press(selector, 'Enter');
  await page.waitForLoadState('networkidle').catch(() => {});
  await page.waitForTimeout(300);
  const after = await page.evaluate(COUNT);
  const empty = await page.evaluate(() => [...document.querySelectorAll('.empty, .workspace-empty')].map((n) => n.textContent.trim())[0] ?? null);
  return { base, after, empty, url: page.url().replace(BASE, '') };
}

for (const [url, selector, realTerm] of CASES) {
  const none = await rowsWith(url, selector, 'zzzz-no-match-zzzz');
  const some = await rowsWith(url, selector, realTerm);
  console.log(`${url}  [${selector}]
   impossible term: ${none.base} -> ${none.after}  empty=${JSON.stringify(none.empty)}
   real term ${JSON.stringify(realTerm)}: ${some.base} -> ${some.after}  -> ${some.url}`);
}

await browser.close();
