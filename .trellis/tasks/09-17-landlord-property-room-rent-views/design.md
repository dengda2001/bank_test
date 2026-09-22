# 技术设计：房间入住与租金计划（方案 A）

## 1. 文档目的

本文是可直接交给实现 Agent 的技术契约。实现过程中若源码与本文冲突，以本文的产品决策为准；若遇到本文没有覆盖的技术细节，优先沿用仓库现有的 Go、GORM、MySQL 和服务端模板模式，不新增框架。

本次是未上线系统的破坏性重构：测试数据可以丢弃，不提供旧模型双读、数据回填或旧 URL 兼容。

## 2. 决策摘要

### 2.1 采用的方案

采用“前台无租约、后台有时间化房间收租计划”的方案：

```text
Property
  └─ Room
      └─ RoomRentPlan（按月生效的完整版本）
          └─ RoomRentPlanMember（每位租客的责任）
              └─ RentCharge（房间月应收事实）
                  └─ RentObligation（租客月责任事实）
                      └─ PaymentAllocation / CashReceipt / DunningAttempt
```

“租客”始终是人员档案。租客详情可以查看其责任，但不能编辑整个房间的计划。

### 2.2 不采用的方案

- 不保留 `TenancyAgreement` 仅隐藏 UI：内部术语会继续污染服务、错误和测试。
- 不把租金、缴租日、房间关系重新塞回 `tenants`：无法正确支持多人同住、代付和历史。
- 不在房间与租客两个入口同时维护计划：会出现双主写入口。
- 不把入住计划直接当账本：计划可变化，已经发生收款的月度事实必须稳定。

## 3. 分层和职责

### 3.1 配置层

`room_rent_plans` 和 `room_rent_plan_members` 描述“从某个月开始，这个房间应如何收租”。它们用于：

- 当前或未来月份预览；
- 生成月度账务事实；
- 展示入住与租金历史。

配置层不保存已收金额、欠款状态或付款来源。

### 3.2 月度事实层

`rent_charges` 和 `rent_obligations` 是账务事实：

- `rent_charges`：一个房间一个月份一条总应收；
- `rent_obligations`：一个租客在该房间月份的一条个人责任；
- 已发生付款或催收动作后，不允许通过修改计划重写对应月份。

### 3.3 支付与运营层

`payment_transactions` 表示实际到账流水，`payment_allocations` 表示流水覆盖了哪些责任。

- 实际付款人由流水和付款人识别关系解释；
- 责任人由 `rent_obligations.tenant_id` 决定；
- 两者不能合并成一个字段；
- 催收只看责任余额，不看谁实际付款。

### 3.4 展示层

- 首页：同一份月度事实的房产、房间、租客三个投影；
- 房间详情：计划的唯一写入口，同时展示月度事实；
- 租客详情：人员档案和责任的只读投影；
- 租客编辑：只编辑人员档案和付款识别。

## 4. 数据库设计

### 4.1 `properties`

保持当前职责。关键字段：

| 字段 | 类型 | 约束 | 说明 |
|---|---|---|---|
| `id` | bigint unsigned | PK | 房产 ID |
| `user_id` | bigint unsigned | NOT NULL | 数据所有者 |
| `name` | varchar(191) | NOT NULL | 房产名 |
| `timezone` | varchar(64) | NOT NULL | 默认 `Europe/Dublin` |
| `status` | varchar(32) | NOT NULL | `active` / `inactive` |

增加或保留唯一键 `(user_id, id)`，供下游组合外键校验所有权。

房产没有按月生效或失效的时间线。`status` 只是当前资料状态，不作为收租截止；计划命中、计划保存和事实生成不受房产当前状态影响，历史账务也不因状态改变而隐藏。

### 4.2 `rooms`

房间只保存物理资料，不保存当前租金。

| 字段 | 类型 | 约束 | 说明 |
|---|---|---|---|
| `id` | bigint unsigned | PK | 房间 ID |
| `user_id` | bigint unsigned | NOT NULL | 数据所有者 |
| `property_id` | bigint unsigned | NOT NULL | 所属房产 |
| `room_label` | varchar(191) | NOT NULL | 房间名称/编号 |
| `room_type` | varchar(64) | NULL | 房型 |
| `capacity` | int | NOT NULL | 提示性容量，本期不做强校验 |
| `notes` | text | NULL | 备注 |
| `status` | varchar(32) | NOT NULL | `active` / `inactive` |
| `rent_plan_version` | bigint unsigned | NOT NULL DEFAULT 0 | 计划时间线乐观锁 |

删除 `monthly_rent_cents`、`due_day`、`active_from`、`inactive_from`。租金、缴租日和月份边界只属于计划。

关键约束：

