# Research: existing tests touching obligation uniqueness, counts, and ensureMonthlyObligations

- **Query**: Which tests assert anything about obligation uniqueness, counts per (tenant, month), or `ensureMonthlyObligations`? Would restoring a unique key or changing the lazy path break them?
- **Scope**: internal
- **Date**: 2026-09-19

## Test harness

All obligation DB tests use `openLedgerMySQLTestDB` (`ledger_mysql_test.go:16-31`), which reads `RENTOPS_MYSQL_TEST_DSN` and `t.Skip`s when unset (`:18-21`). They run migrations from `../../migrations` (`e.g. landlord_rent_ledger_test.go:183`). So none of these run in a default `go test` without a MySQL DSN.

## Tests that call the lazy path

| Test | File:line | Calls |
|---|---|---|
| `TestDunningCandidateReadModelIsScopedAndSkipsTodaySuccessOnMySQL` | `dunning_mysql_test.go:10` | `ensureMonthlyObligations` at `:55,:58` |
| `TestDunningSendWorkflowOnMySQL` | `dunning_send_mysql_test.go:26` | `ensureMonthlyObligations` at `:70,:73` |
| `TestTransactionActionsRevokeAndRestoreOnMySQL` | `transaction_actions_mysql_test.go:10` | `:30` |
| `TestTransactionActionsRematchOneRentAllocationOnMySQL` | `transaction_actions_mysql_test.go:111` | `:138` |
| `TestManualBalanceSettlesOnlyTheOutstandingRentOnMySQL` | `dashboard_manual_balance_mysql_test.go:14` | `:46` |
| `TestManualBalanceConcurrentRequestsCreateOneTransactionOnMySQL` | `dashboard_manual_balance_mysql_test.go:127` | `:148` |
| `TestTenantLifecycleVoidsUnpaidFutureBillsButRejectsPaidOnMySQL` | `tenant_profile_mysql_test.go:42` | `generateMonthlyObligations` `:63`, `ensureMonthlyObligations` `:86` |
| `TestRentDashboardSummaryFiltersAndArrivalMetricsOnMySQL` | `dashboard_mysql_test.go:10` | `:63,:66,:78` |
| `TestDunningDashboardHTTPWorkflowOnMySQL` | `dunning_handlers_mysql_test.go:14` | `:37` |
| `TestCashReceiptLifecycleOnMySQL` | `cash_receipts_mysql_test.go:40` | `:63` |
| `TestCashReceiptConcurrentBalanceGuardOnMySQL` | `cash_receipts_mysql_test.go:217` | `:238` |

Almost all of them resolve an obligation with `First()` on `(user_id, tenant_id, period_month)` and then operate on its `ID`, e.g. `dunning_send_mysql_test.go:79`, `transaction_actions_mysql_test.go:34`, `cash_receipts_mysql_test.go:67,242`, `dashboard_manual_balance_mysql_test.go:50,152`. `First()` returns the lowest-id row, so duplicates do not break these lookups; they only make the tests pass against an unintended extra row.

## Tests that assert counts per (tenant, month)

| Test | Assertion | File:line |
|---|---|---|
| `TestTenantLifecycleVoidsUnpaidFutureBillsButRejectsPaidOnMySQL` | `voided != 2` fails, i.e. exactly 2 voided obligations for `(owner, created tenant)` after `generateMonthlyObligations(2026-11..2026-12)` | `tenant_profile_mysql_test.go:71-74` |
| same test | `stillActive != 1` fails, i.e. exactly 1 active obligation for `(owner, paidTenant, 2026-11)` | `tenant_profile_mysql_test.go:105-108` |

Relevance: the first assertion counts voided rows for a tenant ignoring period; a re-run of the lazy path in that test could inflate it. In the current test the extra `ensureMonthlyObligations(2026-11)` at `:86` runs *after* the `voided != 2` check and covers all tenants, so tenant `created` would gain duplicate 2026-11 rows — but nothing counts them, so the test still passes. This is a test blind spot, not a guarantee.

