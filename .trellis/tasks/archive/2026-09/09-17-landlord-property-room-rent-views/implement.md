# 房间入住与租金计划（方案 A）：串行实施交接

> 本文件是给实现 Agent 的施工清单。产品口径见 `prd.md`，完整数据和接口契约见 `design.md`。三者冲突时，先停止实现并修正文档；不要自行恢复旧“租约”模型或兼容逻辑。

## 0. 交付目标与执行纪律

最终产品只有以下心智：

```text
房间详情 = 入住与租金计划的唯一写入口
租客详情 = 人员档案 + 租金责任只读视图
首页租客视角 = 按所选月份查看每个租客唯一房间责任
```

底层模型为：

```text
Property -> Room -> RoomRentPlan -> RoomRentPlanMember
                                -> RentCharge -> RentObligation
                                                  -> Payment / Cash / Dunning
```

实施时必须遵守：

1. 串行完成阶段 0～11；下一阶段开始前，上一阶段的验收项必须通过。阶段 1 的破坏性 schema 一旦落盘，会迫使所有旧模型引用同时切换；若单个 Agent 无法在一个提交内完成，可把阶段 1～3 作为同一原子 cutover 批次，在临时提交中保持分步 diff，但不得部署或交付中间红态，最终通过阶段 3 的联合测试后再形成可交付提交。
2. 每个提交只包含本阶段文件，不得提交仓库里的 `收租明细_Rosewood_20260916.xlsx` 或其他无关改动。
3. 本系统未上线，旧数据是测试数据。不要实现双读、回填、功能开关、旧 URL 重定向或旧业务数据转换。
4. “无兼容”不等于迁移脚本可静默删数据：操作人必须先显式重建开发/测试库；检测到旧业务数据时应拒绝切换。
5. 中间提交可以先增加尚未接线的新代码，但一旦加入 `014_room_rent_plan_reset.sql`，同一提交必须完成模型和仓储切换，保持迁移后的代码可启动。
6. 每个数据库查询都必须带 `user_id` 所有权约束；所有金额进入领域层前转为整数分。
7. 每个阶段先补合同测试，再实现；不要用更新 golden 文本的方式掩盖业务口径变化。
8. 所有月份按 `Europe/Dublin` 解释，数据库保存规范化后的月初日期。

明确不在本任务内：

- 法律合同、合同附件、押金和电子签名；
- 日级租金、月中拆分和按天折算；
- 跨房间自动分配一笔付款；
- 旧数据导入和旧链接兼容；
- 在租客页面编辑房间其他成员的计划。

## 1. 阶段 0：建立基线与冻结契约

### 目标

让后续 Agent 知道哪些失败是改造前已存在的，并用测试先固定本方案不能退让的产品口径。

### 依赖

无。开始前先阅读：

- `.trellis/tasks/09-17-landlord-property-room-rent-views/prd.md`
- `.trellis/tasks/09-17-landlord-property-room-rent-views/design.md`
- `.trellis/spec/` 中与将修改目录对应的规范

### 修改文件

- 新增 `cmd/truelayer-demo/room_rent_plan_contract_test.go`
- 新增或扩展 `cmd/truelayer-demo/workspace_alignment_test.go`
- 新增或扩展 `cmd/truelayer-demo/tenant_detail_layout_test.go`
- 如需记录基线，只更新当前 Trellis task 的工作日志，不改业务代码

### 先写的合同测试

测试先允许失败，但命名和断言必须明确表达：

- `/tenancies` 最终不注册 GET 或 POST；
- 导航和页面不出现“租约管理”“新建租约”“按租约金额计算”；
- 房间详情存在“入住与租金”卡片和唯一编辑入口；
- 房产和房间表单不出现 `active_from` / `inactive_from`，月份边界只存在于 rent plan；
- 租客表单不含 `room_id`、`monthly_rent`、`due_day`、`responsibility_*`；
- 租客详情按所选月份显示房间级责任，只给出房间跳转；
- 首页有效数据中同一租客同月最多一行，并显示所属房间；
- `status=outstanding` 只返回余额大于 0 的有效责任，不包含 `needs_review`；
- 顶部使用“所选月份”和“未结清责任”，不用“本月”和“需继续跟进”。

### 基线命令

```bash
git status --short
go test ./cmd/truelayer-demo -count=1
go test ./cmd/rentops-e2e -count=1
```

将既有失败的测试名和错误摘要记录到任务工作日志；禁止为了得到绿色基线而改掉无关断言。

### 验收与提交边界

- 新测试准确失败在缺少新能力，而不是编译错误或 fixture 错误；
- 基线失败已区分“改造前存在”和“本任务新增合同”；
- 提交只含合同测试和任务日志。

建议提交：`test(rentops): lock room rent plan product contracts`

## 2. 阶段 1：完成破坏性 Schema 切换与新领域模型

### 目标

在空库上得到唯一的新结构，同时让 Go 模型和仓储只认识 `room_rent_plans` / `room_rent_plan_members`。这是唯一允许范围较大的原子提交。

### 依赖

阶段 0 完成；使用一次性 MySQL 库。不得指向需要保留数据的数据库。

### 修改文件

- 新增 `migrations/014_room_rent_plan_reset.sql`
- 修改 `cmd/truelayer-demo/db.go`
- 修改 `cmd/truelayer-demo/landlord_rent_models.go`
- 修改 `cmd/truelayer-demo/landlord_rent_repository.go`
- 重写 `cmd/truelayer-demo/landlord_rent_migration_test.go`
- 重写 `cmd/truelayer-demo/landlord_rent_repository_test.go`
- 删除或改写只验证旧结构的 `cmd/truelayer-demo/rent_obligation_dedup_migration_test.go`
- 必要时机械更新当前包内引用，保证迁移后可以编译；业务行为留到后续阶段重构

### Schema 任务

