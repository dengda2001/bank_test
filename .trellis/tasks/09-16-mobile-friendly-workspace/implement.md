# 执行计划：移动端友好版工作台（父任务）

父任务**不做实现**。它拥有需求集、任务映射、跨子任务验收标准与最终集成验收。实际代码改动全部落在 4 个子任务里。

---

## 1. 任务树与依赖

```
09-16-mobile-friendly-workspace（本文，父）
├─ 09-17-mobile-audit-fixtures    可复现的浏览器验收环境      P0
├─ 09-17-mobile-billing-cards     移动卡片基元与 /billing 卡片流  P1
├─ 09-17-mobile-dashboard-cards   /rent-dashboard 移动卡片列表   P2
└─ 09-17-mobile-profile-cards     /tenants、/expenses 与录入页移动化 P2
```

依赖是**顺序**的，不是并行：`fixtures → billing → dashboard → profile`。

Trellis 的父子结构不是依赖系统，所以这条顺序必须写在每个子任务自己的 `prd.md` / `implement.md` 里，当作启动前门禁。

**为什么是这个顺序**：

- `fixtures` 先行，因为它是后面三个子任务共同的验收手段。没有它，三个子任务的验收都会退回「无头浏览器打生产」或「只跑字符串断言」，而 PRD §10.3 要求前者不可复现、后者已被上一轮证明不够。
- `billing` 先于另外两个，因为它是最复杂的页面（4KB 的行内动作单元格、7 个筛选控件、3 个隐藏参数、一段做三件事的行内脚本）。卡片基元的抽象在这里第一次被真实需求压测；如果基元不够用，在 `/billing` 上暴露出来比在 `/dashboard` 上暴露更早也更便宜。
- `dashboard` 先于 `profile`，因为 dashboard 已有 view model 之外的全部筛选基础设施（`dashboard_filters.go`），而 profile 组的三个页面连 view model 都没有，需要新建。

---

## 2. 子任务交付物与验收

### 2.1 `mobile-audit-fixtures` — 可复现的浏览器验收环境

**交付**

- 一份可提交的 seed，覆盖 PRD §9 的数据要求与全部渲染分支（至少：自动匹配、部分收款、逾期未缴、跨月拆分、未确认、已撤销、无匹配、空列表；长中文姓名、长英文姓名、长地址、长银行描述、金额小数、多条付款记录）。
- harness 修复：归档 harness（`.trellis/tasks/archive/2026-09/09-16-mobile-responsive-audit/research/harness/`）里约 20 个脚本把 Chromium 路径写死为已不存在的 `/tmp/mobile-audit/chrome-linux64/chrome`；统一改为支持 `CHROME_BIN` 或 Playwright 的 `channel: 'chrome'`。
- 一条从空仓库到出报告的完整命令序列，写进 `research/`。

**已知约束（研究结论，必须绕开）**

- `cmd/rentops-e2e` 的 fixture 机制**不能直接复用**：入口全是未导出的 `package main`，且 `executeE2ERun` 退出前会删光自己造的数据（`cmd/rentops-e2e/cleanup.go:639-669`），`run-e2e-local.sh` 随后还会 drop 整个库。它造的数据也不覆盖需要的分支：结束时只剩 `matched`×4 + `ignored`×1，没有 DEBIT/支出数据，没有 `/expenses` 数据。
- `POST /import-legacy` 能造租客 + `unmatched` 流水，但**造不出** allocation、`matched`/`partial`、现金收款、付款人关系。
- `RENTOPS_TENANT_FILE`/`RENTOPS_EXPENSE_FILE` 是输入路径不是 seed 器，启动时不会自动导入。

**验收**

- 从干净的 checkout 出发，按文档命令能起一次性实例并跑出报告。
- 报告中 `/billing`、`/rent-dashboard`、`/tenants`、`/expenses` 的每一张卡片/每一行形态分支都有数据命中，逐条列出「分支 → 命中的数据」。
- 全程不接触 `:8081` / `bank.ddpl.top`。
- 脚本对 `CHROME_BIN` 的缺失给出明确报错，而不是静默用错路径。

### 2.2 `mobile-billing-cards` — 移动卡片基元与 `/billing` 卡片流

**交付**

- 卡片基元：卡片容器、卡片、摘要行、展开区、状态标签的 CSS 与模板约定（`design.md` §D3/§D4/§D5）。
- `/billing` 手机卡片流，按 `design.md` §D4 的状态分支表组织动作。
- 筛选分层：3 个首要筛选 + `<details>` 更多筛选（`design.md` §D6）。
- 默认筛选变更（`design.md` §D7）——**单独一个提交**。

**验收**（`design.md` §4 契约 + PRD §5.2）

- 375px 下不存在依赖横向滚动的宽表；卡片摘要含方向、付款人/描述、金额、到账日期、匹配状态。
- 卡片内可完成：匹配、选租金月、归类/拆分、忽略、恢复、撤销。
- 金额完整显示，不被截断。
- 默认只看待处理；`match_status=all` 可达且结果与变更前一致；空状态有出口；状态徽章链接不再丢筛选。
- 桌面端在 `match_status=all` 下与基线逐页一致。
- 同步修正的现有测试逐个说明「为什么改、改成了什么」。

