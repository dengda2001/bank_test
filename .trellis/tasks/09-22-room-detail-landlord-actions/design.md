# 技术设计：房间详情页房东视角改造

## 边界

改的是 `/rooms/{id}` 这一个页面的展示层，加上它自己的视图数据派生。**不碰**账务事实、生成逻辑、查询口径、路由语义与状态机。

具体不做的事：
- 不给 `LoadRoomDetail` 加新查询（`OverdueDays` 只消费已加载的 `Summary.DueDate`）。
- 不改 `rentStatusLabel` / `obligationStatus` 的状态口径。
- 不改 `/rooms/{id}` 的 POST 处理（`action=delete` / `action=save` 的解析与校验保持在 `handleRoomDetail`）。

## 契约变化

### 1) 视图数据：`rentRoomDetailPageData` 增 `OverdueDays int`

`loadRoomDetail`（`rent_workspace_page.go:316`）里，从 `Summary.DueDate` 与当前时间推出整数天，赋给新字段。模板只在 `Summary.Status == "overdue"` 时渲染。

为什么放在视图数据而不是模板函数：仓库里没有可注入的时钟（无 `timeNow` / `nowFunc`），而 `detail_pages_alignment_test.go`、`entity_drawers_test.go`、`desktop_layout_test.go` 都是直接构造 `rentRoomDetailPageData` 执行模板的。做成字段，测试可以确定性地给定 N；做成 `time.Now()` 模板函数则不可测。

派生规则：`OverdueDays = floor(now - DueDate)` 的天数，未逾期或日期不可解析时为 0。注意 `Summary.DueDate` 是字符串（`2026-09-01`），解析失败一律退化为 0，不 panic。

### 2) 第三个 settle 适配器：`RoomDetailSettleForm`

`collection_settle_form.go` 的注释已经把这件事写明了——partial 只读 `collectionSettleFormView`，页面用自己的行类型构造它，"so a page builds that view from its own row type and does not have to inherit /bills' page context"。现有两个适配器：`rentDashboardPageData.SettleForm`、`rentWorkspacePageData.TenantSettleForm`。本次加第三个：

```go
func (data rentRoomDetailPageData) SettleForm(row <房间详情的租客行类型>) collectionSettleFormView
```

- `DutyLabel`：`{租客名} · {月份} 租金责任`
- `OutstandingAmount`：`row.BalanceAmount`，空则退回 `row.ExpectedAmount`
- `EffectiveDate`：当天
- `Dispositions`：`collectionSettleDispositions(settleDispositionMatchPayment)`
- `ReturnTo`：本房间详情页 URL，带 `period` 与来源上下文

模板侧只加一行：`{{if gt .BalanceCents 0}}{{template "collection-settle-form" ($.SettleForm .)}}{{end}}`——与 `rent-workspace.html:105/148/159` 的写法完全一致。

**返回地址**：表单 POST 到 `/rent-dashboard/settle`，隐藏字段 `return_to` 已在 partial 里。`handleDashboardManualBalance` → `manualBalanceRedirectURL(values, requestPath, message, actionError)` 需要接受房间详情页作为返回目标。这里是本次唯一有真实风险的点：必须确认 `return_to` 的白名单/校验允许 `/rooms/...`，且不允许外站。实现时先读 `manualBalanceRedirectURL` 再决定是否需要放行。

### 3) 删除表单搬家

`room-detail.html:31`（页头）与 `:137`（移动底栏）的两份删除表单，合并成一份，移入 `{{if .Editing}}` 的编辑抽屉底部。表单原样保留：

```html
<form method="post" action="/rooms/{{.RoomID}}" class="delete-object-form">
  <input type="hidden" name="action" value="delete">
  ...原有 FromList 上下文字段不变...
  <button class="btn danger" type="submit" data-confirm="true">删除房间</button>
</form>
```

**新建抽屉为什么天然没有危险区**：编辑抽屉是内联在 `room-detail.html` 里的，新建抽屉是独立 partial `partials/room-create-drawer.html`。两者本来就是两个模板，所以危险区只写进前者即可，不需要引入 `mode` 参数或共用组件。这比"同一组件两种形态"更省事，也不改新建路径。

