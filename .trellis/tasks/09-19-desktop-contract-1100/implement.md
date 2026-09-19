# 执行计划：桌面契约解冻与 1100 断点

本子任务必须先于 ②③④⑤ 完成。它是其余四个的样式基线。

## 执行顺序

1. [ ] 通读 `responsive-conventions.md`，标记所有依赖"桌面冻结"表述的段落
2. [ ] 查清 `max-width:980px` / `min-width:981px` 各自承担的行为（读规则 + 查测试 + 实机验证），记录结论
3. [ ] 改写 `:17` 的冻结契约表述（按 design.md §2.3 的三个条件）
4. [ ] 在共享样式表新增 `@media (max-width: 1100px)`，位置在基准之后、640 之前
5. [ ] **实机验证 1100 档是否真的生效**（design.md §2.1 第 2 步）—— 不生效则升级优先级或改用页面表追加
6. [ ] 同步更新因桌面解冻而失效的断言
7. [ ] 确认 640 档规则与断言未变（diff 证明）
8. [ ] 四档截图，存 `research/screenshots/`
9. [ ] 跑全量测试与 vet
10. [ ] 提交

不要跳过第 5 步 —— 它是本子任务唯一容易"看起来做完了但实际没生效"的地方。

## 验证命令

```bash
go test ./cmd/truelayer-demo/...
go vet ./...
git diff --check
```

改动前后对比 640 档：

```bash
git diff .trellis/spec/frontend/responsive-conventions.md
# 以及各 pages/*.css 中 max-width:640px 块的 diff，应无变化
```

## 风险文件

| 文件 | 风险 | 处置 |
|---|---|---|
| `web/static/css/workspace.css` | 全部页面共享 | 改后十一个页面全截图 |
| `cmd/truelayer-demo/mobile_layout_test.go` | 断言桌面不被污染，解冻后必然失败 | 与代码改动**同批**更新 |
| `responsive-conventions.md` | 是后续四个子任务的依据 | 改动要能被后续任务直接引用 |

## 开工前检查

- [x] 用户已审阅本子任务的 `prd.md` / `design.md` / `implement.md`
- [ ] 已加载 `trellis-before-dev`
- [ ] 已确认本机可运行应用并截图
- [ ] 已确认用 `scripts/run-audit-local.sh` 起一次性实例，不指向 `:8081` / `bank.ddpl.top`

## 阻塞记录（2026-09-19，开工后实测发现）

第 5 步「实机验证 1100 档是否真的生效」**当前无法执行**，卡在审计实例起不来。以下是实测到的事实，供本子任务恢复时直接引用。

### B1 审计 harness 在 HEAD 上是坏的（已修，未提交）

`scripts/audit/seed.mjs` 的 `parseBillingRows` 用 `/<tr class="(income|expense)">/` 匹配，要求 `class` 紧跟 `<tr`。
`billing_page.go:301` 现在是 `<tr id="transaction-row-{{.DetailKey}}" class="{{.Direction}}">` —— 属性顺序变了，正则失配，
seeder 把「`/billing` 没有任何行」当成导入失败。已改为不依赖属性顺序的 `/<tr\b[^>]*\bclass="(income|expense)"[^>]*>/`。
修完 seeder 通过了 billing 解析，前进到后续断言。

`rent-row`（`dashboard.go:476`）与 `tenant-row`（`main.go:2682`）仍是 `class` 紧随 `<tr`，解析正常，未动。

### B2 真正的阻塞：seeder 的 fixture 与它自己的断言互相矛盾

> **2026-09-19 更正。** 本节原先写「根因是租金义务被重复生成」。**该归因已被证伪**，
> 见下方「复验」。结论（本子任务仍被卡住）不变，但**解除条件变了**——不是等 dedup，是要改 fixture。

seeder 的下一条断言失败于「没有 `一键匹配` 建议」（`scripts/audit/seed.mjs:433`）。
该分支在 `billing_page.go:308` 的 `{{if .CanConfirm}}`，模板本身没问题。

**原先的归因**：`rent_obligations` 每个 (租户, 月份) 累积多条 `record_status='active'` 的行
（全新审计库 116 条，应为 18；开发库 248 条），数据脏导致读数不对。
该 bug 真实存在，已由 `.trellis/tasks/09-19-rent-obligation-dedup/` 的迁移 013 修复。

**复验（2026-09-19，dedup 完成后）**：`MYSQL_PORT=3306 APP_PORT=18090 scripts/run-audit-local.sh`
真实退出码 **1**，同一断言依旧失败；此时义务为 **18 条、零重复**。
→ 013 是必要修复，但**不是 B2 的原因**。

**真实根因**：`matching.go:94` —— 流水未解析出月份时只能返回 `candidate`，渲染「请确认租金月份」
（`NeedsMonthChoice`），拿不到 `CanConfirm`。三笔候选里：

- `audit-tx-candidate-priya` 的描述本就没有月份 → 永远只能是 candidate；
- 唯一同时具备「已记住付款人 + 解析出 2026-09 + 金额 70000 恰等于义务 16 应收」的是
  `Rent payment 2026-09 - MICHAEL OBRIEN`，而 **seeder 把它标成了 `ignored`**
  （`seed.mjs:29-36` 的 `TX` 表 + `:415` 的状态映射），于是它根本不进这条分支。

