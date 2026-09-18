# Research: `/rent-dashboard`, `/tenants`, `/expenses` render map

- **Query**: Map how the three list pages render today (handler, data struct, template, table columns, row markup, summaries, filters, ≤640px rules, row actions, reusable pieces).
- **Scope**: internal only (code in `/Users/dd/projects/bank/cmd/truelayer-demo/`).
- **Date**: 2026-09-17
- **Explicitly excluded**: `/billing` (owned by another agent).

## Summary of the three pages

| Page | Handler | Data struct | Template constant | Template location |
|---|---|---|---|---|
| `/rent-dashboard` | `(*app).handleRentDashboard` `dashboard.go:10`; real work in `(*app).renderRentDashboard` `dashboard.go:17` | `rentDashboardPageData` `main.go:246` | `rentDashboardTemplate` `dashboard.go:141` | inline raw string in `dashboard.go`, body starts `dashboard.go:171` |
| `/tenants` | `(*app).handleTenants` `main.go:896` (POST delegates to `createTenant` `main.go:969`) | `tenantPageData` `main.go:227` | `tenantTemplate` `main.go:2524` | inline raw string in `main.go`, `main.go:2524-2695` |
| `/expenses` | `(*app).handleExpenses` `main.go:1062` (POST delegates to `createExpense` `main.go:1107`) | `expensePageData` `main.go:238` | `expenseTemplate` `main.go:2697` | inline raw string in `main.go`, `main.go:2697-2790` |

Route registration: `main.go:412` (`/tenants`), `main.go:418` (`/expenses`). `/rent-dashboard` is registered elsewhere in the same `main.go` mux block (near `main.go:412`).

Note: `tenants.go` and `expenses.go` contain **only** models/services/repository helpers — no handler, no template. The tenants/expenses UI lives entirely in `main.go`. `dashboard.go` contains handler + template; its filter/sort/paginate logic lives in `dashboard_filters.go` and `sort_links.go`, and its aggregation in `obligations.go`.

All three templates are built by `newWorkspacePageTemplate(name, functs, body)` (`workspace_shell.go:65`), which clones `workspaceBase` so each page can call `{{template "workspace-nav" .}}`. `workspaceBase` is parsed from `workspaceNav` (`workspace_shell.go:46-58`) and is the **only** `{{define}}` block shared by these pages.

---

## 1. `/rent-dashboard`

### Render path

- `handleRentDashboard` (`dashboard.go:10`) → `requireAuth` → `renderRentDashboard(w, r, nil)`.
- `renderRentDashboard` (`dashboard.go:17`) parses `period`, filters (`rentDashboardFiltersFromQuery`, `dashboard.go:19`), builds `rentDashboardPageData` (`dashboard.go:33`), then when DB-backed calls `newObligationService(a.db).summarizeRentDashboardWithFilters(...)` (`dashboard.go:76`) and copies ~25 fields + `CollectionPercent` into `data`.
- Fallback (no DB / no session): `dashboard.go:120-134` loads tenants/expenses/demo JSON and fills only a subset of metrics.
- Template executed `dashboard.go:136`.
- `rentDashboardTemplate` FuncMap (`dashboard.go:141-170`): `rentDashboardURL`, `dashboardPreviousPage`, `dashboardNextPage`, `dunningDeliveryLabel`. No other custom funcs.

### Table structure

`<thead>` is a single literal at `dashboard.go:403`, inside `<table>` opened at `dashboard.go:402` (`<div class="table-wrap"><table>`). 7 columns in order:

| # | Label | Sortable? | Cell markup location / conditional |
|---|---|---|---|
| 1 | 租客 | yes — `TenantSort` | `dashboard.go:405` first `<td>`; alias line conditional on `.TenantAlias` |
| 2 | 房间 | no (plain text) | `dashboard.go:405`, `{{if .RoomLabel}}{{.RoomLabel}}<br>{{end}}{{.RoomAddress}}` |
| 3 | 应缴日 | yes — `DueSort` | `dashboard.go:405` `<td class="mono">{{.DueDate}}</td>` |
| 4 | 应收 | yes — `AmountSort` | `<td class="amount">{{.ExpectedAmount}}</td>` |
| 5 | 已收 | no | `<td class="amount">{{.PaidAmount}}</td>` |
| 6 | 未收 | no | `<td class="amount">{{.BalanceAmount}}</td>` |
| 7 | 状态 | yes — `StatusSort` (single direction) | `<td><div class="status-actions">…`; the cell also conditionally contains the 一键平账 POST form when `{{if gt .ExpectedCents .PaidCents}}` |

Sort-link markup is inline per `<th>`: `dashboard.go:403`, class `sort-link{{if .XSort.Active}} active{{end}}` with optional `<span class="sort-arrow">`.

### Row markup

- A **literal `<tr>`** inside `{{range .Rows}}` (`dashboard.go:404` → row at `dashboard.go:405`). Not string-concatenated in Go.
- `.Rows` is `[]rentDashboardRow` (`main.go:288-306`) — the **template reaches directly into the domain struct**; there is **no per-row view model**. Fields used by the template: `ObligationID, TenantID, Period, TenantName, TenantAlias, RoomLabel, RoomAddress, DueDate, ExpectedAmount, PaidAmount, BalanceAmount, Status, StatusLabel, Payments, ExpectedCents, PaidCents`.
- Pre-formatted display strings (`ExpectedAmount`, `PaidAmount`, `BalanceAmount`, `DueDate`) are built inside `summarizeRentDashboardWithFilters` (`obligations.go:402-420`), not by template funcs.
- Each row is followed by a second literal `<tr id="rent-details-{{.ObligationID}}" class="rent-details" hidden>` (`dashboard.go:406`) that holds the expandable payment list.
- Empty-state branches at `dashboard.go:409-411` (three-way: current page empty / filter empty / no obligations).

### Summary / metrics blocks (above the table)

Section 1 `收租汇总` (`dashboard.go:369-387`):

- `.dashboard-summary` grid of 4 `.panel.metric` cards (`dashboard.go:373-378`): 本月应收 `{{.ExpectedTotal}}`; 已收租金 `{{.PaidTotal}}` (linked, status=paid); 剩余未收 `{{.BalanceTotal}}` (linked, status=unpaid); 待处理 `{{.PendingCount}}`. The middle two are `<a class="… metric-link">` wrapping the whole card.
- `.dashboard-counts` chips (`dashboard.go:379-385`): 本月共 N 户, 逾期, 部分缴纳, 待确认 (linked with `status=`), 当前显示 `{{.FilteredCount}}`.
- `.collection-panel` 收款进度 (`dashboard.go:386`): `{{.CollectionPercent}}%`, `.progress-track/.progress-value` width style.
- Sync banner `dashboard.go:356`.
- All values computed in `summarizeRentDashboardWithFilters` (`obligations.go:340-...`); counts accumulated at `obligations.go:381-423`; `CollectionPercent` computed in the handler at `dashboard.go:108-113`. Card strings formatted via `formatMoney` (`main.go:1871`). `rentDashboardSummary` struct is `obligations.go:306-334`.

### Filters, sorting, pagination

Three separate GET forms, all `action="/rent-dashboard"`:

1. **Month picker** (`dashboard.go:358-367`): `period` (`<input type="month">`, auto-submit onchange), flanked by `.month-nav` ‹/› links built with `rentDashboardURL`; hidden `search`, `status`, `sort`, `page_size`. Label 收租月份 is `display:none` at ≤640 (`dashboard.go:294`).
2. **List filter** (`dashboard.go:394-400`): hidden `period`; `search` (`<input type="search">`, `id=dashboard-search`); `status` (`<select id=dashboard-status>`, options `all / unpaid / overdue / needs_review / partial / open / paid`); hidden `sort`; buttons 筛选 (submit) and 清除筛选 (link to `/rent-dashboard?period=…`).
3. **Page size** (`dashboard.go:413-419`): hidden `period`, `search`, `status`, `sort`; `page_size` select (`12 / 24 / 50`), auto-submit.

