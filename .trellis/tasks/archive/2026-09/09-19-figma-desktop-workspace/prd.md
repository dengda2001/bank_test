# Figma 桌面端工作台还原

父任务：`09-18-figma-prototype-alignment`

## Goal

以 `figma/rentops-desktop-suite.html` 为桌面端视觉和任务流基线，用已交付的真实页面数据还原完整房东工作台：深色侧栏、顶栏、十一页工作区、实体详情与高风险动作确认。

## Dependencies

- 前置：`09-19-figma-page-data-routes` 已通过路由、读取模型和 action contract 验收。
- 后置：`09-19-figma-mobile-workspace` 在本任务稳定共享模板／token 契约后启动；移动任务不回退桌面已验证的数据语义。

## Requirements

- 提取原型的 OKLCH 色彩、字体、间距、状态、表格、抽屉和按钮 token，建立统一桌面 shell：固定深色侧栏、64px 顶栏、浅色内容区。
- 实现总览、账单、流水、催收、房产、房间、租客、租住安排、现金、支出、银行设置全部桌面页面及房产／房间／租客／流水详情。
- 总览首屏固定四项：本月应收、已收租金、剩余未收、房产支出；经营净额保留为次级／房产详情信息。
- 原型示意控件必须替换为真实等价流程：同步银行流水、人工平账原因＋确认、催收预览＋确认、资产创建／编辑／停用、自动租住安排只读、发票 URL 输入。
- 桌面可保留高信息密度表格，但详情抽屉、弹层和强确认需可键盘操作、焦点管理正确、错误／空状态有文字。
- 不把尚未实现的发票上传、多银行或按天分摊表现成可用按钮。

## Acceptance Criteria

- [ ] 1366×768、1440×900 下导航、顶栏、主操作、表格和详情抽屉与原型层级一致，无页面级横向溢出。
- [ ] 十一个页面与四类详情只显示当前用户的真实数据；无数据、同步失败、无授权、不可发送催收均有明确状态。
- [ ] 从新建房产、房间、租客到未来月应收，以及流水同步／匹配、人工平账、催收确认、支出 URL 可在桌面完成。
- [ ] 抽屉／确认层打开时焦点进入、Escape 和关闭可用、背景不可误操作；状态不只以颜色区分。
- [ ] 已有 `/billing`、现金、OAuth 和其他兼容路由在桌面仍可访问。
- [ ] 浏览器截图与交互验收记录保存到本任务研究材料；受影响 Go 渲染测试同步更新。

## Out of Scope

- 新增领域字段、账务规则、资产关系写入或 canonical view model。
- 移动底部导航、移动卡片主形态和 sheet 布局。

## Notes

- Keep `prd.md` focused on requirements, constraints, and acceptance criteria.
- Lightweight tasks can remain PRD-only.
- For complex tasks, add `design.md` for technical design and `implement.md` for execution planning before `task.py start`.
