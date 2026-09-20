# Responsive Conventions

> Executable contracts for the responsive rendering of the workspace pages.
> Every rule here is asserted by `cmd/truelayer-demo/mobile_layout_test.go` and
> `cmd/truelayer-demo/desktop_layout_test.go`.

---

## 1. Scope / Trigger

Read this before any of:

- adding or editing a rule inside a page's `<style>` block
- touching `workspacePageCSS` (the shared stylesheet)
- adding a page, or a column to an existing table
- changing `workspaceNav` or `workspaceShell`

The desktop rendering is **not frozen**. It may be aligned to
`figma/rentops-desktop-suite.html`, but a desktop change is legitimate only when
all three of these hold:

1. **It names the prototype difference it implements.** A comment has to say
   which declaration in the export it is porting — "the prototype's
   `@media(max-width:1100px)` collapses its wide two-column grids to one
   column", not "looks better at 1024". Without that, nobody can review it.
2. **It updates the assertions it invalidates, in the same change.** Several
   tests split `workspacePageCSS` at the first narrow-screen block and assert on
   the head. Those assertions were written while the head was frozen, so a
   desktop change can trip one. Retarget or re-justify the assertion then and
   there; never delete it, and never leave it red for a later task.
3. **It lives in a media query, not in the base rules.** A desktop-tier
   declaration belongs in `@media (max-width: 1100px)` (see §3.1). The base
   rules stay the ≥1101px rendering.

What is still absolute is the **≤640px mobile contract**. The breakpoint, the
drawer, the frozen last column, the bottom navigation, the 44px floor, and the
card layouts must keep behaving exactly as they do today: none of the three
conditions above weakens them, and §3.1 fixes their order in the file.

---

## 2. Signatures

```go
// The shared stylesheet: one file, consumed two ways.
//
//	var workspacePageCSS = embeddedWebText("web/static/css/workspace.css")
//
// Legacy inline templates concatenate it into <style> (`"<style>"+workspacePageCSS+...`);
// embedded templates (/bills, /dunning, rent-workspace) load the same file with
// <link rel="stylesheet" href="/static/css/workspace.css">. Either way it lands
// BEFORE the page's own sheet (pages/*.css), which is the order §3.3 depends on.
var workspacePageCSS = `...`

// The shared chrome. Defined once; every page template is cloned from
// workspaceBase and calls {{template "workspace-nav" .}}.
const workspaceNav = `{{define "workspace-nav"}}...{{end}}`

type workspaceShell struct {
    ActivePage    string // canonical page key: "rent-dashboard" | "bills" | "transactions" | "dunning" | "properties" | "rooms" | "tenants" | "tenancies" | "cash-receipts" | "expenses" | "bank" | "more"; legacy "billing" remains supported
    Username      string
    Environment   string
    FootNote      string
    CompactTitle  string // mobile-only page/object context beside "RentOps"
    ShowNavCounts bool   // false on pages that never rendered count badges
    TenantCount   int
    IncomeCount   int
    ExpenseCount  int

    // Filled by (*app).fillWorkspaceShell. Empty status fields render no card.
    StatusTitle        string // sidebar data-status card; no card when empty
    StatusUpdatedAt    string
}

// fillWorkspaceShell completes the optional real-data status card. It is called
// from canonicalPageShell and every inline workspaceShell literal.
func (a *app) fillWorkspaceShell(r *http.Request, shell workspaceShell) workspaceShell

func newWorkspacePageTemplate(name string, functs template.FuncMap, body string) *template.Template
```

`NavLabel` was removed: the sidebar's single 「工作台」 label became the
prototype's three group headings (收租决策 / 资产与关系 / 资金与系统), so no page
sets a label any more.

---

## 3. Contracts

### 3.1 Three tiers, written in this order

| Tier | Applies | Owns |
|---|---|---|
| base rules | ≥1101px | the wide desktop layout |
| `@media (max-width: 1100px)` | ≤1100 | the prototype's compact desktop tier — 641–1100 as a desktop tier, and it still matches below 981 alongside the 980/981 navigation switch |
| `@media (max-width: 640px)` | ≤640 | the frozen mobile contract |

Range nesting comes first and the narrow tier last, because a later rule of
equal specificity wins: an 1100 rule written after the base rule overrides it,
and a 640 rule written after the 1100 rule overrides that. Written the wrong way
round the 640 rules would apply at 1440px.

