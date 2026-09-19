// Independent re-verification for 09-19-detail-pages-alignment (check agent).
// Deliberately does NOT reuse detail-pages.mjs; re-derives every AC from the DOM.
import { chromium } from 'playwright';
import { launchOptions } from './launch.mjs';

const BASE = (process.env.AUDIT_BASE || 'http://127.0.0.1:18092').replace(/\/+$/, '');
const PERIOD = process.env.AUDIT_PERIOD || '2026-09';
const ROOM_A = process.env.AUDIT_ROOM_ID || 5;
const ROOM_B = process.env.AUDIT_ROOM_ZERO_ID || 6;
const PROP = process.env.AUDIT_PROPERTY_ID || 1;
const TENANT = process.env.AUDIT_TENANT_ID || 3;

const out = { base: BASE, aps: {}, notes: [] };
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

const geom = () => ({
  clientW: document.documentElement.clientWidth,
  scrollW: document.documentElement.scrollWidth,
  bodyScrollW: document.body.scrollWidth,
});

// ---- AC11: no document-level horizontal overflow, 4 pages x 4 widths ----
const pages = [
  ['property-detail', `/properties/${PROP}?period=${PERIOD}`],
  ['room-detail', `/rooms/${ROOM_A}?period=${PERIOD}`],
  ['tenant-detail', `/tenants/${TENANT}`],
  ['transaction-detail', `/transactions?detail=1`],
];
out.overflow = [];
for (const w of [1024, 1366, 1440, 1920]) {
  await page.setViewportSize({ width: w, height: 900 });
  for (const [name, url] of pages) {
    const resp = await page.goto(BASE + url, { waitUntil: 'networkidle' });
    const g = await page.evaluate(geom);
    const ov = g.scrollW - g.clientW;
    out.overflow.push({ w, name, status: resp?.status(), ov });
  }
}

// ---- AC2: room-detail desktop vs mobile facts ----
await page.setViewportSize({ width: 1440, height: 900 });
await page.goto(BASE + `/rooms/${ROOM_A}?period=${PERIOD}`, { waitUntil: 'networkidle' });
out.aps.ac2_desktop = await page.evaluate(() => {
  const vis = (s) => { const e = document.querySelector(s); if (!e) return null; const r = e.getBoundingClientRect(); return r.width > 0 && r.height > 0; };
  const mobilePanel = document.querySelector('.room-mobile-facts');
  const desktopPanel = document.querySelector('.room-desktop-facts');
  return {
    desktopFactsVisible: vis('.room-desktop-facts'),
    mobileFactsVisible: vis('.room-mobile-facts'),
    // a mobile-only fact that must NOT be visible on desktop
    mobilePanelHasBillingDayVisible: mobilePanel ? vis('.room-mobile-facts') && mobilePanel.querySelector('dd')?.textContent.trim() !== null : null,
    desktopPanelSectionClass: desktopPanel?.className ?? null,
    mobilePanelSectionClass: mobilePanel?.className ?? null,
    countOfBillingDayFacts: Array.from(document.querySelectorAll('.room-facts dt')).filter((d) => d.textContent.includes('账单日')).length,
  };
});
await page.setViewportSize({ width: 390, height: 844 });
await page.goto(BASE + `/rooms/${ROOM_A}?period=${PERIOD}`, { waitUntil: 'networkidle' });
out.aps.ac2_mobile390 = await page.evaluate(() => {
  const vis = (s) => { const e = document.querySelector(s); if (!e) return null; const r = e.getBoundingClientRect(); return r.width > 0 && r.height > 0; };
  return {
    overflow: document.documentElement.scrollWidth - document.documentElement.clientWidth,
    desktopFactsVisible: vis('.room-desktop-facts'),
    mobileFactsVisible: vis('.room-mobile-facts'),
    billingDayVisible: Array.from(document.querySelectorAll('.room-facts dt')).some((d) => d.textContent.includes('账单日') && d.getBoundingClientRect().height > 0),
  };
});

// ---- AC3: 未分配 vs 未覆盖, two rooms ----
const allocProbe = () => ({
  cells: Array.from(document.querySelectorAll('.room-allocation-metrics article')).map((a) => ({
    label: a.querySelector('span')?.textContent.trim() || '',
    value: a.querySelector('strong')?.textContent.trim() || '',
  })),
  kpi: Array.from(document.querySelectorAll('.detail-summary article, .room-detail-metrics article, .metric')).map((a) => ({
    l: a.querySelector('.label, .metric-label')?.textContent.trim() || a.querySelector('span')?.textContent.trim() || '',
    v: a.querySelector('strong')?.textContent.trim() || '',
  })),
});
for (const [label, id] of [['roomA', ROOM_A], ['roomB', ROOM_B]]) {
  await page.setViewportSize({ width: 1440, height: 900 });
  const resp = await page.goto(BASE + `/rooms/${id}?period=${PERIOD}`, { waitUntil: 'networkidle' });
  out.aps['ac3_' + label] = { status: resp?.status(), ...(await page.evaluate(allocProbe)) };
}

