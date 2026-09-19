# 执行计划：删除无库降级 dashboard 模板

> 前置：`prd.md`（需求与验收）、`design.md`（边界与依据）。
> 本计划里带 **⛔ 门** 的步骤是停机点，不过门不许往下走。

---

## 0. 开工前

- [x] 已加载 `trellis-before-dev`
- [x] 确认前置：`09-19-desktop-contract-1100` 已归档。
      两边都会改 `mobile_layout_test.go` 与 `scripts/audit/`，同时动会打架。
- [x] 确认工作区状态：`git status --porcelain | wc -l`，把**不属于本任务**的脏文件记下来。
      **不清理**、不 `checkout`、不 `stash`、不 `clean`。

---

## 1. 先核实两件事（写代码之前）

- [x] **1.1 核实 `dunning_handlers.go:82` 的调用链是否先鉴权。** —— **已通过，2026-09-19 开工前查证。**
      `handleDunningAction`（`dunning_handlers.go:61`）三道都过：`requireAuth`（`:62`）、
      `a.db == nil` → 503（`:65`）、`currentUserID` 无则 401（`:69`）。
      ⇒ 四个调用点全部先鉴权，`design.md` §4 的结论成立，`else` 分支确实死，本任务**不动它**。
      ⇒ 另外：`:65-68` 就是"无库时明确报错"的现成范例，§3.1 照它的样式写。

- [x] **1.2 抓改动前的基线输出。**
      对 `/bills` 与 `/dunning` 各渲染一份同一输入的 HTML，存到 `/tmp/before-bills.html`、
      `/tmp/before-dunning.html`。这是 AC「逐字节不变」的对比基准。
      用法：跑现有 HTTP 测试并把响应体落盘，或起本地实例 `curl` 后保存。

- [x] **1.3 记录基线行数**：`wc -l cmd/truelayer-demo/dashboard.go`（预期 569）。

---

## 2. 删模板本体

- [x] **2.1 列出 `rentDashboardTemplate` 的 funcmap 所有键**，逐个判断底层 Go 函数是否被别处调用。
      - 只被这个 funcmap 用的 → 连函数一起删
      - 被 `rentCollectionPageFuncs` 或测试直接调用的 → **只删 map 里的条目，函数留下**
      - 依据：`billsPageTemplate` / `dunningPageTemplate` 各自携带 `rentCollectionPageFuncs`，
        与这个 funcmap 无共用关系，所以删 map 不会波及它们。**但要逐个确认，不要靠推断。**
- [x] **2.2 删除 `rentDashboardTemplate` 变量**（含内联 raw string HTML 与 funcmap 字面量）。
- [x] **2.3 校验清零**：`/usr/bin/grep -rn 'rentDashboardTemplate' --include='*.go' .` → 期望无输出。

---

## 3. 收敛 `db == nil` 入口与模板选择

- [x] **3.1 改 `handleRentDashboard`（`dashboard.go:11-20`）**：
      把「无库 → `renderRentDashboard(w, r, nil)`」改成**明确报错**（如 503 + 一行可读信息），
      而不是继续降级。
- [x] **3.2 保持 `renderRentDashboard` 的签名与函数体不动**（它是 `/bills`、`/dunning` 的活渲染体）。
- [x] **3.3 收敛 `switch`（`dashboard.go:168-174`）**：
      默认分支不再指向已删除的变量，改成**显式穷举 + 默认报错**。
      四个活调用点都保证 `r.URL.Path` 匹配前两个 case，默认分支因此不可达 —— 报错是为了让
      「不可达」这件事在代码里看得见，不是留后路。
- [x] **3.4 校验活路径未变**：重跑 §1.2 的两个渲染，与 `/tmp/before-*.html` `diff`。
      **⛔ 门：diff 必须为空。** 非空 → 回滚 §3.3 重做。

---

## 4. 测试处置（按 `design.md` §6 的表，17 项逐个走）

对每一项，先在 `implement.jsonl` 里写一句「它原本在测什么」，再动手。**顺序不能反。**

- [x] **4.1 改指向类**（#1、#2、#3、#5、#8、#11、#13、#14、#16）：
      载体换成活模板。**改完必须自问：这个测试现在还在断言原来那件事吗？**
      答不上来 → 它其实是模板专属测试，按 4.3 处理。
- [x] **4.2 清单类**（#6）：只从模板清单里移除旧模板那一项，清单本身与其余项不动。
- [x] **4.3 删除类**（#7、#9、#12，以及 4.1 里判定为专属的）：
      删除前在 `implement.jsonl` 写明原意；删除后在提交信息里能对上。