A/B 实测：保持现状 → `一键匹配` 渲染 **0** 次；把该笔改回 `unmatched` → 渲染 **1** 次。
该矛盾在 HEAD 上原样存在，与 013 无关。

**对本子任务的影响**：验收项「四档 11 页 `scrollWidth === clientWidth`」与「四档截图存入 `research/screenshots/`」
都依赖这个实例，故仍**挂起**，恢复前不要勾选。但**恢复的前置条件已不是 dedup**（那已完成），
而是二选一：把 `ignored` 演示位换到一笔本就无付款人/无月份的流水（如 `audit-tx-savings-reserve`，
可同时保住「已忽略」这个演示态），或弱化该断言。

在这之前，本子任务可做的部分是：第 1–4 步、第 6–7 步（spec 改写、1100 档落地、980/981 处置、断言同步）与第 9 步的 Go 测试部分。

### B3 审计的 dashboard 断言是结构性死代码（已修，未提交）

B2 修完后暴露出来的**第三处**失修。`seed.mjs` 原有 4 条断言读 `/rent-dashboard`
并解析 `rent-row` / `tenant-link`，它们在**任何有库的环境里都不可能通过**：

- `rent-row` 只出现在 `rentDashboardTemplate`（`dashboard.go:476`）；
- `renderRentDashboard` **按请求路径选模板**（`dashboard.go:168-174`）：
  `/bills` → `billsPageTemplate`，`/dunning*` → `dunningPageTemplate`，其余 → `rentDashboardTemplate`；
- 而 `/rent-dashboard` 在有会话时根本走不到这段 —— `dashboard.go:15-17` 已短路到房间视角。

三者合起来：有库时 `rent-row` 不可达。此前它被 B2 挡在前面，所以一直没暴露。

**修法**：4 条断言改读 `/bills?period=X&status=all&page_size=50`，解析账单表行
（`rent_collection_pages.go:49`）。`status=all` 是必须的：页面自带的「未结清」筛选项是
open ∪ overdue ∪ partial（`dashboard_filters.go:81-90`），会把 `paid` 藏掉。
解析器锚定桌面表格的 `<tr>` 形状，避免移动端卡片列表（`collection-bill-card`）
把同一租客算两次。

**复验（2026-09-19）**：`MYSQL_PORT=3306 APP_PORT=18090 scripts/run-audit-local.sh` →
seeder 全过，打印 `==> seed complete: all reachable §D3 branches present`，
其中 `/bills 2026-09: paid / partial / open / overdue` 与
`/bills 2026-08: partial (mixed bank + cash payments)` 两条即 B3 修复的证据。

**结论**：B1 / B2 / B3 是**同一个** seeder 的三处失修，现已全部修好，
第 5 步「实机验证 1100 档」的前置阻塞**解除**。

### 审计实机观测（2026-09-19，B1–B3 修复后）

`scripts/run-audit-local.sh` 起实例 + `node scripts/audit/audit.mjs`（375×667 / 390×844 / 768×1024
三档 × 6 页 + login）。报告在 `/tmp/mobile-audit/out/report.json`（**临时目录，未入仓库，
需要时重跑即可复现**）。

**页面级：全部干净。** 三档所有页面的 `horizontalOverflow` 均为 **0**，`consoleErrors` 为 **0**。

**768×1024 档有 `offenders`**（tenants / tenant-detail / billing / expenses 四页）：
`table` 的 `min-width: 760px`（`workspace.css:238`）在容器左偏约 41px 下右溢 33px，
但外层 `div.tenant-table-wrap.table-wrap` 是 `activeScroller`，**文档级没有横向滚动**。
768 正落在本子任务要处理的 640–1100 区间内 —— 这是第 5 步的输入数据。

**两个交互态探针 ERROR**（Playwright click 30s 超时），**都不是产品缺陷**，第 5 步不要照单全收：

1. `dashboard-dunning-drawer`：`[data-dunning-open]` 只存在于 `dashboard.go:461`，
   即 `rentDashboardTemplate`（`:180` 起）——**与 B3 同根因**，有库会话下 `/rent-dashboard`
   渲染的是房间视角，这个按钮根本不在页面上。修它之前要先回答：旧的
   `rentDashboardTemplate` 是否还算一个受支持的「无库演示面」？若不算，它就是整块死代码
   （`rent-row`、`data-dunning-open` 都只在这里），该删而不是该迁就。
2. `tenants-row-expanded`：在 390×844 点 `.tenant-row`。该表格被 `workspace.css:541-548`
   的移动端卡片化规则**有意排除**（`.tenant-table-wrap` 在 `:not()` 列表里），窄屏下仍是
   760px 宽的表格 + 横向滚动容器；Playwright 无法把一个比视口还宽的行滚进视口，故超时。
   是**探针假设与设计不符**，不是布局 bug。

两条都要在第 5 步一并裁决，但它们都**不阻塞** 1100 档本身的验证。
