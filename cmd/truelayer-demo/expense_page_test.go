package main

import (
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestFilterExpensePageRowsAppliesMonthInvoiceAndSearch(t *testing.T) {
	rows := []expenseRecord{
		{ID: "1", ExpenseDate: "2026-09-10", Description: "Heating repair", PropertyName: "Old County"},
		{ID: "2", ExpenseDate: "2026-09-12", Description: "Water charge", PropertyName: "Walkinstown", InvoiceURL: "https://example.test/invoice"},
		{ID: "3", ExpenseDate: "2026-08-12", Description: "Old repair", PropertyName: "Old County"},
	}
	period := time.Date(2026, time.September, 1, 0, 0, 0, 0, time.UTC)

	got := filterExpensePageRows(rows, period, "invoice_missing", "county")
	if len(got) != 1 || got[0].ID != "1" {
		t.Fatalf("filtered rows = %+v", got)
	}
	got = filterExpensePageRows(rows, period, "invoice_linked", "")
	if len(got) != 1 || got[0].ID != "2" {
		t.Fatalf("invoice-linked rows = %+v", got)
	}
}

func TestExpensePageUsesPrototypeListAndAddDrawer(t *testing.T) {
	var body strings.Builder
	err := expenseTemplate.Execute(&body, expensePageData{
		workspaceShell: workspaceShell{ExpenseCount: 1},
		Period:         "2026-09", StatusFilter: "all", Search: "repair", ShowForm: true,
		FilteredCount: 1, Today: "2026-09-19",
		Properties: []expensePropertyOption{{ID: 7, Name: "Old County"}},
		Rooms:      []expenseRoomOption{{ID: 3, PropertyID: 7, Label: "03"}},
		Rows:       []expenseRecord{{ID: "expense-1", PeriodDisplay: "2026-09", Description: "Heating repair", PropertyName: "Old County", RoomLabel: "03", Category: "维修", AmountDisplay: "€400.00"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	page := body.String()
	for _, marker := range []string{"费用支出", "搜索当前列表", "未绑定发票", "expense-table", "expense-mobile-list", "entity-drawer-backdrop", "保存支出"} {
		if !strings.Contains(page, marker) {
			t.Errorf("expense page missing %q", marker)
		}
	}
}

func TestExpensePageURLKeepsFiltersAndPreservesLegacyDefault(t *testing.T) {
	if got := expensePageURL(url.Values{}, "expense_added", "", false); got != "/expenses?message=expense_added" {
		t.Fatalf("legacy redirect=%q", got)
	}
	values := url.Values{"period": {"2026-09"}, "status": {"invoice_missing"}, "search": {"old county"}}
	got := expensePageURL(values, "", "invalid_expense", true)
	parsed, err := url.Parse(got)
	if err != nil {
		t.Fatal(err)
	}
	query := parsed.Query()
	if parsed.Path != "/expenses" || query.Get("period") != "2026-09" || query.Get("status") != "invoice_missing" || query.Get("search") != "old county" || query.Get("add") != "1" || query.Get("error") != "invalid_expense" {
		t.Fatalf("expense redirect URL=%q", got)
	}
}