1. `tenancy_agreements` 重命名为 `room_rent_plans`。
2. `agreement_parties` 重命名为 `room_rent_plan_members`。
3. 将开始/结束列收敛为 `effective_from_month` / `effective_to_month`，删除合同日期、入住日期和状态列。
4. 成员外键改为 `room_rent_plan_id`，删除 `joined_at`、`left_at` 和 `status`。
5. `rooms` 增加 `rent_plan_version BIGINT UNSIGNED NOT NULL DEFAULT 0`，删除月租、缴租日、`active_from` 和 `inactive_from`；`properties` 删除 `inactive_from`。
6. `tenants` 删除租金、周期、日期、房间文本和 payer 镜像字段；付款识别保留在 `tenant_payers`。
7. `rent_charges.tenancy_agreement_id` 政名为 `room_rent_plan_id` 并设为非空。
8. `rent_obligations` 增加非空 `room_rent_plan_id` 和 `room_rent_plan_member_id`，将 `rent_charge_id` 改为非空，删除 legacy 责任唯一键/生成列。
9. 删除 `tenants.payer_id` / `payer_name_hint` 镜像，付款识别只保留在 `tenant_payers`。
10. 将 `cash_receipts.tenant_id` 改为可空 `payer_tenant_id`，增加 `payer_name_snapshot`；责任人只从 obligation 获取。
11. 将 `payment_allocations`、`cash_receipts`、`dunning_send_attempts` 到 obligation 的外键改为 `ON DELETE RESTRICT`，防止绕过服务级联删除审计记录。
12. 重建 `user_id` 组合外键、唯一键和设计文档列出的查询索引：charge 必须引用同一 room 的 plan、同一 room 的 property；obligation 必须与 charge 同 plan/month，且与 plan member 同 plan/tenant。
13. 迁移内容不能包含旧数据映射、复制或默认责任生成。

### 空库前置检查

在 `runMigrations` 应用 014 前调用独立函数，例如：

```go
func validateRoomRentPlanResetPreconditions(db *sql.DB) error
```

当 014 尚未应用时，检查除 `schema_migrations` 外的所有业务表。至少覆盖：

```text
users, bank_connections, properties, rooms, tenants, tenant_payers,
tenancy_agreements, agreement_parties, rent_charges, rent_obligations,
payment_transactions, payment_allocations, cash_receipts,
dunning_sender_configs, dunning_send_attempts, manual_expenses,
manual_expense_invoices, bank_sync_runs, bank_sync_run_accounts,
payment_transaction_actions
```

任一非空都返回包含“重建开发/测试数据库”的稳定错误；不要在检查函数中执行 `DELETE`、`TRUNCATE` 或自动备份。若以后新增业务表，也必须加入检查，不能把上面列表当作永久白名单。fresh database 全部为空时正常执行 001～014，应用启动后再创建管理员。

### Go 模型命名

删除：

```go
tenancyAgreement
agreementParty
TenancyAgreementID
AgreementID // 仅指旧模型语义的字段
```

建立：

```go
roomRentPlan
roomRentPlanMember
RoomRentPlanID
RoomRentPlanMemberID
```

字段与 `design.md` 第 4 节一一对应。不要保留旧类型 alias；它会让运行时代码继续产生旧心智。

### 验收

- fresh database 可从 001 连续迁移到 014；再次执行迁移幂等；
- 含任一旧业务行的数据库在应用 014 前被明确拒绝，数据仍在；
- 014 应用前先校验所需表、列、索引和外键名称；MySQL DDL 可能隐式提交，014 中途失败时必须重建该 disposable DB，不尝试在半迁移结构上续跑；
- `information_schema` 中不存在 `tenancy_agreements`、`agreement_parties`；
- `properties` 不存在 `inactive_from`，`rooms` 不存在 `active_from` / `inactive_from`；切换资产当前状态不自动修改 rent plan 或既有账务事实，inactive 房间仍可保存/结束计划；
- 新表、非空字段、唯一键和组合所有权外键完整；
- 人工插入 room/property、charge/plan/room/month 或 member/plan/tenant 关系不一致的行会被组合外键拒绝；
- Go 运行时代码不再引用旧表名和旧模型类型；
- repository 的跨用户读取/写入测试返回 `gorm.ErrRecordNotFound`。

### 验证命令

```bash
gofmt -w cmd/truelayer-demo/db.go cmd/truelayer-demo/landlord_rent_models.go cmd/truelayer-demo/landlord_rent_repository.go
go test ./cmd/truelayer-demo -run 'Test.*(Migration|Repository|TableName|Reset)' -count=1
RENTOPS_MYSQL_TEST_DSN='<disposable-dsn>' go test ./cmd/truelayer-demo -run 'Test.*(Migration|Repository|Reset)' -count=1
rg -n 'tenancy_agreements|agreement_parties|tenancyAgreement|agreementParty' cmd migrations
```

最后一条只允许旧迁移 009～013 和说明其被 014 替换的测试注释命中；运行时代码不得命中。

### 提交边界

本提交只做 schema、模型、仓储和保证包可编译所需的调用点切换，不接新 UI。调用点切换包括删除 `legacy_import.go` 及 `/import-legacy` 注册，并将仍读取 tenant 旧租金/房间/payer 镜像字段的代码改为新 repository 查询；禁止用旧类型 alias、旧表 view 或字段 fallback 维持编译。若这些调用点必须同时完成阶段 2/3 才能正确工作，则按本文件第 0 节将阶段 1～3 合成一个原子 cutover 提交。

建议提交：`refactor(rentops): replace tenancy schema with room rent plans`

## 3. 阶段 2：切换月度事实生成与事实锁定

### 目标

让当前/历史账务事实只由新 plan 生成；未来月只预览；提供计划写服务可直接复用的事实锁定与重建原语。

### 依赖

阶段 1 的新 schema 与 repository 已可用。测试直接通过 repository 建立计划 fixture，本阶段不依赖 HTTP 或计划写服务。

### 修改文件

- 修改 `cmd/truelayer-demo/monthly_rent_facts.go`
- 修改 `cmd/truelayer-demo/landlord_rent_ledger.go`
- 修改 `cmd/truelayer-demo/obligations.go`
- 新增 `cmd/truelayer-demo/rent_fact_lock.go`
- 新增 `cmd/truelayer-demo/rent_fact_lock_test.go`
- 重写 `cmd/truelayer-demo/rent_monthly_facts_test.go`
- 重写 `cmd/truelayer-demo/landlord_rent_ledger_test.go`
- 删除 `cmd/rentops-e2e/legacy_fixture.go` 的 legacy responsibility 用法；需要 fixture 时改成新计划 fixture

