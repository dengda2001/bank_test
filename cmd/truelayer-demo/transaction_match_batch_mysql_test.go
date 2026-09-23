package main

import (
	"fmt"
	"testing"
	"time"
)

func TestConfirmRentMatchBatchAcrossTenantsAndMonthsOnMySQL(t *testing.T) {
	f := newRoomRentPlanConflictFixture(t)
	t.Cleanup(func() {
		_ = f.db.WithContext(f.ctx).Exec("DELETE FROM payment_transaction_actions WHERE user_id = ?", f.owner.ID).Error
		_ = f.db.WithContext(f.ctx).Exec("DELETE FROM payment_transactions WHERE user_id = ?", f.owner.ID).Error
	})
	otherTenant := repositoryTestTenant(f.owner.ID, "Other roommate")
	if err := f.db.WithContext(f.ctx).Create(&otherTenant).Error; err != nil {
		t.Fatal(err)
	}
	month := dublinCurrentMonth(time.Now())
	if _, _, err := newRoomRentPlanService(f.db).SaveRoomRentPlan(f.ctx, SaveRoomRentPlanCommand{
		UserID: f.owner.ID, RoomID: f.roomOne.ID, EffectiveMonth: month,
		MonthlyRentCents: 120000, Currency: ledgerCurrencyEUR, DueDay: 1,
		Members: []RoomRentPlanMemberInput{
			{TenantID: f.tenant.ID, ResponsibilityCents: 60000},
			{TenantID: otherTenant.ID, ResponsibilityCents: 60000},
		},
	}); err != nil {
		t.Fatal(err)
	}
	payerName := "Shared tenant"
	createSource := func(suffix string) paymentTransaction {
		t.Helper()
		source := paymentTransaction{
			UserID: f.owner.ID, Source: "test",
			StableTransactionKey: fmt.Sprintf("batch-review-%d-%s", time.Now().UnixNano(), suffix),
			Direction:            "income", AmountCents: 120000, Currency: ledgerCurrencyEUR,
			PayerName: &payerName, MatchStatus: "unmatched",
		}
		if err := f.db.WithContext(f.ctx).Create(&source).Error; err != nil {
			t.Fatal(err)
		}
		return source
	}
	assertCount := func(source paymentTransaction, want int64) {
		t.Helper()
		var count int64
		if err := f.db.WithContext(f.ctx).Model(&paymentAllocation{}).Where("user_id = ? AND payment_transaction_id = ? AND status = ?", f.owner.ID, source.ID, allocationStatusConfirmed).Count(&count).Error; err != nil {
			t.Fatal(err)
		}
		if count != want {
			t.Fatalf("source %d allocation count = %d, want %d", source.ID, count, want)
		}
	}
	service := newTransactionService(f.db)
	first := createSource("roommates")
	sameMonth := month.Format("2006-01")
	roommates := []rentMatchBatchItem{
		{TenantID: f.tenant.ID, Period: sameMonth, AmountCents: 60000},
		{TenantID: otherTenant.ID, Period: sameMonth, AmountCents: 60000},
	}
	summary, err := service.confirmRentMatchBatch(f.ctx, f.owner.ID, first.ID, roommates, f.tenant.ID, "batch-roommates")
	if err != nil || summary.Status != "matched" || summary.RemainingCents != 0 {
		t.Fatalf("roommate batch = %+v, %v", summary, err)
	}
	assertCount(first, 2)
	if _, err := service.confirmRentMatchBatch(f.ctx, f.owner.ID, first.ID, roommates, f.tenant.ID, "batch-roommates"); err != nil {
		t.Fatalf("idempotent retry: %v", err)
	}
	assertCount(first, 2)
	changed := []rentMatchBatchItem{{TenantID: f.tenant.ID, Period: sameMonth, AmountCents: 50000}, {TenantID: otherTenant.ID, Period: sameMonth, AmountCents: 70000}}
	if _, err := service.confirmRentMatchBatch(f.ctx, f.owner.ID, first.ID, changed, f.tenant.ID, "batch-roommates"); err == nil {
		t.Fatal("same request key accepted different amounts")
	}
	var payerCount int64
	if err := f.db.WithContext(f.ctx).Model(&tenantPayer{}).Where("user_id = ? AND tenant_id = ? AND removed_at IS NULL", f.owner.ID, f.tenant.ID).Count(&payerCount).Error; err != nil || payerCount != 1 {
		t.Fatalf("payer association for actual payer = %d, %v", payerCount, err)
	}
	if err := f.db.WithContext(f.ctx).Model(&tenantPayer{}).Where("user_id = ? AND tenant_id = ? AND removed_at IS NULL", f.owner.ID, otherTenant.ID).Count(&payerCount).Error; err != nil || payerCount != 0 {
		t.Fatalf("roommate was incorrectly remembered as payer = %d, %v", payerCount, err)
	}

	next := month.AddDate(0, 1, 0)
	afterNext := month.AddDate(0, 2, 0)
	second := createSource("two-months")
	twoMonths := []rentMatchBatchItem{
		{TenantID: f.tenant.ID, Period: next.Format("2006-01"), AmountCents: 60000},
		{TenantID: f.tenant.ID, Period: afterNext.Format("2006-01"), AmountCents: 60000},
	}
	summary, err = service.confirmRentMatchBatch(f.ctx, f.owner.ID, second.ID, twoMonths, 0, "batch-two-months")
	if err != nil || summary.Status != "matched" || summary.RemainingCents != 0 {
		t.Fatalf("two-month batch = %+v, %v", summary, err)
	}
	assertCount(second, 2)
	for _, targetMonth := range []time.Time{next, afterNext} {
		var obligation rentObligation
		if err := f.db.WithContext(f.ctx).Where("user_id = ? AND tenant_id = ? AND period_month = ?", f.owner.ID, f.tenant.ID, targetMonth).First(&obligation).Error; err != nil {
			t.Fatal(err)
		}
		if obligation.PaidAmountCents != 60000 {
			t.Fatalf("%s paid = %d, want 60000", targetMonth.Format("2006-01"), obligation.PaidAmountCents)
		}
	}

	third := createSource("partial")
	future := month.AddDate(0, 3, 0).Format("2006-01")
	summary, err = service.confirmRentMatchBatch(f.ctx, f.owner.ID, third.ID, []rentMatchBatchItem{{TenantID: f.tenant.ID, Period: future, AmountCents: 60000}}, 0, "batch-partial-one")
	if err != nil || summary.Status != "partial" || summary.RemainingCents != 60000 {
		t.Fatalf("first partial = %+v, %v", summary, err)
	}
	summary, err = service.confirmRentMatchBatch(f.ctx, f.owner.ID, third.ID, []rentMatchBatchItem{{TenantID: otherTenant.ID, Period: future, AmountCents: 60000}}, 0, "batch-partial-two")
	if err != nil || summary.Status != "matched" || summary.RemainingCents != 0 {
		t.Fatalf("partial continuation = %+v, %v", summary, err)
	}
	assertCount(third, 2)

	fourth := createSource("over-budget")
	overBudgetMonth := month.AddDate(0, 4, 0).Format("2006-01")
	if _, err := service.confirmRentMatchBatch(f.ctx, f.owner.ID, fourth.ID, []rentMatchBatchItem{
		{TenantID: f.tenant.ID, Period: overBudgetMonth, AmountCents: 60000},
		{TenantID: otherTenant.ID, Period: overBudgetMonth, AmountCents: 70000},
	}, 0, "batch-over-budget"); err == nil {
		t.Fatal("over-budget batch succeeded")
	}
	assertCount(fourth, 0)
}
