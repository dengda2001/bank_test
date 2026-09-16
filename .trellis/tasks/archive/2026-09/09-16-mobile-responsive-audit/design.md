# 移动端适配整改 — 技术设计

## 目标与硬约束

消除审计报告（`research/mobile-audit.md`）中的 P0 与 P1 问题，使 RentOps 在手机上可完成日常收租操作。

**硬约束（来自 PRD）**：

1. **桌面端呈现不得改变**。所有新规则必须落在 `@media (max-width: 640px)` 内，或写成在宽屏下不产生视觉差异的声明。
2. **不重构信息架构**。不引入卡片流、不改路由、不改数据结构语义。
3. **验收必须基于真实浏览器**，不接受静态推断。

第 1 条是本设计的核心约束：它同时排除了「把预览页配色并入全站 token」这类看似顺手的改动——那会改动桌面端外观。

## 决策记录

| # | 决策 | 选择 | 理由 |
| --- | --- | --- | --- |
| D1 | 修复范围 | P0 + P1 | 全部为 CSS/模板层改动，不动信息架构 |
| D2 | 关键列策略 | 冻结**最右列** | 首列在 scroll=0 时本就在屏内，钉最右列即可同时看到「是谁」与「能做什么」 |
| D3 | `/billing` 例外 | 窄屏把该列换成约 88px 的「状态 + 处理」列，点击就地展开 | 该列实测 320px，钉不住；复用该列已有的 `<details>` 折叠范式 |
| D4 | 侧边栏 | 顶部紧凑栏 56px + 汉堡抽屉 | 首屏还给内容 309px；仅 4 个导航项、切换频率低 |
| D5 | 断点 | 不重指旧断点，新增规则统一挂 `≤640px` | 零桌面回归风险 |
| D6 | 外壳改造 | **先抽公共外壳再改** | 消除 7 份副本的漂移（现存 bug 的成因） |

## 架构：外壳抽取

### 现状

`<aside class="sidebar">` 有 **8 份副本、8 种不同内容**，分布在 7 个模板中（`main.go` 3 份、`cash_receipt_handlers.go` 2 份，`dashboard.go` / `tenant_detail.go` / `billing_page.go` 各 1 份）。

其中 `legacyBillingTemplate`（`main.go:2658`）是**死代码**——全仓 `grep` 只有声明行，无任何渲染点，**不在本次改动范围内**。因此实际是 **7 份活副本**。

副本已经漂移：

- `.nav-count` 计数徽标：`cash_receipt_handlers.go` 的两份**没有**
- `.side-foot` 文案三家不同（「现金租金补录」/「租客资料：X」/「支出资料：X」）
- 缩进风格三套

### 方案

新增 `cmd/truelayer-demo/workspace_shell.go`：

```go
// 所有页面共用的外壳字段。各页 data struct 以嵌入方式获得这些字段，
// 因此模板中既有的 {{.Username}} 等访问方式不变。
type workspaceShell struct {
    ActivePage    string
    Username      string
    Environment   string
    FootNote      string
    ShowNavCounts bool // 现金补录页当前不显示计数，抽取后必须保持原样
    TenantCount   int
    IncomeCount   int
    ExpenseCount  int
}

var workspaceBase = template.Must(template.New("workspace").Parse(
    `{{define "workspace-nav"}} …紧凑栏 + <aside class="sidebar">… {{end}}`))

// 每个页面模板都从 base Clone 出来，从而共享 "workspace-nav" 定义。
func newWorkspacePageTemplate(name, body string) *template.Template {
    return template.Must(template.Must(workspaceBase.Clone()).New(name).Parse(body))
}
```

要点：

- **嵌入而非继承**：`workspaceShell` 嵌入各页 data struct 后字段被提升，模板里既有的 `{{.Username}}` / `{{.ActivePage}}` 写法无需修改，改动面被压到最小。
- **`ShowNavCounts` 是必需的**：现金补录/作废/租客详情三页当前不渲染计数徽标。抽取后若无条件渲染，会改变这三页的桌面端外观，违反硬约束 1。
- 7 个模板各自把 `<aside>…</aside>` 整块替换为 `{{template "workspace-nav" .}}`。
- `topbar` **不抽取**——已逐一核对，7 份的标题与操作按钮各不相同，是页面自有内容。

