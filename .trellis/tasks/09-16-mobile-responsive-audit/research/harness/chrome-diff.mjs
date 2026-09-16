// Capture each page's chrome (sidebar + topbar) so the pre-change and
// post-change builds can be compared textually. The full page HTML legitimately
// differs — the drawer markup is always in the DOM, just display:none above
// 640px — so the comparison is scoped to the chrome, which must not move at all.
//
// Usage: AUDIT_OUT=/tmp/chrome-check/old node chrome-diff.mjs
import { chromium } from 'playwright';
import fs from 'node:fs';
import crypto from 'node:crypto';

const BASE = process.env.AUDIT_BASE || 'http://localhost:8081';
const OUT = process.env.AUDIT_OUT;
fs.mkdirSync(OUT, { recursive: true });

const browser = await chromium.launch({
  executablePath: process.env.CHROME_BIN || '/tmp/mobile-audit/chrome-linux64/chrome',
  args: ['--no-sandbox', '--disable-dev-shm-usage'],
});
// Desktop viewport: this is the width at which nothing may move.
const ctx = await browser.newContext({ viewport: { width: 1440, height: 900 } });
const page = await ctx.newPage();
await page.goto(BASE + '/', { waitUntil: 'domcontentloaded' });
await page.fill('#username', process.env.AUDIT_USER);
await page.fill('#password', process.env.AUDIT_PASS);
await Promise.all([page.waitForNavigation({ waitUntil: 'domcontentloaded' }).catch(() => {}), page.click('button[type=submit]')]);

// Discover the ids the pages need, from the pages themselves.
await page.goto(BASE + '/tenants', { waitUntil: 'networkidle' });
const tenantId = await page.evaluate(() => {
  const m = document.body.innerHTML.match(/\/tenants\/(\d+)/);
  return m ? m[1] : null;
});
await page.goto(BASE + '/billing', { waitUntil: 'networkidle' });
const txnId = await page.evaluate(() => {
  const m = document.body.innerHTML.match(/name="transaction_id" value="(\d+)"/);
  return m ? m[1] : null;
});
let receiptId = null;
for (const h of ['/billing', '/rent-dashboard', '/expenses']) {
  await page.goto(BASE + h, { waitUntil: 'networkidle' });
  receiptId = await page.evaluate(() => {
    const a = document.querySelector('a[href*="cash-receipts/void"]');
    return a ? new URL(a.href).searchParams.get('receipt_id') : null;
  });
  if (receiptId) break;
}

const PAGES = [
  ['tenants', '/tenants'],
  ['tenant-detail', tenantId ? `/tenants/${tenantId}` : null],
  ['rent-dashboard', '/rent-dashboard'],
  ['billing', '/billing'],
  ['expenses', '/expenses'],
  ['cash-receipt-new', '/cash-receipts/new'],
  ['cash-receipt-void', receiptId ? `/cash-receipts/void?receipt_id=${receiptId}` : null],
  ['billing-revoke', txnId ? `/billing/revoke?transaction_id=${txnId}` : null],
];

const norm = (s) => s.replace(/\s+/g, ' ').replace(/>\s+</g, '><').trim();

for (const [name, url] of PAGES) {
  if (!url) { console.log(`${name.padEnd(18)} SKIPPED (no id)`); continue; }
  const resp = await page.goto(BASE + url, { waitUntil: 'networkidle', timeout: 30000 });
  const found = await page.evaluate(() => {
    const side = document.querySelector('aside.sidebar');
    const top = document.querySelector('.topbar');
    const nav = document.querySelector('aside.sidebar nav');
    return {
      sidebar: side ? side.outerHTML : null,
      topbar: top ? top.outerHTML : null,
      navLabel: nav ? nav.getAttribute('aria-label') : null,
      navCounts: document.querySelectorAll('aside.sidebar .nav-count').length,
      navCountText: [...document.querySelectorAll('aside.sidebar .nav-count')].map((e) => e.textContent.trim()).join(','),
      active: [...document.querySelectorAll('aside.sidebar nav a.active')].map((e) => e.getAttribute('href')).join(','),
      scatter: [...document.querySelectorAll('aside.sidebar nav a')].map((e) => e.getAttribute('href')).join(','),
    };
  });
  fs.writeFileSync(`${OUT}/${name}.sidebar.html`, found.sidebar || '');
  fs.writeFileSync(`${OUT}/${name}.topbar.html`, found.topbar || '');
  const h = crypto.createHash('md5').update(norm(found.sidebar || '')).digest('hex').slice(0, 12);
  console.log(`${name.padEnd(18)} ${resp.status()} md5=${h} navLabel=${JSON.stringify(found.navLabel)} counts=${found.navCounts}(${found.navCountText}) active=${found.active}`);
}

await browser.close();
console.log('wrote ' + OUT);
