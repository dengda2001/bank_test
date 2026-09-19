// Detail-page alignment verification for 09-19-detail-pages-alignment.
//
// Four detail pages x four desktop widths (1024 / 1366 / 1440 / 1920), plus
// room-detail at a phone width. Asserts document-level horizontal overflow is
// zero, screenshots each combination, and collects the DOM evidence the task's
// acceptance criteria need (未分配 cell values, room mobile-only fields,
// transaction 原始描述 / 交易时间, and the design.md §2.5 room observations).
//
// Usage:
//   AUDIT_BASE=http://127.0.0.1:18092 AUDIT_USER=... AUDIT_PASS=... \
//     AUDIT_OUT=<task>/research/screenshots node scripts/audit/detail-pages.mjs
import { chromium } from 'playwright';
import { launchOptions } from './launch.mjs';
import fs from 'node:fs';

const BASE = (process.env.AUDIT_BASE || 'http://127.0.0.1:18092').replace(/\/+$/, '');
const OUT = process.env.AUDIT_OUT || '/tmp/detail-pages';
fs.mkdirSync(OUT, { recursive: true });

const WIDTHS = [
  { name: '1024', width: 1024, height: 768 },
  { name: '1366', width: 1366, height: 768 },
  { name: '1440', width: 1440, height: 900 },
  { name: '1920', width: 1920, height: 1080 },
];

const PERIOD = process.env.AUDIT_PERIOD || '2026-09';

const PAGES = [
  { name: 'property-detail', url: `/properties/${process.env.AUDIT_PROPERTY_ID || 1}?period=${PERIOD}` },
  { name: 'room-detail', url: `/rooms/${process.env.AUDIT_ROOM_ID || 5}?period=${PERIOD}` },
  { name: 'tenant-detail', url: `/tenants/${process.env.AUDIT_TENANT_ID || 3}` },
  { name: 'transaction-detail', url: `/transactions?detail=${process.env.AUDIT_TX_ID || 1}` },
];