### 2.3 `mobile-dashboard-cards` — `/rent-dashboard` 移动卡片列表

**交付**

- 先补 `rentDashboardRow` 的行级 view model（`design.md` §D2）。
- 汇总卡片 + 租客卡片列表；卡片默认折叠付款明细。
- 月份切换的「上一月 / 月份 / 下一月」三段布局。
- 默认 `status=unpaid`（`design.md` §D7）。

**验收**（PRD §5.1）

- 汇总指标（应收、已收、未收、待处理）在手机上优先展示。
- 卡片首屏含姓名、房间/地址摘要、应收、已收、未收、状态。
- 「查看租客详情」与「展开缴费明细」是可区分的两个操作。
- 平账、催缴入口可见且不需横向滚动。
- 桌面端在 `status=all` 下与基线一致。

### 2.4 `mobile-profile-cards` — `/tenants`、`/expenses` 与录入页移动化

**交付**

- `/tenants` 租客卡片列表（补 view model）。
- `/tenants/:id` 单列信息组 + 账单卡片。
- `/expenses` 支出卡片列表 + 录入表单单列化（补 view model）。
- `/cash-receipts/new`、`/cash-receipts/void`、`/billing/revoke`、`/billing/payer/preview` 的窄屏收尾（PRD §6.7/§6.8）。

**验收**（PRD §5.3/§5.4/§5.5）

- 租客卡片的详情/编辑稳定可见，不依赖横向滚动。
- 支出列表卡片首屏含日期、描述、金额、类别；备注与支付方式为次级信息。
- 现金补录表单首个可填控件在首屏可见；作废页先展示不可逆影响与收款事实。
- 预览页标题、说明、返回入口固定在卡片可见区域；只有内部宽内容允许横向滚动。
- 320px 下单列表单与通栏主按钮。

---

## 3. 集成门禁（父任务完成判据）

四个子任务全部归档后，父任务还需要：

- [ ] `go test ./...` 全绿。
- [ ] 全部 11 个页面在 320/360/375/390/412px 下 `documentElement.scrollWidth === documentElement.clientWidth`。
- [ ] 核心四页（`/billing`、`/rent-dashboard`、`/tenants`、`/expenses`）在手机上不以宽表为主要浏览方式——这是 PRD §3.0.1 的阻断条件。
- [ ] 所有主要操作命中区 ≥44×44px。
- [ ] 桌面端在「显式全部」参数下与基线一致；`/billing`、`/rent-dashboard` 的默认首屏差异是唯一已知差异且已记录。
- [ ] 每页首屏主要动作 ≤1、次要 ≤2，首屏筛选控件 ≤3。
- [ ] 验收报告逐页、逐视口、逐状态记录，含失败元素，不只写「看起来正常」。
- [ ] `research/` 下有每个子任务留下的验收证据。
- [ ] 真机抽查完成，或明确记录为未完成并说明原因（上一轮就是栽在这里）。

---

## 4. 提交策略

- 每个子任务独立提交，可单独回滚。
- `design.md` §D7 的默认筛选变更单独成一个提交，与卡片结构分开。
- 子任务 1 的 seed 与 harness 修复必须是提交的一部分——**这是本任务相对上一轮最重要的改进**：上一轮的验收数据只存在于生产库，导致那次审计无法重跑。
- 不在本任务内做数据库迁移或业务规则重写。

---

## 5. 风险

| 风险 | 应对 |
| --- | --- |
| 卡片与桌面行漂移 | view model 先行（§D2）；卡片只消费 view model |
| 卡片退场规则误伤共享类 `.table-wrap` | 退场规则限定到该页自己的表格容器（§D3） |
| 默认筛选变更让用户掉进空列表 | `match_status=all` 可达 + 空状态出口 + 状态徽章链接保留筛选（§D7） |
| `/billing` 行内脚本被卡片误用 | 该脚本三件事里只有折叠是卡片需要的（§5 末条） |
| 现有 Go 测试大面积失败，分不清是预期还是回归 | 先把「预期要改」的测试列进子任务 2 的清单，逐个说明改动理由 |
| seed 工作量超预期 | `seed-options.md` 已列四种方案的工作量与缺口，交给子任务 1 决策时参考 |
| 验收退回打生产 | 写进 PRD §10.3 与子任务 1 的验收：禁止指向 `:8081` / `bank.ddpl.top` |
| 规范里的 harness 路径已失效 | `.trellis/spec/frontend/responsive-conventions.md:244` 指向 `.trellis/tasks/09-16-mobile-responsive-audit/research/harness/`，实际已归档到 `archive/2026-09/` 下。实现期间以归档路径为准；**Phase 3 的 spec 更新步骤要顺手修掉这一处** |
