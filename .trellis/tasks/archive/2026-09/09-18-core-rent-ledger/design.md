# 设计：核心账务服务

## 1. 目标与边界

本任务把“租约规则”和“月度账务事实”接起来，并统一银行流水与现金收款对租客责任的覆盖计算。已有旧租客账单路径必须继续可读；新 Property → Room → TenancyAgreement → AgreementParty 路径作为明确的房东租金账务入口，不自动从旧文本字段推断关系。

本任务负责：

- 根据指定房间、月份和有效租约生成一个 `rent_charge` 及其个人 `rent_obligations`。
- 校验责任金额为正、责任人不重复、责任合计等于房间总应收；等分时使用确定性的余数分配。
- 从有效银行分配和有效现金收款重算责任已收、未收和状态，避免依赖调用方传入的缓存金额。
- 支持一笔银行付款拆成多个责任，允许付款人和被覆盖责任属于不同租客，以表达代付。
- 保持幂等、锁定和撤销后的审计事实，不删除或覆盖历史分配。

本任务不负责初始化向导、工作簿导入、跨房间付款入口、HTTP 页面和前端样式。

## 2. 账务事实与生成流程

```text
有效租约 + 有效参与人
          ↓ 校验金额、月份、所有权
     rent_charge（房间/月总应收快照）
          ↓ 一对多
     rent_obligations（租客/月个人责任）
          ↓ 有效覆盖
 bank payment_allocations + cash_receipts
          ↓ 重算
 obligation paid/status + transaction allocation summary
```

生成时读取同一用户下的 property、room、agreement、agreement parties 和 tenants。租约必须覆盖目标月份，参与人必须在目标月份有效。写入的 charge 保存房产名、房间标签和地址快照；obligation 保存租客名称快照、责任金额、月份和到期日。

同一房间同一月份通过 `rent_charges` 唯一键幂等。并发生成时先锁定房间/租约相关事实，使用唯一键冲突后的重新读取；不能创建第二个 charge 或第二组重复责任。

## 3. 责任分配契约

纯函数负责生成前校验，避免把业务规则埋在 SQL 或模板中：

```go
type rentResponsibilityInput struct {
    TenantID uint64
    AmountCents int64
}

func equalRentResponsibilities(totalCents int64, tenantIDs []uint64) ([]rentResponsibilityInput, error)
func validateRentResponsibilities(totalCents int64, inputs []rentResponsibilityInput) error
```

明确金额要求：

- `totalCents > 0`，每个租客 ID 非零且不重复，每个责任金额为正。
- 责任合计必须严格等于房间总应收；不接受静默补差或截断。
- 等分使用整数分配，余数按输入租客 ID 的稳定顺序从前到后各加一分，结果可重复。
- 责任金额只在生成时从租约参与人读取；后续修改租约不得改写已有月份的责任。

## 4. 覆盖、代付与状态

`paymentAllocation.TenantID` 表示本条租金分配覆盖的责任租客；付款来源身份保留在 `paymentTransaction.PayerID/PayerName` 与已有匹配投影中。这样一笔付款可以覆盖多个责任，且不会把“谁付款”和“谁的责任被覆盖”混为同一个字段。

银行分配验证规则：

- 只允许收入流水，金额、币种、用户归属必须匹配。
- 每条租金分配不能超过对应责任的当前未覆盖金额；同一次请求内同一责任的多条 draft 也要累计校验。
- 所有 draft 的合计不能超过来源流水金额；剩余金额保持 `partial`/待处理，不制造负余额。
- 不再限制一个来源流水只能关联一个租客；允许 `600 + 400` 分配到两个租客责任，适用于一人代付同房其他租客。
- 若自动匹配的付款金额超过付款人自己的责任，只自动建议/覆盖其责任，超出部分保留来源余款等待人工拆分；人工拆分可以在同一事务中覆盖其他责任。

责任状态只由有效覆盖重算：

```text
paid >= expected                  → paid
0 < paid < expected               → partial
paid = 0 且当前日期超过到期日    → overdue
paid = 0 且尚未到期               → open
历史事实作废                      → voided
```

有效银行分配必须是 `confirmed` 且有效租金用途；有效现金收款必须是 `confirmed`、EUR 且金额为正。押金、其他收入和已撤销记录不计入租金责任覆盖。

## 5. 事务、幂等与撤销

- 分配事务锁定来源 `payment_transactions` 行，并锁定本次涉及的 `rent_obligations`，在锁内重新读取有效银行/现金覆盖后再校验。
- 插入分配后重算所有受影响责任，再更新来源流水的匹配投影；任一步失败则整体回滚。
- `idempotency_key` 在用户范围内唯一。重试同一事实返回原结果；同一 key 携带不同事实返回错误。
- 撤销只将当前有效分配标记为 `voided`，写入原因、操作者、时间和 operation；不删除银行流水或旧分配。之后锁定并重算全部受影响责任。
- 现金收款沿用同样的锁定/幂等/余额规则；现金作废只改变现金记录状态并重算责任。
- 旧记录没有新字段时，`AllocationKind` 空值按历史房租解释，`rent_charge_id` 为空的旧责任仍可读取，不强行迁移。

## 6. 与现有代码的兼容策略

- 保留 `obligationService.ensureMonthlyObligations` 作为旧租客数据的兼容路径；新路径新增明确的 rent-charge 生成服务，不能让旧页面因为没有 Property 关系而失效。
- 在现有 `transactionService` 和 `cashReceiptService` 上补齐跨租客分配/自动覆盖所需的最小能力，复用现有 `projectRentObligation`、`summarizeTransactionAllocations` 和撤销审计逻辑。
- 不通过 GORM `AutoMigrate` 改 schema，不在 SQL 中拼接用户输入；所有查询继续显式带 `user_id`。

## 7. 风险与回滚

- 最大风险是放开跨租客分配后改变既有 `MatchedTenantID` 语义。实现时保留付款人匹配投影，分配行仅承载责任租客，并为单租客旧场景保留原有结果测试。
- 生成新 charge 是新增路径，不改变旧 obligations 的生成数据；若新路径失败可关闭入口，历史账务继续读取。
- 所有余额和状态更新都由有效事实重算，撤销可独立回滚，不删除审计记录。

