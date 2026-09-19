# 删除无库降级 dashboard 模板

> 配套文档：`design.md`（删除边界、调用链、17 项测试逐个处置、风险回滚）、
> `implement.md`（带停机门的执行清单）。

## Goal

删掉 `rentDashboardTemplate`（`dashboard.go:180` 起，HTML 是内联 raw string，到文件末尾附近）
以及只有它能走到的 `/rent-dashboard` 无库降级渲染路径。

用户的决定（2026-09-19）：**「删掉吧」。**

## 背景：为什么它是死代码

这不是推测，是三段事实合起来的结果：

1. **没有数据库，应用根本起不来。** `db.go:23` 调 `resolveDatabaseDSN`，
   后者在 `MYSQL_DSN` 与 `DATABASE_URL` 都为空时返回
   `errors.New("MYSQL_DSN or DATABASE_URL is required")`（`main.go:602-607`，唯一调用点在启动路径上）。
   → **生产环境里 `a.db == nil` 不可达。**
2. **模板按请求路径选**（`dashboard.go:168-174`）：
   `/bills` → `billsPageTemplate`，`/dunning*` → `dunningPageTemplate`，
   其余 → `rentDashboardTemplate`（**默认分支**）。
3. **能落到默认分支的只有 `/rent-dashboard`**，而它在有会话且有库时被 `dashboard.go:15-17` 短路到房间视角，
   只有 `a.db == nil` 时才走到 `dashboard.go:19` 的 `renderRentDashboard`。

三条合起来：默认分支 ∧ `db == nil` → **不可达**。

### 重要更正（2026-09-19 实现前的代码核实）

上面的推理只覆盖了 `dashboard.go:19` 这一个入口，容易读成「`renderRentDashboard` 整个函数随模板一起死」。
**这是错的，已核实。** `renderRentDashboard`（`dashboard.go:22`）有 4 个调用点，其中 3 个是活的：

| 调用点 | 谁 | 死活 |
|---|---|---|
| `dashboard.go:19` | `handleRentDashboard` 的 `db == nil` 回退 | **死**（本任务收敛它） |
| `page_data_routes.go:447` | `handleBills` → `/bills` | **活** |
| `page_data_routes.go:478` | `handleDunningPage` → `/dunning` | **活** |
| `dunning_handlers.go:82` | 催收发送动作 | **活** |

⇒ **`/bills` 和 `/dunning` 的渲染体就是 `renderRentDashboard`**，里面那个按路径选模板的 `switch`
正是它们选中 `billsPageTemplate` / `dunningPageTemplate` 的地方。

⇒ **函数必须留，`switch` 也必须留。** 本任务删的只有：模板变量本身、`dashboard.go:19` 这个
唯一的 `db == nil` 入口、以及入口死掉后 `switch` 的默认分支。详见 `design.md` §1–§2。

**状态**：`abb74ac` 已把「有库时该模板不可达」写进
`.trellis/spec/backend/database-guidelines.md`；`8e9dc5c` 已把
`/rent-dashboard` 的验收项从 `09-19-desktop-contract-1100` 撤回。
本任务是把「已记录在案」变成「已删除」。

## Confirmed Facts

- 模板体是**内联字符串**，不是独立文件：`dashboard.go:180` 的
  `var rentDashboardTemplate = newWorkspacePageTemplate("rent-dashboard", template.FuncMap{...}, ` + 反引号 HTML。
  `dashboard.go` 全文 569 行，该 var 占其中约 380 行（`data-dunning-open` 在 `:461`、`tr.rent-row` 在 `:476`，
  两者都在它内部）。
- **直接引用 `rentDashboardTemplate` 的测试有 15 处，分布在 6 个文件**：
  - `mobile_layout_test.go`（5 处：`:95,204,250,377,475`）
  - `main_test.go`（3 处：`:74,904,1202`）
  - `dashboard_layout_test.go`（3 处：`:31,177,198`）
  - `dashboard_filters_test.go`（2 处：`:108,161`）
  - `dunning_handlers_test.go`（1 处：`:68`）
  - `page_data_routes_test.go`（1 处：`:42`）
