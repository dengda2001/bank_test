# Design: workspace UI polish

## Scope and invariants

This task implements the seven requirements in `prd.md` as one workspace-polish slice. It reuses the current server-rendered pages and shared controls; it does not introduce a new UI framework or change transaction matching semantics.

- Room `room_type` and `capacity` remain persisted for compatibility. New room creation keeps the existing `其他` / `1` defaults. Room edits that omit these fields must not overwrite stored values. Current tenant count is separate from capacity and remains visible.
- Transaction match state, allocation state, and rent-payment state remain separate. Existing queue and match endpoints stay unchanged.
- Tenant option search is client-side filtering over the already-rendered tenant options, case-insensitive and substring-based. It does not change IDs, eligibility, or the tenant-to-period filtering contract.
- The shared select continues to dispatch native `input` and `change` events after selection so existing dependent controls and form behavior keep working.

## Shared searchable-select popup

The shared enhancement in `web/static/js/workspace-controls.js` currently mounts a `position: fixed` options popup inside its select wrapper. The dashboard processing dialog is itself transformed and scrollable, so it can become the fixed-position containing block and clip/misplace that popup.

Portal the popup surface to `document.body` while retaining the original trigger as its anchor. Recompute its viewport position from the trigger's bounding box on open, resize, and scroll; place it above the dashboard dialog's current `z-index` of 1600. Update outside-pointer handling so both the trigger wrapper and its portaled popup count as inside the active select. Preserve Escape dismissal, focus restoration, option selection, disabled-option handling, and click-outside behavior.

Add an opt-in `data-searchable` mode and apply it to tenant-picking selects across dashboard processing, transaction matching/detail, transaction filters, and cash receipt entry. In this mode the popup includes an accessible search input and a filtered listbox. Matching uses case-insensitive substring comparison against visible option labels; blank prompt options are not results for non-empty queries, and a no-results message is shown. Selecting a result updates the native select and emits its existing events. Non-searchable selects keep their current keyboard type-ahead behavior.

## Page-specific changes

1. **Room fields:** Remove room type/capacity controls from the shared create drawer and detail edit drawer, and remove their values from desktop/mobile room facts. Keep current tenant counts, rent, due-day and other useful room data. In the create handler, omitted capacity still resolves to the current default. On update, treat absent room type/capacity as unchanged: conditionally include those keys in the update map only when explicitly supplied, rather than applying create defaults to an edit.
2. **Dashboard queue:** Add an empty-state modifier when `PendingCount == 0`. Apply a calm green treatment to the queue panel/header/count and empty message only in that state; non-empty queues retain the current warning treatment.
3. **Transaction status auto-submit:** Both match-status selects on `/transactions` (compact and advanced filter forms) submit their own GET form on change. Because the shared control emits `change`, use `requestSubmit()` or an equivalent submit-event-preserving path; keep the rest of each form's current query fields.
4. **Object-list status width:** Increase the shared collection-status select width/min-width enough to show “全部状态” without ellipsis on both property and room management pages, including the mobile filter layout. Keep the status option values and filter behavior unchanged.
5. **Transaction detail action:** Always render “查看详情” in each transaction row's operations cell. For directly matchable rows, show it alongside (not instead of) “匹配流水”; rows without direct match options keep the existing detail-only action. The mobile renderer already exposes the detail link and should remain consistent.
6. **Safe deletion:** Add a single transaction-scoped deletion service for property, room, and tenant detail actions. It locks and revalidates account ownership before deletion. A property may be deleted only with no rooms, charges, or property expenses; a room only with no rent plans, charges, or room expenses; a tenant only with no rent-plan memberships, obligations, allocations, matched transactions, cash receipts, or dunning attempts. Tenant payer aliases may be removed by the database's existing cascade only after these history checks pass. Dependency failures map to a stable, human-readable blocked-deletion notice; no handler relies on a foreign-key error as product feedback.
7. **Matching month calendar:** Replace each direct matching form's native month select with the existing shared month input. Match metadata remains in an inert template rather than visible `<option>` nodes, so duplicate months are never rendered. A lightweight shared initializer reads the selected tenant, constrains the calendar to that tenant's matching months, clears an invalid previous selection, and writes an `aria-live` helper below the control with the selected month's outstanding amount. The server continues to validate the tenant/month pair.

## Compatibility and risks

- No schema migration or destructive data rewrite is expected. Update behavior must be tested because the current service defaults omitted values and writes them on every room edit.
- Portaling changes the popup's DOM ancestry. Outside-click detection, keyboard focus, z-index, required-select validation, and tenant/period dependent filtering are regression risks.
- Auto-submit must only bind the two match-status controls on the transaction-matching page; unrelated status selects continue to behave as before.
- The exact truncation and popup behavior should be verified at desktop and narrow viewport widths in a real browser, not only by asserting CSS/template strings.
- Deletion is intentionally restrictive. Historical ledger and expense relations are not deletion candidates, and an object that cannot be deleted remains available for the existing inactive lifecycle.

## Rollback

Each presentation slice is independently revertible: room-field UI/data-preservation, shared searchable select and popup portal, transaction filters/actions, and queue/object-list styling. No migration rollback is needed.
