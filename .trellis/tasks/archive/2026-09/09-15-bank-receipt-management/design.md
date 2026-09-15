# 银行入账归类、拆分与纠错：技术设计

## 目标与边界

本子任务把现有银行原始流水变成可追溯、可继续处理的收款来源：支持严格自动识别、人工归类、同一租客内拆分、部分确认、余款续配、整体撤销与重新匹配，并提供真实的同步覆盖状态和组合查询。

本任务不负责现金收款写入、月度 Dashboard 的业务汇总、催缴邮件、跨租客拆分、退款、预存款或完整押金台账。账单金额和状态继续由 `09-15-rent-ledger-foundation` 的有效分配投影维护；租客付款人关系继续由 `09-15-tenant-profile-and-history` 提供。

## 现状依据

- `payment_transactions` 已保存原始流水、稳定幂等键、账户、方向、金额、币种、到账时间、描述、参考号、付款人和原始 JSON。
- `payment_allocations` 已支持 `rent`／`deposit`／`other_income`、有效／撤销状态、操作号和幂等键，但当前服务只支持一笔房租整笔确认，页面也没有显示分配明细。
- `rent_obligations` 的 `paid_amount_cents` 是读优化投影，不能由页面或 handler 直接加减；有效房租分配是唯一实收来源。
- `tenant_payers` 已支持一个租客多个付款人及软删除；同一个付款人可关联多个租客，必须作为共享冲突处理。
- 当前同步只有 `bank_connections.last_sync_at`，单次 `demoResult` 的账户错误不会持久化；当前 `period` 筛选实际按到账月过滤，不能代替租金所属月。

## 领域状态与不变量

### 来源与分配

`payment_transactions` 是不可变银行原文。所有收入处理都通过 `payment_allocations` 或显式的“无需匹配”动作表达，不复制或拆分原始流水。

- `allocation_kind=rent`：必须有同一租客的有效账单月份，计入该月实收。
- `allocation_kind=deposit`：必须有关联租客，不需要账单月份，不计入租金实收或其他收入。
- `allocation_kind=other_income`：备注必填，租客可选，不需要账单月份；只有有效分配金额计入其他收入。
- “无需匹配”不是第四种用途，使用独立的交易动作记录原因；它不能覆盖仍有有效分配的流水。
- 同一原流水的有效分配金额之和不得超过原金额；所有房租、押金和其他收入用途共享同一金额预算。
- 一期所有有效分配必须属于同一租客；跨租客请求整次拒绝。
- 只接受 `EUR` 有效分配。原始币种保持可见，非 EUR 只能保留待处理或被明确拒绝。

交易匹配状态由有效分配和当前处理动作投影：

| 状态 | 判定 |
| --- | --- |
| `unmatched` | 没有有效分配，且没有有效候选或无需匹配动作 |
| `candidate` | 只有候选租客或待确认月份，尚未产生有效分配 |
| `needs_review` | 存在冲突、币种／金额／月份问题或人工需要处理 |
| `partial` | 有效分配金额小于原金额，余款仍可续配 |
| `matched` | 有效分配金额等于原金额 |
| `ignored` | 有效的无需匹配动作且没有有效分配 |

状态是处理状态，不得在 UI 上复用账单的“未缴／部分缴纳／已缴清／逾期”。账单状态只从 `rent_obligations` 的有效房租分配投影得出。

### 月份来源

在同步入库时将描述和参考号交给共享 `parseReferencedPeriod`，保存识别出的月份、来源和可读原因。自动确认只接受明确的备注月份；到账月仅用于查询和人工默认提示，不得作为自动回退的租金归属。最终确认月份来自 `rent_obligation.period_month`，两者同时展示。

## 数据模型与迁移

新增 `migrations/005_bank_receipt_management.sql`，沿用启动时按文件序号执行的显式 SQL migration，不使用 GORM `AutoMigrate`。

### 同步批次

新增 `bank_sync_runs`：

- `id`, `user_id`, `provider`, `environment`；
- `mode`（`initial_year`／`refresh_90d`）、`requested_from`, `requested_to`；
- `status`（`running`／`succeeded`／`partial`／`failed`）、`started_at`, `finished_at`；
- `error_message`, `created_at`, `updated_at`。

新增 `bank_sync_run_accounts`：

- `id`, `user_id`, `bank_sync_run_id`, `account_id`, `account_name`；
- `requested_from`, `requested_to`, `covered_from`, `covered_to`；
- `status`, `transaction_count`, `error_message`, `started_at`, `finished_at`。

批次和账户表均带 `user_id`、索引和外键。初次授权保存 `initial_year`，手动刷新保存 `refresh_90d`；账户级失败汇总为 `partial`，全部失败为 `failed`。不生成逐日缺口报告。

### 交易和分配扩展

在 `payment_transactions` 增加：

- `parsed_period_month`、`parsed_period_source`、`parsed_period_note`，保存备注解析结果；
- `match_reason`，保存当前待处理／拒绝原因的稳定展示文本；
- 针对处理状态和解析月份增加用户范围索引。

在 `payment_allocations` 增加 `note`（其他收入备注及必要的用途说明）。继续使用已有 `operation_id`、`idempotency_key`、`voided_*` 字段；旧行空 `allocation_kind` 仍按房租解释。

新增 `payment_transaction_actions` 保存没有分配行也必须可追溯的操作：

- `id`, `user_id`, `payment_transaction_id`, `action_kind`（`ignore`／`restore`／`revoke_allocations`）；
- `reason`, `operation_id`, `idempotency_key`, `acted_by_user_id`, `created_at`。

所有外键和唯一幂等键都带 `user_id`。迁移只增加字段、索引和审计表，不删除原始流水、不重写历史分配、不合并重复数据。

