// Interaction-level verification for 09-19-detail-pages-alignment.
//
// Covers the two acceptance criteria that a static DOM probe cannot: the
// tenant 付款识别 copy button (AC5) and the transaction header's three action
// entries actually posting to the same endpoints, with the same parameters, as
// the list-row actions (AC9). Writes are posted only to the disposable audit
// instance.
//
// Usage:
//   AUDIT_BASE=http://127.0.0.1:18092 AUDIT_USER=... AUDIT_PASS=... \
//     node scripts/audit/detail-pages-actions.mjs
import { chromium } from 'playwright';
import { launchOptions } from './launch.mjs';

const BASE = (process.env.AUDIT_BASE || 'http://127.0.0.1:18092').replace(/\/+$/, '');
const TENANT_ID = process.env.AUDIT_TENANT_ID || 3;
const TX_ID = process.env.AUDIT_TX_ID || 5;

const browser = await chromium.launch(launchOptions());
const ctx = await browser.newContext({ viewport: { width: 1440, height: 900 }, permissions: ['clipboard-read', 'clipboard-write'] });
const page = await ctx.newPage();
await page.goto(BASE + '/', { waitUntil: 'domcontentloaded' });
await page.fill('#username', process.env.AUDIT_USER);
await page.fill('#password', process.env.AUDIT_PASS);
await Promise.all([page.waitForNavigation({ waitUntil: 'domcontentloaded' }).catch(() => {}), page.click('button[type=submit]')]);

const results = {};

// --- AC5: the copy button copies the payment reference code. ---------------
await page.goto(`${BASE}/tenants/${TENANT_ID}`, { waitUntil: 'networkidle' });
const reference = await page.locator('.reference-row strong').innerText();
const copyButton = page.locator('.copy-reference');
results.copyButtonValue = await copyButton.getAttribute('data-copy-value');
await copyButton.click();
results.clipboard = await page.evaluate(() => navigator.clipboard.readText());
results.buttonFlash = await copyButton.innerText();
results.referenceMatchesClipboard = results.clipboard === results.copyButtonValue;
results.referenceText = reference;
await page.waitForTimeout(1700);
results.buttonRestored = await copyButton.innerText();

// --- AC9: the three header entries render and post to the same endpoints. --
const MATCHED_TX = process.env.AUDIT_MATCHED_TX_ID || 1;

async function formsOnDetail(txID) {
  await page.goto(`${BASE}/transactions?detail=${txID}`, { waitUntil: 'networkidle' });
  return {
    entries: await page.locator('.transaction-detail-actions > details > summary').allInnerTexts(),
    forms: await page.$$eval('.transaction-detail-actions form', (forms) =>
      forms.map((f) => ({
        action: f.getAttribute('action'),
        fields: Array.from(f.elements).filter((e) => e.name && e.name !== 'return_to').map((e) => e.name).sort(),
        returnTo: f.querySelector('input[name=return_to]')?.value ?? null,
      }))
    ),
  };
}

async function formsOnListRow(txID) {
  await page.goto(`${BASE}/transactions?match_status=all`, { waitUntil: 'networkidle' });
  return page.$$eval(`#transaction-row-${txID} form`, (forms) =>
    forms.map((f) => ({
      action: f.getAttribute('action'),
      fields: Array.from(f.elements).filter((e) => e.name && e.name !== 'return_to').map((e) => e.name).sort(),
    }))
  );
}

results.headerEntryLabels = [];
results.parity = {};
for (const txID of [MATCHED_TX, TX_ID]) {
  const detail = await formsOnDetail(txID);
  const list = await formsOnListRow(txID);
  results.headerEntryLabels = detail.entries;
  results[`detailForms@${txID}`] = detail.forms;
  results[`listRowForms@${txID}`] = list;
  results.parity[txID] = detail.forms.every((d) =>
    list.some((l) => l.action === d.action && JSON.stringify(l.fields) === JSON.stringify(d.fields))
  );
}
results.detailForms = results[`detailForms@${TX_ID}`];

// Post the 标记非租金 entry from the detail header and confirm it lands back on
// the list it came from (the return chain stays intact).
await page.goto(`${BASE}/transactions?detail=${TX_ID}&match_status=unmatched`, { waitUntil: 'networkidle' });
const ignoreDetails = page.locator('.transaction-detail-actions > details').filter({ hasText: '标记非租金' });
await ignoreDetails.locator('summary').click();
const ignoreForm = ignoreDetails.locator('form');
results.ignoreAction = await ignoreForm.getAttribute('action');
results.ignoreReturnTo = await ignoreForm.locator('input[name=return_to]').inputValue();
await ignoreForm.locator('input[name=reason]').fill('detail-pages alignment live check');
await Promise.all([
  page.waitForNavigation({ waitUntil: 'networkidle', timeout: 20000 }).catch(() => {}),
  ignoreForm.locator('button[type=submit]').click(),
]);
results.urlAfterIgnore = page.url();
results.landedOnList = page.url().includes('/transactions') && !page.url().includes('detail=');

console.log(JSON.stringify(results, null, 2));
await browser.close();

const failures = [];
if (!results.referenceMatchesClipboard || !results.clipboard) failures.push('AC5 copy button did not copy the reference code');
if (results.headerEntryLabels.length !== 3) failures.push('AC9 header does not expose three action entries');
for (const txID of [MATCHED_TX, TX_ID]) {
  if (!results.parity[txID]) failures.push(`AC9 the header forms for transaction ${txID} do not match the same-named list-row forms`);
  if (!results[`detailForms@${txID}`].every((f) => f.returnTo)) failures.push(`AC9 header form for transaction ${txID} is missing return_to`);
}
if (!results[`detailForms@${MATCHED_TX}`].some((f) => f.action === '/transactions/rematch')) failures.push('AC9 a matched transaction does not expose 编辑分配');
if (!results.detailForms.some((f) => f.action === '/transactions/confirm')) failures.push('AC9 header is missing the 确认匹配 entry');
if (!results.detailForms.some((f) => f.action === '/transactions/ignore')) failures.push('AC9 header is missing the 标记非租金 entry');
if (!results.landedOnList) failures.push(`AC9 posting 标记非租金 did not return to the list (${results.urlAfterIgnore})`);
if (failures.length) {
  console.error('\nFAILURES:');
  for (const f of failures) console.error('  - ' + f);
  process.exit(1);
}
console.log('\nAC5 copy button and AC9 header actions verified against the live instance');
