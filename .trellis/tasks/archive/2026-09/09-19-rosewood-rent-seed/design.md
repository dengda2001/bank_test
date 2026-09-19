# 技术设计：Rosewood 数据灌入

## 1. 数据管线

```
收租明细_Rosewood_20260916.xlsx
   │  ① 一次性提取（开发期跑，产物入库）
   ▼
test-data/rosewood/{properties,rooms,tenants,agreements,payments}.json   ← 提交进版本库
   │  ② 每次灌数据跑
   ▼
scripts/seed-rosewood.sh  →  SQL  →  mysql rentops
```

### 为什么把中间产物落成 JSON

- xlsx 解析要手写 XML（本机 `openpyxl` 装不上、`pyexpat` dylib 损坏），且源表大量 `#REF!`、
  列错位、续行姓名——解析规则是**一次性**的脏活，不该在每次灌数据时重跑。
- `test-data/audit/` 已经是「提交进版本库的 JSON fixtures」这一模式，保持一致。
- JSON 可人工审阅和微调：归一后的姓名、金额、租期都能直接改，不用回头动 xlsx。

提取脚本本身也提交（`test-data/rosewood/extract.py`），保证 JSON 可追溯、可重新生成。

## 2. 源表解析要点

- 每 sheet：r2 = 地址，r4 = 表头，r5 起为月分组。组首行带年月（Excel 序列值 46023=2026-01-01 起算），
  后续行年月为空；组内汇总行的特征是「列2 有值且是房间数、列4 是数字」。
- **陷阱**：组首行的房间号常常就是 `1`/`01`——数字。任何「用 `num(row)` 判断是否为汇总行」的写法
  都会把房间 1 整行吃掉（本设计的第一版提取脚本就踩了这个坑，导致 4 个房产各少一个房间）。
  判断汇总行必须同时要求**年月列为空**。
- 列错位：部分行 合同日期/入住日期 为空时，应收值会左移进 合同日期 列。提取时对
  「应收」取「第 8 列，若空则回退第 6 列」的兜底，并对两者都空的行记入异常清单人工确认。

## 3. 姓名归一

同一人跨月会出现大小写/顺序/标点变体。归一规则（按房产内作用域）：

1. 去首尾空白、去尾部逗号、折叠连续空格、统一大写做**匹配键**，保留一个显示用原形。
2. 丢弃非人名占位：`#REF!`、`空`、`和同屋一起付`、纯数字。
3. 续行合并：以 `,` 开头或紧跟在同房间同月另一行之后的碎片（如 `, PHAM TRUNG HIEU (Harvey`），
   并入上一行所指的人，或按括号内别名单独成条。
4. 归一结果落进 JSON 的 `aliases` 字段——`tenants.display_alias` 正是为此存在，页面会显示
   「别名：xxx」，所以别名不必丢弃，可以保留成一条线索。

未匹配/歧义的条目写进 `test-data/rosewood/review.json`，人工过一遍再灌。

## 4. 写入的表与顺序

| 表 | 行数（估） | 关键字段 |
|---|---|---|
| `properties` | 4 | `name`（地址）、`address`、`city_region='Dublin'`、`timezone='Europe/Dublin'`、`status='active'` |
| `rooms` | 25 | `property_id`、`room_label`（源表房间号原样，含 `01` 这种前导零）、`capacity`、`monthly_rent_cents`（该房间当月各租客份额之和）、`due_day`、`active_from`、`status` |
| `tenants` | ~50 | `name`、`display_alias`、**`monthly_rent_cents`（个人份额）**、`currency='EUR'`、`due_day`、`billing_start_date`、`rent_start_date`、`rent_end_date`、`status`、**`room_label` / `room_address` / `property_hint`（冗余，必须与关系模型一致）** |
| `tenancy_agreements` | 每房间每个连续入住段 1 条 | `room_id`、`contract_date`、`move_in_date`、`start_date`、`end_date`、`monthly_rent_cents`（房间合计）、`currency`、`due_day`、`status` |
| `agreement_parties` | 每租客每段 1 条 | `agreement_id`、`tenant_id`、`responsibility_cents`（个人份额）、`joined_at`、`left_at`、`status` |
| `payment_transactions` | ~100+ | 按源表「实收金额」生成，见 §6 |

