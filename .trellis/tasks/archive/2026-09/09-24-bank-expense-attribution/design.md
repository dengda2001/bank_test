# Design: bank expense attribution

## Existing boundaries

- `payment_transactions` is the source of bank movements. Bank ingestion is idempotent on `(user_id, stable_transaction_key)` and leaves existing rows untouched on resync.
- `manual_expenses` is the expense fact used by property, room, and operating net views. New manual expenses create a synthetic `payment_transactions` debit.
- `manual_expense_invoices` stores validated private invoice files and replacement history.
- The transaction page is server-rendered, and its rent review drawer uses query state to preserve list filters.

## Persistence contract

Add nullable `payment_transaction_id` to `manual_expenses`, with a unique `(user_id, payment_transaction_id)` index and a foreign key to `payment_transactions`. A linked bank debit remains the one source transaction; its expense fact stores its original amount, currency, date, and description plus the chosen property, optional room, and category. Saving a bank attribution must never create the synthetic debit used by `createExpense`.

The new service locks the account-scoped source and linked expense inside one transaction. It rejects a non-expense or non-bank source, conflicting ownership, mismatched property/room, and a second expense fact for the same source. Create and edit are one idempotent upsert keyed by the source ID. It sets the source `match_status` to `matched`, so the existing 已关联 filter agrees with the visible state. A reimport uses `DoNothing` and preserves that state.

Existing manual expense transactions need a migration backfill to connect their expense facts to the synthetic source rows and mark those source rows linked. Their existing amount and property/room data stay unchanged. The transaction page can then show correct state for both bank and manually entered expenses. The bank attribution drawer is limited to bank debits; the existing manual expense flow remains the creation path for manual payments.

## Invoice contract

Invoice upload is optional at initial association and during edits. When present, reuse the existing size, MIME, filename, and metadata validation. Save association and invoice replacement in the same database transaction. The current invoice becomes historical only after the new one is valid and stored. Download remains account scoped through `/expenses/invoices/{id}`. When no new file is supplied during edit, retain the current invoice.

## UI and routing

The transaction list gets a dedicated query-backed expense drawer (`?expense_link=<transaction id>`), with a clear close URL derived from the active filters. Unlinked bank debits show “关联房间”; linked bank debits show “编辑关联”. The drawer shows immutable bank identity, amount, and date, property and dependent room selects, the expense category, and optional invoice fields/file. Property is required; room is optional to support building-wide costs. On save or validation error, redirect back to the list/drawer with filters intact. The row and mobile card show `已关联`, property/room, category, and invoice presence based on persisted expense data.

The `/expenses` list reuses the same expense fact and invoice, so linked bank expenses appear there without a second debit. The transaction detail page should use the same expense state and action instead of telling users that expenses cannot be classified.

## Compatibility and rollout

- Apply a new numbered SQL migration; do not edit earlier migrations or call AutoMigrate.
- Backfill existing synthetic manual expense links before adding the uniqueness constraint. The migration must be safe to retry after partial DDL application according to the flat SQL runner rules.
- Existing rent allocations are untouched. `matched` on a debit means expense attribution; `matched` on a credit retains rent/allocation meaning.
- On rollback, remove the UI/service code first, then leave the additive link column and expense facts in place to avoid losing attribution or invoice history.

## Domain decision

The user confirmed a required property and optional room. This prevents a shared property cost from being assigned to an arbitrary room.
