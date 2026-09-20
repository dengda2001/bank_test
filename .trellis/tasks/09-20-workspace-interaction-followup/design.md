# Design: workspace interaction follow-up

## Scope and invariants

This child task implements the current follow-up request under the broader workspace remediation assessment. The parent assessment's older research is a snapshot; the confirmed facts in this task's `prd.md` reflect the current source tree.

Keep existing route behavior for standalone history pages, but make the requested entry actions page-local. Preserve these contracts:

- Cash receipt preview and save remain a two-step cash-only rent flow.
- Tenant, property and room IDs are validated against the current account on every POST.
- A return target is an enum/structured context reconstructed on the server, never an arbitrary URL.
- Match status, allocation kind and bill payment status remain separate.
- The dashboard's “暂不处理” continues to use the existing persistent `deferred` behavior, not `ignored`.
- “记住付款人” starts checked and remains optional; the server must interpret checked and unchecked values consistently.
- Keep the pre-existing `tenant-form-drawer.html` template fix in the worktree.

## 1. Page-local entry drawers

### Hosts

- `/transactions`: host the existing cash-receipt and expense drawers. Preserve list filters in close, preview, validation and save return contexts.
- `/tenants/<id>`: host the cash-receipt drawer with the current tenant preselected. Keep the existing tenant-local edit drawer and preserve history range/page state through edit and validation.
- `/properties/<id>`: host the room-create drawer with the property fixed/preselected. Keep the form fields and ownership checks used by `/rooms`.

### Shared form boundaries

Extract cash and room creation markup into partials if needed so standalone pages and detail/transaction pages use the same fields and validation messages. Expense creation already has a shared partial; render it in the transaction host without changing its business fields. Forms post to the existing handlers and pass a safe return context such as `transactions`, `tenant:<id>`, or `property:<id>`. The server accepts only known context shapes and reconstructs the route/query state.

On a cash preview, both “返回修改” and final save return to the originating host. Validation errors reopen the same drawer with submitted values. Room creation follows the same rule for property detail and retains its period/list context.

## 2. Shared calendar and clear controls

- Replace the property/room list `period` selects with `input type="month"` so the already-loaded shared calendar enhancement supplies the calendar popover.
- Preserve all other list query parameters. The shared object-list filter submit path should remain the only submit source for property/room filters.
- Remove the dashboard month form's submit button and submit on the calendar input's `change` event; preserve its view, scope, page size and other filters.
- Keep the search clear button and its hit box, remove pointer-hover fill/edge that overlays the input, and retain keyboard focus styling.
- Add opt-in clear behavior to the shared enhanced-select wrapper (or an equivalent shared control) for the expense room select. The clear button sets the native select to its blank option, dispatches `input` and `change`, updates the enhanced label, and leaves the selected property unchanged. If a property change disables the selected room, reset that room selection too.

## 3. Transaction list action and status

The server-rendered row view model already has manual tenant options and period options for eligible transactions. Render `状态` and `操作` as separate desktop table columns. The operation cell exposes a visible “匹配流水” action for eligible unmatched/pending rows and reuses the same tenant/month option data and confirmation endpoint as the existing mobile quick-match form. The action reveals a compact row-local form/popover; rows that cannot be matched directly retain a detail action. Keep unmatched rows in the pending/unmatched query scope; do not broaden or narrow the existing transaction query without evidence that its current pending status filter is wrong.

## 4. Dashboard processing popover

Replace the per-card `<details>` expansion with a small “处理流水” trigger and a floating panel. The panel overlays the dashboard and does not change the queue card's dimensions or move adjacent cards. It contains the transaction facts, tenant and month controls, an adjacent remember-payer checkbox and match action, plus the defer action and detail link.

Only one processing panel is open at a time. Pointer dismissal and Escape close it; focus returns to its trigger. At narrow widths, keep the same content in a viewport-safe bottom-aligned panel. Successful match/defer still posts to the existing action endpoints and returns to the dashboard so the next eligible transaction refills the queue.

Read all `remember_payer` values in the confirmation handler: with the existing hidden `0` fallback, any submitted `1` means checked; only `0` means unchecked. This preserves unchecked form submission while honoring the visible default-checked control.

## 5. Compatibility and rollback

No data-model or schema change is expected. Each slice can be reverted independently: shared controls, contextual forms/return handling, transaction row actions, and dashboard popover. If a host-specific POST cannot safely reconstruct its return context, keep the operation on its existing route until the context is explicitly allowlisted; never fall back to trusting request-supplied paths.

## 6. Main risks

- Cash preview and validation paths have separate POST handlers; every return context must survive both.
- The transaction table has desktop and mobile renderers. Keep status/action separation and the visible match action coherent at both breakpoints.
- The custom month input renames the visible input and submits through its generated hidden field. Auto-submit must observe the calendar's `change` event after enhancement.
- A clear button beside an enhanced select must not hide the native form value or steal the popup's anchor/focus behavior.
- The dashboard panel must avoid clipping inside page/card overflow and remain usable on narrow screens.
- Removing the hidden/checkbox ordering bug must preserve the unchecked value, not only the checked default.