- `UNIQUE (user_id, property_id, room_label)`；
- `UNIQUE (user_id, id, property_id)`，供 charge 校验房间与房产的对应关系；
- 组合外键 `(user_id, property_id) -> properties(user_id, id)`。

房间没有自己的有效月份。`status` 只是当前资产管理状态，不作为收租截止；计划命中、计划保存/结束和事实生成不受房间当前状态影响。结束收租必须通过 rent plan 的结束操作；停用不会自动修改或隐藏计划与历史事实。状态只影响默认资产列表和房间物理资料编辑，不限制租金计划操作。

所属房产可在尚无 `rent_charges` 时更正；一旦该房间已有任何月份事实，所属房产关系不可变更。`rent_charges.property_id` 与房间的组合外键依赖此不变量，且更改归属会重写历史资产视角。房间资料更新若改变 `property_id`，必须先按统一协议锁 room，再检查是否已有 charge；有记录返回 `ErrRoomPropertyLocked`，映射为 `room_property_locked`（“房间已有租金记录，不能更改所属房产。”），禁止与并发事实生成穿透检查。房间名称等其他物理资料仍可编辑，历史账单继续使用快照。

### 4.3 `tenants`

租客只保存人员档案。

| 字段 | 类型 | 约束 | 说明 |
|---|---|---|---|
| `id` | bigint unsigned | PK | 租客 ID |
| `user_id` | bigint unsigned | NOT NULL | 数据所有者 |
| `name` | varchar(191) | NOT NULL | 正式名称 |
| `display_alias` | varchar(191) | NULL | 界面别名 |
| `email` | varchar(191) | NULL | 催收邮箱 |
| `status` | varchar(32) | NOT NULL | `active` / `inactive` |
| `created_at` / `updated_at` | timestamp | NOT NULL | 审计时间 |

删除以下旧字段：

```text
payer_id, payer_name_hint, monthly_rent_cents, currency,
interval_unit, interval_count, billing_start_date, due_day,
rent_start_date, rent_end_date, room_label, room_address, property_hint
```

付款识别只写 `tenant_payers`。

### 4.4 `room_rent_plans`

替代 `tenancy_agreements`。区间按月份、两端包含。

| 字段 | 类型 | 约束 | 说明 |
|---|---|---|---|
| `id` | bigint unsigned | PK | 计划 ID |
| `user_id` | bigint unsigned | NOT NULL | 数据所有者 |
| `room_id` | bigint unsigned | NOT NULL | 房间 |
| `effective_from_month` | date | NOT NULL | 生效月，必须为月初 |
| `effective_to_month` | date | NULL | 最后有效月，必须为月初 |
| `monthly_rent_cents` | bigint | NOT NULL | 房间月租，必须大于 0 |
| `currency` | char(3) | NOT NULL | 本期只允许 `EUR` |
| `due_day` | tinyint unsigned | NOT NULL | 1～31 |
| `created_at` / `updated_at` | timestamp | NOT NULL | 审计时间 |

索引和约束：

- `UNIQUE (user_id, room_id, effective_from_month)`；
- `UNIQUE (user_id, room_id, id)`，供 charge 校验计划与房间的对应关系；
- `INDEX (user_id, room_id, effective_from_month, effective_to_month)`；
- 组合外键 `(user_id, room_id) -> rooms(user_id, id)`；
- `effective_to_month IS NULL OR effective_to_month >= effective_from_month`；
- MySQL 无区间排斥约束，禁止重叠由事务服务保证。

不保留 `contract_date`、`move_in_date` 和 `status`。当前产品按整月处理，计划是否有效完全由区间决定。

### 4.5 `room_rent_plan_members`

替代 `agreement_parties`。成员生命周期和计划版本一致，因此不再保存 `joined_at` / `left_at` / `status`。

| 字段 | 类型 | 约束 | 说明 |
|---|---|---|---|
| `id` | bigint unsigned | PK | 成员版本 ID |
| `user_id` | bigint unsigned | NOT NULL | 数据所有者 |
| `room_rent_plan_id` | bigint unsigned | NOT NULL | 所属计划 |
| `tenant_id` | bigint unsigned | NOT NULL | 租客 |
| `responsibility_cents` | bigint | NOT NULL | 月度个人责任，必须大于 0 |
| `created_at` / `updated_at` | timestamp | NOT NULL | 审计时间 |

约束：

- `UNIQUE (user_id, room_rent_plan_id, tenant_id)`；
- `UNIQUE (user_id, id, room_rent_plan_id, tenant_id)`，供 obligation 组合外键校验来源；
- 组合外键分别约束计划与租客所有权；
- 责任合计等于计划月租由同一事务内的服务校验。

### 4.6 `rent_charges`

