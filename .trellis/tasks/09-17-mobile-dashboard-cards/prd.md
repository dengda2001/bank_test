# PRD：`/rent-dashboard` 移动卡片列表

父任务：`09-16-mobile-friendly-workspace`（移动端友好版工作台）

## Goal

把月度总览改造成房东在手机上真正用得上的页面：打开就看到本月收款概况，一眼定位未缴与需处理的租客，并就地展开缴费明细或进入平账／催缴。

这个页面回答的是「这个月钱收齐了没有」，是房东最高频的一次查看。

## 依赖与启动前门禁

- **前置**：`09-17-mobile-audit-fixtures` 已归档，且 `09-17-mobile-billing-cards` 已归档——本任务消费后者建立的卡片基元，不另起一套。
- **共享契约**：父任务 `design.md` §4 的七条子任务契约。

## Confirmed Facts

研究见父任务 `research/list-pages-render-map.md`。

- 模板是 `{{range .Rows}}` + 字面量 `<tr>`，**直接读 domain struct `rentDashboardRow`**（`main.go:288`），没有行级 view model。显示字符串在 Go 侧预格式化（`obligations.go:402-420`），但状态标签等仍在模板里。
- 筛选基础设施完整且已独立成文件：`dashboard_filters.go` 拥有解析、校验、过滤、排序、分页与 URL 组装（`rentDashboardFiltersFromQuery`、`filterAndSortRentDashboardRows`、`paginateRentDashboardRows`、`rentDashboardURL`）。
- 筛选与排序是 **Go 侧内存过滤**，不在 SQL 里；分页在过滤之后。改默认值不涉及索引或查询性能。
- 默认是 `Status: "all"`、`Sort: "status"`、`Page: 1`（`dashboard_filters.go:28-30`）。
- `status=unpaid` 已经等于 `open | overdue | partial`（`dashboard_filters.go:85-87`），且排序已有优先级 `overdue→needs_review→partial→open→paid`（`:113`）。所以默认值改成「未缴优先」是改一个字符串。
- `status=all` 已在 `validateRentDashboardFilters` 白名单里（`dashboard_filters.go:66`）——「查看全部」天然可达。
- 已有的展开机制：`data-details-target`（`dashboard.go:405`）、平账表单、催缴抽屉（`dashboard.go:391`、`429`，JS 在 `461-484`）。
- 页面局部窄屏块在 `dashboard.go:290-337`。
- `sort_links.go` 提供 `tableSortLink` / `sortLinkFor` / `normalisedSort`，**与 `/billing` 共用**（`main.go:756-758`）。

## Requirements

- R1 **先补 `rentDashboardRow` 的行级 view model**（父 `design.md` §D2）：金额、日期、状态标签与文案、动作可见性开关全部预计算，桌面行与手机卡片都只消费它，不在模板里做格式化。
- R2 手机端：汇总卡片 + 租客卡片列表；汇总指标（应收、已收、未收、待处理）优先展示。
- R3 卡片首屏含姓名、房间／地址摘要、应收、已收、未收、状态（父 PRD §5.1）。
- R4 卡片默认折叠付款明细，展开后就地显示付款记录与操作。
- R5「查看租客详情」与「展开缴费明细」是两个可区分的操作（父 PRD §5.1）。
- R6 平账、催缴入口在卡片内可见，不需要横向滚动。
- R7 月份切换用「上一月 / 月份 / 下一月」三段布局，不被挤压或折成竖排文字。
- R8 默认 `status=unpaid`（父 `design.md` §D7）；`status=all` 保持可达且结果与变更前一致。
- R9 排序、月份切换、状态筛选控件在窄屏不折行、不溢出。
- R10 卡片基元复用 `/billing` 建立的那套，不新增第二套卡片样式。

## Acceptance Criteria

- [ ] 320/360/375/390/412px 下页面级 `scrollWidth === clientWidth`，无宽表作为主要浏览方式。
- [ ] 默认打开即见当前月份的未缴／待处理租客，无需先操作筛选（父 PRD §10.1）。
- [ ] R5 的两个操作在 375px 下可区分、可点、命中区 ≥44×44px。
- [ ] 卡片内可完成平账与催缴入口的进入（父 PRD §10.2）。
- [ ] `status=all` 结果与变更前一致；空列表有出口。
- [ ] 桌面端在 `status=all` 下与基线一致（截图 + DOM 指标）。
- [ ] 新增 Go 回归断言：卡片容器默认隐藏、窄屏块内激活、卡片消费 view model。
- [ ] 现有 dashboard 排序断言若因结构替换失败，逐个说明改动理由。
- [ ] 浏览器验收报告落 `research/`。

## Out of Scope

- 不改桌面表格与汇总布局。
- 不给 dashboard 新增筛选条件（现有条件已足够；PRD 只要求重排与分层）。
- 不改业务规则、不改催缴与平账的服务端行为。
- 不做 `/billing`、`/tenants`、`/expenses` 的卡片。
