# 执行计划：删除无库降级 dashboard 模板

> 前置：`prd.md`（需求与验收）、`design.md`（边界与依据）。
> 本计划里带 **⛔ 门** 的步骤是停机点，不过门不许往下走。

---

## 0. 开工前

- [ ] 已加载 `trellis-before-dev`
- [ ] 确认前置：`09-19-desktop-contract-1100` 已归档。
      两边都会改 `mobile_layout_test.go` 与 `scripts/audit/`，同时动会打架。
- [ ] 确认工作区状态：`git status --porcelain | wc -l`，把**不属于本任务**的脏文件记下来。
      **不清理**、不 `checkout`、不 `stash`、不 `clean`。

---

## 1. 先核实两件事（写代码之前）

- [x] **1.1 核实 `dunning_handlers.go:82` 的调用链是否先鉴权。** —— **已通过，2026-09-19 开工前查证。**
      `handleDunningAction`（`dunning_handlers.go:61`）三道都过：`requireAuth`（`:62`）、
      `a.db == nil` → 503（`:65`）、`currentUserID` 无则 401（`:69`）。
      ⇒ 四个调用点全部先鉴权，`design.md` §4 的结论成立，`else` 分支确实死，本任务**不动它**。
      ⇒ 另外：`:65-68` 就是"无库时明确报错"的现成范例，§3.1 照它的样式写。

- [ ] **1.2 抓改动前的基线输出。**
      对 `/bills` 与 `/dunning` 各渲染一份同一输入的 HTML，存到 `/tmp/before-bills.html`、
      `/tmp/before-dunning.html`。这是 AC「逐字节不变」的对比基准。
      用法：跑现有 HTTP 测试并把响应体落盘，或起本地实例 `curl` 后保存。

- [ ] **1.3 记录基线行数**：`wc -l cmd/truelayer-demo/dashboard.go`（预期 569）。

---

## 2. 删模板本体

- [ ] **2.1 列出 `rentDashboardTemplate` 的 funcmap 所有键**，逐个判断底层 Go 函数是否被别处调用。
      - 只被这个 funcmap 用的 → 连函数一起删
      - 被 `rentCollectionPageFuncs` 或测试直接调用的 → **只删 map 里的条目，函数留下**
      - 依据：`billsPageTemplate` / `dunningPageTemplate` 各自携带 `rentCollectionPageFuncs`，
        与这个 funcmap 无共用关系，所以删 map 不会波及它们。**但要逐个确认，不要靠推断。**
- [ ] **2.2 删除 `rentDashboardTemplate` 变量**（含内联 raw string HTML 与 funcmap 字面量）。
- [ ] **2.3 校验清零**：`/usr/bin/grep -rn 'rentDashboardTemplate' --include='*.go' .` → 期望无输出。

---

## 3. 收敛 `db == nil` 入口与模板选择

- [ ] **3.1 改 `handleRentDashboard`（`dashboard.go:11-20`）**：
      把「无库 → `renderRentDashboard(w, r, nil)`」改成**明确报错**（如 503 + 一行可读信息），
      而不是继续降级。
- [ ] **3.2 保持 `renderRentDashboard` 的签名与函数体不动**（它是 `/bills`、`/dunning` 的活渲染体）。
- [ ] **3.3 收敛 `switch`（`dashboard.go:168-174`）**：
      默认分支不再指向已删除的变量，改成**显式穷举 + 默认报错**。
      四个活调用点都保证 `r.URL.Path` 匹配前两个 case，默认分支因此不可达 —— 报错是为了让
      「不可达」这件事在代码里看得见，不是留后路。
- [ ] **3.4 校验活路径未变**：重跑 §1.2 的两个渲染，与 `/tmp/before-*.html` `diff`。
      **⛔ 门：diff 必须为空。** 非空 → 回滚 §3.3 重做。

---

## 4. 测试处置（按 `design.md` §6 的表，17 项逐个走）

对每一项，先在 `implement.jsonl` 里写一句「它原本在测什么」，再动手。**顺序不能反。**

