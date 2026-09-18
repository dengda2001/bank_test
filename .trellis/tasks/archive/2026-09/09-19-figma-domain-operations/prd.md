# Figma 领域与运营能力

父任务：`09-18-figma-prototype-alignment`

## Goal

为原型还原提供可审计、可测试的资产与租住安排写入能力，而不依赖静态样例或页面自行计算账务。这个任务只负责领域服务、迁移与必要的 action contract，不交付最终桌面／移动视觉页面。

## Dependencies

- 前置：无。
- 后置消费者：`09-19-figma-page-data-routes` 必须在本任务通过后才可读取／呈现新的资产和安排数据；桌面、移动任务不得绕过这里的服务直接写表。

## Requirements

- 房产与房间支持用户隔离的新建、编辑、详情读取所需的写入、停用；房间必须绑定当前用户的房产。
- 房间的月租和账单日由内部版本化 `tenancyAgreement` 承载，房东不维护第二套“租约”表单。
- 新租客可不选房间；选房间时，从一个未来账单月起自动创建完整租住安排。默认均分同房总月租，允许手工改分，保存前责任总和必须精确守恒。
- 同一租客每个账单月最多一个房间；多人可同住一个房间。合同日期保存但一期按整月生效，不实现按天分摊。
- 变更未来生效安排、换房、退租和停用均不得改写已生成账单、有效收款分配或历史流水。仍有当前／未来安排的房产或房间不能停用。
- 人工平账必须改为“必填原因＋确认”所需的服务输入，创建可审计的余额交易和分配，保持并发不超收。
- 支出增加可选 `invoice_url`，仅接受安全的外部 HTTP(S) 链接；不实现文件上传或存储。
- 不迁移、不猜测旧租客的文本房间地址／月租资料。

## Acceptance Criteria

- [ ] MySQL 升级只追加字段／索引；含旧数据的表不会被删除或批量改写。
- [ ] 单人、同住自动均分、手动不等分、守恒失败、未来换房、退租、空房、不允许同月双房间均有领域测试。
- [ ] 所有关系写入都重新验证 user ID、房产、房间、租客和未来有效月份；失败时事务无残留。
- [ ] 停用被有效安排阻止；已出账月份的应收、收款与流水快照不变。
- [ ] 人工平账缺少原因被拒绝，原因可在审计读取中看到，并发／双击不超额入账。
- [ ] 发票 URL 可为空；非法 scheme、跨用户资产关联和房间不属于选中房产均被拒绝。
- [ ] `go test ./cmd/truelayer-demo/...` 中受影响测试通过；可用时运行受控 MySQL 集成测试。

## Out of Scope

- 规范 GET 页面、导航、桌面／移动 CSS 与原型视觉。
- 旧文本租客资料的结构化迁移。
- 日租金、月中比例、发票文件、多银行授权或同一租客同月多房间。

## Notes

- Keep `prd.md` focused on requirements, constraints, and acceptance criteria.
- Lightweight tasks can remain PRD-only.
- For complex tasks, add `design.md` for technical design and `implement.md` for execution planning before `task.py start`.