保留表名，改为引用 `room_rent_plan_id`。

```text
id, user_id, property_id, room_id, room_rent_plan_id,
period_month, due_date, expected_amount_cents, currency, record_status,
property_name_snapshot, room_label_snapshot, room_address_snapshot,
created_at, updated_at
```

关键约束：

- `UNIQUE (user_id, room_id, period_month)`；
- `UNIQUE (user_id, id, room_rent_plan_id, period_month)`，供 obligation 校验账期；
- `(user_id, room_id, room_rent_plan_id) -> room_rent_plans(user_id, room_id, id)`；
- `(user_id, room_id, property_id) -> rooms(user_id, id, property_id)`；
- `room_rent_plan_id NOT NULL`；
- `period_month` 为月初；
- `expected_amount_cents > 0`。

### 4.7 `rent_obligations`

保留现有技术名称，产品和 UI 统一称“租金责任”。

新增 `room_rent_plan_id NOT NULL`、`room_rent_plan_member_id NOT NULL`，并要求 `rent_charge_id NOT NULL`。保留：

```text
tenant_id, period_month, due_date, expected_amount_cents,
paid_amount_cents, currency, status, record_status, tenant_name_snapshot
```

关键约束：

- `UNIQUE (user_id, rent_charge_id, tenant_id)`；
- `UNIQUE (user_id, rent_charge_id, room_rent_plan_member_id)`；
- `(user_id, rent_charge_id, room_rent_plan_id, period_month) -> rent_charges(user_id, id, room_rent_plan_id, period_month)`；
- `(user_id, room_rent_plan_member_id, room_rent_plan_id, tenant_id) -> room_rent_plan_members(user_id, id, room_rent_plan_id, tenant_id)`；
- 不再允许 `rent_charge_id IS NULL` 的 legacy responsibility；
- `paid_amount_cents` 是可重算缓存，事实来源仍是有效分配与有效现金收款。

所有引用责任事实的审计表必须使用 `ON DELETE RESTRICT`，至少包括：

- `payment_allocations.rent_obligation_id`；
- `cash_receipts.rent_obligation_id`；
- `dunning_send_attempts.rent_obligation_id`。

当前旧迁移中的 `CASCADE` 必须在 014 中删除并重建为 `RESTRICT`。服务层仍需先做锁定检查；外键是防止绕过服务误删审计链的最后防线。

### 4.8 `cash_receipts` 付款人语义

旧 `cash_receipts.tenant_id` 同时被当作责任人和付款人，无法表达 A 现金替 B 付款。014 将其改名为可空的 `payer_tenant_id`：

```text
rent_obligation_id  NOT NULL  -> 被覆盖的责任，责任人从 obligation.tenant_id 读取
payer_tenant_id     NULL      -> 实际交现金的已知租客；不是系统租客时允许为空
payer_name_snapshot NULL      -> 非租客付款人或当时付款名称快照
```

`payer_tenant_id` 若非空，必须和 receipt 属于同一 `user_id`，但不要求等于 obligation 的责任人。现金收款的金额投影只按 `rent_obligation_id + status + currency` 汇总，不再用 payer 是否等于责任人过滤。

### 4.9 关系图

```text
users 1─N properties 1─N rooms 1─N room_rent_plans
users 1─N tenants                 1─N room_rent_plan_members N─1 tenants

room_rent_plans 1─N rent_charges (每月最多一条)
rent_charges 1─N rent_obligations (每位成员一条)
rent_obligations 1─N payment_allocations N─1 payment_transactions
rent_obligations 1─N cash_receipts
rent_obligations 1─N dunning_send_attempts
```

### 4.10 外键删除策略与所有权索引

014 必须补齐每个组合外键所依赖的唯一键：

```text
properties       UNIQUE (user_id, id)
rooms            UNIQUE (user_id, id)
                 UNIQUE (user_id, id, property_id)
tenants          UNIQUE (user_id, id)
room_rent_plans  UNIQUE (user_id, id)
                 UNIQUE (user_id, room_id, id)
plan_members     UNIQUE (user_id, id)
                 UNIQUE (user_id, id, room_rent_plan_id, tenant_id)
rent_charges     UNIQUE (user_id, id)
                 UNIQUE (user_id, id, room_rent_plan_id, period_month)
rent_obligations UNIQUE (user_id, id)
```

删除策略固定为：

