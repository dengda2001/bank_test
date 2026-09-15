# 月度收租总览与查询工作台：技术设计

## 目标与边界

扩展现有 `/rent-dashboard` 数据库页面，使所选租金所属月份的全量汇总与可筛选账单列表使用同一份有效账本投影。页面只读取本地账单、银行同步状态、银行流水和现金收款；银行到账月份仍只用于待处理／其他收入入口，不改变租金所属月份。

本任务不新增账本写接口、不实现催缴抽屉或邮件发送、不改变银行同步窗口，也不做多币种合计。现有租金账单生成和现金／银行写入服务继续负责账本事实。

## 关键决定

- 先取得所选月份的全部有效 `rent_obligations`，计算全量应收、实收、未收和状态人数；再在内存中执行搜索、状态筛选、排序和分页，保证卡片数值不随列表筛选变化。
- 实收沿用账单的 `paid_amount_cents` 投影；列表明细沿用 `listRentPayments`，因此有效银行房租和有效现金收款在 Dashboard 与租客历史中一致。
- Dashboard 列表状态排序固定为逾期、需处理、部分缴纳、未缴、已缴清，再按选定排序字段和租客 ID 做稳定排序。默认每页 12 条，页码越界返回空列表而保留全量计数。
- 待处理金额／笔数按所选月份的银行 `transaction_time` 聚合，金额使用每笔流水 `amount - effective allocations` 的剩余；只统计收入且剩余大于零的流水。其他收入金额／笔数按同一到账月的有效 `other_income` 分配聚合，按原流水去重计数。
- 所有查询都带当前 `user_id`。同步状态使用当前用户最近一次 `bank_sync_runs` 及其账户覆盖结果；失败或部分成功必须单独显示，不折叠成“无待处理”。

## 数据流

```text
GET /rent-dashboard?period=&search=&status=&sort=&page=&page_size=
  -> signed v2 session user_id
  -> reconcile bank rows for user
  -> monthly obligations + effective payment projection
  -> full summary + filtered/paged rows + pending/other/sync metrics
  -> typed rentDashboardPageData
  -> HTML cards, filters, rows, payment details and links
```

筛选链接通过 URL 保留 `period/search/status/sort/page_size`；改变月份、搜索、状态或排序时页码重置为 1。金额／人数卡片使用同一月份但可带入口专用银行筛选参数跳转 `/billing`。

## 接口与模型契约

- `rentDashboardFilters`: `Search`, `Status`, `Sort`, `Page`, `PageSize`，状态只允许 `all/open/overdue/partial/paid/unpaid/needs_review`，排序只允许 `status/tenant_asc/amount_desc/amount_asc/due_asc/due_desc`。
- `obligationService.summarizeRentDashboardWithFilters(ctx, userID, periodMonth, filters)` 返回全量汇总、筛选后数量、分页行、页码、总页数、待处理金额／笔数、其他收入金额／笔数和同步覆盖文案。
- `rentDashboardRow` 增加 `TenantID`，用于进入同租客历史／现金补录；支付明细继续标识现金来源、收款日和现金收据号。
- `rentDashboardPageData` 增加搜索、状态、排序、分页、结果计数、其他收入、待处理和同步状态字段，模板不得直接查询 GORM 模型。

## 兼容与风险

- 保留无数据库登录前的 JSON fallback 页面；数据库会话路径使用新查询，旧模板测试数据仍可只填已有字段。
- 汇总不依赖当前页；筛选零结果仍显示全量卡片并区分“没有符合条件的账单”和“本月无账单”。
- 多币种账单不被静默换算；当前一期 EUR 数据汇总使用 EUR，出现非 EUR 时以安全错误返回而不是混加。
- 通过内存切片分页避免把用户输入拼接进 SQL `ORDER BY`；若未来改为 SQL 分页，必须保留白名单排序映射。
