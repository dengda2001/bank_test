# 设计：移动端友好版工作台

父任务的技术设计。子任务 2/3/4 的实现都必须遵守本文的共享契约；子任务 1（验收环境）独立，不受本文约束。

关联研究：

- `research/billing-render-map.md` — `/billing` 渲染路径、7 列表格、筛选表单、`<details>` 折叠、逐行动作
- `research/list-pages-render-map.md` — `/rent-dashboard`、`/tenants`、`/expenses` 渲染路径与筛选归属
- `research/verification-infra.md` — 规范、Go 测试 helper、扁平化守卫、harness 现状、seed 缺口
- `research/seed-options.md` — 四种 seed 方案的工作量与缺口
- `.trellis/spec/frontend/responsive-conventions.md` — 上一轮沉淀的窄屏契约（**本文不推翻它**）

---

## 1. 边界

**做**：为 `/billing`、`/rent-dashboard`、`/tenants`、`/tenants/:id`、`/expenses` 增加窄屏卡片渲染路径；筛选分层与默认值调整；录入页与预览页的窄屏收尾。

**不做**：不改路由、不改 POST action、不改查询参数名、不改业务规则、不改桌面表格结构。桌面端唯一的、已批准的变更是**默认筛选值**（PRD §14.1）。

---

## 2. 现状（决定设计的事实）

| 事实 | 证据 |
| --- | --- |
| 所有页面是 Go 字符串模板，`newWorkspacePageTemplate` 从 `workspaceBase` clone | `workspace_shell.go:47` |
| 行渲染全部是 `{{range .Rows}}` + 字面量 `<tr>`，没有 Go 侧字符串拼接 | `billing_page.go:185`、`main.go:2636`（tenants）、`main.go:2777`（expenses）、`dashboard.go:404` |
| `/billing` 有逐行 view model `transactionPageRow`，显示字符串与动作开关都在 Go 侧预计算 | `transactions.go:371-412` |
| `/rent-dashboard`、`/tenants`、`/expenses` **没有** view model，模板直接读 domain struct | `rentDashboardRow` `main.go:288`；`tenantRecord` `main.go:185`；`expenseRecord` `main.go:211` |
| 模板 partial 只有 `workspace-nav`；tenants/expenses 传 `nil` FuncMap | `workspace_shell.go:46` |
| 唯一的断点是 640px；共享窄屏块在 `main.go:2409-2521`，另有 4 处页面局部窄屏块 | `main.go`、`dashboard.go:290-337`、`billing_page.go`、`main.go:2557-2565`、`main.go:2704-2720` |
| 扁平化守卫禁止 `box-shadow:` / `rgba(` / 渐变出现在 `workspacePageCSS` 第一个 640px 块**之前** | `main_test.go:129-152` |
| `pending` 筛选已在 SQL 实现，非读时计算 | `transactions.go:592`：`direction='income' AND match_status IN ('candidate','needs_review','unmatched','partial')` |
| dashboard 的 `status=unpaid` 已经等于「open \| overdue \| partial」，且已有优先级排序 | `dashboard_filters.go:85-87`、`:113` |
| 匹配状态下拉的「全部状态」选项是 `value=""` | `billing_page.go:170` |

最后两条是本次默认值变更可行且低风险的原因；倒数第一条是变更的**主要陷阱**。

---

## 3. 核心决策

### D1：双 DOM，不用 CSS 重排表格，也不用服务端分支

同一份数据渲染两套结构：桌面 `<table>` 原样保留，手机卡片新增；由 `@media (max-width: 640px)` 决定显示哪一套。

**为什么不把表格 CSS 重排成卡片**（`display:block` + `td::before` 老套路）：

- 卡片的操作区需要按状态分支、需要独立折叠，`<td>` 语义下做不出来。
- 与上一轮建立的冻结列 sticky 规则（规范 §3.6）直接冲突：sticky 要求单元格保持表格布局。
- 上一轮已经撞过这个天花板：`/billing` 只能靠「把金额搬进冻结的操作列」腾出 88px（规范 §3.6 结尾）。那是表格布局内能做的极限，不是能继续走的路。

