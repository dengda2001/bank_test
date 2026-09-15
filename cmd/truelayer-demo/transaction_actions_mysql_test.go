package main

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestTransactionActionsRevokeAndRestoreOnMySQL(t *testing.T) {
	db, sqlDB := openLedgerMySQLTestDB(t)
	if err := runMigrations(sqlDB, "../../migrations"); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	ctx := context.Background()
	owner := user{Username: fmt.Sprintf("transaction-actions-%d", time.Now().UnixNano()), PasswordHash: "test"}
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

	transaction := paymentTransaction{
		UserID:               owner.ID,
		Source:               "test",
		StableTransactionKey: fmt.Sprintf("transaction-actions-%d", time.Now().UnixNano()),
		Direction:            "income",
		AmountCents:          obligation.ExpectedAmountCents,
		Currency:             "EUR",
		MatchStatus:          "unmatched",
	}
	if err := db.WithContext(ctx).Create(&transaction).Error; err != nil {
		t.Fatal(err)
	}
	service := newTransactionService(db)
	if _, err := service.allocateTransaction(ctx, owner.ID, transaction.ID, []transactionAllocationDraft{{
		TenantID:         tenantRow.ID,
		RentObligationID: obligation.ID,
		AmountCents:      obligation.ExpectedAmountCents,
		Kind:             allocationKindRent,
	}}, "allocation-request", "manual"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.revokeTransactionAllocations(ctx, owner.ID, transaction.ID, "wrong rent month", "revoke-request"); err != nil {
		t.Fatal(err)
	}
	var revoked paymentAllocation
	if err := db.WithContext(ctx).Where("user_id = ? AND payment_transaction_id = ?", owner.ID, transaction.ID).First(&revoked).Error; err != nil {
		t.Fatal(err)
	}
	if revoked.Status != allocationStatusVoided || stringValue(revoked.VoidReason) != "wrong rent month" {
		t.Fatalf("allocation after revoke=%+v", revoked)
	}
	var projected rentObligation
	if err := db.WithContext(ctx).Where("id = ? AND user_id = ?", obligation.ID, owner.ID).First(&projected).Error; err != nil {
		t.Fatal(err)
	}
	if projected.PaidAmountCents != 0 {
		t.Fatalf("paid projection=%d want 0", projected.PaidAmountCents)
	}
	var revokedTransaction paymentTransaction
	if err := db.WithContext(ctx).Where("id = ? AND user_id = ?", transaction.ID, owner.ID).First(&revokedTransaction).Error; err != nil {
		t.Fatal(err)
	}
	if revokedTransaction.MatchStatus != "unmatched" || revokedTransaction.MatchedTenantID != nil {
		t.Fatalf("transaction after revoke=%+v", revokedTransaction)
	}

	ignoredTransaction := paymentTransaction{
		UserID:               owner.ID,
		Source:               "test",
		StableTransactionKey: fmt.Sprintf("transaction-ignore-%d", time.Now().UnixNano()),
		Direction:            "income",
		AmountCents:          100,
		Currency:             "EUR",
		MatchStatus:          "unmatched",
	}
	if err := db.WithContext(ctx).Create(&ignoredTransaction).Error; err != nil {
		t.Fatal(err)
	}
	if err := service.ignoreTransaction(ctx, owner.ID, ignoredTransaction.ID, "non-rent transfer", "ignore-request"); err != nil {
		t.Fatal(err)
	}
	if err := service.restoreTransaction(ctx, owner.ID, ignoredTransaction.ID, "needs review", "restore-request"); err != nil {
		t.Fatal(err)
	}
	var restored paymentTransaction
	if err := db.WithContext(ctx).Where("id = ? AND user_id = ?", ignoredTransaction.ID, owner.ID).First(&restored).Error; err != nil {
		t.Fatal(err)
	}
	if restored.MatchStatus != "unmatched" || restored.MatchReason != "needs review" {
		t.Fatalf("transaction after restore=%+v", restored)
	}
}
