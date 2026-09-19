// Attack point 2 of the 09-19-list-pages-alignment review: the two `onchange`
// auto-submit selects on /properties and /rooms are a page-local workaround.
// Does a keyboard user get the same submission a mouse user does, and can the
// page be driven without a pointer at all?
//
// Usage: AUDIT_BASE=... AUDIT_USER=... AUDIT_PASS=... node scripts/audit/probe-list-keyboard.mjs
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
await Promise.all([page.waitForNavigation({ waitUntil: 'domcontentloaded' }).catch(() => {}), page.click('button[type=submit]')]);

for (const [url, sel, label] of [
  ['/rooms?period=2026-09&status=all', 'form.object-list-filters select[name="property_id"]', 'rooms/property_id'],
  ['/rooms?period=2026-09&status=all', 'form.object-list-filters select[name="collection"]', 'rooms/collection'],
  ['/properties?period=2026-09&status=active', 'form.object-list-filters select[name="collection"]', 'properties/collection'],
  ['/bills?period=2026-09', 'form.bills-filters select[name="status"]', 'bills/status (pre-existing onchange)'],
]) {
  await page.goto(BASE + url, { waitUntil: 'networkidle' });
  const before = page.url();
  const valueBefore = await page.$eval(sel, (e) => e.value);
  await page.focus(sel);
  const focused = await page.evaluate((s) => document.activeElement === document.querySelector(s), sel);
  let nav = false;
  page.waitForNavigation({ timeout: 3000 }).then(() => { nav = true; }).catch(() => {});
  await page.keyboard.press('ArrowDown');
  await page.waitForTimeout(1500);
  const valueAfter = page.url() === before ? await page.$eval(sel, (e) => e.value).catch(() => '?') : '(page reloaded)';
  console.log(`kbd ${label}: focusable=${focused} arrow-navigates=${nav} urlChanged=${page.url() !== before} value ${valueBefore} -> ${valueAfter}`);
}

// Can a keyboard-only user reach and submit the search box (no submit button)?
for (const [url, sel, label] of [
  ['/rooms?period=2026-09&status=all', 'form.object-list-filters input[name="search"]', 'rooms/search'],
  ['/properties?period=2026-09&status=active', 'form.object-list-filters input[name="search"]', 'properties/search'],
]) {
  await page.goto(BASE + url, { waitUntil: 'networkidle' });
  const before = page.url();
  await page.focus(sel);
  await page.keyboard.type('zzz');
  await page.keyboard.press('Enter');
  await page.waitForTimeout(1200);
  console.log(`kbd ${label}: enter-submits=${page.url() !== before} -> ${page.url()}`);
}

// Is there any pointer-reachable submit button on the desktop properties/rooms filterbar?
for (const url of ['/rooms?period=2026-09&status=all', '/properties?period=2026-09&status=active']) {
  await page.goto(BASE + url, { waitUntil: 'networkidle' });
  const buttons = await page.evaluate(() => [...document.querySelectorAll('.object-list-filters button, .object-list-filters input[type=submit]')].map((b) => `${b.type}:${b.textContent.trim()}:visible=${b.offsetParent !== null}`));
  console.log(`buttons on ${url}: ${JSON.stringify(buttons)}`);
}

await browser.close();
