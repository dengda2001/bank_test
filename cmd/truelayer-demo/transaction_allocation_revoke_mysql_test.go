package main

import (
	"errors"
	"fmt"
	"testing"
	"time"
)

func TestRevokeRentAllocationPreservesOtherTenantShareOnMySQL(t *testing.T) {
	f := newRoomRentPlanConflictFixture(t)
	month := dublinCurrentMonth(time.Now())
	roommate := repositoryTestTenant(f.owner.ID, "Second occupant")
	if err := f.db.WithContext(f.ctx).Create(&roommate).Error; err != nil {
		t.Fatal(err)
	}
	if _, _, err := newRoomRentPlanService(f.db).SaveRoomRentPlan(f.ctx, SaveRoomRentPlanCommand{
		UserID: f.owner.ID, RoomID: f.roomOne.ID, EffectiveMonth: month,
		MonthlyRentCents: 120000, Currency: ledgerCurrencyEUR, DueDay: 5,
		Members: []RoomRentPlanMemberInput{
			{TenantID: f.tenant.ID, ResponsibilityCents: 60000},
			{TenantID: roommate.ID, ResponsibilityCents: 60000},
		},
	}); err != nil {
		t.Fatal(err)
	}
	if err := newMonthlyRentFactsService(f.db).ensureMonthlyRentFacts(f.ctx, f.owner.ID, month, rentFactsIntentExplicitPayment); err != nil {
		t.Fatal(err)
	}
	var obligations []rentObligation
	if err := f.db.WithContext(f.ctx).Where("user_id = ? AND period_month = ?", f.owner.ID, month).Order("tenant_id ASC").Find(&obligations).Error; err != nil || len(obligations) != 2 {
		t.Fatalf("obligations=%+v err=%v", obligations, err)
	}
	source := paymentTransaction{
		UserID: f.owner.ID, Source: "test", StableTransactionKey: fmt.Sprintf("share-revoke-%d", time.Now().UnixNano()),
		Direction: "income", AmountCents: 120000, Currency: ledgerCurrencyEUR,
		TransactionTime: &month, MatchStatus: "unmatched",
	}
	if err := f.db.WithContext(f.ctx).Create(&source).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = f.db.WithContext(f.ctx).Where("user_id = ?", f.owner.ID).Delete(&paymentTransactionAction{}).Error
		_ = f.db.WithContext(f.ctx).Where("user_id = ?", f.owner.ID).Delete(&paymentTransaction{}).Error
	})
	drafts := make([]transactionAllocationDraft, 0, 2)
	for _, obligation := range obligations {
		drafts = append(drafts, transactionAllocationDraft{TenantID: obligation.TenantID, RentObligationID: obligation.ID, AmountCents: 60000, Kind: allocationKindRent})
	}
	service := newTransactionService(f.db)
	if _, err := service.allocateTransaction(f.ctx, f.owner.ID, source.ID, drafts, "two-shares", "manual_review"); err != nil {
		t.Fatal(err)
	}
	var allocations []paymentAllocation
	if err := f.db.WithContext(f.ctx).Where("user_id = ? AND payment_transaction_id = ?", f.owner.ID, source.ID).Order("id ASC").Find(&allocations).Error; err != nil || len(allocations) != 2 {
		t.Fatalf("allocations=%+v err=%v", allocations, err)
	}
	if _, err := service.revokeRentAllocation(f.ctx, f.owner.ID, allocations[0].ID, source.ID, "revoke-first"); err != nil {
		t.Fatal(err)
	}
	if _, err := service.revokeRentAllocation(f.ctx, f.owner.ID, allocations[0].ID, source.ID, "revoke-first"); err != nil {
		t.Fatalf("same request key should be idempotent: %v", err)
	}
	if _, err := service.revokeRentAllocation(f.ctx, f.owner.ID, allocations[0].ID, source.ID, "new-request"); !errors.Is(err, ErrRentAllocationNotRevocable) {
		t.Fatalf("stale share err=%v", err)
	}
	if _, err := service.revokeRentAllocation(f.ctx, f.owner.ID+1, allocations[1].ID, source.ID, "foreign-owner"); !errors.Is(err, ErrRentAllocationNotRevocable) {
		t.Fatalf("foreign share err=%v", err)
	}
	if err := f.db.WithContext(f.ctx).Where("user_id = ? AND payment_transaction_id = ?", f.owner.ID, source.ID).Order("id ASC").Find(&allocations).Error; err != nil {
		t.Fatal(err)
	}
	if allocations[0].Status != allocationStatusVoided || allocations[1].Status != allocationStatusConfirmed {
		t.Fatalf("share statuses after exact revoke=%+v", allocations)
	}
	for _, obligation := range obligations {
		var current rentObligation
		if err := f.db.WithContext(f.ctx).Where("id = ? AND user_id = ?", obligation.ID, f.owner.ID).First(&current).Error; err != nil {
			t.Fatal(err)
		}
		want := int64(60000)
		if obligation.TenantID == *allocations[0].TenantID {
			want = 0
		}
		if current.PaidAmountCents != want {
			t.Fatalf("tenant %d paid=%d want=%d", obligation.TenantID, current.PaidAmountCents, want)
		}
	}
	var projected paymentTransaction
	if err := f.db.WithContext(f.ctx).Where("id = ? AND user_id = ?", source.ID, f.owner.ID).First(&projected).Error; err != nil {
		t.Fatal(err)
	}
	if projected.MatchStatus != "partial" {
		t.Fatalf("source match status=%q want partial", projected.MatchStatus)
	}
	var queueCount int64
	if err := rentWorkspacePendingTransactions(f.db, f.ctx, f.owner.ID, month).Count(&queueCount).Error; err != nil || queueCount != 1 {
		t.Fatalf("home pending count=%d err=%v", queueCount, err)
	}
	if _, err := service.revokeRentAllocation(f.ctx, f.owner.ID, allocations[1].ID, source.ID+1, "wrong-source"); !errors.Is(err, ErrRentAllocationNotRevocable) {
		t.Fatalf("wrong source err=%v", err)
	}
	if _, err := service.revokeRentAllocation(f.ctx, f.owner.ID, allocations[1].ID, source.ID, "revoke-last"); err != nil {
		t.Fatal(err)
	}
	if err := f.db.WithContext(f.ctx).Where("id = ? AND user_id = ?", source.ID, f.owner.ID).First(&projected).Error; err != nil {
		t.Fatal(err)
	}
	if projected.MatchStatus != "unmatched" {
		t.Fatalf("source match status after last share=%q want unmatched", projected.MatchStatus)
	}
}

