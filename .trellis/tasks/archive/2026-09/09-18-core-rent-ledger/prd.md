# 核心账务服务

## Goal

实现月度房间总应收、多人个人责任、有效覆盖、银行和现金分配、全额代付、部分代付、超付、撤销及缴费状态计算。

## Requirements

- 根据有效租约生成房间月度总应收和租客个人责任，责任金额支持等分快捷方式和明确不等分金额。
- 固化房产、房间、租约、租客、月份、金额和到期日等历史账务事实。
- 统一银行付款和现金收款的责任覆盖、代付、超付、撤销和幂等规则。
- 允许一笔付款覆盖多个责任，但本期产品入口只处理当前房间。
- 将付款来源与责任完成状态分开，欠租只按责任未覆盖金额判断。

## Acceptance Criteria

- [x] EUR 1,000 两人 600/400 的责任合计正确。
- [x] 一人支付 EUR 1,000 时，两位租客责任均结清，并保留付款人与被覆盖责任的区分。
- [x] 一人支付 EUR 800 时，只自动覆盖本人责任，剩余金额待人工确认。
- [x] 超付不会制造负未收；撤销后所有受影响责任和房间状态正确回退。
- [x] 重复生成、重复提交和分配幂等规则不会重复记账或超过可分配余额。

## Dependencies

- 前置：`09-18-data-model-and-migrations`、`09-18-repository-and-query-boundary`。
- 后置：聚合查询、后端接口和前端代付展示依赖本任务的账务事实。

## Verification

- 核心账务单元测试。
- MySQL 事务锁、并发分配和撤销测试。
- 银行与现金收款的同口径测试。

验收记录（2026-09-18）：责任计划、月份边界、跨租客分配、自动覆盖上限、付款人投影和幂等 key 的单元测试通过；`go test ./cmd/truelayer-demo -count=1`、`go test ./...`、`go vet ./...` 和 `git diff --check` 通过。MySQL 生成、真实事务和并发测试已编译，但因本地未设置 `RENTOPS_MYSQL_TEST_DSN` 按约定跳过。

## Notes

- Keep `prd.md` focused on requirements, constraints, and acceptance criteria.
- Lightweight tasks can remain PRD-only.
- For complex tasks, add `design.md` for technical design and `implement.md` for execution planning before `task.py start`.
