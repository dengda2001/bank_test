# design.md §2.5 未解观察 — 实机结论

**观察**（`09-19-prd-prototype-page-audit/research/screenshots/verified-current-desktop-room-detail.png`）：
`78 Old County Road · 03` 房间，KPI「付款记录 1 笔」，但「租客责任」两行的「付款来源」都是 `—`，
「关联收款」只有表头没有数据行。

## 结论

**这不是渲染缺陷，也不是生产数据的状态：那张截图是模板夹具的产物。**

### 证据一：截图可被测试夹具逐字复现

`cmd/truelayer-demo/prototype_preview_render_test.go` 的 `TestWritePrototypePreviewHTML`
在 `/private/tmp/rentops-prototype-preview/room-detail.html` 直接渲染 `rentRoomDetailPageData`，
其中：

```go
Summary: ... PaidAmount: "€1,250", BalanceAmount: "€0" ... PaymentCount: 1,
Tenants: []rentWorkspaceTenantRow{
    {TenantID: 11, TenantName: "WAHAJULLAH KHAN", ExpectedAmount: "€625", PaidAmount: "€625", BalanceAmount: "€0", Status: "paid", StatusLabel: "已缴清"},
    {TenantID: 12, TenantName: "同住人",          ExpectedAmount: "€625", PaidAmount: "€625", BalanceAmount: "€0", Status: "paid", StatusLabel: "已缴清", PaidByOther: true},
},
```

`PaymentCount: 1` 是字面量，每行的 `PaidAmount/BalanceAmount` 也是字面量，
而 `Payments` 切片**根本没赋值**。跑该测试后抓取渲染结果：

```
KPI 付款记录 = 1 笔
租客责任: WAHAJULLAH KHAN €625 €625 €0 — 已缴清
          同住人 他人代付 €625 €625 €0 — 已缴清
关联收款: 只有表头（0 条数据行）
```

与截图完全一致（含 `同住人`、`他人代付`、`€625`、`已缴清`、`78 Old County Road · 03`）。

### 证据二：真实加载路径下 KPI 与表体不可能不一致

`loadRoomDetail`（`rent_workspace_page.go`）里：

```go
paymentIDs := make(map[uint64]struct{})
for _, tenantRow := range data.TenantRows {
    for _, payment := range tenantRow.Payments {
        if payment.PaymentID != 0 { paymentIDs[payment.PaymentID] = struct{}{} }
    }
}
...
PaymentCount: len(paymentIDs),
Tenants:      data.TenantRows,
```

`PaymentCount`、`{{range .Payments}}` 的「付款来源」单元格、「关联收款」表体
**读的是同一个 `tenantRow.Payments`**。所以只要 `付款记录 = N 笔`，表体必然有 N 行；
`N>0` 而表体为空在真实路径下不可达。

### 证据三：真实数据里确实存在「已覆盖 > 0 但付款来源为 —」

这条是**可达的数据状态**，但与截图里的 KPI 矛盾无关。构造（一次性实例）：

```sql
-- 租约把 tenant 41 绑到 room 12，为其 2026-09 责任 186（70000）造一条
-- 来源交易 direction='expense' 的有效租金分配
INSERT INTO payment_transactions (user_id,source,stable_transaction_key,direction,amount_cents,currency,transaction_time,description,match_status)
VALUES (2,'truelayer','probe-expense-direction','expense',70000,'EUR','2026-09-15 12:00:00','Utilities direct debit','unmatched');
INSERT INTO payment_allocations (user_id,payment_transaction_id,rent_obligation_id,tenant_id,amount_cents,allocation_kind,status,confirmed_by_user_id,confirmation_source,idempotency_key)
VALUES (2,LAST_INSERT_ID(),186,41,70000,'rent','confirmed',2,'manual','probe-expense-direction');
```

`/rooms/12?period=2026-09` 实机渲染：

```
KPI: 本月房间应收=EUR 700.00  已覆盖=EUR 700.00  未付=EUR 0.00  付款记录=0 笔
租客责任: Eider Esneir | EUR 700.00 | EUR 700.00 | EUR 0.00 | — | 已缴清
关联收款: 0 条数据行
```

成因是两个取数函数口径不同：

- `projectRentObligation` → `ledgerPaidAmount(allocations)`：只要求分配有效且
  `allocation_kind=rent`，**不看来源交易方向**，所以 70000 计入「已覆盖」；
- `workspacePayments`：`if !ok || transaction.Direction != "income" { continue }`，
  同一笔分配被跳过，所以 `Payments` 为空 → 「付款来源」落 `—`、「关联收款」无行。

同一函数还会跳过 `allocation.TenantID != obligation.TenantID` 的分配（代付场景），
即「已覆盖」与「付款来源」在代付、异向交易下都可能不同步。

## 对后续任务的含义

- 不需要为这个截图改渲染层；截图对应的数据状态在生产路径下不存在。
- 真正值得记的既有口径差异是：**「已覆盖」可能包含没有可展示来源的分配**
  （来源交易非收入方向 / 分配归属人与责任人不一致）。这是显示口径问题，
  影响「付款来源」列与「关联收款」表的可解释性，不影响金额与余额。
- 本次未改动该口径（超出本子任务 prd 范围），仅记录。