- [ ] **4.1 改指向类**（#1、#2、#3、#5、#8、#11、#13、#14、#16）：
      载体换成活模板。**改完必须自问：这个测试现在还在断言原来那件事吗？**
      答不上来 → 它其实是模板专属测试，按 4.3 处理。
- [ ] **4.2 清单类**（#6）：只从模板清单里移除旧模板那一项，清单本身与其余项不动。
- [ ] **4.3 删除类**（#7、#9、#12，以及 4.1 里判定为专属的）：
      删除前在 `implement.jsonl` 写明原意；删除后在提交信息里能对上。
- [ ] **4.4 拆分类**（#15）：服务端重试逻辑的覆盖必须活下来，
      只删针对旧模板标记（抽屉 DOM）的断言。
- [ ] **4.5 保留类**（#17 + `dashboard_manual_balance_test.go` 整文件）：
      **一行都不许动**，除非是为了配合 §3 的签名变化。
- [ ] **4.6 ⛔ 门：自检有没有「靠削弱测试变绿」。**
      逐条回答：每个被删/被改的测试，原来在测什么？现在还有没有别的地方覆盖它？
      答不出「还有」的，要么补一个等价测试，要么留着不删。

---

## 5. 审计脚本

- [ ] **5.1 `scripts/audit/metrics.mjs:23`**：选择器 `.rent-row, tbody tr` 里的 `.rent-row` 分支
      已永久失配，收敛为活的那些。
- [ ] **5.2 `scripts/audit/seed.mjs:143-148`**：更新那段描述调用链的注释措辞。
- [ ] **5.3 确认脚本仍可跑**（本地、非生产）：
      ```bash
      MYSQL_PORT=3306 APP_PORT=18090 scripts/run-audit-local.sh
      ```
      **不要指向 `:8081` / `bank.ddpl.top`** —— 那是生产应用连生产库。

---

## 6. 验证

- [ ] **6.1** `cd cmd/truelayer-demo && go test ./... -count=1 2>&1 | tail -30`
      记下退出码：`go test ./... -count=1 > /tmp/gotest.log 2>&1; echo "GO_TEST_EXIT=$?"`
      （注意：管道里 `$?` 是最后一个命令的码，必须单独取。）
- [ ] **6.2** `go vet ./...`
- [ ] **6.3** 行数：`wc -l cmd/truelayer-demo/dashboard.go` → 目标 **200 行以内**（基线 569）。
- [ ] **6.4** 逐字节复核：`diff /tmp/before-bills.html <(重跑)` → 空；`/dunning` 同。
- [ ] **6.5** 已知失败对照：`TestDunningDashboardHTTPWorkflowOnMySQL` 在本任务之前就已失败
      （已在 `git archive HEAD` 的干净副本上独立复现）。它**不是**本任务引入的，
      但本任务也不负责修它 —— 报告时如实说明，不要算进本任务的红绿。
- [ ] **6.6** 无残留：确认没有遗留的 `rentops_audit%` 数据库或仍在监听的实例。

---

## 7. 收尾

- [ ] **7.1** 更新 spec（`trellis-update-spec`）：
      - `database-guidelines.md` 里记录「有库时该模板不可达」的那一小节 → 改为「已删除」
      - 新增一条：**催收抽屉在真实路径上缺失**（`design.md` §7 的发现），
        并指向 ③ 作为输入
      - 新增一条：`renderRentDashboard` 是 `/bills`、`/dunning` 的活渲染体 ——
        防止后来者看到函数名又以为是死代码
- [ ] **7.2** 提交（Phase 3.4）。提交信息要能读出：
      删了什么、为什么它是死的、哪些测试改指向了哪里、催收抽屉那条发现。
- [ ] **7.3** `/trellis:finish-work`。

---

## 不做的事

- 不动 `else` 分支与 `loadTenants` / `loadExpenses` / `loadLatestDemoResult`（`design.md` §4）
- 不删 `/rent-dashboard/settle` 路由、`handleDashboardManualBalance`、
  `dashboard_manual_balance_test.go`（`design.md` §5）
- 不并入 `legacyExpenseTemplate`（`design.md` §11）
- 不清理工作区里不属于本任务的未提交变更
