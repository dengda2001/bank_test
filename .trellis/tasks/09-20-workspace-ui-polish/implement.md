# Implementation plan: workspace UI polish

## Ordered slices

### 1. Remove unused room fields without changing stored data

- Update renderer tests first: room create/edit/details must not expose room type or capacity; retain the current tenant count and other room fields.
- Remove the controls and detail facts from the shared create drawer and room-detail template.
- Keep create defaults (`其他`, capacity `1`). Make update handling preserve both stored values when fields are omitted; add a focused unit or opt-in MySQL test for that contract.
- Review desktop and mobile room detail layouts after removing the facts.

**Risk files:** `page_data_routes.go`, `landlord_domain_operations.go`, `web/templates/partials/room-create-drawer.html`, `web/templates/pages/room-detail.html`, `desktop_layout_test.go`.

**Rollback point:** form/template removal and update-preservation logic can be reverted together without a schema change.

### 2. Fix and extend the shared tenant select

- Add `data-searchable` to tenant-picking controls identified in the current templates.
- Add a search input and case-insensitive substring filtering to the shared select popup while retaining native selected values and dependent change events.
- Portal the popup to `document.body`, update placement and outside-click logic, and ensure its stacking order is above the dashboard processing dialog.
- Preserve keyboard navigation, Escape, focus restoration, disabled options, and the empty/no-results states.

**Risk files:** `web/static/js/workspace-controls.js`, `web/static/css/workspace-controls.css`, tenant-select markup in `billing_page.go`, `rent-workspace.html`, `transaction-detail.html`, and cash-receipt templates.

**Rollback point:** the searchable mode is opt-in; the shared select can fall back to its existing in-wrapper popup and type-ahead behavior if portal or filtering regressions appear.

### 3. Make transaction status selection immediate and keep detail access

- Submit both transaction-page match-status filter forms on status change, preserving all fields in the changed form.
- Always render the transaction detail action alongside direct matching in desktop row operations; retain the existing mobile detail action.
- Keep the match option list, query scope, and confirmation endpoint unchanged.

**Risk files:** `billing_page.go` and focused transaction page tests.

**Rollback point:** the auto-submit hooks and detail link are independent template changes.

### 4. Clarify the empty queue and widen object-list status selectors

- Add the dashboard empty modifier from the server-rendered pending count and a green empty-state treatment; retain warning styling when work exists.
- Widen the property/room collection-status select without changing option/query semantics.
- Check both desktop and mobile layouts for overflow, legibility, and focus/hover states.

**Risk files:** `rent-workspace.html`, `rent-workspace.css`, `object-lists.css`.

**Rollback point:** CSS modifiers and width rules can be independently reverted.

## Verification

- Run focused Go tests for room create/edit/detail rendering and room-update value preservation.
- Run transaction/filter and page-render tests; run `go test ./cmd/truelayer-demo` when the local database test configuration permits (the existing MySQL integration suite skips when `RENTOPS_MYSQL_TEST_DSN` is unset).
- Build the demo binary and run `git diff --check`.
- Use a real browser at desktop and narrow widths to verify: empty/non-empty dashboard queue colors; opening the processing panel and searching/selecting a tenant without panel movement/clipping; tenant-to-month option filtering; Escape/outside dismissal; both transaction status selectors auto-submit with other query values intact; all transaction rows retain detail access; property/room “全部状态” text is fully visible; room create/edit/details omit only the two requested fields while actual occupancy remains.

## Review gates

1. Before coding, read the applicable Trellis frontend/backend specs and responsive conventions.
2. Review the room update contract to ensure an edit cannot silently reset stored room type/capacity.
3. Review portaled-popup stacking, pointer dismissal, keyboard focus, and mobile geometry in the browser.
4. Inspect the final diff and ensure `docs/architecture/` and the rental spreadsheet remain untouched.

## Rollback / non-goals

- No DB migration, removal of persisted columns, change to transaction status semantics, or pinyin/transliteration search.
- If popup portaling causes an accessibility or stacking regression, revert the portal change first and preserve the searchable mode only if it can work safely in the current container.
