# PRD：可复现的浏览器验收环境

父任务：`09-16-mobile-friendly-workspace`（移动端友好版工作台）

## Goal

让移动端改造的浏览器验收**可以从仓库复现**。

上一轮（`09-16-mobile-responsive-audit`）的审计报告写得很细，但**没有人能重跑它**：审计用的是生产库 `rentops-demo` 账号的真实数据，仓库里没有任何 seed；harness 依赖的 Chromium 解压在 `/tmp/mobile-audit/`（重启即失，现已不存在），`node_modules` 也没提交。结果是一份无法复核、无法回归的证据。

本任务补齐这个缺口，它是后续三个子任务共同的验收手段。

## 依赖与启动前门禁

**无前置依赖，最先做。**

本子任务不产生产品代码，但后续三个子任务（`mobile-billing-cards`、`mobile-dashboard-cards`、`mobile-profile-cards`）的 PRD 都把它列为启动前门禁——它们需要在真实浏览器里验收卡片布局，而本子任务是那个能力的来源。

## Confirmed Facts

研究见父任务 `research/seed-options.md` 与 `research/verification-infra.md`。

- `cmd/rentops-e2e` 的 fixture 机制**不能直接复用**：入口全是未导出的 `package main`；`executeE2ERun` 退出前会删光自己造的数据（`cmd/rentops-e2e/cleanup.go:639-669`），`scripts/run-e2e-local.sh:154-167` 随后 drop 整个库，`KEEP_DATABASE=1` 也留不下有效数据。
- e2e 跑完静止态只剩 5 笔流水（`matched`×4 + `ignored`×1），**没有** DEBIT/支出数据、没有 `/expenses` 数据；`partial` 只是瞬态。
- `candidate` / `needs_review` 在读时由 `matching_service.go:140-163` 计算，但 `transaction_actions.go:62-64` 表明它们**可以**被持久化——所以分支覆盖要么造得出、要么必须说明造不出。
- `POST /import-legacy`（`legacy_import.go:64,90`）只产出租客 + `unmatched` 流水，造不出 allocation、`matched`/`partial`、现金收款、付款人关系。
- `RENTOPS_TENANT_FILE` / `RENTOPS_EXPENSE_FILE` 是输入路径，启动时不会自动导入（只有管理员账号会被 seed，`main.go:389`）。
- `scripts/run-e2e-local.sh` 已经能起一次性 MySQL 库 + 应用实例，并拒绝碰任何非 `rentops_e2e_*` 名字的库——这是安全起点的既有先例。
- harness 有 29 个脚本，其中约 20 个把 Chromium 路径硬编码为 `/tmp/mobile-audit/chrome-linux64/chrome`；只有 `audit*.mjs` / `desktop*` / `chrome-diff` / `imgdiff` / `probe-wide` / `verify-p1` 认 `CHROME_BIN`。
- 本机有 `/Applications/Google Chrome.app`，node/npm 可用（已核实）。

## Requirements

- R1 从干净的 checkout 出发，按文档命令能起一次性实例并跑出报告；不需要任何手工 SQL、不需要生产数据。
- R2 seed 必须覆盖父 PRD §9 的数据要求：长中文姓名、长英文姓名、长地址、长银行描述、金额小数、空列表、错误提示、多条付款记录。
- R3 seed 必须覆盖每一行/每张卡片的形态分支，至少：自动匹配、部分收款、逾期未缴、跨月拆分、未确认、已撤销、无匹配、空列表。**造不出的分支必须逐条书面说明原因**，不能沉默略过。
- R4 全程不接触 `:8081` / `bank.ddpl.top`。脚本必须在库名/主机名不符预期时拒绝运行（沿用 `run-e2e-local.sh` 的先例）。
- R5 harness 统一支持 `CHROME_BIN`；缺失时给出明确报错，而不是静默用错路径。
- R6 报告输出到可配置目录，默认不落进仓库工作树；截图不入库。
- R7 数据形态必须**用户可达**：优先通过应用自己的 POST 端点造状态，而不是直接写库，否则可能验出用户实际到不了的状态。哪些状态只能用 SQL 造，逐条说明理由。

## Acceptance Criteria

- [ ] 一条可复制的命令序列，从空仓库到出报告，写进 `research/`。
- [ ] 在一次性库上跑通，产出 `report.json` + 截图目录。
- [ ] 逐条列出「形态分支 → 命中的数据」，覆盖 R3 的全部条目。
- [ ] 跑两次，两次结论一致（这就是「可复现」的判据）。
- [ ] `git status` 干净——报告与截图不落进工作树，除 seed/harness 修复本身外没有多余改动。
- [ ] 一次反向验证：库名/主机名不符时脚本确实拒绝运行。

## Out of Scope

- 不改产品代码、不改模板、不改路由。
- 不做 CI 集成（只要求本机可复现）。
- 不重构归档的 29 个脚本，只修到能跑。
- 不造「用户界面上到不了」的数据来凑分支覆盖。
