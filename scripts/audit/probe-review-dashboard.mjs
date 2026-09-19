// Independent re-verification probe for 09-19-dashboard-alignment (check role).
//
// Reads the live audit instance's session cookie from the runtime cookie jar
// (AUDIT_COOKIE_JAR) so it never has to know or print a credential, then
// measures the geometry the implementer's claims rest on:
//   * where does the 1440px horizontal scroll of the property view come from —
//     the column minima, or the grid's own `min-width`?
//   * can the 12 columns fit the available width with zero painted overlap?
//   * does the tool's own overlap probe stay silent on the shipped CSS?
//   * chips vs the rendered table rows, per view;
//   * indent levels and <=640 layout overflow.
//
// Usage:
//   AUDIT_COOKIE_JAR=/path/cookies.txt node scripts/audit/probe-review-dashboard.mjs
import { chromium } from 'playwright';
import { launchOptions } from './launch.mjs';
import fs from 'node:fs';

const BASE = (process.env.AUDIT_BASE || 'http://127.0.0.1:18093').replace(/\/+$/, '');
const PERIOD = process.env.AUDIT_PERIOD || '2026-09';
const JAR = process.env.AUDIT_COOKIE_JAR;
if (!JAR || !fs.existsSync(JAR)) {
  console.error('AUDIT_COOKIE_JAR must point at the running instance cookie jar');
  process.exit(2);
}

