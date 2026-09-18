# 移动端 Figma 还原验收

## 实现切片

- 共享移动 shell：五项底部导航、对象／更多飞出菜单、44px 触控目标、底栏安全区。
- 首页：双列指标和租金卡片，平账入口要求填写原因并强确认。
- 租客：移动卡片展示身份、房间、月租、详情／编辑和最近缴费。
- 支出：移动卡片展示日期、金额、付款方式、归属、租客备注和发票 URL。
- 流水：每条流水重排为卡片，状态／金额／处理入口同屏，原有匹配、归类、撤销表单继续复用。
- 对象及更多页面：简单宽表在移动端重排为纵向卡片，房产／房间／租住安排／现金／银行空状态保持可读。

## 浏览器结果

Chrome 移动仿真视口 360×960、390×960、430×960、600×960 逐页访问：

`/rent-dashboard`、`/bills`、`/transactions`、`/properties`、`/rooms`、`/tenants`、`/tenancies`、`/dunning`、`/cash-receipts`、`/expenses`、`/bank`。

每个页面均满足 `document.documentElement.scrollWidth === clientWidth`，底部导航高度为 68px；600px 时底栏居中在 560px 工作台内。390px 有数据状态截图：

- `390-dashboard-cards.png`
- `390-billing-cards.png`
- `390-tenants-cards.png`
- `390-expenses-cards.png`

## 自动化验证

- `go test ./cmd/truelayer-demo/... -count=1`：仅剩既有租客日期 fixture 的两个失败：
  `TestTenantActiveInMonthUsesRentDatesWithoutProration`、`TestTenantActiveInMonthRespectsBillingStartDate`。
- `go vet ./...`：通过。
- `./scripts/run-mysql-test-clean.sh ./cmd/truelayer-demo -run '^TestCanonicalPropertyPagesAreUserScopedOnMySQL$' -count=1`：通过，测试库自动清理。

浏览器验证期间创建的临时房产和支出已按 ID 定向删除，开发数据库无残留验证数据。
