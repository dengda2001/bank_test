# 执行计划：可复现的浏览器验收环境

设计见 `design.md`。启动前门禁：无（本子任务最先做）。

---

## 阶段 0：把 harness 提升为仓库资产

- [ ] 复制归档 harness 到 `scripts/audit/`（源：`.trellis/tasks/archive/2026-09/09-16-mobile-responsive-audit/research/harness/`，29 个脚本 + README + package.json + package-lock.json）
- [ ] 归档副本保持不动（历史证据）
- [ ] `cd scripts/audit && npm install`（playwright ^1.63.0），确认 lock 文件可用
- [ ] 写 `launch.mjs`：Chromium 解析唯一入口（`design.md` §D5）
- [ ] 把约 20 个硬编码 `/tmp/mobile-audit/chrome-linux64/chrome` 的脚本改为用 `launch.mjs`
- [ ] 修掉 README 里指向 `/home/ubuntu/projects/bank_test`、`rentops-app-live.service` 的服务器路径，改为本机流程

**验证**：`node scripts/audit/smoke.mjs` 能在本机起 Chromium；`CHROME_BIN` 缺失时给出明确报错而不是静默失败。

**回滚点 A**：harness 可在本机启动。

## 阶段 1：静态夹具

- [ ] `test-data/audit/bank-results.jsonl`：`demoResult` JSONL 格式（`main.go:101-106`）
  - CREDIT 与 **DEBIT** 各若干（DEBIT 才能渲染 `/billing` 的支出行与 `/expenses` 之外的支出分支）
  - 长英文银行描述；金额含 950.00 / 400.00 / 25.00 / 12,345.67 之类的大额与非整数
  - 描述里带 `2026-08` / `2026-09` 触发跨月解析
- [ ] `test-data/audit/tenants.json`：`tenantRecord` 数组（`main.go:185-209`，注意 `monthly_rent` 是 `float64` 不是分）
  - 长中文姓名、长英文姓名、长地址（逐字竖排问题的诱因）
  - 至少一个到期日晚于运行日 → dashboard `open` 分支
  - 至少一个已过期未缴 → `overdue`
- [ ] `test-data/audit/expenses.json`：`expenseRecord` 数组（`main.go:211-225`），含长备注与不同类别
- [ ] 逐条对照 PRD §9 的数据要求清单核对

**验证**：字段名与类型对着 `main.go` 里的 struct 逐个核对，不用猜。

**回滚点 B**：夹具就位，格式正确。

## 阶段 2：一次性环境脚本

- [ ] `scripts/run-audit-local.sh`，形状沿用 `scripts/run-e2e-local.sh`：
  - [ ] 一次性 MySQL 库，名字强制 `rentops_audit_*`，不符即拒绝（`design.md` §D6）
  - [ ] 起应用，env 指向夹具：`TL_LOG_FILE` / `RENTOPS_TENANT_FILE` / `RENTOPS_EXPENSE_FILE` / `MYSQL_DSN` / `MIGRATIONS_DIR`
  - [ ] seed 账号（`APP_ADMIN_USERNAME`/`APP_ADMIN_PASSWORD` → `auth.go:37 seedDefaultUser`）
  - [ ] 等就绪（沿用轮询 `GET /login-local` 的做法）
  - [ ] `POST /import-legacy` 导入静态夹具
  - [ ] 跑 `seed.mjs` 造动作状态
  - [ ] **保持应用运行**并打印 base URL / 账号 / 退出方式——这是与 `run-e2e-local.sh` 的关键差别
  - [ ] 收尾：drop 库；失败时保留并打印路径
- [ ] 第二个空账号用于空列表分支

**验证**：脚本能在本机跑通并停在「应用运行中」状态，curl 能拿到登录页。

**回滚点 C**：环境可起。

## 阶段 3：动作层 seeder

- [ ] `scripts/audit/seed.mjs`：登录后按 `design.md` §D3 的表逐条造状态
  - [ ] `/billing/confirm` → `matched`
  - [ ] `/billing/allocate` 少分配 → `partial`
  - [ ] `/billing/ignore` → `ignored`
  - [ ] 留若干不动作 → `unmatched`
  - [ ] `/cash-receipts/preview` + `/cash-receipts` → 现金收款；`/cash-receipts/void` → 作废
  - [ ] `/expenses` → 支出记录（若静态夹具已覆盖，则只补交互所需的）
  - [ ] 触发 dashboard 惰性生成租金义务（`obligations.go:164-201` 是按页加载惰性生成的——需先 GET 一次）
- [ ] **幂等**：重复运行不产生重复数据（静态层靠 `ON CONFLICT`，动作层需自己判断目标状态已达成则跳过）

**验证**：跑完 `seed.mjs` 后，逐条 curl 各页并确认 §D3 表中每一个「可达」分支真的渲染出来了。

**回滚点 D**：全部可达分支有数据。

## 阶段 4：端到端验证与文档

- [ ] 按 PRD R3 产出「形态分支 → 命中的数据」对照表
- [ ] **跑两次**，比对两次 `report.json` 结论一致（这就是可复现的判据）
- [ ] 反向验证：库名不符时脚本确实拒绝运行
- [ ] `git status` 干净：报告与截图不落进工作树
- [ ] `scripts/audit/README.md` 写全：env 变量、从空仓库到出报告的命令序列、已知限制（含两条不可达分支）
- [ ] 报告落 `research/`

---

## 高风险点

| 风险 | 说明 |
| --- | --- |
| `/import-legacy` 忽略请求体 | 夹具必须在应用**启动时**就位；路径配错会静默 seed 空数据（`readJSONFile` 返回 nil，`main.go:1983-1998`）|
| dashboard 义务是惰性生成的 | 不先 GET 一次页面就没有 obligation 可分配，`/billing/confirm` 会失败 |
| 夹具值被 run-ID 前缀污染 | 那是 e2e manifest 校验的行为（`manifest.go:230-280`）；本方案不走 e2e manifest，不受影响——但也意味着**校验安全网要自己写** |
| 20 个脚本的 Chromium 路径 | 逐个改，漏一个就会在跑到一半时失败；用 grep 收口，不靠人工记忆 |
| 把 harness 提升后与原副本漂移 | `scripts/audit/` 是唯一维护点，README 里写明归档副本不再维护 |

## 完成判据

- 从干净 checkout 出发，按 README 命令能起环境并出报告，全程不碰 `:8081` / `bank.ddpl.top`。
- §D3 表中全部「可达」分支有数据命中，两条「不可达」有书面理由。
- 连跑两次结论一致。
- `go test ./...` 仍全绿（本任务不应改动任何 Go 文件）。
