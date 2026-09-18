# Research: verification infrastructure for mobile workspace redesign

- **Query**: What spec, Go regression tests, browser-audit harness, run instructions and fixtures already exist that a "list tables -> mobile cards" change can reuse?
- **Scope**: internal (repo + archive); no external search needed.
- **Date**: 2026-09-17
- **Related task**: active `/Users/dd/projects/bank/.trellis/tasks/09-16-mobile-friendly-workspace`
- **Predecessor**: `.trellis/tasks/archive/2026-09/09-16-mobile-responsive-audit/`

All paths below are absolute. Nothing outside this `research/` dir was modified.

---

## 1. Spec documents

### Files

| File | Holds |
|---|---|
| `/Users/dd/projects/bank/.trellis/spec/frontend/index.md` | Frontend overview + guide index + "where the UI lives" table |
| `/Users/dd/projects/bank/.trellis/spec/frontend/responsive-conventions.md` | The only indexed guide (Status: Filled) — executable narrow-screen contracts |

`index.md` indexes exactly one guide (line 25). No other frontend spec files exist. The index's "Where the UI lives" table (lines 31-36) names `workspace_shell.go`, `main.go` (`workspacePageCSS`), one `<page>.go` per page, and `mobile_layout_test.go`.

### Binding conventions (responsive-conventions.md)

- **§3.1 One breakpoint.** All new narrow-screen rules use `@media (max-width: 640px)`. The older `@media (max-width: 980px)` "only collapses `.app` to one column; do not add to it and do not retarget it." `transaction_previews.go` owns a standalone 760px block that "predates this convention."
- **§3.2 Narrow rules must not move the desktop rendering.** A rule that must sit outside a media query must be actively undone above the breakpoint. Quote: *"a narrow-screen rule that could move the desktop rendering is a bug."*
- **§3.3 Source order decides.** `workspacePageCSS` is concatenated first, so "the shared sheet therefore loses every specificity tie to a page-local rule." Page-local overrides must be written after the rule they override.
- **§3.4 Touch targets: 44px floor.** Baseline control height 40px; narrow screens raise `input, select, textarea, .btn`, `th .sort-link`, `.nav a`, `.tenant-link`, `.status.status-link` to 44px.
- **§3.5 Focusable controls stay focusable.** No `hidden`; sr-only pattern; `translateX` needs `visibility: hidden` too.
- **§3.6 Frozen last column is mobile-only.** Table action column is `position: sticky; right: 0` with opaque `background: var(--surface)` and inset shadow.
- **§3.7 Collapse a wide cell with `<details>` shipped `open`.** Billing action cell pattern; ~3 lines of `matchMedia` script strip `open` at ≤640px.
- **CSS comments are stripped** by `html/template`; quote: *"Anchor assertions on a declaration, never a comment."*

### What a "replace table with cards on mobile" change CONFLICTS with

1. **§3.6 frozen last column** — a card layout has no table action column, so the frozen-column contract becomes inapplicable. The rule text itself is asserted by `TestFrozenLastColumnIsMobileOnly` (see §2), not by rendered page markup, so the CSS can stay; but the semantic contract ("freezing it keeps a row's identity and its actions on screen while the table scrolls sideways") no longer describes the mobile UI. Quote: *"The action column of every list is the last column and sits outside the default viewport. Freezing it keeps a row's identity and its actions on screen while the table scrolls sideways"* (lines 135-138).

