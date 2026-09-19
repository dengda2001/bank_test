// Independent post-fix verification for 09-20, written by the main session.
//
// This deliberately does NOT reuse the implementing agent's probes
// (probe-mobile-filter-toggle.mjs / probe-notice-convergence.mjs). Its job is to
// re-derive the three defects' acceptance criteria from the PRD text, so a
// mistaken assumption shared by the implementer and its own probe cannot pass
// unnoticed. Where the two agree, the result is worth more than either alone.
//
// Assertions, in PRD order:
//   A. <=640px: /properties and /rooms open and close the filter panel.
//   B. >=641px: the same two pages keep the toggle hidden and the fields visible.
//   C. the filter form has exactly ONE submit path (a bare `change` submits once).
//   D. /tenancies has no toggle at either width and still submits (reverse assertion).
//   E. one error code renders exactly one visible message.
//   F. no URL query parameter is ever rendered as toast text.
//
// Read-only except for C and D, which submit the *filter* form (a GET that
// re-renders the list). No mutation form is ever submitted.
import { chromium } from 'playwright';
import { launchOptions } from './launch.mjs';

const BASE = (process.env.AUDIT_BASE || 'http://127.0.0.1:18097').replace(/\/+$/, '');
const USER = process.env.AUDIT_USER;
const PASS = process.env.AUDIT_PASS;
const PERIOD = process.env.AUDIT_PERIOD || '2026-09';

// A marker that cannot occur in any real page text, so "is it on the page" is
// unambiguous. Kept ASCII and HTML-safe: html/template would escape anything
// exotic, and an escaped marker would make a positive finding look negative.
const MARKER = 'INJECT-7f3a91';

if (!USER || !PASS) {
  console.error('set AUDIT_USER and AUDIT_PASS (see the launcher output)');
  process.exit(1);
}

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
const pageErrors = [];
page.on('pageerror', (e) => pageErrors.push(String(e).slice(0, 160)));

await page.goto(BASE + '/', { waitUntil: 'domcontentloaded' });
if (await page.$('#username')) {
  await page.fill('#username', USER);
  await page.fill('#password', PASS);
  await Promise.all([page.waitForNavigation({ waitUntil: 'domcontentloaded' }), page.click('button[type=submit]')]);
}
if (await page.$('#username')) {
  console.error('login did not take: still on the login form');
  await browser.close();
  process.exit(1);
}

// Reads the toggle/panel pair in one shot so the two can never be observed
// from different moments (a click in between would make them inconsistent).
const toggleState = () =>
  page.evaluate(() => {
    const btn = document.querySelector('.object-list-filter-toggle');
    const panel = document.querySelector('.object-list-filter-fields');
    return {
      present: !!btn,
      aria: btn ? btn.getAttribute('aria-expanded') : null,
      btnDisplay: btn ? getComputedStyle(btn).display : null,
      panelDisplay: panel ? getComputedStyle(panel).display : null,
      fieldsVisible: panel ? getComputedStyle(panel).display !== 'none' : null,
    };
  });

