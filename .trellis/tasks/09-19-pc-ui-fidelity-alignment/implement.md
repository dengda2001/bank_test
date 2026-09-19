# 执行计划：PC 端原型 UI 还原度对齐

## 执行顺序

```
① 桌面契约解冻与 1100 断点   ← 必须先完成
        │
        ├── ② 全局外壳对齐      ← 其 toast 容器在共享壳内，③④⑤ 自动获得
        ├── ④ 列表页对齐        ← 复用 ② 的外壳
        │      │
        │      └── ③ 收租总览对齐   ← 硬依赖 ④：复用其定稿的平账表单
        └── ⑤ 详情页对齐        ← 复用 ② 的外壳
```

②④⑤ 之间无依赖，① 完成后可并行推进。**③ 是例外**：用户已确认它串行排在 ④ 之后，因为租客视角的「一键平账」必须复用 ④ 定稿的表单，两边各自实现会产生同一 action 的两个渲染点。

**不要跳过 ①** —— 它改动桌面渲染基线，其余四个都在同一批 CSS 上工作。

## 父任务清单

1. [ ] 确认 `figma/rentops-desktop-suite.html` 可在浏览器打开，作为对照基准
2. [ ] 确认子任务 ① 已完成（`responsive-conventions.md` 已更新、1100 档已落地、受影响测试已同步）
3. [ ] 逐子任务复核：完成后按子任务的验收项实机检查，不只读它的完成报告
4. [ ] 集成复核：全部子任务完成后，在 1024 / 1366 / 1440 / 1920 四档逐页截图，与原型对照
5. [ ] 判断降级模板（`dashboard.go:210`）的去留，写入结论
6. [ ] 汇总仍存在的差异，逐条给出保留理由
7. [ ] 更新 `.trellis/spec/frontend/` 相关约定
8. [ ] 提交并归档

## 验证命令

```bash
# Go 测试（含响应式断言）
go test ./cmd/truelayer-demo/... ./cmd/rentops-e2e/...

# 静态检查
go vet ./...

# 空白与冲突标记
git diff --check
```

浏览器验证：`figma/rentops-desktop-suite.html`（原型）与本地运行的应用逐页对照，四档宽度各截一次，断言 `document.documentElement.scrollWidth === document.documentElement.clientWidth`。

可复用的 Playwright 脚本在 `.trellis/tasks/archive/2026-09/09-16-mobile-responsive-audit/research/harness/`（29 个 `.mjs`，已随任务归档）。适合几何、命中区域、焦点类问题；`desktop-full.mjs` 是现成的桌面回归对，`probe-wide.mjs` 能把页面级溢出与滚动容器内的溢出分开。

### 关于那套 harness 的安全边界（必读）

该 README 的原文是：**"There is no sandbox. `:8081` is the production app."** —— 它由 nginx 以 `bank.ddpl.top` 对外，跑在**生产数据库**上，并没有隔离的沙箱实例。演示账号 `rentops-demo` 的数据行就在生产库里。

因此复用这套脚本时：

- 只发只读请求：GET，加上那一个只读的 `POST /billing/payer/preview`。
- 不得对任何共享或生产实例执行写入、平账、生成账单、批量提醒等动作。本次涉及写操作（平账表单、生成账单）的验证，必须在本机自有实例与自有数据上进行。
- 不要照搬 README 里那段"构建并 `systemctl restart rentops-app-live.service`"的部署片段 —— 它会把工作区（含用户未提交改动）直接推上生产。

## 风险文件与回滚点

| 文件 | 风险 | 回滚 |
|---|---|---|
| `web/static/css/workspace.css` | 被所有页面共享，改一处可能波及未预期页面 | 每个子任务独立提交，可单独 revert |
| `web/templates/partials/workspace-nav.html` | 所有页面经它渲染外壳；改错会同时打挂全部页面 | 同上；改后必须全页面截图而非只查目标页 |
| `cmd/truelayer-demo/mobile_layout_test.go` | 断言桌面不被污染；桌面解冻后必然失败 | 与代码改动**同批**更新，不可延后 |
| `page_data_routes.go` / `rent_collection_pages.go` | 内联字符串模板，无编译期检查 | 改动后必须实机渲染该页 |

**列出但不要动**：工作区有 82 项未提交变更（含用户自己的改动与其它任务的文件）。不要 `git checkout`、`git stash`、`git clean`，不要清理 `.DS_Store` 或未跟踪目录。

## 开工前检查

- [ ] 用户已审阅 `prd.md`、`design.md`、`implement.md`
- [ ] 另一个 agent 已停止（旧任务 `09-19-prd-prototype-page-audit` 已归档，不应再有进程写它）
- [ ] 已加载 `trellis-before-dev` 与 `.trellis/spec/frontend/responsive-conventions.md`
- [ ] 已确认本机可运行应用并截图
- [ ] 子任务 ① 的 PRD 已被审阅 —— 它决定其余四个的样式基线