// ---- AC4/5/6: tenant-detail ----
await page.goto(BASE + `/tenants/${TENANT}`, { waitUntil: 'networkidle' });
out.aps.ac4_5_6 = await page.evaluate(() => {
  const s = document.querySelector('select[name=range]');
  return {
    range: s ? { value: s.value, options: Array.from(s.options).map((o) => o.textContent.trim()) } : null,
    reference: document.querySelector('.reference-row strong')?.textContent.trim() ?? null,
    copyValue: document.querySelector('.copy-reference')?.getAttribute('data-copy-value') ?? null,
    paidForHeaders: Array.from(document.querySelectorAll('.paid-for-table thead th')).map((t) => t.textContent.trim()),
    paidForRows: Array.from(document.querySelectorAll('.paid-for-table tbody tr')).map((tr) => Array.from(tr.querySelectorAll('td')).map((td) => td.textContent.trim().replace(/\s+/g, ' '))),
    paidForEmpty: !!document.querySelector('.tenant-paid-for-panel .empty'),
    hasFromMonth: !!document.querySelector('input[name=from_month]'),
  };
});

// ---- AC7/8/9: transaction-detail ----
await page.goto(BASE + '/transactions?detail=1', { waitUntil: 'networkidle' });
out.aps.ac7_8_9 = await page.evaluate(() => ({
  codeBlockWS: (() => { const e = document.querySelector('.code-block'); return e ? getComputedStyle(e).whiteSpace : null; })(),
  codeBlockNL: (document.querySelector('.code-block')?.textContent || '').includes('\n'),
  facts: Array.from(document.querySelectorAll('.transaction-facts div')).map((d) => d.querySelector('dt')?.textContent.trim()),
  txTime: (() => { const d = Array.from(document.querySelectorAll('.transaction-facts div')).find((x) => x.querySelector('dt')?.textContent.trim() === '交易时间'); return d?.querySelector('dd')?.textContent.trim() ?? null; })(),
  headLabels: Array.from(document.querySelectorAll('.transaction-detail-actions > details > summary')).map((s) => s.textContent.trim()),
  headForms: Array.from(document.querySelectorAll('.transaction-detail-actions form')).map((f) => ({
    action: f.getAttribute('action'),
    fields: Array.from(f.querySelectorAll('input[name],select[name]')).map((i) => i.name).sort(),
  })),
}));

// ---- AC9 parity: same endpoints/fields as the list row for the same tx ----
await page.goto(BASE + '/transactions', { waitUntil: 'networkidle' });
out.aps.ac9_listRow = await page.evaluate(() => {
  const row = document.querySelector('#transaction-row-1') || document.querySelector('.billing-table-wrap tbody tr');
  const forms = Array.from(document.querySelectorAll('.billing-table-wrap tbody tr form'));
  return {
    detailLink: row?.querySelector('.transaction-detail-link')?.getAttribute('href') ?? null,
    forms: forms.map((f) => ({
      action: f.getAttribute('action'),
      fields: Array.from(f.querySelectorAll('input[name],select[name]')).map((i) => i.name).sort(),
    })),
  };
});

// ---- property-detail at 390: did the nth-child(2) -> class change reveal a panel? ----
await page.setViewportSize({ width: 390, height: 844 });
await page.goto(BASE + `/properties/${PROP}?period=${PERIOD}`, { waitUntil: 'networkidle' });
out.aps.property_390 = await page.evaluate(() => {
  const vis = (s) => { const e = document.querySelector(s); if (!e) return null; const r = e.getBoundingClientRect(); return r.width > 0 && r.height > 0; };
  return {
    overflow: document.documentElement.scrollWidth - document.documentElement.clientWidth,
    mobileFactsVisible: vis('.property-mobile-facts'),
    expenseListVisible: vis('.property-expense-list'),
    desktopFactsVisible: vis('.property-desktop-facts'),
    financialBridgeVisible: vis('.property-financial-bridge'),
  };
});

await browser.close();
console.log(JSON.stringify(out, null, 2));