- [x] **4.4 拆分类**（#15）：服务端重试逻辑的覆盖必须活下来，
      只删针对旧模板标记（抽屉 DOM）的断言。
- [x] **4.5 保留类**（#17 + `dashboard_manual_balance_test.go` 整文件）：
      **一行都不许动**，除非是为了配合 §3 的签名变化。
- [x] **4.6 ⛔ 门：自检有没有「靠削弱测试变绿」。**
      逐条回答：每个被删/被改的测试，原来在测什么？现在还有没有别的地方覆盖它？
      答不出「还有」的，要么补一个等价测试，要么留着不删。

---

## 5. 审计脚本

- [x] **5.1 `scripts/audit/metrics.mjs:23`**：选择器 `.rent-row, tbody tr` 里的 `.rent-row` 分支
      已永久失配，收敛为活的那些。
- [x] **5.2 `scripts/audit/seed.mjs:143-148`**：更新那段描述调用链的注释措辞。
- [x] **5.3 确认脚本仍可跑**（本地、非生产）：
      ```bash
      MYSQL_PORT=3306 APP_PORT=18090 scripts/run-audit-local.sh
      ```
      **不要指向 `:8081` / `bank.ddpl.top`** —— 那是生产应用连生产库。

---

## 6. 验证

- [x] **6.1** `cd cmd/truelayer-demo && go test ./... -count=1 2>&1 | tail -30`
      记下退出码：`go test ./... -count=1 > /tmp/gotest.log 2>&1; echo "GO_TEST_EXIT=$?"`
      （注意：管道里 `$?` 是最后一个命令的码，必须单独取。）
- [x] **6.2** `go vet ./...`
- [x] **6.3** 行数：`wc -l cmd/truelayer-demo/dashboard.go` → 目标 **200 行以内**（基线 569）。
- [x] **6.4** 逐字节复核：`diff /tmp/before-bills.html <(重跑)` → 空；`/dunning` 同。
- [x] **6.5** 已知失败对照：`TestDunningDashboardHTTPWorkflowOnMySQL` 在本任务之前就已失败
      （已在 `git archive HEAD` 的干净副本上独立复现）。它**不是**本任务引入的，
      但本任务也不负责修它 —— 报告时如实说明，不要算进本任务的红绿。
- [x] **6.6** 无残留：确认没有遗留的 `rentops_audit%` 数据库或仍在监听的实例。

---

## 7. 收尾

- [x] **7.1** 更新 spec（`trellis-update-spec`）：
      - `database-guidelines.md` 里记录「有库时该模板不可达」的那一小节 → 改为「已删除」
      - 新增一条：**催收抽屉在真实路径上缺失**（`design.md` §7 的发现），
        并指向 ③ 作为输入
      - 新增一条：`renderRentDashboard` 是 `/bills`、`/dunning` 的活渲染体 ——
        防止后来者看到函数名又以为是死代码
- [x] **7.2**（c3c6ecb） 提交（Phase 3.4）。提交信息要能读出：
      删了什么、为什么它是死的、哪些测试改指向了哪里、催收抽屉那条发现。
- [x] **7.3** `/trellis:finish-work`。

---

## 不做的事

- 不动 `else` 分支与 `loadTenants` / `loadExpenses` / `loadLatestDemoResult`（`design.md` §4）
- 不删 `/rent-dashboard/settle` 路由、`handleDashboardManualBalance`、
  `dashboard_manual_balance_test.go`（`design.md` §5）
- 不并入 `legacyExpenseTemplate`（`design.md` §11）
- 不清理工作区里不属于本任务的未提交变更

---

## 执行痕迹（2026-09-19 实施 + 复核，按门回填）

> 补这一节是因为检查方指出：门 A 与门 B 当时**只存在于散文自述里，仓库中没有执行痕迹**。
> 下面每条都是可复查的。

### 门 A（实现前：`else` 分支是否真死）—— 通过

`handleDunningAction` 的三道鉴权已在上文 1.1 记录，实现方**独立复核后确认成立**，
`else` 分支保持原样未动。

### 门 B（删测试前：逐项「原来在测什么」）—— 通过，且**复核推翻了实现方的"全绿"**

实现方按 `design.md` §6 逐项处理，并补了清单遗漏的第 18 项
（`TestRentDashboardHTTPRendersSafeFallbackAndFilterError`，`dashboard_filters_test.go:175`
—— 它不点名 `rentDashboardTemplate`，但靠"有 session 无 DB"直接驱动 `handleRentDashboard`，
受 R3 影响）。

**复核发现 3 处实质问题，均已修复**（这是本任务最值得记住的部分）：