A page may own a *page-local* 640px block (it must, for anything more specific
than the shared sheet — see §3.3), and a page-local 1100px block follows the
same rule: place it after the rule it overrides, in that page's own sheet.
`transaction_previews.go` has one standalone 760px block that predates this
convention.

**The 980/981 pair is kept, and it is not the 1100 tier.** It is the
*navigation-layout* threshold:

- `@media (max-width: 980px)` collapses `.app` to one column and stacks the
  sidebar above the content with rounded corners. It no longer touches
  `.grid-two`; that rule moved up into the 1100 tier.
- `@media (min-width: 981px)` makes the sidebar the fixed 236px rail and pins
  `.workspace-page-topbar` to it with `.content { padding-top: 88px; }` so the
  fixed breadcrumb cannot cover the content.

It survives for two reasons. Merging it into the 1100 tier would stack the
sidebar at 981–1100, changing navigation the prototype keeps as a rail down to
760px; deleting it would strand the fixed breadcrumb, which has no other switch.
Its only job is navigation layout, so **all grid and typography compaction
belongs in the 1100 tier**, not here. (`.grid-two` itself was moved out of the
980 block into the 1100 tier for exactly that reason; ≤980 renders identically
because the 1100 tier also matches there.)

The prototype has only two thresholds, 1100 and 760, so the page-local
thresholds that predate this convention are inventoried here and their
disposition is fixed:

| Page sheet | Threshold | Carries | Disposition |
|---|---|---|---|
| `collection-pages.css` | 900 | `.dunning-layout` two-column → one | kept at 900; it is a page-local layout choice the prototype makes at 760 |
| `collection-pages.css` | 900 → **1100** | `.collection-summary` four columns → two | **moved to 1100**: the prototype does it in its own `@media(max-width:1100px)` |
| `rent-workspace.css` | 981 | the desktop action-queue's **single link-styled button** | kept, but **narrowed**: the 3-column grid and the `:not(:last-child)` border moved down into the base rules (the prototype lays the queue out the same way at every width it draws), so the 981 block now owns only the desktop button treatment. It never owned the nav rail switch — the shared 981 in `workspace.css` does. Recorded 2026-09-20 after `09-19-dashboard-alignment` deleted the whole block and regressed both the button count and the border rule |
| `object-lists.css` | 980/981 | table floors / a heading nudge | kept; it is the page-local copy of the nav threshold |
| `property-detail.css`, `room-detail.css` | 980 / 900 | two-column detail grids → one | candidates for 1100 in the page-alignment subtasks; not moved here |

**What the shared tier carries today.** Exactly one declaration — `.grid-two`'s
collapse. `.grid-two` is the shared sheet's two-column primitive, and its only
renderer is `legacyExpenseTemplate`, a template nothing references. Measured at
1024 / 1100 / 1101 / 1366 / 1440 / 1920, the 1100 tier's one rendered effect on
the eleven audited pages is the *page-local* `.collection-summary` collapse on
`/dunning` (2 columns at ≤1100, 4 at ≥1101). The remaining prototype 1100
collapses are the deferred work of the page-alignment subtasks (see the
disposition table above). **"The tier exists" is not "the tier is populated"**:
adding a page's own 1100 collapse is that page's change, not this tier's.

A page-local grid that the prototype collapses at 1100 moves to 1100 by
rewriting its existing threshold, not by stacking a second block on top of it —
two blocks for one selector is how the 900/1100 pair becomes unreadable.

Rule of thumb: **a mobile-only *mechanism* has to stay below the 640 tier**, and
the tests enforce that by splitting `workspacePageCSS` at the first
`@media (max-width: 640px)` and asserting the head — which now legitimately
contains the desktop shell and the 1100 tier — holds no `sticky`, `#nav-drawer`,
`translateX` or `nav-burger`. The head is no longer immutable; those four are
mobile mechanisms, not desktop rules.

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

This is the trap under the 1100 tier. A shared `@media (max-width: 1100px)`
block reaches the selectors the shared sheet owns (`.grid-two`, `.summary`,
`.content`, the shell) and **nothing else**. A page-local selector such as
`.collection-summary`, `.detail-summary` or `.room-allocation-metrics` is
declared in `pages/*.css`, which is concatenated *after* `workspacePageCSS`, so
a same-specificity 1100 rule in the shared sheet loses and silently does
nothing. Those pages need their own 1100 block, appended after the base rule in
their own sheet:

