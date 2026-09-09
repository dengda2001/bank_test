# 租客租金匹配 MySQL Demo 实施计划

## 实施原则

- 先打通数据库、账号隔离和迁移，再迁移页面和匹配。
- 每一步都保持 `go test ./...` 可运行。
- handler 不直接拼 SQL；通过服务层调用 GORM repo。
- 所有业务读写都显式传入当前 `userID`。
- 不在生产启动路径使用 GORM `AutoMigrate`。

## 步骤 1：依赖和配置

- 添加 GORM、MySQL driver、bcrypt 依赖。
- 扩展配置：
  - `MYSQL_DSN`
  - `DATABASE_URL`
  - `BANK_TOKEN_ENCRYPTION_KEY`
  - `ALLOW_PLAINTEXT_TOKENS`
- 实现 DSN 解析和优先级：`MYSQL_DSN` > `DATABASE_URL`。
- 实现 token 加密配置校验：live 环境默认必须有加密密钥。

验证：

- 单元测试覆盖 DSN 优先级和 token 加密配置校验。
- `go test ./...`

## 步骤 2：Migration 运行器和初始 schema

- 新增 `migrations/001_initial_mysql.sql`。
- 创建 `schema_migrations` 表。
- 创建一期核心表：
  - `users`
  - `bank_connections`
  - `tenants`
  - `rent_obligations`
  - `payment_transactions`
  - `payment_allocations`
  - `manual_expenses`
- 应用启动连接 MySQL 并执行未应用 migration。
- migration 失败时启动失败。

验证：

- migration runner 单元测试可用 fake/sql 或临时测试库策略。
- 如本地 MySQL 可用，跑一次真实启动检查。
- `go test ./...`

## 步骤 3：账号体系改造

- 新增 `User` GORM model 和 AuthService。
- 启动时用 `APP_ADMIN_USERNAME` / `APP_ADMIN_PASSWORD` seed 默认用户。
- 登录从 MySQL `users` 表验证 bcrypt hash。
- session cookie 保存签名后的 `user_id` 和过期时间。
- 保留现有登录保护语义。

验证：

- 默认用户 seed 测试。
- bcrypt 登录成功/失败测试。
- 未登录访问保护测试继续通过。
- session 解析 current user 测试。
- `go test ./...`

## 步骤 4：Bank token 迁移到账号维度

- 新增 BankConnectionStore。
- `/callback` 保存当前用户的 refresh token 到 `bank_connections`。
- `/refresh` 只读取当前用户 token。
- 实现加密保存和解密读取。
- 旧 `TL_TOKEN_FILE` 只作为一次性导入/兼容来源，不再是新流程 token source。

验证：

- 账号 A/B token 隔离测试。
- 加密存储不包含明文 refresh token 测试。
- plaintext dev escape 测试。
- refresh 读取当前用户 token 测试。
- `go test ./...`

## 步骤 5：租客管理迁移到 MySQL

- 扩展租客表单和 handler 字段：
  - 姓名
  - payer id
  - payer name hint
  - rent amount/currency
  - interval unit/count
  - billing start date
  - due day
  - rent start/end date
  - status
  - room label/address/property hint
- 租客 CRUD 先保持创建和列表，后续再做编辑。
- 所有查询按 `userID` 过滤。

验证：

- 创建租客字段校验测试。
- 账号隔离测试。
- 页面模板可渲染测试。
- `go test ./...`

## 步骤 6：交易归一化入库

- 把当前 `normalizeIncomeTransactions` 拆成通用交易归一化：
  - income: `CREDIT` 或正数；
  - expense: `DEBIT` 或负数；
  - 保存 payer id/name/reference/description/raw payload。
- TrueLayer callback/refresh 成功后，把 demo result 入库。
- 保留 JSONL 写入可作为兼容日志，但 UI 读 MySQL。
- 幂等 key 防止重复交易。

验证：

- provider stable id 幂等测试。
- fallback hash key 幂等测试。
- income/expense 方向测试。
- 当前收入归一化测试迁移或保留。
- `go test ./...`

## 步骤 7：一次性导入旧数据

