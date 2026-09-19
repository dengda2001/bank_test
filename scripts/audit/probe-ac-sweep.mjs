// Remaining live-DOM facts for 09-19-list-pages-alignment:
//   AC 3  /tenants and /bank carry no month filter
//   AC 5  the settle form's prototype field set
//   AC 7  the settle form is rendered inline by the rent workspace too
//   AC 11 no isolated 自 marker on /rooms
//   AC 1  /dunning rows carry a detail link
// Usage: AUDIT_BASE=... AUDIT_USER=... AUDIT_PASS=... node scripts/audit/probe-ac-sweep.mjs
import { chromium } from 'playwright';
import { launchOptions } from './launch.mjs';

const BASE = (process.env.AUDIT_BASE || 'http://127.0.0.1:18094').replace(/\/+$/, '');
const USER = process.env.AUDIT_USER;
const PASS = process.env.AUDIT_PASS;

const browser = await chromium.launch(launchOptions());
const ctx = await browser.newContext({ viewport: { width: 1440, height: 900 } });
const page = await ctx.newPage();
await page.goto(BASE + '/', { waitUntil: 'domcontentloaded' });
await page.fill('#username', USER);
await page.fill('#password', PASS);
await Promise.all([
  page.waitForNavigation({ waitUntil: 'domcontentloaded' }).catch(() => {}),
  page.click('button[type=submit]'),
]);

// AC 3
for (const url of ['/tenants', '/bank']) {
  await page.goto(BASE + url, { waitUntil: 'networkidle' });
  const months = await page.evaluate(() => ({
    monthInputs: document.querySelectorAll('input[type=month]').length,
    periodControls: [...document.querySelectorAll('select[name=period], input[name=period]')].length,
    filterForms: document.querySelectorAll('form[class*=filter]').length,
  }));
  console.log(`AC3 ${url}: ${JSON.stringify(months)}`);
}

// AC 5
await page.goto(BASE + '/bills?period=2026-09', { waitUntil: 'networkidle' });
const settle = await page.evaluate(() => {
  const form = document.querySelector('form.collection-balance-form');
  if (!form) return { found: false };
  const field = (sel) => {
    const el = form.querySelector(sel);
    return el ? { name: el.name, value: el.value, readOnly: el.readOnly, tag: el.tagName.toLowerCase() } : null;
  };
  const facts = [...form.querySelectorAll('dt')].map((dt, i) => dt.textContent.trim());
  return {
    found: true,
    action: form.getAttribute('action'),
    method: form.getAttribute('method'),
    facts,
    disposition: field('select[name=disposition]'),
    dispositionOptions: [...form.querySelectorAll('select[name=disposition] option')].map((o) => `${o.value}:${o.textContent.trim()}${o.selected ? ' (selected)' : ''}`),
    amount: field('input[name=amount]'),
    effectiveDate: field('input[name=effective_date]'),
    reason: field('textarea[name=reason]'),
    hiddenReturn: [...form.querySelectorAll('input[type=hidden]')].map((i) => i.name),
    obligationId: form.querySelector('input[name=obligation_id]')?.value,
  };
});
console.log(`AC5 /bills settle form: ${JSON.stringify(settle, null, 1)}`);

// AC 7 -- the workspace's tenant view is the third carrier ③ renders the form from.
for (const view of ['properties', 'tenants']) {
  await page.goto(`${BASE}/rent-dashboard?period=2026-09&view=${view}`, { waitUntil: 'networkidle' });
  const reuse = await page.evaluate(() => {
    const forms = [...document.querySelectorAll('form.collection-balance-form')];
    return {
      settleForms: forms.length,
      actions: [...new Set(forms.map((f) => f.getAttribute('action')))],
      summaryLabels: [...document.querySelectorAll('details.collection-settle > summary')].map((s) => s.textContent.trim()),
      facts: [...document.querySelectorAll('.collection-balance-fact')].map((f) => f.textContent.trim().replace(/\s+/g, ' ')),
      dispositionOptions: forms[0] ? [...forms[0].querySelectorAll('select[name=disposition] option')].map((o) => o.textContent.trim()) : [],
      returnFields: forms[0] ? [...forms[0].querySelectorAll('input[type=hidden]')].map((i) => `${i.name}=${i.value}`) : [],
    };
  });
  console.log(`AC7 /rent-dashboard?view=${view}: ${JSON.stringify(reuse)}`);
}

// AC 11
await page.goto(BASE + '/rooms?period=2026-09&status=all', { waitUntil: 'networkidle' });
const rooms = await page.evaluate(() => {
  const body = document.body.innerHTML;
  const markers = [...body.matchAll(/<small>\s*自[^<]*<\/small>/g)].map((m) => m[0]);
  return { isolatedSelfMarkers: markers.filter((m) => !/自\s*\d/.test(m)), allSelfMarkers: markers.slice(0, 5) };
});
console.log(`AC11 /rooms: ${JSON.stringify(rooms)}`);

// AC 1 (/dunning rows)
await page.goto(BASE + '/dunning?period=2026-09', { waitUntil: 'networkidle' });
const dunning = await page.evaluate(() => ({
  candidates: document.querySelectorAll('.dunning-candidate').length,
  detailLinks: document.querySelectorAll('.dunning-candidate-detail').length,
  primary: [...document.querySelectorAll('header button.btn, header a.btn')].filter((b) => b.offsetParent !== null).map((b) => b.textContent.trim()),
  sendFormAction: document.querySelector('form#dunning-candidate-form, form.dunning-candidate-form')?.getAttribute('action'),
  sendFormActionAttr: document.querySelector('[formaction]')?.getAttribute('formaction'),
}));
console.log(`AC1 /dunning: ${JSON.stringify(dunning)}`);

await browser.close();
