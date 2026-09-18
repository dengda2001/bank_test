# 设计：可复现的浏览器验收环境

## 1. 边界

**做**：把验收从「依赖生产库 + 已丢失的 /tmp 工件」变成「仓库里一条命令跑出来」。

**不做**：不改产品代码、不改模板、不改路由、不改业务规则。本任务不产生任何进入生产二进制的代码。

---

## 2. 核心决策

### D1：seeder 走 HTTP，不写 Go 二进制

候选是「在 `cmd/truelayer-demo` 加一个 seed 开关」或「新建 Go seeder」。两者都被否决，原因是 Go 侧**没有任何可复用的东西**：

- `cmd/truelayer-demo` 是 `package main`，外部无法 import。
- 在它内部加 seed 开关意味着**给生产二进制加一条写数据的代码路径**——为了一个只用于验收的工具，扩大生产攻击面，不划算。

而应用暴露的写入口本来就是 HTTP：`POST /import-legacy`、`/tenants`、`/billing/confirm`、`/billing/allocate`、`/billing/ignore`、`/billing/revoke`、`/cash-receipts`、`/expenses`。harness 本来就是 Node/Playwright，seeder 用 Node 写没有引入新运行时。

**这个选择还有一个正确性上的收益**：走真实端点造出来的状态，必然是用户实际能走到的状态。直接写 SQL 可以造出界面到不了的组合，那样验的是不存在的页面。

### D2：夹具分两层，各司其职

| 层 | 载体 | 负责 |
| --- | --- | --- |
| 静态夹具 | 提交进仓库的 `bank-results.jsonl` / `tenants.json` / `expenses.json` | 租客（含长中文名、长地址）、银行流水（含 CREDIT 与 DEBIT、长英文描述、大小额、跨月描述）、支出记录 |
| 动作脚本 | `seed.mjs` 打真实端点 | 由静态夹具的 `unmatched` 出发，走出 `matched` / `partial` / `ignored` / 现金收款 / 催缴 |

静态层用既有的 `POST /import-legacy`：它读 `TL_LOG_FILE` / `RENTOPS_TENANT_FILE` / `RENTOPS_EXPENSE_FILE`（`legacy_import.go:64,90`），且**幂等**（`ON CONFLICT (user_id, stable_transaction_key) DO NOTHING`，`transactions.go:150-155`）。请求体被忽略，所以文件必须在应用启动时就位——这是脚本的职责。

### D3：能造与造不出的分支，逐条钉死

| 分支 | 可达 | 手段 |
| --- | --- | --- |
| `unmatched` | 是 | import 后不动作 |
| `matched` | 是 | `/billing/confirm` 全额 |
| `partial` | 是 | `/billing/allocate` **少分配**（余款 > 0 → `partial`，`transaction_allocation.go:43-50`） |
| `ignored` | 是 | `/billing/ignore` |
| 支出行（`Direction=expense`） | 是 | 夹具里的 DEBIT 流水 |
| dashboard `paid` / `unpaid` / `overdue` | 是 | 全额匹配 / 不匹配 / 过期不匹配 |
| dashboard `partial` | 是 | 少分配 |
| dashboard `open` | 是 | 到期日晚于运行日的租约 |
| dashboard `needs_review` | **否** | 需要 `rent_obligations.status = "needs_review"`，无写入路径（`obligations.go:280,371`）|
| `/billing` `candidate` / `needs_review` | **否** | 无任何写入路径能持久化这两个值（`transaction_actions.go:55-67`、`transactions.go:137`、`dashboard_manual_balance.go:98` 是全部写入点）|
| 空列表 | 是 | 第二个账号不带数据；或用无命中的筛选 |

两条「造不出」是 PRD R3 明确要求书面说明的，必须同时写进验收报告与最终交付说明——**不能让读者以为这两条被静默略过**。

### D4：harness 从归档目录提升为仓库资产

