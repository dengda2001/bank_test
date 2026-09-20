// Probe for the two dashboard follow-ups:
//   1. the month calendar's left/right edges must sit flush with its neighbours
//   2. the property and status filters must submit as soon as they change
//
// Run against a disposable audit instance:
//   AUDIT_BASE=http://127.0.0.1:18090 AUDIT_USER=... AUDIT_PASS=... \
//     node scripts/audit/probe-dashboard-polish.mjs
import { chromium } from 'playwright';
import { existsSync } from 'node:fs';

const BASE = process.env.AUDIT_BASE || 'http://127.0.0.1:18090';
const USER = process.env.AUDIT_USER;
const PASS = process.env.AUDIT_PASS;
const SHOTS = process.env.AUDIT_SHOTS || '/tmp';

if (!USER || !PASS) {
  console.error('AUDIT_USER / AUDIT_PASS are required');
  process.exit(2);
}

const results = [];
const record = (name, ok, detail) => {
  results.push({ name, ok, detail });
  console.log(`${ok ? 'PASS' : 'FAIL'}  ${name}${detail ? `  — ${detail}` : ''}`);
};

const login = async (page) => {
  await page.goto(BASE + '/login', { waitUntil: 'networkidle' });
  await page.fill('input[name="username"]', USER);
  await page.fill('input[name="password"]', PASS);
  await Promise.all([page.waitForLoadState('networkidle'), page.click('button[type="submit"]')]);
};

const candidate = [
  process.env.CHROME_PATH,
  '/Applications/Google Chrome.app/Contents/MacOS/Google Chrome',
  '/Applications/Chromium.app/Contents/MacOS/Chromium',
].filter(Boolean).find((p) => existsSync(p));

const browser = await chromium.launch(candidate ? { executablePath: candidate } : {});
console.log(`browser: ${candidate || 'bundled chromium'}\n`);

