// Checker probe for 09-20. Targets the blind spot shared by the implementer's
// probes and the main-session probe: the forbidden raw echo is only scanned in
// its literal `{{.Message}}` spelling, so a template that echoes the request
// value through a *function* (`{{billsNotice .Message}}`, whose default arm
// returns the code verbatim) would pass every existing assertion while still
// putting arbitrary request text in a green success toast.
//
// Read-only: GETs only.
import { chromium } from 'playwright';
import { launchOptions } from './launch.mjs';

const BASE = (process.env.AUDIT_BASE || 'http://127.0.0.1:18097').replace(/\/+$/, '');
const USER = process.env.AUDIT_USER;
const PASS = process.env.AUDIT_PASS;
const PERIOD = process.env.AUDIT_PERIOD || '2026-09';
const MARKER = 'INJECT-7f3a91';

const results = [];
const record = (name, ok, detail) => {
  results.push({ name, ok, detail });
  console.log(`${ok ? 'PASS' : 'FAIL'}  ${name}${detail ? ' — ' + detail : ''}`);
};
const withQuery = (path, params) => {
  const u = new URL(BASE + path);
  for (const [k, v] of Object.entries(params)) u.searchParams.set(k, v);
  return u.toString();
};

const browser = await chromium.launch(launchOptions());
const ctx = await browser.newContext({ viewport: { width: 1440, height: 900 } });
const page = await ctx.newPage();

await page.goto(BASE + '/', { waitUntil: 'domcontentloaded' });
if (await page.$('#username')) {
  await page.fill('#username', USER);
  await page.fill('#password', PASS);
  await Promise.all([page.waitForNavigation({ waitUntil: 'domcontentloaded' }), page.click('button[type=submit]')]);
}

// Every page whose template can render a `notice ok` marked data-toast.
for (const path of [
  '/bills',
  '/cash-receipts',
  '/properties',
  '/rooms',
  '/tenancies',
  '/expenses',
  '/rent-dashboard',
  '/bank',
  '/transactions',
]) {
  await page.goto(withQuery(path, { period: PERIOD, message: MARKER }), { waitUntil: 'networkidle' });
  const probe = await page.evaluate((m) => {
    const body = document.body.innerText;
    const ok = [...document.querySelectorAll('.notice.ok')].map((el) => el.textContent.trim());
    const marked = [...document.querySelectorAll('.notice[data-toast]')].map((el) => el.textContent.trim());
    const toast = document.querySelector('#workspace-toast');
    return { inBody: body.includes(m), ok, marked, toast: toast ? toast.textContent.trim() : null };
  }, MARKER);
  const leaked = probe.inBody || probe.ok.some((t) => t.includes(MARKER)) || probe.marked.some((t) => t.includes(MARKER)) || (probe.toast || '').includes(MARKER);
  record(
    `X ${path}?message=<注入文本> 不回显`,
    !leaked,
    `inBody=${probe.inBody} okNotices=${JSON.stringify(probe.ok)} marked=${JSON.stringify(probe.marked)} toast=${JSON.stringify(probe.toast)}`,
  );
}

await browser.close();
const failed = results.filter((r) => !r.ok);
console.log(`\n${results.length - failed.length}/${results.length} passed`);
if (failed.length) for (const f of failed) console.log(`  - ${f.name} (${f.detail})`);
process.exit(failed.length === 0 ? 0 : 1);
