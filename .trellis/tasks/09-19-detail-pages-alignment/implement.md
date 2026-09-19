# 执行计划：详情页对齐

依赖子任务 ① 先落地。

## 执行顺序

1. [ ] 修复 `room-detail.html:104` 的 wrapper 结构（design.md §2.1）
2. [ ] 排查另外三个详情页有无同类"桌面类 + 移动类同元素"写法
3. [ ] 房产详情结构对齐
4. [ ] 房间详情结构对齐 + 「未分配」格
5. [ ] 租客详情结构对齐 + 缴费历史范围预设 + 付款参考码与复制 + 代付与被代付表
6. [ ] 流水详情结构对齐 + 原始描述 code-block + 交易时间行 + 三操作入口
7. [ ] 实机确认 design.md §2.5 的未解观察，结论写入 `research/`
8. [ ] 四页 × 四档截图；`room-detail` 另加窄屏一次
9. [ ] 跑全量测试与 vet
10. [ ] 提交

第 1 步先做，因为它是唯一"代码看着对、渲染是错的"类缺陷；先修掉才能把后续的视觉对照建立在正确基线上。

## 验证命令

```bash
go test ./cmd/truelayer-demo/...
go vet ./...
git diff --check
```

重点回归：

```bash
go test ./cmd/truelayer-demo/ -run 'TenantProfile|RoomDetail|PropertyDetail|Transaction'
```

## 风险文件

| 文件 | 风险 | 处置 |
|---|---|---|
| `room-detail.css` | `:20` 与 `:25` 的优先级冲突是本子任务的核心 | 修后 PC 与窄屏两侧都验 |
| `tenant_detail.go` | 内联字符串模板，无编译期检查 | 改后实机渲染该页 |
| `tenant_detail.go:290` 付款识别 | 加参考码与复制按钮牵动既有共享/冲突标记 | 跑 `tenant_profile_test.go` |
| `transaction-detail.html` | 三操作入口涉及 detail-return 上下文 | 每个入口点一次，确认返回链路未断 |

## 开工前检查

- [ ] 子任务 ① 已完成
- [ ] 用户已审阅本子任务的 `prd.md` / `design.md` / `implement.md`
- [ ] 已确认「未分配」与「未覆盖」在数据层可区分（不可区分则先记录再决定，见 design.md §2.2）
- [ ] 已确认不会对生产实例发起写入

## 执行次序与并行边界（2026-09-19 主会话补记）

本子任务**排在 `09-19-list-pages-alignment`（④）之后**执行，不与它并跑。

原因是**共用样式文件**，两边原计划里都没提：

| 共享文件 | ④ 在用 | 本子任务在用 |
|---|---|---|
| `web/static/css/pages/entity-drawers.css` | `rooms.html`、`properties.html` | `room-detail.html`、`property-detail.html`、`tenancies.html` |
| `web/static/css/pages/object-navigation.css` | `rooms.html`、`properties.html` | `room-detail.html` |

并跑会导致提交时（Phase 3.4）无法把两个任务的改动分开。文件不相交时
`git add <路径>` 就能干净分离；共用一个文件就只能人工挑。

**开工前先读 ④ 的回报**：若 ④ 改过上述两个文件，它会在回报里点名写出改了哪几条规则，
本子任务在此基础上继续，避免互相覆盖。

**与 ② 的关系**：本子任务与 `09-19-shell-alignment`（②）的写入面不重叠，
② 先跑只是为了让截图基线稳定，不是文件冲突。

**端口**：轮到本子任务时用 `APP_PORT=18092`（18090 / 18091 可能仍被占用）。
`MYSQL_PORT=3306`，仍然**绝不允许**指向 `:8081` / `bank.ddpl.top`。

## 主会话独立复验（2026-09-20）

实施完成后由 `trellis-check` 独立复验，主会话再复核。**核心风险已澄清**：

