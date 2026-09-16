# 真实 API 全链路验收：实施清单

## 前置安全门槛

- [x] 从当前路由和服务代码生成业务接口清单 `api-inventory.md`。
- [x] 确认现有认证入口、数据库配置和无现成 E2E runner/清理脚本。
- [x] 配置并确认可丢弃的非生产 MySQL 目标、账户播种方式和受控查询方式；封装为 `scripts/run-e2e-local.sh`。
- [x] 确认本轮不验收真实投递：`RENTOPS_E2E_SKIP_DUNNING_DELIVERY=1` 跳过 `/dunning/send` 并将其标记为未验收。

## 纵向执行切片

### 切片 1：安全预检、认证和数据清单

- [x] 实现运行标识、目标 allowlist 预检和脱敏机器报告；dry-run 不连接目标。
- [x] 实现同一 HTTP cookie 会话；通过本地 HTTP 测试验证登录后的受保护请求复用 session。
- [x] runner 通过 `/login-local` 登录唯一隔离账户，断言 v2 session 和未授权拒绝；本地 httptest 已验证。
- [x] 生成可复核的 runID 前缀 fixture/expected manifest，不把密码、DSN 或 token 写入报告。

### 切片 2：账本业务 HTTP 验收

- [x] 通过 HTTP 创建/读取租客和创建/读取付款人关系。
- [x] 通过 HTTP 更新租客档案并用详情接口验证更新结果。
- [x] 通过受控原始流水导入创建唯一流水，并验证 EUR、非 EUR、跨月、待处理和重复导入幂等。
- [x] 通过 HTTP 完成匹配、分类、拆分、续配、撤销和重复请求，并校验最终金额守恒。
- [x] runner 通过第二个隔离账户验证跨用户 ID 拒绝；本地 httptest 已验证。
- [x] 通过 HTTP 完成现金预览、入账、幂等、作废、更正、非 EUR 拒绝及余额竞争路径。
- [x] runner 对场景记录响应，并交叉读取 Dashboard、历史和流水汇总，断言金额守恒；本地 httptest 已验证。

### 切片 3：Dashboard/催缴、报告和清理

- [x] runner 验收 Dashboard 查询/筛选/分页、历史和催缴候选/预览/发送/幂等/隔离；本地 httptest 已验证。
- [x] 失败断言停止后续写入并保留 runID、请求、预期/实际和错误报告；本地 httptest 已验证。
- [x] 实现全部通过后的 allowlist 清理、事务删除，以及清理后 HTTP + 受控查询双重零残留验证；已在真实隔离库执行并通过。
- [x] 已记录验收命令和环境限制，禁止把 skipped 当作 passed。
- [x] 新增「只跳过真实投递」开关：拆分配置／预览与投递两个场景，跳过时报告 `skipped` 并写入 `unverified`。

## 质量门禁

- `go test ./... -count=1`
- `go vet ./...`
- `git diff --check`
- runner dry-run 只能验证配置和计划，不得连接或写入未授权目标。
- 有真实环境时执行完整 runner，并保存机器可读 JSON 报告；无环境时保持任务未完成。

## 当前实现与 live gate

runner 工程实现已完成，并在本机隔离环境完成一次全量真实运行：

- `go test ./... -count=1`
- `go vet ./...`
- `git diff --check`
- `bash scripts/run-e2e-local.sh` → `run status: passed`（16 场景：15 passed、1 skipped）

运行标识 `rentops-e2e-20260916-032709-60dd6b72`，报告 `status: passed`，
`cleanup` 的 `verified`／`http_verified`／`fixture_removed` 均为真；
运行结束后一次性库与账号已删除，受控查询确认零残留。

`-execute` 的完整前置条件如下（`scripts/run-e2e-local.sh` 已自动满足除 SMTP 之外的全部项）：

