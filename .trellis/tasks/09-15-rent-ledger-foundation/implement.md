# 收租账本基础与账单生命周期：实施计划

## 交付顺序

1. **EUR 策略与纯逻辑测试（RED → GREEN）**
   - 先为 EUR-only 规范、分配类型、有效分配过滤、金额预算和 Dublin 逾期投影写失败单元测试。
   - 实现最小常量、校验和投影函数；保留现有调用行为，先运行 `go test ./cmd/truelayer-demo`。

2. **兼容迁移与 GORM 模型**
   - 新增下一个有序 migration，增加 allocation／obligation 审计和作废字段，调整旧唯一键并添加索引／外键。
   - 更新 `paymentAllocation`、`rentObligation` 结构和常量；为迁移文本增加静态约束测试或 MySQL 集成测试入口。
   - 用旧字段默认值验证已有分配可读，非 EUR 新写入被阻断。

3. **账本写入与读取契约**
   - 增加事务边界内的金额守恒、币种、用户归属、幂等和有效分配读取函数。
   - 重算 `paid_amount_cents` 和账单状态，不直接对缓存总额做未经校验的加减。
   - 保持既有银行匹配路径编译和行为兼容；后续银行／现金子任务接入新契约。

4. **验证与交付审查**
   - 运行格式化、单元测试、完整 Go 测试和静态检查。
   - 检查用户隔离、错误信息不泄露数据、迁移幂等和未提交大范围改动。
   - 通过本子任务验收后再交由父任务集成；不启动其他子任务。

## 主要文件范围

- `migrations/003_rent_ledger_foundation.sql`（新增）
- `cmd/truelayer-demo/ledger.go` 或现有账务服务中的共享领域函数（新增／最小修改）
- `cmd/truelayer-demo/obligations.go`、`cmd/truelayer-demo/matching_service.go`（仅为模型／投影契约做必要兼容）
- 对应 `*_test.go`（纯逻辑优先，数据库测试按现有测试基础设施补充）

## 验证命令

- `gofmt -w <changed-go-files>`
- `go test ./cmd/truelayer-demo`
- `go test ./...`
- `git diff --check`
- 若提供 MySQL 测试环境：执行 migration 两次，验证第二次 no-op；用既有分配数据核对金额、行数和状态。

## 风险／回滚点

- 删除旧唯一键前必须确认旧数据不存在同一来源／账单的重复冲突；发现冲突先停止迁移，不自动合并。
- 账单 `paid_amount_cents` 与有效分配不一致时，先记录差异并由重算函数修复，不能在 migration 中猜测业务金额。
- EUR-only 是输入策略，不删除 `currency` 字段；任何非 EUR 数据必须拒绝或保持待处理，不能被格式化成 EUR。
- 本任务只完成账本基础；不要顺手实现 Dashboard、银行拆分、现金表单或邮件。

## 当前增量进度（2026-09-16）

- 已完成 EUR-only 规范、分配类型、有效分配过滤、来源／账单预算校验和
  Europe/Dublin 状态投影的纯逻辑实现及测试。
- 已新增 `003_rent_ledger_foundation.sql` 与 GORM 字段，保留旧分配的
  `rent` 语义，并为押金／其他收入、作废、更正、操作幂等和账单作废留出
  审计字段。
- 已将现有租客输入、月度账单生成、Dashboard 有效账单查询和租客历史的
  房租收款读取接入 EUR／有效分配边界。
- 完整 Go 测试、`go vet`、格式和任务上下文校验已通过；MySQL 实例上的
  真实迁移与并发写入验证仍留在数据库环境可用后的质量门禁。