### 唯一生成路径

对 `(user_id, room_id, period_month)`：

1. 查命中月份的唯一 plan；0 条表示空置，超过 1 条返回冲突；
2. 加载所有 plan members，责任合计必须等于 plan 月租；
3. 建一条 `rent_charge`，`room_rent_plan_id` 非空；
4. 每位成员建一条 `rent_obligation`，同时写 `room_rent_plan_member_id`；
5. 写入房产、房间、租客名称快照；
6. 验证 charge 金额等于 obligation 合计；
7. obligation 同时写 `room_rent_plan_id`，并由组合外键保证 charge 和 member 来自同一 plan、tenant 与 member 一致；
8. 并发请求依赖 room 行锁和唯一键最终只生成一套事实。

删除所有从 `tenants.monthly_rent_cents`、room 缓存或 legacy obligation 兜底生成的代码。

### 当前、历史、未来语义

- 历史月和 Dublin 当前月：读取可以幂等补齐 facts；
- 未来月普通页面读取：只返回 forecast DTO，不落库；
- 明确发起未来月付款：允许先物化 facts；
- 已有 facts 与 plan 不一致：返回 `needs_review` / 冲突，不静默更新。

### 锁定和重建原语

实现并测试以下事务内函数，阶段 3 直接复用，禁止另写一份判断：

```go
func rentFactsLockedFromMonth(tx *gorm.DB, userID, roomID uint64, fromMonth time.Time) (bool, error)
func deleteUnlockedRentFactsFromMonth(tx *gorm.DB, userID, roomID uint64, fromMonth time.Time) error
func materializeRoomRentFactsInTx(tx *gorm.DB, userID, roomID uint64, periodMonth time.Time) error
```

只要受影响 obligation 曾关联以下任一记录，`rentFactsLockedFromMonth` 就返回 true：

- `payment_allocations`，包括 voided 和人工平账产生的 allocation；
- `cash_receipts`，包括 voided；
- `dunning_send_attempts`，包括 failed。

不能只看当前有效记录，因为历史审计链同样不可被删除。

`deleteUnlockedRentFactsFromMonth` 必须先调用锁定检查，再按外键顺序删除 obligation、charge；若已锁定则返回 `ErrRentPlanFactsLocked`，不得部分删除。调用方必须已经开启事务并锁定 room。

所有创建或引用 obligation 的路径共用以下锁协议：银行分配先锁 payment transaction，再按 `room_id` 升序锁 rooms；现金收款、催收预留和事实生成先锁 room；之后才按 ID 升序锁 charge/obligation。取得 room 锁后必须重新读取 obligation 并核对 room、period 和 record status。催收请求键在持锁事务内再次检查并由唯一约束兜底；短事务预留 attempt 后释放锁，再执行邮件发送。

### 稳定错误

```text
ErrRentPlanTimelineConflict
ErrRentPlanFactsLocked
ErrRentFactsConflict
```

不要通过字符串比较来区分错误；用 sentinel error 和 `errors.Is`。

### 验收场景

- 一人 1000 生成 charge 1000 + obligation 1000；
- 两人 700/500 生成一条 charge 和两条 obligation；
- 同一请求、并发请求都不重复；
- 计划修改与 allocation/cash/dunning 并发时只有一方先提交，另一方得到稳定 locked/stale 错误，不出现外键裸错误或审计丢失；
- 未来月看首页/详情后数据库仍无 charge；
- allocation、人工平账 allocation、cash、dunning 四类任一存在均判定 locked；
- 作废/失败记录仍锁定；
- 未锁定 facts 可以整组删除，失败时不留下半套数据；
- 生成 facts 后修改房间名/租客名，不改历史快照；
- 不存在 `rent_charge_id IS NULL` 的 obligation。

### 验证命令

```bash
gofmt -w cmd/truelayer-demo/monthly_rent_facts.go cmd/truelayer-demo/landlord_rent_ledger.go cmd/truelayer-demo/obligations.go cmd/truelayer-demo/rent_fact_lock.go cmd/truelayer-demo/rent_fact_lock_test.go cmd/truelayer-demo/rent_monthly_facts_test.go cmd/truelayer-demo/landlord_rent_ledger_test.go
go test ./cmd/truelayer-demo -run 'Test.*(MonthlyRentFacts|RentChargePlan|FactsLocked|Forecast)' -count=1
RENTOPS_MYSQL_TEST_DSN='<disposable-dsn>' go test ./cmd/truelayer-demo -run 'Test.*(MonthlyRentFacts|FactsLocked).*MySQL' -count=1
```

### 提交边界

只交 facts 生成、锁定/删除原语和 fixture 迁移；不要同时改页面或计划写入口。

建议提交：`refactor(rentops): generate monthly facts from room rent plans`

## 4. 阶段 3：实现计划时间线写服务

### 目标

实现房间级唯一写模型：从月份 M 起替换完整计划，或从月份 M 起让房间空置。

### 依赖

阶段 2 的 facts 生成、锁定和删除原语完成。

### 修改文件

- 新增 `cmd/truelayer-demo/room_rent_plans.go`
- 新增 `cmd/truelayer-demo/room_rent_plans_test.go`
- 新增 `cmd/truelayer-demo/room_rent_plans_mysql_test.go`
- 收窄或删除 `cmd/truelayer-demo/landlord_domain_operations.go` 中旧 arrangement 写逻辑
- 修改 `cmd/truelayer-demo/landlord_rent_repository.go`，仅补齐新服务真正需要的最小仓储方法

### 必须实现的接口

```go
type RoomRentPlanMemberInput struct {
    TenantID            uint64
    ResponsibilityCents int64
}

type SaveRoomRentPlanCommand struct {
    UserID                  uint64
    RoomID                  uint64
    EffectiveMonth          time.Time
    MonthlyRentCents        int64
    Currency                string
    DueDay                  int
    Members                 []RoomRentPlanMemberInput
    ExpectedTimelineVersion uint64
}

type EndRoomRentPlanCommand struct {
    UserID                  uint64
    RoomID                  uint64
    VacantFromMonth         time.Time
    ExpectedTimelineVersion uint64
}
```

