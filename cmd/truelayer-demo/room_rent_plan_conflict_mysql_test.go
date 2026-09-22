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

type roomRentPlanConflictFixture struct {
	db      *gorm.DB
	ctx     context.Context
	owner   user
	tenant  tenant
	roomOne room
	roomTwo room
}

func newRoomRentPlanConflictFixture(t *testing.T) roomRentPlanConflictFixture {
	t.Helper()
	db, sqlDB := openLedgerMySQLTestDB(t)
	if err := runMigrations(sqlDB, "../../migrations"); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	ctx := context.Background()
	owner := user{Username: fmt.Sprintf("tenant-room-conflict-%d", time.Now().UnixNano()), PasswordHash: "test"}
	if err := db.WithContext(ctx).Create(&owner).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		// The fixture intentionally materializes current-month facts. Remove the
		// dependent rows explicitly so an opt-in MySQL run does not leave a
		// partially deleted owner behind when foreign-key RESTRICT is enabled.
		for _, statement := range []string{
			"DELETE FROM dunning_send_attempts WHERE user_id = ?",
			"DELETE FROM cash_receipts WHERE user_id = ?",
			"DELETE FROM payment_allocations WHERE user_id = ?",
			"DELETE FROM rent_obligations WHERE user_id = ?",
			"DELETE FROM rent_charges WHERE user_id = ?",
			"DELETE FROM room_rent_plan_members WHERE user_id = ?",
			"DELETE FROM room_rent_plans WHERE user_id = ?",
			"DELETE FROM rooms WHERE user_id = ?",
			"DELETE FROM properties WHERE user_id = ?",
			"DELETE FROM tenant_payers WHERE user_id = ?",
			"DELETE FROM tenants WHERE user_id = ?",
		} {
			_ = db.WithContext(ctx).Exec(statement, owner.ID).Error
		}
		_ = db.WithContext(ctx).Delete(&user{}, owner.ID).Error
	})
	tenantRow := repositoryTestTenant(owner.ID, "Shared tenant")
	if err := db.WithContext(ctx).Create(&tenantRow).Error; err != nil {
		t.Fatal(err)
	}
	domain := newLandlordDomainService(db)
	propertyRow, err := domain.createProperty(ctx, owner.ID, propertyInput{Name: "Conflict property"})
	if err != nil {
		t.Fatal(err)
	}
	roomOne, err := domain.createRoom(ctx, owner.ID, roomInput{PropertyID: propertyRow.ID, RoomLabel: "Room 1"})
	if err != nil {
		t.Fatal(err)
	}
	roomTwo, err := domain.createRoom(ctx, owner.ID, roomInput{PropertyID: propertyRow.ID, RoomLabel: "Room 2"})
	if err != nil {
		t.Fatal(err)
	}
	return roomRentPlanConflictFixture{db: db, ctx: ctx, owner: owner, tenant: tenantRow, roomOne: roomOne, roomTwo: roomTwo}
}

func roomRentPlanCommandFor(f roomRentPlanConflictFixture, roomID uint64, month time.Time) SaveRoomRentPlanCommand {
	return SaveRoomRentPlanCommand{
		UserID: f.owner.ID, RoomID: roomID, EffectiveMonth: month,
		MonthlyRentCents: 100000, Currency: ledgerCurrencyEUR, DueDay: 5,
		Members: []RoomRentPlanMemberInput{{TenantID: f.tenant.ID, ResponsibilityCents: 100000}},
	}
}

func TestSaveRoomRentPlanStoresAnEmptyCurrentMonthPlanWithoutRentFactsOnMySQL(t *testing.T) {
	f := newRoomRentPlanConflictFixture(t)
	month := dublinCurrentMonth(time.Now())
	plan, version, err := newRoomRentPlanService(f.db).SaveRoomRentPlan(f.ctx, SaveRoomRentPlanCommand{
		UserID: f.owner.ID, RoomID: f.roomOne.ID, EffectiveMonth: month,
		MonthlyRentCents: 100000, Currency: ledgerCurrencyEUR, DueDay: 1,
	})
	if err != nil {
		t.Fatalf("save empty room rent plan: %v", err)
	}
	if version != 1 {
		t.Fatalf("empty room rent plan version = %d, want 1", version)
	}
	var memberCount, chargeCount int64
	if err := f.db.WithContext(f.ctx).Model(&roomRentPlanMember{}).Where("user_id = ? AND room_rent_plan_id = ?", f.owner.ID, plan.ID).Count(&memberCount).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.db.WithContext(f.ctx).Model(&rentCharge{}).Where("user_id = ? AND room_id = ? AND period_month = ?", f.owner.ID, f.roomOne.ID, month).Count(&chargeCount).Error; err != nil {
		t.Fatal(err)
	}
	if memberCount != 0 || chargeCount != 0 {
		t.Fatalf("empty room plan members=%d charges=%d, want neither", memberCount, chargeCount)
	}
}

