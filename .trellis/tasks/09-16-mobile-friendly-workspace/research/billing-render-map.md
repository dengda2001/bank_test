# Research: /billing render map (mobile card-stream planning)

- **Query**: Precise map of how `/billing` renders today (handler, data struct, template, table, rows, filters, ≤640px rules, per-row actions, reusable pieces).
- **Scope**: internal (Go + html/template, `cmd/truelayer-demo/`)
- **Date**: 2026-09-17

## 1. Render path

| Piece | Location |
|---|---|
| Route registration `mux.HandleFunc("/billing", a.handleBilling)` | `cmd/truelayer-demo/main.go:402` |
| GET handler `handleBilling` | `cmd/truelayer-demo/main.go:637-777` |
| Template constant `billingTemplate` | `cmd/truelayer-demo/billing_page.go:3-220` |
| Data struct `billingPageData` | `cmd/truelayer-demo/main.go:127-160` |
| Per-row view model `transactionPageRow` | `cmd/truelayer-demo/transactions.go:371-412` |
| Filter struct `transactionFilters` | `cmd/truelayer-demo/transactions.go:293-308` |
| Filter parsing `filtersFromQuery` | `cmd/truelayer-demo/transactions.go:244-281` |
| Filter validation `validateTransactionFilters` | `cmd/truelayer-demo/transactions.go:310-369` |
| Row loading (DB) `listTransactionPageRowsWithTotal` | `cmd/truelayer-demo/matching_service.go:100` |
| Row loading (demo fallback) `fallbackTransactionPageRows` | `cmd/truelayer-demo/transactions.go:686` |
| Template constructor `newWorkspacePageTemplate` | `cmd/truelayer-demo/workspace_shell.go:65-71` |

`billingTemplate` is built with `newWorkspacePageTemplate("billing", nil, …)` — the **FuncMap is `nil`**, so the billing template has no page-local helper functions. It is a clone of `workspaceBase` (`workspace_shell.go:60`) and calls `{{template "workspace-nav" .}}` at `billing_page.go:137`.

The handler builds `data := billingPageData{…}` at `main.go:728-769` and executes at `main.go:774`. Note `billingPageData` embeds `workspaceShell` (`main.go:128`), so nav fields (`ActivePage`, `Username`, `Environment`, counts) come along.

POST actions are separate mux entries (`main.go:403-410`): `/billing/confirm`, `/billing/rematch`, `/billing/allocate`, `/billing/ignore`, `/billing/restore`, `/billing/revoke`, `/billing/payer/preview`, `/billing/payer/confirm`. They redirect back to `/billing` (usually with `?message=`/`?error=`), they do not render the template.

## 2. Table structure

`<thead>` is a single literal on `billing_page.go:184`; `<tbody>` is `{{range .TransactionRows}}` on `billing_page.go:185`.

Column classes are `txn-col-*`. There are **no per-column `min-width` declarations** — the only width floors are table-level: `.transaction-table { min-width: 1040px; }` (`billing_page.go:30`) overridden to `680px` at ≤640px (`billing_page.go:106`). The action column also gets `width: 118px` at ≤640px (`billing_page.go:115`).

| # | Header label | `<th>` class | Cell markup (line) | Conditionally rendered? | Hidden ≤640px |
|---|---|---|---|---|---|
| 1 | 类型 | `txn-col-type` | `:187` `<span class="direction {{.Direction}}">{{.DirectionLabel}}</span>` | no | no |
| 2 | 付款人 (sort link) | `txn-col-payer` | `:188` `<div class="match-metadata"><strong>{{.PayerName}}</strong></div>` | no | no |
| 3 | 金额／余额 (sort link) | `txn-col-amount` | `:189` `<span class="amount…">{{.AmountDisplay}}</span>` + 已分配/余款 `.tiny` | no | **yes** (`:114`) — amount is re-rendered inside col 7 as `.txn-amount-mobile` |
| 4 | 到账／租金月 (sort link) | `txn-col-date` | `:190` `{{.DateDisplay}}` + `租金月：{{.FinalPeriodDisplay}}` (or 未确认) | no (inner label is `{{if}}`) | no |
| 5 | 描述 | `txn-col-desc` | `:191` `<div class="description">{{.Description}}</div>` | no | **yes** (`:107-108`) |
| 6 | 账户 | `txn-col-account` | `:192` `{{.AccountName}}` | no | **yes** (`:107-108`) |
| 7 | 用途／处理 | `txn-col-action` | `:193` one long line: `<details class="txn-action" open><summary class="txn-summary">…</summary><div class="txn-body">…all actions…</div></details>` | no | narrowed to 118px, summary becomes the collapse trigger |

