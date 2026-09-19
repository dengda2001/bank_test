# 技术设计：租金义务去重

## 0. 这是恢复成文契约，不是新设计

`.trellis/spec/backend/database-guidelines.md` 的 **Tenant Billing History Projection** 一节已经写死了本 bug 违反的不变量：

- `:427-428`「Each eligible tenant/month has at most one `rent_obligations` parent
  because `(user_id, tenant_id, period_month)` is unique.」
- `:446-448`「Repeated GET/refresh -> obligation generation remains idempotent
  **through the existing unique key**; no payment allocation or transaction rows are written.」

迁移 009 删掉了那个键，代码却还按它写。所以本任务**不是引入新的唯一性语义，而是让实现重新符合已批准的 spec**。
实现完成后无需改这条 spec —— 它本来就是对的；需要的是让代码追上它。

（该 spec 段落同时是本任务的验收依据：`at most one parent per tenant/month` 就是验收项 1。）

## 1. 核心决策：用生成列做「部分唯一索引」

### 问题

需要同时满足 R1（惰性行每 `(租户, 月)` 至多一条）与 R2（charge 行不设此限制）。MySQL 没有部分索引，
而研究建议的 option (a)（直接恢复 `UNIQUE (user_id, tenant_id, period_month)`）会把 R2 打破：
charge 路径按房间建义务，一个租客同月出现在两个房间就会撞键，而那条路径用的是普通 `Create`，
会直接报错而非静默去重。

### 方案

MySQL 唯一索引中 NULL 互不相等。用一个**存储生成列**把「惰性」编码成可索引的值：

```sql
ALTER TABLE rent_obligations
  ADD COLUMN lazy_period_month date
    GENERATED ALWAYS AS (CASE WHEN rent_charge_id IS NULL THEN period_month ELSE NULL END) STORED,
  ADD UNIQUE KEY idx_rent_obligations_lazy_tenant_period (user_id, tenant_id, lazy_period_month);
```

- 惰性行（`rent_charge_id IS NULL`）：`lazy_period_month = period_month` → 每个 `(user, tenant, month)` 唯一。
- charge 行：`lazy_period_month = NULL` → 互不相等，本键不施加任何约束；由 009 的
  `idx_rent_obligations_user_charge_tenant` 继续管辖。

**已在 MySQL 8.4.11 实测**（scratch 库，验完即删）：

- 同 `(user, tenant, month)` 插第二条惰性行 → `ERROR 1062 Duplicate entry '1-1-2026-09-01' for key 'uq_lazy'` ✅
- 同 `(user, tenant, month)` 插两条不同 `rent_charge_id` 的行 → 两条都成功 ✅

### 为什么不选其它写法

| 方案 | 否决理由 |
|---|---|
| 直接恢复 `(user_id, tenant_id, period_month)`（研究 option a） | 打破 R2，charge 路径接线后即炸 |
| `OnConflict` 换成显式 `SELECT` 预检（option c） | 无唯一键兜底时是竞态的：两个并发页面加载可以同时查不到再同时插入。可作为补充，不能作为唯一手段 |
| 惰性路径改写 `rent_charge_id`（option b） | 要求每个可计租租客都有租约+当事人，惰性路径刻意不要求；且会让这些行进入 charge 视角的页面（`transaction_detail.go:204`、`rent_workspace.go:433`） |
| 退役惰性路径（option d） | charge 路径无生产调用者，退役后产品**没有任何义务写入者**，仪表盘直接空 |

## 2. 迁移 013：去重 + 建键

新迁移 `migrations/013_dedupe_rent_obligations.sql`（不编辑 009，见 R5）。顺序：

### 2.1 选幸存行

每组 `(user_id, tenant_id, period_month)` 选一行保留。规则按「持有状态」优先：

1. 被 `payment_allocations` / `cash_receipts` / `dunning_send_attempts` 引用的行
2. 否则 `record_status = 'active'` 优先于 `'voided'`
3. 否则 `paid_amount_cents > 0` 的行
4. 否则 id 最小

理由：删掉被引用的行会触发 CASCADE 静默摧毁还款记录；优先保留有状态的行可以让 remap 的工作量最小。

第 2 条解决的是原先的待确认项。`voidFutureTenantObligations`（`tenants.go:518-529`）的 void 是
**集合式 UPDATE**，带 `record_status='active'` 条件；一组内既有 active 又有 voided，说明 void 只
命中了一部分（正是本 bug 的症状）。若不加这条，幸存行会按 min(id) 落到已被作废的那行上，把整个
`(租户, 月)` 的账单标记成 voided。加了之后混合组保留 active 行；整组都已 voided 时（例如本机
scratch 的 2026-03 组）才落到 voided 行上，此时回填出的 `status='voided'` 也正是应有的语义。

