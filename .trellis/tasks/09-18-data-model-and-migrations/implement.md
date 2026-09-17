# 数据结构与数据库迁移：执行计划

## 实施顺序

1. 先增加迁移契约测试，确认目标 migration 必须包含的表、列、索引、外键和兼容字段。
2. 新增版本化 SQL migration，保持只增不删；对已有 `rent_obligations` 的旧唯一键进行兼容替换。
3. 增加/更新 GORM model，使字段、可空指针和表名与 SQL 完全一致。
4. 增加空库和已有数据的 MySQL 集成测试；验证 migration 重复运行只应用一次。
5. 运行本任务范围内测试及完整后端测试，确认没有影响现有账本行为。

## 预计文件

- `migrations/009_landlord_rent_model.sql`
- `cmd/truelayer-demo/landlord_rent_models.go`
- `cmd/truelayer-demo/landlord_rent_migration_test.go`
- 必要时更新 `cmd/truelayer-demo/*_mysql_test.go` 的共享 fixture

## 验收场景

- 空库应用全部 migration。
- 已有租客、月度责任、银行分配、现金收款和支出数据的数据库升级。
- 同一租客同月关联两个不同房间的责任数据可以共存。
- 房间、租约、参与人和月度总应收可以通过外部 fixture 写入。
- 旧历史责任没有 `rent_charge_id` 时仍可读取，不被迁移删除或重建。
- 重复执行 migration 不产生重复表、索引或数据。

## 验证命令

```bash
gofmt -w cmd/truelayer-demo/landlord_rent_models.go cmd/truelayer-demo/landlord_rent_migration_test.go
go test ./cmd/truelayer-demo -run 'LandlordRent|LedgerMigration|CashReceiptMigration'
go test ./cmd/truelayer-demo
go test ./...
```

带真实 MySQL 时额外设置 `RENTOPS_MYSQL_TEST_DSN`，运行 migration 升级和重复执行测试。

## 回滚点

- SQL migration 只新增表/列和索引；若验证失败，关闭尚未启用的新功能，不删除已有账务数据。
- GORM model 与 migration 必须作为同一逻辑变更回退，避免应用启动时字段不匹配。
