# Rent overpayment and tenant prepayment ledger

## Goal

Allow an explicitly chosen overpayment match without overstating a month's rent or losing any part of the bank receipt.

## Requirements

- A bank receipt of €620 against €600 unpaid rent can be explicitly split into €600 rent and €20 pending prepayment for the selected tenant.
- A tenant prepayment is tied to its bank source, owner, tenant, currency, and audit history. It must not count as paid rent until applied to a specific obligation.
- The bank source's allocated/remaining projection accounts for the whole €620 while the monthly rent bill remains capped at €600.
- A pending prepayment can later be applied to rent by an explicit user action; no future bill consumes it automatically. Any application must be owner-scoped, atomic, idempotent, and traceable. Refund handling is deferred.
- A void/reversal never silently rewrites prior rent or prepayment history.

## Acceptance Criteria

- [ ] Explicit €620 overpayment match leaves the selected €600 bill paid and creates €20 tenant prepayment, with no bank-source remainder and no collection percentage above 100%.
- [ ] Reject source over-allocation, wrong currency, cross-user tenant/obligation, duplicate request, and conflicting retries without partial writes.
- [ ] Prepayment balance is visible by tenant and source; its future application cannot exceed the live balance. There is no refund action in this task.
- [ ] Existing rent/deposit/other-income allocations and historical snapshots remain valid.

## Product decisions

- The user has chosen €600 rent plus €20 tenant pending prepayment, with manual future application. Refund functionality is out of scope for this task.
