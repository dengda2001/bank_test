# Implementation plan: workspace quick fixes

## Scope

Implement parent requirements 2, 5, 9 and 15. Work is serial in this Codex inline session.

## Steps

1. Locate every existing pagination default and page-size selector; update defaults to 10 and ensure 10 is selectable without removing supported explicit sizes.
2. Fix the bills status template so `all` and `unpaid` each select only their matching option.
3. Rename visible filter submit and disclosure actions to “搜索”, preserving form fields and URL state.
4. Remove the shared topbar search and pending-review count action, then remove fields and lookup code used only by those widgets.
5. Inspect the final diff and search for stale topbar symbols and old filter action labels.

## Validation

- The user requested implementation but did not request test execution; do not add or run tests in this slice.
- Perform static source review and inspect the final diff for unrelated changes.
- Leave the unrelated workbook untouched.

## Rollback points

Keep changes localized to the bill template, pagination defaults/selectors, filter copy and shared workspace shell. Do not mix this slice with numbered pager links or shared UI control refactors.
