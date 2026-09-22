package main

import (
	"html/template"
	"strings"
	"testing"
)

// executeTemplate renders one page and hands back the markup.
func executeTemplate(tmpl *template.Template, data any) (string, error) {
	var body strings.Builder
	if err := tmpl.Execute(&body, data); err != nil {
		return "", err
	}
	return body.String(), nil
}

// splitWorkspaceCSS returns everything written above the 640 tier and the
// mobile tail that follows it. The head is no longer the frozen desktop
// stylesheet: it legitimately carries the base rules, the desktop shell and the
// @media (max-width: 1100px) tier (responsive-conventions.md §1). What it still
// must not contain is a mobile-only *mechanism* -- the drawer, the frozen
// column, the bottom navigation -- because those exist only at <=640.
func splitWorkspaceCSS(t *testing.T) (head, mobile string) {
	t.Helper()
	head, mobile, found := strings.Cut(workspacePageCSS, "@media (max-width: 640px)")
	if !found {
		t.Fatal("workspacePageCSS has no @media (max-width: 640px) block")
	}
	return head, mobile
}

// The tiers have to be written base -> 1100 -> 640 so range nesting comes first
// and the narrow tier last: at equal specificity the later rule wins. Written
// the other way round the 640 rules would also be the last match at 1440px.
func TestWorkspaceCSSOrdersThe1100TierBeforeThe640Tier(t *testing.T) {
	tier1100 := strings.Index(workspacePageCSS, "@media (max-width: 1100px)")
	tier640 := strings.Index(workspacePageCSS, "@media (max-width: 640px)")
	if tier1100 < 0 {
		t.Fatal("workspacePageCSS has no @media (max-width: 1100px) tier")
	}
	if tier640 < 0 {
		t.Fatal("workspacePageCSS has no @media (max-width: 640px) tier")
	}
	if tier1100 > tier640 {
		t.Fatal("the 1100 tier is written after the 640 tier; at equal specificity the later rule wins, so the mobile contract would override the desktop tier")
	}
	// A tier that carries no rule is a tier that does nothing. The prototype's
	// @media(max-width:1100px) folds its wide two-column grids to one column, and
	// .grid-two is the shared sheet's counterpart.
	if !strings.Contains(workspacePageCSS[tier1100:tier640], "grid-template-columns: 1fr;") {
		t.Fatal("the 1100 tier carries no prototype-backed compaction")
	}
}

// The shared sheet only reaches the selectors it owns (§3.3), and the prototype's
// remaining 1100 collapses sit on page-local selectors, so the tier is only real
// if a page sheet carries its own 1100 block. "The tier exists" and "the tier does
// something" are two distinct claims; this asserts the second one where the
// prototype makes it, and that the page rewrote its old threshold instead of
// stacking a second block on the same selector.
func TestCollectionSummaryCollapsesInThe1100Tier(t *testing.T) {
	sheet := embeddedWebText("web/static/css/pages/collection-pages.css")
	const collapse = ".collection-summary { grid-template-columns: repeat(2,minmax(0,1fr)); }"

	tier1100 := strings.Index(sheet, "@media (max-width: 1100px)")
	if tier1100 < 0 {
		t.Fatal("collection-pages.css has no page-local @media (max-width: 1100px) block")
	}
	tier640 := strings.Index(sheet, "@media (max-width: 640px)")
	if tier640 >= 0 && tier1100 > tier640 {
		t.Fatal("the page-local 1100 block is written after the 640 block; the later rule wins at every width")
	}
	if !strings.Contains(sheet[tier1100:], collapse) {
		t.Fatal("the page-local 1100 block does not collapse .collection-summary to the prototype's two columns")
	}
	if got := strings.Count(sheet, collapse); got != 1 {
		t.Fatalf(".collection-summary is collapsed in %d blocks, want 1; rewrite its threshold instead of stacking blocks", got)
	}
}