### 数据结构的补充

| struct | 现状 | 需要补 |
| --- | --- | --- |
| `billingPageData` / `tenantPageData` / `expensePageData` / `rentDashboardPageData` | 已有全部 6 个字段 | 仅改为嵌入 |
| `cashReceiptFormData` / `cashReceiptVoidPageData` / `tenantDetailPageData` | 只有 `Username`、`Environment` | 补 `ActivePage`、`FootNote`；`ShowNavCounts` 置 `false` |

各页 handler 需在构造 data 时填 `ActivePage`（值已存在于 `dashboard.go:36`、`main.go:741/939/1077` 等处的既有约定中，直接沿用）。

## 抽屉实现

纯 CSS 复选框方案，不引入新 JS 依赖：

```html
<input type="checkbox" id="nav-drawer" class="nav-drawer-input" hidden>
<label for="nav-drawer" class="nav-compact-bar" aria-label="展开导航">
  <span class="mark">R</span><span class="brand-title">RentOps</span><span class="nav-burger">☰</span>
</label>
```

CSS 写在 `workspacePageCSS`（共用一个位置，7 页同时生效）：

- `> 640px`：`.nav-compact-bar { display: none }`，`.sidebar` 维持现状 → **桌面端零变化**
- `≤ 640px`：`.nav-compact-bar { display: flex; height: 56px }`；`.sidebar` 移出文档流（`position: fixed`）作为抽屉，默认 `transform: translateX(-100%)`，`#nav-drawer:checked ~ .app .sidebar` 时滑入

选择纯 CSS 而非 JS 的理由：与 `calendar.go` 那类有状态组件不同，抽屉没有状态需要同步；复选框方案在 JS 失败时仍可用，且可被 Go 模板测试直接断言。

## 冻结最右列

```css
@media (max-width: 640px) {
  .table-wrap th:last-child, .table-wrap td:last-child {
    position: sticky; right: 0;
    background: var(--surface);
    box-shadow: -8px 0 8px -8px rgba(0, 0, 0, .18);
  }
}
```

- `> 640px` 时整条规则不生效 → 桌面端零变化。
- 需要 `background`，否则横滑时下层单元格会透过。
- 阴影是**必需的可见提示**：移动端不渲染滚动条，没有它用户不知道还有内容。
- 展开行（`/tenants` 的 `.tenant-history-table`）是嵌套表格，其 `td:last-child` 也会被命中。需确认不产生双重 sticky；若不期望，用 `> tbody > tr > td:last-child` 限定直接子级。

### `/billing` 例外（D3）

该页最右列「用途／处理」实测 320px，不可冻结。方案：

- `≤ 640px` 时该列内容包进 `<details class="txn-action">`，`<summary>` 渲染为「状态徽章 + 处理」，列宽收到约 88px
- `> 640px` 时 `<summary>` 隐藏、内容常显（`summary { display: none }` + 强制内容可见）
- `<details>` 的展开态无法用 CSS 覆盖（关闭态内容由 UA 隐藏），因此需要约 3 行 JS 在 `≤640px` 时移除 `open` 属性。这与 `billing_page.go:163` 既有的内联脚本范式一致
- 同时 `≤640px` 隐藏 `账户`、`描述／参考号` 两列（实测 111px + 107px，移动端为次要信息）

**待浏览器验证**：冻结组（金额 + 处理）与可滚动区的最终宽度分配需在 375px 下实测确认金额列不再被截断。审计工装已具备复测能力，不接受纸面推算。

## 其余修复

