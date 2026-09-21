# 实施计划：统一自动生成与入住分担领域

## 顺序

1. 盘点 `ensureMonthlyObligations`、`ensureRentCharge`、`saveRentArrangementInTx` 的调用图、事务边界和唯一索引；记录现有重复／冲突处理。
2. 先为统一确保服务、结构化租客不走 legacy、未来月份只预览、唯一总应收／责任和分担守恒添加针对性测试。
3. 实现 `EnsureMonthlyRentFacts`，以结构化 arrangement 为优先来源；保留明确受限的 legacy fallback，并统一各业务入口调用。
4. 收敛安排写入：事务性版本替换、单房间／单月约束、整月生效、租金变更分担策略和已固化月份保护。
5. 根据真实索引和并发竞争结果判断是否需要只增 migration；禁止改写旧 migration 或删除历史账务关系。
6. 更新父任务实施清单，供后续 UI 子任务依赖；完成本子任务质量检查。

## 重点文件

- `cmd/truelayer-demo/obligations.go`
- `cmd/truelayer-demo/landlord_rent_ledger.go`
- `cmd/truelayer-demo/rent_workspace.go`
- `cmd/truelayer-demo/landlord_domain_operations.go`
- `cmd/truelayer-demo/tenants.go`
- `cmd/truelayer-demo/landlord_rent_repository.go`
- 相关 Go 测试与必要的新 migration

## 验证

```bash
go test ./cmd/truelayer-demo -run 'Test.*(RentCharge|MonthlyRent|Obligation|Workspace|Dunning|Cash|Matching|Arrangement|TenantRoom|RoomRent|Responsibilit).*' -count=1
go test ./...
go vet ./...
git diff --check
```

MySQL DSN 可用时，另跑迁移／ledger 集成测试，覆盖重复执行、并发、跨用户隔离和 legacy 冲突；确认测试前后无残留数据。

## 风险控制

- 先统一读取／生成入口，再改变安排写入，避免把 UI 过渡期留在“两套 generator 都能运行”的状态。
- 对 legacy 与结构化冲突只报可识别错误，不自动删除或重绑已有个人责任。
- 如需要 migration，只追加新版本；新增约束前需在迁移内幂等修复数据并保留所有付款／现金／催收外键目标。
- 不处理导航与表单；这些变更在依赖本子任务通过后由第二子任务实施。
