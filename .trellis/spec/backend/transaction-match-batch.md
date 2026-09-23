# Batch rent matching contract

## 1. Scope / trigger

Use this contract when changing the transaction review drawer, `/transactions/confirm-batch`, or rent allocation orchestration. A bank receipt can pay several tenants or several months for one tenant. The receipt remains one source record with several effective allocation rows.

## 2. Signatures

- HTTP: `POST /transactions/confirm-batch` (`handleRentMatchBatchConfirmation`).
- Service: `confirmRentMatchBatch(ctx, userID, transactionID, items, rememberTenantID, requestKey) (transactionAllocationSummary, error)`.
- Item: `rentMatchBatchItem{TenantID uint64, Period string, AmountCents int64}`.
- Storage: reuse `allocateTransactionInTx` and the existing `payment_allocations` and rent obligation tables; no separate batch table.

## 3. Contracts

- Form fields: `transaction_id`, `request_key`, repeated aligned `tenant_id[]`, `period[]` (`YYYY-MM`), and `amount[]` (positive decimal EUR amount), optional `remember_tenant_id`, `match_tenant`, and `return_to`. Maximum 20 items.
- The review drawer's selected tenant controls evidence lookup only. Each draft row owns its tenant, month, and amount. Tenant/month pairs must be unique within the batch; a tenant may appear for several months.
- The service verifies the source belongs to the authenticated user and is income before explicit month materialization. It verifies every tenant belongs to the user, materializes facts only for months explicitly chosen for payment, and reloads the obligations.
- The service locks the source and uses `allocateTransactionInTx` to validate current obligation and source balances, currency, and ownership and write the entire batch in one database transaction. An optional payer association is saved in that same transaction for one explicitly selected tenant from the batch.
- Successful POST redirects with `message=rent_confirmed`. Invalid form or changed balances redirect back to the review drawer with an error code. The browser preserves an unsaved draft across tenant lookup, history pagination, and error redirect; cancel or close discards it.
- `request_key` is required. An exact retry returns the existing allocation summary; reuse for different allocation targets or amounts fails.

## 4. Validation and error matrix

| Condition | Behavior |
| --- | --- |
| Missing, empty, misaligned, or more than 20 item fields | `invalid_batch_match`; no allocation rows |
| Duplicate tenant/month, invalid month, nonpositive amount, or remembered tenant absent from batch | Reject before materialization |
| Source or tenant belongs to another user | Reject before materialization |
| Voided or missing obligation, different currency, exceeded obligation balance, or exceeded source balance | Roll back every allocation in the batch |
| Same request key and same rows | Return current summary without duplicate writes |
| Same request key and different rows | Reject key reuse |
| Payer association cannot be saved | Roll back allocations and payer association together |

## 5. Good / base / bad cases

- Good: one €1,200 receipt allocates €600 to tenant A in September and €600 to tenant B in September. Both obligations advance and source remaining becomes zero.
- Good: one €1,200 receipt allocates €600 to one tenant in September and €600 in October.
- Base: a partially allocated receipt shows existing allocations and accepts only a new batch within the live remaining amount.
- Bad: a batch whose second item exceeds its obligation fails with zero new allocation rows.

## 6. Tests required

- `transaction_match_batch_test.go`: distinct tenants, distinct months for one tenant, duplicate pair, invalid month, amount, and payer selection.
- `transaction_match_batch_mysql_test.go` with `RENTOPS_MYSQL_TEST_DSN`: assert two allocation rows and projected paid amounts for the two main cases, exact retry row count, changed-key payload rejection, partial continuation, over-budget rollback, and payer saved for only one tenant.
- Browser review at desktop and mobile widths: add two months, paginate same-payer history, switch tenant, add another tenant, submit both rows, and check overflow and console errors.

## 7. Wrong vs correct

Wrong: save a separate allocation each time the reviewer clicks a month, or infer the payer relationship for every tenant receiving a share. This leaves partial ledger state and misidentifies roommates.

Correct: keep each month selection as a browser draft, then submit all rows once through the locked allocation transaction; save a payer relationship only when the reviewer selects one actual payer tenant.
