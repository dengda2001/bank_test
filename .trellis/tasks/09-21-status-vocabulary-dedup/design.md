# 技术设计：状态控件去重与文案统一

## 边界

只改**展示层**：Go 模板字符串与内嵌 HTML 里的控件与文案。

明确不改：

- 存储状态值与其校验白名单（`transactions.go:317-321`）。
- URL 参数名与取值（`match_status`、`status`、`scope`）。
- 状态推导：`ledgerObligationStatus`（`ledger.go:119-139`）、
  `obligationStatus`（`obligations.go:165-170`）、
  `workspaceWorstStatus`（`rent_workspace.go:920-928`）、
  伪筛选展开（`pendingMatchStatuses`、`workspaceStatusMatches`）。
- `statusLabel`（`transactions.go:461-468`）与 `rentStatusLabel`
  （`obligations.go:589-597`）的返回值——它们已经是「关联」词系，正是要对齐的目标。

## 落点

| 文件 | 位置 | 改动 |
|---|---|---|
| `cmd/truelayer-demo/billing_page.go` | 页签，约 254 行 | 文案 已匹配 → 已关联；href 不变 |
| `cmd/truelayer-demo/billing_page.go` | 快捷筛选，约 295 行 | 删除整个 `<select name="match_status">`，保留同表单内其余控件 |
| `cmd/truelayer-demo/web/templates/pages/rent-workspace.html` | 状态筛选，第 50 行 | 3 个 option 文案对齐徽章；`value` 不变 |
| `cmd/truelayer-demo/billing_layout_test.go` | 53-61 行等 | 改写"两个下拉都存在"的断言 |

## 关键取舍

### 删哪一个下拉

`/transactions` 与 `/billing` 都由 `handleBilling` 渲染（`main.go:502`、`main.go:741`），
靠 `PageKey` 分流。两者控件不同：

- `/transactions`（`PageKey == "transactions"`）：快捷筛选表单可见；
  筛选栏被包在 `<details class="transaction-route-filters">` 里折叠
  （`billing_page.go:299` 与 `:313`）。
- `/billing`：不渲染快捷筛选表单；筛选栏直接可见。

**若删筛选栏里的下拉**，`/billing` 会彻底失去状态筛选。
故只能删快捷筛选里的那一个——它只影响 `/transactions`，筛选栏对两个页面都保留。

### 为什么是删而不是"改成一样的"

两个下拉最初是同一套词，已经漂移过一次（快捷筛选那份丢了「已忽略」）。
留两个同义控件无法阻止再次漂移，而页签（待处理/已关联/全部）已经覆盖了
决策层最常用的三个状态。细分状态退到「搜索」展开区内，属可接受的信息层级。

### 主页往哪个方向对齐

两种可能：把徽章改成下拉的词，或把下拉改成徽章的词。选后者，因为
`workspaceStatusLabel` 的输出出现在更多位置（房产行、房间行、租客行、
移动端卡片、详情页），行内徽章是被复用的一侧；且下拉是唯一一处用
「未到期未缴」这种长描述的控件。

## 契约（改完后必须成立）

- `/transactions` 与 `/billing` 渲染出的 `match_status` 下拉合计各 1 个，均含 8 项。
- 全部 6 个存储状态 + 伪筛选 `pending` 仍各有一个可达入口。
- 主页 8 个 option 的 `value` 集合不变：
  `all, unpaid, needs_review, overdue, partial, open, paid, vacant`。
- 主页每个 option 的文案等于 `workspaceStatusLabel(对应状态)` 的输出
  （`all` 除外，它不是状态）。

## 兼容性与回滚

- **URL 兼容**：无参数名或取值变化，旧书签（如 `?match_status=partial`）照常工作。
- **无数据迁移**：不涉及 schema、缓存或已存记录。
- **回滚**：改动集中在 3 个文件，一次 revert 即可；无状态残留。
- **不做的事**：不引入新的共享组件或抽象。此处是文案与控件的删改，
  抽一层反而增加后续对齐原型的阻力。

## 留给 dd 的决策点

1. 主页列头「已交满 / 未交满」（房间计数）与下拉改后的「已缴清 / 未缴」
   会出现新的不一致——是统一，还是承认二者用途不同？
2. `needs_review` 的文案在 `rentStatusLabel`（需处理）与
   `workspaceStatusLabel`（待处理）之间不一致，是否一并统一。
3. `/bills` 的第三套用词是否纳入后续任务。
