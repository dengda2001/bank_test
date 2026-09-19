// Per-pixel PNG diff with a bounding box, so "these two screenshots differ" turns
// into "they differ in this rectangle". No ImageMagick on this box.
// Usage: node imgdiff.mjs a.png b.png
import { chromium } from 'playwright';
import { launchOptions } from './launch.mjs';
import fs from 'node:fs';

const [a, b] = process.argv.slice(2);
const toDataURL = (p) => 'data:image/png;base64,' + fs.readFileSync(p).toString('base64');
const browser = await chromium.launch(launchOptions());
const page = await browser.newPage();
const out = await page.evaluate(async ([da, db]) => {
  const load = (src) => new Promise((res, rej) => { const i = new Image(); i.onload = () => res(i); i.onerror = rej; i.src = src; });
  const ia = await load(da), ib = await load(db);
  if (ia.width !== ib.width || ia.height !== ib.height) {
    return { sizeMismatch: true, a: [ia.width, ia.height], b: [ib.width, ib.height] };
  }
  const cv = document.createElement('canvas');
  cv.width = ia.width; cv.height = ia.height;
  const ctx = cv.getContext('2d', { willReadFrequently: true });
  ctx.drawImage(ia, 0, 0);
  const A = ctx.getImageData(0, 0, cv.width, cv.height).data;
  ctx.clearRect(0, 0, cv.width, cv.height);
  ctx.drawImage(ib, 0, 0);
  const B = ctx.getImageData(0, 0, cv.width, cv.height).data;
  let n = 0, x0 = 1e9, y0 = 1e9, x1 = -1, y1 = -1, maxDelta = 0;
  for (let i = 0; i < A.length; i += 4) {
    const d = Math.abs(A[i] - B[i]) + Math.abs(A[i + 1] - B[i + 1]) + Math.abs(A[i + 2] - B[i + 2]);
    if (d > 6) {
      n++;
      if (d > maxDelta) maxDelta = d;
      const p = i / 4, x = p % cv.width, y = (p / cv.width) | 0;
      if (x < x0) x0 = x; if (x > x1) x1 = x;
      if (y < y0) y0 = y; if (y > y1) y1 = y;
    }
  }
  return { w: cv.width, h: cv.height, n, bbox: n ? [x0, y0, x1, y1] : null, maxDelta };
}, [toDataURL(a), toDataURL(b)]);
console.log(JSON.stringify(out));
await browser.close();
