// Decisive experiment for the 1440px property-table scroll (check role).
//
// Measures, per column of the property grid, the widest painted content the
// rows actually need, then re-renders the grid with the explicit `min-width`
// removed and with tightened column minima, reporting the horizontal scroll and
// the painted overlap each time. The point is to separate "the column minima
// cannot fit" from "the grid was told not to fit".
import { chromium } from 'playwright';
import { launchOptions } from './launch.mjs';
import fs from 'node:fs';

const BASE = (process.env.AUDIT_BASE || 'http://127.0.0.1:18093').replace(/\/+$/, '');
const PERIOD = process.env.AUDIT_PERIOD || '2026-09';
const JAR = process.env.AUDIT_COOKIE_JAR;

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

const browser = await chromium.launch(launchOptions());
const ctx = await browser.newContext({ viewport: { width: 1440, height: 1400 } });
await ctx.addCookies(readCookies(JAR));
const page = await ctx.newPage();

const probe = () => {
  const wrap = document.querySelector('.workspace-tree-list');
  const grids = Array.from(document.querySelectorAll('.workspace-tree-property-grid'));
  // Per-column painted width, taking the widest cell in that grid position.
  const perColumn = [];
  for (const grid of grids) {
    Array.from(grid.children).forEach((cell, i) => {
      if (cell.querySelector('.workspace-row-action, .workspace-progress')) return;
      const style = getComputedStyle(cell);
      if (style.overflowX !== 'visible') return;
      const r = document.createRange();
      r.selectNodeContents(cell);
      const painted = Math.ceil(r.getBoundingClientRect().width);
      const box = Math.round(cell.getBoundingClientRect().width);
      perColumn[i] = perColumn[i] || { painted: 0, box: 0, text: '' };
      if (painted > perColumn[i].painted) perColumn[i] = { painted, box, text: (cell.textContent || '').trim().slice(0, 14) };
    });
  }
  return {
    wrapClient: Math.round(wrap.clientWidth),
    wrapScroll: Math.round(wrap.scrollWidth),
    docOverflow: document.documentElement.scrollWidth - document.documentElement.clientWidth,
    gridWidth: Math.round(grids[0].getBoundingClientRect().width),
    cols: getComputedStyle(grids[0]).gridTemplateColumns.split(' ').map((v) => Math.round(parseFloat(v))),
    gap: getComputedStyle(grids[0]).columnGap,
    perColumn,
  };
};

const out = {};
await page.goto(`${BASE}/rent-dashboard?period=${PERIOD}&view=properties`, { waitUntil: 'networkidle' });
// Open the first property so the room rows and their money cells are measurable.
await page.locator('details.workspace-property > summary').first().click();
await page.waitForTimeout(150);
out.shipped = await page.evaluate(probe);

const variants = {
  // What the content actually needs: the widest painted amount is 98px
  // ("EUR 5200.00"), the widest count 35, the action button 70. Sized to those
  // with a few px of slack and a 10px gap, the grid has no reason to scroll.
  'fit-to-content': `.workspace-tree-property-grid{min-width:0!important;column-gap:10px!important;grid-template-columns:
     minmax(130px,1.5fr) repeat(3,minmax(38px,.42fr)) repeat(5,minmax(104px,.74fr))
     minmax(84px,.85fr) minmax(62px,.6fr) minmax(88px,.62fr)!important}`,
  'min-width:0': '.workspace-tree-property-grid{min-width:0!important}',
  'tight-minima': `.workspace-tree-property-grid{min-width:0!important;grid-template-columns:
     minmax(120px,1.5fr) repeat(3,minmax(34px,.42fr)) repeat(5,minmax(96px,.74fr))
     minmax(80px,.85fr) minmax(60px,.6fr) minmax(84px,.62fr)!important}`,
  'tight+gaps8': `.workspace-tree-list{--x:0}.workspace-tree-property-grid{min-width:0!important;column-gap:8px!important;grid-template-columns:
     minmax(120px,1.5fr) repeat(3,minmax(34px,.42fr)) repeat(5,minmax(96px,.74fr))
     minmax(80px,.85fr) minmax(60px,.6fr) minmax(84px,.62fr)!important}`,
};
for (const [name, css] of Object.entries(variants)) {
  await page.evaluate((c) => {
    let s = document.getElementById('probe-v');
    if (!s) { s = document.createElement('style'); s.id = 'probe-v'; document.head.appendChild(s); }
    s.textContent = c;
  }, css);
  await page.waitForTimeout(80);
  out[name] = await page.evaluate(probe);
  // And the tool's own overlap probe, verbatim, under this variant.
  out[name].overlap = await page.evaluate(() => {
    const range = document.createRange();
    const bad = [];
    for (const cell of document.querySelectorAll('.workspace-tree-summary > *, .workspace-tree-head > *, .workspace-obligation-row > *, .workspace-obligation-head > *')) {
      if (cell.querySelector('.workspace-row-action, .collection-settle, .workspace-progress')) continue;
      const box = cell.getBoundingClientRect();
      if (box.width < 1) continue;
      const style = getComputedStyle(cell);
      const clipped = style.overflowX !== 'visible' || [...cell.querySelectorAll('*')].some((d) => getComputedStyle(d).overflowX !== 'visible');
      if (clipped) continue;
      if (!style.whiteSpace.includes('nowrap')) continue;
      range.selectNodeContents(cell);
      const painted = range.getBoundingClientRect().width;
      if (painted - box.width > 1) bad.push({ text: (cell.textContent || '').trim().slice(0, 16), box: Math.round(box.width), painted: Math.round(painted) });
    }
    return bad;
  });
}

await browser.close();
console.log(JSON.stringify(out, null, 2));
