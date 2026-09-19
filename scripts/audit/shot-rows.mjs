// Clip a screenshot to the first few rows of a table, at phone width, so a layout
// that has only been measured can also be looked at.
import { chromium } from 'playwright';
import { launchOptions } from './launch.mjs';
import fs from 'node:fs';

const BASE = process.env.AUDIT_BASE || 'http://localhost:8081';
const W = Number(process.env.PROBE_W || 375);
const URL_PATH = process.env.PROBE_URL || '/billing';
const OUT = process.env.OUT || '/tmp/mobile-audit/rows.png';
const ROWS = Number(process.env.PROBE_ROWS || 2);

const browser = await chromium.launch(launchOptions());
const ctx = await browser.newContext({ viewport: { width: W, height: 1000 }, deviceScaleFactor: 1 });
const page = await ctx.newPage();
await page.goto(BASE + '/', { waitUntil: 'domcontentloaded' });
await page.fill('#username', process.env.AUDIT_USER);
await page.fill('#password', process.env.AUDIT_PASS);
await Promise.all([page.waitForNavigation({ waitUntil: 'domcontentloaded' }).catch(() => {}), page.click('button[type=submit]')]);
await page.goto(BASE + URL_PATH, { waitUntil: 'networkidle', timeout: 30000 });

const clip = await page.evaluate((n) => {
  const wrap = document.querySelector('main.content section .table-wrap') || document.querySelector('.table-wrap');
  const thead = wrap.querySelector('thead');
  const rows = [...wrap.querySelectorAll('tbody > tr')].slice(0, n);
  const first = thead.getBoundingClientRect();
  const last = rows[rows.length - 1].getBoundingClientRect();
  return { x: 0, y: Math.round(first.top - 6), width: document.documentElement.clientWidth, height: last.bottom - first.top + 12 };
}, ROWS);

await page.screenshot({ path: OUT, clip, fullPage: true });
fs.writeFileSync(OUT.replace(/\.png$/, '.txt'), JSON.stringify(clip));
console.log(`wrote ${OUT} clip=${JSON.stringify(clip)}`);
await browser.close();
