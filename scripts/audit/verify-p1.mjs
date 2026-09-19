// Measures the specific P1 findings the audit named, at phone widths, on the real
// app. Each row asserts a number, not a CSS reading: "the control is >=44px" is a
// geometry claim and only a browser can settle it.
//
// Usage: AUDIT_USER=... AUDIT_PASS=... node verify-p1.mjs
import { chromium } from 'playwright';
import { launchOptions } from './launch.mjs';

const BASE = process.env.AUDIT_BASE || 'http://localhost:8081';
const WIDTHS = (process.env.PROBE_W || '375,390').split(',').map(Number);
const browser = await chromium.launch(launchOptions());

let fails = 0;
const check = (label, ok, detail) => {
  if (!ok) fails++;
  console.log(`  ${ok ? 'PASS' : 'FAIL'}  ${label.padEnd(52)} ${detail}`);
};

for (const W of WIDTHS) {
  const ctx = await browser.newContext({ viewport: { width: W, height: 800 } });
  const page = await ctx.newPage();
  await page.goto(BASE + '/', { waitUntil: 'domcontentloaded' });
  await page.fill('#username', process.env.AUDIT_USER);
  await page.fill('#password', process.env.AUDIT_PASS);
  await Promise.all([page.waitForNavigation({ waitUntil: 'domcontentloaded' }).catch(() => {}), page.click('button[type=submit]')]);

  await page.goto(BASE + '/tenants', { waitUntil: 'networkidle' });
  const tenantId = await page.evaluate(() => (document.body.innerHTML.match(/\/tenants\/(\d+)/) || [])[1]);
  await page.goto(BASE + '/billing', { waitUntil: 'networkidle' });
  const txnId = await page.evaluate(() => (document.body.innerHTML.match(/name="transaction_id" value="(\d+)"/) || [])[1]);
  let receiptId = null;
  for (const h of ['/billing', '/rent-dashboard', '/expenses']) {
    await page.goto(BASE + h, { waitUntil: 'networkidle' });
    receiptId = await page.evaluate(() => {
      const a = document.querySelector('a[href*="cash-receipts/void"]');
      return a ? new URL(a.href).searchParams.get('receipt_id') : null;
    });
    if (receiptId) break;
  }

  // What column N actually holds differs per page; so does the width it needs.
  const COL4 = { tenants: '账单安排', expenses: '日期', 'rent-dashboard': '租金' };
  const COL4_MIN = { tenants: 100, expenses: 100, 'rent-dashboard': 100 };
  const COL3 = { tenants: '租金', expenses: '类别' };
  const COL3_MIN = { tenants: 70, expenses: 70 };

  const PAGES = [
    ['tenants', '/tenants'],
    ['tenant-detail', tenantId ? `/tenants/${tenantId}` : null],
    ['rent-dashboard', '/rent-dashboard'],
    ['billing', '/billing'],
    ['expenses', '/expenses'],
    ['cash-receipt-new', '/cash-receipts/new'],
    ['cash-receipt-void', receiptId ? `/cash-receipts/void?receipt_id=${receiptId}` : null],
  ];

  console.log(`\n================ viewport ${W} ================`);
  for (const [name, url] of PAGES) {
    if (!url) { console.log(`\n-- ${name}: SKIPPED (no id)`); continue; }
    await page.goto(BASE + url, { waitUntil: 'networkidle', timeout: 30000 });
    const r = await page.evaluate(() => {
      const box = (el) => { if (!el) return null; const b = el.getBoundingClientRect(); return [Math.round(b.width), Math.round(b.height)]; };
      const all = (sel) => [...document.querySelectorAll(sel)];
      const tooSmall = (sel, need) => all(sel)
        .map((el) => ({ el, b: el.getBoundingClientRect() }))
        .filter(({ b }) => b.width > 0 && b.height > 0 && (b.width < need.w || b.height < need.h))
        .map(({ el, b }) => `${el.tagName.toLowerCase()}.${(typeof el.className === 'string' ? el.className : '').trim().split(/\s+/).join('.')} ${Math.round(b.width)}x${Math.round(b.height)}`);

      // Hit area of a link, including its own ::after extension.
      const hit = (el) => {
        if (!el) return null;
        const b = el.getBoundingClientRect();
        const a = getComputedStyle(el, '::after');
        let w = b.width, h = b.height;
        if (a && a.content !== 'none' && a.position === 'absolute') {
          const l = parseFloat(a.left), rr = parseFloat(a.right);
          if (!Number.isNaN(l) && !Number.isNaN(rr)) w = b.width - l - rr;
        }
        return [Math.round(w), Math.round(h)];
      };

      // How many lines does a table cell's text occupy?
      const lines = (el) => {
        if (!el) return null;
        const cs = getComputedStyle(el);
        const lh = parseFloat(cs.lineHeight) || parseFloat(cs.fontSize) * 1.45;
        return Math.round((el.getBoundingClientRect().height - parseFloat(cs.paddingTop) - parseFloat(cs.paddingBottom)) / lh);
      };

      const headCell = (n) => document.querySelector(`.table-wrap > table > thead > tr > th:nth-child(${n})`);
      const bodyCell = (n) => document.querySelector(`.table-wrap > table > tbody > tr > td:nth-child(${n})`);

      return {
        pageOverflow: document.documentElement.scrollWidth - document.documentElement.clientWidth,
        // P1-1 the date picker button
        calendarTrigger: box(document.querySelector('.calendar-trigger')),
        // P1-2 status filter chips on the dashboard
        chips: all('a.count-chip').map((e) => [e.textContent.trim(), ...box(e)]),
        // P1-4 the tenant name link, whose ::after extends the hit area
        tenantLinkHit: hit(document.querySelector('.tenant-link')),
        tenantLinkBox: box(document.querySelector('.tenant-link')),
        // every control the shared rule claims to have raised
        controlsUnder44: tooSmall('input:not([type=checkbox]):not([type=radio]), select, textarea, .btn, a.month-nav', { w: 0, h: 44 }),
        // P1-4 the remember-payer checkbox, bare and label-wrapped
        confirmRememberPayer: box(document.querySelector('.confirm-form input[name="remember_payer"]')),
        wrappedRememberPayer: box(document.querySelector('.bind-form label.tiny, .month-choice-form label.tiny')),
        // P1-3 the two named cramped columns
        col4: box(headCell(4)),
        col4Lines: lines(bodyCell(4)),
        col3: box(headCell(3)),
        col3Lines: lines(bodyCell(3)),
        // the expense form's only submit button, compared against the form's
        // *content* box — a full-width block child spans the content box, not the
        // border box, so the form's own padding must be subtracted.
        expenseSubmit: box(document.querySelector('.panel.form > .btn.primary')),
        expenseFormContentW: (() => {
          const f = document.querySelector('.panel.form');
          if (!f) return null;
          const cs = getComputedStyle(f);
          return Math.round(f.getBoundingClientRect().width - parseFloat(cs.paddingLeft) - parseFloat(cs.paddingRight) - parseFloat(cs.borderLeftWidth) - parseFloat(cs.borderRightWidth));
        })(),
        // the void page's panel and its direct children's *content* inset: .facts
        // carries the inset as padding while its siblings carry it as margin, so
        // compare content edges (left + paddingLeft), not element edges.
        voidPanel: box(document.querySelector('.void-shell .panel')),
        voidChildren: [...document.querySelectorAll('.void-shell .panel.surface > *')]
          .map((e) => {
            const r = e.getBoundingClientRect();
            const cs = getComputedStyle(e);
            const cls = (typeof e.className === 'string' ? e.className : '').trim().split(/\s+/).join('.');
            return `${e.tagName.toLowerCase()}.${cls}@${Math.round(r.left + parseFloat(cs.paddingLeft))}`;
          }),
        factsColumns: getComputedStyle(document.querySelector('.facts') || document.body).gridTemplateColumns,
      };
    });

    console.log(`\n-- ${name}`);
    check('页面无横向溢出', r.pageOverflow <= 0, `scrollW-clientW=${r.pageOverflow}`);
    if (r.calendarTrigger) check('日历触发按钮 >=44x44', r.calendarTrigger[0] >= 44 && r.calendarTrigger[1] >= 44, `${r.calendarTrigger.join('x')}`);
    if (r.chips.length) check('状态 chip 可点高度 >=44', r.chips.every((c) => c[2] >= 44), r.chips.map((c) => `${c[0]}=${c[1]}x${c[2]}`).join(' '));
    if (r.tenantLinkBox) check('租客姓名链接命中宽度 >=44', r.tenantLinkHit[0] >= 44, `box=${r.tenantLinkBox.join('x')} 命中=${r.tenantLinkHit.join('x')}`);
    check('控件无一低于 44px 高', r.controlsUnder44.length === 0, r.controlsUnder44.length ? r.controlsUnder44.join(', ') : `全部达标`);
    if (r.confirmRememberPayer) check('确认表单勾选框 44x44', r.confirmRememberPayer[0] >= 44 && r.confirmRememberPayer[1] >= 44, r.confirmRememberPayer.join('x'));
    if (r.wrappedRememberPayer) check('label 包裹的勾选框命中高度 >=44', r.wrappedRememberPayer[1] >= 44, r.wrappedRememberPayer.join('x'));
    // The cramped-column assertions are per-page, not generic: column 4 is the
    // 账单安排 column only on /tenants, and column 3 is 类别 only on /expenses.
    // On /billing column 3 is the amount column this task deliberately hides at
    // narrow widths (display:none -> 0px), so it has no width to assert.
    if (r.col4 && r.col4[0] > 0) check(`第 4 列（${COL4[name] || '第 4 列'}）宽度 >=${COL4_MIN[name] || 100}`, r.col4[0] >= (COL4_MIN[name] || 100), `${r.col4[0]}px, 单元格 ${r.col4Lines} 行`);
    if (r.col3 && r.col3[0] > 0) check(`第 3 列（${COL3[name] || '第 3 列'}）宽度 >=${COL3_MIN[name] || 70}`, r.col3[0] >= (COL3_MIN[name] || 70), `${r.col3[0]}px, 单元格 ${r.col3Lines} 行`);
    if (r.expenseSubmit) check('保存支出按钮整宽', Math.abs(r.expenseSubmit[0] - r.expenseFormContentW) <= 1, `${r.expenseSubmit.join('x')} vs 表单内容宽 ${r.expenseFormContentW}`);
    if (r.voidPanel) {
      check('作废页 facts 单列', r.factsColumns.split(' ').length === 1, r.factsColumns);
      check('作废页面板子元素同为 20px 内缩', new Set(r.voidChildren.map((s) => s.split('@')[1])).size === 1, r.voidChildren.join(' '));
    }
  }
  await ctx.close();
}

await browser.close();
console.log(`\n${fails === 0 ? 'ALL PASS' : fails + ' FAILURES'}`);
process.exit(fails === 0 ? 0 : 1);