服务方法可遵循包内小写命名，但输入、错误和事务语义不得变化。

### 保存事务顺序

1. 做无数据库输入校验和月份规范化。
2. 开启事务，按 `(user_id, room_id)` `FOR UPDATE` 锁定房间。
3. `SaveRoomRentPlan` 和 `EndRoomRentPlan` 校验房间归属与 `ExpectedTimelineVersion`；都不因房间或房产 inactive 状态拒绝。
4. 批量查询成员租客，验证都属于当前用户、active、无重复。
5. 调用阶段 2 的 `rentFactsLockedFromMonth`；锁定时返回 `ErrRentPlanFactsLocked`。
6. 调用 `deleteUnlockedRentFactsFromMonth`，清理 M 起可能存在的未锁定 facts。
7. 将覆盖 M 的旧计划截止到 `M - 1 month`。
8. 删除 `effective_from_month >= M` 的未来计划；依赖外键级联删除成员。
9. 插入从 M 起的新 plan 和完整 member 集合。
10. 运行区间不重叠与责任合计不变量检查。
11. 若 M 是 Dublin 当前月，调用 `materializeRoomRentFactsInTx` 按新计划重建当月 facts；未来月不物化。
12. 将 `rooms.rent_plan_version` 原子加一并返回新值。
13. 提交事务；任一步失败整体回滚。

这里的 `rentFactsLockedFromMonth` 与删除范围都是 `period_month >= M`，覆盖所有将被 replace-from 删除的未来版本。未来月份若只有未引用 facts，可先删除 facts 再删 plan/member；若已有任意 allocation、cash 或 dunning 历史，则必须在时间线变更前返回 `ErrRentPlanFactsLocked`。

`EndRoomRentPlan` 使用相同锁和版本，先删除 M 起未锁定 facts，再截断覆盖 M 的版本、删除 M 起未来版本，不插入新版本。`VacantFromMonth=M` 的含义是 M 月开始空置，`M-1 month` 是最后一个产生责任的月份。房间或房产 status 变化不自动结束计划，也不改变事实生成；只有该操作改变租金时间线。

### 稳定错误

```text
ErrInvalidRentPlan
ErrRentPlanTimelineConflict
ErrRentPlanFactsLocked
ErrStaleRentPlanTimeline
gorm.ErrRecordNotFound
```

不要通过字符串比较来区分错误；用 sentinel error 和 `errors.Is`。

### 关键测试

- 首次保存；
- 从 M 起替换并保留 M 前历史；
- 删除 M 起多个未来版本；
- `VacantFromMonth=M` 后 M 为空置、M-1 保留；
- 两人 700/500 合法，700/400 拒绝；
- 全部责任留空时均分，1000.01 两人按 tenant ID 升序为 500.01/500.00；
- 部分金额留空拒绝；
- 重复租客、跨用户租客、inactive 租客拒绝；
- 同一租客同月加入不同房间必须拒绝，返回 `ErrTenantRoomMonthConflict`；目标月份前已结束的历史计划不冲突；
- 当前月无动作时修改计划可原子重建；
- allocation、cash、dunning 任一历史存在时保存/结束均拒绝；
- 两个并发版本只有一个成功，另一个 `ErrStaleRentPlanTimeline`；
- 任意失败后旧时间线、facts 和版本号均不变；
- 人工制造重叠时返回 `ErrRentPlanTimelineConflict`。

### 验证命令

```bash
gofmt -w cmd/truelayer-demo/room_rent_plans.go cmd/truelayer-demo/room_rent_plans_test.go cmd/truelayer-demo/room_rent_plans_mysql_test.go cmd/truelayer-demo/landlord_rent_repository.go
go test ./cmd/truelayer-demo -run 'Test(RoomRentPlan|SplitResponsibility)' -count=1
RENTOPS_MYSQL_TEST_DSN='<disposable-dsn>' go test ./cmd/truelayer-demo -run 'TestRoomRentPlan.*MySQL' -count=1
```

### 提交边界

只交领域写服务和测试，不接 HTTP，不改页面。

建议提交：`feat(rentops): add versioned room rent plan service`

## 5. 阶段 4：收窄租客写模型

### 目标

租客新增/编辑只维护人员档案和付款识别，不再隐式改变任何房间计划。

### 依赖

阶段 2 已完全移除 legacy tenant fallback。

### 修改文件

- 修改 `cmd/truelayer-demo/tenants.go`
- 修改 `cmd/truelayer-demo/web/templates/partials/tenant-form-drawer.html`
- 修改 `cmd/truelayer-demo/web/static/css/pages/entity-drawers.css`（仅需要时）
- 重写 `cmd/truelayer-demo/tenant_profile_test.go`
- 重写 `cmd/truelayer-demo/tenant_profile_mysql_test.go`
- 更新 `cmd/truelayer-demo/entity_drawers_test.go`

### 输入合同

tenant profile 只允许：

```text
name
display_alias
email
status
payer_id
payer_name_hint
```

其中 payer 信息写 `tenant_payers`，不写回 `tenants`。删除 `tenantInput` 中的房间、月份、月租、缴租日、入住起止和全体责任 JSON。

### 行为约束

- 修改租客姓名只影响当前档案，不重写历史 obligation 快照；
- inactive 前若仍参与当前或未来 plan，返回可操作错误并引导去房间详情处理；不要级联结束其他成员；
- 新增租客后保持未分配状态，房东再去房间详情加入计划；
- POST 中即使手工注入旧字段也必须忽略或拒绝，绝不能产生 plan。推荐拒绝并记录 `invalid tenant profile field`。

### 验收

- 租客抽屉桌面和移动端均不出现房间、月租、缴租日和责任分配；
- 保存租客前后 `room_rent_plans`、members、charges、obligations 计数不变；
- payer 新增/更新仍能被银行流水匹配；
- 跨用户 payer/tenant 操作失败；
- 旧表单字段不能触发隐藏写入。

### 验证命令

