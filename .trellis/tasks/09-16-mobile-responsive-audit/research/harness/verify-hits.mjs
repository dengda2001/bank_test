// metrics.mjs measures element boxes, so two fixes it still reports as small read
// as unfixed: the tenant-name link (26x44 box, hit area widened by ::after) and the
// label-wrapped remember_payer checkbox (13x13 box, hit area is the label). A hit
// area is a claim about what the browser delivers a click to, and only a click can
// settle it — so this clicks the extension itself and checks what happened.
import { chromium } from 'playwright';

const BASE = process.env.AUDIT_BASE || 'http://localhost:8081';
const W = Number(process.env.PROBE_W || 375);
let fails = 0;
const check = (ok, label, detail) => { if (!ok) fails++; console.log(`  ${ok ? 'PASS' : 'FAIL'}  ${label.padEnd(46)} ${detail}`); };

const browser = await chromium.launch({
  executablePath: '/tmp/mobile-audit/chrome-linux64/chrome',
  args: ['--no-sandbox', '--disable-dev-shm-usage'],
});
const ctx = await browser.newContext({ viewport: { width: W, height: 900 } });
const page = await ctx.newPage();
await page.goto(BASE + '/', { waitUntil: 'domcontentloaded' });
await page.fill('#username', process.env.AUDIT_USER);
await page.fill('#password', process.env.AUDIT_PASS);
await Promise.all([page.waitForNavigation({ waitUntil: 'domcontentloaded' }).catch(() => {}), page.click('button[type=submit]')]);
console.log(`\n===== ${W}px =====`);

// 1. The tenant-name link's ::after extension must deliver clicks to the link.
await page.goto(BASE + '/rent-dashboard', { waitUntil: 'networkidle' });
const link = await page.evaluate(() => {
  const a = document.querySelector('.tenant-link');
  // elementFromPoint works in viewport coordinates, and the dashboard's table sits
  // far below the fold — scroll it up first or every probe returns null.
  a.scrollIntoView({ block: 'center' });
  const b = a.getBoundingClientRect();
  const probe = (x, y) => { const el = document.elementFromPoint(x, y); return el ? el.tagName.toLowerCase() + (el === a || a.contains(el) ? '(link)' : '') : 'none'; };
  return {
    rect: [Math.round(b.left), Math.round(b.top), Math.round(b.right), Math.round(b.bottom)],
    href: a.getAttribute('href'),
    insideBox: probe(b.left + 2, b.top + b.height / 2),
    left6: probe(b.left - 6, b.top + b.height / 2),
    left12: probe(b.left - 12, b.top + b.height / 2),
    right6: probe(b.right + 6, b.top + b.height / 2),
    // The element box is already 44 tall; ::after only widens it, so the top and
    // bottom edges of the box itself are the vertical extent of the hit area.
    topEdge: probe(b.left + b.width / 2, b.top + 2),
    bottomEdge: probe(b.left + b.width / 2, b.bottom - 2),
    aboveBox: probe(b.left + b.width / 2, b.top - 6),
  };
});
console.log(`\n-- 租客姓名链接 元素盒 ${link.rect.join(',')}  href=${link.href}`);
check(link.insideBox === 'a(link)', '元素盒内命中链接', link.insideBox);
check(link.left6 === 'a(link)', '左侧 6px（::after 区）命中链接', link.left6);
check(link.right6 === 'a(link)', '右侧 6px（::after 区）命中链接', link.right6);
check(link.left12 !== 'a(link)', '左侧 12px（扩展区外）不命中链接', link.left12);
check(link.topEdge === 'a(link)', '元素盒上边缘命中链接', link.topEdge);
check(link.bottomEdge === 'a(link)', '元素盒下边缘命中链接', link.bottomEdge);
check(link.aboveBox !== 'a(link)', '元素盒上方 6px 不命中（命中区高 44，不向上溢出）', link.aboveBox);

// Actually navigate by clicking the extension, not just hit-test it.
await page.goto(BASE + '/rent-dashboard', { waitUntil: 'networkidle' });
const target = await page.evaluate(() => {
  const a = document.querySelector('.tenant-link');
  a.scrollIntoView({ block: 'center' });
  const b = a.getBoundingClientRect();
  return { x: b.left - 6, y: b.top + b.height / 2, href: a.href };
});
await Promise.all([
  page.waitForNavigation({ waitUntil: 'domcontentloaded', timeout: 15000 }).catch(() => {}),
  page.mouse.click(target.x, target.y),
]);
check(page.url().startsWith(target.href.replace(/\/$/, '')), '点扩展区真的跳转到租客详情', page.url());

// 2. The label-wrapped checkbox must toggle from anywhere in its 44px label.
// The label lives inside the action cell's <details>, which the narrow-screen script
// collapses (content-visibility: hidden) — such a subtree keeps a box but is not
// hit-testable, so a click there proves nothing. Expand the row first.
await page.goto(BASE + '/billing', { waitUntil: 'networkidle' });
const openedRow = await page.evaluate(() => {
  const d = [...document.querySelectorAll('.txn-col-action > .txn-action')].find((x) => x.querySelector('label.tiny'));
  if (!d) return null;
  d.open = true;
  d.querySelector('summary').scrollIntoView({ block: 'center' });
  return true;
});
if (openedRow) await page.waitForTimeout(200);
const toggled = await page.evaluate(() => {
  const label = document.querySelector('.bind-form label.tiny, .month-choice-form label.tiny');
  if (!label) return { skipped: true };
  const input = label.querySelector('input[name="remember_payer"]');
  label.scrollIntoView({ block: 'center' });
  const lr = label.getBoundingClientRect();
  // Aim at the label's top-right corner: far from the 13px box, inside the 44px label.
  const x = lr.left + lr.width - 4, y = lr.top + 3;
  // Report what actually receives that point, so a non-toggle is attributable.
  const hit = document.elementFromPoint(x, y);
  return { skipped: false, initial: input.checked, labelRect: [Math.round(lr.left), Math.round(lr.top), Math.round(lr.width), Math.round(lr.height)], x, y, hitAt: hit ? hit.tagName.toLowerCase() + (label.contains(hit) ? '(label 内)' : '') : 'none', inputRect: (() => { const b = input.getBoundingClientRect(); return [Math.round(b.width), Math.round(b.height)]; })() };
});
if (toggled.skipped) {
  console.log('\n-- 未找到 label 包裹的勾选框（该行形态未出现）');
} else {
  console.log(`\n-- 勾选框 ${toggled.inputRect.join('x')}，所在 label ${toggled.labelRect.join(',')}，初始 checked=${toggled.initial}`);
  console.log(`   点击点 (${Math.round(toggled.x)},${Math.round(toggled.y)}) 命中 ${toggled.hitAt}`);
  await page.mouse.click(toggled.x, toggled.y);
  const after = await page.evaluate(() => document.querySelector('.bind-form label.tiny input[name="remember_payer"], .month-choice-form label.tiny input[name="remember_payer"]').checked);
  check(after !== toggled.initial, '点 label 上边缘（远离 13px 勾选框）也能切换', `checked ${toggled.initial} -> ${after}`);
}

await browser.close();
console.log(`\n${fails === 0 ? 'ALL PASS' : fails + ' FAILURES'}`);
process.exit(fails === 0 ? 0 : 1);
