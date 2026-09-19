# Research: live DB verification of the rent_obligations duplication bug

- **Query**: Verify the confirmed duplicate-obligation bug against the running dev database (read-only).
- **Scope**: internal (code) + live read-only MySQL query
- **Date**: 2026-09-19

## Method

Read-only queries against the DSN in `.env`:
`rentops:rentops_local@tcp(127.0.0.1:3306)/rentops`

No writes were performed.

## Findings (rentops DB)

```sql
SELECT COUNT(*) total, COUNT(DISTINCT tenant_id, period_month) distinct_pairs
FROM rent_obligations WHERE record_status='active';
-- total=248, distinct_pairs=9
```

Per-pair duplication (all rows are `user_id=1`, `tenant_id=1`):

| tenant_id | period_month | active rows |
|---|---|---|
| 1 | 2026-09-01 | 86 |
| 1 | 2026-04-01 | 30 |
| 1 | 2026-05-01 | 30 |
| 1 | 2026-06-01 | 30 |
| 1 | 2026-07-01 | 30 |
| 1 | 2026-08-01 | 30 |
| 1 | 2026-03-01 | 4 |
| 1 | 2026-01-01 | 4 |
| 1 | 2026-02-01 | 4 |

Confirms the reported "tenant 1 holds 86 active obligations for 2026-09 alone" and the 248 total.

```sql
SELECT COUNT(*) FROM rent_obligations WHERE rent_charge_id IS NULL;   -- 248
SELECT COUNT(*) FROM rent_charges;                                    -- 0
```

**Every active obligation in the dev DB was written by the lazy path (`generated_by='lazy'`, `rent_charge_id IS NULL`), and there are zero `rent_charges`.** Sample row:

```
id=69, user_id=1, tenant_id=1, period_month=2026-09-01,
expected_amount_cents=105000, paid_amount_cents=0, record_status=active,
generated_by=lazy, rent_charge_id=NULL
```

This is stronger than the diagnosis in the task prompt: the charge-based obligation writer is not just failing to dedupe, it has **never run in this database** (see `obligation-insert-paths.md` for why — `ensureRentCharge` has no non-test caller).

## Payment references (relevant to repair, see `data-repair.md`)

```sql
SELECT COUNT(*) FROM payment_allocations;                       -- 4
SELECT COUNT(*) FROM cash_receipts;                             -- 0
```

All 4 allocations point at the two lowest-id rows of their pairs:

| allocation id | rent_obligation_id | tenant_id | period_month |
|---|---|---|---|
| 1 | 13 | 1 | 2026-05-01 |
| 2 | 13 | 1 | 2026-05-01 |
| 3 | 2  | 1 | 2026-04-01 |
| 4 | 2  | 1 | 2026-04-01 |

In this DB a "keep lowest id per (user_id, tenant_id, period_month)" strategy would delete only rows that have no FK references, so no remap would be required here. That is a property of this dataset, not a guarantee in general.

`dunning_attempts` does not exist in the rentops DB; the applied table is `dunning_send_attempts` / `dunning_sender_configs`. So migration 008's `dunning_attempts` FK is not present here, and the only live FK holders to `rent_obligations` are `payment_allocations` (001) and `cash_receipts` (006).

## Caveats

- Only the `rentops` dev DB was reachable. The disposable audit DB / any production-like DB were not queried.
- The rentops DB has only one tenant with rows and 4 allocations, so its repair is trivial; it does not exercise the general remap case.
