# Manual transaction matching and editing

## Goal

Make matching a deliberate action from the bank-transaction page, and let a
landlord correct a transaction's matched tenant and rent month even after a
rent allocation has been made.

## Confirmed facts

- `transactionService.reconcileTransactions` currently writes matching
  decisions and rent allocations automatically. It runs while rendering both
  `/billing` and `/rent-dashboard`, and after legacy import.
- `/billing` already renders candidate, tenant, and rent-month controls, but
  its existing candidate path is reached after automatic reconciliation and it
  has no transaction-editing form.
- Confirmed rent allocations affect obligation paid/status projections. The
  existing revoke flow preserves allocation audit history; changing a match
  must likewise revoke/supersede old allocations rather than rewriting them.
- The bank source fields include provider identifiers, amount, currency, raw
  payload, and a stable import key. Those facts are part of deduplicated bank
  ingestion and must not be silently mutated as ordinary display edits.

## Requirements

- Stop background matching from automatically creating or changing rent
  allocations. Opening `/billing` or `/rent-dashboard`, and importing legacy
  data, must not auto-match a transaction.
- Add an explicit one-click matching action to the transaction page. A row may
  still present its calculated candidate/period information, but an allocation
  is created only after the user submits the action.
- Allow a matched transaction to change its matched tenant and rent month from
  the transaction page. Arrival date and all bank-originated transaction fields
  remain unchanged.
- The match-edit control uses two independent selectors—one for tenant and one
  for rent month—without rendering a tenant × month Cartesian-product list.
- Preserve account isolation, transaction/allocation auditability, EUR and
  rent-balance protections, and the existing correction/revoke path.
- Limit direct match editing to a transaction with exactly one effective rent
  allocation. A split or mixed-purpose transaction remains on the existing
  revoke-then-reclassify correction path.

## Acceptance Criteria

- [x] Rendering Dashboard or the transaction page and importing legacy data do
  not create a new allocation or change a transaction's match state.
- [x] A valid candidate can be deliberately matched from `/billing` in one
  user action; rows that need a month or tenant still request that missing
  decision explicitly.
- [x] A previously matched row exposes a match-edit action. Saving it replaces
  the affected rent allocation safely and retains the old allocation audit
  history.
- [x] The match-edit action presents separate tenant and rent-month selectors.
- [x] Edited matches are account-scoped and validated for EUR, tenant ownership
  and outstanding rent balance; original bank identity/deduplication facts
  remain unchanged.
- [x] Existing matched rent, partial allocation, revocation, and cross-account
  protections remain correct.

## Out of scope

- Changing transaction date, payer, description, reference, amount, currency,
  provider transaction ID, stable import identity, or raw bank payload.
- Automatically moving an existing rent allocation without an explicit manual
  matching action.

## Resolved decision

- Split or mixed-purpose transactions do not expose direct match editing. The
  match-edit action moves only one effective rent allocation, and retains the
  voided allocation plus the new allocation as audit history.
