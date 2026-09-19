# 执行计划：Rosewood 数据灌入

## 前置

**`09-19-rent-obligation-dedup` 必须先完成。** 否则灌完后每次加载页面都重复生成义务，
验收项「每个 `(tenant, month)` 恰好 1 行」不成立，页面金额被放大。

```bash
python3 ./.trellis/scripts/task.py current   # 确认当前任务
```

## 执行顺序

1. [x] 把 `/tmp/xlsx/parse.py` + `/tmp/xlsx/extract.py` 整理成 `test-data/rosewood/extract.py` 并提交
2. [x] 跑提取，产出 `test-data/rosewood/*.json`（含 `seed-data.sql` / `seed-match.sql`）
3. [x] 过 `review.json`：归一后的姓名、异常行、金额为空的记录（32 个房产-月的
   `源表合计 = 建模应收 + 占位行 + 折叠差额` 全部残差为 0）
4. [x] 写 `scripts/seed-rosewood.sh`（§5 的两阶段：`data` / `match`）
5. [x] 灌 `data` 阶段，核对 4 / 25 / 58 的行数
6. [x] 起本机 app（`:18097`），访问 `/rent-dashboard` 触发义务生成（该次启动同时把迁移 013
   应用到本机 `rentops`，此前它只有 `001`–`012`）
7. [x] 灌 `match` 阶段，核对义务无重复、账单页出现三种收款状态
8. [x] 跑验收清单（见下方「验收结果」）
9. [x] 更新 `.trellis/spec/`：新增「Room-Centric Rent Workspace Read Model」（记录 charge 门控
   与 `/bills` 的分流，并修正原来把义务视角记成 `/rent-dashboard` 读模型的那节）与
   「Local Test Data Seeding」（本步骤的字面交付物）

## 验证命令

```bash
# 起本机实例（一次性、可丢弃）
scripts/run-audit-local.sh          # 或直接跑 dev app

# 行数核对
mysql -h 127.0.0.1 -P 3306 -u root rentops -e "
SELECT 'properties' t, COUNT(*) n FROM properties
UNION ALL SELECT 'rooms', COUNT(*) FROM rooms
UNION ALL SELECT 'tenants', COUNT(*) FROM tenants
UNION ALL SELECT 'tenancy_agreements', COUNT(*) FROM tenancy_agreements
UNION ALL SELECT 'agreement_parties', COUNT(*) FROM agreement_parties
UNION ALL SELECT 'payment_transactions', COUNT(*) FROM payment_transactions"

# 义务无重复（本任务的核心不变量）
mysql -h 127.0.0.1 -P 3306 -u root rentops -e "
SELECT user_id, tenant_id, period_month, COUNT(*) c FROM rent_obligations
WHERE rent_charge_id IS NULL GROUP BY 1,2,3 HAVING c>1"   # 必须空

# 冗余字段与关系模型一致（§4 的坑）
mysql -h 127.0.0.1 -P 3306 -u root rentops -e "
SELECT t.name, t.room_label, t.property_hint, r.room_label, p.name
FROM tenants t
JOIN tenancy_agreements a ON a.tenant_id IS NULL   -- 占位：实现时改为按 agreement_parties 关联
LIMIT 5"

# 幂等：重跑后行数不变
scripts/seed-rosewood.sh --stage=data && mysql ... -e "SELECT COUNT(*) FROM tenants"
```

## 风险文件

| 文件 | 风险 | 处置 |
|---|---|---|
| `test-data/rosewood/*.json` | 归一错误会让页面显示脏名字 | 第 3 步人工过 `review.json` |
| `scripts/seed-rosewood.sh` | 清理范围写错会删掉用户自己造的数据 | 删除条件**只按** 4 个已知地址 + 已知姓名集合 + `ROSEWOOD-` 前缀 |
| `rentops` 库 | 灌数据会级联删掉 `rent_obligations` | 这是预期行为（义务惰性重建）；但执行前应确认无手工造的重要数据 |

## 开工前检查

