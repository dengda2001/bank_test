# Commit plan

## Proposed commit

`fix: include multi-part payer tenant suggestions`

- `.trellis/spec/backend/transaction-auto-match.md`
- `.trellis/tasks/09-26-multi-name-tenant-suggestions/` (task.json, prd.md, context manifests, this plan)
- `cmd/truelayer-demo/transaction_tenant_suggestions.go`
- `cmd/truelayer-demo/transaction_tenant_suggestions_regression_test.go`

## Other dirty paths excluded from this commit

- `.trellis/tasks/09-26-match-drawer-prepayment-and-green-cards/`
- `.trellis/tasks/09-26-tenant-find-transaction-match/`
- `cmd/truelayer-demo/collection_settle_form.go`
- `cmd/truelayer-demo/main.go`
- `cmd/truelayer-demo/rent_workspace_page.go`
- `cmd/truelayer-demo/transaction_handlers.go`
- `cmd/truelayer-demo/transaction_match_review.go`
- `cmd/truelayer-demo/transaction_match_suggestion_test.go`
- `cmd/truelayer-demo/web/static/css/pages/entity-drawers.css`
- `cmd/truelayer-demo/web/static/css/pages/rent-workspace.css`
- `cmd/truelayer-demo/web/static/css/pages/transaction-match-review.css`
- `cmd/truelayer-demo/web/static/js/transaction-match-review.js`
- `cmd/truelayer-demo/web/templates/pages/rent-workspace.html`
- `cmd/truelayer-demo/web/templates/partials/collection-settle-form.html`
- `cmd/truelayer-demo/web/templates/partials/tenant-form-drawer.html`
- `cmd/truelayer-demo/web/templates/partials/transaction-match-review-drawer.html`
- `cmd/truelayer-demo/workspace_alignment_test.go`
- `cmd/truelayer-demo/workspace_shell.go`
- `cmd/truelayer-demo/tenant_receipt_finder.go`
- `cmd/truelayer-demo/tenant_receipt_finder_test.go`
- `cmd/truelayer-demo/web/static/css/pages/tenant-receipt-finder.css`
- `cmd/truelayer-demo/web/static/js/tenant-receipt-finder.js`
- `cmd/truelayer-demo/web/templates/partials/tenant-receipt-finder.html`

Before staging, check the working tree again and stage only the proposed paths.
