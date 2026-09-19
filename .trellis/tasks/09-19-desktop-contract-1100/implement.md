# 执行计划：桌面契约解冻与 1100 断点

本子任务必须先于 ②③④⑤ 完成。它是其余四个的样式基线。

## 执行顺序

1. [ ] 通读 `responsive-conventions.md`，标记所有依赖"桌面冻结"表述的段落
2. [ ] 查清 `max-width:980px` / `min-width:981px` 各自承担的行为（读规则 + 查测试 + 实机验证），记录结论
3. [ ] 改写 `:17` 的冻结契约表述（按 design.md §2.3 的三个条件）
4. [ ] 在共享样式表新增 `@media (max-width: 1100px)`，位置在基准之后、640 之前
5. [ ] **实机验证 1100 档是否真的生效**（design.md §2.1 第 2 步）—— 不生效则升级优先级或改用页面表追加
6. [ ] 同步更新因桌面解冻而失效的断言
7. [ ] 确认 640 档规则与断言未变（diff 证明）
8. [ ] 四档截图，存 `research/screenshots/`
9. [ ] 跑全量测试与 vet
10. [ ] 提交

不要跳过第 5 步 —— 它是本子任务唯一容易"看起来做完了但实际没生效"的地方。

## 验证命令

```bash
go test ./cmd/truelayer-demo/...
go vet ./...
git diff --check
```

改动前后对比 640 档：

```bash
git diff .trellis/spec/frontend/responsive-conventions.md
# 以及各 pages/*.css 中 max-width:640px 块的 diff，应无变化
```

## 风险文件

| 文件 | 风险 | 处置 |
|---|---|---|
| `web/static/css/workspace.css` | 全部页面共享 | 改后十一个页面全截图 |
| `cmd/truelayer-demo/mobile_layout_test.go` | 断言桌面不被污染，解冻后必然失败 | 与代码改动**同批**更新 |
| `responsive-conventions.md` | 是后续四个子任务的依据 | 改动要能被后续任务直接引用 |

## 开工前检查

- [ ] 用户已审阅本子任务的 `prd.md` / `design.md` / `implement.md`
- [ ] 已加载 `trellis-before-dev`
- [ ] 已确认本机可运行应用并截图
- [ ] 已确认不会对生产实例（`:8081` / `bank.ddpl.top`）发起写入