function probe() {
  const vw = document.documentElement.clientWidth;
  const lim = vw + 1;
  const offenders = [];
  for (const el of document.querySelectorAll('body *')) {
    const r = el.getBoundingClientRect();
    if (r.width === 0 || r.height === 0) continue;
    if (r.right <= lim) continue;
    const pr = el.parentElement?.getBoundingClientRect();
    if (pr && pr.right > lim) continue;
    const cls = typeof el.className === 'string' && el.className.trim()
      ? '.' + el.className.trim().split(/\s+/).join('.')
      : '';
    offenders.push({ sel: el.tagName.toLowerCase() + cls, right: Math.round(r.right), width: Math.round(r.width) });
  }
  const visible = (sel) => {
    const el = document.querySelector(sel);
    if (!el) return null;
    const r = el.getBoundingClientRect();
    return r.width > 0 && r.height > 0;
  };
  return {
    vw,
    clientW: document.documentElement.clientWidth,
    scrollW: document.documentElement.scrollWidth,
    // Block structure: every panel heading in DOM order, plus whether it sits in
    // the sidebar and whether it actually renders at this width, so the page's
    // block list can be diffed against the prototype without counting the
    // mobile-only variants that are display:none on desktop.
    blocks: Array.from(document.querySelectorAll('section.panel, details.panel')).map((el) => {
      const head = el.querySelector('.panel-head h2, .panel-head h3, summary');
      const r = el.getBoundingClientRect();
      return {
        title: head ? head.textContent.trim() : '',
        sidebar: !!el.closest('aside'),
        visible: r.width > 0 && r.height > 0,
      };
    }),
    metricCards: Array.from(document.querySelectorAll('.detail-summary .metric, .metric')).map((el) => {
      const r = el.getBoundingClientRect();
      const label = el.querySelector('.label, .metric-label');
      return { label: label ? label.textContent.trim() : '', visible: r.width > 0 && r.height > 0 };
    }),
    offenders: offenders.slice(0, 8),
    // room-detail responsive evidence
    desktopFactsVisible: visible('.room-desktop-facts'),
    mobileFactsVisible: visible('.room-mobile-facts'),
    mobileActionsVisible: visible('.room-detail-mobile-actions'),
    // 未分配 / 未覆盖 cells
    allocationCells: Array.from(document.querySelectorAll('.room-allocation-metrics article')).map((a) => ({
      label: a.querySelector('span')?.textContent?.trim() || '',
      value: a.querySelector('strong')?.textContent?.trim() || '',
    })),
    kpis: Array.from(document.querySelectorAll('.detail-summary article')).map((a) => ({
      label: a.querySelector('.label')?.textContent?.trim(),
      value: a.querySelector('strong')?.textContent?.trim(),
      note: a.querySelector('span')?.textContent?.trim(),
    })),
    responsibilityRows: Array.from(document.querySelectorAll('.room-responsibility-section tbody tr')).map((tr) =>
      Array.from(tr.querySelectorAll('td')).map((td) => td.textContent.trim().replace(/\s+/g, ' '))
    ),
    paymentRows: Array.from(document.querySelectorAll('.room-payments-section tbody tr')).map((tr) =>
      Array.from(tr.querySelectorAll('td')).map((td) => td.textContent.trim().replace(/\s+/g, ' '))
    ),
    paymentCountMetric: document.querySelector('.detail-summary article:nth-child(4) strong')?.textContent?.trim() ?? null,
    // transaction-detail evidence
    descriptionText: document.querySelector('.transaction-detail-page .code-block')?.textContent ?? null,
    descriptionWhiteSpace: (() => {
      const el = document.querySelector('.transaction-detail-page .code-block');
      return el ? getComputedStyle(el).whiteSpace : null;
    })(),
    factLabels: Array.from(document.querySelectorAll('.transaction-facts dt')).map((d) => d.textContent.trim()),
    transactionTime: (() => {
      const dts = Array.from(document.querySelectorAll('.transaction-facts div'));
      const row = dts.find((d) => d.querySelector('dt')?.textContent?.trim() === '交易时间');
      return row ? row.querySelector('dd')?.textContent?.trim() : null;
    })(),
    actionLabels: Array.from(document.querySelectorAll('.transaction-detail-actions > details > summary')).map((s) => s.textContent.trim()),
    actionForms: Array.from(document.querySelectorAll('.transaction-detail-actions form')).map((f) => ({
      action: f.getAttribute('action'),
      hasReturnTo: !!f.querySelector('input[name=return_to]'),
    })),
    // tenant-detail evidence
    rangeSelect: (() => {
      const s = document.querySelector('select[name=range]');
      return s ? { value: s.value, options: Array.from(s.options).map((o) => o.textContent.trim()) } : null;
    })(),
    referenceText: document.querySelector('.reference-row strong')?.textContent?.trim() ?? null,
    copyButtonValue: document.querySelector('.copy-reference')?.getAttribute('data-copy-value') ?? null,
    paidForHeaders: Array.from(document.querySelectorAll('.paid-for-table thead th')).map((t) => t.textContent.trim()),
    paidForRows: Array.from(document.querySelectorAll('.paid-for-table tbody tr')).map((tr) =>
      Array.from(tr.querySelectorAll('td')).map((td) => td.textContent.trim().replace(/\s+/g, ' '))
    ),
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

const report = { base: BASE, period: PERIOD, pages: [], probes: {}, failures: [] };

for (const vp of WIDTHS) {
  await page.setViewportSize({ width: vp.width, height: vp.height });
  console.log(`\n===== ${vp.name} =====`);
  for (const p of PAGES) {
    const resp = await page.goto(BASE + p.url, { waitUntil: 'networkidle', timeout: 30000 });
    const m = await page.evaluate(probe);
    const overflow = m.scrollW - m.clientW;
    const ok = overflow <= 0;
    report.pages.push({ page: p.name, url: p.url, width: vp.name, status: resp?.status() ?? null, overflow, ok, offenders: m.offenders });
    if (!ok) report.failures.push({ page: p.name, width: vp.name, overflow, offenders: m.offenders });
    if (resp?.status() !== 200) report.failures.push({ page: p.name, width: vp.name, status: resp?.status() });
    console.log(`${ok ? 'ok  ' : 'FAIL'} ${p.name.padEnd(20)} http=${resp?.status()} client=${m.clientW} scroll=${m.scrollW} overflow=${overflow}`);
    for (const o of m.offenders) console.log(`        offender right=${o.right} w=${o.width} ${o.sel}`);
    // Keep the full DOM evidence for the widest desktop run only.
    if (vp.name === '1440') report.probes[p.name] = m;
    await page.screenshot({ path: `${OUT}/${p.name}-${vp.name}.png`, fullPage: true });
  }
}

// room-detail at a phone width: the mobile-only fields must come back.
await page.setViewportSize({ width: 390, height: 844 });{
  const resp = await page.goto(BASE + PAGES[1].url, { waitUntil: 'networkidle', timeout: 30000 });
  const m = await page.evaluate(probe);
  const overflow = m.scrollW - m.clientW;
  report.pages.push({ page: 'room-detail', url: PAGES[1].url, width: '390', status: resp?.status() ?? null, overflow, ok: overflow <= 0 });
  if (overflow > 0) report.failures.push({ page: 'room-detail', width: '390', overflow });
  report.probes['room-detail@390'] = m;
  console.log(`\n===== 390 (phone) =====`);
  console.log(`${overflow <= 0 ? 'ok  ' : 'FAIL'} room-detail          http=${resp?.status()} client=${m.clientW} scroll=${m.scrollW} overflow=${overflow}`);
  console.log(`        desktopFacts=${m.desktopFactsVisible} mobileFacts=${m.mobileFactsVisible}`);
  await page.screenshot({ path: `${OUT}/room-detail-390.png`, fullPage: true });
}

// A second room whose payments are fully allocated: the 未分配 cell must read a
// real zero there, which is a different state from the room above.
await page.setViewportSize({ width: 1440, height: 900 });
{
  const zeroRoomURL = `/rooms/${process.env.AUDIT_ROOM_ZERO_ID || 6}?period=${PERIOD}`;
  const resp = await page.goto(BASE + zeroRoomURL, { waitUntil: 'networkidle', timeout: 30000 });
  const m = await page.evaluate(probe);
  report.probes['room-detail-fully-allocated'] = m;
  console.log(`\n===== room-detail (no unallocated balance) =====`);
  console.log(`http=${resp?.status()} unallocated=${JSON.stringify(m.allocationCells.filter((c) => c.label === '未分配'))}`);
  await page.screenshot({ path: `${OUT}/room-detail-fully-allocated-1440.png`, fullPage: true });
}

fs.writeFileSync(`${OUT}/report.json`, JSON.stringify(report, null, 2));
await browser.close();

console.log(`\n===== DOM evidence =====`);
console.log(JSON.stringify(report.probes, null, 2));
console.log(`\nwrote ${OUT}`);
if (report.failures.length) {
  console.error(`\n${report.failures.length} failure(s)`);
  process.exit(1);
}
console.log('no document-level horizontal overflow on any audited detail page');
