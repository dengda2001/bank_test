// Independent walk-through of the /rent-dashboard alignment
// (09-19-dashboard-alignment).
//
// It drives the live workspace — never the removed legacy fallback template —
// and does three things:
//   1. asserts the document never scrolls horizontally at 1024/1366/1440/1920;
//   2. screenshots each of the three views at each of those widths into the
//      task's research/screenshots directory;
//   3. re-derives every structural claim from the DOM (chips, section title,
//      currency note, metric emphasis, compact queue, the new rate and action
//      columns, the reused settle form) plus the keyboard expansion contract.
//
// Usage:
//   AUDIT_BASE=http://127.0.0.1:18093 AUDIT_USER=... AUDIT_PASS=... \
//     node scripts/audit/dashboard-alignment.mjs
import { chromium } from 'playwright';
import { launchOptions } from './launch.mjs';
import fs from 'node:fs';
import path from 'node:path';

const BASE = (process.env.AUDIT_BASE || 'http://127.0.0.1:18093').replace(/\/+$/, '');
const PERIOD = process.env.AUDIT_PERIOD || '2026-09';
const OUT = process.env.AUDIT_OUT
  || path.resolve('.trellis/tasks/09-19-dashboard-alignment/research/screenshots');
fs.mkdirSync(OUT, { recursive: true });

const WIDTHS = [
  { name: '1024', width: 1024, height: 768 },
  { name: '1366', width: 1366, height: 768 },
  { name: '1440', width: 1440, height: 900 },
  { name: '1920', width: 1920, height: 1080 },
];
const VIEWS = [
  { name: 'properties', label: '房产' },
  { name: 'rooms', label: '房间' },
  { name: 'tenants', label: '租客' },
];

const url = (view) => `${BASE}/rent-dashboard?period=${PERIOD}&view=${view}`;

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

const report = { base: BASE, period: PERIOD, widths: [], shots: [], structure: {}, failures: [] };
const fail = (message) => report.failures.push(message);

