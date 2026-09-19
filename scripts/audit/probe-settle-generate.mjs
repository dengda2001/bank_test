// AC 6 (settle dispositions) and AC 8 (「生成本月账单」 idempotency) of
// 09-19-list-pages-alignment, driven through the real UI so the surrounding
// shell/CSRF/session path is the production one. The *accounting* verdict is
// read out of MySQL by the caller, before and after each invocation, not from
// the notice text on the page.
//
//   MODE=unimplemented   submit 登记现金收款 / 登记减免 / 结转下月 in turn
//   MODE=match           submit 匹配现有收款 once
//   MODE=twice           submit 匹配现有收款 again (expect no further write)
//   MODE=generate        press 生成本月账单 twice on /bills?period=2026-10
//
// Usage: AUDIT_BASE=... AUDIT_USER=... AUDIT_PASS=... MODE=... node scripts/audit/probe-settle-generate.mjs
import { chromium } from 'playwright';
import { launchOptions } from './launch.mjs';

const BASE = (process.env.AUDIT_BASE || 'http://127.0.0.1:18094').replace(/\/+$/, '');
const USER = process.env.AUDIT_USER;
const PASS = process.env.AUDIT_PASS;
const MODE = process.env.MODE || 'unimplemented';

const browser = await chromium.launch(launchOptions());
const ctx = await browser.newContext({ viewport: { width: 1440, height: 900 } });
const page = await ctx.newPage();
// The settle/send buttons carry data-confirm, and the shell turns that into a
// native confirm(). Playwright dismisses dialogs by default, which cancels the
// submit -- accept them so the click is the same submission a user makes.
page.on('dialog', (dialog) => dialog.accept());
await page.goto(BASE + '/', { waitUntil: 'domcontentloaded' });
await page.fill('#username', USER);
await page.fill('#password', PASS);
await Promise.all([
  page.waitForNavigation({ waitUntil: 'domcontentloaded' }).catch(() => {}),
  page.click('button[type=submit]'),
]);

const BILLS = '/bills?period=2026-09';
const formState = `(() => {
  const out = [];
  document.querySelectorAll('form.collection-balance-form').forEach((form, index) => {
    const select = form.querySelector('select[name=disposition]');
    const obligation = form.querySelector('input[name=obligation_id]');
    const amount = form.querySelector('input[name=amount]');
    out.push({ index, obligation: obligation?.value, amount: amount?.value,
               dispositions: select ? [...select.options].map((o) => o.value + ':' + o.textContent.trim()) : null });
  });
  return out;
})()`;

async function settle(dispositionValue) {
  await page.goto(BASE + BILLS, { waitUntil: 'networkidle' });
  const forms = await page.evaluate(formState);
  if (!forms.length) { console.log('NO SETTLE FORM on ' + BILLS); return; }
  if (dispositionValue && forms[0].dispositions) {
    const known = forms[0].dispositions.some((d) => d.startsWith(dispositionValue + ':'));
    if (!known) { console.log(`disposition ${dispositionValue} not offered: ${forms[0].dispositions.join(', ')}`); return; }
  }
  // The form lives inside a collapsed <details>; open it first so the controls a
  // real user sees are the ones being driven.
  await page.evaluate(() => { const d = document.querySelector('details.collection-settle'); if (d) d.open = true; });
  await page.waitForTimeout(200);
  if (dispositionValue) await page.selectOption('form.collection-balance-form >> nth=0 >> select[name=disposition]', dispositionValue);
  await page.fill('form.collection-balance-form >> nth=0 >> textarea[name=reason]', `复验-${MODE}-${dispositionValue || 'default'}`);
  await Promise.all([
    page.waitForNavigation({ waitUntil: 'domcontentloaded', timeout: 15000 }).catch(() => {}),
    page.click('form.collection-balance-form >> nth=0 >> button[type=submit]'),
  ]);
  await page.waitForLoadState('networkidle').catch(() => {});
  const notice = await page.evaluate(() => [...document.querySelectorAll('.notice')].map((n) => n.textContent.trim()));
  console.log(`settle ${dispositionValue || '(default)'} obligation=${forms[0].obligation} -> ${page.url().replace(BASE, '')}
   notices=${JSON.stringify(notice)}`);
}

if (MODE === 'unimplemented') {
  const first = await (async () => { await page.goto(BASE + BILLS, { waitUntil: 'networkidle' }); return page.evaluate(formState); })();
  console.log(`options offered: ${JSON.stringify(first[0]?.dispositions)}`);
  for (const value of ['cash_receipt', 'waiver', 'carry_forward']) await settle(value);
} else if (MODE === 'match' || MODE === 'twice') {
  await settle('match_payment');
} else if (MODE === 'generate') {
  await page.goto(BASE + '/bills?period=2026-10', { waitUntil: 'networkidle' });
  const before = await page.evaluate(() => document.querySelectorAll('table tbody tr').length);
  for (const attempt of [1, 2]) {
    await Promise.all([
      page.waitForNavigation({ waitUntil: 'domcontentloaded', timeout: 15000 }).catch(() => {}),
      page.click('form.bills-generate-action button[type=submit]'),
    ]);
    await page.waitForLoadState('networkidle').catch(() => {});
    const after = await page.evaluate(() => document.querySelectorAll('table tbody tr').length);
    const notice = await page.evaluate(() => [...document.querySelectorAll('.notice')].map((n) => n.textContent.trim()));
    console.log(`generate attempt ${attempt}: rowsBefore=${before} rowsAfter=${after} -> ${page.url().replace(BASE, '')} notices=${JSON.stringify(notice)}`);
  }
  const getCode = await page.evaluate(async () => (await fetch('/bills/generate', { method: 'GET', redirect: 'manual' })).status);
  console.log(`GET /bills/generate -> status ${getCode} (0 == opaque redirect)`);
}

await browser.close();
