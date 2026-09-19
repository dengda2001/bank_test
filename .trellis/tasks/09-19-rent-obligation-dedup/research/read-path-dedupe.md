# Research: read-path dedupe of rent_obligations

- **Query**: Confirm or refute that nothing on the read path groups or dedupes obligations, so duplicates are user-visible and summed into rent-due totals.
- **Scope**: internal
- **Date**: 2026-09-19

## Conclusion

**Confirmed.** No obligation read path uses `DISTINCT`, `GROUP BY`, or any in-memory dedupe keyed on `(tenant_id, period_month)`. Every duplicate row becomes its own dashboard row, its own billing-history month entry, its own match option, and its `expected_amount_cents` is added into the dashboard totals.

A repo-wide grep for `DISTINCT` / `Group(` over non-test files (`cmd/truelayer-demo/*.go`) found no grouping on obligations.

## Dashboard summary — `summarizeRentDashboardWithFilters` (`obligations.go:348-490`)

- Loads all active obligations for `(user_id, period_month)` with an `Order("tenant_id ASC")` and no dedupe (`obligations.go:356-359`).
- Loops every row (`obligations.go:374-429`) and for each one:
  - accumulates totals: `summary.ExpectedCents += obligation.ExpectedAmountCents` (`:389`), `summary.PaidCents += obligation.PaidAmountCents` (`:390`), `BalanceCents += expected - paid` when positive (`:391-393`);
  - increments status counters (`:394-405`);
  - appends a `rentDashboardRow` (`:410-428`).
- Then `summary.TotalRows = len(allRows)` (`:430`) counts duplicates, and the page rows are the un-deduped list (`:432-438`).

So N duplicate rows for one tenant-month contribute N rows and N x expected rent to `ExpectedCents` / `BalanceCents`. In the live dev DB that is up to 86 extra rows for tenant 1 / 2026-09 at 105000 cents each.

- The same rows feed the dunning candidate flow: the dunning drawer calls `listCandidatesForRows` on the dashboard rows (`dunning_handlers.go:153`), so duplicates become duplicate dunning candidates.

## Tenant billing history — `buildTenantBillingHistory` (`obligations.go:264-312`)

- Called from `listTenantBillingHistory` (`obligations.go:261`) and `listTenantBillingHistoryPage` (`tenant_detail.go:150`).
- Iterates every obligation and appends one `tenantBillingMonth` per row (`obligations.go:283-305`). The append key is `history[obligation.TenantID]`, so all duplicate months land in the same tenant's slice; there is no de-dup pass, only a sort (`:306-310`).
- Result: the tenant list (6-month expansion, `main.go:1049`) and `/tenants/{id}` history pagination (`tenant_detail.go:151`) both show repeated month rows, and `TotalRows`/pagination include the duplicates.

The underlying queries are also undeduped:
- `listTenantBillingHistory` query at `obligations.go:240-242`.
- `listTenantBillingHistoryPage` query at `tenant_detail.go:130-132`.

## Matching paths

- `matching_service.go:70` loads all obligations for a tenant (`Find(&obligations)`), and `matching_service.go:118` all obligations for a user, with no dedupe.
- `availableRentMatchOptions` (`matching_service.go:214-235`) emits one option per obligation whose remaining balance covers the amount (`:221`). Duplicate rows each have `paid_amount_cents=0`, so the same month appears many times as an allocatable target, and automatic matching (`matching_service.go:255-275`) can pick any of them.
- `landlordRentRepository.listRentObligations` (`landlord_rent_repository.go:334-365`) selects `ro.*` with no dedupe; it is a general read helper.

## Charge-centric views (these do discard lazy rows)

These are the only read paths that filter the lazy rows out, and they do it by rejecting `rent_charge_id IS NULL` rather than by deduping:

- `rent_workspace.go:1118` — obligations loaded with `AND rent_charge_id IS NOT NULL`.
- `rent_workspace.go:433` — skips `row.RentChargeID == nil`.
- `transaction_detail.go:204-211` — skips obligations with nil/zero `rent_charge_id`.

They are not a dedupe mechanism: they only hide the lazy rows from workspace/transaction-detail rendering.

## Caveats

- `matching.go:208` / `landlord_rent_repository.go` helpers were read only where they touch obligation selection; a full audit of every matching branch was not performed.
