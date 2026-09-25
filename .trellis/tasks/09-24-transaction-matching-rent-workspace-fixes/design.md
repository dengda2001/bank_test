# Integration design: matching and rent workspace fixes

## Task boundaries

| Child | Deliverable | Dependency |
| --- | --- | --- |
| `09-24-rent-prepayment-ledger` | Explicit tenant prepayment and manual application contract | None; establish this before overpayment UI |
| `09-24-match-review-ux` | Review overlay, lookup, feedback, identity defaults, month evidence, overpayment control | Prepayment service and migration for overpayment control |
| `09-24-transaction-list-dashboard` | Direction markers, count scope, paid-rent progress | None; coordinate shared list/review URL parameters |
| `09-24-expense-invoice-upload` | Generic multi-file expense attachments in create and file-management flows | None |
| `09-24-room-lifecycle-guidance` | Room succession, zero-rent copy, property/room soft deletion | None; match review may link to room plan |

## Shared contracts

- `payment_transactions` remains the immutable bank source. Effective `payment_allocations` must never exceed source amount. Rent obligation paid amounts include only `rent` allocations; tenant prepayments are a separate liability-like balance until applied manually.
- Tenant identity requires an unambiguous saved payer relation or unique exact official-name evidence for preselection. Room and fuzzy-name suggestions only navigate the manual evidence view.
- Dashboard pending queue means non-deferred pending income in the selected bank-arrival month. The dashboard count, preview, and dedicated destination filter share this meaning; the general transaction history remains broader.
- Historical room plans, paid obligations, legacy invoice files, and allocation audit rows remain available. Soft deletion only hides an asset from daily lists and new operations; historical views may still resolve it.

## Data and UI flow

Bank receipt → owner-scoped review → property/room/tenant evidence lookup → draft rent shares → optional explicit prepayment split → one locked, idempotent confirmation → updated obligation/source/credit read models. An existing credit → explicit later bill selection → locked credit reclassification and audit trail. No automatic month application or refund flow.

Expense create → bounded multipart parse → expense and zero or more attachment files in one DB transaction. Existing expense → add, download, or remove individual attachments. No invoice metadata is fabricated. Legacy invoice files remain readable.

## Compatibility and operational limits

The prepayment, expense-attachment, and asset soft-deletion migrations are additive. The matching UI must not offer prepayment until its schema and service are deployed. Local MySQL is currently unreachable (`ERROR 2003`), so specific IE and Arslan record checks remain a read-only verification gate when data becomes available. Do not mutate those records based on fixtures. Browser verification must include desktop and narrow mobile widths.
