# 技术设计：删除无库降级 dashboard 模板

> 行号基于 2026-09-19 的工作区状态；本任务会改动 `dashboard.go`，行号会漂移，实现时以符号名为准。

## 0. 一句话

删掉 `rentDashboardTemplate` 这个模板变量，把「没有数据库」这条路径从"悄悄降级到旧模板"
改成"明确报错"，并保住 `/bills`、`/dunning` 的渲染与筛选覆盖。

---

## 1. 先更正 prd 的一个前提

prd 的「背景」一节只描述了 `dashboard.go:19` 这一个入口，读起来像
`renderRentDashboard` 整个函数都随模板一起死。**这是错的。**

`renderRentDashboard`（`dashboard.go:22`）有 **4 个调用点，其中 3 个是活的**：

| 调用点 | 谁 | 是否活 |
|---|---|---|
| `dashboard.go:19` | `handleRentDashboard` 的 `db == nil` 回退 | **死**（本任务要收敛的） |
| `page_data_routes.go:447` | `handleBills` → `/bills` | **活** |
| `page_data_routes.go:478` | `handleDunningPage` → `/dunning` | **活** |
| `dunning_handlers.go:82` | 催收发送动作 | **活** |

也就是说：**`/bills` 和 `/dunning` 的渲染体就是 `renderRentDashboard`**，
里面那个按路径选模板的 `switch` 正是它们选中 `billsPageTemplate` / `dunningPageTemplate` 的地方。

> **报错写法照抄现成范例。** `handleDunningAction`（`dunning_handlers.go:65-68`）已经在做同一件事：
> `a.db == nil` → `http.Error(w, "dunning requires database-backed user sessions", http.StatusServiceUnavailable)`。
> §3.1 的改写**按这个样式来**（503 + 一句能读懂的英文说明），不要自创风格或静默重定向。

结论：**函数留着，函数里的 `switch` 也留着。** 要删的只有三样：

1. `rentDashboardTemplate` 变量本身（HTML 体 + 它的 funcmap）；
2. `dashboard.go:19` 这个唯一能走到 `db == nil` 的入口；
3. 上面这条入口死掉之后，`switch` 的默认分支（`templateForPath := rentDashboardTemplate`）。

---

## 2. 调用链（现状）

```
/rent-dashboard ──▶ handleRentDashboard (dashboard.go:11)
                      ├─ requireAuth? 否 → 拦掉
                      ├─ 有会话 ∧ a.db != nil → renderRentWorkspaceDashboard   ← 生产唯一实际路径
                      └─ 否则 → renderRentDashboard(w, r, nil)                 ← 只可能 a.db == nil
                                                                                 ★ 本任务：改成明确报错

/bills ──────────▶ handleBills (page_data_routes.go:439)
                      ├─ scopedPageUser? 否 → 拦掉
                      └─ renderRentDashboard(w, r, nil) ──┐
                                                          │
/dunning ────────▶ handleDunningPage (:461)               │
                      ├─ scopedPageUser? 否 → 拦掉        │
                      └─ renderRentDashboard(...) ────────┤
                                                          │
/dunning/send ───▶ dunning_handlers.go:82 ────────────────┤
                                                          ▼
                                        renderRentDashboard (dashboard.go:22)
                                          ├─ if 有会话 ∧ a.db != nil → 从库里取数
                                          ├─ else → loadTenants/loadExpenses/loadLatestDemoResult   ★ 见 §4
                                          └─ switch r.URL.Path
                                               /bills          → billsPageTemplate      ← 活
                                               /dunning*       → dunningPageTemplate    ← 活
                                               (默认)          → rentDashboardTemplate  ★ 本任务：改成明确报错
```

---

## 3. 删除边界

### 3.1 确删

- `rentDashboardTemplate`（`dashboard.go:180` 到文件末附近的 var + 内联 raw string HTML）
- 它的 `template.FuncMap{...}` 字面量
- `dashboard.go:19` 的 `a.renderRentDashboard(w, r, nil)`
- `dashboard.go:168-174` 的 `templateForPath := rentDashboardTemplate` 默认值与 `switch` 的整体形态（改成显式穷举 + 默认报错）