// The sidebar used to be copy-pasted into every page template and the copies had
// already drifted: the cash-receipt pages rendered no count badges, the footer
// carried three different captions, and the dashboard alone labelled its <nav>.
// One shared definition is what makes the drawer a one-place change, so every
// page has to go on rendering exactly one of it. This is a template contract and
// it survives the unfreeze of the desktop rendering (responsive-conventions.md
// §1): the chrome is shared, not duplicated.
func TestEveryWorkspacePageRendersTheSharedChromeOnce(t *testing.T) {
	pages := map[string]func() (string, error){
		"billing": func() (string, error) {
			return executeTemplate(billingTemplate, billingPageData{})
		},
		"rent-workspace": func() (string, error) {
			// The legacy no-database fallback template was removed; the dashboard
			// path that still renders is the room workspace.
			return executeTemplate(rentWorkspaceTemplate, rentWorkspacePageData{})
		},
		"more": func() (string, error) {
			return executeTemplate(morePageTemplate, morePageData{})
		},
		"transaction-detail": func() (string, error) {
			return executeTemplate(transactionDetailPageTemplate, transactionDetailPageData{})
		},
		"tenants": func() (string, error) {
			return executeTemplate(tenantTemplate, tenantPageData{})
		},
		"expenses": func() (string, error) {
			return executeTemplate(expenseTemplate, expensePageData{})
		},
		"tenant-detail": func() (string, error) {
			return executeTemplate(tenantDetailTemplate, tenantDetailPageData{})
		},
		"cash-receipt": func() (string, error) {
			return executeTemplate(cashReceiptTemplate, cashReceiptFormData{})
		},
		"cash-receipt-void": func() (string, error) {
			return executeTemplate(cashReceiptVoidTemplate, cashReceiptVoidPageData{})
		},
	}

	for name, render := range pages {
		page, err := render()
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		for marker, want := range map[string]int{
			`id="nav-drawer"`:               1,
			`<aside class="sidebar"`:        1,
			`class="nav-compact-bar"`:       1,
			`class="nav-scrim"`:             1,
			`class="workspace-page-topbar"`: 1,
			// The toast ships with the chrome, so "every page has one" is the same
			// contract as the rest of this map -- and one page having two would make
			// the shared script write into whichever it found first.
			`id="workspace-toast"`: 1,
		} {
			if got := strings.Count(page, marker); got != want {
				t.Fatalf("%s renders %q %d times, want %d", name, marker, got, want)
			}
		}
	}
}

func TestDesktopWorkspaceBreadcrumbHasMatchingContentOffset(t *testing.T) {
	if !strings.Contains(workspacePageCSS, "@media (min-width: 981px)") || !strings.Contains(workspacePageCSS, "left: 236px;") || !strings.Contains(workspacePageCSS, ".content { padding-top: 88px; }") {
		t.Fatal("desktop page breadcrumb and content offset must share the 236px sidebar, 64px header, and 24px page gutter")
	}
	if !strings.Contains(workspacePageCSS, ".workspace-page-topbar { display: none; }") {
		t.Fatal("workspace breadcrumb must be suppressed outside the desktop breakpoint")
	}
}