- **但影响面是 17 项、7 个文件**：还有两处不引用变量名、却依赖旧模板输出的测试：
  - `dashboard_layout_test.go:224` —— 断言旧模板输出含 `action="/rent-dashboard/settle"`
  - `dashboard_manual_balance_test.go` 整个文件（2 个测试）—— 覆盖 handler，**要保住**
  逐项处置见 `design.md` §6。
- **`/bills` 与 `/dunning` 不受影响**：它们各自有模板，且是本任务要保住的。
  两者共用 `rentDashboardPageData` 与 `rentDashboardFiltersFromQuery` / `defaultRentDashboardFilters`，
  这些**必须保留**。两者的渲染体 `renderRentDashboard` 同样**必须保留**（见上面的更正）。
- **`scripts/audit/` 也命中旧模板选择器**：`metrics.mjs:23` 的 `.rent-row`、`seed.mjs:143-148`
  的注释。见 `design.md` §8。

## Requirements

- **R1 删除模板本身**：`rentDashboardTemplate` 变量、它的 HTML 体、以及**只被它使用**的 funcmap 项
  （`dashboardPreviousPage` / `dashboardNextPage` / `dunningDeliveryLabel` 等需逐个确认是否被
  `billsPageTemplate` / `dunningPageTemplate` 复用；复用的必须留下）。
- **R2 `switch` 的默认分支不再是「降级」**：`templateForPath := rentDashboardTemplate` 这个默认值
  改为**显式穷举 + 默认报错**。**不要**留一个指向已删除变量的回退。
  注意这个 `switch` 在 `renderRentDashboard` 内部，而该函数是 `/bills`、`/dunning` 的活渲染体，
  所以 `switch` 本身留下，只改默认分支 —— 两个活 case 一行不动。
- **R3 `/rent-dashboard` 无库路径收敛**：`dashboard.go:19` 的 `a.renderRentDashboard(w, r, nil)`
  在 `db == nil` 时已不可达，改为明确报错（不要静默重定向，那样会掩盖配置缺失）。
  **`renderRentDashboard` 函数本身保留不动**（见上面的更正）。
- **R4 保住筛选/排序的测试覆盖**：`dashboard_filters_test.go`、`dashboard_layout_test.go`
  里的部分测试是**借这个模板**测筛选与排序行为的。删模板前必须判断每个测试在测什么：
  - 测纯数据逻辑（`filterAndSortRentDashboardRows` 等）→ 保留，改断言方式；
  - 测该模板专属的渲染 → 删；
  - 两者都有 → **改指向 `billsPageTemplate`**，不要直接删掉。
  **禁止**用「测试删了，`go test` 就绿了」的方式完成本任务。
- **R5 不动无库演示子系统、也不动 `else` 分支**：`loadTenants`（`main.go:2335`）、
  `loadExpenses`（`:2347`）、`loadLatestDemoResult`（`:2023`）被 `main.go` 多处与
  `transaction_detail.go:142` 使用，**不是本任务的删除对象**。

  关于 `dashboard.go:152-155` 那个 `else` 分支（调用上面三个函数的地方）：守卫是 `:107` 的
  `ok && a.db != nil`，要走进 `else` 需要 `!ok`（无会话）**或** `a.db == nil`。四个调用点
  **都先鉴权**，因此 `!ok` 不可能，`else` 等价于 `a.db == nil` ⇒ **不可达**。

  **但本任务仍然不动它。** 删它只省行数，却要牵动三个函数的整条调用链，与 R5 的
  「不是删除对象」自相矛盾，也让这个以"删一个模板"为主题的任务扩散成大扫除。
  处置：把这条死因结论**写进 spec 作为独立后续任务**。

  ⛔ **前置核实**：实现时必须先确认催收发送那条调用链（`dunning_handlers.go:82`）确实先鉴权。
  **若否，`else` 是活的，结论作废，必须停下来回报。** 见 `design.md` §4。
