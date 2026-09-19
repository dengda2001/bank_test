import { chromium } from 'playwright';
import { launchOptions } from './launch.mjs';
const BASE = 'http://localhost:8081';
const b = await chromium.launch(launchOptions());
const ctx = await b.newContext({ viewport: { width: 375, height: 900 } });
const p = await ctx.newPage();
await p.goto(BASE + '/', { waitUntil: 'domcontentloaded' });
await p.fill('#username', process.env.AUDIT_USER); await p.fill('#password', process.env.AUDIT_PASS);
await Promise.all([p.waitForNavigation({ waitUntil: 'domcontentloaded' }).catch(()=>{}), p.click('button[type=submit]')]);
await p.goto(BASE + '/billing', { waitUntil: 'networkidle' });
await Promise.all([
  p.waitForNavigation({ waitUntil: 'networkidle', timeout: 20000 }).catch(()=>{}),
  p.evaluate(() => { const f = document.createElement('form'); f.method='POST'; f.action='/billing/payer/preview'; document.body.appendChild(f); f.submit(); }),
]);
console.log('url:', p.url());
console.log(await p.evaluate(() => {
  const forms = [...document.querySelectorAll('form.action')];
  return {
    rows: document.querySelectorAll('tbody tr').length,
    confirmForms: forms.length,
    components: [...document.querySelectorAll('a,button,input,select,textarea,label.muted')]
      .map(e => { const r = e.getBoundingClientRect(); return `${e.tagName.toLowerCase()}${e.type?'['+e.type+']':''}${e.name?' name='+e.name:''}${e.className==='muted'?' label.muted':''} ${Math.round(r.width)}x${Math.round(r.height)}`; })
      .filter(s => !s.includes('x0')),
  };
}));
await b.close();
