# Codebase findings

- `cmd/truelayer-demo/main.go` builds `MatchURL` only for income rows, so expense rows have no action today.
- `cmd/truelayer-demo/transactions.go` derives both income and expense row badges from `payment_transactions.match_status`; bank debits ingest as `unmatched`.
- `cmd/truelayer-demo/expenses.go` stores property and optional room attribution in `manual_expenses`. `createExpense` inserts a new synthetic debit after storing the expense, so it cannot be reused directly for a bank debit.
- `cmd/truelayer-demo/expense_invoices.go` validates PDF/JPEG/PNG/WebP files up to 8 MB, owns invoice replacement history, and scopes downloads by account.
- `cmd/truelayer-demo/rent_workspace.go` and `rent_workspace_page.go` use `manual_expenses` for property/room costs. `cmd/truelayer-demo/obligations.go` counts debit transactions in its overall expense summary. A linked bank debit needs one expense fact but no second payment transaction.
- `cmd/truelayer-demo/transactions.go` ingests duplicate bank keys with `OnConflict DoNothing`, preserving a later manually linked status on resync.
- `prdfile/design.md` defines property-only shared expenses as valid and excludes them from room-level allocation.
- `migrations/` is the SQL schema source. The flat runner has strict no-comments and retry caveats described in `.trellis/spec/backend/database-guidelines.md`.
