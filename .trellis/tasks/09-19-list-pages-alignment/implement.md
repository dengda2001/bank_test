# 执行计划：列表页对齐

依赖子任务 ① 先落地。**③ 依赖本子任务先完成** —— 第 5～6 步定稿的平账表单是 ③ 的输入。

## 执行顺序

1. [x] 逐页清点现有 filterbar 控件，标出哪些有真实数据支撑、哪些没有
2. [x] 逐页统一为「页头 + filterbar + 表格 + 操作列」，按 design.md §2.1 只渲染有效控件
3. [x] 补齐各页行内操作
4. [x] 修复 `rooms.html:61` 的孤立「自」字
5. [x] `/bills` 平账表单补齐字段（不改已有字段名与 action）
6. [x] 处理方式下拉按 design.md §2.2 只启用「匹配现有收款」
7. [x] `/bills`「生成本月账单」改显式动作，保持幂等
8. [x] `/dunning` 主操作文案改「批量发送提醒」
9. [x] `/bank` 补「添加银行账户」、「同步银行流水」改「立即同步」
10. [x] 十页 × 四档截图
11. [x] 跑全量测试与 vet
12. [ ] 提交（不在本代理职责内）

第 1 步必须先做：不先清点就统一结构，会直接滑向"复制同一段 filterbar"，即 A1 禁止的假控件。

## 验证命令

```bash
go test ./cmd/truelayer-demo/...
go vet ./...
git diff --check
```

重点回归：

```bash
go test ./cmd/truelayer-demo/ -run 'PageData|RentCollection|Billing|Rooms'
```

## 风险文件

| 文件 | 风险 | 处置 |
|---|---|---|
| `rent_collection_pages.go` | 内联字符串模板，无编译期检查 | 改后实机渲染 `/bills`、`/dunning` |
| `page_data_routes.go` | 同上，且体量大 | 改后实机渲染对应页 |
| `rent_collection_pages.go:49` 平账表单 | 字段改动牵连 handler | 提交测试覆盖四个处理方式分支 |
| `web/static/css/pages/*.css` | 十份页面样式，可能互相覆盖 | 每改一页跑十页截图 |

## 开工前检查

- [x] 子任务 ① 已完成（`c944538`，已归档）
- [ ] 用户已审阅本子任务的 `prd.md` / `design.md` / `implement.md`
- [x] 已知悉 filterbar 按页渲染真实维度：`/tenants`、`/bank` 不加月份下拉（已确认决策，见 `prd.md`）
- [x] 已确认平账/生成账单的验证在本机自有实例上进行，**不对生产实例发起写入**

## 与 ②③⑤ 的并行边界（2026-09-19 主会话补记）

本子任务与 `09-19-shell-alignment`（②）**并跑**。已核：本子任务对 ② 的任何交付物
（toast / 搜索框 / 计数 / 外壳 / 导航）引用均为 **0 处**，文件写入面也不重叠。

**但本子任务与 `09-19-detail-pages-alignment`（⑤）共用两个样式文件**，两边计划里都没提：

| 共享文件 | 本子任务在用 | ⑤ 在用 |
|---|---|---|
| `web/static/css/pages/entity-drawers.css` | `rooms.html`、`properties.html` | `room-detail.html`、`property-detail.html`、`tenancies.html` |
| `web/static/css/pages/object-navigation.css` | `rooms.html`、`properties.html` | `room-detail.html` |

⇒ 为免提交时无法分离，**⑤ 排在本子任务之后**。若确实需要改这两个文件，在回报里
**点名写出改了哪几条规则**，便于日后与 ⑤ 的改动对照。

**端口**：本子任务用 `APP_PORT=18091`（② 占着 18090）。
`MYSQL_PORT=3306`，仍然**绝不允许**指向 `:8081` / `bank.ddpl.top`。

**尚未由用户逐行审阅**：本子任务的 `prd.md` / `design.md` / `implement.md` 是主会话此前
编写的，用户对整批子任务给了执行授权，但未逐行读过。其中一处产品决定值得留意 ——
filterbar 按页渲染（`/tenants`、`/bank` 不加月份下拉），理由与核查实据见 `prd.md`
「已确认的决策」。

## 收尾补正（2026-09-20，主会话）

