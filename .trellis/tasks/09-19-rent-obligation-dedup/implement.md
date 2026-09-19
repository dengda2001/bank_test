# 执行计划：租金义务去重

## 执行顺序

1. [x] dump 带重复的库 → `/tmp/rentops-backup-20260919-205534.sql`（248 条义务、9 个重复组）
2. [x] 重复样本进版本库。**偏离原计划**：`go test` 里没有数据库（既有 `*_migration_test.go` 全是
       读 .sql 做文本断言），所以回归样本落成两层 ——
       `rent_obligation_dedup_migration_test.go` 的 5 条静态用例守卫迁移内容与切分器陷阱；
       真实「插重复 → 跑迁移 → 断言」，跑在本机 `rentops_mig013_test` / `rentops_pageload` 上
3. [x] 实测 GORM `OnConflict{DoNothing}` 的编译产物 → **`Columns` 被完全忽略**，发出的是
       `ON DUPLICATE KEY UPDATE \`id\`=\`id\``。结论：Go 侧零改动（见 design.md §3/§5）
4. [x] 写 `migrations/013_dedupe_rent_obligations.sql`。相对 design.md 初稿的**三处更正**：
       remap 会撞的是 `dunning_send_attempts` 而非 `payment_allocations`（后者的键在 003 已被换掉）；
       幸存规则加 `record_status` 平局判定；文件必须**零注释**（`;` 在注释里也会切断文件）
5. [x] 在带重复的库上跑迁移 —— 用**真实 app runner**（`MYSQL_DSN` 指向测试库启动二进制），
       不是 mysql CLI，以确保分号切分器被真实路径覆盖
6. [x] 无需调整 `obligations.go:201-204`
7. [x] 起一次性实例，`/rent-dashboard`、`/tenants`、`/tenants/1` 各加载 5 轮，义务数不增长
8. [ ] **`scripts/run-e2e-local.sh` 本机跑不了** —— 它硬编码 `sudo mysql` 做管理连接
       （`scripts/run-e2e-local.sh:58-60`），不认免密 root，也没有 sudo 终端。
       第 7 步是它的替代验证。在能 sudo 的机器上应补跑一次
9. [x] `go build ./... && go vet ./...` 干净；`go test ./... -count=1` 全绿
10. [x] spec 更新 —— check 阶段在 `database-guidelines.md:85` 新增
       `## Scenario: Migration Authoring with the Flat SQL Runner`，固化执行器的
       `--`/`;` 陷阱、`ALTER TABLE` 置尾、唯一性声明在 SQL 而非 `AutoMigrate`、不得编辑既有迁移
11. [ ] 提交（待用户发话）

## 验证命令（实测可用的那几条）

```bash
go build ./... && go vet ./...
go test ./... -count=1

# 静态用例逐条
go test ./cmd/truelayer-demo/ -count=1 -run 'Dedupe' -v
```

端到端（本机已验证的形态）：

```bash
# 1. 从原始脏库复制一份可丢弃的库
mysql -u root -e "DROP DATABASE IF EXISTS rentops_pageload; CREATE DATABASE rentops_pageload"
mysqldump -u root --single-transaction rentops_dedup_scratch | mysql -u root rentops_pageload

# 2. 用真实 runner 跑迁移（app 启动即执行）
MYSQL_DSN='root@tcp(127.0.0.1:3306)/rentops_pageload?charset=utf8mb4&parseTime=True&loc=UTC' \
MIGRATIONS_DIR=migrations TL_ADDR=':18098' \
TL_ENV=sandbox TL_CLIENT_ID=dummy TL_CLIENT_SECRET=dummy \
TL_REDIRECT_URI='http://localhost:18098/callback' \
BANK_TOKEN_ENCRYPTION_KEY=0123456789abcdef0123456789abcdef \
APP_ADMIN_USERNAME=ddrzh APP_ADMIN_PASSWORD=ddrzh512 \
go run ./cmd/truelayer-demo &
# /tmp/pageload-test.sh 把 1、2 和下面的断言串在一起

# 3. 登录后反复加载三个触发页面，断言义务数不增长
curl -c jar -X POST -d 'username=ddrzh&password=ddrzh512' http://127.0.0.1:18098/login-local
for i in 1 2 3 4 5; do
  curl -b jar -o /dev/null http://127.0.0.1:18098/rent-dashboard
  curl -b jar -o /dev/null http://127.0.0.1:18098/tenants
  curl -b jar -o /dev/null http://127.0.0.1:18098/tenants/1
done
```

核心不变量（任何时刻都必须为空）：

```sql
SELECT user_id, tenant_id, period_month, COUNT(*) c
FROM rent_obligations WHERE rent_charge_id IS NULL
GROUP BY 1,2,3 HAVING c > 1;
```

## 实测结果

