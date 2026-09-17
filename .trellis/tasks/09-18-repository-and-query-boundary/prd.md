# DAO / Repository 层

## Goal

建立房产、房间、租约、租客责任、付款分配和支出的用户隔离数据访问边界；不依赖文本关系，覆盖越权、空结果和不存在目标测试。

## Requirements

- 建立房产、房间、租约、参与人、月度账务、付款分配和支出的数据访问边界。
- 查询和写入均必须显式接收当前 `user_id`，不使用全局单例或文本字段拼接关系。
- 支持按月份、房产、房间、租客和责任明细读取，为后续账务服务和 Dashboard 聚合提供稳定输入。
- 保持项目现有 GORM + 显式 SQL migration 约定，不引入无实际价值的通用 ORM 抽象。

## Acceptance Criteria

- [ ] 跨用户 ID 查询不到数据，越权写入被拒绝或返回安全的不存在结果。
- [ ] 可读取有效租约、房间责任、付款分配和房产/房间支出。
- [ ] 缺少关系、空列表和不存在房间都有稳定结果，不回退到文本匹配。
- [ ] Repository/Query 层有针对所有权、月份过滤和关键关联的测试。

## Dependencies

- 前置：`09-18-data-model-and-migrations`。
- 后置：核心账务服务和聚合查询依赖本任务提供的数据访问边界。

## Verification

- GORM/MySQL 查询测试。
- 跨用户隔离、空结果、缺失关联和不存在目标测试。
- 关键查询的月份与 ID 条件测试。

## Notes

- Keep `prd.md` focused on requirements, constraints, and acceptance criteria.
- Lightweight tasks can remain PRD-only.
- For complex tasks, add `design.md` for technical design and `implement.md` for execution planning before `task.py start`.