// The drawer is pure CSS: a hidden checkbox holds the open state and the sidebar
// slides in on :checked. It has to keep working with JavaScript disabled, so the
// rules live in the stylesheet and not in a script.
//
// The head guard below used to mean "the desktop rendering is frozen". It now
// means something narrower and still true: the drawer is a <=640 mechanism, so
// none of its declarations may sit above the 640 tier. The head may carry
// desktop alignment and the 1100 tier; it may not carry the drawer.
func TestWorkspaceCSSDrawerIsCSSOnlyAndMobileScoped(t *testing.T) {
	base, mobile := splitWorkspaceCSS(t)
	if !strings.Contains(mobile, ".app { display: flex; flex-direction: column; padding: 8px; }") {
		t.Fatal("mobile workspace content must follow the compact navigation without an empty grid row")
	}

	for _, forbidden := range []string{"#nav-drawer", "translateX", "nav-burger"} {
		if strings.Contains(base, forbidden) {
			t.Fatalf("drawer declaration %q reached the desktop stylesheet", forbidden)
		}
	}
	if !strings.Contains(base, ".nav-compact-bar,\n    .nav-scrim { display: none; }") {
		t.Fatal("the compact bar and the scrim are not hidden above the breakpoint")
	}

	for _, expected := range []string{
		".nav-compact-bar {",
		"transform: translateX(-100%);",
		"#nav-drawer:checked ~ .sidebar { transform: translateX(0); visibility: visible; }",
		"#nav-drawer:checked ~ .nav-scrim {",
	} {
		if !strings.Contains(mobile, expected) {
			t.Fatalf("mobile drawer CSS is missing %q", expected)
		}
	}

	// The tab order is the half of the drawer that does not show up in a
	// screenshot, and both halves have failed at some point. Closed, the sidebar
	// must be out of the tab order as well as off-screen (translate alone leaves
	// its links focusable and the focus ring lands past the viewport edge); open,
	// it must come back. The state checkbox must not be `hidden`, which is
	// display:none and therefore unreachable by keyboard — that would make the
	// narrow-screen navigation openable by pointer only.
	if !strings.Contains(mobile, "visibility: hidden;") {
		t.Fatal("the closed drawer leaves its links in the tab order")
	}
	if !strings.Contains(mobile, ".nav-drawer-input {") {
		t.Fatal("the drawer state checkbox has no narrow-screen style; a bare checkbox would render in the layout")
	}
	if strings.Contains(base, ".nav-drawer-input { display: none; }") == false {
		t.Fatal("the drawer state checkbox is not suppressed above the breakpoint")
	}
	if strings.Contains(workspaceNav, `class="nav-drawer-input" hidden`) {
		t.Fatal("the drawer checkbox carries `hidden`, which cannot take focus")
	}
}

