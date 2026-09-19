# 房东多房源房间租金视图 MVP

## Goal

按精简后的七阶段拆分实现房东多房源、多房间、多人责任与代付的月度租金工作台；父任务负责需求基线、子任务地图和最终集成验收。

## Requirements

- 以 `prd file/prd.md` 为产品需求基线，以同目录 `design.md` 和 `implement.md` 为方案参考。
- MVP 支持 Property -> Room、多租客同住、不等分个人责任、同房代付、房产/房间/租客三种视角。
- MVP 不提供房产初始化、工作簿导入、跨房间付款入口、月中按天折算或发票附件功能；测试数据由系统外部准备。
- 所有金额和状态必须从同一套月度账务事实向上聚合，历史账务不可被静默改写。
- 子任务按以下顺序推进：数据结构与迁移 → DAO/Repository → 核心账务 → 聚合查询与后端接口 → 模板外置与桌面/移动前端边界 → 前端工作台 → 集成验收。

## Acceptance Criteria

> 以下五条于 2026-09-19 收尾时按子任务记录逐条核对后回填。第 1 条是原文的规划门措辞，
> 已按实际结果改写（见下方「第 1 条的措辞更正」）；第 4 条只完成了一半，保留未勾并单列。

- [x] 七个子任务均已规划、实施、验证并归档，各自 `prd.md` 的验收项全部勾选（34/34）。
- [x] 单房间核心链路覆盖多人、不等分责任、足额代付、部分代付、超付和撤销。
      2026-09-19 实跑佐证（`go test ./cmd/truelayer-demo -count=1 -run ...`，退出码 0）：
      `TestScaleRentResponsibilitiesPreservesSharesAndDistributesRemainderCents`（不等分与余分）、
      `TestValidateRentResponsibilityPlanRequiresExactUniquePositiveOwnership`（守恒校验）、
      `TestBuildRentChargePlanUsesActivePartiesAndExactResponsibilities`（多人责任）、
      `TestTransactionAllocationAllowsOnePaymentToCoverMultipleTenantResponsibilities`（代付覆盖他人）、
      `TestAutomaticRentMatchCapsAllocationAtPayerResponsibility` 与
      `TestDecideRentMatchOverpaymentCapsAtTenantResponsibility`（部分付与超付封顶）。
- [x] 房产、房间、租客和全局汇总口径一致，空置与待处理状态不混淆。
      佐证：集成子任务「个人责任、房间、房产和全局的应收/已收/未收由统一 fixture 与
      聚合测试验证一致」；`09-18-monthly-rent-workspace-ui` 记录空置／待处理／部分缴纳／
      已缴清／他人代付均由状态标签与金额字段区分表达。
- [ ] 旧数据兼容、用户隔离、MySQL 迁移、桌面端和移动端验收通过。**（只完成一半）**
  - [x] 用户隔离：集成子任务已验证跨用户 repository 隔离。
  - [x] MySQL 空库迁移：已在新库上执行并通过。
  - [ ] **MySQL 含旧数据的升级路径未验证** —— 集成子任务明确记录「本地未配置
        `RENTOPS_MYSQL_TEST_DSN`，按约定跳过并记录，未伪造通过结果」。
  - [ ] **桌面端 / 移动端的真实浏览器验收未执行** —— 集成子任务记录「环境未提供可连接的
        Chrome DevTools 运行态」，实际由服务端渲染、CSS 契约与移动布局测试替代覆盖。
- [x] 最终实现范围不包含 PRD 明确列出的后置功能（附件、初始化向导、跨房间入口均未实现）。

### 第 1 条的措辞更正

原文写「七个子任务均有明确验收结果**并保持规划状态，未提前启动实现**」。这半句是父任务
**规划阶段**的门，写在子任务尚未开工时；七个任务后来按计划全部实施并归档，该措辞已不再描述
任何事实，故改写为实际结果。此处保留原文，以免后人以为门被绕过。

### 已知被后续任务推翻的一条（不在本父任务 AC 内，但影响追溯）

集成子任务 `09-18-integration-and-compatibility-verification` 的 AC5 写
「无房产数据时仍走旧 Dashboard fallback」，已勾选。该 fallback 指 `dashboard.go` 的
`rentDashboardTemplate`（旧的无库降级模板）。2026-09-19 的
`09-19-legacy-dashboard-template-removal` 经用户明确裁决（「删掉吧」）将其删除，
断言它的测试一并移除（理由：它断言的标记「None of those render on a live page」）。
⇒ **那条 AC 描述的行为已不存在**，不得再当作待办勾选。删除本身是有据的：该路径在
`db == nil` 时才可达，而应用无库根本起不动，生产不可达。

## 归档时的未验证项（不得读作通过）

1. **MySQL 含旧数据的迁移升级** —— 无 DSN，从未在既有租客/账单/流水/已确认分配的库上跑过。
2. **真实浏览器下的桌面端与移动端验收** —— 无浏览器运行态，全部验收止于服务端渲染与 CSS 契约。

这两项在本任务归档时仍然成立，未随归档消失。第 2 项的验收环境后来由
`09-17-mobile-audit-fixtures`（P0）承接。

## Child Tasks

1. `09-18-data-model-and-migrations`：数据结构与数据库迁移
2. `09-18-repository-and-query-boundary`：DAO / Repository 层
3. `09-18-core-rent-ledger`：核心账务服务
4. `09-18-dashboard-aggregates-and-handlers`：聚合查询与后端接口
5. `09-18-frontend-template-extraction`：模板外置与桌面/移动前端边界
6. `09-18-monthly-rent-workspace-ui`：前端月度工作台
7. `09-18-integration-and-compatibility-verification`：集成验收与兼容验证

## Verification

- 每个子任务完成后独立运行其验收测试。
- 全部子任务完成后运行 `go test ./...`、MySQL 迁移升级测试和端到端场景。

## Notes

- Keep `prd.md` focused on requirements, constraints, and acceptance criteria.
- Lightweight tasks can remain PRD-only.
- For complex tasks, add `design.md` for technical design and `implement.md` for execution planning before `task.py start`.
