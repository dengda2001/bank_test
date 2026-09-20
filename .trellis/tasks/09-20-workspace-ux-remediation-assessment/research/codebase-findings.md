# Codebase findings

## Current architecture

- The product is a Go `html/template` server-rendered application. Shared chrome is in `web/templates/partials/workspace-nav.html`; newer pages use embedded templates and page CSS, while tenants, transactions, bills and some detail pages still use Go string templates.
- The shared calendar implementation already exists in `web/static/js/calendar.js` and `web/static/css/calendar.css`, but it is loaded only by a subset of pages. This explains the visual drift between transaction matching and property, room, tenancy, cash receipt, bill and dashboard controls.
- Existing list pagination has four separate renderers: monthly workspace, bills, transactions and tenant billing history. Defaults are split between `dashboardDefaultPageSize = 12`, `defaultTenantHistoryPage = 12`, and transaction page size `50`.

## Requirement mapping

| # | Evidence and likely change | Size | Main risk |
|---|---|---:|---|
| 1 | Property and room detail actions link to `/expenses?...&add=1`. The expense drawer is embedded only in `expenses.html`; its property and room selects do not read the query IDs, and POST always returns to `/expenses`. Extract the expense drawer as a shared partial/view model, render it in property and room detail, preselect or lock the contextual asset, and validate a safe return target after save/error. | M | Preserving detail/list return state and preventing cross-user asset IDs. |
| 2 | Confirmed template bug in `rent_collection_pages.go`: the “unpaid” option is marked selected when status is either `unpaid` or `all`, while the `all` option never marks itself selected. Fix the selected conditions and cover query round-trip. | XS | Very low. |
| 3 | Existing pagers expose only previous/next and text. Add a shared bounded page-window helper and page links to workspace, bills, transactions and tenant history while retaining previous/next and all active filters. | M | Query-state drift across four URL builders; large page counts need ellipses/windowing. |
| 4 | Dashboard desktop renders one month input plus “查看”; mobile already has previous/next links. Load the shared calendar on this page and render previous/current/next as one consistent control at all widths. | S | Keeping view/filter state in month navigation URLs. |
| 5 | Visible filter submit labels occur in bills, transactions, tenancies, cash receipts, expenses and the dashboard; properties/rooms use a disclosure button also labeled “筛选”. Rename action/disclosure labels to “搜索” without changing query behavior. | XS | Copy assertions and mobile disclosure tests. |
| 6 | The shared calendar auto-enhances every `date` and `month` input, but many embedded pages never load its script or stylesheet. Centralize calendar assets in the workspace template constructors and remove page-specific duplicate injection. This reaches property/room forms, tenancies, bills, dashboard, cash receipts, expenses, tenants and transaction detail. | M | Duplicate initialization, readonly/disabled inputs, popover clipping and mobile bottom-sheet geometry. |
| 7 | `/tenants` loads and renders all records and has no `search` field or filter step. Add search state, a list form and filtering across name, display alias, email, payer hints and room/property text. | S | Keep total tenant counts distinct from filtered row counts; edit links must retain search state. |
| 8 | Tenant detail edit links point to `/tenants?edit=<id>`, and the only tenant drawer lives in the list template. Extract the drawer, load its room options in the detail handler, render on `/tenants/<id>?edit=1`, and return POST success/error to the same detail page. | M | The current form mixes tenant profile, arrangement and room binding rules; return handling must not weaken validation. |
| 9 | Defaults are 12 for workspace/bills/tenant history and 50 for transactions. Change the shared/default constants and selectors to 10. Existing unpaginated object lists remain outside this requirement unless pagination is separately requested. | S | Tests and links that hard-code 12/50; transaction query load changes substantially but safely. |
| 10 | Bank payer already exists as `payment_transactions.payer_name` and is exposed as `PayerName`. Matched tenant ID exists, but list rows do not expose a matched tenant display name. The `/transactions` desktop table currently labels column two “付款记录”; manual matching combines tenant/month in `rent_obligation_id`, although the backend already accepts separate `tenant_id` + `period`. Add `MatchedTenantName`, a dedicated column, split selectors, and expose the existing detail match action as a direct right-side action. | M | Tenant/month choices must be dependent so an invalid pair cannot be submitted; preserve partial and rematch flows. |
| 11 | Dashboard currently counts all pending rows but loads only `Limit(2)`. Queue items contain summary text and two links, not inline handling. Expand the dashboard view model with payer/time/note/amount plus match options, load three decorated transaction rows, render standalone cards and post existing match actions back to the dashboard. “暂不处理” is confirmed as a distinct persistent `deferred` action/state. | L | Concurrency/refill behavior, preserving month context, and keeping `deferred` separate from the existing semantic “ignored/non-rent” state. |
| 12 | There are about 42 native `<select>` controls across legacy and embedded templates. Closed controls are styled, but the operating-system popup cannot be fully restyled. Meeting the anchored, light system style requires one accessible custom-select enhancement with native-select synchronization and progressive fallback. | L | Keyboard, focus, screen-reader, form submission, dynamic options and mobile behavior across every page. |
| 13 | Cash receipts and expenses have sidebar links and also appear on `/more`. Transaction matching has room in its header for actions. Both drawers are page-local and their POST/preview return contracts are page-specific. Remove the two sidebar entries, add “现金补录” and “新增支出” to `/transactions`, extract both drawers as shared partials and extend safe return handling so preview/save stays in the transaction context. Keep their standalone list routes for history and later edits. | L | Cash preview is a two-step flow; both drawers must preserve transaction filters and validation errors. |
| 14 | Search fields rely on the browser-provided cancel glyph and no shared hit-area rule exists. Add a shared search-field wrapper/custom clear button or a WebKit cancel-button enlargement, with a 44px mobile target and keyboard label. | S | Browser pseudo-element sizing is inconsistent; an explicit button is more reliable. |
| 15 | The shared topbar renders `TopSearch` and a pending-review count link; `fillWorkspaceShell` also runs database work for the count on every page. Remove both widgets and their shell plumbing/tests while retaining the breadcrumb. | S | Do not remove page-local search or the dashboard queue. |

Size legend: XS = under half a day; S = about half to one day; M = about one to two days; L = about two to four days, including targeted automated and browser verification. Estimates assume one engineer familiar with this repository and include refactoring needed to avoid duplicate drawers/components.

## Cross-cutting opportunities

1. A shared control layer can handle requirements 4, 6, 12 and 14.
2. A shared pagination helper and URL-state contract can handle requirements 3 and 9.
3. Shared drawer partials and safe return-target handling can handle requirements 1, 8 and 13.
4. A reusable transaction-match form/view model can handle requirements 10 and 11.

## Confirmed implementation constraints

- The previously agreed matching contract keeps “remember payer” checked by default but optional.
- Match status, allocation kind and bill payment status are separate concepts and must remain separate.
- Cash receipt remains a cash-only rent receipt flow and must not be generalized into arbitrary manual income.
- Existing user file `收租明细_Rosewood_20260916.xlsx` is unrelated and must remain untouched.

## Confirmed defer contract

“暂不处理” does not reuse the existing `ignored` status because that status means “non-rent / no matching required”. The confirmed behavior introduces a distinct `deferred` transaction state/action. It is excluded from the dashboard pending queue, remains visible and directly matchable in the transaction list, and matching replaces the deferred state with the resulting matched/partial state. This is a model and workflow change but can use the existing status column and action audit table without a schema migration.