| # | 问题 | 修法 |
|---|---|---|
| 1 | `TestRentWorkspaceLinksPendingCountToSelectedPeriod` 是**恒真断言** —— fixture 的 `ListURL` 换个月份就能满足，等于没测 | 重写为钉模板自建锚点 `class="workspace-queue-more"`，并故意把 `ListURL` 指向 `2026-08`。注入「模板不再从 `.Period` 推导」→ FAIL |
| 2 | `TenantAlias`（别名）渲染覆盖被静默丢掉 | `rent_workspace_test.go` fixture 加 `TenantAlias: "Sample A"` + 期望 `"别名：Sample A"` |
| 3 | 删除让「非法筛选/非法月份在页面上可见」失去**唯一**的 Go 覆盖 | 新增 `TestBillsPageSurfacesInvalidFilterAndPeriodErrors`；注入「删掉模板 error 分支」→ FAIL |

另外验证了重写后的 503 测试**有牙**：注入 `w.WriteHeader(http.StatusOK)` → FAIL。

**结论**：实现方报的"全绿"在**测试强度上不成立** —— 断言没被删，但有一条被改成了恒真、
一条覆盖静默消失。这正是 R4 要防的东西，而它躲过了实现方的自检。

### 逐字节不变（AC 硬项）—— 独立复现通过

**没有采信实现方自己抓的"改前"快照。** 复核方用 `git archive c944538 | tar -x -C /tmp/head-clean`
取干净基线，两棵树各建独立库（`rentops_bytecheck_base` / `rentops_bytecheck`），
用原始 SQL 固定 `user id=1` / `tenant id=1` 保证自增 ID 一致，再经 `newAppMux` + 真实 session
渲染同一输入：

| 路径 | 归一化后与基线逐字节相同 |
|---|---|
| `GET /bills` | ✓ |
| `GET /dunning` | ✓ |
| `POST /dunning/send` | ✓ |

归一化只处理 `name="request_key" value="[^"]*"`（`recordID` 经 `randomState()` 带随机后缀），
这是唯一非确定字段。

**更正实现方的一处自述**：它报 `GET /dunning` 与 `POST /dunning/send` sha256 相同，
**该数字不可复现**（实测 GET 10264 B、POST 10367 B）。结论不受影响（POST 确实经
`dunningPageTemplate` 返回 200），但**不要在提交信息里复述那个数字**。

### 验证结果

| 项 | 结果 |
|---|---|
| `go test ./... -count=1` | `GO_TEST_EXIT=0`（注意：无 DSN 时 MySQL 门控测试被 skip，见下） |
| `go vet ./...` | `VET_EXIT=0` |
| `grep -rn 'rentDashboardTemplate' --include='*.go' .` | 无输出 |
| `wc -l dashboard.go` | **189**（目标 ≤200，基线 569）|
| 注入实验还原 | `rent_collection_pages.go` / `rent-workspace.html` 均无 diff |
| 残留清理 | 已 DROP 全部 `rentops_bytecheck*` / `rentops_check*` / `rentops_vfy*` / `rentops_test_head`；无 `rentops_audit*`；18090/8081 无监听 |

### 已知红 / 未登记的残余（复核方查明，留给后续）

1. **`TestDunningDashboardHTTPWorkflowOnMySQL`（`dunning_handlers_mysql_test.go:60`）是未登记的依赖者。**
   用官方 harness 分别跑当前树与 `c944538` 基线 → **两者都 `EXIT=1`，同一行、同一断言、同一消息**
   （`/rent-dashboard` 已渲染房间工作台）。**前后一致失败 ⇒ 不是本任务回归**，但应补进
   `design.md` §6 的 known-red 清单。
2. **`web/static/css/calendar.css:12,197`** 的 `.dashboard-toolbar .calendar-input` 随模板变成死规则
   （文件本身仍 live）。未清理，可接受但此前未登记。
3. **`scripts/audit/probe-p1-4.mjs:61`** 的 `.dashboard-pagination` 回退分支在本任务**之前**就是死的。

### 复核方明确未验证的部分（如实记录，不要当成已验证）

- 未跑浏览器 / Playwright，未做几何尺寸测量（spec 说桌面侧断点契约要靠浏览器核）。
- 未跑 `scripts/run-audit-local.sh`、未起过应用；全部用 `httptest` 进程内渲染，
  因此**未**验证真实服务器、静态资源服务、以及 MySQL 实际驱动下的 `/bills` 渲染。
- `go test` 无 DSN 时跳过 MySQL 门控测试，「全套绿」不含其余 MySQL 测试。
- `/dunning/send` 用的是 recording stub，未验证真实 SMTP。
