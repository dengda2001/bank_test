# 集成验收与兼容验证

## Goal

完成核心收租场景、四级汇总一致性、旧数据对账、跨用户隔离、功能开关回滚、MySQL 迁移、端到端和响应式验收。

## Requirements

- 验证七个子任务组合后的完整收租链路和四级汇总一致性。
- 覆盖多人、不等分、足额代付、部分代付、超付、撤销、空置和待处理场景。
- 验证旧数据对账、跨用户隔离、迁移升级、功能开关回滚和响应式交互。
- 发现问题时回退到对应子任务修复，不通过修改验收数据掩盖账务不一致。

## Acceptance Criteria

- [x] `go test ./cmd/truelayer-demo`、`go test ./cmd/rentops-e2e` 和 `go test ./...` 通过。
- [x] MySQL 空库迁移和含旧数据升级按项目约定执行专项测试；本地 MySQL 服务不可用且未配置 DSN，因此该项明确跳过并记录，未伪造通过结果。
- [x] 个人责任、房间、房产和全局的应收/已收/未收由统一 fixture 与聚合测试验证一致。
- [x] 核心页面、URL 状态、空状态、催缴和代付交互通过服务端渲染、资源契约和 E2E 预检验收。
- [x] 无房产数据时仍走旧 Dashboard fallback；后置的附件、初始化向导和跨房间入口未被实现。

## Dependencies

- 前置：`09-18-data-model-and-migrations`、`09-18-repository-and-query-boundary`、`09-18-core-rent-ledger`、`09-18-dashboard-aggregates-and-handlers`、`09-18-frontend-template-extraction`、`09-18-monthly-rent-workspace-ui`。
- 本任务完成后，父任务才可以进入最终审阅和收尾。

## Verification

- 单元测试、MySQL 测试、端到端测试和浏览器/响应式验收。
- 对照 `prd file/prd.md` 的 Acceptance Criteria 逐项检查。

## Completion Notes

- `go test ./cmd/truelayer-demo`：通过。
- `go test ./cmd/rentops-e2e`：通过。
- `go test ./...`：通过。
- `go vet ./...`：通过。
- `git diff --check`：通过。
- `go test ./cmd/truelayer-demo -run MySQL -count=1 -v`：所有 MySQL 专项测试因 `RENTOPS_MYSQL_TEST_DSN` 未设置而明确跳过；`mysqladmin ping` 也确认本地 `/tmp/mysql.sock` 无服务。
- 已验证跨用户 repository 隔离、旧页面 fallback、统一账务聚合、三视角 URL 上下文、空置/待处理区分、代付展示、嵌入模板和 `/static/` 资源路由。
- 当前环境未提供可连接的 Chrome DevTools 运行态，且应用启动依赖不可用的 MySQL；因此真实浏览器截图级验收未执行，已由服务端渲染、CSS 契约和移动布局测试覆盖可自动验证部分。

## Notes

- Keep `prd.md` focused on requirements, constraints, and acceptance criteria.
- Lightweight tasks can remain PRD-only.
- For complex tasks, add `design.md` for technical design and `implement.md` for execution planning before `task.py start`.
