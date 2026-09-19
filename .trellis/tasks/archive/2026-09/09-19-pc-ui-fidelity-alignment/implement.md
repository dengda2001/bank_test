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

1. [x] 确认 `figma/rentops-desktop-suite.html` 可在浏览器打开，作为对照基准
2. [x] 确认子任务 ① 已完成（`responsive-conventions.md` 已更新、1100 档已落地、受影响测试已同步）
3. [x] 逐子任务复核：完成后按子任务的验收项实机检查，不只读它的完成报告
4. [x] 集成复核：全部子任务完成后，在 1024 / 1366 / 1440 / 1920 四档逐页截图，与原型对照
5. [x] 判断降级模板（`dashboard.go:210`）的去留，写入结论
6. [x] 汇总仍存在的差异，逐条给出保留理由
7. [x] 更新 `.trellis/spec/frontend/` 相关约定
8. [ ] 提交并归档（提交 `518fe08`；归档待 `task.py archive`）

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

可复用的 Playwright 脚本在 **`scripts/audit/`**（已入库）。适合几何、命中区域、焦点类问题；`desktop-full.mjs` 是现成的桌面回归对，`probe-wide.mjs` 能把页面级溢出与滚动容器内的溢出分开。

### 怎么把被测实例跑起来（必读）

**用 `scripts/run-audit-local.sh`。** 它建一个一次性 `rentops_audit_*` MySQL 库、导入 `test-data/audit/` 里的已提交夹具、起应用、跑完 drop 掉临时库。三道硬防护写在脚本里：

- `:62` 数据库名不匹配 `rentops_audit_*` 就拒绝运行；
- `:66` 端口是 `8081` 或 URL 含 `bank.ddpl.top` 就拒绝运行；
- `:73` `MYSQL_HOST` 不是 loopback 就拒绝运行。

**不要指向 `:8081` / `bank.ddpl.top`** —— 那是生产应用连生产库，`scripts/audit/README.md:106-108` 明确要求指向你自己起的可丢弃实例。

注意 `scripts/audit/README.md:110-118` 那段"该脚本尚未实现"的说明已经过时：脚本已随本次提交落地，以脚本本身为准。

本次涉及写操作的验证（平账表单、生成账单）就在这个一次性实例上做 —— 它有可丢弃的数据，不必再像早期那样限制为只读。

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

## 父任务清单第 5 项结论：降级模板的去留（2026-09-20 主会话）

**`/rent-dashboard` 降级模板：已删除，结论是「删」且已落地。**
`dashboard.go:172-175` 的注释记录了这件事：该函数用一份数据契约渲染三个活的工作区，
模板按请求路径选择；legacy `/rent-dashboard` 回退模板已经不存在，所以未知路径是
**编程错误而不是降级请求** —— 现在的实现是枚举活的分支，落到 `default` 就
`http.Error(..., 500)` 硬失败。由已归档的 `09-19-legacy-dashboard-template-removal` 完成。

**另外三个 legacy 模板仍是死代码**（主会话 `grep` 全仓核实，除定义行外引用数均为 0）：

| 模板 | 位置 | 状态 |
|---|---|---|
| `legacyExpenseTemplate` | `main.go:2754` | 有定义、零引用 |
| `legacyCashReceiptPageTemplate` | `page_data_routes.go:1511` | 有定义、零引用 |
| `legacyDashboardTemplate` | —— | 已不存在（即上文的降级模板） |

三者都不影响用户可见行为，但 `legacyCashReceiptPageTemplate` 里含一处**查询参数裸回显
成绿色成功提示**（见 `09-20-mobile-filter-and-duplicate-errors` 缺陷 3），
在它被删除之前先按白名单守卫住。删除死模板另立任务，不在本轮范围内。

## 父任务清单第 3、4、6 项：集成复核（2026-09-20 主会话）

五个子任务全部提交后，在一次性实例（`MYSQL_PORT=3306 APP_PORT=18097
scripts/run-audit-local.sh`，未指向 `:8081` / `bank.ddpl.top`）上跑
`scripts/audit/probe-integration-review.mjs`：11 个页面 × 原型视口矩阵四档。

### 第 4 项结论：四档逐页对照

```
pages x widths: 11 x 4
horizontal overflow: 0
errors: 0
```

