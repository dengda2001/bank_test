# Implementation plan — tenant billing and payment hierarchy

## Ordered checklist

1. Before editing, load `trellis-before-dev` and the backend package guidance; re-check the current worktree diff so existing monthly dashboard changes remain intact.
2. Add typed tenant-history and monthly-bill view models, reusing the existing payment detail formatter and status rules.
3. Implement a bounded obligation-service query for the current month and previous two months. Ensure eligible monthly obligations lazily, exclude months outside each tenant's billing validity, and preserve zero-payment obligations.
4. Implement batched loading/grouping of confirmed allocations joined to payment transactions. Scope every query by the authenticated user and the target obligations; avoid N+1 queries.
5. Integrate the projection into the database-backed `/tenants` handler without changing tenant create/edit behavior. Leave legacy file-mode rows valid with an empty history projection.
6. Extend the tenant template with nested, initially collapsed tenant and month toggles; add keyboard/ARIA behavior and mobile-safe styling. Prevent nested toggle bubbling.
7. Extend `parseReferencedPeriod` with compact English month-year text (`JULY26`, `SEP26`) without changing existing format precedence or transaction-date fallback.
8. Add regression tests for: three-month ordering, one parent per month, three payments summed under one month, zero-payment month, excluded pre-start/end months, unconfirmed payments excluded, user isolation, duplicate/refresh non-mutation, and compact English month parsing.
9. Run formatting, unit tests, vet, and a real-browser interaction check for both nesting levels and keyboard activation. Do not trigger live bank refresh or write real financial data.
10. Review the final diff against this PRD and design after the user approves the planning artifacts, then complete the quality gate and commit the implementation.

## Validation commands

```sh
gofmt -w cmd/truelayer-demo/*.go
go test ./...
go vet ./...
```

For UI verification, run the demo against an isolated test database, open `/tenants`, and verify:

- tenant history is collapsed initially;
- Enter/Space expands and collapses the tenant history;
- month summaries remain collapsed until clicked;
- Enter/Space expands only the selected month's payment details;
- three payment rows appear under one month and no unconfirmed row appears.

## Risky files and checkpoints

- `cmd/truelayer-demo/tenants.go` / `obligations.go`: accounting projection and query boundaries.
- `cmd/truelayer-demo/main.go`: route data assembly and the large server-rendered tenant template.
- `cmd/truelayer-demo/main_test.go`: existing tests and new nested view assertions.
- No migration file is expected. If implementation discovers a schema gap, stop and revise `design.md` before adding one.

## Review gate

Do not run `task.py start` or begin implementation until the user has reviewed and approved `prd.md`, `design.md`, and `implement.md`.
