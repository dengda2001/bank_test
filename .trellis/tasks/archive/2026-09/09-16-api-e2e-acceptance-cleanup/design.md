# 真实 API 全链路验收：技术设计

## 目标与安全边界

本任务使用一次性运行标识 `runID` 驱动的 HTTP E2E runner，对运行中的
RentOps 应用进行接口级验收。runner 只允许在显式配置的非生产 MySQL 目标上运行；
没有目标、认证配置或安全查询能力时，预检失败且不执行写请求。

## 运行方式

- runner 使用标准库 HTTP client，维护同一 cookie 会话，从 `/login-local` 开始。
- 应用以本轮唯一 `APP_ADMIN_USERNAME`、`APP_ADMIN_PASSWORD` 启动，启动时由现有
  `auth.seedDefaultUser` 在隔离库播种账户；账户创建并非通过不存在的用户注册路由。
- 跨用户断言使用隔离库中预先播种的第二测试账户；runner 只通过 `/login-local` 登录它，
  不通过直接改库创建或切换用户。
- 催缴发送必须配置 `RENTOPS_E2E_SMTP_SINK` 标识，并由应用自身的 SMTP 配置指向该 sink；
  显式设置 `RENTOPS_E2E_SKIP_DUNNING_DELIVERY=1` 时改为记录一个跳过场景（见下）。
- `MYSQL_DSN`/`DATABASE_URL` 只从进程环境读取，不写入报告；报告保存脱敏目标标识。
- legacy 输入使用 `RENTOPS_E2E_FIXTURE_DIR` 指定的专用空目录；runner 只独占创建
  `bank-results.jsonl`、`tenants.json` 和 `expenses.json`，目录或文件已存在时停止。
- `runID` 使用时间戳加随机后缀，并作为用户名、租客、付款人、稳定交易键、描述、
  幂等键、请求号和催缴配置的前缀。
- 原始银行流水通过受控的 legacy JSONL 输入和 `/import-legacy` 进入应用；租客、
  付款人关系、账单操作、分配、分类、撤销、现金和催缴均走对应 HTTP 路由。

## 数据与断言

runner 为每个场景建立 typed fixture 和 expected snapshot，所有金额以 integer cents
和明确币种比较。每个请求记录：方法、路径、脱敏请求摘要、状态码、响应摘要、预期、
实际和断言结果。失败时保留报告与 `runID`，不得自动删除以外的记录。

场景按依赖顺序执行：认证 → 租客/付款人 → 原始流水导入 → 账单与自动/人工匹配 →
分类/拆分/续配/撤销 → 现金补录/作废/更正 → Dashboard/历史读取 → 催缴预览/发送
（发送使用受控 SMTP sink）。每个写请求之后立即读取相关列表、历史或汇总接口交叉验证。

## 清理策略

- 只有所有断言成功后才进入清理；失败时保留运行数据和报告。
- 清理前执行 allowlist 预检：目标数据库标识、测试用户名精确匹配、所有业务文本均以
  `runID` 开头；任一不匹配立即停止。
- 优先通过应用已有业务接口撤销/作废可逆状态；最终删除仅限本轮账户及其外键级联数据，
  并使用受控查询确认用户、租客、付款人、账单、交易、分配、现金、催缴记录为零。
- 清理逻辑不得接受任意用户输入的表名、where 子句或无前缀的删除条件。

## 跳过真实投递的语义

- 触发条件：显式设置 `RENTOPS_E2E_SKIP_DUNNING_DELIVERY=1`。
- 影响范围仅限 `/dunning/send` 及其重复提交；催缴候选发现、发件配置和预览仍执行并断言。
- 该场景以 `status: "skipped"` 记录，同时写入报告顶层的 `unverified` 数组（含原因），
  并在清理归属校验中把预期的投递尝试数从 1 改为 0。
- 报告 `status: "passed"` 只表示「所有已执行断言通过」；未设置该开关且缺少 SMTP sink 时，
  预检失败且不产生任何写请求。`skipped` 永远不会被计为 `passed`。

## 本地隔离运行

外部阻塞决策（可丢弃 DSN、账户播种、受控查询）通过 `scripts/run-e2e-local.sh` 在本机落实：

- 由运行标识派生 `rentops_e2e_<run-id>` 库与 `rentops_e2e_<hex>` 账号，非 `rentops_e2e_*`
  前缀直接拒绝执行；运行结束删除库与账号，失败时打印精确的手工删除命令。
- 账户播种复用应用自身的 `seedDefaultUser`：先后以两个不同 `APP_ADMIN_USERNAME` 启动应用，
  得到主、次两个隔离账户，不直接改库建用户。
- fixture 目录权限要求 `0700`；应用日志、租客与支出文件置于该目录，token 文件在其外。
- 运行门禁为 `go test ./... -count=1`、`go vet ./...`、`git diff --check`，随后以
  `-execute` 跑完整 runner 并保存 JSON 报告。

## 已落实的决策

1. 可丢弃的非生产 MySQL DSN：本机一次性库，允许写入与删除，仅限 `rentops_e2e_*`。
2. 应用启动/登录方式：应用自身启动播种 + `/login-local`。
3. SMTP sink：本轮不验收真实投递，显式跳过并标记未验收。
4. 报告存储位置：运行时的临时目录，失败时保留并在输出中给出路径。
