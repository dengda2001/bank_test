import { chromium } from 'playwright';
import { launchOptions } from './launch.mjs';
const BASE = 'http://127.0.0.1:18092';
const b = await chromium.launch(launchOptions());
const c = await b.newContext({ viewport: { width: 390, height: 844 } });
const p = await c.newPage();
await p.goto(BASE + '/', { waitUntil: 'domcontentloaded' });
await p.fill('#username', process.env.AUDIT_USER); await p.fill('#password', process.env.AUDIT_PASS);
await Promise.all([p.waitForNavigation({ waitUntil: 'domcontentloaded' }).catch(()=>{}), p.click('button[type=submit]')]);
for (const [n,u] of [['property-detail','/properties/1?period=2026-09'],['room-detail','/rooms/5?period=2026-09'],['tenant-detail','/tenants/3'],['transaction-detail','/transactions?detail=1']]) {
  await p.goto(BASE+u,{waitUntil:'networkidle'});
  const r = await p.evaluate(()=>({cw:document.documentElement.clientWidth,sw:document.documentElement.scrollWidth,
    off:[...document.querySelectorAll('body *')].filter(e=>{const r=e.getBoundingClientRect();return r.width>0&&r.height>0&&r.right>document.documentElement.clientWidth+1&&!(e.parentElement&&e.parentElement.getBoundingClientRect().right>document.documentElement.clientWidth+1);}).slice(0,4).map(e=>e.tagName+'.'+(typeof e.className==='string'?e.className.trim().split(/\s+/).join('.'):''))}));
  console.log(n.padEnd(20), 'client='+r.cw, 'scroll='+r.sw, 'overflow='+(r.sw-r.cw), r.off.length?('OFFENDERS '+JSON.stringify(r.off)):'');
}
await b.close();
