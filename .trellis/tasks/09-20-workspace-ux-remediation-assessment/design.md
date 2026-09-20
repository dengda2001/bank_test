# Design: workspace interaction and consistency remediation

This document turns the 15 requested changes into shared contracts. It is an assessment and implementation design; no business code is changed in this planning task.

Related research:

- `research/codebase-findings.md` — requirement-to-code mapping, sizes and risks
- `.trellis/spec/frontend/index.md` — frontend package conventions
- `.trellis/spec/frontend/responsive-conventions.md` — responsive and touch-target rules
- `.trellis/spec/backend/index.md` — backend package conventions
- `.trellis/spec/guides/code-reuse-guide.md` — shared-component decision guide
- `.trellis/spec/guides/cross-layer-thinking-guide.md` — cross-layer data-flow checks

## 1. Scope and invariants

The implementation may change workspace navigation, page-local forms, query defaults, view models and transaction workflow state. It must preserve these contracts:

- Match status, allocation kind and bill payment status remain separate concepts.
- “记住付款人” remains selected by default and optional.
- Cash receipt remains a cash-only rent receipt flow.
- Existing cash-receipt and expense list routes remain available for history, voiding and later edits even after their sidebar entries are removed.
- Every contextual ID submitted from a drawer is revalidated against the current account; preselection is not authorization.
- Return destinations are allowlisted server-side and never accepted as arbitrary URLs.

## 2. Shared UI foundation

### D1. Load shared control assets once through workspace template constructors

`newWorkspacePageTemplate` and `newEmbeddedWorkspacePageTemplate` become the single place that exposes shared calendar, select and search-control assets. Page templates stop injecting duplicate calendar files.

The calendar keeps the existing transaction-matching behavior and visual language. Date and month inputs use the same radius, trigger, anchored popover and mobile geometry on dashboard, property, room, tenancy, tenant, bill, cash, expense and transaction pages.

Initialization must be idempotent because legacy string templates and embedded templates coexist. Disabled and readonly controls remain inert, and controls added when a drawer opens are enhanced without a second page-wide initialization.

### D2. Enhance native selects instead of replacing form semantics

Native `<select>` elements remain the form source of truth. A shared progressive enhancement renders a button-like trigger and an anchored listbox beside the clicked control, then synchronizes the selected value back to the native element and dispatches `input`/`change`.

Required behavior:

- Light system colors and the same radius, height and focus ring as buttons and text inputs.
- Popup anchors to the trigger, flips above it when viewport space requires, and never uses an unrelated dark OS menu in supported browsers.
- Arrow keys, Home/End, Enter/Space, Escape, type-ahead, visible focus and an accessible name work.
- Form submission without JavaScript still uses the native control.
- Existing `change` listeners and `requestSubmit()` behavior continue to work.
- Dynamically filtered room/month options trigger an explicit refresh of the enhanced control.
- Mobile controls meet the existing 44px touch-target rule.

This is the largest UI-foundation item because approximately 42 controls share it.

### D3. Use an explicit clear button for search fields

Search inputs use a shared wrapper with an explicit, labelled clear button. The visible icon can remain compact while its hit box is at least 44px on narrow screens. Clearing sets the input to empty, dispatches `input` and restores focus. Forms do not auto-submit unless that page already has this behavior.

Browser-specific native cancel decorations are hidden only after the explicit button is active, avoiding double clear controls.

### D4. Keep page-local search and remove shared topbar search

Remove `TopSearch`, `PendingReviewCount` and `PendingReviewURL` from the workspace shell and stop their per-page count query. Keep breadcrumbs and page-local search controls. Remove only the cash-receipt and expense links from the primary sidebar; retain their routes and `/more` entries.

## 3. Query state, labels and pagination

### D5. Use one pagination window contract

Introduce a small Go pagination view model shared by workspace, bills, transactions and tenant history:

```go
type paginationItem struct {
    Label     string
    URL       string
    Current   bool
    Ellipsis  bool
}
```

The helper returns first/last pages plus a bounded window around the current page. Previous and next remain visible and disabled at boundaries. Page-specific URL builders continue to own their query parameters so active month, search, sort and status values are preserved.

All currently paginated pages default to 10 rows. Existing unpaginated property, room, tenancy, tenant and expense lists do not gain pagination in this scope.

### D6. Fix bill selected state without changing filtering semantics

The bills template marks exactly the submitted status as selected, including `all`. No service/query change is needed. A query round-trip check protects the visual state.

### D7. Standardize search labels and dashboard month navigation

Visible form actions and filter disclosure controls use “搜索”. The dashboard renders previous month, the shared month picker and next month at all viewport widths. Each link preserves other dashboard query state.

## 4. Shared contextual drawers

### D8. Extract three reusable drawer partials

Create shared partials and focused view models for:

1. Expense creation
2. Cash-receipt preview/save
3. Tenant edit/create

The host page owns whether a drawer is open; the partial owns fields, validation messages and actions. Each POST carries an opaque, allowlisted return context rather than a user-controlled URL.

Recommended return contexts are structured values such as `expenses`, `transactions`, `property:<id>`, `room:<id>` and `tenant:<id>`. The server reconstructs the destination URL and preserved query parameters from validated IDs and known fields.

