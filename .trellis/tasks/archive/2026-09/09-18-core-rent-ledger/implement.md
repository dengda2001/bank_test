# 实施计划：核心账务服务

## 实施顺序

### 1. 先固定纯业务契约

- 为责任等分、责任合计校验、有效覆盖、代付分配、自动覆盖额度和状态重算补充失败测试。
- 测试重点固定为 EUR 1,000 的 600/400、付款人支付 800、付款人支付 1,000 后再拆分到另一责任、超付、作废和旧空字段兼容。
- 不先改数据库或 handler，先让纯函数明确金额与状态口径。

### 2. 实现 Property/Room 租约的月度账务生成

- 增加有效租约/参与人按月份选择与责任计划构造。
- 在用户隔离事务中创建或读取 `rent_charge`，写入房产/房间/租约及名称快照。
- 为每个有效参与人创建 `rent_obligation`，写入责任金额和租客名称快照；重复调用返回同一组事实。
- 保留旧租客 obligation 生成路径，新增路径不自动改写旧数据。

### 3. 扩展银行分配与自动建议

- 去除“一条来源流水只能跨一个租客”的旧限制，但保留每条责任和来源金额的独立预算校验。
- 给匹配决定增加实际可分配金额，使付款人支付超过本人责任时只先覆盖本人责任，余款继续待处理。
- 保持 `MatchedTenantID` 表示付款人匹配；分配行使用责任租客，并新增代付/多责任的投影测试。
- 将自动分配调用纳入稳定幂等 key；手工多行拆分继续复用同一事务。

### 4. 统一现金覆盖与撤销重算

- 检查现金预览、入账、作废与银行分配共同读取有效覆盖事实。
- 补齐银行 + 现金混合覆盖、作废后回退、超付拒绝、同 key 重试和不同事实复用 key 拒绝测试。
- 确认所有更新均带 `user_id`，跨用户目标不能读/写。

### 5. 回归、记录与交接

- 运行核心测试、现有后端测试、`go vet ./...` 和完整 `go test ./...`。
- 若 `RENTOPS_MYSQL_TEST_DSN` 可用，运行 migration、并发锁和真实撤销测试；不可用则明确记录跳过，不伪造通过。
- 为下游聚合查询提供 charge/obligation 读取契约，确认前端不直接依赖新服务内部类型。

## 预计文件

- `cmd/truelayer-demo/landlord_rent_ledger.go`
- `cmd/truelayer-demo/landlord_rent_ledger_test.go`
- `cmd/truelayer-demo/ledger.go`
- `cmd/truelayer-demo/transaction_allocation.go`
- `cmd/truelayer-demo/matching.go`
- `cmd/truelayer-demo/cash_receipts.go`
- 必要时更新对应 MySQL 集成测试文件

## 验收命令

```bash
gofmt -w cmd/truelayer-demo/landlord_rent_ledger.go cmd/truelayer-demo/landlord_rent_ledger_test.go
go test ./cmd/truelayer-demo -run 'Rent|Ledger|Allocation|CashReceipt' -count=1
go test ./cmd/truelayer-demo -count=1
go vet ./...
go test ./...
git diff --check
```

## Review Gate

- 生成逻辑和跨租客分配测试通过后，才能接入聚合查询任务。
- 任何状态字段都必须能回溯到有效 allocation/cash receipt；不能只修改缓存列让测试通过。
- 若发现必须新增 API 字段才能表达代付来源，先记录接口契约，再交给下一个聚合/handler 任务处理。

## 回滚点

- 责任计划和新 charge 生成独立于旧租客 obligation 兼容路径。
- 跨租客分配规则与自动覆盖额度分开提交，发现旧匹配回归时可单独回退。
- 不删除历史 allocation、receipt 或 transaction；回滚只恢复入口/服务行为。

