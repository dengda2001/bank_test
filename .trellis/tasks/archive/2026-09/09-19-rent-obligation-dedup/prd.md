# 租金义务重复生成修复

## Goal

`rent_obligations` 在每次页面加载时被重复插入，同一 `(租户, 月份)` 累积多条 `record_status='active'` 的行。
读路径不做任何去重，这些重复行被直接求和，导致月度总览的应收金额被成倍放大——实测单个租户单月显示
¥90,300，而真实月租是 ¥1,050（86 倍）。

修复后：`(user_id, tenant_id, period_month)` 的惰性义务至多存在一条，任意次数页面加载不再改变数据。

## Confirmed Facts（来自 `research/`，均为实测或逐行读码）

- 迁移 `001_initial_mysql.sql:70` 建立了 `UNIQUE KEY idx_rent_obligations_user_tenant_period (user_id, tenant_id, period_month)`。
- 迁移 `009_landlord_rent_model.sql:92` **删掉**了这个键，换成 `UNIQUE KEY idx_rent_obligations_user_charge_tenant (user_id, rent_charge_id, tenant_id)`。
- `obligations.go:201-204` 的 `ensureMonthlyObligations` 仍在用**已被删除的键**做 `OnConflict{DoNothing}`。
- 该函数从不写 `rent_charge_id`（实测 248/248 行为 `NULL`），而 MySQL 唯一索引中 NULL 互不相等，
  于是 `(user_id, NULL, tenant_id)` 永不冲突 → `ON DUPLICATE KEY UPDATE` 退化成普通 INSERT。
- 触发点只有三个 HTTP GET：`/rent-dashboard`（`dashboard.go:108`）、`/tenants`（`main.go:1049`，6 个月窗口）、
  `/tenants/{id}`（`tenant_detail.go:333`）。催收抽屉搭在 dashboard 行上。
- 新的 charge 写入路径 `ensureRentCharge`（`landlord_rent_ledger.go:244`）**没有任何非测试调用者**，
  `rent_charges` 表实测 0 行 → 惰性路径是唯一的义务生产者，且它本身已是幂等的（`landlord_rent_ledger_test.go:181-241`）。
- 引用 `rent_obligations` 的三张表全部 `ON DELETE CASCADE`：`payment_allocations`、`cash_receipts`、
  `dunning_send_attempts`（约束名 `fk_dunning_attempts_obligation`，表名是 `dunning_send_attempts`）。
  裸删会静默摧毁还款历史。

## Requirements

- **R1 写入幂等**：`(user_id, tenant_id, period_month)` 的惰性义务至多一条，重复调用为 no-op。
- **R2 不阻塞 charge 路径**：charge 路径按**房间**建义务（一个房间一次），而同一租客同月可能在两个房间
  都是当事人。任何新约束不得让 charge 路径的普通 `Create`（`landlord_rent_ledger.go:366`，无 OnConflict）
  撞唯一键失败。
- **R3 存量数据修复**：修复已有重复，且不得丢失 `payment_allocations` / `cash_receipts` / `dunning_send_attempts`。
- **R4 迁移自足**：修复必须写在迁移里，使得任何环境（dev / 审计 / 未来生产）跑完迁移即自愈，
  不能依赖人工先清库——否则 `ADD UNIQUE KEY` 会在有重复的库上失败、应用起不来。
- **R5 不编辑迁移 009**：`landlord_rent_migration_test.go:39-56` 逐字断言 009 含 `drop index idx_rent_obligations_user_tenant_period`。

## Acceptance Criteria

- [x] 连续多次加载 `/rent-dashboard`、`/tenants`、`/tenants/{id}`，每个 `(tenant, month)` 始终只有 1 行 active 义务（用 SQL 计数验证，加载前后相等）
      —— 5 轮 × 3 页面，义务数恒为 9，重复组 0（`implement.md` 实测表）
- [x] 在**带重复数据**的库上执行迁移成功；执行后不存在任何 `(user_id, tenant_id, period_month)` 重复组
      —— 248 行 / 9 组 → 9 行 / 0 组；另在全新空库上从 0 跑通 13 条迁移
- [x] 迁移前后 `payment_allocations` / `cash_receipts` / `dunning_send_attempts` 的行数与归属不丢失（remap 后仍指向同组幸存行）
      —— 4 条分配全保留；催缴同 key 撞键收敛到幸存行；边界库无孤儿引用
- [x] charge 路径不回归：`TestRentLedgerServiceCreatesOneChargeAndStableObligationsOnMySQL` 通过，且同一 `(tenant, month)` 可容纳多条 charge 义务
      —— 实测两条 charge 行（1397/1398）共存
- [x] `go test ./...` 与 `go vet ./...` 通过（含 `RENTOPS_MYSQL_TEST_DSN` 下的 MySQL 测试）
      —— 另：HEAD 上 5 条 MySQL 用例失败，本改动后仅剩 1 条，且该条在 HEAD 上同样失败（`design.md` §「既有失败用例的定性」）
- [ ] **`scripts/run-audit-local.sh` 跑通 —— 未验证。**
      该脚本的管理连接硬编码 `sudo mysql`（`scripts/run-audit-local.sh:58-60`），不认免密 root、也不接受
      `MYSQL_PORT` 覆盖，本机无 sudo 终端故无法执行。以真实 HTTP 反复加载三个触发页面作为替代验证
      （见第 1 条）。**这不是通过，只是未测**；在有 sudo 的机器上应补跑一次。

## Out of Scope

- 把 charge 路径接到路由上、或让惰性路径改走 charge（研究里的 option b/d）——那是独立的产品决策，
  前提是每个可计租租客都有租约与当事人，惰性路径刻意不要求这些。
- 读路径加 `Group`/`DISTINCT` 兜底。修复后不再需要；若将来要加，属于防御性改动而非本任务验收项。
- 修复 `scripts/audit/README.md:110-118` 关于 `run-audit-local.sh`「尚未实现」的过期描述。

## Notes

- 本任务由 `.trellis/tasks/09-19-desktop-contract-1100/implement.md` 的「阻塞记录 B2」派生。
- `scripts/audit/seed.mjs` 的解析器修复（属性顺序无关的正则）是本任务的连带产物，已改未提交。
