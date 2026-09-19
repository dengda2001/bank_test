// Mobile-readiness audit: drives the real app in a real browser and reports
// which elements actually stick out of the viewport at phone/tablet widths.
//
// Usage:
//   AUDIT_USER=... AUDIT_PASS=... node audit.mjs
import { chromium } from 'playwright';
import { launchOptions } from './launch.mjs';
import fs from 'node:fs';
import path from 'node:path';

const BASE = process.env.AUDIT_BASE || 'http://localhost:8081';
const USER = process.env.AUDIT_USER;
const PASS = process.env.AUDIT_PASS;
const OUT = process.env.AUDIT_OUT || '/tmp/mobile-audit/out';

const VIEWPORTS = [
  { name: '375x667', width: 375, height: 667 },   // iPhone SE — the narrow end
  { name: '390x844', width: 390, height: 844 },   // iPhone 14/15
  { name: '768x1024', width: 768, height: 1024 }, // iPad portrait
];

// Runs in the page. Returns scroll metrics plus the "root" offenders only: an
// element that escapes the viewport while its parent stays inside. Without that
// filter every nested td inside a wide table is reported as its own bug.
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
    // Keep only the outermost element of each overflowing subtree.
    if (e.el.parentElement && overflows.has(e.el.parentElement)) continue;
    offenders.push({
      selector: describe(e.el),
      width: Math.round(e.r.width),
      right: Math.round(e.r.right),
      escapesRightBy: Math.round(e.r.right - vw),
      minWidth: e.st.minWidth,
      overflowX: e.st.overflowX,
      text: (e.el.textContent || '').trim().replace(/\s+/g, ' ').slice(0, 48),
    });
  }
  offenders.sort((a, b) => b.escapesRightBy - a.escapesRightBy);

  // Anything inside a horizontal scroller is fine by design; note it so the
  // report can tell "scrolls on purpose" from "just broken".
  const scrollers = [...document.querySelectorAll('body *')]
    .filter((el) => {
      const st = getComputedStyle(el);
      return (st.overflowX === 'auto' || st.overflowX === 'scroll') && el.scrollWidth > el.clientWidth + 1;
    })
    .map(describe);

  return {
    viewportWidth: vw,
    docScrollWidth: document.documentElement.scrollWidth,
    horizontalOverflow: document.documentElement.scrollWidth - vw,
    offenders: offenders.slice(0, 25),
    activeScrollers: [...new Set(scrollers)].slice(0, 10),
  };
}

async function login(page) {
  await page.goto(BASE + '/', { waitUntil: 'domcontentloaded' });
  if (await page.locator('#username').count()) {
    await page.fill('#username', USER);
    await page.fill('#password', PASS);
    await Promise.all([
      page.waitForNavigation({ waitUntil: 'domcontentloaded' }).catch(() => {}),
      page.click('button[type=submit]'),
    ]);
  }
}

// Discover the real ids the demo account owns, so every page is audited with
// representative content rather than an empty shell.
async function discover(page) {
  const ids = { tenants: [], receipts: [] };
  await page.goto(BASE + '/tenants', { waitUntil: 'domcontentloaded' });
  const hrefs = await page.$$eval('a[href^="/tenants/"]', (a) => a.map((e) => e.getAttribute('href')));
  ids.tenants = [...new Set(hrefs.map((h) => h.match(/^\/tenants\/(\d+)$/)?.[1]).filter(Boolean))].slice(0, 3);

  for (const tid of ids.tenants) {
    await page.goto(`${BASE}/tenants/${tid}`, { waitUntil: 'domcontentloaded' });
    const v = await page.$$eval('a[href^="/cash-receipts/void"]', (a) => a.map((e) => e.getAttribute('href')));
    for (const h of v) {
      const id = new URL(h, BASE).searchParams.get('receipt_id');
      if (id) ids.receipts.push(id);
    }
    if (ids.receipts.length) break;
  }
  ids.receipts = [...new Set(ids.receipts)].slice(0, 1);
  return ids;
}

const report = { base: BASE, generatedAt: new Date().toISOString(), pages: [], states: [] };
fs.mkdirSync(OUT, { recursive: true });

const browser = await chromium.launch(launchOptions());
const context = await browser.newContext({ deviceScaleFactor: 2 });
const page = await context.newPage();
const consoleErrors = [];
page.on('pageerror', (e) => consoleErrors.push(String(e)));

