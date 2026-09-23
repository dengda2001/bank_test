package main

import (
	"testing"
	"time"
)

func TestStrictRentMatchDateWindowAndExactTenantName(t *testing.T) {
	t.Setenv("RENT_NEXT_MONTH_FROM_DAY", "15")
	september := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	october := september.AddDate(0, 1, 0)
	tenants := []tenant{{ID: 7, Name: "Aoife Murphy"}}
	obligations := []rentObligation{
		{ID: 11, TenantID: 7, PeriodMonth: september, ExpectedAmountCents: 90000, Currency: "EUR"},
		{ID: 12, TenantID: 7, PeriodMonth: october, ExpectedAmountCents: 90000, Currency: "EUR"},
	}
	for _, tc := range []struct {
		day        int
		wantStatus string
		wantID     uint64
	}{
		{1, "matched", 11}, {5, "matched", 11},
		{6, "candidate", 0}, {14, "candidate", 0},
		{15, "candidate", 0}, {24, "candidate", 0},
		{25, "matched", 12}, {30, "matched", 12},
	} {
		occurredAt := september.AddDate(0, 0, tc.day-1).Add(12 * time.Hour)
		decision := decideStrictRentMatch(paymentTransactionInput{
			Source: "truelayer", Direction: "income", AmountCents: 90000, Currency: "EUR",
			TransactionTime: &occurredAt, Description: "FASTER PAYMENT", PayerName: "  AOIFE  MURPHY ", PayerNameKind: "confirmed",
		}, nil, tenants, obligations)
		if decision.Status != tc.wantStatus || decision.RentObligationID != tc.wantID {
			t.Errorf("day %d: decision=%+v, want status %s obligation %d", tc.day, decision, tc.wantStatus, tc.wantID)
		}
		if tc.wantID != 0 && decision.ConfirmationSource != "auto_exact_name" {
			t.Errorf("day %d: source=%q, want direct exact name", tc.day, decision.ConfirmationSource)
		}
	}
}

func TestStrictRentMatchDateWindowHonorsIdentityAndPaymentGuards(t *testing.T) {
	september := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	occurredAt := september.AddDate(0, 0, 2)
	payerName := "Aoife Murphy"
	base := paymentTransactionInput{Source: "truelayer", Direction: "income", AmountCents: 90000, Currency: "EUR", TransactionTime: &occurredAt, Description: "FASTER PAYMENT", PayerName: payerName, PayerNameKind: "confirmed"}
	tenants := []tenant{{ID: 7, Name: payerName}, {ID: 8, Name: "Other Tenant"}}
	obligations := []rentObligation{{ID: 11, TenantID: 7, PeriodMonth: september, ExpectedAmountCents: 90000, Currency: "EUR"}}
	tests := []struct {
		name        string
		change      func(*paymentTransactionInput)
		payers      []tenantPayer
		tenants     []tenant
		obligations []rentObligation
	}{
		{"inferred payer name", func(tx *paymentTransactionInput) { tx.PayerNameKind = "inferred" }, nil, tenants, obligations},
		{"ambiguous same name", nil, nil, append(tenants, tenant{ID: 9, Name: payerName}), obligations},
		{"conflicting remembered payer", nil, []tenantPayer{{TenantID: 8, PayerNameNormalized: normalizeMatchText(payerName)}}, tenants, obligations},
		{"amount below remaining", func(tx *paymentTransactionInput) { tx.AmountCents = 89999 }, nil, tenants, obligations},
		{"amount above remaining", func(tx *paymentTransactionInput) { tx.AmountCents = 90001 }, nil, tenants, obligations},
		{"currency mismatch", func(tx *paymentTransactionInput) { tx.Currency = "GBP" }, nil, tenants, obligations},
		{"currency missing", func(tx *paymentTransactionInput) { tx.Currency = "" }, nil, tenants, []rentObligation{{ID: 11, TenantID: 7, PeriodMonth: september, ExpectedAmountCents: 90000, Currency: ""}}},
		{"deposit", func(tx *paymentTransactionInput) { tx.Description = "Deposit September" }, nil, tenants, obligations},
		{"refund", func(tx *paymentTransactionInput) { tx.Description = "Refund" }, nil, tenants, obligations},
		{"borrowed money", func(tx *paymentTransactionInput) { tx.Description = "Borrowed money" }, nil, tenants, obligations},
		{"covered obligation", nil, nil, tenants, []rentObligation{{ID: 11, TenantID: 7, PeriodMonth: september, ExpectedAmountCents: 90000, PaidAmountCents: 90000, Currency: "EUR"}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tx := base
			if tc.change != nil {
				tc.change(&tx)
			}
			decision := decideStrictRentMatch(tx, tc.payers, tc.tenants, tc.obligations)
			if decision.Status == "matched" || decision.Status == "partial" {
				t.Fatalf("unsafe automatic match: %+v", decision)
			}
		})
	}
}

