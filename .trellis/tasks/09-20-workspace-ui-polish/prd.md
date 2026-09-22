# 工作台交互细节修复

## Goal

移除无用房间字段，修复待处理流水浮窗与租客选择，改善交易筛选、列表状态布局和详情入口。

## Requirements

1. Remove room type and occupancy capacity from room creation, room editing, and room-detail facts; retain the other room fields and current save flow.
2. When the dashboard has no transaction awaiting manual review, show the pending-review indicator/state in green rather than warning/error styling.
3. Fix the dashboard “处理流水” floating panel so opening the tenant selector does not move or collapse the panel and tenant choices remain visible and selectable.
4. On the transaction-matching page, changing the match-status filter submits the search immediately; the user should not need to click “搜索” afterward.
5. Workspace tenant selectors used in manual review, transaction matching/filtering, and cash-receipt entry support typing to narrow available tenants by name.
6. The property and room management collection-status selectors display their selected label without truncating it to an ellipsis.
7. Every transaction row has a “查看详情” action, including rows that also expose a direct match action.
8. Properties, rooms, and tenants that have no dependent operational or ledger data can be deleted from their detail pages. A deletion attempt must never erase rent, payment, or expense history: objects with dependencies stay intact and receive a clear explanation instead.
9. Transaction matching uses the shared month calendar rather than rendering a repeated month option for every eligible tenant. After a tenant and an eligible month are selected, the control shows the matching month's outstanding rent in small helper text.

## Acceptance Criteria

- [ ] Room type and capacity are absent from room creation, room editing, and room-detail facts; actual current tenant/occupancy counts remain visible.
- [ ] Creating a room without those fields uses safe existing defaults, and editing a room without those fields preserves its stored values.
- [ ] An empty dashboard manual-review queue has a green visual state; a non-empty queue retains its existing attention state.
- [ ] Opening and using the tenant selector in the dashboard processing panel keeps the panel anchored and visible, with selectable tenant options.
- [ ] Changing match status on `/transactions` automatically submits the current filter form while preserving its other filter values.
- [x] Tenant search accepts partial text and narrows options by tenant name across dashboard processing, transaction list/detail matching, the transaction tenant filter, and cash-receipt entry.
- [ ] The collection-status selector label is fully readable on both property and room management pages at supported widths.
- [ ] Every transaction row exposes “查看详情”; rows eligible for direct matching retain that action alongside “匹配流水”.
- [x] A newly created, otherwise unused property, room, or tenant can be deleted; the user returns to the relevant list with a success notice.
- [x] A property with rooms, a room with a rent plan, rent charge, or expense, and a tenant with plan, obligation, payment, or receipt history cannot be deleted; the original record remains and the UI explains why.
- [x] Matching-month controls contain no duplicate tenant/month options, use the shared calendar, only enable months eligible for the selected tenant, and show that month's outstanding rent below the calendar.

## Constraints and Notes

- Preserve existing transaction-matching and queue state semantics.
- Do not remove persisted room fields or change database/business rules unless repository evidence shows they are UI-only.
- Keep the existing uncommitted `docs/architecture/` draft and rental spreadsheet untouched.
- Interpret tenant “fuzzy search” as case-insensitive substring filtering over option labels; do not add pinyin/transliteration search without a separate product request.
- Enable tenant search consistently on tenant-picking controls across transaction review/matching and cash-receipt entry, including the tenant filter, without changing non-tenant select behavior.

## Confirmed Repository Facts

- Room type and capacity are persisted fields. They currently appear in the room-create drawer, room-edit drawer, and room-detail facts. The service defaults omitted create values to `其他` and `1`; the update path also writes those defaults, so update handling must preserve stored values when the removed fields are omitted. Current tenant count is a separate derived value and stays in the UI.
- The shared `workspace-controls.js` replaces single-selects with custom controls. Its option popup is `position: fixed` but is currently nested inside the dashboard processing dialog, which itself has a CSS transform; that can make the popup use the dialog as its containing block. Confirm the visible failure in a browser during implementation verification.
- Tenant selects exist across the dashboard, transaction list/detail matching, transaction filters, and cash-receipt entry. The shared select currently supports keyboard type-ahead but has no text input for filtering options.
- The transaction matching page has two match-status selectors, one in the compact filter and one in the advanced filter. Neither currently auto-submits on change.
- The dashboard queue and its count badge always use the warning/pink treatment, including when the pending count is zero.
- Property and room collection-status filters both use the shared custom select and a fixed 100px width; the selected option text explicitly uses ellipsis, matching the reported “全…” truncation.
- On desktop transaction rows with direct match options, the operations cell currently renders “匹配流水” instead of “查看详情”; rows without direct match options already show the detail link. The mobile row renderer already provides detail separately.

## Confirmed Product Decision

- Remove room type and capacity from room creation, editing, and detail display. Keep the persisted columns and preserve existing values on edit; new rooms continue to use current defaults. The user confirmed this scope on 2026-09-20.

## Open Questions

- None. Search scope and matching behavior are recorded in Constraints and Notes.
