# 执行计划：状态控件去重与文案统一

## 前置

- 任务状态需为 `in_progress`（步骤 1.4 评审通过后 `task.py start`）。
- 工作目录：`/Users/dd/projects/bank`，直接在 `main` 上提交。

## 已核实的事实（避免执行时重新摸索）

- 快捷筛选表单**只在 `/transactions` 渲染**（`billing_page.go:292` 处在
  `{{if eq .PageKey "transactions"}}` 分支内）。
- 快捷筛选表单**在移动端本来就被隐藏**：`billing_page.go:213` 的
  `.transaction-route-quickfilter { display: none; }` 落在移动端媒体查询里。
  因此删掉它的状态下拉只影响桌面端，移动端行为完全不变。
- 唯一引用该下拉的测试是 `billing_layout_test.go:53-57`。
- 没有任何测试断言「已匹配/未匹配/部分匹配/已交满/未到期未缴」这些字串。

## 步骤

### 1. 流水页页签文案（`cmd/truelayer-demo/billing_page.go` 约 254 行）

把页签「已匹配」改为「已关联」。**只改文案，`href` 与
`{{if eq .TransactionScope "matched"}}` 判断保持原样。**

### 2. 删除快捷筛选里重复的状态下拉（`billing_page.go` 约 295 行）

删掉整行 `<select name="match_status" aria-label="流水状态" onchange="this.form.requestSubmit()">…</select>`。
**保留**同表单内的：`<input type="hidden" name="scope">`、
付款人搜索 `<input type="search" name="payer">`、
到账月份 `<input type="month" name="period">`、`<button>搜索</button>`。

### 3. 清理随之失效的 CSS（`billing_page.go` 约 96 行）

删除 `.transaction-route-quickfilter select[name="match_status"] { flex: 0 0 190px; width: 190px; }`。
同组其它规则（`input`/`input[type="search"]`/`input[type="month"]`/`.btn`）保留。
第 213 行的 `.transaction-route-quickfilter { display: none; }` 保留——表单本身还在。

### 4. 主页状态下拉文案对齐徽章（`cmd/truelayer-demo/web/templates/pages/rent-workspace.html` 第 50 行）

只改 3 个 option 的显示文案，**`value` 与 `selected` 判断一律不动**：

| value | 改前 | 改后 |
|---|---|---|
| `overdue` | 逾期 | 已逾期 |
| `paid` | 已交满 | 已缴清 |
| `open` | 未到期未缴 | 未缴 |

`all` / `unpaid` / `needs_review` / `partial` / `vacant` 的文案保持不变。

### 5. 更新失效断言（`cmd/truelayer-demo/billing_layout_test.go` 53-61 行）

现状断言「快捷筛选与筛选栏各有一个状态下拉，且都带
`onchange="this.form.requestSubmit()"`」。改写为：

- 保留：筛选栏（`<select id="match_status">`）存在且带 `onchange="this.form.requestSubmit()"`。
- 反转：快捷筛选表单内**不再**出现 `<select name="match_status"`。
  写法参照同文件 91-95 行的「已移除控件」断言风格，失败信息写清原因。

### 6. 补防回归断言

在 `billing_layout_test.go` 里增加两条（可并入现有测试或新开）：

- `/transactions` 渲染结果中 `name="match_status"` 只出现 1 次（含筛选栏那一份）。
- 渲染结果中不含「已匹配」「未匹配」「部分匹配」三个字串。

### 7. 验证

```bash
cd /Users/dd/projects/bank
go build ./... && go vet ./cmd/truelayer-demo/

# 定向
go test ./cmd/truelayer-demo/ -run 'Billing|BillingLayout|Transaction|Workspace|Dashboard|Prototype' -count=1

# 全包：退出码 1 是预期的（基线就红）。要看的是失败集合是否与基线逐条相同：
go test ./cmd/truelayer-demo/ -count=1 > /tmp/pkg.log 2>&1; echo "EXIT=$?"
grep '^--- FAIL' /tmp/pkg.log | sort > /tmp/pkg_fail.txt
# 期望恰好为基线那 9 条（见下节表格），多一条就是回归。
# 与基线清单机器比对（基线文件同法在 git stash 后的干净树上取一次）：
#   comm -13 /tmp/baseline_fail.txt /tmp/pkg_fail.txt   # 只应输出新增失败，应为空

# 渲染层人工核对：三个旧词应无输出
grep -n '已匹配\|未匹配\|部分匹配' cmd/truelayer-demo/billing_page.go
```

功能未丢的核对（应各命中一次）：

```bash
# 流水页筛选栏仍含全部 8 项
grep -c 'option value="ignored"' cmd/truelayer-demo/billing_page.go
# 主页 8 个 value 仍在
grep -o 'value="\(all\|unpaid\|needs_review\|overdue\|partial\|open\|paid\|vacant\)"' \
  cmd/truelayer-demo/web/templates/pages/rent-workspace.html | sort -u | wc -l
```

### 6b. 执行中追加的两处用词（超出初版 AC 字面范围）

1. `billing_page.go:305`：`for="match_status">匹配状态<` → `>关联状态<`。
   下拉自己的标签还在说「匹配」，而它 8 个选项已全部说「关联」。
2. `web/templates/pages/rent-workspace.html:25`：
   `流水已匹配，待处理队列已更新。` → `流水已关联，…`。
   主页把已退役的状态词直接说给用户。

**不动动词「匹配」**：`匹配流水`/`确认匹配`/`撤销匹配`/`匹配依据` 指的是动作。

