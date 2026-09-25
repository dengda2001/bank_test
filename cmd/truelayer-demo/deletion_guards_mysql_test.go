package main

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"gorm.io/gorm"
)

func TestObjectDeletionGuardsKeepLedgerHistoryAndRemoveIndependentRecordsOnMySQL(t *testing.T) {
	db, sqlDB := openLedgerMySQLTestDB(t)
	if err := runMigrations(sqlDB, "../../migrations"); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	ctx := context.Background()
	owner := user{Username: fmt.Sprintf("deletion-owner-%d", time.Now().UnixNano()), PasswordHash: "test"}
	other := user{Username: fmt.Sprintf("deletion-other-%d", time.Now().UnixNano()), PasswordHash: "test"}
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

	domain := newLandlordDomainService(db)
	tenants := newTenantService(db)
	independentProperty, err := domain.createProperty(ctx, owner.ID, propertyInput{Name: "Independent property"})
	if err != nil {
		t.Fatal(err)
	}
	independentRoom, err := domain.createRoom(ctx, owner.ID, roomInput{PropertyID: independentProperty.ID, RoomLabel: "Independent room"})
	if err != nil {
		t.Fatal(err)
	}
	independentTenant, err := tenants.createTenant(ctx, owner.ID, tenantInput{Name: "Independent tenant", Status: "active"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tenants.addTenantPayer(ctx, owner.ID, independentTenant.ID, tenantPayerInput{Name: "Independent payer"}); err != nil {
		t.Fatal(err)
	}
	if err := domain.deleteRoom(ctx, owner.ID, independentRoom.ID); err != nil {
		t.Fatalf("delete independent room: %v", err)
	}
	if err := domain.deleteProperty(ctx, owner.ID, independentProperty.ID); err != nil {
		t.Fatalf("delete independent property: %v", err)
	}
	if err := tenants.deleteTenant(ctx, owner.ID, independentTenant.ID); err != nil {
		t.Fatalf("delete independent tenant: %v", err)
	}
	var hiddenProperty property
	if err := db.WithContext(ctx).Where("id = ?", independentProperty.ID).First(&hiddenProperty).Error; err != nil || hiddenProperty.DeletedAt == nil {
		t.Fatalf("property history was not retained as hidden: %+v, %v", hiddenProperty, err)
	}
	var hiddenRoom room
	if err := db.WithContext(ctx).Where("id = ?", independentRoom.ID).First(&hiddenRoom).Error; err != nil || hiddenRoom.DeletedAt == nil {
		t.Fatalf("room history was not retained as hidden: %+v, %v", hiddenRoom, err)
	}
	if visible, err := newLandlordRentRepository(db).listProperties(ctx, owner.ID, propertyQuery{}); err != nil || len(visible) != 0 {
		t.Fatalf("hidden property remains in daily list: %+v, %v", visible, err)
	}
	var tenantCount int64
	if err := db.WithContext(ctx).Model(&tenant{}).Where("id = ?", independentTenant.ID).Count(&tenantCount).Error; err != nil || tenantCount != 0 {
		t.Fatalf("independent tenant still exists: %d, %v", tenantCount, err)
	}
	var payerCount int64
	if err := db.WithContext(ctx).Model(&tenantPayer{}).Where("user_id = ? AND tenant_id = ?", owner.ID, independentTenant.ID).Count(&payerCount).Error; err != nil {
		t.Fatal(err)
	}
	if payerCount != 0 {
		t.Fatalf("independent tenant payers were not removed with the deleted tenant: %d", payerCount)
	}

	historyProperty, err := domain.createProperty(ctx, owner.ID, propertyInput{Name: "History property"})
	if err != nil {
		t.Fatal(err)
	}
	historyRoom, err := domain.createRoom(ctx, owner.ID, roomInput{PropertyID: historyProperty.ID, RoomLabel: "History room"})
	if err != nil {
		t.Fatal(err)
	}
	historyTenant, err := tenants.createTenant(ctx, owner.ID, tenantInput{Name: "History tenant", Status: "active"})
	if err != nil {
		t.Fatal(err)
	}
	period := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	plan, err := newLandlordRentRepository(db).createRoomRentPlan(ctx, owner.ID, roomRentPlan{
		RoomID: historyRoom.ID, EffectiveFromMonth: period, MonthlyRentCents: 100000, Currency: "EUR", DueDay: 5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := newLandlordRentRepository(db).createRoomRentPlanMember(ctx, owner.ID, roomRentPlanMember{
		RoomRentPlanID: plan.ID, TenantID: historyTenant.ID, ResponsibilityCents: 100000,
	}); err != nil {
		t.Fatal(err)
	}
	if err := domain.deleteProperty(ctx, owner.ID, historyProperty.ID); !errors.Is(err, errPropertyDeletionBlocked) {
		t.Fatalf("delete property with room: %v", err)
	}
	if err := domain.deleteRoom(ctx, owner.ID, historyRoom.ID); !errors.Is(err, errRoomDeletionBlocked) {
		t.Fatalf("delete room with rent plan: %v", err)
	}
	if err := tenants.deleteTenant(ctx, owner.ID, historyTenant.ID); !errors.Is(err, errTenantDeletionBlocked) {
		t.Fatalf("delete tenant with rent plan membership: %v", err)
	}

	matchedTenant, err := tenants.createTenant(ctx, owner.ID, tenantInput{Name: "Matched tenant", Status: "active"})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.WithContext(ctx).Create(&paymentTransaction{
		UserID: owner.ID, Source: "test", StableTransactionKey: fmt.Sprintf("matched-tenant-%d", time.Now().UnixNano()),
		Direction: "income", AmountCents: 1, Currency: "EUR", MatchedTenantID: &matchedTenant.ID, MatchStatus: "candidate",
	}).Error; err != nil {
		t.Fatal(err)
	}
	if err := tenants.deleteTenant(ctx, owner.ID, matchedTenant.ID); !errors.Is(err, errTenantDeletionBlocked) {
		t.Fatalf("delete tenant referenced by a matched transaction: %v", err)
	}
	if err := tenants.deleteTenant(ctx, other.ID, historyTenant.ID); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("cross-user tenant deletion: %v", err)
	}
}
