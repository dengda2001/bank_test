import { chromium } from 'playwright';
const exe = '/tmp/mobile-audit/chrome-linux64/chrome';
try {
  const b = await chromium.launch({ executablePath: exe, args: ['--no-sandbox', '--disable-dev-shm-usage'] });
  console.log('LAUNCH OK, version:', b.version());
  const p = await b.newPage();
  await p.setContent('<h1 style="font-size:40px">测试 中文</h1>');
  console.log('page title ok:', await p.evaluate(() => document.querySelector('h1').textContent));
  await b.close();
} catch (e) {
  console.log('LAUNCH FAILED:', String(e).split('\n').slice(0, 12).join('\n'));
  process.exit(1);
}
