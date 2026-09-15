# 现金租金补录与作废更正：实施计划

## 启动门槛

- [x] 完成 `prd.md`、`design.md` 与本计划审阅，并确认现金不创建银行流水。
- [x] 执行 `python3 ./.trellis/scripts/task.py start 09-15-cash-rent-receipts`，状态变为 `in_progress` 后再修改业务代码。
- [x] 执行 `trellis-before-dev`：读取 backend spec、账本投影、租客历史和认证路由约定。

## 纵向切片

### 切片 1：现金账本结构与共享投影

- [x] 新增 `006_cash_rent_receipts.sql`，包含用户范围唯一幂等键、补录编号、有效／作废审计字段和外键。
- [x] 新增 `cashReceipt` 模型与 integer-cent 输入校验。
- [x] 抽取／扩展账单投影，使有效银行房租和有效现金共同计算 `paid_amount_cents`，押金／其他收入不计入。
- [x] 覆盖 EUR-only、非 EUR、零负数、目标账单作废和用户归属测试。

### 切片 2：补录预览与原子写入

- [x] 实现用户范围账单加载、应收／已收／本次／提交后未收预览。
- [x] 实现 `recordCashReceipt`：锁账单、重新校验实时余额、创建现金记录、重算投影，失败整次回滚。
- [x] 支持同月多次现金与银行有效房租累计；重复幂等请求不得重复计账。
- [x] 通过认证 HTTP 路由和账单／租客历史入口提供补录表单。

### 检查点 A：补录安全门

- [x] `go test ./cmd/truelayer-demo -count=1` 与 `go test ./... -count=1` 通过。
- [x] 现金记录不创建 `payment_transactions`、付款人关系或银行流水号。
- [x] 用户不能用猜测的租客／账单 ID 为其他用户补录。

### 切片 3：作废、更正与历史展示

- [x] 实现带原因的现金作废预览／提交，保留原记录、操作者、时间、operation ID 和原因。
- [x] 作废只重算目标账单，银行贡献和其他账单保持不变；重复作废幂等。
- [x] 账单 Dashboard、租客历史和收款明细展示现金来源、实际收款日与补录编号。
- [x] 验证错误 €400 作废后可重录 €300，原现金审计仍可追溯。

### 检查点 B：跨层语义门

- [x] 账单实收同时包含有效银行房租和有效现金；现金不会出现在银行流水页或银行笔数统计。
- [x] 作废现金不删除记录，不改变其他来源，不产生银行付款人绑定。
- [x] preview／GET 不产生任何写入；所有 POST 参数在服务层重新校验。

### 切片 4：并发、迁移与回归

- [x] 增加迁移重复启动无副作用的 opt-in MySQL 测试。
- [x] 增加并发补录余额竞争测试，确认只有余额内的请求成功。
- [x] 运行 `go vet ./...`、`git diff --check`，并按可用性执行真实 HTTP／浏览器检查。

## 主要文件范围

- `migrations/006_cash_rent_receipts.sql`
- `cmd/truelayer-demo/cash_receipts.go`
- `cmd/truelayer-demo/cash_receipt_handlers.go`
- `cmd/truelayer-demo/obligations.go`
- `cmd/truelayer-demo/tenant_detail.go`
- `cmd/truelayer-demo/dashboard.go`
- `cmd/truelayer-demo/main.go`
- `cmd/truelayer-demo/*_test.go`

## 风险与回滚点

| 风险 | 防护 |
| --- | --- |
| 现金被伪造成银行交易 | 使用独立 `cash_receipts` 表，禁止写入 `payment_transactions` |
| 并发补录超额 | 事务内锁 `rent_obligations` 并从有效来源重算余额 |
| 作废误扣银行收款 | 只将目标现金行改为 `voided`，再重算同一账单 |
| 预览后账单余额变化 | 提交时重新加载并校验最新账单，不信任预览值 |
| 重复提交重复入账 | 用户范围 `idempotency_key` 唯一约束和服务层幂等读取 |
| 越权查看或修改 | 所有资源查询及写入包含 `user_id`，不存在安全错误不回显 |

## 完成门槛

- [ ] PRD 验收标准均有单元、MySQL 或 HTTP 证据。
- [ ] 通过 Trellis quality check，更新跨层数据库规范。
- [ ] 当前任务代码无未提交变更，形成独立 commit、push 并归档。
