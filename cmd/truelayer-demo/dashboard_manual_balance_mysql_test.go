package main

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"gorm.io/gorm"
)

func TestManualBalanceSettlesOnlyTheOutstandingRentOnMySQL(t *testing.T) {
	db, sqlDB := openLedgerMySQLTestDB(t)
	if err := runMigrations(sqlDB, "../../migrations"); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	ctx := context.Background()
	owner := user{Username: fmt.Sprintf("manual-balance-owner-%d", time.Now().UnixNano()), PasswordHash: "test"}
	otherUser := user{Username: fmt.Sprintf("manual-balance-other-%d", time.Now().UnixNano()), PasswordHash: "test"}
	if err := db.WithContext(ctx).Create(&owner).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.WithContext(ctx).Create(&otherUser).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.WithContext(ctx).Delete(&user{}, owner.ID).Error
		_ = db.WithContext(ctx).Delete(&user{}, otherUser.ID).Error
	})

	input := validTenantInputForProfile()
	input.Name = "Manual Balance Tenant"
	input.MonthlyRent = 1000
	input.RentStartDate = "2026-01-01"
	input.BillingStartDate = "2026-01-01"
	tenantRow, err := newTenantService(db).createTenant(ctx, owner.ID, input)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := newTenantService(db).addTenantPayer(ctx, owner.ID, tenantRow.ID, tenantPayerInput{Name: tenantRow.Name}); err != nil {
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

	partialAt := time.Date(2026, 9, 4, 9, 0, 0, 0, time.UTC)
	partialTransaction := paymentTransaction{
		UserID:               owner.ID,
		Source:               "test",
		StableTransactionKey: fmt.Sprintf("manual-balance-partial-%d", time.Now().UnixNano()),
		Direction:            "income",
		AmountCents:          40000,
		Currency:             ledgerCurrencyEUR,
		TransactionTime:      &partialAt,
		MatchStatus:          "unmatched",
	}
	if err := db.WithContext(ctx).Create(&partialTransaction).Error; err != nil {
		t.Fatal(err)
	}
	service := newTransactionService(db)
	if _, err := service.allocateTransaction(ctx, owner.ID, partialTransaction.ID, []transactionAllocationDraft{{
		TenantID:         tenantRow.ID,
		RentObligationID: obligation.ID,
		AmountCents:      40000,
		Kind:             allocationKindRent,
	}}, "manual-balance-partial", "manual"); err != nil {
		t.Fatal(err)
	}

	created, err := service.settleRentObligation(ctx, owner.ID, obligation.ID, "补录现金收款")
	if err != nil {
		t.Fatal(err)
	}
	if created.ID == 0 || created.Source != manualBalanceTransactionSource || created.Description != manualBalanceTransactionDescription || created.AmountCents != 60000 || created.Currency != ledgerCurrencyEUR || created.MatchStatus != "matched" || created.MatchedTenantID == nil || *created.MatchedTenantID != tenantRow.ID {
		t.Fatalf("manual balance transaction=%+v", created)
	}
	var allocation paymentAllocation
	if err := db.WithContext(ctx).Where("user_id = ? AND payment_transaction_id = ?", owner.ID, created.ID).First(&allocation).Error; err != nil {
		t.Fatal(err)
	}
	if allocation.RentObligationID == nil || *allocation.RentObligationID != obligation.ID || allocation.AmountCents != 60000 || allocation.AllocationKind != allocationKindRent || allocation.ConfirmationSource != manualBalanceConfirmationSource {
		t.Fatalf("manual balance allocation=%+v", allocation)
	}
	if err := db.WithContext(ctx).Where("id = ? AND user_id = ?", obligation.ID, owner.ID).First(&obligation).Error; err != nil {
		t.Fatal(err)
	}
	if obligation.PaidAmountCents != 100000 || obligation.Status != "paid" {
		t.Fatalf("settled obligation=%+v", obligation)
	}

	if _, err := service.settleRentObligation(ctx, owner.ID, obligation.ID, "重复确认"); !errors.Is(err, errManualBalanceNotNeeded) {
		t.Fatalf("repeat settlement error=%v want no-balance error", err)
	}
	var manualCount int64
	if err := db.WithContext(ctx).Model(&paymentTransaction{}).Where("user_id = ? AND source = ?", owner.ID, manualBalanceTransactionSource).Count(&manualCount).Error; err != nil {
		t.Fatal(err)
	}
	if manualCount != 1 {
		t.Fatalf("manual balance transaction count=%d want 1", manualCount)
	}
	if _, err := service.settleRentObligation(ctx, otherUser.ID, obligation.ID, "跨用户校验"); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("cross-user settlement error=%v want record not found", err)
	}
	if _, err := service.revokeTransactionAllocations(ctx, owner.ID, created.ID, "manual balance correction", "manual-balance-revoke"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.listTransactionPageRows(ctx, owner.ID, transactionFilters{}); err != nil {
		t.Fatal(err)
	}
	var reallocatedCount int64
	if err := db.WithContext(ctx).Model(&paymentAllocation{}).Where("user_id = ? AND payment_transaction_id = ? AND status = ?", owner.ID, created.ID, allocationStatusConfirmed).Count(&reallocatedCount).Error; err != nil {
		t.Fatal(err)
	}
	if reallocatedCount != 0 {
		t.Fatalf("manual balance transaction was automatically reallocated after revoke: %d confirmed allocations", reallocatedCount)
	}
}

func TestManualBalanceConcurrentRequestsCreateOneTransactionOnMySQL(t *testing.T) {
	db, sqlDB := openLedgerMySQLTestDB(t)
	if err := runMigrations(sqlDB, "../../migrations"); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	ctx := context.Background()
	owner := user{Username: fmt.Sprintf("manual-balance-concurrent-%d", time.Now().UnixNano()), PasswordHash: "test"}
	if err := db.WithContext(ctx).Create(&owner).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.WithContext(ctx).Delete(&user{}, owner.ID).Error })

	input := validTenantInputForProfile()
	input.MonthlyRent = 400
	input.RentStartDate = "2026-01-01"
	input.BillingStartDate = "2026-01-01"
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
	service := newTransactionService(db)
	var wait sync.WaitGroup
	for index := 0; index < 2; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			_, err := service.settleRentObligation(ctx, owner.ID, obligation.ID, "并发补录")
			results <- err
		}()
	}
	close(start)
	wait.Wait()
	close(results)

	successes := 0
	notNeeded := 0
	for err := range results {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, errManualBalanceNotNeeded):
			notNeeded++
		default:
			t.Fatalf("concurrent settlement error=%v", err)
		}
	}
	if successes != 1 || notNeeded != 1 {
		t.Fatalf("concurrent results success=%d not-needed=%d want 1/1", successes, notNeeded)
	}
	var count int64
	if err := db.WithContext(ctx).Model(&paymentTransaction{}).Where("user_id = ? AND source = ?", owner.ID, manualBalanceTransactionSource).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("concurrent manual transaction count=%d want 1", count)
	}
	if err := db.WithContext(ctx).Where("id = ? AND user_id = ?", obligation.ID, owner.ID).First(&obligation).Error; err != nil {
		t.Fatal(err)
	}
	if obligation.PaidAmountCents != 40000 || obligation.Status != "paid" {
		t.Fatalf("concurrent settled obligation=%+v", obligation)
	}
}
