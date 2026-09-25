# Implementation plan: match review interaction and feedback

1. Add focused tests for month evidence copy, unique/shared payer defaults, owner-scoped room occupant options, and defer error codes.
2. Add source/history detail overlay with focus return and preserved review draft state.
3. Replace small-only feedback with a floating accessible notice; make defer report the actual failure and retry cleanly.
4. Add blue suggestion treatment, property/room/tenant selection, and actionable missing-bill copy.
5. After `09-24-rent-prepayment-ledger` is validated, add the explicit prepayment split control and submit fields. Verify bank source, bill, and credit balances after confirmation.
6. Run focused tests and `go test ./cmd/truelayer-demo`; browser-check desktop/mobile details, tenant lookup, invalid draft, defer failure/success, and overpayment.

Rollback points: the detail overlay and lookup are read-only and can be reverted separately. Keep the prepayment UI disabled if its service contract or migration is not ready; never remove existing allocation audit rows to roll back UI behavior.
