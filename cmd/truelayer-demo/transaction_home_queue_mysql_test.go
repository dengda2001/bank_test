package main

import (
	"fmt"
	"testing"
	"time"
)

func TestHomeQueueListMatchesDashboardCountOnMySQL(t *testing.T) {
	f := newRoomRentPlanConflictFixture(t)
	t.Cleanup(func() {
		_ = f.db.WithContext(f.ctx).Exec("DELETE FROM payment_transaction_actions WHERE user_id = ?", f.owner.ID).Error
		_ = f.db.WithContext(f.ctx).Exec("DELETE FROM payment_transactions WHERE user_id = ?", f.owner.ID).Error
	})
	month := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	create := func(direction string, received time.Time) paymentTransaction {
		row := paymentTransaction{UserID: f.owner.ID, Source: "test", StableTransactionKey: fmt.Sprintf("home-queue-%d", time.Now().UnixNano()), Direction: direction, TransactionTime: &received, AmountCents: 10000, Currency: "EUR", MatchStatus: "unmatched"}
		if err := f.db.WithContext(f.ctx).Create(&row).Error; err != nil {
			t.Fatal(err)
		}
		return row
	}
	active := create("income", month.AddDate(0, 0, 2))
	deferred := create("income", month.AddDate(0, 0, 3))
	create("expense", month.AddDate(0, 0, 4))
	create("income", month.AddDate(0, -1, 0))
	if err := newTransactionService(f.db).deferTransaction(f.ctx, f.owner.ID, deferred.ID, "人工暂缓核对"); err != nil {
		t.Fatal(err)
	}
	var dashboardCount int64
	if err := rentWorkspacePendingTransactions(f.db, f.ctx, f.owner.ID, month).Count(&dashboardCount).Error; err != nil {
		t.Fatal(err)
	}
	filters, scope := transactionListFiltersFromQuery(map[string][]string{"scope": {"home_queue"}, "period": {"2026-09"}})
	if scope != "home_queue" || !filters.HomeQueue {
		t.Fatalf("home queue filters = %+v, scope = %q", filters, scope)
	}
	rows, listCount, err := newTransactionService(f.db).listTransactionsPage(f.ctx, f.owner.ID, filters)
	if err != nil {
		t.Fatal(err)
	}
	if dashboardCount != 1 || listCount != dashboardCount || len(rows) != 1 || rows[0].ID != active.ID {
		t.Fatalf("dashboard=%d list=%d rows=%+v", dashboardCount, listCount, rows)
	}
}
