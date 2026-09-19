# 技术设计：PC 端原型 UI 还原度对齐

## 1. 架构与边界

### 1.1 本次改动落在哪一层

应用是服务端渲染：Go handler 组装 view model → `html/template` 输出嵌在页面里的 HTML 与 `<style>` 块 → 少量内联 script 处理抽屉、筛选、树形展开。**没有前端框架、没有构建步骤。**

因此"对齐原型"不涉及引入组件库或重构渲染方式，改动只发生在三类文件：

| 层 | 文件 | 改什么 |
|---|---|---|
| 模板 | `web/templates/pages/*.html`、`web/templates/partials/workspace-nav.html` | 区块结构、控件、属性 |
| 样式 | `web/static/css/workspace.css`、`web/static/css/pages/*.css` | 令牌、栅格、间距、组件外观 |
| 视图模型 | `*.go` 里内联字符串模板的页面（`page_data_routes.go`、`rent_collection_pages.go`、`dashboard.go`） | 结构与字段 |

**不改**：数据库 schema、账务计算、路由契约、表单字段名与 action URL。

### 1.2 父任务与子任务的边界

父任务不直接改产品代码。它持有：原型契约的完整清单（`prd.md` 中已固化）、跨子任务的验收口径、最终集成复核。

子任务都依赖 ① 先落地 —— 因为 ① 会改动桌面渲染的基线，而 ②③④⑤ 都在改桌面渲染。若并行开工，① 的改动会与 ②③④⑤ 的改动在同一批 CSS 上冲突。

②④⑤ 之间无相互依赖，可并行。**③ 是例外**：已确认串行排在 ④ 之后 —— ③ 的租客视角「一键平账」必须复用 ④ 定稿的表单，两边各自实现会让同一个 action 出现两个渲染点，日后加权限校验或二次确认时漏一处即是可绕过的入口。其 toast 不构成依赖：toast 容器挂在共享壳内，② 落地后 ③④⑤ 自动获得，无需等待。

## 2. 关键设计决策

### 2.1 断点策略：保留 640，新增 1100

`responsive-conventions.md:54` 要求所有窄屏规则写在 `@media (max-width: 640px)`，且有一批测试按 640px 切分样式表来断言桌面渲染不被污染。原型用的是 760 / 1100。

决策：**640 不动**，新增 `@media (max-width: 1100px)` 承载 1024–1100 这一档的紧凑排版。

理由：本次验收范围只覆盖 PC（1024 及以上），640 这个值服务于手机端，改动它没有任何 PC 收益，却会连带推翻整套窄屏测试与 spec。而 1100 是真正影响 PC 观感的那一档 —— 1024 宽屏下原型的侧边栏与表格列密度与 1366+ 不同。

代价：在 640–760 区间，原型已是窄屏排版而实现仍是桌面排版。这个差异被显式接受，并记入 spec。

样式表内的顺序必须是：基准 → `max-width: 1100px` → `max-width: 640px`。范围嵌套在前、窄屏在后，窄屏规则才能覆盖中间档。

### 2.2 令牌与组件类：复用优先

`responsive-conventions.md:452` 已规定桌面基线是 236px 深色侧边栏 + 64px 顶栏，并要求复用 `.panel` / `.metric` / `.table-wrap` / `.status` / `.entity-form` / `.form-grid` / `.drawer-actions` 与 OKLCH 令牌（`--sidebar`、`--accent`、`--surface`、`--foreground`）。

对齐时**先确认现有类能否承载原型外观，能则只改样式不改类名**。只有原型确实引入了现有类无法表达的结构时才新增类，且命名沿用现有前缀风格（`workspace-`、`object-`、`room-`、`detail-`）。

不引入新的设计令牌体系。原型里的颜色值取到后映射到既有 OKLCH 变量；确有原型独有且语义明确的颜色（如「待人工处理流水」面板的粉色底、指标卡首卡的淡绿底）时，以新变量加入共享样式表，不硬编码。

### 2.3 两套渲染路径必须先收敛

核查发现 `/rent-dashboard` 与 `/bills` 各有两套模板：

- `/rent-dashboard`：有 DB 会话 → `handleRentDashboard` → `rent-workspace.html`；无 DB → `dashboard.go:210` 的降级模板。
- `/bills`：按 URL 路径分派（`dashboard.go:168-174`），`/bills` 恒定命中 `rent_collection_pages.go:42`。

两套模板内容已经分叉：降级模板有「一键平账」按钮与移动卡片，`rent-workspace.html` 没有。这正是首页租客视角缺平账入口的根因。

