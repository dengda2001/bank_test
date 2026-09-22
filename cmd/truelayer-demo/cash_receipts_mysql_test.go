package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"
)

func TestCashReceiptMigrationsAreIdempotentOnMySQL(t *testing.T) {
	db, sqlDB := openLedgerMySQLTestDB(t)
	for i := 0; i < 2; i++ {
		if err := runMigrations(sqlDB, "../../migrations"); err != nil {
			t.Fatalf("migration run %d: %v", i+1, err)
		}
	}
	for _, version := range []string{"006_cash_rent_receipts", "007_cash_receipt_void_audit"} {
		var count int64
		if err := db.Table("schema_migrations").Where("version = ?", version).Count(&count).Error; err != nil {
			t.Fatalf("count %s: %v", version, err)
		}
		if count != 1 {
			t.Fatalf("migration %s count=%d want 1", version, count)
		}
	}
	var columnCount int64
	if err := db.Raw(`SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'cash_receipts' AND COLUMN_NAME = 'void_operation_id'`).Scan(&columnCount).Error; err != nil {
		t.Fatalf("check void operation column: %v", err)
	}
	if columnCount != 1 {
		t.Fatalf("void operation column count=%d want 1", columnCount)
	}
}

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
	// This scenario asserts against a 100000-cent obligation, so pin the rent
	// rather than inheriting whatever the shared profile fixture uses.
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
	preview, err := service.previewCashReceipt(ctx, cashInput)
	if err != nil {
		t.Fatal(err)
	}
	if preview.CurrentPaidCents != 0 || preview.AfterPaidCents != 40000 || preview.AfterRemainingCents != 60000 {
		t.Fatalf("cash preview=%+v", preview)
	}
	var previewCount int64
	if err := db.WithContext(ctx).Model(&cashReceipt{}).Where("user_id = ? AND rent_obligation_id = ?", owner.ID, obligation.ID).Count(&previewCount).Error; err != nil {
		t.Fatal(err)
	}
	if previewCount != 0 {
		t.Fatalf("preview wrote %d cash rows", previewCount)
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
		Source:               "truelayer",
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
	paymentDetails, err := newObligationService(db).listRentPayments(ctx, owner.ID, obligation.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(paymentDetails) != 2 || paymentDetails[0].Reference == "" || paymentDetails[1].Reference == "" {
		t.Fatalf("payment details=%+v want bank and cash rows", paymentDetails)
	}
	seenSources := map[string]bool{}
	for _, detail := range paymentDetails {
		seenSources[detail.Source] = true
	}
	if !seenSources["银行"] || !seenSources["现金"] {
		t.Fatalf("payment detail sources=%v want bank and cash", seenSources)
	}
	historyPage, err := newObligationService(db).listTenantBillingHistoryPage(ctx, owner.ID, tenantRow.ID, period, period, 1, 12)
	if err != nil {
		t.Fatal(err)
	}
	if len(historyPage.Rows) != 1 || len(historyPage.Rows[0].Payments) != 2 {
		t.Fatalf("history page=%+v want one month with two payments", historyPage)
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

func TestCashReceiptConcurrentBalanceGuardOnMySQL(t *testing.T) {
	db, sqlDB := openLedgerMySQLTestDB(t)
	if err := runMigrations(sqlDB, "../../migrations"); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	ctx := context.Background()
	owner := user{Username: fmt.Sprintf("cash-concurrency-%d", time.Now().UnixNano()), PasswordHash: "test"}
	if err := db.WithContext(ctx).Create(&owner).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.WithContext(ctx).Delete(&user{}, owner.ID) })

	input := validTenantInputForProfile()
	tenantRow, err := newTenantService(db).createTenant(ctx, owner.ID, input)
	if err != nil {
		t.Fatal(err)
	}
	period := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)
	if err := newObligationService(db).ensureMonthlyObligations(ctx, owner.ID, period); err != nil {
		t.Fatal(err)
	}
	var obligation rentObligation
	if err := db.WithContext(ctx).Where("user_id = ? AND tenant_id = ? AND period_month = ?", owner.ID, tenantRow.ID, period).First(&obligation).Error; err != nil {
		t.Fatal(err)
	}

	start := make(chan struct{})
	results := make(chan error, 2)
	service := newCashReceiptService(db)
	var wait sync.WaitGroup
	for index := 0; index < 2; index++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			<-start
			_, err := service.recordCashReceipt(ctx, cashReceiptInput{
				UserID:           owner.ID,
				TenantID:         tenantRow.ID,
				RentObligationID: obligation.ID,
				AmountCents:      30000,
				Currency:         "EUR",
				ReceivedAt:       time.Date(2026, 10, 12+index, 0, 0, 0, 0, time.UTC),
				IdempotencyKey:   fmt.Sprintf("cash-concurrent-%d", index),
			})
			results <- err
		}(index)
	}
	close(start)
	wait.Wait()
	close(results)
	successes := 0
	for err := range results {
		if err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("concurrent successes=%d want 1", successes)
	}
	var count int64
	if err := db.WithContext(ctx).Model(&cashReceipt{}).Where("user_id = ? AND rent_obligation_id = ? AND status = ?", owner.ID, obligation.ID, cashReceiptStatusConfirmed).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("confirmed cash rows=%d want 1", count)
	}
	if err := db.WithContext(ctx).Where("id = ? AND user_id = ?", obligation.ID, owner.ID).First(&obligation).Error; err != nil {
		t.Fatal(err)
	}
	if obligation.PaidAmountCents != 30000 {
		t.Fatalf("concurrent paid projection=%d want 30000", obligation.PaidAmountCents)
	}
}
