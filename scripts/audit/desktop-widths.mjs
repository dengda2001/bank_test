// Desktop viewport matrix over the eleven workspace pages.
//
// audit.mjs only ever looked at phone and tablet widths, and desktop-full.mjs
// pins a single 1440x900 viewport. The desktop contract work needs the
// opposite: every top-level page at each width in the handoff's desktop band,
// asserting that the *document* never scrolls horizontally.
//
// A wide table scrolling inside .table-wrap is expected and fine. What this
// reports is document-level overflow, which is always a defect — and it names
// the root offender (the outermost element whose right edge passes the
// viewport) so the cause is distinguishable from its descendants.
//
// Usage:
//   AUDIT_BASE=http://127.0.0.1:18090 AUDIT_USER=... AUDIT_PASS=... \
//     AUDIT_OUT=/tmp/desktop-widths node scripts/audit/desktop-widths.mjs
import { chromium } from 'playwright';
import { launchOptions } from './launch.mjs';
import fs from 'node:fs';

const BASE = (process.env.AUDIT_BASE || 'http://127.0.0.1:18090').replace(/\/+$/, '');
const OUT = process.env.AUDIT_OUT || '/tmp/mobile-audit/desktop-widths';
fs.mkdirSync(OUT, { recursive: true });

// The handoff's desktop band (figma/DESIGN-HANDOFF.md "Responsive contract").
// 1024 sits inside the 1100 tier this work introduces; 1366 is the laptop
// baseline; 1440 and 1920 are the two widths the desktop contract was frozen at.
const WIDTHS = [
  { name: '1024', width: 1024, height: 768, note: 'tablet landscape / 1100 tier' },
  { name: '1366', width: 1366, height: 768, note: 'laptop' },
  { name: '1440', width: 1440, height: 900, note: 'desktop' },
  { name: '1920', width: 1920, height: 1080, note: 'wide' },
];

// The eleven sidebar destinations, in nav order (workspace-nav.html).
const PAGES = [
  ['rent-dashboard', '/rent-dashboard'],
  ['bills', '/bills'],
  ['transactions', '/transactions'],
  ['dunning', '/dunning'],
  ['properties', '/properties'],
  ['rooms', '/rooms'],
  ['tenants', '/tenants'],
  ['tenancies', '/tenancies'],
  ['cash-receipts', '/cash-receipts'],
  ['expenses', '/expenses'],
  ['bank', '/bank'],
];

// Runs in the page: document overflow plus the outermost offenders only.
function probe() {
  const vw = document.documentElement.clientWidth;
  const lim = vw + 1;
  const offenders = [];
  for (const el of document.querySelectorAll('body *')) {
    const r = el.getBoundingClientRect();
    if (r.width === 0 || r.height === 0) continue;
    if (r.right <= lim) continue;
    const pr = el.parentElement?.getBoundingClientRect();
    if (pr && pr.right > lim) continue; // a descendant of an overflowing parent is not the cause
    const cls = typeof el.className === 'string' && el.className.trim()
      ? '.' + el.className.trim().split(/\s+/).join('.')
      : '';
    offenders.push({
      sel: el.tagName.toLowerCase() + cls,
      right: Math.round(r.right),
      width: Math.round(r.width),
      text: (el.textContent || '').trim().slice(0, 48),
    });
  }
  return {
    vw,
    clientW: document.documentElement.clientWidth,
    scrollW: document.documentElement.scrollWidth,
    bodyScrollW: document.body.scrollWidth,
    offenders: offenders.slice(0, 8),
  };
}

const browser = await chromium.launch(launchOptions());
const ctx = await browser.newContext({ viewport: { width: 1440, height: 900 } });
const page = await ctx.newPage();

await page.goto(BASE + '/', { waitUntil: 'domcontentloaded' });
await page.fill('#username', process.env.AUDIT_USER);
await page.fill('#password', process.env.AUDIT_PASS);
await Promise.all([
  page.waitForNavigation({ waitUntil: 'domcontentloaded' }).catch(() => {}),
  page.click('button[type=submit]'),
]);

const report = { base: BASE, widths: WIDTHS.map((w) => w.name), pages: [], failures: [] };

for (const vp of WIDTHS) {
  await page.setViewportSize({ width: vp.width, height: vp.height });
  console.log(`\n===== ${vp.name} (${vp.note}) =====`);
  for (const [name, url] of PAGES) {
    const resp = await page.goto(BASE + url, { waitUntil: 'networkidle', timeout: 30000 });
    const m = await page.evaluate(probe);
    const overflow = m.scrollW - m.clientW;
    const ok = overflow <= 0;
    report.pages.push({ page: name, url, width: vp.name, status: resp?.status() ?? null, ...m, overflow, ok });
    if (!ok) report.failures.push({ page: name, width: vp.name, overflow, offenders: m.offenders });
    console.log(
      `${ok ? 'ok  ' : 'FAIL'} ${name.padEnd(16)} http=${resp?.status()} vw=${m.vw} client=${m.clientW} scroll=${m.scrollW} overflow=${overflow}`
    );
    for (const o of m.offenders) {
      console.log(`        offender right=${o.right} w=${o.width} ${o.sel}  "${o.text}"`);
    }
    await page.screenshot({ path: `${OUT}/${name}-${vp.name}.png`, fullPage: true });
  }
}

fs.writeFileSync(`${OUT}/report.json`, JSON.stringify(report, null, 2));
await browser.close();

console.log(`\nwrote ${OUT}`);
if (report.failures.length) {
  console.error(`\n${report.failures.length} document-level overflow(s):`);
  for (const f of report.failures) {
    console.error(`  ${f.page} @ ${f.width}: +${f.overflow}px`);
    for (const o of f.offenders) console.error(`      ${o.sel} (w=${o.width})`);
  }
  process.exit(1);
}
console.log('no document-level horizontal overflow at any audited width');
