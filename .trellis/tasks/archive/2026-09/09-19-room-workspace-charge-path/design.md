# 技术设计：房间视角直接汇总租客的月租

> **行号说明**：本文件写于改动**之前**，下文所有 `rent_workspace.go` 行号都是改动前的。
> 改动后已漂移，实际位置是：归属表 `399-417`、`chargeByRoom` `419-428`、
> `obligationsByRoom` `455-471`、房间循环（含空义务分支与歧义房间标记）`494-554`、
> 义务取数 `1152`、`ensureMonthlyObligations` 调用 `1133`。
> 未改动的文件（`obligations.go`、`landlord_rent_models.go`、`dashboard.go`）行号未漂移。

## 1. 边界

**做**：让 `/rent-dashboard` 的房间视角按「租客 → 房间」汇总**惰性义务**，
不再要求存在 `rent_charge`。

**不做**：补 charge 生产者；迁移/删除存量义务；改 `tenants.monthly_rent_cents`；
删 `rent_charges` / `ensureRentCharge`（另开任务）；改 UI 样式与响应式。

## 2. 现状：门控在哪

`rent_workspace.go` 里三道门把惰性义务挡在外面：

| 位置 | 代码 | 作用 |
|---|---|---|
| `:1118` | `AND rent_charge_id IS NOT NULL` | 取义务时就滤掉惰性行 |
| `:431-443` | `obligationsByCharge`，`row.RentChargeID == nil` 跳过 | 二次过滤，并按 charge 分组 |
| `:461-507` | `if charge, ok := chargeByRoom[roomRow.ID]; ok` | 整个房间的金额块挂在这个 charge 上 |

三道门源自同一个假设：**房间金额的事实记在 charge 上**。
本任务把它换成「事实记在义务上，房间由租客归属推出来」。

**实测本机数据**：`rent_obligations` 307 条，其中 `rent_charge_id IS NOT NULL` 的 **0 条**；
`rent_charges` 表 **0 行**。所以现状不是「部分缺失」，是**房间视角必然全空**。

## 3. 租客 → 房间的归属

`tenancyAgreement` **没有** `TenantID`（`landlord_rent_models.go:44-58`，只有 `RoomID`），
`tenant` 也**没有** `room_id`（只有 `RoomLabel` / `RoomAddress` 冗余字符串）。
所以归属只能走参与人：

```
obligation.TenantID
  → agreementParty.TenantID（该月有效）
  → tenancyAgreement(party.AgreementID).RoomID（该月覆盖）
```

这条链正是 `activePartyCountByRoom`（`:373-384`）已经在走的，只是键不同。

**归属规则**（按优先级）：

1. 该租客在该月恰好映射到一个生效房间 → 计入该房间；
2. 该租客在该月映射到多个房间 → **两边都不计**，并把涉及到的房间标 `needs_review`。
   宁可不显示，也不能重复计入 —— 重复计入会让合计虚高且看不出来；
3. 该租客没有映射到任何房间 → 不计入任何房间（见 §6 的已知偏差）。

**实测**：规则 2 与 3 在本机 Rosewood 种子上都不触发 ——
58 个租客全部有 agreement party，且没有任何租客同月挂在两个房间。

## 4. 改动点

### 4.1 先补义务（`load` 开头，取数之前）

`load` 现在**不生成**任何义务。`/bills` 之所以有数，是因为
`summarizeRentDashboardWithFilters`（`obligations.go:348`）会调
`ensureMonthlyObligations`（`obligations.go:172`）。房间视角必须做同样的事，
否则当月无义务可聚合：

```go
if err := newObligationService(s.db).ensureMonthlyObligations(ctx, userID, filters.PeriodMonth); err != nil {
    return rentWorkspaceData{}, err
}
```

该函数幂等（`OnConflict{Columns: user_id, tenant_id, period_month, DoNothing: true}`，
`obligations.go:201-204`）：重复调用不新增行，**也不改写已有行**。
它是 `tenantActiveInMonth`（`:130-152`）驱动的，只对 `monthly_rent_cents > 0` 的活跃租客建行。