```bash
gofmt -w cmd/truelayer-demo/tenants.go cmd/truelayer-demo/tenant_profile*_test.go cmd/truelayer-demo/entity_drawers_test.go
go test ./cmd/truelayer-demo -run 'Test(TenantProfile|TenantPayer|TenantDrawer)' -count=1
RENTOPS_MYSQL_TEST_DSN='<disposable-dsn>' go test ./cmd/truelayer-demo -run 'TestTenantProfile.*MySQL' -count=1
```

### 提交边界

只改租客写模型及对应表单；暂不改租客详情读取。

建议提交：`refactor(rentops): keep tenant editing profile only`

## 6. 阶段 5：把房间详情做成唯一计划入口

### 目标

实现“当前入住与租金”卡片、编辑抽屉、结束入住动作，并把新房间创建改为两步。

### 依赖

阶段 2 facts 行为和阶段 3 写服务已稳定。

### 修改文件

- 修改 `cmd/truelayer-demo/page_data_routes.go`
- 修改 `cmd/truelayer-demo/main.go`
- 修改 `cmd/truelayer-demo/web/templates/pages/room-detail.html`
- 修改 `cmd/truelayer-demo/web/templates/partials/room-create-drawer.html`
- 修改 `cmd/truelayer-demo/web/static/css/pages/room-detail.css`
- 修改 `cmd/truelayer-demo/web/static/css/pages/entity-drawers.css`
- 新增 `cmd/truelayer-demo/room_rent_plan_handlers.go`
- 新增 `cmd/truelayer-demo/room_rent_plan_handlers_test.go`
- 更新 `cmd/truelayer-demo/page_data_routes_test.go`
- 更新 `cmd/truelayer-demo/mobile_layout_test.go` 和 `desktop_layout_test.go`

### 路由

```text
GET  /rooms/{roomID}?period=YYYY-MM&rent=1
POST /rooms/{roomID}/rent-plans
POST /rooms/{roomID}/rent-plans/end
```

POST 必须走现有认证中间件，handler 只负责：解析 decimal 为 cents、解析月份、组装 command、调用服务、稳定错误映射、白名单 return_to 跳转。不要在 handler 里复写事务逻辑。

### 页面数据

房间详情 DTO 增加：

```text
SelectedPeriod
TimelineVersion
CurrentRentPlan / ForecastRentPlan
RentPlanMembers
CanEditRentPlan
RentPlanLockedReason
HasFutureVersions
```

计划卡片显示：生效区间、月租、缴租日、成员和个人责任、合计。无命中计划时显示“当前为空置房”和“设置入住与租金”。

### 抽屉字段

```text
effective_month
monthly_rent
currency=EUR
due_day
tenant_ids (repeatable)
responsibility_<tenantID>
expected_timeline_version
return_to
```

全部责任空白代表均分；只填一部分在页面顶部显示错误并保留输入。未来版本存在时必须在提交前显示“会替换该月及之后安排”的提示。

### 稳定错误映射

```text
ErrInvalidRentPlan          -> rent_plan_invalid
ErrRentPlanFactsLocked      -> rent_plan_locked
ErrStaleRentPlanTimeline    -> rent_plan_stale
ErrRentPlanTimelineConflict -> rent_plan_conflict
gorm.ErrRecordNotFound      -> 404
```

不要把 SQL 错误通过 query string 返回。

### 新建房间两步流

- 房间创建只写 `property_id`、名称、房型、容量和备注；移除 rooms 表单中的月租、缴租日、`active_from` 和 `inactive_from`；
- 房间编辑更改 `property_id` 前检查是否已有任一 `rent_charges`；有事实时返回稳定冲突，且不改房间或历史数据；无事实时可更正所属房产；
- 该检查与事实生成共用 room 行锁，避免“检查无事实”后并发生成 charge 再改 property；返回 `ErrRoomPropertyLocked` 时映射为 `room_property_locked` 和明确中文提示；
- 主按钮“保存并设置入住与租金”成功后跳 `/rooms/{id}?period=<month>&rent=1`；
- 次按钮“仅保存空房间”返回房间列表；
- 第二步关闭或失败时，已创建房间是合法空置房，不做补偿删除。

### 验收

- 房间详情是整个产品唯一可编辑月租、缴租日、成员责任的位置；
- 保存成功后卡片、当月 facts 和版本号同步刷新；
- stale、locked、invalid、conflict 分别有清晰中文错误；
- 关闭入住后所选月份显示空置，但历史月份仍显示历史安排；
- `return_to=https://evil.example` 等外站值被拒绝；
- 移动端可完成选择成员、填写金额、提交和查看错误，按钮不被隐藏；
- 新建房间两个按钮行为符合两步流。
- 房间已有月度事实时迁移所属房产被拒绝；无事实时可更正，空置金额页面显示 `—` 且汇总按 0 计算。
- 并发房产更正与首次事实生成串行后，只能保留同一所属房产关系；不能出现 charge/property 与 room/property 不一致。

### 验证命令

```bash
gofmt -w cmd/truelayer-demo/page_data_routes.go cmd/truelayer-demo/main.go cmd/truelayer-demo/room_rent_plan_handlers*.go
go test ./cmd/truelayer-demo -run 'Test(RoomRentPlanHandler|RoomDetail|RoomCreate|Mobile.*Room|Desktop.*Room)' -count=1
```

### 提交边界

只交房间写入口和创建跳转，不改首页聚合。

建议提交：`feat(rentops): manage occupancy and rent from room detail`

## 7. 阶段 6：将租客详情改为责任只读投影

### 目标

租客仍是页面主体；详情按月份展示其在各房间的租金责任，计划编辑只跳往房间。

### 依赖

阶段 2 事实模型和阶段 5 房间入口完成。

### 修改文件

- 修改 `cmd/truelayer-demo/tenant_detail.go`
- 修改相关 tenant detail 模板或 `page_data_routes.go` 中的内嵌数据组装
- 修改 `cmd/truelayer-demo/tenant_detail_layout_test.go`
- 必要时新增 `cmd/truelayer-demo/tenant_detail_mysql_test.go`

### 查询合同

```text
GET /tenants/{tenantID}?period=YYYY-MM
```