**为什么不用服务端分支**（UA / Client-Hints）：

- 不可响应式：旋转屏幕、拖动窗口宽度不会重渲染。
- 破坏 URL 一致性与缓存；PRD §7.1 要求筛选状态全部通过 URL 可复现。
- 无 JS 时要可用（PRD §8 最后一条）。

**代价与对冲**：markup 体积翻倍，且两套结构有漂移风险（PRD §12 第一条风险）。对冲见 D2——**漂移的根因是两套视图各自决定「显示什么」，所以把「显示什么」收敛到 Go 侧，视图只决定「怎么摆」**。

### D2：先补齐 view model，再写卡片

`/billing` 已经有 `transactionPageRow`，新增卡片可以直接复用，零数据管道工作。

`/rent-dashboard`、`/tenants`、`/expenses` 目前模板直接读 domain struct，字段格式化和状态标签散在模板里。**这三个页面必须先补一层行级 view model**，把金额显示、日期显示、状态标签与文案、动作可见性开关全部预计算好，然后桌面行与手机卡片都只消费这个 struct。

这一层不是为移动端加的——它是「两套视图不漂移」的前提，也是能用 Go 测试断言卡片字段的原因（字符串测试只能断言它渲染出的文本，断言不了模板里的表达式）。

### D3：窄屏用 CSS 切换，不写 JS

```css
/* base：新增的卡片容器默认隐藏，桌面渲染字节不变 */
.billing-cards { display: none; }

/* ≤640：表格退场，卡片上场 */
@media (max-width: 640px) {
  .billing-cards { display: block; }
  .table-wrap { display: none; }   /* 仅限该页的流水表，不碰全站 .table-wrap */
}
```

这是规范 §3.2 允许的第二种写法（宽屏默认 + 窄屏激活），`/billing` 的 `.txn-amount-mobile` 是既有先例。

**注意**：`.table-wrap` 是全站共享类。卡片页必须把退场规则限定到该页自己的表格上（如 `.transaction-table` 的容器），不能写一条全局的 `.table-wrap{display:none}`，否则 `/tenants/:id` 等仍要保留表格的页面会被一起干掉。

### D4：卡片按状态分支显示动作，默认只留一个主操作

PRD §10.1 要求「默认状态的主要动作不超过 1 个，次要动作不超过 2 个」。现有 `/billing` 行内动作是一个约 4KB 的 `<td>`，把所有分支塞在一起；卡片要按 `MatchStatus` 拆开：

| 状态 | 主操作 | 展开区 |
| --- | --- | --- |
| `unmatched` / `candidate` / `needs_review` | 确认匹配（或选月份） | 归类／拆分、无需匹配 |
| `partial` | 修改匹配 | 撤销、归类／拆分 |
| `matched` | 修改匹配 | 撤销 |
| `ignored` | 恢复处理 | — |

「归类／拆分」「无需匹配」「撤销」这类低频或不可逆动作进卡片的展开区（`<details>`），不在首屏露出。

### D5：折叠一律用 `<details>`，不用自研 JS 折叠

`<details>/<summary>` 无 JS 可用、天然可键盘操作、无需维护状态。规范 §6 记录了一个坑：**折叠/零字号的 `<details>` 子树不可点击**（实测过，label 点不动）。卡片里的动作按钮因此不能放在零字号容器内，摘要行也不要用尺寸塌缩的样式。

### D6：筛选分层与 URL 契约

`/billing` 现有 7 个可见控件 + 3 个隐藏参数。分层：

- **首要筛选（卡片页首屏）**：`payer`、`period`、`match_status` — 3 个，正好是 PRD §7.1 的上限。
- **更多筛选（`<details>` 内）**：`tenant_id`、`rent_period`、`allocation`、`direction`、`sort`、`arrival_from`、`arrival_to`。

