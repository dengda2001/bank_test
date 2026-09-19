# Responsive Conventions

> Executable contracts for the narrow-screen rendering of the workspace pages.
> Every rule here is asserted by `cmd/truelayer-demo/mobile_layout_test.go`.

---

## 1. Scope / Trigger

Read this before any of:

- adding or editing a rule inside a page's `<style>` block
- touching `workspacePageCSS` (the shared stylesheet)
- adding a page, or a column to an existing table
- changing `workspaceNav` or `workspaceShell`

The desktop rendering is a frozen contract. It was captured before this work as a
`:8082` reference build, and moved only once, deliberately (see §5). Anything you
add has to land on the narrow side of the breakpoint, or be justified in a
comment as a deliberate desktop change.

---

## 2. Signatures

```go
// The shared stylesheet. Concatenated into every workspace page's <style>,
// BEFORE that page's own rules.
const workspacePageCSS = `...`

// The shared chrome. Defined once; every page template is cloned from
// workspaceBase and calls {{template "workspace-nav" .}}.
const workspaceNav = `{{define "workspace-nav"}}...{{end}}`

type workspaceShell struct {
    ActivePage    string // canonical page key: "rent-dashboard" | "bills" | "transactions" | "dunning" | "properties" | "rooms" | "tenants" | "tenancies" | "cash-receipts" | "expenses" | "bank" | "more"; legacy "billing" remains supported
    Username      string
    Environment   string
    FootNote      string
    ShowNavCounts bool   // false on pages that never rendered count badges
    NavLabel      string // only the dashboard sets one
    TenantCount   int
    IncomeCount   int
    ExpenseCount  int
}

func newWorkspacePageTemplate(name string, functs template.FuncMap, body string) *template.Template
```

---

## 3. Contracts

### 3.1 One breakpoint

All new narrow-screen rules use **`@media (max-width: 640px)`**. There is an
older `@media (max-width: 980px)` that only collapses `.app` to one column; do
not add to it and do not retarget it. A page may own a *page-local* 640px block
(it must, for anything more specific than the shared sheet — see §3.3), and
`transaction_previews.go` has one standalone 760px block that predates this
convention.

Rule of thumb: **a narrow-screen rule that could move the desktop rendering is a
bug**, and the tests enforce that by splitting `workspacePageCSS` at the first
`@media (max-width: 640px)` and asserting the head contains no `sticky`,
`#nav-drawer`, `translateX` or `nav-burger`.

### 3.2 Two ways to be narrow-screen-only

- A rule in the shared block, or in a page's own 640px block, affects nothing
  above 640px. **Prefer this.**
- A rule that must sit outside a media query (e.g. a wide-screen default for a
  mobile-only element) has to be actively undone above the breakpoint. The
  billing amount is the worked example:

```css
/* page-local, always on */
.txn-amount-mobile { display: none; }
/* inside @media (min-width: 641px) */
.txn-col-action > .txn-action > summary { display: none; }
```

### 3.3 Source order decides, and the shared sheet is first

`html/template` renders `<style>{{workspacePageCSS}} ...page rules...</style>`.
The shared sheet therefore **loses every specificity tie** to a page-local rule.

This is why a global `input, select, textarea, .btn { min-height: 44px; }` does
not fix the ~30px billing classification controls: `.list-filter input` (0,1,1)
beats `input` (0,0,1). Those pages carry their own narrow-screen override, placed
*after* their own 40px/30px rule.

**Corollary**: when you add a page-local override, put it after the rule it
overrides, in the same block. When you add a shared override, it will only reach
elements no page-local rule has claimed.

### 3.4 Touch targets: 44px floor

The baseline control height is 40px. Narrow screens raise the interactive
elements to 44px:

- `input, select, textarea, .btn`
- `th .sort-link` (the `th` padding is not clickable; the `<a>` must be)
- `.nav a` (the drawer makes them the only navigation)
- `.tenant-link, .status.status-link` (44px tall)

Two techniques, and the choice between them matters:

- **Grow the box.** `min-height: 44px` on the element itself.
- **Grow the hit area only**, with a positioned `::after`. A bare two-character
  link is 26px wide; widening it with `padding` would also stretch its own
  dashed `text-decoration`, which then appears to underline empty space.