// ---- A / B: the toggle on the two pages that have one ----------------------
for (const path of ['/properties', '/rooms']) {
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto(withQuery(path, { period: PERIOD }), { waitUntil: 'networkidle' });
  const before = await toggleState();
  const urlBefore = page.url();
  await page.click('.object-list-filter-toggle');
  await page.waitForTimeout(80);
  const open = await toggleState();
  await page.click('.object-list-filter-toggle');
  await page.waitForTimeout(80);
  const closed = await toggleState();
  const urlAfter = page.url();

  record(
    `A1 ${path} @390 有筛选开关`,
    before.present && before.aria === 'false' && before.panelDisplay === 'none',
    `present=${before.present} aria=${before.aria} panel=${before.panelDisplay}`,
  );
  record(
    `A2 ${path} @390 首点展开`,
    open.aria === 'true' && open.panelDisplay === 'grid',
    `aria=${open.aria} panel=${open.panelDisplay}`,
  );
  record(
    `A3 ${path} @390 再点收起`,
    closed.aria === 'false' && closed.panelDisplay === 'none',
    `aria=${closed.aria} panel=${closed.panelDisplay}`,
  );
  record(`A4 ${path} @390 开关不触发导航`, urlBefore === urlAfter, urlAfter.replace(BASE, ''));

  await page.setViewportSize({ width: 1440, height: 900 });
  await page.goto(withQuery(path, { period: PERIOD }), { waitUntil: 'networkidle' });
  const wide = await toggleState();
  record(
    `B1 ${path} @1440 按钮隐藏、字段可见`,
    wide.btnDisplay === 'none' && wide.fieldsVisible === true,
    `btn=${wide.btnDisplay} fields=${wide.fieldsVisible ? 'visible' : 'hidden'}`,
  );

  // ---- C: exactly one submit path -----------------------------------------
  // A bare `change` is what a keyboard user produces. Count the requests the
  // browser actually issues: 1 means one binding owns the submit, 2 means the
  // shell binding and an inline onchange are both alive and duplicate it.
  // Two things must hold for this to measure anything, and both cost a run when
  // they are assumed rather than checked: the control must have an option that
  // DIFFERS from the current value (selecting the already-selected option fires
  // no `change`), and it must not be the first select if that one is degenerate
  // (rooms' first select is the property filter, which can legitimately hold
  // only its "全部房产" placeholder).
  const pick = await page.evaluate(() => {
    const selects = [...document.querySelectorAll('.object-list-filter-fields select')];
    for (const el of selects) {
      const values = [...el.options].map((o) => o.value);
      const target = values.find((v) => v !== el.value);
      if (target !== undefined) return { found: true, name: el.name || el.getAttribute('aria-label'), target, count: selects.length };
    }
    return { found: false, count: selects.length, shapes: selects.map((el) => [...el.options].map((o) => o.value)) };
  });

  if (!pick.found) {
    record(`C1 ${path} @1440 一次 change 只提交一次`, false, `no select with an alternative option: ${JSON.stringify(pick)}`);
  } else {
    let requests = 0;
    const count = (req) => {
      if (req.url().startsWith(BASE + path)) requests += 1;
    };
    page.on('request', count);
    const [nav] = await Promise.all([
      page.waitForNavigation({ waitUntil: 'networkidle', timeout: 8000 }).catch(() => null),
      page.selectOption(`.object-list-filter-fields select[name="${pick.name}"]`, pick.target).catch(() => null),
    ]);
    await page.waitForTimeout(400);
    page.off('request', count);
    record(
      `C1 ${path} @1440 一次 change 只提交一次`,
      nav !== null && requests === 1,
      `${pick.name} -> "${pick.target}" navigated=${nav !== null} requests=${requests}`,
    );
  }
}

// ---- D: /tenancies has no toggle and still submits -------------------------
// At <=640 the whole .tenancy-filters form is `display: none` (tenancies.css:55)
// until the compact-bar search icon adds .mobile-search-open. That is the page's
// own disclosure, not the toggle this task fixed -- so at 390 the probe must open
// it first, exactly as a user would, and D2 asserts the submit that follows.
for (const [width, height] of [[390, 844], [1440, 900]]) {
  await page.setViewportSize({ width, height });
  await page.goto(withQuery('/tenancies', { period: PERIOD }), { waitUntil: 'networkidle' });
  const hasToggle = !!(await page.$('.object-list-filter-toggle'));
  record(`D1 /tenancies @${width} 没有开关`, !hasToggle, `hasToggle=${hasToggle}`);

  let revealed = true;
  if (width <= 640) {
    revealed = await page.evaluate(() => {
      const form = document.querySelector('.tenancy-filters');
      return !!form && getComputedStyle(form).display !== 'none';
    });
    if (!revealed) {
      await page.click('[data-mobile-search]').catch(() => null);
      await page.waitForTimeout(80);
      revealed = await page.evaluate(() => {
        const form = document.querySelector('.tenancy-filters');
        return !!form && getComputedStyle(form).display !== 'none';
      });
    }
  }

  const submit = await page.$('.object-list-filters button[type=submit], .object-list-filters input[type=submit]');
  if (!submit) {
    record(`D2 /tenancies @${width} 能提交筛选`, false, 'no submit control found');
  } else {
    const [nav] = await Promise.all([
      page.waitForNavigation({ waitUntil: 'networkidle', timeout: 8000 }).catch(() => null),
      submit.click().catch(() => null),
    ]);
    record(
      `D2 /tenancies @${width} 能提交筛选`,
      nav !== null,
      `revealedFirst=${revealed} navigated=${nav !== null}`,
    );
  }
}

// ---- E: one error code, one visible message --------------------------------
// The message text lives in the template, so the assertion is on the rendered
// page: no sentence about the error may appear twice.
const countVisible = (needle) =>
  page.evaluate((n) => {
    const visible = (el) => {
      if (!el) return false;
      const s = getComputedStyle(el);
      if (s.display === 'none' || s.visibility === 'hidden') return false;
      const r = el.getBoundingClientRect();
      return r.width > 0 && r.height > 0;
    };
    // Only leaf-ish notice boxes, so a wrapping container cannot double-count.
    return [...document.querySelectorAll('.notice')]
      .filter((el) => visible(el) && el.textContent.includes(n))
      .length;
  }, needle);

