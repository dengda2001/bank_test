// Desktop regression sweep over every live page, at 1440x900.
//
// desktop.mjs covers the four list pages; this one adds the three pages that only
// exist for one row (tenant detail, cash receipt void, billing revoke) plus the
// cash receipt entry form, because those are exactly the pages whose chrome the
// shared-shell extraction touched and whose panels the void page's padding fix
// moved. Ids are discovered from the app rather than hardcoded.
//
// Usage: AUDIT_OUT=/tmp/mobile-audit/final-desktop node desktop-full.mjs
import { chromium } from 'playwright';
import { launchOptions } from './launch.mjs';
import fs from 'node:fs';
import crypto from 'node:crypto';

const BASE = process.env.AUDIT_BASE || 'http://localhost:8081';
const OUT = process.env.AUDIT_OUT || '/tmp/mobile-audit/desktop-full';
fs.mkdirSync(OUT, { recursive: true });

const browser = await chromium.launch(launchOptions());
const ctx = await browser.newContext({ viewport: { width: 1440, height: 900 } });
const page = await ctx.newPage();
await page.goto(BASE + '/', { waitUntil: 'domcontentloaded' });
await page.fill('#username', process.env.AUDIT_USER);
await page.fill('#password', process.env.AUDIT_PASS);
await Promise.all([page.waitForNavigation({ waitUntil: 'domcontentloaded' }).catch(() => {}), page.click('button[type=submit]')]);

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
  const geo = await page.evaluate(() => {
    const side = document.querySelector('aside.sidebar');
    const r = (el) => { const b = el?.getBoundingClientRect(); return b ? [Math.round(b.width), Math.round(b.height)] : null; };
    return {
      scrollW: document.documentElement.scrollWidth,
      clientW: document.documentElement.clientWidth,
      bodyH: document.documentElement.scrollHeight,
      sidebar: r(document.querySelector('aside.sidebar')),
      // The main content panel's own box: the void page's padding fix shows up here.
      panel: r(document.querySelector('main.content > section.panel')),
      content: r(document.querySelector('main.content')),
      sidebarHTML: side ? side.outerHTML : null,
    };
  });
  await page.screenshot({ path: `${OUT}/${name}.png`, fullPage: true });
  fs.writeFileSync(`${OUT}/${name}.sidebar.html`, geo.sidebarHTML || '');
  const h = crypto.createHash('md5').update(norm(geo.sidebarHTML || '')).digest('hex').slice(0, 12);
  console.log(
    `${name.padEnd(18)} ${resp.status()} scrollW=${String(geo.scrollW).padStart(5)} clientW=${geo.clientW} bodyH=${String(geo.bodyH).padStart(5)} ` +
    `sidebar=${JSON.stringify(geo.sidebar)} content=${JSON.stringify(geo.content)} panel=${JSON.stringify(geo.panel)} md5=${h}`
  );
}

await browser.close();
console.log('wrote ' + OUT);
