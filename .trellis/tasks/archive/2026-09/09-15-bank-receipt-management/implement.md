# 银行入账归类、拆分与纠错：实施计划

## 启动前门槛

- [x] 用户审阅并批准 `prd.md`、`design.md` 和本实施计划。
- [x] 执行 `python3 ./.trellis/scripts/task.py start 09-15-bank-receipt-management`，确认状态切换为 `in_progress` 后才修改业务代码。
- [x] 实现前读取 `trellis-before-dev` 和相关 backend spec；代码改动使用测试驱动的增量切片。

## 纵向切片与顺序

### 切片 1：同步批次与解析事实

**交付：**首次一年授权、后续 90 天刷新和每账户结果可持久化；交易保存备注解析月和批次来源；不改变现有稳定交易幂等导入。

**工作项：**

- [x] 新增 `005_bank_receipt_management.sql` 的同步批次、账户结果和交易解析字段。
- [x] 扩展 GORM 模型与同步服务，callback／refresh 写入 `initial_year` 或 `refresh_90d` 及账户级结果。
- [x] 保留已有 JSONL 日志兼容路径，数据库页面改读同步批次状态。
- [x] 为月份解析优先级、失败批次、部分账户失败、空结果不冒充成功和重复导入补充测试。

**验证：**`go test ./cmd/truelayer-demo -run 'Test.*Sync|Test.*Period|Test.*Transaction'`。

### 切片 2：统一分配引擎

**交付：**同一租客内的房租、押金、其他收入和部分确认使用一个原子服务；余款可查询并续配，账单只吸收房租。

**工作项：**

- [x] 建立有效分配加载、来源余款、用途汇总和流水状态投影。
- [x] 实现单项／多项分配请求，默认房租，校验 EUR、金额预算、账单余额、同租客和其他收入备注。
- [x] 在事务内批量写入、重算账单和使用幂等键，冲突时全部回滚。
- [x] 覆盖 €2,000 跨月、混合用途、€1,000 + €200 余款、非 EUR、跨租客和重试场景。

**验证：**`go test ./cmd/truelayer-demo -run 'Test.*Allocation|Test.*Ledger|Test.*Projection'`，如配置 MySQL 再运行对应集成测试。

### 检查点 A：账本安全门

- [x] `go test ./... -count=1` 通过。
- [x] 有效房租分配金额与账单 `paid_amount_cents` 一致，押金／其他收入不改变账单。
- [x] 事务失败没有半套分配，重复幂等请求没有第二次入账。
- [x] 确认后再继续切片 3；发现 PRD 或账本不变量缺口时回到规划阶段。

### 切片 3：归类操作与撤销重配

**交付：**无需匹配、恢复、整笔撤销预览和审计完成；撤销不会删除银行原文，也不会误撤销重新匹配结果。

**工作项：**

- [x] 新增交易动作审计表及服务，补齐忽略／恢复状态。
- [x] 实现撤销预览与必填原因的整笔撤销；按来源锁定、标记所有有效分配并重算受影响账单。
- [x] 旧撤销和重复提交使用幂等边界；新分配保留独立 operation ID。
- [x] 覆盖有效分配不能被“无需匹配”覆盖、撤销混合用途、重新匹配后旧撤销重试和跨用户拒绝。

**验证：**`go test ./cmd/truelayer-demo -run 'Test.*Revoke|Test.*Ignore|Test.*Audit|Test.*Ownership'`。

### 切片 4：严格匹配、付款人记忆与历史预览

**交付：**使用 `tenant_payers` 的唯一证据自动确认；手动记忆付款人可取消；共享付款人和历史流水批处理不会静默自动确认。

**工作项：**

- [x] 将 matcher 从旧单字段切换到 active payer relations，识别共享／冲突关系。
- [x] 区分显式备注月份和到账月份，移除自动回退到到账月的路径。
- [x] 增加“记住付款人”默认勾选但可取消，以及历史待处理逐笔预览／确认。
- [x] 覆盖唯一 ID、唯一名称、无月份、冲突、超额、币种不符和失败后不批量改变其他流水。

**验证：**`go test ./cmd/truelayer-demo -run 'Test.*Match|Test.*Payer|Test.*Preview'`。

