// Defect 1 (09-20-mobile-filter-and-duplicate-errors): the shared chrome script
// is parsed before <main>, so its top-level querySelectorAll for the page-body
// filter controls came back empty and none of the listeners were attached. The
// consequence is not a dead button: at <=640px the filter panel is opened only by
// that listener, so filtering was unreachable on a narrow screen; the fields'
// change->submit was dead at every width.
//
// This drives the real toggles and expects:
//   * 390x844 on /properties and /rooms: click "筛选" -> aria-expanded true and
//     .object-list-filter-fields computes to display:grid; click again -> back;
//   * >=641px on the same pages: the toggle is display:none (unchanged), and a
//     select change submits the form (the shared binding is the only submit path);
//   * /tenancies, which has no toggle and a type=submit button, still submits at
//     both widths (the reverse assertion: the toggle mechanism must not be pushed
//     onto a page that has none).
//
// Read-only (it only navigates; every request is a GET filter submit).
import { chromium } from 'playwright';
import { launchOptions } from './launch.mjs';

const BASE = (process.env.AUDIT_BASE || 'http://127.0.0.1:18098').replace(/\/+$/, '');
const USER = process.env.AUDIT_USER;
const PASS = process.env.AUDIT_PASS;
const PERIOD = process.env.AUDIT_PERIOD || '2026-09';

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
const pageErrors = [];
page.on('pageerror', (err) => pageErrors.push(String(err)));

await page.goto(BASE + '/', { waitUntil: 'domcontentloaded' });
if (await page.$('#username')) {
  await page.fill('#username', USER);
  await page.fill('#password', PASS);
  await Promise.all([page.waitForNavigation({ waitUntil: 'domcontentloaded' }), page.click('button[type=submit]')]);
}

const state = () =>
  page.evaluate(() => {
    const toggle = document.querySelector('.object-list-filter-toggle');
    const fields = document.querySelector('.object-list-filter-fields');
    return {
      hasToggle: !!toggle,
      expanded: toggle ? toggle.getAttribute('aria-expanded') : null,
      toggleDisplay: toggle ? getComputedStyle(toggle).display : null,
      fieldsDisplay: fields ? getComputedStyle(fields).display : null,
    };
  });

// --- <=640px: the toggle opens and closes the field panel -------------------
for (const path of ['/properties', '/rooms']) {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto(`${BASE}${path}?period=${PERIOD}`, { waitUntil: 'networkidle' });
  const before = await state();
  report(
    before.hasToggle && before.toggleDisplay !== 'none',
    `${path} @390 toggle visible`,
    `display=${before.toggleDisplay}`,
  );
  report(before.expanded === 'false', `${path} @390 collapsed at rest`, `aria-expanded=${before.expanded}`);
  report(before.fieldsDisplay === 'none', `${path} @390 fields hidden at rest`, `display=${before.fieldsDisplay}`);

  await page.click('.object-list-filter-toggle');
  const open = await state();
  report(open.expanded === 'true', `${path} @390 click opens (aria-expanded)`, `aria-expanded=${open.expanded}`);
  report(open.fieldsDisplay === 'grid', `${path} @390 click opens (computed display)`, `display=${open.fieldsDisplay}`);

  await page.click('.object-list-filter-toggle');
  const closed = await state();
  report(closed.expanded === 'false', `${path} @390 second click collapses (aria-expanded)`, `aria-expanded=${closed.expanded}`);
  report(closed.fieldsDisplay === 'none', `${path} @390 second click collapses (computed display)`, `display=${closed.fieldsDisplay}`);
  report(page.url().includes(path), `${path} @390 toggle does not navigate`, page.url());
}

