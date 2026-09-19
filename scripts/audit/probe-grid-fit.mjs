// Fit probe for the workspace grids (check role, 09-19-dashboard-alignment).
//
// For every view x width it reports, per grid: the scroll wrapper's client and
// scroll width, the container box / padding / gap / resolved tracks, and the
// widest *painted* content each column actually needs. It then reports the
// painted-overlap list, so a "fits" reading can never be bought by crushing the
// columns into each other.
//
// Set PROBE_CSS_FILE=/tmp/candidate.css to inject a candidate stylesheet before
// measuring, which makes the tighten-then-verify loop one command.
import { chromium } from 'playwright';
import { launchOptions } from './launch.mjs';
import fs from 'node:fs';

const BASE = (process.env.AUDIT_BASE || 'http://127.0.0.1:18093').replace(/\/+$/, '');
const PERIOD = process.env.AUDIT_PERIOD || '2026-09';
const JAR = process.env.AUDIT_COOKIE_JAR;
const CSS_FILE = process.env.PROBE_CSS_FILE || '';

function readCookies(jarPath) {
  const out = [];
  for (const raw of fs.readFileSync(jarPath, 'utf8').split('\n')) {
    const line = raw.trim().replace(/^#HttpOnly_/, '');
    if (!line || line.startsWith('#')) continue;
    const f = line.split('\t');
    if (f.length < 7) continue;
    out.push({ name: f[5], value: f[6], domain: f[0].replace(/^\./, ''), path: f[2] || '/', expires: Number(f[4]) || -1, secure: f[3] === 'TRUE' });
  }
  return out;
}

const GRIDS = {
  properties: ['.workspace-tree-head.workspace-tree-property-grid', '.workspace-tree-summary.workspace-tree-property-grid'],
  rooms: ['.workspace-tree-head.workspace-tree-room-grid', '.workspace-tree-summary.workspace-tree-room-grid'],
  tenants: [],
};
const OBLIGATION = ['.workspace-obligation-head', '.workspace-obligation-row'];

const browser = await chromium.launch(launchOptions());
const ctx = await browser.newContext({ viewport: { width: 1440, height: 900 } });
await ctx.addCookies(readCookies(JAR));
const page = await ctx.newPage();

function gridProbe(selectors) {
  const round = (v) => Math.round(v);
  const out = [];
  for (const sel of selectors) {
    const grids = Array.from(document.querySelectorAll(sel));
    if (!grids.length) continue;
    const cs = getComputedStyle(grids[0]);
    const box = round(grids[0].getBoundingClientRect().width);
    const tracks = cs.gridTemplateColumns.split(' ').map((v) => round(parseFloat(v)));
    // Widest painted content per column, over every row that uses this grid.
    const per = [];
    for (const grid of grids) {
      Array.from(grid.children).forEach((cell, i) => {
        const st = getComputedStyle(cell);
        if (st.overflowX !== 'visible') return;
        const r = document.createRange();
        r.selectNodeContents(cell);
        const painted = Math.ceil(r.getBoundingClientRect().width);
        const cellBox = round(cell.getBoundingClientRect().width);
        if (!per[i] || painted > per[i].painted) {
          per[i] = { painted, box: cellBox, text: (cell.textContent || '').replace(/\s+/g, ' ').trim().slice(0, 18) };
        }
      });
    }
    out.push({
      sel,
      rows: grids.length,
      box,
      padX: round(parseFloat(cs.paddingLeft) + parseFloat(cs.paddingRight)),
      gap: cs.columnGap,
      tracks,
      trackSum: tracks.reduce((a, b) => a + b, 0),
      minWidth: cs.minWidth,
      perColumn: per.map((p, i) => (p ? { i, painted: p.painted, box: p.box, text: p.text } : { i, painted: 0, box: 0, text: '' })),
      needSum: per.reduce((a, p) => a + (p ? p.painted : 0), 0),
    });
  }
  return out;
}

function overlapProbe() {
  const range = document.createRange();
  const bad = [];
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
    if (!style.whiteSpace.includes('nowrap')) continue;
    const clipped = style.overflowX !== 'visible'
      || [...cell.querySelectorAll('*')].some((d) => getComputedStyle(d).overflowX !== 'visible');
    if (clipped) continue;
    range.selectNodeContents(cell);
    const painted = range.getBoundingClientRect().width;
    if (painted - box.width > 1) {
      bad.push({ text: (cell.textContent || '').replace(/\s+/g, ' ').trim().slice(0, 18), box: Math.round(box.width), painted: Math.round(painted) });
    }
  }
  return bad;
}

const report = { base: BASE, period: PERIOD, injected: CSS_FILE || null, widths: [] };
const candidate = CSS_FILE ? fs.readFileSync(CSS_FILE, 'utf8') : null;

