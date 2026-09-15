package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCashReceiptMigrationDefinesAuditableUserScopedTable(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "..", "migrations", "006_cash_rent_receipts.sql"))
	if err != nil {
		t.Fatalf("read cash migration: %v", err)
	}
	sql := strings.ToLower(string(body))
	for _, fragment := range []string{
		"create table if not exists cash_receipts",
		"receipt_number",
		"received_at",
		"idempotency_key",
		"void_reason",
		"idx_cash_receipts_user_obligation_status",
		"fk_cash_receipts_obligation",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("cash migration missing %q", fragment)
		}
	}
	voidAudit, err := os.ReadFile(filepath.Join("..", "..", "migrations", "007_cash_receipt_void_audit.sql"))
	if err != nil {
		t.Fatalf("read cash void audit migration: %v", err)
	}
	if sql := strings.ToLower(string(voidAudit)); !strings.Contains(sql, "add column void_operation_id") {
		t.Fatalf("cash void audit migration missing void operation column")
	}
}

func TestValidateCashReceiptInputEnforcesEURPositiveAmountAndOwnership(t *testing.T) {
	base := cashReceiptInput{
		UserID:           7,
		TenantID:         11,
		RentObligationID: 21,
		AmountCents:      40000,
		Currency:         "EUR",
		ReceivedAt:       time.Date(2026, 9, 12, 0, 0, 0, 0, time.UTC),
		IdempotencyKey:   "cash-request-1",
	}
	if err := validateCashReceiptInput(base); err != nil {
		t.Fatalf("valid cash receipt rejected: %v", err)
	}

	cases := []struct {
		name   string
		mutate func(*cashReceiptInput)
		want   string
	}{
		{name: "missing user", mutate: func(v *cashReceiptInput) { v.UserID = 0 }, want: "user"},
		{name: "zero amount", mutate: func(v *cashReceiptInput) { v.AmountCents = 0 }, want: "positive"},
		{name: "non EUR", mutate: func(v *cashReceiptInput) { v.Currency = "GBP" }, want: "EUR"},
		{name: "missing date", mutate: func(v *cashReceiptInput) { v.ReceivedAt = time.Time{} }, want: "date"},
		{name: "missing idempotency", mutate: func(v *cashReceiptInput) { v.IdempotencyKey = "" }, want: "idempotency"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			input := base
			tc.mutate(&input)
			err := validateCashReceiptInput(input)
			if err == nil || !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(tc.want)) {
				t.Fatalf("error=%v want substring %q", err, tc.want)
			}
		})
	}
}

func TestCashReceiptEffectiveAndPaidAmountIgnoreVoidedRows(t *testing.T) {
	rows := []cashReceipt{
		{AmountCents: 40000, Currency: "EUR", Status: cashReceiptStatusConfirmed},
		{AmountCents: 30000, Currency: "EUR", Status: cashReceiptStatusVoided},
		{AmountCents: 20000, Currency: "GBP", Status: cashReceiptStatusConfirmed},
	}
	if !cashReceiptIsEffective(rows[0]) || cashReceiptIsEffective(rows[1]) || cashReceiptIsEffective(rows[2]) {
		t.Fatalf("unexpected cash effectiveness: %+v", rows)
	}
	if got := cashReceiptPaidAmount(rows); got != 40000 {
		t.Fatalf("cash paid=%d want 40000", got)
	}
}

func TestProjectRentObligationIncludesEffectiveBankAndCashRentOnly(t *testing.T) {
	obligation := rentObligation{
		UserID:              7,
		TenantID:            11,
		ExpectedAmountCents: 100000,
		Currency:            "EUR",
		DueDate:             time.Date(2026, 9, 5, 0, 0, 0, 0, time.UTC),
		RecordStatus:        obligationRecordActive,
	}
	tenantID := uint64(11)
	projected := projectRentObligation(obligation, []paymentAllocation{
		{TenantID: &tenantID, AmountCents: 60000, AllocationKind: allocationKindRent, Status: allocationStatusConfirmed},
		{TenantID: &tenantID, AmountCents: 90000, AllocationKind: allocationKindDeposit, Status: allocationStatusConfirmed},
	}, []cashReceipt{
		{TenantID: tenantID, AmountCents: 40000, Currency: "EUR", Status: cashReceiptStatusConfirmed},
		{TenantID: tenantID, AmountCents: 20000, Currency: "EUR", Status: cashReceiptStatusVoided},
	}, time.Date(2026, 9, 20, 0, 0, 0, 0, time.UTC))
	if projected.PaidAmountCents != 100000 || projected.Status != "paid" {
		t.Fatalf("projected paid/status=%d/%q want 100000/paid", projected.PaidAmountCents, projected.Status)
	}
}