当前/历史月从 `rent_obligations` 出发，按 `user_id + tenant_id + period_month` 查询，连接 charge 获取唯一 property/room 快照。未来月从 plan member 预览。返回行粒度固定为：

```text
tenant_id × period_month（行内包含唯一 room_id）
```

不得只按 `tenant_id + period_month` 聚合。

### 页面合同

表格列：

```text
房产 / 房间 | 个人责任 | 已覆盖 | 未付 | 付款来源 | 状态 | 操作
```

操作只有“查看房间”或“前往房间调整入住与租金”。删除“租期”“租约金额”“按租约金额计算”等文案。

### 验收场景

- A、B 在同月同一房间分别承担 600、300，详情展示两行；同一租客同月不能出现在两个房间；
- A 付款覆盖 B，B 行显示已覆盖和“他人代付”，B 不显示欠租；
- A 的档案改名后历史责任金额和历史快照不变；
- future period 显示 forecast，但不插入数据库；
- 切换月份保留 tenant ID 和返回上下文；
- 租客页不存在任何计划 POST 或隐藏计划字段。

### 验证命令

```bash
gofmt -w cmd/truelayer-demo/tenant_detail.go cmd/truelayer-demo/tenant_detail*_test.go
go test ./cmd/truelayer-demo -run 'TestTenantDetail' -count=1
RENTOPS_MYSQL_TEST_DSN='<disposable-dsn>' go test ./cmd/truelayer-demo -run 'TestTenantDetail.*MySQL' -count=1
```

### 提交边界

只交租客详情读取和文案，不混入首页聚合。

建议提交：`feat(rentops): show room-level liabilities on tenant detail`

## 8. 阶段 7：首页三视角切换到统一责任事实

### 目标

保留房产、房间、租客三视角，尤其保证房东可以在所选月份按租客找到所有未结清责任。

### 依赖

阶段 2 新 facts 和阶段 6 责任投影稳定。

### 修改文件

- 修改 `cmd/truelayer-demo/rent_workspace.go`
- 修改 `cmd/truelayer-demo/rent_workspace_page.go`
- 修改 `cmd/truelayer-demo/dashboard_filters.go`
- 修改 `cmd/truelayer-demo/dashboard_metrics.go`
- 修改 `cmd/truelayer-demo/ledger.go`
- 修改 `cmd/truelayer-demo/cash_receipts.go`
- 修改 `cmd/truelayer-demo/web/templates/pages/rent-workspace.html`
- 修改 `cmd/truelayer-demo/web/static/css/pages/rent-workspace.css`
- 重写/扩展 `cmd/truelayer-demo/rent_workspace_test.go`
- 更新 `cmd/truelayer-demo/dashboard_filters_test.go` 和 `dashboard_layout_test.go`

### 读取管线

先生成 canonical responsibility rows，并独立加载资产骨架：

```text
obligation/forecast + charge/plan + room + property
+ effective allocations + effective cash/manual settlement

asset spine = properties + rooms
```

资产骨架包含当前 active 资产，以及所选月份有 facts、命中 rent plan 或有 forecast 的 inactive 资产；任何纳入房间的所属房产也纳入 property view。先按候选范围幂等补齐当前/历史 facts，再构造列表，避免 inactive 资产因尚未物化而消失。

然后：

```text
tenant view   = responsibility rows，不聚合
room view     = asset spine LEFT JOIN responsibility，按 property_id + room_id 聚合
property view = properties LEFT JOIN room aggregates，按 property_id 聚合
```

不能让三个 view 各自重算 paid/balance/status。共用一个金额与状态函数。

零责任房间生成 `vacant` room row，expected/paid/balance 内部按 0 汇总、页面金额显示 `—`，计入房间数量但不计入已交满/未交满，也不生成 tenant row；零房间或全空置房产仍生成 property row。

### 状态算法

```text
paid = 有效租金分配（银行分配及人工平账分配） + 有效现金收款
balance = max(expected - paid, 0)

paid >= expected              -> paid
else DublinToday > dueDate    -> overdue
else paid > 0                 -> partial
else                          -> open
```

单条责任分类顺序：

```text
needs_review -> forecast（未来预览） -> paid -> overdue -> partial -> open
```

聚合判断按以下顺序实现，不直接取任一子行状态：`needs_review` -> `vacant`（零责任）-> `forecast`（未来计划预览）-> `overdue`（任一未结清责任过期）-> `paid`（全部结清）-> `partial`（有覆盖且仍有余额）-> `open`。`outstanding` 不包含 forecast、vacant 或 needs_review。

注意：已过期的部分付款是 `overdue`，不是 `partial`。

### 筛选合同

```text
GET /rent-dashboard?period=YYYY-MM&view=tenants&status=outstanding
```

`outstanding` = 有效且 `balance_cents > 0`；不包含 `needs_review`。筛选值包括：

```text
all, outstanding, needs_review, overdue, partial, open, paid, forecast
```

文本和状态筛选只改变列表行，不改变顶部所选 scope 汇总；property/room scope 可以改变汇总。

### 页面文案

- 所有月份汇总写“所选月份”；
- “需继续跟进”改为“未结清责任”；
- `needs_review` 另列“待核对”，不能算欠租；
- tenant 行展示责任人、房产/房间、个人责任、已覆盖、未付、付款来源、状态和催缴操作；
- 催缴依据 obligation balance，而不是实际付款人。

### 必测数据

1. 两套房产；
2. 一间空置房；
3. 一间 700/500 两人同住房；
4. A 付款 1200 并分别覆盖 A/B；
5. 一条部分付款且已逾期责任；
6. 一条 needs_review；
7. 同一租客同月两个房间的冲突拒绝，以及目标月份前结束后的边界放行；
8. 当前、历史、未来月份。

### 验收

- `view=tenants&status=outstanding` 精确列出欠款责任，不漏、不合并、不重复；
- 被同住人代付后，责任人显示 paid，不出现在 outstanding；
- needs_review 不进入 outstanding，但有独立计数和筛选；
- 三视角 expected/paid/balance 逐层求和一致；
- 空置房计入房间数量但不计入已交满/未交满；
- 没有责任的房间和没有房间的房产仍分别出现在 room/property 视角，tenant 视角不生成占位行；
- inactive 房间在所选当前/历史月份命中计划但尚无 facts 时仍进入资产骨架并幂等补齐 facts；
- inactive 房产只要包含一个被纳入的房间，就必须出现在 property view；
- 支出和经营净额逻辑不影响租金结清状态；
- 所选月份和筛选参数在视角切换、详情跳转后保留。

