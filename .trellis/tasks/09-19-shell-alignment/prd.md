# 全局外壳对齐

父任务：`.trellis/tasks/09-19-pc-ui-fidelity-alignment`。依赖子任务 ① 先落地。

## Goal

让侧边栏与顶栏按原型 `figma/rentops-desktop-suite.html` 呈现，并补上 toast 操作反馈组件。外壳被全部 11 个页面共享，因此本子任务是其余子任务的观感底座。

## Confirmed Facts

- `web/templates/partials/workspace-nav.html` 是唯一的外壳模板，全部工作区页面经它渲染。
- 当前侧边栏：单层 `nav-label`「工作台」（`:9`）；11 个导航项图标是汉字单字 总/账/流/催/产/房/租/约/现/支/银（`:11-21`）；无分组标题、无底部状态卡、无页脚。
- 当前顶栏：只有面包屑 `RentOps / {{.FootNote}}`（`:25`），无搜索框、无计数按钮、无主操作按钮。
- 原型侧边栏：三个分组标题「收租决策 / 资产与关系 / 资金与系统」；`01`–`11` 两位数字图标；4 项带计数（收租总览 6、应收账单 12、流水匹配 4、催收任务 5）；底部状态卡「测试数据已载入 / Rosewood 收租明细 / 更新于 2026-09-16」；页脚「房东工作区 所有者视角」。
- 原型顶栏：全局搜索框（占位「搜索房产、房间、租客或记录」）+ 计数按钮「3」+ 主 CTA「重新载入测试数据」。
- 计数约束：`TestNavCountsOnlyRenderWhereTheyDidBefore` 断言导航计数只在既有位置渲染 —— 补 `01`–`11` 图标与分组标题时不得改变现有 4 个计数的渲染条件。
- `.trellis/spec/frontend/responsive-conventions.md` §8 已定义桌面基线为 236px 深色侧边栏 + 64px 顶栏，并要求复用 OKLCH 令牌。
- 全仓库无 `showToast` 实现；当前操作反馈用服务端 notice 块（`expenses.html:21-22`、`cash-receipts.html:21-24`、`dashboard.go:420-424`）。
- 父任务已决定：顶栏搜索框必须做真实工作（提交到当前页已有的 `search` 参数）；无列表检索能力的页面不渲染该搜索框。
- 父任务已排除「重新载入测试数据」——`figma/DESIGN-HANDOFF.md:9` 要求生产 UI 不得保留原型专属装饰。

## Requirements

- 侧边栏补齐三个分组标题、`01`–`11` 两位数字图标、底部状态卡与页脚；保留现有 4 个计数的渲染条件不变。
- 底部状态卡的内容需来自真实数据（如当前数据源与最近更新时间）。若某项无真实来源，不渲染该行，**不写死原型文案**——`DESIGN-HANDOFF.md:37` 禁止用占位内容替换真实文案，反向亦然：不得把演示文案当产品文案。
- 顶栏补齐搜索框与计数按钮，尺寸与样式对齐原型。
- 搜索框按 A1 约束实现为当前页列表检索；无 `search` 参数的页面不渲染。
- 计数按钮链接到收租总览的「待人工处理流水」面板，显示真实待处理条数。
- 新增服务端渲染的 toast 容器，挂在共享外壳内，使全部页面自动具备；由共享内联 script 负责显示与 2.2 秒自动消失。
- toast 与现有 notice 块不得同时可见 —— 同一条消息只能出现一次。保留 notice 作为无脚本降级。
- 侧边栏在 1024–1100 档按原型收紧（配合子任务 ① 的 1100 断点）。
- **侧边栏在桌面档滚动时不得随页面移走**（用户于 2026-09-19 报告；见下方「用户报告的滚动缺陷」）。

## Acceptance Criteria

