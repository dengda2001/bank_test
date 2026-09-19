// The P1-4 finding list is ~20 separate layout breakages. Before writing any
// "fixed / not fixed" status against it, measure each claim on the deployed build
// rather than recalling which ones the CSS edits happened to cover.
import { chromium } from 'playwright';
import { launchOptions } from './launch.mjs';

const BASE = process.env.AUDIT_BASE || 'http://localhost:8081';
const W = Number(process.env.PROBE_W || 375);
const browser = await chromium.launch(launchOptions());
const ctx = await browser.newContext({ viewport: { width: W, height: 900 } });
const page = await ctx.newPage();
await page.goto(BASE + '/', { waitUntil: 'domcontentloaded' });
await page.fill('#username', process.env.AUDIT_USER);
await page.fill('#password', process.env.AUDIT_PASS);
await Promise.all([page.waitForNavigation({ waitUntil: 'domcontentloaded' }).catch(() => {}), page.click('button[type=submit]')]);

const LINES = `(el) => {
  if (!el) return null;
  const texts = [];
  const walk = (n) => { for (const c of n.childNodes) { if (c.nodeType === 3 && c.textContent.trim()) texts.push(c); else if (c.nodeType === 1 && c.tagName !== 'BR') walk(c); } };
  walk(el);
  const tops = new Set();
  for (const t of texts) {
    const r = document.createRange(); r.selectNodeContents(t);
    for (const b of r.getClientRects()) if (b.width > 0 && b.height > 0) tops.add(Math.round(b.top));
  }
  return tops.size;
}`;

const report = (label, v) => console.log(`  ${label.padEnd(44)} ${v}`);

// ---- /tenants : 创建时间 column, and the row height it contributes to
await page.goto(BASE + '/tenants', { waitUntil: 'networkidle' });
{
  console.log('\n== /tenants');
  const r = await page.evaluate((src) => {
    const n = eval(src);
    const t = document.querySelector('main.content > section .table-wrap > table');
    const th = [...t.querySelectorAll(':scope > thead > tr > th')];
    const row = t.querySelector(':scope > tbody > tr');
    const cells = [...row.children];
    return {
      headers: th.map((h) => h.textContent.trim()),
      cols: cells.map((td, i) => ({ i: i + 1, w: Math.round(td.getBoundingClientRect().width), lines: n(td), txt: (td.textContent || '').trim().replace(/\s+/g, ' ').slice(0, 34) })),
      rowH: Math.round(row.getBoundingClientRect().height),
      tableW: Math.round(t.getBoundingClientRect().width),
    };
  }, LINES);
  r.cols.forEach((c) => report(`第${c.i}列 ${r.headers[c.i - 1] || ''} ${c.w}px`, `${c.lines} 行  "${c.txt}"`));
  report('行高', r.rowH + 'px');
  report('表格总宽', r.tableW + 'px');
}

