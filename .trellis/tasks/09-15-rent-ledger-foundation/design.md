# 收租账本基础与账单生命周期：技术设计

## 边界

本子任务只负责数据库兼容迁移、账本领域模型／不变量和可复用读取契约，不实现租客、银行匹配、现金表单、Dashboard 或邮件页面。当前阶段固定 EUR，但保留数据库 `currency char(3)`、服务层 currency 参数和比较边界，未来可在不重做账本结构的情况下扩展。

## 现状与问题

- `rent_obligations` 以 `(user_id, tenant_id, period_month)` 唯一表示账单，并把 `paid_amount_cents` 作为读优化总额。
- `payment_transactions` 保存不可变银行原始流水。
- `payment_allocations` 当前以 `(user_id, payment_transaction_id, rent_obligation_id)` 唯一，无法表达同一账单的多次／多用途分配，也没有可逆审计字段。
- 现有 GORM 服务使用事务，但金额检查和重复保护还依赖整笔流水；后续子任务必须迁移到本设计提供的分配契约。

## 数据模型与迁移

新增一个版本化 SQL migration（沿用 `db.go` 的文件排序和事务执行）：

- `payment_allocations`：增加 `allocation_kind`（`rent`／`deposit`／`other_income`，默认 `rent`）、`operation_id`、`idempotency_key`、`voided_at`、`voided_by_user_id`、`void_reason`；删除旧的 transaction-obligation 唯一键，增加用户／来源／状态索引和非空幂等键唯一索引（NULL 允许旧数据）。
- `rent_obligations`：增加 `record_status`（`active`／`voided`，默认 `active`）、`voided_at`、`voided_by_user_id`、`void_reason`。保留现有 `status` 作为缴费进度，以免把作废和缴费状态混为一谈。
- 为新增外键使用当前用户范围的外键约束；迁移先增加列、回填默认值和索引，再删除冲突唯一键，确保旧分配仍可读取。

旧分配全部视为 `allocation_kind=rent`、`status=confirmed`、`record_status=active`。迁移不重写金额、不创建重复分配、不物理删除记录。迁移失败由现有 migration transaction 回滚。

## 领域契约

新增共享常量／校验函数，作为后续服务的唯一入口：

- `normalizeLedgerCurrency` 只接受非空 `EUR`（大小写归一化为 `EUR`）；非 EUR 返回可展示的拒绝错误。数据模型保留原始字段，未来扩展只需替换策略，不改金额单位。
- `allocation_kind` 和 `allocation.status` 使用显式常量；只有 `status=confirmed` 且未被撤销的 `rent` 分配进入账单实收，`deposit`／`other_income` 不能改变账单 `paid_amount_cents`。
- `validateAllocationBudget` 校验正金额、来源币种／账单币种为 EUR 且相同、来源累计有效分配加本次不超过原金额、房租加本次不超过账单未收、租客和用户边界一致。
- `recomputeObligationPaid` 从有效 rent allocation 重新计算 `paid_amount_cents`，再用统一的 Dublin 日期函数重算缴费进度；撤销／作废调用同一投影，不做手工减法。
- `operation_id` 标识一次原子分配／撤销操作，`idempotency_key` 标识可重试请求；重复 key 返回原结果或 no-op，不产生第二次金额变更。

## 事务与并发

所有写操作在一个数据库事务中：锁定来源流水和目标账单（或使用等价条件更新），读取有效余额，校验预算，插入分配／审计记录，再重算账单投影。任何校验失败整次回滚。数据库约束是最后防线，错误转换为稳定领域错误；不能通过刷新或重试绕过余额检查。

读模型以有效记录为条件：`allocation.status='confirmed'`、`rent_obligations.record_status='active'`；作废／撤销记录仍可按操作 ID 查询但不进入金额、人数或催缴候选。按账单所属月和银行到账月分别过滤，避免把到账月份当作租金月份。

## 时间与隔离

账单月和应缴日以日期值保存；逾期计算统一使用 `time.LoadLocation("Europe/Dublin")` 的本地日期，不能在各 handler 中自行调用 UTC。每一个查询和更新都带 `user_id`，外键和路径参数不能跨用户解析。

## 兼容、回滚与风险

- 先在含旧数据的副本执行 migration，并核对旧分配数量、金额和 `paid_amount_cents`；新代码发布前旧代码仍能读取新增列的默认值。
- 迁移为新增列／索引和可回滚的列删除设计；生产回滚优先回退应用版本，禁止直接删除已有审计记录。
- MySQL 是运行数据库；没有把 SQLite 引入生产。纯校验和投影用单元测试，真实事务／migration 用可控 MySQL 集成测试或明确的 SQL 检查脚本。

## 跨层数据流

`银行/现金来源 → 事务内账本写入 → 有效分配查询 → 账单状态投影 → 历史、Dashboard、催缴读模型`。边界格式固定为 cents（整数）、ISO currency code、ISO 日期／时间和用户 ID；UI 不得自行累加原流水总额。
