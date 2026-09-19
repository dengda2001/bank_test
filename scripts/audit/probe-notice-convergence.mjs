// Defects 2 and 3 (09-20-mobile-filter-and-duplicate-errors), verified on real
// pages rather than by reading the templates.
//
// Defect 3: three templates printed the ?message= query value straight into a
// green "notice ok" toast. A crafted link therefore showed arbitrary text inside
// the trusted UI. The message parameter must be a code matched against a fixed
// template sentence.
//   * /bank?message=注入测试文本        must show no success notice at all
//   * /bank?message=refreshed           must show 银行数据已刷新。 (the redirect
//     target the POST /bank/sync handler sends the browser to)
//   * /rent-workspace?message=注入测试文本 must show no success notice
//
// Defect 2: each cash receipt error code rendered two differently-worded messages
// in one render of /cash-receipts (a page-level banner plus the same error inside
// the drawer). Both codes are checked, and each is checked on both of its render
// paths (the drawer and the inline /cash-receipts/new page):
//   * exactly one element carries the canonical sentence, and
//   * no element carries a retired wording.
// Exact text equality is deliberate: the retired short form is a substring of the
// canonical one, so an `includes` test would have passed on the broken tree.
//
// Read-only.
import { chromium } from 'playwright';
import { launchOptions } from './launch.mjs';

const BASE = (process.env.AUDIT_BASE || 'http://127.0.0.1:18098').replace(/\/+$/, '');
const USER = process.env.AUDIT_USER;
const PASS = process.env.AUDIT_PASS;
const PERIOD = process.env.AUDIT_PERIOD || '2026-09';
const INJECTION = '注入测试文本';
const REFRESHED = '银行数据已刷新。';

// The one surviving sentence per error code, and every wording this repository has
// retired for it. Compared by exact equality after whitespace normalisation.
const CANONICAL = {
  cash_overbalance: '这笔现金会超过该月份未收余额，请核对已有收款。',
  cash_receipt_failed: '现金补录失败，请检查租客、月份、金额和日期。',
};
const RETIRED = {
  cash_overbalance: ['入账金额大于当前未收余额。', '这笔现金会超过该月份的未收余额，请重新核对银行与现金收款。'],
  cash_receipt_failed: ['请检查租客、月份、金额和日期。', '现金补录失败，请检查租客、月份、金额和币种。'],
};

if (!USER || !PASS) {
  console.error('set AUDIT_USER and AUDIT_PASS (see the launcher output)');
  process.exit(1);
}

let failures = 0;
const report = (ok, label, detail) => {
  if (!ok) failures++;
  console.log(`${ok ? 'ok  ' : 'FAIL'} ${label}${detail ? '  ' + detail : ''}`);
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

// Count visible success notices plus the shared toast, which the chrome script
// fills from the marked notice, so either carrier is seen.
const successText = () =>
  page.evaluate((needle) => {
    const visible = (el) => {
      const r = el.getBoundingClientRect();
      const s = getComputedStyle(el);
      return r.width > 0 && r.height > 0 && s.display !== 'none' && s.visibility !== 'hidden';
    };
    const nodes = [
      ...document.querySelectorAll('.notice.ok, #workspace-toast'),
    ].filter(visible);
    return nodes
      .map((el) => (el.textContent || '').replace(/\s+/g, ' ').trim())
      .filter((text) => text.includes(needle));
  }, INJECTION);

// --- defect 3: the injection value must not surface as a success notice -----
for (const [label, url] of [
  ['bank', `${BASE}/bank?message=${encodeURIComponent(INJECTION)}`],
  ['rent-workspace', `${BASE}/rent-workspace?period=${PERIOD}&message=${encodeURIComponent(INJECTION)}`],
]) {
  await page.goto(url, { waitUntil: 'networkidle' });
  await page.waitForTimeout(2400); // the toast lives 2.2s; catch it either way
  const seen = await successText();
  report(seen.length === 0, `${label}: injected message is not rendered as a success notice`, seen.join(' | ') || '(none)');
  const anywhere = await page.evaluate((needle) => document.body.innerText.includes(needle), INJECTION);
  report(!anywhere, `${label}: injected text is not on the page at all`, anywhere ? 'found in body text' : '(absent)');
}

// --- defect 3: the whitelisted code still renders its own sentence ----------
await page.goto(`${BASE}/bank?message=refreshed`, { waitUntil: 'networkidle' });
const refreshedSeen = await page.evaluate((needle) => {
  const visible = (el) => {
    const s = getComputedStyle(el);
    return s.display !== 'none' && s.visibility !== 'hidden';
  };
  return [...document.querySelectorAll('.notice.ok, #workspace-toast')]
    .filter(visible)
    .some((el) => (el.textContent || '').includes(needle));
}, REFRESHED);
report(refreshedSeen, 'bank: ?message=refreshed renders 银行数据已刷新。', refreshedSeen ? '' : 'not seen');

// --- defect 2: each error code renders one message, on both of its paths -----
const noticeTexts = () =>
  page.evaluate(() =>
    [...document.querySelectorAll('.notice')]
      .map((el) => (el.textContent || '').replace(/\s+/g, ' ').trim())
      .filter(Boolean),
  );

for (const [label, code, url] of [
  ['drawer', 'cash_overbalance', `${BASE}/cash-receipts?period=${PERIOD}&status=all&error=cash_overbalance&add=1`],
  ['inline', 'cash_overbalance', `${BASE}/cash-receipts/new?period=${PERIOD}&error=cash_overbalance`],
  ['drawer', 'cash_receipt_failed', `${BASE}/cash-receipts?period=${PERIOD}&status=all&error=cash_receipt_failed&add=1`],
  ['inline', 'cash_receipt_failed', `${BASE}/cash-receipts/new?period=${PERIOD}&error=cash_receipt_failed`],
]) {
  await page.goto(url, { waitUntil: 'networkidle' });
  const texts = await noticeTexts();
  const canonical = texts.filter((text) => text === CANONICAL[code]).length;
  const extras = texts.filter((text) => text !== CANONICAL[code]);
  report(
    canonical === 1 && extras.length === 0,
    `${label} ${code}: renders the canonical sentence exactly once`,
    `canonical=${canonical} notices=${JSON.stringify(texts)}`,
  );
  for (const retired of RETIRED[code]) {
    report(!texts.includes(retired), `${label} ${code}: retired wording is gone`, retired);
  }
}

await browser.close();
console.log(failures === 0 ? '\nprobe-notice-convergence: PASS' : `\nprobe-notice-convergence: ${failures} FAILURE(S)`);
process.exit(failures === 0 ? 0 : 1);
