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
		row := paymentTransaction{UserID: userID, Source: "test", StableTransactionKey: fmt.Sprintf("review-%d-%s", time.Now().UnixNano(), suffix), Direction: "income", AmountCents: 95000, Currency: "EUR", PayerName: &payer, PayerNameKind: "confirmed", MatchStatus: "unmatched"}
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
	if err := newTenantService(db).rememberTenantPayer(ctx, owner.ID, ownerTenant.ID, "", payer); err != nil {
		t.Fatal(err)
	}
	review, err = newTransactionService(db).transactionMatchReview(ctx, owner.ID, source.ID, 0, 1)
	if err != nil || review.SelectedTenantID != 0 || len(review.SuggestedTenants) != 1 || review.SuggestedTenants[0].ID != ownerTenant.ID {
		t.Fatalf("remembered payer should offer a button without selecting a tenant: %+v, %v", review, err)
	}
	if _, err := newTransactionService(db).transactionMatchReview(ctx, owner.ID, otherHistory.ID, 0, 1); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("other account transaction lookup error = %v, want not found", err)
	}
	if _, err := newTransactionService(db).transactionMatchReview(ctx, owner.ID, source.ID, otherTenant.ID, 1); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("other account tenant selection error = %v, want not found", err)
	}
}

func TestTransactionReviewLocationsFollowSelectedRentMonthOnMySQL(t *testing.T) {
	f := newRoomRentPlanConflictFixture(t)
	month := dublinCurrentMonth(time.Now())
	if _, _, err := newRoomRentPlanService(f.db).SaveRoomRentPlan(f.ctx, roomRentPlanCommandFor(f, f.roomOne.ID, month)); err != nil {
		t.Fatal(err)
	}
	properties, rooms, err := loadTransactionReviewLocations(f.ctx, f.db, f.owner.ID, month)
	if err != nil {
		t.Fatal(err)
	}
	if len(properties) != 1 || len(rooms) != 2 || rooms[0].PropertyID != properties[0].ID || rooms[0].OccupantIDs != fmt.Sprintf("%d", f.tenant.ID) || rooms[1].OccupantIDs != "" {
		t.Fatalf("current month locations = %+v, %+v", properties, rooms)
	}
	_, previousRooms, err := loadTransactionReviewLocations(f.ctx, f.db, f.owner.ID, month.AddDate(0, -1, 0))
	if err != nil {
		t.Fatal(err)
	}
	if previousRooms[0].OccupantIDs != "" {
		t.Fatalf("tenant leaked into previous month: %+v", previousRooms)
	}
}

func TestDeferPendingSourceIsIdempotentAndRejectsStaleStatusOnMySQL(t *testing.T) {
	f := newRoomRentPlanConflictFixture(t)
	t.Cleanup(func() {
		_ = f.db.WithContext(f.ctx).Exec("DELETE FROM payment_transaction_actions WHERE user_id = ?", f.owner.ID).Error
		_ = f.db.WithContext(f.ctx).Exec("DELETE FROM payment_transactions WHERE user_id = ?", f.owner.ID).Error
	})
	source := paymentTransaction{UserID: f.owner.ID, Source: "test", StableTransactionKey: fmt.Sprintf("defer-review-%d", time.Now().UnixNano()), Direction: "income", AmountCents: 10000, Currency: "EUR", MatchStatus: "unmatched"}
	if err := f.db.WithContext(f.ctx).Create(&source).Error; err != nil {
		t.Fatal(err)
	}
	service := newTransactionService(f.db)
	for attempt := 0; attempt < 2; attempt++ {
		if err := service.deferTransaction(f.ctx, f.owner.ID, source.ID, "首页暂不处理"); err != nil {
			t.Fatalf("attempt %d: %v", attempt, err)
		}
	}
	deferred, err := transactionDeferredState(f.ctx, f.db, f.owner.ID, source.ID)
	if err != nil || !deferred {
		t.Fatalf("deferred = %v, %v", deferred, err)
	}
	if err := f.db.WithContext(f.ctx).Model(&paymentTransaction{}).Where("user_id = ? AND id = ?", f.owner.ID, source.ID).Update("match_status", "matched").Error; err != nil {
		t.Fatal(err)
	}
	if err := service.deferTransaction(f.ctx, f.owner.ID, source.ID, "首页暂不处理"); !errors.Is(err, ErrTransactionNotDeferrable) {
		t.Fatalf("stale source defer error = %v", err)
	}
}
