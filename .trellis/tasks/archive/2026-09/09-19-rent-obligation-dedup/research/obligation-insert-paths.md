# Research: every production path that INSERTs into rent_obligations

- **Query**: Enumerate non-test code paths that insert into `rent_obligations`; for each, its caller chain, whether it sets `rent_charge_id`, its idempotency guard and the key that guard depends on. Also: what creates `rent_charges`, and is the lazy path legacy or load-bearing?
- **Scope**: internal
- **Date**: 2026-09-19

## Summary

There are exactly **two** non-test insert sites for `rent_obligations`:

1. `ensureMonthlyObligations` (`cmd/truelayer-demo/obligations.go:172`) — lazy, `rent_charge_id IS NULL`, guard on a key that no longer exists. **Reachable from HTTP.**
2. `ensureRentCharge` (`cmd/truelayer-demo/landlord_rent_ledger.go:244`) — charge-based, sets `rent_charge_id`, guard on `idx_rent_charges_user_room_period`. **Not reachable from HTTP:** it has no non-test caller.

A third function, `landlordRentRepository.createRentObligation` (`cmd/truelayer-demo/landlord_rent_repository.go:370`), also inserts, but is only called from tests.

## Insert site 1 — lazy path

`ensureMonthlyObligations` — `cmd/truelayer-demo/obligations.go:172-209`

- Builds one `rentObligation` per tenant of the user (`obligations.go:181-200`).
- **Does not set `RentChargeID`** → inserted rows have `rent_charge_id IS NULL` (`obligations.go:189-200`).
- Sets `GeneratedBy: "lazy"` (`obligations.go:199`).
- Guard (`obligations.go:201-204`):

```go
Clauses(clause.OnConflict{
    Columns:   []clause.Column{{Name: "user_id"}, {Name: "tenant_id"}, {Name: "period_month"}},
    DoNothing: true,
}).Create(&obligation)
```

  - Depends on the unique key `idx_rent_obligations_user_tenant_period`, which migration 009 **dropped** (`migrations/009_landlord_rent_model.sql:92`).
  - The replacement key `idx_rent_obligations_user_charge_tenant (user_id, rent_charge_id, tenant_id)` (`migrations/009_landlord_rent_model.sql:93`) cannot match because `rent_charge_id` is NULL and MySQL/PG treat NULLs as distinct. On MySQL GORM's `OnConflict` therefore degrades to a plain INSERT.

Caller chain to HTTP:

| Function | File:line | Reached from |
|---|---|---|
| `ensureMonthlyObligations` | `obligations.go:172` | — |
| `generateMonthlyObligations` (loops months) | `obligations.go:211-220` (calls at `:215`) | — |
| `listTenantBillingHistory` (calls generate at `:231`) | `obligations.go:222-262` | `handleTenants` GET `/tenants` (`main.go:1049`, route `main.go:498`) |
| `listTenantBillingHistoryPage` (calls generate at `:126`) | `tenant_detail.go:113-167` | `handleTenantDetail` via `handleTenantSubroute` GET `/tenants/{id}` (`tenant_detail.go:333`, route `main.go:499`, dispatch `tenant_detail.go:366-377`) |
| `summarizeRentDashboardWithFilters` (calls ensure at `:353`) | `obligations.go:348-490` | `handleRentDashboard` GET `/rent-dashboard` (`dashboard.go:108`, route `main.go:464`) |

Notes on the other entry points:

- `summarizeRentDashboard` (`obligations.go:344-346`) is just a thin wrapper over `summarizeRentDashboardWithFilters`.
- `dunningService.listCandidates` (`dunning_candidates.go:19-31`) also calls `summarizeRentDashboardWithFilters` at `:26`, but it has **no non-test caller**. The dunning handlers instead call `listCandidatesForRows` (`dunning_handlers.go:153`) with rows already produced by the dashboard summary, so the dunning drawer does not add an independent lazy trigger.
- `handleBilling` (`main.go:725`) and `/bills` do not call the obligation service; repeated `/billing` loads do not insert obligations directly.

So the HTTP-reachable lazy triggers are exactly three GET paths: `/rent-dashboard`, `/tenants`, and `/tenants/{id}`. `/tenants` is the widest: `tenantHistoryMonths = 6` (`main.go:1029`) means `generateMonthlyObligations` runs over a 6-month window on every list render, re-inserting a full set for each active tenant.