### 3.2 必留（删了就是事故）

| 符号 | 位置 | 为什么留 |
|---|---|---|
| `renderRentDashboard` | `dashboard.go:22` | `/bills`、`/dunning` 的活渲染体 |
| `billsPageTemplate` | `rent_collection_pages.go:38` 前后 | `/bills` 唯一模板 |
| `dunningPageTemplate` | `rent_collection_pages.go:56` | `/dunning` 唯一模板 |
| `rentDashboardPageData` | `dashboard.go` | 两个活模板共用的数据契约 |
| `rentDashboardFiltersFromQuery` / `defaultRentDashboardFilters` | `dashboard.go` | `/bills`、`/dunning` 的筛选解析 |
| `filterAndSortRentDashboardRows` | `dashboard.go` | 纯数据逻辑，与本模板无关 |
| `handleDashboardManualBalance` + `/rent-dashboard/settle` 路由 | `dashboard_manual_balance.go:117`、`main.go:465` | **见 §5** |
| `loadTenants` / `loadExpenses` / `loadLatestDemoResult` | `main.go:2335/2347/2023` | **见 §4**（先证明分支死，再决定动不动） |

### 3.3 禁止

- 为了 `go test` 变绿而删除覆盖筛选/排序的断言（prd R4）
- 顺手删 `main.go` 里的无库演示子系统（prd R5）
- `git checkout` / `stash` / `clean`（工作区有 82 项未提交变更，含用户自己的改动）

---

## 4. `else` 分支（`dashboard.go:152`）的死因

守卫是 `dashboard.go:107`：

```go
if userID, ok := a.currentUserID(r); ok && a.db != nil {
    // 从库里取数
} else {
    // loadTenants / loadExpenses / loadLatestDemoResult
}
```

`else` 要成立，需要 `!ok`（无会话）**或** `a.db == nil`。逐个调用点看：

- `handleBills`（`page_data_routes.go:444`）先过 `a.scopedPageUser(w, r)`，不过就 `return`
- `handleDunningPage`（`:466`）同上
- `handleRentDashboard`（`dashboard.go:12`）先过 `requireAuth`
- `dunning_handlers.go:82` 的调用点 —— **✅ 已核实（见下）**

**门 1.1 已通过（2026-09-19，实现前查证）：** `dunning_handlers.go:82` 所在的
`handleDunningAction`（`:61`）三道都过：

```go
if !a.requireAuth(w, r) { return }                 // :62  必须已登录
if a.db == nil { http.Error(..., 503); return }    // :65  必须已配库
if _, ok := a.currentUserID(r); !ok { http.Error(..., 401); return }  // :69 必须有会话用户
```

四个调用点都先鉴权 ⇒ `!ok` 不可能 ⇒ `else` 等价于 `a.db == nil` ⇒ **不可达**（`db.go:23` 已经在启动时拦掉了无库启动）。

**处置建议**：本任务**先不动这个 `else` 分支**。

理由：删了它只是删掉一段已经死的代码，收益是行数；但它牵动 `loadTenants` 等三个函数整条调用链，
而 prd R5 已经声明它们不是本任务的删除对象。**把它作为一条独立结论写进 spec，交给后续任务**，
比在这个以"删一个模板"为主题的任务里顺手扩大战线更安全。

> 如果实现时发现 `dunning_handlers.go:82` 的调用链**没有**先鉴权，那 `else` 分支就是活的，
> 上面的结论作废 —— 此时**必须停下来**回报，不要按本设计继续。

---

## 5. `/rent-dashboard/settle` 的处置（重要）

删掉旧模板后，**没有任何模板再 POST 到 `/rent-dashboard/settle`**：

- 旧模板里两处表单 action（`dashboard.go:472`、`:476`）随模板消失
- `/bills` 的平账 POST 到 `/bills/settle`（`rent_collection_pages.go:49-50`）
- **房间视角 `web/templates/pages/rent-workspace.html` 里没有平账入口**（全文件搜 `settle`/`平账` 无命中）

