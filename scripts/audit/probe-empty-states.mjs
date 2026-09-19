// Every list page must say something when a filter matches nothing.
//
// A zero-row page that renders as a bare table header looks like a broken page,
// not like an empty result, and the user cannot tell "no matches" from "still
// loading". This walks each list page with a filter value that cannot match and
// asserts a visible empty-state message appears -- at both a mobile and a
// desktop width, because the property/room pages render two presentations.
//
// Read-only.
import { chromium } from 'playwright';
import { launchOptions } from './launch.mjs';

const BASE = (process.env.AUDIT_BASE || 'http://127.0.0.1:18097').replace(/\/+$/, '');
const USER = process.env.AUDIT_USER;
const PASS = process.env.AUDIT_PASS;
const PERIOD = process.env.AUDIT_PERIOD || '2026-09';

if (!USER || !PASS) {
  console.error('set AUDIT_USER and AUDIT_PASS (see the launcher output)');
  process.exit(1);
}

// A search term no fixture can contain, so the result set is empty by
// construction rather than by luck.
const IMPOSSIBLE = 'ZZZNOMATCHZZZ';
// The free-text filter is not called `search` everywhere: /transactions filters
// by payer. Feeding `search` to a page that does not read it changes nothing and
// would report a false failure, so the parameter is named per page.
// /tenants is deliberately absent: it renders no filter form at all (see
// 09-19-list-pages-alignment AC 3), so it has no zero-row state to reach.
const PAGES = [
  ['/properties', 'properties', 'search'],
  ['/rooms', 'rooms', 'search'],
  ['/tenancies', 'tenancies', 'search'],
  ['/bills', 'bills', 'search'],
  ['/expenses', 'expenses', 'search'],
  ['/cash-receipts', 'cash-receipts', 'search'],
  ['/transactions', 'transactions', 'payer'],
];
const WIDTHS = [390, 1440];

const browser = await chromium.launch(launchOptions());
const ctx = await browser.newContext({ viewport: { width: 1440, height: 900 } });
const page = await ctx.newPage();
await page.goto(BASE + '/', { waitUntil: 'domcontentloaded' });
if (await page.$('#username')) {
  await page.fill('#username', USER);
  await page.fill('#password', PASS);
  await Promise.all([page.waitForNavigation({ waitUntil: 'domcontentloaded' }), page.click('button[type=submit]')]);
}

const SELECTORS = '.object-list-empty, .empty, .workspace-empty, .empty-state';
let failures = 0;

for (const [base, name, param] of PAGES) {
  for (const width of WIDTHS) {
    await page.setViewportSize({ width, height: 900 });
    const sep = base.includes('?') ? '&' : '?';
    await page.goto(`${BASE}${base}${sep}period=${PERIOD}&${param}=${IMPOSSIBLE}`, { waitUntil: 'networkidle' });
    const result = await page.evaluate((sel) => {
      const visible = (el) => {
        const r = el.getBoundingClientRect();
        const s = getComputedStyle(el);
        return r.width > 0 && r.height > 0 && s.display !== 'none' && s.visibility !== 'hidden';
      };
      const candidates = Array.from(document.querySelectorAll(sel)).filter(visible);
      const text = (el) => (el.textContent || '').replace(/\s+/g, ' ').trim();
      // Row count, scoped so the desktop table and the mobile cards are not
      // double-counted: count each entity once.
      const rows = document.querySelectorAll('tbody tr').length
        + Array.from(document.querySelectorAll('.object-mobile-list > article, .tenant-mobile-card, .property-room-card')).length;
      return { emptyVisible: candidates.length > 0, message: candidates.map(text).find((t) => t) || null, rows };
    }, SELECTORS);
    const ok = result.emptyVisible && result.rows === 0;
    if (!ok) failures++;
    console.log(`${ok ? 'ok  ' : 'FAIL'} ${name.padEnd(14)} @${String(width).padEnd(5)} rows=${result.rows} empty=${result.emptyVisible} ${result.message ? JSON.stringify(result.message).slice(0, 60) : ''}`);
  }
}

await browser.close();
console.log(failures === 0 ? '\nall pages state an empty result' : `\n${failures} page/width combination(s) show nothing`);
process.exit(failures === 0 ? 0 : 1);
