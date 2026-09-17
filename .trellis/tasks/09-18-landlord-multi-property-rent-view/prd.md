# 房东多房源房间租金视图 MVP

## Goal

按精简后的六阶段拆分实现房东多房源、多房间、多人责任与代付的月度租金工作台；父任务负责需求基线、子任务地图和最终集成验收。

## Requirements

- 以 `prd file/prd.md` 为产品需求基线，以同目录 `design.md` 和 `implement.md` 为方案参考。
- MVP 支持 Property -> Room、多租客同住、不等分个人责任、同房代付、房产/房间/租客三种视角。
- MVP 不提供房产初始化、工作簿导入、跨房间付款入口、月中按天折算或发票附件功能；测试数据由系统外部准备。
- 所有金额和状态必须从同一套月度账务事实向上聚合，历史账务不可被静默改写。
- 子任务按以下顺序推进：数据结构与迁移 → DAO/Repository → 核心账务 → 聚合查询与后端接口 → 前端工作台 → 集成验收。

## Acceptance Criteria

- [ ] 六个子任务均有明确验收结果并保持规划状态，未提前启动实现。
- [ ] 单房间核心链路覆盖多人、不等分责任、足额代付、部分代付、超付和撤销。
- [ ] 房产、房间、租客和全局汇总口径一致，空置与待处理状态不混淆。
- [ ] 旧数据兼容、用户隔离、MySQL 迁移、桌面端和移动端验收通过。
- [ ] 最终实现范围不包含 PRD 明确列出的后置功能。

## Child Tasks

1. `09-18-data-model-and-migrations`：数据结构与数据库迁移
2. `09-18-repository-and-query-boundary`：DAO / Repository 层
3. `09-18-core-rent-ledger`：核心账务服务
4. `09-18-dashboard-aggregates-and-handlers`：聚合查询与后端接口
5. `09-18-monthly-rent-workspace-ui`：前端月度工作台
6. `09-18-integration-and-compatibility-verification`：集成验收与兼容验证

## Verification

- 每个子任务完成后独立运行其验收测试。
- 全部子任务完成后运行 `go test ./...`、MySQL 迁移升级测试和端到端场景。

## Notes

- Keep `prd.md` focused on requirements, constraints, and acceptance criteria.
- Lightweight tasks can remain PRD-only.
- For complex tasks, add `design.md` for technical design and `implement.md` for execution planning before `task.py start`.
