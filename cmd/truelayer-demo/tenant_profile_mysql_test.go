package main

import (
	"context"
	"errors"
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
	futureMonths := []time.Time{time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 12, 1, 0, 0, 0, 0, time.UTC)}
	for _, month := range futureMonths {
		if err := newMonthlyRentFactsService(db).ensureMonthlyRentFacts(ctx, owner.ID, month, rentFactsIntentExplicitPayment); err != nil {
			t.Fatal(err)
		}
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

func TestTenantRoomBindingUpdatesOnMySQL(t *testing.T) {
	db, sqlDB := openLedgerMySQLTestDB(t)
	if err := runMigrations(sqlDB, "../../migrations"); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	ctx := context.Background()
	owner := user{Username: fmt.Sprintf("tenant-room-binding-%d", time.Now().UnixNano()), PasswordHash: "test"}
	if err := db.WithContext(ctx).Create(&owner).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.WithContext(ctx).Delete(&user{}, owner.ID) })

	domain := newLandlordDomainService(db)
	propertyRow, err := domain.createProperty(ctx, owner.ID, propertyInput{Name: "Binding test home"})
	if err != nil {
		t.Fatal(err)
	}
	period := monthStart(time.Now().UTC())
	firstRoom, err := domain.createRoom(ctx, owner.ID, roomInput{PropertyID: propertyRow.ID, RoomLabel: "A-01", ActiveFrom: period})
	if err != nil {
		t.Fatal(err)
	}
	secondRoom, err := domain.createRoom(ctx, owner.ID, roomInput{PropertyID: propertyRow.ID, RoomLabel: "A-02", ActiveFrom: period})
	if err != nil {
		t.Fatal(err)
	}

	service := newTenantService(db)
	unboundInput := validTenantInputForProfile()
	unboundInput.Name = "Room binding target"
	unboundInput.RentStartDate = period.Format(dateLayout)
	unboundInput.BillingStartDate = period.Format(dateLayout)
	unboundInput.Structured = true
	target, err := service.createTenant(ctx, owner.ID, unboundInput)
	if err != nil {
		t.Fatal(err)
	}
	siblingInput := unboundInput
	siblingInput.Name = "Existing room tenant"
	sibling, err := service.createTenant(ctx, owner.ID, siblingInput)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := domain.saveRentArrangement(ctx, owner.ID, rentArrangementInput{
		RoomID: firstRoom.ID, EffectiveMonth: period, MonthlyRentCents: 120000,
		Currency: "EUR", DueDay: 5, TenantIDs: []uint64{sibling.ID},
	}); err != nil {
		t.Fatal(err)
	}

	bindInput := unboundInput
	bindInput.RoomID = firstRoom.ID
	bindInput.MonthlyRent = 1200
	bindInput.RoomPlanProvided = true
	bindInput.RoomTenantIDs = []uint64{target.ID, sibling.ID}
	bindInput.Responsibilities = []rentResponsibilityInput{
		{TenantID: target.ID, AmountCents: 50000},
		{TenantID: sibling.ID, AmountCents: 70000},
	}
	if _, err := service.updateTenant(ctx, owner.ID, target.ID, bindInput); err != nil {
		t.Fatalf("bind tenant to room: %v", err)
	}
	var boundTenant tenant
	if err := db.Where("user_id = ? AND id = ?", owner.ID, target.ID).First(&boundTenant).Error; err != nil {
		t.Fatal(err)
	}
	if boundTenant.MonthlyRentCents != 0 {
		t.Fatalf("structured tenant monthly rent=%d; want room arrangement to remain the only rent source", boundTenant.MonthlyRentCents)
	}
	var current tenancyAgreement
	if err := db.Where("user_id = ? AND room_id = ? AND status = ?", owner.ID, firstRoom.ID, "active").First(&current).Error; err != nil {
		t.Fatal(err)
	}
	var parties []agreementParty
	if err := db.Where("user_id = ? AND agreement_id = ? AND status = ?", owner.ID, current.ID, "active").Order("tenant_id ASC").Find(&parties).Error; err != nil {
		t.Fatal(err)
	}
	if len(parties) != 2 || parties[0].TenantID != target.ID && parties[1].TenantID != target.ID || parties[0].TenantID != sibling.ID && parties[1].TenantID != sibling.ID {
		t.Fatalf("bound room parties=%+v, want target and existing tenant", parties)
	}
	responsibilityByTenant := map[uint64]int64{}
	for _, party := range parties {
		responsibilityByTenant[party.TenantID] = party.ResponsibilityCents
	}
	if responsibilityByTenant[target.ID] != 50000 || responsibilityByTenant[sibling.ID] != 70000 {
		t.Fatalf("bound room responsibilities=%v, want target=50000 and sibling=70000", responsibilityByTenant)
	}
	records, err := service.listTenants(ctx, owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got := tenantRoomIDForTest(records, target.ID); got != firstRoom.ID {
		t.Fatalf("tenant edit room selection=%d, want %d", got, firstRoom.ID)
	}

	moveInput := bindInput
	moveInput.RoomID = secondRoom.ID
	moveInput.MonthlyRent = 800
	moveInput.Name = "Edited while moving"
	moveInput.RoomPlanProvided = false
	moveInput.RoomTenantIDs = nil
	moveInput.Responsibilities = nil
	lockedCharge := rentCharge{
		UserID: owner.ID, PropertyID: propertyRow.ID, RoomID: firstRoom.ID,
		TenancyAgreementID: current.ID, PeriodMonth: period,
		DueDate: period.AddDate(0, 0, 5), ExpectedAmountCents: 120000,
		Currency: "EUR", RecordStatus: "active",
	}
	if err := db.WithContext(ctx).Create(&lockedCharge).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := service.updateTenant(ctx, owner.ID, target.ID, moveInput); !errors.Is(err, errArrangementHistoryLocked) {
		t.Fatalf("move with a generated current-month charge error=%v, want arrangement history lock", err)
	}
	var unchanged tenant
	if err := db.Where("id = ? AND user_id = ?", target.ID, owner.ID).First(&unchanged).Error; err != nil {
		t.Fatal(err)
	}
	if unchanged.Name != "Room binding target" {
		t.Fatalf("tenant profile partially updated after locked move: name=%q", unchanged.Name)
	}
	if err := db.Where("id = ? AND user_id = ?", lockedCharge.ID, owner.ID).Delete(&rentCharge{}).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := service.updateTenant(ctx, owner.ID, target.ID, moveInput); err != nil {
		t.Fatalf("move tenant to second room: %v", err)
	}
	var firstRoomCurrent tenancyAgreement
	if err := db.Where("user_id = ? AND room_id = ? AND status = ?", owner.ID, firstRoom.ID, "active").First(&firstRoomCurrent).Error; err != nil {
		t.Fatal(err)
	}
	var firstRoomParties []agreementParty
	if err := db.Where("user_id = ? AND agreement_id = ? AND status = ?", owner.ID, firstRoomCurrent.ID, "active").Find(&firstRoomParties).Error; err != nil {
		t.Fatal(err)
	}
	if len(firstRoomParties) != 1 || firstRoomParties[0].TenantID != sibling.ID {
		t.Fatalf("source room parties after move=%+v, want only sibling %d", firstRoomParties, sibling.ID)
	}
	records, err = service.listTenants(ctx, owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got := tenantRoomIDForTest(records, target.ID); got != secondRoom.ID {
		t.Fatalf("tenant room selection after move=%d, want %d", got, secondRoom.ID)
	}

	moveInput.RoomID = 0
	if _, err := service.updateTenant(ctx, owner.ID, target.ID, moveInput); err != nil {
		t.Fatalf("clear tenant room binding: %v", err)
	}
	records, err = service.listTenants(ctx, owner.ID)
	if err != nil {
		t.Fatal(err)
	}
	if got := tenantRoomIDForTest(records, target.ID); got != 0 {
		t.Fatalf("tenant room selection after unbinding=%d, want 0", got)
	}
}

func tenantRoomIDForTest(records []tenantRecord, tenantID uint64) uint64 {
	for _, record := range records {
		if record.ID == fmt.Sprint(tenantID) {
			return record.RoomID
		}
	}
	return 0
}

func TestRoomCreateDefaultsAndUpdatePreservesOptionalFieldsOnMySQL(t *testing.T) {
	db, sqlDB := openLedgerMySQLTestDB(t)
	if err := runMigrations(sqlDB, "../../migrations"); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	ctx := context.Background()
	owner := user{Username: fmt.Sprintf("room-optional-fields-%d", time.Now().UnixNano()), PasswordHash: "test"}
	if err := db.WithContext(ctx).Create(&owner).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.WithContext(ctx).Delete(&user{}, owner.ID).Error })

	domain := newLandlordDomainService(db)
	propertyRow, err := domain.createProperty(ctx, owner.ID, propertyInput{Name: "Room optional fields test"})
	if err != nil {
		t.Fatal(err)
	}
	period := monthStart(time.Now().UTC())
	legacyRoom, err := domain.createRoom(ctx, owner.ID, roomInput{
		PropertyID: propertyRow.ID, RoomLabel: "Legacy values", RoomType: "双人间", Capacity: 4, ActiveFrom: period,
	})
	if err != nil {
		t.Fatal(err)
	}
	defaultRoom, err := domain.createRoom(ctx, owner.ID, roomInput{
		PropertyID: propertyRow.ID, RoomLabel: "Create defaults", ActiveFrom: period,
	})
	if err != nil {
		t.Fatal(err)
	}
	if defaultRoom.RoomType != "其他" || defaultRoom.Capacity != 1 {
		t.Fatalf("new room defaults=(%q,%d), want (其他,1)", defaultRoom.RoomType, defaultRoom.Capacity)
	}

	if _, err := domain.updateRoom(ctx, owner.ID, legacyRoom.ID, roomInput{
		PropertyID: propertyRow.ID, RoomLabel: "Legacy values edited",
	}); err != nil {
		t.Fatalf("update room without optional fields: %v", err)
	}
	var stored room
	if err := db.Where("id = ? AND user_id = ?", legacyRoom.ID, owner.ID).First(&stored).Error; err != nil {
		t.Fatal(err)
	}
	if stored.RoomType != "双人间" || stored.Capacity != 4 {
		t.Fatalf("omitted room fields changed stored values to (%q,%d), want (双人间,4)", stored.RoomType, stored.Capacity)
	}

	if _, err := domain.updateRoom(ctx, owner.ID, legacyRoom.ID, roomInput{
		PropertyID: propertyRow.ID, RoomLabel: "Legacy values edited", RoomType: "整租单元", Capacity: 5,
	}); err != nil {
		t.Fatalf("update explicitly supplied optional fields: %v", err)
	}
	if err := db.Where("id = ? AND user_id = ?", legacyRoom.ID, owner.ID).First(&stored).Error; err != nil {
		t.Fatal(err)
	}
	if stored.RoomType != "整租单元" || stored.Capacity != 5 {
		t.Fatalf("explicit room fields=(%q,%d), want (整租单元,5)", stored.RoomType, stored.Capacity)
	}
}
