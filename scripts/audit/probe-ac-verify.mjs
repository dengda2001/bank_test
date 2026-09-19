// Second independent pass: walks the subtask's acceptance criteria against the
// live instance and reports what it observed for each.
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

const url = (view) => `${BASE}/rent-dashboard?period=${PERIOD}&view=${view}`;
const browser = await chromium.launch(launchOptions());
const ctx = await browser.newContext({ viewport: { width: 1440, height: 900 } });
await ctx.addCookies(readCookies(JAR));
const page = await ctx.newPage();
const out = {};

for (const view of ['properties', 'rooms', 'tenants']) {
  await page.goto(url(view), { waitUntil: 'networkidle' });
  out[view] = await page.evaluate(() => {
    const t = (el) => (el?.textContent || '').replace(/\s+/g, ' ').trim();
    const controls = document.querySelector('.workspace-list-controls');
    const tabs = document.querySelector('.workspace-tabs');
    const filters = document.querySelector('.workspace-filters');
    const tr = tabs?.getBoundingClientRect();
    const fr = filters?.getBoundingClientRect();
    const desktop = document.querySelector('.workspace-desktop-list');
    const headers = desktop
      ? Array.from(desktop.querySelectorAll('.workspace-tree-head > span, thead th')).map(t)
      : [];
    const metrics = Array.from(document.querySelectorAll('.workspace-summary .metric')).map((m) => ({
      label: t(m.querySelector('.label')), borderTop: getComputedStyle(m).borderTopWidth, cls: m.className,
    }));
    const queue = {
      columns: getComputedStyle(document.querySelector('.workspace-queue-list') || document.body).gridTemplateColumns,
      perItemButtons: Array.from(document.querySelectorAll('.workspace-queue-item')).map((i) => Array.from(i.querySelectorAll('.workspace-queue-actions a')).map(t)),
      headMore: t(document.querySelector('.workspace-queue-head-meta .workspace-queue-more')),
    };
    const detailLinks = Array.from(document.querySelectorAll('.workspace-row-action a.btn.subtle, .workspace-table tbody td:last-child a.btn.subtle'))
      .map((a) => ({ text: t(a), href: a.getAttribute('href') }));
    const settle = Array.from(document.querySelectorAll('.collection-settle')).map((s) => ({
      summary: t(s.querySelector('summary')),
      action: s.querySelector('form')?.getAttribute('action'),
      template: s.querySelector('.collection-balance-form') ? 'collection-balance-form' : '?',
    }));
    return {
      tabsInsidePanel: !!document.querySelector('.workspace-list-panel')?.contains(tabs),
      controlsRow: tr && fr ? Math.abs(tr.top - fr.top) < Math.max(tr.height, fr.height) : null,
      tabsTop: tr ? Math.round(tr.top) : null, filtersTop: fr ? Math.round(fr.top) : null,
      sectionTitle: t(document.querySelector('.workspace-dimension-head h2')),
      sectionDesc: t(document.querySelector('.workspace-dimension-head p')),
      currencyNote: t(document.querySelector('.workspace-currency-note')),
      hasDemoLabel: document.body.innerHTML.includes('演示数据'),
      headers, metrics, queue, detailLinks, settle,
      rateBars: document.querySelectorAll('.workspace-progress > span').length,
    };
  });
}

// Keyboard expansion + aria state in the accessibility tree.
await page.setViewportSize({ width: 1440, height: 900 });
await page.goto(url('properties'), { waitUntil: 'networkidle' });
const cdp = await ctx.newCDPSession(page);
const doc = await cdp.send('DOM.getDocument', { depth: -1 });
const node = (await cdp.send('DOM.querySelector', { nodeId: doc.root.nodeId, selector: 'details.workspace-property' })).nodeId;
const axState = async () => {
  const { nodes } = await cdp.send('Accessibility.getPartialAXTree', { nodeId: node, fetchRelatives: true });
  const s = nodes.find((n) => n.role?.value === 'DisclosureTriangle');
  return s ? { role: s.role.value, expanded: s.properties?.find((p) => p.name === 'expanded')?.value?.value, focusable: s.properties?.find((p) => p.name === 'focusable')?.value?.value } : null;
};
const open = () => page.evaluate(() => document.querySelector('details.workspace-property').open);
await page.locator('details.workspace-property > summary').first().focus();
out.keyboard = { focused: await page.evaluate(() => document.activeElement?.tagName), before: await open(), axBefore: await axState() };
await page.keyboard.press('Enter'); out.keyboard.afterEnter = await open(); out.keyboard.axAfterEnter = await axState();
await page.keyboard.press('Space'); out.keyboard.afterSpace = await open(); out.keyboard.axAfterSpace = await axState();

// Detail links must actually resolve.
const hrefs = new Set();
for (const view of ['properties', 'rooms', 'tenants']) {
  await page.goto(url(view), { waitUntil: 'networkidle' });
  (await page.evaluate(() => Array.from(document.querySelectorAll('.workspace-row-action a.btn.subtle, .workspace-table tbody td:last-child a.btn.subtle')).map((a) => a.href))).forEach((h) => hrefs.add(h));
}
out.detailStatus = {};
for (const h of hrefs) {
  const r = await page.request.get(h);
  out.detailStatus[h.replace(BASE, '')] = r.status();
}

// <=640 with every tree node expanded: does the added row-action markup clip?
for (const view of ['properties', 'rooms', 'tenants']) {
  await page.setViewportSize({ width: 390, height: 900 });
  await page.goto(url(view), { waitUntil: 'networkidle' });
  const roots = await page.locator('details.workspace-tree-mobile, details.workspace-tree-item').count().catch(() => 0);
  for (let i = 0; i < Math.min(roots, 8); i++) {
    await page.locator('details.workspace-tree-mobile > summary, details.workspace-tree-item > summary').nth(i).click({ timeout: 3000 }).catch(() => {});
  }
  await page.waitForTimeout(200);
  out[`mobile390expanded_${view}`] = await page.evaluate(() => {
    const doc = document.documentElement;
    const over = [];
    for (const el of document.querySelectorAll('*')) {
      const r = el.getBoundingClientRect();
      if (r.width < 1) continue;
      if (r.right - doc.clientWidth > 1 && getComputedStyle(el).overflowX === 'visible') {
        over.push({ tag: el.tagName, cls: (el.className || '').toString().slice(0, 40), over: Math.round(r.right - doc.clientWidth) });
      }
    }
    over.sort((a, b) => b.over - a.over);
    return { docOverflow: doc.scrollWidth - doc.clientWidth, worst: over.slice(0, 5) };
  });
}

await browser.close();
console.log(JSON.stringify(out, null, 2));