func TestEnsureMonthlyRentFactsSkipsAnEmptyRoomPlanOnMySQL(t *testing.T) {
	f := newRoomRentPlanConflictFixture(t)
	month := dublinCurrentMonth(time.Now())
	if _, _, err := newRoomRentPlanService(f.db).SaveRoomRentPlan(f.ctx, SaveRoomRentPlanCommand{
		UserID: f.owner.ID, RoomID: f.roomOne.ID, EffectiveMonth: month,
		MonthlyRentCents: 100000, Currency: ledgerCurrencyEUR, DueDay: 1,
	}); err != nil {
		t.Fatalf("save empty room rent plan: %v", err)
	}
	if err := newMonthlyRentFactsService(f.db).ensureMonthlyRentFacts(f.ctx, f.owner.ID, month, rentFactsIntentRead); err != nil {
		t.Fatalf("ensure monthly facts for empty room plan: %v", err)
	}
	var chargeCount int64
	if err := f.db.WithContext(f.ctx).Model(&rentCharge{}).Where("user_id = ? AND room_id = ? AND period_month = ?", f.owner.ID, f.roomOne.ID, month).Count(&chargeCount).Error; err != nil {
		t.Fatal(err)
	}
	if chargeCount != 0 {
		t.Fatalf("empty room plan charge count = %d, want 0", chargeCount)
	}
}

func TestCreateRoomWithRentPlanStoresAnEmptyRentRuleAtomicallyOnMySQL(t *testing.T) {
	f := newRoomRentPlanConflictFixture(t)
	var propertyRow property
	if err := f.db.WithContext(f.ctx).Where("user_id = ?", f.owner.ID).First(&propertyRow).Error; err != nil {
		t.Fatal(err)
	}
	month := dublinCurrentMonth(time.Now())
	created, plan, err := newLandlordDomainService(f.db).createRoomWithRentPlan(f.ctx, f.owner.ID, roomInput{
		PropertyID: propertyRow.ID, RoomLabel: "Rent Rule First",
	}, roomRentPlanSetupInput{
		EffectiveMonth: month, MonthlyRentCents: 123456, Currency: ledgerCurrencyEUR, DueDay: 1,
	})
	if err != nil {
		t.Fatalf("create room with rent plan: %v", err)
	}
	if created.RentPlanVersion != 1 || plan.RoomID != created.ID || plan.MonthlyRentCents != 123456 || plan.DueDay != 1 {
		t.Fatalf("created room=%+v plan=%+v, want versioned empty rent rule", created, plan)
	}
	var memberCount, chargeCount int64
	if err := f.db.WithContext(f.ctx).Model(&roomRentPlanMember{}).Where("user_id = ? AND room_rent_plan_id = ?", f.owner.ID, plan.ID).Count(&memberCount).Error; err != nil {
		t.Fatal(err)
	}
	if err := f.db.WithContext(f.ctx).Model(&rentCharge{}).Where("user_id = ? AND room_id = ?", f.owner.ID, created.ID).Count(&chargeCount).Error; err != nil {
		t.Fatal(err)
	}
	if memberCount != 0 || chargeCount != 0 {
		t.Fatalf("new empty room members=%d charges=%d, want neither", memberCount, chargeCount)
	}
}

func TestSaveRoomRentPlanRejectsCurrentMonthTenantRoomConflictOnMySQL(t *testing.T) {
	f := newRoomRentPlanConflictFixture(t)
	month := dublinCurrentMonth(time.Now())
	plans := newRoomRentPlanService(f.db)
	if _, _, err := plans.SaveRoomRentPlan(f.ctx, roomRentPlanCommandFor(f, f.roomTwo.ID, month)); err != nil {
		t.Fatalf("save existing current-month plan: %v", err)
	}
	if _, _, err := plans.SaveRoomRentPlan(f.ctx, roomRentPlanCommandFor(f, f.roomOne.ID, month)); !errors.Is(err, ErrTenantRoomMonthConflict) {
		t.Fatalf("current-month conflict error=%v; want ErrTenantRoomMonthConflict", err)
	}
}