| 父 -> 子 | 删除策略 | 原因 |
|---|---|---|
| property -> room | RESTRICT | 房产停用走状态，不级联删除房间 |
| room -> plan | RESTRICT | 房间状态变化不应删除计划历史 |
| plan -> plan member | CASCADE | 尚未产生事实的未来完整版本可整体替换 |
| plan -> charge | RESTRICT | 已物化月度事实必须先通过服务校验 |
| plan member -> obligation | RESTRICT | 责任事实不可随配置静默消失 |
| charge -> obligation | RESTRICT | 月度事实只能由受控重建事务删除 |
| obligation -> allocation / cash / dunning | RESTRICT | 任何审计记录都锁定责任 |

所有业务表仍保留独立 `user_id`，不能只依赖跨表 JOIN 推导所有者。

`rent_obligations.room_rent_plan_id` 是有意保留的来源冗余字段，用于让数据库同时证明 charge 与 member 来自同一个计划。charge 也通过组合外键保证计划属于同一个 room，property_id 与 room 所属房产一致，责任月份与 charge 账期一致。生成事实时一次写入，业务代码不得单独修改它。

## 5. 计划时间线语义

### 5.1 命中规则

月份 `M` 命中计划的条件是：

```text
effective_from_month <= M
AND (effective_to_month IS NULL OR effective_to_month >= M)
```

任何房间月份命中 0 条计划表示空置；命中 1 条为正常；命中多条是数据错误并显示“待核对”。

### 5.2 “从某月起保存”的确定语义

`SaveRoomRentPlan(effective_month=M)` 表示“从 M 月起采用这套完整安排”：

1. 保留 M 之前的历史。
2. 将覆盖 M 的旧计划截止到 `M - 1 month`。
3. 删除所有 `effective_from_month >= M` 的未来计划及其成员。
4. 插入从 M 开始、无结束月份的新计划和完整成员集合。
5. UI 若检测到未来版本，应提示“会替换 M 月及之后的安排”。

这是一条 replace-from 操作，不是对某个成员打补丁，能够避免残留成员和重叠区间。

同一租客在同一个月份只能属于一个房间计划。保存计划时，事务必须在校验成员归属后锁定涉及的租客行，再检查这些租客从生效月份起是否命中其他房间的计划；发现重叠返回 `ErrTenantRoomMonthConflict`，页面提示“该租客从所选月份起已在其他房间入住，请先结束原房间的入住计划。”历史房间计划在目标月份前已经结束的不算冲突。`rooms.capacity` 只做界面提醒，也不阻断保存。

### 5.3 “从某月起结束入住”的确定语义

`EndRoomRentPlan(vacant_from_month=M)` 表示 M 月开始空置：

1. 覆盖 M 的计划截止到 `M - 1 month`；
2. 删除从 M 起的未来计划；
3. 删除 M 起尚未锁定的月度事实；
4. 若 M 起存在已锁定事实，整个操作失败。

### 5.4 并发控制

房间行的 `rent_plan_version` 是计划时间线版本。读取抽屉时返回该值，提交时携带 `expected_timeline_version`。事务中：

1. `SELECT rooms ... FOR UPDATE`；
2. 比较版本，不一致返回 `ErrStaleRentPlanTimeline`；
3. 按 `tenant_id` 升序锁定本次成员对应的租客行；
4. 检查这些租客从生效月份起是否命中其他房间计划，冲突返回 `ErrTenantRoomMonthConflict`；
5. 完成时间线变更和事实重建；
6. `rent_plan_version = rent_plan_version + 1`。

这样可以防止两个浏览器标签页互相覆盖。

### 5.5 与财务动作的统一锁顺序

所有可能创建、删除或引用房间责任的事务都必须经过同一房间锁。统一顺序为：

```text
payment_transaction（仅银行分配需要）
-> rooms（多房间时按 room_id 升序）
-> rent_charges / rent_obligations（按 id 升序）
-> allocation / cash receipt / dunning attempt
```

- 计划保存、结束、事实生成继续先锁 `rooms`；
- 银行分配可先锁自己的 transaction，但在锁 obligation 前必须解析并按升序锁定相关 rooms，然后重新读取 obligation；
- 现金收款和催收尝试在锁定或写入 obligation 审计记录前先锁对应 room；催收请求键必须在持锁事务内再次检查，并由唯一约束兜底；
- 催收必须在持有 room 锁的短事务内预留 attempt，提交后才执行外部发送；预留记录本身立即锁定该月份；
- 获取 room 锁后必须重新读取责任并验证其仍存在、仍属于原 room/period，不能沿用锁前快照。

因此竞争结果只有两种：财务动作先提交，计划修改随后返回 `ErrRentPlanFactsLocked`；或计划修改先提交，财务动作重新读取后使用新责任或返回可重试的 stale/not-found 错误。外键冲突和死锁需转换为稳定冲突错误，不能直接暴露 SQL 错误。

## 6. 写入服务契约

### 6.1 类型

