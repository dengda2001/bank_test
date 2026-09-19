package main

import (
	"net/url"
	"strings"
	"testing"
)

func TestCashReceiptPageRendersPrototypeListCardsAndDrawer(t *testing.T) {
	var body strings.Builder
	err := cashReceiptPageTemplate.Execute(&body, cashReceiptPageData{
		Period: "2026-09", StatusFilter: "all", ShowForm: true,
		Form: cashReceiptFormData{Period: "2026-09", Currency: "EUR", ReceivedAt: "2026-09-19", IdempotencyKey: "cash-test", Tenants: []tenant{{ID: 3, Name: "Tenant A"}}},
		Rows: []cashReceiptPageRow{{ID: 8, ReceiptNumber: "CASH-8", TenantID: 3, TenantName: "Tenant A", RoomLabel: "03", Amount: "€400.00", Period: "2026-09", ReceivedAt: "2026-09-19", Status: cashReceiptStatusConfirmed, StatusLabel: "已确认"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	page := body.String()
	for _, marker := range []string{"现金收款", "搜索当前列表", "cash-receipt-table", "cash-receipt-mobile-list", "entity-drawer-backdrop", "return_to", "预览入账"} {
		if !strings.Contains(page, marker) {
			t.Errorf("cash receipt page missing %q", marker)
		}
	}
}

func TestCashReceiptListURLPreservesListContext(t *testing.T) {
	values := url.Values{
		"list_period": {"2026-09"}, "list_status": {cashReceiptStatusVoided}, "list_search": {"Tenant A"},
	}
	got := cashReceiptListReturnURL(values, "", "cash_receipt_saved", false, "")
	parsed, err := url.Parse(got)
	if err != nil {
		t.Fatal(err)
	}
	query := parsed.Query()
	if query.Get("period") != "2026-09" || query.Get("status") != cashReceiptStatusVoided || query.Get("search") != "Tenant A" || query.Get("message") != "cash_receipt_saved" {
		t.Fatalf("cash receipt list redirect=%q", got)
	}
}