Pagination nav `dashboard.go:420-424`: 上一页 / 下一页 anchors via `rentDashboardURL` + `dashboardPreviousPage`/`dashboardNextPage` funcs, plus `第 {{.Page}} / {{.TotalPages}} 页 · 共 {{.FilteredCount}} 户`.

Query params and whether they are read back:

| Param | Owner | Read back into data | Re-emitted as control value |
|---|---|---|---|
| `period` | `renderRentDashboard` `dashboard.go:18` | `.Period` | month input `value="{{.Period}}"` |
| `search` | `rentDashboardFiltersFromQuery` | `.SearchFilter` | search input + hidden fields |
| `status` | same | `.StatusFilter` | select `selected` + hidden fields |
| `sort` | same | `.SortFilter` | hidden fields + the four `tableSortLink` URLs |
| `page` | same | `.Page` (overwritten by summary) | pagination links |
| `page_size` | same | `.PageSize` | select + hidden fields |
| `message`, `error` | `r.URL.Query().Get` `dashboard.go:51-52` | `.Message`, `.Error` | notices only |

Sorting: no dropdown. Each sortable heading is a link; links built in `renderRentDashboard` at `dashboard.go:66-73` using `sortLinkFor` + `normalisedSort`. Default sort is `status` (priority ordering), see `dashboard_filters.go:11-18`.

**`dashboard_filters.go` — confirmed owner of filter parsing.** Exported/package surface (all lowercase, package-internal):
- consts `dashboardDefaultPageSize=12`, `dashboardMaxPageSize=50`, `dashboardDefaultSort="status"` (`:11-18`)
- `type rentDashboardFilters{Search, Status, Sort string; Page, PageSize int}` (`:20-26`)
- `defaultRentDashboardFilters()` (`:28`)
- `rentDashboardFiltersFromQuery(url.Values) (rentDashboardFilters, error)` (`:32`) — reads `search/status/sort/page/page_size`, validates; `period` is **not** handled here
- `validateRentDashboardFilters` (`:64`) — allowed statuses and sort values
- `dashboardStatusMatches` (`:81`) — maps `unpaid` → open|overdue|partial
- `dashboardSearchMatches` (`:92`) — case-insensitive contains over TenantName/TenantAlias/RoomLabel/RoomAddress
- `filterAndSortRentDashboardRows` (`:105`) — filter + stable sort
- `paginateRentDashboardRows` (`:154`)
- `rentDashboardURL(period, search, status, sortValue string, page, pageSize int) string` (`:170`) — canonical URL builder; omits defaults (`status=all`, default sort, page 1, default page size, empty search) and first-sets `period`.

Called from `obligations.go:424-425` (filter/sort/paginate) and `dashboard_manual_balance.go:147` (redirect URL), plus the handler/template func.

**`sort_links.go` — confirmed.** Contains only:
- `type tableSortLink{URL string; Active bool; Arrow string}` (`:11-15`)
- `sortLinkFor(baseURL func(string) string, current, primary, secondary string) tableSortLink` (`:20`) — toggles primary/secondary, sets `Arrow` `▼`/`▲` based on `_asc` suffix; a single-direction column (`secondary==""`) gets no arrow.
- `normalisedSort(current, fallback string) string` (`:48`).

Also used by `/billing` (`main.go:146-148`, `756-758`) — it is shared, not dashboard-only.

### ≤640px rules (dashboard)

Page-local block `dashboard.go:290-337`. Quoted below (abridged to the load-bearing lines):