```go
type RoomRentPlanMemberInput struct {
    TenantID            uint64
    ResponsibilityCents int64
}

type SaveRoomRentPlanCommand struct {
    UserID                  uint64
    RoomID                  uint64
    EffectiveMonth          time.Time
    MonthlyRentCents        int64
    Currency                string
    DueDay                  int
    Members                 []RoomRentPlanMemberInput
    ExpectedTimelineVersion uint64
}

type EndRoomRentPlanCommand struct {
    UserID                  uint64
    RoomID                  uint64
    VacantFromMonth         time.Time
    ExpectedTimelineVersion uint64
}
```

实现名称遵循仓库现有非导出风格时可使用小写类型，但字段语义不得变化。

### 6.2 校验

边界校验顺序固定：

1. `user_id`、`room_id` 非零；
2. 生效月份规范化为月初，且不得早于 Dublin 当前月份；
3. 月租大于 0、币种为 EUR、缴租日在 1～31；
4. 至少一个成员，租客 ID 不重复，责任均大于 0；
5. 责任合计严格等于房间月租；
6. 房间和所有租客属于同一 `user_id`；
7. 所有新增成员均为 active；
8. 时间线版本一致；
9. 受影响月份没有锁定事实。

`SaveRoomRentPlan` 与 `EndRoomRentPlan` 都只要求 room 属于当前 user 且时间线版本匹配；room 或 property 的 `status` 不参与计划写入判断。停用资产仍可维护收租计划，避免当前状态被误作计划有效期。

“均分”只是一种界面快捷方式：由服务端按 tenant ID 升序分配余数分币，最终仍保存明确金额。全部责任留空时允许均分；只填写一部分时拒绝。

### 6.3 锁定事实

`rent_charge` 本身不算锁定，因为它可以安全重建。下列任何记录曾关联到某责任，即视为该月份已发生业务动作，不允许改写：

- 任意 `payment_allocations`，包括后来作废的分配；
- 任意 `cash_receipts`，包括后来作废的收据；
- 任意 `dunning_send_attempts`，无论发送成功还是失败。

原因是这些记录都引用或快照了当时的责任，删除重建会破坏审计链。

### 6.4 错误类型

```text
ErrInvalidRentPlan          输入字段或责任合计无效
ErrRentPlanTimelineConflict 数据库已有重叠计划
ErrRentPlanFactsLocked      受影响月份已有业务动作
ErrStaleRentPlanTimeline    页面基于过期版本提交
gorm.ErrRecordNotFound      房间或租客不属于当前用户
```

Handler 只把这些稳定错误映射为界面错误码，不向用户输出数据库错误。

## 7. 月度事实生成

### 7.1 生成策略

保留现有 `ensureMonthlyRentFacts` 策略：

- 当前月和历史月读取时可以幂等补齐事实；
- 未来月普通读取只计算预览，不写数据库；
- 明确对未来月份执行付款时，允许先物化该月份事实。

删除所有 legacy tenant fallback。

### 7.2 单房间生成事务

对房间月份 `(user_id, room_id, period_month)`：

1. 锁定房间；
2. 查询唯一有效计划；
3. 查询计划成员并校验责任合计；
4. `INSERT ... ON DUPLICATE KEY` 或先查后建一条 `rent_charge`；
5. 为每位成员建立一条 `rent_obligation`；
6. 复制房产、房间、租客名称快照；
7. 校验已有事实与计划一致，不一致则返回冲突，不能静默覆盖；
8. 同一事务提交。

事实生成、计划替换和显式未来付款都使用第 5.5 节的 room 锁。未来付款的“物化事实 + 创建分配/现金收款”必须处于同一业务事务；若调用链不能共用事务，则物化提交后必须再次取得 room 锁并重新读取 obligation，禁止拿物化前后的裸 ID 继续写。

到期日使用 `dueDateForMonth(period, due_day)`；若月份没有对应日期，例如 2 月 31 日，则使用该月最后一天。

### 7.3 调整当前月

若当前月已生成 `rent_charge` 但没有任何锁定事实，保存计划时允许：

1. 删除该月 `rent_obligations`；
2. 删除该月 `rent_charge`；
3. 保存新计划；
4. 在同一事务内按新计划重建当前月事实。

任一步失败则整体回滚。

## 8. 页面与路由设计

### 8.1 导航

保持：

```text
月度总览 / 流水匹配 / 催收任务
房产管理 / 房间管理 / 租客管理
银行设置
```

不得增加“租约”入口。

### 8.2 房间列表与新建

房间列表继续按所选月份展示应收、已收、未收和状态。

新建房间抽屉只创建物理房间，按钮为：

- 主按钮：`保存并设置入住与租金`；成功后跳转 `/rooms/{id}?period=YYYY-MM&rent=1`；
- 次按钮：`仅保存空房间`；成功后回房间列表或房产详情。