func TestMobileBottomNavExposesFiveSectionsAndObjectsMenu(t *testing.T) {
	page, err := executeTemplate(rentWorkspaceTemplate, rentWorkspacePageData{workspaceShell: workspaceShell{ActivePage: "rent-dashboard"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{
		`class="mobile-bottom-nav"`,
		`href="/rent-dashboard"`,
		`href="/transactions"`,
		`href="/dunning"`,
		`data-mobile-menu="objects"`,
		`href="/more"`,
		`id="mobile-menu-objects"`,
		`href="/properties"`,
		`href="/rooms"`,
		`href="/tenants"`,
	} {
		if !strings.Contains(page, marker) {
			t.Fatalf("mobile shell missing %q", marker)
		}
	}
	if strings.Contains(page, `id="mobile-menu-more"`) {
		t.Fatal("more navigation should open its own page instead of a flyout")
	}
	if strings.Contains(strings.Split(workspacePageCSS, "@media (max-width: 640px)")[0], "mobile-bottom-nav {") {
		t.Fatal("bottom navigation rules must stay mobile-only")
	}
}

func TestMorePageMatchesMobilePrototypeAndKeepsMoreNavigationActive(t *testing.T) {
	page, err := executeTemplate(morePageTemplate, morePageData{workspaceShell: workspaceShell{ActivePage: "more"}})
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{
		`class="mobile-bottom-nav-item active" href="/more"`,
		`class="more-page-grid"`,
		`href="/cash-receipts"`, `href="/expenses"`, `href="/dunning"`, `href="/bank"`, `低频操作集中在这里。`,
	} {
		if !strings.Contains(page, marker) {
			t.Fatalf("more page missing %q", marker)
		}
	}
	for _, removed := range []string{`href="/tenancies"`, "租约管理", `href="/bills"`, "应收账单"} {
		if strings.Contains(page, removed) {
			t.Fatalf("cash or expenses still has a navigation card %q", removed)
		}
	}
}

// The high-frequency rent-row list collapses to cards on narrow screens instead
// of a sideways-scrolling table, and the settle affordance travels with the card.
// This was asserted on the legacy /rent-dashboard fallback template, which was
// removed; the contract is now pinned on /bills, its live carrier (the room
// workspace has its own card markup, covered in rent_workspace_test.go).
func TestMobileBillsListUsesCardsForRentRows(t *testing.T) {
	page, err := executeTemplate(billsPageTemplate, rentDashboardPageData{
		Period:      "2026-09",
		PeriodLabel: "2026年9月",
		Page:        1,
		PageSize:    12,
		TotalPages:  1,
		Rows:        []rentDashboardRow{{TenantID: 7, TenantName: "陈先生", RoomLabel: "2B", RoomAddress: "Rosewood Court", Period: "2026-09", ExpectedAmount: "EUR 1280.00", PaidAmount: "EUR 640.00", BalanceAmount: "EUR 640.00", ExpectedCents: 128000, PaidCents: 64000, Status: "partial", StatusLabel: "部分缴纳", ObligationID: 9}},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{`class="collection-mobile-list"`, `class="collection-bill-card"`, `填写平账原因`, `class="table-wrap collection-table-wrap"`, `details class="collection-settle"`} {
		if !strings.Contains(page, marker) {
			t.Fatalf("mobile bills cards missing %q", marker)
		}
	}
}

func TestMobileTenantListUsesCards(t *testing.T) {
	page, err := executeTemplate(tenantTemplate, tenantPageData{
		Rows: []tenantRecord{{
			ID:           "7",
			Name:         "陈先生",
			DisplayAlias: "陈先生",
			Status:       "active",
			BillingHistory: []tenantBillingMonth{{
				PeriodLabel:   "2026年9月",
				StatusLabel:   "部分缴纳",
				BalanceAmount: "EUR 640.00",
			}},
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{
		`class="tenant-mobile-list"`,
		`class="tenant-mobile-card panel"`,
		`href="/tenants/7"`,
		`class="tenant-mobile-history"`,
		`class="tenant-table-wrap table-wrap"`,
	} {
		if !strings.Contains(page, marker) {
			t.Fatalf("mobile tenant cards missing %q", marker)
		}
	}
}

func TestMobileExpenseListUsesCards(t *testing.T) {
	page, err := executeTemplate(expenseTemplate, expensePageData{
		Rows: []expenseRecord{{
			ID:            "11",
			Description:   "水管维修",
			Category:      "维修",
			AmountDisplay: "EUR 240.00",
			DateDisplay:   "10 Sep 2026",
			PaymentMethod: "手动",
			RoomID:        "2",
			TenantHint:    "陈先生",
			InvoiceURL:    "https://example.test/invoice/11",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{
		`class="expense-mobile-list"`,
		`class="expense-mobile-card panel"`,
		`class="expense-mobile-invoice"`,
		`绑定发票`,
		`class="expense-table-wrap table-wrap"`,
	} {
		if !strings.Contains(page, marker) {
			t.Fatalf("mobile expense cards missing %q", marker)
		}
	}
}

func TestMobileBillingRowsStackAsCards(t *testing.T) {
	page, err := executeTemplate(billingTemplate, billingPageData{
		TransactionRows: []transactionPageRow{{
			ID:               "7",
			Direction:        "income",
			DirectionLabel:   "收入",
			PayerName:        "陈先生",
			AmountDisplay:    "EUR 640.00",
			DateDisplay:      "10 Sep 2026",
			Description:      "September rent",
			MatchStatus:      "candidate",
			MatchStatusLabel: "待确认",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{
		`class="billing-table-wrap table-wrap"`,
		`class="transaction-table"`,
		`.billing-table-wrap .transaction-table tbody tr { display: grid;`,
		`class="txn-amount-mobile"`,
	} {
		if !strings.Contains(page, marker) {
			t.Fatalf("mobile billing cards missing %q", marker)
		}
	}
}

func TestMobileSimpleTablesStackAsCards(t *testing.T) {
	base, mobile := splitWorkspaceCSS(t)
	if strings.Contains(base, "table-wrap:not(.billing-table-wrap)") {
		t.Fatal("simple-table mobile rules leaked into the desktop stylesheet")
	}
	for _, marker := range []string{
		`.table-wrap:not(.billing-table-wrap):not(.tenant-table-wrap):not(.expense-table-wrap) > table { display: block;`,
		`> table > thead { display: none; }`,
		`> table > tbody > tr { display: grid;`,
		`> table > tbody > tr > td:last-child { position: static !important;`,
	} {
		if !strings.Contains(mobile, marker) {
			t.Fatalf("mobile simple-table rule missing %q", marker)
		}
	}
}

// TestMobileDunningUsesSafeBottomSheet was removed with the legacy
// /rent-dashboard fallback template (09-19-legacy-dashboard-template-removal).
// The data-dunning-open drawer and its safe-area bottom sheet existed only in
// that template: neither the live /dunning page nor the room workspace renders
// a drawer, so there is no surviving markup to assert. "Launch dunning from the
// workspace" is a prototype feature that is missing on the real path; it is
// registered in the backend spec and handed to the dashboard-alignment subtask.

// Sticky is what keeps a row's identity and its actions on screen while the table
// scrolls sideways, and it only makes sense once the action column has left the
// viewport. It is therefore a <=640 mechanism and must never leak above the 640
// tier -- including into the 1100 tier, which is desktop. The unfrozen head
// (responsive-conventions.md §1) may carry desktop alignment; it may not carry a
// frozen column, because freezing one at 1024px would pin the action cell over
// the columns that fit there.
//
// The rule above is about the frozen *table cell*. The desktop sidebar rail is
// the one sticky rule allowed above 640 (prd.md「用户报告的滚动缺陷」: the
// prototype pins the rail with `position:sticky;top:0`), so this test asserts the
// sticky rule it finds there is the top-anchored rail and not a `right:0` cell.
func TestFrozenLastColumnIsMobileOnly(t *testing.T) {
	base, mobile := splitWorkspaceCSS(t)

	if !strings.Contains(base, "position: sticky;\n        top: 0;") {
		t.Fatal("the desktop sidebar rail lost its sticky top anchor; it must stay pinned while the page scrolls")
	}
	if strings.Contains(base, "position: sticky;\n        right: 0;") {
		t.Fatal("a frozen table cell reached the stylesheet above the 640 tier; the frozen column is mobile-only")
	}
	for _, expected := range []string{
		".table-wrap > table > thead > tr > th:last-child,",
		".table-wrap > table > tbody > tr > td:last-child:not([colspan]) {",
		"position: sticky;",
		"right: 0;",
		// Without an opaque background the scrolled cells show through, and without
		// the inset shadow there is no hint that more columns exist: mobile browsers
		// draw no scrollbar.
		"background: var(--surface);",
		"box-shadow: -8px 0 8px -8px rgba(0, 0, 0, 0.18);",
	} {
		if !strings.Contains(mobile, expected) {
			t.Fatalf("frozen last column CSS is missing %q", expected)
		}
	}
}

// The tenant detail card is the defect the audit started from: the 130px label
// column wasted 98px per row and pinched the value column to 139px, breaking the
// email address across lines. Narrow screens stack the pairs; the wide rule has to
// stay untouched.
func TestTenantDetailProfileListStacksOnlyOnNarrowScreens(t *testing.T) {
	page, err := executeTemplate(tenantDetailTemplate, tenantDetailPageData{})
	if err != nil {
		t.Fatal(err)
	}

	wideRule := ".profile-list { display: grid; grid-template-columns: 130px 1fr; gap: 10px 18px; margin: 0; padding: 18px 20px; }"
	wide := strings.Index(page, wideRule)
	if wide < 0 {
		t.Fatal("the wide .profile-list rule was modified or removed")
	}

	mobileRule := ".profile-list { grid-template-columns: 1fr; gap: 4px 0; }"
	mobile := strings.Index(page, mobileRule)
	if mobile < 0 {
		t.Fatal("the narrow-screen single-column .profile-list rule is missing")
	}

	// The rule must sit inside the page's own narrow-screen block, which is the
	// last one written: html/template strips CSS comments from <style>, so the
	// anchor has to be a declaration.
	block := strings.LastIndex(page[:mobile], "@media (max-width: 640px)")
	if block < wide {
		t.Fatal("the single-column rule is not inside a @media (max-width: 640px) block")
	}
	if strings.Contains(page[block+len("@media (max-width: 640px)"):mobile], "@media") {
		t.Fatal("the single-column rule is not in the last narrow-screen block")
	}
}

// The count badges are a per-page property, not a responsive one: four pages
// render them, three never did. This used to be phrased as a desktop-freeze
// constraint, but it survives the unfreeze (responsive-conventions.md §1) as
// exactly what it says -- the shared chrome must not gain markup a page never
// had. The shell's ShowNavCounts flag is what keeps those three pages clean
// while the other four keep their badges.
func TestNavCountsOnlyRenderWhereTheyDidBefore(t *testing.T) {
	withBadges := map[string]func() (string, error){
		"billing": func() (string, error) {
			return executeTemplate(billingTemplate, billingPageData{workspaceShell: workspaceShell{
				ShowNavCounts: true, TenantCount: 3, IncomeCount: 4, ExpenseCount: 5,
			}})
		},
		"rent-workspace": func() (string, error) {
			return executeTemplate(rentWorkspaceTemplate, rentWorkspacePageData{workspaceShell: workspaceShell{
				ShowNavCounts: true, TenantCount: 3, IncomeCount: 4, ExpenseCount: 5,
			}})
		},
		"tenants": func() (string, error) {
			return executeTemplate(tenantTemplate, tenantPageData{workspaceShell: workspaceShell{
				ShowNavCounts: true, TenantCount: 3, IncomeCount: 4, ExpenseCount: 5,
			}})
		},
		"expenses": func() (string, error) {
			return executeTemplate(expenseTemplate, expensePageData{workspaceShell: workspaceShell{
				ShowNavCounts: true, TenantCount: 3, IncomeCount: 4, ExpenseCount: 5,
			}})
		},
	}
	for name, render := range withBadges {
		page, err := render()
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got := strings.Count(page, `class="nav-count"`); got != 4 {
			t.Fatalf("%s renders %d count badges, want 4", name, got)
		}
	}

	// These three never had badges. The counts are populated anyway: the flag, not
	// an empty value, is what has to keep the markup out.
	withoutBadges := map[string]func() (string, error){
		"tenant-detail": func() (string, error) {
			return executeTemplate(tenantDetailTemplate, tenantDetailPageData{workspaceShell: workspaceShell{
				ShowNavCounts: false, TenantCount: 3, IncomeCount: 4, ExpenseCount: 5,
			}})
		},
		"cash-receipt": func() (string, error) {
			return executeTemplate(cashReceiptTemplate, cashReceiptFormData{workspaceShell: workspaceShell{
				ShowNavCounts: false, TenantCount: 3, IncomeCount: 4, ExpenseCount: 5,
			}})
		},
		"cash-receipt-void": func() (string, error) {
			return executeTemplate(cashReceiptVoidTemplate, cashReceiptVoidPageData{workspaceShell: workspaceShell{
				ShowNavCounts: false, TenantCount: 3, IncomeCount: 4, ExpenseCount: 5,
			}})
		},
	}
	for name, render := range withoutBadges {
		page, err := render()
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if got := strings.Count(page, `class="nav-count"`); got != 0 {
			t.Fatalf("%s renders %d count badges, want none", name, got)
		}
	}
}

// The payer preview turned the whole card into the horizontal scroller: heading,
// blurb and the 返回流水 link all slid out of view together. Only the table should
// scroll, so the overflow moves to a wrapper around it. The page keeps its own
// palette; this is a scrolling fix, not a re-theming.
func TestPayerPreviewScrollsTheTableNotTheCard(t *testing.T) {
	page, err := executeTemplate(payerPreviewTemplate, payerPreviewPageData{
		Rows: []payerPreviewRow{{TransactionID: "1", PayerName: "Aoife", AmountDisplay: "EUR 8.00"}},
	})
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(page, ".tp-scroll{overflow-x:auto}") {
		t.Fatal("the narrow-screen media block does not scroll .tp-scroll")
	}
	if strings.Contains(page, ".card{padding:18px;overflow-x:auto}") {
		t.Fatal("the card is still the horizontal scroller")
	}
	if !strings.Contains(page, `.card{max-width:1100px;margin:auto;padding:24px;border:1px solid var(--border);border-radius:12px;background:var(--surface)}`) {
		t.Fatal("the payer preview card rule changed shape")
	}

	if !strings.Contains(page, `<div class="tp-scroll"><table>`) {
		t.Fatal("the table is not wrapped in .tp-scroll")
	}
	if !strings.Contains(page, "</table></div>") {
		t.Fatal("the .tp-scroll wrapper is not closed after the table")
	}
}

// The billing action column measured 320px, so it cannot be frozen; narrow
// screens swap it for a status badge plus a 处理 toggle that expands in place and
// drop the two secondary columns. Desktop keeps the content inline: there is no
// summary to click, and the ~3 lines of script only strip the open attribute
// where the narrow-screen block applies.
//
// The amount moves into that summary rather than staying frozen beside it. Frozen
// as its own column it was 136-146px, which together with the 88px action column
// took 72% of a 327px window and — once sticky pushed it left — covered the payer
// column outright. The payer name is the only thing that says which row is being
// acted on, so the amount rides along in the frozen cell instead.
func TestBillingActionCellCollapsesOnlyOnNarrowScreens(t *testing.T) {
	page := renderBillingPage(t, billingPageData{workspaceShell: workspaceShell{
		ShowNavCounts: true, TenantCount: 1, IncomeCount: 1, ExpenseCount: 1,
	},
		TransactionRows: []transactionPageRow{{ID: "7", Direction: "income", DirectionLabel: "收入"}},
	})

	if !strings.Contains(page, `<details class="txn-action" open>`) {
		t.Fatal("the action cell is not a <details> opened for the wide-screen rendering")
	}
	if !strings.Contains(page, `<summary class="txn-summary">`) {
		t.Fatal("the collapsed action cell has no summary")
	}
	if !strings.Contains(page, `.txn-col-action > .txn-action > summary { display: none; }`) {
		t.Fatal("the summary is not hidden on wide screens")
	}
	// A closed <details> hides its content through the UA stylesheet; the markup
	// therefore ships open and the script closes it only where the narrow-screen
	// block applies.
	if !strings.Contains(page, `window.matchMedia('(max-width: 640px)')`) {
		t.Fatal("nothing collapses the action cell on narrow screens")
	}
	if !strings.Contains(page, `document.querySelectorAll('details.txn-action[open]').forEach(function(node){node.removeAttribute('open');})`) {
		t.Fatal("the collapse script does not remove the open attribute")
	}

	for _, expected := range []string{
		// Dropping the two secondary columns lowers the table's floor from 1040px,
		// but it must not fall to 0: with no floor CJK wraps one character per line.
		".transaction-table { min-width: 680px; }",
		".transaction-table .txn-col-desc,\n      .transaction-table .txn-col-account { display: none; }",
		// The amount column goes too, and its content reappears inside the frozen
		// action cell. A frozen amount that covers the payer is worse than a frozen
		// amount the user reaches by scrolling a little.
		".transaction-table .txn-col-amount { display: none; }",
		".transaction-table .txn-col-action { width: 118px; padding: 10px 8px; }",
		".txn-amount-mobile { display: block; font-weight: 700; font-variant-numeric: tabular-nums; white-space: nowrap; }",
	} {
		if !strings.Contains(page, expected) {
			t.Fatalf("billing narrow-screen CSS is missing %q", expected)
		}
	}
	// The carried amount must not reach wide screens. Two rules keep it out: the
	// page-local default below the wide-screen block hides it always, and the
	// wide-screen block hides the whole summary it lives in.
	if !strings.Contains(page, ".txn-amount-mobile { display: none; }") {
		t.Fatal("the carried amount is not hidden by default")
	}
	if wide := strings.Index(page, ".txn-amount-mobile { display: none; }"); wide > strings.Index(page, ".txn-amount-mobile { display: block;") {
		t.Fatal("the mobile override is written before the wide-screen default")
	}
}

// The billing narrow-screen block has to sit after the wide-screen one, or the
// sticky amount column and the hidden secondary columns would apply at 1280px.
func TestBillingNarrowScreenBlockFollowsTheWideScreenOne(t *testing.T) {
	page := renderBillingPage(t, billingPageData{})
	wide := strings.Index(page, "@media (min-width: 641px) {")
	// Anchored on a declaration, not a comment: html/template strips CSS comments.
	narrow := strings.Index(page, ".confirm-form .btn,\n      .bind-form select,")
	if wide < 0 || narrow < 0 {
		t.Fatalf("billing media blocks are missing (wide=%d narrow=%d)", wide, narrow)
	}
	if narrow < wide {
		t.Fatal("the narrow-screen block is written before the wide-screen block")
	}
}