**When you grow a box, re-check every decoration that was pinned to the old box
edge.** A `border-bottom` on a link that just grew from 16px to 44px lands 22px
below the glyphs and reads as a stray divider, not an underline. Use the text's
own `text-decoration` (with `text-underline-offset`) for anything that must hug
the text inside a taller box.

### 3.5 Any focusable control must stay focusable

`hidden` is `display: none` and cannot take focus. The drawer's state checkbox is
therefore a **sr-only** input — `display: block; position: absolute; width: 1px;
height: 1px; opacity: 0; pointer-events: none` — re-shown only at ≤640px, with a
`:focus-visible` ring drawn on `.nav-compact-bar`.

Same trap for elements that are merely moved: `transform: translateX(-100%)`
moves the *visible position*, not the tab order. The closed sidebar also needs
`visibility: hidden`, restored to `visible` when checked. `visibility`
transitions discretely — to `visible` immediately, to `hidden` only after the
transition ends — which is exactly the behaviour a drawer wants.

### 3.6 Frozen last column is mobile-only

The action column of every list is the last column and sits outside the default
viewport. Freezing it keeps a row's identity and its actions on screen while the
table scrolls sideways:

```css
.table-wrap > table > thead > tr > th:last-child,
.table-wrap > table > tbody > tr > td:last-child:not([colspan]) {
  position: sticky; right: 0;
  background: var(--surface);
  box-shadow: -8px 0 8px -8px rgba(0, 0, 0, 0.18);
}
```

Both extras are load-bearing: without the opaque background the scrolled cells
show through, and mobile browsers draw no scrollbar, so the inset shadow is the
only hint that more columns exist. `:not([colspan])` keeps full-width rows from
being clipped.

**A frozen column has to be narrow.** `/billing` proved this: a frozen amount
column (136–146px) plus the 88px action column took 72% of a 327px window, and
once sticky pushed it left it covered the payer name — the one thing that says
which row is being acted on. The fix was to stop freezing the amount and move it
*into* the frozen action cell.

### 3.7 Collapse a wide cell with `<details>`, shipping it open

The billing action column collapses on narrow screens via
`<details class="txn-action" open>`. The markup ships `open` because desktop must
render it expanded with no summary visible (`.txn-col-action > .txn-action >
summary { display: none; }`), and a closed `<details>` hides its content through
the UA stylesheet. About three lines of script strip the attribute where
`window.matchMedia('(max-width: 640px)')` matches.

Both `<style>` and `<script>` are affected by `html/template`'s escapers: CSS
`/* */` comments are **stripped** from `<style>`, so a Go test cannot anchor on
one. **Anchor assertions on a declaration, never a comment.**

### 3.8 Cardize simple mobile tables

Object and low-frequency list pages (properties, rooms, tenancies, cash receipts,
and similar simple tables) use the shared mobile card rule instead of requiring a
horizontal scan. At `max-width: 640px`, a direct child table of `.table-wrap`
becomes a block with a hidden header, a grid body, and one bordered row card per
record. The final cell is reset from the generic sticky rule so links and buttons
remain in the card flow. Billing, tenant, expense, dashboard, and nested history
tables opt out with page-specific wrapper classes because they have richer card or
details behavior.

The card rule must remain mobile-only and must not change the table's desktop
markup. Add a rendered-markup test for every page-specific card wrapper and keep a
CSS test asserting the generic selector is inside the mobile tail.

### 3.9 Bottom sheets and the fixed navigation

Long-running or high-risk dashboard tasks such as dunning may use a page-local
bottom sheet at `max-width: 640px`. The sheet is fixed with 8px side insets,
`bottom: calc(76px + env(safe-area-inset-bottom))`, a viewport-relative max-height,
and `overflow: auto`; this leaves the 68px shared navigation plus an 8px gap
visible. The existing server-side form and confirmation handler remain unchanged.

---

## 3.10 Property and room list views keep responsive markup and route state together

The property and room list pages are embedded documents in
`web/templates/pages/properties.html` and `rooms.html`; page styles live in
`web/static/css/pages/object-lists.css`. They render two presentations from the
same Go view model: `.object-table-wrap` on desktop and `.object-mobile-list`
cards on narrow screens. At `max-width: 640px`, hide the table wrapper and show
the cards. Above the breakpoint, do the reverse. Keep the same entity IDs,
values, and actions in both presentations. Both trees exist in the HTML, so
browser checks must query visible elements or deliberately scope to the desktop
`tr` / mobile card; an unscoped row count double-counts each entity.