```css
/* pages/collection-pages.css, after .collection-summary's base rule */
@media (max-width: 1100px) {
  .collection-summary { grid-template-columns: repeat(2, minmax(0, 1fr)); }
}
```

**"The declaration exists" is not "the declaration applies."** Verify a new
1100 rule by measuring the rendered geometry at 1024 and at 1366 — the computed
value has to differ — not by grepping the stylesheet.

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
| A mobile-only mechanism (`sticky`, `#nav-drawer`, `translateX`, `nav-burger`) written above the 640 tier | The mechanism leaks to every width above 640 — caught by `TestFrozenLastColumnIsMobileOnly` / `TestWorkspaceCSSDrawerIsCSSOnlyAndMobileScoped` |
| A page-local selector given an 1100 rule in the shared sheet only | The rule exists but does not apply — the page-local sheet is concatenated last and wins the tie (§3.3). Only measurement catches it |
| The 640 tier written before the 1100 tier | At equal specificity the later rule wins, so the 1100 rules apply at ≤640 and the mobile contract is deleted — caught by the tier-order test |
| A desktop change landed without its prototype justification comment | No test can catch it; review has to ask which prototype declaration it ports (§1) |
| A desktop change landed without updating the assertions it trips | The suite goes red and stays red; that is the signal, not an obstacle to work around (§1) |
| Drawer checkbox carries `hidden` | Unfocusable nav at ≤640px; `TestWorkspaceCSSDrawerIsCSSOnlyAndMobileScoped` fails, and `verify-check-fixes.mjs` fails on real focus |
| sr-only rule omits `display: block` | Base `display: none` still wins; checkbox stays unfocusable — the CSS reads fine, only measurement catches it |
| Closed sidebar only `translateX`'d | Links stay in the tab order, focus ring lands off-screen |
| Page renders `workspace-nav` twice, or not at all | `TestEveryWorkspacePageRendersTheSharedChromeOnce` fails (one each of `id="nav-drawer"`, `<aside class="sidebar"`, `class="nav-compact-bar"`, `class="nav-scrim"`) |
| `ShowNavCounts` left at its zero value on a page that used to show badges | That page loses 4 badges; `TestNavCountsOnlyRenderWhereTheyDidBefore` fails |
| `ShowNavCounts: true` on tenant-detail / cash-receipt / cash-receipt-void | Those pages gain badges they never had, a template change a page never asked for |
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
| `TestWorkspaceCSSOrdersThe1100TierBeforeThe640Tier` | The 1100 tier exists, is written before the 640 tier, and carries a prototype-backed compaction |
| `TestCollectionSummaryCollapsesInThe1100Tier` | A page sheet carries its own 1100 block, written before its 640 block, and collapses `.collection-summary` in exactly one place |
| `TestEveryWorkspacePageRendersTheSharedChromeOnce` | All 9 pages render exactly one of each chrome marker |
| `TestWorkspaceCSSDrawerIsCSSOnlyAndMobileScoped` | Drawer rules are in the mobile tail only; `visibility: hidden` present; `.nav-drawer-input` styled at ≤640 and suppressed above; `workspaceNav` has no `hidden` |
| `TestFrozenLastColumnIsMobileOnly` | `sticky` never above the 640 tier; the last-child rules, opaque background and inset shadow all present |
| `TestTenantDetailProfileListStacksOnlyOnNarrowScreens` | The 130px wide rule is intact *and* the 1fr rule is in the last 640px block |
| `TestNavCountsOnlyRenderWhereTheyDidBefore` | 4 badges on four pages, 0 on three |
| `TestPayerPreviewScrollsTheTableNotTheCard` | `.tp-scroll{overflow-x:auto}` present; `.card{...overflow-x:auto}` gone; card rule unchanged in shape; wrapper opens and closes around the table |
| `TestBillingActionCellCollapsesOnlyOnNarrowScreens` | `<details class="txn-action" open>`, summary hidden wide, `matchMedia` script strips `open`, dropped columns, 680px floor, mobile amount default and override order |
| `TestBillingNarrowScreenBlockFollowsTheWideScreenOne` | Narrow block is written after the wide one |
| `TestMobileBillsListUsesCardsForRentRows`, `TestMobileTenantListUsesCards`, `TestMobileExpenseListUsesCards`, `TestMobileBillingRowsStackAsCards` | High-frequency pages expose mobile card markup while retaining desktop wrappers. (The first was renamed from `TestMobileDashboardUsesCardsForRentRows` when the legacy /rent-dashboard fallback template was removed; the contract now anchors on `/bills`, its live carrier.) |
| `TestMobileSimpleTablesStackAsCards` | Generic object-table card rules stay in the mobile stylesheet tail |

