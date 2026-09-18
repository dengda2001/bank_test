# PRD：移动卡片基元与 `/billing` 卡片流

父任务：`09-16-mobile-friendly-workspace`（移动端友好版工作台）

## Goal

建立全站移动卡片的基元（卡片容器、卡片、摘要行、展开区、状态标签），并在最复杂的页面 `/billing` 上第一次落地。

`/billing` 是收租日常的核心：筛选项最多（7 个可见 + 3 个隐藏参数）、每行动作最多（一个约 4KB 的行内 `<td>`）、金额与匹配信息必须同时阅读。卡片基元在这里被真实需求压测——如果抽象不够用，在这里暴露比在后两个页面暴露更早也更便宜。

## 依赖与启动前门禁

- **前置**：`09-17-mobile-audit-fixtures` 必须已归档。本任务需要在真实浏览器里验收卡片布局的几何（44px 命中区、金额不截断、无横向溢出），Go 字符串断言看不见这类缺陷——上一轮实测已有三个这样的缺陷漏过字符串测试。
- **共享契约**：父任务 `design.md`。本任务的实现必须满足它 §4 的七条子任务契约。

## Confirmed Facts

研究见父任务 `research/billing-render-map.md`。

- 行渲染是 `{{range .TransactionRows}}` + 字面量 `<tr>`，消费逐行 view model `transactionPageRow`（`transactions.go:371-412`），显示字符串与动作开关都已预计算——**卡片可直接复用，零新增数据管道**。
- 表格 7 列：类型、付款人、金额／余额、到账／租金月、描述、账户、用途／处理。≤640px 时已隐藏账户与描述两列。
- `pending` 筛选已在 SQL 实现（`transactions.go:592`：`direction='income' AND match_status IN ('candidate','needs_review','unmatched','partial')`），不是读时计算。
- 匹配状态下拉的「全部状态」选项是 `value=""`（`billing_page.go:170`）——默认值变更后这个值必须重新定义。
- `validateTransactionFilters`（`transactions.go:299-303`）**不接受** `"all"`。
- 页面行内脚本（`billing_page.go:217`）做三件事：`<details>` 折叠、把 `/billing/allocate` 改写成可重复的分割行、把 `/billing/revoke` 改成 GET 并删掉 `reason`。**卡片路径不能直接复用**——三件事里只有折叠是卡片需要的。
- 状态徽章链接 `/billing?match_status={{.MatchStatus}}`（`billing_page.go:193`）今天会丢掉其他全部筛选条件。

## Requirements

- R1 卡片基元建成可复用件，后两个子任务直接消费，不各写一套。
- R2 375px 下 `/billing` 不以宽表为主要浏览方式（父 PRD §3.0.1，阻断条件）。
- R3 卡片摘要含：方向、付款人／描述、金额、到账日期、匹配状态（父 PRD §5.2）。
- R4 卡片内可完成匹配、选租金月、归类／拆分、忽略、恢复、撤销（父 PRD §5.2）。
- R5 金额完整显示，不被截断成看似合法但不准确的数字（父 PRD §5.2）。
- R6 动作按状态分支，默认只留一个主操作（父 `design.md` §D4 的状态表）。
- R7 筛选分层：`payer`、`period`、`match_status` 为首要；其余进 `<details>` 更多筛选（父 `design.md` §D6）。
- R8 默认筛选变更 + `match_status=all` 可达 + 空状态出口 + 状态徽章链接保留筛选（父 PRD §14.1、父 `design.md` §D7）。**单独一个提交**。
- R9 桌面端在 `match_status=all` 下与基线逐页一致。

## Acceptance Criteria

- [ ] 375px 下卡片流可用，页面级 `scrollWidth === clientWidth`。
- [ ] 320/360/375/390/412px 全覆盖，无重叠、裁切、逐字竖排、屏外不可达元素。
- [ ] 卡片内主要操作命中区 ≥44×44px。
- [ ] R4 的六个动作在卡片内逐一手工走通。
- [ ] 默认只看待处理；`match_status=all` 结果与变更前一致；空列表有出口；状态徽章链接保留当前筛选。
- [ ] 桌面端在 `match_status=all` 下与基线一致（截图 + DOM 指标）。
- [ ] 现有 Go 测试若因结构替换而失败，逐个书面说明「为什么改、改成了什么」，不能默默删除。
- [ ] 新增 Go 回归断言：卡片容器存在且默认隐藏、窄屏块内激活、卡片消费 view model。
- [ ] 浏览器验收报告落 `research/`，记录每个视口、每个状态与失败元素。

## Out of Scope

- 不改桌面表格结构、列定义、排序入口、分页行为。
- 不改路由、POST action、查询参数名（唯一新增的是 `match_status=all` 这个取值）。
- 不改业务规则。
- 不给 `/tenants`、`/expenses` 加筛选——那两个页面今天没有筛选，PRD 也没要求（父 `design.md` §D6）。
- 不做 `/rent-dashboard`、`/tenants`、`/expenses` 的卡片（后续子任务）。