List navigation state is part of the server-rendered route contract. Detail and
edit links must carry the selected period and enough source-list filters to
restore the list after a save or return action. Build mutation redirects with
`net/url.Values`, and carry the same values in hidden POST fields where the
mutation handler needs them. Let `html/template` escape query values in links;
do not concatenate user search text into a redirect URL.

### 1. Scope / Trigger

- Trigger: adding a property/room list filter, detail link, edit action, or
  responsive list presentation.
- Why: the template contains both desktop and mobile entities, and a form
  mutation must return to the exact period/filter the user came from.

### 2. Signatures

- GET `/properties?period=YYYY-MM&status=active|inactive|all&search=text`
- GET `/rooms?period=YYYY-MM&property_id=id&status=active|inactive|all&search=text`
- Room detail source context: `from=rooms`, `return_property_id`,
  `return_status`, and `return_search`.
- Property detail source context: `list_status` and `list_search`.
- Go helpers: `filterPropertyPageRows`, `filterRoomPageRows`,
  `redirectPropertyList`, `redirectRoomList`, `redirectPropertyMutation`, and
  `copyRoomReturnContext`.

### 3. Contracts

- Property search matches property name and address. Its status defaults to
  `active`; its month controls the financial columns.
- Room search matches room name, property name, and current tenant name. Its
  status defaults to `all`; `property_id` narrows the parent property, and the
  month controls monthly receipts.
- Create forms carry `period` plus the originating filters as hidden fields.
  Room creation also carries the selected destination `property_id` separately
  from `filter_property_id`.
- Property detail links carry `period`, `list_status`, and `list_search`.
  Property edit POST carries those values so a successful save returns to its
  detail state with the same list source.
- Room detail links carry `period`, `from=rooms`, and the `return_*` fields.
  Room edit POST carries the same context; save returns to detail, whose return
  link restores the source room list.
- List mutation redirects use `url.Values.Encode()` to preserve search strings
  containing spaces, ampersands, or other reserved characters.

### 4. Validation & Error Matrix

| Input | Result |
|---|---|
| Invalid GET period | HTTP 400; do not silently change the requested month |
| Property status outside `active`, `inactive`, `all` | HTTP 400 |
| Room status outside `active`, `inactive`, `all` | HTTP 400 |
| Invalid room `property_id` query | HTTP 400 |
| Invalid mutation return period | Mutation redirect uses the current-month fallback |
| Invalid optional `return_property_id` | Omit the parent filter from the return URL |
| Search is empty | Omit it from redirect query values; render rows allowed by other filters |

### 5. Good / Base / Bad Cases

- Good: a room searched by tenant opens its detail; edit/save returns to that
  detail, and the return link restores the tenant search, selected property,
  status, and period.
- Base: a property list with default active status and no search returns to
  `/properties?period=...&status=active` after a create or deactivate action.
- Bad: rendering both tables and cards and counting `[data-page]` globally in a
  browser script counts each record twice. Scope checks to a visible table row
  or mobile card.
- Bad: dropping hidden `return_*` fields on edit POST makes the detail work but
  loses the list state after the user goes back.

### 6. Tests Required

- `TestObjectListFiltersSearchAndStatus`: case-insensitive name/address/tenant
  matching and active/inactive status filtering.
- `TestObjectListMutationRedirectsPreservePeriodAndFilters`: query state
  survives property and room server redirects.
- `TestEmbeddedObjectListsKeepFiltersAndResponsiveDetailLinks`: filter
  controls, entity IDs, desktop/mobile markup, and detail/edit links render.
- `TestEmbeddedRoomDetailUsesTypedDisplayFields`: return link and hidden edit
  context fields render when the source is the room list.
- Browser verification at 1440×900, 1366×768, and 390×844: filter values,
  detail return, edit/save return, empty results, no page-level overflow, and
  only the active responsive presentation is interactable.

### 7. Wrong vs Correct

#### Wrong — lose context or concatenate a search value

```go
http.Redirect(w, r, "/rooms?period="+period+"&search="+search, http.StatusFound)
```

#### Correct — carry list state and encode it as query values

