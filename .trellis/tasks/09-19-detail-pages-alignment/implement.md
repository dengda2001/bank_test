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