现状：29 个脚本躺在 `.trellis/tasks/archive/2026-09/09-16-mobile-responsive-audit/research/harness/`，而 `.trellis/spec/frontend/responsive-conventions.md:244` 指向的是**没有 archive 前缀的路径**——那个路径根本不存在。

把 harness 提升到 `scripts/audit/`：

- 修掉规范里的失效路径（规范引用的是工具位置，工具不该住在会被归档的任务目录里）。
- 让它成为可被后续三个子任务直接调用的仓库资产。
- 避免「任务归档 → 工具消失 → 验收再也跑不了」的循环，这正是上一轮的病根。

原归档副本保留不动（历史证据），`scripts/audit/` 是唯一维护点。

### D5：Chromium 用一个入口解析

约 20 个脚本把路径写死成 `/tmp/mobile-audit/chrome-linux64/chrome`。统一改为一个共享的 `launch.mjs`：

1. 读 `CHROME_BIN`；为空时依次探测 `/Applications/Google Chrome.app/Contents/MacOS/Google Chrome`、`chromium`、`google-chrome`。
2. 都找不到就**报错退出并说明如何设置 `CHROME_BIN`**，不静默降级。
3. Playwright 用 `channel: 'chrome'` 或 `executablePath`。

本机已核实存在 `/Applications/Google Chrome.app`，node/npm 可用。

### D6：安全闸门沿用既有先例

`scripts/run-e2e-local.sh` 已经建立了正确做法，直接沿用其形状：

- 库名必须匹配 `rentops_audit_*` 前缀，否则拒绝运行（对应 `run-e2e-local.sh:38-41` 的 `rentops_e2e_*` 检查）。
- 拒绝在 `MYSQL_HOST` 指向非本地时运行。
- **明确拒绝** `:8081` / `bank.ddpl.top`：脚本不接受任何指向已存在实例的 base URL，只接受自己刚起的那个端口。
- 用完即 drop；失败时保留并打印可复现的路径（沿用 `:154-167`）。

### D7：产物不落进工作树

报告与截图默认写 `AUDIT_OUT`（默认在系统临时目录），不进仓库。截图体积大（上一轮约 14MB），不提交。只有夹具、seeder、脚本进仓库。

---

## 3. 组件

```
scripts/
  run-audit-local.sh          起一次性 DB + 应用 + 导入夹具 + 跑 seed + 保持运行 + 收尾
  audit/
    launch.mjs                Chromium 解析（唯一入口）
    seed.mjs                  动作层：打真实端点造 matched/partial/ignored/现金/催缴
    audit.mjs                 主审计（8 页 × 3 视口）
    audit-extra.mjs           首轮遗漏的 2 个预览页
    verify-*.mjs              断点校验（命中区、裁切、溢出）
    README.md                 env 变量、命令序列、已知限制
test-data/audit/
  bank-results.jsonl          CREDIT + DEBIT、长描述、大小额、跨月
  tenants.json                长中文名、长英文名、长地址、到期日差异
  expenses.json               各金额与类别，含长备注
```

`run-audit-local.sh` 与前作的差别：**不跑破坏性的验收套件**。它起库、起应用、导入、seed，然后**保持应用运行**并把 base URL 与账号打印出来，等 harness 连上来；harness 跑完由脚本收尾。这是研究里指出 `run-e2e-local.sh` 做不到的事——那个脚本的实现在 `executeE2ERun` 里把数据删光后才退出（`cmd/rentops-e2e/execution.go:62`）。

---

## 4. 兼容性与回滚

- 不改产品代码 → 生产二进制零风险。
- 新增的都是新文件（`scripts/audit/`、`test-data/audit/`），唯一被修改的既有文件是归档 harness 的副本（提升为 `scripts/audit/`）与规范里的一行失效路径。
- 回滚即删除新增目录。

---

## 5. 未决

- `dashboard needs_review` 与 `/billing candidate/needs_review` 两条不可达分支：本任务只负责**证明并记录**不可达。是否值得为它们补一条写入路径是产品决定，不在本任务范围（会改业务代码）。