```css
@media (max-width: 640px) {
  .dashboard-summary { grid-template-columns: 1fr; }                    /* :291 */
  .dashboard-toolbar { justify-content: stretch; }                       /* :292 */
  .dashboard-toolbar .period-picker { display: grid; width: 100%; grid-template-columns: 44px minmax(0,1fr) 44px; ... } /* :293 */
  .dashboard-toolbar .period-label { display: none; }                    /* :294 */
  .list-filter { display: grid; grid-template-columns: 1fr; gap: 10px; } /* :296 */
  .list-filter .search-field, .list-filter .select-field { flex: none; width: 100%; min-width: 0; } /* :297 */
  .dashboard-pagination { flex-direction: column; align-items: stretch; } /* :298 */
  .list-filter .filter-actions { margin-left: 0; display: grid; grid-template-columns: repeat(2, minmax(0,1fr)); gap: 8px; } /* :301 */
  .section-head { align-items: flex-start; flex-direction: column; }     /* :303 */
  .count-chip.shown { margin-left: 0; }                                   /* :305 */
  .list-filter input, .list-filter select { min-height: 44px; }          /* :309 */
  a.count-chip { min-height: 44px; }                                      /* :317 */
  .tenant-link { border-bottom: 0; text-decoration: underline dashed var(--border-strong); ... } /* :331-335 */
}
```

There is **no rule that changes the dashboard list into anything but a horizontal-scrolling table** at ≤640px. The only structural change is the `.dashboard-summary` grid going to 1 column, the toolbar/filter stack, and the pager stacking.

Wider breakpoints: `dashboard.go:289` `@media (max-width: 900px) { .dashboard-summary { grid-template-columns: repeat(2, …) } }`.

### Row actions + expandable content

- **Row expansion (payment detail)**: row `<tr class="rent-row" ... data-details-target="rent-details-{{.ObligationID}}">` (`dashboard.go:405`) toggles the sibling `<tr id="rent-details-…">` (`dashboard.go:406`, `hidden` by default). JS at `dashboard.go:461-473` wires `[data-details-target]` click and Enter/Space. The detail row contains `.payment-list` with one `.payment-item` per `.Payments` entry (`rentPaymentDetail`, `main.go:308-316`) plus a 补录现金 link to `/cash-receipts/new`.
- **Per-row action**: 一键平账 POST form inside the status cell (`dashboard.go:405`), action `/rent-dashboard/settle`, guarded by `{{if gt .ExpectedCents .PaidCents}}`; carries hidden `obligation_id, period, search, status, sort, page, page_size`.
- **Dunning drawer**: `.section-actions` button `邮件催缴` with `data-dunning-open` / `aria-controls="dunning-drawer"` (`dashboard.go:391`), only when `{{if .Dunning.Enabled}}`. The drawer is `<section id="dunning-drawer" class="panel dunning-drawer" … {{if not .Dunning.Open}} hidden{{end}}>` (`dashboard.go:429-457`). JS `dashboard.go:474-484` (open/close + `scrollIntoView`). Data is `dunningDrawerData` (`dunning_handlers.go:17-35`), produced by `(*app).dunningDrawerForDashboard` (`dunning_handlers.go:149`). Drawer CSS `dashboard.go:259-287`; its own narrow block is `@media (max-width: 760px)` (`dashboard.go:287-288`), not 640.

---

## 2. `/tenants`

### Render path

- `handleTenants` (`main.go:896`): GET → `listTenantRecords` (`main.go:995` — DB service `tenantService.listTenants` `tenants.go:430` or JSON `loadTenants` `main.go:1959`); when DB-backed it additionally loads `listTenantBillingHistory` (`obligations.go:214`) for the last `tenantHistoryMonths = 6` months (`main.go:894`) and attaches each tenant's slice to `tenants[i].BillingHistory` (`main.go:919-924`); `requestCounts` for nav badges; `prepareTenants` (`main.go:1918`); builds `tenantPageData` (`main.go:944-962`); executes `tenantTemplate` (`main.go:964`).
- `ShowForm` is driven by `?edit=<id>` or `?add=1` (`main.go:934-943`).
- `tenantTemplate` FuncMap is `nil` (`main.go:2524`) — no custom template funcs.