**44 格全部 `documentElement.scrollWidth === clientWidth`，零页面报错。**
截图 44 张落在 `research/screenshots/`，按 1024×768 / 1366×768 / 1440×900 /
1920×1080 拍摄，与原型 `figma/DESIGN-HANDOFF.md` 的视口矩阵同高，可逐张对照。

同一支探针另外确认了两件子任务层面看不到的事：

- **641–980 档侧边栏不再盖住正文**（② 的修复在真实浏览器里独立确认）：
  800 / 900 / 980 三档 `position=static`，滚到文档底部时视口中心命中的是正文
  （`SELECT`、`LABEL.property`），`coversBody=false`。
- **09-20 缺陷 1 在实机复现**：390×844 下 `/properties`、`/rooms` 的开关点击后
  `aria-expanded` 仍为 `false`、`.object-list-filter-fields` 仍为 `display:none`。
  同一次探测还纠正了 09-20 prd 的一处事实错误：`/tenancies` **没有**这个开关
  （它用真正的 `type="submit"` 按钮），不在该缺陷范围内。

### 第 3 项结论：逐子任务的实机复核

四个子任务各由一名独立复验 agent 复核，主会话只做两件事：核验复验**自己**的改动，
以及用 `git show <commit>^:` 复核复验报上来的「既有缺陷」判定是否成立。过程中
**推翻了三条结论**（详见各子任务 `implement.md` 的「主会话独立复验记录」）：

| 被推翻的结论 | 推翻依据 |
|---|---|
| ③ 复验：队列按钮 1 个还是 2 个是产品决定，搁置 | `git show HEAD:…rent-workspace.css` 显示被删的 981 块编码了原型的桌面按钮设计，且规格记着「kept」 |
| ② 复验：641–980 sticky 只是「没设 position」 | 源码顺序表明 641 档是 `min-width`，在该档同样命中，必须显式撤销 |
| ④ 复验：`/properties`、`/rooms` 过滤到 0 行无空状态 | 实机逐页实测 7 页 × 2 档全部有空状态；`{{else}}` 分支本就不分桌面/移动 |

同时**吸收了复验发现的四处真实缺陷**：`/tenants` 与房产详情内嵌房间表的
「查看详情」漏网（`f538a6f`、`da1b2ea`）、641–980 档侧边栏盖住正文（`e47702e`）、
toast 断言的空白脆弱（`e47702e`）。

### 第 6 项结论：仍存在的差异与保留理由

| 仍存在的差异 | 保留理由 |
|---|---|
| 详情页未渲染「联系与催收记录」区块 | ⑤ 的 Out of Scope：该区块依赖催收发送记录，详情页数据契约里没有这个字段，属新增功能而非对齐 |
| 「演示数据 · EUR」的处置**三处不一致**（收租总览只留币种／列表页照渲染／详情页两样都没有） | 收租总览那一处是**用户 2026-09-20 明确答复**「只保留币种」，依据 `figma/DESIGN-HANDOFF.md:9,37`（该文件把原型自用文案 `测试数据已载入` / `Rosewood 收租明细` 排除在生产 UI 之外）。**但该答复只覆盖收租总览**，列表页的标注来自 `df8997b`（本轮之前就有），详情页则从未有过该标注。**本条不是「已保留的差异」，是待决策项** —— 见 `prd.md` 该条勘误的分布表与下方「待用户晨间决策」第 5 条 |
| 1024 档有四个列表页在**容器内**横向滚动 | 主会话实测（1024×768）：`/bills` +210px、`/transactions` +230px、`/expenses` +200px、`/tenants` +30px 在 `.table-wrap` 内横滚；总览的三个视图、`/tenancies`、`/cash-receipts` 不滚。**文档级仍为零溢出**（44 格结果已含这一档），即「宁可横滚，不许压字」是按设计落在容器上的，没有溢出到页面 |
| `/dunning` 没有字面的「操作」列 | 它是候选清单（勾选 + 批量发送），不是行内操作表；强行加一列会与原型不符 |
| `/bank` 的「添加银行账户」与「重新授权」同指 `/bank/connect` | 原型同样只有一个连接入口；两个入口的语义差异属产品决策，未擅自拆分 |

### 待用户晨间决策（主会话未擅自决定）