// --- >=641px: toggle stays hidden, and a select change submits --------------
for (const [path, select, value] of [
  ['/properties', 'select[name="collection"]', 'unpaid'],
  ['/rooms', 'select[name="collection"]', 'unpaid'],
]) {
  await page.setViewportSize({ width: 1440, height: 900 });
  await page.goto(`${BASE}${path}?period=${PERIOD}`, { waitUntil: 'networkidle' });
  const desktop = await state();
  report(desktop.toggleDisplay === 'none', `${path} @1440 toggle hidden`, `display=${desktop.toggleDisplay}`);
  report(desktop.fieldsDisplay !== 'none', `${path} @1440 fields shown`, `display=${desktop.fieldsDisplay}`);

  // Count the document requests for this select change: two submit sources (the
  // shared binding plus a page-local onchange) would fire the form twice.
  const documentRequests = [];
  const onRequest = (req) => {
    if (req.isNavigationRequest() && req.url().startsWith(BASE + path)) documentRequests.push(req.url());
  };
  page.on('request', onRequest);
  await Promise.all([
    page.waitForNavigation({ waitUntil: 'networkidle' }),
    page.selectOption(`.object-list-filter-fields ${select}`, value),
  ]);
  page.off('request', onRequest);
  const url = new URL(page.url());
  report(
    url.searchParams.get('collection') === value,
    `${path} @1440 select change submits through the shared binding`,
    page.url(),
  );
  report(
    documentRequests.length === 1,
    `${path} @1440 select change submits the form exactly once`,
    `requests=${documentRequests.length}`,
  );
}

// --- /tenancies has no toggle: its submit button must still work ------------
for (const width of [390, 1440]) {
  await page.setViewportSize({ width, height: 900 });
  await page.goto(`${BASE}/tenancies?period=${PERIOD}`, { waitUntil: 'networkidle' });
  const toggles = await page.$$('.object-list-filter-toggle');
  report(toggles.length === 0, `/tenancies @${width} has no filter toggle`, `toggles=${toggles.length}`);
  if (width <= 640) {
    // /tenancies hides its filter form at <=640px and reveals it from the shared
    // mobile-search button, so this leg also regresses the [data-mobile-search]
    // binding (defect 1's third deferred lookup).
    await page.click('[data-mobile-search]');
    const shown = await page.evaluate(() => {
      const form = document.querySelector('.tenancy-filters');
      return form ? getComputedStyle(form).display : null;
    });
    report(shown === 'grid', `/tenancies @${width} mobile-search button reveals the filter form`, `display=${shown}`);
  }
  await page.fill('.object-list-filters input[name="search"]', 'ZZZNOMATCHZZZ');
  await Promise.all([
    page.waitForNavigation({ waitUntil: 'networkidle' }),
    page.click('.object-list-filters button[type=submit]'),
  ]);
  report(
    new URL(page.url()).searchParams.get('search') === 'ZZZNOMATCHZZZ',
    `/tenancies @${width} submit button still filters`,
    page.url(),
  );
}

// --- the nav bindings moved into the same deferred block, so regress them ----
await page.setViewportSize({ width: 390, height: 844 });
await page.goto(`${BASE}/properties?period=${PERIOD}`, { waitUntil: 'networkidle' });
await page.click('.nav-compact-brand');
await page.waitForTimeout(300); // visibility transitions over 200ms; read it settled
const drawer = await page.evaluate(() => ({
  checked: document.getElementById('nav-drawer')?.checked,
  sidebarVisibility: getComputedStyle(document.querySelector('.sidebar')).visibility,
}));
report(drawer.checked === true, 'mobile drawer opens from the compact bar', `checked=${drawer.checked}`);
report(drawer.sidebarVisibility === 'visible', 'opened drawer is not visibility:hidden', drawer.sidebarVisibility);
// The open rail (280px) sits above the scrim, so click the scrim clear of it.
await page.click('.nav-scrim', { position: { x: 350, y: 400 } });
report(
  (await page.evaluate(() => document.getElementById('nav-drawer')?.checked)) === false,
  'mobile drawer closes on the scrim',
);

await page.click('.mobile-bottom-nav [data-mobile-menu="objects"]');
report(
  (await page.evaluate(() => document.getElementById('mobile-menu-objects')?.hidden)) === false,
  'mobile object flyout opens from the bottom nav',
);
await page.keyboard.press('Escape');
report(
  (await page.evaluate(() => document.getElementById('mobile-menu-objects')?.hidden)) === true,
  'mobile object flyout closes on Escape',
);

report(pageErrors.length === 0, 'no uncaught page errors', pageErrors.join(' | '));

await browser.close();
console.log(failures === 0 ? '\nprobe-mobile-filter-toggle: PASS' : `\nprobe-mobile-filter-toggle: ${failures} FAILURE(S)`);
process.exit(failures === 0 ? 0 : 1);
