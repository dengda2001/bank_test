# 数据结构与数据库迁移：技术设计

## 1. 目标与边界

本任务只建立多房源租金账务所需的数据结构和迁移，不实现初始化界面、DAO、月度账单生成、付款分配或 Dashboard。

当前 MVP 的数据由系统外部准备。新结构需要支持未来多房产、多房间、多人同住和个人责任，但不能破坏现有租客、月度责任、付款分配、现金收款和支出数据。

## 2. 领域关系

```text
Property 1:N Room
Room 1:N TenancyAgreement
TenancyAgreement 1:N AgreementParty N:1 Tenant
RentCharge 1:N RentObligation
PaymentTransaction 1:N PaymentAllocation N:1 RentObligation
ManualExpense N:1 Property / optional N:1 Room
```

`TenancyAgreement` 是持续一段时间的租约规则；`RentCharge` 是某个租金月份生成的房间总应收；现有 `rent_obligations` 扩展为 `RentObligation`，表示该房间账单下某个租客的个人责任。

## 3. 新增表

### `properties`

- `id bigint unsigned primary key`
- `user_id bigint unsigned not null`
- `name varchar(191) not null`
- `address text null`
- `status varchar(32) not null default 'active'`
- `created_at`, `updated_at`
- 索引：`(user_id, status)`

房产名称不是业务主键，不对名称做唯一约束。

### `rooms`

- `id bigint unsigned primary key`
- `user_id bigint unsigned not null`
- `property_id bigint unsigned not null`
- `room_label varchar(191) not null`
- `status varchar(32) not null default 'active'`
- `active_from date not null`
- `inactive_from date null`
- `created_at`, `updated_at`
- 唯一键：`(user_id, property_id, room_label)`
- 索引：`(user_id, property_id, status)`

房间是否在所选月份有效由日期和状态共同判断；房间被停用时不删除历史账务。

### `tenancy_agreements`

- `id bigint unsigned primary key`
- `user_id bigint unsigned not null`
- `room_id bigint unsigned not null`
- `start_date date not null`
- `end_date date null`
- `monthly_rent_cents bigint not null`
- `currency char(3) not null default 'EUR'`
- `due_day int not null`
- `status varchar(32) not null default 'active'`
- `created_at`, `updated_at`
- 索引：`(user_id, room_id, start_date, end_date)`

同一房间有效租约不能重叠；MySQL migration 只提供查询索引，重叠校验留给后续服务层事务。

### `agreement_parties`

- `id bigint unsigned primary key`
- `user_id bigint unsigned not null`
- `agreement_id bigint unsigned not null`
- `tenant_id bigint unsigned not null`
- `responsibility_cents bigint not null`
- `joined_at date null`
- `left_at date null`
- `status varchar(32) not null default 'active'`
- `created_at`, `updated_at`
- 唯一键：`(user_id, agreement_id, tenant_id)`
- 索引：`(user_id, agreement_id, status)`

MVP 责任按整月计算，`responsibility_cents` 是生成月度个人责任时使用的明确金额；日期字段为后续成员变化保留，不在本任务启用按天折算。

### `rent_charges`

- `id bigint unsigned primary key`
- `user_id bigint unsigned not null`
- `property_id bigint unsigned not null`
- `room_id bigint unsigned not null`
- `tenancy_agreement_id bigint unsigned not null`
- `period_month date not null`（统一为每月第一天）
- `due_date date not null`
- `expected_amount_cents bigint not null`
- `currency char(3) not null default 'EUR'`
- `record_status varchar(32) not null default 'active'`
- `property_name_snapshot varchar(191) null`
- `room_label_snapshot varchar(191) null`
- `room_address_snapshot text null`
- `created_at`, `updated_at`
- 唯一键：`(user_id, room_id, period_month)`
- 索引：`(user_id, property_id, period_month, record_status)`、`(user_id, room_id, period_month)`

这是房间月度总应收事实，不记录某个租客的付款状态。

## 4. 旧表扩展

### `rent_obligations`

保留原表和原始记录，新增：

- `rent_charge_id bigint unsigned null`：新账务必填，旧历史记录允许为空
- `tenant_name_snapshot varchar(191) null`

保留现有 `expected_amount_cents`、`paid_amount_cents`、`period_month`、状态和作废审计字段。删除旧唯一键 `(user_id, tenant_id, period_month)`，新增 `(user_id, rent_charge_id, tenant_id)`；新模型因此允许同一租客同月在不同房间产生不同责任。

旧记录不自动重建、不自动归类；新账务启用前由兼容层决定如何读取没有 `rent_charge_id` 的历史责任。

### `manual_expenses`

新增：

- `property_id bigint unsigned null`
- `room_id bigint unsigned null`
- `record_status varchar(32) not null default 'active'`
- `voided_at`, `voided_by_user_id`, `void_reason`

旧行允许 `property_id` 为空。新写入必须由后续服务层提供稳定房产归属；`room_id` 为空表示房产公共支出。

### `cash_receipts`

新增可空 `payment_transaction_id`，为后续统一银行与现金付款分配保留关联；现有 `tenant_id` 和 `rent_obligation_id` 暂时保留，历史现金记录不在本任务中重写。

## 5. 外键与删除策略

- 所有新增表均关联 `users`，并对 Property、Room、Agreement、Party、Charge、Tenant 建立外键。
- 账务事实使用 `RESTRICT` 或由服务层禁止硬删除；房间、租约和租客采用状态/结束日期表达生命周期。
- 所有权一致性（例如 `room.user_id` 与 `property.user_id` 一致）沿用项目现有约定，由服务查询显式带 `user_id` 校验；不依赖单列外键自动解决跨用户关系。

## 6. 不纳入本次迁移

- 初始化向导、工作簿导入和旧文本自动合并。
- 其他收入专用表或房产归属规则。
- 发票附件表和文件存储。
- 跨房间付款操作。
