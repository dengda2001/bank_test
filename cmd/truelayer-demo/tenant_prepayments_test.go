package main

import (
	"fmt"
	"os"
	"strings"
	"testing"
	"time"
)

func TestTenantPrepaymentMigrationUsesFlatRunnerSafeStatements(t *testing.T) {
	content, err := os.ReadFile("../../migrations/017_tenant_prepayments.sql")
	if err != nil {
		t.Fatal(err)
	}
	sqlText := string(content)
	if strings.Contains(sqlText, "--") || strings.Contains(sqlText, "/*") {
		t.Fatal("migration comments can break the flat SQL runner")
	}
	statements := splitSQLStatements(sqlText)
	if len(statements) != 2 || !strings.Contains(statements[0], "tenant_prepayments") || !strings.Contains(statements[1], "prepayment_id") {
		t.Fatalf("migration statements = %v", statements)
	}
}

func TestPrepaymentConsumesSourceButNotRent(t *testing.T) {
	tenantID := uint64(9)
	source := paymentTransaction{UserID: 1, Direction: "income", AmountCents: 62000, Currency: ledgerCurrencyEUR}
	rent := paymentAllocation{UserID: 1, TenantID: &tenantID, AmountCents: 60000, AllocationKind: allocationKindRent, Status: allocationStatusConfirmed}
	credit := paymentAllocation{UserID: 1, TenantID: &tenantID, AmountCents: 2000, AllocationKind: allocationKindPrepayment, Status: allocationStatusConfirmed}
	summary := summarizeTransactionAllocations(source, []paymentAllocation{rent, credit})
	if summary.Status != "matched" || summary.AllocatedCents != 62000 || summary.RemainingCents != 0 || ledgerPaidAmount([]paymentAllocation{rent, credit}) != 60000 {
		t.Fatalf("rent and credit projection = %+v", summary)
	}
	if err := validateLedgerAllocation(ledgerAllocationCheck{UserID: 1, SourceUserID: 1, TenantID: tenantID, SourceAmountCents: 62000, ExistingAllocatedCents: 60000, AmountCents: 2000, SourceCurrency: ledgerCurrencyEUR, Kind: allocationKindPrepayment}); err != nil {
		t.Fatalf("valid credit rejected: %v", err)
	}
	if err := validateLedgerAllocation(ledgerAllocationCheck{UserID: 1, SourceUserID: 1, TenantID: tenantID, SourceAmountCents: 62000, ExistingAllocatedCents: 60000, AmountCents: 2001, SourceCurrency: ledgerCurrencyEUR, Kind: allocationKindPrepayment}); err == nil {
		t.Fatal("source over-allocation was accepted")
	}
}

