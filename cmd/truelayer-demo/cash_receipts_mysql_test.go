package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"
)

func TestCashReceiptLifecycleOnMySQL(t *testing.T) {
	db, sqlDB := openLedgerMySQLTestDB(t)
	if err := runMigrations(sqlDB, "../../migrations"); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	ctx := context.Background()
	owner := user{Username: fmt.Sprintf("cash-receipt-%d", time.Now().UnixNano()), PasswordHash: "test"}
	if err := db.WithContext(ctx).Create(&owner).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.WithContext(ctx).Delete(&user{}, owner.ID) })

	input := validTenantInputForProfile()
	input.RentStartDate = "2026-01-01"
	input.BillingStartDate = "2026-01-01"
	tenantRow, err := newTenantService(db).createTenant(ctx, owner.ID, input)
	if err != nil {
		t.Fatal(err)
	}
	period := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	if err := newObligationService(db).ensureMonthlyObligations(ctx, owner.ID, period); err != nil {
		t.Fatal(err)
	}
	var obligation rentObligation
	if err := db.WithContext(ctx).Where("user_id = ? AND tenant_id = ? AND period_month = ?", owner.ID, tenantRow.ID, period).First(&obligation).Error; err != nil {
		t.Fatal(err)
	}
	service := newCashReceiptService(db)
	receivedAt := time.Date(2026, 9, 12, 0, 0, 0, 0, time.UTC)
	cashInput := cashReceiptInput{
		UserID:           owner.ID,
		TenantID:         tenantRow.ID,
		RentObligationID: obligation.ID,
		AmountCents:      40000,
		Currency:         "eur",
		ReceivedAt:       receivedAt,
		Note:             "cash rent",
		IdempotencyKey:   "cash-400",
	}
	first, err := service.recordCashReceipt(ctx, cashInput)
	if err != nil {
		t.Fatal(err)
	}
	if first.ID == 0 || first.Status != cashReceiptStatusConfirmed || first.Currency != ledgerCurrencyEUR || first.OperationID == "" || first.VoidOperationID != nil {
		t.Fatalf("cash receipt=%+v", first)
	}
	retry, err := service.recordCashReceipt(ctx, cashInput)
	if err != nil {
		t.Fatal(err)
	}
	if retry.ID != first.ID {
		t.Fatalf("idempotent retry id=%d want %d", retry.ID, first.ID)
	}
	conflicting := cashInput
	conflicting.AmountCents = 40001
	if _, err := service.recordCashReceipt(ctx, conflicting); err == nil || !strings.Contains(err.Error(), "idempotency") {
		t.Fatalf("conflicting idempotency error=%v", err)
	}

	transaction := paymentTransaction{
		UserID:               owner.ID,
		Source:               "test",
		StableTransactionKey: fmt.Sprintf("cash-bank-%d", time.Now().UnixNano()),
		Direction:            "income",
		AmountCents:          60000,
		Currency:             "EUR",
		MatchStatus:          "unmatched",
	}
	if err := db.WithContext(ctx).Create(&transaction).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := newTransactionService(db).allocateTransaction(ctx, owner.ID, transaction.ID, []transactionAllocationDraft{{
		TenantID:         tenantRow.ID,
		RentObligationID: obligation.ID,
		AmountCents:      60000,
		Kind:             allocationKindRent,
	}}, "cash-bank-allocation", "manual"); err != nil {
		t.Fatal(err)
	}
	var projected rentObligation
	if err := db.WithContext(ctx).Where("id = ? AND user_id = ?", obligation.ID, owner.ID).First(&projected).Error; err != nil {
		t.Fatal(err)
	}
	if projected.PaidAmountCents != 100000 || projected.Status != "paid" {
		t.Fatalf("paid projection=%d/%q want 100000/paid", projected.PaidAmountCents, projected.Status)
	}

	voided, err := service.voidCashReceipt(ctx, owner.ID, first.ID, "cash entry correction")
	if err != nil {
		t.Fatal(err)
	}
	if voided.Status != cashReceiptStatusVoided || voided.OperationID != first.OperationID || voided.VoidOperationID == nil || *voided.VoidOperationID == first.OperationID {
		t.Fatalf("voided receipt=%+v", voided)
	}
	if _, err := service.voidCashReceipt(ctx, owner.ID, first.ID, "cash entry correction"); err != nil {
		t.Fatalf("idempotent void: %v", err)
	}

	corrected := cashInput
	corrected.AmountCents = 30000
	corrected.Note = "cash rent corrected"
	corrected.IdempotencyKey = "cash-300"
	if _, err := service.recordCashReceipt(ctx, corrected); err != nil {
		t.Fatal(err)
	}
	if _, err := service.recordCashReceipt(ctx, cashReceiptInput{
		UserID:           owner.ID,
		TenantID:         tenantRow.ID,
		RentObligationID: obligation.ID,
		AmountCents:      50000,
		Currency:         "EUR",
		ReceivedAt:       receivedAt,
		IdempotencyKey:   "cash-overbalance",
	}); err == nil {
		t.Fatal("overbalance cash receipt unexpectedly accepted")
	}
	if err := db.WithContext(ctx).Where("id = ? AND user_id = ?", obligation.ID, owner.ID).First(&projected).Error; err != nil {
		t.Fatal(err)
	}
	if projected.PaidAmountCents != 90000 || projected.Status != "partial" {
		t.Fatalf("corrected paid projection=%d/%q want 90000/partial", projected.PaidAmountCents, projected.Status)
	}

	var cashCount int64
	if err := db.WithContext(ctx).Model(&cashReceipt{}).Where("user_id = ? AND rent_obligation_id = ?", owner.ID, obligation.ID).Count(&cashCount).Error; err != nil {
		t.Fatal(err)
	}
	var transactionCount int64
	if err := db.WithContext(ctx).Model(&paymentTransaction{}).Where("user_id = ?", owner.ID).Count(&transactionCount).Error; err != nil {
		t.Fatal(err)
	}
	if cashCount != 2 || transactionCount != 1 {
		t.Fatalf("cash/bank row counts=%d/%d want 2/1", cashCount, transactionCount)
	}
	if _, err := service.voidCashReceipt(ctx, owner.ID+1, first.ID, "cross-user"); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("cross-user void error=%v want record not found", err)
	}
}
