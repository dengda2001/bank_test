package main

import (
	"strings"
	"testing"
)

// markupBetween returns the markup between the first occurrence of start and the
// next occurrence of end. Both markers are required: a renamed class must fail
// the test loudly rather than quietly turn an assertion into a no-op.
func markupBetween(t *testing.T, page, start, end string) string {
	t.Helper()
	startIndex := strings.Index(page, start)
	if startIndex < 0 {
		t.Fatalf("dashboard markup has no %q", start)
	}
	rest := page[startIndex:]
	endIndex := strings.Index(rest, end)
	if endIndex < 0 {
		t.Fatalf("dashboard markup has no %q after %q", end, start)
	}
	return rest[:endIndex]
}

// The dashboard used to be one toolbar holding the title, the dunning button
// and seven filter controls, with the per-status counts floating underneath as
// bare text. It now reads as two blocks: a period bar that only picks a month,
// a summary block, and a list block that owns every control acting on the list.
func TestRentDashboardSeparatesPeriodBarSummaryAndList(t *testing.T) {
	var body strings.Builder
	err := rentDashboardTemplate.Execute(&body, rentDashboardPageData{
		Period:         "2026-09",
		PeriodLabel:    "2026年9月",
		PreviousPeriod: "2026-08",
		NextPeriod:     "2026-10",
		SearchFilter:   "Aoife",
		StatusFilter:   "unpaid",
		SortFilter:     "due_desc",
		Page:           1,
		PageSize:       12,
		TotalPages:     1,
		FilteredCount:  3,
		TotalRows:      3,
		OverdueCount:   1,
		PartialCount:   1,
		ReviewCount:    1,
		Rows:           []rentDashboardRow{{TenantID: 7, TenantName: "Aoife Murphy", Period: "2026-09", Status: "partial", StatusLabel: "部分缴纳"}},
		Dunning:        dunningDrawerData{Enabled: true, Period: "2026-09"},
	})
	if err != nil {
		t.Fatal(err)
	}
	page := body.String()

	periodBar := markupBetween(t, page, `<div class="dashboard-toolbar">`, `<section class="dashboard-section"`)
	if got := strings.Count(periodBar, "<form"); got != 1 {
		t.Fatalf("period bar holds %d forms, want only the month picker: %s", got, periodBar)
	}
	if !strings.Contains(periodBar, `name="period"`) {
		t.Fatalf("period bar has no month picker: %s", periodBar)
	}
	for _, moved := range []string{`id="dashboard-search"`, `id="dashboard-status"`, `id="dashboard-sort"`, `id="dashboard-page-size"`, "邮件催缴", `class="filter-actions"`} {
		if strings.Contains(periodBar, moved) {
			t.Fatalf("period bar still holds %q, it belongs to the list block: %s", moved, periodBar)
		}
	}

	summaryBlock := markupBetween(t, page,
		`<section class="dashboard-section" aria-labelledby="dashboard-summary-title">`,
		`<section class="dashboard-section" aria-labelledby="rent-status-title">`)
	for _, expected := range []string{"本月应收", "已收租金", "剩余未收", "待处理", `class="panel dashboard-counts"`, "当前显示 3 户"} {
		if !strings.Contains(summaryBlock, expected) {
			t.Fatalf("summary block is missing %q: %s", expected, summaryBlock)
		}
	}
	for _, leaked := range []string{`class="list-filter"`, "租客缴费情况", `id="dashboard-search"`} {
		if strings.Contains(summaryBlock, leaked) {
			t.Fatalf("summary block leaked list markup %q: %s", leaked, summaryBlock)
		}
	}

	// The status counts are still the entry point into the filtered list, they
	// just live in a real container now instead of floating between sections.
	for _, chip := range []string{`class="count-chip overdue"`, `class="count-chip partial"`, `class="count-chip review"`} {
		if !strings.Contains(summaryBlock, chip) {
			t.Fatalf("status counts are missing %q: %s", chip, summaryBlock)
		}
	}

	listBlock := markupBetween(t, page,
		`<section class="dashboard-section" aria-labelledby="rent-status-title">`,
		`<section id="dunning-drawer"`)
	for _, expected := range []string{
		`id="rent-status-title"`,
		`class="list-filter"`,
		`id="dashboard-search"`,
		`id="dashboard-status"`,
		"清除筛选",
		"邮件催缴",
		`name="period" value="2026-09"`,
	} {
		if !strings.Contains(listBlock, expected) {
			t.Fatalf("list block is missing %q: %s", expected, listBlock)
		}
	}

	// Sorting is done by clicking a heading now, so the 排序 dropdown is gone and
	// the filter row must not grow it back.
	if strings.Contains(listBlock, `id="dashboard-sort"`) {
		t.Fatalf("list block still offers the 排序 dropdown: %s", listBlock)
	}
	if got := strings.Count(listBlock, `class="sort-link`); got < 4 {
		t.Fatalf("list block has %d sortable headings, want the tenant/due/amount/status columns: %s", got, listBlock)
	}
	if !strings.Contains(listBlock, `<input type="hidden" name="sort" value="due_desc">`) {
		t.Fatalf("filtering would silently drop the column sort: %s", listBlock)
	}

	// The page size sits with the pager: it is reachable from every page of the
	// list, and it carries the current filters so switching it does not reset them.
	pager := markupBetween(t, page, `<nav class="dashboard-pagination"`, `</nav>`)
	for _, expected := range []string{`id="dashboard-page-size"`, `name="period" value="2026-09"`, `name="search" value="Aoife"`, `name="status" value="unpaid"`, `name="sort" value="due_desc"`, "第 1 / 1 页"} {
		if !strings.Contains(pager, expected) {
			t.Fatalf("pager is missing %q: %s", expected, pager)
		}
	}
	filterForm := markupBetween(t, listBlock, `<form class="list-filter"`, `</form>`)
	if strings.Contains(filterForm, `id="dashboard-page-size"`) {
		t.Fatalf("page size is still in the filter row: %s", filterForm)
	}
}