- [x] 用户已审阅 `prd.md` / `design.md` / `implement.md`
- [x] `09-19-rent-obligation-dedup` 已完成（提交 `b252349`，迁移 013 已就位）
- [x] 确认只写本机 `rentops`，不指向 `:8081` / `bank.ddpl.top`

## 验收结果（2026-09-19，本机 `rentops`）

| 验收项 | 结果 |
|---|---|
| `properties` = 4，`rooms` = 25 | 通过 |
| `/properties` `/rooms` `/tenants` `/tenancies` 有数据 | 通过（4 房产 / 25 房间 / 58 租客 / 40 租约） |
| 至少一个房间呈现合租，且房间树人数一致 | 通过：2026-05 有 13 个房间 ≥2 人（Kimmage 05 为 3 人）；房间树渲染出同样的 1 人 ×12 / 2 人 ×12 / 3 人 ×1 |
| `/tenants/{id}` 显示房间、月租、租期，无「未填写」 | 通过（房间 `2 · 72 Walkinstown Rd Dublin 12`，月租 `EUR 580.00`，租期 `2026-05-01 至 2026-12-31`；只有「邮箱」为未填写，不在该项范围内） |
| 每个 `(tenant, month)` 恰好 1 行 | 通过：307 条义务，重复组 0 |
| 重跑脚本各表行数不变 | 通过：重跑前后完全一致 |
| `/rent-dashboard` 各月应收合计不为 0 且随月份变化 | **不通过**，见下 |

### `/rent-dashboard` 应收合计为 0 的根因

`/rent-dashboard` 是 `renderRentWorkspaceDashboard`（`rent-workspace.html`），它的房间金额
只来自 `rent_charges`（`rent_workspace.go:1118` 要求 `rent_charge_id IS NOT NULL`，
`:433` 跳过 `RentChargeID == nil` 的行）。而全仓库**没有任何生产代码写 `rent_charges`**
（只有 `ensureRentCharge`，`landlord_rent_ledger.go:244`，无 HTTP 调用方），本机
`rent_charges` 为 0 行。因此房间树的「应收/已收/未收」全是 `EUR 0.00`，房间状态全是「待处理」。

同时 `obligations.go:139` 的 `MonthlyRentCents <= 0` 守卫把两条路互斥：
租客行有月租（legacy 路）→ 惰性义务、无 charge；月租为 0（structured 路）→ charge 义务。
本任务按 R2/R4 写的是 legacy 路（`tenants.monthly_rent_cents` = 源表「应收」），
所以拿不到房间树的金额。

### 处置决定（2026-09-19，用户已确认）

**只记录，不在本任务修；另开独立任务处理 charge 路径。**

理由：房间视角该不该改用 charge 模型是**产品语义决策**，不是灌数据能解决的；
塞进本任务会让「哪条改动为哪件事负责」变模糊。本任务交付的 3 个产物
（extract.py / seed-rosewood.sh / 任务记录）本身是对的，覆盖了 11 个页面中的 10 个。

本次**未**实施的两个候选方案（留给后续任务，此处仅存档）：

1. 种子额外写 `rent_charges` + charge 背书义务，并让 `tenants.monthly_rent_cents` 保持 >0 ——
   但那样 `ensureMonthlyObligations` 会在每次加载页面时再插一条惰性义务，
   `(tenant, month)` 变成 2 行，违反硬不变量。已用 SQL 实验验证（charge 背书行的
   `lazy_period_month` 为 NULL，`INSERT ... ON DUPLICATE KEY UPDATE` 不会命中）。
2. 在 `ensureMonthlyObligations` 里加一句「该 (tenant, month) 已有 charge 背书义务则跳过」，
   再配合方案 1。这会修改应用代码（`obligations.go`）。

**后续任务至少要回答**：房间视角与租客视角是否应该共用同一份义务数据？
若共用，惰性义务要不要补 `rent_charge_id`；若继续并行，则要明确谁在什么时机写 `rent_charges`。
