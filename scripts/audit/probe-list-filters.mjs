// Independent verification of 09-19-list-pages-alignment AC 2:
// "every filterbar control has real data behind it and actually filters the list".
//
// Each control is driven the way a user would: choose a select / type in a search
// / set a month, then submit the enclosing form (auto-submit onchange, or the
// form's submit button when the control has no onchange). The comparison is the
// full text of the visible desktop rows, so a month filter that only moves money
// columns still counts as a change.
//
// Usage: AUDIT_BASE=... AUDIT_USER=... AUDIT_PASS=... node scripts/audit/probe-list-filters.mjs
import { chromium } from 'playwright';
import { launchOptions } from './launch.mjs';

const BASE = (process.env.AUDIT_BASE || 'http://127.0.0.1:18094').replace(/\/+$/, '');
const USER = process.env.AUDIT_USER;
const PASS = process.env.AUDIT_PASS;

const browser = await chromium.launch(launchOptions());
const ctx = await browser.newContext({ viewport: { width: 1440, height: 900 } });
const page = await ctx.newPage();

await page.goto(BASE + '/', { waitUntil: 'domcontentloaded' });
await page.fill('#username', USER);
await page.fill('#password', PASS);
await Promise.all([page.waitForNavigation({ waitUntil: 'domcontentloaded' }).catch(() => {}), page.click('button[type=submit]')]);

const ROWS_JS = `(() => {
  const rows = [];
  const seen = new Set();
  const scopes = ['.table-wrap tbody tr', '.object-table-wrap tbody tr', '.tenant-table tbody tr',
                  '.collection-mobile-list article', '.expense-mobile-list article', '.tenancy-mobile-list article',
                  '.object-mobile-list article', '.cash-receipt-mobile-list article', '.transaction-route-mobile-list article'];
  for (const sel of scopes) for (const node of document.querySelectorAll(sel)) {
    if (seen.has(node)) continue; seen.add(node);
    const t = node.textContent.replace(/\\s+/g, ' ').trim();
    if (t) rows.push(t);
  }
  return rows;
})()`;

async function snap() { return page.evaluate(ROWS_JS); }
async function rowsAt(url) {
  await page.goto(BASE + url, { waitUntil: 'networkidle', timeout: 30000 });
  return snap();
}

const results = [];
function record(scope, control, changed, before, after, note = '') {
  results.push({ scope, control, changed, beforeCount: before.length, afterCount: after.length, before, after, note });
  console.log(`${changed ? 'CHANGED' : 'SAME   '} ${scope} :: ${control}  ${before.length}->${after.length} ${note}`);
}

// Submit the form the control lives in, preferring the onchange auto-submit and
// falling back to the form's own submit button.
async function submitFormOf(selector) {
  const before = page.url();
  await page.waitForTimeout(350);
  if (page.url() !== before) { await page.waitForLoadState('networkidle').catch(() => {}); return; }
  const clicked = await page.evaluate((sel) => {
    const el = document.querySelector(sel);
    const form = el?.closest('form');
    const btn = form?.querySelector('button[type="submit"]');
    if (!btn) return false;
    btn.click();
    return true;
  }, selector);
  if (clicked) await page.waitForNavigation({ timeout: 8000 }).catch(() => {});
  await page.waitForLoadState('networkidle').catch(() => {});
}

async function probeSelect(url, selector, scope) {
  const base = await rowsAt(url);
  const options = await page.$$eval(selector + ' option', (os) => os.map((o) => ({ value: o.value, label: o.textContent.trim() })));
  for (const opt of options) {
    await page.goto(BASE + url, { waitUntil: 'domcontentloaded' });
    const handle = await page.$(selector);
    if (!handle) { console.log(`MISSING ${scope} ${selector}`); continue; }
    await handle.selectOption(opt.value).catch((e) => console.log(`SELECT-FAIL ${scope} ${selector}=${opt.value}: ${e.message.split('\n')[0]}`));
    await submitFormOf(selector);
    const after = await snap();
    record(scope, `select ${selector} = ${opt.value} (${opt.label})`, JSON.stringify(base) !== JSON.stringify(after), base, after, page.url().includes('error=') ? 'URL has error= !' : '');
  }
}

