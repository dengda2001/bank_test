package main

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"gorm.io/gorm"
)

func TestTransactionMatchReviewScopesTenantsAndHistoryOnMySQL(t *testing.T) {
	db, sqlDB := openLedgerMySQLTestDB(t)
	if err := runMigrations(sqlDB, "../../migrations"); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	owner := user{Username: fmt.Sprintf("review-owner-%d", time.Now().UnixNano()), PasswordHash: "test"}
	other := user{Username: fmt.Sprintf("review-other-%d", time.Now().UnixNano()), PasswordHash: "test"}
	for _, row := range []*user{&owner, &other} {
		if err := db.WithContext(ctx).Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		_ = db.WithContext(ctx).Delete(&user{}, owner.ID).Error
		_ = db.WithContext(ctx).Delete(&user{}, other.ID).Error
	})
	makeTenant := func(userID uint64, name string) tenant {
		input := validTenantInputForProfile()
		input.Name = name
		row, err := newTenantService(db).createTenant(ctx, userID, input)
		if err != nil {
			t.Fatal(err)
		}
		return row
	}
	ownerTenant := makeTenant(owner.ID, "Owner tenant")
	otherTenant := makeTenant(other.ID, "Other tenant")
	payer := "SHARED BANK PAYER"
	makeTransaction := func(userID uint64, suffix string) paymentTransaction {
		row := paymentTransaction{UserID: userID, Source: "test", StableTransactionKey: fmt.Sprintf("review-%d-%s", time.Now().UnixNano(), suffix), Direction: "income", AmountCents: 95000, Currency: "EUR", PayerName: &payer, MatchStatus: "unmatched"}
		if err := db.WithContext(ctx).Create(&row).Error; err != nil {
			t.Fatal(err)
		}
		return row
	}
	source := makeTransaction(owner.ID, "source")
	ownerHistory := makeTransaction(owner.ID, "history")
	otherHistory := makeTransaction(other.ID, "history")
	review, err := newTransactionService(db).transactionMatchReview(ctx, owner.ID, source.ID, 0, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(review.TenantOptions) != 1 || review.TenantOptions[0].ID != ownerTenant.ID || review.SelectedTenantID != 0 || len(review.History) != 1 || review.History[0].DetailURL != fmt.Sprintf("/transactions?detail=%d", ownerHistory.ID) {
		t.Fatalf("review leaked another account or inferred a tenant from a payer name: %+v", review)
	}
	if _, err := newTransactionService(db).transactionMatchReview(ctx, owner.ID, otherHistory.ID, 0, 1); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("other account transaction lookup error = %v, want not found", err)
	}
	if _, err := newTransactionService(db).transactionMatchReview(ctx, owner.ID, source.ID, otherTenant.ID, 1); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("other account tenant selection error = %v, want not found", err)
	}
}