- 实现 `ImportLegacyFiles(ctx, userID)`。
- 导入 `TL_LOG_FILE` 银行 JSONL 到 `payment_transactions`。
- 导入 `RENTOPS_TENANT_FILE` 到 `tenants`。
- 导入 `RENTOPS_EXPENSE_FILE` 到 `manual_expenses` 和 `payment_transactions`。
- 文件不存在/为空跳过。
- 提供入口：优先做 CLI flag 或受保护 POST `/import-legacy`。

验证：

- JSONL 导入测试。
- 旧租客 JSON 默认字段映射测试。
- 旧支出 JSON 导入测试。
- 重复导入幂等测试。
- `go test ./...`

## 步骤 8：月租应收懒生成

- 实现 `EnsureMonthlyObligations(ctx, userID, periodMonth)`。
- 有效期判断：
  - `rent_start_date` 与月份有交集；
  - `rent_end_date` 为空或不早于月份开始。
- 不做按天折算。
- 已存在应收不覆盖。
- 保留批量入口 `GenerateMonthlyObligations(ctx, userID, fromMonth, toMonth)`。

验证：

- 有效期内生成测试。
- 有效期外不生成测试。
- 已存在不覆盖测试。
- 批量入口复用单月逻辑测试。
- `go test ./...`

## 步骤 9：匹配引擎

- 实现备注月份解析。
- 实现 payer ID 自动匹配。
- 实现姓名候选匹配。
- 实现金额规则：
  - 等额自动 paid；
  - 小于应收余额自动 partial；
  - 大于应收余额 needs_review。
- 创建 `payment_allocations` 并更新 obligation/transaction 状态。

验证：

- payer ID 等额自动匹配测试。
- payer ID 部分付款测试。
- payer ID 超付待审核测试。
- 姓名候选不自动确认测试。
- 备注月份优先匹配测试。
- 月份解析格式测试。
- `go test ./...`

## 步骤 10：人工确认

- 在 `/billing` 流水表中对单一候选提供确认按钮。
- POST 确认时：
  - 校验当前用户拥有交易和应收。
  - 创建 allocation。
  - 必要时回填 tenant payer id。
  - 更新交易状态和月租应收状态。
- 多候选不显示快捷确认按钮。

验证：

- 确认候选创建 allocation 测试。
- payer id 回填测试。
- 跨账号确认被拒测试。
- 多候选不自动确认测试。
- `go test ./...`

## 步骤 11：页面改造

- 新增 `/rent-dashboard`：
  - 月份选择；
  - 应收/已收/未收/部分/待审核/支出汇总；
  - 租客月度表。
- 简化 `/billing` 为纯流水页：
  - 银行状态和刷新；
  - 月份、matched、unmatched、expense 筛选；
  - 交易表和候选确认按钮。
- 更新 `/tenants` 和 `/expenses` 字段。
- 侧边栏入口：
  - Rent Dashboard
  - Transactions
  - Tenants
  - Expenses

验证：

- 模板渲染测试。
- 浏览器或 curl 检查核心页面 200。
- 筛选参数测试。
- `go test ./...`

## 步骤 12：README 和 spec 更新

- README 增加：
  - 本地 MySQL 建库和 DSN 配置；
  - migration 自动执行；
  - 默认用户初始化；
  - token 加密配置；
  - 一次性导入旧 JSON/JSONL；
  - `/rent-dashboard` 和 `/billing` 新语义。
- 更新 `.trellis/spec/backend/database-guidelines.md`：
  - MySQL/GORM/migration 约定；
  - 账号隔离；
  - token 加密；
  - 流水幂等。

验证：

- 文档和代码 env key 一致性 grep。
- `go test ./...`

## 风险文件和回滚点

- `cmd/truelayer-demo/main.go` 当前过大，实施时建议拆分文件但保持同 package。
- `go.mod` 会新增依赖。
- `migrations/001_initial_mysql.sql` 是长期 schema 基础，提交前必须重点 review。
- auth/session 改动影响所有受保护路由，必须先有测试再替换。
- token 迁移影响真实银行刷新，必须保留清晰错误提示和重新绑定路径。

## 最终质量门

- `go test ./...` 通过。
- 本地 MySQL 启动 app 成功，migration 已应用。
- 默认用户可登录。
- 可以导入已有银行 JSONL。
- 可以创建租客并生成指定月份应收。
- 至少一条交易能通过 ID 自动匹配，至少一条姓名候选能人工确认并回填 payer ID。
- `/rent-dashboard`、`/billing`、`/tenants`、`/expenses` 都能渲染。
