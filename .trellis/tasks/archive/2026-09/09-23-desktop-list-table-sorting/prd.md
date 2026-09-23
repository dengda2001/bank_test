# Desktop list table sorting

## Goal

Make every desktop table on a live list page sortable by clicking a supported
column heading. The heading must communicate ascending or descending order
with a small arrow and preserve the list's existing context when the order
changes.

## Confirmed facts

- The homepage is `/rent-dashboard`. Its desktop table is rendered from
  `web/templates/pages/rent-workspace.html` and has three list views:
  properties, rooms, and tenants.
- `/bills` and `/transactions` already accept validated server-side `sort`
  values, but their desktop headings do not expose those values consistently.
- `/properties`, `/rooms`, `/tenants`, `/cash-receipts`, and `/expenses` are
  live desktop list pages with tables and currently have no list-sort contract.
- The detail routes, action previews, and the bank settings page also contain
  tables, but they are not list pages and are excluded from this task.
- The shared `tableSortLink` / `sortLinkFor` helper exists, so the UI must
  extend that contract rather than create competing heading conventions.

## Requirements

1. Every sortable data column in each listed desktop table has a keyboard
   accessible column-heading link. Action, invoice/link, and other
   non-orderable columns remain plain headings.
2. A first click applies that column's documented default direction; repeated
   clicks on the same heading toggle ascending and descending order.
3. The active sort has an adjacent visual arrow and an accessible indication of
   the active order. Inactive headings remain recognizably sortable without
   falsely claiming an active direction.
4. Sorting is performed before a list is rendered. For database-backed list
   queries, the server uses only whitelisted sort expressions. For existing
   in-memory projections, sorting remains deterministic with stable tie
   breakers.
5. Invalid `sort` query values return the page's established invalid-filter
   behaviour; raw query values must never become a SQL `ORDER BY` clause.
6. A sort link resets pagination to page one while retaining relevant search,
   period, scope/property, status/collection, page-size, and drawer context.
7. Existing mobile card views and the mobile interaction contract are not
   changed.

## In-scope tables

- `/rent-dashboard`: the property, room, and tenant desktop views.
- `/bills`: the billing list.
- `/transactions`: the transaction list.
- `/properties`, `/rooms`, `/tenants`, `/cash-receipts`, and `/expenses`.

## Acceptance criteria

- [ ] Each in-scope desktop list table renders sortable headings for its
      meaningful data columns and leaves action-only columns unsortable.
- [ ] Each heading generates the correct primary/alternate sort URL and
      indicates the active ascending or descending direction accessibly.
- [ ] Each supported list rejects or safely falls back from unknown sort input
      without changing the data scope.
- [ ] Sorting preserves current filters/context and starts at page one.
- [ ] Unit/handler/template tests demonstrate toggle behaviour, sort outcome,
      URL preservation, and coverage for every in-scope route.
- [ ] The affected Go package tests, formatting, and desktop/mobile rendering
      checks pass.

## Out of scope

- Detail-page tables, transaction/cash-receipt confirmation previews, and the
  bank settings run-account table.
- Client-only table sorting, schema migrations, and changes to mobile card
  ordering controls.
