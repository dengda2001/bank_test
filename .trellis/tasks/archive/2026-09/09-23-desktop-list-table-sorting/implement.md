# Implementation plan: desktop list table sorting

1. Add test coverage around the shared sort-heading state: first click,
   toggle, active arrow/ARIA state, and query preservation. Run this focused
   test red before changing the helper.
2. Extend the shared heading view model/helper and shared table-heading CSS.
   Attach its links to the homepage, bills, and transaction table headings;
   preserve each page's existing sort contract.
3. Add typed sort filters, validation, URL construction, stable comparators,
   view-model links, and template headings for `/properties`, `/rooms`, and
   `/tenants`. Verify route/template tests after this slice.
4. Repeat the same vertical slice for `/cash-receipts` and `/expenses`,
   including drawer/action return context.
5. Add/extend transaction sorting only for remaining visible table columns
   with safe, whitelisted backing data; avoid broadening query scope.
6. Run all package tests and Go formatting. Start the local audit target and
   verify rendered desktop table headings, keyboard links, arrows, and URL
   changes at a desktop viewport; recheck the narrow/mobile contract.
7. Run the Trellis quality check, record any reusable convention in the spec,
   review the full diff, and commit only task-owned changes.

## Validation commands

```sh
go test ./cmd/truelayer-demo -run 'Test.*(Sort|Workspace|Bills|Transaction|Property|Room|Tenant|CashReceipt|Expense)' -count=1
go test ./cmd/truelayer-demo -count=1
gofmt -w <changed-go-files>
scripts/run-audit-local.sh
```

## Risks

- Query-string context is currently distributed among several page-specific
  builders; a missing `sort` field can make a drawer/action appear to reset a
  list.
- Existing workspace rows use in-memory projections, while transactions are
  paginated in SQL; comparators and whitelists must stay at their current data
  boundary.
- Table headers must remain unobtrusive on the frozen mobile card/table
  breakpoints, so changes are scoped to desktop table renderers and existing
  responsive tests are retained.

## Verification

- `go test ./... -count=1` passed.
- `go vet ./...` and `git diff --check` passed.
- The isolated Playwright audit could not start because no MySQL admin client
  was available at its configured local endpoint (`127.0.0.1:53306`); it
  stopped before creating a database or starting the application.
