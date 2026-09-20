// Verification for: /transactions/<id> rendered as unstyled HTML.
//
// The page template never linked /static/css/workspace.css, which owns both the
// design tokens (--border, --surface, --accent, ...) and the chrome/layout
// classes (.app, .content, .sidebar, .panel, .btn, .status, .table-wrap). None
// of those are defined in any other stylesheet, so without the link the page is
// not "slightly off" — it is bare HTML.
//
// This probe measures the same four facts twice, on one running instance:
//
//   WITH   the stylesheet, as shipped
//   WITHOUT the stylesheet, by aborting that one request via page.route()
//
// The second pass is the negative control. It proves the measurements can
// actually tell a styled page from an unstyled one; without it, four green
// numbers would only prove the numbers are easy to satisfy.
//
// The control is a request-level abort, not a rebuild, so both passes run
// against byte-identical server output — the only difference is the one file.
import { chromium } from 'playwright';
import { launchOptions } from './launch.mjs';

const BASE = (process.env.AUDIT_BASE || 'http://127.0.0.1:18098').replace(/\/+$/, '');
const USER = process.env.AUDIT_USER;
const PASS = process.env.AUDIT_PASS;
const SHOTS = process.env.AUDIT_SHOTS || '/tmp';

if (!USER || !PASS) {
  console.error('set AUDIT_USER and AUDIT_PASS (see the launcher output)');
  process.exit(1);
}

const results = [];
const record = (name, ok, detail) => {
  results.push({ name, ok, detail });
  console.log(`${ok ? 'PASS' : 'FAIL'}  ${name}${detail ? ' — ' + detail : ''}`);
};

// Four facts that are true of a styled page and false of a bare one. Each reads
// a computed value, never a class name or a stylesheet's presence: what matters
// is what the browser actually painted, not what the markup asked for.
const measure = (page) =>
  page.evaluate(() => {
    const root = getComputedStyle(document.documentElement);
    const token = root.getPropertyValue('--border').trim();
    const panel = document.querySelector('.panel');
    const sidebar = document.querySelector('.sidebar');
    const btn = document.querySelector('.btn');
    const de = document.documentElement;
    return {
      // A token only exists if workspace.css was parsed.
      borderToken: token,
      // .panel's background comes from var(--surface); with no token the
      // declaration is invalid at computed-value time and resolves to
      // rgba(0, 0, 0, 0).
      panelBg: panel ? getComputedStyle(panel).backgroundColor : null,
      panelBorder: panel ? getComputedStyle(panel).borderTopWidth : null,
      // .sidebar is display:grid / position:sticky only in workspace.css.
      sidebarDisplay: sidebar ? getComputedStyle(sidebar).display : null,
      sidebarPosition: sidebar ? getComputedStyle(sidebar).position : null,
      // .btn carries the border-radius and padding.
      btnRadius: btn ? getComputedStyle(btn).borderTopLeftRadius : null,
      btnPadding: btn ? getComputedStyle(btn).paddingLeft : null,
      overflow: de.scrollWidth - de.clientWidth,
    };
  });

const styled = (m) =>
  m.borderToken !== '' &&
  m.panelBg !== 'rgba(0, 0, 0, 0)' &&
  m.sidebarPosition !== 'static' &&
  m.btnRadius !== '0px';

const bare = (m) =>
  m.borderToken === '' &&
  m.panelBg === 'rgba(0, 0, 0, 0)' &&
  m.btnRadius === '0px';

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

// Reach a real detail page by following the list's own link, so the probe is
// independent of how detail URLs happen to be shaped. The shape here is
// /transactions?detail=<id>, not a path segment.
await page.goto(BASE + '/transactions', { waitUntil: 'domcontentloaded' });
const detailHref = await page.evaluate(() => {
  const a = document.querySelector('a[href^="/transactions?detail="]');
  return a ? a.getAttribute('href') : null;
});
if (!detailHref) {
  console.error('no /transactions?detail=<id> link on /transactions — nothing to measure');
  await browser.close();
  process.exit(1);
}
const detailURL = new URL(detailHref, BASE).toString();
console.log(`detail page: ${detailURL}\n`);