## 服务层与事务边界

### 同步服务

新增同步批次服务，包住 callback 和 refresh 的现有 fetch 流程：

1. 在调用 provider 前按当前用户创建批次及账户请求记录；
2. 拉取每个账户，成功时写入实际覆盖区间和交易数，失败时保存安全错误摘要；
3. 将 `source_batch_id` 设为批次稳定标识，幂等写入交易；
4. 只有至少一个账户成功入库才更新连接的最后同步时间；批次状态由账户结果汇总；
5. 日志仍可写入兼容 JSONL，但数据库批次状态才是已认证 UI 的覆盖事实。

账户列表失败时创建失败批次并不写空结果；交易接口部分失败不能丢弃已成功账户的流水，也不能显示“全年同步成功”。

### 统一分配写入

新增统一的 `allocateTransaction` 写入入口，所有房租、押金、其他收入和拆分表单都使用同一实现。事务内必须：

1. 按 `user_id` 锁定原流水、加载当前有效分配并计算余款；
2. 校验所有请求项的用途、金额、EUR、租客一致性、账单归属、账单余额和备注；
3. 通过 `idempotency_key` 防止重试重复入账；
4. 一次请求先完整校验再批量插入，任何一项失败都回滚全部新增项；
5. 重新计算受影响账单的有效房租总额和状态，并投影流水匹配状态；
6. 只有成功提交后才更新付款人关系或其 `last_matched_at`。

单项确认只是拆分项数为一；部分确认只消耗本次指定金额，剩余金额继续保留在原流水。

### 撤销与无需匹配

- 撤销事务锁定原流水，将该流水当前所有有效分配统一标记为 `voided`，写入相同的 `operation_id`、原因、操作者和时间，再对所有受影响账单从有效分配重算；最后清理当前交易的处理投影。
- 已撤销分配只保留审计用途，旧撤销请求通过幂等键重试不影响新产生的分配。
- “无需匹配”只有在没有有效分配时新增 `ignore` 动作；恢复时新增 `restore` 动作并回到待处理状态。任何已有有效分配的流水必须转到撤销重配流程。

## 自动识别与付款人记忆

匹配读取 `tenant_payers` 的 active 关系，而不是别名或旧单字段；付款人 ID 优先，标准化精确姓名次之。唯一付款人关系仍需同时满足：备注有明确唯一月份、币种为 EUR、金额不超过该账单未收、没有用途和身份冲突，才自动写入房租分配。

只有找到租客但月份不明确、存在多个租客关系、金额超额、币种不符或备注冲突时，保留候选／待处理原因，不回退到账月或任意欠款月。手动匹配默认保存付款人关系但显示可取消的“记住付款人”；历史流水批处理是独立勾选，先生成逐笔预览，再逐笔按当前余额和规则确认，共享付款人禁止批量自动选人。

## 查询、HTTP 与页面

### 查询契约

`transactionFilters` 扩展为到账日期起止、付款人文本／ID、租客、匹配状态、租金所属月、入账用途、收支方向、排序、页码、页大小和待处理。保留旧 `period` 参数作为到账月兼容入口，并增加明确的 `rent_period` 参数；不得混用两者。

用途筛选通过有效分配的 `EXISTS` 条件完成，租金月份通过有效 `rent` 分配连接 `rent_obligations` 完成；待处理笔数按原流水去重，不能按拆分项计数。所有查询都带当前 `user_id`，排序字段使用白名单。

### 路由

保留 `GET /billing`、`GET|POST /refresh` 和 OAuth callback，新增或扩展：

- `POST /billing/allocate`：单项或多项原子确认／拆分；
- `GET /billing/revoke`：渲染当前有效分配和余额变化预览；
- `POST /billing/revoke`：带必填原因执行整笔撤销；
- `POST /billing/ignore`、`POST /billing/restore`：管理无需匹配动作；
- `POST /billing/payer/preview`、`POST /billing/payer/confirm`：历史批处理预览与确认。

表单 ID、金额、月份和用途全部在服务层重新读取并校验，不能信任页面隐藏字段。所有路由沿用现有认证，跨用户资源统一返回安全错误或无结果。

### 页面

流水表展示内部 ID 与银行流水号、到账日期与租金所属月、备注解析月与最终确认月、分配金额／余款、用途、匹配状态、同步覆盖状态和安全原因。收入用途操作在同一面板中选择单项或拆分项；新增拆分项默认房租。撤销必须先看预览再提交，历史批处理必须先看逐笔预览。

## 兼容性、并发与回滚

- 旧分配按已确认房租读取；旧交易没有解析字段时可按当前 parser 惰性补全，但不能把到账月写成最终租金月。
- JSON／JSONL 继续作为未认证或 legacy import 的兼容来源；新功能的持久化真相是用户隔离的 MySQL 表。
- 来源锁、账单锁或等价的条件更新必须在同一事务内完成；并发续配只能有一个请求消费同一余款，另一个返回余额冲突且不留半套分配。
- 应用回滚时新增列和表可被旧代码忽略；不直接删除审计字段。迁移失败由现有迁移事务回滚，禁止手工清理已有银行流水。
- 错误和日志不得包含 token、原始 JSON 全文或不必要的付款人敏感信息；页面显示安全的领域原因。

## 跨层数据流

```text
TrueLayer fetch
  -> sync run/account coverage + immutable payment_transactions
  -> parser + payer candidate
  -> one transaction allocation transaction
  -> effective allocation projection
  -> rent history / dashboard / dunning readers
```

跨层边界统一使用整数 cents、ISO 币种、UTC 时间戳、月份首日和当前用户 ID；模板不自行汇总原始流水或直接读取 GORM 结构。