// ---- /rent-dashboard : pagination wrap, dunning collapse button, tenant col, row toggle
await page.goto(BASE + '/rent-dashboard', { waitUntil: 'networkidle' });
{
  console.log('\n== /rent-dashboard');
  const r = await page.evaluate((src) => {
    const n = eval(src);
    const b = (el) => { if (!el) return null; const x = el.getBoundingClientRect(); return [Math.round(x.width), Math.round(x.height)]; };
    const pag = document.querySelector('.pagination-nav') || document.querySelector('.dashboard-pagination');
    const links = pag ? [...pag.querySelectorAll('a, .btn')].map((e) => ({ t: e.textContent.trim().slice(0, 10), box: b(e), lines: n(e) })) : [];
    const dunning = document.querySelector('.dunning-config, .dunning-panel');
    const collapse = dunning ? [...dunning.querySelectorAll('button, summary, .btn')].map((e) => ({ t: e.textContent.trim().slice(0, 10), box: b(e), lines: n(e) })) : [];
    const t = document.querySelector('main.content > section .table-wrap > table');
    const row = t && t.querySelector(':scope > tbody > tr');
    let firstCol = null, dash = null;
    if (row) {
      const td = row.children[0];
      firstCol = { w: Math.round(td.getBoundingClientRect().width), lines: n(td), txt: (td.textContent || '').trim().replace(/\s+/g, ' ').slice(0, 40) };
      const nameCell = row.querySelector('td:nth-child(2)');
      dash = { h: Math.round(row.getBoundingClientRect().height) };
    }
    const aliases = [...document.querySelectorAll('td .mono, .tenant-cell .mono')].slice(0, 3).map((e) => ({ lines: n(e), txt: e.textContent.trim().slice(0, 26) }));
    return { links, collapse, firstCol, dash, aliases };
  }, LINES);
  r.links.forEach((l) => report(`分页「${l.t}」`, `${l.box.join('x')} ${l.lines} 行`));
  r.collapse.forEach((l) => report(`催缴面板「${l.t}」`, `${l.box.join('x')} ${l.lines} 行`));
  if (r.firstCol) report('首列', `${r.firstCol.w}px ${r.firstCol.lines} 行 "${r.firstCol.txt}"`);
  if (r.dash) report('数据行高', r.dash.h + 'px');
  r.aliases.forEach((a) => report('别名行', `${a.lines} 行 "${a.txt}"`));
}

// ---- /billing : type column + row height + header buttons
await page.goto(BASE + '/billing', { waitUntil: 'networkidle' });
{
  console.log('\n== /billing');
  const r = await page.evaluate((src) => {
    const n = eval(src);
    const t = document.querySelector('main.content > section .table-wrap > table');
    const row = t.querySelector(':scope > tbody > tr');
    const type = row.querySelector('.txn-col-type');
    const heads = [...document.querySelectorAll('main.content > section .panel-head a.btn, main.content > section .panel-head .btn, main.content > section > .btn')].map((e) => ({ t: e.textContent.trim().slice(0, 14), w: Math.round(e.getBoundingClientRect().width) }));
    return {
      typeW: type ? Math.round(type.getBoundingClientRect().width) : null,
      typeLines: type ? n(type) : null,
      rowH: Math.round(row.getBoundingClientRect().height),
      heads,
    };
  }, LINES);
  report('类型列', `${r.typeW}px ${r.typeLines} 行`);
  report('数据行高', r.rowH + 'px');
  r.heads.forEach((h) => report(`页头按钮「${h.t}」`, h.w + 'px'));
}

// ---- /expenses : the unlabelled bare number in the description sub-line
await page.goto(BASE + '/expenses', { waitUntil: 'networkidle' });
{
  console.log('\n== /expenses');
  const r = await page.evaluate(() => {
    const row = document.querySelector('main.content > section .table-wrap > table > tbody > tr');
    const cell = row && row.children[0];
    if (!cell) return {};
    return { html: cell.innerHTML.replace(/\s+/g, ' ').trim().slice(0, 160) };
  });
  report('描述列首行 HTML', r.html || 'n/a');
}

