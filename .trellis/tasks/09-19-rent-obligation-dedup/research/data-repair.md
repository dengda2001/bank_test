# Research: data repair for existing duplicate rent_obligations

- **Query**: What is needed to clean existing duplicates in the dev DB and any disposable audit DB without losing payment allocations that reference the duplicate rows? Check `payment_allocations.rent_obligation_id` and `cash_receipts.rent_obligation_id` FKs — how many allocations point at rows that would be deleted, and can they be remapped to the surviving row per (user_id, tenant_id, period_month)?
- **Scope**: internal + live read-only query
- **Date**: 2026-09-19

## Foreign keys that reference `rent_obligations`

| Referencing table | Column | Constraint | On delete | Definition |
|---|---|---|---|---|
| `payment_allocations` | `rent_obligation_id` | `fk_payment_allocations_obligation` | **CASCADE** | `migrations/001_initial_mysql.sql:120` (column `:108`, unique key `:116`) |
| `cash_receipts` | `rent_obligation_id` | `fk_cash_receipts_obligation` | **CASCADE** | `migrations/006_cash_rent_receipts.sql:27` (column `:5`) |
| `dunning_attempts` | `rent_obligation_id` | `fk_dunning_attempts_obligation` | **CASCADE** | `migrations/008_dunning_mail.sql:44` (column `:16`) |

`migrations/003_rent_ledger_foundation.sql:2` makes `payment_allocations.rent_obligation_id` nullable; it does not change the cascade.

**Consequence: a plain `DELETE FROM rent_obligations WHERE ...` silently deletes every allocation, cash receipt, and dunning attempt attached to the removed rows.** Any repair must remap references before deleting.

## Counts in the live dev DB (rentops, read-only)

- `payment_allocations`: 4 rows total; all 4 point at active obligations.
- `cash_receipts`: 0 rows.
- `dunning_attempts`: table does not exist in this DB (`SHOW TABLES LIKE '%dunning%'` → `dunning_send_attempts`, `dunning_sender_configs`). In a DB built from migration 008 the cascade applies.

Reference detail:

| allocation id | rent_obligation_id | tenant_id | period_month |
|---|---|---|---|
| 1 | 13 | 1 | 2026-05-01 |
| 2 | 13 | 1 | 2026-05-01 |
| 3 | 2  | 1 | 2026-04-01 |
| 4 | 2  | 1 | 2026-04-01 |

Both referenced obligations (2 and 13) are the lowest-id row of their `(user_id, tenant_id, period_month)` group, so a "keep lowest id" repair would delete **zero** referenced rows in this DB and no remap would be needed here. This is a property of the current dataset, not a general guarantee — in a DB where a payment attached to a later duplicate, the FK would cascade-delete it.

## Remap feasibility

Remapping is well-defined: every duplicate group shares the same `(user_id, tenant_id, period_month)`, so a survivor can be chosen per group and every other row's references rewritten to it.

Sketch of the required steps (not implemented):

1. Pick a survivor per `(user_id, tenant_id, period_month)`. A safe rule should prefer a row that already holds references or paid amounts, e.g. lowest id, or the id referenced by `payment_allocations`/`cash_receipts` if any. The live data shows survivors 2 and 13 already hold the allocations.
2. Remap:
   - `UPDATE payment_allocations SET rent_obligation_id = <survivor> WHERE user_id=? AND rent_obligation_id IN (<dupes>)`
   - same for `cash_receipts`, and for `dunning_attempts` where that table exists.
3. Recompute the survivor's `paid_amount_cents` / `status` from the remapped rows (`cash_receipts.go:135-136` shows the sum-of-allocations model).
4. Delete the now-unreferenced duplicates.
5. Verify no `(user_id, tenant_id, period_month)` group has more than one row before adding any unique key.

## Risks / gotchas

- **Remap collision**: `payment_allocations` has `UNIQUE KEY idx_payment_allocations_tx_obligation (user_id, payment_transaction_id, rent_obligation_id)` (`001:116`). If the same transaction already has an allocation on the survivor, remapping a duplicate's allocation onto the survivor violates this unique key. The repair must detect and merge/drop those rows (deduplicating by `(user_id, payment_transaction_id, survivor)`).
- **Silent loss**: because the FK is `ON DELETE CASCADE` rather than `RESTRICT`, a mistaken delete produces no error — it just removes payment history. Run the repair with the cascade in mind and in a transaction.
- **Voided rows**: `voidFutureTenantObligations` marks rows `record_status='voided'` (`tenants.go:500-529`) and `buildTenantBillingHistory` still renders voided rows (`obligations.go:289-291`). Duplicates may include both active and voided rows for the same group; the survivor choice and the unique key must account for voided rows (the original 001 key made voided + active for the same month impossible too).
- **Paid-amount understatement today**: duplicates were inserted with `paid_amount_cents=0` (`obligations.go:195`), and allocations attach to only one row. `summarizeRentDashboardWithFilters` sums each row's `PaidAmountCents` (`obligations.go:390`), so before repair the dashboard over-counts expected rent and under-counts paid rent.
- **Audit DB**: not reachable from this environment; only the `rentops` DSN in `.env` was queried. The same cascade/remap logic applies, but the counts must be measured there separately.

## Caveats

- No repair was executed; the above is derived from migrations and the live counts.
- The suggestion to prefer a referenced row as survivor assumes allocations should not be moved when avoidable; an equally valid rule is "lowest id" with an explicit remap, as long as the unique-key collision in step 2 is handled.
