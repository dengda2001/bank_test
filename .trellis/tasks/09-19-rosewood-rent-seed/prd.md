# Rosewood 收租明细灌入测试数据

## Goal

`rentops` 库已清空（只保留 `users` 与 `schema_migrations`），应用点进去处处是空列表，无法验证任何页面。
本任务把 `收租明细_Rosewood_20260916.xlsx` 里的真实租赁关系灌进去，让 11 个页面都有内容可点、可筛、可翻页。

数据来源是真实表（都柏林 4 处房产的收租台账），不需要逐格还原——目标是**关系自洽、金额合理、状态多样**，
让空态/正常态/异常态都能被看到。

## Confirmed Facts（实测）

- 源文件：`收租明细_Rosewood_20260916.xlsx`（仓库根目录），4 个 sheet，每 sheet 一处房产。
- 结构：r2 = 地址；r4 = 表头；r5 起为「月分组」，每组首行带年月（Excel 序列值），后续行为同月其余租客；
  每组末尾有汇总行（列2=房间数、列4=应收合计）。
- 规模：**4 处房产 / 25 个房间 / 8 个月（2026-05 ~ 2026-12）/ 320 条租客-月记录**。

| 房产 | 房间 | 租客-月 | 去重后姓名 |
|---|---|---|---|
| 72 Walkinstown Rd Dublin 12 | 1–8 | 96 | ~27 |
| 78 Old County Road Dublin 12 | 1–5 | 72 | ~19 |
| 116 Kimmage Rd W Dublin 12 | 01–06 | 88 | ~17 |
| 169 Windmill Park Dublin 12 | 01–06 | 64 | ~9 |

- 序列值换算：`46023`=2026-01-01，故 `46143`=2026-05、`46174`=2026-06、`46204`=2026-07、
  `46235`=2026-08、`46266`=2026-09，其后每月一组至 2026-12。
- **存在合租**：同一房间同月多行（如 72 Walkinstown 的 2 号房 = Ardra 580 + Meghana 580；
  4 号房 = Raj 610 + Nagendra 610）。这正是 `agreement_parties` 模型要表达的场景。
- 数据是脏的，需归一：`#REF!` 作姓名、大小写变体（`ABDULRAHMAN ELFEKY` / `Abdulrhman elfeky`）、
  续行姓名（`, PHAM TRUNG HIEU (Harvey`）、中文备注混入姓名列、`空`/`和同屋一起付` 等占位、
  部分行 应收/实收 列错位。

- 应用侧读法（已读码确认，决定要写哪些表）：
  - **计租走 `tenants` 表**：`ensureMonthlyObligations`（`obligations.go:177-200`）读
    `tenant.MonthlyRentCents` / `DueDay` / `rent_start_date` / `rent_end_date` / `currency` / `status`。
  - **列表与详情读 `tenants` 的冗余字段**：`room_label`、`room_address`、`property_hint`
    （`obligations.go:414-415`、`dunning.go:228-229`、`matching_service.go:179-202`）。
  - **房间视角（`/rent-dashboard` 的房间树、房间详情）读 `properties` + `rooms` 关系模型。**

## Requirements

- **R1 关系完整**：写入 `properties` / `rooms` / `tenants` / `tenancy_agreements` / `agreement_parties`，
  且 `tenants` 的冗余字段（`room_label` / `room_address` / `property_hint`）与关系模型一致。
- **R2 合租正确**：一个房间同月多人时，每人在 `tenants` 有独立行（各自 `monthly_rent_cents`），
  且同属一份 `tenancy_agreements` 的 `agreement_parties`，`responsibility_cents` 等于各自份额。
- **R3 时间跨度**：租约与租客的 `rent_start_date` / `rent_end_date` 落在 2026-05 ~ 2026-12，
  使 `/rent-dashboard?period=` 切换月份能看到入住/退租带来的差异。
- **R4 货币与金额**：`currency='EUR'`，金额以分存储。月租取源表「应收」，合计与源表汇总行的偏差可解释。
- **R5 姓名归一但要可追溯**：同一人跨月的大小写/顺序变体合并为一条 `tenants` 行；
  非人名的占位值（`#REF!`、`空`、`和同屋一起付`）不得成为租客。
- **R6 幂等可重跑**：灌数据脚本可重复执行而不产生重复行（按自然键 upsert，或先清后灌并在脚本内说明）。
- **R7 不碰线上**：只写本机 `rentops`；不得指向 `:8081` / `bank.ddpl.top`。

## Acceptance Criteria

- [x] `properties` = 4，`rooms` = 25，地址与房间号与源表一致
- [x] `/properties`、`/rooms`、`/tenants`、`/tenancies` 四个列表页均有数据且可翻页/筛选
      —— 4 房产 / 25 房间 / 58 租客 / 40 租约
- [x] 至少一个房间呈现合租（同月 ≥2 名租客，且 `/rent-dashboard` 房间树里该房间人数与之一致）
      —— 2026-05 有 13 个房间 ≥2 人；房间树渲染 1 人 ×12 / 2 人 ×12 / 3 人 ×1
- [ ] **`/rent-dashboard` 切换 2026-05 ~ 2026-12 各月，应收合计不为 0 且随月份变化 —— 不通过。
      该验收项靠灌数据无法满足**：该路由在有库时渲染房间视角，其取义务的查询自带
      `rent_charge_id IS NOT NULL`（`rent_workspace.go:1118`），而没有任何生产代码写
      `rent_charges`。根因与处置见 `implement.md`「验收结果」与「处置决定」。
      本条验收项本身写得有误——它假定该页读义务表，实际读的是 charge。
- [x] `/tenants/{id}` 详情页显示房间、月租、租期，无「未填写」占位
      —— 房间 `2 · 72 Walkinstown Rd Dublin 12`、月租 `EUR 580.00`、租期 `2026-05-01 至 2026-12-31`
- [x] 每个 `(tenant, month)` 在 `rent_obligations` 中恰好 1 行（依赖 `09-19-rent-obligation-dedup` 先完成）
      —— 307 条义务，重复组 0
- [x] 重跑灌数据脚本，各表行数不变（幂等）

## Out of Scope

- 逐格还原源表，或复刻其汇总行的所有列（源表大量 `#REF!`，本身已损坏）。
- 把源表变成应用的一个导入功能（本任务是一次性灌数据，不是产品特性）。

> 银行流水与收款匹配（`payment_transactions` / `payment_allocations`）**在本任务范围内** ——
> 见 `Resolved Decisions` 第 3 条。本条原先写成 Out of Scope，与第 3 条冲突，已更正。

## Resolved Decisions

1. **币种 = EUR**（都柏林房产；源表未标币种，按地址推断）。
2. **全灌 8 个月（2026-05 ~ 2026-12）**，包含三个未来月份——用于演示未到期账单与
   「租约提前结束作废未来账单」流程（`voidFutureTenantObligations`）。
3. **顺带造银行流水**：按源表「实收金额」生成 `payment_transactions`，一部分自动匹配、
   一部分留未匹配，使「流水匹配」「一键匹配建议」「平账」都有实际操作对象。

## Dependencies

- **必须先完成 `09-19-rent-obligation-dedup`**。否则灌完数据后每次加载页面都会重复生成租金义务，
  验收项「每个 `(tenant, month)` 恰好 1 行」无法成立，且页面金额会被放大。