The desktop side of the contract is checked in the browser, not in Go:
`scripts/audit/desktop-widths.mjs` walks all 11 sidebar destinations at
1024 / 1366 / 1440 / 1920 and fails on any document-level horizontal overflow
(a wide table scrolling *inside* `.table-wrap` is expected and not a failure).
It is the acceptance probe for the 1100 tier; run it against an instance from
`scripts/run-audit-local.sh`, never against `:8081` / `bank.ddpl.top`.

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

### 8.1 The rail is sticky; the shell invents no copy

The desktop sidebar is `position: sticky; top: 0; height: 100vh` inside the
`@media (min-width: 641px)` tier, matching the prototype
(`figma/rentops-desktop-suite.html:20`). Do not write `relative` there: the rail
then scrolls away with the document, and `top: 0` on a relative box offsets
nothing. `body { overflow-x: hidden }` is not an obstacle — that overflow
propagates to the viewport, so `body` never becomes the scroll container and
sticky keeps working. `TestFrozenLastColumnIsMobileOnly` allows exactly this one
sticky rule above the 640 tier and rejects a sticky `right: 0` table cell.

The shell's optional status card is real data or nothing: no card when bank
authorization could not be read. The shared top-right search and pending-review
count/action were removed by the 2026-09 workspace remediation; search remains
page-local, and the dashboard's manual-review queue is its own page content. Do
not add shell widgets back for those functions. Do not paste the prototype's
demo copy (`测试数据已载入` / `Rosewood 收租明细` / `更新于 …`) or its
`reload the sample data` CTA into the shell: `figma/DESIGN-HANDOFF.md:9,37`
reserves those for the prototype.

Post-redirect operation feedback is rendered twice from one source. A success
flash (`notice ok`) renders its notice exactly as before, marked `data-toast`; the
shared stylesheet hides `.js .notice[data-toast]` and the shared script copies the
text into `#workspace-toast`, dismissing it after 2.2s. Without scripting the root
never gets the `js` class, so the notice stays visible: the notice is the fallback,
the toast is the enhancement. If a page ever renders two marked notices the script
adopts only the first and strips the marker from the rest, so no message can be
hidden without being shown.

The marker is never put on an error banner. It is an opt-in on the block, not a
positional guess, and it is success-only: an error usually carries text the user
has to read and act on, and 2.2s is not enough for that. The prototype agrees —
its toast is a save confirmation (`showToast("操作已完成")`,
`figma/rentops-desktop-suite.html:286`). The block that sits in a form or a drawer
(e.g. the inline error inside the cash-receipt sheet) is not marked either, so the
same failure is never announced twice. `TestOnlySuccessFlashesAreMarkedForTheToast`
scans every template and Go template literal, so a page added later cannot quietly
mark an error.

A marked notice renders **fixed text chosen by the template**, never a value taken
from the request. The `Message` query parameter is a *code*, matched with
`{{if eq .Message "some_code"}}`, and the sentence lives in the template; the
`{{if .Message}}{{.Message}}{{end}}` form prints whatever the URL said, so a crafted
link shows arbitrary text in a green success toast inside the trusted UI
(`html/template` escapes it, so this is content injection, not XSS, but the
affordance is a system-generated confirmation the user did not cause). Only the
code form may carry `data-toast`. Recorded 2026-09-20; four pages used the raw form
and all four are fixed in `09-20-mobile-filter-and-duplicate-errors`.

**The contract is about what gets rendered, not how the template is spelled.** The
fourth page (`/bills`) passed every source scan: its guard read
`{{if .Message}}` and its text came from `{{billsNotice .Message}}`, a mapping func
whose `default:` fell through to `return code`. A scan for the literal `{{.Message}}`
cannot see a value that reaches the page through a func, so source-shape scanning
has a blind spot exactly where the indirection is. Two consequences:

- When the interpolated value and the fixed sentence are chosen by different
  expressions (a guard plus a func, a `default:` arm), assert on the **rendered
  page** — `TestBillsMessageRendersOnlyWhitelistedCodes` checks that
  `/bills?message=<unknown>` renders no `.notice.ok` at all — and keep a literal
  scan only as a backstop for the direct spelling.
