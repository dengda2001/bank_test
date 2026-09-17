# 设计：模板外置与桌面/移动前端边界

## 1. 现状与设计目标

当前 `cmd/truelayer-demo` 使用 `html/template` 在 Go 源码中定义共享导航、页面 HTML、CSS 和脚本。`workspace_shell.go` 负责共享模板 clone，`main.go`、`dashboard.go`、`billing_page.go` 等文件同时承担 handler、页面数据准备和大段展示字符串。项目没有独立的前端工程或 Node 构建链。

这套方式适合早期单体 SSR，但会产生两个问题：

1. 修改布局需要在业务 Go 文件中编辑 HTML/CSS/JS，模板审查和前端迭代成本高。
2. 移动端后续需要与桌面端不同的信息架构时，容易继续把卡片逻辑塞进桌面表格，或让 handler 按 User-Agent 选择页面，最终造成重复业务判断和不可响应式的服务端分支。

本任务的目标是拆开“数据是什么”和“怎么展示”，而不是立即把 SSR 改成 SPA。

## 2. 推荐边界

```text
数据库 / Repository
        ↓
账务服务与聚合查询
        ↓
页面 handler：权限、参数、页面级 view model
        ↓
共享 typed view model ───────────────┐
        ↓                            │
桌面模板（表格/高密度）          移动模板（卡片/折叠/任务流）
        └──────── CSS 视口规则选择 ───┘
        ↓
浏览器：同源 CSS / JS 静态资源，JS 只做渐进增强
```

handler 不判断“这是手机还是桌面”，也不在字符串中拼接 HTML。页面模板决定标记结构，CSS 决定当前视口显示哪套结构；两套模板只共享业务字段、状态和操作 URL。

## 3. 资源组织建议

具体根目录可结合包规范落地，建议放在 `cmd/truelayer-demo/web/`，使资源和当前 SSR 应用同包管理：

```text
cmd/truelayer-demo/web/
├── embed.go                    # go:embed，导出 templates/static FS
├── templates/
│   ├── layouts/
│   │   └── workspace.html      # 页面外壳、公共 block
│   ├── partials/
│   │   ├── navigation.html
│   │   └── form-controls.html
│   └── pages/
│       ├── rent-dashboard/
│       │   ├── page.html
│       │   ├── desktop.html
│       │   └── mobile.html
│       ├── billing/
│       │   ├── page.html
│       │   ├── desktop.html
│       │   └── mobile.html
│       └── ...
└── static/
    ├── css/
    │   ├── tokens.css
    │   ├── base.css
    │   ├── workspace.css
    │   ├── pages/
    │   ├── desktop/
    │   └── mobile/
    └── js/
        ├── workspace.js
        └── pages/
```

目录不是为了强行规定每个页面必须有三份文件，而是让新增页面有可预测的位置。简单页面可以只有一个模板；只有在桌面和移动信息架构明显不同的页面才拆成 `desktop.html` 与 `mobile.html`。

`embed.go` 使用 `//go:embed templates static` 导出资源文件系统。应用启动时从嵌入 FS 解析模板，并把 `static` 子目录交给同源静态文件 handler。模板与静态资源不读取本地工作目录，保持单二进制部署和测试环境一致。

## 4. 页面数据契约

每个迁移页面增加明确的页面 view model；列表项使用语义化的行级 view model。例如：

```go
type RentDashboardPage struct {
    Workspace WorkspaceView
    Filters   RentDashboardFiltersView
    Summary   RentDashboardSummaryView
    Rows      []RentDashboardRowView
}

type RentDashboardRowView struct {
    TenantName   string
    RoomLabel    string
    Amounts      AmountBreakdownView
    Status       StatusView
    DetailURL    string
    PrimaryAction *ActionView
}
```

字段名称和类型以实际页面为准，重点是：

- 金额、日期、状态标签、显示文案和操作可见性在 Go 侧一次准备；模板不重新推导业务状态。
- 桌面行和移动卡片都消费同一个 `Rows`，移动模板可以少展示字段或把字段重新分组，但不能另起一套账务计算。
- URL、表单 action、hidden 参数由 view model 或通用模板辅助函数生成，保持现有路由和查询上下文。
- 页面级数据与布局选择分离。不能为了输出移动模板而复制一份 repository 查询或 handler 业务分支。

这层 view model 也是未来前端框架迁移的缓冲区：将来若需要独立前端，可以把它整理成 API DTO；但本任务不假设 SSR 模板和 API DTO 必须完全相同，也不提前设计一套未经需求验证的 API。

## 5. 桌面/移动展示策略

需要不同信息架构的列表采用双 DOM：

```html
<section class="rent-dashboard-desktop">...</section>
<section class="rent-dashboard-mobile">...</section>
```

桌面结构默认可见，移动结构默认隐藏；在既有 `640px` 断点内切换显示。移动结构使用卡片、分组列表或 `<details>` 渐进披露，不把宽表格 CSS 重排成卡片，也不把手机主流程交给横向滚动。

这样会增加部分 HTML 体积和双模板漂移风险，但换来更清晰的语义、稳定的操作区和真正独立的移动信息架构。漂移风险由共享 typed view model、同一组渲染 fixture、桌面/移动字段对照测试来控制。

共享 CSS 只放 token、基础排版、导航、表单和通用控件。页面专属的布局规则放在页面目录；移动专属规则放在窄屏媒体块或 `mobile/` 资源中，默认不影响桌面。脚本按页面拆分，并以渐进增强为原则：核心导航、筛选和表单在无 JS 时仍能工作。

## 6. 关键取舍

| 选择 | 结论 | 原因 |
| --- | --- | --- |
| 继续 SSR，先不引入 SPA | 采用 | 当前产品以表单、列表、单体部署为主；能先解决维护边界，减少构建和部署复杂度 |
| `go:embed` 外置资源 | 采用 | 文件可独立编辑，同时保留单二进制和离线/测试运行能力 |
| 桌面/移动双 DOM | 采用 | 两端信息架构不同，符合既有移动端设计；比 CSS 重排表格更可维护 |
| User-Agent 服务端分支 | 不采用 | 旋转屏幕和窗口变化不能及时切换，也会造成 URL/缓存和测试复杂度 |
| 本任务直接建立完整 API | 不采用 | 会把模板外置扩大成后端 API 重构；未来独立前端应另立接口契约任务 |

## 7. 兼容与回滚

- 先让外置模板在现有 handler 下渲染出等价页面，再逐页引入双展示结构，降低一次性风险。
- 现有路由、POST action、查询参数和权限校验保持不变；模板拆分不改变账务服务和 repository。
- 资源加载失败时应在测试中可识别；不通过读取磁盘源码目录作为生产回退，避免“本地正常、单二进制失效”。
- 双 DOM 的移动结构默认隐藏，抽离阶段可以按页面回滚；未来移动样式也能独立回滚而不撤销桌面模板和后端数据。
