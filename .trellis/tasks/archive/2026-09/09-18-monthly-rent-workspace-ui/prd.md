# 前端月度工作台

## Goal

实现默认房子视角、房子/房间/租客切换、房产下钻、房间列表与详情、租客责任和代付展示、支出与空状态，以及桌面/移动端交互。

## Requirements

- 在本任务提供的模板、静态资源和 view model 边界上实现统一月度工作台，不新增独立前端工程。
- 默认进入房子视角，支持房子、房间、租客三个视角切换并保留 URL 上下文。
- 房产视角展示收缴指标、支出和经营净额，并支持下钻到房间。
- 房间视角展示空置房、收缴状态和房间详情；租客视角展示责任、付款来源和催缴状态。
- 提供多人责任、他人代付、部分代付、超付和公共支出的清晰交互反馈。
- 遵循现有 640px 响应式约定、44px 触控目标和键盘可访问性要求。

## Acceptance Criteria

- [x] 首次加载默认 `view=properties`，刷新、返回和切换后月份/视角/筛选不丢失。
- [x] 房产、房间、租客三种视角使用同一月份和金额口径。
- [x] 空置房金额显示 `—`；待处理与空置视觉和文案明确区分。
- [x] 已被他人完整代付的租客显示已结清且不进入催缴候选。
- [x] 桌面端、移动端、键盘导航和现有布局测试通过。

## Dependencies

- 前置：`09-18-dashboard-aggregates-and-handlers`、`09-18-frontend-template-extraction`。
- 后置：集成验收任务。

## Verification

- 页面渲染和交互状态测试。
- `cmd/truelayer-demo/mobile_layout_test.go` 及相关布局测试。
- 使用核心账务 fixture 验证房产、房间和租客视图。

## Completion Notes

- 月份切换表单会携带当前视角、房产/房间范围、搜索、状态、排序和分页大小，避免改变月份时丢失工作上下文。
- 新工作台页面复用 `.app` / `.content` 共享外壳；桌面显示表格，640px 以下显示同一 view model 的移动卡片。
- 空置、待处理、部分缴纳、已缴清和他人代付均通过状态标签、金额字段和说明文案表达；已缴清的他人代付租客没有催缴链接。
- `go test ./...`、`go vet ./...`、`git diff --check` 均通过；本地未配置 `RENTOPS_MYSQL_TEST_DSN`，数据库专项测试按约定跳过。

## Notes

- Keep `prd.md` focused on requirements, constraints, and acceptance criteria.
- Lightweight tasks can remain PRD-only.
- For complex tasks, add `design.md` for technical design and `implement.md` for execution planning before `task.py start`.