各补一条断言，并做过反向对照（把词改回旧写法 → 两条断言各报错，见验证记录）。

### 7b. 布局影响核查（已做，静态）

删掉的下拉是 `.transaction-route-quickfilter` 的 flex 子项，随它的
`flex: 0 0 190px` 一并删除。`billing_page.go` 全文件**无** `nth-child`/
`last-child`/`first-child` 等结构选择器，同表单的搜索框是 `flex: 1 1 320px`
（唯一的可伸缩项），会自然吸收腾出的 190px。**结论：不破版。**
第 94 行 `.transaction-route-quickfilter input, … select { min-height: 36px }`
里的 `select` 分支现在对本表单是空转，属通用规则，未清理。

### 8. 走查（可选但推荐）—— **本轮未做**

需要一个带库的本地实例。本机 MySQL 在跑但当前用户无凭据，
且看不到可复用的遗留审计库；起实例需建库 + 灌种子数据，未在本轮内做。
**渲染层证据来自 Go 测试**（含反向对照），**未做人眼视觉确认**。
若要补，用 `scripts/audit/` 下的 harness，`AUDIT_BASE` 指向本地端口。
**不要**连生产库（`bank.ddpl.top`、`:8081` 等）。

## 评审卡点

| 卡点 | 位置 | 通过条件 |
|---|---|---|
| 方案评审 | 步骤 1.4 | dd 认可 prd/design/implement 后才 `task.py start` |
| 改动后自检 | 步骤 2.2 | 步骤 7 命令跑完，**失败集合与基线 9 条逐条相同**（无新增），且旧词 grep 无输出 |
| 完成前核验 | 步骤 3.1 | AC 逐条勾选；SKIP 数量与基线一致 |

## 回滚

改动仅落在 4 个文件（`billing_page.go`、`rent-workspace.html`、
`billing_layout_test.go`、`rent_workspace_test.go`），无 schema、无迁移、无缓存。
出问题 `git revert` 单次提交即可，无残留状态。

## 测试基线（勿把既有失败当回归）

**全包当前就是红的。** 2026-09-21 在 `git stash` 掉全部改动后的干净工作区实测
`EXIT=1`，稳定失败 **9 条**（`TestWorkspaceTemplatesIncludeSharedCalendarPicker`
另有 3 个子测试失败），**与本次改动无关，不在本任务范围内修**：

| 失败测试 | 位置 | 现象 |
|---|---|---|
| `TestWorkspaceNavExposesDesktopSections` | `desktop_layout_test.go:57` | nav 缺 `资金与系统` 分组标题 |
| `TestTenantCreateFormCanBindAnExistingRoom` | `desktop_layout_test.go:178` | 缺 `tenantRoomSelect` |
| `TestExpensePageUsesPrototypeListAndAddDrawer` | `expense_page_test.go:44` | 缺 `entity-drawer-backdrop`、`保存支出` |
| `TestTransactionRouteActionColumnSaysViewDetails` | `list_pages_alignment_test.go:317` | 表头缺 `状态 / 操作` |
| `TestWorkspaceTemplatesIncludeSharedCalendarPicker` | `main_test.go:105` | billing/tenants/expenses 三个子测试缺 `calendar-input` / `.calendar-popover` |
| `TestBillingTemplateShowsMonthChoiceForRememberedTenant` | `main_test.go:784` | 缺 `分别选择要匹配的租客和月份` |
| `TestNavCountsOnlyRenderWhereTheyDidBefore` | `mobile_layout_test.go:509` | 期望 4 个 nav-count，实际 3 个 |
| `TestParseTenantHistoryRangeDefaultsToTwelveMonths` | `tenant_profile_test.go:164` | 期望 size=12，实际 10 |
| `TestTenantHistoryRangePresetWinsOverExplicitMonths` | `tenant_profile_test.go:237` | 期望 size=12，实际 10 |

通过标准是**失败集合不扩大**，不是"全绿"。

> 这份表格的初版只有 3 条，是错的：当时用 `go test ... | tail -30` 取数，
> 输出文件里只剩最后 30 行，grep 出来的就是 `tail` 截断后的残骸。
> 基线必须 `git stash` 后全量取、不接管道。

另外：

- 不设 `RENTOPS_MYSQL_TEST_DSN` 时走库的 22 条测试**静默 SKIP**，
  包仍报 `ok` —— 不能据此声称"全绿"。
- `TestDunningDashboardHTTPWorkflowOnMySQL` 稳定红（09-19 工作台改版导致，与本次无关）。
- `TestDunningSendWorkflowOnMySQL` flaky，单次红不算数，需采样。
- **不要把 `go test` 接管道**（`| tail` 之类）：退出码取自管道末端，恒为 0，
  会把失败伪装成成功。要接就配 `set -o pipefail`。

本次不触碰数据库代码，走库测试非必需；若要跑，用一次性库：

```bash
mysql -e "create database rentops_check_$$ character set utf8mb4"
RENTOPS_MYSQL_TEST_DSN="root@tcp(127.0.0.1:3306)/rentops_check_$$?parseTime=true&multiStatements=true" \
  go test ./cmd/truelayer-demo/ -run 'MySQL' -count=1 -v
mysql -e "drop database rentops_check_$$"
```

## 提交

中文提交信息，直接落 `main`。建议：

```
fix(ui): 流水页去掉重复的状态筛选，统一「关联」用词并对齐主页状态文案
```