function probe() {
  const text = (el) => (el?.textContent || '').replace(/\s+/g, ' ').trim();
  const panel = document.querySelector('.workspace-list-panel');
  const tabs = document.querySelector('.workspace-tabs');
  const chips = Array.from(document.querySelectorAll('.workspace-chip')).map((c) => ({
    kind: c.className.replace('workspace-chip', '').trim(),
    text: text(c),
    value: text(c.querySelector('strong')),
  }));
  const metrics = Array.from(document.querySelectorAll('.workspace-summary .metric')).map((m) => ({
    cls: m.className,
    label: text(m.querySelector('.label')),
    value: text(m.querySelector('strong')),
    topBorder: getComputedStyle(m).borderTopWidth,
    topColor: getComputedStyle(m).borderTopColor,
  }));
  const queue = {
    inHead: !!document.querySelector('.workspace-action-queue .panel-head .workspace-queue-more'),
    more: text(document.querySelector('.workspace-queue-more')),
    moreHref: document.querySelector('.workspace-queue-more')?.getAttribute('href') || '',
    columns: getComputedStyle(document.querySelector('.workspace-queue-list') || document.body).gridTemplateColumns,
    items: Array.from(document.querySelectorAll('.workspace-queue-item')).map((item) => ({
      index: text(item.querySelector('.workspace-queue-index')),
      title: text(item.querySelector('.workspace-queue-body strong')),
      amount: text(item.querySelector('.workspace-queue-amount')),
      actions: Array.from(item.querySelectorAll('.workspace-queue-actions a')).map(text),
      // The prototype's desktop queue carries one borderless text link, not a
      // pair of buttons; the count and chrome are read off the live DOM.
      visibleActions: Array.from(item.querySelectorAll('.workspace-queue-actions a')).filter((a) => getComputedStyle(a).display !== 'none').map(text),
      lastActionChrome: (() => {
        const a = item.querySelector('.workspace-queue-actions a:last-child');
        if (!a) return null;
        const cs = getComputedStyle(a);
        return { border: cs.borderTopWidth, background: cs.backgroundColor };
      })(),
    })),
  };
  const grids = {
    property: Array.from(document.querySelectorAll('.workspace-tree-property-grid')).slice(0, 1)
      .flatMap((grid) => Array.from(grid.children).map(text)),
    room: Array.from(document.querySelectorAll('.workspace-tree-room-grid')).slice(0, 1)
      .flatMap((grid) => Array.from(grid.children).map(text)),
    tenant: Array.from(document.querySelectorAll('.workspace-table thead th')).map(text),
  };
  const rows = {
    properties: document.querySelectorAll('.workspace-property').length,
    rooms: document.querySelectorAll('.workspace-room-tree, .workspace-room-item').length,
    tenants: document.querySelectorAll('.workspace-table tbody tr').length,
    obligationRows: document.querySelectorAll('.workspace-obligation-row').length,
  };
  const rates = Array.from(document.querySelectorAll('.workspace-rate')).map((rate) => ({
    value: text(rate.querySelector('.workspace-rate-value')),
    bar: rate.querySelector('.workspace-progress > span')?.style.width || '',
  }));
  const actions = {
    total: document.querySelectorAll('.workspace-row-action, .workspace-table tbody td:last-child').length,
    detailLinks: Array.from(document.querySelectorAll('.workspace-row-action a.btn.subtle, .workspace-table tbody td:last-child a.btn.subtle'))
      .map((a) => ({ text: text(a), href: a.getAttribute('href') })),
    settle: document.querySelectorAll('.workspace-row-action .collection-settle, .workspace-table tbody td:last-child .collection-settle').length,
    settleAction: document.querySelector('.workspace-row-action .collection-balance-form, .workspace-table tbody td:last-child .collection-balance-form')?.getAttribute('action') || '',
  };
  const indent = {
    level1: document.querySelectorAll('.tree-level-1').length,
    level2: document.querySelectorAll('.tree-level-2').length,
    level1Pad: getComputedStyle(document.querySelector('.tree-level-1') || document.body).paddingLeft,
    level2Pad: getComputedStyle(document.querySelector('.tree-level-2') || document.body).paddingLeft,
  };
  // `scrollWidth` cannot see text that overflows a `white-space: nowrap` cell,
  // so measure the painted text with a Range instead. This is the check that
  // catches narrow money columns: the numbers are short enough for `scrollWidth`
  // to look innocent while the text paints over the next column.
  //
  // Only cells that would actually let text escape are judged: a cell that wraps
  // or that clips with its own ellipsis cannot collide with its neighbour, and
  // cells inside a collapsed <details> have no box at all.
  const range = document.createRange();
  const overlap = [];
  const truncated = [];
  const cells = document.querySelectorAll([
    '.workspace-tree-summary > *',
    '.workspace-tree-head > *',
    '.workspace-obligation-row > *',
    '.workspace-obligation-head > *',
    '.workspace-table tbody td',
    '.workspace-table thead th',
  ].join(','));
  for (const cell of cells) {
    if (cell.querySelector('.workspace-row-action, .collection-settle, .workspace-progress')) continue;
    const box = cell.getBoundingClientRect();
    if (box.width < 1) continue;
    const style = getComputedStyle(cell);
    range.selectNodeContents(cell);
    const painted = range.getBoundingClientRect().width;
    const excess = Math.round(painted - box.width);
    const label = (cell.textContent || '').trim().slice(0, 18);
    // Something inside clips the text (the app's own ellipsis on the entity
    // subtitle is deliberate), so it can never paint over the next column.
    const clipped = style.overflowX !== 'visible'
      || [...cell.querySelectorAll('*')].some((d) => getComputedStyle(d).overflowX !== 'visible');
    if (clipped) {
      if (excess > 1) truncated.push({ text: label, excess });
      continue;
    }
    if (!style.whiteSpace.includes('nowrap')) continue; // wraps instead of colliding
    if (excess > 1) overlap.push({ text: label, box: Math.round(box.width), painted: Math.round(painted), excess });
  }
  return {
    view: new URL(location.href).searchParams.get('view'),
    tabsInsidePanel: !!(panel && tabs && panel.contains(tabs)),
    tabsInControls: !!tabs && tabs.closest('.workspace-list-controls') === tabs.parentElement,
    controlsRow: (() => {
      const controls = document.querySelector('.workspace-list-controls');
      if (!controls) return null;
      const t = tabs?.getBoundingClientRect();
      const f = document.querySelector('.workspace-filters')?.getBoundingClientRect();
      if (!t || !f) return null;
      return { sameRow: Math.abs(t.top - f.top) < Math.max(t.height, f.height) };
    })(),
    tabsOrder: (() => {
      const controls = document.querySelector('.workspace-list-controls');
      return controls ? Array.from(controls.children).map((c) => c.className) : null;
    })(),
    sectionTitle: text(document.querySelector('.workspace-dimension-head h2')),
    sectionDesc: text(document.querySelector('.workspace-dimension-head p')),
    currencyNote: text(document.querySelector('.workspace-currency-note')),
    chips,
    metrics,
    queue,
    grids,
    rows,
    rates,
    actions,
    indent,
    overlap,
    truncated,
    demoLabelPresent: document.body.innerHTML.includes('演示数据'),
    activeTab: text(document.querySelector('.workspace-tabs a.active')),
    overflow: document.documentElement.scrollWidth - document.documentElement.clientWidth,
  };
}