### 4) 确认文案

`workspace-nav.html:134` 目前是写死的：

```js
if (trigger && !window.confirm("确认执行此高风险操作吗？")) event.preventDefault();
```

改为优先读 `data-confirm-message`，缺失时回退到现有文案：

```js
const message = trigger.getAttribute("data-confirm-message") || "确认执行此高风险操作吗？";
```

删除按钮带上"删除房间 {房产} · {房间}？会同时删除收租安排与历史责任记录，且无法撤销。"。回退分支保证 `/properties` 与「确认平账」的现有行为一字不变。

### 5) 空态判据

「关联收款」现在是 `{{if .Tenants}}` —— 有租客就渲染空表头。"代付分配详情"完全无条件渲染指标框。

两者的"空"都应以 `.PaymentCount` 为判据。`.PaymentCount` 已存在于视图数据（`rent_workspace_page.go` 里 `PaymentCount: len(paymentIDs)`），无需新增字段。

折叠用原生 `<details>`。摘要行内的按钮要 `event.stopPropagation()`（或在按钮上 `onclick="event.stopPropagation()"`），否则点"登记一笔收款"会连带展开。

### 6) 砍块后的信息归位

`room-plan-summary` 整段移除。三个字段的去向：

| 原字段 | 去向 |
| --- | --- |
| 房间月租 | 「房间信息」已有"当前月租"，不动 |
| 每月缴租日 | 「房间信息」桌面 facts 新增（移动 facts 已有"缴租日"） |
| 计划区间 | 「房间信息」新增 |
| 调整安排按钮 | 不新增——页头「入住与租金」就是该编辑器的入口 |
| 入住租客与责任 | 删除，「租客责任」表已表达 |

随之删掉的 CSS：`.room-plan-summary*` 系列（含 `:107-109` 的移动规则）。若 `room-detail.css` 之外还有引用，一并确认。

## 取舍

**为什么补 `metric-*` 全局定义而不是本页私有类。** 这三个类名已经在 5 处模板里用了，CSS 里一条定义都没有——补全它们是把死代码接上，而不是新增风格。代价是 `property-detail.html` 与 `rent-workspace.html` 的 KPI 卡会同时获得颜色。PRD 里已把这列为待拍板项：若要零外溢，就退化成房间详情页的局部类名，同一套视觉写两遍。

**为什么逾期天数不做成前端 JS。** 服务端渲染在无 JS 时依然正确，且测试可直接断言；JS 方案还要把日期塞进 data 属性，收益为零。

**为什么不用 `<details>` 承载删除表单的"先展开再删除"。** 多一层点击不增加安全性，真正的护栏是明确的后果文案 + 浏览器 confirm。二次输入房间名属于后续可选项。

## 兼容性

- 删除的 POST 契约、幂等性与错误码不变。
- `/rent-dashboard/settle` 的既有载体（`/bills`、工作台）行为不变；新载体只增加一个合法 `return_to`。
- 移动端：`.room-detail-mobile-actions` 去掉删除后，`grid-template-columns:repeat(2,...)` 与 `.delete-object-form{display:contents}` 规则需要相应调整；页头 `.actions` 在移动端本来就是 `display:none`。

## 回滚

改动集中在 4 个模板、2–3 个 CSS、1 个 Go 文件（视图字段 + 适配器）与 1 处 JS 片段。无数据迁移、无路由变更，`git revert` 即可完整回滚。

## 已知的既有问题（本次不修，但要说明）

- `collection-settle-form` 的四个处理方式只实现了"匹配现有收款"，其余三种会回 `manual_balance_disposition_unimplemented`。房东在平账面板里仍可能选到未实现的项。
- 测试基线本身是红的（不设 DSN 时 dunning/mysql 一组静默 SKIP；另有若干条非库测试稳定红）。验收时必须区分"本次引入的失败"与"既有失败"，不能把既有红算到本次头上。