用户 2026-09-20 授权主会话「自主判定」把队列跑完。以下五条**不属于队列**——
它们是产品取舍，主会话一律**没有改代码**，只把事实与代价记在这里。
第 1–4 条由子任务 ③ 的实施者在收尾时如实上报（详见
`.trellis/tasks/09-19-dashboard-alignment/implement.md:102-112`），第 5 条由主会话终审时发现。

| # | 事项 | 事实（可复核） | 主会话建议 |
|---|---|---|---|
| 1 | 队列条目的按钮文案 | 原型 `figma/rentops-desktop-suite.html:57` 每条队列是**单颗无边框文字链「处理」**；本仓库 ≥981 渲染「处理流水」 | **维持现状**。「处理流水」是侧边栏导航与多处页面标题在用的**页面名**，改回「处理」会与页面名脱节；且用户已明示字面文案不属对齐面 |
| 2 | ≥981 档队列没有「查看建议」入口 | 原型每颗按钮只有一个，本仓库 ≥981 用 `display:none` 隐掉「查看建议」（`rent-workspace.css:89-93`），641–980 与 ≤640 两颗都在 | **维持现状或统一为单颗**。这不是与原型的功能差（原型本就只有一颗），是**分档之间自己不一致**；统一成单颗最贴原型，代价是窄档少一个入口 |
| 3 | 统计 chips 不是穷尽划分 | 实时房间视角 6 行，`1+1+2=4`，剩 1 行 `open` 与 1 行 `vacant` 无桶可归；原型 `rowState` 是穷尽划分 | **维持现状**。属数据层口径，不属用户定义的对齐面（样式/排版/控件/适配），且数值无算术矛盾 |
| 4 | 「一键平账」提交后把用户踢出工作区 | 实测 `POST /bills/settle` → 跳回 `/bills?...`。两半共同造成：模板 `partials/collection-settle-form.html:4` 把 `action` **写死**为 `/bills/settle`，而处理器 `dashboard_manual_balance.go:133` 按 `strings.HasPrefix(r.URL.Path, "/bills")` 判断回跳目标 —— 于是从总览页提交也命中 `/bills` 分支。**落在子任务 ④ 的边界内** | **建议修**（改完停在 `/rent-dashboard` 并保留 period）。这是功能可用性问题，不是文案问题；但它要动 ④ 已提交并已通过复验的文件，故未擅自改。最小改法与既有 `ReturnPeriod`／`ReturnSearch` 同构：给 partial 加一个 `ReturnPath` 隐藏域，处理器优先按它回跳 |
| 5 | 「演示数据 · EUR」三处不一致 | 见下表 | **建议统一为「只保留币种」** |

第 5 条展开。用户在 Phase 1 就**收租总览这一处**明确答复「只保留币种（推荐）」
（`figma/DESIGN-HANDOFF.md:9` 禁止生产 UI 保留原型专属装饰与预览标签）。
但该答复的问法限定了「③ 收租总览」，未覆盖其余两个表面，实际分布因此是三处不一致：

| 表面 | 原型 | 本仓库 | 依据 |
|---|---|---|---|
| 收租总览 | `figma/rentops-desktop-suite.html:60` 有 | 只渲染币种 `EUR` | **用户已确认** |
| 列表页 | `:122` —— `buildPage()` 把该标注注入**每一个**列表页的 filterbar | 仅 `properties.html:49`、`rooms.html:55` 渲染「演示数据 · EUR」 | ④ 沿用 `df8997b` 既有写法，未动 |
| 详情页 | `:146` `detailHeader()`、`:210` 为其四页赋值 | 只渲染真实字段（城市、房间数、责任数、状态） | ⑤ 未涉及该标注 |

**建议统一为「只保留币种」的理由**：本文件第 149 行原本记的就是全局口径
（「演示数据」不渲染），用户答复时的理由（`DESIGN-HANDOFF.md:9`）对三个表面同样成立，
且列表页的「演示数据 · EUR」与总览页的「EUR」摆在同一款应用里自相矛盾。
**若采纳，改动量很小**：`properties.html:49`、`rooms.html:55` 两行去掉「演示数据 · 」四字
（`.object-list-demo-note` 类名与 CSS 可保留）。
**若维持现状**，则需把本文件第 149 行改成「仅总览页不渲染」，并接受三处不一致。
