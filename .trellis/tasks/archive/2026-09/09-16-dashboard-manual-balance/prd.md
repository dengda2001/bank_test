# Add dashboard manual balance action

## Goal

Let a landlord settle one tenant's remaining rent for the selected Dashboard
month by creating an auditable manual-income transaction and allocating its
exact amount to that month's rent obligation.

## Confirmed Facts

- `/rent-dashboard` renders every tenant's current-month payment status and
  remaining rent amount in `cmd/truelayer-demo/dashboard.go`.
- Existing cash receipts only represent `现金` and therefore cannot fulfil the
  requested visible income type `手动平账`.
- The existing ledger allocates income transactions to rent obligations, and
  validates the amount within a locked database transaction.

## Requirements

- Add a `一键平账` action beside the status in each Dashboard row with an
  outstanding balance.
- On success, create an income transaction visibly identified as `手动平账`, for
  the exact remaining balance of that row's selected rent month, and allocate
  it to that rent obligation.
- Do not show the action for fully paid or voided obligations.
- Preserve account isolation, EUR-only validation, idempotency, and the
  existing balance guard under concurrent requests.
- Show a browser confirmation prompt before submitting the settlement action.

## Acceptance Criteria

- [ ] A partial or unpaid row exposes the action beside its status; a paid or
  voided row does not.
- [ ] The action creates one `手动平账` income transaction for exactly the
  current outstanding amount and marks the selected month's obligation paid.
- [ ] Repeated or concurrent requests never overpay the obligation or create
  duplicate manual-balance transactions.
- [ ] Another user's obligation cannot be settled by this action.
- [ ] The full Go test suite passes.

## Resolved Decisions

- The action uses a browser confirmation prompt to protect against accidental
  financial records. Confirming it submits the settlement once.

## Notes

- Keep `prd.md` focused on requirements, constraints, and acceptance criteria.
- Lightweight tasks can remain PRD-only.
- For complex tasks, add `design.md` for technical design and `implement.md` for execution planning before `task.py start`.