不在同一个 POST 中混合创建房间和计划，避免部分失败语义；第一步创建成功、第二步未完成时，结果就是合法空置房。

### 8.3 房间详情

新增“当前入住与租金”卡片，显示所选月份命中的计划：

- 生效区间；
- 月租与缴租日；
- 租客及个人责任；
- 责任合计；
- `调整入住与租金`；
- `从某月起结束入住`。

没有计划时显示空状态和 `设置入住与租金`。

原“编辑房间”仅保留房间名称、房产、房型、容量和备注。月租、缴租日从该表单移除。

### 8.4 入住与租金抽屉

打开方式：

```text
GET /rooms/{roomID}?period=YYYY-MM&rent=1
```

提交：

```text
POST /rooms/{roomID}/rent-plans
```

表单字段：

```text
effective_month
monthly_rent
currency=EUR
due_day
tenant_ids=<repeatable>
responsibility_<tenantID>=<decimal amount>
expected_timeline_version
return_to
```

全部责任金额留空表示均分；部分留空返回校验错误。

结束：

```text
POST /rooms/{roomID}/rent-plans/end
vacant_from_month
expected_timeline_version
return_to
```

服务端必须验证 `return_to` 为站内允许路径；不能直接信任表单 URL。

建议错误码：

| query error | 文案 |
|---|---|
| `rent_plan_invalid` | 请检查月份、月租、缴租日和责任合计。 |
| `rent_plan_locked` | 受影响月份已有收款或催收记录，不能改写。 |
| `rent_plan_stale` | 入住与租金已在其他页面更新，请刷新后重试。 |
| `rent_plan_conflict` | 收租计划时间线异常，请先处理重叠记录。 |

### 8.5 租客新增与编辑

只保留：姓名、别名、邮箱、付款人编号和付款人名称提示。

移除：房间选择、生效月份、月租、缴租日、责任计划隐藏 JSON 及相关 JavaScript。提交只调用 tenant profile service 和 tenant payer service。

### 8.6 租客详情

路由：

```text
GET /tenants/{tenantID}?period=YYYY-MM
```

页面顶部仍是租客身份，不改成“租约详情”。新增/调整“所选月份租金责任”区：

| 房产 / 房间 | 个人责任 | 已覆盖 | 未付 | 付款来源 | 状态 |
|---|---:|---:|---:|---|---|

同一租客在同月最多有一条房间责任。每行提供“查看房间”；页面提供“前往房间调整入住与租金”，不在租客页直接写计划。

历史列表按 `tenant × period` 展示，并带出该月份唯一所属房间；不能把不同月份或不同责任错误合并。

### 8.7 首页租客视角

保留：

```text
GET /rent-dashboard?period=YYYY-MM&view=tenants&status=outstanding
```

一行一条 `rent_obligation`，字段保持：租客、房产/房间、个人责任、已覆盖、未付、收缴率、付款来源、状态、操作。

状态筛选调整为：

| 值 | 含义 |
|---|---|
| `all` | 全部可展示责任 |
| `outstanding` | 有效且 `balance_cents > 0`，不含 `needs_review` |
| `needs_review` | 数据冲突或无法可靠核算 |
| `overdue` | 到期日已过且余额大于 0 |
| `partial` | 未逾期且 `0 < paid < expected` |
| `open` | 未逾期且 `paid = 0` |
| `paid` | `paid >= expected` |
| `forecast` | 未来月份的计划预览 |

单条责任状态优先级：

```text
needs_review -> forecast（未来预览） -> paid -> overdue -> partial -> open
```

`vacant` 只属于零责任的房间/房产聚合行，不属于 tenant responsibility row。

状态算法先判断是否结清，再判断是否逾期：部分付款但已经超过到期日时显示“已逾期”，已覆盖金额仍照常展示。

顶部文案使用“所选月份”，不使用可能误导历史查询的“本月”。“需继续跟进”改为“未结清责任”，计数只包括有效且余额大于 0 的租客责任。

“待核对”必须单独统计，不能混进未结清。

### 8.8 删除旧入口

删除：

- `/tenancies` 路由注册；
- `tenancy_pages.go`；
- `web/templates/pages/tenancies.html`；
- 对应 CSS、页面类型和测试；
- `workspaceShell` 中的 `tenancies` 标题映射；
- 所有“租约状态”“租约金额”“按租约金额计算”等用户文案。

由于没有线上兼容要求，GET 和 POST 均返回 404，不保留重定向。

## 9. 读取模型

### 9.1 唯一事实源

当前月和历史月：

```text
rent_charges + rent_obligations + effective allocations/receipts
```

未来月：