| 断言 | 结果 |
|---|---|
| 原始脏库 248 行 → 迁移后 | **9 行**，重复组 9 → **0**，4 条分配全部保留 |
| 边界库 251 行（含 fixture） | 惰性行 10、charge 行 2；无孤儿引用 |
| 幸存规则：被引用行胜出 | 2026-06 组保留 id **1392**（被分配引用）而非 min(id) 130 ✅ |
| remap | 分配 #6 从 1394 改指 **112** ✅；催缴 `mig013-move` 同样改指 112 ✅ |
| 撞键删除 | 与幸存行同 `request_key` 的催缴 #2 被删，催缴数 3 → 2 ✅ |
| R2：charge 行不受限 | 同 `(租户, 2026-10)` 的两条 charge 行（1397/1398）共存 ✅ |
| 回填 paid/status | 2026-06 → 42000/partial；2026-08 → 50000/partial；2026-12 单例 → 105000/paid ✅ |
| voided 分支 | 整组作废的 2026-03 → status=`voided` ✅ |
| 都柏林时区 | 2026-09 due 2026-09-30 → `open`（未逾期）；2026-01..02 → `overdue` ✅ |
| R1 写入幂等 | `ON DUPLICATE KEY UPDATE \`id\`=\`id\`` 为 no-op；裸 INSERT 被 1062 拒绝 ✅ |
| 迁移失败可回滚 | 首次因注释中的 `;` 失败，库无任何残留（251 行、无辅助表、无新列、无迁移记录）✅ |
| **全新空库从 0 跑 13 条迁移** | 013 成功；生成列 + 唯一键就位，009 的键保留，helper 已清，义务 0 行 ✅ |
| 空库上二次启动 | runner 正确跳过已应用的 013（仍 13 条），无报错、无残留 ✅ |
| §6 窗口实测 | 手工重放 013 在 `ALTER TABLE` 报 `ERROR 1060 Duplicate column name`（**line 133**）；中断后仅残留 helper 表，无数据损坏、迁移记录完好 → 恢复 = `DROP TABLE _mig013_keep` + 删列删键后重跑 ✅ |

## check 阶段修正（缺陷真实，已独立复现）

`trellis-check` 报回 1 处**会导致迁移在生产库崩溃**的缺陷，以及 3 处描述失真。

**dunning 撞键预删覆盖不全**。原实现只删「已经坐在幸存行上的那条尝试」，但
「两条待删行各持一条同 `request_key` 的尝试、幸存行上一条都没有」时预删删 0 条，remap 随即撞键。
可达性成立：`dunningService.send` 对同一页所有义务用**同一个 `request_key`**，而重复行在页面上是独立行。

旧逻辑原样复现（scratch 库）：

```
旧预删删除数: 0
ERROR 1062 (23000): Duplicate entry '999-K-900030' for key
  'dunning_send_attempts.idx_dunning_attempts_user_request_obligation'
```

新逻辑（组内自连接，保留「幸存行上的那条，或其 id 最小者」）同一 fixture：预删 1 条 → remap 成功 →
收敛到恰好一条且指向幸存行。无环性已验算：两行比较时只有 id 大的被判 doomed。

三处描述更正：

- 初稿称「`paid_amount_cents` 没有任何生产代码路径写这一列」**是错的**，实有 4 处写入者
  （`transaction_allocation.go:242`、`cash_receipts.go:269,330`、`transaction_actions.go:282`）。
  结论不变：它们各自只刷新自己操作的那一条义务，remap 后幸存行多出别的行的分配，缓存必然偏低。
- 注释守卫从只拒 `--` 扩展到同时拒 `/*` 与 `*/`。
- 删除失效的 spec 行号引用，改用段落名。

## 既有失败用例的定性（HEAD 对照）

| | 全量 MySQL 用例失败数 |
|---|---|
| HEAD `7e2a30c`（无 013） | **5** |
| 本工作区（含 013） | **1** |

唯一残留的 `TestDunningDashboardHTTPWorkflowOnMySQL` 在 HEAD 干净 worktree 上同样失败，错误一致
（仪表盘渲染「暂无符合条件的房产」，用例却断言出现「邮件催缴」与租客邮箱）—— 系 `df8997b` 把仪表盘
改成房产树视角后该用例失效，与本改动无关。013 顺带修好了另外 4 条。

## 风险文件

| 文件 | 风险 | 处置 |
|---|---|---|
| `migrations/013_dedupe_rent_obligations.sql` | 去重不可逆；CASCADE 会静默删还款记录 | 先 remap 再删；末尾 `ALTER` 兼作自检 |
| 同上 | **注释里出现 `;` 或 `--` 会静默丢弃语句/切断文件** | 迁移零注释；静态用例守卫 |
| `cmd/truelayer-demo/obligations.go` | 11 个测试依赖惰性路径 | 本次未改；若将来要改需跑 `run-mysql-test-clean.sh` |
| `landlord_rent_migration_test.go` | 逐字断言 009 文本 | **不要**编辑 009，只加新迁移 |

## 开工前检查

- [x] 迁移 013 的编号未被占用（当时最新是 012）
- [x] GORM driver 对 `OnConflict{DoNothing}` 的编译行为已实测（design.md §3）
- [x] `/tmp/rentops-backup-20260919-205534.sql` 存在
