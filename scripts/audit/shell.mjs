import { chromium } from 'playwright';
import { launchOptions } from './launch.mjs';
const b = await chromium.launch(launchOptions());
const c = await b.newContext({ viewport: { width: 375, height: 667 } });
const p = await c.newPage();
await p.goto('http://localhost:8081/', { waitUntil: 'domcontentloaded' });
await p.fill('#username', process.env.AUDIT_USER); await p.fill('#password', process.env.AUDIT_PASS);
await Promise.all([p.waitForNavigation({waitUntil:'domcontentloaded'}).catch(()=>{}), p.click('button[type=submit]')]);
await p.goto('http://localhost:8081/tenants', { waitUntil: 'networkidle' });
const m = await p.evaluate(() => {
  const g = (s) => { const e = document.querySelector(s); if (!e) return null; const r = e.getBoundingClientRect(); return { w: Math.round(r.width), h: Math.round(r.height) }; };
  return { viewportH: window.innerHeight, sidebar: g('.sidebar'), topbar: g('.topbar'), nav: g('.nav'),
           firstPanel: g('.panel.surface'), contentTop: Math.round(document.querySelector('.content').getBoundingClientRect().top) };
});
console.log(JSON.stringify(m, null, 2));
console.log(`\n侧边栏占首屏: ${Math.round(m.sidebar.h / m.viewportH * 100)}%  首个面板开始于 y=${m.contentTop} (首屏的 ${Math.round(m.contentTop/m.viewportH*100)}%)`);
await b.close();