func TestStrictRentMatchDateWindowUsesRememberedPayerAndCrossesYear(t *testing.T) {
	december25 := time.Date(2026, 12, 25, 12, 0, 0, 0, time.UTC)
	january := time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC)
	name := "Bank Payer"
	decision := decideStrictRentMatch(paymentTransactionInput{
		Source: "truelayer", Direction: "income", AmountCents: 90000, Currency: "EUR",
		TransactionTime: &december25, Description: "Transfer", PayerName: name, PayerNameKind: "confirmed",
	}, []tenantPayer{{TenantID: 7, PayerNameNormalized: normalizeMatchText(name)}},
		[]tenant{{ID: 7, Name: "Different Legal Name"}},
		[]rentObligation{{ID: 12, TenantID: 7, PeriodMonth: january, ExpectedAmountCents: 90000, Currency: "EUR"}})
	if decision.Status != "matched" || decision.RentObligationID != 12 || decision.ConfirmationSource != "auto_name" {
		t.Fatalf("remembered payer December->January decision=%+v", decision)
	}
}

func TestStrictRentMatchExplicitDescriptionMonthOverridesMiddleDate(t *testing.T) {
	september := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	august := september.AddDate(0, -1, 0)
	paidAt := time.Date(2026, 8, 17, 12, 0, 0, 0, time.UTC)
	decision := decideStrictRentMatch(paymentTransactionInput{
		Source: "truelayer", Direction: "income", AmountCents: 90000, Currency: "EUR",
		TransactionTime: &paidAt, Description: "SEPT RENT", PayerName: "Aoife Murphy", PayerNameKind: "confirmed",
	}, nil, []tenant{{ID: 7, Name: "Aoife Murphy"}}, []rentObligation{
		{ID: 11, TenantID: 7, PeriodMonth: august, ExpectedAmountCents: 90000, Currency: "EUR"},
		{ID: 12, TenantID: 7, PeriodMonth: september, ExpectedAmountCents: 90000, Currency: "EUR"},
	})
	if decision.Status != "matched" || decision.RentObligationID != 12 {
		t.Fatalf("explicit description month lost priority: %+v", decision)
	}
}

func TestDateAutoWindowIsConfigurableAndUsesDublinCalendarDay(t *testing.T) {
	t.Setenv("RENT_NEXT_MONTH_FROM_DAY", "15")
	t.Setenv("RENT_AUTO_CURRENT_THROUGH_DAY", "4")
	t.Setenv("RENT_AUTO_NEXT_MONTH_FROM_DAY", "26")
	september := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	october := september.AddDate(0, 1, 0)
	for _, tc := range []struct {
		at         time.Time
		wantStatus string
		wantID     uint64
	}{
		{time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC), "matched", 11},
		{time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC), "candidate", 0},
		{time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC), "candidate", 0},
		{time.Date(2026, 9, 25, 23, 30, 0, 0, time.UTC), "matched", 12},
	} {
		decision := decideStrictRentMatch(paymentTransactionInput{
			Source: "truelayer", Direction: "income", AmountCents: 90000, Currency: "EUR", TransactionTime: &tc.at,
			Description: "TRANSFER", PayerName: "Aoife Murphy", PayerNameKind: "confirmed",
		}, nil, []tenant{{ID: 7, Name: "Aoife Murphy"}}, []rentObligation{
			{ID: 11, TenantID: 7, PeriodMonth: september, ExpectedAmountCents: 90000, Currency: "EUR"},
			{ID: 12, TenantID: 7, PeriodMonth: october, ExpectedAmountCents: 90000, Currency: "EUR"},
		})
		if decision.Status != tc.wantStatus || decision.RentObligationID != tc.wantID {
			t.Errorf("at %s: decision=%+v, want %s/%d", tc.at.Format(time.RFC3339), decision, tc.wantStatus, tc.wantID)
		}
	}
}

func TestStrictRentMatchDateWindowRejectsMultipleOpenObligations(t *testing.T) {
	period := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	paidAt := period.AddDate(0, 0, 2)
	decision := decideStrictRentMatch(paymentTransactionInput{
		Source: "truelayer", Direction: "income", AmountCents: 90000, Currency: "EUR",
		TransactionTime: &paidAt, Description: "TRANSFER", PayerName: "Aoife Murphy", PayerNameKind: "confirmed",
	}, nil, []tenant{{ID: 7, Name: "Aoife Murphy"}}, []rentObligation{
		{ID: 11, TenantID: 7, PeriodMonth: period, ExpectedAmountCents: 90000, Currency: "EUR"},
		{ID: 12, TenantID: 7, PeriodMonth: period, ExpectedAmountCents: 90000, Currency: "EUR"},
	})
	if decision.Status == "matched" || decision.Status == "partial" {
		t.Fatalf("ambiguous obligations must remain for review: %+v", decision)
	}
}

func TestFutureRentPlanGateRequiresOneExactPlan(t *testing.T) {
	matching := futureRentPlanCandidate{ResponsibilityCents: 95000, Currency: "EUR"}
	for _, tc := range []struct {
		name     string
		plans    []futureRentPlanCandidate
		amount   int64
		currency string
		want     bool
	}{
		{"one exact plan", []futureRentPlanCandidate{matching}, 95000, "eur", true},
		{"missing plan", nil, 95000, "EUR", false},
		{"two matching plans", []futureRentPlanCandidate{matching, matching}, 95000, "EUR", false},
		{"wrong amount", []futureRentPlanCandidate{matching}, 90000, "EUR", false},
		{"wrong currency", []futureRentPlanCandidate{matching}, 95000, "GBP", false},
		{"currency missing", []futureRentPlanCandidate{{ResponsibilityCents: 95000, Currency: ""}}, 95000, "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := oneFutureRentPlanMatches(tc.plans, tc.amount, tc.currency); got != tc.want {
				t.Fatalf("gate=%v want %v", got, tc.want)
			}
		})
	}
}
