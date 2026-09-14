# Technical design — tenant billing and payment hierarchy

## Status

Planning complete; implementation approved and in progress. This task remains one integrated task because the backend projection and nested tenant UI share the same accounting contract and must be verified together.

## Domain model and boundaries

The existing relational model already expresses the required parent-child relationship:

```text
Tenant
└── RentObligation (one per tenant + billing month)
    └── PaymentAllocation (zero or many confirmed allocations)
        └── PaymentTransaction (bank transaction detail)
```

- `rent_obligations` is the persisted monthly bill/parent, identified by `user_id`, `tenant_id`, and `period_month`.
- `payment_allocations` is the confirmed child association. Its `rent_obligation_id` allows multiple transfers to roll up into one month.
- `payment_transactions` remains the immutable/raw bank-side record and is joined only for detail display.
- No new parent table or migration is needed. The UI-only projection adds a tenant history collection containing monthly bill projections and their payment detail rows.

## Data flow

```text
authenticated /tenants request
  → tenantService loads user-scoped tenants
  → obligationService ensures eligible obligations for current and previous two months
  → one bounded obligation query + one allocation/transaction detail query
  → group by tenant and obligation
  → tenant page view model
  → collapsed tenant row
  → collapsed monthly bill row
  → confirmed payment detail rows
```

The history query must be bounded to the three requested months and the authenticated `user_id`. It must include eligible bill months even when no allocation exists, so a zero-payment month renders as an open/overdue bill rather than disappearing.

Use the existing `tenantActiveInMonth` and `obligationStatus` rules. A tenant's months before billing/rent start or after rent end are not fabricated. Current and prior months are normalized with `monthStart` and formatted for display at the template boundary.

`paid_amount_cents` remains the existing read-optimized bill total and must stay consistent with confirmed `payment_allocations`; detail rows are sourced only from confirmed allocations joined to their transactions. Do not include unmatched, candidate, or needs-review bank transactions in actual rent received.

The existing `parseReferencedPeriod` parser remains the single owner for month references in descriptions and references. Extend it with a compact English month-year pattern such as `JULY26` / `SEP26`, mapping the two-digit year to the 2000s. Preserve the existing precedence for numeric and spaced English formats, and keep transaction-date fallback for genuinely unrecognized text.

## View contracts

Add typed presentation fields rather than exposing database structs to templates:

- Tenant row: existing tenant fields plus `BillingHistory []tenantBillingMonth`.
- Monthly bill: period label, due date, expected amount, paid amount, remaining amount, status/label, and `Payments []rentPaymentDetail`.
- Payment detail: reuse the existing amount/date/description/reference/confirmation-source formatting contract where possible.

The database-backed tenant route populates this projection. The legacy file fallback has no allocation model and remains without payment history; the supported application startup path requires MySQL, and the fallback is retained for existing lightweight route tests.

## UI interaction

- Keep the existing tenant table layout and add a keyboard-operable tenant row toggle with `aria-expanded` and `aria-controls`.
- Render the tenant's history in a hidden detail row. The detail contains one monthly summary row for each eligible month, ordered newest first.
- Each monthly summary row is a second keyboard-operable toggle. Its hidden child section contains confirmed payment details or an explicit empty state.
- Both levels start collapsed. Inner monthly toggle events must not accidentally toggle the containing tenant row.
- Keep the implementation server-rendered and progressive: all data is in the initial response; JavaScript only changes visibility and ARIA state. This avoids an extra endpoint and preserves existing auth boundaries.

## Compatibility and migration

- No schema migration and no change to bank ingestion, matching, allocation, or existing dashboard routes.
- Existing `/rent-dashboard` continues to use its selected-month projection and payment detail list.
- Month parsing remains backward compatible: compact English month-year text is additive and does not change existing numeric or spaced-English matches.
- The new tenant-history projection must use the same currency formatting and status labels as the dashboard.
- Preserve the worktree's existing uncommitted changes; implementation must be additive to those changes and avoid broad template rewrites.

## Risks and rollback

- Risk: inconsistent parent/child totals if the view calculates a different source than the dashboard. Mitigation: centralize the projection and test both `paid_amount_cents` and allocation detail behavior.
- Risk: N+1 queries with roughly 300 rooms. Mitigation: fetch the bounded obligation set and all child details in batched queries, then group in memory.
- Risk: nested row click bubbling. Mitigation: use explicit toggle selectors and stop propagation for inner toggles; verify with browser interaction tests.
- Rollback is code-only because no migration or financial records are changed. Reverting the view projection and template restores the previous tenant page without reversing allocations.
