# 精简流水与主页的状态控件与用词

## Goal

去掉流水页上重复的状态筛选控件，并把"匹配/关联"两套名字统一到「关联」；
同时把主页状态下拉的选项文案与行内徽章文案对齐，让同一状态在全站只有一种写法。

本轮**只做「去重 + 统一文案」**，不动任何状态值、不改筛选能力、不改状态推导逻辑。

## 背景：问题清单（含证据）

### 流水页 `/transactions`

数据库里真实存在的匹配状态是 6 个（`transactions.go:317-321` 的校验白名单、
`transactions.go:461-468` 的文案映射）：

| 存储值 | 代码文案（`statusLabel`） |
|---|---|
| `matched` | 已关联 |
| `partial` | 部分关联 |
| `candidate` | 待确认 |
| `needs_review` | 需处理 |
| `unmatched` | 未关联 |
| `ignored` | 已忽略 |

外加一个**不是状态的伪筛选** `pending`（`transactions.go:78-84`）：库里无此值，
展开为 `candidate`+`needs_review`+`unmatched`+`partial` 四个，且只对收入生效。

界面上问同一件事的控件有 **3 个**：

1. 页签（`billing_page.go:252-256`）：待处理 / 已匹配 / 全部
2. 快捷筛选下拉（`billing_page.go:295`）：7 项，用「已匹配 / 部分匹配 / 未匹配」
3. 筛选栏下拉（`billing_page.go:307`）：8 项，用「已关联 / 部分关联 / 未关联」

由此产生三个缺陷：

- **同一状态两套名字**：`matched` = 已匹配 / 已关联；`unmatched` = 未匹配 / 未关联；
  `partial` = 部分匹配 / 部分关联。两套词在同一页并排出现。
- **两个下拉选项集合不一致**：快捷筛选下拉**没有「已忽略」**，
  从它进入后无法筛出已忽略的流水。
- **「待处理」在同一页出现 3 次**（页签 + 两个下拉），含义完全相同。

### 主页 `/rent-dashboard`（月度总览）

状态筛选 8 项（`web/templates/pages/rent-workspace.html:50`）：
全部 / 未交满 / 待处理 / 逾期 / 部分缴纳 / 未到期未缴 / 已交满 / 空置

其中 `unpaid`（未交满）是伪筛选，等于「待处理+逾期+部分缴纳+未到期未缴」
（`rent_workspace.go:944-946`）。

**下拉文案与行内徽章文案对不上**（徽章走 `workspaceStatusLabel`，`rent_workspace.go:930-938`）：

| 下拉文案 | 徽章文案 | 存储值 |
|---|---|---|
| 逾期 | 已逾期 | `overdue` |
| 已交满 | 已缴清 | `paid` |
| 未到期未缴 | 未缴 | `open` |
| 部分缴纳 | 部分缴纳 | `partial` |
| 待处理 | 待处理 | `needs_review` |
| 空置 | 空置 | `vacant` |

### 与原型的关系

原型 `figma/rentops-desktop-suite.html` 里，主页收缴状态筛选只有 4 项
（全部状态 / 未收齐 / 已收齐 / 超额付款），流水匹配页的状态只有「建议分配 / 需核对」。
按 dd 已确认的判据，控件形态属于要还原的范畴，但本轮**不**向原型收敛——
dd 明确选了「第一层 + 统一用词」，结构性收敛另行决策（见 Out of scope）。

## Requirements

1. **流水页去重**：删掉快捷筛选表单（`transaction-route-quickfilter`）里重复的
   `match_status` 下拉。保留其中的付款人搜索、到账月份、搜索按钮。
   状态筛选由可见的 3 个页签承担；细分状态走「搜索」展开的筛选栏（8 项齐全）。
2. **流水页统一用词**：页签「已匹配」改为「已关联」，链接目标不变。
   全仓库渲染结果中不再出现「已匹配」「未匹配」「部分匹配」。
3. **流水页筛选栏保持**：`billing_page.go:307` 的下拉维持 8 项，用词沿用
   「已关联 / 部分关联 / 未关联」。
4. **主页文案对齐**：状态下拉的选项文案改为与徽章一致——
   逾期→已逾期、已交满→已缴清、未到期未缴→未缴；部分缴纳/待处理/空置/全部不变。
   每个 `<option>` 的 `value` 不变，筛选行为不变。