for (const [code, needle] of [
  ['cash_overbalance', '未收余额'],
  ['cash_receipt_failed', '现金补录'],
]) {
  await page.setViewportSize({ width: 1440, height: 900 });
  await page.goto(withQuery('/cash-receipts', { period: PERIOD, status: 'all', error: code, add: '1' }), {
    waitUntil: 'networkidle',
  });
  const n = await countVisible(needle);
  record(`E1 ${code} 只渲染一条可见消息`, n === 1, `visible notices containing "${needle}": ${n}`);
}

// The inline template is a second, parallel route; it must also show the code
// exactly once, and its sentence must be the same one the list page shows.
// The needle is per-code: the overbalance sentence never says "检查", so a shared
// needle would read 0 and look like a failure on a correct page.
await page.setViewportSize({ width: 1440, height: 900 });
for (const [code, needle] of [
  ['cash_overbalance', '未收余额'],
  ['cash_receipt_failed', '检查'],
]) {
  await page.goto(withQuery('/cash-receipts/new', { period: PERIOD, error: code }), { waitUntil: 'networkidle' });
  const n = await countVisible(needle);
  record(`E2 ${code} 内联路由只渲染一条可见消息`, n === 1, `visible notices containing "${needle}": ${n}`);
}

// Wording convergence: whatever the list page shows must equal what the inline
// route shows, otherwise one code still has two sentences in the repository.
// Reads the page's current notice text; the caller navigates first, so this
// deliberately takes no arguments.
const sentenceOn = () =>
  page.evaluate(() =>
    [...document.querySelectorAll('.notice')].map((el) => el.textContent.trim()).filter(Boolean).join(' | '),
  );
const listWording = {};
for (const code of ['cash_overbalance', 'cash_receipt_failed']) {
  await page.goto(withQuery('/cash-receipts', { period: PERIOD, status: 'all', error: code, add: '1' }), {
    waitUntil: 'networkidle',
  });
  listWording[code] = await sentenceOn();
}
const inlineWording = {};
for (const code of ['cash_overbalance', 'cash_receipt_failed']) {
  await page.goto(withQuery('/cash-receipts/new', { period: PERIOD, error: code }), { waitUntil: 'networkidle' });
  inlineWording[code] = await sentenceOn();
}
for (const code of ['cash_overbalance', 'cash_receipt_failed']) {
  record(
    `E3 ${code} 两条渲染路径措辞一致`,
    listWording[code] !== '' && listWording[code] === inlineWording[code],
    `list="${listWording[code]}" inline="${inlineWording[code]}"`,
  );
}

// ---- F: a query parameter is never rendered as toast text ------------------
const toastText = () =>
  page.evaluate(() => {
    const el = document.querySelector('#workspace-toast');
    return el ? el.textContent.trim() : null;
  });

// /bills is the fourth instance of the same defect, found by the verification
// pass rather than by the PRD's list: its toast guard was `{{if .Message}}` with
// the text chosen by a mapping func whose default fell through to the raw code,
// so the literal `{{.Message}}` scan could not see it. It is asserted here
// because a fix nobody re-measures is a fix nobody has.
for (const path of ['/bank', '/rent-workspace', '/bills']) {
  await page.goto(withQuery(path, { period: PERIOD, message: MARKER }), { waitUntil: 'networkidle' });
  const body = await page.evaluate(() => document.body.innerText);
  const toasts = await toastText();
  const okNotices = await page.evaluate(() =>
    [...document.querySelectorAll('.notice.ok')].map((el) => el.textContent.trim()),
  );
  record(
    `F1 ${path}?message=<注入文本> 不回显`,
    !body.includes(MARKER) && !(toasts || '').includes(MARKER) && !okNotices.join(' ').includes(MARKER),
    `inBody=${body.includes(MARKER)} toast=${JSON.stringify(toasts)} okNotices=${JSON.stringify(okNotices)}`,
  );
}

// The positive leg: the whitelisted code must still render its own sentence.
// Asserted on the DOM rather than on visibility, because the shared script
// hides the marked notice and replays it as a 2.2s toast -- a visibility read
// would race that dismissal and report a false failure.
await page.goto(withQuery('/bank', { period: PERIOD, message: 'refreshed' }), { waitUntil: 'networkidle' });
const refreshed = await page.evaluate(() => {
  const el = document.querySelector('.notice.ok[data-toast]');
  return el ? el.textContent.trim() : null;
});
record(
  'F2 正规路径 /bank?message=refreshed 仍显示白名单文案',
  refreshed === '银行数据已刷新。',
  `marked notice=${JSON.stringify(refreshed)}`,
);

