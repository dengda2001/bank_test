package main

import (
	"net/url"
	"testing"
)

func TestSortPropertyPageRowsUsesNumericColumnsAndStableTies(t *testing.T) {
	rows := sortPropertyPageRows([]propertyPageRow{
		{ID: 2, Name: "Bravo", ExpectedCents: 120000},
		{ID: 1, Name: "Alpha", ExpectedCents: 120000},
		{ID: 3, Name: "Charlie", ExpectedCents: 80000},
	}, "expected_desc")
	if rows[0].ID != 1 || rows[1].ID != 2 || rows[2].ID != 3 {
		t.Fatalf("expected descending sort with ID tie-breaker, got %#v", rows)
	}
}

func TestSortRoomPageRowsSortsTenantCountAndRoomName(t *testing.T) {
	rows := sortRoomPageRows([]roomPageRow{
		{ID: 3, RoomLabel: "C-03", TenantNames: []string{"A"}},
		{ID: 2, RoomLabel: "B-02", TenantNames: []string{"A", "B"}},
		{ID: 1, RoomLabel: "A-01", TenantNames: []string{"A", "B"}},
	}, "tenants_desc")
	if rows[0].ID != 1 || rows[1].ID != 2 || rows[2].ID != 3 {
		t.Fatalf("expected tenant count descending with room-name tie-breaker, got %#v", rows)
	}
}

func TestSortTenantRecordsUsesCreationTimeWithoutChangingNameTies(t *testing.T) {
	rows := sortTenantRecords([]tenantRecord{
		{ID: "2", Name: "Alex", CreatedAt: "2026-09-02T00:00:00Z"},
		{ID: "1", Name: "Alex", CreatedAt: "2026-09-02T00:00:00Z"},
		{ID: "3", Name: "Bea", CreatedAt: "2026-09-01T00:00:00Z"},
	}, "created_desc")
	if rows[0].ID != "1" || rows[1].ID != "2" || rows[2].ID != "3" {
		t.Fatalf("expected newest-first tenant ordering with stable ID ties, got %#v", rows)
	}
}

func TestSortCashReceiptRowsUsesReceivedDateAndAmount(t *testing.T) {
	rows := sortCashReceiptPageRows([]cashReceiptPageRow{
		{ID: 2, ReceivedAt: "2026-09-02", AmountCents: 5000},
		{ID: 1, ReceivedAt: "2026-09-02", AmountCents: 5000},
		{ID: 3, ReceivedAt: "2026-09-01", AmountCents: 12000},
	}, "received_desc")
	if rows[0].ID != 1 || rows[1].ID != 2 || rows[2].ID != 3 {
		t.Fatalf("expected date descending and ID-stable ties, got %#v", rows)
	}
}

func TestSortExpenseRecordsUsesNumericAmount(t *testing.T) {
	rows := sortExpenseRecords([]expenseRecord{
		{ID: "2", Description: "Utilities", Amount: 80},
		{ID: "1", Description: "Utilities", Amount: 80},
		{ID: "3", Description: "Repairs", Amount: 120},
	}, "amount_desc")
	if rows[0].ID != "3" || rows[1].ID != "1" || rows[2].ID != "2" {
		t.Fatalf("expected numeric expense amount sort with stable description ties, got %#v", rows)
	}
}

func TestTenantDetailHistoryURLRetainsValidReturnSort(t *testing.T) {
	history := tenantBillingHistoryPage{FromPeriod: "2026-08", ToPeriod: "2026-09", PageSize: 25}
	parsed, err := url.Parse(tenantDetailHistoryURL(42, history, 3, "alex", "created_desc"))
	if err != nil {
		t.Fatal(err)
	}
	query := parsed.Query()
	if parsed.Path != "/tenants/42" || query.Get("page") != "3" || query.Get("search") != "alex" || query.Get("sort") != "created_desc" {
		t.Fatalf("history page URL did not retain list context: %s", parsed.String())
	}
	invalid, err := url.Parse(tenantDetailHistoryURL(42, history, 3, "", "not-a-sort"))
	if err != nil {
		t.Fatal(err)
	}
	if invalid.Query().Has("sort") {
		t.Fatalf("history page URL retained an invalid sort: %s", invalid.String())
	}
}
