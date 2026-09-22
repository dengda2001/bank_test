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

func TestRentFactsMaterializationPolicyKeepsFutureReadsAsPreviews(t *testing.T) {
	now := time.Date(2026, 9, 21, 12, 0, 0, 0, time.UTC)
	past := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	current := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	future := time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC)

	if !rentFactsCanMaterialize(past, now, rentFactsIntentRead) {
		t.Fatal("historical read should materialize stable rent facts")
	}
	if !rentFactsCanMaterialize(current, now, rentFactsIntentRead) {
		t.Fatal("current month read should materialize stable rent facts")
	}
	if rentFactsCanMaterialize(future, now, rentFactsIntentRead) {
		t.Fatal("ordinary future month read must remain a preview")
	}
	if !rentFactsCanMaterialize(future, now, rentFactsIntentExplicitPayment) {
		t.Fatal("explicit allocation to a future month should materialize its facts")
	}
}

func TestEnsureMonthlyRentFactsGeneratesRoomPlanFactsOnceOnMySQL(t *testing.T) {
	db, sqlDB := openLedgerMySQLTestDB(t)
	if err := runMigrations(sqlDB, "../../migrations"); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	ctx := context.Background()
	owner := user{Username: fmt.Sprintf("monthly-facts-owner-%d", time.Now().UnixNano()), PasswordHash: "test"}
	if err := db.WithContext(ctx).Create(&owner).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.WithContext(ctx).Delete(&user{}, owner.ID).Error })

	firstTenant := repositoryTestTenant(owner.ID, "First room-plan tenant")
	secondTenant := repositoryTestTenant(owner.ID, "Second room-plan tenant")
	for _, row := range []*tenant{&firstTenant, &secondTenant} {
		if err := db.WithContext(ctx).Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}

	domain := newLandlordDomainService(db)
	propertyRow, err := domain.createProperty(ctx, owner.ID, propertyInput{Name: "Monthly facts property"})
	if err != nil {
		t.Fatal(err)
	}
	roomRow, err := domain.createRoom(ctx, owner.ID, roomInput{PropertyID: propertyRow.ID, RoomLabel: "Room 1"})
	if err != nil {
		t.Fatal(err)
	}
	period := monthStart(time.Now().UTC())
	plan, _, err := newRoomRentPlanService(db).SaveRoomRentPlan(ctx, SaveRoomRentPlanCommand{
		UserID: owner.ID, RoomID: roomRow.ID, EffectiveMonth: period,
		MonthlyRentCents: 100000, Currency: ledgerCurrencyEUR, DueDay: 5,
		Members: []RoomRentPlanMemberInput{
			{TenantID: firstTenant.ID, ResponsibilityCents: 60000},
			{TenantID: secondTenant.ID, ResponsibilityCents: 40000},
		},
	})
	if err != nil {
		t.Fatal(err)
	}

	var charge rentCharge
	if err := db.WithContext(ctx).Where("user_id = ? AND room_id = ? AND period_month = ?", owner.ID, roomRow.ID, period).First(&charge).Error; err != nil {
		t.Fatal(err)
	}
	if charge.RoomRentPlanID != plan.ID || charge.ExpectedAmountCents != 100000 {
		t.Fatalf("room charge=%+v; want plan %d and 100000 cents", charge, plan.ID)
	}
	var obligations []rentObligation
	if err := db.WithContext(ctx).Where("user_id = ? AND rent_charge_id = ?", owner.ID, charge.ID).Order("tenant_id ASC").Find(&obligations).Error; err != nil {
		t.Fatal(err)
	}
	if len(obligations) != 2 || obligations[0].ExpectedAmountCents+obligations[1].ExpectedAmountCents != 100000 {
		t.Fatalf("room obligations=%+v; want two responsibilities summing to room rent", obligations)
	}
	for _, obligation := range obligations {
		if obligation.RoomRentPlanID != plan.ID || obligation.RoomRentPlanMemberID == 0 {
			t.Fatalf("obligation is not linked to its plan member: %+v", obligation)
		}
	}

	// Remove the first materialization so concurrent readers exercise the creation path.
	if err := db.WithContext(ctx).Where("user_id = ? AND rent_charge_id = ?", owner.ID, charge.ID).Delete(&rentObligation{}).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.WithContext(ctx).Where("user_id = ? AND id = ?", owner.ID, charge.ID).Delete(&rentCharge{}).Error; err != nil {
		t.Fatal(err)
	}
	service := newMonthlyRentFactsService(db)
	var wait sync.WaitGroup
	errs := make(chan error, 4)
	for index := 0; index < cap(errs); index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			errs <- service.ensureMonthlyRentFacts(ctx, owner.ID, period, rentFactsIntentRead)
		}()
	}
	wait.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent ensure: %v", err)
		}
	}
	var chargeCount, obligationCount int64
	if err := db.WithContext(ctx).Model(&rentCharge{}).Where("user_id = ? AND room_id = ? AND period_month = ?", owner.ID, roomRow.ID, period).Count(&chargeCount).Error; err != nil {
		t.Fatal(err)
	}
	var rebuilt rentCharge
	if err := db.WithContext(ctx).Where("user_id = ? AND room_id = ? AND period_month = ?", owner.ID, roomRow.ID, period).First(&rebuilt).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.WithContext(ctx).Model(&rentObligation{}).Where("user_id = ? AND rent_charge_id = ?", owner.ID, rebuilt.ID).Count(&obligationCount).Error; err != nil {
		t.Fatal(err)
	}
	if chargeCount != 1 || obligationCount != 2 {
		t.Fatalf("concurrent facts: charges=%d obligations=%d; want 1 and 2", chargeCount, obligationCount)
	}
}

