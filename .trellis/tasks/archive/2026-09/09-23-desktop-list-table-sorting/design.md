# Design: desktop list table sorting

## Boundary and contract

Each list owns a typed `sort` value and validates it at its request boundary.
The view model receives prebuilt `tableSortLink` values rather than letting a
template construct query strings. The heading helper owns direction toggling,
visual state, and ARIA text.

```
GET query -> typed filters / page context -> whitelist + deterministic sort
          -> list view model + heading links -> desktop table heading links
```

The three homepage views retain their existing shared `rentWorkspaceFilters`.
The database-backed transaction list retains its existing SQL whitelist. The
remaining projection lists add small page-specific filter/sort values and sort
their typed rows after their existing filters, before rendering. This avoids
introducing generic reflection-based ordering or putting formatted strings in
charge of numeric/date order.

## Heading interaction

`tableSortLink` is expanded to carry the current direction and accessible
label. For a two-direction column:

- first click uses its primary order;
- clicking an active heading switches to the alternate order;
- an active heading shows the current direction arrow (`▲` ascending, `▼`
  descending) and `aria-sort` is rendered on its `<th>`;
- inactive headings remain keyboard-focusable text links without an arrow or
  `aria-sort`; their first click uses the documented primary direction.

One-way semantic sort columns (for example a priority status) stay clickable
but do not advertise a toggle. The action column is never a sort control.

## Sort data

Each page has a small, explicit whitelist matched to real typed values:

- Workspace: use/extend the current `name`, `balance`, `due`, and status
  priority rules for each view.
- Bills: tenant/room, due date, expected, paid, balance, and status priority.
- Transactions: extend its current database whitelist only for table values
  that have a reliable source field or explicit join/projection; no free-form
  expression reaches GORM.
- Property, room, tenant, cash receipt, and expense lists: meaningful name,
  date, monetary, status, and count fields from their typed row models.

Every comparator ends in a stable, immutable ID/name tie-breaker so successive
loads do not reshuffle equal values.

## Context preservation

Each page's canonical URL builder is extended once to accept `sort` and an
explicit target page. Heading links use page one. Existing search/filter forms,
pagers, drawers, mutation redirects, and safe `return_to` builders include the
active sort, so opening/closing a drawer or running an action does not discard
the chosen order.

## Compatibility and rollback

An absent sort preserves the current default order for every page. Existing
sort values for the workspace, bills, and transactions remain valid. The work
is additive to query strings and does not migrate persisted data; reverting the
change restores existing defaults.
