package main

import (
	"strings"
	"testing"
	"time"
)

func TestBuildTenantBillingHistoryGroupsPaymentsUnderMonthlyBills(t *testing.T) {
	tenantRow := tenant{ID: 7, Currency: "EUR"}
	currentMonth := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	obligations := []rentObligation{
		{ID: 71, TenantID: 7, PeriodMonth: currentMonth, DueDate: time.Date(2026, 9, 5, 0, 0, 0, 0, time.UTC), ExpectedAmountCents: 95000, PaidAmountCents: 95000, Currency: "EUR"},
		{ID: 72, TenantID: 7, PeriodMonth: currentMonth.AddDate(0, -1, 0), DueDate: time.Date(2026, 8, 5, 0, 0, 0, 0, time.UTC), ExpectedAmountCents: 95000, PaidAmountCents: 0, Currency: "EUR"},
	}
	paymentRows := []tenantBillingPaymentRow{
		{TenantID: 7, ObligationID: 71, AmountCents: 30000, Currency: "EUR", TransactionTime: ptrTime(time.Date(2026, 9, 1, 8, 0, 0, 0, time.UTC)), Description: "first transfer", Reference: "rent-2026-09", ConfirmationSource: "auto_id"},
		{TenantID: 7, ObligationID: 71, AmountCents: 30000, Currency: "EUR", TransactionTime: ptrTime(time.Date(2026, 9, 2, 8, 0, 0, 0, time.UTC)), Description: "second transfer", Reference: "rent-2026-09", ConfirmationSource: "manual"},
		{TenantID: 7, ObligationID: 71, AmountCents: 35000, Currency: "EUR", TransactionTime: ptrTime(time.Date(2026, 9, 3, 8, 0, 0, 0, time.UTC)), Description: "final transfer", Reference: "rent-2026-09", ConfirmationSource: "auto_name"},
	}

	got := buildTenantBillingHistory([]tenant{tenantRow}, obligations, paymentRows, time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC))
	history := got[tenantRow.ID]
	if len(history) != 2 {
		t.Fatalf("history length=%d want 2: %#v", len(history), history)
	}
	if history[0].Period != "2026-09" || history[0].ExpectedAmount != "EUR 950.00" || history[0].PaidAmount != "EUR 950.00" || len(history[0].Payments) != 3 {
		t.Fatalf("current month summary=%+v want one summary with three payments", history[0])
	}
	if history[1].Period != "2026-08" || history[1].PaidAmount != "EUR 0.00" || history[1].BalanceAmount != "EUR 950.00" || len(history[1].Payments) != 0 {
		t.Fatalf("empty month summary=%+v want zero paid and no details", history[1])
	}
}

func TestBuildTenantBillingHistoryIgnoresObligationsForOtherTenants(t *testing.T) {
	currentMonth := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	got := buildTenantBillingHistory(
		[]tenant{{ID: 7, Currency: "EUR"}},
		[]rentObligation{
			{ID: 71, TenantID: 7, PeriodMonth: currentMonth, ExpectedAmountCents: 95000, Currency: "EUR"},
			{ID: 81, TenantID: 8, PeriodMonth: currentMonth, ExpectedAmountCents: 120000, Currency: "EUR"},
		},
		nil,
		time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC),
	)
	if len(got) != 1 || len(got[7]) != 1 || got[7][0].ObligationID != 71 {
		t.Fatalf("history=%+v want only tenant 7 obligation 71", got)
	}
}

func TestBuildTenantBillingHistoryLabelsCashReceiptRows(t *testing.T) {
	when := ptrTime(time.Date(2026, 9, 12, 0, 0, 0, 0, time.UTC))
	got := buildTenantBillingHistory(
		[]tenant{{ID: 7, Currency: "EUR"}},
		[]rentObligation{{ID: 71, TenantID: 7, PeriodMonth: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC), ExpectedAmountCents: 100000, PaidAmountCents: 40000, Currency: "EUR"}},
		[]tenantBillingPaymentRow{{TenantID: 7, ObligationID: 71, AmountCents: 40000, Currency: "EUR", Source: "cash", TransactionTime: when, Description: "现金租金补录", Reference: "cash-123", ConfirmationSource: "manual_cash"}},
		time.Date(2026, 9, 13, 0, 0, 0, 0, time.UTC),
	)[7][0]
	if len(got.Payments) != 1 || got.Payments[0].Source != "现金" || got.Payments[0].Reference != "cash-123" || got.Payments[0].DateDisplay != "12 Sep 2026 00:00" {
		t.Fatalf("cash payment=%+v", got.Payments)
	}
}

func TestRentPaymentDetailLabelsCashSourceAndReceiptDate(t *testing.T) {
	detail := rentPaymentDetailFromRow(rentPaymentDetailRow{
		AmountCents:        40000,
		Currency:           "EUR",
		Source:             "cash",
		TransactionTime:    ptrTime(time.Date(2026, 9, 12, 0, 0, 0, 0, time.UTC)),
		Description:        "现金租金补录",
		Reference:          "cash-123",
		ConfirmationSource: "manual_cash",
	})
	if detail.Source != "现金" || detail.Reference != "cash-123" || detail.DateDisplay != "12 Sep 2026 00:00" || detail.ConfirmationSource != "manual_cash" {
		t.Fatalf("cash detail=%+v", detail)
	}
}

func TestTenantTemplateRendersNestedBillingHistoryToggles(t *testing.T) {
	var body strings.Builder
	err := tenantTemplate.Execute(&body, tenantPageData{Rows: []tenantRecord{{
		ID:   "7",
		Name: "Aoife Murphy",
		BillingHistory: []tenantBillingMonth{{
			ObligationID:   71,
			Period:         "2026-09",
			PeriodLabel:    "2026年9月",
			ExpectedAmount: "EUR 950.00",
			PaidAmount:     "EUR 950.00",
			BalanceAmount:  "EUR 0.00",
			Status:         "paid",
			StatusLabel:    "已缴清",
			Payments: []rentPaymentDetail{{
				AmountDisplay: "EUR 950.00",
				DateDisplay:   "14 Sep 2026 09:00",
				Description:   "September rent",
				Reference:     "rent-2026-09",
			}},
		}},
	}}})
	if err != nil {
		t.Fatal(err)
	}
	page := body.String()
	for _, expected := range []string{
		`aria-controls="tenant-billing-7"`,
		`id="tenant-billing-7"`,
		`aria-controls="tenant-month-71"`,
		`id="tenant-month-71"`,
		"<th>应收</th>",
		"<th>实际收</th>",
		"EUR 950.00",
		"September rent",
	} {
		if !strings.Contains(page, expected) {
			t.Fatalf("tenant template missing %q: %s", expected, page)
		}
	}
	for _, unwanted := range []string{"参考号", "rent-2026-09"} {
		if strings.Contains(page, unwanted) {
			t.Fatalf("tenant payment detail still renders %q: %s", unwanted, page)
		}
	}
}
