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
	_, err = repo.createManualExpense(context.Background(), 0, manualExpense{Description: "unscoped"})
	if err == nil || !strings.Contains(err.Error(), "userID") {
		t.Fatalf("createManualExpense error = %v, want a userID validation error", err)
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

func TestLandlordRentRepositoryScopesAgreementsChargesAndLedgerReadsOnMySQL(t *testing.T) {
	db, sqlDB := openLedgerMySQLTestDB(t)
	if err := runMigrations(sqlDB, "../../migrations"); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	ctx := context.Background()
	owner := user{Username: fmt.Sprintf("rent-repo-ledger-owner-%d", time.Now().UnixNano()), PasswordHash: "test"}
	other := user{Username: fmt.Sprintf("rent-repo-ledger-other-%d", time.Now().UnixNano()), PasswordHash: "test"}
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

	ownerTenant := repositoryTestTenant(owner.ID, "Owner tenant")
	otherTenant := repositoryTestTenant(other.ID, "Other tenant")
	if err := db.WithContext(ctx).Create(&ownerTenant).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.WithContext(ctx).Create(&otherTenant).Error; err != nil {
		t.Fatal(err)
	}

	repo := newLandlordRentRepository(db)
	ownerProperty, err := repo.createProperty(ctx, owner.ID, property{Name: "Owner property"})
	if err != nil {
		t.Fatal(err)
	}
	otherProperty, err := repo.createProperty(ctx, other.ID, property{Name: "Other property"})
	if err != nil {
		t.Fatal(err)
	}
	ownerRoom, err := repo.createRoom(ctx, owner.ID, room{PropertyID: ownerProperty.ID, RoomLabel: "Owner room", ActiveFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	otherRoom, err := repo.createRoom(ctx, other.ID, room{PropertyID: otherProperty.ID, RoomLabel: "Other room", ActiveFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	agreement, err := repo.createTenancyAgreement(ctx, owner.ID, tenancyAgreement{
		RoomID:           ownerRoom.ID,
		StartDate:        time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		MonthlyRentCents: 100000,
		Currency:         "EUR",
		DueDay:           5,
	})
	if err != nil {
		t.Fatal(err)
	}
	jan := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	otherAgreement, err := repo.createTenancyAgreement(ctx, other.ID, tenancyAgreement{
		RoomID:           otherRoom.ID,
		StartDate:        time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		MonthlyRentCents: 100000,
		Currency:         "EUR",
		DueDay:           5,
	})
	if err != nil {
		t.Fatal(err)
	}
	otherCharge, err := repo.createRentCharge(ctx, other.ID, rentCharge{
		PropertyID:          otherProperty.ID,
		RoomID:              otherRoom.ID,
		TenancyAgreementID:  otherAgreement.ID,
		PeriodMonth:         jan,
		DueDate:             time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC),
		ExpectedAmountCents: 100000,
		Currency:            "EUR",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.findTenancyAgreement(ctx, owner.ID, otherAgreement.ID); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("cross-user agreement lookup error = %v, want gorm.ErrRecordNotFound", err)
	}
	party, err := repo.createAgreementParty(ctx, owner.ID, agreementParty{AgreementID: agreement.ID, TenantID: ownerTenant.ID, ResponsibilityCents: 100000})
	if err != nil {
		t.Fatal(err)
	}
	if party.UserID != owner.ID {
		t.Fatalf("party user ID = %d, want %d", party.UserID, owner.ID)
	}

	feb := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)
	janCharge, err := repo.createRentCharge(ctx, owner.ID, rentCharge{
		PropertyID:          ownerProperty.ID,
		RoomID:              ownerRoom.ID,
		TenancyAgreementID:  agreement.ID,
		PeriodMonth:         jan,
		DueDate:             time.Date(2026, 1, 5, 0, 0, 0, 0, time.UTC),
		ExpectedAmountCents: 100000,
		Currency:            "EUR",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.createRentCharge(ctx, owner.ID, rentCharge{PropertyID: ownerProperty.ID, RoomID: otherRoom.ID, TenancyAgreementID: agreement.ID, PeriodMonth: feb, DueDate: feb, ExpectedAmountCents: 100000, Currency: "EUR"}); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("cross-user charge create error = %v, want gorm.ErrRecordNotFound", err)
	}
	if _, err := repo.createTenancyAgreement(ctx, owner.ID, tenancyAgreement{RoomID: otherRoom.ID, StartDate: jan, MonthlyRentCents: 100000, Currency: "EUR", DueDay: 5}); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("cross-user agreement create error = %v, want gorm.ErrRecordNotFound", err)
	}
	if _, err := repo.createAgreementParty(ctx, owner.ID, agreementParty{AgreementID: agreement.ID, TenantID: otherTenant.ID, ResponsibilityCents: 100000}); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("cross-user party create error = %v, want gorm.ErrRecordNotFound", err)
	}

	obligation, err := repo.createRentObligation(ctx, owner.ID, rentObligation{RentChargeID: &janCharge.ID, TenantID: ownerTenant.ID, PeriodMonth: jan, DueDate: janCharge.DueDate, ExpectedAmountCents: 100000, Currency: "EUR", Status: "open", RecordStatus: obligationRecordActive})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.createRentObligation(ctx, owner.ID, rentObligation{RentChargeID: &janCharge.ID, TenantID: otherTenant.ID, PeriodMonth: jan, DueDate: janCharge.DueDate, ExpectedAmountCents: 100000, Currency: "EUR", Status: "open", RecordStatus: obligationRecordActive}); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("cross-user obligation create error = %v, want gorm.ErrRecordNotFound", err)
	}

	transaction := paymentTransaction{UserID: owner.ID, Source: "test", StableTransactionKey: fmt.Sprintf("repo-test-%d", time.Now().UnixNano()), Direction: "income", AmountCents: 100000, Currency: "EUR"}
	if err := db.WithContext(ctx).Create(&transaction).Error; err != nil {
		t.Fatal(err)
	}
	allocation := paymentAllocation{UserID: owner.ID, PaymentTransactionID: transaction.ID, RentObligationID: &obligation.ID, TenantID: &ownerTenant.ID, AmountCents: 100000, AllocationKind: allocationKindRent, Status: allocationStatusConfirmed, ConfirmedByUserID: owner.ID, ConfirmedAt: jan, ConfirmationSource: "test"}
	if err := db.WithContext(ctx).Create(&allocation).Error; err != nil {
		t.Fatal(err)
	}
	expense, err := repo.createManualExpense(ctx, owner.ID, manualExpense{PropertyID: &ownerProperty.ID, Description: "Owner repair", Category: "repair", AmountCents: 1000, Currency: "EUR", ExpenseDate: jan, PaymentMethod: "cash", RecordStatus: "active"})
	if err != nil {
		t.Fatal(err)
	}
	if expense.UserID != owner.ID {
		t.Fatalf("expense user ID = %d, want %d", expense.UserID, owner.ID)
	}
	if _, err := repo.createManualExpense(ctx, owner.ID, manualExpense{PropertyID: &otherProperty.ID, Description: "Cross-user expense", Category: "repair", AmountCents: 1000, Currency: "EUR", ExpenseDate: jan, PaymentMethod: "cash", RecordStatus: "active"}); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("cross-user expense create error = %v, want gorm.ErrRecordNotFound", err)
	}

	agreements, err := repo.listTenancyAgreements(ctx, owner.ID, agreementQuery{RoomID: ownerRoom.ID})
	if err != nil || len(agreements) != 1 || agreements[0].ID != agreement.ID {
		t.Fatalf("owner agreements = %+v, err = %v", agreements, err)
	}
	if _, err := repo.findTenancyAgreement(ctx, owner.ID, agreement.ID+otherRoom.ID); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("missing agreement lookup error = %v, want gorm.ErrRecordNotFound", err)
	}
	parties, err := repo.listAgreementParties(ctx, owner.ID, agreementPartyQuery{AgreementID: agreement.ID})
	if err != nil || len(parties) != 1 || parties[0].TenantID != ownerTenant.ID {
		t.Fatalf("owner parties = %+v, err = %v", parties, err)
	}
	charges, err := repo.listRentCharges(ctx, owner.ID, rentChargeQuery{PropertyID: ownerProperty.ID, RoomID: ownerRoom.ID, PeriodMonth: &jan})
	if err != nil || len(charges) != 1 || charges[0].ID != janCharge.ID {
		t.Fatalf("owner charges = %+v, err = %v", charges, err)
	}
	obligations, err := repo.listRentObligations(ctx, owner.ID, rentObligationQuery{PropertyID: ownerProperty.ID, RoomID: ownerRoom.ID, TenantID: ownerTenant.ID, PeriodMonth: &jan})
	if err != nil || len(obligations) != 1 || obligations[0].ID != obligation.ID {
		t.Fatalf("owner obligations = %+v, err = %v", obligations, err)
	}
	allocations, err := repo.listPaymentAllocations(ctx, owner.ID, paymentAllocationQuery{RentObligationID: obligation.ID})
	if err != nil || len(allocations) != 1 || allocations[0].ID != allocation.ID {
		t.Fatalf("owner allocations = %+v, err = %v", allocations, err)
	}
	expenses, err := repo.listManualExpenses(ctx, owner.ID, manualExpenseQuery{PropertyID: ownerProperty.ID, FromDate: &jan, ToDate: &feb})
	if err != nil || len(expenses) != 1 || expenses[0].ID != expense.ID {
		t.Fatalf("owner expenses = %+v, err = %v", expenses, err)
	}

	otherCharges, err := repo.listRentCharges(ctx, owner.ID, rentChargeQuery{PropertyID: otherProperty.ID})
	if err != nil || len(otherCharges) != 0 || otherCharges == nil {
		t.Fatalf("cross-user charge list = %+v, err = %v, want non-nil empty list", otherCharges, err)
	}
	if _, err := repo.findRentCharge(ctx, owner.ID, otherCharge.ID); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("missing charge lookup error = %v, want gorm.ErrRecordNotFound", err)
	}
}

func repositoryTestTenant(userID uint64, name string) tenant {
	return tenant{
		UserID:           userID,
		Name:             name,
		MonthlyRentCents: 100000,
		Currency:         "EUR",
		IntervalUnit:     "month",
		IntervalCount:    1,
		BillingStartDate: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		DueDay:           5,
		RentStartDate:    time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		Status:           "active",
		RoomLabel:        "legacy",
		RoomAddress:      "legacy address",
	}
}