Header labels are literal text except the three `<a class="sort-link">` wrappers (付款人/金额／余额/到账／租金月) which carry `{{.PayerSort.URL}}` / `{{.AmountSort.URL}}` / `{{.ArrivalSort.URL}}` and an optional arrow. Sorting is **not** offered on 类型/描述/账户/用途／处理.

## 3. Row markup — the key question

**A row is a literal inline template inside a `{{range}}`, rendering a per-row view model struct directly.** It is NOT string-concatenated in Go, and NOT a `{{range}}` over domain structs.

- `transactionPageRow` (`transactions.go:371-412`) is a purpose-built view model. It carries pre-formatted display strings (`AmountDisplay`, `AllocatedAmountDisplay`, `RemainingAmountDisplay`, `DirectionLabel`, `MatchStatusLabel`, `DateDisplay`, `ParsedPeriodDisplay`, `FinalPeriodDisplay`, `Description`, `AccountName`) and every action flag/option list (`CanConfirm`, `CandidateTenantName`, `CandidateRentObligationID`, `CandidatePeriod`, `TenantID`, `NeedsMonthChoice`, `MonthOptions`, `ManualMatchOptions`, `CanRematch`, `CanEditRentMatch`, `RematchOptions`, `RematchTenantOptions`, `RematchMonthOptions`). No template helper funcs are needed because formatting happens in Go.
- The template reaches only into this struct (`{{.AmountDisplay}}`, `{{.MonthOptions}}`, `$.TenantOptions`), never into `paymentTransaction` domain structs.
- Each `<tr class="{{.Direction}}">` (`:186`) has exactly 7 `<td>`s; all cells are literal, one per line (`:187`-`:193`). The action cell is a single 1-line literal of ~4 KB — everything is on `:193`.