实施者在回报里把 `/transactions` 的行内操作列为「文案没动」，理由是 `import bank file`
不在本子任务范围。**这个理由把两件事混了**：`prd.md` 的页面表里「导入银行文件」是
**页头主操作**那一栏（父任务已排除，且代码里从未建过这个入口），而 `处理` 是
**行内操作**那一栏 —— 同一张表里明明白白写着 `/transactions` 的行内操作应当是
**「查看详情」**。九个页面都统一了，只有这一页漏了。

- 已改：`billing_page.go:297` 的 `>处理</a>` → `>查看详情</a>`。
- `处理流水`（`billing_page.go:340`、`workspace-nav.html:32` 等）**未动** —— 那是
  `/transactions` 的**页面名**，不是行内操作文案。改它会动到侧边栏导航与多处页面标题。
- 已补测试 `TestTransactionRouteActionColumnSaysViewDetails`（`list_pages_alignment_test.go`）。
  该测试经反向验证：把文案改回 `处理` 会失败，不是空断言。

**页头主操作**：`/transactions` 依父任务决定不渲染任何页头主操作，实施者的实现与之一致。

## 主会话独立复验记录（2026-09-20）

复验 agent 只改了 `main.go` 与 `list_pages_alignment_test.go` 两个文件；
实例已停、可丢弃库已 drop、端口 18094 已释放；未触碰 `:8081` / `bank.ddpl.top`，
未做任何 git 写操作。

### 复验发现并修复的真实缺陷

**`/tenants` 行内操作仍写着「详情」。** `2395a28` 声称把列表页行内操作统一为
「查看详情」，实际漏了这一页的**两处**：桌面表格 `main.go:2690` 与移动卡片
`main.go:2676`。`8c62c3f` 对 `main.go` 的改动只加了 `data-toast`，从未碰行内文案。

复验已修，并给 `TestListPageTemplatesKeepHeadActionAndActionColumn` 补了 `tenants`
子用例（页头 `添加租客` / `>操作</th>` / `>查看详情</a>`），经变异验证非空转。

**主会话补充**：同一次统一还漏了第三处 —— 房产详情内嵌的「房间收款概览」表
（`property-detail.html`，桌面表格与移动卡片各一处），已由主会话单独提交 `da1b2ea`。
三处合起来，`2395a28` 的统一才真正覆盖全站；主会话已用一次全仓行内操作标签普查确认
（17 处「查看详情」，无裸「详情」/「处理」）。

### 主会话独立复核后**推翻**的复验结论

**「`/properties`、`/rooms` 过滤到 0 行没有空状态文案」——不成立。**
主会话在真实实例上逐页实测（`scripts/audit/probe-empty-states.mjs`，
7 个列表页 × 390/1440 两档，用不可能命中的词构造 0 行）：

```
properties / rooms / tenancies / bills / expenses / cash-receipts / transactions
   全部 ok：rows=0 且可见空状态文案非空
```

`properties.html:65` 与 `rooms.html:68` 的空状态在 `{{if .Rows}}…{{else}}` 的
`{{else}}` 分支里，**不在桌面/移动任一分支内**，所以两档都显示。
复验当时很可能是用了页面不认的参数名（`/transactions` 用的是 `payer` 而不是
`search`）导致没构造出 0 行，从而误判。故本项**不立项、不改代码**。

### 主会话独立复核确认的既有缺陷（不属本任务）

`TestDunningDashboardHTTPWorkflowOnMySQL` 在带 `RENTOPS_MYSQL_TEST_DSN` 时失败，
复验在 ④ 之前的 `71bf16d` 上同样失败 —— 既有缺陷，非本轮引入，但它意味着
**带真库的 CI 是红的**。记录在案，本轮不改。

### 复验结论

14 条 AC 中 13 条通过、1 条部分通过（AC 10 的「立即同步」需 TrueLayer 沙盒授权才能
点通；路由本身接受 POST 并返回 302，已实测）。逐控件真实提交验证 7 个列表页的
筛选器没有「URL 变了列表没变」的假控件，每个恒等项都做了反向验证。
10 组变异全部被捕获且还原后字节一致。

### 截图证据已刷新

复验指出 `transactions-*.png` 是 `2395a28` 改文案之前拍的。主会话核对后发现
**44 张全部过期**（② 改的共享外壳影响每一页、① 又动了桌面基线），不只是这两页。
已用 `scripts/audit/probe-integration-review.mjs` 按原型视口矩阵
（1024×768 / 1366×768 / 1440×900 / 1920×1080）重拍并整批替换，
逐张尺寸与旧集完全一致。