- **`matching_service.go` 改的 94 行是纯提取**（`git diff -w` 后未配对行只剩函数签名、
  调用点与被移走的括号），`decorateTransactionPageRow` 体与被删内联块逐字相同；
  `rent_workspace_page.go` 的 155 行是新增 `UnallocatedAmount` 字段与新函数，纯增量。
  **未改任何金额口径、匹配判定或分摊结果** —— prd 的 Out of Scope 未被触碰。
- **8 条断言经独立反向验证**（复验者自己挑 8 条改坏实现 → 确认 FAIL → 从 `/tmp` 副本还原），
  全部非空。本项目此前抓到过空断言（永远为真的假测试），此项为必查。
- §2.5 的三条证据逐条复现通过，结论「截图是夹具产物、真实路径不可达」成立。

### 复验发现并修正的两处（均已补测试钉住）

**1. 租客责任表「责任人」没链接**（复验者补，本子任务 prd 明确要求「责任人可跳租客详情」）。
`room-detail.html` 的桌面表与窄屏责任行都改成 `href="/tenants/{{.TenantID}}"`，
无 `TenantID` 时退回纯文本。补测试 `TestRoomDetailResponsiblePartyLinksToTheTenant`，
经反向验证会失败。

**2. `property-detail` 移动端「基础信息」卡在任何宽度都看不见**（实施者未提及，
复验者报为「意外变化」，主会话核实后确认是**既有缺陷**）。

`property-detail.css` 的 ≤640 档原本用 `.property-detail-stack > section:nth-child(2)`
表达「隐藏关联支出」。但 `.property-detail-stack` 这个类**同时用在主区（:57）和侧栏（:77）
两个元素上**，`:nth-child(2)` 于是也命中了侧栏第 2 个 section —— 正是 `property-mobile-facts`
（「基础信息」卡）。该规则优先级 (0,2,0) 高于基础档的 `.property-mobile-facts{display:none}`
与 ≤640 档的 `{display:block}`（均 0,1,0），把它在**所有宽度**都藏了。

⇒ 与 §2.1 的房间详情缺陷**同一类**：规则跨元素误伤。实施者插入收支构成桥后
`:nth-child(2)` 位置还会再次错位，因而改为按类定位（`.property-expense-list`），
顺带让这张卡恢复可达。**这是修复，不是回归** —— 390 实测无横向溢出。
补测试 `TestPropertyDetailHidesTheExpenseListByClassNotByChildIndex`，
经反向验证会失败。

### 复验者列出、本任务未修（留待产品决定）

- **「查看分配」跳转缺失**：原型「关联收款」表有「操作-查看流水」列，实现里该表无操作列。
  补它需要区分银行流水（`PaymentID`=交易 ID）与现金收款（`PaymentID`=收据 ID），属产品决定。
- **AC1 两处保留**：三页第 4 张指标卡语义与原型不同（非改词，是**不同量**）；`tenant-detail`
  首卡无 `metric-primary` 强调。**两项均为 HEAD 既有状态，非本子任务引入。**
- **4 条既有显示问题**（缴费历史表列被压成竖排、`room-detail`/`property-detail` 表在窄列内横滚、
  `property-detail` 房间表比原型多「月租」列）：经复核**均为 HEAD 既有**，非本次引入，留着不动。

### 未验证到的（诚实记录）

- `roomUnallocatedCents` / `listTenantPaidByOtherRows` 的边界（多币种、`record_status=voided`、
  `matched_tenant_id IS NULL`）**无 MySQL 直连单测**，只有实机两条路径 + 复验者直连库核数。
  这是真实缺口。
- 页头三操作只有「标记非租金」做了真实提交；`确认匹配`/`编辑分配` 只比对表单 action 与字段集。
- 1024/1366/1920 三档只断言溢出，未逐档存 DOM 结构快照（结构证据取 1440）。

### 越界口径差异（记录，未改）

**「已覆盖」可能包含没有可展示来源的分配** —— 来源交易非 `income` 方向，或分配归属人与
责任人不一致时，`ledgerPaidAmount` 计入「已覆盖」而 `workspacePayments` 会跳过它，
于是「付款来源」列落 `—`、「关联收款」无行。这是**显示口径**问题，不影响金额与余额。
超出本子任务 prd 范围，仅记录。
