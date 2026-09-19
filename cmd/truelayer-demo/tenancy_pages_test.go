package main

import (
	"net/url"
	"strings"
	"testing"
)

func TestParseTenancyArrangementKeepsExplicitTenantShares(t *testing.T) {
	values := url.Values{
		"room_id":           {"3"},
		"effective_month":   {"2026-09"},
		"start_date":        {"2026-09-03"},
		"contract_date":     {"2026-08-30"},
		"move_in_date":      {"2025-09-01"},
		"monthly_rent":      {"1250.00"},
		"due_day":           {"10"},
		"tenant_ids":        {"11", "22"},
		"responsibility_11": {"625.00"},
		"responsibility_22": {"625.00"},
	}

	got, err := parseTenancyArrangement(values)
	if err != nil {
		t.Fatal(err)
	}
	if got.RoomID != 3 || got.MonthlyRentCents != 125000 || got.DueDay != 10 {
		t.Fatalf("unexpected arrangement: %+v", got)
	}
	if got.ContractDate == nil || got.ContractDate.Format(dateLayout) != "2026-08-30" || got.MoveInDate == nil || got.MoveInDate.Format(dateLayout) != "2025-09-01" {
		t.Fatalf("tenancy dates = contract %v, move-in %v", got.ContractDate, got.MoveInDate)
	}
	if len(got.TenantIDs) != 2 || got.TenantIDs[0] != 11 || got.TenantIDs[1] != 22 {
		t.Fatalf("tenant ids = %v", got.TenantIDs)
	}
	if len(got.Responsibilities) != 2 || got.Responsibilities[0].AmountCents != 62500 || got.Responsibilities[1].AmountCents != 62500 {
		t.Fatalf("responsibilities = %+v", got.Responsibilities)
	}
}

func TestParseTenancyArrangementAllowsServerEvenSplit(t *testing.T) {
	values := url.Values{
		"room_id":         {"3"},
		"effective_month": {"2026-09"},
		"start_date":      {"2026-09-03"},
		"monthly_rent":    {"1250.00"},
		"tenant_ids":      {"11", "22"},
	}

	got, err := parseTenancyArrangement(values)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.TenantIDs) != 2 || len(got.Responsibilities) != 0 {
		t.Fatalf("expected two tenants and no explicit shares: %+v", got)
	}
}

func TestParseTenancyArrangementRejectsPartialTenantShares(t *testing.T) {
	values := url.Values{
		"room_id":           {"3"},
		"effective_month":   {"2026-09"},
		"start_date":        {"2026-09-03"},
		"monthly_rent":      {"1250.00"},
		"tenant_ids":        {"11", "22"},
		"responsibility_11": {"625.00"},
	}

	if _, err := parseTenancyArrangement(values); err == nil || !strings.Contains(err.Error(), "every selected tenant") {
		t.Fatalf("partial shares err = %v", err)
	}
}

func TestTenancyPageMatchesDesktopAndMobilePrototypeStates(t *testing.T) {
	var body strings.Builder
	err := tenancyPageTemplate.Execute(&body, tenancyPageData{
		Period: "2026-09", StatusFilter: "all", ShowCreate: true,
		Rooms:   []tenancyRoomOption{{ID: 3, Label: "78 Old County · 03"}},
		Tenants: []tenancyTenantOption{{ID: 11, Name: "Tenant A"}},
		Form:    tenancyFormData{Period: "2026-09", StartDate: "2026-09-03", DueDay: 1},
		Rows: []tenancyPageRow{{
			ID: 7, RecordLabel: "LEASE-000007", RoomID: 3, PropertyName: "Old County", RoomLabel: "03",
			StartDate: "2026-09-03", MonthlyRent: "€1,250.00", DueDay: 1, Status: "active", StatusLabel: "执行中",
			Parties: []tenancyPartyView{{TenantID: 11, TenantName: "Tenant A", Responsibility: "€1,250.00"}},
		}},
		TableRows: []tenancyPageRow{{
			ID: 7, RecordLabel: "LEASE-000007", RoomID: 3, PropertyName: "Old County", RoomLabel: "03",
			StartDate: "2026-09-03", MonthlyRent: "€1,250.00", Responsibility: "€1,250.00", TenantID: 11,
			TenantName: "Tenant A", Status: "active", StatusLabel: "执行中",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	page := body.String()
	for _, marker := range []string{
		"租约管理", "搜索当前列表", "查看月份", "tenancy-table", "tenancy-mobile-list", "合同日期", "入住日期",
		"新建租约", "name=\"effective_month\"", "name=\"tenant_ids\"", "name=\"responsibility_11\"",
		"查看房间与租客", "执行中",
	} {
		if !strings.Contains(page, marker) {
			t.Errorf("tenancy page missing %q", marker)
		}
	}
}

func TestExpandTenancyTableRowsCreatesOneRowPerTenantResponsibility(t *testing.T) {
	rows := []tenancyPageRow{{
		ID: 7, RecordLabel: "LEASE-000007", RoomID: 3, MonthlyRent: "€1,250.00",
		Parties: []tenancyPartyView{
			{TenantID: 11, TenantName: "Tenant A", Responsibility: "€625.00"},
			{TenantID: 12, TenantName: "Tenant B", Responsibility: "€625.00"},
		},
	}}

	got := expandTenancyTableRows(rows)
	if len(got) != 2 {
		t.Fatalf("table rows=%d, want 2 tenant responsibility rows", len(got))
	}
	if got[0].TenantID != 11 || got[0].TenantName != "Tenant A" || got[0].Responsibility != "€625.00" {
		t.Fatalf("first table row=%+v", got[0])
	}
	if got[1].TenantID != 12 || got[1].TenantName != "Tenant B" || got[1].Responsibility != "€625.00" {
		t.Fatalf("second table row=%+v", got[1])
	}
	if len(rows[0].Parties) != 2 {
		t.Fatalf("mobile grouped row was mutated: %+v", rows[0])
	}
}