- `RENTOPS_E2E_BASE_URL` 指向运行中的应用 origin；远程目标还必须显式设置 `RENTOPS_E2E_ALLOW_REMOTE=1`。
- `RENTOPS_E2E_TARGET_NAME` 与 `RENTOPS_E2E_TARGET_ALLOWLIST` 完全相同，且明确标识为非生产目标。
- `RENTOPS_E2E_MYSQL_DSN`（或现有测试 DSN fallback）连接可删除的库；`RENTOPS_E2E_DATABASE_ALLOWLIST` 必须等于 DSN 数据库名，runner 会再执行 `SELECT DATABASE()` 复核。
- `RENTOPS_E2E_USERNAME` 必须以本轮 `runID` 开头，由应用启动配置播种；`RENTOPS_E2E_SECOND_USERNAME/PASSWORD` 为同库中预先播种的另一测试账户。
- `RENTOPS_E2E_FIXTURE_DIR` 是 runner 独占的空目录；应用的 `TL_LOG_FILE`、`TL_TENANT_FILE`、`TL_EXPENSE_FILE` 指向其中三个文件。
- `RENTOPS_E2E_SMTP_SINK` 已指向受控 sink，且应用自身的 `DUNNING_SMTP_*` 配置不会把邮件发送到真实收件人。
- `RENTOPS_E2E_CONFIRM_WRITES=I_UNDERSTAND_NON_PRODUCTION` 与 `RENTOPS_E2E_CONFIRM_CLEANUP=I_UNDERSTAND_DELETE_RUN_ID_ONLY` 均显式设置。

运行命令为 `go run ./cmd/rentops-e2e -execute -report <report-path>`。目标探针失败时不会创建 fixture；业务断言失败时会保留 fixture 和报告且不会清理；数据库清理后还会用第二账户通过 HTTP 验证租客详情、租客列表和流水列表不再暴露本轮数据；只有数据库清理、HTTP 零残留验证及本地 fixture 删除都完成后报告才会是 `passed`。

## 环境

- 本机 Linux，MySQL 8.0.45 监听 `127.0.0.1:53306`，`sudo mysql` 免密；早先记录的
  macOS 阻塞（无 Docker/MariaDB、Homebrew MySQL 初始化 SIGSEGV）在本机不成立。
- 隔离方式为「一次性库 + 专属账号」，既有 `rentops` 等非本次运行的库从未被读写。
- 失败运行会保留一次性库并在输出中打印精确删除命令；每次重跑前应确认
  `SHOW DATABASES` 与 `mysql.user` 中 `rentops_e2e_*` 已清空。

## 本轮真实运行暴露的断言／查询缺陷

真实环境第一次让下列 4 处「看着合理但与应用契约不符」的断言暴露出来，均已修复：

1. **银行流水号应比对规范化 provider id**：入库与 `/billing` 展示的是
   `NormalisedProviderTransactionID`（`transactions.go:110`），runner 原先断言原始
   `ProviderTransactionID`。新增 `e2eBankTransactionFixture.StoredProviderTransactionID()`，
   并统一替换全部回读与清理校验点。
2. **现金预览金额渲染格式**：预览页把本次现金渲染为 `350.00 EUR`（`{{.Amount}} {{.Currency}}`），
   余额卡片才是 `EUR 0.00`；原先断言 `EUR 350.00` 永不成立。改为按真实渲染断言，并把子断言拆开上报。
3. **Dashboard 银行口径按流水日期归期**：其他收入归类挂在 9 月流水上，8 月口径必然为 0。
   改为 8 月断言 `EUR 0.00`，另加 9 月读断言《其他收入 = EUR 50.00》覆盖期间隔离。
4. **清理归属校验的期间格式**：`loadE2ECleanupObligations` 使用 `%Y-%m-%d`，而所有期望键
   使用 `%Y-%m`，导致义务键永不匹配（并连带使分配／现金／催缴查找全部落空）。修正为 `%Y-%m`，
   同时把失败原因写入清理报告，避免只报「cleanup ownership validation failed」。
5. **清理后 HTTP 零残留扫描误报**：应用外壳渲染操作员账号名（`当前用户：{{.Username}}`），
   而该用户名本身以 run ID 为前缀，裸 run ID 标记因此把外壳文案当成残留。扫描前先剔除两个
   账号名；真实业务残留仍会被检出（新增单测覆盖两个方向）。


## 回滚/停止点

- 任何目标标识、账户前缀、外键关系或清理查询不匹配时立即停止，不扩大删除范围。
- 任意接口断言失败时保留隔离数据和报告，先诊断再决定是否单独运行 allowlist 清理。
- 不修改生产配置，不复用已有用户，不使用无运行前缀的测试实体。