### Table structure

`<thead>` literal at `main.go:2634`; `<table>` opened `main.go:2633` inside `<div class="table-wrap">` (`main.go:2632`). 7 columns:

| # | Label | Cell markup location / conditional |
|---|---|---|
| 1 | 租客 | `main.go:2638`: `{{if .DisplayAlias}}{{.DisplayAlias}}{{else}}{{.Name}}{{end}}`, then conditional secondary `{{.Name}}`, then `<span class="mono">{{.ID}}</span>` |
| 2 | 付款人 | `main.go:2639`: `{{if .PayerNameHint}}…{{else}}暂无付款人别名{{end}}` + 编号 `{{if .PayerID}}…{{else}}暂无{{end}}` |
| 3 | 租金 | `main.go:2640`: `<td class="amount">{{.RentDisplay}}</td>` |
| 4 | 账单安排 | `main.go:2641`: `每月 {{.DueDay}} 日` + `{{.RentStartDate}}{{if .RentEndDate}} 至 {{.RentEndDate}}{{end}}` |
| 5 | 房间 | `main.go:2642`: conditional `{{.RoomLabel}}`, `{{.RoomAddress}}`, conditional `{{.PropertyHint}}` |
| 6 | 创建时间 | `main.go:2643`: `<td class="mono">{{.CreatedAt}}</td>` |
| 7 | 操作 | `main.go:2644`: `.row-actions` with 详情 + 编辑 |

No sorting, no pagination, no filtering on this page.

### Row markup

- **Literal `<tr class="tenant-row">` inside `{{range .Rows}}`** (`main.go:2636-2645`). Not Go-concatenated.
- `.Rows` is `[]tenantRecord` (`main.go:185-209`) — template reaches into the **domain struct**; no per-row view model. Display string `RentDisplay` is precomputed by `prepareTenants` (`main.go:1918-1923`); `BillingHistory` is `[]tenantBillingMonth` (`obligations.go:38-49`).
- Each row is followed by a literal expansion `<tr id="tenant-billing-{{.ID}}" class="tenant-history-row" hidden>` (`main.go:2646-2657`) containing a **nested table** `.tenant-history-table` (its own `<thead>` `main.go:2650`: 月份/应收/实际收/未收/状态), whose rows are themselves literal `<tr class="tenant-month-row">` with a second-level expansion `<tr id="tenant-month-{{.ObligationID}}" class="tenant-month-details" hidden>` (`main.go:2653`).

### Summary / metrics blocks

`.summary` section `main.go:2585-2588`, two `.panel.metric` cards:
- 租客数量 `{{.TenantCount}}` — count is `len(tenants)` / nav count.
- 月租合计 `{{.RentTotal}}` — `formatMoney(sumTenantRent(tenants), "EUR", 2)` computed in the handler `main.go:958`; `sumTenantRent` at `main.go:1943`.

List header shows `{{.TenantCount}} 条记录` (`main.go:2630`).

### Filters

**None.** There is no filter form, no sort, no pagination. The only query params consumed are `edit` (`main.go:934`), `add` (`main.go:943`), `message` / `error` (`main.go:955-956`), all echoed into notices/form state. No URL-persisted filter state exists to preserve.

### ≤640px rules (tenants)

Page-local block `main.go:2557-2565`:

```css
@media (max-width: 640px) {
  .row-actions .btn { min-height: 44px; }                                /* :2558 */
  .table-wrap > table > thead > tr > th:nth-child(4),
  .table-wrap > table > tbody > tr > td:nth-child(4) { min-width: 112px; } /* :2562-2563 */
  table { min-width: 740px; }                                            /* :2564 */
}
```

Plus a `@media (max-width: 760px)` block for the nested payment list (`main.go:2556`) and the base `.status` pill styles (`main.go:2535-2539`). Again: no non-table layout — the table keeps a `740px` floor and scrolls horizontally with a frozen last column (see shared rules below).

