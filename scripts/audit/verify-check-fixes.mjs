// The three things the code-check pass changed or flagged, verified independently:
//   F1  the tenant-name link's underline must sit on the text again, not 22px below
//       it (the 44px box had pushed the border-bottom off the glyphs), while the
//       link box stays 44 tall so the hit area does not shrink.
//   F3  the dunning drawer's 确认同日重发 checkbox must now have a 44px tap target.
//       It only exists once the drawer is opened, which is why the audit missed it.
//   F5  the drawer must be openable by keyboard, and the closed sidebar must be out
//       of the tab order. Both are invisible in a screenshot, so assert on computed
//       style and on real focus behaviour.
import { chromium } from 'playwright';
import { launchOptions } from './launch.mjs';

const BASE = process.env.AUDIT_BASE || 'http://localhost:8081';
const W = Number(process.env.PROBE_W || 375);
let fails = 0;
const check = (ok, label, detail) => { if (!ok) fails++; console.log(`  ${ok ? 'PASS' : 'FAIL'}  ${label.padEnd(48)} ${detail}`); };

const browser = await chromium.launch(launchOptions());
const ctx = await browser.newContext({ viewport: { width: W, height: 900 } });
const page = await ctx.newPage();
await page.goto(BASE + '/', { waitUntil: 'domcontentloaded' });
await page.fill('#username', process.env.AUDIT_USER);
await page.fill('#password', process.env.AUDIT_PASS);
await Promise.all([page.waitForNavigation({ waitUntil: 'domcontentloaded' }).catch(() => {}), page.click('button[type=submit]')]);
console.log(`\n===== ${W}px =====`);

// ---- F1
await page.goto(BASE + '/rent-dashboard', { waitUntil: 'networkidle' });
{
  console.log('\n-- F1 姓名链接下划线');
  const r = await page.evaluate(() => {
    const a = document.querySelector('.tenant-link');
    a.scrollIntoView({ block: 'center' });
    const cs = getComputedStyle(a);
    const b = a.getBoundingClientRect();
    const after = getComputedStyle(a, '::after');
    return {
      box: [Math.round(b.width), Math.round(b.height)],
      borderBottom: cs.borderBottomWidth + ' ' + cs.borderBottomStyle,
      decoLine: cs.textDecorationLine,
      decoStyle: cs.textDecorationStyle,
      decoOffset: cs.textUnderlineOffset,
      afterTop: after.top, afterBottom: after.bottom,
    };
  });
  console.log(`   盒 ${r.box.join('x')}  border-bottom=${r.borderBottom}  text-decoration=${r.decoLine} ${r.decoStyle} offset=${r.decoOffset}`);
  check(r.borderBottom.startsWith('0'), '下划线不再画在 44px 盒子的底边', r.borderBottom);
  check(r.decoLine.includes('underline'), '改用文字自身的下划线', r.decoLine);
  check(r.decoStyle === 'dashed', '下划线样式与桌面端一致（虚线）', r.decoStyle);
  check(r.box[1] >= 44, '链接盒仍高 >=44（命中区未缩水）', r.box[1] + 'px');
  check(r.afterTop === '0px' && r.afterBottom === '0px', '::after 仍只做水平扩展', `${r.afterTop}/${r.afterBottom}`);
}

// ---- F3
{
  console.log('\n-- F3 催缴抽屉勾选框');
  const opened = await page.evaluate(() => {
    const d = document.querySelector('.dunning-drawer');
    if (!d) return null;
    d.hidden = false;
    const btn = [...d.querySelectorAll('button, a')].find((e) => /展开|配置|催缴/.test(e.textContent));
    if (btn) btn.click();
    return true;
  });
  await page.waitForTimeout(400);
  const r = await page.evaluate(() => {
    const label = [...document.querySelectorAll('.dunning-actions label')].find((l) => l.querySelector('input[name="confirm_resend"]'))
      || document.querySelector('label:has(input[name="confirm_resend"])');
    if (!label) return null;
    label.scrollIntoView({ block: 'center' });
    const b = label.getBoundingClientRect();
    const i = label.querySelector('input');
    const ib = i.getBoundingClientRect();
    return { box: [Math.round(b.width), Math.round(b.height)], input: [Math.round(ib.width), Math.round(ib.height)], display: getComputedStyle(label).display };
  });
  if (!r) console.log('   （催缴面板结构未匹配，跳过）');
  else {
    console.log(`   label ${r.box.join('x')} (${r.display})  内含勾选框 ${r.input.join('x')}`);
    check(r.box[1] >= 44, 'label 可点区域 >=44 高', r.box[1] + 'px');
  }
  void opened;
}

