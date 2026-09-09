# 租客租金匹配 MySQL Demo 技术设计

## 架构边界

一期继续保留单进程 Go HTTP 服务，不引入前端构建链、后台 worker、正式组织权限系统或复杂房源合同模型。

新增边界：

- 数据库访问层：GORM + MySQL driver，连接由配置提供。
- Schema 管理：仓库内 SQL migration，应用启动自动执行未应用版本。
- 账号体系：`users` 表是真实账号来源；环境变量只负责初始化默认用户。
- 账号隔离：所有业务读写都带 `user_id`。
- 银行 token：从全局 token 文件迁移到 `bank_connections` 表，并按 `user_id` 加密存储。
- 租金规则：租客表保存一期所需租金、周期、房间文本和付款方识别字段。
- 月租应收：稳定入库，作为匹配、人工确认、部分付款和月度状态计算的目标对象。
- 交易池：银行收入、银行支出、手动支出进入统一可筛选视图；租金匹配只作用于收入交易。

## 运行配置

新增配置：

- `MYSQL_DSN`：优先使用的 Go MySQL DSN。
- `DATABASE_URL`：兼容部署平台的连接 URL；如果是 `mysql://...`，转换成 MySQL DSN。
- `BANK_TOKEN_ENCRYPTION_KEY`：refresh token 加密密钥，建议 base64 编码 32 字节。
- `ALLOW_PLAINTEXT_TOKENS=1`：仅本地 demo 允许明文 token 存储的逃生口。

规则：

- `MYSQL_DSN` 优先于 `DATABASE_URL`。
- `TL_ENV=live` 且没有 `BANK_TOKEN_ENCRYPTION_KEY` 时启动失败，除非显式允许本地明文逃生口且不是 live。
- 没有配置数据库时，MySQL 版本启动失败；不再默默退回 JSON 文件作为长期存储。

## 数据表

### `schema_migrations`

记录已执行 migration。

- `version`：主键，例如 `001_initial_mysql`。
- `applied_at`：执行时间。

### `users`

真实登录账号。

- `id`：BIGINT 主键。
- `username`：唯一。
- `password_hash`：bcrypt hash。
- `created_at`、`updated_at`。

启动时如果 `APP_ADMIN_USERNAME` 不存在，则用 `APP_ADMIN_PASSWORD` 创建默认用户。存在时不覆盖密码，避免重启误改人工维护账号。

### `bank_connections`

账号维度银行连接和 refresh token。

- `id`。
- `user_id`。
- `provider`：一期 `truelayer`。
- `environment`：`sandbox` / `live`。
- `refresh_token_ciphertext`。
- `refresh_token_nonce`。
- `token_storage_mode`：`encrypted` / `plaintext_dev`。
- `saved_at`、`last_sync_at`。
- `created_at`、`updated_at`。

唯一约束：`user_id + provider + environment`。

### `tenants`

一期租客和月租规则。

- `id`。
- `user_id`。
- `name`。
- `payer_id`：银行付款方 ID，可空。
- `payer_name_hint`：银行付款方姓名/别名，可空。
- `monthly_rent_cents`。
- `currency`。
- `interval_unit`：一期固定保存 `month`，后续可扩展。
- `interval_count`：一期固定 `1`。
- `billing_start_date`：账单规则开始日期。
- `due_day`：每月应收日，1-31。
- `rent_start_date`：开始收租日期。
- `rent_end_date`：可空。
- `status`：`active` / `inactive`。
- `room_label`。
- `room_address`。
- `property_hint`：可空。
- `created_at`、`updated_at`。

唯一约束建议：

- `user_id + payer_id` 在 `payer_id` 非空时唯一，避免同一付款方自动匹配到多个租客。

### `rent_obligations`

每个租客每个账期的应收租金。

- `id`。
- `user_id`。
- `tenant_id`。
- `period_month`：`YYYY-MM-01`。
- `due_date`。
- `expected_amount_cents`。
- `paid_amount_cents`：可由 allocation 汇总，也可以作为缓存字段。
- `currency`。
- `status`：`open` / `partial` / `paid` / `overdue` / `needs_review`。
- `generated_by`：`lazy` / `batch` / `manual`。
- `created_at`、`updated_at`。

