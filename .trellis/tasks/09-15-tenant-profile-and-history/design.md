# 技术设计：租客档案、付款人关系与缴费历史

## 目标与边界

本子任务扩展现有 MySQL-backed `cmd/truelayer-demo`，完成租客资料维护、付款人关系维护、生命周期安全更新和独立缴费历史页。收款归类、自动匹配、现金写入和 Dashboard 汇总仍由后续子任务负责；本任务只提供可被它们消费的租客与账单事实。

## 现状依据

- `tenants` 已有姓名、单个 `payer_id`／`payer_name_hint`、月租、计费／租期日期、状态和房间字段，但没有别名、邮箱或一对多付款人表。
- `tenantService` 已负责 MySQL 租客增改查，并通过 `user_id` 隔离；`/tenants` 目前同时承载表单、列表和最近三个月内嵌历史。
- `rent_obligations` 已有有效／作废记录字段；`payment_allocations` 以有效 `confirmed`、`rent` 项作为租金收款事实。
- `listTenantBillingHistory` 已能按月份生成账单并投影收款明细，但查询窗口固定为三个月。
- 实际 `bank-data.json` 和 `log.json` 的流水没有 `payer_name`、`payer_id`、`remitter_*` 或 `counterparty_*` 顶层字段；付款人名称主要来自 `meta.counter_party_preferred_name`（例如 `Mike`、`ZR Institute`），没有稳定付款人 ID 时解析结果为名称 + `PayerID=unknown`，再缺名称才退回描述文本。

## 数据模型与迁移

新增 `migrations/004_tenant_profile_and_payers.sql`：

1. `tenants` 增加 `display_alias`、`email`；保留原有 `payer_id` 和 `payer_name_hint` 作为向后兼容字段。
2. 移除 `tenants(user_id, payer_id)` 的唯一约束，改为普通索引；共享付款人必须可以存在于多个租客关系中。
3. 新增 `tenant_payers`：
   - `id`, `user_id`, `tenant_id`；
   - `payer_id` 可空；名称关系是一期的第一类数据，使用 `payer_name_original` 与 `payer_name_normalized`，不为 name 伪造稳定 ID；
   - `source`, `created_at`, `last_matched_at`, `removed_at`, `removed_by_user_id`；
   - 同一账户下稳定付款人 ID和标准化姓名均建索引，不用唯一约束；相同标准化姓名出现在多个租客时标记 shared/conflict，不能据此自动选择租客；
   - 外键均带 `user_id` 业务校验，租客删除级联，移除关系采用软删除。
4. 迁移已有非空 `tenants.payer_id`／`payer_name_hint` 为一条 active `tenant_payers` 关系。迁移必须可重复执行，并且不覆盖已经存在的关系。

保留旧列是为了兼容已有导入和当前银行匹配代码；本任务的资料页以 `tenant_payers` 为权威关系来源，后续银行子任务再切换自动匹配读取路径。对现有只有名称的资料，回填 `payer_id=NULL`、保留原始名称并生成标准化名称；不从银行流水的 `counter_party_preferred_name` 自动创建租客关系。历史分配只引用交易和账单，不因移除付款人关系而改写。

## 服务边界

### Tenant service

扩展 `tenant`、`tenantRecord` 和 `tenantInput`，增加别名与邮箱，并提供：

- `createTenant`：规范化姓名、别名、邮箱、EUR 和日期；未显式提供计费开始日时使用租期开始日。
- `updateTenant`：先按 `id AND user_id` 读取原租客，再在同一数据库事务中校验生命周期变更，避免跨账户更新。
- `listTenantPayers`、`addTenantPayer`、`removeTenantPayer`：所有读写都带账户和租客条件；名称必填，稳定 ID 可选；移除只写 `removed_at`／操作者，不删除历史。
- `validateTenantInput`：校验邮箱格式、租期日期关系、计费开始日在租期内、EUR、正月租和有效状态。

租客更新的生命周期规则：

- 停用只阻止未来懒生成账单，不删除或隐藏既有账单、欠款和收款历史。
- 新租期结束日所在月仍可计费；仅处理结束月之后的账单。
- 延后结束日期不会重写已生成账单金额。
- 提前结束时，查询受影响的未来有效账单；只要任一账单存在有效租金分配，整个更新事务失败，不部分作废。
- 没有有效收款的受影响账单在同一事务中标记 `record_status=voided`，写入时间、操作者和原因；原账单保留可查。

### Billing history service

保留现有 `buildTenantBillingHistory` 作为纯投影函数，并把固定三个月的读取拆成可复用查询：

- 单租客详情按 `from_month`、`to_month` 读取，默认最近 12 个适用月；页码只影响返回的账单行，不影响账单金额。
- 期间内先按租客租期／计费开始日判断适用月份，再幂等生成缺失账单；查询既有作废账单并以“已作废”状态展示，确保历史不消失。
- 有效账单无收款时返回零实收和明确空明细；有效收款只读取 `confirmed + rent` 分配并连接原始收入流水。
- 结果使用专用页面 view model，不把 GORM 结构直接交给模板；返回总适用月数、总页数、当前页和筛选范围。

付款人关系在历史页只作为当前档案信息展示；不会根据关系重写旧交易或在本任务执行自动匹配。

## HTTP 与页面

- 保留 `GET|POST /tenants` 作为列表和增改入口，新增别名、邮箱字段，并将现有新增表单的计费开始日默认设置为租期开始日；编辑时始终使用数据库已有值。
- 增加 `GET /tenants/{id}` 独立租客详情页。支持 `from_month`、`to_month`、`page`、`page_size`，显示档案、付款人关系、期间账单和每月有效银行／现金来源明细。
- 在列表中把租客姓名、别名和“查看详情”链接指向独立详情页；可以保留现有折叠历史作为兼容展示，但不再把三个月限制作为唯一历史入口。
- 详情页提供新增／移除付款人关系操作，移除要求当前用户会话并保持历史不变。
- 所有失败都使用现有 redirect/error 模式或安全的 404／400；不回显其他用户资源是否存在。

## 兼容性与回滚

- 现有 JSON fallback 继续可读；新增字段为空时使用旧格式，数据库会话下的租客页面仍以 MySQL 为准。
- 迁移不删除旧资料列、不重写账单或收款；若应用回滚，新增列可被旧代码忽略，旧单付款人字段仍保留。
- 迁移、生命周期更新和付款人关系写入必须通过事务；并发更新在读取受影响账单后重新校验有效分配，任何冲突都回滚整次保存。

## 风险与取舍

- 采用软删除付款人关系而非物理删除，换取审计和未来解释能力；列表默认只显示 active 关系。
- 采用 `/tenants/{id}` 而不是继续扩展 `/tenants?view=`，使详情页可单独授权和分页；保留列表路由降低现有链接和测试的迁移成本。
- 详情默认 12 个月且允许自定义期间，避免首次打开拉取无限历史，同时满足超过三个月和完整可追溯查询需求。