- The **message** leg and the **error** leg may legitimately differ, so do not
  "unify" them. `/bills`'s `billsMessageText` now returns `""` for an unknown code
  while `billsErrorText` still returns the code verbatim: that raw fall-through is
  a pinned contract (`list_pages_alignment_test.go:58`) and a red banner is not the
  affordance a crafted link abuses. Grep will not tell you which leg you are on —
  read which notice class the value lands in before tightening the guard.

### 8.2 The 980 tier must undo the rail's sticky explicitly

`@media (min-width: 641px)` gives `.sidebar` `position: sticky; top: 0;
height: 100vh`. That tier **also matches from 641 to 980**, where
`@media (max-width: 980px)` collapses `.app` to one column — so the rail becomes a
100vh sticky box in the normal flow, scrolls with the document, and, because
positioned elements paint above in-flow content, **covers the body**: at 800×700
scrolled to the bottom, `elementFromPoint` at the viewport centre hits a node
inside the rail and no heading is visible.

"Leave `position` unset in the 980 tier" does not mean "unset" — the 641 rule is
still in force there. The 980 block must write `position: static`. Source order
makes it win (`.sidebar` in both, equal specificity, the 980 block comes later),
and the `max-width: 640px` drawer that follows still turns the rail into a `fixed`
off-canvas panel. Recorded 2026-09-20 after `09-19-shell-alignment` shipped the
sticky without the reset.

### 8.3 The shared script runs before the page body it has to bind

`workspace-nav.html` renders inside `.app` but **before** `<main class="content">`,
so a `document.querySelector*` at the top level of its `<script>` executes while
the page body has not been parsed. Any lookup of a `<main>` element returns an
empty set and attaches no listener.

The failure is not a dead button, which is what makes it easy to under-report:
`.object-list-filter-fields` is `display: none` at `max-width: 640px` and only the
listener adds `.filters-open`, so the whole filter feature was **unreachable** on a
narrow screen; the fields' `change -> requestSubmit` binding was dead at every
width, so `/properties` and `/rooms` had no submit path at all on desktop. Both are
fixed in `09-20-mobile-filter-and-duplicate-errors`.

Contract: **every lookup of a page-body element goes inside `initChrome()`**, which
is registered on `DOMContentLoaded` (and called directly when
`document.readyState !== "loading"`, so a late-injected partial still works). That
includes the bindings that would work anyway, so the block keeps one rule instead
of a mix. A lookup of an element emitted by the *same partial, above the script* is
still fine at top level — `#workspace-toast` precedes the toast script — and
**that is the trap**: because that lookup succeeds, the block looks healthy while
page-body lookups silently bind nothing.

The same script owns the object-list filter's **single submit path**
(`.object-list-filter-fields select/input` -> `change` -> `requestSubmit()`). Pages
must not add their own `onchange="this.form.submit()"`: two sources means two
submits per change. When the shared binding was dead, `/properties` and `/rooms`
carried inline `onchange` as a workaround; both were removed once the shared
binding was restored.

`TestWorkspaceNavBodyLookupsWaitForDOMContentLoaded` renders the partial **on its
own** — the minimal page the failure needs — and asserts the object-list lookups
appear after `initChrome()`. It proves the shape, not the behaviour; a new lookup
must still be included in the review. Recorded 2026-09-20.

### 8.4 Shared workspace controls keep native form state

`newWorkspacePageTemplate` and `newEmbeddedWorkspacePageTemplate` pass page HTML
through `withWorkspaceControlAssets`. That helper injects the calendar and
workspace-control styles/scripts once, before `</head>`. Do not add duplicate
page-local includes when introducing a workspace date, month, select or search
control.

The native input/select remains the submitted form control. With JavaScript,
`calendar.js` enhances enabled, writable `date` and `month` inputs and watches for
controls inserted later in drawers. It preserves the form name/value in a hidden
input and dispatches `input` and `change` on selection. The shared select enhancer
leaves the native select as the value source, mirrors its options into a light
listbox positioned from the trigger, and dispatches both events after selection.
It observes option mutations so dependent selectors keep their labels and hidden
options in sync. Without JavaScript, native controls remain usable.

Search clear buttons are added to non-empty `input[type=search]` fields by the
same shared script. The button clears the native input, dispatches `input`, and
returns focus to the field; its target is 44px on narrow screens. Do not add an
auto-submit unless the page already submits in response to the input event.