### Row actions + expandable content

- **Row expansion**: `<tr class="tenant-row" tabindex="0" role="button" aria-expanded="false" aria-controls="tenant-billing-{{.ID}}" data-tenant-target="tenant-billing-{{.ID}}">` (`main.go:2637`) toggles the sibling `tenant-history-row`. JS `toggleDetails` + `[data-tenant-target]` loop at `main.go:2667-2682`; it explicitly ignores clicks on `a, button, input, select, textarea` (`main.go:2676`) so 详情/编辑 still work.
- **Nested month expansion**: `[data-month-target]` (`main.go:2652`) → `#tenant-month-{{.ObligationID}}` (`main.go:2653`); JS at `main.go:2683-2691` with `stopPropagation` so the month toggle does not collapse the tenant row.
- **Row actions**: `.row-actions` (`main.go:2644`) contains `<a class="btn subtle" href="/tenants/{{.ID}}">详情</a>` and `<a class="btn subtle" href="/tenants?edit={{.ID}}">编辑</a>`. No per-row POST actions.
- Expanded tenant row content: `最近六个月缴费` nested table (`main.go:2649-2655`) → each month → `.payment-list` of `.payment-item` (amount / date / description / reference / confirmation source) (`main.go:2653`).

---

## 3. `/expenses`

### Render path

- `handleExpenses` (`main.go:1062`): GET → `listExpenseRecords` (DB `expenseService.listExpenses` `expenses.go:117`, else JSON) → `requestCounts` → `prepareExpenses` (`main.go:1925`) → `expensePageData` (`main.go:1085-1100`) → `expenseTemplate.Execute` (`main.go:1102`).
- `expenseTemplate` FuncMap is `nil` (`main.go:2697`) — no custom template funcs.
- The page is a two-column layout: `<section class="grid-two">` (`main.go:2743`) with the add-expense `<form class="panel form">` (`main.go:2744`) on the left and the list `<section class="panel surface">` (`main.go:2770`) on the right.

### Table structure

`<thead>` literal at `main.go:2775`; `<table>` opened `main.go:2774`, wrapped in `<div class="table-wrap">` (`main.go:2773`). 6 columns:

| # | Label | Cell markup location / conditional |
|---|---|---|
| 1 | 描述 | `main.go:2778`: `<strong>{{.Description}}</strong><br><span class="mono">{{.ID}}</span>` |
| 2 | 金额 | `<td class="amount expense">{{.AmountDisplay}}</td>` |
| 3 | 类别 | `<td>{{.Category}}</td>` |
| 4 | 日期 | `<td>{{.DateDisplay}}</td>` |
| 5 | 备注 | `main.go:2778`: `{{if .RoomHint}}房间：…<br>{{end}}{{if .TenantHint}}租客：…{{else if not .RoomHint}}暂无备注{{end}}` |
| 6 | 付款方式 | `<td>{{.PaymentMethod}}</td>` |

No sorting, no pagination, no filters.

### Row markup

- **Literal `<tr>` inside `{{range .Rows}}`** (`main.go:2777-2779`). Not Go-concatenated.
- `.Rows` is `[]expenseRecord` (`main.go:211-225`) — template reaches into the **domain struct**; no per-row view model. Display fields `AmountDisplay` / `DateDisplay` are precomputed by `prepareExpenses` (`main.go:1925-1931`) and by `expenseRecordFromModel` (`expenses.go:129-145`).
- Empty-state at `main.go:2783`.

### Summary / metrics blocks

`.summary` section `main.go:2738-2741`, two `.panel.metric` cards:
- 支出笔数 `{{.ExpenseCount}}`
- 支出合计 `{{.ExpenseTotal}}` — `formatMoney(sumExpenses(expenses), "EUR", 2)` computed in the handler `main.go:1099`; `sumExpenses` at `main.go:1951`.

