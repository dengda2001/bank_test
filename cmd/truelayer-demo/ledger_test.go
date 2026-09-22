package main

import (
	"strings"
	"testing"
	"time"
)

func TestNormalizeLedgerCurrencyIsEUROnlyButKeepsExtensionBoundary(t *testing.T) {
	if got, err := normalizeLedgerCurrency(" eur "); err != nil || got != "EUR" {
		t.Fatalf("normalize EUR = %q, %v; want EUR, nil", got, err)
	}
	if _, err := normalizeLedgerCurrency("GBP"); err == nil || !strings.Contains(err.Error(), "EUR") {
		t.Fatalf("normalize GBP error = %v; want EUR-only error", err)
	}
}

func TestTenantValidationDoesNotStoreRentCurrencyOnPersonProfile(t *testing.T) {
	if err := validateTenantInput(tenantInput{Name: "Tenant", Email: "tenant@example.test", Status: "active"}); err != nil {
		t.Fatalf("valid person profile rejected: %v", err)
	}
}

func TestValidateLedgerAllocationEnforcesBudgetAndOwnership(t *testing.T) {
	base := ledgerAllocationCheck{
		UserID:                  7,
		SourceUserID:            7,
		TenantID:                11,
		ObligationTenantID:      11,
		SourceAmountCents:       100000,
		ExistingAllocatedCents:  25000,
		AmountCents:             75000,
		ObligationExpectedCents: 100000,
		ObligationPaidCents:     25000,
		SourceCurrency:          "EUR",
		ObligationCurrency:      "EUR",
		Kind:                    allocationKindRent,
	}
	if err := validateLedgerAllocation(base); err != nil {
		t.Fatalf("valid rent allocation rejected: %v", err)
	}

	cases := []struct {
		name   string
		mutate func(*ledgerAllocationCheck)
		want   string
	}{
		{name: "zero amount", mutate: func(v *ledgerAllocationCheck) { v.AmountCents = 0 }, want: "positive"},
		{name: "source overdrawn", mutate: func(v *ledgerAllocationCheck) { v.AmountCents = 75001 }, want: "source"},
		{name: "rent over obligation", mutate: func(v *ledgerAllocationCheck) { v.ObligationPaidCents = 99999 }, want: "obligation"},
		{name: "currency mismatch", mutate: func(v *ledgerAllocationCheck) { v.SourceCurrency = "GBP" }, want: "EUR"},
		{name: "cross user", mutate: func(v *ledgerAllocationCheck) { v.SourceUserID = 8 }, want: "user"},
		{name: "cross tenant", mutate: func(v *ledgerAllocationCheck) { v.ObligationTenantID = 12 }, want: "tenant"},
		{name: "negative existing allocation", mutate: func(v *ledgerAllocationCheck) { v.ExistingAllocatedCents = -1 }, want: "budget"},
		{name: "unknown kind", mutate: func(v *ledgerAllocationCheck) { v.Kind = "refund" }, want: "kind"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input := base
			tc.mutate(&input)
			err := validateLedgerAllocation(input)
			if err == nil || !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(tc.want)) {
				t.Fatalf("error = %v; want substring %q", err, tc.want)
			}
		})
	}

	deposit := base
	deposit.Kind = allocationKindDeposit
	deposit.TenantID = 0
	deposit.ObligationTenantID = 0
	deposit.ObligationExpectedCents = 0
	deposit.ObligationPaidCents = 0
	deposit.ObligationCurrency = ""
	if err := validateLedgerAllocation(deposit); err != nil {
		t.Fatalf("valid deposit allocation rejected without obligation: %v", err)
	}
}

func TestLedgerAllocationEffectiveRecognizesLegacyAndExcludesVoidedOrUnknown(t *testing.T) {
	cases := []struct {
		name string
		row  paymentAllocation
		want bool
	}{
		{name: "legacy confirmed rent", row: paymentAllocation{Status: allocationStatusConfirmed}, want: true},
		{name: "confirmed deposit", row: paymentAllocation{Status: allocationStatusConfirmed, AllocationKind: allocationKindDeposit}, want: true},
		{name: "voided rent", row: paymentAllocation{Status: allocationStatusVoided, AllocationKind: allocationKindRent}, want: false},
		{name: "pending rent", row: paymentAllocation{Status: "pending", AllocationKind: allocationKindRent}, want: false},
		{name: "unknown kind", row: paymentAllocation{Status: allocationStatusConfirmed, AllocationKind: "refund"}, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ledgerAllocationIsEffective(tc.row); got != tc.want {
				t.Fatalf("effective = %v; want %v", got, tc.want)
			}
		})
	}
}