await login(page);
const ids = await discover(page);
console.log('discovered ids:', JSON.stringify(ids));

const pages = [
  { name: 'tenants', url: '/tenants' },
  { name: 'tenant-detail', url: ids.tenants[0] ? `/tenants/${ids.tenants[0]}` : '/tenants' },
  { name: 'rent-dashboard', url: '/rent-dashboard' },
  { name: 'billing', url: '/billing' },
  { name: 'expenses', url: '/expenses' },
  { name: 'cash-receipt-new', url: '/cash-receipts/new' },
  { name: 'cash-receipt-void', url: ids.receipts[0] ? `/cash-receipts/void?receipt_id=${ids.receipts[0]}` : null },
];

for (const vp of VIEWPORTS) {
  await page.setViewportSize({ width: vp.width, height: vp.height });
  for (const p of pages) {
    if (!p.url) continue;
    const entry = { page: p.name, viewport: vp.name, url: p.url };
    try {
      const resp = await page.goto(BASE + p.url, { waitUntil: 'networkidle', timeout: 30000 });
      entry.status = resp ? resp.status() : null;
      entry.result = await page.evaluate(probe);
      await page.screenshot({ path: path.join(OUT, `${p.name}__${vp.name}.png`), fullPage: true });
    } catch (err) {
      entry.error = String(err).slice(0, 300);
    }
    report.pages.push(entry);
    const r = entry.result;
    if (r) {
      console.log(
        `${vp.name} ${p.name.padEnd(18)} scrollW=${String(r.docScrollWidth).padStart(5)} ` +
        `overflow=${String(r.horizontalOverflow).padStart(4)} offenders=${r.offenders.length}`,
      );
    } else {
      console.log(`${vp.name} ${p.name.padEnd(18)} ERROR ${entry.error || entry.status}`);
    }
  }
}

// Hidden layouts the default screenshot never reaches: an expanded tenant row
// and the dunning drawer both add tables/forms that only exist once opened.
await page.setViewportSize({ width: 390, height: 844 });
for (const [name, url, action] of [
  ['tenants-row-expanded', '/tenants', async () => { await page.locator('.tenant-row').first().click(); }],
  ['dashboard-dunning-drawer', '/rent-dashboard', async () => { await page.locator('[data-dunning-open]').first().click(); }],
]) {
  const entry = { state: name, viewport: '390x844', url };
  try {
    await page.goto(BASE + url, { waitUntil: 'networkidle', timeout: 30000 });
    await action();
    await page.waitForTimeout(700);
    entry.result = await page.evaluate(probe);
    await page.screenshot({ path: path.join(OUT, `${name}__390x844.png`), fullPage: true });
  } catch (err) {
    entry.error = String(err).slice(0, 300);
  }
  report.states.push(entry);
  console.log(`state ${name.padEnd(28)} ${entry.result ? 'overflow=' + entry.result.horizontalOverflow : 'ERROR ' + entry.error}`);
}

// The login page can only be seen logged out: any other visit redirects to the
// dashboard. Audit it in its own context so it never inherits the session.
{
  const anon = await browser.newContext({ deviceScaleFactor: 2 });
  const anonPage = await anon.newPage();
  for (const vp of VIEWPORTS) {
    await anonPage.setViewportSize({ width: vp.width, height: vp.height });
    const entry = { page: 'login', viewport: vp.name, url: '/' };
    try {
      const resp = await anonPage.goto(BASE + '/', { waitUntil: 'networkidle', timeout: 30000 });
      entry.status = resp ? resp.status() : null;
      entry.result = await anonPage.evaluate(probe);
      await anonPage.screenshot({ path: path.join(OUT, `login__${vp.name}.png`), fullPage: true });
    } catch (err) {
      entry.error = String(err).slice(0, 300);
    }
    report.pages.push(entry);
    console.log(`${vp.name} ${'login'.padEnd(18)} scrollW=${entry.result ? entry.result.docScrollWidth : '?'} overflow=${entry.result ? entry.result.horizontalOverflow : 'ERR'}`);
  }
  await anon.close();
}

report.consoleErrors = consoleErrors;
fs.writeFileSync(path.join(OUT, 'report.json'), JSON.stringify(report, null, 2));
await browser.close();
console.log('\nwrote ' + path.join(OUT, 'report.json'));
