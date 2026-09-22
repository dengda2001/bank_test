// 三件事：
//   1. 房产详情窄屏那张「未收」卡，欠着钱时到底有没有着色（只有 ≤640px 才显示）。
//   2. /transactions 已关联那一行的「修改匹配」「撤销匹配」在桌面上能不能用。
//   3. 同一个出口在窄屏卡片上有没有（桌面表格 ≤640px 是藏起来的）。
import { chromium } from 'playwright';

const EXE = '/Users/dd/Library/Caches/ms-playwright/chromium-1178/chrome-mac/Chromium.app/Contents/MacOS/Chromium';
const BASE = 'http://127.0.0.1:8099';
const browser = await chromium.launch({ executablePath: EXE });

// --- 1. 未收卡 ---
{
  const page = await browser.newPage({ viewport: { width: 390, height: 844 } });
  await page.goto(`${BASE}/property-detail-mobile.html`, { waitUntil: 'load' });
  const card = page.locator('.property-mobile-unpaid');
  console.log('--- 未收卡 @390 ---');
  console.log('可见:', await card.isVisible(), await card.evaluate((el) => {
    const cs = getComputedStyle(el);
    return {
      classes: el.className, 金额: el.querySelector('strong').textContent,
      底色: cs.backgroundColor, 左边框: `${cs.borderLeftWidth} ${cs.borderLeftColor}`,
      金额色: getComputedStyle(el.querySelector('strong')).color,
    };
  }));
  await page.close();
}

// --- 2 + 3. 改错出口 ---
for (const [宽度, label] of [[1440, '桌面'], [390, '窄屏']]) {
  const page = await browser.newPage({ viewport: { width: 宽度, height: 900 } });
  await page.goto(`${BASE}/transactions.html`, { waitUntil: 'load' });
  const scope = 宽度 > 640 ? '.transaction-route-desktop-table' : '.transaction-route-mobile-list';
  const row = page.locator(`${scope} tr, ${scope} article`).filter({ has: page.locator('summary', { hasText: '撤销匹配' }) }).first();
  console.log(`\n--- ${label} @${宽度} ---`);
  console.log('改错出口:', await row.locator('summary').allTextContents());

  await row.locator('summary', { hasText: '修改匹配' }).click();
  const rematch = row.locator('form[action$="/rematch"]');
  await rematch.waitFor({ state: 'visible' });
  console.log('修改表单:', await rematch.evaluate((f) => ({
    method: f.method, action: f.getAttribute('action'),
    return_to: f.querySelector('input[name="return_to"]')?.value,
    租客: [...f.querySelectorAll('select[name="tenant_id"] option')].map((o) => o.textContent),
    月份: [...f.querySelectorAll('select[name="period"] option')].map((o) => o.textContent),
  })));

  await row.locator('summary', { hasText: '撤销匹配' }).click();
  const revoke = row.locator('form[action$="/revoke"]');
  await revoke.waitFor({ state: 'visible' });
  console.log('撤销表单:', await revoke.evaluate((f) => ({
    method: f.method, action: f.getAttribute('action'),
    字段: [...f.querySelectorAll('input')].map((i) => `${i.name}=${i.value}`),
    原因框还在吗: !!f.querySelector('input[name="reason"]'),
  })));
  if (宽度 > 640) {
    console.log('表格:', await page.evaluate(() => {
      const w = document.querySelector('.table-wrap');
      return { 容器: w.clientWidth, 表格: w.querySelector('table').scrollWidth, 横向滚动: w.scrollWidth > w.clientWidth };
    }));
  }
  await page.screenshot({ path: `/tmp/actions-${宽度}.png` });
  await page.close();
}
await browser.close();
