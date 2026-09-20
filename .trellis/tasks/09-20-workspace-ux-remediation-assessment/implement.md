# Implementation plan: workspace interaction and consistency remediation

The assessment recommends six independently reviewable implementation tasks. The order below limits duplicate work and keeps the highest-risk workflow changes behind shared foundations.

## 1. Effort summary

Size definitions in `research/codebase-findings.md` include focused automated and browser verification.

| Workstream | Requirements | Estimate | Main output |
| --- | --- | ---: | --- |
| Shared workspace controls | 4, 5, 6, 12, 14, 15 | 3–5 engineer-days | Unified calendar/select/search controls, dashboard month navigation, topbar cleanup |
| Pagination and list defaults | 2, 3, 9 | 1.5–3 engineer-days | Bill fix, numbered pagination, 10-row defaults |
| Tenant search and detail edit | 7, 8 | 1.5–3 engineer-days | Tenant search and detail-local edit drawer |
| Contextual cash/expense drawers | 1, 13 | 3–5 engineer-days | Shared drawers on property/room/transactions and sidebar cleanup |
| Transaction matching clarity | 10 plus `deferred` domain support | 2–3 engineer-days | Payer/tenant columns, split match selectors, visible action, deferred transition |
| Dashboard manual-review flow | 11 | 2–4 engineer-days | Three processing cards, match/defer/refill behavior |

Because drawer, calendar and match-form work overlaps, the realistic combined estimate is **about 12–18 engineer-days for one engineer familiar with the repository**, including focused verification and integration cleanup. A strict page-by-page implementation would be closer to the raw 14.5–27.5 day sum. The range excludes deployment, production-data repair and unrelated redesign.

The two dominant risks are the system-wide custom select (requirement 12) and the dashboard transaction workflow (requirement 11). If requirement 12 is reduced to styling the closed native select while accepting the OS popup, subtract roughly 2–3 days, but that would not fully meet the requested anchored light popup behavior.

## 2. Recommended task tree and dependencies

```text
workspace-ux-remediation (parent integration task)
├── shared-workspace-controls          P0
├── pagination-and-list-defaults       P0, can proceed beside controls
├── tenant-search-detail-edit          P1, after shared controls
├── contextual-entry-drawers           P1, after shared controls
├── transaction-matching-clarity       P1, after controls + pagination contracts
└── dashboard-manual-review            P2, after transaction matching clarity
```

Within one engineer’s sequence: controls → pagination → tenant/drawers → transaction matching → dashboard → integration. Separate commits should keep quick bug fixes, shared component refactors and workflow-state changes independently reversible.

## 3. Task details

### 3.1 Shared workspace controls

**Requirements:** 4, 5, 6, 12, 14, 15

**Likely files**

- `cmd/truelayer-demo/workspace_shell.go`
- `cmd/truelayer-demo/web/templates/partials/workspace-nav.html`
- `cmd/truelayer-demo/web/templates/pages/rent-workspace.html`
- `cmd/truelayer-demo/web/static/js/calendar.js`
- `cmd/truelayer-demo/web/static/css/calendar.css`
- New shared select/search JS and CSS under `cmd/truelayer-demo/web/static/`
- Page templates with duplicate calendar assets or “筛选”/filter-submit labels

**Steps**

1. Centralize calendar/style/script inclusion in both workspace template constructors and make initialization idempotent.
2. Add the native-select progressive enhancement defined by design D2; migrate a small representative page first, then enable it across workspace pages.
3. Add the explicit search clear control and shared touch-target styles.
4. Replace visible filter action/disclosure labels with “搜索”.
5. Rebuild dashboard month navigation using previous/shared picker/next.
6. Remove shared topbar search/count markup, data plumbing and count query.

**Verification gate**

- Keyboard and pointer checks for select open/choose/close on desktop and narrow viewport.
- Calendar opens once, anchors correctly and works in a drawer.
- Clear button target is at least 44px on narrow screens and remains keyboard accessible.
- Breadcrumb and local search remain; shared topbar controls are absent.

### 3.2 Pagination and list defaults

**Requirements:** 2, 3, 9

**Likely files**

- `cmd/truelayer-demo/dashboard_filters.go`
- `cmd/truelayer-demo/rent_collection_pages.go`
- `cmd/truelayer-demo/billing_page.go`
- `cmd/truelayer-demo/tenant_detail.go`
- A new shared pagination helper/view model near workspace page helpers

**Steps**

1. Fix bills’ `selected` conditions for `unpaid` and `all`.
2. Add a shared bounded page-number window with first/last, ellipses, current state, previous and next.
3. Adapt all four paginated renderers without changing their filter parameter names.
4. Change existing paginated defaults and size choices to 10 while preserving valid explicit sizes.

**Verification gate**

- First, middle and last page render correct disabled/current states.
- Page-number links preserve each page’s complete active query state.
- Large page counts use a bounded number of links.
- Bills round-trip each status without visual reset.

### 3.3 Tenant search and detail-local edit

**Requirements:** 7, 8

**Likely files**

- `cmd/truelayer-demo/main.go`
- `cmd/truelayer-demo/tenant_detail.go`
- New shared tenant-form partial under `cmd/truelayer-demo/web/templates/partials/`

