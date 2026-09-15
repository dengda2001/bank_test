# 现金租金补录与作废更正：技术设计

## 目标与边界

现金收款是租金账单的另一种有效收款来源，但不是银行流水。补录只针对一个租客的一个月度房租账单，允许同月多笔现金与银行房租共同结清；错误记录通过作废并重录修正。现金记录必须能在租客历史、Dashboard 明细和审计中与银行来源区分。

本任务不创建银行 `payment_transactions`、付款人关系或银行流水号，不支持现金押金、现金多月拆分、退款和非 EUR 收款。

## 现状依据与关键决定

- `payment_allocations.payment_transaction_id` 是非空外键，因此现金不能伪装成一笔 `payment_transaction`，也不能复用银行分配表承载无银行来源的现金。
- `rent_obligations.paid_amount_cents` 是读优化投影；现金写入必须和银行有效房租分配一起重算，不能直接累加。
- 采用新增 `cash_receipts` 表：一行即一笔“现金收款＋一个目标账单月份”，通过 `status=confirmed/voided` 保留作废历史。
- 使用独立的用户范围幂等键和补录编号。补录编号只标识现金记录，绝不作为银行交易编号展示。

## 数据模型与迁移

新增 `migrations/006_cash_rent_receipts.sql`，并由 `007_cash_receipt_void_audit.sql` 补充作废操作号字段：

```text
cash_receipts
  id, user_id, tenant_id, rent_obligation_id
  receipt_number, amount_cents, currency, received_at, note
  status, operation_id, void_operation_id, idempotency_key
  recorded_by_user_id, recorded_at
  voided_at, voided_by_user_id, void_reason
  created_at, updated_at
```

- `(user_id, receipt_number)` 唯一；`(user_id, idempotency_key)` 唯一，幂等键由服务层强制要求。
- 外键均带用户范围查询：用户、租客、账单和操作者；删除租客／用户按账本既有级联规则处理。
- `rent_obligation_id`、`tenant_id` 必填，`amount_cents` 为正整数，`currency` 保存原值但一期只接受 `EUR` 有效写入。
- `received_at` 存实际现金收款日；`recorded_at` 存系统记录时间，两者不能混淆。
- 作废只更新现金行的状态及作废审计字段，不删除原记录。

## 服务层与事务边界

新增 `cashReceiptService`：

1. `previewCashReceipt` 只读加载当前用户的租客和有效账单，计算应收、当前实收、本次金额和提交后未收，不写任何账本行。
2. `recordCashReceipt` 开启数据库事务，按 `user_id` 锁定账单，重新加载租客、账单和当前有效银行／现金收款；校验金额、币种、账单状态和实时余额后创建一行现金记录。
3. 在同一事务中重新读取该账单的有效银行房租分配与有效现金记录，调用共享投影函数更新 `paid_amount_cents` 和账单状态。有效现金只计入目标账单，不影响银行流水金额、笔数和待处理数。
4. 幂等键已存在时返回原现金记录的逻辑结果；同一键携带不同租客、月份或金额时拒绝，不产生第二笔记录。
5. `voidCashReceipt` 按账单锁顺序锁定目标账单和现金行，校验用户归属及当前状态，带原因将 `confirmed` 改为 `voided`，写操作者、时间、原因和独立的 `void_operation_id`，保留原始 `operation_id`，再重算目标账单。重复作废幂等返回，不重复扣减。

并发补录通过账单行锁串行化；第二个请求基于最新余额重新校验并失败时，事务回滚且不留下半套现金记录。银行分配服务仍锁同一账单并使用同一投影入口。

## HTTP 与页面契约

- `GET /cash-receipts/new?tenant_id=&period=`：认证后渲染单月现金补录表单；租客和月份来自用户范围数据。
- `POST /cash-receipts/preview`：认证后只生成确认页，提交金额、收款日、备注和幂等键；确认页显示应收、已收、本次金额和提交后未收。
- `POST /cash-receipts`：认证后写入一笔现金收款，成功跳回原账单／租客历史。
- `GET /cash-receipts/void?receipt_id=`：认证后显示原记录、金额、目标月份和当前状态。
- `POST /cash-receipts/void`：认证后带必填原因作废，成功返回账单／历史。

租客详情和月度 Dashboard 的有效收款明细分别显示来源“现金”、补录编号、实际收款日和备注；银行明细继续显示银行来源和原始字段。补录表单只从账单／租客历史入口产生，不提供跨月或押金用途。

## 数据流

```text
账单／租客历史入口
  -> authenticated preview
  -> POST recordCashReceipt
  -> cash_receipts (confirmed, source=现金)
  -> effective bank rent + effective cash rent projection
  -> rent_obligations paid/status
  -> Dashboard + tenant history
                  ^
                  └── POST voidCashReceipt -> audit + recompute
```

所有跨层金额使用 integer cents；模板只消费类型化 view model，不直接查询 GORM 结构。错误页面只展示安全领域错误，不回显 SQL、请求体或其他用户数据。

## 兼容、回滚与风险

- 旧数据库通过显式 `006` 和 `007` 迁移增加结构，迁移 runner 记录版本并支持重复启动；旧代码忽略新表时不会改变既有银行账本。
- 回滚应用版本不删除现金表或审计行；后续版本仍可根据状态重算账单。
- 现金与银行同时更新同一账单时依赖相同锁顺序（先账单，再收款行）避免超额写入。
- 未完成真实 MySQL 时，必须至少保留服务层原子性、用户隔离、幂等和 EUR 校验的单元测试；可用隔离 DSN 时再运行迁移、并发和 HTTP 集成验收。