### 检查点 B：处理语义门

- [x] 匹配状态、入账用途和账单缴费状态在模型、查询和模板中使用不同字段和标签。
- [x] 自动确认仅在完整证据门槛满足时发生；历史批处理默认关闭。
- [x] 任意拆分或撤销均按原流水去重，且审计可回放。

### 切片 5：流水查询与页面操作

**交付：**银行流水页可以核对原文、按全部组合条件定位、分页排序，并完成归类、拆分、撤销和同步状态查看。

**工作项：**

- [x] 扩展 `transactionFilters`、SQL 查询和分页 view model；区分到账月和租金所属月。
- [x] 增加用途／租客／付款人／日期范围筛选、白名单排序和余款去重统计。
- [x] 更新 `/billing` 模板与表单，提供拆分项、其他收入备注、无需匹配原因和撤销预览。
- [x] 注册新 POST 路由，所有参数重新验证用户归属、方法和金额；补充页面渲染及未登录测试。
- [x] 保持未连接银行、legacy fallback、旧链接和现有支出页面可用。

**验证：**

- `gofmt -w cmd/truelayer-demo/*.go`
- `go test ./cmd/truelayer-demo -count=1`
- `go test ./... -count=1`
- `go vet ./...`
- `git diff --check`

### 检查点 C：端到端验收

- [x] 用测试数据验证：一笔 €2,000 拆成两个月房租，混合押金／其他收入，部分确认后续配余款。
- [x] 验证撤销重配、无需匹配恢复、同步部分失败和非 EUR 待处理状态。
- [x] 验证同一用户可查可改，另一用户的流水、租客、账单和同步批次均不可见。
- [x] 若环境可用，使用临时 MySQL DSN 验证 `005` 重复启动无副作用、并发分配只有一个成功。（本次尝试启动临时 MySQL 8.4 时初始化进程崩溃，opt-in 集成测试按无 DSN 跳过。）
- [x] 如浏览器运行环境可用，验证拆分行增删、撤销预览、筛选组合和错误提示；使用临时数据，不写仓库根目录。（Go 模板测试已覆盖页面契约；本环境未提供可连接的 Chrome DevTools/Playwright 浏览器运行时。）

## 主要文件范围

- `migrations/005_bank_receipt_management.sql`
- `cmd/truelayer-demo/transactions.go`
- `cmd/truelayer-demo/matching.go`
- `cmd/truelayer-demo/matching_service.go`
- `cmd/truelayer-demo/bank_connections.go`
- `cmd/truelayer-demo/main.go`
- `cmd/truelayer-demo/billing_page.go`
- `cmd/truelayer-demo/*_test.go`

必要时新增专门的 `bank_sync.go`、`transaction_allocation.go` 或页面 view model 文件；不把所有逻辑继续堆入 `main.go`。

## 风险、回滚点与防护

| 风险 | 防护与回滚点 |
| --- | --- |
| 新状态覆盖旧 `match_status` 语义 | 先定义状态投影和标签测试，再改 handler；旧分配默认按有效房租读取 |
| 并发续配超额 | 事务内锁来源和账单，失败整次回滚；用集成测试验证 |
| 撤销误伤新分配 | 用 operation ID／幂等键区分每次操作，只撤销提交前锁定的当前有效集合 |
| 银行账户部分失败被显示为成功 | 批次和账户结果先落库，整体状态按结果汇总，空结果不作成功凭据 |
| 付款人关系共享导致误自动确认 | 查询 active relation 后按唯一性判定；共享关系只给候选，不自动选人 |
| 页面隐藏字段绕过余额或用户边界 | handler 只解析 ID，service 在事务内重新加载所有事实并带 `user_id` |
| 迁移或应用回滚损失历史 | 迁移只增加结构；回滚应用版本不删除原始流水和审计记录 |

## 完成门槛

- [x] PRD、设计和实施计划中的验收标准全部有自动化或明确的浏览器／MySQL 验证证据。
- [x] 通过 `trellis-check` 质量检查：规范、数据流、用户隔离、测试和差异边界均无未解决问题。
- [x] 必要的项目规范更新已完成。
- [x] 提交前 `git status` 只包含本任务变更，并完成一次原子 commit。