try {
  const ctx = await browser.newContext({ viewport: { width: 1280, height: 900 } });
  const page = await ctx.newPage();
  await login(page);

  await page.goto(BASE + '/rent-dashboard', { waitUntil: 'networkidle' });
  await page.waitForTimeout(200);

  // ---------------------------------------------------------------------------
  // Requirement 1 — the month control's outer edges line up with the row it sits
  // in, and the calendar body fills the control instead of floating inside it.
  // ---------------------------------------------------------------------------
  {
    const geo = await page.evaluate(() => {
      const controls = document.querySelector('.workspace-month-controls');
      const period = document.querySelector('.workspace-period');
      const control = document.querySelector('.workspace-period .calendar-control');
      const input = document.querySelector('.workspace-period .calendar-input');
      const trigger = document.querySelector('.workspace-period .calendar-trigger');
      const read = (el) => {
        if (!el) return null;
        const r = el.getBoundingClientRect();
        return {
          left: Math.round(r.left), right: Math.round(r.right),
          top: Math.round(r.top), bottom: Math.round(r.bottom),
          width: Math.round(r.width), height: Math.round(r.height),
        };
      };
      const steps = [...document.querySelectorAll('.workspace-month-step')];
      return {
        controls: read(controls),
        period: read(period),
        control: read(control),
        input: read(input),
        trigger: read(trigger),
        prevStep: read(steps[0]),
        nextStep: read(steps[steps.length - 1]),
        rowEdges: controls ? [Math.round(controls.getBoundingClientRect().left), Math.round(controls.getBoundingClientRect().right)] : null,
      };
    });

    // The calendar body must not leave a gutter inside its own control: the input
    // fills the control, and the trigger sits inside the input's right edge.
    const fillsControl = geo.control && geo.input
      && Math.abs(geo.control.left - geo.input.left) <= 1
      && Math.abs(geo.control.right - geo.input.right) <= 1;
    const triggerInside = geo.input && geo.trigger
      && geo.trigger.right <= geo.input.right + 1
      && geo.trigger.right >= geo.input.right - 16;

    // The real complaint: the calendar stopped well short of the step buttons, so
    // it read as attached to neither. Anything wider than a normal 8px gutter
    // between the control and its next-door button is the bug.
    const gapRight = geo.control && geo.nextStep ? geo.nextStep.left - geo.control.right : null;
    const hugsNeighbours = gapRight !== null && gapRight >= 0 && gapRight <= 16;

    record(
      'D1 calendar fills its control and sits flush with the step buttons',
      Boolean(fillsControl && triggerInside && hugsNeighbours),
      `gapRight=${gapRight}px ${JSON.stringify(geo)}`,
    );

    await page.screenshot({ path: `${SHOTS}/dashboard-month.png`, clip: geo.controls ? {
      x: Math.max(0, geo.controls.left - 20),
      y: Math.max(0, geo.controls.top - 10),
      width: Math.min(600, geo.controls.width + 40),
      height: geo.controls.height + 20,
    } : undefined });
  }

  // The popover must stay inside the viewport when opened from the toolbar.
  {
    await page.click('.workspace-period .calendar-trigger');
    await page.waitForTimeout(200);
    const pop = await page.evaluate(() => {
      const el = document.querySelector('.calendar-popover');
      if (!el) return null;
      const r = el.getBoundingClientRect();
      const input = document.querySelector('.workspace-period .calendar-control');
      const ir = input ? input.getBoundingClientRect() : null;
      return {
        left: Math.round(r.left), right: Math.round(r.right), width: Math.round(r.width),
        inputLeft: ir ? Math.round(ir.left) : null, inputRight: ir ? Math.round(ir.right) : null,
        viewport: window.innerWidth,
      };
    });
    const inside = pop && pop.left >= 0 && pop.right <= pop.viewport;
    record(
      'D1 calendar popover stays inside the viewport',
      Boolean(inside),
      JSON.stringify(pop),
    );
    await page.screenshot({ path: `${SHOTS}/dashboard-month-open.png` });
    await page.keyboard.press('Escape');
    await page.waitForTimeout(120);
  }

  // ---------------------------------------------------------------------------
  // Requirement 2 — the property and status selects submit the filter form the
  // moment they change, and each keeps the other field's value.
  //
  // The fixtures seed no properties, so the property filter has nothing to pick
  // until one exists. Create it through the real UI rather than inserting rows.
  // ---------------------------------------------------------------------------
  {
    await page.goto(BASE + '/rent-dashboard', { waitUntil: 'networkidle' });
    const hasOption = await page.evaluate(() =>
      [...(document.querySelector('#workspace-property')?.options || [])].some((o) => o.value !== ''));
    if (!hasOption) {
      await page.goto(BASE + '/properties?add=1', { waitUntil: 'networkidle' });
      const drawer = page.locator('form[action="/properties"]').first();
      if ((await drawer.count()) > 0) {
        await drawer.locator('input[name="name"]').fill('探针房产 ' + Date.now());
        const address = drawer.locator('input[name="address"]');
        if (await address.count()) await address.fill('1 Probe Lane');
        await Promise.all([
          page.waitForNavigation({ waitUntil: 'networkidle' }).catch(() => {}),
          drawer.locator('button[type=submit], input[type=submit]').first().click(),
        ]);
      }
    }
  }

  for (const [label, selector, pick] of [
    ['property', '#workspace-property', 'first'],
    ['status', '#workspace-status', 'unpaid'],
  ]) {
    await page.goto(BASE + '/rent-dashboard', { waitUntil: 'networkidle' });
    const before = page.url();

    const chosen = await page.evaluate(
      ({ selector, pick }) => {
        const select = document.querySelector(selector);
        if (!select) return null;
        return [...select.options].map((o) => ({ value: o.value, text: o.textContent.trim() }))
          .find((o) => (pick === 'first' ? o.value !== '' : o.value === pick)) || null;
      },
      { selector, pick },
    );
    if (!chosen) {
      record(`D2 ${label} filter submits on change`, false, `no option to pick on ${selector}`);
      continue;
    }

    const navigated = page.waitForNavigation({ timeout: 5000 }).catch(() => null);
    await page.selectOption(selector, chosen.value);
    await navigated;
    await page.waitForTimeout(150);

    const url = new URL(page.url());
    const param = label === 'property' ? 'property_id' : 'status';
    const submitted = url.searchParams.get(param) === chosen.value;
    const kept = url.searchParams.get('period') !== null && url.searchParams.get('period') !== '';
    record(
      `D2 ${label} filter submits on change`,
      Boolean(submitted && kept && page.url() !== before),
      `picked ${JSON.stringify(chosen)} → ${url.pathname}${url.search} (period kept=${kept})`,
    );
  }

  // Both filters must survive each other: set property, then status.
  {
    await page.goto(BASE + '/rent-dashboard', { waitUntil: 'networkidle' });
    const first = await page.evaluate(() => {
      const s = document.querySelector('#workspace-property');
      const o = [...s.options].find((x) => x.value !== '');
      return o ? o.value : null;
    });
    if (first) {
      const nav1 = page.waitForNavigation({ timeout: 5000 }).catch(() => null);
      await page.selectOption('#workspace-property', first);
      await nav1;
      await page.waitForTimeout(150);
      const nav2 = page.waitForNavigation({ timeout: 5000 }).catch(() => null);
      await page.selectOption('#workspace-status', 'unpaid');
      await nav2;
      await page.waitForTimeout(150);
      const url = new URL(page.url());
      record(
        'D2 property choice survives a later status change',
        url.searchParams.get('property_id') === first && url.searchParams.get('status') === 'unpaid',
        `${url.pathname}${url.search}`,
      );
    } else {
      record('D2 property choice survives a later status change', false, 'no property option available');
    }
  }

  await ctx.close();
} finally {
  await browser.close();
}

const failed = results.filter((r) => !r.ok);
console.log(`\n${results.length - failed.length}/${results.length} checks passed`);
if (failed.length) console.log('failed: ' + failed.map((f) => f.name).join(' | '));
process.exit(failed.length ? 1 : 0);
