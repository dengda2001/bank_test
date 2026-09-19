import { chromium } from 'playwright';
import { launchOptions } from './launch.mjs';
const BASE = process.env.AUDIT_BASE || 'http://localhost:8081';
const b = await chromium.launch(launchOptions());
const ctx = await b.newContext({ viewport: { width: 375, height: 800 } });
const p = await ctx.newPage();
await p.goto(BASE + '/', { waitUntil: 'domcontentloaded' });
await p.fill('#username', process.env.AUDIT_USER); await p.fill('#password', process.env.AUDIT_PASS);
await Promise.all([p.waitForNavigation({ waitUntil: 'domcontentloaded' }).catch(()=>{}), p.click('button[type=submit]')]);
await p.goto(BASE + '/rent-dashboard', { waitUntil: 'networkidle' });
console.log(await p.evaluate(() => [...document.querySelectorAll('input, select, textarea, a.month-nav')].map((e) => {
  const r = e.getBoundingClientRect();
  return `${e.tagName.toLowerCase()} type=${e.getAttribute('type')||'-'} class="${e.className}" name=${e.name||'-'} ${Math.round(r.width)}x${Math.round(r.height)}`;
}).join('\n')));
await b.close();