// ---- 1 + 2: width matrix and screenshots ----
for (const vp of WIDTHS) {
  await page.setViewportSize({ width: vp.width, height: vp.height });
  for (const view of VIEWS) {
    const resp = await page.goto(url(view.name), { waitUntil: 'networkidle', timeout: 30000 });
    const m = await page.evaluate(probe);
    const shot = `${OUT}/${view.name}-${vp.name}.png`;
    await page.screenshot({ path: shot, fullPage: true });
    report.shots.push({ view: view.name, width: vp.name, path: shot });

    // The list is wider than its panel once the merged column set meets the
    // app's own "EUR 1234.56" format, exactly like the prototype's
    // `.table-wrap{overflow:auto}`. What must hold is that nothing is
    // unreachable: scroll the wrapper to its end and the row's action link has
    // to be inside the viewport.
    const reach = await page.evaluate(async () => {
      const wrap = document.querySelector('.workspace-tree-list, .table-wrap');
      if (!wrap) return { scroll: 0, reachable: false, reason: 'no list wrapper' };
      const scroll = wrap.scrollWidth - wrap.clientWidth;
      wrap.scrollLeft = wrap.scrollWidth;
      await new Promise((r) => requestAnimationFrame(r));
      const link = wrap.querySelector('tbody tr:last-child td:last-child a, .workspace-tree-summary .workspace-row-action a, .workspace-obligation-row .workspace-row-action a');
      const box = link?.getBoundingClientRect();
      return { scroll, reachable: !!box && box.right <= window.innerWidth + 1 && box.left >= 0, right: Math.round(box?.right ?? -1) };
    });
    if (!reach.reachable) fail(`${view.name} @ ${vp.name}: the row action column is unreachable: ${JSON.stringify(reach)}`);

    report.widths.push({ view: view.name, width: vp.name, status: resp?.status() ?? null, overflow: m.overflow, overlap: m.overlap.length, listScroll: reach.scroll, actionReachable: reach.reachable });
    if (m.overflow > 0) fail(`${view.name} @ ${vp.name}: document overflow +${m.overflow}px`);
    if (m.overlap.length) fail(`${view.name} @ ${vp.name}: ${m.overlap.length} cell(s) paint over their neighbour: ${JSON.stringify(m.overlap)}`);
    if (vp.name === '1440') report.structure[view.name] = m;
  }
}

// ---- 2c: the queue row anatomy (01/02 badge + the desktop text-link control) ----
// The prototype prefixes every queued row with its ordinal and, on desktop,
// collapses the row control to a single borderless text link. Both are read off
// the 1440 capture so a regression in either shows up here, not only in review.
const queueAnatomy = report.structure.properties.queue;
const queueIndexes = queueAnatomy.items.map((item) => item.index);
if (queueIndexes.join(',') !== '01,02') {
  fail(`the queue rows are not numbered 01/02 in order: ${JSON.stringify(queueIndexes)}`);
}
for (const item of queueAnatomy.items) {
  if (item.visibleActions.length !== 1) {
    fail(`the desktop queue row shows ${item.visibleActions.length} action(s), want the prototype's single text link: ${JSON.stringify(item.actions)}`);
  }
  const chrome = item.lastActionChrome;
  if (chrome && (chrome.border !== '0px' || !/rgba\(0, 0, 0, 0\)|transparent/.test(chrome.background))) {
    fail(`the desktop queue action is not a borderless text link: ${JSON.stringify(chrome)}`);
  }
}

// ---- 2b: the tree with its rows expanded ----
// A tall viewport instead of fullPage: the shell's sticky sidebar and topbar
// would otherwise be re-laid-out by the stitched capture and land mid-page.
await page.setViewportSize({ width: 1440, height: 1500 });
await page.goto(url('properties'), { waitUntil: 'networkidle' });
await page.locator('details.workspace-property > summary').first().click();
await page.locator('details.workspace-room-tree > summary').first().click();
await page.waitForTimeout(150);
report.expanded = await page.evaluate(probe);
report.expanded.shot = `${OUT}/properties-expanded-1440.png`;
await page.screenshot({ path: report.expanded.shot });
report.shots.push({ view: 'properties-expanded', width: '1440', path: report.expanded.shot });
if (report.expanded.overlap.length) {
  fail(`properties (expanded) @ 1440: ${report.expanded.overlap.length} cell(s) paint over their neighbour: ${JSON.stringify(report.expanded.overlap)}`);
}
if (report.expanded.indent.level2Pad !== '30px') {
  fail(`the nested obligation list is not indented to the second level: ${report.expanded.indent.level2Pad}`);
}

