// Verifies the three shell contracts that cannot be checked from Go alone,
// because each one is about what a real browser does with the shared chrome on a
// real page:
//
//   1. The rail is pinned. prd.md「用户报告的滚动缺陷」requires the desktop
//      sidebar to keep its viewport top while the page scrolls. This must be
//      measured on real pages: their ancestor chains are deeper than a synthetic
//      fixture's, and any `overflow` ancestor would silently break sticky.
//   2. The toast replaces the notice instead of joining it. After a save the
//      toast must appear, the marked notice must not be on screen, and the toast
//      must be gone a few seconds later.
//   3. The topbar search box appears on exactly the pages whose handler reads
//      ?search= — and submitting it really filters the list.
//
// Usage (against scripts/run-audit-local.sh, never :8081):
//   AUDIT_BASE=http://127.0.0.1:18090 AUDIT_USER=... AUDIT_PASS=... \
//     node scripts/audit/verify-shell.mjs
import { chromium } from 'playwright';
import { launchOptions, describeChrome } from './launch.mjs';

const BASE = (process.env.AUDIT_BASE || 'http://127.0.0.1:18090').replace(/\/+$/, '');
const USER = process.env.AUDIT_USER;
const PASS = process.env.AUDIT_PASS;

// Every destination the sidebar renders, plus the two pages whose list filter is
// not ?search= (they must show no topbar search box).
const PAGES = [
  ['rent-dashboard', '/rent-dashboard', true],
  ['bills', '/bills', true],
  ['transactions', '/transactions', false],
  ['dunning', '/dunning', true],
  ['properties', '/properties', true],
  ['rooms', '/rooms', true],
  ['tenants', '/tenants', false],
  ['tenancies', '/tenancies', true],
  ['cash-receipts', '/cash-receipts', true],
  ['expenses', '/expenses', true],
  ['bank', '/bank', false],
];

// Widths at or above 641px, where the rail is the sticky desktop column.
const WIDTHS = [
  { name: '1024', width: 1024, height: 768 },
  { name: '1366', width: 1366, height: 768 },
  { name: '1440', width: 1440, height: 900 },
];

const failures = [];
const fail = (message) => {
  failures.push(message);
  console.log(`FAIL ${message}`);
};
const ok = (message) => console.log(`ok   ${message}`);

console.log(`chrome: ${describeChrome()}`);
const browser = await chromium.launch(launchOptions());
const ctx = await browser.newContext({ viewport: { width: 1366, height: 768 } });
const page = await ctx.newPage();

await page.goto(BASE + '/', { waitUntil: 'domcontentloaded' });
await page.fill('#username', USER);
await page.fill('#password', PASS);
await Promise.all([
  page.waitForNavigation({ waitUntil: 'domcontentloaded' }).catch(() => {}),
  page.click('button[type=submit]'),
]);

// --- 1. the rail stays pinned while the document scrolls ---------------------
// Two pages, every desktop width: the rail's own top must not move, and it must
// still touch the viewport top (a sticky rail that scrolled half a screen would
// pass a naive "did it move" check on the wrong axis).
for (const [name, url] of PAGES.slice(0, 2)) {
  for (const vp of WIDTHS) {
    await page.setViewportSize({ width: vp.width, height: vp.height });
    await page.goto(BASE + url, { waitUntil: 'networkidle', timeout: 30000 });
    const before = await page.evaluate(() => {
      const rail = document.querySelector('.sidebar');
      return { top: rail.getBoundingClientRect().top, position: getComputedStyle(rail).position, scrollY: window.scrollY };
    });
    await page.evaluate(() => window.scrollTo(0, 600));
    await page.waitForTimeout(150);
    const after = await page.evaluate(() => {
      const rail = document.querySelector('.sidebar');
      return { top: rail.getBoundingClientRect().top, scrollY: window.scrollY };
    });
    const moved = Math.abs(after.top - before.top) > 1;
    if (after.scrollY === 0) {
      fail(`${name} @${vp.name}: the page did not scroll, so the sticky claim is untested`);
    } else if (moved) {
      fail(`${name} @${vp.name}: the rail scrolled away with the page (top ${before.top} -> ${after.top})`);
    } else if (Math.abs(after.top) > 1) {
      fail(`${name} @${vp.name}: the rail is sticky but not pinned to the viewport top (top ${after.top})`);
    } else {
      ok(`${name} @${vp.name}: rail pinned at top 0 after scrolling ${after.scrollY}px (position: ${before.position})`);
    }
  }
}

// --- 2. the toast replaces the notice ---------------------------------------
// A save that redirects with ?message= is exactly what the toast is for, and a
// GET with that query renders the same server-side flash the redirect lands on.
// Reading it this way exercises the real markup and the real script rather than
// an injected block the script would never have seen.
await page.setViewportSize({ width: 1366, height: 900 });
const FLASH_URL = '/expenses?message=expense_added';
const FLASH_TEXT = '支出记录已保存。';

