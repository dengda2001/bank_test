# Allocation share revocation

## Goal

Correct a paid month without undoing other shares of the same bank transaction.

## Requirements

- Revoke only one effective, owner-scoped rent allocation by allocation ID. Preserve the bank source and all other effective shares.
- Show enough server-provided data for a confirmation dialog: bank source, payer, date, description, selected tenant, month, and share amount.
- Atomically update the allocation, affected rent obligation, source match projection, and append-only audit action.
- A newly free source balance becomes eligible for manual dashboard review when the source is in the displayed month and is not currently deferred.
- Keep existing full-source revoke semantics separate.

## Acceptance Criteria

- [ ] Revoking one share of a source split across tenants or months leaves every other share effective.
- [ ] A fully matched source becomes partial or unmatched as appropriate; affected obligation paid/status decreases by exactly the share amount.
- [ ] A foreign, stale, already voided, or non-rent allocation is rejected without changing state.
- [ ] Double submit is idempotent or safely rejected with no double decrement; an audit record identifies the exact allocation operation.
- [ ] Eligible freed source appears in the home manual queue.

## Dependency

None. The transaction review/search UI child depends on this POST contract and evidence row allocation ID.
