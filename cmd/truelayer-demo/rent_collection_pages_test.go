package main

import (
	"strings"
	"testing"
)

func TestBillsPageRendersMonthlyResponsibilityList(t *testing.T) {
	page, err := executeTemplate(billsPageTemplate, rentDashboardPageData{
		Period: "2026-09", PeriodLabel: "2026年9月", ExpectedTotal: "EUR 1,000.00",
		PaidTotal: "EUR 400.00", BalanceTotal: "EUR 600.00", Page: 1, PageSize: 12, TotalPages: 1,
		Rows: []rentDashboardRow{{TenantID: 7, TenantName: "Aoife Murphy", RoomLabel: "A-01", Period: "2026-09", ExpectedAmount: "EUR 1,000.00", PaidAmount: "EUR 400.00", BalanceAmount: "EUR 600.00", Status: "partial", StatusLabel: "部分缴纳", ObligationID: 11, ExpectedCents: 100000, PaidCents: 40000}},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"应收账单", "账单列表", "Aoife Murphy", "EUR 600.00", `action="/bills/settle"`} {
		if !strings.Contains(page, expected) {
			t.Fatalf("bills page missing %q", expected)
		}
	}
	if strings.Contains(page, "租客缴费情况") {
		t.Fatal("bills page should not render the legacy dashboard heading")
	}
}

func TestDunningPageRendersFocusedQueueWithoutSavingConfigOnGet(t *testing.T) {
	page, err := executeTemplate(dunningPageTemplate, rentDashboardPageData{
		PeriodLabel: "2026年9月", UnpaidCount: 1, OverdueCount: 1,
		Dunning: dunningDrawerData{
			Period: "2026-09", Enabled: true, Open: true, RequestKey: "request-1",
			Candidates: []dunningCandidate{{ObligationID: 11, TenantName: "Aoife Murphy", RoomLabel: "A-01", DueDate: "2026-09-05", BalanceAmount: "EUR 600.00", Status: "overdue", StatusLabel: "逾期", Email: "aoife@example.com", EmailValid: true, Selectable: true, Selected: true}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"催收任务", "待核对责任", "Aoife Murphy", `action="/dunning/preview"`, `action="/dunning/config"`} {
		if !strings.Contains(page, expected) {
			t.Fatalf("dunning page missing %q", expected)
		}
	}
	if strings.Contains(page, "月度收租工作台") {
		t.Fatal("dunning page rendered the unrelated monthly dashboard")
	}
}
