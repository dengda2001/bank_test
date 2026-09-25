# Implementation plan: tenant prepayment ledger

1. Add schema migration, model, ledger-kind validation, and tests proving prepayment consumes a bank source but does not increase rent paid.
2. Add the explicit overpayment split to batch confirmation with one transaction and idempotency coverage; prove no partial write when the credit or payer relation fails.
3. Add owner-scoped credit balance read models and visible tenant/transaction credit state.
4. Add manual apply-to-rent service and UI. Keep available balance derived from effective prepayment shares under a credit/source lock.
5. Extend full-source and exact-share revoke to keep credit balances correct. Add race/retry and historical-audit tests.
6. Run focused tests, full `go test ./cmd/truelayer-demo`, `go vet ./cmd/truelayer-demo`, and disposable MySQL tests if the local DB becomes reachable.

Gate before the dependent match-review UI: migration, ledger tests, and overpayment service contract must pass. Rollback point: disable new overpayment actions while preserving additive schema and existing credit rows; do not delete audit history.
