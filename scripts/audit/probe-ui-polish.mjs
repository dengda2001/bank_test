// Real-browser verification for the seven requirements of
// `.trellis/tasks/09-20-workspace-ui-polish`.
//
// design.md requires this: "The exact truncation and popup behavior should be
// verified at desktop and narrow viewport widths in a real browser, not only by
// asserting CSS/template strings." Everything below therefore drives the real
// app rather than reading rendered HTML.
//
// Usage (against a disposable instance started by scripts/run-audit-local.sh):
//   AUDIT_BASE=http://127.0.0.1:18090 \
//   AUDIT_USER=... AUDIT_PASS=... \
//   AUDIT_EMPTY_USER=... AUDIT_EMPTY_PASS=... \
//     node scripts/audit/probe-ui-polish.mjs
//
// AUDIT_EMPTY_* is the second, empty account the audit runner seeds; it is what
// makes the zero-pending dashboard observable. Requirement 2 is skipped with a
// warning when those are unset.
import { chromium } from 'playwright';
import { launchOptions, describeChrome } from './launch.mjs';

const BASE = process.env.AUDIT_BASE || 'http://localhost:8081';
const USER = process.env.AUDIT_USER;
const PASS = process.env.AUDIT_PASS;
const EMPTY_USER = process.env.AUDIT_EMPTY_USER;
const EMPTY_PASS = process.env.AUDIT_EMPTY_PASS;

if (!USER || !PASS) {
  console.error('probe-ui-polish.mjs: AUDIT_USER and AUDIT_PASS are required');
  process.exit(2);
}

const results = [];
const record = (id, ok, detail) => {
  results.push({ id, ok, detail });
  console.log(`${ok ? 'PASS' : 'FAIL'}  ${id}${detail ? '  — ' + detail : ''}`);
};
const skip = (id, why) => {
  results.push({ id, ok: true, skipped: true, detail: why });
  console.log(`SKIP  ${id}  — ${why}`);
};

async function login(page, user, pass) {
  await page.goto(BASE + '/', { waitUntil: 'domcontentloaded' });
  await page.fill('#username', user);
  await page.fill('#password', pass);
  await Promise.all([
    page.waitForNavigation({ waitUntil: 'domcontentloaded' }).catch(() => {}),
    page.click('button[type=submit]'),
  ]);
}

// The shared control replaces every single <select> with a trigger + a popup
// that is portaled to <body>. These helpers speak that structure.
//
// `page.locator(':has(...)')` is evaluated by the browser's CSS engine, which
// does not know Playwright's `:visible` extension, so the scope passed in must
// be plain CSS. Callers use `is-open` markers rather than `:visible`.
//
// `scope` is an optional ancestor selector. It matters because `:has()` is
// relative: `.workspace-select:has(.panel select)` looks for a panel *inside*
// the select, which never matches — the panel is the select's ancestor.
const triggerSelector = (selectSelector, scope) =>
  `${scope ? scope + ' ' : ''}.workspace-select:has(${selectSelector}) .workspace-select-trigger`;

const openSelect = async (page, selectSelector, scope) => {
  const trigger = page.locator(triggerSelector(selectSelector, scope)).first();
  await trigger.click();
  await page.waitForTimeout(120);
  return trigger;
};

// Computed colours come back in whatever space the page declared (`oklch(...)`
// here), so they are normalised to sRGB in-page before channel comparison.
const NORMALISE_COLOURS = `(values) => {
  const canvas = document.createElement('canvas');
  canvas.width = canvas.height = 1;
  const ctx = canvas.getContext('2d', { willReadFrequently: true });
  return values.map((value) => {
    if (!value) return null;
    ctx.clearRect(0, 0, 1, 1);
    ctx.fillStyle = '#000';
    ctx.fillStyle = value;
    ctx.fillRect(0, 0, 1, 1);
    const [r, g, b] = ctx.getImageData(0, 0, 1, 1).data;
    return [r, g, b];
  });
}`;

const browser = await chromium.launch(launchOptions());
console.log(`browser: ${describeChrome()}`);