// The shell's calendar widget replaces every input[type=month] with a text box +
// a hidden input that carries the real value, so drive the hidden input.
async function probeMonth(url, formSelector, value, scope) {
  const base = await rowsAt(url);
  await page.goto(BASE + url, { waitUntil: 'domcontentloaded' });
  const ok = await page.evaluate(([sel, v]) => {
    const form = document.querySelector(sel);
    const hidden = form?.querySelector('input[type=hidden][name="period"]');
    if (!hidden) return false;
    hidden.value = v;
    form.querySelector('button[type="submit"]')?.click();
    return true;
  }, [formSelector, value]);
  if (!ok) { console.log(`MISSING ${scope} month in ${formSelector}`); return; }
  await page.waitForNavigation({ timeout: 8000 }).catch(() => {});
  await page.waitForLoadState('networkidle').catch(() => {});
  const after = await snap();
  record(scope, `month ${formSelector} = ${value}`, JSON.stringify(base) !== JSON.stringify(after), base, after, page.url());
}

async function probeSearch(url, selector, term, scope) {
  const base = await rowsAt(url);
  await page.goto(BASE + url, { waitUntil: 'domcontentloaded' });
  await page.fill(selector, term);
  await page.press(selector, 'Enter');
  await page.waitForLoadState('networkidle').catch(() => {});
  const after = await snap();
  record(scope, `search ${selector} = ${JSON.stringify(term)}`, JSON.stringify(base) !== JSON.stringify(after), base, after, page.url().includes('error=') ? 'URL has error= !' : '');
}

console.log('=== selects ===');
await probeSelect('/properties?period=2026-09&status=active', 'form.object-list-filters select[name="collection"]', '/properties');
await probeSelect('/properties?period=2026-09&status=active', 'form.object-list-filters select[name="period"]', '/properties');
await probeSelect('/rooms?period=2026-09&status=all', 'form.object-list-filters select[name="property_id"]', '/rooms');
await probeSelect('/rooms?period=2026-09&status=all', 'form.object-list-filters select[name="collection"]', '/rooms');
await probeSelect('/tenancies?period=2026-09&status=all', 'form.tenancy-filters select[name="status"]', '/tenancies');
await probeSelect('/bills?period=2026-09', 'form.bills-filters select[name="status"]', '/bills');
await probeSelect('/cash-receipts?period=2026-08', 'form.cash-receipt-filters select[name="status"]', '/cash-receipts');
await probeSelect('/expenses?period=2026-09', 'form.expense-filters select[name="status"]', '/expenses');
await probeSelect('/transactions?scope=pending&match_status=pending', 'form.transaction-route-quickfilter select[name="match_status"]', '/transactions-quickfilter');

console.log('\n=== months ===');
await probeMonth('/tenancies?period=2026-09&status=all', 'form.tenancy-filters', '2026-08', '/tenancies');
await probeMonth('/expenses?period=2026-09', 'form.expense-filters', '2026-08', '/expenses');
await probeMonth('/cash-receipts?period=2026-09', 'form.cash-receipt-filters', '2026-08', '/cash-receipts');
await probeMonth('/transactions?scope=pending&match_status=pending', 'form.transaction-route-quickfilter', '2026-08', '/transactions-quickfilter');
await probeMonth('/dunning?period=2026-09', 'form.collection-period', '2026-08', '/dunning');
await probeMonth('/bills?period=2026-09', 'form.bills-filters', '2026-08', '/bills');
await probeMonth('/properties?period=2026-09&status=active', 'form.object-list-filters', '2026-08', '/properties');

console.log('\n=== search ===');
await probeSearch('/properties?period=2026-09&status=active', 'form.object-list-filters input[name="search"]', 'Aurora', '/properties');
await probeSearch('/rooms?period=2026-09&status=all', 'form.object-list-filters input[name="search"]', 'A-01', '/rooms');
await probeSearch('/tenancies?period=2026-09&status=all', 'form.tenancy-filters input[name="search"]', 'A-01', '/tenancies');
await probeSearch('/bills?period=2026-09', 'form.bills-filters input[name="search"]', 'LI', '/bills');
await probeSearch('/cash-receipts?period=2026-08', 'form.cash-receipt-filters input[name="search"]', 'cash', '/cash-receipts');
await probeSearch('/expenses?period=2026-09', 'form.expense-filters input[name="search"]', 'repair', '/expenses');
await probeSearch('/transactions?scope=pending&match_status=pending', 'form.transaction-route-quickfilter input[name="payer"]', 'CHEN', '/transactions-quickfilter');

await browser.close();
console.log('\n=== summary ===');
for (const r of results) console.log(`${r.changed ? 'CHG ' : 'same'} ${r.scope} :: ${r.control}`);