所以这个路由会变成"没有模板调用它，但 HTTP 上仍可达"的孤岛。

**处置：保留路由、保留 `handleDashboardManualBalance`、保留 `dashboard_manual_balance_test.go`。**

理由（这是本任务最容易做错的地方）：

1. 它是一个 HTTP 端点，不是死变量。外部可以直接 POST，不因为"没有模板指向它"而失去意义。
2. **平账是原型图里的功能**（父任务 prd 的需求 C2）。目前真实的 `/rent-dashboard`（房间视角）
   **缺这个功能**，子任务 ③ 要对齐它，最省事的实现方式就是让房间视角的表单 POST 到这个已经存在的
   handler。现在删掉，等于让 ③ 从零再写一遍已经写好并被测试覆盖的服务端逻辑。
3. 删掉它会让 `dashboard_manual_balance_test.go`（2 个测试）连带失效，正是 prd R4 要禁止的
   "测试没了就绿了"。

**只有一个断言随模板删**：`dashboard_layout_test.go:224` 断言旧模板输出里有
`action="/rent-dashboard/settle"` —— 这是模板专属的标记断言，随模板走。

---

## 6. 测试逐个处置（17 项）

分类口径：**测数据逻辑 → 保留**；**测共享外壳契约 → 改指向**；**只测旧模板标记 → 删，且要能说出原来测什么**。

| # | 文件:行 | 函数 | 在测什么 | 处置 |
|---|---|---|---|---|
| 1 | `mobile_layout_test.go:95` | `TestEveryWorkspacePageRendersTheSharedChromeOnce` | 工作区页面共享外壳只渲染一次 | **改指向**房间视角模板 |
| 2 | `mobile_layout_test.go:204` | `TestMobileBottomNavExposesFiveSectionsAndObjectsMenu` | 移动端底部导航五个分区 | **改指向** |
| 3 | `mobile_layout_test.go:250` | `TestMobileDashboardUsesCardsForRentRows` | 移动端租金行渲染成卡片 | **改指向 `billsPageTemplate`**（它有 `.collection-bill-card`，是同一契约的真实实现） |
| 4 | `mobile_layout_test.go:377` | `TestMobileDunningUsesSafeBottomSheet` | 催收抽屉在移动端用安全区底部弹层 | **⚠️ 无处可指** —— 见 §7 |
| 5 | `mobile_layout_test.go:475` | `TestNavCountsOnlyRenderWhereTheyDidBefore` | 侧栏计数只在原有页面出现 | **改指向** |
| 6 | `main_test.go:74` | `TestWorkspaceTemplatesIncludeSharedCalendarPicker` | 模板清单都含共享日历选择器 | 从清单**移除旧模板那一项**，清单本身保留 |
| 7 | `main_test.go:904` | `TestRentDashboardTemplateRendersMonthlyStatus` | 旧模板月状态渲染 | **删**（模板专属） |
| 8 | `main_test.go:1202` | `TestRentDashboardTemplateLinksPendingCountToSelectedPeriod` | 待处理数链接带周期参数 | **改指向 `/bills`**；若断言绑死旧类名则删，须注明原意 |
| 9 | `dashboard_layout_test.go:31` | `TestRentDashboardSeparatesPeriodBarSummaryAndList` | 周期栏/汇总/列表三段结构 | **删**（旧模板专属布局，等价契约归 ③） |
| 10 | `dashboard_layout_test.go:177` | `TestRentDashboardMetricsKeepTheE2EShape` | E2E 依赖的指标形状 | **改指向房间视角**；须先确认 E2E 依赖的是哪套选择器 |
| 11 | `dashboard_layout_test.go:198` | `TestRentDashboardManualBalanceActionOnlyAppearsForOutstandingRent` | 平账只在有未付时出现 | **改指向 `billsPageTemplate`**（同一表达式 `gt .ExpectedCents .PaidCents` 真实存在于 `rent_collection_pages.go:49`） |
| 12 | `dashboard_layout_test.go:224` | （同文件断言） | 旧模板输出含 `action="/rent-dashboard/settle"` | **删该断言**（§5） |
| 13 | `dashboard_filters_test.go:108` | `TestRentDashboardTemplateRendersFiltersMetricsAndPagination` | 筛选/指标/分页渲染 | **改指向 `billsPageTemplate`** |
| 14 | `dashboard_filters_test.go:161` | `TestRentDashboardTemplateDistinguishesNoMatchesFromNoBills` | 空态区分 | **改指向 `billsPageTemplate`**（`:52` 的 `{{if gt .TotalRows 0}}` 两态真实存在） |
| 15 | `dunning_handlers_test.go:68` | `TestRentDashboardTemplateRendersDunningDrawerAndRetry` | 抽屉 + 重试 | **拆**：服务端重试逻辑保留，抽屉标记断言随模板删 |
| 16 | `page_data_routes_test.go:42` | `TestDashboardManualBalanceRequiresReasonField` | 服务端 reason 必填校验 | **保留**，渲染载体改成 `billsPageTemplate` |
| 17 | `dashboard_manual_balance_test.go`（整文件，2 个测试） | `...RedirectPreservesDashboardFilters` / `...HandlerAcceptsPostOnly` | handler 行为 | **保留**（§5） |

