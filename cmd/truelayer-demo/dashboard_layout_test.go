package main

import (
	"net/url"
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

// TestRentDashboardSeparatesPeriodBarSummaryAndList was removed with the legacy
// /rent-dashboard fallback template. It asserted that template's own three-block
// structure (month-only period bar, summary block, list block) and its
// dashboard-search / dashboard-status / dashboard-sort / dashboard-page-size
// control IDs. The live /bills and room-workspace pages have their own layouts
// with their own tests; the legacy structure has no surviving carrier.

// A heading click flips its own column and adopts the primary direction for any
// other. The arrow is an indicator of the current sort rather than a preview of
// the next click, so a reader can tell the active order without activating it.
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
		{"inactive column has no current-order arrow", "due_desc", "tenant_asc", "tenant_desc", "/x?sort=tenant_asc", "", false},
		{"ascending active column points at its current order", "tenant_asc", "tenant_asc", "tenant_desc", "/x?sort=tenant_desc", "▲", true},
		{"descending active column points at its current order", "tenant_desc", "tenant_asc", "tenant_desc", "/x?sort=tenant_asc", "▼", true},
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

func TestTableSortHeadingExposesTheActiveDirection(t *testing.T) {
	page := newWorkspacePageTemplate("sort-heading-test", nil, `<table><thead><tr>{{template "table-sort-heading" (tableSortHeading "租客" .Link)}}</tr></thead></table>`)
	markup, err := executeTemplate(page, struct{ Link tableSortLink }{
		Link: tableSortLink{URL: "/x?sort=tenant_asc", Active: true, Arrow: "▲"},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		`aria-sort="ascending"`,
		`class="table-sort-link is-active"`,
		`href="/x?sort=tenant_asc"`,
		`<span class="table-sort-arrow" aria-hidden="true">▲</span>`,
	} {
		if !strings.Contains(markup, expected) {
			t.Fatalf("sortable heading is missing %q: %s", expected, markup)
		}
	}
}

func TestTableSortHeadingMarksOneWaySortAsOther(t *testing.T) {
	page := newWorkspacePageTemplate("one-way-sort-heading-test", nil, `<table><thead><tr>{{template "table-sort-heading" (tableSortHeading "状态" .Link)}}</tr></thead></table>`)
	markup, err := executeTemplate(page, struct{ Link tableSortLink }{
		Link: tableSortLink{URL: "/x?sort=status", Active: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(markup, `aria-sort="other"`) || strings.Contains(markup, `table-sort-arrow`) {
		t.Fatalf("one-way sort heading has an invalid accessible direction: %s", markup)
	}
}

func TestTransactionSortLinksResetPaginationAndRetainFilters(t *testing.T) {
	links := transactionSortLinks(url.Values{"payer": {"Aoife"}, "page": {"3"}, "page_size": {"24"}}, "status_asc")
	status := links["status"]
	if !status.Active || status.Arrow != "▲" || status.URL != "/transactions?page=1&page_size=24&payer=Aoife&sort=status_desc" {
		t.Fatalf("unexpected status sort link: %+v", status)
	}
	object := links["object"]
	if object.Active || object.Arrow != "" || object.URL != "/transactions?page=1&page_size=24&payer=Aoife&sort=object_asc" {
		t.Fatalf("unexpected object sort link: %+v", object)
	}
}

func TestWorkspaceTenantTableRendersSortHeadings(t *testing.T) {
	page, err := executeTemplate(rentWorkspaceTemplate, rentWorkspacePageData{
		View:   rentWorkspaceViewTenants,
		Period: "2026-09",
		TenantRows: []rentWorkspaceTenantRow{
			{TenantName: "Aoife Murphy", ExpectedAmount: "EUR 800.00", PaidAmount: "EUR 0.00", BalanceAmount: "EUR 800.00"},
		},
		SortLinks: map[string]tableSortLink{
			"tenant": {URL: "/rent-dashboard?sort=name_asc", Active: true, Arrow: "▲"},
			"room":   {URL: "/rent-dashboard?sort=room_asc"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		`aria-sort="ascending"`,
		`href="/rent-dashboard?sort=name_asc"`,
		`href="/rent-dashboard?sort=room_asc"`,
		`>租客 <span class="table-sort-arrow" aria-hidden="true">▲</span>`,
	} {
		if !strings.Contains(page, expected) {
			t.Fatalf("workspace table is missing sortable header %q: %s", expected, page)
		}
	}
}

// The room workspace uses selected-month and outstanding-responsibility labels
// in its summary cards; keep each label adjacent to its value for stable page
// extraction.
func TestRentWorkspaceMetricsKeepTheE2EShape(t *testing.T) {
	page, err := executeTemplate(rentWorkspaceTemplate, rentWorkspacePageData{
		Period:      "2026-09",
		PeriodLabel: "2026年9月",
		TotalRows:   1,
		Summary:     rentWorkspaceSummary{ExpectedAmount: "EUR 950.00", PaidAmount: "EUR 950.00", BalanceAmount: "EUR 0.00"},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, metric := range []string{"所选月份应收", "已收租金", "未结清责任"} {
		if !strings.Contains(page, `<div class="label">`+metric+`</div><strong>`) {
			t.Fatalf("metric %q does not match the E2E extraction pattern", metric)
		}
	}
	for _, hiddenMetric := range []string{"待分配金额", "其他收入"} {
		if strings.Contains(page, hiddenMetric) {
			t.Fatalf("workspace still renders hidden metric %q", hiddenMetric)
		}
	}
	if !strings.Contains(page, "当前显示") {
		t.Fatal("workspace lost the filtered-count text the E2E runner asserts on")
	}
}

// 警示色和正向色是"状态真的成立"的信号，不是装饰：一分钱没收就不该绿，
// 余额为 0 就不该红，未来月份那张卡自己写着"未到期，不计入催收"，顶一张
// 红卡等于自相矛盾。这四条把三类卡片的有色/无色组合钉住。
func TestWorkspaceSummaryColoursOnlyReflectRealState(t *testing.T) {
	for _, test := range []struct {
		name        string
		summary     rentWorkspaceSummary
		future      bool
		wantSuccess bool
		wantWarning bool
	}{
		{"当月收齐", rentWorkspaceSummary{PaidCents: 125000, BalanceCents: 0}, false, true, false},
		{"当月还有未结清", rentWorkspaceSummary{PaidCents: 0, BalanceCents: 80000}, false, false, true},
		{"当月既没进账也没应收", rentWorkspaceSummary{}, false, false, false},
		{"未来月份的预计缺口", rentWorkspaceSummary{BalanceCents: 80000}, true, false, false},
	} {
		page, err := executeTemplate(rentWorkspaceTemplate, rentWorkspacePageData{
			Period: "2026-09", PeriodLabel: "2026年9月", TotalRows: 1,
			IsFuturePeriod: test.future, Summary: test.summary,
		})
		if err != nil {
			t.Fatalf("%s: %v", test.name, err)
		}
		block := markupBetween(t, page, `<section class="workspace-summary"`, `</section>`)
		if got := strings.Contains(block, "metric-success"); got != test.wantSuccess {
			t.Errorf("%s: metric-success=%v want %v", test.name, got, test.wantSuccess)
		}
		if got := strings.Contains(block, "metric-warning"); got != test.wantWarning {
			t.Errorf("%s: metric-warning=%v want %v", test.name, got, test.wantWarning)
		}
	}
}

// The settle action belongs to a bill with an outstanding balance; a paid bill
// offers only the plain detail link. This used to be asserted on the removed
// legacy dashboard (and on its /rent-dashboard/settle form). /bills is the live
// carrier and shares the same `gt .ExpectedCents .PaidCents` condition, so the
// contract moves there unchanged.
func TestBillsManualBalanceActionOnlyAppearsForOutstandingRent(t *testing.T) {
	page, err := executeTemplate(billsPageTemplate, rentDashboardPageData{
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
	for name, block := range map[string]string{
		"mobile cards":  markupBetween(t, page, `<div class="collection-mobile-list">`, `<nav class="collection-pager"`),
		"desktop table": markupBetween(t, page, `<div class="table-wrap collection-table-wrap">`, `</table></div>`),
	} {
		if got := strings.Count(block, `>一键平账</summary>`); got != 1 {
			t.Fatalf("manual balance action count in %s=%d want 1: %s", name, got, block)
		}
	}
	for _, expected := range []string{
		`action="/rent-dashboard/settle"`,
		`name="return_to"`,
		`name="obligation_id" value="71"`,
		`name="period" value="2026-09"`,
		`data-confirm="true"`,
		`placeholder="平账原因"`,
	} {
		if !strings.Contains(page, expected) {
			t.Fatalf("bills page is missing manual balance markup %q: %s", expected, page)
		}
	}
	if strings.Contains(page, `name="obligation_id" value="72"`) {
		t.Fatalf("paid obligation unexpectedly renders manual balance action: %s", page)
	}
}
