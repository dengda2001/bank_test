# Bank expense attribution

## Goal

Attribute a bank expense to a property or room, record what it was for, attach an invoice, and correct that attribution later.

## Confirmed existing behavior

- The transaction list has bank debits but no expense attribution action.
- Manually entered expenses already support a property, optional room, category, and a separate invoice upload/version history.
- Creating a manual expense also creates a synthetic payment transaction. Attributing an existing bank debit must avoid another financial transaction.

## Requirements

- Show an “关联房间” action on an unattributed bank expense. It opens a drawer in the transaction page.
- The drawer shows the immutable bank transaction details, requires a property, allows an optional room within it, and records an expense purpose/category.
- The drawer offers optional invoice upload using the existing accepted formats and size limit.
- Saving changes the transaction’s visible state to “已关联” and displays its associated property/room and expense purpose.
- A linked expense has an edit action that reopens the drawer with the saved values; edits may change attribution, purpose, and invoice.
- Validate account ownership and room/property consistency, and retain filter context after save or error.
- Do not duplicate the bank debit or count it twice in financial reporting.

## Acceptance criteria

- [x] An unlinked bank expense opens the drawer and saves a valid attribution.
- [x] A linked bank expense displays “已关联” and reopens in editable form with its saved values.
- [x] Property/room selections are scoped to the signed-in account, and a room cannot be paired with another property.
- [x] Invoice upload and replacement follow the existing validation and history behavior.
- [x] Repeated saves and edits do not create duplicate financial transactions or duplicate expense records.
- [x] The transaction list, expense list, and reporting agree on the new attribution.

## Confirmed product decision (2026-09-24)

- The user confirmed that a property is required and a room is optional. Property-only expense attribution is valid for shared costs; it must not be assigned to an arbitrary room.
