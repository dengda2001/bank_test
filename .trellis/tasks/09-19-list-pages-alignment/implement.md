# 执行计划：列表页对齐

依赖子任务 ① 先落地。**③ 依赖本子任务先完成** —— 第 5～6 步定稿的平账表单是 ③ 的输入。

## 执行顺序

1. [ ] 逐页清点现有 filterbar 控件，标出哪些有真实数据支撑、哪些没有
2. [ ] 逐页统一为「页头 + filterbar + 表格 + 操作列」，按 design.md §2.1 只渲染有效控件
3. [ ] 补齐各页行内操作
4. [ ] 修复 `rooms.html:61` 的孤立「自」字
5. [ ] `/bills` 平账表单补齐字段（不改已有字段名与 action）
6. [ ] 处理方式下拉按 design.md §2.2 只启用「匹配现有收款」
7. [ ] `/bills`「生成本月账单」改显式动作，保持幂等
8. [ ] `/dunning` 主操作文案改「批量发送提醒」
9. [ ] `/bank` 补「添加银行账户」、「同步银行流水」改「立即同步」
10. [ ] 十页 × 四档截图
11. [ ] 跑全量测试与 vet
12. [ ] 提交

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

- [ ] 子任务 ① 已完成
- [ ] 用户已审阅本子任务的 `prd.md` / `design.md` / `implement.md`
- [ ] 已知悉 filterbar 按页渲染真实维度：`/tenants`、`/bank` 不加月份下拉（已确认决策，见 `prd.md`）
- [ ] 已确认平账/生成账单的验证在本机自有实例上进行，**不对生产实例发起写入**
