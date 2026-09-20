# Workspace quick fixes

## Goal

Implement the first approved slice of the workspace remediation plan: fix the receivable-bill selected state, default existing paginated lists to 10 rows, label filter actions “搜索”, and remove shared topbar search/count controls.

## Requirements

1. A bill status selected in the filter remains selected after submitting the filter. `all` and `unpaid` are distinct options.
2. Every existing paginated list defaults to 10 rows. Explicit supported page sizes continue to work, and 10 is an available choice.
3. Every visible filter submit/disclosure action uses the label “搜索”.
4. Remove the shared top-right search input and adjacent pending-review count/action from the workspace chrome. Keep breadcrumbs, the dashboard pending queue and page-local search controls.

## Acceptance criteria

- Bill status query and rendered selected option agree for each supported status.
- Workspace, bills, transaction and tenant-history pagers default to 10.
- Existing explicit page-size query values are preserved within their existing validation rules.
- Filter actions/disclosures no longer display “筛选” or equivalent old action copy.
- Shared topbar search and pending-review count/action are absent, and no shared count query runs solely to populate that chrome.
- Page-local tenant/transaction/object search remains available.

## Out of scope

- Numbered pagination links (requirement 3 of the parent plan).
- Custom select/calendar/search-clear UI.
- Removal of sidebar cash-receipt or expense links.
- Business-code changes for contextual drawers or transaction matching.
- Adding or running tests in this slice unless the user separately requests test execution.