### Filters

**None.** Only `message` / `error` query params (`main.go:1096-1097`). No sort, no pagination, no URL-persisted filter state.

### ≤640px rules (expenses)

Page-local block `main.go:2704-2720`:

```css
@media (max-width: 640px) {
  .table-wrap > table > thead > tr > th:nth-child(3),
  .table-wrap > table > tbody > tr > td:nth-child(3) { min-width: 76px; }  /* :2708-2709 */
  .table-wrap > table > thead > tr > th:nth-child(4),
  .table-wrap > table > tbody > tr > td:nth-child(4) { min-width: 125px; } /* :2715-2716 */
  table { min-width: 700px; }                                               /* :2717 */
  .panel.form > .btn.primary { width: 100%; }                               /* :2719 */
}
```

The form/list two-column layout collapses at ≤980px via the shared rule `.grid-two { grid-template-columns: 1fr; }` (`main.go:2407`). That is a layout collapse, **not** a card rendering of the list.

### Row actions + expandable content

None. Expenses rows have no actions, no expansion, no nested content.

---

## Cross-cutting questions

### Does any of the three pages already render a non-table layout at any breakpoint?

**No.** For all three pages the rows are always `<tr>` and the list is always `<table>` inside `.table-wrap`. The only breakpoint-driven changes are:
- metric/summary grids reflow (`.summary` → 1 col at ≤640 `main.go:2416`; `.dashboard-summary` 4→2 cols at ≤900 `dashboard.go:289`, →1 col at ≤640 `dashboard.go:291`);
- filter/toolbar/pager stacking (dashboard only);
- `.grid-two` collapsing to one column at ≤980 (`main.go:2407`) — expenses form stacks above the list, both still tables;
- the table gets a **minimum width floor** (680px shared `main.go:2418`; 740px tenants `main.go:2564`; 700px expenses `main.go:2717`) and scrolls horizontally inside `.table-wrap`.

At ≤640 the shared CSS also **freezes the last column** (sticky right) — `main.go:2483-2490`:

```css
.table-wrap > table > thead > tr > th:last-child,
.table-wrap > table > tbody > tr > td:last-child:not([colspan]) {
  position: sticky; right: 0; background: var(--surface);
  box-shadow: -8px 0 8px -8px rgba(0,0,0,0.18);
}
```

No `display:none` is ever applied to a table or a `<thead>` on these pages. There is no card-list markup anywhere in `dashboard.go`, `tenants.go`, or the tenants/expenses templates.

### Shared metric/summary card CSS

- The shared metric card class used by these three pages is **`.metric`**, defined in `workspacePageCSS` at `main.go:2361-2364`:
  ```css
  .metric { padding: 18px; min-height: 118px; }
  .metric strong { display: block; margin-top: 12px; font-size: clamp(26px, 3vw, 38px); line-height: 1; font-weight: 800; }
  .metric span, .tiny { display: block; margin-top: 8px; color: var(--foreground-muted); font-size: 12px; line-height: 1.55; }
  ```
  Its container **`.summary`** is `main.go:2359`: `display: grid; grid-template-columns: repeat(2, minmax(0,1fr)); gap: 12px; margin-bottom: 18px;` with the ≤640 override to `1fr` at `main.go:2416`. Dashboard uses a 4-up variant `.dashboard-summary` (`dashboard.go:201`, overrides at `:289`, `:291`) and attaches extra `metric-primary` / `metric-success` / `metric-warning` / `metric-link` classes (`dashboard.go:243-245`, `374-377`) — those colour modifiers are **dashboard-local**, not in `workspacePageCSS`.
