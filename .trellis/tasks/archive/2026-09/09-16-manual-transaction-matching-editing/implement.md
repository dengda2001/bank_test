# Implementation plan: manual transaction matching and limited match editing

## Ordered work

1. Remove calls to `reconcileTransactions` from billing rendering, dashboard
   rendering, and legacy import. Refactor billing-row enrichment to calculate
   suggestions in memory only.
2. Extend page row/view data with the suggested target and the safe-rematch
   capability plus independent tenant/month options. Update the billing
   template to render `一键匹配`, separate tenant and rent-month selectors for
   eligible `修改匹配` rows, and revoke/reclassify guidance for ineligible rows.
3. Adapt the explicit confirmation handler/service to resolve and validate a
   user-scoped rent target on every POST. Make rematch accept tenant ID plus
   rent month, resolve the obligation server-side, and keep the POST-only
   rematch route.
4. Extract the in-transaction revoke projection work needed to replace one
   rent allocation atomically, then call the existing allocation ledger writer
   for the equivalent replacement. Preserve void rows and actions.
5. Add focused template/handler tests plus MySQL integration tests for manual
   matching and rematching behavior. Update existing tests that asserted the
   former auto-reconciliation behavior.

## Review gates

- Before edit: load Trellis package/layer guidelines with `trellis-before-dev`.
- During edit: preserve row-locking, account predicates, and the existing
  allocation validation path; do not add editable bank transaction fields.
- After edit: run `gofmt`, targeted Go tests, `go test ./...`, `go vet ./...`,
  and the Trellis quality check. Inspect the diff for accidental changes to
  unrelated work.

## Risk and rollback

The highest-risk point is the rematch transaction. A failed validation or
insert must roll back the old allocation void, obligation projections, and
source projection together. If the change must be rolled back, restoring the
previous code re-enables only the old automatic behavior; existing allocations
remain intact because this task has no destructive migration.
