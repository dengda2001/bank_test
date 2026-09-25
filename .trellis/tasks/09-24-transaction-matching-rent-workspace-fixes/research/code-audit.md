# Code audit (2026-09-24)

## Matching and rent

- Review projection, month evidence, and existing allocations: `cmd/truelayer-demo/transaction_match_review.go`.
- Review partial and client state: `web/templates/partials/transaction-match-review-drawer.html`, `web/static/js/transaction-match-review.js`, `web/static/css/pages/transaction-match-review.css`.
- Batch rent contract: `.trellis/spec/backend/transaction-match-batch.md`; identity contract: `.trellis/spec/backend/transaction-auto-match.md`.
- The date-inferred period has `Explicit=false` in `transaction_period.go`, but the month-card template prints “描述提及” for every highlighted row.
- `handleTransactionAction` redirects with generic `transaction_action_failed` for most defer failures, and the review JS reduces all error redirects to “暂不处理未保存，请重试”. The exact user's failure cannot be established from code alone.
- Current `payment_allocations` supports rent, deposit, and other income, but has no tenant-prepayment kind or later application contract. Rent validation caps a rent allocation at the obligation's unpaid amount.
- `transactionMatchReviewForMonthWithOrigin` preselects only a strict identity or source's existing rent allocation; the `remember_tenant_id` select is rebuilt empty until the user chooses it.

## Lists, invoices, and rooms

- The home queue's `rentWorkspacePendingTransactions` excludes deferred income. `applyTransactionFilters` expands the list's pending filter without that exclusion and includes unmatched expenses when direction is not specified.
- The transaction list is an inline Go template in `transaction_list_page.go`, with direction-tinted desktop rows and mobile cards.
- The dashboard metric in `web/templates/pages/rent-workspace.html` has only percentage text, while the same page already uses a progress bar for rows.
- Expense creation uses `r.ParseForm()` and `persistExpenseRecord` in `main.go`/`expenses.go`; the separate invoice route parses multipart and requires number, vendor, date, and amount. The invoice table has one current file plus replacement history.
- `validateRoomRentPlanCommand` rejects zero rent; the HTTP route sends a broad `rent_plan_invalid` message. Saving a new plan from a later month replaces the future timeline while preserving earlier months, subject to locked financial facts.
- Property and room deletion guards in `landlord_domain_operations.go` preserve child and financial records. The room edit drawer currently implies it deletes financial history, which contradicts the guard.
- `deactivateProperty` and `deactivateRoom` currently only set `status=inactive`; `ensureMonthlyRentFacts` selects active room rent plans without filtering the room/property status. Therefore deactivation alone does not end future rent generation. Guidance must not imply otherwise.

## Specific data availability

- Both IE references are absent from repository fixtures.
- Arslan Arshad is present in the Rosewood fixture with an end date of 2026-08-31, but the fixture is not evidence of the live room plan.
- A read-only attempt to connect to the local MySQL DSN from `.env` failed with `ERROR 2003`; no live records were queried or changed. Recheck when the database is available.