- **`.summary-card` is NOT part of the shared stylesheet.** It is defined only inside the cash-receipt page's own inline `<style>` in `cash_receipt_handlers.go:215`, together with `.summary-grid`:
  ```css
  .summary-grid{display:grid;grid-template-columns:repeat(4,minmax(0,1fr));gap:10px;margin:18px 0}
  .summary-card{padding:14px;border:1px solid var(--border);border-radius:8px;background:var(--surface-muted)}
  .summary-card strong{display:block;margin-top:6px;font:700 18px var(--mono)}
  @media(max-width:700px){.cash-form,.summary-grid{grid-template-columns:1fr} ...}
  ```
  It is used only on the cash-receipt preview markup (`cash_receipt_handlers.go:216`) and is scoped to that page's template clone. `/rent-dashboard`, `/tenants` and `/expenses` do not use `.summary-card` or `.summary-grid`.
- `.sync-card` (`main.go:3116`) belongs to the legacy billing template — out of scope.

### What is reusable for a card layout

- **Template partials**: only `{{define "workspace-nav"}}` (`workspace_shell.go:46`). There are **no** row/table/card partials for these three pages; every table and row is inline in its page template.
- **FuncMap helpers**: only the dashboard registers a FuncMap (`dashboard.go:141-170`): `rentDashboardURL`, `dashboardPreviousPage`, `dashboardNextPage`, `dunningDeliveryLabel`. Tenants/expenses register none; they would need any card-shape helpers added to their own `newWorkspacePageTemplate(..., nil, ...)` call, or the funcs hoisted. Formatting (`formatMoney`, `formatDate`, `formatMonthLabel`, `rentStatusLabel`) is done in Go, not exposed to templates.
- **CSS classes available in every page** (from `workspacePageCSS`): `.panel`, `.panel-head`, `.surface`, `.metric`, `.summary`, `.label`, `.tiny`, `.mono`, `.amount` (+ `.amount.expense`), `.empty`, `.notice`, `.btn` (+ `.primary`/`.danger`/`.subtle`), `.grid-two`, `.table-wrap`, and existing layout breakpoints at 980px/640px.
- **Page-local reusable-looking pieces**: dashboard `.status` pill modifiers and `.status-actions` (`dashboard.go:235-240`, duplicated in tenants `main.go:2535-2539`); dashboard `.count-chip` (`dashboard.go:203-207`); `.rent-row`/`.rent-details`/`.payment-list`/`.payment-item` (`dashboard.go:246-255`); tenants `.tenant-row`/`.tenant-history-row`/`.tenant-history-table`/`.tenant-month-details`/`.row-actions` (`main.go:2531-2556`). These are duplicated per page rather than shared.
- **Expansion JS pattern**: identical `toggleDetails` + `[data-*-target]` pattern appears both in dashboard (`dashboard.go:461-473`) and tenants (`main.go:2666-2692`), written per page.
- **Filter state plumbing** (`dashboard_filters.go`): `rentDashboardFilters` + `rentDashboardURL` + `rentDashboardFiltersFromQuery` are reusable only in the sense that they are package-level and already consumed by `dashboard_manual_balance.go`; they are hard-coded to `/rent-dashboard` (see `rentDashboardURL` returning `"/rent-dashboard?" + …`, `dashboard_filters.go:187`).

---

## Caveats / Not Found

- `/rent-dashboard` route registration line was not isolated (it is in the `main.go` mux block near lines 400-420, where `/tenants`=412 and `/expenses`=418); only the handler and template locations are asserted here.
- The dashboard's no-DB fallback branch (`dashboard.go:120-134`) populates a different subset of metric fields than the DB branch; a card layout must not assume `ExpectedTotal`/`CollectionPercent`/`Dunning` are always non-zero (fallback leaves `CollectionPercent` at 0 and does not set `ExpectedTotal` from obligations).
- `tenantRecord.BillingHistory` is attached in the handler (`main.go:919-924`) only when a DB-backed user session exists; the JSON-file fallback path leaves it nil and the expanded row renders the 最近六个月暂无适用账单 empty state.
- `expenses.go` contains no page code; anything that looks like "the expenses page" in that file is the service/model layer only.
- No external references were needed; this is a pure internal map.