2. **§3.6 "A frozen column has to be narrow."** The rule records that on `/billing` a frozen amount column "plus the 88px action column took 72% of a 327px window" and the fix was to move the amount *into* the frozen action cell. Cards dissolve this, but the historical constraint explains current billing CSS that tests assert (see §2 conflict #3).

3. **§3.1/§3.2 breakpoint + desktop-freeze.** Any card CSS must be inside a 640px block (or undone above it). The desktop rendering is a frozen contract: *"Anything you add has to land on the narrow side of the breakpoint, or be justified in a comment as a deliberate desktop change"* (lines 18-20). A card layout that also restructures the desktop table markup would violate this.

4. **§3.4 44px floor.** Cards must still keep interactive elements ≥44px; the spec's two techniques (grow box vs. grow hit area via `::after`) and the decoration-repinning warning (lines 114-118) still apply to card controls.

5. **§3.7 `<details>` collapse pattern.** If billing rows become cards, the `txn-amount-mobile` "carried amount inside the frozen cell" mechanism is replaced; `TestBillingActionCellCollapsesOnlyOnNarrowScreens` asserts that mechanism by exact string (see §2).

6. **Spec §6 references a harness path that no longer exists**: `.trellis/tasks/09-16-mobile-responsive-audit/research/harness/` (lines 244-246). The task was archived; the real path is `.trellis/tasks/archive/2026-09/09-16-mobile-responsive-audit/research/harness/`. Stale quote: *"The harness that catches that class of defect lives in `.trellis/tasks/09-16-mobile-responsive-audit/research/harness/` (Playwright, 29 scripts)."*

---

## 2. Go regression tests

All live in `/Users/dd/projects/bank/cmd/truelayer-demo/`. Run with `go test ./cmd/truelayer-demo/` from repo root (package `main`).

### Helper signatures available for new tests

| Helper | File:line | Signature / behaviour |
|---|---|---|
| `executeTemplate` | `mobile_layout_test.go:10` | `func executeTemplate(tmpl *template.Template, data any) (string, error)` — renders one template, returns markup |
| `splitWorkspaceCSS` | `mobile_layout_test.go:21` | `func splitWorkspaceCSS(t *testing.T) (base, mobile string)` — `strings.Cut(workspacePageCSS, "@media (max-width: 640px)")`; `t.Fatal` if absent |
| `renderBillingPage` | `billing_layout_test.go:9` | `func renderBillingPage(t *testing.T, data billingPageData) string` |
| `markupBetween` | `dashboard_layout_test.go:11` | `func markupBetween(t *testing.T, page, start, end string) string` — both markers required, else `t.Fatal` |
| `styleRuleFor` | `tenant_detail_layout_test.go:11` | `func styleRuleFor(t *testing.T, page, selector string) string` — declaration block of first `selector {` rule |

Template vars / data structs used with `executeTemplate`: `billingTemplate`/`billingPageData`, `rentDashboardTemplate`/`rentDashboardPageData`, `tenantTemplate`/`tenantPageData`, `expenseTemplate`/`expensePageData`, `tenantDetailTemplate`/`tenantDetailPageData`, `cashReceiptTemplate`/`cashReceiptFormData`, `cashReceiptVoidTemplate`/`cashReceiptVoidPageData`, `payerPreviewTemplate`/`payerPreviewPageData`.

### `mobile_layout_test.go` (8 tests)

| Test | Asserts |
|---|---|
| `TestEveryWorkspacePageRendersTheSharedChromeOnce` (35) | Each of 7 pages renders exactly one of `id="nav-drawer"`, `<aside class="sidebar"`, `class="nav-compact-bar"`, `class="nav-scrim"` |
| `TestWorkspaceCSSDrawerIsCSSOnlyAndMobileScoped` (81) | `#nav-drawer`/`translateX`/`nav-burger` absent from base; compact bar hidden above breakpoint; mobile tail has `transform: translateX(-100%)`, `:checked ~ .sidebar`, `visibility: hidden;`, `.nav-drawer-input {`; `workspaceNav` has no `hidden` |
| `TestFrozenLastColumnIsMobileOnly` (128) | `sticky` never in base; mobile tail contains exact last-child selectors, `position: sticky;`, `right: 0;`, `background: var(--surface);`, `box-shadow: -8px 0 8px -8px rgba(0, 0, 0, 0.18);` |
| `TestTenantDetailProfileListStacksOnlyOnNarrowScreens` (155) | Exact wide `.profile-list` rule intact; exact narrow single-column rule present inside the last 640px block |
| `TestNavCountsOnlyRenderWhereTheyDidBefore` (189) | 4 `class="nav-count"` on billing/dashboard/tenants/expenses; 0 on tenant-detail/cash-receipt/cash-receipt-void |
| `TestPayerPreviewScrollsTheTableNotTheCard` (256) | `.tp-scroll{overflow-x:auto}` present; old `.card{padding:18px;overflow-x:auto}` gone; exact `.card{...}` rule unchanged; `<div class="tp-scroll"><table>` and `</table></div>` present |
| `TestBillingActionCellCollapsesOnlyOnNarrowScreens` (293) | Exact billing strings (see conflict list below) |
| `TestBillingNarrowScreenBlockFollowsTheWideScreenOne` (348) | `@media (min-width: 641px) {` occurs before anchor `.confirm-form .btn,\n      .bind-form select,` |

### `billing_layout_test.go`

`renderBillingPage` helper plus: `TestBillingFilterBarKeepsOnlyTheSixQuestions` (21), `TestBillingLegacyArrivalRangeSurvivesAsHiddenInputs` (49), `TestBillingTransactionRowsShowOnlyConfirmedRentMonthAndNoTechnicalIDs` (62), `TestBillingTemplateUsesExplicitOneClickMatchAndLimitedRematch` (102), `TestBillingRematchSelectsStayBehindTheRematchButton` (150) — asserts `<details class="rematch-details">` has no `open`, summary `<summary class="btn">修改匹配</summary>`; `TestRematchFilterOptionsStayIndependentAndDeduplicated` (179), `TestBillingTemplateKeepsSplitMatchOnTheRevokeFlow` (193), `TestBillingListCarriesTheColumnSort` (214), `TestPendingFilterAndPendingMatchStatusParseAlike` (249), `TestBillingSortFlipsBothWays` (272).

**Card-redesign conflicts in this file / mobile_layout_test:**
- `TestBillingActionCellCollapsesOnlyOnNarrowScreens` requires these exact rendered strings, all of which a card layout would remove: `<details class="txn-action" open>`, `.txn-col-action > .txn-action > summary { display: none; }`, `.transaction-table { min-width: 680px; }`, `.transaction-table .txn-col-desc,\n      .transaction-table .txn-col-account { display: none; }`, `.transaction-table .txn-col-amount { display: none; }`, `.transaction-table .txn-col-action { width: 118px; padding: 10px 8px; }`, `.txn-amount-mobile { display: block; ... }`, `.txn-amount-mobile { display: none; }` with the block rule after the none rule.
- `TestBillingTemplateUsesExplicitOneClickMatchAndLimitedRematch` (102) requires `一键匹配`, `修改匹配`, `aria-label="修改匹配租客"`, `aria-label="修改租金月份"` etc.
- `TestBillingListCarriesTheColumnSort` (214) requires the three sort links as `<a class="sort-link" ...>...</a>` with exact hrefs — a card layout must keep sortable headings or replace them with equivalent links carrying `sort=`.
- `TestPayerPreviewScrollsTheTableNotTheCard` (256) requires `<table>` + `.tp-scroll` in `payerPreviewTemplate`.

### `dashboard_layout_test.go`

`TestRentDashboardSeparatesPeriodBarSummaryAndList` (29) — markup structure split into period bar / summary / list; `markupBetween` anchors on `<div class="dashboard-toolbar">`, `<section class="dashboard-section" aria-labelledby="dashboard-summary-title">`, `...rent-status-title...`, `<section id="dunning-drawer"`; requires `class="panel dashboard-counts"`, count chips, `name="period" value="2026-09"`, `class="sort-link` count ≥4, hidden `name="sort" value="due_desc"`. `TestSortLinkTogglesDirections` (135), `TestNormalisedSortFallsBackToTheDefaultColumn` (163), `TestRentDashboardMetricsKeepTheE2EShape` (175) — **requires `<div class="label">本月应收</div><strong>` with no whitespace/wrapper between label and value** (E2E runner regex), and `当前显示` text; `TestRentDashboardManualBalanceActionOnlyAppearsForOutstandingRent` (196) — requires `>一键平账</button>` and `action="/rent-dashboard/settle"`.

**Card-redesign note:** `TestRentDashboardSeparatesPeriodBarSummaryAndList` anchors on the exact `dashboard-section`/`aria-labelledby` markup and on `<nav class="dashboard-pagination"`; restructuring list rows into cards does not by itself break it, but removing the `<table>` or the sort links would (it counts `class="sort-link`).

### `tenant_detail_layout_test.go`

`TestTenantDetailPanelBodiesMatchTheHeaderEdge` (30) — for `.profile-list`, `.payer-list`, `.history-filter`, `.pagination` requires padding `18px 20px;`, `18px 20px;`, `16px 20px 0;`, `0 20px 18px;` respectively (via `styleRuleFor`). Not a table test, but a card redesign that reuses these blocks must keep the paddings.

### Relevant `main_test.go` guards

- `TestWorkspaceTemplatesIncludeSharedCalendarPicker` (65) — calendar markers in every workspace page.
- `TestWorkspaceUIPrimitivesHaveConsistentInteractionStates` (115) — `.btn:focus-visible`, `.btn:disabled`, `.btn[aria-disabled="true"]`, `input:focus-visible, select:focus-visible, textarea:focus-visible`, `.btn {\n      min-height: 40px;`.
- `TestWorkspaceCSSUsesFlatDesignTokens` (129) — the flat design guard (see §3).
- `TestWorkspaceCSSGuardsResponsiveContent` (154) — requires `overflow-x: hidden;`, `.table-wrap { max-width: 100%; overflow-x: auto; }`, `@media (max-width: 640px)`, `input[type="checkbox"], input[type="radio"] { width: auto,`.
- `TestRentDashboardTemplateRendersMonthlyStatus` (901), `TestBillingTemplateRendersTransactionFilters` (731), `TestBillingTemplateShowsMonthChoiceForRememberedTenant` (760), `TestBillingTemplateOffersHistoricalPayerPreview` (788), `TestBillingTemplateHidesAllocationForCompletedOrIgnoredIncome` (801) — existing billing/dashboard template assertions worth re-reading before restructuring.

---

## 3. `main_test.go` flat design guard

**File**: `/Users/dd/projects/bank/cmd/truelayer-demo/main_test.go`, **lines 129-152** (function `TestWorkspaceCSSUsesFlatDesignTokens`). The cut is at **line 135**; the forbidden list is at **line 147-151**.

```go
func TestWorkspaceCSSUsesFlatDesignTokens(t *testing.T) {
	// The flat look is a desktop constraint: the base stylesheet must stay
	// free of depth effects. The `@media (max-width: 640px)` block below it is
	// mobile-only, and there the drawer scrim and the frozen column's inset
	// shadow are functional (they say "there is more to the right"), so the
	// guard stops at the breakpoint.
	baseCSS, _, _ := strings.Cut(workspacePageCSS, "@media (max-width: 640px)")
	for _, expected := range []string{
		"--surface: #ffffff;",
		"--foreground: #111827;",
		"--accent: #2563eb;",
		"background: var(--background-base);",
		".panel { border: 1px solid var(--border); border-radius: 12px; background: var(--surface); }",
	} {
		if !strings.Contains(baseCSS, expected) {
			t.Fatalf("flat workspace CSS missing %q", expected)
		}
	}
	for _, forbidden := range []string{"backdrop-filter:", "box-shadow:", "radial-gradient", "linear-gradient", "rgba("} {
		if strings.Contains(baseCSS, forbidden) {
			t.Fatalf("flat workspace CSS still contains depth effect %q", forbidden)
		}
	}
}
```

**Precisely what fails if a new card layout adds shadows/rgba:**
- The guard only inspects `workspacePageCSS` — not page-local `<style>` blocks. `workspacePageCSS` is defined at `main.go:2164`; its first `@media (max-width: 640px)` is at `main.go:2409`. So `baseCSS` = `main.go:2164` up to (excluding) line 2409.
- If a card layout adds `box-shadow:`, `rgba(`, `linear-gradient`, `radial-gradient` or `backdrop-filter:` **to `workspacePageCSS` before its first 640px block**, the test fails with `flat workspace CSS still contains depth effect "<token>"`.
- Adding the same token **inside the 640px mobile tail** is explicitly allowed (the drawer scrim `background: rgba(17, 24, 39, 0.45);` at `main.go:2476` and the frozen-column shadow at `main.go:2488` are in the tail).
- Adding the token to a **page-local `<style>`** does not trip this guard. The dark-theme `:root` with many `rgba(`/gradients at `main.go:2803+` belongs to separate templates (revoke/payer-preview pages) that bypass `workspacePageCSS`, which is why the guard passes today.

---

## 4. Browser harness

**Actual path**: `/Users/dd/projects/bank/.trellis/tasks/archive/2026-09/09-16-mobile-responsive-audit/research/harness/` (29 `.mjs` scripts + README.md + package.json/package-lock.json). No `node_modules` present.

### Script inventory (one line each; from README + file headers)

| Script | Purpose |
|---|---|
| `audit.mjs` | Main pass: 8 pages x 3 viewports (375/390/768), probes horizontal overflow + "root" offenders + active scrollers, screenshots, writes `report.json` |
| `audit-extra.mjs` | The 2 pages the first pass missed (billing revoke GET, payer preview POST) into `report-extra.json` |
| `metrics.mjs` | Touch targets <44px, text <12px, per-table scroll range + per-column `scrollToSee`, font histogram -> `metrics.json` |
| `verify-p1.mjs` | Asserts each named P1 item at 375 and 390 (page overflow, calendar trigger, chip heights, tenant-link hit width, every control ≥44, named column widths) |
| `verify-offenders.mjs` | Re-walks each audit offender's ancestors, buckets `in scroller` / `off-canvas` / `PAGE-LEVEL?` |
| `verify-hits.mjs` | Clicks hit areas (tenant-link `::after`, label-wrapped checkbox) — the only script that settles hit area by clicking |
| `verify-clipping.mjs` | Measures whether `/billing` amount column is truncated mid-glyph; re-checks the 26x16 dashboard target |
| `verify-claims.mjs` | Checks 2 design-driving claims: tenant name still on screen when scrolled to 操作; `/expenses` 类别 column breaks CJK per-glyph |
| `verify-check-fixes.mjs` | Verifies the 3 code-check fixes (tenant-link underline, sr-only drawer checkbox focus, collapsed `<details>` hit testability) |
| `verify-tenants.mjs` | Row heights, column rects, right-column appearance after horizontal scroll on `/tenants` |
| `probe-p1-4.mjs` | Measures all ~20 P1-4 claims on the deployed build |
| `probe-controls.mjs` | Dumps every control with measured box, to trace a sub-44px control to the winning rule |
| `probe-rows.mjs` | Real line counting via `Range.getClientRects()`, plus `/billing` frozen-column geometry |
| `probe-expanded.mjs` | Collapsed vs expanded `/billing` row (the 处理 summary form) |
| `probe-dash.mjs` | Dashboard control boxes |
| `probe-wide.mjs` | Walks every element at 1440 to find the `/billing` desktop wide-overflow root offender |
| `probe-css.mjs` | Whether preview templates' CSS survives Go's `html/template` CSS escaper |
| `probe-payer.mjs` | Whether the payer-preview confirm form is reachable with live data |
| `probe-start.mjs` | Content top edge ≤ y=120 at 375 on every page; P2 page heights; re-settles P0-2 |
| `desktop-full.mjs` | Desktop regression sweep, all live pages at 1440x900; logs scrollWidth/clientWidth/heights + md5s sidebar HTML |
| `desktop.mjs` | Older desktop baseline: full-page screenshots at 1440x900 for the four list pages |
| `chrome-diff.mjs` | Captures each page's chrome (sidebar + topbar) for textual pre/post comparison |
| `imgdiff.mjs` | Per-pixel PNG diff with bounding box (no ImageMagick) |
| `tiles.mjs` | Slices tall full-page screenshots into viewport-sized tiles (2x) |
| `crop.mjs` | Cuts one region out of a tile in `/tmp/mobile-audit/tiles/` |
| `shot-rows.mjs` | Clips a screenshot to the first N table rows at phone width |
| `shell.mjs` | Ad-hoc shell measurement on `/tenants` |
| `smoke.mjs` | Launch/login smoke check |
| `v-profile.mjs` | Ad-hoc tenant profile measurements |

### Configuration (env vars)

- `CHROME_BIN` — Chromium path. Defaults to `/tmp/mobile-audit/chrome-linux64/chrome` in `audit.mjs`, `audit-extra.mjs`, `desktop.mjs`, `desktop-full.mjs`, `chrome-diff.mjs`, `imgdiff.mjs`, `probe-wide.mjs`, `verify-p1.mjs`. **`metrics.mjs`, `probe-controls.mjs`, `probe-css.mjs`, `probe-dash.mjs`, `probe-expanded.mjs`, `probe-p1-4.mjs`, `probe-payer.mjs`, `probe-rows.mjs`, `probe-start.mjs`, `shot-rows.mjs`, `shell.mjs`, `smoke.mjs`, `tiles.mjs`, `v-profile.mjs`, `verify-check-fixes.mjs`, `verify-claims.mjs`, `verify-clipping.mjs`, `verify-hits.mjs`, `verify-offenders.mjs`, `verify-tenants.mjs` hardcode `/tmp/mobile-audit/chrome-linux64/chrome` with no override** (except `verify-p1.mjs`, `probe-wide.mjs`, `chrome-diff.mjs` which honour `CHROME_BIN`).
- `AUDIT_BASE` — base URL, default `http://localhost:8081`. Hardcoded to `http://localhost:8081` in `probe-css.mjs`, `probe-payer.mjs`, `v-profile.mjs`; `shell.mjs` hardcodes `http://localhost:8081/`.
- `AUDIT_USER` / `AUDIT_PASS` — login. `verify-tenants.mjs:7-8` carries a **hardcoded fallback credential pair** (see §7 security note); `v-profile.mjs` reads `process.env.U`/`process.env.P` instead.
- `AUDIT_OUT` — screenshot + report dir, default `/tmp/mobile-audit/out` (explicit on most; `metrics.mjs` and `verify-tenants.mjs` write to `/tmp/mobile-audit/out` hardcoded).
- `REPORT` — `verify-offenders.mjs` input report, default `/tmp/mobile-audit/out/report.json`.
- `PROBE_W`, `PROBE_URLS`, `PROBE_URL`, `PROBE_ROWS`, `TILE_OUT` — per-probe overrides.

### How a local instance is started

README's two options:
1. **Local build + run** — `cd /home/ubuntu/projects/bank_test` (a server path, not this machine), `go build -o /home/ubuntu/rentops-app/rentops-app ./cmd/truelayer-demo`, `sudo systemctl restart rentops-app-live.service` (:8081, nginx `bank.ddpl.top`). **README explicitly warns there is no sandbox: `:8081` is the production app against the production DB.** The harness's writes are read-only by convention (GETs + the read-only `/billing/payer/preview` POST).
2. **Desktop-regression worktree** — `git worktree add /tmp/head-wt HEAD`, build, run on `:8082` with `TL_ADDR=:8082 TL_LOG_FILE=... TL_TOKEN_FILE=...`, then `AUDIT_BASE=http://localhost:8082 ... node desktop-full.mjs`.

### `audit.mjs` output

`report.json` (written to `AUDIT_OUT`): `{ base, generatedAt, pages[], states[], consoleErrors[] }`. Each page entry: `{ page, viewport, url, status, result }` where `result = { viewportWidth, docScrollWidth, horizontalOverflow, offenders[<=25], activeScrollers[<=10] }`. Offenders are the outermost elements escaping the viewport by >1px, with `selector,width,right,escapesRightBy,minWidth,overflowX,text`. It also writes full-page PNGs per page/viewport, plus two hidden-layout states (`tenants-row-expanded`, `dashboard-dunning-drawer`) and a logged-out `login` page.

### Which scripts measure what

- **Horizontal overflow**: `audit.mjs` (`docScrollWidth - vw`, `offenders`), `audit-extra.mjs`, `probe-wide.mjs` (desktop), `verify-offenders.mjs` (page overflow per bucket), `desktop.mjs`/`desktop-full.mjs` (scrollWidth vs clientWidth).
- **Hit areas**: `verify-hits.mjs` (only real-click script), `metrics.mjs` (element boxes <44px), `verify-p1.mjs` (≥44 assertions), `verify-clipping.mjs` (smallest links), `probe-controls.mjs`, `probe-dash.mjs`.
- **Clipping**: `verify-clipping.mjs` (per-cell visible fraction / `sliced` / `selfClipped`), `metrics.mjs` (`scrollToSee` per header), `probe-rows.mjs`.

### Paths hardcoded that no longer exist

- **`/tmp/mobile-audit/chrome-linux64/chrome` does not exist on this machine** (checked; `/tmp/mobile-audit` is absent). ~20 scripts hardcode it. README says the Chromium was unpacked in `/tmp` and "is **not** committed (170MB); point `CHROME_BIN` at any Chromium" — but only the `AUDIT_*` scripts actually honour `CHROME_BIN`; the `verify-*`/`probe-*`/`metrics`/`tiles`/`crop`/`shot-rows` scripts need editing or `CHROME_BIN` support added before they run.
- **`node_modules` is absent**; `npm install` (playwright ^1.63.0) is required.
- **Spec §6 points at `.trellis/tasks/09-16-mobile-responsive-audit/research/harness/` which does not exist** — it is archived under `archive/2026-09/`.
- README references server paths `/home/ubuntu/projects/bank_test` and the `rentops-app-live.service` — not present on a dev machine.
- `crop.mjs` reads `/tmp/mobile-audit/tiles/`; `metrics.mjs`/`verify-tenants.mjs` write `/tmp/mobile-audit/out/` — absent until recreated.

---

## 5. How to run the app locally

**Command**: `./scripts/run-truelayer-demo.sh` (from `/Users/dd/projects/bank`). It sources `.env` (set -a), sets defaults for `TL_ENV`, `TL_ADDR=:8080`, `TL_REDIRECT_URI`, `TL_LOG_FILE`, `TL_TOKEN_FILE`, `MIGRATIONS_DIR=migrations`, validates required vars, then `exec go run ./cmd/truelayer-demo`.

**What it needs**:
- A `TL_CLIENT_ID` + `TL_CLIENT_SECRET` (TrueLayer). A `.env` exists at repo root and `cmd/truelayer-demo/.env`.
- A MySQL-compatible DB via `MYSQL_DSN` or `DATABASE_URL`. The repo-root `.env` (gitignored) supplies one pointing at `127.0.0.1:3306` — needs a local MySQL with that DB/user.
- Migrations in `migrations/` are run on startup (`001`-`008`).
- Admin user is seeded from `APP_ADMIN_USERNAME`/`APP_ADMIN_PASSWORD` if absent (`auth.go:37 seedDefaultUser`, called `main.go:389`); the values are in the gitignored `.env`. **No tenant/demo data is seeded by the app.**
- Repo `.env` sets `TL_ENV=live`, which makes `BANK_TOKEN_ENCRYPTION_KEY` required (present in `.env`); a sandbox run can use `ALLOW_PLAINTEXT_TOKENS=1` instead.
- Optional: `TL_PROVIDERS`/`TL_PROVIDER_ID`/`TL_AUTH_URL`, `TL_FROM`, `TL_LOG_FILE`, `TL_TOKEN_FILE`, `RENTOPS_TENANT_FILE`, `RENTOPS_EXPENSE_FILE`, `APP_SESSION_SECRET`.

Open `http://localhost:8080`; login redirects to `/billing`. Sidebar: `/rent-dashboard`, `/tenants`, `/expenses`.

**Disposable E2E**: `./scripts/run-e2e-local.sh` builds `cmd/truelayer-demo` + `cmd/rentops-e2e`, creates a throwaway MySQL DB (`rentops_e2e_*`), seeds two isolated accounts via app startup, runs the runner on `:18080`, drops the DB. It requires `sudo mysql` on `127.0.0.1:53306` (see script defaults) and deliberately never touches a live DB. Note: `AGENTS.md` contains only Trellis boilerplate; it has no run instructions.

---

## 6. Existing test data / fixtures

**The "demo account's 10 tenants" are NOT reproducible from the repo.** Findings:

- `mobile-audit.md:3` states the audit ran against `https://bank.ddpl.top`, "生产库，`rentops-demo` 账号的真实数据，10 个租客" (production DB, real data of account `rentops-demo`, 10 tenants).
- `harness/README.md:107-110`: "The demo account used as `AUDIT_USER` is `rentops-demo`; it owns 10 seeded tenants that between them cover every row shape the tables render (auto-match, partial, overdue, cross-month split, unconfirmed, revoked)." It notes these rows "live in the **production** database" and that its requests must therefore be read-only.
- `harness/verify-tenants.mjs:7-8` hardcodes a fallback username/password pair for the live demo account. **This file is committed and the credential is in git history** — see §7.
- The predecessor PRD `archive/2026-09/09-16-mobile-responsive-audit/prd.md:55` (R5): "校验所用的页面必须**带真实数据**（demo 账号的 10 个租客），不能用空数据蒙混过关" — pages must be verified with real data.
- **No repo seed script/SQL creates these tenants.** `grep "INSERT INTO tenants"` across `migrations/`, `scripts/`, `docs/` returns nothing; the only seeding in code is the admin user (`auth.go:37`). Migrations only transform existing rows (e.g. `004_tenant_profile_and_payers.sql` backfills `tenant_payers` from existing `tenants`). There is no `rentops-demo` string anywhere in `.go`/`.sql`/`.sh` outside the archived audit docs and `verify-tenants.mjs`.
- The tenants were therefore created through the app UI / `POST /import-legacy` into the production DB, not from a committed fixture.
- Other in-repo data: `bank-data.json`, `log.json` (legacy JSONL compatibility logs), `demo/rent-management-v1-dashboard.html` (a static design mock), and `docs/superpowers/specs/2026-06-02-rent-management-v1-design.md`.
- `cmd/rentops-e2e/` generates its own runtime fixtures (manifest / legacy_fixture.go) inside a `chmod 700` temp dir and cleans them up; these are for the disposable E2E run, not usable as a browser-audit data set.

**Caveat for the implement plan**: a new browser audit run against `:8081` would hit the production app and DB (README warns explicitly). Re-running the audit requires either that live instance with the real demo account, or creating an equivalent tenant/transaction set in a fresh DB — the repo provides no seed for it.

---

## 7. Security notes found while researching

1. **Committed credential.** `harness/verify-tenants.mjs:7-8` contains a working username/password pair for the live demo account on `bank.ddpl.top`. It was committed by the previous task (squashed into `0f92566`) and is therefore in git history, not just the working tree. Reported here so the owner can decide; **not fixed by this research** because it is outside this task's scope.
2. **Re-running the archived harness must not target the live instance.** Every write path in this task must use a disposable DB — `scripts/run-e2e-local.sh` already creates a throwaway `rentops_e2e_*` DB and is the safe precedent. Any browser verification against `:8081` reads production data.
3. Credentials for local runs live in the gitignored `.env`. Nothing in the repo should copy their values into tracked files, including task research notes.