唯一约束：`user_id + tenant_id + period_month`。

### `payment_transactions`

归一化银行流水和手动付款/支出池。

- `id`。
- `user_id`。
- `source`：`truelayer` / `manual_expense`。
- `source_batch_id`：可空，用于导入/同步批次。
- `provider_transaction_id`。
- `stable_transaction_key`：幂等唯一 key。
- `account_id`、`account_name`。
- `direction`：`income` / `expense`。
- `amount_cents`：正数保存金额，方向由 `direction` 表达。
- `currency`。
- `transaction_time`。
- `description`。
- `reference`。
- `payer_id`。
- `payer_name`。
- `payer_name_kind`：`confirmed` / `inferred` / `unknown`。
- `match_status`：`unmatched` / `candidate` / `matched` / `needs_review` / `ignored`。
- `raw_payload_json`。
- `created_at`、`updated_at`。

唯一约束：`user_id + stable_transaction_key`。

稳定 key 生成顺序：

1. provider stable id：`normalised_provider_transaction_id`、`provider_transaction_id`、meta 中 provider/bank id。
2. 如果没有稳定 id，使用 account、timestamp、amount、currency、description/reference hash 的组合。

### `payment_allocations`

已确认交易到应收账单的分配。

- `id`。
- `user_id`。
- `payment_transaction_id`。
- `rent_obligation_id`。
- `tenant_id`。
- `amount_cents`。
- `status`：一期只创建 `confirmed`。
- `confirmed_by_user_id`。
- `confirmed_at`。
- `confirmation_source`：`auto_id` / `manual_name`。
- `created_at`。

唯一约束建议：`user_id + payment_transaction_id + rent_obligation_id`。

### `manual_expenses`

手动支出源记录。保存后也投影到 `payment_transactions`，方向为 `expense`。

- `id`。
- `user_id`。
- `description`。
- `category`。
- `amount_cents`。
- `currency`。
- `expense_date`。
- `payment_method`。
- `room_hint`。
- `tenant_hint`。
- `created_at`、`updated_at`。

## 服务层

### AuthService

- `SeedDefaultUser(ctx)`：根据 env 创建默认用户，不覆盖已有用户密码。
- `Authenticate(ctx, username, password) (User, error)`：查 `users` 并 bcrypt 校验。
- `SessionUserID(r)`：从签名 cookie 解析当前用户。

### BankConnectionStore

- `SaveRefreshToken(ctx, userID, token)`：按用户和环境保存 token。
- `LoadRefreshToken(ctx, userID)`：只读取当前用户 token。
- 加密逻辑封装在 store 内，handler 不接触密钥细节。

### TenantService

- `CreateTenant(ctx, userID, input)`。
- `ListTenants(ctx, userID, filters)`。
- `UpdateTenantPayerID(ctx, userID, tenantID, payerID)`。
- 校验 due day、日期、金额、周期字段。

### ObligationService

- `EnsureMonthlyObligations(ctx, userID, periodMonth)`。
- `GenerateMonthlyObligations(ctx, userID, fromMonth, toMonth)`：一期不做 UI，但保留批量入口。
- `SummarizeMonth(ctx, userID, periodMonth)`。

懒生成逻辑：

1. 查询当前用户有效租客。
2. 判断目标月份和租客有效期是否有交集。
3. 对缺失的 `user_id + tenant_id + period_month` 插入应收。
4. 已存在记录不覆盖，避免人工调整被静默改写。

### TransactionService

- `IngestDemoResult(ctx, userID, demoResult)`：TrueLayer sync 后直接入库。
- `ImportLegacyFiles(ctx, userID)`：一次性导入 `TL_LOG_FILE`、旧租客 JSON、旧支出 JSON。
- `ListTransactions(ctx, userID, filters)`。
- `ConfirmCandidate(ctx, userID, transactionID, obligationID, backfillPayerID)`。

### MatchService

匹配流程：

