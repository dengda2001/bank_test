// Two pages the first pass missed because they are not linked as plain GETs:
//   GET  /billing/revoke?transaction_id=N   (billing JS rewrites that form to GET)
//   POST /billing/payer/preview             (POST-only, but the handler only reads)
// Both bypass workspacePageCSS and ship their own :root token set, so they need
// their own viewport check rather than an assumption inherited from the other 8.
//
// Two traps this script exists to avoid:
//  1. /billing/revoke redirects to /billing?error=... when the transaction has no
//     effective allocations, so "did we land where we asked" must be asserted on
//     the final URL. Otherwise the measurements silently describe the billing page.
//  2. A form submitted from page.evaluate() navigates asynchronously; reading
//     page.url() or counting rows right after silently reads the *previous* page.
import { chromium } from 'playwright';
import { launchOptions } from './launch.mjs';
import fs from 'node:fs';

const BASE = process.env.AUDIT_BASE || 'http://localhost:8081';
const USER = process.env.AUDIT_USER;
const PASS = process.env.AUDIT_PASS;
const OUT = process.env.AUDIT_OUT || '/tmp/mobile-audit/out';

function probe() {
  const vw = document.documentElement.clientWidth;
  const all = [];
  for (const el of document.querySelectorAll('body *')) {
    const r = el.getBoundingClientRect();
    if (r.width === 0 && r.height === 0) continue;
    const st = getComputedStyle(el);
    if (st.position === 'fixed' || st.display === 'none' || st.visibility === 'hidden') continue;
    all.push({ el, r, st, escapesRight: r.right - vw > 1, escapesLeft: r.left < -1 });
  }
  const overflows = new Set(all.filter((e) => e.escapesRight || e.escapesLeft).map((e) => e.el));
  const describe = (el) => {
    let sel = el.tagName.toLowerCase();
    if (el.id) sel += '#' + el.id;
    const cls = typeof el.className === 'string' ? el.className.trim().split(/\s+/).filter(Boolean) : [];
    if (cls.length) sel += '.' + cls.slice(0, 2).join('.');
    return sel;
  };
  const offenders = [];
  for (const e of all) {
    if (!e.escapesRight && !e.escapesLeft) continue;
    if (e.el.parentElement && overflows.has(e.el.parentElement)) continue;
    offenders.push({
      selector: describe(e.el),
      width: Math.round(e.r.width),
      escapesRightBy: Math.round(e.r.right - vw),
      overflowX: e.st.overflowX,
      text: (el_text(e.el) || '').slice(0, 40),
    });
  }
  function el_text(el) {
    return (el.textContent || '').trim().replace(/\s+/g, ' ');
  }
  offenders.sort((a, b) => b.escapesRightBy - a.escapesRightBy);

  // Which element actually scrolls, and does the page heading live inside it?
  // Scrolling the heading away is the difference between a wrapped table and a
  // whole card turned into a scroller.
  const scrollerDetail = [...document.querySelectorAll('body *')]
    .filter((el) => {
      const st = getComputedStyle(el);
      return (st.overflowX === 'auto' || st.overflowX === 'scroll') && el.scrollWidth > el.clientWidth + 1;
    })
    .map((el) => ({
      selector: describe(el),
      scrollRange: Math.round(el.scrollWidth - el.clientWidth),
      containsHeading: !!el.querySelector('h1,h2'),
      padding: getComputedStyle(el).padding,
    }));

  const small = [];
  for (const el of document.querySelectorAll('a, button, input, select, textarea, [role=button]')) {
    const r = el.getBoundingClientRect();
    if (r.width === 0 || r.height === 0) continue;
    const label = (el.textContent || el.getAttribute('aria-label') || el.name || el.id || '').trim().replace(/\s+/g, ' ').slice(0, 26);
    if (r.width < 44 || r.height < 44) small.push({ tag: el.tagName.toLowerCase(), label, w: Math.round(r.width), h: Math.round(r.height) });
  }

  return {
    heading: (document.querySelector('h1') || {}).textContent || null,
    viewportWidth: vw,
    docScrollWidth: document.documentElement.scrollWidth,
    horizontalOverflow: document.documentElement.scrollWidth - vw,
    pageHeight: document.documentElement.scrollHeight,
    rowCount: document.querySelectorAll('tbody tr').length,
    offenders: offenders.slice(0, 20),
    scrollerDetail,
    smallTargets: small,
  };
}