**改指向 ≠ 改名字。** 判据是：改完之后这个测试**仍在断言原来那件事**，
只是换了一个活的载体。如果换成 `billsPageTemplate` 后断言不出原来那件事，
说明它本来就是模板专属测试，按"删"处理并写明原意。

---

### 6.1 实施后补记：清单外的依赖者与残余

**第 18 项（本设计漏了，实施时补上）**：`TestRentDashboardHTTPRendersSafeFallbackAndFilterError`
（`dashboard_filters_test.go:175`）。它不点名 `rentDashboardTemplate`，所以按名字搜不到；
但它靠"有 session 无 DB"直接驱动 `handleRentDashboard`，受 R3 影响。
⇒ **教训：按符号名 grep 不足以找出全部依赖者，要按语义找**（搜 `db == nil`、
`renderRentDashboard`、`handleRentDashboard`，以及断言里出现旧模板专属类名的测试）。

**第 19 项不存在**，但查明三处残余，此前均未登记：

| 残余 | 位置 | 性质 |
|---|---|---|
| `TestDunningDashboardHTTPWorkflowOnMySQL` | `dunning_handlers_mysql_test.go:60` | **未登记的依赖者**。用官方 harness 跑当前树与 `c944538` 基线 → 两者都 `EXIT=1`，同一行、同一断言、同一消息。**前后一致失败 ⇒ 不是本任务回归**，属于既有 known-red |
| `.dashboard-toolbar .calendar-input` | `web/static/css/calendar.css:12,197` | 随模板变成**死规则**（该 CSS 文件本身仍 live）。未清理，可接受 |
| `.dashboard-pagination` 回退分支 | `scripts/audit/probe-p1-4.mjs:61` | 在本任务**之前**就是死的，非本任务引入 |

**已知红清单（本任务不修，报告时不要算进红绿）**：
`TestDunningDashboardHTTPWorkflowOnMySQL` —— 改动前后一致失败，已在干净副本上双向复现。

---

## 7. 已知损失：催收抽屉没有活的落点

`TestMobileDunningUsesSafeBottomSheet`（#4）测的是旧模板里的 `data-dunning-open` 抽屉
（`dashboard.go:461`）。全仓库核对结果：

- `dunningPageTemplate`（`/dunning`）**没有**抽屉/弹层
- 房间视角 `rent-workspace.html` **没有** `dunning-open` / `drawer` / `sheet` 任何命中

也就是说：**"从工作区直接发起催收"这个原型功能，目前只存在于即将被删的模板里。**

### 7.1 第二处缺口：催收「重试此人」按钮（本设计原先不知道）

上面只覆盖了抽屉。**复核阶段查明还有第二处**：`TestRentDashboardTemplateRendersDunningDrawerAndRetry`
（第 15 项）同时还断言了一个 **"重试此人"** 按钮。核对结果同样是**全仓无 live 载体**
（`/dunning` 模板与 `rent-workspace.html` 都没有对应 DOM）。

