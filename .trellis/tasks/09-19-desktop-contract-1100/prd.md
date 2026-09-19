# 桌面契约解冻与 1100 断点

父任务：`.trellis/tasks/09-19-pc-ui-fidelity-alignment`。**本子任务必须先于 ②③④⑤ 完成。**

## Goal

解除 `responsive-conventions.md` 中"桌面渲染是冻结契约"的约束，为后续四个子任务的桌面改动提供合法基线；同时新增 `1100px` 中间档，使 1024–1100 与 1100+ 的排版差异可按原型表达。

## Confirmed Facts

- `.trellis/spec/frontend/responsive-conventions.md:17` 声明：*The desktop rendering is a frozen contract. It was captured before this work as a `:8082` reference build, and moved only once, deliberately (see §5).*
- `:54` 规定所有窄屏规则必须使用 `@media (max-width: 640px)`；`:63` 的判据是"能移动桌面渲染的窄屏规则就是缺陷"，并由 `TestFrozenLastColumnIsMobileOnly` / `TestWorkspaceCSSDrawerIsCSSOnlyAndMobileScoped` 断言。
- `:83` 说明 `html/template` 把共享样式表拼在页面样式之前，**共享表在同等优先级下必输**给页面局部规则。新增的 1100 档若放在页面局部规则之后，才能覆盖它们。
- `:170` 说明 `html/template` 会剥离 `<style>` 里的 CSS 注释，测试断言必须锚定声明而非注释。
- 原型 `figma/rentops-desktop-suite.html` 只有两档：`@media(max-width:760px)` 与 `@media(max-width:1100px)`。
- 当前实现 `web/static/css/workspace.css` 有 5 档：`max-width:640px`（2 处）、`min-width:981px`、`min-width:641px`、`600px–640px`、`max-width:980px`。
- 用户已确认：保留 640 不动，新增 1100 中间档；本次适配验收只覆盖 1024 / 1366 / 1440 / 1920。

## Requirements

- 更新 `responsive-conventions.md`：把"桌面渲染是冻结契约"改为新的契约表述 —— 桌面**可按原型对齐**，但任何桌面改动必须同时更新受影响的断言，并说明改的是哪一处原型差异。不得简单删掉该段而留下无约束状态。
- 在共享样式表中新增 `@media (max-width: 1100px)` 档，承载 1024–1100 的紧凑排版。
- 样式表顺序固定为：基准 → `max-width:1100px` → `max-width:640px`。范围嵌套在前、窄屏在后。
- 梳理并记录现有 `max-width:980px` 与 `min-width:981px` 这一对的处置：是保留、并入 1100 档，还是删除。给出理由。
- 同步更新因桌面解冻而失效的测试断言。至少覆盖 `TestFrozenLastColumnIsMobileOnly`、`TestWorkspaceCSSDrawerIsCSSOnlyAndMobileScoped`、`TestNavCountsOnlyRenderWhereTheyDidBefore`、`TestEveryWorkspacePageRendersTheSharedChromeOnce`。
- `640px` 的行为与断言不得改变。

## Acceptance Criteria

- [x] `responsive-conventions.md` 中不再存在"桌面渲染是冻结契约"的无条件表述，取而代之的是可执行的桌面改动规则。
- [x] 共享样式表含 `@media (max-width: 1100px)` 档，且该档位于 640 档之前。
- [x] 640 档的规则与断言与改动前完全一致（可用 diff 证明）。
- [x] 980/981 这一对的处置有明确结论并写入 spec。
- [x] `go test ./cmd/truelayer-demo/...` 全部通过。
- [x] `go vet ./...` 通过。
- [x] 1024 / 1366 / 1440 / 1920 四档下，全部 11 个页面 `scrollWidth === clientWidth`。
- [x] 四档截图存入本任务 `research/screenshots/`，作为后续子任务的对照基线。

## Out of Scope

- 具体的页面视觉对齐（由 ②③④⑤ 承担）。
- 手机端（≤640px）行为改动。
- 把断点从 640 改成 760。

## Notes

- 本子任务的产物是**基线与规则**，不是视觉成果。它的完成标准是"后续子任务可以合法地改桌面渲染"，而不是"页面变好看了"。
- 若在梳理 980/981 时发现该档承担了未记录的行为，先记录再决定，不要直接删除。

---

## 验收证据（2026-09-19 收尾时逐条对过）

| 验收项 | 证据 |
|---|---|
| 冻结表述已改写 | `responsive-conventions.md` 全文搜 "frozen" 只剩 `:28` 的历史叙述与移动端的 "frozen last column"/"frozen mobile contract"（后者是 ≤640 的绝对契约，应保留） |
| 1100 档存在且在 640 之前 | `workspace.css:363`（1100）早于 `:367`（640）；`TestWorkspaceCSSOrdersThe1100TierBeforeThe640Tier` 守这条 |
| 640 档逐字节不变 | 从第一个 640 块到 EOF 的 sha256：`workspace.css` = `7e572e72…f565`，`collection-pages.css` = `75375c16…8a0b` |
| 980/981 处置已入 spec | §3.1 单列一段说明它只是导航阈值、`.grid-two` 已迁出、≤980 渲染等价 |
| `go test` / `go vet` | 退出码均为 0（收尾复跑一次确认） |
| 四档 11 页无文档级横向溢出 | `scripts/audit/desktop-widths.mjs` exit 0，44/44 `overflow=0`；重跑报告与归档报告逐字节相同 |
| 四档截图 | `research/screenshots/` 44 张 = 11 页 × {1024,1366,1440,1920}，随本任务提交 |

**已知保留问题（不阻塞，但后续任务要知道）**：共享 1100 档目前只装了一条声明（`.grid-two`），
而它唯一的渲染者是无人引用的 `legacyExpenseTemplate`。**"档位存在"不等于"档位已填满"** ——
其余原型 1100 折叠是 ②③④⑤ 各自的活。spec §3.1 已写明。