5. **测试同步**：更新因上述改动而失效的断言，并补上防止重复控件回归的断言。
6. **（执行中追加，超出初版 AC 字面范围；2026-09-21 dd 已追认保留）两处同源的漏网用词**：
   - 筛选栏下拉自己的标签仍是「匹配状态」（`billing_page.go:305`），而它 8 个选项
     全部已说「关联」——控件自相矛盾。改为「关联状态」。
   - 主页成功提示仍是「流水**已匹配**，待处理队列已更新。」
     （`web/templates/pages/rent-workspace.html:25`），把已退役的状态词直接说给用户听。
     改为「流水已关联」。
   两处都只改可见文案，无行为变化，各补一条防回潮断言（含反向对照验证）。

   > 边界：动词「匹配」不动。`匹配流水`／`确认匹配`／`撤销匹配`／`匹配依据`
   > 指的是「把流水匹配到租金月」这个**动作**，与状态词「关联」是两回事。
   > 已写入 `.trellis/spec/frontend/status-vocabulary.md`，避免下次被一起误改。

## Constraints

- **不改存储状态值**：`matched`/`partial`/`candidate`/`needs_review`/`unmatched`/`ignored`
  六个值及其校验白名单不变；`ledgerObligationStatus`（`ledger.go:119-139`）与
  `workspaceWorstStatus`（`rent_workspace.go:920-928`）的推导逻辑不变。
- **不改筛选能力**：每个状态值仍可通过某个控件筛到；URL 参数名与取值不变。
- **`/billing` 是独立页面**：`main.go:502` 仍注册 `/billing`，其筛选栏不折叠
  且不渲染快捷筛选表单。改动只影响 `/transactions`，不得使 `/billing` 失去状态筛选。
- **不碰原型差异**：下拉选项集合与原型不一致属既有差异，本轮不处理。
- 保留未跟踪文件 `收租明细_Rosewood_20260916.xlsx`。

## Acceptance Criteria

- [ ] `/transactions` 渲染结果中 `<select ... name="match_status">` 只有 **1 个**；
      该下拉含全部 8 项，且包含「已忽略」。
- [ ] `/transactions` 页签文案为 待处理 / 已关联 / 全部，href 仍分别为
      `?match_status=pending`、`?match_status=matched`、`?scope=all`。
- [ ] `/transactions` 与 `/billing` 的渲染结果中不再出现字串「已匹配」「未匹配」「部分匹配」。
- [ ] 主页状态下拉每个 option 的 `value` 仍为
      `all/unpaid/needs_review/overdue/partial/open/paid/vacant`（8 个，集合不变），
      文案与 `workspaceStatusLabel` 对同一状态值的输出一致。
- [ ] `/billing` 仍渲染 8 项状态下拉，`/transactions` 的筛选栏（折叠内）仍渲染 8 项。
- [ ] `go test ./cmd/truelayer-demo/ -count=1` 的失败集合与基线**逐条相同**（9 条，
      无新增）（退出码仍为 1，因为基线本就是红的——见下）。
- [ ] 筛选栏状态下拉的标签渲染为「关联状态」（`for="match_status">关联状态<`）。
- [ ] `/rent-dashboard` 的 `rent_confirmed` 提示渲染「流水已关联」，不含「已匹配」。
- [ ] 不设 `RENTOPS_MYSQL_TEST_DSN` 时，走库测试的 SKIP 数量与改动前一致（见下方基线）。

### 测试基线（避免把既有失败当回归）

**`go test ./cmd/truelayer-demo/ -count=1` 在 `main` 上当前就是红的**。
2026-09-21 `git stash` 掉全部改动后在干净工作区实测 `EXIT=1`，
稳定失败 **9 条**（`TestWorkspaceTemplatesIncludeSharedCalendarPicker` 另有
3 个子测试失败）：

| 失败测试 | 位置 | 现象 |
|---|---|---|
| `TestWorkspaceNavExposesDesktopSections` | `desktop_layout_test.go:57` | nav 缺 `资金与系统` 分组标题 |
| `TestTenantCreateFormCanBindAnExistingRoom` | `desktop_layout_test.go:178` | 缺 `tenantRoomSelect` |
| `TestExpensePageUsesPrototypeListAndAddDrawer` | `expense_page_test.go:44` | 缺 `entity-drawer-backdrop`、`保存支出` |
| `TestTransactionRouteActionColumnSaysViewDetails` | `list_pages_alignment_test.go:317` | 表头缺 `状态 / 操作` |
| `TestWorkspaceTemplatesIncludeSharedCalendarPicker` | `main_test.go:105` | billing/tenants/expenses 三个子测试缺 `calendar-input` / `.calendar-popover` |
| `TestBillingTemplateShowsMonthChoiceForRememberedTenant` | `main_test.go:784` | 缺 `分别选择要匹配的租客和月份` |
| `TestNavCountsOnlyRenderWhereTheyDidBefore` | `mobile_layout_test.go:509` | 期望 4 个 `class="nav-count"`，实际 3 个 |
| `TestParseTenantHistoryRangeDefaultsToTwelveMonths` | `tenant_profile_test.go:164` | 期望 size=12，实际 size=10 |
| `TestTenantHistoryRangePresetWinsOverExplicitMonths` | `tenant_profile_test.go:237` | 期望 size=12，实际 size=10 |

