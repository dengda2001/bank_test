# 移动端适配整改 — 执行计划

## 提交策略（用户明确要求）

**本任务最终只产生 1 个提交**，便于出问题时单次回滚。

- 开发过程中可以用本地临时提交推进，但推送前**必须 squash 成 1 个提交**。
- 下方各阶段的「回滚点」是**本地检查点**，不是独立提交——它们只用于开发中定位问题。
- 现有未提交改动属于**另一个任务**，必须先作为独立提交落在前面（见启动前门禁），不与本任务的提交混合。

## 启动前门禁（必须先行）

**工作区当前是脏的，且脏的正是本任务要动的同一批文件。**

```
 M cmd/truelayer-demo/main.go                  (外壳副本 ×3)
 M cmd/truelayer-demo/billing_page.go          (外壳副本 ×1)
 M cmd/truelayer-demo/dashboard.go             (外壳副本 ×1)
 M cmd/truelayer-demo/tenant_detail.go         (外壳副本 ×1)
 M cmd/truelayer-demo/cash_receipt_handlers.go (外壳副本 ×2)
 M cmd/truelayer-demo/dashboard_filters.go
 M cmd/truelayer-demo/{cash_receipts,dashboard,tenant_profile}_mysql_test.go
 M cmd/truelayer-demo/main_test.go
 M cmd/truelayer-demo/{transaction_actions,transactions}.go
?? cmd/truelayer-demo/{sort_links.go,billing_layout_test.go,dashboard_layout_test.go,tenant_detail_layout_test.go}
--- 12 files changed, 383 insertions(+), 135 deletions(-)
```

未提交改动共 383 增 / 135 删，覆盖全部 5 个含外壳副本的文件。**在脏基线上做重构会让「回归是本次引入的还是原本就有的」无法判断**，而本任务的核心恰恰是一次跨 7 个渲染点的重构。

因此：**先把这批未提交改动提交（或 stash）并跑通一次全量测试，再开始阶段 1**。这一步不完成不要进入实现。

另有两点需一并处理：

1. **审计工装目前在 `/tmp`，重启即失**。PRD 的验收要求「整改后重跑审计」，而 `/tmp/mobile-audit/` 是临时目录。阶段 0 需把 `audit.mjs`、`audit-extra.mjs`、`verify-claims.mjs`、`tiles.mjs`、`metrics.mjs` 复制到 `research/harness/` 并附一份 README（如何设置 `AUDIT_USER`/`AUDIT_PASS`/`AUDIT_BASE`、如何拉起本地实例）。截图体积大（约 14MB），不入库。
2. **基线必须留证**。阶段 0 要跑一次基线审计并把 `report.json` / `metrics.json` 存为 `research/baseline-*.json`，供改动后逐页比对——这是验证「桌面端未变」的唯一手段。

## 阶段 0：基线固定

- [ ] 提交或 stash 现有未提交改动，`git status` 干净
- [ ] `go build ./... && go test ./...` 全绿，记录结果
- [ ] 复制审计工装到 `research/harness/`，写 README
- [ ] 跑基线审计（含 768×1024 与一张 ≥1280px 宽屏截图），存 `research/baseline-report.json`、`research/baseline-metrics.json`

**回滚点 A**：`git tag mobile-audit-baseline`

## 阶段 1：外壳抽取（纯重构，行为不变）

目标：7 份 `<aside>` 副本收敛为 1 处，**不改变任何渲染输出**。

- [ ] 新建 `cmd/truelayer-demo/workspace_shell.go`：`workspaceShell` 结构体、`workspaceBase` 模板（`{{define "workspace-nav"}}` 内含原 `.sidebar` 内容）、`newWorkspacePageTemplate` 辅助函数
- [ ] 7 个页面 data struct 改为嵌入 `workspaceShell`；`cashReceiptFormData` / `cashReceiptVoidPageData` / `tenantDetailPageData` 补 `ActivePage`、`FootNote`，`ShowNavCounts` 置 `false`
- [ ] 4 个已有全部字段的 struct（`billingPageData` / `tenantPageData` / `expensePageData` / `rentDashboardPageData`）改为嵌入，`ShowNavCounts` 置 `true`
- [ ] 各页 handler 填 `ActivePage`（沿用 `dashboard.go:36`、`main.go:741/939/1077` 既有取值）
- [ ] 7 个模板把 `<aside>…</aside>` 整块换成 `{{template "workspace-nav" .}}`
- [ ] `legacyBillingTemplate` **不动**（死代码，全仓无渲染点）

**验证**：`go test ./...` 全绿；额外断言「原样渲染」——改动前后各渲染一次 7 个页面，除 `<aside>` 的缩进差异外应逐字节一致。若无法做到逐字节一致，说明抽取改变了输出，需修正后再继续。

**回滚点 B**（本地检查点）：抽取完成、测试全绿后记一个本地检查点，便于后续阶段出问题时快速定位是「抽取引入的」还是「新功能引入的」。最终会 squash 掉。

