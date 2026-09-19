// Parent-task integration review for 09-19-pc-ui-fidelity-alignment.
//
// The parent checklist asks for one pass over every workspace page at
// 1024 / 1366 / 1440 / 1920, not per-subtask spot checks, because the five
// subtasks all edit the same stylesheets and a per-subtask pass cannot see a
// defect that only shows up once two of them are combined.
//
// Checks, in order:
//   1. every nav destination renders and does not scroll horizontally;
//   2. the 641-980 range, where a subtask's sticky sidebar once covered the body;
//   3. the <=640 filter toggle, which is 09-20's defect 1 and is expected to FAIL
//      here -- this run records the pre-fix behaviour so the later fix has a
//      baseline to be compared against.
//
// Read-only: it navigates and reads geometry, it never submits a mutation form.
import { chromium } from 'playwright';
import { launchOptions } from './launch.mjs';
import fs from 'node:fs';
import path from 'node:path';

const BASE = (process.env.AUDIT_BASE || 'http://127.0.0.1:18097').replace(/\/+$/, '');
const USER = process.env.AUDIT_USER;
const PASS = process.env.AUDIT_PASS;
const PERIOD = process.env.AUDIT_PERIOD || '2026-09';
const OUT = process.env.AUDIT_OUT || '/tmp/integration-review';

if (!USER || !PASS) {
  console.error('set AUDIT_USER and AUDIT_PASS (see the launcher output)');
  process.exit(1);
}
fs.mkdirSync(OUT, { recursive: true });

// Heights come from the handoff's viewport matrix (figma/DESIGN-HANDOFF.md) so a
// screenshot is directly comparable with the prototype's own frame and with the
// per-subtask sets already in the repo.
const WIDTHS = [
  [1024, 768],
  [1366, 768],
  [1440, 900],
  [1920, 1080],
];
const PAGES = [
  ['/rent-dashboard?period=' + PERIOD, 'rent-dashboard'],
  ['/bills?period=' + PERIOD, 'bills'],
  ['/transactions?period=' + PERIOD, 'transactions'],
  ['/properties?period=' + PERIOD, 'properties'],
  ['/rooms?period=' + PERIOD, 'rooms'],
  ['/tenancies?period=' + PERIOD, 'tenancies'],
  ['/tenants', 'tenants'],
  ['/expenses?period=' + PERIOD, 'expenses'],
  ['/cash-receipts', 'cash-receipts'],
  ['/dunning', 'dunning'],
  ['/bank', 'bank'],
];

// A page may legitimately scroll inside a wrapper; only the document must not.
const docGeometry = () => ({
  scrollWidth: document.documentElement.scrollWidth,
  clientWidth: document.documentElement.clientWidth,
  bodyScrollWidth: document.body.scrollWidth,
});

const browser = await chromium.launch(launchOptions());
const ctx = await browser.newContext({ viewport: { width: 1440, height: 900 } });
const page = await ctx.newPage();

await page.goto(BASE + '/', { waitUntil: 'domcontentloaded' });
if (await page.$('#username')) {
  await page.fill('#username', USER);
  await page.fill('#password', PASS);
  await Promise.all([page.waitForNavigation({ waitUntil: 'domcontentloaded' }), page.click('button[type=submit]')]);
}
if (await page.$('#username')) {
  console.error('login did not take: still on the login form');
  await browser.close();
  process.exit(1);
}

const report = { base: BASE, period: PERIOD, overflow: [], errors: [], toggle: null, sidebar: null };

// ---- 1. four widths, every page -------------------------------------------
for (const [url, name] of PAGES) {
  for (const [width, height] of WIDTHS) {
    await page.setViewportSize({ width, height });
    let resp;
    try {
      resp = await page.goto(BASE + url, { waitUntil: 'networkidle', timeout: 30000 });
    } catch (err) {
      report.errors.push({ page: name, width, error: String(err).slice(0, 200) });
      continue;
    }
    if (!resp || resp.status() >= 400) {
      report.errors.push({ page: name, width, error: `HTTP ${resp ? resp.status() : 'no response'}` });
      continue;
    }
    const geo = await page.evaluate(docGeometry);
    if (geo.scrollWidth !== geo.clientWidth) {
      report.overflow.push({ page: name, width, ...geo, over: geo.scrollWidth - geo.clientWidth });
    }
    await page.screenshot({ path: path.join(OUT, `${name}-${width}.png`), fullPage: true });
  }
}

