package main

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestDunningCandidateReadModelIsScopedAndSkipsTodaySuccessOnMySQL(t *testing.T) {
	db, sqlDB := openLedgerMySQLTestDB(t)
	if err := runMigrations(sqlDB, "../../migrations"); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	ctx := context.Background()
	owner := user{Username: fmt.Sprintf("dunning-owner-%d", time.Now().UnixNano()), PasswordHash: "test"}
	otherUser := user{Username: fmt.Sprintf("dunning-other-%d", time.Now().UnixNano()), PasswordHash: "test"}
	if err := db.WithContext(ctx).Create(&owner).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.WithContext(ctx).Create(&otherUser).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.WithContext(ctx).Delete(&user{}, owner.ID).Error
		_ = db.WithContext(ctx).Delete(&user{}, otherUser.ID).Error
	})

	period := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	validInput := validTenantInputForProfile()
	validInput.Name = "Valid Recipient"
	validInput.Email = "valid@example.test"
	validInput.DisplayAlias = "Valid"
	validInput.MonthlyRent = 1000
	validInput.RentStartDate = "2026-01-01"
	validInput.BillingStartDate = "2026-01-01"
	validTenant, err := newTenantService(db).createTenant(ctx, owner.ID, validInput)
	if err != nil {
		t.Fatal(err)
	}
	noEmailInput := validInput
	noEmailInput.Name = "No Email"
	noEmailInput.Email = ""
	noEmailTenant, err := newTenantService(db).createTenant(ctx, owner.ID, noEmailInput)
	if err != nil {
		t.Fatal(err)
	}
	otherInput := validInput
	otherInput.Name = "Other User"
	otherInput.Email = "other@example.test"
	if _, err := newTenantService(db).createTenant(ctx, otherUser.ID, otherInput); err != nil {
		t.Fatal(err)
	}
	service := newObligationService(db)
	if err := service.ensureMonthlyObligations(ctx, owner.ID, period); err != nil {
		t.Fatal(err)
	}
	if err := service.ensureMonthlyObligations(ctx, otherUser.ID, period); err != nil {
		t.Fatal(err)
	}
	var validObligation rentObligation
	if err := db.WithContext(ctx).Where("user_id = ? AND tenant_id = ? AND period_month = ?", owner.ID, validTenant.ID, period).First(&validObligation).Error; err != nil {
		t.Fatal(err)
	}

	candidates, err := newDunningService(db).listCandidates(ctx, owner.ID, period, defaultRentDashboardFilters())
	if err != nil {
		t.Fatal(err)
	}
	if len(candidates) != 2 {
		t.Fatalf("candidate count=%d rows=%+v", len(candidates), candidates)
	}
	byTenant := make(map[uint64]dunningCandidate, len(candidates))
	for _, candidate := range candidates {
		byTenant[candidate.TenantID] = candidate
	}
	if byTenant[validTenant.ID].Selectable == false || byTenant[validTenant.ID].EmailValid == false {
		t.Fatalf("valid candidate=%+v", byTenant[validTenant.ID])
	}
	if byTenant[noEmailTenant.ID].Selectable || byTenant[noEmailTenant.ID].EmailValid {
		t.Fatalf("no-email candidate=%+v", byTenant[noEmailTenant.ID])
	}

	now := time.Now().UTC()
	attempt := dunningSendAttempt{
		UserID:              owner.ID,
		TenantID:            validTenant.ID,
		RentObligationID:    validObligation.ID,
		PeriodMonth:         period,
		RecipientEmail:      validInput.Email,
		TemplateKind:        dunningTemplateReminder,
		Subject:             "Rent reminder",
		Body:                "body",
		ExpectedAmountCents: validObligation.ExpectedAmountCents,
		PaidAmountCents:     0,
		BalanceAmountCents:  validObligation.ExpectedAmountCents,
		Currency:            "EUR",
		SenderDisplayName:   "Dublin Homes",
		ReplyToEmail:        "landlord@example.test",
		ServiceFromEmail:    "mailer@example.test",
		DeliveryStatus:      dunningDeliverySent,
		OperationID:         fmt.Sprintf("dunning-test-%d", now.UnixNano()),
		RequestKey:          fmt.Sprintf("request-%d", now.UnixNano()),
		RequestedAt:         now,
		SentAt:              &now,
		CreatedAt:           now,
		UpdatedAt:           now,
	}
	if err := db.WithContext(ctx).Create(&attempt).Error; err != nil {
		t.Fatal(err)
	}
	candidates, err = newDunningService(db).listCandidates(ctx, owner.ID, period, defaultRentDashboardFilters())
	if err != nil {
		t.Fatal(err)
	}
	for _, candidate := range candidates {
		if candidate.TenantID == validTenant.ID && (!candidate.SentToday || candidate.DefaultSelected) {
			t.Fatalf("today-success candidate=%+v", candidate)
		}
	}
}
