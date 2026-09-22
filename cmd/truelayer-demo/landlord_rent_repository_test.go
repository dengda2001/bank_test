package main

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"gorm.io/gorm"
)

func TestLandlordRentRepositoryRejectsMissingUserBeforeDatabaseAccess(t *testing.T) {
	repo := newLandlordRentRepository(nil)
	if _, err := repo.findProperty(context.Background(), 0, 1); !errors.Is(err, errLandlordRentUserRequired) {
		t.Fatalf("findProperty error=%v", err)
	}
}

func TestLandlordRentRepositoryScopesPlansAndFactsToOwnerOnMySQL(t *testing.T) {
	db, sqlDB := openLedgerMySQLTestDB(t)
	if err := runMigrations(sqlDB, "../../migrations"); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	ctx := context.Background()
	owner := user{Username: fmt.Sprintf("room-plan-owner-%d", time.Now().UnixNano()), PasswordHash: "test"}
	other := user{Username: fmt.Sprintf("room-plan-other-%d", time.Now().UnixNano()), PasswordHash: "test"}
	if err := db.WithContext(ctx).Create(&owner).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.WithContext(ctx).Create(&other).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.WithContext(ctx).Delete(&user{}, owner.ID).Error
		_ = db.WithContext(ctx).Delete(&user{}, other.ID).Error
	})

	repo := newLandlordRentRepository(db)
	ownerProperty, err := repo.createProperty(ctx, owner.ID, property{Name: "Owner home"})
	if err != nil {
		t.Fatal(err)
	}
	otherProperty, err := repo.createProperty(ctx, other.ID, property{Name: "Other home"})
	if err != nil {
		t.Fatal(err)
	}
	ownerRoom, err := repo.createRoom(ctx, owner.ID, room{PropertyID: ownerProperty.ID, RoomLabel: "A-01"})
	if err != nil {
		t.Fatal(err)
	}
	otherRoom, err := repo.createRoom(ctx, other.ID, room{PropertyID: otherProperty.ID, RoomLabel: "B-01"})
	if err != nil {
		t.Fatal(err)
	}
	ownerTenant, err := newTenantService(db).createTenant(ctx, owner.ID, tenantInput{Name: "Owner tenant", Status: "active"})
	if err != nil {
		t.Fatal(err)
	}
	otherTenant, err := newTenantService(db).createTenant(ctx, other.ID, tenantInput{Name: "Other tenant", Status: "active"})
	if err != nil {
		t.Fatal(err)
	}
	period := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	ownerPlan, err := repo.createRoomRentPlan(ctx, owner.ID, roomRentPlan{RoomID: ownerRoom.ID, EffectiveFromMonth: period, MonthlyRentCents: 100000, Currency: "EUR", DueDay: 5})
	if err != nil {
		t.Fatal(err)
	}
	otherPlan, err := repo.createRoomRentPlan(ctx, other.ID, roomRentPlan{RoomID: otherRoom.ID, EffectiveFromMonth: period, MonthlyRentCents: 50000, Currency: "EUR", DueDay: 5})
	if err != nil {
		t.Fatal(err)
	}
	member, err := repo.createRoomRentPlanMember(ctx, owner.ID, roomRentPlanMember{RoomRentPlanID: ownerPlan.ID, TenantID: ownerTenant.ID, ResponsibilityCents: 100000})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.findRoomRentPlan(ctx, owner.ID, otherPlan.ID); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("cross-user plan lookup error=%v", err)
	}
	plans, err := repo.listRoomRentPlans(ctx, owner.ID, roomRentPlanQuery{})
	if err != nil || len(plans) != 1 || plans[0].ID != ownerPlan.ID {
		t.Fatalf("owner plans=%+v err=%v", plans, err)
	}
	charge, err := repo.createRentCharge(ctx, owner.ID, rentCharge{PropertyID: ownerProperty.ID, RoomID: ownerRoom.ID, RoomRentPlanID: ownerPlan.ID, PeriodMonth: period, DueDate: dueDateForMonth(period, 5), ExpectedAmountCents: 100000, Currency: "EUR", RecordStatus: obligationRecordActive})
	if err != nil {
		t.Fatal(err)
	}
	obligation, err := repo.createRentObligation(ctx, owner.ID, rentObligation{RentChargeID: charge.ID, RoomRentPlanID: ownerPlan.ID, RoomRentPlanMemberID: member.ID, TenantID: ownerTenant.ID, PeriodMonth: period, DueDate: charge.DueDate, ExpectedAmountCents: 100000, Currency: "EUR", Status: "open", RecordStatus: obligationRecordActive})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.createRentObligation(ctx, owner.ID, rentObligation{RentChargeID: charge.ID, RoomRentPlanID: ownerPlan.ID, RoomRentPlanMemberID: member.ID, TenantID: otherTenant.ID, PeriodMonth: period, DueDate: charge.DueDate, ExpectedAmountCents: 100000, Currency: "EUR", RecordStatus: obligationRecordActive}); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("cross-user responsibility tenant error=%v", err)
	}
	if got, err := repo.findRentObligation(ctx, owner.ID, obligation.ID); err != nil || got.RoomRentPlanMemberID != member.ID {
		t.Fatalf("owner obligation=%+v err=%v", got, err)
	}
}