### 2.2 remap 引用

把将被删除行上的引用改写到幸存行：

```sql
UPDATE payment_allocations pa
  JOIN rent_obligations o ON o.id = pa.rent_obligation_id
  JOIN _mig013_keep k ON k.user_id = o.user_id AND k.tenant_id = o.tenant_id
                     AND k.period_month = o.period_month
  SET pa.rent_obligation_id = k.obligation_id
WHERE o.rent_charge_id IS NULL AND o.id <> k.obligation_id;
```

`cash_receipts`、`dunning_send_attempts` 同构。

**碰撞处理**（原设计写错了，此处是实测更正）：原稿引用
`UNIQUE KEY idx_payment_allocations_tx_obligation (user_id, payment_transaction_id, rent_obligation_id)`，
但那个键在 **003** 就被 `DROP INDEX` 换成了 `(user_id, idempotency_key)`（`003_rent_ledger_foundation.sql:10-11`）。
remap 不改 `idempotency_key`，所以 `payment_allocations` 本身**不会**因 remap 撞键。

真正会撞键的是 **`dunning_send_attempts`**：它的
`UNIQUE KEY idx_dunning_attempts_user_request_obligation (user_id, request_key, rent_obligation_id)`
包含 `rent_obligation_id`。同一 `(user, request_key)` 在幸存行和待删行上各有一条时，remap 必然失败。
必须在 remap **之前**删掉待删行上会碰撞的那条：

```sql
DELETE da FROM dunning_send_attempts da
  JOIN rent_obligations o ON o.id = da.rent_obligation_id
  JOIN _mig013_keep k ON k.user_id = o.user_id AND k.tenant_id = o.tenant_id
                     AND k.period_month = o.period_month
  JOIN dunning_send_attempts keep
    ON keep.user_id = da.user_id AND keep.request_key = da.request_key
   AND keep.rent_obligation_id = k.obligation_id
WHERE o.rent_charge_id IS NULL AND o.id <> k.obligation_id;
```

`cash_receipts` 同理，它有三个含自然键的唯一索引，任一被 remap 撞到都会中止迁移：
`(user_id, receipt_number)`、`(user_id, idempotency_key)`、`(user_id, payment_transaction_id)`。
`payment_allocations` 也保留了 `(user_id, idempotency_key)` 的同类删除，因为任意数据上都可能撞。

这些删除**只**删除「某个唯一键已经声明它与幸存行上的某行是同一逻辑记录」的行，因此删掉的是重复项而非历史。

### 2.2b 迁移必须绕开执行器的两个限制

`cmd/truelayer-demo/db.go:137-148` 的 `splitSQLStatements` 按 `;` 朴素切分，并且
**丢弃任何 trim 后以 `--` 开头的片段**。后果：

- **迁移文件里不能有任何 `--` 行注释。** 一条注释行会和它下面的语句落在同一个片段里，
  导致**那条语句被静默跳过** —— 不报错、不打日志。013 因此零注释（001–012 同样是零注释，
  说明这是既成惯例而非我的新规矩）。`rent_obligation_dedup_migration_test.go` 里有守卫用例。
- **`;` 在注释里同样致命。** 切分发生在词法分析之前，所以注释里的分号照样切断文件。
  013 的第一版就在说明这条规则的段首注释里写了一个 `;`，结果注释被切成两半、前半个片段是
  未闭合的 `/*`，迁移以 `Error 1064 ... near '/* 013_dedupe_rent_obligations'` 失败。
  结论是注释一律不写，规则写进本文件。
- 语句体内不能出现 `;`，所以 `CREATE TEMPORARY TABLE`、存储过程、`SIGNAL` 自检都不可用。
  改用一张**普通辅助表** `_mig013_keep`（迁移开头 `DROP TABLE IF EXISTS` 保证可重入，末尾清理），
  自检则**由 `ALTER TABLE ... ADD UNIQUE KEY` 本身承担**：只要还有重复组残留，它就会以
  `ER_DUP_ENTRY` 中止迁移。

DDL 会隐式提交，所以 `ALTER TABLE` 之后事务边界失效。把它放在**最后一条数据变更语句之后**：
即使它失败，前面的清理已提交，而清理是幂等的（`_mig013_keep` 每次重建），重跑即可。

