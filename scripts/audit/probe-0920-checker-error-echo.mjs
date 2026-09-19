// Checker probe: does any page echo ?error=<value> verbatim into a notice?
// Read-only GETs.
import { chromium } from 'playwright';
import { launchOptions } from './launch.mjs';

const BASE = (process.env.AUDIT_BASE || 'http://127.0.0.1:18097').replace(/\/+$/, '');
const USER = process.env.AUDIT_USER;
const PASS = process.env.AUDIT_PASS;
const PERIOD = process.env.AUDIT_PERIOD || '2026-09';
const MARKER = 'INJECT-7f3a91';

const withQuery = (path, params) => {
  const u = new URL(BASE + path);
  for (const [k, v] of Object.entries(params)) u.searchParams.set(k, v);
  return u.toString();
};

const browser = await chromium.launch(launchOptions());
const ctx = await browser.newContext({ viewport: { width: 1440, height: 900 } });
const page = await ctx.newPage();

await page.goto(BASE + '/', { waitUntil: 'domcontentloaded' });
if (await page.$('#username')) {
  await page.fill('#username', USER);
  await page.fill('#password', PASS);
  await Promise.all([page.waitForNavigation({ waitUntil: 'domcontentloaded' }), page.click('button[type=submit]')]);
}

for (const path of ['/bank', '/cash-receipts', '/properties', '/rooms', '/tenancies', '/bills', '/dunning', '/expenses']) {
  await page.goto(withQuery(path, { period: PERIOD, status: 'all', error: MARKER }), { waitUntil: 'networkidle' });
  const r = await page.evaluate((m) => {
    const body = document.body.innerText;
    const notices = [...document.querySelectorAll('.notice')].map((el) => ({
      cls: el.className,
      text: el.textContent.trim(),
    }));
    return { inBody: body.includes(m), notices };
  }, MARKER);
  const leaking = r.notices.filter((n) => n.text.includes(MARKER));
  console.log(
    `${leaking.length ? 'LEAK' : 'ok  '}  ${path}?error=<marker>  inBody=${r.inBody} leakingNotices=${JSON.stringify(leaking)}`,
  );
}

await browser.close();
