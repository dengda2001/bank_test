# PRD：`/tenants`、`/expenses` 与录入页移动化

父任务：`09-16-mobile-friendly-workspace`（移动端友好版工作台）

## Goal

把资料与录入类页面在手机上做完：租客列表与详情、支出列表与录入、现金收款的录入／预览／作废，以及两个预览页。

这组页面是「录入一笔」和「查一个人」，与 `/billing`、`/rent-dashboard` 的「处理一批」不同——它们的移动端形态是短表单和资料卡，不是待办流。

## 依赖与启动前门禁

- **前置**：`09-17-mobile-audit-fixtures` 已归档；`09-17-mobile-billing-cards` 已归档（卡片基元来源）；`09-17-mobile-dashboard-cards` 已归档（view model 先行的做法已跑通一遍）。
- **共享契约**：父任务 `design.md` §4 的七条子任务契约。

## Confirmed Facts

研究见父任务 `research/list-pages-render-map.md` 与 `research/verification-infra.md`。

- `tenants.go` 与 `expenses.go` **不含页面代码**——`/tenants` 的 handler 与模板在 `main.go:896` / `main.go:2524`，`/expenses` 在 `main.go:1062` / `main.go:2697`。改「tenants.go / expenses.go」会完全打空。
- 两个页面都直接读 domain struct（`tenantRecord` `main.go:185`、`expenseRecord` `main.go:211`），**没有行级 view model**，字符串在 `prepareTenants`（`main.go:1918`）/ `prepareExpenses`（`main.go:1925`）里预格式化。
- 两个页面**没有筛选、没有排序、没有分页**，只有 `edit` / `add` / `message` / `error` 参数。
- 两个页面都传 `nil` FuncMap 给 `newWorkspacePageTemplate`——若卡片需要模板函数，得先补上。
- 现有窄屏块：`main.go:2557-2565`（tenants）、`main.go:2704-2720`（expenses），都是 grid-only 的小块。
- `/tenants` 的行展开用 `data-tenant-target` + 嵌套 `data-month-target`（`main.go:2637`、`2652`，JS 在 `2666-2692`）。
- `/expenses` 没有任何展开机制。
- `/billing/payer/preview` 的表格滚动已经在上一轮修好（`.tp-scroll` 包住 `<table>`，`TestPayerPreviewScrollsTheTableNotTheCard` 守着它）——本任务**不应**把这个测试改失败。
- 上一轮的审计结论：`/expenses` 类别列实测 52px 时「物业」被逐字断成两行；该问题已在上一轮修（借了 22px 给日期列）。

## Requirements

### `/tenants`

- R1 补 `tenantRecord` 的行级 view model（父 `design.md` §D2）。
- R2 手机端租客卡片列表：姓名、付款人、月租、房间摘要、状态（父 PRD §5.3）。
- R3 详情、编辑是稳定可见的操作，不依赖横向滚动。
- R4 地址、账单安排、创建时间等次要信息允许折叠，但不能逐字断裂或破坏卡片高度。
- R5 默认优先显示欠款或需要处理的租客（父 PRD §6.4）。**不新增筛选 UI**——该页今天没有筛选，PRD 也没要求。
- R6 添加／编辑表单在 320px 下仍为单列，提交按钮全宽或占据明确主操作位置。

### `/tenants/:id`

- R7 档案信息由两列标签布局改为手机单列信息组。
- R8 付款人关系卡片化，每条关系的移除按钮固定在条目内。
- R9 缴费历史手机端用账单卡片；平板及以上保留表格。
- R10 起止月份筛选在手机上单列，两个控件宽度一致。

### `/expenses`

- R11 补 `expenseRecord` 的行级 view model。
- R12 支出卡片列表：首屏含日期、描述、金额、类别；备注与支付方式为次级信息（父 PRD §5.4）。
- R13 录入表单按「描述、金额、日期、类别、支付方式、关联租客／房间、备注」排序；金额 `inputmode="decimal"`，日期用日期控件，币种明确标识只读。
- R14「保存支出」在窄屏下为清晰的主按钮，不被其他操作抢占视觉层级。

### 现金收款与预览页（父 PRD §6.7 / §6.8）

- R15 保留现有服务端提交与预览流程，只做布局与尺寸收尾。
- R16 现金补录表单打开后，首个可填写控件在首屏可见区域。
- R17 作废页先展示不可逆影响与收款事实，再展示原因输入与确认按钮。
- R18 预览页标题、说明、返回入口固定在卡片可见区域；只有内部宽内容允许横向滚动，禁止整张卡片横向滚动。
- R19 两个预览页的关键按钮与原因输入框达到移动端点击尺寸。

### 通用

- R20 320px 下不允许任何字段或按钮超出内容区（父 PRD §6.7）。
- R21 卡片基元复用 `/billing` 建立的那套。

## Acceptance Criteria

- [ ] 320/360/375/390/412px 下所有纳入页面 `scrollWidth === clientWidth`。
- [ ] `/expenses` 类别不再逐字断裂；长地址、长描述不破坏卡片高度（用 seed 里的长字段数据验）。
- [ ] `/tenants` 详情与编辑在 375px 下稳定可见且可点，命中区 ≥44×44px。
- [ ] `/tenants/:id` 缴费历史在手机是账单卡片、在 ≥768px 仍是表格。
- [ ] 现金补录首个可填控件在 375px 首屏可见。
- [ ] 作废页的信息顺序符合 R17。
- [ ] `TestPayerPreviewScrollsTheTableNotTheCard` 仍然通过（预览页结构不应被误伤）。
- [ ] 桌面端与基线一致（截图 + DOM 指标）。
- [ ] 新增 Go 回归断言：各页卡片容器默认隐藏、窄屏块内激活、卡片消费 view model。
- [ ] 浏览器验收报告落 `research/`。

## Out of Scope

- 不给 `/tenants`、`/expenses` 新增筛选、排序或分页。
- 不改录入表单的服务端字段名、校验规则或提交 action。
- 不改现金收款与预览页的业务流程。
- 不改 `/tenants/:id` 平铺及以上断点的表格。
