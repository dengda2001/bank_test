package main

import (
	"context"
	"fmt"
	"testing"
	"time"
)

func TestRentDashboardSummaryFiltersAndArrivalMetricsOnMySQL(t *testing.T) {
	db, sqlDB := openLedgerMySQLTestDB(t)
	if err := runMigrations(sqlDB, "../../migrations"); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	ctx := context.Background()
	owner := user{Username: fmt.Sprintf("dashboard-owner-%d", time.Now().UnixNano()), PasswordHash: "test"}
	otherUser := user{Username: fmt.Sprintf("dashboard-other-%d", time.Now().UnixNano()), PasswordHash: "test"}
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
	ownerTenantInput := validTenantInputForProfile()
	ownerTenantInput.MonthlyRent = 1000
	ownerTenantInput.Name = "Aoife Murphy"
	ownerTenantInput.DisplayAlias = "Aoife A"
	ownerTenantInput.RoomLabel = "A-01"
	ownerTenantInput.RentStartDate = "2026-01-01"
	ownerTenantInput.BillingStartDate = "2026-01-01"
	ownerTenant, err := newTenantService(db).createTenant(ctx, owner.ID, ownerTenantInput)
	if err != nil {
		t.Fatal(err)
	}
	secondTenantInput := ownerTenantInput
	secondTenantInput.Name = "Zoe Byrne"
	secondTenantInput.DisplayAlias = "Zoe Z"
	secondTenantInput.RoomLabel = "B-02"
	secondTenant, err := newTenantService(db).createTenant(ctx, owner.ID, secondTenantInput)
	if err != nil {
		t.Fatal(err)
	}
	thirdTenantInput := ownerTenantInput
	thirdTenantInput.Name = "Mia Chen"
	thirdTenantInput.DisplayAlias = "Mia M"
	thirdTenantInput.RoomLabel = "C-03"
	if _, err := newTenantService(db).createTenant(ctx, owner.ID, thirdTenantInput); err != nil {
		t.Fatal(err)
	}
	otherTenantInput := ownerTenantInput
	otherTenantInput.Name = "Other User Tenant"
	otherTenant, err := newTenantService(db).createTenant(ctx, otherUser.ID, otherTenantInput)
	if err != nil {
		t.Fatal(err)
	}
	obligations := newObligationService(db)
	if err := obligations.ensureMonthlyObligations(ctx, owner.ID, period); err != nil {
		t.Fatal(err)
	}
	if err := obligations.ensureMonthlyObligations(ctx, otherUser.ID, period); err != nil {
		t.Fatal(err)
	}
	var ownerObligation rentObligation
	if err := db.WithContext(ctx).Where("user_id = ? AND tenant_id = ? AND period_month = ?", owner.ID, ownerTenant.ID, period).First(&ownerObligation).Error; err != nil {
		t.Fatal(err)
	}
	var secondObligation rentObligation
	if err := db.WithContext(ctx).Where("user_id = ? AND tenant_id = ? AND period_month = ?", owner.ID, secondTenant.ID, period).First(&secondObligation).Error; err != nil {
		t.Fatal(err)
	}
	previousPeriod := period.AddDate(0, -1, 0)
	if err := obligations.ensureMonthlyObligations(ctx, owner.ID, previousPeriod); err != nil {
		t.Fatal(err)
	}
	var previousObligation rentObligation
	if err := db.WithContext(ctx).Where("user_id = ? AND tenant_id = ? AND period_month = ?", owner.ID, ownerTenant.ID, previousPeriod).First(&previousObligation).Error; err != nil {
		t.Fatal(err)
	}

	createTransaction := func(userID uint64, key string, amount int64, arrival time.Time) paymentTransaction {
		row := paymentTransaction{UserID: userID, Source: "test", StableTransactionKey: key, Direction: "income", AmountCents: amount, Currency: "EUR", TransactionTime: &arrival, MatchStatus: "unmatched"}
		if err := db.WithContext(ctx).Create(&row).Error; err != nil {
			t.Fatal(err)
		}
		return row
	}
	partialRent := createTransaction(owner.ID, fmt.Sprintf("dashboard-rent-%d", time.Now().UnixNano()), 60000, time.Date(2026, 9, 4, 9, 0, 0, 0, time.UTC))
	if _, err := newTransactionService(db).allocateTransaction(ctx, owner.ID, partialRent.ID, []transactionAllocationDraft{{TenantID: ownerTenant.ID, RentObligationID: ownerObligation.ID, AmountCents: 40000, Kind: allocationKindRent}}, "dashboard-rent-allocation", "manual"); err != nil {
		t.Fatal(err)
	}
	fullRent := createTransaction(owner.ID, fmt.Sprintf("dashboard-full-rent-%d", time.Now().UnixNano()), 100000, time.Date(2026, 9, 5, 9, 0, 0, 0, time.UTC))
	if _, err := newTransactionService(db).allocateTransaction(ctx, owner.ID, fullRent.ID, []transactionAllocationDraft{{TenantID: secondTenant.ID, RentObligationID: secondObligation.ID, AmountCents: 100000, Kind: allocationKindRent}}, "dashboard-full-rent-allocation", "manual"); err != nil {
		t.Fatal(err)
	}
	otherIncome := createTransaction(owner.ID, fmt.Sprintf("dashboard-other-%d", time.Now().UnixNano()), 50000, time.Date(2026, 9, 10, 9, 0, 0, 0, time.UTC))
	if _, err := newTransactionService(db).allocateTransaction(ctx, owner.ID, otherIncome.ID, []transactionAllocationDraft{{AmountCents: 30000, Kind: allocationKindOther}}, "dashboard-other-allocation", "manual"); err != nil {
		t.Fatal(err)
	}
	previousRent := createTransaction(owner.ID, fmt.Sprintf("dashboard-previous-rent-%d", time.Now().UnixNano()), 100000, time.Date(2026, 9, 20, 9, 0, 0, 0, time.UTC))
	if _, err := newTransactionService(db).allocateTransaction(ctx, owner.ID, previousRent.ID, []transactionAllocationDraft{{TenantID: ownerTenant.ID, RentObligationID: previousObligation.ID, AmountCents: 100000, Kind: allocationKindRent}}, "dashboard-previous-rent-allocation", "manual"); err != nil {
		t.Fatal(err)
	}
	_ = createTransaction(owner.ID, fmt.Sprintf("dashboard-october-%d", time.Now().UnixNano()), 100000, time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC))
	_ = createTransaction(otherUser.ID, fmt.Sprintf("dashboard-other-user-%d", time.Now().UnixNano()), 90000, time.Date(2026, 9, 8, 9, 0, 0, 0, time.UTC))
	_ = otherTenant

	startedAt := time.Date(2026, 9, 16, 8, 0, 0, 0, time.UTC)
	coveredFrom := period
	coveredTo := time.Date(2026, 9, 16, 8, 0, 0, 0, time.UTC)
	run := bankSyncRun{UserID: owner.ID, Provider: "truelayer", Environment: "sandbox", Mode: bankSyncModeRefresh90d, RequestedFrom: period, RequestedTo: coveredTo, Status: bankSyncStatusSucceeded, StartedAt: startedAt}
	if err := db.WithContext(ctx).Create(&run).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.WithContext(ctx).Create(&bankSyncRunAccount{UserID: owner.ID, BankSyncRunID: run.ID, AccountID: "account-1", Status: bankSyncAccountSucceeded, CoveredFrom: &coveredFrom, CoveredTo: &coveredTo}).Error; err != nil {
		t.Fatal(err)
	}

	filters := defaultRentDashboardFilters()
	filters.Search = "Aoife A"
	filters.Status = "unpaid"
	summary, err := obligations.summarizeRentDashboardWithFilters(ctx, owner.ID, period, filters)
	if err != nil {
		t.Fatal(err)
	}
	if summary.TotalRows != 3 || summary.FilteredCount != 1 || len(summary.Rows) != 1 || summary.Rows[0].TenantID != ownerTenant.ID {
		t.Fatalf("dashboard rows=%+v summary=%+v", summary.Rows, summary)
	}
	if summary.ExpectedCents != 300000 || summary.PaidCents != 140000 || summary.BalanceCents != 160000 || summary.PaidCount != 1 || summary.PartialCount != 1 || summary.OpenCount != 1 || summary.UnpaidCount != 2 {
		t.Fatalf("dashboard rent totals=%+v", summary)
	}
	if summary.IncomeCount != 4 || summary.PendingCount != 2 || summary.PendingCents != 40000 || summary.OtherIncomeCount != 1 || summary.OtherIncomeCents != 30000 {
		t.Fatalf("dashboard bank metrics=%+v", summary)
	}
	if summary.SyncStatus != bankSyncStatusSucceeded || summary.SyncCoverage == "" {
		t.Fatalf("dashboard sync=%q/%q", summary.SyncStatus, summary.SyncCoverage)
	}
	failedRun := bankSyncRun{UserID: owner.ID, Provider: "truelayer", Environment: "sandbox", Mode: bankSyncModeRefresh90d, RequestedFrom: period, RequestedTo: coveredTo, Status: bankSyncStatusFailed, StartedAt: startedAt.Add(time.Hour)}
	if err := db.WithContext(ctx).Create(&failedRun).Error; err != nil {
		t.Fatal(err)
	}
	failedSummary, err := obligations.summarizeRentDashboardWithFilters(ctx, owner.ID, period, defaultRentDashboardFilters())
	if err != nil {
		t.Fatal(err)
	}
	if failedSummary.SyncStatus != bankSyncStatusFailed || failedSummary.LastSuccessfulSyncCoverage == "" {
		t.Fatalf("dashboard failed sync=%q latest-success=%q", failedSummary.SyncStatus, failedSummary.LastSuccessfulSyncCoverage)
	}
}
