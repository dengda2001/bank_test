package main

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestTransactionKeywordSearchOnMySQL(t *testing.T) {
	db, sqlDB := openLedgerMySQLTestDB(t)
	if err := runMigrations(sqlDB, "../../migrations"); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	owner := user{Username: fmt.Sprintf("transaction-search-%d", time.Now().UnixNano()), PasswordHash: "test"}
	if err := db.Create(&owner).Error; err != nil {
		t.Fatal(err)
	}
	other := user{Username: fmt.Sprintf("transaction-search-other-%d", time.Now().UnixNano()), PasswordHash: "test"}
	if err := db.Create(&other).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.Where("user_id IN ?", []uint64{owner.ID, other.ID}).Delete(&paymentAllocation{}).Error
		_ = db.Where("user_id IN ?", []uint64{owner.ID, other.ID}).Delete(&paymentTransaction{}).Error
		_ = db.Where("user_id IN ?", []uint64{owner.ID, other.ID}).Delete(&tenant{}).Error
		_ = db.Delete(&user{}, owner.ID).Error
		_ = db.Delete(&user{}, other.ID).Error
	})
	assigned := tenant{UserID: owner.ID, Name: "Aoife Murphy", DisplayAlias: "A. Murphy", Status: "active"}
	if err := db.Create(&assigned).Error; err != nil {
		t.Fatal(err)
	}
	otherTenant := tenant{UserID: other.ID, Name: "Private Tenant", Status: "active"}
	if err := db.Create(&otherTenant).Error; err != nil {
		t.Fatal(err)
	}
	transactions := []paymentTransaction{
		{UserID: owner.ID, Source: "test", StableTransactionKey: fmt.Sprintf("search-payer-%d", owner.ID), Direction: "income", AmountCents: 100, Currency: "EUR", MatchStatus: "unmatched", PayerName: nullableString("Sender Name"), PayerID: nullableString("payer-abc"), Description: "monthly payment"},
		{UserID: owner.ID, Source: "test", StableTransactionKey: fmt.Sprintf("search-description-%d", owner.ID), Direction: "income", AmountCents: 100, Currency: "EUR", MatchStatus: "unmatched", Description: "special repair refund"},
		{UserID: owner.ID, Source: "test", StableTransactionKey: fmt.Sprintf("search-tenant-%d", owner.ID), Direction: "income", AmountCents: 100, Currency: "EUR", MatchStatus: "matched", MatchedTenantID: &assigned.ID},
		{UserID: owner.ID, Source: "test", StableTransactionKey: fmt.Sprintf("search-allocation-%d", owner.ID), Direction: "income", AmountCents: 100, Currency: "EUR", MatchStatus: "matched"},
		{UserID: other.ID, Source: "test", StableTransactionKey: fmt.Sprintf("search-private-%d", owner.ID), Direction: "income", AmountCents: 100, Currency: "EUR", MatchStatus: "matched", MatchedTenantID: &otherTenant.ID},
	}
	for index := range transactions {
		if err := db.Create(&transactions[index]).Error; err != nil {
			t.Fatal(err)
		}
	}
	allocation := paymentAllocation{UserID: owner.ID, PaymentTransactionID: transactions[3].ID, TenantID: &assigned.ID, AmountCents: 100, AllocationKind: allocationKindDeposit, Status: allocationStatusConfirmed, ConfirmedByUserID: owner.ID, ConfirmedAt: time.Now()}
	if err := db.Create(&allocation).Error; err != nil {
		t.Fatal(err)
	}
	service := newTransactionService(db)
	for _, tc := range []struct {
		keyword string
		wantIDs []uint64
	}{
		{keyword: "Sender", wantIDs: []uint64{transactions[0].ID}},
		{keyword: "payer-abc", wantIDs: []uint64{transactions[0].ID}},
		{keyword: "repair", wantIDs: []uint64{transactions[1].ID}},
		{keyword: "Aoife", wantIDs: []uint64{transactions[2].ID, transactions[3].ID}},
		{keyword: "A. Murphy", wantIDs: []uint64{transactions[2].ID, transactions[3].ID}},
		{keyword: "Private Tenant"},
	} {
		rows, total, err := service.listTransactionsPage(ctx, owner.ID, transactionFilters{Payer: tc.keyword, Page: 1, PageSize: 10})
		if err != nil {
			t.Fatalf("search %q: %v", tc.keyword, err)
		}
		if total != int64(len(tc.wantIDs)) || len(rows) != len(tc.wantIDs) {
			t.Fatalf("search %q: count=%d rows=%v, want %v", tc.keyword, total, rows, tc.wantIDs)
		}
		found := make(map[uint64]bool, len(rows))
		for _, row := range rows {
			found[row.ID] = true
		}
		for _, wantID := range tc.wantIDs {
			if !found[wantID] {
				t.Fatalf("search %q omitted transaction %d", tc.keyword, wantID)
			}
		}
	}
}
