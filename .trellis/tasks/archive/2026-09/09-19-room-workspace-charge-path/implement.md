# 执行计划：房间视角直接汇总租客的月租

## 执行顺序

1. [x] 读 `rent_workspace.go` 的 `load`（`:1070`）与 `buildRentWorkspace` 的
       `:350-517`，对照 design.md §4 标出四个改动点
2. [x] 在 `load` 取数之前调 `newObligationService(s.db).ensureMonthlyObligations(...)`
       （design.md §4.1）
3. [x] 去掉 `:1118` 的 `AND rent_charge_id IS NOT NULL`（§4.2）
4. [x] 建 `roomIDByTenant` / `ambiguousTenant` 归属表（§4.3）
5. [x] 把 `obligationsByCharge` 换成 `obligationsByRoom`，改 `:461` 的入口条件（§4.4）
6. [x] 修回退链：`Currency` 容忍 charge 缺失；房间级 `DueDate` 取最早义务（§4.4）
7. [x] 把 `:505` 的「无义务 → needs_review」挪出 charge 块；确认 `vacant` 分支未被动（§4.4）
8. [x] **核对模板**：确认金额块没有 `{{if .Charge}}` 条件（§4.5）——
       有的话一并修
9. [x] 补/改单测：合租房间（一房多租客）金额 = 各租客义务之和；
       空置房间走 `vacant`；歧义租客（同月挂两房）两边都不计
10. [x] **实机验证**：起一次性实例，`/rent-dashboard` 出现非零金额且与 `/bills` 对得上
11. [x] 跑全量测试与 vet
12. [x] 更新 `.trellis/spec/backend/database-guidelines.md` 的
       「Room-Centric Rent Workspace Read Model」一节：电荷门控已解除，
       房间金额来源是惰性义务 + 参与人归属
13. [x] 提交

**不要跳过第 10 步。** design.md §4.5 的模板条件会让「测试全绿但页面还是 0」
成为一个真实的失败模式 —— 要亲眼看页面。

## 验证命令

```bash
go test ./cmd/truelayer-demo/... && go vet ./...

# 起一次性实例并灌数据（注意：不要指向 :8081 / bank.ddpl.top）
MYSQL_PORT=3306 APP_PORT=18090 scripts/run-audit-local.sh
```

**对账（核心，必须逐月做）**：房间视角的合计（页面上的 summary）应当等于
该月 `/bills` 的应缴合计。先在页面上读房间视角的合计，再读 `/bills` 的合计，
两个数必须相等。DB 侧可交叉验证：

```bash
# 该月义务总额（= /bills 口径 = 房间视角口径，若映射率 100%）
mysql -h 127.0.0.1 -P 3306 -u root rentops -e "
SELECT period_month, COUNT(*) rows_, SUM(expected_amount_cents) cents
FROM rent_obligations WHERE record_status='active' GROUP BY 1 ORDER BY 1"

# 存量义务行未被改写（改动前后逐行可比；条数应恒为 307）
mysql -h 127.0.0.1 -P 3306 -u root rentops -e "
SELECT COUNT(*) total, SUM(expected_amount_cents) cents FROM rent_obligations"

# 归属率：每个月的义务都必须能归到房间（month 列若有缺口即为分叉点）
mysql -h 127.0.0.1 -P 3306 -u root rentops -e "
SELECT o.period_month, COUNT(*) FROM rent_obligations o
LEFT JOIN agreement_parties p ON p.tenant_id = o.tenant_id
WHERE p.id IS NULL GROUP BY 1"    # 必须为空

# 歧义租客（同月挂两个房间）—— 必须为空，非空则验证 §3 规则 2
mysql -h 127.0.0.1 -P 3306 -u root rentops -e "
SELECT p.tenant_id, COUNT(DISTINCT a.room_id) rooms
FROM agreement_parties p JOIN tenancy_agreements a ON a.id = p.agreement_id
GROUP BY 1 HAVING rooms > 1"
```

**基准值（2026-09，本机 Rosewood 种子）**：`/bills` 38 条义务 / 38 个租客，
房间视角 38 条归属 / 25 个房间，两页合计相等。

## 风险文件

| 文件 | 风险 | 处置 |
|---|---|---|
| `cmd/truelayer-demo/rent_workspace.go` | 房间视角读模型，改动影响 `/rent-dashboard` 全部视图与 `/bills` 共用的 `buildRentWorkspace` | 改动集中在 `load` 与聚合段；`load` 里加的补建调用与 `/bills` 同源 |
| 消费 `rentWorkspaceRoomAggregate.Charge` 的模板 | 有金额无 charge 成为常态，条件渲染会静默吞掉金额 | 第 8、10 步专门核对 |
| `cmd/truelayer-demo/obligations.go` | `:130-152` 的守卫是惰性路径的唯一入口 | **不修改**；只调用 `ensureMonthlyObligations` |
| `rent_obligations` 存量 307 行 | 补建调用有写库副作用 | `DoNothing` upsert，只增不改；验证命令带逐行对账 |
| `cmd/truelayer-demo/landlord_rent_ledger.go` | 本任务**不该出现**在这个 diff 里 | 若它出现在 `git status`，说明又走回 charge 方案了，停下来 |

## 开工前检查

- [x] 用户已确认 Q1「房间页面的金额怎么取」→ **直接取租客的**（放弃补 charge 生产者）
- [x] 实测确认映射率 100%（58 租客全部有参与人；无比租客跨两房）
- [x] 实测确认 charge 背书义务 0 条、`rent_charges` 0 行 —— 房间视角现状必然全空
- [ ] 已加载 `trellis-before-dev`
- [x] 已确认本机可运行实例，且不指向 `:8081` / `bank.ddpl.top`