// A code that is not whitelisted must produce nothing at all.
await page.goto(withQuery('/bank', { period: PERIOD, message: 'not_a_real_code' }), { waitUntil: 'networkidle' });
const unknown = await page.evaluate(() => document.querySelectorAll('.notice.ok').length);
record('F3 未登记的错误码不渲染任何成功提示', unknown === 0, `.notice.ok count=${unknown}`);

// The positive leg for /bills: tightening the message mapping must not cost the
// three codes real handlers redirect with. Asserted on the marked notice rather
// than on visibility, for the same reason as /bank: the shared script replays it
// as a toast and hides the notice.
for (const [code, want] of [
  ['bills_generated', '本月账单已生成'],
  ['manual_balance_not_needed', '无需平账'],
]) {
  await page.goto(withQuery('/bills', { period: PERIOD, message: code }), { waitUntil: 'networkidle' });
  const text = await page.evaluate(() => {
    const el = document.querySelector('.notice.ok[data-toast]');
    return el ? el.textContent.trim() : null;
  });
  record(`F4 /bills?message=${code} 仍渲染白名单文案`, (text || '').includes(want), `marked notice=${JSON.stringify(text)}`);
}

// The reverse assertion: /bills' *error* leg deliberately keeps the raw-code
// fall-through (list_pages_alignment_test.go pins it, and a red banner is not the
// affordance a crafted link abuses). Tightening the message leg must not have
// tightened this one -- if it did, the fix over-reached and silently blanked a
// contract another test depends on.
await page.goto(withQuery('/bills', { period: PERIOD, error: 'invalid_period' }), { waitUntil: 'networkidle' });
const rawError = await page.evaluate(() =>
  [...document.querySelectorAll('.notice.error')].map((el) => el.textContent.trim()),
);
record(
  'F5 /bills 的红条仍原样回退错误码（未修过头）',
  rawError.join(' ').includes('invalid_period'),
  `error notices=${JSON.stringify(rawError)}`,
);

// ---- G: the nav bindings the implementer moved must still work -------------
// Defect 1's fix moved more than the reported lookup: the drawer/flyout/Escape
// bindings went into the same deferred block. If that move broke one of them,
// only a real click would show it, so both are exercised here.
await page.setViewportSize({ width: 390, height: 844 });
await page.goto(withQuery('/rent-dashboard', { period: PERIOD }), { waitUntil: 'networkidle' });

const flyout = await page.evaluate(async () => {
  const button = document.querySelector('.mobile-bottom-nav [data-mobile-menu]');
  if (!button) return { found: false };
  const menu = document.getElementById('mobile-menu-' + button.dataset.mobileMenu);
  button.click();
  await new Promise((r) => setTimeout(r, 60));
  const opened = button.getAttribute('aria-expanded') === 'true' && menu && menu.hidden === false;
  document.dispatchEvent(new KeyboardEvent('keydown', { key: 'Escape', bubbles: true }));
  await new Promise((r) => setTimeout(r, 60));
  const closed = button.getAttribute('aria-expanded') === 'false' && (!menu || menu.hidden === true);
  return { found: true, opened, closed };
});
record('G1 底部导航 flyout 开合仍正常', flyout.found && flyout.opened && flyout.closed, JSON.stringify(flyout));

// G2 needs a page whose <main> actually holds a search input -- the handler
// bails out when none is found, so running it on /rent-dashboard would fail for
// a reason that has nothing to do with the binding. /tenancies is the strongest
// case: its whole filter form is display:none at <=640, so a working binding
// must both add .mobile-search-open AND make the form visible.
await page.goto(withQuery('/tenancies', { period: PERIOD }), { waitUntil: 'networkidle' });
const search = await page.evaluate(async () => {
  const button = document.querySelector('[data-mobile-search]');
  const input = document.querySelector('main input[type=search]');
  if (!button || !input) return { found: false };
  const form = input.closest('form');
  const before = form ? getComputedStyle(form).display : null;
  button.click();
  await new Promise((r) => setTimeout(r, 80));
  return {
    found: true,
    hasClass: !!form && form.classList.contains('mobile-search-open'),
    before,
    after: form ? getComputedStyle(form).display : null,
  };
});
record(
  'G2 ≤640 移动搜索框仍能展开',
  search.found && search.hasClass && search.after !== 'none',
  JSON.stringify(search),
);

record('H1 无未捕获的页面错误', pageErrors.length === 0, pageErrors.join(' | ') || 'none');

await browser.close();

const failed = results.filter((r) => !r.ok);
console.log(`\n${results.length - failed.length}/${results.length} passed`);
if (failed.length) {
  console.log('failures:');
  for (const f of failed) console.log(`  - ${f.name} (${f.detail})`);
}
process.exit(failed.length === 0 ? 0 : 1);