- [ ] 侧边栏渲染出三个分组标题、11 个 `01`–`11` 数字图标、4 个计数、底部状态卡与页脚。
- [ ] `TestNavCountsOnlyRenderWhereTheyDidBefore` 通过，计数渲染条件与改动前一致。
- [ ] 顶栏含搜索框与计数按钮；搜索框在具备 `search` 参数的页面上提交后确实过滤列表，在不具备该参数的页面上不渲染。
- [ ] 计数按钮显示的条数与收租总览「待人工处理流水」的实际条数一致。
- [ ] toast 容器在共享外壳中渲染；触发一次保存操作后出现提示并在约 2.2 秒后消失。
- [ ] 同一操作不会同时出现 toast 与 notice 两条消息。
- [ ] 无脚本环境下 notice 块仍可见。
- [ ] 11 个页面在外壳改动后全部正常渲染（`TestEveryWorkspacePageRendersTheSharedChromeOnce` 通过）。
- [ ] 1024 / 1366 / 1440 / 1920 四档下侧边栏与顶栏无横向溢出，全页面 `scrollWidth === clientWidth`。
- [ ] 四档截图存入 `research/screenshots/`。
- [ ] **桌面档（≥641px）页面滚动时侧边栏保持钉在视口顶部**，滚动前后
      `getBoundingClientRect().top` 不变（见下方「用户报告的滚动缺陷」）。
      在**真实页面**上量，不只在合成页面上量。

## Out of Scope

- 顶栏跨实体全局检索（父任务已拆为后续独立任务）。
- 「重新载入测试数据」按钮（父任务已排除）。
- 各页面内部的区块与表格对齐（由 ③④⑤ 承担）。

## 已确认的决策

**顶栏不放主操作按钮**（用户已确认）。原型顶栏的主 CTA 是「重新载入测试数据」，已被父任务排除为产品功能。顶栏因此只保留搜索框与计数按钮；页面级主操作按 A4 留在页头。

理由：把页头主操作复制进顶栏会产生两个渲染点、两个可点击控件执行同一动作，日后加权限校验或二次确认时两处都得改，漏一处即是可绕过的入口。

**代价（显式接受）**：顶栏比原型窄一块，与原型不逐像素一致。这是父任务 Out of Scope 中「重新载入测试数据不做成产品功能」的直接后果，记入本子任务的已知偏差。

## 用户报告的滚动缺陷（2026-09-19）

用户报告：**滚动页面时侧边栏跟着一起滚走**。这不是本子任务引入的，是既有缺陷，但属于外壳，
由本子任务一并修复。

**根因**（已定位到行）：`workspace.css` 的 `@media (min-width: 641px)` 档里，桌面侧边栏写的是

```css
.sidebar { position: relative; top: 0; height: 100vh; overflow: auto; ... }
```

`position: relative` 让侧边栏留在正常文档流里，因此随文档滚动。`top: 0` 对 `relative` 是
「相对自身原位偏移 0」，等于没写；`height: 100vh` 只给了它自己的高度，不产生吸顶效果。

**原型写的是 `sticky`**（`figma/rentops-desktop-suite.html` 内联 CSS）：
`.sidebar{position:sticky;top:0;height:100vh;...}`。除这一个词外两边规则逐字相同 ——
移植时把 `sticky` 写成了 `relative`。

**已验证的修法**（真 Chrome 最小复现，1366×768，内容高 3000px）：

| 变体 | 滚动前 top | 滚 600px 后 top | 结果 |
|---|---|---|---|
| 现状 `relative` | 0 | −600 | ❌ 复现 |
| 只改 `sticky` | 0 | 0 | ✅ 钉住 |
| `sticky` + `overflow-x: clip` | 0 | 0 | ✅ 钉住 |

⇒ **只需把该档的 `relative` 改成 `sticky`**，不必动 `body` 的 `overflow-x: hidden`。

**一个曾被怀疑但不成立的原因**：`body { overflow-x: hidden }`（`workspace.css:36`）通常被当作
`position: sticky` 的杀手。此例不成立 —— `body` 的 overflow 会传播给视口，`body` 自己不成
滚动容器，sticky 照常生效。上表第三行是实测反证。**不要**为了"修 sticky"而去改 `overflow-x`。

**验证要求**：必须在**真实页面**上量（`scripts/run-audit-local.sh` + `scripts/audit/launch.mjs`），
不能用合成页面替代 —— 真实页面的祖先链更复杂，可能有别的 `overflow` 祖先。对至少两个页面、
在 ≥641px 的档位上，滚动前后各取一次 `getBoundingClientRect().top` 比对。

**640 档无需改动**：`@media (max-width: 640px)` 里侧边栏是 `position: fixed` 的抽屉，行为正确。
980 档不设 `position`。
