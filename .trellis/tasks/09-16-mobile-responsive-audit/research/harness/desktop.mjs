// Desktop regression baseline: full-page screenshots at 1440x900.
// The mobile work must not move anything at >=1280px, and a screenshot pair is
// the only way to see a silent shift in the frozen column, the shell or the type
// scale. Run once before the change and once after, then compare.
import { chromium } from 'playwright';
import fs from 'node:fs';

const BASE = process.env.AUDIT_BASE || 'http://localhost:8081';
const OUT = process.env.AUDIT_OUT || '/tmp/mobile-audit/desktop';
const PAGES = [
  ['tenants', '/tenants'],
  ['rent-dashboard', '/rent-dashboard'],
  ['billing', '/billing'],
  ['expenses', '/expenses'],
];

fs.mkdirSync(OUT, { recursive: true });
const browser = await chromium.launch({
  executablePath: process.env.CHROME_BIN || '/tmp/mobile-audit/chrome-linux64/chrome',
  args: ['--no-sandbox', '--disable-dev-shm-usage'],
});
const ctx = await browser.newContext({ viewport: { width: 1440, height: 900 } });
const page = await ctx.newPage();

await page.goto(BASE + '/', { waitUntil: 'domcontentloaded' });
await page.fill('#username', process.env.AUDIT_USER);
await page.fill('#password', process.env.AUDIT_PASS);
await Promise.all([page.waitForNavigation({ waitUntil: 'domcontentloaded' }).catch(() => {}), page.click('button[type=submit]')]);

for (const [name, url] of PAGES) {
  const resp = await page.goto(BASE + url, { waitUntil: 'networkidle', timeout: 30000 });
  const path = `${OUT}/${name}.png`;
  await page.screenshot({ path, fullPage: true });
  const geo = await page.evaluate(() => ({
    scrollW: document.documentElement.scrollWidth,
    clientW: document.documentElement.clientWidth,
    h: document.documentElement.scrollHeight,
    sidebar: document.querySelector('.sidebar')?.getBoundingClientRect().width ?? null,
  }));
  console.log(`${name.padEnd(16)} ${resp.status()} scrollW=${geo.scrollW} clientW=${geo.clientW} h=${geo.h} sidebar=${geo.sidebar}`);
}

await browser.close();
console.log('wrote ' + OUT);