// The page class is the marker that the detail template, not the list, rendered.
const marker = await page.goto(detailURL, { waitUntil: 'networkidle' }).then(() =>
  page.evaluate(() => !!document.querySelector('.transaction-detail-page')),
);
if (!marker) {
  console.error(`${detailURL} did not render .transaction-detail-page`);
  await browser.close();
  process.exit(1);
}

// --- pass 1: as shipped ------------------------------------------------
await page.goto(detailURL, { waitUntil: 'networkidle' });
const withCSS = await measure(page);
await page.screenshot({ path: `${SHOTS}/txn-detail-with-workspace-css.png`, fullPage: true });

record(
  'A. 加载 workspace.css 后，设计令牌生效',
  withCSS.borderToken !== '',
  `--border=${withCSS.borderToken || '(空)'}`,
);
record(
  'B. .panel 有实际背景与边框（不是透明裸块）',
  withCSS.panelBg !== 'rgba(0, 0, 0, 0)' && withCSS.panelBorder !== '0px',
  `background=${withCSS.panelBg} border=${withCSS.panelBorder}`,
);
record(
  'C. 侧栏按布局渲染（不是默认静态流）',
  withCSS.sidebarPosition !== 'static',
  `display=${withCSS.sidebarDisplay} position=${withCSS.sidebarPosition}`,
);
record(
  'D. 按钮有圆角与内边距',
  withCSS.btnRadius !== '0px' && withCSS.btnPadding !== '0px',
  `radius=${withCSS.btnRadius} padding-left=${withCSS.btnPadding}`,
);
record('E. 桌面档无横向溢出', withCSS.overflow <= 0, `overflow=${withCSS.overflow}px`);
record('F. 整体判定：看起来是「已排版」的页面', styled(withCSS), JSON.stringify(withCSS));

// --- pass 2: negative control, stylesheet aborted ----------------------
await page.route('**/static/css/workspace.css', (route) => route.abort());
await page.goto(detailURL, { waitUntil: 'networkidle' });
const withoutCSS = await measure(page);
await page.screenshot({ path: `${SHOTS}/txn-detail-without-workspace-css.png`, fullPage: true });
await page.unroute('**/static/css/workspace.css');

record(
  'G. 反面控制：拦掉 workspace.css 后，同样的量测判定为「裸页面」',
  bare(withoutCSS),
  JSON.stringify(withoutCSS),
);
record(
  'H. 反面控制确实改变了渲染（两次量测不相同）',
  JSON.stringify(withCSS) !== JSON.stringify(withoutCSS),
  `with=${withCSS.borderToken || '(空)'} without=${withoutCSS.borderToken || '(空)'}`,
);

// --- mobile ------------------------------------------------------------
await page.setViewportSize({ width: 390, height: 844 });
await page.goto(detailURL, { waitUntil: 'networkidle' });
const mobile = await measure(page);
await page.screenshot({ path: `${SHOTS}/txn-detail-390.png`, fullPage: true });
// The sidebar is expected to be off-canvas at this width, so only the token,
// panel and button facts are asserted here.
record(
  'I. 390 档仍有令牌、面板背景与按钮圆角',
  mobile.borderToken !== '' && mobile.panelBg !== 'rgba(0, 0, 0, 0)' && mobile.btnRadius !== '0px',
  `--border=${mobile.borderToken || '(空)'} panel=${mobile.panelBg} radius=${mobile.btnRadius}`,
);
record('J. 390 档无横向溢出', mobile.overflow <= 0, `overflow=${mobile.overflow}px`);

record('K. 无未捕获的页面错误', pageErrors.length === 0, pageErrors.join(' | ') || 'none');

await browser.close();

const failed = results.filter((r) => !r.ok);
console.log(`\n${results.length - failed.length}/${results.length} passed`);
console.log(`screenshots: ${SHOTS}/txn-detail-with-workspace-css.png, ${SHOTS}/txn-detail-without-workspace-css.png, ${SHOTS}/txn-detail-390.png`);
process.exit(failed.length ? 1 : 0);
