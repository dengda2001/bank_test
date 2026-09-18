# Figma 共享页面数据与路由

父任务：`09-18-figma-prototype-alignment`

## Goal

把现有与新增领域能力组织成稳定、用户隔离的页面读取模型、规范路由和操作流程，使桌面与移动端都消费同一份真实数据与状态，而不是各自拼装账务规则。

## Dependencies

- 前置：`09-19-figma-domain-operations` 已通过其领域／迁移验收。
- 后置消费者：`09-19-figma-desktop-workspace` 和 `09-19-figma-mobile-workspace`。它们只改变呈现，不重写本任务的查询、权限或提交语义。

## Requirements

- 提供总览、账单、流水、催收、房产、房间、租客、租住安排、现金、支出、银行设置所需的服务端 view model；所有金额、状态和详情均从真实账务事实读取。
- 注册并保证规范路由：`/bills`、`/transactions`、`/dunning`、`/properties`、`/tenancies`、`/bank`；旧 `/billing`、银行 OAuth／refresh、现金子路由和既有深链保持兼容。
- 建立共享导航语义和桌面／移动均可消费的页面状态；不能在此阶段固化桌面或手机视觉布局。
- 将房产、房间、租客、流水详情所需的数据组织为同一权限边界的读取模型，并提供可复用的空、错、同步不完整状态。
- 接入强确认流程的服务端契约：人工平账原因、催收预览／显式发送／同日重发、现金作废原因、流水撤销原因。
- 银行设置只展示一项授权及其实际同步账户、上次同步、覆盖范围和错误；同步动作调用 refresh-token 流程。
- 支出读取／提交支持真实房产、可选房间和外部发票 URL；旧无关联记录可读为未归属。

## Acceptance Criteria

- [ ] 每个新 GET／POST 路由均要求登录，跨用户 ID 返回不可访问而非泄露详情。
- [ ] 总览四项指标、账单余额、流水匹配、催收候选、现金、支出与详情的金额口径使用同一领域服务。
- [ ] `/billing` 兼容入口、旧表单 action、银行 `/refresh` 与现金预览／作废链路未失效。
- [ ] 账单平账、催收、撤销和作废的页面数据能展示确认前影响、必填原因或不可发送原因。
- [ ] 页面数据测试覆盖空数据、部分收款、同住代付、同步失败、未绑定租客、无效邮箱及用户隔离。
- [ ] 为桌面和手机提供稳定的语义字段，不让 UI 从字符串中重新解析金额、状态或对象 ID。

## Out of Scope

- 数据库领域规则和迁移（前置领域任务所有）。
- 最终桌面／移动 CSS、原型间距、抽屉或 sheet 的视觉实现。

## Notes

- Keep `prd.md` focused on requirements, constraints, and acceptance criteria.
- Lightweight tasks can remain PRD-only.
- For complex tasks, add `design.md` for technical design and `implement.md` for execution planning before `task.py start`.