1. 只处理 `direction=income` 且未分配交易。
2. 如果交易有 `payer_id`，查当前用户 `tenants.payer_id`。
3. 如果唯一命中租客，解析备注月份；优先匹配备注月份未结清账单，否则找付款日期附近未结清账单。
4. 金额等于应收余额：自动确认。
5. 金额小于应收余额：自动确认部分付款。
6. 金额大于应收余额：标记 `needs_review`。
7. 如果无 `payer_id` 或租客未保存 payer id，则按 `payer_name` 与租客姓名/别名找候选；唯一候选也只标记 `candidate`，等待人工确认。
8. 多候选或无候选：保持 `needs_review` 或 `unmatched`。

备注月份解析支持 PRD 中列出的格式。无年份月份按交易日期附近推断年份。

## 页面和路由

新增/调整路由：

- `GET /rent-dashboard`：月度交租工作台。
- `GET /billing`：纯流水页，支持筛选和刷新/连接状态。
- `POST /transactions/{id}/confirm` 或 `POST /confirm-match`：确认单一候选匹配。
- `POST /import-legacy`：一期可放在管理区域或先做命令/隐藏入口，用于一次性导入。

保留：

- `GET /tenants` / `POST /tenants`：租客管理，字段扩展。
- `GET /expenses` / `POST /expenses`：支出管理，字段扩展。
- `/login`、`/callback`、`/refresh`：TrueLayer 入口和刷新，全部按当前 `user_id` 读写。

侧边栏：

- Rent Dashboard -> `/rent-dashboard`
- Transactions -> `/billing`
- Tenants -> `/tenants`
- Expenses -> `/expenses`

`/billing` 页面只做流水：

- 顶部：银行连接状态、刷新、月份/类型筛选。
- 表格：日期、方向、金额、付款方、reference、description、匹配状态、候选确认操作。
- 不展示租客月度汇总或工作台指标。

`/rent-dashboard` 页面：

- 月份选择器。
- 汇总：应收总额、已收、未收、部分付款、待审核、支出总额。
- 租客月度表：租客、房间、应收、已付、余额、due date、状态。
- 可进入对应流水筛选。

## 一次性导入

一期做小而明确的一次性导入：

- 读取 `TL_LOG_FILE` 每行 `demoResult`，把每个账户交易归一化入 `payment_transactions`。
- 读取 `RENTOPS_TENANT_FILE`，映射到 `tenants`；没有新字段时使用合理默认：
  - `interval_unit=month`
  - `interval_count=1`
  - `billing_start_date=created_at` 或当前月份第一天
  - `rent_start_date=billing_start_date`
  - `due_day=1`
  - `status=active`
- 读取 `RENTOPS_EXPENSE_FILE`，映射到 `manual_expenses` 并投影到 `payment_transactions`。
- 文件不存在或为空时跳过。
- 导入可重复运行，但不能生成重复可见交易或重复租客/支出记录。

## 安全和隔离

- 所有业务表查询必须带 `user_id`。
- handler 不接受来自表单的 `user_id` 作为可信来源，只使用 session 中的当前用户。
- bank token 按用户读取；账号 A 不能刷新账号 B 的银行连接。
- refresh token 默认加密保存。
- 原始银行 payload 是敏感数据，只用于审计/调试和重新归一化，不在页面大段展示。

## 兼容和迁移

- JSON/JSONL 文件只作为一次性导入来源，不再是长期 source of truth。
- README 需要更新 MySQL 配置、migration、默认用户初始化、一次性导入说明。
- 历史测试要继续覆盖登录保护、OAuth state、refresh token 行为。
- 新服务层要尽量从当前单文件中拆出，但一期可以保持小规模包结构，例如 `cmd/truelayer-demo` 内部分文件。

## 风险和取舍

- 银行字段不稳定：匹配必须保留 `candidate` / `needs_review`，不能过度自动化。
- 同名租客风险：姓名匹配只生成候选，不自动确认。
- 金额超付风险：超付进入审核，不自动关闭账单。
- 账号隔离风险：所有 repo 方法必须显式接收 `userID`，避免误查全表。
- MySQL 集成会引入外部依赖：本地测试需要区分纯单元测试和真实 MySQL 集成测试。