```text
room_rent_plans + room_rent_plan_members（只读预览）
```

不得从 `rooms` 或 `tenants` 回退读取月租、缴租日、房间文本。

### 9.2 首页三视角

先生成 canonical responsibility rows，再把它左连接到资产骨架：

```text
properties + rooms                         -> asset spine
obligation/forecast + effective payments   -> responsibility rows

tenant view   = responsibility rows，不聚合
room view     = asset spine LEFT JOIN responsibility grouped by room
property view = properties LEFT JOIN room aggregates grouped by property
```

没有责任行的房间仍生成一条 `vacant` 房间行并计入房间数量，汇总计算中的 expected/paid/balance 均为 0，页面金额显示 `—`，且不计入已交满或未交满。没有责任行时不得伪造 tenant row。房产没有房间或其房间全部空置时仍可出现在房产视角。

当前/历史月份的责任行来自 facts，未来月份来自 forecast。资产骨架包含当前 active 的房产/房间，以及所选月份有 facts、命中 rent plan 或有 forecast 的 inactive 房产/房间；任何被纳入房间的所属房产也必须纳入。先按这个候选范围生成当前/历史缺失 facts，再构造资产行，避免 inactive 资产因尚未物化而消失。资产当前 `status` 只控制默认资产列表与房产/房间物理资料编辑，不用于改变计划、责任或历史金额。文本和状态筛选只作用于最终列表，不改变顶部所选范围汇总；分页也不改变汇总。房产/房间 scope 会改变汇总。

### 9.3 租客详情查询

租客详情查询从 `rent_obligations` 出发，连接 charge 获取房间和房产快照；当前对象名称用于导航，账务行名称使用快照。这样房间后来改名也不会改写历史账单。

## 10. 状态与金额算法

对有效责任：

```text
paid = 有效租金分配（银行分配及人工平账分配） + 有效现金收款
balance = max(expected - paid, 0)

if paid >= expected: paid
else if DublinToday > dueDate: overdue
else if paid > 0: partial
else: open
```

房间和房产聚合状态固定为：任一子项 `needs_review` 则为 `needs_review`；零责任为 `vacant`；未来计划预览为 `forecast`；否则任一未结清责任已逾期为 `overdue`；全部责任结清为 `paid`；有任意覆盖金额为 `partial`；其余为 `open`。`outstanding` 只匹配当前/历史有效责任的 `balance > 0`，不包含 `forecast`、`vacant` 或 `needs_review`。

超出责任的金额不得让 `paid_amount_cents` 超过 expected；多余部分保持流水未分配余额。

房间应收不变量：

```text
rent_charge.expected_amount_cents
  == SUM(active rent_obligations.expected_amount_cents)
  == source room_rent_plan.monthly_rent_cents
```

## 11. 数据库变更策略

### 11.1 迁移方式

新增 `014_room_rent_plan_reset.sql`，作为明确的破坏性切换：

1. 重命名 `tenancy_agreements` 为 `room_rent_plans`；
2. 重命名 `agreement_parties` 为 `room_rent_plan_members`；
3. 重命名相应列和外键；
4. 删除旧计划的 `contract_date`、`move_in_date` 和 `status`，删除旧成员的独立起止时间与 `status`；保留房产、房间和租客的当前 `status`；
5. 增加 `rooms.rent_plan_version`；
6. 删除 properties 的 `inactive_from`，删除 rooms 的租金缓存及 `active_from` / `inactive_from`；房产和房间不保留月份有效期；
7. 删除 tenants 的旧租金/房间/周期字段以及 `payer_id` / `payer_name_hint` 镜像；付款识别只保留在 `tenant_payers`；
8. 将 `rent_obligations.rent_charge_id`、`room_rent_plan_id` 和 `room_rent_plan_member_id` 改为 NOT NULL，并增加组合外键，校验 charge 与 member 属于同一 plan 且 tenant 一致；
9. 删除 legacy obligation 的生成列和唯一键；
10. 将 payment allocation、cash receipt、dunning attempt 到 obligation 的删除行为从 `CASCADE` 改为 `RESTRICT`；
11. 重建所需组合所有权约束与索引。

不要自动删除用户库里的数据。执行前由操作者显式删除并重建开发/测试数据库；若除 `schema_migrations` 外任一业务表非空，迁移或启动前检查应报错并要求重建，不能只检查计划表，也不能悄悄转换测试数据。

### 11.2 回滚

本次没有数据级回滚。代码回滚必须配合重新建库，不能只回退二进制。实施说明和 README 必须明确这一点。

## 12. 安全性与一致性