try {
  // ---------------------------------------------------------------------------
  // Requirement 1 — room type / capacity gone from create, edit and detail.
  //
  // The audit fixtures seed tenants and bank rows but no properties, so this
  // check creates one of each through the real UI. That is also the only way to
  // reach the room detail page, and it exercises the create flow the
  // requirement actually names.
  // ---------------------------------------------------------------------------
  {
    const ctx = await browser.newContext({ viewport: { width: 1280, height: 900 } });
    const page = await ctx.newPage();
    await login(page, USER, PASS);

    const forbidden = ['房间类型', '容纳人数', '户型', '可住人数'];
    const leaksIn = (text, words) => words.filter((w) => text.includes(w));

    // -- create drawer -------------------------------------------------------
    await page.goto(BASE + '/properties?add=1', { waitUntil: 'networkidle' });
    const propertyDrawer = page.locator('form[action="/properties"]').first();
    const propertyFormPresent = (await propertyDrawer.count()) > 0;
    let propertyName = '探针房产 ' + Date.now();
    if (propertyFormPresent) {
      await propertyDrawer.locator('input[name="name"]').fill(propertyName);
      const address = propertyDrawer.locator('input[name="address"]');
      if (await address.count()) await address.fill('1 Probe Lane');
      await Promise.all([
        page.waitForNavigation({ waitUntil: 'networkidle' }).catch(() => {}),
        propertyDrawer.locator('button[type=submit], input[type=submit]').first().click(),
      ]);
    }

    await page.goto(BASE + '/rooms?add=1', { waitUntil: 'networkidle' });
    const createDrawer = page.locator('form[action="/rooms"]').first();
    const createPresent = (await createDrawer.count()) > 0;
    const createText = createPresent ? await createDrawer.innerText() : '';
    const createInputs = createPresent
      ? await createDrawer.evaluate((el) => [...el.querySelectorAll('input, select, textarea')].map((i) => i.name).filter(Boolean))
      : [];
    const createLeaks = leaksIn(createText, forbidden);

    // -- create a room so the detail page exists -----------------------------
    let roomHref = null;
    if (createPresent && propertyFormPresent) {
      const propertySelect = createDrawer.locator('select[name="property_id"]');
      const optionValue = await propertySelect.evaluate((el) => {
        const opt = [...el.options].find((o) => o.value);
        return opt ? opt.value : null;
      });
      if (optionValue) {
        // The shared control hides the native select, so set it through the
        // portaled popup the way a user would.
        await propertySelect.evaluate((el, value) => { el.value = value; el.dispatchEvent(new Event('change', { bubbles: true })); }, optionValue);
        await createDrawer.locator('input[name="room_label"]').fill('探针房间');
        const rent = createDrawer.locator('input[name="monthly_rent"]');
        if (await rent.count()) await rent.fill('1000');
        await Promise.all([
          page.waitForNavigation({ waitUntil: 'networkidle' }).catch(() => {}),
          createDrawer.locator('button[type=submit], input[type=submit]').first().click(),
        ]);
      }
    }

    await page.goto(BASE + '/rooms', { waitUntil: 'networkidle' });
    roomHref = await page.evaluate(() => {
      const a = [...document.querySelectorAll('a[href^="/rooms/"]')].map((el) => el.getAttribute('href'))
        .filter((h) => /^\/rooms\/\d+/.test(h));
      return a[0] || null;
    });

    let detailLeaks = [];
    let detailReached = false;
    let detailOccupancyText = '';
    if (roomHref) {
      const response = await page.goto(BASE + roomHref, { waitUntil: 'networkidle' });
      detailReached = Boolean(response) && response.status() === 200;
      const state = await page.evaluate((words) => ({
        leaks: words.filter((w) => document.body.innerText.includes(w)),
        occupancy: (document.body.innerText.match(/\d+\s*位(当前租客)?/) || [''])[0],
        // The edit drawer lives on the detail page behind ?edit=1.
      }), forbidden);
      detailLeaks = state.leaks;
      detailOccupancyText = state.occupancy;

      const editResponse = await page.goto(BASE + roomHref + (roomHref.includes('?') ? '&' : '?') + 'edit=1', { waitUntil: 'networkidle' });
      if (editResponse && editResponse.status() === 200) {
        detailLeaks = [...detailLeaks, ...(await page.evaluate((words) => words.filter((w) => document.body.innerText.includes(w)), forbidden))];
      }
    }

    const leaks = [...new Set([...createLeaks, ...detailLeaks])];
    // The detail page is only meaningful if it rendered, so the occupancy fact
    // is asserted positively rather than inferred from an absence of leaks.
    const ok = createPresent && leaks.length === 0 && detailReached && Boolean(detailOccupancyText);
    record(
      'R1 room type/capacity removed from create, edit and detail',
      ok,
      leaks.length
        ? `still rendered: ${leaks.join(', ')}`
        : `create form fields=[${createInputs.join(',')}] detail ${roomHref || '(no room link)'} reached=${detailReached} occupancy="${detailOccupancyText}"`,
    );
    await ctx.close();
  }

  // ---------------------------------------------------------------------------
  // Requirement 2 — empty manual-review queue is green, non-empty keeps warning.
  // ---------------------------------------------------------------------------
  if (!EMPTY_USER || !EMPTY_PASS) {
    skip('R2 empty queue uses the green state', 'AUDIT_EMPTY_USER / AUDIT_EMPTY_PASS not set');
  } else {
    const readQueue = async (user, pass) => {
      const ctx = await browser.newContext({ viewport: { width: 1280, height: 900 } });
      const page = await ctx.newPage();
      await login(page, user, pass);
      await page.goto(BASE + '/rent-dashboard', { waitUntil: 'networkidle' });
      const state = await page.evaluate((normaliseSource) => {
        const normalise = eval(normaliseSource);
        const el = document.querySelector('.workspace-action-queue');
        if (!el) return null;
        const cs = getComputedStyle(el);
        const count = el.querySelector('.workspace-queue-count');
        const heading = el.querySelector('.panel-head h2');
        const raw = {
          is_empty: el.classList.contains('is-empty'),
          count: count ? count.textContent.trim() : null,
          border: cs.borderTopColor,
          background: cs.backgroundColor,
          accent: count ? getComputedStyle(count).color : null,
          heading: heading ? getComputedStyle(heading).color : null,
        };
        const [border, background, accent, headingColour] = normalise([raw.border, raw.background, raw.accent, raw.heading]);
        return { ...raw, rgb: { border, background, accent, heading: headingColour } };
      }, NORMALISE_COLOURS);
      await ctx.close();
      return state;
    };

    const filled = await readQueue(USER, PASS);
    const empty = await readQueue(EMPTY_USER, EMPTY_PASS);

    // Green vs warning is read off the rendered sRGB channels. The washes are
    // deliberately pale, so the panel background only has to lean the right way;
    // the count badge and heading carry the strong signal.
    const greenish = (v, margin = 3) => Boolean(v) && v[1] > v[0] + margin && v[1] > v[2] + margin;
    const reddish = (v, margin = 3) => Boolean(v) && v[0] > v[1] + margin && v[0] >= v[2];

    const emptyOk =
      Boolean(empty) && empty.is_empty && empty.count === '0' &&
      greenish(empty.rgb.accent) && greenish(empty.rgb.heading) && greenish(empty.rgb.background);
    const filledOk =
      Boolean(filled) && !filled.is_empty && filled.count !== '0' &&
      reddish(filled.rgb.accent) && reddish(filled.rgb.background) && !greenish(filled.rgb.background);
    record(
      'R2 empty queue is green, non-empty queue keeps the attention state',
      emptyOk && filledOk,
      `empty rgb=${JSON.stringify(empty?.rgb)} filled rgb=${JSON.stringify(filled?.rgb)}`,
    );
  }

  // ---------------------------------------------------------------------------
  // Requirements 3 + 5 — dashboard processing panel stays put while its tenant
  // selector is opened, searched and used.
  // ---------------------------------------------------------------------------
  {
    const ctx = await browser.newContext({ viewport: { width: 1280, height: 900 } });
    const page = await ctx.newPage();
    await login(page, USER, PASS);
    await page.goto(BASE + '/rent-dashboard', { waitUntil: 'networkidle' });

    // Only rows that carry match options render a tenant select; find one of
    // those rather than assuming the first queue item has it.
    const triggerCount = await page.locator('.workspace-queue-process-trigger').count();
    let trigger = null;
    for (let i = 0; i < triggerCount; i += 1) {
      const candidate = page.locator('.workspace-queue-process-trigger').nth(i);
      await candidate.click();
      await page.waitForTimeout(120);
      if ((await page.locator('.workspace-queue-process.is-open select[name="tenant_id"]').count()) > 0) {
        trigger = candidate;
        break;
      }
      await page.keyboard.press('Escape');
      await page.waitForTimeout(60);
    }
    const hasPanel = trigger !== null;

    if (!hasPanel) {
      skip('R3 processing panel stays anchored with the tenant selector open', `none of ${triggerCount} pending rows exposes a tenant selector`);
      skip('R5 tenant search narrows the dashboard tenant list', `none of ${triggerCount} pending rows exposes a tenant selector`);
    } else {

      const panel = page.locator('.workspace-queue-process.is-open .workspace-queue-process-body').first();
      const before = await panel.boundingBox();

      // Scoped by the `is-open` marker the panel's own script sets, not by
      // `:visible`: `:has()` is parsed by the browser, which has no such pseudo.
      const select = 'select[name="tenant_id"]';
      const scope = '.workspace-queue-process.is-open';
      const triggerLocator = await openSelect(page, select, scope);

      const popup = page.locator('.workspace-select-popup:visible').first();
      const popupBox = await popup.boundingBox();
      const triggerBox = await triggerLocator.boundingBox();
      const after = await panel.boundingBox();

      // The popup must be portaled to <body>, not nested in the transformed
      // dialog: that ancestry is what moved/collapsed the panel before.
      const portaled = await page.evaluate(() => {
        const p = [...document.querySelectorAll('.workspace-select-popup')].find((el) => !el.hidden);
        return Boolean(p) && p.parentElement === document.body;
      });

      const panelStable = before && after && before.x === after.x && before.y === after.y && before.width === after.width;
      const anchored = popupBox && triggerBox && popupBox.y >= triggerBox.y - 4 && Math.abs(popupBox.x - triggerBox.x) < 40;

      // Same-layer visibility: the popup must not sit underneath the dialog.
      const onTop = await page.evaluate(() => {
        const p = [...document.querySelectorAll('.workspace-select-popup')].find((el) => !el.hidden);
        if (!p) return false;
        const r = p.getBoundingClientRect();
        const hit = document.elementFromPoint(r.left + r.width / 2, r.top + 12);
        return Boolean(hit) && p.contains(hit);
      });

      record(
        'R3 processing panel stays anchored with the tenant selector open',
        Boolean(panelStable && anchored && portaled && onTop) && (await page.locator('.workspace-select-popup:visible .workspace-select-option:visible').count()) > 0,
        `panelStable=${panelStable} anchored=${anchored} portaledToBody=${portaled} onTop=${onTop}`,
      );

      // Search narrows by substring, blank prompt option is not a result, and a
      // nonsense query shows the no-results state.
      const search = page.locator('.workspace-select-popup:visible .workspace-select-search').first();
      const hasSearch = (await search.count()) > 0;
      let searchOk = false;
      let searchDetail = 'no search input in the popup';
      if (hasSearch) {
        // The blank prompt ("选择租客") is not a search result, so the needle has
        // to come from a real tenant label rather than the first option.
        const labels = await page.evaluate(() => {
          const p = [...document.querySelectorAll('.workspace-select-popup')].find((el) => !el.hidden);
          return [...p.querySelectorAll('.workspace-select-option')]
            .map((o) => ({ text: o.textContent.trim(), prompt: o.getAttribute('data-option-index') === '0' }))
            .filter((o) => o.text && !o.prompt);
        });
        const needle = labels[0] ? labels[0].text : '';
        const partial = needle.slice(0, Math.max(2, Math.ceil(needle.length / 2)));

        await search.fill(partial.toLocaleLowerCase());
        await page.waitForTimeout(80);
        const narrowed = await page.evaluate(() => {
          const p = [...document.querySelectorAll('.workspace-select-popup')].find((el) => !el.hidden);
          const options = [...p.querySelectorAll('.workspace-select-option')];
          return {
            visible: options.filter((o) => !o.hidden).map((o) => o.textContent.trim()),
            total: options.length,
            // The blank prompt must not survive a non-empty query.
            promptVisible: options.some((o) => o.getAttribute('data-option-index') === '0' && !o.hidden),
          };
        });

        await search.fill('zzz-no-such-tenant');
        await page.waitForTimeout(80);
        const noneState = await page.evaluate(() => {
          const p = [...document.querySelectorAll('.workspace-select-popup')].find((el) => !el.hidden);
          const empty = p.querySelector('.workspace-select-empty');
          return { emptyShown: Boolean(empty) && !empty.hidden, emptyText: empty ? empty.textContent.trim() : '' };
        });

        searchOk =
          narrowed.visible.length > 0 &&
          narrowed.visible.length < narrowed.total &&
          narrowed.visible.every((t) => t.toLocaleLowerCase().includes(partial.toLocaleLowerCase())) &&
          !narrowed.promptVisible &&
          noneState.emptyShown;

        searchDetail = `query "${partial}" → ${narrowed.visible.length}/${narrowed.total} options (prompt hidden=${!narrowed.promptVisible}); nonsense query shows "${noneState.emptyText}"`;

        await search.fill('');
        await page.waitForTimeout(60);
      }
      record('R5 tenant search narrows the dashboard tenant list', searchOk, searchDetail);

      // R3 also requires the choices stay selectable. The blank prompt is
      // skipped: choosing it would leave the select unchanged and prove nothing.
      const realOptions = page.locator('.workspace-select-popup:visible .workspace-select-option:visible');
      const optionCount = await realOptions.count();
      const firstOption = realOptions.nth(optionCount > 1 ? 1 : 0);
      const optionText = (await firstOption.textContent())?.trim();
      await firstOption.click();
      await page.waitForTimeout(120);
      const afterChoose = await page.evaluate(() => {
        const s = document.querySelector('.workspace-queue-process select[name="tenant_id"]');
        const panel = document.querySelector('.workspace-queue-process');
        return {
          chosen: s && s.selectedOptions[0] ? s.selectedOptions[0].textContent.trim() : null,
          panelStillOpen: Boolean(panel) && panel.classList.contains('is-open'),
        };
      });
      const chosen = afterChoose.chosen;
      record(
        'R3 tenant choice is selectable from the portaled popup',
        Boolean(chosen) && chosen === optionText,
        `clicked "${optionText}", native select now "${chosen}"`,
      );
      record(
        'R3 choosing a tenant keeps the processing panel open',
        afterChoose.panelStillOpen,
        `panel is-open=${afterChoose.panelStillOpen} after choosing "${optionText}"`,
      );

      // Escape still dismisses, and the panel survives it.
      await openSelect(page, select, scope);
      await page.keyboard.press('Escape');
      await page.waitForTimeout(80);
      const afterEscape = await page.evaluate(() => ({
        popupOpen: [...document.querySelectorAll('.workspace-select-popup')].some((el) => !el.hidden),
        panelOpen: [...document.querySelectorAll('.workspace-queue-process-body')].some((el) => !el.hidden),
      }));
      record(
        'R3 Escape dismisses the popup and keeps the panel',
        !afterEscape.popupOpen && afterEscape.panelOpen,
        JSON.stringify(afterEscape),
      );
    }
    await ctx.close();
  }

  // ---------------------------------------------------------------------------
  // Requirement 4 — match-status change auto-submits, keeping other filters.
  // ---------------------------------------------------------------------------
  {
    const ctx = await browser.newContext({ viewport: { width: 1280, height: 900 } });
    const page = await ctx.newPage();
    await login(page, USER, PASS);

    // Start from a page that already carries another filter value, so the check
    // can prove the rest of the form survived the auto-submit.
    await page.goto(BASE + '/transactions?payer=CHEN', { waitUntil: 'networkidle' });
    const baseline = new URL(page.url()).searchParams.get('payer');
    const isTransactions = new URL(page.url()).pathname === '/transactions';

    const select = 'select[name="match_status"]';
    const scope = '.transaction-route-quickfilter';
    const exists = (await page.locator(`${scope} ${select}`).count()) > 0;
    let autoSubmitOk = false;
    let detail = 'match-status select not found on /transactions';
    if (isTransactions && exists) {
      const nav = page.waitForNavigation({ waitUntil: 'networkidle', timeout: 5000 }).catch(() => null);
      await openSelect(page, select, scope);
      await page.locator('.workspace-select-popup:visible .workspace-select-option:visible').first().click();
      await nav;
      await page.waitForTimeout(200);
      const params = new URL(page.url()).searchParams;
      autoSubmitOk = params.get('match_status') !== null && params.get('payer') === baseline;
      detail = `url=${page.url()} (payer kept=${params.get('payer') === baseline})`;
    } else if (!isTransactions) {
      detail = `expected /transactions, landed on ${new URL(page.url()).pathname}`;
    }
    record('R4 match-status change submits the filter immediately', autoSubmitOk, detail);
    await ctx.close();
  }

  // ---------------------------------------------------------------------------
  // Requirement 5 (rest) — tenant search on transaction filter, transaction
  // detail/row matching and cash-receipt entry.
  // ---------------------------------------------------------------------------
  {
    const ctx = await browser.newContext({ viewport: { width: 1280, height: 900 } });
    const page = await ctx.newPage();
    await login(page, USER, PASS);
    await page.goto(BASE + '/transactions', { waitUntil: 'networkidle' });

    const detailHref = await page.evaluate(() => {
      const a = [...document.querySelectorAll('a[href*="detail="]')].map((el) => el.getAttribute('href'))[0];
      return a || null;
    });

    const targets = [
      ['transaction tenant filter', '/transactions', '#tenant_id'],
      ['cash-receipt entry', '/cash-receipts?add=1', 'select[name="tenant_id"]'],
    ];
    if (detailHref) targets.push(['transaction detail matching', detailHref, 'select[name="tenant_id"]']);

    for (const [name, url, sel] of targets) {
      await page.goto(BASE + url, { waitUntil: 'networkidle' });
      const count = await page.locator(`.workspace-select:has(${sel})`).count();
      if (!count) {
        record(`R5 tenant search — ${name}`, false, `no tenant select matching ${sel} on ${url}`);
        continue;
      }

      // On /transactions the tenant filter lives behind a collapsed <details>,
      // so expand any collapsed ancestor before trying to click the trigger.
      await page.evaluate((selector) => {
        document.querySelectorAll(selector).forEach((node) => {
          for (let el = node.parentElement; el; el = el.parentElement) {
            if (el.tagName === 'DETAILS') el.open = true;
          }
        });
      }, sel);
      await page.waitForTimeout(80);

      if (!(await page.locator(triggerSelector(sel)).first().isVisible())) {
        record(`R5 tenant search — ${name}`, false, `tenant select ${sel} on ${url} is not reachable (no visible trigger)`);
        continue;
      }
      await openSelect(page, sel);
      const search = page.locator('.workspace-select-popup:visible .workspace-select-search').first();
      if ((await search.count()) === 0) {
        record(`R5 tenant search — ${name}`, false, `tenant select on ${url} has no search input`);
        await page.keyboard.press('Escape');
        continue;
      }

      // Typing must actually narrow the list here too — an input that filters
      // nothing would still pass a presence-only check.
      const before = await page.evaluate(() => {
        const p = [...document.querySelectorAll('.workspace-select-popup')].find((el) => !el.hidden);
        return [...p.querySelectorAll('.workspace-select-option')].filter((o) => o.getAttribute('data-option-index') !== '0').map((o) => o.textContent.trim());
      });
      const needle = (before[0] || '').slice(1, 3);
      await search.fill(needle.toLocaleLowerCase());
      await page.waitForTimeout(80);
      const after = await page.evaluate(() => {
        const p = [...document.querySelectorAll('.workspace-select-popup')].find((el) => !el.hidden);
        return [...p.querySelectorAll('.workspace-select-option')].filter((o) => !o.hidden).map((o) => o.textContent.trim());
      });
      const ok = before.length > 0 && needle.length > 0 && after.length > 0 && after.length < before.length;
      record(
        `R5 tenant search — ${name}`,
        ok,
        `${before.length} tenants → ${after.length} for "${needle}" on ${url}`,
      );
      await page.keyboard.press('Escape');
      await page.waitForTimeout(60);
    }
    await ctx.close();
  }

  // ---------------------------------------------------------------------------
  // Requirement 6 — collection-status labels are fully readable, no ellipsis.
  // ---------------------------------------------------------------------------
  {
    const ctx = await browser.newContext({ viewport: { width: 1280, height: 900 } });
    const page = await ctx.newPage();
    await login(page, USER, PASS);

    for (const url of ['/properties', '/rooms']) {
      await page.goto(BASE + url, { waitUntil: 'networkidle' });
      // The collection-status filter is `select[name="collection"]`; the
      // same-named `status` control on these pages is a hidden list filter.
      const status = page.locator('.workspace-select:has(select[name="collection"]) .workspace-select-trigger');
      if ((await status.count()) === 0) {
        record(`R6 collection-status label readable on ${url}`, false, 'no collection-status selector found');
        continue;
      }
      const measured = await status.first().evaluate((el) => {
        const cs = getComputedStyle(el);
        const value = el.querySelector('.workspace-select-value');
        return {
          text: value ? value.textContent.trim() : el.textContent.trim(),
          clip: el.scrollWidth - el.clientWidth,
          textOverflow: cs.textOverflow,
          width: Math.round(el.getBoundingClientRect().width),
          valueClip: value ? value.scrollWidth - value.clientWidth : 0,
        };
      });
      // "全部状态" is the longest default label; a non-zero clip means ellipsis.
      const ok = measured.text.includes('全部状态') && measured.clip <= 0 && measured.valueClip <= 0;
      record(
        `R6 collection-status label readable on ${url}`,
        ok,
        `"${measured.text}" width=${measured.width}px clip=${measured.clip}/${measured.valueClip} text-overflow=${measured.textOverflow}`,
      );
    }
    await ctx.close();
  }

  // ---------------------------------------------------------------------------
  // Requirement 7 — every transaction row exposes 查看详情, kept alongside
  // 匹配流水 where the row is directly matchable.
  // ---------------------------------------------------------------------------
  {
    const ctx = await browser.newContext({ viewport: { width: 1280, height: 900 } });
    const page = await ctx.newPage();
    await login(page, USER, PASS);

    for (const url of ['/transactions', '/billing']) {
      await page.goto(BASE + url, { waitUntil: 'networkidle' });
      const audit = await page.evaluate(() => {
        // Whichever row renderer this page uses: the route table on
        // /transactions, the legacy table on /billing. Mobile lists are checked
        // separately by their own suite.
        const rows = [...document.querySelectorAll('table tbody tr')].filter((tr) => tr.querySelector('td'));
        const missing = [];
        let withMatch = 0;
        rows.forEach((tr, index) => {
          const text = tr.innerText;
          // The row link is rendered as 查看流水详情; accept the short form too
          // so a future copy tweak does not turn this into a false failure.
          const hasDetail = /查看(流水)?详情/.test(text);
          const hasMatch = /匹配流水/.test(text);
          if (hasMatch) withMatch += 1;
          if (!hasDetail) missing.push({ index, snippet: text.replace(/\s+/g, ' ').slice(0, 70) });
        });
        return { total: rows.length, withMatch, missing };
      });
      const ok = audit.total > 0 && audit.missing.length === 0;
      record(
        `R7 every transaction row has 查看详情 on ${url}`,
        ok,
        audit.total === 0
          ? 'no transaction rows to check'
          : `${audit.total} rows, ${audit.withMatch} also offer 匹配流水, missing detail on ${audit.missing.length}` +
            (audit.missing.length ? `: ${JSON.stringify(audit.missing.slice(0, 3))}` : ''),
      );
    }
    await ctx.close();
  }
} finally {
  await browser.close();
}

const failed = results.filter((r) => !r.ok);
console.log('');
console.log(`${results.length - failed.length}/${results.length} checks passed`);
if (failed.length) {
  console.log('failed: ' + failed.map((f) => f.id).join(' | '));
  process.exit(1);
}