设计决策：**以 DB 会话路径（`rent-workspace.html`）为对齐基准**，降级模板保持可用但不再作为视觉对齐目标；对齐过程中发现的字段与组件差异，在基准模板上补齐。降级模板是否最终删除，留到父任务集成复核时判断，不在本次范围。

### 2.4 toast 组件

原型用 `showToast(msg)`（2.2 秒消失）替代页面跳转式反馈；当前实现用服务端 notice 块（`expenses.html:21-22`、`cash-receipts.html:21-24`、`dashboard.go:420-424`）。

设计：新增一个**服务端渲染的 toast 容器**，notice 数据仍由服务端在重定向后传入（沿用现有机制，不改成 AJAX），由一个共享的少量内联 script 负责显示与自动消失。

- 容器挂在共享 chrome（`workspace-nav.html`）里，所有页面自动具备。
- 服务端 notice 存在时渲染为 toast 并带 `data-autoshow`，script 读到后触发显示与 2.2 秒消失。
- 保留现有 notice 块作为无脚本降级（`<noscript>` 语义），但默认样式下不重复可见 —— 避免同一条消息出现两次。
- 与既有约定一致：`responsive-conventions.md:170` 指出 `html/template` 会剥离 `<style>` 里的 CSS 注释，因此测试断言必须锚定声明而非注释。

### 2.5 树形表格的展开方式

原型用 JS 维护 `expandedDashboardRows` 集合并整表重绘。当前 `rent-workspace.html` 已用原生 `<details>/<summary>` 实现逐级展开（`workspace-room-mobile`、`workspace-tree-mobile` 等类）。

设计：**继续用 `<details>/<summary>`，不再引入 JS 状态管理**。理由：原生元素自带键盘可达性与无障碍语义，且当前实现已通过浏览器验证。原型要求的 `aria-expanded`、点行触发、Enter/空格触发由 `<summary>` 原生满足。

需要做的是把现有的移动端 details 结构在 PC 表格上表达为原型的三级树形缩进（`tree-level-1` / `tree-level-2`），这是样式工作而非结构重写。

### 2.6 「一键平账」表单补齐

原型表单：待平账责任（只读）、处理方式下拉（匹配现有收款 / 登记现金收款 / 登记减免 / 结转下月）、平账金额、生效日期、处理说明。

当前 `rent_collection_pages.go:49` 的表单只有 `obligation_id` / `period` / `search` / `status` / `sort` / `page` / `page_size` / `reason`，POST 到 `/bills/settle` → `handleDashboardManualBalance`。

设计：**新增字段，不改变已有字段名与 action**。处理方式是本次唯一引入新账务语义的字段，需要 `handleDashboardManualBalance` 对四个选项分别处理；其中「结转下月」与「登记减免」涉及账务口径，超出"UI 对齐"的性质。

处理方式：本次**只落地原型表单的字段与交互**，处理方式下拉先只启用「匹配现有收款」（与当前行为等价），其余三项渲染为可选但提交时返回明确的未实现提示，并在 PRD 的 Out of Scope 中记录。这样表单结构对齐原型，但不静默引入未经确认的账务规则。

## 3. 兼容性与回滚

- **兼容**：不改变任何路由、表单字段名、action URL 或 view model 字段，因此服务端契约不变。列表页的筛选/排序/分页/detail-return 上下文（`responsive-conventions.md:209-214`）必须原样保留 —— 这是已有测试与浏览器验证覆盖的行为，对齐过程中不得破坏。
- **回滚**：每个子任务独立提交，可单独 revert。父任务不做大范围一次性改动。
- **风险点**：
  - `workspace.css` 与各页面 CSS 被多页共享，改一处可能影响未预期页面 —— 每个子任务完成后须跑全量页面截图，不只检查目标页。
  - 桌面渲染解冻后，`mobile_layout_test.go` 中断言桌面不被污染的测试会失败，必须与代码改动同批更新，不能留到后面。

## 4. 验收方式

- **几何证据优先于像素观感**：`responsive-conventions.md:379` 明确记录过"三个缺陷只有在真实浏览器渲染时才暴露"。因此每个子任务都要在 1024 / 1366 / 1440 / 1920 四档实际渲染，断言 `scrollWidth === clientWidth`，并保存前后截图到子任务目录。
- **截图对照**：原型侧已有 `figma/rentops-desktop-suite.html` 可在浏览器打开；对照按页面逐一进行，产物存 `research/screenshots/`。
- **测试**：Go 测试与 `go vet` 必须通过；涉及断点与桌面契约的断言在同一提交内更新。
- **不作数的证据**：模板字符串里存在某标记，不等于页面渲染正确。核查报告已经显示这一陷阱（降级模板有平账按钮，实际路径没有）。