## Insert site 2 — charge-based path (dead in production)

`ensureRentCharge` — `cmd/truelayer-demo/landlord_rent_ledger.go:244-373`

- Charge insert (`:313-330`) uses `OnConflict{Columns: user_id, room_id, period_month}.DoNothing()`, which **matches the real key** `idx_rent_charges_user_room_period (user_id, room_id, period_month)` (`migrations/009_landlord_rent_model.sql:80`). This guard is valid.
- If `RowsAffected == 0` the existing charge is loaded and returned (`:334-339`), so obligations are **not** re-created.
- If the charge is newly created, obligations are written with `RentChargeID: &chargeID` (`:341-369`) and `GeneratedBy: "rent_charge"` (`:353`).
- Callers (repo-wide grep for `ensureRentCharge`, `newRentLedgerService`):

| Site | File:line |
|---|---|
| `landlord_rent_ledger_test.go:225,235` | test |
| `rent_workspace_test.go:396` | test |

**No production caller exists.** `newRentLedgerService` is likewise only constructed in tests (`landlord_rent_ledger_test.go:224`, `rent_workspace_test.go:395`). Live DB corroborates: 0 rows in `rent_charges`, 248/248 obligations `rent_charge_id IS NULL` (see `live-db-verification.md`).

`buildRentChargePlan` (`landlord_rent_ledger.go:26`) etc. are pure helpers feeding `ensureRentCharge`; also reachable only through it.

## Insert site 3 — repository helper (tests only)

`landlordRentRepository.createRentObligation` — `cmd/truelayer-demo/landlord_rent_repository.go:370-389`

- Plain `Create` with **no idempotency guard** (`:387`).
- Can set `RentChargeID` (`:379-383`) but nothing in non-test code calls it. Only caller: `landlord_rent_repository_test.go:194,198`.

## What creates `rent_charges`?

Only `ensureRentCharge` (`landlord_rent_ledger.go:313-330`). No other non-test `Create(&charge)` in the repo. Since `ensureRentCharge` has no production caller, **no production code path creates `rent_charges` today.**

If it were called, charge creation itself is idempotent: the `OnConflict` uses the real unique key `(user_id, room_id, period_month)`, and the `RowsAffected == 0` branch short-circuits to the existing charge/obligations. Repeated page loads could not duplicate charges through that function.

## Is `ensureMonthlyObligations` legacy or load-bearing?

**Load-bearing today, in the DBs observed.**

- It is the only writer that has produced any row: 248/248 obligations are `generated_by='lazy'`, `rent_charge_id IS NULL`; zero charges exist (`live-db-verification.md`).
- It is on live GET paths (`/rent-dashboard`, `/tenants`, `/tenants/{id}`).
- In `summarizeRentDashboardWithFilters`, the lazy call at `obligations.go:353` runs **before** the read at `:357`; nothing else in this function would create obligations. Since charge-based rows are never created by any route, the dashboard would render empty if the lazy call were removed (assuming no pre-seeded rows).

Two pieces of code treat `rent_charge_id IS NULL` as invalid, which shows the lazy row shape is already considered second-class elsewhere:

- `transaction_detail.go:204-211` skips obligations with nil/zero `rent_charge_id` when collecting charge IDs for the detail view.
- `rent_workspace.go:1118` filters `AND rent_charge_id IS NOT NULL` when loading obligations for the workspace, and `rent_workspace.go:433` skips rows with `RentChargeID == nil`.

However, neither rejects the rows outright — they merely ignore them in charge-centric views. The dashboard and tenant pages still consume them, so the lazy path is not "already retired"; it is the sole source of the numbers users see.

`generateMonthlyObligations` (`obligations.go:211-220`) exists only to widen the lazy path over a month range; it is called from `listTenantBillingHistory` (`:231`) and `listTenantBillingHistoryPage` (`tenant_detail.go:126`).

## Caveats

- "No production caller" is established by repo-wide grep for the function/service names. If a caller is generated or reflection-invoked, it would not appear; nothing in the tree suggests that.
- The `matching` code loads obligations without any `rent_charge_id` filter (`matching_service.go:70`, `:118`), so lazy rows do participate in matching (see `read-path-dedupe.md`).
