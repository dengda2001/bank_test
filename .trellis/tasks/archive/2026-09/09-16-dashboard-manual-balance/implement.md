# Implementation Plan

1. Add the Dashboard row action and the confirmation prompt, preserving the
   current period/filter/pagination values in its return URL.
2. Register and implement `POST /rent-dashboard/settle`; validate only the
   obligation ID at the HTTP boundary and derive all settlement facts in the
   service.
3. Extract the shared allocation-in-transaction path if needed so manual
   transaction creation and allocation share one database transaction.
4. Add `settleRentObligation` with row locks, exact-balance calculation,
   `manual_balance` transaction creation, confirmed rent allocation, and
   projection updates. Exclude the source from automatic reconciliation.
5. Add regression coverage for template visibility/confirmation, handler
   method/auth failures, exact transaction/allocation facts, repeat/concurrent
   calls, cross-user isolation, and full test-suite compatibility.

## Validation

```sh
go test ./cmd/truelayer-demo -run 'ManualBalance|Dashboard' -count=1
go test ./...
go vet ./...
```

## Rollback

- Revert the feature commit to remove the entry point for any new action.
- Existing manual-balance allocations remain auditable and can be revoked using
  the existing transaction-revoke workflow; no schema rollback is needed.