- 所有仓储方法第一个过滤条件包含 `user_id`。
- 从 URL 取得的 room/tenant/plan ID 必须再次用 `user_id` 查询。
- 金额只使用整数分，不使用浮点数进入领域层。
- 表单 decimal 只在边界转换为 cents。
- 计划保存、事实删除/重建和 timeline version 增长必须在一个事务中。
- 生成事实和分配付款时使用行锁与唯一键双重防重。
- 不将数据库错误、SQL 或内部 ID 枚举泄露给页面。
- 所有 POST 延续现有认证要求；`return_to` 必须白名单校验。

## 13. 关键场景合同

### 13.1 一人入住

```text
房间月租 1000
A 责任 1000
=> charge 1000, obligation(A) 1000
```

### 13.2 两人不等分

```text
房间月租 1200
A 责任 700
B 责任 500
=> 合法
```

责任为 700 + 400 时保存失败，不能自动补齐。

### 13.3 余数均分

```text
1000.01 / 2 人
按 tenant_id 升序：500.01 + 500.00
```

### 13.4 同住人代付

```text
A 责任 700，B 责任 500
A 实际支付 1200，并分别分配 700 / 500
=> A paid，B paid + 他人代付，未结清筛选不返回任何人
```

### 13.5 部分付款已逾期

```text
A 责任 700，已覆盖 200，到期日已过
=> balance 500，status overdue，进入 outstanding 和 overdue
```

### 13.6 当前月调整

```text
当前月事实已生成，但没有 allocation / receipt / dunning
=> 允许保存新计划并原子重建
```

### 13.7 已锁定月份

```text
当前月已经发送催收，后来付款被撤销
=> 仍视为锁定，不允许通过计划编辑改写责任
```

### 13.8 同一租客同月冲突

```text
A 已在 Room-1 的 2026-09 计划中
=> 保存 Room-2 从 2026-09 起的计划时返回 ErrTenantRoomMonthConflict

A 在 Room-1 的计划于 2026-08 结束
=> 保存 Room-2 从 2026-09 起的计划允许通过
```

## 14. 可观测性

计划变更至少记录结构化日志：

```text
user_id, room_id, effective_month/vacant_from_month,
old_timeline_version, new_timeline_version, member_count,
monthly_rent_cents, result, error_code
```

日志不得记录邮箱、银行付款人名称或完整表单。

建议增加只读一致性检查函数供测试和诊断复用：

- 每个房间月份最多一个计划；
- 计划责任合计等于月租；
- charge 等于 obligation 合计；
- obligation paid cache 等于有效收款合计。

## 15. 测试策略

### 15.1 纯单元测试

- 表单金额、月份和成员解析；
- 均分及余数；
- 责任合计与重复租客；
- 状态算法和 Dublin 日期边界；
- 首页 `outstanding` 过滤；
- URL/return context；
- 页面术语禁止列表。

### 15.2 MySQL 集成测试

- 空库执行全部迁移；
- 计划 replace-from 时间线；
- timeline version 并发冲突；
- 当前月无业务动作时重建；
- allocation、cash 或 dunning 任一存在时拒绝重建；
- 月度事实并发幂等；
- charge/plan 不一致和 member/plan/tenant 不一致的 obligation 被组合外键拒绝；
- 计划修改与 allocation、cash、dunning 并发时遵循统一 room 锁并返回稳定冲突；
- 跨用户 ID 注入失败；
- 一人代付多人后的责任状态；
- 保存计划时同一租客同月跨房间重叠必须拒绝；历史上在目标月份前结束的计划不构成冲突。

### 15.3 模板与页面测试

- 导航和页面不出现“租约管理”；
- `/tenancies` 不注册；
- 房间详情桌面和移动端都能打开入住与租金抽屉；
- 租客表单无房间、月租和责任编辑字段；
- 租客详情显示所选月份唯一责任及房间链接；
- 首页租客视角和未结清筛选保留月份上下文。

### 15.4 全链路验收数据

最小验收必须包含：

1. 两套房产；
2. 一间空置房；
3. 一间两人同住且 700/500 不等分的房间；
4. 一笔由 A 支付并同时覆盖 A、B 的 1200 收款；
5. 一条部分付款且逾期的责任；
6. 一条待核对异常；
7. 当前月、历史月和未来月各一次查询。

## 16. 完成定义

- 数据库最终结构只保留新计划模型和非 legacy 责任事实。
- 运行时代码中不再出现 `tenancyAgreement`、`agreementParty` 或旧表名。
- 页面中不再出现“租约管理”“新建租约”“租期”“按租约金额计算”。
- 房间详情能完整维护计划；租客页只读责任。
- 首页可按所选月份、租客视角和未结清状态找到所有欠款责任。
- 定向测试、全量 Go 测试、MySQL 集成测试及桌面/移动审计通过。