**Steps**

1. Extract the tenant form/drawer with a page-context view model.
2. Add URL-addressable tenant search and filtered/total count state.
3. Preserve search in list/detail/edit return links.
4. Load edit form data and room choices in tenant detail; render `?edit=1` locally.
5. Add safe return-context handling for successful and invalid submissions.

**Verification gate**

- Search matches each specified field and handles no-result state.
- Detail edit opens and closes without visiting `/tenants`.
- Validation errors remain on detail with submitted values.
- Room/property ownership validation is unchanged.

### 3.4 Contextual cash and expense drawers

**Requirements:** 1, 13

**Likely files**

- `cmd/truelayer-demo/page_data_routes.go`
- `cmd/truelayer-demo/main.go`
- `cmd/truelayer-demo/cash_receipt_pages.go`
- `cmd/truelayer-demo/billing_page.go`
- `cmd/truelayer-demo/web/templates/pages/property-detail.html`
- `cmd/truelayer-demo/web/templates/pages/room-detail.html`
- `cmd/truelayer-demo/web/templates/pages/expenses.html`
- `cmd/truelayer-demo/web/templates/pages/cash-receipts.html`
- New expense/cash partials under `cmd/truelayer-demo/web/templates/partials/`

**Steps**

1. Extract existing expense and cash-receipt drawer markup into partials without changing fields.
2. Add allowlisted return contexts with filter/page preservation.
3. Render the expense drawer on property and room detail; preselect and constrain asset context.
4. Add cash/expense actions and both drawers to transaction matching.
5. Preserve cash preview/save and validation flow within transaction context.
6. Remove only the two primary sidebar links; retain list routes and `/more` access.

**Verification gate**

- Property and room creation never navigates to `/expenses` and saves against the correct asset.
- Invalid IDs cannot cross account/property boundaries.
- Cash preview and final save return to the same filtered transaction list.
- Standalone cash/expense pages still support history and their existing follow-up actions.

### 3.5 Transaction matching clarity and defer support

**Requirements:** 10 and the domain prerequisite for 11

**Likely files**

- `cmd/truelayer-demo/transactions.go`
- `cmd/truelayer-demo/billing_page.go`
- Transaction matching service/action files found during implementation

**Steps**

1. Resolve and expose matched tenant display names in transaction rows.
2. Rename column two to “付款人” and add the tenant column.
3. Extract a reusable tenant/month match-form view model and separate the selectors.
4. Expose the match action directly in transaction detail while preserving secondary actions.
5. Add `deferred` to validated status values, labels, filters and audit action kinds.
6. Implement the atomic/idempotent defer transition and ensure normal matching accepts deferred rows.

**Verification gate**

- Payer always comes from the bank transaction; tenant shows the resolved allocation match.
- Invalid tenant/month pairs are rejected server-side.
- Deferred transactions disappear from pending scope, appear under all/deferred filters and can later be matched.
- Existing ignored, partial, rematch, revoke and payer-memory behavior remains intact.

### 3.6 Dashboard manual-review flow

**Requirement:** 11

**Likely files**

- `cmd/truelayer-demo/rent_workspace_page.go`
- `cmd/truelayer-demo/web/templates/pages/rent-workspace.html`
- Shared transaction match view model/service from task 3.5

**Steps**

1. Replace `Limit(2)` with a stable three-row pending query and align count semantics with the pending-status set.
2. Decorate rows with payer, time, note/reference, amount and reusable tenant/month options.
3. Render independent rounded cards with collapsed summary and processing state.
4. Post match/defer with dashboard month return context.
5. Handle stale concurrent submissions and reload the current queue.

**Verification gate**

- Dashboard renders exactly three cards when at least three eligible rows exist, otherwise the actual count.
- Match or defer removes the handled row and the next eligible row fills the queue after redirect.
- Defer does not mark the transaction ignored and does not affect bill-payment semantics.
- Detail fields and tenant/month choice remain usable at narrow widths.

## 4. Integration gate

After the six tasks are complete:

- [ ] Focused package tests and the full repository test suite pass.
- [ ] Shared calendar, select and clear controls work in legacy and embedded templates.
- [ ] Every existing paginated list defaults to 10 and retains query state in numbered links.
- [ ] Property/room/tenant/transaction drawers preserve context on success and validation failure.
- [ ] `deferred` state and action are audited, list-visible and dashboard-hidden.
- [ ] Browser checks cover desktop plus 320/375/390px narrow widths, keyboard navigation and focus restoration.
- [ ] The workbook `收租明细_Rosewood_20260916.xlsx` is unchanged.

## 5. Delivery recommendation

Deliver in four user-visible batches:

1. **Quick correctness and cleanup:** requirements 2, 5, 9 and 15.
2. **Shared UI consistency:** requirements 3, 4, 6, 12 and 14.
3. **Stay-in-context workflows:** requirements 1, 7, 8 and 13.
4. **Matching workflow:** requirements 10 and 11, including the confirmed deferred state.

Each batch is independently reviewable. The Trellis implementation phase should start only after the user reviews this assessment and confirms which batch to implement first.
