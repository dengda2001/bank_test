// Slice each page into viewport-sized tiles so the screenshots stay legible at
// 375px wide. A 7400px-tall full-page shot is unreadable once scaled down.
import { chromium } from 'playwright';
import fs from 'node:fs';
import path from 'node:path';

const BASE = process.env.AUDIT_BASE || 'http://localhost:8081';
const USER = process.env.AUDIT_USER;
const PASS = process.env.AUDIT_PASS;
const OUT = process.env.TILE_OUT || '/tmp/mobile-audit/tiles';
const W = 375, H = 667, MAX_TILES = 9;

fs.rmSync(OUT, { recursive: true, force: true });
fs.mkdirSync(OUT, { recursive: true });

const browser = await chromium.launch({
  executablePath: '/tmp/mobile-audit/chrome-linux64/chrome',
  args: ['--no-sandbox', '--disable-dev-shm-usage'],
});

async function login(page) {
  await page.goto(BASE + '/', { waitUntil: 'domcontentloaded' });
  if (await page.locator('#username').count()) {
    await page.fill('#username', USER);
    await page.fill('#password', PASS);
    await Promise.all([
      page.waitForNavigation({ waitUntil: 'domcontentloaded' }).catch(() => {}),
      page.click('button[type=submit]'),
    ]);
  }
}

async function tile(page, name, prepare) {
  if (prepare) await prepare();
  await page.waitForTimeout(400);
  const height = await page.evaluate(() => document.documentElement.scrollHeight);
  const tiles = Math.min(Math.ceil(height / H), MAX_TILES);
  const made = [];
  for (let i = 0; i < tiles; i++) {
    await page.evaluate((y) => window.scrollTo(0, y), i * H);
    await page.waitForTimeout(250);
    const file = path.join(OUT, `${name}__${String(i).padStart(2, '0')}.png`);
    await page.screenshot({ path: file });
    made.push(path.basename(file));
  }
  console.log(`${name.padEnd(26)} height=${height} tiles=${made.length}${height > tiles * H ? ' (TRUNCATED)' : ''}`);
  return made;
}

const index = {};
const ctx = await browser.newContext({ viewport: { width: W, height: H }, deviceScaleFactor: 2 });
const page = await ctx.newPage();
await login(page);

for (const [name, url, prep] of [
  ['tenants', '/tenants', null],
  ['tenant-detail', null, null],
  ['rent-dashboard', '/rent-dashboard', null],
  ['billing', '/billing', null],
  ['expenses', '/expenses', null],
  ['cash-receipt-new', '/cash-receipts/new', null],
]) {
  if (url) await page.goto(BASE + url, { waitUntil: 'networkidle' });
  else {
    // tenant-detail needs a real id
    const id = await page.evaluate(async (b) => {
      const r = await fetch(b + '/tenants');
      const t = await r.text();
      return (t.match(/\/tenants\/(\d+)"/) || [])[1];
    });
    await page.goto(`${BASE}/tenants/${id || 6}`, { waitUntil: 'networkidle' });
  }
  index[name] = await tile(page, name, prep);
}

// States the default page never shows.
await page.goto(BASE + '/tenants', { waitUntil: 'networkidle' });
index['tenants-row-expanded'] = await tile(page, 'tenants-row-expanded', async () => {
  await page.locator('.tenant-row').first().click();
});

await page.goto(BASE + '/rent-dashboard', { waitUntil: 'networkidle' });
index['dashboard-dunning-drawer'] = await tile(page, 'dashboard-dunning-drawer', async () => {
  await page.locator('[data-dunning-open]').first().click();
});

await ctx.close();

// Login is only reachable logged out.
const anon = await browser.newContext({ viewport: { width: W, height: H }, deviceScaleFactor: 2 });
const anonPage = await anon.newPage();
await anonPage.goto(BASE + '/', { waitUntil: 'networkidle' });
index['login'] = await tile(anonPage, 'login');
await anon.close();

await browser.close();
fs.writeFileSync(path.join(OUT, 'index.json'), JSON.stringify(index, null, 2));
console.log('\nwrote ' + OUT);