### D9. Property and room expense context is preselected and constrained

Property detail opens the expense drawer locally with its property selected. Room detail opens it locally with both property and room selected. The UI may lock fixed context, but the server still checks that the room belongs to the property and both belong to the account.

Success returns to the originating detail page. Validation errors render the same page with the drawer open and the submitted values/errors intact.

### D10. Tenant detail owns its edit drawer

`/tenants/<id>?edit=1` loads the shared tenant form plus room options and renders it on the detail page. Successful edits and validation errors both return to the same tenant detail context. Tenant list editing continues to use the same partial in list context.

### D11. Transaction matching hosts cash and expense entry

The transaction page header exposes “现金补录” and “新增支出”. Each opens the extracted existing drawer flow without navigating away. Cash preview remains a two-step operation; expense creation keeps its existing fields. Successful actions return to the same transaction list with active filters and page state preserved.

## 5. Tenant and transaction information

### D12. Tenant search is server-side and URL-addressable

Add a `q` query field to `/tenants`. Case-insensitive matching covers tenant name, display alias, email, payer hints, room label and property label. The page distinguishes total count from filtered count, keeps `q` through edit/detail return links and displays a clear empty state.

The initial implementation may filter the already loaded account-scoped tenant rows because this list is currently unpaginated. If the query is moved into storage later, the URL and view model contract remain unchanged.

### D13. Transaction rows expose bank payer and matched tenant separately

The transaction list view model adds `MatchedTenantName`. Column two is labelled “付款人” and continues to read the bank transaction `payer_name`; a new column displays the resolved matched tenant name or an empty-state dash.

The detail action area exposes a visible match button. The reusable match form uses separate `tenant_id` and `period` controls. Period choices update after tenant selection and the server validates the pair against an open obligation. Existing partial-match, rematch, allocation and revoke behavior remains available.

## 6. Dashboard manual-review workflow

### D14. `deferred` is a first-class transaction workflow state

Add `deferred` as a recognized match status and an auditable transaction action. It is distinct from `ignored`, which keeps its existing “non-rent / no matching required” meaning.

State behavior:

| Current state | Action | Result |
| --- | --- | --- |
| candidate / needs_review / unmatched / partial | 暂不处理 | deferred |
| deferred | match | matched or partial, according to allocation result |
| deferred | open in list | remains deferred until another explicit action |

`deferred` is excluded from the dashboard queue and pending counts, but is included in the transaction list’s all/deferred filters. Existing status storage and the transaction action audit table can hold the new value/action, so no schema migration is expected.

The state transition and audit write occur in one transaction. Repeated defer requests are idempotent or return the already-deferred result without duplicate side effects.

### D15. Dashboard shows a three-item queue with per-item processing

The dashboard query loads the first three eligible pending transactions in stable priority/order. Each rounded card always shows a short summary and a “处理” action. Processing opens a floating panel with payer, arrival time, bank note/reference and amount, followed by separate tenant and month selectors, “匹配” and “暂不处理”.

Both POST actions include the dashboard month and an allowlisted dashboard return context. After a successful action the server redirects back; the next eligible row naturally fills the vacated slot. The page therefore shows up to three current rows without client-side cache reconciliation.

Concurrent handling is resolved by the transaction service: if the row changed after render, the action returns a clear stale-state message and reloads current queue data instead of overwriting the newer result.

**Follow-up clarification:** the panel overlays the queue without reflowing its cards; the remember-payer checkbox sits beside the match action and its submitted state is honored.

### D16. Reuse one transaction-match form contract

Transaction detail and dashboard cards use the same match-form view model, option generation and server validation. Templates may differ in layout but do not duplicate tenant/month eligibility rules.

## 7. Compatibility and risk controls

- No schema migration is planned. Implementation must stop and revise the design if the current status constraints reject `deferred`.
- Native form controls remain usable without JavaScript; enhanced selects and calendars are progressive enhancements.
- Contextual drawers add no authorization shortcut. All asset and tenant IDs are validated again on POST.
- Query defaults change only where requested: existing paginated pages move to 10. Explicit `page_size` values remain honored within current bounds.
- Removing the topbar pending count reduces repeated database work but does not change the dashboard queue or transaction list.
- The unrelated workbook `收租明细_Rosewood_20260916.xlsx` remains untouched.

## 8. Verification strategy for implementation

Each implementation slice should include focused Go checks for query parsing, view-model output, state transitions and safe return handling, plus real-browser checks for anchored popovers, keyboard behavior, focus, drawer validation and responsive geometry. Full repository tests and visual regression checks belong to the final integration task.

## Follow-up implementation clarification (2026-09-20)

The current user follow-up is scoped in `09-20-workspace-interaction-followup/design.md`. It also covers transaction cash/expense drawers, tenant-detail cash entry, property-detail room creation, property/room month calendars, a transaction-list match action with separate status/operations columns, the search-clear hover treatment and a clear action on the optional expense-room select.

The older research records the assessment-time source state. The child task's current-code map is authoritative where later commits have changed behavior. In particular, tenant detail already has a local edit drawer and currently has a cash action but no expense action. The child plan assumes those current actions remain; the user can adjust this while reviewing the child PRD.