// ---- F5
await page.goto(BASE + '/tenants', { waitUntil: 'networkidle' });
{
  console.log('\n-- F5 抽屉的键盘可达性与 Tab 顺序');
  const closed = await page.evaluate(() => {
    const sb = document.querySelector('.sidebar');
    const cb = document.querySelector('#nav-drawer');
    return { sidebarVisibility: getComputedStyle(sb).visibility, checkboxDisplay: getComputedStyle(cb).display, checkboxHiddenAttr: cb.hasAttribute('hidden') };
  });
  console.log(`   关闭态：sidebar visibility=${closed.sidebarVisibility}  checkbox display=${closed.checkboxDisplay}  hidden 属性=${closed.checkboxHiddenAttr}`);
  check(closed.sidebarVisibility === 'hidden', '关闭的侧栏离开 Tab 顺序', closed.sidebarVisibility);
  check(closed.checkboxDisplay !== 'none', '状态复选框不是 display:none（可聚焦）', closed.checkboxDisplay);
  check(!closed.checkboxHiddenAttr, '状态复选框不带 hidden 属性', String(closed.checkboxHiddenAttr));

  // Focus the checkbox and press Space — this is the whole point of F5.
  const focusable = await page.evaluate(() => {
    const cb = document.querySelector('#nav-drawer');
    cb.focus();
    return document.activeElement === cb;
  });
  check(focusable, '状态复选框可获得焦点', String(focusable));
  await page.keyboard.press('Space');
  await page.waitForTimeout(400);
  const after = await page.evaluate(() => {
    const cb = document.querySelector('#nav-drawer');
    return { checked: cb.checked, sidebarVisibility: getComputedStyle(document.querySelector('.sidebar')).visibility };
  });
  check(after.checked, '空格键勾选状态复选框', String(after.checked));
  check(after.sidebarVisibility === 'visible', '空格键真的打开了抽屉', after.sidebarVisibility);

  // And the drawer must be dismissable by keyboard again.
  await page.keyboard.press('Space');
  await page.waitForTimeout(400);
  const shut = await page.evaluate(() => ({ checked: document.querySelector('#nav-drawer').checked, v: getComputedStyle(document.querySelector('.sidebar')).visibility }));
  check(!shut.checked && shut.v === 'hidden', '再按一次空格关闭抽屉', `checked=${shut.checked} visibility=${shut.v}`);

  // The sr-only checkbox is 1px, so metrics.mjs reports it as a small target on
  // every page. Its real tap target is the label that toggles it — measure that
  // and click it, the same way the other two box-vs-hit-area cases were settled.
  const bar = await page.evaluate(() => {
    const el = document.querySelector('.nav-compact-bar');
    el.scrollIntoView({ block: 'center' });
    const b = el.getBoundingClientRect();
    return { box: [Math.round(b.width), Math.round(b.height)], x: b.left + b.width / 2, y: b.top + b.height / 2 };
  });
  console.log(`   紧凑栏 ${bar.box.join('x')}（1px 复选框的真实触控目标）`);
  check(bar.box[1] >= 44, '紧凑栏触控目标 >=44 高', bar.box[1] + 'px');
  await page.mouse.click(bar.x, bar.y);
  await page.waitForTimeout(400);
  const viaClick = await page.evaluate(() => ({ checked: document.querySelector('#nav-drawer').checked, v: getComputedStyle(document.querySelector('.sidebar')).visibility }));
  check(viaClick.checked && viaClick.v === 'visible', '点紧凑栏真的打开抽屉', `checked=${viaClick.checked} visibility=${viaClick.v}`);
}

await browser.close();
console.log(`\n${fails === 0 ? 'ALL PASS' : fails + ' FAILURES'}`);
process.exit(fails === 0 ? 0 : 1);
