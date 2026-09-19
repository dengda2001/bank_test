// Attack point 2 (root cause) of the 09-19-list-pages-alignment review.
//
// HISTORICAL DIAGNOSTIC -- kept for before/after comparison, not as a description
// of the current tree. It records the shape of the defect as it existed on
// 09-19: the shared chrome registered
//   document.querySelectorAll(".object-list-filter-fields select, ...")
// inside an IIFE that ran while the parser was still above <main>, so the
// NodeList came back empty on every page. Two consequences were measured:
//   1. dispatching a bare `change` on an .object-list-filter-fields control
//      (i.e. what a keyboard user produces when they move the selection) did
//      NOT submit unless the control also carried inline onchange;
//   2. therefore /properties and /rooms -- whose only submit affordance was that
//      inline onchange (no visible submit button) -- depended entirely on it.
//
// 09-20 (09-20-mobile-filter-and-duplicate-errors) fixed both: the chrome now
// defers every page-body lookup to DOMContentLoaded, so this binding attaches and
// is the single submit path, and the inline onchange escapes were deleted from
// /properties and /rooms. This script asserts nothing -- it prints what it sees --
// so it is safe to run against either revision. Run it on the current tree and the
// readings below flip: `inline onchange present` reports none, and BOTH legs submit
// (the removed-onchange leg is the one that used to be the only working path).
//
// Also exercises the /bank "立即同步" POST path, which no probe had hit before
// and which remains worth regressing.
//
// Usage: AUDIT_BASE=... AUDIT_USER=... AUDIT_PASS=... node scripts/audit/probe-shell-listener.mjs
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

// 1. Is the shared listener bound at all? Register a sentinel after load and see
//    whether the controls exist *now* (they do) but have no `change` behaviour.
async function dispatchChange(url, removeInline) {
  await page.goto(BASE + url, { waitUntil: 'networkidle', timeout: 30000 });
  const result = await page.evaluate((strip) => {
    const node = document.querySelector('.object-list-filter-fields select');
    if (!node) return { found: false };
    const inline = node.getAttribute('onchange');
    if (strip) node.removeAttribute('onchange');
    const before = location.href;
    node.dispatchEvent(new Event('change', { bubbles: true }));
    return { found: true, inline, name: node.name, before };
  }, removeInline);
  await page.waitForTimeout(1500);
  const after = page.url();
  return { ...result, submitted: after !== result.before, after: after.replace(BASE, '') };
}

console.log('=== shared .object-list-filter-fields change listener ===');
for (const url of ['/properties?period=2026-09&status=all', '/rooms?period=2026-09&status=all']) {
  const withInline = await dispatchChange(url, false);
  const without = await dispatchChange(url, true);
  console.log(`${url}
   inline onchange present: submitted=${withInline.submitted} (${withInline.inline ? 'onchange=' + withInline.inline : 'none'})
   inline onchange removed: submitted=${without.submitted} -> ${without.after}`);
}

// 2. Same question for a control that never had inline onchange and lives in
//    .object-list-filter-fields -- is there any page that still has one?
const bare = await (async () => {
  await page.goto(BASE + '/tenancies?period=2026-09&status=all', { waitUntil: 'networkidle' });
  return page.evaluate(() => ({
    fields: document.querySelectorAll('.object-list-filter-fields').length,
    controls: [...document.querySelectorAll('.object-list-filter-fields select, .object-list-filter-fields input')].map((e) => e.name),
  }));
})();
console.log(`\n/tenancies .object-list-filter-fields = ${JSON.stringify(bare)}`);

// 3. /bank 立即同步: the page only renders the POST form when connected, so probe
//    the route the button points at with the session cookie the page already has.
const sync = await page.evaluate(async () => {
  const resp = await fetch('/bank/sync', { method: 'POST', credentials: 'same-origin', redirect: 'manual' });
  return { status: resp.status, type: resp.type, url: resp.url };
});
console.log(`\nPOST /bank/sync -> ${JSON.stringify(sync)}`);
const syncGet = await page.evaluate(async () => {
  const resp = await fetch('/bank/sync', { method: 'GET', credentials: 'same-origin', redirect: 'manual' });
  return { status: resp.status, type: resp.type, url: resp.url };
});
console.log(`GET  /bank/sync -> ${JSON.stringify(syncGet)}`);

// 4. And what the *disconnected* /bank page renders in the account-actions block.
await page.goto(BASE + '/bank', { waitUntil: 'networkidle' });
const bank = await page.evaluate(() => {
  const actions = document.querySelector('.bank-account-actions');
  return { connected: /已连接/.test(document.body.textContent), actionsHTML: actions?.innerHTML.slice(0, 400) };
});
console.log(`\n/bank account-actions: ${JSON.stringify(bank, null, 1)}`);

await browser.close();