// A heading click flips its own column and adopts the primary direction for any
// other, so the arrow always describes what the next click will do.
func TestSortLinkTogglesDirections(t *testing.T) {
	baseURL := func(sortValue string) string { return "/x?sort=" + sortValue }

	cases := []struct {
		name      string
		current   string
		primary   string
		secondary string
		wantURL   string
		wantArrow string
		wantOn    bool
	}{
		{"unsorted column previews its primary direction", "due_desc", "tenant_asc", "tenant_desc", "/x?sort=tenant_asc", "▲", false},
		{"primary click flips to the secondary direction", "tenant_asc", "tenant_asc", "tenant_desc", "/x?sort=tenant_desc", "▼", true},
		{"secondary click flips back to the primary direction", "tenant_desc", "tenant_asc", "tenant_desc", "/x?sort=tenant_asc", "▲", true},
		{"single-direction column offers no arrow", "status", "status", "", "/x?sort=status", "", true},
		{"inactive single-direction column stays plain", "tenant_asc", "status", "", "/x?sort=status", "", false},
	}
	for _, testCase := range cases {
		got := sortLinkFor(baseURL, testCase.current, testCase.primary, testCase.secondary)
		if got.URL != testCase.wantURL || got.Arrow != testCase.wantArrow || got.Active != testCase.wantOn {
			t.Fatalf("%s: got %+v, want url %q arrow %q active %v", testCase.name, got, testCase.wantURL, testCase.wantArrow, testCase.wantOn)
		}
	}
}

// The list defaults to its urgent-first order without a sort parameter, so the
// 状态 heading has to read as the active one on a plain /rent-dashboard visit.
func TestNormalisedSortFallsBackToTheDefaultColumn(t *testing.T) {
	if got := normalisedSort("", dashboardDefaultSort); got != "status" {
		t.Fatalf("empty sort resolved to %q, want the default column", got)
	}
	if got := normalisedSort("amount_asc", dashboardDefaultSort); got != "amount_asc" {
		t.Fatalf("explicit sort was rewritten to %q", got)
	}
}

// The E2E runner reads the summary cards with a regex requiring the label and
// the value to be adjacent inside one element, so the layout refactor must not
// insert whitespace or a wrapper between them.
func TestRentDashboardMetricsKeepTheE2EShape(t *testing.T) {
	var body strings.Builder
	if err := rentDashboardTemplate.Execute(&body, rentDashboardPageData{Period: "2026-09", PeriodLabel: "2026年9月"}); err != nil {
		t.Fatal(err)
	}
	page := body.String()
	for _, metric := range []string{"本月应收", "已收租金", "剩余未收", "待处理"} {
		if !strings.Contains(page, `<div class="label">`+metric+`</div><strong>`) {
			t.Fatalf("metric %q does not match the E2E extraction pattern", metric)
		}
	}
	for _, hiddenMetric := range []string{"待分配金额", "其他收入"} {
		if strings.Contains(page, hiddenMetric) {
			t.Fatalf("dashboard still renders hidden metric %q", hiddenMetric)
		}
	}
	if !strings.Contains(page, "当前显示") {
		t.Fatal("dashboard lost the filtered-count text the E2E runner asserts on")
	}
}

func TestRentDashboardManualBalanceActionOnlyAppearsForOutstandingRent(t *testing.T) {
	var body strings.Builder
	err := rentDashboardTemplate.Execute(&body, rentDashboardPageData{
		Period:       "2026-09",
		PeriodLabel:  "2026年9月",
		StatusFilter: "all",
		SortFilter:   dashboardDefaultSort,
		Page:         1,
		PageSize:     dashboardDefaultPageSize,
		TotalPages:   1,
		Rows: []rentDashboardRow{
			{ObligationID: 71, TenantID: 7, TenantName: "有差额租客", Period: "2026-09", ExpectedCents: 100000, PaidCents: 40000, Status: "partial", StatusLabel: "部分缴纳"},
			{ObligationID: 72, TenantID: 8, TenantName: "已缴清租客", Period: "2026-09", ExpectedCents: 100000, PaidCents: 100000, Status: "paid", StatusLabel: "已缴清"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	page := body.String()
	if got := strings.Count(page, `>一键平账</button>`); got != 1 {
		t.Fatalf("manual balance action count=%d want 1: %s", got, page)
	}
	for _, expected := range []string{
		`action="/rent-dashboard/settle"`,
		`name="obligation_id" value="71"`,
		`name="period" value="2026-09"`,
		`onkeydown="event.stopPropagation()"`,
		`onsubmit="return confirm('确认一键平账吗？')"`,
		`class="status-actions"`,
	} {
		if !strings.Contains(page, expected) {
			t.Fatalf("dashboard is missing manual balance markup %q: %s", expected, page)
		}
	}
	if strings.Contains(page, `name="obligation_id" value="72"`) {
		t.Fatalf("paid obligation unexpectedly renders manual balance action: %s", page)
	}
}