`arrival_from`/`arrival_to` 今天就没有可见控件，只在已存在于 URL 时以 hidden input 保留（`billing_page.go:172-173`）。移进「更多筛选」后它们才第一次有可见入口——这是修复，不是新增。

**不新增任何查询参数名**。「更多筛选」的展开状态**不写 URL**：展开是可逆的纯 UI 状态，写进去会污染分享链接；而它包含的所有值本来就在 URL 里，收起不会丢。

`/tenants` 和 `/expenses` 今天**没有**筛选、排序、分页（研究已确认）。PRD 也没有要求给它们加。**本任务不给这两个页面新增筛选 UI**——只按 PRD §6.4 加默认排序（欠款／待处理在前）。加筛选是无 PRD 依据的范围扩张。

### D7：默认值变更（已批准）与它的陷阱

`/billing`：查询里既没有 `match_status` 也没有 `pending` 时，默认按「待处理」处理。

**陷阱**：不能简单地把 `filters.MatchStatus` 设成 `"pending"`。`transactions.go:258-262` 会把 `"pending"` 展开成 `PendingOnly` 并清空 `MatchStatus`，`matchStatusSelection` 再据此反推下拉框选中项。所以正确做法是在 `filtersFromQuery` 里：

1. 无 `match_status` 且无 `pending=1` → `PendingOnly = true`、`MatchStatus = ""`，`matchStatusSelection` 自然返回 `"pending"`，下拉框正确。
2. 显式 `match_status=all` → `PendingOnly = false`、`MatchStatus = ""`，表示「不过滤」。

**第 2 条需要新增**：`validateTransactionFilters`（`transactions.go:299-303`）目前不接受 `"all"`。而且下拉框的「全部状态」选项现在是 `value=""`（`billing_page.go:170`），变更后 `""` 的含义变成「默认＝待处理」，**必须把该选项改成 `value="all"`**，否则「查看全部」从 UI 上不可达——这正是 PRD §14.1 要求保留的能力。

连带必须一起改的（否则用户会掉进空列表出不来）：

- `清除筛选` 链接现在指向 `/billing`（`billing_page.go:175`），变更后它只会回到「待处理」。要么改指向 `/billing?match_status=all`，要么改文案为「重置为待处理」。
- 空状态文案「没有符合当前筛选条件的流水」必须附「查看全部」入口（PRD §14.1）。
- 状态徽章链接 `/billing?match_status={{.MatchStatus}}`（`billing_page.go:193`）今天会丢掉其他全部筛选条件；PRD §14.1 要求一并修正为保留当前筛选。

`/rent-dashboard`：`defaultRentDashboardFilters()`（`dashboard_filters.go:28-30`）的 `Status` 从 `"all"` 改为 `"unpaid"`。`Sort` 保持 `"status"`（已有 overdue→needs_review→partial→open→paid 的优先级）。`status=all` 已在 `validateRentDashboardFilters` 的白名单里，所以「查看全部」天然可达，无需改动。

**注意**：dashboard 的筛选是 Go 侧内存过滤（`filterAndSortRentDashboardRows`），不在 SQL 里；分页在过滤之后。改默认值不涉及索引或查询性能。

### D8：桌面不变性怎么保证

1. 新增规则全部落在 `@media (max-width: 640px)` 内，或写在 base 里但默认 `display: none`。
2. 复用上一轮的 `splitWorkspaceCSS(t)` 把共享样式表按第一个 640px 切开，断言桌面段不含新卡片规则。
3. `/billing` 的卡片退场规则要避开扁平化守卫的禁项：`box-shadow`/`rgba` **在 640px 块内是允许的**（守卫只看第一个 640px 块之前），但卡片本身不需要它们——用边框和背景色分组，与全站视觉一致（PRD §8）。
4. 桌面截图比对时的**已知差异**：`/billing` 与 `/rent-dashboard` 的默认筛选变了，首屏内容必然不同。验收脚本必须把这个已知差异排除，否则会产生假阳性。建议对这两页的桌面比对改为「带上 `match_status=all` / `status=all` 与基线比对」，这样剩下的任何差异都是真回归。

