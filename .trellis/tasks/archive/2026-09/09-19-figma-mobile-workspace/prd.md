# Figma 移动端工作台还原

父任务：`09-18-figma-prototype-alignment`

## Goal

以 `mobile/rentops-mobile-suite.html` 为基线，将同一套真实 RentOps 数据重排为手机可独立完成的房东工作台，而不是把桌面表格缩小：五项底部导航、指标卡、对象层级、详情页和强确认 sheet。

## Dependencies

- 前置：`09-19-figma-page-data-routes` 的数据和 action contract 已稳定；`09-19-figma-desktop-workspace` 已交付共享 token／模板语义。
- 启动前：复核尚在进行的 `09-16-mobile-friendly-workspace` 及其子任务，复用已合入的浏览器 fixture／移动卡片基础，明确合并或替代边界，禁止两套窄屏规则互相覆盖。
- 后置：父任务最终跨端集成验收。

## Requirements

- 采用顶栏和五项固定底部导航：首页、账单、流水、对象、更多；对象收纳房产／房间／租客，更多收纳租住安排、催收、现金、支出和银行。
- 首页使用两列四指标与可展开房产→房间→租客卡片；账单、流水、现金、支出和租客采用纵向卡片，金额、身份、状态和主操作必须同屏可见。
- 实体详情为可返回的移动详情页；桌面抽屉在手机由底部 sheet 或详情页替代。
- 编辑、人工平账、催收预览／确认、现金、支出、资产操作采用单列 sheet，主确认栏避开安全区且点击目标至少 44px。
- 手机同步按钮必须使用 refresh-token 流程；平账必须录原因；催收必须显示收件人、欠款、正文和明确发送确认；发票只显示外部 URL 输入／打开。
- 支持 360px 窄屏单列降级与 600px 居中窄工作台；不使用主流程宽表横向滚动。

## Acceptance Criteria

- [ ] 360×800、390×844、430×932、600×960 下没有页面级横向滚动，固定底栏／sheet 不遮挡主操作。
- [ ] 五项导航、返回路径、对象层级、更多菜单与原型一致，当前页面和焦点状态可辨认。
- [ ] 首页、账单、流水、对象、催收、现金、支出、银行的核心操作都可以只靠卡片／详情／sheet 完成，不要求横向找表格列。
- [ ] 手机端可走通房产→房间→租客入住、同步→匹配、原因平账、催收强确认、支出 URL；错误、无数据和同步异常可理解且可恢复。
- [ ] 主操作与危险操作命中区均至少 44×44px；状态不只用颜色；键盘／焦点和 sheet 关闭行为正常。
- [ ] 桌面视觉和功能不因移动断点规则回归；浏览器验收材料记录每个目标视口和关键状态。

## Out of Scope

- 重做领域写入、路由／view model 或桌面页面数据。
- 原生 App、离线、PWA、文件上传、按天分摊或多银行连接。

## Notes

- Keep `prd.md` focused on requirements, constraints, and acceptance criteria.
- Lightweight tasks can remain PRD-only.
- For complex tasks, add `design.md` for technical design and `implement.md` for execution planning before `task.py start`.