```go
query := url.Values{}
query.Set("period", validatedPeriodValue(form.Get("period")))
status := strings.TrimSpace(form.Get("filter_status"))
if status != "active" && status != "inactive" {
    status = "all"
}
query.Set("status", status)
if search := strings.TrimSpace(form.Get("search")); search != "" {
    query.Set("search", search)
}
http.Redirect(w, r, "/rooms?"+query.Encode(), http.StatusFound)
```

---

## 4. Validation & Error Matrix

| Condition | Result |
|---|---|
| Narrow rule written outside a media query, not undone above 640px | Desktop rendering moves — caught by `TestFrozenLastColumnIsMobileOnly` / `TestWorkspaceCSSDrawerIsCSSOnlyAndMobileScoped` |
| `sticky` / `#nav-drawer` / `translateX` / `nav-burger` in the head of `workspacePageCSS` | Test failure |
| Drawer checkbox carries `hidden` | Unfocusable nav at ≤640px; `TestWorkspaceCSSDrawerIsCSSOnlyAndMobileScoped` fails, and `verify-check-fixes.mjs` fails on real focus |
| sr-only rule omits `display: block` | Base `display: none` still wins; checkbox stays unfocusable — the CSS reads fine, only measurement catches it |
| Closed sidebar only `translateX`'d | Links stay in the tab order, focus ring lands off-screen |
| Page renders `workspace-nav` twice, or not at all | `TestEveryWorkspacePageRendersTheSharedChromeOnce` fails (one each of `id="nav-drawer"`, `<aside class="sidebar"`, `class="nav-compact-bar"`, `class="nav-scrim"`) |
| `ShowNavCounts` left at its zero value on a page that used to show badges | That page loses 4 badges; `TestNavCountsOnlyRenderWhereTheyDidBefore` fails |
| `ShowNavCounts: true` on tenant-detail / cash-receipt / cash-receipt-void | Those pages gain badges they never had, changing their desktop appearance |
| A narrow-screen block written before the wide-screen block | Wide rules apply at 1280px; `TestBillingNarrowScreenBlockFollowsTheWideScreenOne` fails |

---

## 5. Good / Base / Bad Cases

**Good** — `/expenses` date column. The column measured 103px, leaving 75px of
content box for `10 Sep 2026`, which needs ~78px, so it wrapped to two lines.
Fixed by borrowing 22px from the note column, which had slack:

```css
.table-wrap > table > thead > tr > th:nth-child(4),
.table-wrap > table > tbody > tr > td:nth-child(4) { min-width: 125px; }
```

Total table width and horizontal scroll range are unchanged — auto table layout
takes the space from a column that can spare it. The rule of thumb this encodes:
**fixed-format values (amounts, dates, categories) must not wrap; free text
(descriptions, notes) may.** Wrapping a date is a readability break, wrapping a
description is normal.

**Base** — the shared 44px floor. Correct and sufficient for elements no
page-local rule has claimed.

**Bad** — a `min-width` added where the real problem is that the column does not
need to be a column. `/billing`'s secondary columns are dropped outright rather
than widened; `/tenant-detail`'s 130px label grid becomes one column rather than
a narrower two.

---

## 6. Tests Required

`cmd/truelayer-demo/mobile_layout_test.go` — responsive tests, all parsing rendered markup
or the CSS constants:

| Test | Asserts |
|---|---|
| `TestEveryWorkspacePageRendersTheSharedChromeOnce` | All 7 pages render exactly one of each chrome marker |
| `TestWorkspaceCSSDrawerIsCSSOnlyAndMobileScoped` | Drawer rules are in the mobile tail only; `visibility: hidden` present; `.nav-drawer-input` styled at ≤640 and suppressed above; `workspaceNav` has no `hidden` |
| `TestFrozenLastColumnIsMobileOnly` | `sticky` never in the head; the last-child rules, opaque background and inset shadow all present |
| `TestTenantDetailProfileListStacksOnlyOnNarrowScreens` | The 130px wide rule is intact *and* the 1fr rule is in the last 640px block |
| `TestNavCountsOnlyRenderWhereTheyDidBefore` | 4 badges on four pages, 0 on three |
| `TestPayerPreviewScrollsTheTableNotTheCard` | `.tp-scroll{overflow-x:auto}` present; `.card{...overflow-x:auto}` gone; card rule unchanged in shape; wrapper opens and closes around the table |
| `TestBillingActionCellCollapsesOnlyOnNarrowScreens` | `<details class="txn-action" open>`, summary hidden wide, `matchMedia` script strips `open`, dropped columns, 680px floor, mobile amount default and override order |
| `TestBillingNarrowScreenBlockFollowsTheWideScreenOne` | Narrow block is written after the wide one |
| `TestMobileDashboardUsesCardsForRentRows`, `TestMobileTenantListUsesCards`, `TestMobileExpenseListUsesCards`, `TestMobileBillingRowsStackAsCards` | High-frequency pages expose mobile card markup while retaining desktop wrappers |
| `TestMobileSimpleTablesStackAsCards` | Generic object-table card rules stay in the mobile stylesheet tail |

