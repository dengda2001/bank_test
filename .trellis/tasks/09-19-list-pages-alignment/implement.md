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