### 验证命令

```bash
gofmt -w cmd/truelayer-demo/rent_workspace*.go cmd/truelayer-demo/dashboard_filters*.go cmd/truelayer-demo/dashboard_metrics.go cmd/truelayer-demo/ledger.go cmd/truelayer-demo/cash_receipts.go
go test ./cmd/truelayer-demo -run 'Test(RentWorkspace|WorkspaceDimension|DashboardFilter)' -count=1
RENTOPS_MYSQL_TEST_DSN='<disposable-dsn>' go test ./cmd/truelayer-demo -run 'TestRentWorkspace.*MySQL' -count=1
```

### 提交边界

只交统一读取模型、首页筛选和页面，不顺手重构付款服务。

建议提交：`feat(rentops): project monthly liabilities across dashboard views`

## 9. 阶段 8：移除独立入口和全部旧术语

### 目标

彻底删除独立“租约”入口；GET 和 POST `/tenancies` 都返回 404，不重定向。

### 依赖

房间和租客新入口已经可完成所有所需操作。

### 删除文件

- `cmd/truelayer-demo/tenancy_pages.go`
- `cmd/truelayer-demo/tenancy_pages_test.go`
- `cmd/truelayer-demo/web/templates/pages/tenancies.html`
- `cmd/truelayer-demo/web/static/css/pages/tenancies.css`

### 修改文件

- `cmd/truelayer-demo/main.go`：移除 GET/POST 注册
- `cmd/truelayer-demo/page_data_routes.go`：删除 tenancy DTO、template 和旧处理器
- `cmd/truelayer-demo/workspace_shell.go`：删除标题映射和资源引用
- `cmd/truelayer-demo/web/templates/partials/workspace-nav.html`：确保无入口
- `cmd/truelayer-demo/web_embed_test.go`
- `cmd/truelayer-demo/workspace_shell_test.go`
- `cmd/truelayer-demo/page_data_routes_test.go`
- `cmd/truelayer-demo/prototype_preview_render_test.go`
- `cmd/truelayer-demo/list_pages_alignment_test.go`

### 禁止残留的产品文案

```text
租约管理
新建租约
租约状态
租期
按租约金额计算
```

底层允许 `rent_obligation` 技术名；前台统一称“租金责任”。旧 migrations 009～013 可作为迁移历史保留旧表名，运行时代码和最终页面不允许。

### 验收

- 未登录和已登录请求 GET/POST `/tenancies` 都不命中业务 handler；
- 不产生 301/302 到其他页面；
- 导航、更多页、页面模板、CSS 和 JS 无旧入口；
- 页面可见文案无禁止词；
- 删除旧文件后 embed 构建仍成功。

### 验证命令

```bash
gofmt -w cmd/truelayer-demo/main.go cmd/truelayer-demo/page_data_routes.go cmd/truelayer-demo/workspace_shell.go
go test ./cmd/truelayer-demo -run 'Test.*(Tenancies|Navigation|WorkspaceShell|Embedded|Terminology)' -count=1
rg -n '租约管理|新建租约|租约状态|按租约金额计算|/tenancies' cmd/truelayer-demo
rg -n 'tenancyAgreement|agreementParty|tenancy_agreements|agreement_parties' cmd/truelayer-demo
```

两个 `rg` 在运行时代码应为零命中；若测试在断言“禁止出现”时包含字面量，应逐项人工确认。

### 提交边界

只做旧入口和旧术语清理。

建议提交：`refactor(rentops): remove standalone tenancy workflow`

## 10. 阶段 9：付款、现金、平账和催收回归

### 目标

证明新计划模型没有把责任人与实际付款人重新耦合，也没有破坏现有账本。

### 依赖

新 facts 和首页都已经使用 `rent_obligations`。

### 重点审计文件

- `cmd/truelayer-demo/transaction_allocation.go`
- `cmd/truelayer-demo/ledger.go`
- `cmd/truelayer-demo/cash_receipts.go`
- `cmd/truelayer-demo/dashboard_manual_balance.go`
- `cmd/truelayer-demo/dunning_candidates.go`
- `cmd/truelayer-demo/dunning_service.go`
- 对应 `*_test.go` / `*_mysql_test.go`

### 保持的不变量

- 一笔付款可以分配到同房多条 obligation；
- obligation 的 tenant 是责任人，transaction 的 payer 是实际付款来源；
- 现金收款用 `payer_tenant_id` / `payer_name_snapshot` 解释实际付款人，不要求等于 obligation 的 tenant；
- 分配总额不能超过流水可用余额；
- 单条 obligation 有效覆盖不能超过 expected；
- 多余款项留在流水未分配余额，不制造负 balance；
- 作废后有效 paid 重算，但历史 allocation/receipt 仍存在并锁定 plan；
- 催收候选只由 responsibility balance 和 due date 决定。

### 必测案例

```text
Room rent 1200
A responsibility 700
B responsibility 500
A actual payment 1200
allocations: A=700, B=500
```

结果：A/B 均 paid；B 标记他人代付；无 outstanding；实际付款来源仍是 A；作废任一分配后只相应恢复责任余额。

再覆盖：不足额人工分配、超付、并发分配、幂等 request key、A 现金替 B 支付、非租客现金付款人、人工平账和催收候选。

### 验收与验证命令

```bash
gofmt -w cmd/truelayer-demo/transaction_allocation.go cmd/truelayer-demo/ledger.go cmd/truelayer-demo/cash_receipts.go cmd/truelayer-demo/dashboard_manual_balance.go cmd/truelayer-demo/dunning_candidates.go cmd/truelayer-demo/dunning_service.go
go test ./cmd/truelayer-demo -run 'Test.*(Allocation|Ledger|Cash|ManualBalance|Dunning)' -count=1
RENTOPS_MYSQL_TEST_DSN='<disposable-dsn>' go test ./cmd/truelayer-demo -run 'Test.*(Allocation|Ledger|Cash|ManualBalance|Dunning).*MySQL' -count=1
```

