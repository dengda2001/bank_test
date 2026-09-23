package main

import "testing"

func TestValidateRentMatchBatchItemsAllowsRoommatesAndTwoMonths(t *testing.T) {
	cases := []struct {
		name        string
		items       []rentMatchBatchItem
		wantTenants int
		wantMonths  int
	}{
		{name: "two roommates", items: []rentMatchBatchItem{{TenantID: 7, Period: "2026-09", AmountCents: 60000}, {TenantID: 8, Period: "2026-09", AmountCents: 60000}}, wantTenants: 2, wantMonths: 1},
		{name: "same tenant two months", items: []rentMatchBatchItem{{TenantID: 7, Period: "2026-09", AmountCents: 60000}, {TenantID: 7, Period: "2026-10", AmountCents: 60000}}, wantTenants: 1, wantMonths: 2},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tenants, months, err := validateRentMatchBatchItems(tc.items, 7)
			if err != nil || len(tenants) != tc.wantTenants || len(months) != tc.wantMonths {
				t.Fatalf("targets = %v tenants/%v months, %v", tenants, months, err)
			}
		})
	}
	for _, tc := range []struct {
		name     string
		items    []rentMatchBatchItem
		remember uint64
	}{
		{name: "duplicate tenant month", items: []rentMatchBatchItem{{TenantID: 7, Period: "2026-09", AmountCents: 60000}, {TenantID: 7, Period: "2026-09", AmountCents: 10000}}},
		{name: "invalid month", items: []rentMatchBatchItem{{TenantID: 7, Period: "2026-13", AmountCents: 60000}}},
		{name: "zero amount", items: []rentMatchBatchItem{{TenantID: 7, Period: "2026-09", AmountCents: 0}}},
		{name: "payer outside batch", items: []rentMatchBatchItem{{TenantID: 7, Period: "2026-09", AmountCents: 60000}}, remember: 8},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, _, err := validateRentMatchBatchItems(tc.items, tc.remember); err == nil {
				t.Fatal("invalid batch was accepted")
			}
		})
	}
}
