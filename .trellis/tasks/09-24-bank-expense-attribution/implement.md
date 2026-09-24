# Implementation plan: bank expense attribution

1. Add a migration for `manual_expenses.payment_transaction_id`, uniqueness, ownership-safe linkage, and manual-expense backfill. Add the model field and migration contract tests.
2. Add an account-scoped service to load/create/edit the expense fact for an existing bank debit. Lock the source and expense, validate property/room and immutable bank fields, update status, and cover repeated saves and cross-account attempts.
3. Reuse invoice validation and replacement in the association transaction. Test optional upload, invalid file rollback, replacement history, and download ownership.
4. Load expense attribution data into transaction list/detail rows and the query-backed drawer. Add desktop/mobile actions, saved state, dependent asset selection, and form error recovery.
5. Verify the `/expenses`, room/property, and operating-net projections include the linked expense once and the bank transaction once. Verify bank resync preserves the linked state.
6. Run focused Go tests, the package test suite, migration checks, and browser checks at desktop/mobile sizes. Inspect `git diff --check` and review against backend database and frontend responsive/status specs.

## Risk and rollback points

- Migration is additive but links older synthetic transactions; inspect those records before backfill and make the SQL retry-safe.
- Invoice bytes and attribution must commit together so a failed file cannot leave a linked row without its intended document.
- Avoid changes to rent allocation code. Recheck income and expense action routing after introducing expense-specific status handling.