**这是本任务唯一有数据面副作用的一步，语义是「只增不改」。**
在 GET 里补建当月义务是既有模式（`/bills` 已经这么做），不是新引入的行为。

### 4.2 取数放开（`:1118`）

去掉 `AND rent_charge_id IS NOT NULL`。

### 4.3 建房间归属（替换 `:395-404` 的 `chargeByRoom` 用途）

```go
roomIDByTenant := make(map[uint64]uint64)
ambiguousTenant := make(map[uint64]bool)
for _, row := range input.Parties {
    if row.UserID != input.UserID || !agreementPartyActiveInMonth(row, filters.PeriodMonth) {
        continue
    }
    agreement, ok := agreementByID[row.AgreementID]
    if !ok || !tenancyAgreementCoversMonth(agreement, filters.PeriodMonth) {
        continue
    }
    if existing, seen := roomIDByTenant[row.TenantID]; seen && existing != agreement.RoomID {
        ambiguousTenant[row.TenantID] = true
        ambiguousRoom[existing] = true
        ambiguousRoom[agreement.RoomID] = true
        continue
    }
    roomIDByTenant[row.TenantID] = agreement.RoomID
}
```

`chargeByRoom` **保留**（`input.Charges` 的查询不动）：charge 一旦将来被写，
它的 `DueDate` / `Currency` 比义务的推断值更权威。删掉它会牵动
`rentWorkspaceRoomAggregate.Charge` 的模板消费点，收益不成比例。

### 4.4 按房间聚合义务（替换 `:431-443` 与 `:461` 的入口）

把 `obligationsByCharge`（按 charge 分组）换成按房间分组：

```go
obligationsByRoom := make(map[uint64][]rentObligation)
for _, row := range input.Obligations {
    if row.UserID != input.UserID || row.RecordStatus != obligationRecordActive {
        continue
    }
    if monthStart(row.PeriodMonth) != filters.PeriodMonth {
        continue
    }
    if _, ok := tenantByID[row.TenantID]; !ok {
        continue
    }
    if ambiguousTenant[row.TenantID] {          // §3 规则 2
        continue
    }
    roomID, ok := roomIDByTenant[row.TenantID]  // §3 规则 3
    if !ok || roomByID[roomID].ID == 0 {
        continue
    }
    obligationsByRoom[roomID] = append(obligationsByRoom[roomID], row)
}
```

房间循环（`:461`）的入口条件从

```go
if charge, ok := chargeByRoom[roomRow.ID]; ok {
```

改成

```go
if rows := obligationsByRoom[roomRow.ID]; len(rows) > 0 {
```

块内逐条义务的处理逻辑（`projectRentObligation` → 余额 → `workspacePayments` →
`tenantRow` → 累加 `ExpectedCents` / `PaidCents` / `BalanceCents` / `Status`）**一行都不用改**，
只是数据源从 `obligationsByCharge[charge.ID]` 换成 `rows`。

必须改的两处回退链：

- `currency := firstNonEmpty(projected.Currency, charge.Currency, ledgerCurrencyEUR)`
  → `charge` 可能不存在，回退链变成
  `firstNonEmpty(projected.Currency, chargeCurrency, ledgerCurrencyEUR)`，
  其中 `chargeCurrency` 从 `chargeByRoom[roomRow.ID]` 取（不存在则为空串）；
- `aggregate.DueDate = charge.DueDate` → 改成取该房间**最早**的一条义务的 `DueDate`。
  同房间不同租客的 `DueDay` 可以不同（`dueDateForMonth(period, tenant.DueDay)`），
  取最早 = 最先该收的那笔，与「房间应收到期日」的直觉一致。

`:505` 的 `if len(aggregate.Obligations) == 0 { aggregate.Status = "needs_review" }`
要从 charge 块里挪到外面：改动后它表达「有归属但一条义务都没落进来」，语义仍成立。

