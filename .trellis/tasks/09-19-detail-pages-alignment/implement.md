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