function readCookies(jarPath) {
  const out = [];
  for (const raw of fs.readFileSync(jarPath, 'utf8').split('\n')) {
    // Netscape jars mark HttpOnly cookies with a leading "#HttpOnly_", which is
    // the only cookie this app sets — skipping comment lines wholesale would
    // silently drop the whole session.
    const line = raw.trim().replace(/^#HttpOnly_/, '');
    if (!line || line.startsWith('#')) continue;
    const f = line.split('\t');
    if (f.length < 7) continue;
    out.push({
      name: f[5],
      value: f[6],
      domain: f[0].replace(/^\./, ''),
      path: f[2] || '/',
      expires: Number(f[4]) || -1,
      secure: f[3] === 'TRUE',
    });
  }
  return out;
}

const url = (view) => `${BASE}/rent-dashboard?period=${PERIOD}&view=${view}`;

const browser = await chromium.launch(launchOptions());
const ctx = await browser.newContext({ viewport: { width: 1440, height: 900 } });
await ctx.addCookies(readCookies(JAR));
const page = await ctx.newPage();

// Mirrors dashboard-alignment.mjs's overlap probe exactly, so the verdict is
// comparable, but reports what it measured rather than only a count.
function measure() {
  const text = (el) => (el?.textContent || '').replace(/\s+/g, ' ').trim();
  const wrap = document.querySelector('.workspace-tree-list, .table-wrap');
  const grid = document.querySelector('.workspace-tree-head.workspace-tree-property-grid')
    || document.querySelector('.workspace-tree-head.workspace-tree-room-grid');
  const gridStyle = grid ? getComputedStyle(grid) : null;
  const cols = gridStyle ? gridStyle.gridTemplateColumns.split(' ').map((v) => Math.round(parseFloat(v))) : [];

  const range = document.createRange();
  const overlap = [];
  const cells = document.querySelectorAll([
    '.workspace-tree-summary > *', '.workspace-tree-head > *',
    '.workspace-obligation-row > *', '.workspace-obligation-head > *',
    '.workspace-table tbody td', '.workspace-table thead th',
  ].join(','));
  for (const cell of cells) {
    if (cell.querySelector('.workspace-row-action, .collection-settle, .workspace-progress')) continue;
    const box = cell.getBoundingClientRect();
    if (box.width < 1) continue;
    const style = getComputedStyle(cell);
    range.selectNodeContents(cell);
    const painted = range.getBoundingClientRect().width;
    const excess = Math.round(painted - box.width);
    const clipped = style.overflowX !== 'visible'
      || [...cell.querySelectorAll('*')].some((d) => getComputedStyle(d).overflowX !== 'visible');
    if (clipped) continue;
    if (!style.whiteSpace.includes('nowrap')) continue;
    if (excess > 1) {
      overlap.push({ text: (cell.textContent || '').trim().slice(0, 16), box: Math.round(box.width), painted: Math.round(painted), excess });
    }
  }

  // Widest content the money columns must hold, measured intrinsically.
  const widths = [];
  for (const cell of document.querySelectorAll('.workspace-tree-summary.workspace-tree-property-grid > *')) {
    const clone = cell.cloneNode(true);
    const probe = document.createElement('span');
    probe.style.cssText = 'position:absolute;visibility:hidden;white-space:nowrap;left:-9999px';
    const cs = getComputedStyle(cell);
    probe.style.font = cs.font;
    probe.textContent = text(cell);
    document.body.appendChild(probe);
    widths.push(Math.ceil(probe.getBoundingClientRect().width));
    probe.remove();
    void clone;
  }

  const rect = grid ? grid.getBoundingClientRect() : null;
  return {
    docOverflow: document.documentElement.scrollWidth - document.documentElement.clientWidth,
    wrapClient: wrap ? Math.round(wrap.clientWidth) : null,
    wrapScroll: wrap ? Math.round(wrap.scrollWidth) : null,
    gridMinWidth: gridStyle ? gridStyle.minWidth : null,
    gridWidth: rect ? Math.round(rect.width) : null,
    gridPad: gridStyle ? gridStyle.paddingLeft + '/' + gridStyle.paddingRight : null,
    gridGap: gridStyle ? gridStyle.columnGap : null,
    cols,
    colSum: cols.reduce((a, b) => a + b, 0),
    overlap,
    intrinsic: widths,
  };
}

const report = { base: BASE, period: PERIOD, widths: [], chips: {}, mobile: {}, indent: {} };

const gridCSS = {
  property: parseInt((process.env.PROBE_PROPERTY_MIN || '0'), 10) || null,
  room: null,
};

for (const vp of [
  { name: '1920', width: 1920, height: 1080 },
  { name: '1440', width: 1440, height: 900 },
  { name: '1366', width: 1366, height: 768 },
  { name: '1024', width: 1024, height: 768 },
]) {
  await page.setViewportSize({ width: vp.width, height: vp.height });
  for (const view of ['properties', 'rooms', 'tenants']) {
    await page.goto(url(view), { waitUntil: 'networkidle' });
    const asShipped = await page.evaluate(measure, null);
    const entry = { view: view, width: vp.name, shipped: asShipped };
    // The decisive experiment: drop the grid's explicit `min-width` so it
    // resolves to the wrapper's content width. If the columns then still hold
    // their content without painting over each other, the scroll was the
    // min-width, not the column minima.
    const gridClass = view === 'properties' ? '.workspace-tree-property-grid'
      : view === 'rooms' ? '.workspace-tree-room-grid' : null;
    if (gridClass) {
      await page.evaluate((cls) => {
        const s = document.createElement('style');
        s.id = 'probe-override';
        s.textContent = `${cls}{min-width:0!important}`;
        document.head.appendChild(s);
      }, gridClass);
      entry.forcedToClient = await page.evaluate(measure);
    }
    report.widths.push(entry);
  }
}

// Chips vs the rows actually rendered.
for (const view of ['properties', 'rooms', 'tenants']) {
  await page.setViewportSize({ width: 1440, height: 1400 });
  await page.goto(url(view), { waitUntil: 'networkidle' });
  report.chips[view] = await page.evaluate(() => {
    const t = (el) => (el?.textContent || '').replace(/\s+/g, ' ').trim();
    const chips = Array.from(document.querySelectorAll('.workspace-chip')).map((c) => ({ cls: c.className, text: t(c) }));
    // Statuses as the *table* renders them. Scope to the desktop presentation so
    // the mobile twin does not double-count.
    const desktop = document.querySelector('.workspace-desktop-list');
    const scope = desktop || document;
    const statuses = [...scope.querySelectorAll('.workspace-status')]
      .filter((el) => !el.closest('.workspace-mobile-list'))
      .map((el) => el.className.replace('workspace-status', '').trim() + ':' + t(el));
    const counts = {};
    for (const s of statuses) counts[s.split(':')[1]] = (counts[s.split(':')[1]] || 0) + 1;
    const rows = {
      property: scope.querySelectorAll('.workspace-property').length,
      room: scope.querySelectorAll('.workspace-room-tree, .workspace-room-item').length,
      tenant: scope.querySelectorAll('.workspace-table tbody tr').length,
      obligation: scope.querySelectorAll('.workspace-obligation-row').length,
    };
    return { chips, statusCounts: counts, rows, totalRowsText: t(document.querySelector('.workspace-pager')) };
  });
}

// <=640: is anything pushed sideways by the newly added row actions/cards?
for (const view of ['properties', 'rooms', 'tenants']) {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto(url(view), { waitUntil: 'networkidle' });
  report.mobile[view] = await page.evaluate(async () => {
    const worst = [];
    const doc = document.documentElement;
    const all = document.querySelectorAll('*');
    for (const el of all) {
      const r = el.getBoundingClientRect();
      if (r.width < 1) continue;
      const over = Math.round(r.right - doc.clientWidth);
      if (over > 1) worst.push({ tag: el.tagName, cls: (el.className || '').toString().slice(0, 48), right: Math.round(r.right), over });
    }
    worst.sort((a, b) => b.over - a.over);
    return {
      docOverflow: doc.scrollWidth - doc.clientWidth,
      docWidth: doc.clientWidth,
      worst: worst.slice(0, 6),
      rowActions: document.querySelectorAll('.workspace-row-actions').length,
      settleForms: document.querySelectorAll('.workspace-row-actions .collection-settle').length,
      detailLinks: document.querySelectorAll('.workspace-row-actions a.btn.subtle').length,
    };
  });
}

// Indent levels in the real DOM (the implementer's self-found padding bug).
for (const view of ['properties', 'rooms']) {
  await page.setViewportSize({ width: 1440, height: 1400 });
  await page.goto(url(view), { waitUntil: 'networkidle' });
  const root = view === 'properties' ? 'details.workspace-property' : 'details.workspace-room-item';
  if (await page.locator(root).count()) {
    await page.locator(`${root} > summary`).first().click({ timeout: 5000 }).catch(() => {});
    await page.waitForTimeout(120);
  }
  if (await page.locator('details.workspace-room-tree').count()) {
    await page.locator('details.workspace-room-tree > summary').first().click({ timeout: 5000 }).catch(() => {});
    await page.waitForTimeout(120);
  }
  report.indent[view] = await page.evaluate(() => {
    const l1 = document.querySelector('.tree-level-1');
    const l2 = document.querySelector('.tree-level-2');
    return {
      l1: l1 ? getComputedStyle(l1).paddingLeft : null,
      l2: l2 ? getComputedStyle(l2).paddingLeft : null,
      obligationRows: document.querySelectorAll('.workspace-obligation-row').length,
    };
  });
}

await browser.close();
console.log(JSON.stringify(report, null, 2));