没有发现问题时，本阶段允许只提交新增回归测试；不要为“顺手整洁”重写账本。

建议提交：`test(rentops): cover third-party payment against rent liabilities`

## 11. 阶段 10：端到端、移动端与运维文档

### 目标

用一套接近真实的数据跑完整业务链，并明确开发环境重建和回滚方法。

### 修改文件

- 修改 `cmd/rentops-e2e/tenant_scenario.go`
- 修改 `cmd/rentops-e2e/dashboard_scenario.go`
- 修改 `cmd/rentops-e2e/ledger_scenario.go`
- 删除/替换 `cmd/rentops-e2e/legacy_fixture.go`
- 必要时新增 `cmd/rentops-e2e/room_rent_plan_scenario.go`
- 修改 `README.md`
- 更新相关浏览器/布局测试

### E2E 顺序

1. 新建房产和物理房间；
2. 新建 A/B 两个租客，只填档案和 payer；
3. 从房间详情设置 1200 月租、5 日到期、A=700/B=500；
4. 首页房间视角看到应收 1200；
5. 首页租客视角看到两条责任；
6. 导入/创建 A 支付的 1200 流水，分配到 A/B；
7. 两条责任均结清，B 显示他人代付；
8. 在未来月份调整责任，确认普通预览不落 facts；
9. 制造部分付款逾期责任，确认 `outstanding` 和 `overdue`；
10. 查看租客详情两行责任并跳转房间；
11. 确认 `/tenancies` GET/POST 404。

### README 必须说明

- 本变更需要删除并重建开发/测试数据库；
- 014 检测到旧业务数据会拒绝执行；
- 不支持直接在旧库上升级；
- 代码回滚必须同时重建与旧代码匹配的数据库，不能只回退二进制；
- 如何配置 disposable `RENTOPS_MYSQL_TEST_DSN` 并运行迁移/集成测试。

### 浏览器验收

在工具可用时至少检查 1440px 桌面与 390px 移动宽度：

- 房间详情卡片和抽屉完整可见；
- 租客表单没有计划字段；
- 租客详情责任表在移动端不横向丢失关键动作；
- 首页 tenant 视角能按月份、outstanding 和 overdue 筛选；
- 键盘可打开、提交、关闭抽屉，错误区域可聚焦；
- 空置、forecast、needs_review、paid、overdue 均有可区分文本，不只靠颜色。

### 验证命令

```bash
go test ./cmd/rentops-e2e -count=1
go test ./cmd/truelayer-demo -run 'Test.*(Mobile|Desktop|Layout|Render|Navigation)' -count=1
```

### 提交边界

只交 E2E、可访问性/布局修复和 README。

建议提交：`test(rentops): verify room rent plan workflow end to end`

## 12. 阶段 11：最终质量门与交付

### 代码和文档审计

```bash
git diff --name-only --diff-filter=ACM -- '*.go' | while IFS= read -r file; do [ -z "$file" ] || gofmt -w "$file"; done
go test ./cmd/truelayer-demo -count=1
go test ./cmd/rentops-e2e -count=1
go test ./... -count=1
rg -n 'tenancyAgreement|agreementParty|tenancy_agreements|agreement_parties' cmd
rg -n '租约管理|新建租约|租约状态|按租约金额计算|/tenancies' cmd
git diff --check
git status --short
```

MySQL 最终门：

```bash
RENTOPS_MYSQL_TEST_DSN='<disposable-dsn>' go test ./cmd/truelayer-demo -count=1
```

### 最终一致性核对

- `rent_charge.expected_amount_cents == SUM(active obligation.expected_amount_cents)`；
- obligation 合计等于来源 plan 月租；
- 每条 obligation 的 charge 与 plan member 属于同一 `room_rent_plan_id`，且 obligation tenant 等于 member tenant；
- paid cache 等于有效租金分配（银行分配及人工平账分配）+ 有效现金收款；
- 同一 room/month 最多一条 charge；
- 同一 room/month 最多命中一条 plan；
- 同一 tenant/month 最多对应一个 room，跨房间冲突由保存事务拒绝；
- 所有跨实体查询包含 `user_id`；
- 任意支付/现金/催收历史都会锁定相关月份；
- future 普通读取不写数据库；
- 页面只有房间详情可写计划。

### 完成定义

只有同时满足以下条件才能把 Trellis task 标记完成：

1. fresh database 迁移、重复迁移和非空库拒绝测试通过；
2. 定向、包级、全仓和 MySQL 测试均通过，或对与本任务无关的既有失败提供可复现证据；
3. 桌面和移动端完成实际页面审计；
4. `/tenancies` GET/POST 均 404；
5. 运行时代码无旧模型、旧表名、legacy fallback 和旧产品入口；
6. 首页可用 `view=tenants&status=outstanding&period=YYYY-MM` 找到全部未结清责任；
7. 租客详情展示房间级责任，修改入口跳向房间；
8. 房间详情可保存、替换、结束完整计划，且并发和锁定错误可解释；
9. README 已明确重建数据库与代码回滚边界；
10. `git status` 中无误提交的 Excel、数据库文件、日志或本地缓存。

## 13. Agent 交接模板

每个阶段结束时，交接信息必须使用以下格式，避免下一位 Agent 重新猜测：

```text
阶段：N / 名称
完成内容：
- ...

修改文件：
- ...

关键决策：
- ...

验证：
- 命令：...
- 结果：PASS / FAIL
- 若 FAIL，说明是否为阶段 0 已记录的既有失败

数据库：
- 使用的是否为 disposable DB：是/否
- schema_migrations 最新版本：...

未解决问题：
- ...

下一阶段注意：
- ...

提交：
- branch / commit SHA
```

任何 Agent 若发现必须改变以下内容，应先回到 `prd.md` / `design.md` 提交设计变更，不得直接在代码里偏航：唯一写入口、整月时间线、责任合计、事实锁定、同一租客同月唯一房间、责任人与付款人分离、无旧数据兼容。
