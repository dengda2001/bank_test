# 房间视角应收金额：charge 路径缺口

## Goal

让 `/rent-dashboard`（房间视角）显示真实应收金额。当前它恒为 `EUR 0.00`，
房间状态恒为「待处理」，与 `/bills`、`/tenants`、`/tenants/{id}` 的口径对不上。

来源：`09-19-rosewood-rent-seed` 的验收过程中发现并转出
（见 `.trellis/tasks/archive/2026-09/09-19-rosewood-rent-seed/` 的
`prd.md`「撤回」条与 `implement.md`「处置决定」）。

## Confirmed Facts（已读码 + 实测，2026-09-19）

1. **症状**：本机灌入 Rosewood 数据后（4 房产 / 25 房间 / 58 租客 / 307 条义务），
   `/rent-dashboard` 房间树的「应收 / 已收 / 未收」全为 `EUR 0.00`，房间状态全是「待处理」；
   同一批数据在 `/bills`、`/tenants`、`/tenants/{id}` 上金额正常。
2. **路由**：有库会话下 `/rent-dashboard` 走 `renderRentWorkspaceDashboard`
   （`dashboard.go:15-17` 短路），不是义务视角的 `renderRentDashboard`。
3. **金额只来自 `rent_charges`**：`rent_workspace.go:1118` 的取数带
   `AND rent_charge_id IS NOT NULL`；`:433` 跳过 `RentChargeID == nil` 的行。
4. **没有任何生产代码写 `rent_charges`**：`ensureRentCharge`
   （`landlord_rent_ledger.go:244`）的调用方只有 `landlord_rent_ledger_test.go:225,235`
   与 `rent_workspace_test.go:396`；全仓库唯一的 `INSERT INTO rent_charges` 在
   `rent_obligation_dedup_migration_test.go:308`。**本机 `rent_charges` 为 0 行。**
5. **两条义务模型目前互斥**：`obligations.go:139` 的 `MonthlyRentCents <= 0` 守卫决定
   `ensureMonthlyObligations` 是否生成惰性义务。
   - **惰性路**（`rent_charge_id IS NULL`）：读 `tenants.monthly_rent_cents > 0`，
     由 `ensureMonthlyObligations` 写；`/bills`、`/tenants/{id}`、催收都读它。
     **当前所有真实数据都在这条路上。**
   - **charge 背书路**（`rent_charge_id NOT NULL`）：只由 `ensureRentCharge` 写，而它无人调用。
6. **迁移 013 的不变量约束的是惰性行，不是 charge 行**：
   `(user_id, tenant_id, lazy_period_month)` 唯一，`lazy_period_month` 是 STORED 生成列，
   仅当 `rent_charge_id IS NULL` 时等于 `period_month`，否则为 NULL
   （MySQL 唯一索引中 NULL 互不相等）。**因此给同一个 `(tenant, month)` 再加一条
   charge 背书义务不会被这条约束拦住 —— 这正是下面第 2 个候选方案的坑。**
7. **关系模型是好的**：房间树的人数（1 人 ×12 / 2 人 ×12 / 3 人 ×1）来自
   `activePartyCountByRoom`（agreement/party 模型），与实际入住一致。
   断的只是金额这一条链路。

## 根因：一个没接完的实现，不是产品取舍

设计意图是明确写着的。`obligations.go:136-138`：

```go
// Structured tenants are intentionally unbound until a room arrangement
// creates rent charges. Legacy tenant rows keep their monthly rent and room
// address, so this guard preserves their existing lazy-obligation behavior.
```

配合 `09-18-core-rent-ledger/design.md` 的链路设计
（`有效租约+参与人 → rent_charge（房间/月总应收快照） → rent_obligations → 覆盖`），
意图很清楚：**「房间安排」应当产生 rent charge**，structured 租客的义务挂在 charge 上。

但这条接线**不存在**：

- UI 建租约的路径是 `/tenancies` POST → `createTenancyFromPage`
  （`tenancy_pages.go:43-45`）→ `saveRentArrangement`
  （`landlord_domain_operations.go:354`，及其 Tx 变体 `:373`）——
  **两个函数全程不碰 charge / ledger / obligation**（已 grep 确认，零命中）；
- `ensureRentCharge`（`landlord_rent_ledger.go:244`）无生产调用方。

**所以两条模型互斥不是需要裁决的设计分歧，而是「生产者缺席」的后果。**
`rent_charges` 的写入方从来没被接上，structured 租客于是永远拿不到义务，
房间视角永远聚合出 `EUR 0.00`。

附带后果：`09-19-rosewood-rent-seed` 灌的种子同时写了 `monthly_rent_cents > 0`
与 agreement，属于「两路都占」—— 这些租客会出现在 `/bills`（惰性路）
却不出现在房间视角（charge 路）。