### 2.3 重算幸存行的已收金额（实测后升级为必需项）

幸存行的 `paid_amount_cents` / `status` 按 remap 后的引用重算。口径对齐两个投影函数：

- `ledgerPaidAmount`（`ledger.go:109-117`）：只计 `status='confirmed'` 且 `allocation_kind` 为
  `'rent'` 或空串的分配（`ledgerAllocationIsEffective` + `ledgerAllocationKind`）。
- `projectRentObligation`（`cash_receipts.go:121-137`）：再加上 `cash_receipts`，条件是
  `status='confirmed'`、`amount_cents>0`、币种等于义务币种且规范化后为 EUR、`tenant_id` 相同。
- `ledgerObligationStatus`（`ledger.go:119-139`）推导 status，优先级为
  `needs_review` > `voided` > `paid` > `partial` > `overdue` > `open`。

**为什么这一步是承重的，而不是锦上添花**：`paid_amount_cents` 是一个**只对自己那次写入的服务
目标**做刷新的反规范化缓存：`allocateTransaction`（`transaction_allocation.go:242-245`）、
`recordCashReceipt` 与作废路径（`cash_receipts.go:269-274,330-333`）、
`transaction_actions.go:282-285` 都会为自己操作的那一条义务重算这一列（**设计初稿此处的
「没有任何生产代码路径写这一列」是错的**，check 阶段更正）；但
`ensureMonthlyObligations` 插入新惰性行时一律置 0（`obligations.go:195`），而
`summarizeRentDashboardWithFilters`（`obligations.go:352-390`）是**裸 `Find` 后直接对该列求和**
（`summary.PaidCents += obligation.PaidAmountCents`），不经过任何投影。

所以：remap 把分配搬到幸存行之后，若不同步回填，该行的缓存仍是 0，
**仪表盘的「已收」会归零**——还款记录虽在库里，页面上却看不见。这正是 R3 要防的「丢失还款历史」。
回填覆盖**全部惰性行**（不只发生过重复的组），因为惰性写入器对每一行都写 0，
任何有分配的惰性行都是陈旧的。

一个由此暴露的独立缺陷（不在本任务范围，见 §6）：charge 路径（`landlord_rent_ledger.go:343-354`）
同样不写 `paid_amount_cents`，所以 charge 行的同一个 bug 依然存在。

**status 里的欧洲时区**：`ledgerObligationStatus` 按 Europe/Dublin 的日历日比较
（`nowLocal.Year()/YearDay()` 对 `dueLocal`），而本机 MySQL **时区表未加载**，
`CONVERT_TZ(...,'Europe/Dublin')` 返回 NULL，且 `@@system_time_zone=CST` 与都柏林差 7 小时
（用 `CURDATE()` 会有约 7 小时/天算错一天）。迁移因此内联爱尔兰夏令时规则：
夏季 = 3 月最后一个周日 01:00 UTC 至 10 月最后一个周日 01:00 UTC，落在区间内则在 UTC 上 +1 小时
再取日期。已在 MySQL 8.4.11 上逐点验证（含 2026-03-29 / 2026-10-25 两个切换瞬间）。

### 2.4 删重复行

删掉 `ro_dupes` 中除幸存行外的全部行。此时引用已清空，CASCADE 不再有可摧毁的对象。

### 2.5 建键

执行 §1 的 `ALTER TABLE`。

### 2.6 自检

迁移末尾断言无重复组，否则中止：

```sql
SELECT COUNT(*) INTO @dupes FROM (
  SELECT 1 FROM rent_obligations
  WHERE rent_charge_id IS NULL
  GROUP BY user_id, tenant_id, period_month HAVING COUNT(*) > 1
) x;
-- @dupes > 0 时 SIGNAL SQLSTATE '45000'
```

## 3. Go 侧改动

`obligations.go:201-204` 的 `OnConflict{Columns: user_id, tenant_id, period_month}` 现在指向一个**真实存在**的键
（生成列参与，但 MySQL 的 `ON DUPLICATE KEY UPDATE` 不关心是哪条键命中）。

**必须先实测 GORM 实际发出的 SQL**（gorm.io/driver/mysql 对 `OnConflict{DoNothing:true}` 的编译产物），
确认：

- 发出的语句在第二次调用时确实是 no-op；
- `Columns` 里列出的列不会让 GORM 尝试写生成列 `lazy_period_month`。

若 GORM 的行为依赖 `Columns`，则改为显式列出与生成列对应的语义（或退化为 `INSERT ... ON DUPLICATE KEY UPDATE id=id`）。
`rentObligation` 结构体**不要**新增 `LazyPeriodMonth` 字段，否则 GORM 会尝试 INSERT 生成列。

