# 移动端适配校验与整改

## Goal

RentOps 目前只按桌面宽度设计过。本任务要给出**每个页面的移动端实测结论**，并修掉窄屏下真正出问题的排版，使房东能在手机上完成日常操作（看总览、查租客、核对流水、补录现金）。

价值：这是唯一的线上运维入口（https://bank.ddpl.top），手机上不可用等于外出时无法处理收租事务。

## Confirmed Facts

静态扫描（`cmd/truelayer-demo/`）已确认：

- **断点散乱**：全项目 11 个不同断点（`480 / 520 / 640 / 680 / 700 / 760 / 820 / 900 / 980 / 1040 / 1120`），另有 `@media(max-width:760px)` 漏写空格的写法。缺少统一的移动端策略。
- **存在固定像素列宽**，窄屏必然溢出或挤压：
  - `grid-template-columns: 140px 170px minmax(180px,1fr) minmax(160px,1fr) 100px`（最小 750px）
  - `grid-template-columns: 360px minmax(0,1fr)`、`minmax(0,1fr) 340px`
  - `grid-template-columns: 236px minmax(0,1fr)`（`.app` 侧边栏布局）
- **`.panel.surface` 正文需要各自带 padding**（卡片本身没有内边距）。租客详情页的四处已修；`/cash-receipts/void` 的 `.facts{grid-template-columns:150px 1fr}`（`cash_receipt_handlers.go:212`）有同样的缺陷，尚未修。
- 表格普遍用 `.table-wrap{overflow-x:auto}` 包裹，并配 `table{min-width:760px}` —— 桌面端是刻意的横向滚动，移动端需要确认这是否是可接受的交互。
- **两个预览页完全绕开 `workspacePageCSS`**，各自内联一套 `:root{--canvas/--ink/--muted}` 配色（`transaction_previews.go:38` 与 `:132`），与全站的 `--surface/--foreground/--foreground-muted` 不是同一体系；断点也各自为政（640px / 760px）。

真实浏览器实测已确认的三条硬事实（375×667，非静态推断）：

- **`/tenants` 不存在任何滚动位置能同时看到租客姓名与「详情/编辑」按钮**：表宽 680px、`.table-wrap` 可视宽 327px；姓名列占 x 0–78，操作列占 x 544–680，两者相距 466px，大于容器可视宽度 327px。实测 scroll=0 时可见列 0/1/2、隐藏列 3/4/5/6；滚到底时可见列 4/5/6、隐藏列 0/1/2。因此「滚到右边就能操作」不成立——到位后已无法辨认操作对象。
- **`/billing/payer/preview` 的横向滚动容器是整个 `main.card`**（`scrollRange` 415px，`containsHeading: true`，含 18px 内边距），即横滑时页标题、说明文字与「返回流水」链接一并滚出视野；与全站「只把表格包进 `.table-wrap`」的做法不一致。
- **`/expenses` 类别列实测 52px**，「物业」被逐字断成两行；日期/备注/付款方式在默认视口下全部在屏外（`scrollRange` 353px）。

## Scope

需要逐页校验的 10 个页面：

| 页面 | 路由 | 备注 |
| --- | --- | --- |
| 登录 | `/` | |
| 租客列表 + 表单 | `/tenants` | |
| 租客详情 | `/tenants/:id` | |
| 月度总览 | `/rent-dashboard` | |
| 银行流水 | `/billing` | |
| 支出记录 | `/expenses` | |
| 现金补录 | `/cash-receipts/new` | |
| 作废现金收款 | `/cash-receipts/void` | |
| 撤销匹配预览 | `GET /billing/revoke?transaction_id=N` | 首轮遗漏 |
| 历史付款人预览 | `POST /billing/payer/preview` | 首轮遗漏；handler 内部只读 |

后两页首轮遗漏的原因：它们不是普通链接。`/billing` 的内联脚本把撤销表单的 `method` 改写成 `get` 并删除 `reason` 字段（`billing_page.go:163`），所以预览页是 GET 可达的；`/billing/payer/preview` 只接受 POST（`transaction_handlers.go:186`），但 handler 只调用 `previewHistoricalPayerMatches`（纯查询），审计时发 POST 无副作用。

另需覆盖默认截图到不了的隐藏布局：租客列表展开行、总览页催缴抽屉。

## Requirements

- R1 每个页面在 375 / 390 / 768 三个视口下**不得出现横向溢出**（`documentElement.scrollWidth == clientWidth`）。
- R2 溢出定位必须给出**具体元素**（选择器 + 实际宽度），不能只给"页面有溢出"。
- R3 表格与宽内容允许横向滚动，但必须是**有意为之**且被容器包裹，不能撑破页面。
- R4 触控目标不小于 44×44px（按钮、行操作、分页）。
- R5 校验所用的页面必须**带真实数据**（demo 账号的 10 个租客），不能用空数据蒙混过关。
- R6 修复后的判定必须基于**真实浏览器渲染**，不接受纯静态推断作为验收证据。

## Acceptance Criteria

- [ ] 产出 `research/mobile-audit.md`：8 个页面 × 3 个视口的溢出结论，含逐元素的溢出宽度与选择器。
- [ ] 产出截图证据目录，覆盖 8 个页面 × 3 个视口 + 2 个隐藏布局状态。
- [ ] 整改后重跑审计，全部页面在 375px 下 `horizontalOverflow == 0`。
- [ ] 触控目标检查通过（列出小于 44px 的交互元素并说明取舍）。
- [ ] `go test ./...` 全绿，并为修复补上可回归的布局断言（沿用 `billing_layout_test.go` / `dashboard_layout_test.go` / `tenant_detail_layout_test.go` 的写法）。
- [ ] 结论写入任务 `research/`，修复前先经用户确认优先级。

## Out of Scope

- 原生 App / PWA 改造、离线能力。
- 桌面端视觉重做：移动端修复不得改变桌面端现有呈现。
- 新增功能或信息架构调整（例如把表格改成卡片流）——除非审计证明不做就无法通过 R1，届时需单独确认。

## 已决事项（原开放问题，2026-09-16 经用户确认）

| 决策 | 选择 |
| --- | --- |
| 修复范围 | **P0 + P1**（不含宽表改卡片流） |
| 关键列策略 | **冻结最右列**；`/billing` 为例外，窄屏换成 88px「状态 + 处理」列并就地展开 |
| 侧边栏形态 | **顶部紧凑栏 56px + 汉堡抽屉**（纯 CSS 复选框方案） |
| 断点 | **不重指旧断点**，本次新增规则统一挂 `≤640px` |
| 外壳改造 | **先抽公共外壳再改**（7 份活副本收敛为 1 处） |
| 验收视口 | 375 / 390 / 768，另加 ≥1280px 宽屏做「桌面端未变」比对 |

`legacyBillingTemplate`（`main.go:2658`）经查为死代码（全仓无渲染点），不在范围内。
两个预览页与全站不一致的 `:root` 配色体系不在范围内（改动会波及桌面端，违反 Out of Scope 第 2 条）。