- **R6 `/rent-dashboard/settle` 保持存活**：删掉旧模板后没有任何模板再 POST 到它，但它是可达的
  HTTP 端点，且平账是原型功能（父任务需求 C2）、真实房间视角目前缺这个功能（子任务 ③ 要用）。
  ⇒ **保留路由、保留 `handleDashboardManualBalance`、保留 `dashboard_manual_balance_test.go`。**
  只删 `dashboard_layout_test.go:224` 那条模板标记断言。依据见 `design.md` §5。
- **R7 登记催收抽屉的功能缺口**：`TestMobileDunningUsesSafeBottomSheet` 测的抽屉标记**只存在于
  即将被删的模板里**（`/dunning` 模板和房间视角都没有）。删测试可以，但**必须**把
  「原型功能在真实路径上缺失」登记进 spec 并交给 ③，不许悄悄删。见 `design.md` §7。

## Acceptance Criteria

- [ ] `rentDashboardTemplate` 及其 HTML 体已删除，全仓库无残留引用
      （`/usr/bin/grep -rn 'rentDashboardTemplate' --include='*.go' .` 无输出）
- [ ] `renderRentDashboard` **仍然存在**，`/bills` 与 `/dunning` 两个 case 一行未动
- [ ] `switch` 不再有指向已删除变量的回退分支，默认分支是显式报错
- [ ] `/bills` 与 `/dunning*` 渲染**逐字节不变**（改动前后对同一输入对比输出）
- [ ] `rentDashboardPageData` / 筛选与排序 helper / `renderRentDashboard` 未被误删
- [ ] 测试文件中**没有**为了通过而删除的、原本覆盖筛选/排序行为的断言
      （每个被删或被改的测试都要能说出它原本在测什么）
- [ ] `/rent-dashboard/settle` 路由与 `handleDashboardManualBalance` 仍存在，
      `dashboard_manual_balance_test.go` 两个测试仍在且通过（R6）
- [ ] 催收抽屉的功能缺口已登记进 spec 并标注交给 ③（R7）
- [ ] `scripts/audit/` 对旧选择器的依赖已同步（`metrics.mjs:23`、`seed.mjs:143-148`）
- [ ] `go test ./... -count=1` 与 `go vet ./...` 通过
- [ ] `dashboard.go` 行数显著下降（预期从 569 行降到 200 行以内）

> **注**：`TestDunningDashboardHTTPWorkflowOnMySQL` 在本任务之前就已失败
> （已在 `git archive HEAD` 的干净副本上独立复现），不计入本任务的红绿，本任务也不修它。
> 报告时必须如实说明，不许把它算作"通过"。

## Out of Scope

- **`dashboard.go:152-155` 的 `else` 分支**及其调用的 `loadTenants` / `loadExpenses` /
  `loadLatestDemoResult`（死因已查清，见 R5，作为独立后续任务）。
- `legacyExpenseTemplate`（`main.go:2753`，同样无人引用的第二个死模板，见 `design.md` §11）。
- 其它页面的无库降级路径（`main.go` 里的 `loadTenants` 等调用点）。
- `/bills`、`/dunning` 模板自身的对齐（属于 `09-19-list-pages-alignment`）。
- 房间视角（`rent-workspace.html`）的任何改动 —— 包括补催收入口（那是 ③ 的活）。
- 工作区里不属于本任务的未提交变更。

## 与父任务的关系

本任务是从 `09-19-pc-ui-fidelity-alignment` 的「结构性隐患」一节派生出来的。
父 PRD 记录：「两套模板内容不同，对齐时需要确认以哪一套为准，并避免两套继续分叉。」
用户已裁决：**以房间视角为准，删掉旧的那套。**
本任务完成后，`09-19-dashboard-alignment`（子任务 ③）只需对齐单一模板。

**执行顺序**：本任务应在 ③ 之前完成（它是 ③ 的前置清理），但与 ①②④⑤ 无依赖冲突。
