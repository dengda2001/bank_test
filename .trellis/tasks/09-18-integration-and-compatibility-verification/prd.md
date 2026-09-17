# 集成验收与兼容验证

## Goal

完成核心收租场景、四级汇总一致性、旧数据对账、跨用户隔离、功能开关回滚、MySQL 迁移、端到端和响应式验收。

## Requirements

- 验证六个子任务组合后的完整收租链路和四级汇总一致性。
- 覆盖多人、不等分、足额代付、部分代付、超付、撤销、空置和待处理场景。
- 验证旧数据对账、跨用户隔离、迁移升级、功能开关回滚和响应式交互。
- 发现问题时回退到对应子任务修复，不通过修改验收数据掩盖账务不一致。

## Acceptance Criteria

- [ ] `go test ./cmd/truelayer-demo`、`go test ./cmd/rentops-e2e` 和 `go test ./...` 通过。
- [ ] MySQL 空库迁移和含旧数据升级通过，历史全局汇总可解释且不漂移。
- [ ] 个人责任、房间、房产和全局的应收/已收/未收一致。
- [ ] 核心页面、URL 状态、空状态、催缴和代付交互通过验收。
- [ ] 关闭新功能后旧页面仍可读取，后置功能未被误实现。

## Dependencies

- 前置：`09-18-data-model-and-migrations`、`09-18-repository-and-query-boundary`、`09-18-core-rent-ledger`、`09-18-dashboard-aggregates-and-handlers`、`09-18-monthly-rent-workspace-ui`。
- 本任务完成后，父任务才可以进入最终审阅和收尾。

## Verification

- 单元测试、MySQL 测试、端到端测试和浏览器/响应式验收。
- 对照 `prd file/prd.md` 的 Acceptance Criteria 逐项检查。

## Notes

- Keep `prd.md` focused on requirements, constraints, and acceptance criteria.
- Lightweight tasks can remain PRD-only.
- For complex tasks, add `design.md` for technical design and `implement.md` for execution planning before `task.py start`.