const browser = await chromium.launch(launchOptions());
const ctx = await browser.newContext({ deviceScaleFactor: 2 });
const page = await ctx.newPage();

await page.goto(BASE + '/', { waitUntil: 'domcontentloaded' });
await page.fill('#username', USER);
await page.fill('#password', PASS);
await Promise.all([page.waitForNavigation({ waitUntil: 'domcontentloaded' }).catch(() => {}), page.click('button[type=submit]')]);

// Every transaction currently offering 撤销匹配. Most of them redirect (a matched
// transaction whose allocations were all revoked has nothing left to preview), so
// walk the list until one actually renders the preview.
await page.goto(BASE + '/billing', { waitUntil: 'networkidle' });
const candidates = await page.evaluate(() =>
  [...document.body.innerHTML.matchAll(/action="\/billing\/revoke"><input type="hidden" name="transaction_id" value="(\d+)"/g)].map((m) => m[1]),
);
console.log('revoke candidates:', candidates.join(',') || '(none)');

fs.mkdirSync(OUT, { recursive: true });
const report = { generatedAt: new Date().toISOString(), pages: [] };

for (const vp of [{ name: '375x667', width: 375, height: 667 }, { name: '390x844', width: 390, height: 844 }]) {
  await page.setViewportSize({ width: vp.width, height: vp.height });

  // --- GET /billing/revoke ---------------------------------------------------
  let landed = null;
  for (const id of candidates) {
    const url = `/billing/revoke?transaction_id=${id}`;
    const resp = await page.goto(BASE + url, { waitUntil: 'networkidle', timeout: 30000 });
    const finalUrl = page.url();
    if (new URL(finalUrl).pathname === '/billing/revoke') {
      landed = { id, url, status: resp && resp.status(), finalUrl };
      break;
    }
    console.log(`  tx ${id} -> redirected to ${new URL(finalUrl).pathname}${new URL(finalUrl).search}`);
  }
  if (landed) {
    const entry = { page: 'billing-revoke-preview', viewport: vp.name, ...landed };
    entry.result = await page.evaluate(probe);
    await page.screenshot({ path: `${OUT}/billing-revoke-preview__${vp.name}.png`, fullPage: true });
    report.pages.push(entry);
    console.log(`revoke-preview   ${vp.name} tx=${landed.id} overflow=${entry.result.horizontalOverflow} h=${entry.result.pageHeight} scrollers=${JSON.stringify(entry.result.scrollerDetail)}`);
    console.log('   small:', JSON.stringify(entry.result.smallTargets));
  } else {
    console.log(`revoke-preview   ${vp.name} UNREACHABLE - every candidate redirected`);
    report.pages.push({ page: 'billing-revoke-preview', viewport: vp.name, unreachable: true, candidates });
  }

  // --- POST /billing/payer/preview -------------------------------------------
  await page.goto(BASE + '/billing', { waitUntil: 'networkidle' });
  await Promise.all([
    page.waitForNavigation({ waitUntil: 'networkidle', timeout: 30000 }).catch(() => {}),
    page.evaluate(() => {
      const f = document.createElement('form');
      f.method = 'post';
      f.action = '/billing/payer/preview';
      document.body.appendChild(f);
      f.submit();
    }),
  ]);
  const entry = { page: 'billing-payer-preview', viewport: vp.name, url: 'POST /billing/payer/preview', finalUrl: page.url() };
  entry.result = await page.evaluate(probe);
  await page.screenshot({ path: `${OUT}/billing-payer-preview__${vp.name}.png`, fullPage: true });
  report.pages.push(entry);
  console.log(`payer-preview    ${vp.name} heading=${entry.result.heading} rows=${entry.result.rowCount} overflow=${entry.result.horizontalOverflow} h=${entry.result.pageHeight}`);
  console.log('   scrollers:', JSON.stringify(entry.result.scrollerDetail));
  console.log('   small    :', JSON.stringify(entry.result.smallTargets));
}

fs.writeFileSync(`${OUT}/report-extra.json`, JSON.stringify(report, null, 2));
await browser.close();
console.log('\nwrote ' + OUT + '/report-extra.json');
