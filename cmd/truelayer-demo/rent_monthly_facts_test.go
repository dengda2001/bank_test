package main

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
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

func TestEnsureMonthlyRentFactsGeneratesStructuredAndLegacyRowsOnceOnMySQL(t *testing.T) {
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

	firstTenant := repositoryTestTenant(owner.ID, "First structured tenant")
	secondTenant := repositoryTestTenant(owner.ID, "Second structured tenant")
	legacyTenant := repositoryTestTenant(owner.ID, "Legacy tenant")
	for _, row := range []*tenant{&firstTenant, &secondTenant, &legacyTenant} {
		if err := db.WithContext(ctx).Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}

	domain := newLandlordDomainService(db)
	propertyRow, err := domain.createProperty(ctx, owner.ID, propertyInput{Name: "Monthly facts property"})
	if err != nil {
		t.Fatal(err)
	}
	roomRow, err := domain.createRoom(ctx, owner.ID, roomInput{
		PropertyID: propertyRow.ID, RoomLabel: "Room 1", MonthlyRentCents: 100000,
		DueDay: 5, ActiveFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	period := monthStart(time.Now().UTC())
	_, err = domain.saveRentArrangement(ctx, owner.ID, rentArrangementInput{
		RoomID: roomRow.ID, EffectiveMonth: period, MonthlyRentCents: 100000, Currency: "EUR", DueDay: 5,
		Responsibilities: []rentResponsibilityInput{
			{TenantID: firstTenant.ID, AmountCents: 60000},
			{TenantID: secondTenant.ID, AmountCents: 40000},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	emptyRoom, err := domain.createRoom(ctx, owner.ID, roomInput{
		PropertyID: propertyRow.ID, RoomLabel: "Empty room", MonthlyRentCents: 70000,
		DueDay: 5, ActiveFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := domain.saveRentArrangement(ctx, owner.ID, rentArrangementInput{
		RoomID: emptyRoom.ID, EffectiveMonth: period, MonthlyRentCents: 70000, Currency: "EUR", DueDay: 5,
	}); err != nil {
		t.Fatal(err)
	}

	service := newMonthlyRentFactsService(db)
	if err := service.ensureMonthlyRentFacts(ctx, owner.ID, period, rentFactsIntentRead); err != nil {
		t.Fatal(err)
	}
	var charges []rentCharge
	if err := db.WithContext(ctx).Where("user_id = ? AND room_id = ? AND period_month = ?", owner.ID, roomRow.ID, period).Find(&charges).Error; err != nil {
		t.Fatal(err)
	}
	if len(charges) != 1 || charges[0].ExpectedAmountCents != 100000 {
		t.Fatalf("structured room charges = %+v; want one 100000-cent charge", charges)
	}
	var emptyRoomChargeCount int64
	if err := db.WithContext(ctx).Model(&rentCharge{}).Where("user_id = ? AND room_id = ? AND period_month = ?", owner.ID, emptyRoom.ID, period).Count(&emptyRoomChargeCount).Error; err != nil {
		t.Fatal(err)
	}
	if emptyRoomChargeCount != 0 {
		t.Fatalf("empty room generated %d rent charges; want none", emptyRoomChargeCount)
	}
	var structured []rentObligation
	if err := db.WithContext(ctx).Where("user_id = ? AND rent_charge_id = ?", owner.ID, charges[0].ID).Order("tenant_id ASC").Find(&structured).Error; err != nil {
		t.Fatal(err)
	}
	if len(structured) != 2 || structured[0].ExpectedAmountCents+structured[1].ExpectedAmountCents != 100000 {
		t.Fatalf("structured obligations = %+v; want two obligations summing to room rent", structured)
	}
	var structuredLegacyCount int64
	if err := db.WithContext(ctx).Model(&rentObligation{}).Where("user_id = ? AND tenant_id IN ? AND period_month = ? AND rent_charge_id IS NULL", owner.ID, []uint64{firstTenant.ID, secondTenant.ID}, period).Count(&structuredLegacyCount).Error; err != nil {
		t.Fatal(err)
	}
	if structuredLegacyCount != 0 {
		t.Fatalf("structured tenants also received %d legacy obligations", structuredLegacyCount)
	}
	var legacy rentObligation
	if err := db.WithContext(ctx).Where("user_id = ? AND tenant_id = ? AND period_month = ? AND rent_charge_id IS NULL", owner.ID, legacyTenant.ID, period).First(&legacy).Error; err != nil {
		t.Fatalf("legacy tenant obligation: %v", err)
	}

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
	var chargeCount, structuredCount int64
	if err := db.WithContext(ctx).Model(&rentCharge{}).Where("user_id = ? AND room_id = ? AND period_month = ?", owner.ID, roomRow.ID, period).Count(&chargeCount).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.WithContext(ctx).Model(&rentObligation{}).Where("user_id = ? AND rent_charge_id = ?", owner.ID, charges[0].ID).Count(&structuredCount).Error; err != nil {
		t.Fatal(err)
	}
	if chargeCount != 1 || structuredCount != 2 {
		t.Fatalf("concurrent facts: charges=%d obligations=%d; want 1 and 2", chargeCount, structuredCount)
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
	tenantRow.MonthlyRentCents = 0
	if err := db.WithContext(ctx).Create(&tenantRow).Error; err != nil {
		t.Fatal(err)
	}
	domain := newLandlordDomainService(db)
	propertyRow, err := domain.createProperty(ctx, owner.ID, propertyInput{Name: "Future facts property"})
	if err != nil {
		t.Fatal(err)
	}
	roomRow, err := domain.createRoom(ctx, owner.ID, roomInput{
		PropertyID: propertyRow.ID, RoomLabel: "Room 1", MonthlyRentCents: 50000,
		DueDay: 5, ActiveFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	future := monthStart(time.Now().UTC()).AddDate(0, 1, 0)
	_, err = domain.saveRentArrangement(ctx, owner.ID, rentArrangementInput{
		RoomID: roomRow.ID, EffectiveMonth: future, MonthlyRentCents: 50000, Currency: "EUR", DueDay: 5,
		Responsibilities: []rentResponsibilityInput{{TenantID: tenantRow.ID, AmountCents: 50000}},
	})
	if err != nil {
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

func TestSaveRentArrangementRejectsExistingLegacyMonthOnMySQL(t *testing.T) {
	db, sqlDB := openLedgerMySQLTestDB(t)
	if err := runMigrations(sqlDB, "../../migrations"); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	ctx := context.Background()
	owner := user{Username: fmt.Sprintf("arrangement-legacy-owner-%d", time.Now().UnixNano()), PasswordHash: "test"}
	if err := db.WithContext(ctx).Create(&owner).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.WithContext(ctx).Delete(&user{}, owner.ID).Error })
	tenantRow := repositoryTestTenant(owner.ID, "Legacy before binding")
	if err := db.WithContext(ctx).Create(&tenantRow).Error; err != nil {
		t.Fatal(err)
	}
	period := monthStart(time.Now().UTC())
	if err := newObligationService(db).ensureMonthlyObligations(ctx, owner.ID, period); err != nil {
		t.Fatal(err)
	}
	domain := newLandlordDomainService(db)
	propertyRow, err := domain.createProperty(ctx, owner.ID, propertyInput{Name: "Legacy binding property"})
	if err != nil {
		t.Fatal(err)
	}
	roomRow, err := domain.createRoom(ctx, owner.ID, roomInput{
		PropertyID: propertyRow.ID, RoomLabel: "Room 1", MonthlyRentCents: 100000,
		DueDay: 5, ActiveFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = domain.saveRentArrangement(ctx, owner.ID, rentArrangementInput{
		RoomID: roomRow.ID, EffectiveMonth: period, MonthlyRentCents: 100000, Currency: "EUR", DueDay: 5,
		Responsibilities: []rentResponsibilityInput{{TenantID: tenantRow.ID, AmountCents: 100000}},
	})
	if !errors.Is(err, errRentFactsConflict) {
		t.Fatalf("arrangement with a legacy obligation error=%v; want a rent facts conflict", err)
	}
	var agreementCount, legacyCount int64
	if err := db.WithContext(ctx).Model(&tenancyAgreement{}).Where("user_id = ? AND room_id = ?", owner.ID, roomRow.ID).Count(&agreementCount).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.WithContext(ctx).Model(&rentObligation{}).Where("user_id = ? AND tenant_id = ? AND period_month = ? AND rent_charge_id IS NULL", owner.ID, tenantRow.ID, period).Count(&legacyCount).Error; err != nil {
		t.Fatal(err)
	}
	if agreementCount != 0 || legacyCount != 1 {
		t.Fatalf("after rejected arrangement: agreements=%d legacy obligations=%d; want 0 and 1", agreementCount, legacyCount)
	}
}

func TestEnsureMonthlyRentFactsRejectsLegacyStructuredConflictOnMySQL(t *testing.T) {
	db, sqlDB := openLedgerMySQLTestDB(t)
	if err := runMigrations(sqlDB, "../../migrations"); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	ctx := context.Background()
	owner := user{Username: fmt.Sprintf("conflict-monthly-facts-owner-%d", time.Now().UnixNano()), PasswordHash: "test"}
	if err := db.WithContext(ctx).Create(&owner).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.WithContext(ctx).Delete(&user{}, owner.ID).Error })
	tenantRow := repositoryTestTenant(owner.ID, "Conflicting tenant")
	if err := db.WithContext(ctx).Create(&tenantRow).Error; err != nil {
		t.Fatal(err)
	}
	domain := newLandlordDomainService(db)
	propertyRow, err := domain.createProperty(ctx, owner.ID, propertyInput{Name: "Conflict facts property"})
	if err != nil {
		t.Fatal(err)
	}
	roomRow, err := domain.createRoom(ctx, owner.ID, roomInput{
		PropertyID: propertyRow.ID, RoomLabel: "Room 1", MonthlyRentCents: 100000,
		DueDay: 5, ActiveFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	period := monthStart(time.Now().UTC())
	if _, err := domain.saveRentArrangement(ctx, owner.ID, rentArrangementInput{
		RoomID: roomRow.ID, EffectiveMonth: period, MonthlyRentCents: 100000, Currency: "EUR", DueDay: 5,
		Responsibilities: []rentResponsibilityInput{{TenantID: tenantRow.ID, AmountCents: 100000}},
	}); err != nil {
		t.Fatal(err)
	}
	legacy := rentObligation{
		UserID: owner.ID, TenantID: tenantRow.ID, PeriodMonth: period,
		DueDate: dueDateForMonth(period, 5), ExpectedAmountCents: 100000, Currency: "EUR",
		Status: "open", RecordStatus: obligationRecordActive, GeneratedBy: "lazy",
	}
	if err := db.WithContext(ctx).Create(&legacy).Error; err != nil {
		t.Fatal(err)
	}

	err = newMonthlyRentFactsService(db).ensureMonthlyRentFacts(ctx, owner.ID, period, rentFactsIntentRead)
	if err == nil {
		t.Fatal("legacy and structured facts for one tenant/month must be rejected")
	}
	if !errors.Is(err, errRentFactsConflict) {
		t.Fatalf("conflict error = %v; want a rent facts conflict", err)
	}
	var chargeCount int64
	if err := db.WithContext(ctx).Model(&rentCharge{}).Where("user_id = ? AND room_id = ? AND period_month = ?", owner.ID, roomRow.ID, period).Count(&chargeCount).Error; err != nil {
		t.Fatal(err)
	}
	if chargeCount != 0 {
		t.Fatalf("conflict created %d charges; want none", chargeCount)
	}
}