// ---- 3a: keyboard expansion + aria state ----
await page.setViewportSize({ width: 1440, height: 900 });
await page.goto(url('properties'), { waitUntil: 'networkidle' });
const cdp = await ctx.newCDPSession(page);
const axDoc = await cdp.send('DOM.getDocument', { depth: -1 });
const axNode = (await cdp.send('DOM.querySelector', {
  nodeId: axDoc.root.nodeId,
  selector: 'details.workspace-property',
})).nodeId;
// Read the state the browser actually exposes to assistive tech, not the
// attribute we wrote: `<summary>` must surface as a DisclosureTriangle whose
// `expanded` property tracks details.open.
const axState = async () => {
  const { nodes } = await cdp.send('Accessibility.getPartialAXTree', { nodeId: axNode, fetchRelatives: true });
  const sum = nodes.find((n) => n.role?.value === 'DisclosureTriangle');
  return sum
    ? {
      role: sum.role.value,
      expanded: sum.properties?.find((p) => p.name === 'expanded')?.value?.value,
      focusable: sum.properties?.find((p) => p.name === 'focusable')?.value?.value,
    }
    : null;
};
const openState = () => page.evaluate(() => document.querySelector('details.workspace-property').open);
await page.locator('details.workspace-property > summary').first().focus();
const focusedTag = await page.evaluate(() => document.activeElement?.tagName);
const beforeOpen = await openState();
const axBefore = await axState();
await page.keyboard.press('Enter');
const afterEnter = await openState();
const axAfterEnter = await axState();
await page.keyboard.press('Space');
const afterSpace = await openState();
const axAfterSpace = await axState();
report.keyboard = { focusedTag, beforeOpen, afterEnter, afterSpace, axBefore, axAfterEnter, axAfterSpace };
if (!(beforeOpen === false && afterEnter === true && afterSpace === false)) {
  fail(`keyboard expansion: open sequence was ${beforeOpen} -> ${afterEnter} -> ${afterSpace}`);
}
if (focusedTag !== 'SUMMARY') fail(`keyboard expansion: focus was on ${focusedTag}, not SUMMARY`);
if (report.keyboard.axBefore?.role !== 'DisclosureTriangle') {
  fail(`the summary is not exposed as a disclosure control: ${JSON.stringify(report.keyboard.axBefore)}`);
}
if (axAfterEnter?.expanded !== true || axAfterSpace?.expanded !== false) {
  fail(`the accessibility tree did not track the expanded state: ${JSON.stringify(report.keyboard)}`);
}

// The three indent levels are the tree's whole visual hierarchy, and an
// element-level `padding` shorthand silently outranks them, so guard them.
report.indent = {};
for (const [view, pad1, pad2] of [['properties', '22px', '30px'], ['rooms', null, '30px']]) {
  await page.goto(url(view), { waitUntil: 'networkidle' });
  const got = await page.evaluate(() => {
    const l1 = document.querySelector('.tree-level-1');
    const l2 = document.querySelector('.tree-level-2');
    return {
      l1: l1 ? getComputedStyle(l1).paddingLeft : null,
      l2: l2 ? getComputedStyle(l2).paddingLeft : null,
    };
  });
  report.indent[view] = got;
  if (got.l1 !== pad1 || got.l2 !== pad2) {
    fail(`${view}: indent levels rendered as ${JSON.stringify(got)}, expected {l1:${pad1}, l2:${pad2}}`);
  }
}

// ---- 3b: the settle form on a tenant row opens inline, not as a jump ----
await page.goto(url('tenants'), { waitUntil: 'networkidle' });
const settle = page.locator('.workspace-table tbody .collection-settle').first();
report.settleRow = {
  count: await page.locator('.workspace-table tbody .collection-settle').count(),
  action: await page.locator('.workspace-table tbody .collection-balance-form').first().getAttribute('action'),
  labels: await page.locator('.workspace-table tbody .collection-settle > summary').first().textContent(),
};
await settle.locator('> summary').click();
report.settleRow.formVisibleAfterOpen = await settle.evaluate((el) => {
  const form = el.querySelector('.collection-balance-form');
  const rect = form.getBoundingClientRect();
  return el.open && rect.width > 0 && rect.height > 0;
});
if (!report.settleRow.formVisibleAfterOpen) fail('the row settle form did not become visible when opened');

await browser.close();
fs.writeFileSync(`${OUT}/report.json`, JSON.stringify(report, null, 2));

console.log(JSON.stringify(report, null, 2));
console.log(`\nscreenshots: ${report.shots.length} -> ${OUT}`);
if (report.failures.length) {
  console.error(`\n${report.failures.length} failure(s):`);
  for (const f of report.failures) console.error(`  ${f}`);
  process.exit(1);
}
console.log('dashboard alignment audit passed');