func TestEnsureMonthlyRentFactsDoesNotPersistFutureReadButAllowsExplicitPaymentOnMySQL(t *testing.T) {
	db, sqlDB := openLedgerMySQLTestDB(t)
	if err := runMigrations(sqlDB, "../../migrations"); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	ctx := context.Background()
	owner := user{Username: fmt.Sprintf("future-monthly-facts-owner-%d", time.Now().UnixNano()), PasswordHash: "test"}
	if err := db.WithContext(ctx).Create(&owner).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.WithContext(ctx).Delete(&user{}, owner.ID).Error })
	tenantRow := repositoryTestTenant(owner.ID, "Future tenant")
	if err := db.WithContext(ctx).Create(&tenantRow).Error; err != nil {
		t.Fatal(err)
	}
	domain := newLandlordDomainService(db)
	propertyRow, err := domain.createProperty(ctx, owner.ID, propertyInput{Name: "Future facts property"})
	if err != nil {
		t.Fatal(err)
	}
	roomRow, err := domain.createRoom(ctx, owner.ID, roomInput{PropertyID: propertyRow.ID, RoomLabel: "Room 1"})
	if err != nil {
		t.Fatal(err)
	}
	future := monthStart(time.Now().UTC()).AddDate(0, 1, 0)
	if _, _, err := newRoomRentPlanService(db).SaveRoomRentPlan(ctx, SaveRoomRentPlanCommand{
		UserID: owner.ID, RoomID: roomRow.ID, EffectiveMonth: future,
		MonthlyRentCents: 50000, Currency: ledgerCurrencyEUR, DueDay: 5,
		Members: []RoomRentPlanMemberInput{{TenantID: tenantRow.ID, ResponsibilityCents: 50000}},
	}); err != nil {
		t.Fatal(err)
	}

	service := newMonthlyRentFactsService(db)
	if err := service.ensureMonthlyRentFacts(ctx, owner.ID, future, rentFactsIntentRead); err != nil {
		t.Fatal(err)
	}
	var count int64
	if err := db.WithContext(ctx).Model(&rentCharge{}).Where("user_id = ? AND room_id = ? AND period_month = ?", owner.ID, roomRow.ID, future).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("future read created %d rent charges; want none", count)
	}
	if err := service.ensureMonthlyRentFacts(ctx, owner.ID, future, rentFactsIntentExplicitPayment); err != nil {
		t.Fatal(err)
	}
	if err := db.WithContext(ctx).Model(&rentCharge{}).Where("user_id = ? AND room_id = ? AND period_month = ?", owner.ID, roomRow.ID, future).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("explicit future payment created %d rent charges; want one", count)
	}
}