## 阶段 2：抽屉

- [ ] `workspacePageCSS` 增加紧凑栏与抽屉规则，全部落在 `@media (max-width: 640px)` 内（`>640px` 仅 `display:none`）
- [ ] `{{define "workspace-nav"}}` 内加隐藏复选框 + `<label class="nav-compact-bar">`
- [ ] 补回归测试：紧凑栏规则存在、`≤640` 块内、桌面端不显示

**验证**：375px 下首屏内容起点应从 y≈372 上移到 y≈120 以内；桌面端截图与基线逐像素比对无差异。

**回滚点 C**

## 阶段 3：冻结最右列

- [ ] `workspacePageCSS` 的 `≤640px` 块内加 sticky 规则（`background` + 内侧阴影为必需项，不是装饰）
- [ ] 若展开行产生双重 sticky，改用直接子级选择器 `> tbody > tr > td:last-child` 限定
- [ ] 补回归测试

**验证**：375px 下 `/tenants` 默认视口应同时看到租客姓名与「详情/编辑」——这正是审计中「不存在任何滚动位置能同时看到两者」的判据，用 `verify-claims.mjs` 的 `nameVisibleAtMax` / `actionsVisibleAtZero` 复测。

**回滚点 D**

## 阶段 4：`/billing` 例外

- [ ] 最右列包进 `<details class="txn-action">`，`<summary>` 显示状态徽章 + 「处理」
- [ ] `≤640px` 时隐藏 `账户`、`描述／参考号` 两列；`金额／余额` 单元格 `white-space: nowrap`
- [ ] 约 3 行 JS 在 `≤640px` 时移除 `open` 属性（`<details>` 关闭态无法用 CSS 覆盖）
- [ ] `>640px`：`summary` 隐藏、内容常显，与基线一致

**验证**：**以 375px 实测为准**。判据是 `verify-clipping.mjs` 的 `sliced` 字段——`金额／余额` 列必须 `sliced: false` 且 `visibleW > 0`。若列宽重分配后金额仍被截断，回退到「不隐藏 `描述／参考号`」重新分配。

**回滚点 E**

## 阶段 5：排版与可读性（P1）

按 `design.md` 的对照表逐项落地。注意两个易错点：

- [ ] 触控目标：`th` 的 13px padding **不响应点击**，必须把 padding 给 `<a>` 本身，而不是给 `th`
- [ ] `.profile-list` 单列规则只写在 `≤640px`，宽屏的 `130px 1fr` 不得改动
- [ ] `.facts` 补 padding（`cash_receipt_handlers.go:212`），沿用 `.panel.surface` 正文块的既有约定
- [ ] `payer-preview` 的 `.card` 去掉 `overflow-x: auto`，改为包住 `<table>` 的 `.tp-scroll`；**该页配色不动**
- [ ] 补回归测试（`design.md` 列的 6 条）

**回滚点 F**

## 阶段 6：验证与收尾

- [ ] `go test ./...` 全绿
- [ ] 重跑全部三个审计脚本，与基线逐页比对：**新增溢出必须为 0**、768×1024 与宽屏结果与基线一致
- [ ] 逐张复核修复后的截图（10 个页面 × 375px）
- [ ] 更新 `research/mobile-audit.md`，标注每条发现的状态
- [ ] 部署到 https://bank.ddpl.top 并在真实手机上抽查（审计用的是无头 Chromium，真机是最终判据）

## 高风险文件

| 文件 | 风险 | 说明 |
| --- | --- | --- |
| `main.go` | **最高** | 3 份外壳副本 + `workspacePageCSS` 常量（7 页共用）。CSS 改动的影响面是全部页面 |
| `dashboard.go` | 高 | 外壳副本 + 214 行未提交改动 |
| `billing_page.go` | 高 | 外壳副本 + 阶段 4 的模板与 JS 改动 |
| `cash_receipt_handlers.go` | 中 | 2 份外壳副本 + `.facts` 缺陷 |
| `tenant_detail.go` | 中 | 外壳副本 + `.profile-list`（用户最初报告的问题） |
| `transaction_previews.go` | 中 | 两个独立页面，绕开 `workspacePageCSS`，改动不能想当然套用全站规则 |
| `main_test.go` | 中 | 已有对 `workspacePageCSS` 的字符串断言（`:123/137/142/155`），CSS 改动可能触发现有断言失败——这是**预期内的信号**，需判断是断言过时还是真的改坏了 |

## 完成判据

- 10 个页面在 375px 下 `horizontalOverflow == 0`（基线已满足，不得回退）
- 375px 下 `/tenants` 同时可见租客姓名与操作按钮
- 375px 下 `/billing` 金额列 `sliced == false`
- 375px 下所有页面首屏内容起点 ≤ y=120
- 全部交互元素 ≥44×44px，或逐个书面说明取舍
- 768×1024 与 ≥1280px 的审计结果与基线一致（桌面端未变）
- `go test ./...` 全绿，且新增回归测试覆盖 `design.md` 列的 6 条断言