要点：**服务端重试逻辑仍然完好** —— `dunning_service.go:254/266/301-303` 实现了重试，
且被 `dunning_send_mysql_test.go:165`、`dunning_test.go:109` 覆盖。
**丢的是"入口"，不是"能力"**。所以 ③ 要做的是把按钮接回去，不是重写服务端。

⇒ 两处缺口（抽屉、重试按钮）都必须登记进 spec 的「交给 ③ 的已知缺口」段。
**只登记一处就是漏报。**

因此本任务**不能悄悄删掉这个测试**。处置：

1. 在实现时把这条结论写进 `check.jsonl` 的说明与 spec（`database-guidelines.md` 的对应小节），
   明确登记为「原型功能在真实路径上缺失」；
2. 测试本身按"删"处理（它测的标记确实不存在了），但**必须**在提交信息或 spec 里保留这条发现；
3. 把"房间视角补催收入口"登记为 ③ 的输入（父任务 prd 的需求 C 系列）。

同理，`metrics.mjs` 一类审计脚本若依赖旧选择器，也要在实现时同步核对（§8）。

---

## 8. 审计脚本影响

`scripts/audit/` 下两个文件命中旧模板选择器：

- `metrics.mjs:23`：`page.locator('.rent-row, tbody tr').count()` —— 选择器是**或**关系，
  第二个分支 `tbody tr` 对房间视角仍有效，所以不会直接失败，但 `.rent-row` 这个分支会永久失配。
- `seed.mjs:143-148`：一段英文注释，已经准确描述了本 PRD 的调用链分析
  （"`/rent-dashboard` never reaches that branch once a session exists"）。

处置：`metrics.mjs` 的选择器收敛为活的那些（或保留 `, tbody tr` 并把 `.rent-row` 拿掉）；
`seed.mjs` 的注释随模板删除更新措辞。

**注意**：`scripts/audit/` 是 ① 正在使用的工具（`desktop-widths.mjs`）。本任务排在 ① 之后执行，
改动前先确认 ① 已归档，避免两边同时动审计脚本。

---

## 9. 风险与回滚

| 风险 | 影响 | 缓解 |
|---|---|---|
| 误删 `renderRentDashboard` 或它的 `switch` | `/bills`、`/dunning` 直接 500 | §3.2 必留表；`go test ./...` 覆盖 `/bills` 全流程 |
| 改 `switch` 默认分支时写错条件 | `/bills`、`/dunning` 输出变化 | AC 要求逐字节对比；改动只在默认分支，活路径不经过 |
| `else` 分支其实是活的 | 删了会丢无库演示数据 | §4 的核实前置；不确定就**不动** |
| 测试改指向后断言变弱 | 覆盖静默流失 | §6 判据 + 逐项"原来测什么"说明；R4 |
| 顺手扩大战线（`main.go` 大扫除） | 与工作区 82 项未提交变更打架 | prd Out of Scope；§3.3 |

**回滚**：本任务是纯删除 + 局部收敛，`git revert` 单个提交即可回到现状；不涉及数据迁移、无 schema 变更。

---

## 10. 验证命令

```bash
cd cmd/truelayer-demo

# 1) 模板与引用清零
/usr/bin/grep -rn 'rentDashboardTemplate' --include='*.go' .   # 期望：无输出

# 2) 活路径不回归
go test ./... -count=1 2>&1 | tail -30

# 3) 静态检查
go vet ./...

# 4) 逐字节对比 /bills 与 /dunning（改动前后各跑一次，diff 必须为空）
#    用现有 HTTP 测试或 audit 脚本渲染同一输入后 diff
```

行数目标：`dashboard.go` 从 **569 行**降到 **200 行以内**（prd AC）。

---

## 11. 遗留：第二个死模板

① 的实现代理发现 `legacyExpenseTemplate`（`main.go:2753`）同样**无人引用**。

**本任务不并入。** 理由：它不在 `dashboard.go`，与"删 dashboard 模板"主题无关，
且并进来会把改动扩散到 `main.go`（工作区未提交变更最密集的文件）。
登记为后续独立小任务。
