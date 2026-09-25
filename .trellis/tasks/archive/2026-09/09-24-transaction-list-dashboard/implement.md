# Implementation plan: transaction list and dashboard clarity

1. Add a failing regression for the home queue count and its destination query, including an active deferral and an unmatched expense.
2. Share or reproduce the exact eligible queue predicate in the destination list filter; keep the broader history pending filter distinct.
3. Add the separate desktop direction column and mobile marker, remove tint backgrounds, and update layout assertions.
4. Add the paid-rent card progress bar using the existing bounded style helper.
5. Run focused Go tests, `go test ./cmd/truelayer-demo`, `go vet ./cmd/truelayer-demo`, and browser checks at 1440px and 390px.

Rollback point: this child has no schema/data changes; revert the template, CSS, query filter, and associated tests together if the queue predicate is wrong.
