package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestRentDashboardFiltersFromQuery(t *testing.T) {
	filters, err := rentDashboardFiltersFromQuery(url.Values{
		"search":    []string{" Aoife "},
		"status":    []string{"unpaid"},
		"sort":      []string{"due_desc"},
		"page":      []string{"2"},
		"page_size": []string{"5"},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := rentDashboardFilters{Search: "Aoife", Status: "unpaid", Sort: "due_desc", Page: 2, PageSize: 5}
	if !reflect.DeepEqual(filters, want) {
		t.Fatalf("filters=%+v want %+v", filters, want)
	}

	for name, query := range map[string]url.Values{
		"invalid status": {"status": []string{"unknown"}},
		"invalid page":   {"page": []string{"0"}},
		"invalid size":   {"page_size": []string{"51"}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := rentDashboardFiltersFromQuery(query); err == nil {
				t.Fatal("expected invalid dashboard filter error")
			}
		})
	}
}

func TestFilterAndSortRentDashboardRows(t *testing.T) {
	rows := []rentDashboardRow{
		{TenantID: 1, TenantName: "Zoe", TenantAlias: "Z", RoomLabel: "A-01", Status: "paid", ExpectedCents: 1000, PaidCents: 1000, DueDateValue: time.Date(2026, 9, 5, 0, 0, 0, 0, time.UTC)},
		{TenantID: 2, TenantName: "Aoife", TenantAlias: "A", RoomLabel: "B-02", Status: "overdue", ExpectedCents: 1200, PaidCents: 0, DueDateValue: time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC)},
		{TenantID: 3, TenantName: "Mia", TenantAlias: "M", RoomLabel: "C-03", Status: "partial", ExpectedCents: 900, PaidCents: 400, DueDateValue: time.Date(2026, 9, 3, 0, 0, 0, 0, time.UTC)},
	}

	filtered := filterAndSortRentDashboardRows(rows, rentDashboardFilters{Search: "i", Status: "unpaid", Sort: "tenant_asc", Page: 1, PageSize: 12})
	if got := []uint64{filtered[0].TenantID, filtered[1].TenantID}; !reflect.DeepEqual(got, []uint64{2, 3}) {
		t.Fatalf("filtered tenant ids=%v", got)
	}

	amountRows := filterAndSortRentDashboardRows(rows, rentDashboardFilters{Status: "all", Sort: "amount_desc", Page: 1, PageSize: 12})
	if got := []uint64{amountRows[0].TenantID, amountRows[1].TenantID, amountRows[2].TenantID}; !reflect.DeepEqual(got, []uint64{2, 1, 3}) {
		t.Fatalf("amount-sorted tenant ids=%v", got)
	}
}

func TestPaginateRentDashboardRows(t *testing.T) {
	rows := []rentDashboardRow{{TenantID: 1}, {TenantID: 2}, {TenantID: 3}, {TenantID: 4}, {TenantID: 5}}
	page, totalPages := paginateRentDashboardRows(rows, 2, 2)
	if totalPages != 3 || len(page) != 2 || page[0].TenantID != 3 || page[1].TenantID != 4 {
		t.Fatalf("page=%+v totalPages=%d", page, totalPages)
	}
	page, totalPages = paginateRentDashboardRows(rows, 4, 2)
	if totalPages != 3 || len(page) != 0 {
		t.Fatalf("out-of-range page=%+v totalPages=%d", page, totalPages)
	}
}

func TestSummarizeRentDashboardBankMetrics(t *testing.T) {
	transactions := []paymentTransaction{
		{ID: 1, AmountCents: 1000, Currency: "EUR", MatchStatus: "partial"},
		{ID: 2, AmountCents: 500, Currency: "EUR", MatchStatus: "unmatched"},
		{ID: 3, AmountCents: 200, Currency: "GBP", MatchStatus: "matched"},
	}
	allocations := []paymentAllocation{
		{PaymentTransactionID: 1, AmountCents: 700, AllocationKind: allocationKindRent, Status: allocationStatusConfirmed},
		{PaymentTransactionID: 2, AmountCents: 300, AllocationKind: allocationKindOther, Status: allocationStatusConfirmed},
		{PaymentTransactionID: 2, AmountCents: 100, AllocationKind: allocationKindOther, Status: allocationStatusConfirmed},
		{PaymentTransactionID: 3, AmountCents: 200, AllocationKind: allocationKindOther, Status: allocationStatusConfirmed},
	}

	metrics := summarizeRentDashboardBankMetrics(transactions, allocations)
	if metrics.PendingCount != 2 || metrics.PendingCents != 400 {
		t.Fatalf("pending metrics=%+v", metrics)
	}
	if metrics.OtherIncomeCount != 1 || metrics.OtherIncomeCents != 400 {
		t.Fatalf("other income metrics=%+v", metrics)
	}
}

