// Attack points 1+2 of the 09-19-list-pages-alignment review.
//
// For every list page filter form: enumerate the real controls, and for each
// control decide whether a user can actually submit it.
//   * does the form contain a *visible* submit button (pointer + keyboard)?
//   * does the control carry an inline onchange auto-submit?
//   * is the shell's shared `.object-list-filter-fields` change listener alive
//     (i.e. does dispatching a bare `change` event submit)?
//
// A control with none of the three is unreachable-but-rendered: it looks like a
// filter, changes the URL never, and is a fake control.
//
// Usage: AUDIT_BASE=... AUDIT_USER=... AUDIT_PASS=... node scripts/audit/probe-filter-inventory.mjs
import { chromium } from 'playwright';
import { launchOptions } from './launch.mjs';

const BASE = (process.env.AUDIT_BASE || 'http://127.0.0.1:18094').replace(/\/+$/, '');
const USER = process.env.AUDIT_USER;
const PASS = process.env.AUDIT_PASS;

const PAGES = [
  '/properties?period=2026-09&status=all',
  '/rooms?period=2026-09&status=all',
  '/tenants',
  '/tenancies?period=2026-09&status=all',
  '/bills?period=2026-09',
  '/transactions?scope=pending',
  '/dunning?period=2026-09',
  '/cash-receipts?period=2026-09',
  '/expenses?period=2026-09',
  '/bank',
];

const INVENTORY = `(() => {
  const out = [];
  for (const form of document.querySelectorAll('form')) {
    const controls = [...form.querySelectorAll('select, input:not([type=hidden]), textarea, button, a.btn')]
      .filter((el) => el.name || ['SELECT','TEXTAREA','BUTTON'].includes(el.tagName) || el.type === 'submit');
    if (!controls.length) continue;
    const submitBtns = [...form.querySelectorAll('button[type=submit], input[type=submit]')];
    out.push({
      action: form.getAttribute('action') || '(self)',
      method: (form.getAttribute('method') || 'get').toLowerCase(),
      cls: form.className,
      visibleSubmit: submitBtns.some((b) => b.offsetParent !== null),
      submitLabels: submitBtns.map((b) => b.textContent.trim() || b.value),
      controls: [...form.querySelectorAll('select, input:not([type=hidden]), textarea')]
        .filter((el) => el.name)
        .map((el) => ({
          tag: el.tagName.toLowerCase(), name: el.name, type: el.type || '',
          visible: el.offsetParent !== null,
          onchange: el.getAttribute('onchange'),
          value: el.value,
        })),
    });
  }
  return out;
})()`;

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

for (const url of PAGES) {
  await page.goto(BASE + url, { waitUntil: 'networkidle', timeout: 30000 });
  const forms = await page.evaluate(INVENTORY);
  console.log(`\n=== ${url} (${forms.length} forms)`);
  for (const f of forms) {
    console.log(`  form[${f.method}] ${f.action} .${f.cls} visibleSubmit=${f.visibleSubmit} ${JSON.stringify(f.submitLabels)}`);
    for (const c of f.controls) {
      console.log(`     - ${c.tag}[name=${c.name}${c.type ? ' type=' + c.type : ''}] visible=${c.visible} onchange=${c.onchange ? 'YES' : 'no'} value=${JSON.stringify(c.value)}`);
    }
    if (!f.controls.length) console.log('     (no named controls)');
  }
}

// Is the shell's shared change listener alive anywhere?
console.log('\n=== shell .object-list-filter-fields change listener liveness');
for (const url of PAGES) {
  await page.goto(BASE + url, { waitUntil: 'networkidle', timeout: 30000 });
  const res = await page.evaluate(async () => {
    const fields = document.querySelectorAll('.object-list-filter-fields');
    if (!fields.length) return { fields: fields.length };
    const target = document.querySelector('.object-list-filter-fields select, .object-list-filter-fields input');
    if (!target) return { fields: fields.length, control: false };
    const before = location.href;
    target.dispatchEvent(new Event('change', { bubbles: true }));
    await new Promise((r) => setTimeout(r, 900));
    return { fields: fields.length, control: target.name, submitted: location.href !== before,
             inlineOnchange: target.getAttribute('onchange') };
  });
  console.log(`  ${url}: ${JSON.stringify(res)}`);
}

await browser.close();
