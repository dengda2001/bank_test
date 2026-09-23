# Sortable List Headings

> Server-rendered ordering contract for live desktop list pages.

## Scenario: Adding or changing a sortable list column

### 1. Scope / Trigger

- Applies when a live list page adds a desktop table/grid heading that changes
  row order: `/rent-dashboard`, `/transactions`, `/properties`, `/rooms`,
  `/tenants`, `/cash-receipts`, and `/expenses`.
- Detail tables, confirmation previews, bank sync history, and the retained
  `/bills` template are not live list surfaces. `GET /bills` redirects to the
  workspace tenant view; see `rent-workspace-navigation.md`.

### 2. Signatures

```go
type tableSortLink struct {
    URL    string
    Active bool
    Arrow  string // ▲ for current ascending, ▼ for current descending
}

func sortLinkFor(baseURL func(string) string, current, primary, secondary string) tableSortLink
func tableSortHeading(label string, link tableSortLink) tableSortHeadingData
```

Templates render the shared `table-sort-heading` partial for `<th>` elements
and `list-sort-heading` for the workspace's CSS-grid column headers. The
partial sets `aria-sort` only on the active heading.

### 3. Contracts

- A page parses `sort` at its request boundary, accepts only a page-owned
  allowlist, and orders typed values before rendering. Never sort formatted
  money/date strings or pass request text to SQL `ORDER BY`.
- Each comparator has an immutable stable tie-breaker. SQL list queries use an
  explicit whitelist of complete `ORDER BY` clauses.
- The URL builder carries the page's period/search/status/scope/collection and
  page-size values; a header link changes only `sort` and resets `page` to 1.
- `primary` is the first-click order. An active second click uses `secondary`.
  The arrow describes the current order, not the proposed next click.
- Forms, drawers, pagination, and safe same-site return URLs retain the active
  `sort` value. Mobile cards keep their existing interaction model.

### 4. Validation & Error Matrix

| Input / state | Required result |
|---|---|
| Missing `sort` | Preserve that page's documented default order |
| Valid two-direction column | Render a keyboard link; active `<th>` has `aria-sort` and `▲` or `▼` |
| Valid one-direction column | Render a link without an arrow that falsely implies a toggle; an active heading uses `aria-sort="other"` |
| Unknown `sort` | Use the page's established safe invalid-filter response; never query/order on it |
| Sort link on a later page | Keep filters and page size, select page 1 |
| Drawer/action return | Restore the selected valid sort along with list context |

### 5. Good / Base / Bad Cases

- Good: a cash-receipt amount header builds
  `/cash-receipts?...&sort=amount_desc`; the handler validates that token and
  compares `AmountCents`, then ties with the immutable receipt ID.
- Base: an absent transaction sort maps to its existing arrival-descending SQL
  order and shows that date heading as active.
- Bad: `href="?sort={{.Label}}"`, client-side row shuffling, comparing
  `"EUR 1,000"`, or attaching a link to an action-only column.

### 6. Tests Required

- Unit test `sortLinkFor` for first click, toggle, active arrow, and ARIA
  markup.
- For every new list contract, test valid/invalid sort handling, typed order
  (including a tie), and a link that preserves filter context while resetting
  pagination.
- Render the desktop table/grid and assert meaningful data headings use the
  partial while action columns remain plain. Keep existing mobile-card tests.
- Run `gofmt`, `go vet ./...`, `go test ./... -count=1`, and
  `git diff --check`; use browser checks for desktop geometry when an
  authenticated local acceptance session is available.

### 7. Wrong vs Correct

Wrong:

```go
rows = sortByDisplayText(rows, r.URL.Query().Get("sort"))
```

Correct:

```go
sortValue := strings.TrimSpace(r.URL.Query().Get("sort"))
if !validExpensePageSort(sortValue) {
    http.Error(w, "expense sort is invalid", http.StatusBadRequest)
    return
}
rows = sortExpenseRecords(filteredRows, sortValue)
links := expensePageSortLinks(period, status, search, sortValue)
```