### D9：测试策略

**Go 字符串断言**（必要，不充分）。复用现有 helper：`executeTemplate`、`splitWorkspaceCSS`、`renderBillingPage`、`markupBetween`、`styleRuleFor`。

需要同步修改的现有测试（它们断言的是即将被替换的表格结构，是预期内的信号，不是回归）：

- `TestBillingActionCellCollapsesOnlyOnNarrowScreens` — 断言 `.transaction-table` 列声明与 `.txn-amount-mobile` 的存在与顺序
- `TestBillingListCarriesTheColumnSort` / dashboard 的排序断言 — 数 `class="sort-link` 并断言具体排序 href
- `TestPayerPreviewScrollsTheTableNotTheCard` — 只涉及预览页；`/billing/payer/preview` 本次不改结构，应当**保持通过**，若失败说明误伤了

**真实浏览器验收**（充分性来源）。上一轮实测证明字符串断言看不见三类缺陷：盒子长高后装饰错位、sr-only 漏写 `display:block`、折叠子树不可点击。卡片布局的几何问题正属于这一类。由子任务 1 交付可复现的验收环境。

### D10：触控与可访问性

沿用规范 §3.4（44px 下限，以及「盒子长高后要重新检查贴着盒子边缘的装饰」）与 §3.5（可聚焦元素必须保持可聚焦）。卡片新增的 `<summary>` 必须至少 44px 高，且折叠态下仍可聚焦、可键盘展开。

---

## 4. 子任务契约

子任务 2/3/4 的实现必须满足：

1. **不新增断点**。所有新的窄屏规则用 `@media (max-width: 640px)`。
2. **不动桌面段规则**。新增的桌面可见样式必须默认隐藏、窄屏激活。
3. **不改路由、POST action、查询参数名**。新增的只有 `match_status=all` 这一个取值。
4. **卡片只消费 view model**，不在模板里做格式化或拼状态文案。
5. **每页配 Go 回归断言**：卡片容器存在且默认隐藏、窄屏块内激活、该页的卡片字段渲染成功。
6. **每页跑一次浏览器验收**（子任务 1 的环境），报告落 `research/`。
7. **提交策略**：每个子任务独立提交，可单独回滚。默认筛选变更（D7）单独成一个提交，与卡片结构分开，便于只回退行为不改结构。

---

## 5. 兼容性与回滚

- **兼容**：桌面表格、路由、参数名、业务规则全部不变。唯一的行为变更是已批准的默认筛选值（PRD §14.1）。
- **回滚**：卡片结构默认 `display: none`，删掉窄屏激活规则即回到今天的状态。默认筛选变更因单独提交，可独立 revert。
- **风险最高的文件**：`main.go`（共享样式表 `workspacePageCSS` + `/tenants`、`/expenses` 的模板与 handler 都在里面）、`billing_page.go`（4KB 的行内动作单元格 + 页面脚本）、`dashboard.go`（筛选默认值 + 已有的窄屏块）。
- **`/billing` 的行内脚本**（`billing_page.go:217`）做三件事：`<details>` 折叠、把 `/billing/allocate` 表单改写成可重复的分割行、把 `/billing/revoke` 改成 GET 并删掉 `reason`。卡片路径**不能直接复用这个脚本**——三件事里只有第一件是卡片需要的，另两件依赖表格行的结构。

---

## 6. 未决 / 交给实现阶段

- `/tenants/:id` 的缴费历史、现金收款与两个预览页（PRD §6.5/§6.7/§6.8）只做窄屏收尾，不做卡片化；具体条目在子任务 4 的 `implement.md` 里逐条列。
- 真机抽查（PRD Phase 4）需要一次部署到 `bank.ddpl.top`。这是对外可见且难以撤销的动作，**实施到那一步时需单独取得授权**，不视为本任务已批准。
