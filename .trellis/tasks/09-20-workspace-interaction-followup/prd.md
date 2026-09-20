# 工作台交互修复补充需求

## Goal

Keep the requested transaction, tenant, property and room workflows on their current page, and make the matching, calendar and clear controls behave consistently with the shared workspace UI.

## User value

- Record cash, expenses and rooms without leaving the detail or transaction context.
- Match unmatched transactions directly from the transaction list and process dashboard items without expanding their cards.
- Use a calendar control for property and room month filters, with less visual interference from small clear actions.

## Confirmed repository facts

- `/transactions` currently links cash entry to `/cash-receipts?add=1` and expense entry to `/expenses?add=1`.
- Tenant detail currently links cash entry to `/cash-receipts?add=1` with the tenant ID preselected; it has no expense-entry action today.
- Property detail sends “新增房间” to `/rooms?property_id=…&add=1`, moving the user to the room list.
- Tenant detail already builds an edit URL on `/tenants/<id>?edit=1` and renders `tenant-form-drawer` locally. Preserve this same-page behavior and verify its rendered drawer/return flow while making adjacent changes.
- Property and room list month filters are native `<select>` controls. The shared calendar enhancement already supports `type="month"` inputs.
- The dashboard processing form is currently expanded inside each card with `<details>`; its visible submit and “记住付款人” controls are laid out far apart.
- The desktop transaction table combines status and actions in one column and only exposes “查看详情”; mobile rows already render a matching form for eligible transactions. The pending status set includes `unmatched`.
- The search clear button uses a muted hover background. The expense drawer's optional room select already has a blank “房产级支出” option but no dedicated clear action.
- The transaction confirmation handler reads the first `remember_payer` form value. The dashboard and mobile forms send a hidden `0` before the checked checkbox value `1`, so this can ignore the visible checked state.

## Requirements

1. On `/transactions`, “现金补录” and “新增支出” open their existing drawer forms over the current page. On tenant detail, the existing cash-entry action opens the cash drawer there and retains the tenant preselection. Preserve the current action set; this task does not add a new expense action to tenant detail.
2. On property detail, “新增房间” opens the room-creation drawer without navigating to `/rooms`, preselects that property, and returns to property detail after save or validation failure.
3. Editing a tenant from tenant detail stays on `/tenants/<id>` and uses the side drawer. Success, validation errors, close and cancel preserve tenant detail context and existing form validation.
4. Remove the dashboard month form's adjacent “搜索” button. Selecting a month applies it immediately and preserves the active dashboard filters.
5. Property and room list month filters use the shared calendar control rather than a month dropdown. Preserve their other filters and selected month in the query string.
6. Keep unmatched transactions visible under the pending/unmatched list scope. In the transaction list, provide an explicit “匹配流水” action for eligible rows and render match status and row operations in separate columns.
7. Clicking the dashboard “处理流水” action opens a floating panel for that item instead of expanding/reflowing its card. The panel shows the current transaction details, tenant/month controls, match and defer actions, and can be dismissed without changing the queue. Keep the visible “记住付款人” checkbox beside its matching action, checked by default and optional; its submitted state must be honored.
8. Search clear controls keep their existing hit-area dimensions. Pointer hover must not draw a gray fill/edge over the input; keyboard focus remains visible.
9. The expense drawer's “关联房间（可选）” select has a clear button beside the control. Clearing returns to the blank “房产级支出” value without changing the selected property.

## Acceptance criteria

- [ ] Opening transaction cash or expense entry leaves the user on `/transactions` and renders the corresponding drawer.
- [ ] Opening cash entry from tenant detail leaves the user on `/tenants/<id>` with that tenant selected; cash preview, correction, save and validation-error paths return to that detail page.
- [ ] Property detail opens the room drawer locally with its property selected; successful creation and validation failures return to that property detail.
- [ ] Tenant detail edit remains a detail-local drawer; close/cancel and submit/error flows retain the tenant ID and history query state.
- [ ] The dashboard has no month-adjacent “搜索” button; choosing a month refreshes the same dashboard view with its other filters intact.
- [ ] Property and room list month controls open the shared calendar popup, not a select popup, and selecting a month keeps the remaining list filters.
- [ ] Pending/unmatched rows are present in the transaction list. Status and operations have separate headers/cells, and an eligible row has a visible “匹配流水” action.
- [ ] Processing a dashboard item overlays a floating panel without changing card/list layout. The remember-payer control sits beside the match action, starts checked, can be unchecked, and the submitted value follows the checkbox.
- [ ] Hovering the search clear action does not obscure or gray out the input. The existing click target remains the same size and keyboard focus remains apparent.
- [ ] Clearing the optional expense room sets the value to blank (“房产级支出”) and leaves the property selection unchanged.

## Constraints

- Reuse the existing cash, expense, tenant and room form behavior where possible; cash preview remains a two-step operation.
- Return targets must be reconstructed from known, validated contexts; do not accept arbitrary redirect URLs. Revalidate tenant, property and room IDs against the current account on POST.
- Keep transaction match status, allocation kind and bill payment status separate. “记住付款人” remains checked by default but optional. “暂不处理” remains the existing distinct deferred action and must not become `ignored`.
- Keep standalone cash and expense list routes available for history and follow-up actions.
- Preserve unrelated working-tree changes, especially the existing tenant drawer template fix, `docs/architecture/`, and `收租明细_Rosewood_20260916.xlsx`.

## Planning assumptions for review

- The dashboard month applies immediately when selected, so removing its “搜索” button does not leave a changed value unapplied. This uses the shared calendar's change event and preserves the current dashboard filters.
- Tenant detail keeps its existing cash-entry action; no new expense action is added there. The current source tree has no such action, and expense allocation to a tenant would need a separate product rule.

## Out of scope

- Adding an expense-entry action to tenant detail, where no such action currently exists.
- New transaction states, schema migration, unrelated list redesign or changes to cash/expense business rules.
- Broad redesign of the shared select/calendar system beyond the controls needed for these requirements.