Implication for any card layout: all data needed is already a flat view model on `transactionPageRow`; a card could be rendered from the same `{{range .TransactionRows}}` without new plumbing. The only cross-row data referenced is `$.TenantOptions` (needed by the allocation form's tenant select).

## 4. Filters

Form: `billing_page.go:167-179` — one `<form class="filterbar" method="get" action="/billing">`. Layout is a CSS grid of **4 columns** (`.filterbar { grid-template-columns: repeat(4, minmax(130px,1fr)); }`, `:10`), 2 columns at ≤820px (`:74`), 1 column at ≤480px (`:75`). There is **no 首要/更多 grouping today** — all controls are in one flat grid.

Visible controls (all values are read back from the URL via the `billingPageData` fields set at `main.go:746-754`):

| Control (order) | name | type | Line | Bound field | URL read-back |
|---|---|---|---|---|---|
| 付款人 | `payer` | `<input type="search">` | `:168` | `PayerFilter` | `filters.Payer` |
| 租客 | `tenant_id` | `<select>` | `:169` | `TenantFilter`, `TenantOptions` | `filters.TenantID` |
| 流水到账月 | `period` | `<input type="month" onchange="this.form.submit()">` | `:170` | `PeriodFilter` | `filters.PeriodMonth` |
| 租金所属月 | `rent_period` | `<input type="month">` | `:171` | `RentPeriodFilter` | `filters.RentPeriod` |
| 入账用途 | `allocation` | `<select>` (rent/deposit/other_income) | `:172` | `AllocationFilter` | `filters.AllocationKind` |
| 收支 | `direction` | `<select>` (income/expense) | `:173` | `DirectionFilter` | `filters.Direction` |
| 匹配状态 | `match_status` | `<select>` (pending/matched/partial/candidate/unmatched/needs_review/ignored) | `:174` | `MatchStatusSelection` | `filters.MatchStatus` (+ synthetic `pending`) |
| 应用筛选 / 清除筛选 | — | submit + `<a href="/billing">` | `:178` | — | — |

Hidden carry-through inputs inside the same form: `arrival_from` (`:175`), `arrival_to` (`:176`), `sort` (`:177`). Note: `arrival_from`/`arrival_to` have **no visible control** anywhere on the page — they are preserved only when already present in the URL (e.g. arriving from a dashboard link).

Sort state:
- Parsed from `?sort=` (`filtersFromQuery`, `transactions.go:276`); validated to one of `"" | arrival_desc | arrival_asc | amount_desc | amount_asc | payer_asc | payer_desc` (`transactions.go:360-364`). Empty means newest-first "arrival_desc" (`main.go:723-726`).
- Built into `tableSortLink` values by `sortLinkFor` (`sort_links.go:20-43`); URLs from `billingSortURL` (`main.go:791-803`) which copies all current query params, **drops `page`**, sets `sort`.
- The filter form re-emits current sort as the hidden `sort` input (`:177`).

Pagination state:
- `page`, `page_size` parsed at `transactions.go:245-252`; page size options 25/50/100.
- Block `billing_page.go:198-213`. The page-size `<form>` (`:199-211`) re-emits **all** active filters as hidden inputs (`arrival_from`, `arrival_to`, `payer`, `tenant_id`, `period`, `rent_period`, `allocation`, `direction`, `match_status`, `sort`) so changing page size keeps filters.
- Prev/next URLs are precomputed in Go as `PreviousPageURL`/`NextPageURL` via `billingPageURL` (`main.go:779-786`), which clones the query and sets `page`.

## 5. Existing ≤640px rules for this page

All in `billing_page.go`. The relevant **wide** block first (`@media (min-width: 641px)`, `:78-84`):

```css
.transaction-table .txn-col-action > .txn-action > summary { display: none; }
.transaction-table .txn-col-action > .txn-action::details-content { content-visibility: visible !important; }
```

The **narrow** block (`@media (max-width: 640px)`, `:85-131`):

```css
.confirm-form .btn,
.bind-form select, .bind-form .btn,
.month-choice-form select, .month-choice-form .btn,
.action-form input, .action-form .btn,
.allocation-form select, .allocation-form input, .allocation-form .btn { min-height: 44px; }        /* :87-91 */
.filterbar .btn { min-height: 44px; }                                                              /* :94 */
.pagination .page-size-field select { min-height: 44px; }                                          /* :95 */
.bind-form label.tiny, .month-choice-form label.tiny { display: inline-flex; align-items: center; gap: 8px; min-height: 44px; }  /* :101 */
.confirm-form input[name="remember_payer"] { width: 44px; height: 44px; padding: 15px; }           /* :102 */
.transaction-table { min-width: 680px; }                                                           /* :106 */
.transaction-table .txn-col-desc,
.transaction-table .txn-col-account { display: none; }                                             /* :107-108 */
.transaction-table .txn-col-amount { display: none; }                                              /* :114 */
.transaction-table .txn-col-action { width: 118px; padding: 10px 8px; }                            /* :115 */
.transaction-table .txn-action > summary { list-style: none; }                                     /* :116 */
.transaction-table .txn-action > summary::-webkit-details-marker { display: none; }                /* :117 */
.transaction-table .txn-col-action > .txn-action > summary { display: grid; gap: 6px; justify-items: start; cursor: pointer; }  /* :118 */
.transaction-table .txn-col-action > .txn-action > summary::after { content: "▾ 处理"; color: var(--accent); font-size: 12px; }  /* :119 */
.transaction-table .txn-col-action > .txn-action[open] > summary::after { content: "▴ 收起"; }    /* :120 */
.transaction-table .txn-col-action > .txn-action[open] > .txn-body { margin-top: 8px; }            /* :121 */
.txn-amount-mobile { display: block; font-weight: 700; font-variant-numeric: tabular-nums; white-space: nowrap; }  /* :129 */
.txn-amount-mobile.expense { color: var(--danger); }                                               /* :130 */
```

Page-local default (all widths): `.txn-amount-mobile { display: none; }` (`:32`).

The shared ≤640px block (`main.go:2409-2521`, inside `workspacePageCSS`) also applies to this page: `table { min-width: 680px; }` (`:2418`), the sticky-last-column freeze (`:2483-2490`), `td.mono { white-space: nowrap; }` (`:2497`), the `th .sort-link` 44px hit area (`:2502`), and the blanket `input, select, textarea, .btn { min-height: 44px; }` (`:2518`). The billing page's own blocks are appended after `workspacePageCSS` (template order at `billing_page.go:9`), so the page-local 30px/34px rules at lines 38-51 would override the shared 44px unless re-raised (which lines 87-91 do).

## 6. The `<details class="txn-action">` collapse

Markup: the whole action cell is `<details class="txn-action" open>` on `billing_page.go:193`, with `<summary class="txn-summary">` holding `<span class="status {{.MatchStatus}}">{{.MatchStatusLabel}}</span>` and `<span class="txn-amount-mobile…">{{.AmountDisplay}}</span>`; the rest is inside `<div class="txn-body">`.

The collapse script is the tail of the one inline `<script>` at `billing_page.go:217`:

```js
if(window.matchMedia('(max-width: 640px)').matches){
  document.querySelectorAll('details.txn-action[open]').forEach(function(node){node.removeAttribute('open');});
}
```

The same script does two other page-local mutations:
1. Rewrites every `form[action="/billing/allocate"]`: renames the 5 controls (`allocation_kind`, `tenant_id`, `period`, `amount`, `note`) to `name[]`, wraps them in a `.allocation-line`, and injects an 添加拆分项 button that clones the line.
2. Rewrites every `form[action="/billing/revoke"]`: sets `method='get'` and **removes** the `reason` input, so revoke navigates to the GET preview page (`handleTransactionRevokePreview`, `transaction_handlers.go:201`) instead of POSTing directly.

There is also a `<script>{{workspaceCalendarScript}}</script>` at `:133` (shared, unrelated).

## 7. Per-row actions

All are inside the single `txn-body` at `:193`. Conditions use `{{if}}` chains on the row's status/direction flags.

| Action | Surfaced as | Form `action` | Key params | Condition |
|---|---|---|---|---|
| Status badge link (filters by this status) | `<a class="status status-link {{.MatchStatus}}" href="/billing?match_status={{.MatchStatus}}">` | — (GET link, drops other filters) | `match_status` query | always |
| 一键匹配 | `<form class="confirm-form" method="post">` with hidden ids + remember checkbox | `/billing/confirm` | `transaction_id`, `rent_obligation_id`, `remember_payer` | `.CanConfirm` |
| 确认匹配（月份选择） | `<form class="month-choice-form" method="post">`, `<select name="period">` from `.MonthOptions` | `/billing/confirm` | `transaction_id`, `tenant_id`, `period`, `remember_payer` | `.NeedsMonthChoice` |
| 确认匹配（手动选租客+租金月） | `<form class="bind-form" method="post">`, select from `.ManualMatchOptions` | `/billing/confirm` | `transaction_id`, `rent_obligation_id`, `remember_payer` | income, not matched/ignored, `.ManualMatchOptions` non-empty |
| empty hint 没有可直接匹配的租金月… | `.tiny.bind-empty` div | — | — | income, not matched/ignored, no manual options |
| 归类／拆分 | `<details class="allocation-details"><summary>归类／拆分</summary>` wrapping `<form class="allocation-form">` | `/billing/allocate` | `transaction_id`, `allocation_kind`, `tenant_id`, `period`, `amount`, `note` (JS rewrites to `name[]`, repeatable) | income && not matched && not ignored |
| 恢复处理 | `<form class="action-form" method="post">` | `/billing/restore` | `transaction_id`, `reason` | `.MatchStatus == "ignored"` |
| 修改匹配 | `<details class="rematch-details"><summary class="btn">修改匹配</summary>` wrapping `<form class="rematch-form">` | `/billing/rematch` | `transaction_id`, `tenant_id`, `period` | `.CanEditRentMatch` |
| hint 当前没有可替换的租金目标… / 该流水已拆分或含其他用途… | `.tiny` div | — | — | `.CanRematch` / else, within matched|partial |
| 撤销匹配 | `<form class="action-form" method="post">`; **JS flips to GET and drops `reason`** | `/billing/revoke` | `transaction_id` (POST would also send `reason`) | matched or partial |
| 无需匹配 (ignore) | `<form class="action-form" method="post">` | `/billing/ignore` | `transaction_id`, `reason` | income, not matched/ignored (and not covered above) |

Page-level (not per-row) actions in the topbar: 刷新银行数据/连接银行账户 link (`:142`), 历史付款人预览 POST `/billing/payer/preview` (`:143`), 导入旧数据 POST `/import-legacy` (`:144`), 退出登录 POST `/logout` (`:145`).

## 8. What is reusable for a card layout

- **Shared nav partial**: `{{define "workspace-nav"}}` (`workspace_shell.go:46-58`), already invoked by the page.
- **Per-row view model**: `transactionPageRow` already contains every display string and action flag a card needs (see §3). No new Go data plumbing required for a card that renders the same fields.
- **Existing per-row conditionals** (`CanConfirm`, `NeedsMonthChoice`, `ManualMatchOptions`, `CanEditRentMatch`, `CanRematch`, `MatchStatus`) are all on the row struct and can drive the same action set in a card.
- **Shared CSS classes** from `workspacePageCSS` (`main.go:2164-2522`): `.panel`, `.surface`, `.panel-head`, `.table-wrap`, `.amount`/`.amount.expense` (`:2399-2400`), `.mono` (`:2401`), `.tiny` (`:2364`), `.btn`, `.notice`, `.empty`, plus the ≤640px sticky/44px rules. Reusable card-relevant classes defined page-locally: `.status` + status variants (`.matched/.candidate/.unmatched/.needs_review/.partial/.ignored`, `:20-26`), `.status-link` (`:27-28`), `.direction` (`:33-35`), `.description` (`:29`), `.match-metadata` (`:36`), `.confirm-form`, `.bind-form`, `.month-choice-form`, `.action-form`, `.allocation-form`, `.allocation-line`, `.allocation-details`, `.rematch-details`, `.txn-amount-mobile` (`:129-130`).
- **Cross-row option list**: `$.TenantOptions` is already passed to the template and used by the allocation form's tenant select (`:193`); a card's allocation UI can reuse it.
- **Sort link type/builder**: `tableSortLink` + `sortLinkFor` (`sort_links.go`) and `billingSortURL` (`main.go:791`) are reusable for a card-header sort control.
- **Pagination URL builders**: `billingPageURL` (`main.go:779`) and the page-size form's hidden-filter pattern (`:200-209`) are reusable for mobile pager placement.
- **No page-local FuncMap** exists today, so nothing extra is registered to reuse; adding card helpers would require switching `newWorkspacePageTemplate("billing", nil, …)` to a non-nil FuncMap (`workspace_shell.go:65-71`).

## Caveats / Not Found

- No `首要筛选` / `更多筛选` grouping exists anywhere on `/billing` today; the filter form is one flat 4-column grid.
- No per-column `min-width` for the billing table in CSS (the comment at `billing_page.go:109-113` cites measured widths of 136-146px for the amount column, but those are measurements, not declarations).
- `arrival_from` / `arrival_to` have no visible controls; they survive only as hidden inputs when already in the URL.
- The status-link action is a bare GET link (`/billing?match_status=…`) that drops all other active filters.
- The revoke form's `reason` input is removed and method flipped to GET by JS at runtime; the server-side POST still accepts `reason` (`transaction_handlers.go:121`). So the row's revoke path differs between JS-on (preview page) and JS-off (direct POST).
- The billing template strips CSS comments at render (noted in `mobile_layout_test.go:351`), so tests anchor on declarations, not comments.