写入顺序由外键决定：properties → rooms → tenants → tenancy_agreements → agreement_parties → payment_transactions。

### 冗余字段一致性（本设计的最大坑）

`tenants.room_label` / `room_address` / `property_hint` 与 `tenancy_agreements` / `rooms` 是**两套独立数据**，
应用两边都读（`obligations.go:414-415`、`dunning.go:228-229`、`matching_service.go:179-202` 读冗余字段；
`/rent-dashboard` 房间树读关系模型）。灌数据时必须由同一个源计算两者，否则会出现
「租客页显示 A 房间、房间树显示 B 房间」这类不自洽。

→ 实现上：先从归一后的数据算出「租客 → (房产, 房间, 租期, 份额)」，**同一份结构**分别投影到
`tenants` 冗余字段和 `agreements`/`parties`，不做二次推导。

## 5. 幂等

脚本整体单事务，开头按自然键清理本次要写的范围：

```sql
DELETE FROM properties WHERE user_id=? AND address IN (4 个地址);   -- 级联 rooms → agreements → parties
DELETE FROM tenants    WHERE user_id=? AND name IN (归一后的姓名集合);
DELETE FROM payment_transactions WHERE user_id=? AND reference LIKE 'ROSEWOOD-%';
```

删 `rooms` 会级联 `tenancy_agreements` → `agreement_parties`；删 `tenants` 会级联 `rent_obligations`。
**这正好是想要的**：义务由页面惰性重建，清掉反而保证与新的租期一致。

`payment_transactions` 用 `reference` 前缀标记，避免误删用户自己造的流水。

## 6. 银行流水生成

按源表每租客每月的「实收金额」生成一条收入流水（`direction='income'`），`reference='ROSEWOOD-<月份>-<序号>'`，
金额与租客份额一致。分布：

- **约 70% 留未匹配** → 「流水匹配」页有内容，「一键匹配」能给出建议（`CanConfirm` 分支）。
- **约 30% 直接建 `payment_allocations` 指向对应义务**，`status='confirmed'`，
  使应收账单页出现「已收 / 部分收 / 未收」三种状态，而不是清一色未收。

义务 id 依赖页面惰性生成，所以**匹配必须在灌完数据、并访问过 `/rent-dashboard` 之后再执行**。
→ 脚本分两阶段：`--stage=data`（写租赁关系与流水）、`--stage=match`（触发义务生成后建分配）。

### 6.1 `--stage=match` 必须同步回填 `rent_obligations` 的缓存列

`09-19-rent-obligation-dedup` 的实测发现：**没有任何生产代码维护 `rent_obligations.paid_amount_cents`**
（惰性写入器插入时置 0，charge 路径也不写，无触发器），而
`summarizeRentDashboardWithFilters`（`obligations.go:352-390`）是**裸 `Find` 后直接对该列求和**，
不经过投影。只有 `projectRentObligation`（`cash_receipts.go:121`）这类路径才会重算。

后果：若 `--stage=match` 只建 `payment_allocations` 而不更新义务行，
**仪表盘的「已收」会全是 0**，三种收款状态里只剩「未收」—— 上面想要的展示效果不会出现。

→ 建分配的同一条事务里必须同步：

- `paid_amount_cents` = 该义务的 confirmed rent 分配之和 + confirmed 有效现金收款之和；
- `status` 按 `ledgerObligationStatus` 口径重算（优先级 `needs_review` > `voided` > `paid` >
  `partial` > `overdue` > `open`；逾期用 Europe/Dublin 日历日）。

`migrations/013_dedupe_rent_obligations.sql` 的最后一条 `UPDATE` 就是这段口径的可复制实现，直接照搬。

## 7. 不做的事

- 不走 HTTP 驱动应用的表单（25 房间 + 50 租客 + 租约 ≈ 数百次 POST），也不走 `/import-legacy`
  （它只认 tenants/expenses，不认 properties/rooms/agreements）。
- 不引入 Node/npm 依赖：脚本用 shell + `mysql` CLI，与 `scripts/audit/*.sh` 的风格一致。
