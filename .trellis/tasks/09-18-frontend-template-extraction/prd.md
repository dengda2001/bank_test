# 模板外置与前端边界

## Goal

把当前散落在 Go 源码字符串中的工作台模板、样式和脚本外置，并建立“同一份页面数据、桌面与移动两套展示结构”的前端边界，为后续移动端整体样式重构提供稳定的扩展点。

本任务是前端基础设施任务，不是一次完整的移动端视觉改版。它要让后续 UI 任务可以独立调整手机的信息架构和交互，同时保持桌面端的表格效率与后端接口稳定。

## Requirements

- 将父任务涉及的工作台页面及共享外壳的 HTML 模板、CSS、页面脚本从 Go 的大段 raw string 中外置为可编辑文件；迁移范围至少覆盖月度工作台会改动的页面和共享资源，其他页面可以沿同一机制渐进迁移。
- 通过 Go 的嵌入资源机制继续打包成单个可部署二进制，并提供同源静态资源访问；不新增必须运行 Node/npm 的生产部署步骤。
- 在 handler/service 与模板之间建立 typed view model 边界。模板消费页面 view model 和行级 view model，不直接消费数据库记录，也不负责金额格式化、状态判断或拼接业务文案。
- 对需要不同信息架构的列表同时保留独立的桌面和移动展示结构：桌面可以使用表格，移动端使用卡片、分组列表或折叠详情。两者共享页面数据和业务语义，由 CSS 视口规则切换，不使用 User-Agent 服务端分支。
- 共享层只承载设计 token、基础排版、导航和通用控件；页面层允许分别维护桌面/移动布局和交互样式，避免以后修改移动端必须改动桌面表格。
- 保留现有路由、POST action、查询参数、权限校验、URL 上下文、渐进增强和无 JavaScript 时的核心浏览/提交能力。模板拆分不得改变业务规则。
- 不在本任务引入 React、Vue、Svelte 或新的 SPA/API 体系；为未来可能的独立前端保留清晰的页面数据/API 演进边界，但不提前承担完整前端迁移。

## Acceptance Criteria

- [x] 迁移范围内不再由业务 Go 文件承载大段用户可见 HTML、CSS 或页面脚本；模板、样式、脚本有清晰的目录归属和命名规则。
- [x] 应用仍可通过单个 Go 二进制运行，模板能在启动/测试时被解析，静态资源能通过同源路径加载，不依赖本地源码目录或 Node 构建产物。
- [x] 至少一个核心列表页完成桌面/移动双展示路径：两套结构消费同一个 typed view model，桌面默认显示表格，窄屏显示卡片/列表，不能依赖横向滚动作为手机主流程。
- [x] 新增页面或移动展示变体时，主要工作集中在模板、静态资源和 view model，而不是继续向 handler 追加 HTML 字符串；新增页面的组织方式在文档中有示例。
- [x] 现有路由、表单提交、查询参数、鉴权和业务数据口径保持兼容，已有服务端渲染与移动布局回归测试通过。
- [x] 通过 Go 测试、静态检查和差异检查；未提前启动后续月度工作台的完整视觉重构。

## Notes

- 前置：`09-18-dashboard-aggregates-and-handlers`。后端页面数据和操作契约先稳定，再抽离展示边界。
- 后置：`09-18-monthly-rent-workspace-ui`。该任务在本任务提供的边界上实现月度工作台和具体桌面/移动交互。
- 与既有 `09-16-mobile-friendly-workspace` 的关系：复用其“桌面/移动两种阅读模式”和移动端不得依赖大表格横向滚动的结论；本任务负责把结论落实为可维护的模板和资源边界，不重复规划每个页面的视觉细节。
- 本任务不要求一次性清理所有历史页面；但新代码不得继续扩大 Go 文件内嵌模板的范围。

## Verification

- `go test ./cmd/truelayer-demo -count=1`
- `go test ./...`
- `go vet ./...`
- `git diff --check`
- 新增嵌入资源、静态资源路由、typed view model、桌面/移动双 DOM 和无参考号展示回归测试均通过。
- `RENTOPS_MYSQL_TEST_DSN` 未配置；本任务不新增数据库行为，MySQL 专项测试按项目约定跳过。

- Keep `prd.md` focused on requirements, constraints, and acceptance criteria.
- Lightweight tasks can remain PRD-only.
- For complex tasks, add `design.md` for technical design and `implement.md` for execution planning before `task.py start`.
