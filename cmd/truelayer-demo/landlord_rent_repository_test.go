package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"gorm.io/gorm"
)

func TestLandlordRentRepositoryRejectsMissingUserBeforeDatabaseAccess(t *testing.T) {
	repo := newLandlordRentRepository(nil)

	_, err := repo.findProperty(context.Background(), 0, 1)
	if err == nil || !strings.Contains(err.Error(), "userID") {
		t.Fatalf("findProperty error = %v, want a userID validation error", err)
	}
}

func TestLandlordRentRepositoryScopesPropertiesAndRoomsOnMySQL(t *testing.T) {
	db, sqlDB := openLedgerMySQLTestDB(t)
	if err := runMigrations(sqlDB, "../../migrations"); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	ctx := context.Background()
	owner := user{Username: fmt.Sprintf("rent-repo-owner-%d", time.Now().UnixNano()), PasswordHash: "test"}
	other := user{Username: fmt.Sprintf("rent-repo-other-%d", time.Now().UnixNano()), PasswordHash: "test"}
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
	ownerProperty, err := repo.createProperty(ctx, owner.ID, property{Name: "Shared name"})
	if err != nil {
		t.Fatal(err)
	}
	otherProperty, err := repo.createProperty(ctx, other.ID, property{Name: "Shared name"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.findProperty(ctx, owner.ID, otherProperty.ID); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("cross-user property lookup error = %v, want gorm.ErrRecordNotFound", err)
	}
	properties, err := repo.listProperties(ctx, owner.ID, propertyQuery{})
	if err != nil {
		t.Fatal(err)
	}
	if len(properties) != 1 || properties[0].ID != ownerProperty.ID {
		t.Fatalf("owner properties = %+v, want only property %d", properties, ownerProperty.ID)
	}

	ownerRoom, err := repo.createRoom(ctx, owner.ID, room{PropertyID: ownerProperty.ID, RoomLabel: "A-01", ActiveFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	rooms, err := repo.listRooms(ctx, owner.ID, roomQuery{PropertyID: ownerProperty.ID})
	if err != nil {
		t.Fatal(err)
	}
	if len(rooms) != 1 || rooms[0].ID != ownerRoom.ID {
		t.Fatalf("owner rooms = %+v, want only room %d", rooms, ownerRoom.ID)
	}
	if _, err := repo.createRoom(ctx, owner.ID, room{PropertyID: otherProperty.ID, RoomLabel: "B-01", ActiveFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("cross-user room create error = %v, want gorm.ErrRecordNotFound", err)
	}
}
