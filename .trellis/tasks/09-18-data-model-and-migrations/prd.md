# 数据结构与数据库迁移

## Goal

设计并落地 Property、Room、TenancyAgreement、AgreementParty、RentCharge、个人责任及支出归属的数据结构；完成只增不删迁移、历史账务兼容和功能开关边界。

## Requirements

- 新增 Property、Room、TenancyAgreement、AgreementParty、RentCharge 等稳定实体。
- 将个人月度责任与房间月度总应收分层表达，并保留历史快照所需引用。
- 为支出增加稳定的 `property_id` 和可选 `room_id`，保留旧文本字段兼容期。
- 采用只增不删的显式 SQL migration，建立 `user_id`、外键、唯一键和查询索引。
- 不实现初始化向导、工作簿导入或自动从旧文本字段推断关系。

## Acceptance Criteria

- [ ] 空库可以成功应用迁移，已有租客、账单、付款和分配数据升级不失败。
- [ ] 同一房间同一时间的有效租约约束、责任金额和房间总额约束有明确表达。
- [ ] 所有新增数据具备用户隔离和稳定主键；历史记录 ID、金额、状态和付款关系不改变。
- [ ] 迁移测试覆盖重复应用、旧数据兼容和功能关闭后的读取路径。

## Dependencies

- 前置：无；需求基线来自父任务和 `prd file/prd.md`。
- 后置：DAO、核心账务、聚合查询和后端接口依赖本任务的数据契约。

## Verification

- MySQL 空库迁移测试。
- 含既有租客、月账单、付款分配和支出的升级测试。
- 用户隔离、外键和重复迁移测试。

## Notes

- Keep `prd.md` focused on requirements, constraints, and acceptance criteria.
- Lightweight tasks can remain PRD-only.
- For complex tasks, add `design.md` for technical design and `implement.md` for execution planning before `task.py start`.