No test asserts the *absence* of duplicates, and no test counts active obligations per (tenant, month) directly.

## Tests that assert the migration's key change (would break option (a) as-specified)

`landlord_rent_migration_test.go` reads `migrations/009_landlord_rent_model.sql` verbatim (`readLandlordRentMigration`, `:13-20`) and asserts it **contains** (lower-cased):

- `"drop index idx_rent_obligations_user_tenant_period"` — `:39-56` (list at `:44`)
- `"add unique key idx_rent_obligations_user_charge_tenant"` — `:49`

Two tests do this: `TestLandlordRentMigrationExtendsHistoricalFactsWithoutReplacingThem` (`:39`) and `TestLandlordRentMigrationKeepsNewFactsUserScopedAndIdempotent` (`:57`). Editing migration 009 to keep the old key would break the first; adding a **new** migration that re-adds the key would not (the helper reads only file 009).

## Tests that exercise the charge path (relevant to option (b)/(d))

`TestRentLedgerServiceCreatesOneChargeAndStableObligationsOnMySQL` (`landlord_rent_ledger_test.go:181-241`) is the only idempotency test of the charge path:

- after the first `ensureRentCharge`: `len(first.Obligations) == 2` and the two amounts sum to 100000 (`:232-234`);
- after the second `ensureRentCharge`: same charge ID and **same obligation IDs**, `len == 2` (`:239-241`).

So the charge path is already proven idempotent; any fix that routes the lazy path through it must preserve this contract.

`landlord_rent_repository_test.go:194-198` creates one obligation with a `RentChargeID` and asserts a cross-user create returns `gorm.ErrRecordNotFound`; `landlord_rent_repository_test.go:237` asserts `listRentObligations(...)` returns exactly 1 row for a seeded (property, room, tenant, month). These constrain `createRentObligation`, not the lazy path.

## Runtime verifier (e2e)

`validateE2ECleanupObligations` (`cmd/rentops-e2e/cleanup.go:350-375`) expects **exactly 3** obligations for the run's user:

- `(Tenant, 2026-08)`, `(Tenant, 2026-09)`, `(DunningTenant, 2026-09)` (`:358-360`),
- `len(rows) != len(expected)` → `"unexpected rent obligation count"` (`:363-365`),
- and it rejects a duplicate key inside the result map (`:369-371`, `result[key] != 0`).

The fixture tenants start at `2026-08-01` and `2026-09-01` (`manifest.go:101-123`). The run loads `/rent-dashboard?period=2026-08` once (`dashboard_scenario.go:21`) and `/tenants` (which triggers the 6-month lazy generation) via `tenant_scenario.go:41,165`. If both run against a DB carrying the current schema, the lazy path would re-insert the same (tenant, month) rows and this validation should fail. Whether the current e2e actually fails was **not** executed here — flagged for runtime confirmation.

## Impact summary

| Change | Tests affected |
|---|---|
| Add a new migration re-adding `UNIQUE (user_id, tenant_id, period_month)` | Safe for `landlord_rent_migration_test.go` (reads 009 only). Could make double-insert lazy calls silently no-op, which matches the count-based assertions (`tenant_profile_mysql_test.go:71,105`) and e2e expectation of 3. |
| Edit migration 009 to keep the old key | Breaks `landlord_rent_migration_test.go:39-56` (asserts the drop). |
| Make lazy path create/resolve a charge and set `rent_charge_id` | Must keep `landlord_rent_ledger_test.go:181-241` green; and `transaction_detail.go:204` / `rent_workspace.go:433,1118` would then include these rows in charge-centric views, changing their fixture expectations. |
| Retire the lazy path | Tests calling `ensureMonthlyObligations`/`generateMonthlyObligations` (11 tests above) would need a new seeding route; the dashboard/tenant pages would have no writer unless the charge path is wired to a route. |

## Caveats

- The MySQL-backed tests were not executed (no `RENTOPS_MYSQL_TEST_DSN` in this environment); conclusions about which pass are static (from code reading).
- The e2e obligation-count conclusion is static and marked unverified.
