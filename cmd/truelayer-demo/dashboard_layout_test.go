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

// TestRentDashboardSeparatesPeriodBarSummaryAndList was removed with the legacy
// /rent-dashboard fallback template. It asserted that template's own three-block
// structure (month-only period bar, summary block, list block) and its
// dashboard-search / dashboard-status / dashboard-sort / dashboard-page-size
// control IDs. The live /bills and room-workspace pages have their own layouts
// with their own tests; the legacy structure has no surviving carrier.

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