| 问题 | 方案 |
| --- | --- |
| P0-3 金额被截断 | 金额单元格 `white-space: nowrap`；配合 `/billing` 列收起后重新分配宽度 |
| P0-4 payer-preview 卡片滚动条 | 加 `<div class="tp-scroll">` 包住 `<table>`，把 `overflow-x:auto` 从 `.card` 移到该 div（仍在 `≤760px` 媒体查询内）→ 标题与返回链接不再随表滚动。**该页配色体系不动**（改动会波及桌面端，违反硬约束 1），另立后续项 |
| P1-1 触控目标 | 逐个提升到 ≥44px：租客姓名/状态链接加 `padding` + `min-height`（注意 `th` 的 13px padding 不响应点击，须把 padding 给 `<a>` 本身）、billing 归类控件与 select 提到 44px、详情/编辑按钮提到 44px |
| P1-2 11px 字号 | `.mono` 与 `th` 在 `≤640px` 下提到 12px；全局 `11px` 保留给真正的次要元信息 |
| P1-3 逐字断行 | 标签列改按内容自适应（见下）；日期单元格 `white-space: nowrap`；`.profile-list` 在 `≤640px` 改单列 |
| P1-4 排版破损 | 分页按钮 `white-space: nowrap`；催缴面板「收起」按钮 `white-space: nowrap` + `flex-shrink: 0` |
| `.facts` 内边距缺陷 | `cash_receipt_handlers.go:212` 补 `padding: 18px 20px`，沿用 `.panel.surface` 正文块的既有约定 |

### 档案信息标签列（用户最初报告的问题）

根因已 DOM 实测确认：

```
grid-template-columns: 130px 139px   卡片内宽 287px，gap 18px
「邮箱」「房间」「月租」「租期」文字实宽均只有 32px，各浪费 98px
```

方案：`≤640px` 下 `.profile-list` 改单列（`grid-template-columns: 1fr`，标签在值上方），标签恢复 `--foreground-muted` 小字号。单列后值列获得全部 287px（原 139px，**提升 106%**）。宽屏下 `130px 1fr` 保持不变。

## 兼容性与回归

**桌面端冻结的验证方式**：审计工装已有 768×1024 视口，需在改动前后各跑一次，逐页比对 `report.json` 的 `offenders` 与 `docScrollWidth`；同时补一张宽屏（≥1280px）截图做人工比对。

**新增回归测试**（沿用 `billing_layout_test.go` / `dashboard_layout_test.go` / `tenant_detail_layout_test.go` 的写法，断言模板字符串）：

1. 7 个页面各自渲染且只渲染一次 `workspace-nav`
2. `workspacePageCSS` 含紧凑栏与抽屉规则
3. `≤640px` 块内存在 sticky 最右列规则
4. `.profile-list` 的移动端单列规则存在，且宽屏规则未被改动
5. `ShowNavCounts` 为 false 的三页不渲染 `.nav-count`
6. `transaction_previews.go` 的 `.card` 不再带 `overflow-x: auto`

## 风险与回滚

| 风险 | 缓解 |
| --- | --- |
| 抽取外壳触及全部 7 个渲染点，回归面大 | 分两步落地：先纯抽取（行为不变）+ 跑全量测试，再叠加抽屉。**抽取与功能改动分开提交**，前者可独立验证 |
| 冻结列在展开行产生双重 sticky | 用直接子级选择器限定，并在浏览器中展开行复测 |
| `/billing` 列宽重分配后金额仍被截断 | 以 375px 实测为准；若金额仍截断，回退到「不隐藏 `描述／参考号`」并重新分配 |
| 抽屉遮挡内容 | 抽屉展开时加半透明遮罩；关闭态 `transform` 不影响布局 |

回滚：本次改动集中在模板字符串与 CSS 常量，无数据迁移、无 schema 变更。回滚即 `git revert`，无需数据修复。

## 不在范围内

- `legacyBillingTemplate` 死代码（可另立清理项）
- `payer-preview` / `revoke-preview` 两页与全站不一致的 `:root` 配色体系（改动会波及桌面端）
- 断点数量收敛（D5 已决定不重指旧断点）
- 宽表格改卡片流（D2 已排除）