func TestLedgerPaidAmountSumsOnlyEffectiveRentAllocations(t *testing.T) {
	rows := []paymentAllocation{
		{AmountCents: 10000, Status: allocationStatusConfirmed}, // pre-migration rent row
		{AmountCents: 30000, AllocationKind: allocationKindRent, Status: allocationStatusConfirmed},
		{AmountCents: 20000, AllocationKind: allocationKindDeposit, Status: allocationStatusConfirmed},
		{AmountCents: 10000, AllocationKind: allocationKindRent, Status: allocationStatusVoided},
		{AmountCents: 50000, AllocationKind: allocationKindOtherIncome, Status: allocationStatusConfirmed},
	}
	if got := ledgerPaidAmount(rows); got != 40000 {
		t.Fatalf("ledgerPaidAmount = %d; want 40000", got)
	}
}

func TestLedgerObligationStatusUsesDublinCalendarDay(t *testing.T) {
	due := time.Date(2026, 9, 5, 0, 0, 0, 0, time.UTC)
	if got := ledgerObligationStatus(1000, 1000, due, time.Date(2026, 9, 20, 0, 0, 0, 0, time.UTC), obligationRecordActive); got != "paid" {
		t.Fatalf("paid status = %q; want paid", got)
	}
	if got := ledgerObligationStatus(1000, 400, due, time.Date(2026, 9, 20, 0, 0, 0, 0, time.UTC), obligationRecordActive); got != "partial" {
		t.Fatalf("partial status = %q; want partial", got)
	}
	for _, tc := range []struct {
		name string
		now  time.Time
		want string
	}{
		{name: "due day before midnight Dublin", now: time.Date(2026, 9, 5, 22, 59, 0, 0, time.UTC), want: "open"},
		{name: "next Dublin day", now: time.Date(2026, 9, 6, 0, 1, 0, 0, time.UTC), want: "overdue"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := ledgerObligationStatus(1000, 0, due, tc.now, obligationRecordActive); got != tc.want {
				t.Fatalf("status = %q; want %q", got, tc.want)
			}
		})
	}
	if got := ledgerObligationStatus(1000, 0, due, time.Now(), obligationRecordVoided); got != "voided" {
		t.Fatalf("void status = %q; want voided", got)
	}
}

func TestProjectLedgerObligationKeepsVoidedRecordOutOfPaymentStatuses(t *testing.T) {
	obligation := rentObligation{
		ExpectedAmountCents: 100000,
		PaidAmountCents:     100000,
		DueDate:             time.Date(2026, 9, 5, 0, 0, 0, 0, time.UTC),
		RecordStatus:        obligationRecordVoided,
		Status:              "paid",
	}
	projected := projectLedgerObligation(obligation, []paymentAllocation{
		{AmountCents: 100000, AllocationKind: allocationKindRent, Status: allocationStatusConfirmed},
	}, time.Date(2026, 9, 20, 0, 0, 0, 0, time.UTC))
	if projected.PaidAmountCents != 100000 || projected.Status != obligationRecordVoided {
		t.Fatalf("voided projection = %d/%q; want 100000/voided", projected.PaidAmountCents, projected.Status)
	}
}

func TestProjectLedgerObligationRecomputesPaidAndStatusFromEffectiveRent(t *testing.T) {
	due := time.Date(2026, 9, 5, 0, 0, 0, 0, time.UTC)
	obligation := rentObligation{
		ExpectedAmountCents: 100000,
		PaidAmountCents:     99999,
		DueDate:             due,
		Status:              "paid",
		RecordStatus:        obligationRecordActive,
	}
	projected := projectLedgerObligation(obligation, []paymentAllocation{
		{AmountCents: 40000, AllocationKind: allocationKindRent, Status: allocationStatusConfirmed},
		{AmountCents: 60000, AllocationKind: allocationKindRent, Status: allocationStatusConfirmed},
		{AmountCents: 50000, AllocationKind: allocationKindDeposit, Status: allocationStatusConfirmed},
		{AmountCents: 20000, AllocationKind: allocationKindRent, Status: allocationStatusVoided},
	}, time.Date(2026, 9, 5, 21, 0, 0, 0, time.UTC))
	if projected.PaidAmountCents != 100000 || projected.Status != "paid" {
		t.Fatalf("projected paid/status = %d/%q; want 100000/paid", projected.PaidAmountCents, projected.Status)
	}
}
