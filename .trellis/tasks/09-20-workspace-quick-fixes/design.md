# Design: workspace quick fixes

This child task implements requirements 2, 5, 9 and 15 from the parent assessment.

## Contracts

- Keep filter values and server query semantics unchanged; fix only the bill template's selected-option conditions.
- Default values change only when a page-size parameter is absent. Continue honoring currently valid explicit page sizes and do not expand list pagination to pages that are currently unpaginated.
- “搜索” is action/disclosure copy. It does not rename query parameters or alter filter behavior.
- Remove shared topbar widgets and their data-plumbing/database lookup. Keep page-local search and dashboard queue behavior.

## Likely implementation areas

- `rent_collection_pages.go` — bill status selection.
- `dashboard_filters.go`, `tenant_detail.go`, `transactions.go` and their page-size controls — defaults and selectable values.
- `workspace_shell.go` and `web/templates/partials/workspace-nav.html` — shared topbar state and markup.
- Page templates with visible filter submit or disclosure labels — copy only.

## Compatibility

No database/schema change. Query parameter names, filtering semantics, page-local search and page routes remain unchanged. The shared pending-count query is removed only if it serves the removed topbar control exclusively.
