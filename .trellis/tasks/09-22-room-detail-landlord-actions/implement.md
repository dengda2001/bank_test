# 实施计划：房间详情页房东视角改造

## 前置条件

- [ ] 工作区当前有 11 个未提交改动（`cmd/truelayer-demo/` 下 6 个 Go/模板/CSS 文件 + `entity-drawers.css`、两个 drawer partial）。开工前先确认这些改动是本次要一起带走的，还是该先提交——本任务会改到其中 `room-detail.html` 的近邻文件，混在一起会很难分辨。
- [ ] 记录开工前的测试基线（见"验证"第一条），否则事后无法区分既有失败与本次回归。

## 顺序

1. **取基线**：跑一遍相关测试，把当前的红/绿记下来，尤其是 `detail_pages_alignment_test.go`、`entity_drawers_test.go`、`desktop_layout_test.go`、`prototype_preview_render_test.go`（这四组都直接执行房间详情模板，是本次的主要安全网）。
2. **逾期天数**：`rent_workspace_page.go` 的 `rentRoomDetailPageData` 加 `OverdueDays int`，在 `loadRoomDetail` 里从 `Summary.DueDate` 推出；先补单元测试（逾期／未逾期／日期不可解析三种），再动模板。
3. **平账适配器**：读 `manualBalanceRedirectURL`，确认 `return_to` 允许 `/rooms/...` 且不放行外站；加 `RoomDetailSettleForm` 适配器；模板里给责任表行尾接 `collection-settle-form`。参考 `workspace_alignment_test.go:376` 的 `TestTenantSettleFormCarriesTheWorkspaceContext` 补同款契约测试。
4. **KPI 与表格配色**：在 `workspace.css` 补 `metric-primary/success/warning` 定义；未付卡加警示色与左侧色条；责任表未付列同色系；"已收"为 0 时不显示正向色。
5. ~~**删除搬家**~~ ✅ 已完成（09-23）：页头与移动底栏的删除表单合并进编辑抽屉底部危险区；加后果文案；`workspace-nav.html` 的 confirm 支持 `data-confirm-message` 并保留回退；清掉失效的 `.delete-object-form{display:contents}` 与 `.delete-object-form{margin:0}`。
   - **范围扩大（dd 09-23 追问「删除房间/房产有放进编辑页吗」）**：同样形状的 `property-detail.html` 一并改了（页头 :25、移动 :98 → 编辑抽屉危险区）。新建抽屉在 `room-create-drawer.html` / `properties.html`，天然看不到删除。
   - `TestObjectDetailsExposeConfirmedDeleteActions` 的两个 fixture 补 `Editing: true`，并新增「抽屉关着时页面里不该有 delete」的反向断言——这条才是"平时看不到删除"的守卫。
6. **信息归位与砍块**：删 `room-plan-summary` 整段及其 CSS；「房间信息」补"每月交租日"与"计划区间"。
7. **空态折叠**：判据换成 `.PaymentCount`；`<details>` 折叠 + 摘要行按钮阻止冒泡。
8. **本页术语**：已覆盖→已收、责任人→租客。
9. **更新受影响的既有断言**，逐条说明为什么改；跑全量测试，与第 1 步基线对比。

## 重点文件

- `cmd/truelayer-demo/rent_workspace_page.go` —— 视图数据 + `rentRoomDetailPageData`
- `cmd/truelayer-demo/collection_settle_form.go` —— 新增第三个适配器
- `cmd/truelayer-demo/web/templates/pages/room-detail.html` —— 主要改动面
- `cmd/truelayer-demo/web/static/css/pages/room-detail.css` —— 布局与折叠
- `cmd/truelayer-demo/web/static/css/workspace.css` —— `.metric` 与 `metric-*` 修饰类
- `cmd/truelayer-demo/web/templates/partials/workspace-nav.html` —— confirm 文案
- `cmd/truelayer-demo/web/templates/partials/collection-settle-form.html` —— 复用，预计不改
- `cmd/truelayer-demo/web/templates/partials/room-create-drawer.html` —— 确认不动（新建无危险区）

## 验证

```bash
# 1) 开工前后各跑一次，用于区分既有失败与本次回归
go test ./cmd/truelayer-demo -run 'Test.*(RoomDetail|EntityDrawer|DesktopLayout|PrototypePreview|SettleForm|DetailPages).*' -count=1

# 2) 全量
go test ./...
git diff --check
```

浏览器验收（桌面 1440px + 窄屏 360/390/430px）：
- 逾期房间：确认"已逾期 N 天"、未付高亮、行尾一键平账可提交且回到本页、编辑抽屉有危险区且新建抽屉没有。
- 已缴清房间：确认没有逾期文案、没有平账入口、"已收"显示正向色。
- 无收款房间：确认两个空板块收起为一行，摘要里的入口按钮点了不展开面板。

## 风险控制

- **`return_to` 只允许本站路径**，默认回到当前房间详情页；不要为了省事放开校验。
- **删除表单搬家不等于放松校验**：`action=delete` 的鉴权、所有权与幂等性保持在 `handleRoomDetail`，本次只改 DOM 位置。
- **不要顺手改 `formatMoney`**（千分位）——它是全站共用的。
- **不要顺手把 `metric-*` 的配色推广到其他页面之外**：这一步的影响面已在 design.md 里界定，超出就要回 PRD 谈。
- **既有失败不背锅**：移动端 `detail-summary .metric:nth-child(4){display:none}` 这类规则若因改动失效，要明确是修还是留。

## 回滚点

按上面 1→9 顺序推进，每一段（逾期天数 / 平账 / 配色 / 删除搬家 / 砍块 / 折叠）是独立可回滚的原子改动。若第 3 步的 `return_to` 放行改不动，就先交付其余各段，把平账拆成后续任务——`collection-settle-form` 的接入不影响其他改动。
