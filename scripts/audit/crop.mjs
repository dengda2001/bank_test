import { chromium } from 'playwright';
import { launchOptions } from './launch.mjs';
import fs from 'node:fs';
const [,, file, x, y, w, h, scale, out] = process.argv;
const X=+x,Y=+y,W=+w,H=+h,S=+scale;
const b64 = fs.readFileSync('/tmp/mobile-audit/tiles/' + file).toString('base64');
const browser = await chromium.launch(launchOptions());
const ctx = await browser.newContext({ viewport: { width: 1500, height: 1100 }, deviceScaleFactor: 1 });
const page = await ctx.newPage();
await page.setContent(`<html><body style="margin:0;background:#fff">
<div style="width:${Math.ceil(W*S)}px;height:${Math.ceil(H*S)}px;overflow:hidden;position:relative">
<img src="data:image/png;base64,${b64}" style="position:absolute;left:${-X*S}px;top:${-Y*S}px;width:${750*S}px;image-rendering:pixelated">
</div></body></html>`);
await page.waitForTimeout(600);
await page.screenshot({ path: out, clip: { x: 0, y: 0, width: Math.min(Math.ceil(W*S),1500), height: Math.min(Math.ceil(H*S),1100) } });
await browser.close();
console.log('ok', out, Math.min(Math.ceil(W*S),1500), Math.min(Math.ceil(H*S),1100));