// ---- /tenants/:id : 缴费历史 应缴日 column, row height
{
  const id = await page.evaluate(async () => {
    const r = await fetch('/tenants', { credentials: 'same-origin' });
    const h = await r.text();
    return (h.match(/\/tenants\/(\d+)/) || [])[1];
  });
  await page.goto(`${BASE}/tenants/${id}`, { waitUntil: 'networkidle' });
  console.log(`\n== /tenants/${id}`);
  const r = await page.evaluate((src) => {
    const n = eval(src);
    const tables = [...document.querySelectorAll('table')];
    const hist = tables.find((t) => (t.textContent || '').includes('应缴日')) || tables[0];
    const row = hist && hist.querySelector('tbody > tr');
    const out = { header: hist ? [...hist.querySelectorAll('thead th')].map((h) => h.textContent.trim()) : [] };
    if (row) {
      out.cols = [...row.children].map((td, i) => ({ i: i + 1, w: Math.round(td.getBoundingClientRect().width), lines: n(td), txt: (td.textContent || '').trim().replace(/\s+/g, ' ').slice(0, 30) }));
      out.rowH = Math.round(row.getBoundingClientRect().height);
    }
    const months = [...document.querySelectorAll('.history-filter input')].map((e) => { const x = e.getBoundingClientRect(); return `${Math.round(x.width)}x${Math.round(x.height)}`; });
    const others = [...document.querySelectorAll('.payer-form input, .cash-form input')].slice(0, 3).map((e) => { const x = e.getBoundingClientRect(); return `${Math.round(x.width)}x${Math.round(x.height)}`; });
    return { ...out, months, others };
  }, LINES);
  if (r.cols) r.cols.forEach((c) => report(`第${c.i}列 ${r.header[c.i - 1] || ''} ${c.w}px`, `${c.lines} 行  "${c.txt}"`));
  if (r.rowH) report('历史数据行高', r.rowH + 'px');
  report('起止月份输入框', r.months.join(' '));
  report('同页其它输入框', r.others.join(' '));
}

// ---- /cash-receipts/new : first control position, field spacing, readonly currency
await page.goto(BASE + '/cash-receipts/new', { waitUntil: 'networkidle' });
{
  console.log('\n== /cash-receipts/new');
  const r = await page.evaluate(() => {
    const fields = [...document.querySelectorAll('.cash-form label, .cash-form select, .cash-form input')];
    const first = document.querySelector('.cash-form select, .cash-form input');
    const firstY = first ? Math.round(first.getBoundingClientRect().top) : null;
    const labels = [...document.querySelectorAll('.cash-form label')].map((e) => Math.round(e.getBoundingClientRect().top));
    const gaps = labels.slice(1).map((y, i) => y - labels[i]).filter((g) => g > 0);
    const ro = [...document.querySelectorAll('.cash-form input[readonly], .cash-form input[disabled]')].map((e) => {
      const cs = getComputedStyle(e);
      return { v: e.value, bg: cs.backgroundColor, border: cs.borderColor };
    });
    const editable = [...document.querySelectorAll('.cash-form input:not([readonly]):not([disabled])')].slice(0, 2).map((e) => {
      const cs = getComputedStyle(e);
      return { bg: cs.backgroundColor, border: cs.borderColor };
    });
    return { firstY, gaps, fields: fields.length, ro, editable };
  });
  report('首个表单控件 y', r.firstY);
  report('字段间距', r.gaps.join(', '));
  report('只读输入框', JSON.stringify(r.ro));
  report('可编辑输入框', JSON.stringify(r.editable));
}

// ---- sort header: does the padding respond to a click?
await page.goto(BASE + '/rent-dashboard', { waitUntil: 'networkidle' });
{
  console.log('\n== 排序表头可点区域');
  const r = await page.evaluate(() => {
    const th = [...document.querySelectorAll('th')].find((h) => h.querySelector('a'));
    if (!th) return null;
    const a = th.querySelector('a');
    th.scrollIntoView({ block: 'center' });
    const tb = th.getBoundingClientRect(), ab = a.getBoundingClientRect();
    const probe = (x, y) => { const el = document.elementFromPoint(x, y); return el ? el.tagName.toLowerCase() + (el === a || a.contains(el) ? '(a)' : el === th ? '(th)' : '') : 'none'; };
    return {
      th: [Math.round(tb.width), Math.round(tb.height)], a: [Math.round(ab.width), Math.round(ab.height)],
      padTop: probe(tb.left + tb.width / 2, tb.top + 3),
      padBottom: probe(tb.left + tb.width / 2, tb.bottom - 3),
      onLink: probe(ab.left + ab.width / 2, ab.top + ab.height / 2),
    };
  });
  if (r) { report('th 盒', r.th.join('x')); report('其内 a 盒', r.a.join('x')); report('th 上内边距点命中', r.padTop); report('th 下内边距点命中', r.padBottom); report('链接中心命中', r.onLink); }
}

await browser.close();
