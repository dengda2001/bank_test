# Design: transaction list and dashboard clarity

## Boundaries

The transaction list remains a server-rendered Go template in `transaction_list_page.go`. The dashboard remains the embedded `rent-workspace.html` page. Neither change writes financial data.

## Direction and count

- Add a narrow desktop direction column before the amount column. Use a visible arrow/icon plus the accessible text “收入” or “支出”; color is an additional cue. Remove direction-specific row and card background rules.
- Keep the mobile direction marker in the card header, using the same color/icon vocabulary.
- Define one owner-scoped eligible home-queue query, including income, the selected arrival month, pending match statuses, and exclusion of current deferrals. The home count, three-card preview, and the “view all to match” link use this scope. The general transaction list retains its broader pending filter, but the dashboard link supplies an explicit queue scope, and that scope uses the same query predicate. State labels explain deferred rows in the broader list.
- The dashboard metric reuses its existing `workspace-progress` bar and bounded `workspaceRateStyle` helper, with the percentage adjacent to the bar.

## Compatibility and checks

Preserve existing list sort keys and filter parameters. Update tests that assert desktop column count/headers, mobile cards, queue count/link equivalence, and CSS. Browser-check at desktop and mobile widths because markup string tests cannot prove table geometry.