**2026-09-19 更正**：这里原先写成「是第 5 条守卫本意要避免的状态」，读反了。
守卫的意图是「structured 租客走 charge 路」，而 charge 路**没有生产者、也不会有了**。
那么「租客月租走惰性路」就是事实上唯一存在的那条路 ——
房间视角应当直接读它，而不是等一条永远不会来的 charge。

## Resolved Decisions（2026-09-19，用户已确认）

**房间视角直接汇总租客的月租，不补 charge 生产者。**

- **Q「房间页面的金额怎么取？」→ 直接取租客的**（用户在 AskUserQuestion 中选定）。
- 原先拟定的两个决定（charge 按月惰性补建、structured 租客 `monthly_rent_cents` 归零）
  **已作废**。

为什么这个方向对：

1. 我最初用「改租客月租会改写历史」论证必须走 charge，**这条理由是错的** ——
   `ensureMonthlyObligations` 用 `OnConflict{DoNothing: true}`（`obligations.go:201-204`），
   存量义务行永远不会被更新。两条路在这一点上没有差别，charge 并不更「保值」。
2. charge 多出来的「房间/月总额快照 + 每人责任额」，对「让页面显示出数」没有增量价值。
3. `rent_charges` 已经没有任何生产代码在写（**实测 0 行**），本身就是死代码；
   补生产者等于**给一具尸体接线**。
4. 改动小得多：不加迁移、不重灌数据、不碰 `monthly_rent_cents`，
   而且与 `/bills` 天然同源。

## Requirements

- **R1 房间视角改读义务**：`rent_workspace.go` 的房间聚合改为按
  「租客 → 生效租约参与人 → 房间」把**惰性义务**（`rent_charge_id IS NULL`）
  归集到房间，不再要求 charge 存在。见 design.md §3、§4。
- **R2 与 `/bills` 同源**：两页读同一批义务行，同月合计必须一致。
- **R3 触发义务生成**：`rentWorkspaceService.load` 要先调 `ensureMonthlyObligations`
  （与 `summarizeRentDashboardWithFilters` 一致），否则当月无义务可聚合。
  该调用是 `DoNothing` upsert，**只增不改**。
- **R4 归属不明时不猜**：一个租客同月若映射到多个房间，**两边都不计入**，
  不得重复计入。见 design.md §3 规则 2。
- **R5 空置房间**：无生效租约的房间保持 `vacant` 空态，不显示 0 元账单，不报错。
- **R6 不删不改存量数据**：不加迁移，不改 `rent_obligations` 存量行，
   不动 `tenants.monthly_rent_cents`。
- **R7 `rent_charges` / `ensureRentCharge` 本任务不动**，是删是留另开任务。

## Acceptance Criteria

- [ ] `/rent-dashboard` 房间树的应收合计与 `/bills` 同月合计一致（同口径、可对账）
- [ ] 合租房间金额 = 该房间各租客义务之和（**不是**房间租金 × 人数）
- [ ] 空置房间仍为空态，不显示 0 元账单，不报错
- [ ] 同一租客同月映射到多房间时不被重复计入
- [ ] 不加迁移；`rent_obligations` 改动前后可逐行对账（307 行不变、金额不变）
- [ ] `go test ./cmd/truelayer-demo/...` 与 `go vet ./...` 通过
- [ ] 跨用户不可见

## 对账基准（本机 Rosewood 种子，实测）

2026-09：`/bills` 38 条义务 / 38 个租客；房间视角 38 条归属 / 25 个房间。
全量 8 个月（2026-05 ～ 2026-12）每月 37–40 条义务，映射率 100%
（58 个租客全部有 agreement party，无租客同月跨两房）。

## Out of Scope

- 灌测试数据本身（已由 `09-19-rosewood-rent-seed` 交付）。
- 房间视角的 UI 样式与响应式（属于 `09-19-pc-ui-fidelity-alignment` 及其子任务）。
- **删除 `rent_charges` / `ensureRentCharge` 死代码**（R7）——它们在本任务后彻底无用，
  但删除是独立的一件事，另开任务。
- **「未分配」桶**：归属不到房间的义务不计入房间视角（design.md §6 的已知偏差）。
  本机种子映射率 100%，不触发；引入它要改 UI，与本任务目标不成比例。

## Related

- 父级产品需求：`.trellis/tasks/09-18-landlord-multi-property-rent-view`（7 个子任务已全部归档，
  本缺口不在其原范围内，是后 MVP 缺口）
- 读模型记录：`.trellis/spec/backend/database-guidelines.md`
  的「Scenario: Room-Centric Rent Workspace Read Model」
