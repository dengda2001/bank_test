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

// This used to render the legacy dashboard template. Its live carrier for the
// same parsed-filter and pagination contract is /bills, which shares
// rentDashboardFiltersFromQuery and rentDashboardPageData. The legacy-only parts
// of the old assertion -- the bank sync-status notices and the tenant alias --
// have no /bills carrier; bank sync coverage renders on /bank instead
// (bankPageTemplate), and the legacy dashboard was the only page that showed it
// inline with the rent list.
func TestBillsPageRendersFiltersMetricsAndPagination(t *testing.T) {
	page, err := executeTemplate(billsPageTemplate, rentDashboardPageData{
		Period:        "2026-09",
		PeriodLabel:   "2026年9月",
		SearchFilter:  "Aoife",
		StatusFilter:  "unpaid",
		SortFilter:    "due_desc",
		Page:          1,
		PageSize:      12,
		FilteredCount: 12,
		TotalRows:     14,
		TotalPages:    2,
		Rows:          []rentDashboardRow{{TenantID: 7, TenantName: "Aoife Murphy", RoomLabel: "A-01", Period: "2026-09", ExpectedAmount: "EUR 1,000.00", PaidAmount: "EUR 400.00", BalanceAmount: "EUR 600.00", Status: "partial", StatusLabel: "部分缴纳"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		`name="search"`,
		`value="Aoife"`,
		`value="unpaid" selected`,
		`/tenants/7?from_month=2026-09&amp;to_month=2026-09`,
		"第 1 / 2 页",
		`page=2`,
	} {
		if !strings.Contains(page, expected) {
			t.Fatalf("bills page missing %q: %s", expected, page)
		}
	}
	for _, hiddenMetric := range []string{"待分配金额", "其他收入"} {
		if strings.Contains(page, hiddenMetric) {
			t.Fatalf("bills page still renders hidden metric %q: %s", hiddenMetric, page)
		}
	}
}

// /bills distinguishes "this month has no bills" from "the filters matched
// nothing" with two different empty states; the removed legacy dashboard had the
// same two-state contract, which is what this test originally pinned.
func TestBillsPageDistinguishesNoMatchesFromNoBills(t *testing.T) {
	for name, data := range map[string]rentDashboardPageData{
		"no matches": {TotalRows: 2, FilteredCount: 0},
		"no bills":   {TotalRows: 0, FilteredCount: 0},
	} {
		t.Run(name, func(t *testing.T) {
			page, err := executeTemplate(billsPageTemplate, data)
			if err != nil {
				t.Fatal(err)
			}
			want := "本月暂无应收账单"
			if name == "no matches" {
				want = "没有符合当前筛选条件"
			}
			if !strings.Contains(page, want) {
				t.Fatalf("bills page missing %q: %s", want, page)
			}
		})
	}
}

// With a session but no database there is no dashboard read model left: the
// legacy JSON-backed fallback template was removed, so the handler now fails
// loudly instead of silently degrading to demo data. Mirrors the analogous
// dunning guard (TestDunningPOSTRejectsLegacySessionWithoutDatabase).
func TestRentDashboardWithoutDatabaseReturnsServiceUnavailable(t *testing.T) {
	dir := t.TempDir()
	a := testApp()
	a.cfg.TenantFile = filepath.Join(dir, "tenants.json")
	a.cfg.ExpenseFile = filepath.Join(dir, "expenses.json")
	a.cfg.LogFile = filepath.Join(dir, "bank-data.jsonl")
	req := httptest.NewRequest(http.MethodGet, "/rent-dashboard?period=2026-09", nil)
	req.AddCookie(sessionCookie(a.cfg, time.Now().Add(sessionTTL)))
	rec := httptest.NewRecorder()
	a.handleRentDashboard(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d want %d body=%s", rec.Code, http.StatusServiceUnavailable, rec.Body.String())
	}
}

// The invalid-input contract survives the deletion, but on a different carrier:
// renderRentDashboard still turns a rejected filter/period into rentDashboardPageData.Error
// and /bills (billsPageTemplate) is what renders it now. The removed legacy
// dashboard used to be the only page pinned for this, and it rendered a Chinese
// notice ("筛选条件无效") instead of the handler's code. Both the code and the
// notice element are asserted here so the assertion cannot be satisfied by some
// unrelated error markup appearing elsewhere on the page.
func TestBillsPageSurfacesInvalidFilterAndPeriodErrors(t *testing.T) {
	for code, data := range map[string]rentDashboardPageData{
		"invalid_dashboard_filter": {Error: "invalid_dashboard_filter"},
		"invalid_period":           {Error: "invalid_period"},
	} {
		t.Run(code, func(t *testing.T) {
			page, err := executeTemplate(billsPageTemplate, data)
			if err != nil {
				t.Fatal(err)
			}
			// The error banner deliberately carries no data-toast marker, so this
			// stays a plain notice and is not promoted into the toast.
			if !strings.Contains(page, `<div class="notice error">`+code+`</div>`) {
				t.Fatalf("bills page does not surface %q as an error notice: %s", code, page)
			}
		})
	}
}