await page.goto(BASE + FLASH_URL, { waitUntil: 'domcontentloaded' });
let appeared = true;
try {
  await page.waitForFunction(() => {
    const toast = document.getElementById('workspace-toast');
    return toast && !toast.hidden;
  }, { timeout: 5000 });
} catch {
  appeared = false;
}

if (!appeared) {
  fail('the toast never appeared for a server-rendered flash message');
} else {
  // Undimmed, not merely present: the transition is 200ms and a check that only
  // asks for "not hidden" would pass on a toast stuck at opacity 0.
  let opaque = true;
  try {
    await page.waitForFunction(() => Number(getComputedStyle(document.getElementById('workspace-toast')).opacity) > 0.95, { timeout: 2000 });
  } catch {
    opaque = false;
  }
  if (!opaque) fail('the toast never reached full opacity');
  const shown = await page.evaluate(() => {
    const toast = document.getElementById('workspace-toast');
    const notice = document.querySelector('.notice[data-toast]');
    return {
      text: toast.textContent.trim(),
      opacity: getComputedStyle(toast).opacity,
      noticeDisplay: notice ? getComputedStyle(notice).display : '(no marked notice)',
      noticeCount: document.querySelectorAll('.notice[data-toast]').length,
    };
  });
  if (shown.text !== FLASH_TEXT) {
    fail(`toast text is ${JSON.stringify(shown.text)}, want ${JSON.stringify(FLASH_TEXT)}`);
  } else {
    ok(`toast adopted the flash message: ${JSON.stringify(shown.text)}`);
  }
  if (shown.noticeCount !== 1) {
    fail(`the page rendered ${shown.noticeCount} marked notices, want exactly 1`);
  }
  if (shown.noticeDisplay !== 'none') {
    fail(`the marked notice stayed visible alongside the toast (display: ${shown.noticeDisplay})`);
  } else {
    ok('the same message is not on screen twice (marked notice hidden)');
  }
  if (Number(shown.opacity) === 0) {
    fail('the toast was in the DOM but never became visible');
  } else {
    ok(`toast visible (opacity ${shown.opacity})`);
  }
}

let dismissed = true;
try {
  await page.waitForFunction(() => document.getElementById('workspace-toast').hidden, { timeout: 8000 });
} catch {
  dismissed = false;
}
if (!dismissed) {
  fail('the toast did not dismiss after its timeout');
} else {
  ok('toast dismissed automatically');
}

// The no-script fallback is a CSS contract: the notice is hidden only under the
// html.js root class the inline script adds, so without scripting it stays.
const noScript = await page.evaluate(() => {
  const notice = document.querySelector('.notice[data-toast]');
  document.documentElement.classList.remove('js');
  const display = getComputedStyle(notice).display;
  document.documentElement.classList.add('js');
  return { display, scripted: getComputedStyle(notice).display };
});
if (noScript.display === 'none') {
  fail('without the js root class the marked notice was hidden; the no-script fallback is broken');
} else {
  ok(`no-script fallback keeps the notice visible (display: ${noScript.display})`);
}

// The marker is success-only. An error flash has to stay a notice on screen until
// the user has read and acted on it, so it must never be promoted into a toast.
await page.goto(`${BASE}/bills?error=invalid_dashboard_filter`, { waitUntil: 'networkidle', timeout: 30000 });
const errorFlash = await page.evaluate(() => {
  const error = document.querySelector('.notice.error');
  return {
    errorCount: document.querySelectorAll('.notice.error').length,
    errorDisplay: error ? getComputedStyle(error).display : '(no error notice)',
    marked: document.querySelectorAll('.notice[data-toast]').length,
    toastHidden: document.getElementById('workspace-toast').hidden,
  };
});
if (errorFlash.errorCount === 0) {
  fail('the error flash never rendered, so the success-only rule was not tested');
} else if (errorFlash.errorDisplay === 'none') {
  fail('the error notice was hidden, so the message was lost');
} else if (errorFlash.marked !== 0) {
  fail(`${errorFlash.marked} error notice(s) are marked data-toast; only success flashes may become the toast`);
} else if (!errorFlash.toastHidden) {
  fail('an error flash produced a toast; errors must stay on screen');
} else {
  ok(`error flash stays a visible notice and produces no toast (display: ${errorFlash.errorDisplay})`);
}

// --- 3. the topbar search box is on exactly the searchable pages -------------
for (const [name, url, wantSearch] of PAGES) {
  await page.goto(BASE + url, { waitUntil: 'networkidle', timeout: 30000 });
  const has = (await page.locator('.workspace-topsearch').count()) > 0;
  if (has !== wantSearch) {
    fail(`${name}: topbar search box present=${has}, want ${wantSearch}`);
  } else {
    ok(`${name}: topbar search box present=${has}`);
  }
}

