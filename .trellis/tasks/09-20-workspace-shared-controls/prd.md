# Workspace month and input control consistency

## Goal

Unify dashboard month navigation, workspace date/month pickers, select controls and search-field clear actions.

## Requirements

1. The dashboard month control follows the transaction-matching calendar style and has previous/next month controls at every viewport width.
2. Every date/month input uses the transaction-matching calendar's rounded visual style and interaction, including property, room and tenancy pages.
3. Every workspace select has the light button-like system style and opens an anchored popup from the clicked control.
4. The clear action on populated search inputs has a larger clickable area and remains keyboard accessible.

## Constraints

- Keep server-rendered forms and existing field names/values as the source of truth.
- Preserve existing query behavior and page-specific control behavior.
- Keep a no-JavaScript native-control fallback.
- Respect the existing 44px narrow-screen touch target.
- Do not alter the unrelated workbook `收租明细_Rosewood_20260916.xlsx`.

## Acceptance criteria

- Dashboard has previous, current month picker and next controls on desktop and narrow screens; links preserve other dashboard filters.
- All workspace `date` and `month` inputs receive the same styling and calendar behavior without duplicate initialization.
- Select popups open adjacent to the triggering control, use the light system appearance and support pointer and keyboard selection.
- Select changes update the original native control and existing form listeners continue to work.
- Search clear actions have at least a 44px target on narrow screens, clear the value and retain focus.
- Controls without JavaScript remain usable through their native HTML behavior.
