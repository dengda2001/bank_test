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

func TestTransactionActionsRematchOneRentAllocationOnMySQL(t *testing.T) {
	db, sqlDB := openLedgerMySQLTestDB(t)
	if err := runMigrations(sqlDB, "../../migrations"); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	ctx := context.Background()
	owner := user{Username: fmt.Sprintf("transaction-rematch-%d", time.Now().UnixNano()), PasswordHash: "test"}
	if err := db.WithContext(ctx).Create(&owner).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.WithContext(ctx).Delete(&user{}, owner.ID).Error })

	input := validTenantInputForProfile()
	tenantRow, err := newTenantService(db).createTenant(ctx, owner.ID, input)
	if err != nil {
		t.Fatal(err)
	}
	periods := []time.Time{
		time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
	}
	for _, period := range periods {
		if err := newObligationService(db).ensureMonthlyObligations(ctx, owner.ID, period); err != nil {
			t.Fatal(err)
		}
	}
	obligations := make(map[time.Time]rentObligation, len(periods))
	for _, period := range periods {
		var obligation rentObligation
		if err := db.WithContext(ctx).Where("user_id = ? AND tenant_id = ? AND period_month = ?", owner.ID, tenantRow.ID, period).First(&obligation).Error; err != nil {
			t.Fatal(err)
		}
		obligations[period] = obligation
	}

	september := obligations[periods[0]]
	october := obligations[periods[1]]
	transaction := paymentTransaction{
		UserID:               owner.ID,
		Source:               "test",
		StableTransactionKey: fmt.Sprintf("transaction-rematch-source-%d", time.Now().UnixNano()),
		Direction:            "income",
		AmountCents:          september.ExpectedAmountCents,
		Currency:             "EUR",
		MatchStatus:          "unmatched",
	}
	if err := db.WithContext(ctx).Create(&transaction).Error; err != nil {
		t.Fatal(err)
	}
	service := newTransactionService(db)
	if _, err := service.allocateTransaction(ctx, owner.ID, transaction.ID, []transactionAllocationDraft{{
		TenantID:         tenantRow.ID,
		RentObligationID: september.ID,
		AmountCents:      september.ExpectedAmountCents,
		Kind:             allocationKindRent,
	}}, "rematch-initial", "manual"); err != nil {
		t.Fatal(err)
	}
	if err := service.rematchRentAllocation(ctx, owner.ID, transaction.ID, tenantRow.ID, periods[1]); err != nil {
		t.Fatal(err)
	}

	var allocations []paymentAllocation
	if err := db.WithContext(ctx).Where("user_id = ? AND payment_transaction_id = ?", owner.ID, transaction.ID).Order("id ASC").Find(&allocations).Error; err != nil {
		t.Fatal(err)
	}
	if len(allocations) != 2 {
		t.Fatalf("allocation count=%d want 2", len(allocations))
	}
	if allocations[0].Status != allocationStatusVoided || stringValue(allocations[0].VoidReason) != "修改匹配" || allocations[0].RentObligationIDValue() != september.ID {
		t.Fatalf("old allocation=%+v", allocations[0])
	}
	if allocations[1].Status != allocationStatusConfirmed || allocations[1].ConfirmationSource != "manual_rematch" || allocations[1].RentObligationIDValue() != october.ID || allocations[1].AmountCents != september.ExpectedAmountCents {
		t.Fatalf("new allocation=%+v", allocations[1])
	}
	for _, expected := range []struct {
		obligation rentObligation
		paid       int64
	}{
		{obligation: september, paid: 0},
		{obligation: october, paid: october.ExpectedAmountCents},
	} {
		var projected rentObligation
		if err := db.WithContext(ctx).Where("id = ? AND user_id = ?", expected.obligation.ID, owner.ID).First(&projected).Error; err != nil {
			t.Fatal(err)
		}
		if projected.PaidAmountCents != expected.paid {
			t.Fatalf("obligation %d paid=%d want %d", projected.ID, projected.PaidAmountCents, expected.paid)
		}
	}
	var action paymentTransactionAction
	if err := db.WithContext(ctx).Where("user_id = ? AND payment_transaction_id = ? AND action_kind = ?", owner.ID, transaction.ID, transactionActionRevokeAllocations).First(&action).Error; err != nil {
		t.Fatal(err)
	}
	if action.Reason != "修改匹配" {
		t.Fatalf("rematch audit action=%+v", action)
	}

	november := obligations[periods[2]]
	december := obligations[periods[3]]
	splitTransaction := paymentTransaction{
		UserID:               owner.ID,
		Source:               "test",
		StableTransactionKey: fmt.Sprintf("transaction-rematch-split-%d", time.Now().UnixNano()),
		Direction:            "income",
		AmountCents:          november.ExpectedAmountCents + december.ExpectedAmountCents,
		Currency:             "EUR",
		MatchStatus:          "unmatched",
	}
	if err := db.WithContext(ctx).Create(&splitTransaction).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := service.allocateTransaction(ctx, owner.ID, splitTransaction.ID, []transactionAllocationDraft{
		{TenantID: tenantRow.ID, RentObligationID: november.ID, AmountCents: november.ExpectedAmountCents, Kind: allocationKindRent},
		{TenantID: tenantRow.ID, RentObligationID: december.ID, AmountCents: december.ExpectedAmountCents, Kind: allocationKindRent},
	}, "rematch-split", "manual"); err != nil {
		t.Fatal(err)
	}
	if err := service.rematchRentAllocation(ctx, owner.ID, splitTransaction.ID, tenantRow.ID, periods[4]); err == nil {
		t.Fatal("split transaction rematch unexpectedly succeeded")
	}
}