// ---- 2. the 641-980 range: nothing may cover the body ----------------------
for (const width of [800, 900, 980]) {
  await page.setViewportSize({ width, height: 700 });
  await page.goto(BASE + '/rent-dashboard?period=' + PERIOD, { waitUntil: 'networkidle' });
  await page.evaluate(() => window.scrollTo(0, document.body.scrollHeight));
  const probe = await page.evaluate(() => {
    const rail = document.querySelector('.sidebar');
    if (!rail) return { railPresent: false };
    const r = rail.getBoundingClientRect();
    const cx = Math.round(window.innerWidth / 2);
    const cy = Math.round(window.innerHeight / 2);
    const hit = document.elementFromPoint(cx, cy);
    return {
      railPresent: true,
      position: getComputedStyle(rail).position,
      railRect: { top: Math.round(r.top), bottom: Math.round(r.bottom), height: Math.round(r.height) },
      centerHitInsideRail: !!(hit && rail.contains(hit)),
      centerHit: hit ? hit.tagName + '.' + String(hit.className || '').split(' ')[0] : null,
    };
  });
  report.sidebar = report.sidebar || {};
  report.sidebar[width] = probe;
}

// ---- 3. <=640 filter toggle (09-20 defect 1 baseline) ----------------------
await page.setViewportSize({ width: 390, height: 844 });
report.toggle = {};
for (const [name, url] of [['properties', '/properties?period=' + PERIOD], ['rooms', '/rooms?period=' + PERIOD], ['tenancies', '/tenancies?period=' + PERIOD]]) {
  await page.goto(BASE + url, { waitUntil: 'networkidle' });
  const result = await page.evaluate(async () => {
    const btn = document.querySelector('.object-list-filter-toggle');
    if (!btn) return { togglePresent: false };
    const before = btn.getAttribute('aria-expanded');
    const panelBefore = getComputedStyle(document.querySelector('.object-list-filter-fields') || document.body).display;
    btn.click();
    await new Promise((r) => setTimeout(r, 50));
    const panelEl = document.querySelector('.object-list-filter-fields');
    return {
      togglePresent: true,
      ariaBefore: before,
      ariaAfter: btn.getAttribute('aria-expanded'),
      panelDisplayBefore: panelBefore,
      panelDisplayAfter: panelEl ? getComputedStyle(panelEl).display : null,
      expanded: btn.getAttribute('aria-expanded') === 'true',
    };
  });
  report.toggle[name] = result;
}

await browser.close();
fs.writeFileSync(path.join(OUT, 'report.json'), JSON.stringify(report, null, 2));

const overflows = report.overflow.length;
const errors = report.errors.length;
console.log(`pages x widths: ${PAGES.length} x ${WIDTHS.length}`);
console.log(`horizontal overflow: ${overflows}`);
for (const o of report.overflow) console.log(`  ${o.page} @${o.width}: ${o.scrollWidth} > ${o.clientWidth} (+${o.over})`);
console.log(`errors: ${errors}`);
for (const e of report.errors) console.log(`  ${e.page} @${e.width}: ${e.error}`);
console.log('sidebar (641-980, scrolled to bottom):');
for (const [w, s] of Object.entries(report.sidebar || {})) {
  console.log(`  @${w}: position=${s.position} centerHit=${s.centerHit} coversBody=${s.centerHitInsideRail}`);
}
console.log('<=640 filter toggle:');
for (const [name, t] of Object.entries(report.toggle)) {
  console.log(`  ${name}: aria ${t.ariaBefore} -> ${t.ariaAfter} (expanded=${t.expanded})`);
}
console.log(`artifacts: ${OUT}`);
process.exit(overflows === 0 && errors === 0 ? 0 : 1);