## 4. 数据修复的独立性与可回滚性

- 迁移自带去重，所以**不需要**先手工清库（R4）。这是刻意的：审计库、dev 库、未来的生产库都可能有重复。
- 回滚：`DROP INDEX idx_rent_obligations_lazy_tenant_period` + `DROP COLUMN lazy_period_month` 可恢复 schema。
  但**数据去重不可逆**——执行前必须先 dump。本机 dev 库的备份在 `/tmp/rentops-backup-20260919-205534.sql`
  （清理前，含 248 条重复行，正好可作为迁移的回归样本）。
  → 实施时应把这份脏数据抽成一个**迁移测试用例**（插入重复 → 跑迁移 → 断言无重复组且引用未丢），
  这样回归样本进入版本库，不依赖 /tmp。

## 5. 影响面

| 改动 | 影响 |
|---|---|
| 迁移 013 | 新增文件；`landlord_rent_migration_test.go` 只读 009，不受影响 |
| `cmd/truelayer-demo/rent_obligation_dedup_migration_test.go` | 新增；5 条静态用例，含分号切分器守卫 |
| `obligations.go` 冲突子句 | **不改**。GORM 的 `OnConflict{DoNothing:true}` 编译成 `ON DUPLICATE KEY UPDATE \`id\`=\`id\``，`Columns` 字段被完全忽略（`gorm.io/driver/mysql@v1.5.2/mysql.go:197-234`），所以键一存在它就自动成为真 no-op |
| `obligations.go:24` `rentObligation` 结构体 | **不要**加 `LazyPeriodMonth` 字段，否则 GORM 会尝试 INSERT 生成列 |
| e2e `validateE2ECleanupObligations`（`cleanup.go:350-375`） | 期望恰好 3 条义务并拒绝重复键。修复后应转绿——这本身是一条验证 |
| 读路径 | 不改 |
| `009_landlord_rent_model.sql` | 不动（R5）。`landlord_rent_migration_test.go` 逐字断言它的文本 |

### 本机跑不了 `scripts/run-e2e-local.sh`

它的 admin 调用是硬编码的 `sudo mysql`（`scripts/run-e2e-local.sh:58-60`），不认免密 root，
也不接受 `MYSQL_PORT` 覆盖管理连接。本机没有 sudo 终端，因此该套件无法在本地跑通。
替代验证见 `implement.md`：用真实 HTTP 反复加载三个触发页面，断言义务数不增长。

## 6. 已知残留风险

- **charge 行的 `paid_amount_cents` 仍然陈旧。** 迁移只回填惰性行（`rent_charge_id IS NULL`）。
  charge 路径（`landlord_rent_ledger.go:343-354`）同样不写这个缓存，所以那批行有和惰性行一样的毛病。
  目前 charge 路径没有生产调用者，故不在本任务范围；接线时必须一并处理，否则仪表盘的
  已收/结余对 charge 行仍然是错的。
- **`_mig013_keep` 是全局命名的普通表。** 两个实例同时对同一个库启动迁移会互相踩。
  这是执行器本身的性质（`schema_migrations` 没有加锁，任何迁移都有此问题），本迁移没有让它变新；
  但辅助表让「同时跑」的冲突面从 DDL 扩大到了 DML。生产是单实例启动，可接受。
- **`ALTER TABLE` 成功、但 `schema_migrations` 写入前进程被杀** → 重跑会因列已存在而失败。
  MySQL 不支持 `ADD COLUMN IF NOT EXISTS`，无法用纯 SQL 消除这个窗口。窗口很窄（一条 INSERT 的宽度），
  真要恢复就手工删列删键后重跑。
  **已在空库上实测这个窗口**：手工重放 013 于 `ALTER TABLE` 处报
  `ERROR 1060 (42S21) ... Duplicate column name 'lazy_period_month'`（line 133）。
  中断后库里只多出一张 `_mig013_keep`（`DROP` 还没执行到），数据无损、`schema_migrations` 完好，
  重跑前只需 `DROP TABLE IF EXISTS _mig013_keep` 并删列删键。正常路径不受影响：
  迁移记录已写入时，runner 直接跳过 013。
- 「同一租客同月两个房间」的 charge 语义已实测通过（两条 charge 行共存），但 009 的
  `idx_rent_obligations_user_charge_tenant` 对「一租客多房间」是否足够，仍要等 charge 路径接线时确认。
