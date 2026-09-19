// Independent layout + interaction re-verification (check agent).
import { chromium } from 'playwright';
import { launchOptions } from './launch.mjs';

const BASE = (process.env.AUDIT_BASE || 'http://127.0.0.1:18092').replace(/\/+$/, '');
const PERIOD = process.env.AUDIT_PERIOD || '2026-09';

const browser = await chromium.launch(launchOptions());
const ctx = await browser.newContext({ viewport: { width: 1440, height: 900 }, permissions: ['clipboard-read', 'clipboard-write'] });
const page = await ctx.newPage();
await page.goto(BASE + '/', { waitUntil: 'domcontentloaded' });
await page.fill('#username', process.env.AUDIT_USER);
await page.fill('#password', process.env.AUDIT_PASS);
await Promise.all([page.waitForNavigation({ waitUntil: 'domcontentloaded' }).catch(() => {}), page.click('button[type=submit]')]);

const tableInfo = () => Array.from(document.querySelectorAll('table')).map((t) => {
  const wrap = t.closest('.table-wrap') || t.parentElement;
  const cols = Array.from(t.querySelectorAll('thead th')).map((th) => ({ t: th.textContent.trim(), w: Math.round(th.getBoundingClientRect().width) }));
  let worstWrap = 0, worstText = '';
  for (const td of Array.from(t.querySelectorAll('tbody td'))) {
    const r = td.getBoundingClientRect();
    const fs = parseFloat(getComputedStyle(td).fontSize) || 14;
    const lines = Math.round(r.height / (fs * 1.5));
    if (r.width > 0 && lines > worstWrap) { worstWrap = lines; worstText = td.textContent.trim().slice(0, 24); }
  }
  return {
    cls: (t.className || '(none)'),
    wrapScroll: wrap ? wrap.scrollWidth : null, wrapClient: wrap ? wrap.clientWidth : null,
    cols, maxCellLines: worstWrap, maxCellSample: worstText,
  };
});

const out = {};
for (const w of [1024, 1440]) {
  await page.setViewportSize({ width: w, height: 900 });
  for (const [name, url] of [
    ['tenant-detail', `/tenants/3`],
    ['room-detail', `/rooms/5?period=${PERIOD}`],
    ['property-detail', `/properties/1?period=${PERIOD}`],
  ]) {
    await page.goto(BASE + url, { waitUntil: 'networkidle' });
    out[`${name}@${w}`] = await page.evaluate(tableInfo);
  }
}

// copy button interaction
await page.setViewportSize({ width: 1440, height: 900 });
await page.goto(BASE + '/tenants/3', { waitUntil: 'networkidle' });
const btn = 'button.copy-reference';
const pre = await page.evaluate((s) => document.querySelector(s)?.getAttribute('data-copy-value'), btn);
await page.click(btn);
const after = await page.evaluate((s) => document.querySelector(s)?.textContent.trim(), btn);
const clip = await page.evaluate(async () => { try { return await navigator.clipboard.readText(); } catch (e) { return 'ERR:' + e.message; } });
out.copy = { dataValue: pre, buttonTextAfterClick: after, clipboard: clip };

// live: submit 标记非租金 from the detail header, confirm it lands on the list
try {
  const targetTx = process.env.AUDIT_IGNORE_TX || '7';
  await page.goto(BASE + `/transactions?detail=${targetTx}`, { waitUntil: 'networkidle' });
  const hasIgnore = await page.evaluate(() => !!document.querySelector('.transaction-detail-actions form[action$="/ignore"]'));
  out.ignoreFormPresent = hasIgnore;
  if (hasIgnore) {
    // open the <details> that owns the ignore form, then submit
    await page.evaluate(() => {
      const f = document.querySelector('.transaction-detail-actions form[action$="/ignore"]');
      f.closest('details').open = true;
      f.querySelector('input[name=reason]').value = 'check-agent probe';
    });
    await Promise.all([
      page.waitForNavigation({ waitUntil: 'domcontentloaded', timeout: 15000 }).catch(() => {}),
      page.click('.transaction-detail-actions form[action$="/ignore"] button[type=submit]'),
    ]);
    out.ignoreLanded = page.url();
  }
} catch (e) {
  out.ignoreError = String(e).slice(0, 200);
}

await browser.close();
console.log(JSON.stringify(out, null, 1));