**`ambiguousRoom` 里的房间必须显式标 `needs_review`**（放在上面那个分支之后，
因为 `vacant` 会覆写状态；不过歧义房间必然有租约与参与人，不会走到 `vacant`）。
少了这一步，「歧义租客 + 正常租客」的混合房间会**静默少计**且状态看着正常 ——
只有「歧义租客是该房间唯一租客」时才碰巧间接触发。单测必须用混合房间来钉这一点，
否则它区分不出标记有没有生效。

`:508-512` 的 `else if aggregate.HasAgreement && aggregate.HasActiveParty → needs_review
/ else vacant` **保持不变** —— 空置房间仍走这里。

### 4.5 需要核对的展示字段

`rentWorkspaceRoomAggregate.Charge` 是 `*rentCharge`。改动后
「**有金额但没有 charge**」将成为常态，所以要确认模板没有 `{{if .Charge}}`
之类的条件把金额块藏起来。**这是最容易漏的一处** ——
第 5 步实机验证要专门看页面，不是只看测试通过。

## 5. 与 `/bills` 的一致性

两页读同一批义务行（同 `user_id` / `period_month` / `record_status='active'`），
房间视角只多了「按租客归属到房间」这一步。在映射率 100% 的数据上，同月合计必然一致。

**实测对账基准（本机 Rosewood 种子）**：

| 项 | 值 |
|---|---|
| 租客 / 参与人 / 租约 | 58 / 63 / 40 |
| 义务总数 | 307（8 个月，2026-05 ～ 2026-12） |
| charge 背书义务 | **0** |
| `rent_charges` 行 | **0** |
| 2026-09 义务数 | 38（38 个不同租客） |
| 2026-09 可归属的租客 | **38**（100%） |
| 2026-09 有租约的房间 | 25（= 房间总数） |

**2026-09 是最干净的对账月**：38 条义务全部能归到房间，分布在 25 个房间。

## 6. 已知偏差与明确不做的事

- **归属不到的义务不计入房间视角。** 本机种子映射率 100%，不触发；若将来有租客没有
  租约却被收了租，两页合计会分叉。这是**已知且可见**的偏差（两页并排一眼能看出），
  修它需要引入「未分配」桶并改 UI，与本任务目标不成比例，暂不做。
- **`rent_charges` / `ensureRentCharge` 成为死代码**，本任务不动、不删。
- **不改 `tenants.monthly_rent_cents`**，不加迁移，不碰 `rent_obligations` 存量行。

## 7. 风险

| 风险 | 处置 |
|---|---|
| 模板把金额块藏在 `{{if .Charge}}` 里 | §4.5；第 5 步实机看页面 |
| 合租房间被算成「房间租金 × 人数」 | **金额只来自义务行的 `ExpectedAmountCents`**，与 `tenancyAgreement.MonthlyRentCents` 无关；代码里不得引入任何按房间租金相乘的算法 |
| `ensureMonthlyObligations` 意外改写存量义务 | 它是 `DoNothing` upsert，只增不改；验证命令带逐行对账 |
| 同一租客多房间导致重复计入 | §3 规则 2：判为歧义，两边都不计 |
| 空置房间被算成 0 元账单 | `vacant` 分支不动；但本机 25 个房间全有租约，**该分支需靠既有单测覆盖，不能靠本机页面** |
| 归属链在合租场景下漏人 | 14 个房间挂了 2+ 份租约（房间 41 挂 3 份）；2026-09 的房间 36 有 3 个租客。这正是归属链要处理的形状，实机要看合租房金额 |

## 8. 可回滚性

代码改动集中在 `rent_workspace.go` 的 `load` 与 `buildRentWorkspace` 两个函数，
`git revert` 即可回到「房间视角全空」的现状。

数据面唯一副作用是 `ensureMonthlyObligations` 补建当月义务 —— 而 `/bills`
本来就会补建同一批行，所以这不是新增的数据风险。
