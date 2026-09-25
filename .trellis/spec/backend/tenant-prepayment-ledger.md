# Tenant prepayment ledger contract

## 1. Scope / trigger

Use this contract when a bank receipt exceeds chosen unpaid rent, when applying a tenant's credit to a later bill, or when reversing allocations linked to that credit. The extra money remains a bank-backed liability-like balance until an explicit rent action.

## 2. Signatures

- `confirmRentMatchBatchWithPrepayment(ctx, userID, transactionID, items, rememberTenantID, prepaymentTenantID, prepaymentAmountCents, requestKey)` performs the explicit split.
- `applyTenantPrepayment(ctx, userID, creditID, obligationID, amountCents, requestKey)` applies credit to one bill. `POST /tenants/prepayment/apply` accepts `tenant_id`, `prepayment_id`, `period`, `amount`, and `request_key`.
- `tenant_prepayments` stores owner, tenant, original income source, original cents, EUR currency, and operation identity. `payment_allocations.prepayment_id` links the effective and historical prepayment/rent rows.

## 3. Contracts

- The optional prepayment amount must equal the entire bank source remainder after the batch's rent shares and existing effective allocations. The tenant must be one of the batch tenants. Confirmation writes rent rows, the credit row, source projection, and optional payer relation in one transaction.
- Effective `prepayment` allocation rows consume bank source budget but never count toward `rent_obligations.paid_amount_cents` or collection rate. Their sum per credit is the currently available balance.
- A manual application locks the original bank source before the target room/obligation, validates the same owner, tenant, currency, credit balance, and unpaid bill balance, voids the current credit share, and writes linked rent plus residual credit rows in one transaction. The original credit record is immutable.
- Revoking a credit-funded rent share restores the same amount as effective credit in the same transaction. Revoking the whole source voids all its effective rent and credit shares; no refund operation exists.
- Exact request retries return the current summary. Reusing a key for another credit, bill, or amount fails.

## 4. Validation and error matrix

| Condition | Result |
| --- | --- |
| No explicit excess choice or zero excess | Do not create credit |
| Excess amount differs from remaining bank cash | Reject split without new allocations |
| Foreign tenant, source, credit, or bill | Reject without partial writes |
| Credit use exceeds available balance or bill remainder | Reject without partial writes |
| Duplicate request with identical values | Return current summary without duplicate rows |
| Duplicate request with changed values | Reject key reuse |
| Full source reversal | Available credit becomes zero and the source can be matched again |

## 5. Good / base / bad cases

- Good: €620 source becomes €600 rent plus €20 credit. A later €12 manual use leaves €8 available and counts €12 toward that later bill.
- Base: a receipt with no explicit excess choice stays partially allocated; no credit is inferred.
- Bad: assigning €20 of credit to another tenant or a bill with only €10 unpaid must fail.

## 6. Tests required

- Unit: `summarizeTransactionAllocations` counts the whole bank source; `ledgerPaidAmount` counts only rent, and source over-allocation is rejected.
- MySQL: batch split, exact/changed retry, manual partial use and residue, insufficient credit, cross-user/bill mismatch, exact-share reversal, and full-source reversal. Run with `RENTOPS_MYSQL_TEST_DSN` against a disposable database.
- UI: confirm the split is explicit and visible; tenant details show credit by source and require manual month/amount selection.

## 7. Wrong vs correct

Wrong: insert €620 as rent against a €600 bill or auto-apply €20 to an inferred future month. That overstates paid rent or spends money without the user's instruction.

Correct: insert a €600 `rent` row and a €20 `prepayment` row linked to an immutable credit, then reclassify only when the user chooses a bill.