for (const vp of [{ n: '1920', w: 1920, h: 1080 }, { n: '1440', w: 1440, h: 900 }, { n: '1366', w: 1366, h: 768 }, { n: '1024', w: 1024, h: 768 }]) {
  await page.setViewportSize({ width: vp.w, height: vp.h });
  for (const view of ['properties', 'rooms', 'tenants']) {
    await page.goto(`${BASE}/rent-dashboard?period=${PERIOD}&view=${view}`, { waitUntil: 'networkidle' });
    if (candidate) {
      await page.evaluate((css) => {
        const s = document.createElement('style');
        s.id = 'probe-candidate';
        s.textContent = css;
        document.head.appendChild(s);
      }, candidate);
      await page.waitForTimeout(60);
    } else {
      await page.evaluate(() => { document.getElementById('probe-candidate')?.remove(); });
    }
    // Expand every closed disclosure so the inner grids are measurable.
    for (const sel of ['details.workspace-property', 'details.workspace-room-tree', 'details.workspace-room-item']) {
      const n = await page.locator(sel).count().catch(() => 0);
      for (let i = 0; i < n; i++) {
        await page.locator(`${sel} > summary`).nth(i).click({ timeout: 2000 }).catch(() => {});
      }
    }
    await page.waitForTimeout(120);
    const geo = await page.evaluate(() => {
      const wrap = document.querySelector('.workspace-tree-list') || document.querySelector('.table-wrap');
      return wrap ? { cls: wrap.className.toString().split(' ')[0], client: Math.round(wrap.clientWidth), scroll: Math.round(wrap.scrollWidth) } : null;
    });
    const grids = await page.evaluate(gridProbe, GRIDS[view].concat(OBLIGATION));
    const over = await page.evaluate(overlapProbe);
    const table = view === 'tenants' ? await page.evaluate(() => {
      const t = document.querySelector('.workspace-table');
      if (!t) return null;
      const cells = [...t.querySelectorAll('thead th, tbody tr:first-child td')];
      const range = document.createRange();
      const per = cells.map((c) => {
        range.selectNodeContents(c);
        const cs = getComputedStyle(c);
        return Math.ceil(range.getBoundingClientRect().width) + Math.round(parseFloat(cs.paddingLeft) + parseFloat(cs.paddingRight));
      });
      return { cols: per.length, need: per, tableMinWidth: getComputedStyle(t).minWidth, tableWidth: Math.round(t.getBoundingClientRect().width) };
    }) : null;
    report.widths.push({ width: vp.n, view, geo, grids, overlap: over, table });
  }
}

// The queue is not part of the tree grid, so it gets its own pass: the index
// badge adds a fixed first column, so an item can start overflowing sideways
// even though the list itself is 1fr-based.
report.queue = [];
for (const w of [1920, 1440, 1366, 1100, 1000, 900, 760, 660, 390]) {
  await page.setViewportSize({ width: w, height: 900 });
  await page.goto(`${BASE}/rent-dashboard?period=${PERIOD}&view=properties`, { waitUntil: 'networkidle' });
  if (candidate) {
    await page.evaluate((css) => {
      const s = document.createElement('style');
      s.id = 'probe-candidate';
      s.textContent = css;
      document.head.appendChild(s);
    }, candidate);
    await page.waitForTimeout(60);
  }
  report.queue.push(await page.evaluate((width) => {
    const t = (el) => (el?.textContent || '').replace(/\s+/g, ' ').trim();
    const list = document.querySelector('.workspace-queue-list');
    const items = Array.from(document.querySelectorAll('.workspace-queue-item'));
    const panel = document.querySelector('.workspace-action-queue');
    const doc = document.documentElement;
    return {
      width,
      listCols: list ? getComputedStyle(list).gridTemplateColumns : null,
      itemOverflow: items.map((i) => i.scrollWidth - i.clientWidth).filter((v) => v > 0),
      items: items.length,
      indexes: items.map((i) => t(i.querySelector('.workspace-queue-index'))),
      visibleButtons: items.map((i) => Array.from(i.querySelectorAll('.workspace-queue-actions .btn')).filter((b) => getComputedStyle(b).display !== 'none').length),
      buttonTexts: items.slice(0, 1).map((i) => Array.from(i.querySelectorAll('.workspace-queue-actions .btn')).map((b) => t(b))),
      lastButtonStyle: (() => {
        const b = items[0]?.querySelector('.workspace-queue-actions .btn:last-child');
        if (!b) return null;
        const cs = getComputedStyle(b);
        return { display: cs.display, border: cs.borderTopWidth, background: cs.backgroundColor, color: cs.color };
      })(),
      panelOverflow: panel ? panel.scrollWidth - panel.clientWidth : null,
      docOverflow: doc.scrollWidth - doc.clientWidth,
    };
  }, w));
}

await browser.close();
console.log(JSON.stringify(report, null, 2));