func TestRevokeRentAllocationPreservesOtherMonthOnMySQL(t *testing.T) {
	f := newRoomRentPlanConflictFixture(t)
	firstMonth := dublinCurrentMonth(time.Now())
	secondMonth := firstMonth.AddDate(0, 1, 0)
	if _, _, err := newRoomRentPlanService(f.db).SaveRoomRentPlan(f.ctx, SaveRoomRentPlanCommand{
		UserID: f.owner.ID, RoomID: f.roomOne.ID, EffectiveMonth: firstMonth,
		MonthlyRentCents: 60000, Currency: ledgerCurrencyEUR, DueDay: 5,
		Members: []RoomRentPlanMemberInput{{TenantID: f.tenant.ID, ResponsibilityCents: 60000}},
	}); err != nil {
		t.Fatal(err)
	}
	for _, month := range []time.Time{firstMonth, secondMonth} {
		if err := newMonthlyRentFactsService(f.db).ensureMonthlyRentFacts(f.ctx, f.owner.ID, month, rentFactsIntentExplicitPayment); err != nil {
			t.Fatal(err)
		}
	}
	var obligations []rentObligation
	if err := f.db.WithContext(f.ctx).Where("user_id = ? AND tenant_id = ? AND period_month IN ?", f.owner.ID, f.tenant.ID, []time.Time{firstMonth, secondMonth}).Order("period_month ASC").Find(&obligations).Error; err != nil || len(obligations) != 2 {
		t.Fatalf("two months=%+v err=%v", obligations, err)
	}
	source := paymentTransaction{UserID: f.owner.ID, Source: "test", StableTransactionKey: fmt.Sprintf("two-month-share-%d", time.Now().UnixNano()), Direction: "income", AmountCents: 120000, Currency: ledgerCurrencyEUR, MatchStatus: "unmatched"}
	if err := f.db.WithContext(f.ctx).Create(&source).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = f.db.WithContext(f.ctx).Where("user_id = ?", f.owner.ID).Delete(&paymentTransactionAction{}).Error
		_ = f.db.WithContext(f.ctx).Where("user_id = ?", f.owner.ID).Delete(&paymentTransaction{}).Error
	})
	service := newTransactionService(f.db)
	if _, err := service.allocateTransaction(f.ctx, f.owner.ID, source.ID, []transactionAllocationDraft{
		{TenantID: f.tenant.ID, RentObligationID: obligations[0].ID, AmountCents: 60000, Kind: allocationKindRent},
		{TenantID: f.tenant.ID, RentObligationID: obligations[1].ID, AmountCents: 60000, Kind: allocationKindRent},
	}, "two-month-request", "manual_review"); err != nil {
		t.Fatal(err)
	}
	var shares []paymentAllocation
	if err := f.db.WithContext(f.ctx).Where("user_id = ? AND payment_transaction_id = ?", f.owner.ID, source.ID).Order("id ASC").Find(&shares).Error; err != nil || len(shares) != 2 {
		t.Fatalf("shares=%+v err=%v", shares, err)
	}
	if _, err := service.revokeRentAllocation(f.ctx, f.owner.ID, shares[0].ID, source.ID, "one-month-revoke"); err != nil {
		t.Fatal(err)
	}
	var first, second rentObligation
	if err := f.db.WithContext(f.ctx).First(&first, obligations[0].ID).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.db.WithContext(f.ctx).First(&second, obligations[1].ID).Error; err != nil {
		t.Fatal(err)
	}
	if first.PaidAmountCents != 0 || second.PaidAmountCents != 60000 {
		t.Fatalf("paid first=%d second=%d", first.PaidAmountCents, second.PaidAmountCents)
	}
}