func TestTenantPrepaymentManualUseAndReversalOnMySQL(t *testing.T) {
	f := newRoomRentPlanConflictFixture(t)
	t.Cleanup(func() {
		_ = f.db.WithContext(f.ctx).Exec("DELETE FROM payment_transaction_actions WHERE user_id = ?", f.owner.ID).Error
		_ = f.db.WithContext(f.ctx).Exec("DELETE FROM payment_allocations WHERE user_id = ?", f.owner.ID).Error
		_ = f.db.WithContext(f.ctx).Exec("DELETE FROM tenant_prepayments WHERE user_id = ?", f.owner.ID).Error
		_ = f.db.WithContext(f.ctx).Exec("DELETE FROM payment_transactions WHERE user_id = ?", f.owner.ID).Error
	})
	month := dublinCurrentMonth(time.Now())
	if _, _, err := newRoomRentPlanService(f.db).SaveRoomRentPlan(f.ctx, SaveRoomRentPlanCommand{
		UserID: f.owner.ID, RoomID: f.roomOne.ID, EffectiveMonth: month,
		MonthlyRentCents: 60000, Currency: ledgerCurrencyEUR, DueDay: 1,
		Members: []RoomRentPlanMemberInput{{TenantID: f.tenant.ID, ResponsibilityCents: 60000}},
	}); err != nil {
		t.Fatal(err)
	}
	source := paymentTransaction{UserID: f.owner.ID, Source: "test", StableTransactionKey: fmt.Sprintf("prepay-%d", time.Now().UnixNano()), Direction: "income", AmountCents: 62000, Currency: ledgerCurrencyEUR, MatchStatus: "unmatched"}
	if err := f.db.WithContext(f.ctx).Create(&source).Error; err != nil {
		t.Fatal(err)
	}
	service := newTransactionService(f.db)
	items := []rentMatchBatchItem{{TenantID: f.tenant.ID, Period: month.Format("2006-01"), AmountCents: 60000}}
	summary, err := service.confirmRentMatchBatchWithPrepayment(f.ctx, f.owner.ID, source.ID, items, 0, f.tenant.ID, 2000, "credit-batch")
	if err != nil || summary.Status != "matched" || summary.RemainingCents != 0 {
		t.Fatalf("confirm = %+v, %v", summary, err)
	}
	var credit tenantPrepayment
	if err := f.db.WithContext(f.ctx).Where("user_id = ? AND tenant_id = ?", f.owner.ID, f.tenant.ID).First(&credit).Error; err != nil {
		t.Fatal(err)
	}
	var current rentObligation
	if err := f.db.WithContext(f.ctx).Where("user_id = ? AND tenant_id = ? AND period_month = ?", f.owner.ID, f.tenant.ID, month).First(&current).Error; err != nil {
		t.Fatal(err)
	}
	if current.PaidAmountCents != 60000 {
		t.Fatalf("current paid = %d", current.PaidAmountCents)
	}
	if _, err := service.confirmRentMatchBatchWithPrepayment(f.ctx, f.owner.ID, source.ID, items, 0, f.tenant.ID, 2000, "credit-batch"); err != nil {
		t.Fatalf("retry: %v", err)
	}
	if _, err := service.confirmRentMatchBatchWithPrepayment(f.ctx, f.owner.ID, source.ID, items, 0, f.tenant.ID, 1999, "credit-batch"); err == nil {
		t.Fatal("changed credit amount accepted with same key")
	}
	next := month.AddDate(0, 1, 0)
	if err := newMonthlyRentFactsService(f.db).ensureMonthlyRentFacts(f.ctx, f.owner.ID, next, rentFactsIntentExplicitPayment); err != nil {
		t.Fatal(err)
	}
	var nextBill rentObligation
	if err := f.db.WithContext(f.ctx).Where("user_id = ? AND tenant_id = ? AND period_month = ?", f.owner.ID, f.tenant.ID, next).First(&nextBill).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := service.applyTenantPrepayment(f.ctx, f.owner.ID, credit.ID, nextBill.ID, 1200, "credit-use-1"); err != nil {
		t.Fatal(err)
	}
	var rows []paymentAllocation
	if err := f.db.WithContext(f.ctx).Where("user_id = ? AND prepayment_id = ?", f.owner.ID, credit.ID).Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if available := prepaymentAvailableCents(rows, credit.ID); available != 800 {
		t.Fatalf("available = %d", available)
	}
	if _, err := service.applyTenantPrepayment(f.ctx, f.owner.ID, credit.ID, nextBill.ID, 1200, "credit-use-1"); err != nil {
		t.Fatalf("retry apply: %v", err)
	}
	if _, err := service.applyTenantPrepayment(f.ctx, f.owner.ID, credit.ID, nextBill.ID, 801, "credit-use-2"); err == nil {
		t.Fatal("overspend accepted")
	}
	var applied paymentAllocation
	if err := f.db.WithContext(f.ctx).Where("user_id = ? AND idempotency_key = ?", f.owner.ID, "prepayment-use:credit-use-1").First(&applied).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := service.revokeRentAllocation(f.ctx, f.owner.ID, applied.ID, source.ID, "revoke-credit-use-1"); err != nil {
		t.Fatal(err)
	}
	if err := f.db.WithContext(f.ctx).Where("user_id = ? AND prepayment_id = ?", f.owner.ID, credit.ID).Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if available := prepaymentAvailableCents(rows, credit.ID); available != 2000 {
		t.Fatalf("available after revoke = %d", available)
	}
	if _, err := service.revokeTransactionAllocations(f.ctx, f.owner.ID, source.ID, "撤销整笔流水", "revoke-credit-source"); err != nil {
		t.Fatal(err)
	}
	if err := f.db.WithContext(f.ctx).Where("user_id = ? AND prepayment_id = ?", f.owner.ID, credit.ID).Find(&rows).Error; err != nil {
		t.Fatal(err)
	}
	if available := prepaymentAvailableCents(rows, credit.ID); available != 0 {
		t.Fatalf("available after source revoke = %d", available)
	}
}