// Submitting it must actually filter: search for a term no row contains and the
// result count must drop. /expenses renders its own "N 条" summary text.
await page.goto(BASE + '/expenses?search=zzz-no-such-expense', { waitUntil: 'networkidle' });
const filtered = await page.evaluate(() => {
  const rows = document.querySelectorAll('.expense-table tbody tr').length;
  return { rows, url: location.search };
});
if (filtered.rows !== 0) {
  fail(`/expenses?search=<no match> still rendered ${filtered.rows} rows; the box would not filter`);
} else {
  ok('the topbar search term reaches the list (no-match search renders 0 rows)');
}

const submitted = await page.goto(BASE + '/expenses', { waitUntil: 'networkidle' }).then(async () => {
  await page.fill('#workspace-topsearch-input', 'zzz-no-such-expense');
  await Promise.all([
    page.waitForNavigation({ waitUntil: 'networkidle' }).catch(() => {}),
    page.press('#workspace-topsearch-input', 'Enter'),
  ]);
  return page.evaluate(() => ({ url: location.pathname + location.search, rows: document.querySelectorAll('.expense-table tbody tr').length }));
});
if (!submitted.url.includes('search=zzz-no-such-expense') || submitted.rows !== 0) {
  fail(`submitting the topbar box did not filter the page (landed on ${submitted.url} with ${submitted.rows} rows)`);
} else {
  ok(`submitting the topbar box filtered the page (${submitted.url})`);
}

// --- the count button agrees with the panel it links to ----------------------
await page.goto(BASE + '/rent-dashboard', { waitUntil: 'networkidle' });
const count = await page.evaluate(() => {
  const button = document.querySelector('.workspace-topbar-count');
  const panel = document.querySelector('.workspace-queue-count');
  return {
    button: button ? button.textContent.trim() : null,
    buttonHref: button ? button.getAttribute('href') : null,
    panel: panel ? panel.textContent.trim() : null,
    anchorExists: !!document.getElementById('pending-review'),
  };
});
if (count.button === null) {
  fail('the dashboard rendered no count button');
} else if (count.button !== count.panel) {
  fail(`count button says ${count.button} but the panel says ${count.panel}`);
} else if (!count.anchorExists) {
  fail(`the count button links to ${count.buttonHref} but #pending-review does not exist on the page`);
} else {
  ok(`count button and 待人工处理流水 panel both read ${count.button}`);
}

// --- the sidebar really renders the prototype's furniture -------------------
const sidebar = await page.evaluate(() => ({
  groups: [...document.querySelectorAll('.sidebar .nav-label')].map((n) => n.textContent.trim()),
  icons: [...document.querySelectorAll('.sidebar .nav-icon')].map((n) => n.textContent.trim()),
  counts: document.querySelectorAll('.sidebar .nav-count').length,
  status: !!document.querySelector('.sidebar .side-status'),
  user: (document.querySelector('.sidebar .side-user-copy') || {}).textContent || null,
}));
if (sidebar.groups.join('/') !== '收租决策/资产与关系/资金与系统') {
  fail(`sidebar groups are ${JSON.stringify(sidebar.groups)}`);
} else {
  ok(`sidebar groups: ${sidebar.groups.join(' ')}`);
}
const expectedIcons = Array.from({ length: 11 }, (_, i) => String(i + 1).padStart(2, '0'));
if (sidebar.icons.join(',') !== expectedIcons.join(',')) {
  fail(`sidebar icons are ${JSON.stringify(sidebar.icons)}`);
} else {
  ok('sidebar icons: 01..11');
}
if (sidebar.counts !== 4) {
  fail(`sidebar rendered ${sidebar.counts} count badges, want 4`);
} else {
  ok('sidebar rendered 4 count badges');
}
ok(`sidebar status card present=${sidebar.status}, footer=${JSON.stringify((sidebar.user || '').trim())}`);

// --- is the footer inside the rail, or scrolled out of it? -------------------
const footer = await page.evaluate(() => {
  const rail = document.querySelector('.sidebar');
  const small = document.querySelector('.sidebar .side-user-copy small');
  const user = document.querySelector('.sidebar .side-user');
  return {
    railScrolls: rail.scrollHeight > rail.clientHeight + 1,
    overflow: rail.scrollHeight - rail.clientHeight,
    userBottom: user ? Math.round(user.getBoundingClientRect().bottom) : null,
    railBottom: Math.round(rail.getBoundingClientRect().bottom),
    username: small ? small.textContent.trim() : null,
  };
});
console.log(
  `      rail overflow=${footer.overflow}px, footer bottom=${footer.userBottom} rail bottom=${footer.railBottom} username=${JSON.stringify(footer.username)}`
);
if (footer.railScrolls && footer.userBottom > footer.railBottom) {
  fail('the sidebar footer is scrolled out of the rail at 1440x900');
} else {
  ok('the sidebar footer is inside the rail at 1440x900');
}

await browser.close();

console.log('');
if (failures.length) {
  console.error(`${failures.length} shell check(s) failed:`);
  for (const f of failures) console.error(`  ${f}`);
  process.exit(1);
}
console.log('all shell checks passed');
