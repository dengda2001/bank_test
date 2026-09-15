# 月度收租总览与查询工作台：实施计划

## 启动门槛

- [x] 审阅 `prd.md`、`design.md`，确认汇总以全量账单为准、列表筛选不影响卡片。
- [x] 执行 `python3 ./.trellis/scripts/task.py start 09-15-monthly-rent-dashboard`，确认状态为 `in_progress` 后再改代码。
- [x] 执行 `trellis-before-dev`，读取 backend database、error、quality 及跨层规范。

## 纵向切片

### 切片 1：月度账单读模型与筛选分页

- [x] 定义白名单状态／排序／分页输入和 typed Dashboard 查询结果。
- [x] 计算全量 EUR 汇总、状态人数，支持姓名／别名／房间文本搜索、状态筛选、排序和稳定分页。
- [x] 保持现金与银行支付明细共用既有收款投影。
- [x] 覆盖三户 €1,000 账单的全量金额／人数和筛选不改变卡片的单元测试。

### 切片 2：到账入口与同步状态

- [x] 按银行到账月份计算待处理余款金额／去重笔数。
- [x] 按有效其他收入分配计算金额／去重流水数，保留期间和处理条件跳转。
- [x] 展示最近同步成功、部分成功、失败和无同步的不同状态。
- [x] 覆盖跨月租金所属月与到账月分离、混合用途金额守恒和用户隔离测试。

### 切片 3：Dashboard 交互与页面入口

- [x] 接入 query 参数、筛选工具栏、分页链接、清除筛选和空态文案。
- [x] 保持月份导航、收款明细展开、现金补录／作废和租客历史入口可用。
- [x] 待处理／其他收入卡片跳转 `/billing` 时保留期间、方向和处理条件。
- [x] 更新模板测试并执行可用环境下的真实 HTTP 检查；当前环境无可用 MySQL DSN／DevTools MCP，浏览器数据库路径留待集成环境复核。

## 检查点

- [x] `go test ./cmd/truelayer-demo -count=1` 与 `go test ./... -count=1` 通过。
- [x] `go vet ./...` 与 `git diff --check` 通过。
- [x] 所有数据库查询按当前用户过滤；筛选、分页、无结果和同步失败不改变全量汇总口径。

## 主要文件范围

- `cmd/truelayer-demo/obligations.go`
- `cmd/truelayer-demo/dashboard.go`
- `cmd/truelayer-demo/main.go`
- `cmd/truelayer-demo/*_test.go`

## 风险与回滚点

| 风险 | 防护 |
| --- | --- |
| 当前页误用于汇总 | 汇总先于筛选分页计算，并用三户精确金额测试锁定 |
| 到账月和租金月混淆 | 待处理／其他收入只读 `payment_transactions.transaction_time`，账单只读 `rent_obligations.period_month` |
| 用户输入注入排序 | 状态、排序和页大小先白名单校验，内存排序不拼 SQL |
| 同步失败显示成空数据 | 查询最近 `bank_sync_runs`，保留 failed/partial 状态并显示明确文案 |
| 现金来源丢失 | 复用 `listRentPayments` 和 typed `rentPaymentDetail`，不在模板重新推断来源 |

## 完成门槛

- [x] PRD Dashboard 验收标准均有单元、MySQL 或 HTTP 证据。
- [x] 通过 Trellis quality check，必要的跨层读模型契约已更新至 backend spec。
- [ ] 代码、任务文档和规范无未提交变更，形成独立 commit、push 并归档。