均与本次改动无关（导航、抽屉、分页、日历控件），**不在本任务范围内修**。
因此验收标准是「失败集合不扩大」，不是「全绿」。

> 取证教训：最初用 `go test ... | tail -30` 取的基线，输出文件里只剩最后 30 行，
> 数出「3 条」并据此写进了本文件——错的。管道退出码取自 `tail`（恒为 0），
> 输出也被截断，两个坑叠在一起。**基线必须 `git stash` 后用不接管道的方式全量取。**

另外三条已知问题：

- 不设 `RENTOPS_MYSQL_TEST_DSN` 时整组 22 条走库测试静默 SKIP，包仍报 `ok`，
  **PASS 与 SKIP 在默认输出里长得一样**，不能据此声称"全绿"。
- `TestDunningDashboardHTTPWorkflowOnMySQL` 稳定红（与 09-19 工作台改版有关，与本次无关）。
- `TestDunningSendWorkflowOnMySQL` flaky（负载下约 20–40% 红），单次红不算数。

## Out of scope（已识别，本轮不动，留待 dd 拍板）

### 执行中新发现的两处（我刻意没动）

8. **第三套并行词表 `transactionMatchStatusLabel`**（`transaction_previews.go:183-191`），
   为同一批存储值又写了一版词：`已匹配候选` / `部分匹配候选` / `未找到候选`。
   渲染在两处**用户可见**的地方：
   - `/billing/payer/preview` 批量付款人预览页的「状态／原因」列（`transaction_previews.go:167`）
   - 流水详情页的「建议」状态（`transaction_detail.go:311`）

   于是同一页面上，列表徽章说「已关联」、详情里的建议说「已匹配候选」。
   没动的原因：这里的「候选」是有实义的（指的是**建议的**租金月，不是流水自身状态），
   改成「已关联候选」很别扭。要不要统一、怎么统一，得你定。

9. **审计留痕里的状态散文**：`transaction_actions.go:199` 把
   `"已匹配，恢复待处理状态"` 作为取消暂缓的 reason **写进数据库**。
   没动的原因：这是往库里写值，不只是渲染层文案，越过了本任务「只动表现层」的边界；
   而且历史行已经按老文本存下了，改了会造成新旧不一致。

1. **主页「空置」混在缴费状态里**——它是房间占用维度，不是收没收钱。
2. **合并「部分缴纳」与「未到期未缴」**——对房东是同一个决策（钱没齐、还没到期）。
3. **向原型 4 项收敛**（全部状态 / 未收齐 / 已收齐 / 超额付款）。
4. **「超额付款」缺失**——原型有此筛选项，实现中 `grep '超额|overpaid'` 为 0 处；
   `ledger.go:123` 的 `paid >= expected` 把多付直接归入 `paid`。
   是否属于漏做的功能需单独查证。
5. **`/bills` 的第三套词**（未结清 / 全部账单 / 已结清 / 逾期 / 部分缴纳 / 未到期未缴）。
6. **`needs_review` 的文案在两个函数里不一致**：
   `rentStatusLabel`（`obligations.go:589-597`）输出「需处理」，
   `workspaceStatusLabel`（`rent_workspace.go:930-938`）覆盖为「待处理」；
   流水页的 `needs_review` 也是「需处理」。三处两种写法。
7. **列头/卡片标签「已交满 / 未交满」与徽章「已缴清 / 未缴」不一致**——
   列头是房间计数（`已交满 N 间`），与状态下拉不是同一用途。
   **本轮把下拉对齐到徽章后，同一页面上会出现新的可见不一致**，需 dd 确认是否统一。
   具体位置（改动后仍在，`grep '>已交满<\|>未交满<'` 正好命中这 2 处）：
   - `web/templates/pages/rent-workspace.html:67` —— 树形表头 `<span>已交满</span><span>未交满</span>`
   - `web/templates/pages/rent-workspace.html:115` —— 房产卡片 `<dt>已交满</dt>…<dt>未交满</dt>`

   > 验收时注意：AC 第 4 条只约束**状态下拉的 option 文案**，与这 2 处列头无关。
   > 若用 `grep '已交满'` 做全页扫描会命中它们，那不是本次改动没做完。

## Notes

- Keep `prd.md` focused on requirements, constraints, and acceptance criteria.
- 流水页两个问题（页签 + 两个下拉）在本仓库测试里已有约定：
  `billing_layout_test.go:53-61` 要求两个下拉都存在且都带
  `onchange="this.form.requestSubmit()"`；`billing_layout_test.go:97-101` 要求
  筛选栏下拉的首项为 `<option value="pending" selected>待处理`。
  本改动会破坏前者，需同步改写，并在断言里改为"不再有重复控件"。
