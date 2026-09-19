# Research: fix-shape options for the duplicate rent_obligations bug

- **Query**: Lay out realistic repairs with blast radius; state a recommendation, do not implement.
- **Scope**: internal
- **Date**: 2026-09-19

## Constraints that any fix must respect

1. All 248 rows in the dev DB are lazy rows with `rent_charge_id IS NULL`; zero `rent_charges` exist (`live-db-verification.md`). The charge path has no production caller (`obligation-insert-paths.md`).
2. The lazy path is the only writer the dashboard, `/tenants`, and `/tenants/{id}` rely on, and it runs *before* the reads in `summarizeRentDashboardWithFilters` (`obligations.go:353` vs `:357`).
3. The charge path (`ensureRentCharge`, `landlord_rent_ledger.go:244`) is already idempotent and is covered by `TestRentLedgerServiceCreatesOneChargeAndStableObligationsOnMySQL` (`landlord_rent_ledger_test.go:181-241`).
4. Existing duplicates must be cleaned before any new unique key can be created (otherwise the `ALTER` fails).
5. FK `ON DELETE CASCADE` from `payment_allocations` (001:120), `cash_receipts` (006:27), `dunning_attempts` (008:44) means deleting rows destroys payment history unless references are remapped first (see `data-repair.md`).

## Option (a) — re-add `UNIQUE (user_id, tenant_id, period_month)` alongside the charge key

What changes: a **new** migration (not an edit to 009, which is asserted verbatim by `landlord_rent_migration_test.go:39-56`) that dedupes existing rows then adds:

```sql
ALTER TABLE rent_obligations
  ADD UNIQUE KEY idx_rent_obligations_user_tenant_period (user_id, tenant_id, period_month);
```

This is exactly the 001:70 key that 009:92 dropped.

Risk / conflict:
- MySQL has no partial indexes, so the key applies to charge-based rows too. It enforces "one obligation per (user, tenant, month)". That is the lazy path's intended semantics, but it is **not** obviously true for charges: `ensureRentCharge` is per **room** (`landlord_rent_ledger.go:295-330`), and a tenant can be a party on agreements for two rooms in the same month (`agreementParty` has no per-tenant uniqueness across rooms; `validateRentResponsibilityPlan` only enforces unique ownership *within one plan*, `landlord_rent_ledger.go:144`). If such a tenant existed, the second room's obligation insert would abort with a duplicate-key error — the charge path uses a plain `Create` with no `OnConflict` (`landlord_rent_ledger.go:366`).
- Because the charge path is currently unreachable, this conflict cannot fire today, but it becomes live the moment the charge path is wired to a route.

Behavior/tests disturbed: none of the existing MySQL tests assert absence of duplicates; restoring the key would make the second and later lazy calls silently no-op, which is consistent with `tenant_profile_mysql_test.go:71,105` and the e2e `validateE2ECleanupObligations` expectation of 3 rows (`cleanup.go:358-365`). Does not touch `landlord_rent_migration_test.go`.

## Option (b) — make the lazy path resolve/create a `rent_charge` and set `rent_charge_id`

What changes: `ensureMonthlyObligations` would need to find or create a charge per room and write obligations with `RentChargeID` set so the existing `idx_rent_obligations_user_charge_tenant` (009:93) dedupes.

Risk:
- `ensureRentCharge` requires an **active tenancy agreement** covering the month and at least one active `agreementParty`, and errors otherwise (`landlord_rent_ledger.go:280-289`, `:295-298`). The lazy path currently bills directly from the `tenant` table (`obligations.go:177-200`) and supports tenants with no agreement/room/party. Option (b) would silently stop billing those tenants unless they are backfilled with agreements.
- `ensureRentCharge` takes `propertyID`/`roomID`; the lazy path has neither.
- Charge-based rows would then appear in the charge-centric views that currently skip lazy rows (`transaction_detail.go:204`, `rent_workspace.go:433,1118`), changing those pages' contents and likely their fixture-based tests.

This is the largest change: it is a data-model migration (agreements/parties for every billable tenant) plus a write-path rewrite.

## Option (c) — replace the `OnConflict` guard with an explicit existence check / upsert

What changes: in `ensureMonthlyObligations` (`obligations.go:201-204`), drop the bogus `OnConflict` and either

- `SELECT ... WHERE user_id=? AND tenant_id=? AND period_month=? AND rent_charge_id IS NULL` and skip if present, or
- use `OnConflict{Columns: user_id, tenant_id, period_month}` **after** a real unique key exists (i.e. combined with option (a)).

Risk:
- An explicit existence check with no supporting unique key is racy: two concurrent page loads can both miss and both insert. The current code has the same race, just far more often.
- Removes the write amplification without any schema change; smallest diff.

Tests disturbed: none directly; the 11 tests that call the lazy path keep the same single-row outcome, and `First()` lookups are unaffected.

## Option (d) — retire the lazy path entirely

What changes: delete/deprecate `ensureMonthlyObligations` and `generateMonthlyObligations`, and wire `ensureRentCharge` (or a new bulk charge generator) into the routes that currently trigger lazy generation.

Risk:
- No production caller exists for `ensureRentCharge` today (`obligation-insert-paths.md`), so retiring the lazy path without first wiring the charge path leaves the product with **no** obligation writer — an empty dashboard.
- Requires agreements/parties for every billable tenant (same prerequisite as option (b)).
- All 11 tests that seed via the lazy path would need a new seeding route, and `landlord_rent_migration_test.go` expectations about `generated_by='lazy'` (if any downstream) would shift.

## Recommendation

**Option (a) + (c) together**, sequenced, with (b)/(d) treated as a separate product decision:

1. Data repair first (`data-repair.md`) so no duplicate groups remain.
2. New migration adding back `UNIQUE (user_id, tenant_id, period_month)` (not editing 009).
3. Change `obligations.go:201-204` to match a key that exists (either the restored unique key or an explicit pre-check with `rent_charge_id IS NULL`).

Why: this restores the invariant the code was originally written against (001:70), fixes the current live data without touching the unreachable charge path, and keeps every existing test's single-row expectations valid. The known residual risk is the (user, tenant, month) uniqueness assumption for charge rows; that should be verified against the tenancy model before the charge path is ever wired to a route, because the charge path's plain `Create` at `landlord_rent_ledger.go:366` would then surface duplicate-key errors rather than dedupe.

Option (b) is the "correct long-term" direction but is materially larger: it requires every billable tenant to have an agreement and parties, which the lazy path deliberately does not.

## Caveats

- The two-room-per-tenant scenario is inferred from the schema/`agreementParty` model; no data or code was found that both permits and exercises it.
- No fix was implemented; all line references are to the current working tree.