func TestSaveRoomRentPlanReplacesUnlockedFactsAndRejectsCashLockedFactsOnMySQL(t *testing.T) {
	db, sqlDB := openLedgerMySQLTestDB(t)
	if err := runMigrations(sqlDB, "../../migrations"); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	ctx := context.Background()
	owner := user{Username: fmt.Sprintf("room-plan-lock-owner-%d", time.Now().UnixNano()), PasswordHash: "test"}
	if err := db.WithContext(ctx).Create(&owner).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.WithContext(ctx).Delete(&user{}, owner.ID).Error })
	tenantRow := repositoryTestTenant(owner.ID, "Room-plan lock tenant")
	if err := db.WithContext(ctx).Create(&tenantRow).Error; err != nil {
		t.Fatal(err)
	}
	domain := newLandlordDomainService(db)
	propertyRow, err := domain.createProperty(ctx, owner.ID, propertyInput{Name: "Room-plan lock property"})
	if err != nil {
		t.Fatal(err)
	}
	roomRow, err := domain.createRoom(ctx, owner.ID, roomInput{PropertyID: propertyRow.ID, RoomLabel: "Room 1"})
	if err != nil {
		t.Fatal(err)
	}
	period := monthStart(time.Now().UTC())
	plans := newRoomRentPlanService(db)
	command := SaveRoomRentPlanCommand{
		UserID: owner.ID, RoomID: roomRow.ID, EffectiveMonth: period,
		MonthlyRentCents: 100000, Currency: ledgerCurrencyEUR, DueDay: 5,
		Members: []RoomRentPlanMemberInput{{TenantID: tenantRow.ID, ResponsibilityCents: 100000}},
	}
	_, version, err := plans.SaveRoomRentPlan(ctx, command)
	if err != nil {
		t.Fatal(err)
	}

	command.ExpectedTimelineVersion = version
	command.MonthlyRentCents = 110000
	command.Members = []RoomRentPlanMemberInput{{TenantID: tenantRow.ID, ResponsibilityCents: 110000}}
	updated, version, err := plans.SaveRoomRentPlan(ctx, command)
	if err != nil {
		t.Fatalf("rewrite unlocked current-month facts: %v", err)
	}
	if updated.MonthlyRentCents != 110000 || version != 2 {
		t.Fatalf("updated plan/version=%+v/%d; want 110000 cents/version 2", updated, version)
	}

	var obligation rentObligation
	if err := db.WithContext(ctx).Where("user_id = ? AND period_month = ? AND tenant_id = ?", owner.ID, period, tenantRow.ID).First(&obligation).Error; err != nil {
		t.Fatal(err)
	}
	payerID := tenantRow.ID
	receipt := cashReceipt{
		UserID: owner.ID, PayerTenantID: &payerID, RentObligationID: obligation.ID,
		ReceiptNumber: recordID("cash-test", time.Now().UTC()), AmountCents: 1000,
		Currency: ledgerCurrencyEUR, ReceivedAt: dateOnly(time.Now()), Status: cashReceiptStatusVoided,
		OperationID: recordID("cash-op", time.Now().UTC()), RecordedByUserID: owner.ID, RecordedAt: time.Now().UTC(),
	}
	if err := db.WithContext(ctx).Create(&receipt).Error; err != nil {
		t.Fatal(err)
	}

	command.ExpectedTimelineVersion = version
	command.MonthlyRentCents = 120000
	command.Members = []RoomRentPlanMemberInput{{TenantID: tenantRow.ID, ResponsibilityCents: 120000}}
	if _, _, err := plans.SaveRoomRentPlan(ctx, command); !errors.Is(err, ErrRentPlanFactsLocked) {
		t.Fatalf("rewrite cash-locked facts error=%v; want ErrRentPlanFactsLocked", err)
	}
	var finalPlan roomRentPlan
	if err := db.WithContext(ctx).Where("user_id = ? AND room_id = ? AND effective_from_month = ?", owner.ID, roomRow.ID, period).First(&finalPlan).Error; err != nil {
		t.Fatal(err)
	}
	if finalPlan.MonthlyRentCents != 110000 {
		t.Fatalf("locked plan changed to %d cents; want 110000", finalPlan.MonthlyRentCents)
	}
}

func TestSaveRoomRentPlanRejectsInactiveTenantMembersOnMySQL(t *testing.T) {
	db, sqlDB := openLedgerMySQLTestDB(t)
	if err := runMigrations(sqlDB, "../../migrations"); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	ctx := context.Background()
	owner := user{Username: fmt.Sprintf("room-plan-inactive-tenant-%d", time.Now().UnixNano()), PasswordHash: "test"}
	if err := db.WithContext(ctx).Create(&owner).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.WithContext(ctx).Delete(&user{}, owner.ID).Error })
	tenantRow, err := newTenantService(db).createTenant(ctx, owner.ID, tenantInput{Name: "Inactive tenant", Status: "inactive"})
	if err != nil {
		t.Fatal(err)
	}
	domain := newLandlordDomainService(db)
	propertyRow, err := domain.createProperty(ctx, owner.ID, propertyInput{Name: "Inactive tenant property"})
	if err != nil {
		t.Fatal(err)
	}
	roomRow, err := domain.createRoom(ctx, owner.ID, roomInput{PropertyID: propertyRow.ID, RoomLabel: "Room 1"})
	if err != nil {
		t.Fatal(err)
	}
	_, _, err = newRoomRentPlanService(db).SaveRoomRentPlan(ctx, SaveRoomRentPlanCommand{
		UserID: owner.ID, RoomID: roomRow.ID, EffectiveMonth: monthStart(time.Now().UTC()),
		MonthlyRentCents: 50000, Currency: ledgerCurrencyEUR, DueDay: 5,
		Members: []RoomRentPlanMemberInput{{TenantID: tenantRow.ID, ResponsibilityCents: 50000}},
	})
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("inactive tenant membership error=%v; want gorm.ErrRecordNotFound", err)
	}
}