**Go tests are necessary and not sufficient here.** They parse strings; they do
not lay anything out. Three defects in this task's own work were invisible to
them and were only found by rendering in a real browser:

1. The 44px `.tenant-link` box pushed its `border-bottom` 22px below the text.
2. The drawer checkbox's sr-only rule omitted `display: block`, so the base
   `display: none` still won and the nav was pointer-only.
3. The zero-font-size/collapsed `<details>` subtree is not hit-testable, so a
   label inside it does not toggle on click.

The harness that catches that class of defect lives in
`.trellis/tasks/09-16-mobile-responsive-audit/research/harness/` (Playwright,
29 scripts). Reach for it when a change is about geometry, hit areas, or focus —
not when it is about which declarations exist.

---

## 7. Wrong vs Correct

#### Wrong — growing a box and leaving its decoration behind

```css
/* ≤640 */
.tenant-link { display: inline-flex; align-items: center; min-height: 44px; }
/* base sheet already has: .tenant-link { border-bottom: 1px dashed ...; } */
```

The dashed border is pinned to the bottom of a 44px box, so it draws ~22px under
the two glyphs. It reads as a separator, not an underline.

#### Correct — put the line back on the text

```css
/* ≤640, written after the shared block so equal specificity resolves to this */
.tenant-link {
  border-bottom: 0;
  text-decoration: underline dashed var(--border-strong);
  text-underline-offset: 3px;
}
.tenant-link:hover { text-decoration-color: currentColor; }
```

The link box stays 26×44 — the hit area is untouched; only the line moves.

#### Wrong — hiding a control with `hidden`

```go
const workspaceNav = `...<input type="checkbox" id="nav-drawer" hidden>...`
```

`hidden` is `display: none`. The drawer can then only be opened by pointer, and
at ≤640px the navigation becomes unreachable by keyboard — worse than the plain
sidebar links the drawer replaced.

#### Correct — hide it visually, keep it focusable

```go
const workspaceNav = `...<input type="checkbox" id="nav-drawer" class="nav-drawer-input">...`
```

```css
/* base */
.nav-drawer-input { display: none; }
/* ≤640 — display:block is the part that is easy to forget */
.nav-drawer-input {
  display: block; position: absolute;
  width: 1px; height: 1px; opacity: 0; pointer-events: none;
}
.nav-drawer-input:focus-visible ~ .nav-compact-bar {
  outline: 2px solid var(--accent-bright); outline-offset: 2px;
}
```

## 8. Desktop Figma Shell

The desktop baseline is a 236px dark sidebar plus a 64px topbar. Keep the live
palette in the shared stylesheet's OKLCH tokens (`--sidebar`, `--accent`,
`--surface`, `--foreground`) and let page templates reuse `.panel`, `.metric`,
`.table-wrap`, `.status`, `.entity-form`, `.form-grid`, and `.drawer-actions`.
Desktop action forms should remain ordinary semantic HTML forms; the mobile task
may restyle them as sheets without changing their field names or action URLs.

Asset creation and tenant binding are part of the shared contract: a room form
must carry `property_id`, and a tenant form may carry `room_id`, `structured`,
and `arrangement_start_month`. Selecting a room may prefill its current rent,
but the amount remains editable.

High-risk action buttons expose `data-confirm="true"` so the shared delegated
submit guard can confirm keyboard and pointer submissions. Dunning opens its
drawer with focus moved into the first control, closes on Escape, and restores
focus to the launch button.
