# Implementation plan: workspace interaction follow-up

## Ordered slices

### 1. Contextual drawer hosts and safe return contexts

- Reuse the expense drawer on `/transactions` and preserve list query state.
- Reuse the cash-receipt flow on `/transactions` and tenant detail, including preview, return-to-edit, success and validation-error paths.
- Reuse the room-create form on property detail with the property preselected and validated.
- Confirm tenant detail edit remains a local drawer and keeps tenant/history context; preserve the existing uncommitted template correction.
- Keep standalone cash/expense/room routes available for their current list/history uses.

**Risk files:** `cash_receipt_pages.go`, `page_data_routes.go`, `main.go`, `tenant_detail.go`, `billing_page.go`, `workspace_shell.go`, `web/templates/pages/{cash-receipts,property-detail,rooms}.html`, shared form partials.

**Rollback point:** shared partial extraction and allowlisted return handling can be reverted before enabling the page-local links.

### 2. Month filters and clear controls

- Change property and room list period controls from select to month input using the shared calendar.
- Remove the dashboard calendar's visible submit button and make its selected month apply immediately, preserving the remaining dashboard query.
- Remove the gray pointer-hover fill/edge from the search clear button without reducing the hit target or hiding keyboard focus.
- Add clear behavior for the optional room select and ensure a property change cannot leave a disabled room selected.

**Risk files:** `web/templates/pages/{properties,rooms,rent-workspace}.html`, `web/static/css/workspace-controls.css`, `web/static/js/workspace-controls.js`, `web/static/css/pages/rent-workspace.css`.

**Rollback point:** each calendar/control adjustment is independent and does not require route or schema changes.

### 3. Transaction list matching action

- Split the desktop table's status and operations columns.
- Expose “匹配流水” for directly matchable pending/unmatched rows and reuse the existing tenant/month options and confirmation endpoint.
- Keep unmatched rows visible in pending/unmatched scope; retain a detail action for rows without direct options.
- Keep mobile card actions consistent with the new desktop operation label and status semantics.

**Risk files:** `billing_page.go`, `transactions.go`, transaction row view-model assembly and focused page CSS.

### 4. Dashboard processing overlay

- Replace the large inline-expansion control with a compact trigger and floating panel.
- Keep details, tenant/month selectors, remember-payer checkbox, match and defer actions inside the panel.
- Keep the visible remember-payer control next to the match action and make handler parsing honor both checked and unchecked values.
- Add dismissal, focus return and narrow-screen positioning without changing the queue's server-side fill behavior.

**Risk files:** `rent_workspace_page.go`, `web/templates/pages/rent-workspace.html`, `web/static/js/`, `web/static/css/pages/rent-workspace.css`, `main.go`.

## Acceptance review

- Walk through transaction cash and expense drawer open/close and their submit/error returns.
- Walk through tenant-detail cash entry and verify tenant preselection and the preview/save return path.
- Open room creation from property detail; verify preselection and return to the same property.
- Confirm tenant edit stays in the detail-local drawer.
- Check month selection on home, properties and rooms; verify query context is retained and no month submit button remains on home.
- Check a pending/unmatched transaction has a visible matching action and separate status/action cells.
- Open and dismiss dashboard processing panels at desktop and narrow widths; confirm cards do not expand and the checkbox's submitted state is correct.
- Clear the expense room selection and hover/focus the search clear action.

Tests are not added or run by this plan unless the user asks for test/implementation verification. If the user later requests verification, follow the project browser-verification workflow; browser checks require a running app with a configured database.

## Review gates and rollback

1. Review safe return-context handling before enabling POST actions on additional hosts.
2. Review the transaction status/action table at desktop and mobile breakpoints.
3. Review the floating dashboard panel for clipping and focus return.
4. Keep the pre-existing workbook and unrelated architecture files untouched.

## Implementation status

- [x] Implemented the contextual drawers, calendar controls, transaction matching action, dashboard floating panel, and clear controls.
- [x] Ran `gofmt` on changed Go files and `git diff --check`.
- [x] `go build -o /private/tmp/bank-truelayer-demo ./cmd/truelayer-demo` passed with a task-local Go build cache. Starting that binary with `TL_ENV=invalid` reached config validation without an embedded-template parse error.
- [ ] Browser interaction and responsive visual walkthrough not run.
- [ ] Tests not added or run, following the active instruction not to run tests unless requested.