func TestSaveRoomRentPlanRejectsFutureTenantRoomConflictOnMySQL(t *testing.T) {
	f := newRoomRentPlanConflictFixture(t)
	current := dublinCurrentMonth(time.Now())
	future := current.AddDate(0, 1, 0)
	plans := newRoomRentPlanService(f.db)
	if _, _, err := plans.SaveRoomRentPlan(f.ctx, roomRentPlanCommandFor(f, f.roomTwo.ID, future)); err != nil {
		t.Fatalf("save existing future plan: %v", err)
	}
	if _, _, err := plans.SaveRoomRentPlan(f.ctx, roomRentPlanCommandFor(f, f.roomOne.ID, current)); !errors.Is(err, ErrTenantRoomMonthConflict) {
		t.Fatalf("future conflict error=%v; want ErrTenantRoomMonthConflict", err)
	}
}

func TestSaveRoomRentPlanAllowsTenantAfterOtherRoomPlanEndedBeforeTargetMonthOnMySQL(t *testing.T) {
	f := newRoomRentPlanConflictFixture(t)
	current := dublinCurrentMonth(time.Now())
	future := current.AddDate(0, 1, 0)
	plans := newRoomRentPlanService(f.db)
	_, version, err := plans.SaveRoomRentPlan(f.ctx, roomRentPlanCommandFor(f, f.roomTwo.ID, current))
	if err != nil {
		t.Fatalf("save existing current-month plan: %v", err)
	}
	if _, err := plans.EndRoomRentPlan(f.ctx, EndRoomRentPlanCommand{
		UserID: f.owner.ID, RoomID: f.roomTwo.ID, VacantFromMonth: future,
		ExpectedTimelineVersion: version,
	}); err != nil {
		t.Fatalf("end existing plan before target month: %v", err)
	}
	if _, _, err := plans.SaveRoomRentPlan(f.ctx, roomRentPlanCommandFor(f, f.roomOne.ID, future)); err != nil {
		t.Fatalf("plan after ended occupancy should be allowed: %v", err)
	}
}

func TestSaveRoomRentPlanRejectsTenantWhenOtherRoomEndsInTargetMonthOnMySQL(t *testing.T) {
	f := newRoomRentPlanConflictFixture(t)
	current := dublinCurrentMonth(time.Now())
	target := current.AddDate(0, 1, 0)
	plans := newRoomRentPlanService(f.db)
	_, version, err := plans.SaveRoomRentPlan(f.ctx, roomRentPlanCommandFor(f, f.roomTwo.ID, current))
	if err != nil {
		t.Fatalf("save existing current-month plan: %v", err)
	}
	if _, err := plans.EndRoomRentPlan(f.ctx, EndRoomRentPlanCommand{
		UserID: f.owner.ID, RoomID: f.roomTwo.ID, VacantFromMonth: target.AddDate(0, 1, 0),
		ExpectedTimelineVersion: version,
	}); err != nil {
		t.Fatalf("end existing plan after target month: %v", err)
	}
	if _, _, err := plans.SaveRoomRentPlan(f.ctx, roomRentPlanCommandFor(f, f.roomOne.ID, target)); !errors.Is(err, ErrTenantRoomMonthConflict) {
		t.Fatalf("plan ending in target month error=%v; want ErrTenantRoomMonthConflict", err)
	}
}

func TestSaveRoomRentPlanSerializesSameTenantAcrossRoomsOnMySQL(t *testing.T) {
	f := newRoomRentPlanConflictFixture(t)
	future := dublinCurrentMonth(time.Now()).AddDate(0, 1, 0)
	plans := newRoomRentPlanService(f.db)
	commands := []SaveRoomRentPlanCommand{
		roomRentPlanCommandFor(f, f.roomOne.ID, future),
		roomRentPlanCommandFor(f, f.roomTwo.ID, future),
	}
	start := make(chan struct{})
	results := make(chan error, len(commands))
	var wait sync.WaitGroup
	for _, command := range commands {
		command := command
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			_, _, err := plans.SaveRoomRentPlan(f.ctx, command)
			results <- err
		}()
	}
	close(start)
	wait.Wait()
	close(results)

	var successes, conflicts int
	for err := range results {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, ErrTenantRoomMonthConflict):
			conflicts++
		default:
			t.Fatalf("concurrent save error=%v; want success or ErrTenantRoomMonthConflict", err)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("concurrent saves successes=%d conflicts=%d; want one of each", successes, conflicts)
	}
}
