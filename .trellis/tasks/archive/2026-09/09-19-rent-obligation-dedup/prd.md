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
- [ ] **`scripts/run-audit-local.sh` 跑通 —— 已实测，失败；且与本改动无关。**

      本节原先写「未验证，因为脚本硬编码 `sudo mysql`」——**那个判断是错的**。脚本的 admin 连接
      是三级降级（`run-audit-local.sh:115-134`）：显式 `MYSQL_ADMIN_CMD` → TCP 免密 root → `sudo mysql`。
      第二级正是本机在用的连接，之所以掉到第三级只因 `MYSQL_PORT` 默认值是 `53306`。改用
      `MYSQL_PORT=3306 APP_PORT=18090 scripts/run-audit-local.sh` 即可运行（真实退出码 **1**）。

      失败点是 seeder 的 `一键匹配` 断言（`scripts/audit/seed.mjs:433`）。**根因不是义务重复**：

      - 本次运行义务为 **18 条、零重复**（`116 → 18` 的修复已生效），断言依旧失败。
      - `matching.go:94`：流水未解析出月份 → 只能返回 `candidate`（渲染「请确认租金月份」），拿不到
        `CanConfirm`。三笔候选里 `audit-tx-candidate-priya` 的描述本就没有月份；唯一同时具备
        「已记住付款人 + 解析出 2026-09 + 金额 70000 恰等于义务 16 应收」的是 MICHAEL 那笔，
        而 seeder 把它标成了 `ignored`。
      - A/B 实测：保持现状 → `一键匹配` 渲染 **0** 次；把该笔改回 `unmatched` → 渲染 **1** 次。

      该矛盾在 HEAD 上原样存在，也不在 `seed.mjs` 的未提交改动里（改的是解析器正则）。

      → 结论：013 是必要修复，但**不足以**解除 B2；`09-19-desktop-contract-1100/implement.md:63-72`
      把「一键匹配失败」归因于义务重复，该因果链不成立，需另行更正。

## Out of Scope

- 把 charge 路径接到路由上、或让惰性路径改走 charge（研究里的 option b/d）——那是独立的产品决策，
  前提是每个可计租租客都有租约与当事人，惰性路径刻意不要求这些。
- 读路径加 `Group`/`DISTINCT` 兜底。修复后不再需要；若将来要加，属于防御性改动而非本任务验收项。
- 修复 `scripts/audit/README.md:110-118` 关于 `run-audit-local.sh`「尚未实现」的过期描述。

## Notes

- 本任务由 `.trellis/tasks/09-19-desktop-contract-1100/implement.md` 的「阻塞记录 B2」派生。
- `scripts/audit/seed.mjs` 的解析器修复（属性顺序无关的正则）是本任务的连带产物，已改未提交。
