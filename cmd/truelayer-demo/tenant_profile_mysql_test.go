package main

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestTenantProfileMigrationIsIdempotentOnMySQL(t *testing.T) {
	db, sqlDB := openLedgerMySQLTestDB(t)
	if err := runMigrations(sqlDB, "../../migrations"); err != nil {
		t.Fatalf("first migration run: %v", err)
	}
	if err := runMigrations(sqlDB, "../../migrations"); err != nil {
		t.Fatalf("second migration run: %v", err)
	}
	var tableCount int
	if err := db.Raw(`SELECT COUNT(*) FROM information_schema.TABLES WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'tenant_payers'`).Scan(&tableCount).Error; err != nil {
		t.Fatal(err)
	}
	if tableCount != 1 {
		t.Fatalf("tenant_payers table count=%d want 1", tableCount)
	}
	var payerNameNullable string
	if err := db.Raw(`SELECT IS_NULLABLE FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'tenant_payers' AND COLUMN_NAME = 'payer_name_original'`).Scan(&payerNameNullable).Error; err != nil {
		t.Fatal(err)
	}
	if payerNameNullable != "NO" {
		t.Fatalf("payer_name_original nullable=%s want NO", payerNameNullable)
	}
	var payerIDNullable string
	if err := db.Raw(`SELECT IS_NULLABLE FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'tenant_payers' AND COLUMN_NAME = 'payer_id'`).Scan(&payerIDNullable).Error; err != nil {
		t.Fatal(err)
	}
	if payerIDNullable != "YES" {
		t.Fatalf("payer_id nullable=%s want YES", payerIDNullable)
	}
}

func TestTenantLifecycleVoidsUnpaidFutureBillsButRejectsPaidOnMySQL(t *testing.T) {
	db, sqlDB := openLedgerMySQLTestDB(t)
	if err := runMigrations(sqlDB, "../../migrations"); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	ctx := context.Background()
	owner := user{Username: fmt.Sprintf("tenant-profile-%d", time.Now().UnixNano()), PasswordHash: "test"}
	if err := db.WithContext(ctx).Create(&owner).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.WithContext(ctx).Delete(&user{}, owner.ID) })
	service := newTenantService(db)
	input := validTenantInputForProfile()
	input.RentStartDate = "2026-01-01"
	input.BillingStartDate = "2026-01-01"
	input.RentEndDate = ""
	created, err := service.createTenant(ctx, owner.ID, input)
	if err != nil {
		t.Fatal(err)
	}
	obligations := newObligationService(db)
	if err := obligations.generateMonthlyObligations(ctx, owner.ID, time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	input.RentEndDate = "2026-10-02"
	if _, err := service.updateTenant(ctx, owner.ID, created.ID, input); err != nil {
		t.Fatalf("unpaid future bills should be voidable: %v", err)
	}
	var voided int64
	if err := db.Model(&rentObligation{}).Where("user_id = ? AND tenant_id = ? AND record_status = ?", owner.ID, created.ID, obligationRecordVoided).Count(&voided).Error; err != nil {
		t.Fatal(err)
	}
	if voided != 2 {
		t.Fatalf("voided future bills=%d want 2", voided)
	}

	paidInput := validTenantInputForProfile()
	paidInput.Name = "Paid Future Tenant"
	paidInput.RentStartDate = "2026-01-01"
	paidInput.BillingStartDate = "2026-01-01"
	paidTenant, err := service.createTenant(ctx, owner.ID, paidInput)
	if err != nil {
		t.Fatal(err)
	}
	if err := obligations.ensureMonthlyObligations(ctx, owner.ID, time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	var future rentObligation
	if err := db.Where("user_id = ? AND tenant_id = ? AND period_month = ?", owner.ID, paidTenant.ID, time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)).First(&future).Error; err != nil {
		t.Fatal(err)
	}
	transaction := paymentTransaction{UserID: owner.ID, Source: "test", StableTransactionKey: fmt.Sprintf("tenant-profile-%d", time.Now().UnixNano()), Direction: "income", AmountCents: 95000, Currency: "EUR", MatchStatus: "matched"}
	if err := db.Create(&transaction).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Create(&paymentAllocation{UserID: owner.ID, PaymentTransactionID: transaction.ID, RentObligationID: ptrUint64(future.ID), TenantID: ptrUint64(paidTenant.ID), AmountCents: 95000, AllocationKind: allocationKindRent, Status: allocationStatusConfirmed, ConfirmedAt: time.Now().UTC(), ConfirmedByUserID: owner.ID, ConfirmationSource: "manual"}).Error; err != nil {
		t.Fatal(err)
	}
	paidInput.RentEndDate = "2026-10-02"
	if _, err := service.updateTenant(ctx, owner.ID, paidTenant.ID, paidInput); err == nil {
		t.Fatal("expected early rent end with effective future payment to fail")
	}
	var stillActive int64
	if err := db.Model(&rentObligation{}).Where("user_id = ? AND tenant_id = ? AND period_month = ? AND record_status = ?", owner.ID, paidTenant.ID, time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC), obligationRecordActive).Count(&stillActive).Error; err != nil {
		t.Fatal(err)
	}
	if stillActive != 1 {
		t.Fatal("paid future obligation was partially changed after rejected update")
	}
}
