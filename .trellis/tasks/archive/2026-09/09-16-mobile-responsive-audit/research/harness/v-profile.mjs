import { chromium } from 'playwright';
const BASE='http://localhost:8081';
const b=await chromium.launch({executablePath:'/tmp/mobile-audit/chrome-linux64/chrome',args:['--no-sandbox','--disable-dev-shm-usage']});
const c=await b.newContext({viewport:{width:375,height:667}});
const p=await c.newPage();
await p.goto(BASE+'/',{waitUntil:'domcontentloaded'});
await p.fill('#username',process.env.U); await p.fill('#password',process.env.P);
await Promise.all([p.waitForNavigation({waitUntil:'domcontentloaded'}).catch(()=>{}),p.click('button[type=submit]')]);
const id=await p.evaluate(async()=>{const r=await fetch('/tenants');return ((await r.text()).match(/\/tenants\/(\d+)"/)||[])[1];});
await p.goto(`${BASE}/tenants/${id}`,{waitUntil:'networkidle'});
const r=await p.evaluate(()=>{
  const dl=document.querySelector('.profile-list');
  if(!dl) return {error:'no .profile-list'};
  const cs=getComputedStyle(dl);
  const inkW=(el)=>{const rg=document.createRange();rg.selectNodeContents(el);return Math.round(rg.getBoundingClientRect().width);};
  const lines=(el)=>{const rg=document.createRange();rg.selectNodeContents(el);return rg.getClientRects().length;};
  return {
    cols:cs.gridTemplateColumns, gap:cs.gap, dlW:Math.round(dl.getBoundingClientRect().width),
    dts:[...dl.querySelectorAll('dt')].map(d=>({t:d.textContent.trim(),w:Math.round(d.getBoundingClientRect().width),textW:inkW(d)})),
    dds:[...dl.querySelectorAll('dd')].map(d=>({t:d.textContent.trim().slice(0,36),w:Math.round(d.getBoundingClientRect().width),h:Math.round(d.getBoundingClientRect().height),lines:lines(d)})),
  };
});
console.log('grid-template-columns:',r.cols,'| gap:',r.gap,'| dl 宽:',r.dlW);
console.log('\n标签列  占用宽 / 文字实宽 / 浪费');
for(const d of r.dts) console.log(`  ${d.t.padEnd(6)} ${String(d.w).padStart(4)}px  text=${String(d.textW).padStart(3)}px  waste=${d.w-d.textW}px`);
console.log('\n值列   宽 / 行数 / 高');
for(const d of r.dds) console.log(`  ${String(d.w).padStart(4)}px  ${d.lines}行  h=${String(d.h).padStart(3)}  "${d.t}"`);
await b.close();
