# DAO / Repository 层：技术设计

## 1. 目标与边界

本任务建立房东租金领域的用户隔离数据访问边界，供后续核心账务服务和 Dashboard 查询复用。

Repository 负责：

- 对房产、房间、租约、租约参与人、月度房间总应收、租客责任、付款分配和支出提供 typed 查询/写入方法。
- 每个方法显式接收当前 `userID`，并在查询条件中使用所有权过滤。
- 创建关联对象前验证父对象、租客和房产/房间属于同一用户，失败时返回安全的 `gorm.ErrRecordNotFound` 或校验错误。
- 对月份、房产、房间、租客和责任范围提供稳定过滤。

Repository 不负责：

- 租约重叠、责任合计、付款覆盖和账务状态等业务规则。
- 跨多个写入步骤的业务事务；后续账务服务通过已有 `*gorm.DB` 事务边界协调这些操作。
- HTTP、模板、JSON 兼容层或通用的反射式 CRUD 抽象。

## 2. 组件与数据流

```text
HTTP / Service
      │ userID + typed filter/input
      ▼
landlordRentRepository
      │ explicit user predicates + relationship ownership checks
      ▼
GORM / MySQL tables
```

实现放在 `cmd/truelayer-demo/landlord_rent_repository.go`，与当前单包后端的 model/service 文件保持一致。方法保持未导出，后续同包账务服务直接依赖该稳定边界。

## 3. 查询契约

Repository 提供以下查询族：

- 房产：按 ID 查询、按状态列表。
- 房间：按 ID 查询、按房产/状态列表。
- 租约：按 ID 查询、按房间和日期范围列表。
- 参与人：按租约查询、按租客查询；结果可按有效状态和日期继续由服务层解释。
- 月度总应收：按月份、房产、房间和租约过滤，默认只返回 active 事实。
- 租客责任：按月度总应收、房产、房间、租客和月份过滤；房产/房间条件通过 `rent_charges` 关联，并在 join 上同时约束 `user_id`。
- 付款分配：按责任或付款交易查询，返回当前用户的行。
- 支出：按房产、房间、日期范围和状态查询；公共房产支出允许 `room_id` 为空。

列表方法返回非 nil 的空切片；不存在目标的单行查询返回 `gorm.ErrRecordNotFound`。`userID == 0` 在访问数据库前返回错误。

## 4. 写入契约

写入方法的 `userID` 是唯一可信的所有权来源，传入 model 的 `UserID` 会被覆盖为当前用户，避免调用方伪造归属。

- `createProperty` 只写入当前用户的房产。
- `createRoom` 验证 `propertyID` 属于当前用户。
- `createTenancyAgreement` 验证房间属于当前用户。
- `createAgreementParty` 验证租约和租客均属于当前用户。
- `createRentCharge` 验证房产、房间和租约属于当前用户，且房间确实属于指定房产、租约确实属于指定房间。
- `createRentObligation` 验证租客和可选月度总应收属于当前用户。
- `createManualExpense` 验证可选房产/房间属于当前用户；同时提供房产和房间时验证房间属于该房产。

付款分配写入仍由现有交易服务负责原子分配和锁定；本任务只提供用户隔离读取，避免复制一套账务事务逻辑。

## 5. 关联与安全策略

单列外键不能保证关联对象的 `user_id` 一致，因此不能只按 ID 查询后再信任结果。所有关联读取均使用类似以下条件：

```sql
JOIN rooms r
  ON r.id = rent_charges.room_id
 AND r.user_id = rent_charges.user_id
WHERE rent_charges.user_id = ?
```

创建关联对象时先用相同的用户条件读取父对象，再执行写入。跨用户目标表现为不存在，不泄露目标是否存在。

## 6. 测试设计

使用项目现有 opt-in MySQL 测试约定和真实 migration runner，不引入 SQLite 或 mock ORM：

- owner A 只能读到自己的房产、房间、租约、责任、付款分配和支出。
- owner A 查询 owner B 的 ID 返回 `gorm.ErrRecordNotFound` 或空列表。
- 房产/房间/租约/租客/总应收的跨用户创建被拒绝且不落库。
- 月份、房产、房间和租客过滤只返回匹配事实。
- 缺少关联、空列表、不存在目标和 `userID == 0` 有稳定结果。
- 同一数据库运行 migration 两次后执行上述查询，确保 Repository 使用的表结构与迁移一致。
