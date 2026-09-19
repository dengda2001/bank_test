# 执行计划：全局外壳对齐

依赖子任务 ① 先落地。

## 执行顺序

1. [x] 通读 `workspace-nav.html` 全文件，标出导航计数条件与既有页面依赖
2. [x] 改造侧边栏：分组标题 → `01`–`11` 数字图标 → 状态卡 → 页脚
3. [x] 每步之后跑 `TestNavCountsOnlyRenderWhereTheyDidBefore`，确认计数条件未被顺手改动
4. [x] 顶栏补搜索框（按 design.md §2.2 的作用域规则）与计数按钮
5. [x] 计数按钮接真实待处理条数
6. [x] 在共享外壳内加 toast 容器 + 共享内联 script
7. [x] 处理 notice 块与 toast 的重复可见问题（design.md §2.1）
8. [x] 1024–1100 档侧边栏收紧 —— **与原计划不同**：原型 `@media(max-width:1100px)`
      里没有任何侧边栏声明（该档唯一的 shell 相关声明是 `.input{min-width:160px}`），
      凭空造一条侧边栏规则会违反 spec §1「必须指名原型声明」。改为把原型那条
      `.input{min-width:160px}` 落到顶栏搜索框上。详见交付说明。
8.1 [x] **修侧边栏滚动缺陷**：`@media (min-width: 641px)` 里 `.sidebar` 的
      `position: relative` → `sticky`（`prd.md`「用户报告的滚动缺陷」有完整根因与实测表）。
      只改这一个词；**不要**顺手去动 `body` 的 `overflow-x`。改完在**真实页面**上滚动前后各量一次
      `getBoundingClientRect().top`。
9. [x] **11 个页面**逐页四档截图 —— 不是只查首页
10. [x] 跑全量测试与 vet
11. [ ] 提交 —— **未做**：本次派工明令禁止 `git commit` / `push` / `merge`，留人工提交

第 9 步不可省：外壳是公共渲染点，只检查目标页会漏掉被波及的页面。
第 8.1 步是用户报告的缺陷，且 8.1 与 8 改的是同一段 CSS，放在一起做，别拆成两次改。

## 与计划的偏差

- 第 8 步：原型的 1024–1100 档没有侧边栏规则可移植，改为移植 `.input{min-width:160px}`。
- 第 7 步：`data-toast` 只标成功提示（`notice ok`），**不标**错误横幅 —— 原型 toast 本就是
  保存确认（`showToast("操作已完成")`），错误提示需要用户读完再操作，2.2 秒不够。
- 第 9 步的截图存于本任务目录 `research/screenshots/`（44 张 + `report.json`）。
- 第 8.1 步新写了一个可复跑的浏览器校验脚本 `scripts/audit/verify-shell.mjs`。

**上面两条偏差与「偏差」清单本身，用户已于 2026-09-20 确认接受。**

## 范围外发现（已立项，不在本任务修）

实施者发现两个**既有**缺陷并如实上报、未顺手修改。主会话已用 `git grep <符号> HEAD`
复核确认二者在改动前即存在，并单独立项 `.trellis/tasks/09-20-mobile-filter-and-duplicate-errors`：

1. ≤640px 时 `/properties`、`/rooms`、`/tenancies` 的筛选开关点击无反应
   （绑定语句在 `workspace-nav.html` 顶层执行，早于 `<main>` 解析）。
2. `/cash-receipts?error=cash_overbalance` 同一个错误渲染两条不同措辞的消息。

不顺手改是正确的：本任务正在改同一个 `workspace-nav.html`，混入这两条会让
「外壳对齐」的验收面无法与它们区分。

## 主会话独立复验（2026-09-20）

`scripts/audit/verify-shell.mjs` 由主会话在一次性实例（18090）上复跑，**32 项全过**，
与实施者的自述一致。侧边栏吸顶在 `/rent-dashboard` 与 `/bills` 两页 × 1024/1366/1440
三档共 6 组实测全部 `top: 0`。

复跑时发现脚本自身一处健壮性缺口：若登录后落到无 `.sidebar` 的页面，
`page.evaluate` 会抛 `TypeError` 而非给出可读失败（首次运行即如此挂掉，重跑正常）。
不阻塞验收，留待后续顺手补空值守卫。

## 验证命令

```bash
go test ./cmd/truelayer-demo/...
go vet ./...
git diff --check
```

## 风险文件

| 文件 | 风险 | 处置 |
|---|---|---|
| `web/templates/partials/workspace-nav.html` | 全部页面经它渲染 | 改后 11 页全截图 |
| `web/static/css/workspace.css` | 共享表，同等优先级必输于页面局部规则 | 外壳规则需检查实际是否生效 |
| toast 内联 script | 新增全局脚本，可能影响既有抽屉/筛选脚本 | 改后手动触发抽屉与筛选各一次 |

## 开工前检查

- [x] 子任务 ① 已完成（`c944538`，已归档），1100 档已确认生效
- [x] 用户已审阅本子任务的 `prd.md` / `design.md` / `implement.md`
- [x] 已知悉顶栏不放主操作按钮（已确认决策，见 `prd.md`）—— 不要顺手把页头主操作复制进顶栏
- [x] 已确认用 `scripts/run-audit-local.sh` 起一次性实例，不指向 `:8081` / `bank.ddpl.top`