func TestRentDashboardURLPreservesFilters(t *testing.T) {
	got := rentDashboardURL("2026-09", "Aoife Murphy", "unpaid", "due_desc", 2, 5)
	parsed, err := url.Parse(got)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Path != "/rent-dashboard" || parsed.Query().Get("period") != "2026-09" || parsed.Query().Get("search") != "Aoife Murphy" || parsed.Query().Get("status") != "unpaid" || parsed.Query().Get("sort") != "due_desc" || parsed.Query().Get("page") != "2" || parsed.Query().Get("page_size") != "5" {
		t.Fatalf("URL=%q", got)
	}
}

func TestRentDashboardTemplateRendersFiltersMetricsAndPagination(t *testing.T) {
	var body strings.Builder
	err := rentDashboardTemplate.Execute(&body, rentDashboardPageData{
		Period:                     "2026-09",
		PeriodLabel:                "2026年9月",
		SearchFilter:               "Aoife",
		StatusFilter:               "unpaid",
		SortFilter:                 "due_desc",
		Page:                       1,
		PageSize:                   12,
		FilteredCount:              12,
		TotalRows:                  14,
		TotalPages:                 2,
		PendingTotal:               "EUR 300.00",
		OtherIncomeTotal:           "EUR 400.00",
		OtherIncomeCount:           1,
		PendingCount:               2,
		SyncStatus:                 bankSyncStatusPartial,
		SyncCoverage:               "部分成功 · 覆盖 2026-09-01 至 2026-09-16",
		LastSuccessfulSyncCoverage: "同步成功 · 覆盖 2026-06-01 至 2026-09-01",
		Rows:                       []rentDashboardRow{{TenantID: 7, TenantName: "Aoife Murphy", TenantAlias: "Aoife A", Period: "2026-09", ExpectedAmount: "EUR 1,000.00", PaidAmount: "EUR 400.00", BalanceAmount: "EUR 600.00", Status: "partial", StatusLabel: "部分缴纳"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	page := body.String()
	for _, expected := range []string{
		`name="search"`,
		`value="Aoife"`,
		`value="unpaid" selected`,
		"部分成功",
		"最近一次成功同步",
		"别名：Aoife A",
		`/tenants/7?from_month=2026-09&amp;to_month=2026-09`,
		"第 1 / 2 页",
		`page=2`,
	} {
		if !strings.Contains(page, expected) {
			t.Fatalf("dashboard missing %q: %s", expected, page)
		}
	}
	for _, hiddenMetric := range []string{"待分配金额", "其他收入"} {
		if strings.Contains(page, hiddenMetric) {
			t.Fatalf("dashboard still renders hidden metric %q: %s", hiddenMetric, page)
		}
	}
}

func TestRentDashboardTemplateDistinguishesNoMatchesFromNoBills(t *testing.T) {
	for name, data := range map[string]rentDashboardPageData{
		"no matches": {TotalRows: 2, FilteredCount: 0},
		"no bills":   {TotalRows: 0, FilteredCount: 0},
	} {
		t.Run(name, func(t *testing.T) {
			var body strings.Builder
			if err := rentDashboardTemplate.Execute(&body, data); err != nil {
				t.Fatal(err)
			}
			want := "本月还没有租金账单"
			if name == "no matches" {
				want = "没有符合当前筛选条件"
			}
			if !strings.Contains(body.String(), want) {
				t.Fatalf("dashboard missing %q: %s", want, body.String())
			}
		})
	}
}

func TestRentDashboardHTTPRendersSafeFallbackAndFilterError(t *testing.T) {
	dir := t.TempDir()
	a := testApp()
	a.cfg.TenantFile = filepath.Join(dir, "tenants.json")
	a.cfg.ExpenseFile = filepath.Join(dir, "expenses.json")
	a.cfg.LogFile = filepath.Join(dir, "bank-data.jsonl")
	req := httptest.NewRequest(http.MethodGet, "/rent-dashboard?period=2026-09&status=not-valid", nil)
	req.AddCookie(sessionCookie(a.cfg, time.Now().Add(sessionTTL)))
	rec := httptest.NewRecorder()
	a.handleRentDashboard(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
	for _, expected := range []string{"筛选条件无效", "尚未完成银行同步", `id="dashboard-search"`, "本月还没有租金账单"} {
		if !strings.Contains(rec.Body.String(), expected) {
			t.Fatalf("fallback dashboard missing %q: %s", expected, rec.Body.String())
		}
	}
}
