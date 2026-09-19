package main

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestSplitRentAmountEvenlyProducesDeterministicCents(t *testing.T) {
	got, err := splitRentAmountEvenly(100000, []uint64{11, 12})
	if err != nil {
		t.Fatal(err)
	}
	want := []rentResponsibilityInput{{TenantID: 11, AmountCents: 50000}, {TenantID: 12, AmountCents: 50000}}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("equal responsibilities = %+v; want %+v", got, want)
	}

	got, err = splitRentAmountEvenly(100001, []uint64{12, 11, 13})
	if err != nil {
		t.Fatal(err)
	}
	want = []rentResponsibilityInput{{TenantID: 12, AmountCents: 33334}, {TenantID: 11, AmountCents: 33334}, {TenantID: 13, AmountCents: 33333}}
	if len(got) != len(want) || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Fatalf("remainder responsibilities = %+v; want %+v", got, want)
	}
}

func TestScaleRentResponsibilitiesPreservesSharesAndDistributesRemainderCents(t *testing.T) {
	got, err := scaleRentResponsibilities(1001, []rentResponsibilityInput{
		{TenantID: 22, AmountCents: 2500},
		{TenantID: 11, AmountCents: 5000},
		{TenantID: 33, AmountCents: 2500},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []rentResponsibilityInput{{TenantID: 11, AmountCents: 501}, {TenantID: 22, AmountCents: 250}, {TenantID: 33, AmountCents: 250}}
	if len(got) != len(want) {
		t.Fatalf("scaled responsibilities = %+v; want %+v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("scaled responsibilities = %+v; want %+v", got, want)
		}
	}

	if _, err = scaleRentResponsibilities(1, []rentResponsibilityInput{{TenantID: 22, AmountCents: 1}, {TenantID: 11, AmountCents: 1}}); err == nil {
		t.Fatal("a total smaller than the number of tenants must not create zero-value responsibilities")
	}
}

func TestScaleRentResponsibilitiesRejectsInvalidPlans(t *testing.T) {
	for _, tc := range []struct {
		name string
		plan []rentResponsibilityInput
	}{
		{name: "empty", plan: nil},
		{name: "duplicate tenant", plan: []rentResponsibilityInput{{TenantID: 11, AmountCents: 1}, {TenantID: 11, AmountCents: 1}}},
		{name: "zero amount", plan: []rentResponsibilityInput{{TenantID: 11, AmountCents: 0}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.name == "empty" {
				if got, err := scaleRentResponsibilities(100, nil); err != nil || len(got) != 0 {
					t.Fatalf("empty responsibilities = %+v, %v; want empty plan", got, err)
				}
				return
			}
			if _, err := scaleRentResponsibilities(100, tc.plan); err == nil {
				t.Fatal("invalid responsibility plan was accepted")
			}
		})
	}
}

func TestValidateRentResponsibilityPlanRequiresExactUniquePositiveOwnership(t *testing.T) {
	valid := []rentResponsibilityInput{{TenantID: 11, AmountCents: 60000}, {TenantID: 12, AmountCents: 40000}}
	if err := validateRentResponsibilityPlan(100000, valid); err != nil {
		t.Fatalf("valid 600/400 plan rejected: %v", err)
	}

	cases := []struct {
		name string
		plan []rentResponsibilityInput
		want string
	}{
		{name: "zero tenant", plan: []rentResponsibilityInput{{TenantID: 0, AmountCents: 100000}}, want: "tenant"},
		{name: "duplicate tenant", plan: []rentResponsibilityInput{{TenantID: 11, AmountCents: 50000}, {TenantID: 11, AmountCents: 50000}}, want: "duplicate"},
		{name: "non-positive amount", plan: []rentResponsibilityInput{{TenantID: 11, AmountCents: 0}}, want: "positive"},
		{name: "missing cents", plan: []rentResponsibilityInput{{TenantID: 11, AmountCents: 99999}}, want: "sum"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateRentResponsibilityPlan(100000, tc.plan)
			if err == nil || !strings.Contains(strings.ToLower(err.Error()), tc.want) {
				t.Fatalf("error = %v; want substring %q", err, tc.want)
			}
		})
	}
}

func TestAgreementPartyActiveInMonthUsesStatusAndDateBoundaries(t *testing.T) {
	period := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	joinedAfter := period.AddDate(0, 1, 0)
	leftBefore := period.AddDate(0, 0, -1)
	for _, tc := range []struct {
		name  string
		party agreementParty
		want  bool
	}{
		{name: "active", party: agreementParty{Status: "active"}, want: true},
		{name: "inactive", party: agreementParty{Status: "voided"}, want: false},
		{name: "joined after month", party: agreementParty{Status: "active", JoinedAt: &joinedAfter}, want: false},
		{name: "left before month", party: agreementParty{Status: "active", LeftAt: &leftBefore}, want: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := agreementPartyActiveInMonth(tc.party, period); got != tc.want {
				t.Fatalf("active in month = %v; want %v", got, tc.want)
			}
		})
	}
}

func TestActiveRoomAndAgreementBoundariesUseWholeTargetMonth(t *testing.T) {
	period := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	room := room{Status: "active", ActiveFrom: period}
	if !roomActiveInMonth(room, period) {
		t.Fatal("room active on month start should be active")
	}
	inactiveBeforeMonth := period.AddDate(0, 0, -1)
	room.InactiveFrom = &inactiveBeforeMonth
	if roomActiveInMonth(room, period) {
		t.Fatal("room inactive before month end should not be active for the month")
	}

	agreement := tenancyAgreement{Status: "active", StartDate: period}
	if !tenancyAgreementCoversMonth(agreement, period) {
		t.Fatal("agreement starting on month start should cover the month")
	}
	endBeforeMonth := period.AddDate(0, 0, -1)
	agreement.EndDate = &endBeforeMonth
	if tenancyAgreementCoversMonth(agreement, period) {
		t.Fatal("agreement ending before month start should not cover the month")
	}
}

func TestBuildRentChargePlanUsesActivePartiesAndExactResponsibilities(t *testing.T) {
	period := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	plan, err := buildRentChargePlan(
		tenancyAgreement{MonthlyRentCents: 100000, Currency: "eur", Status: "active", StartDate: period},
		[]agreementParty{
			{TenantID: 11, ResponsibilityCents: 60000, Status: "active"},
			{TenantID: 12, ResponsibilityCents: 40000, Status: "active"},
			{TenantID: 13, ResponsibilityCents: 10000, Status: "left"},
		},
		period,
	)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Currency != ledgerCurrencyEUR || plan.ExpectedAmountCents != 100000 {
		t.Fatalf("plan facts = %+v", plan)
	}
	want := []rentResponsibilityInput{{TenantID: 11, AmountCents: 60000}, {TenantID: 12, AmountCents: 40000}}
	if len(plan.Responsibilities) != len(want) || plan.Responsibilities[0] != want[0] || plan.Responsibilities[1] != want[1] {
		t.Fatalf("plan responsibilities = %+v; want %+v", plan.Responsibilities, want)
	}
}

func TestBuildRentChargePlanRejectsAgreementOutsideMonthOrUnbalancedParties(t *testing.T) {
	period := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	if _, err := buildRentChargePlan(tenancyAgreement{MonthlyRentCents: 100000, Currency: "EUR", Status: "active", StartDate: period.AddDate(0, 1, 0)}, []agreementParty{{TenantID: 11, ResponsibilityCents: 100000, Status: "active"}}, period); err == nil || !strings.Contains(err.Error(), "cover") {
		t.Fatalf("agreement outside month error = %v; want coverage error", err)
	}
	if _, err := buildRentChargePlan(tenancyAgreement{MonthlyRentCents: 100000, Currency: "EUR", Status: "active", StartDate: period}, []agreementParty{{TenantID: 11, ResponsibilityCents: 90000, Status: "active"}}, period); err == nil || !strings.Contains(err.Error(), "sum") {
		t.Fatalf("unbalanced party error = %v; want sum error", err)
	}
}

func TestRentLedgerServiceCreatesOneChargeAndStableObligationsOnMySQL(t *testing.T) {
	db, sqlDB := openLedgerMySQLTestDB(t)
	if err := runMigrations(sqlDB, "../../migrations"); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	ctx := context.Background()
	owner := user{Username: "rent-ledger-generation-" + time.Now().UTC().Format("20060102150405.000000000"), PasswordHash: "test"}
	if err := db.WithContext(ctx).Create(&owner).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.WithContext(ctx).Delete(&user{}, owner.ID).Error })

	ownerTenant := repositoryTestTenant(owner.ID, "Primary tenant")
	secondTenant := repositoryTestTenant(owner.ID, "Second tenant")
	if err := db.WithContext(ctx).Create(&ownerTenant).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.WithContext(ctx).Create(&secondTenant).Error; err != nil {
		t.Fatal(err)
	}
	repo := newLandlordRentRepository(db)
	ownerProperty, err := repo.createProperty(ctx, owner.ID, property{Name: "Ledger property"})
	if err != nil {
		t.Fatal(err)
	}
	ownerRoom, err := repo.createRoom(ctx, owner.ID, room{PropertyID: ownerProperty.ID, RoomLabel: "Room 1", ActiveFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)})
	if err != nil {
		t.Fatal(err)
	}
	period := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	agreement, err := repo.createTenancyAgreement(ctx, owner.ID, tenancyAgreement{RoomID: ownerRoom.ID, StartDate: period.AddDate(0, -3, 0), MonthlyRentCents: 100000, Currency: "EUR", DueDay: 5})
	if err != nil {
		t.Fatal(err)
	}
	for _, party := range []agreementParty{
		{AgreementID: agreement.ID, TenantID: ownerTenant.ID, ResponsibilityCents: 60000},
		{AgreementID: agreement.ID, TenantID: secondTenant.ID, ResponsibilityCents: 40000},
	} {
		if _, err := repo.createAgreementParty(ctx, owner.ID, party); err != nil {
			t.Fatal(err)
		}
	}

	service := newRentLedgerService(db)
	first, err := service.ensureRentCharge(ctx, owner.ID, ownerProperty.ID, ownerRoom.ID, period)
	if err != nil {
		t.Fatal(err)
	}
	if first.Charge.ID == 0 || first.Charge.ExpectedAmountCents != 100000 || first.Charge.PropertyNameSnapshot == nil || *first.Charge.PropertyNameSnapshot != "Ledger property" {
		t.Fatalf("charge=%+v", first.Charge)
	}
	if len(first.Obligations) != 2 || first.Obligations[0].ExpectedAmountCents+first.Obligations[1].ExpectedAmountCents != 100000 {
		t.Fatalf("obligations=%+v", first.Obligations)
	}
	second, err := service.ensureRentCharge(ctx, owner.ID, ownerProperty.ID, ownerRoom.ID, period)
	if err != nil {
		t.Fatal(err)
	}
	if second.Charge.ID != first.Charge.ID || len(second.Obligations) != 2 || second.Obligations[0].ID != first.Obligations[0].ID || second.Obligations[1].ID != first.Obligations[1].ID {
		t.Fatalf("second generation=%+v; first=%+v", second, first)
	}

	payerID := ownerTenant.ID
	transaction := paymentTransaction{
		UserID:               owner.ID,
		Source:               "test",
		StableTransactionKey: "rent-ledger-full-" + time.Now().UTC().Format("20060102150405.000000000"),
		Direction:            "income",
		AmountCents:          100000,
		Currency:             "EUR",
		PayerName:            nullableString(ownerTenant.Name),
		MatchedTenantID:      &payerID,
		MatchStatus:          "unmatched",
	}
	if err := db.WithContext(ctx).Create(&transaction).Error; err != nil {
		t.Fatal(err)
	}
	transactionService := newTransactionService(db)
	summary, err := transactionService.allocateTransaction(ctx, owner.ID, transaction.ID, []transactionAllocationDraft{
		{TenantID: ownerTenant.ID, RentObligationID: first.Obligations[0].ID, AmountCents: 60000, Kind: allocationKindRent},
		{TenantID: secondTenant.ID, RentObligationID: first.Obligations[1].ID, AmountCents: 40000, Kind: allocationKindRent},
	}, "rent-ledger-full-request", "manual")
	if err != nil || summary.Status != "matched" || summary.AllocatedCents != 100000 || summary.RemainingCents != 0 {
		t.Fatalf("full allocation summary=%+v err=%v", summary, err)
	}
	var projected []rentObligation
	if err := db.WithContext(ctx).Where("user_id = ? AND rent_charge_id = ?", owner.ID, first.Charge.ID).Order("tenant_id ASC").Find(&projected).Error; err != nil {
		t.Fatal(err)
	}
	if len(projected) != 2 || projected[0].PaidAmountCents != 60000 || projected[1].PaidAmountCents != 40000 || projected[0].Status != "paid" || projected[1].Status != "paid" {
		t.Fatalf("covered obligations=%+v", projected)
	}
	retry, err := transactionService.allocateTransaction(ctx, owner.ID, transaction.ID, []transactionAllocationDraft{
		{TenantID: ownerTenant.ID, RentObligationID: first.Obligations[0].ID, AmountCents: 60000, Kind: allocationKindRent},
		{TenantID: secondTenant.ID, RentObligationID: first.Obligations[1].ID, AmountCents: 40000, Kind: allocationKindRent},
	}, "rent-ledger-full-request", "manual")
	if err != nil || retry.AllocatedCents != 100000 {
		t.Fatalf("idempotent retry=%+v err=%v", retry, err)
	}
	var allocationCount int64
	if err := db.WithContext(ctx).Model(&paymentAllocation{}).Where("user_id = ? AND payment_transaction_id = ?", owner.ID, transaction.ID).Count(&allocationCount).Error; err != nil {
		t.Fatal(err)
	}
	if allocationCount != 2 {
		t.Fatalf("allocation count=%d want 2 after retry", allocationCount)
	}
	if _, err := transactionService.revokeTransactionAllocations(ctx, owner.ID, transaction.ID, "correction", "rent-ledger-revoke-request"); err != nil {
		t.Fatal(err)
	}
	if err := db.WithContext(ctx).Where("user_id = ? AND rent_charge_id = ?", owner.ID, first.Charge.ID).Order("tenant_id ASC").Find(&projected).Error; err != nil {
		t.Fatal(err)
	}
	if projected[0].PaidAmountCents != 0 || projected[1].PaidAmountCents != 0 {
		t.Fatalf("obligations after revoke=%+v", projected)
	}

	partialTransaction := paymentTransaction{
		UserID:               owner.ID,
		Source:               "test",
		StableTransactionKey: "rent-ledger-partial-" + time.Now().UTC().Format("20060102150405.000000000"),
		Direction:            "income",
		AmountCents:          80000,
		Currency:             "EUR",
		PayerName:            nullableString(ownerTenant.Name),
		MatchedTenantID:      &payerID,
		MatchStatus:          "unmatched",
	}
	if err := db.WithContext(ctx).Create(&partialTransaction).Error; err != nil {
		t.Fatal(err)
	}
	decision := decideForTenantInPeriod(paymentTransactionInput{Direction: "income", AmountCents: 80000, Currency: "EUR"}, ownerTenant, first.Obligations, period, "auto_id")
	if decision.AllocationAmountCents != 60000 || decision.Status != "partial" {
		t.Fatalf("partial decision=%+v", decision)
	}
	if err := transactionService.applyAllocation(ctx, owner.ID, partialTransaction, decision, "auto_id"); err != nil {
		t.Fatal(err)
	}
	var partialAllocation paymentAllocation
	if err := db.WithContext(ctx).Where("user_id = ? AND payment_transaction_id = ?", owner.ID, partialTransaction.ID).First(&partialAllocation).Error; err != nil {
		t.Fatal(err)
	}
	if partialAllocation.AmountCents != 60000 || partialAllocation.TenantID == nil || *partialAllocation.TenantID != ownerTenant.ID {
		t.Fatalf("partial allocation=%+v", partialAllocation)
	}
	partialSummary := summarizeTransactionAllocations(partialTransaction, []paymentAllocation{partialAllocation})
	if partialSummary.AllocatedCents != 60000 || partialSummary.RemainingCents != 20000 || partialSummary.Status != "partial" {
		t.Fatalf("partial summary=%+v", partialSummary)
	}
}

func TestTransactionAllocationAllowsOnePaymentToCoverMultipleTenantResponsibilities(t *testing.T) {
	source := paymentTransaction{UserID: 7, Direction: "income", AmountCents: 100000, Currency: "EUR"}
	drafts := []transactionAllocationDraft{
		{TenantID: 11, RentObligationID: 21, AmountCents: 60000, Kind: allocationKindRent},
		{TenantID: 12, RentObligationID: 22, AmountCents: 40000, Kind: allocationKindRent},
	}
	err := validateTransactionAllocationDrafts(source, nil, drafts, map[uint64]rentObligation{
		21: {ID: 21, UserID: 7, TenantID: 11, ExpectedAmountCents: 60000, Currency: "EUR"},
		22: {ID: 22, UserID: 7, TenantID: 12, ExpectedAmountCents: 40000, Currency: "EUR"},
	})
	if err != nil {
		t.Fatalf("multi-tenant rent allocation rejected: %v", err)
	}
}

func TestAutomaticRentMatchCapsAllocationAtPayerResponsibility(t *testing.T) {
	period := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	decision := decideForTenantInPeriod(
		paymentTransactionInput{Direction: "income", AmountCents: 80000, Currency: "EUR"},
		tenant{ID: 11},
		[]rentObligation{{ID: 21, TenantID: 11, PeriodMonth: period, ExpectedAmountCents: 60000, Currency: "EUR"}},
		period,
		"auto_id",
	)
	if decision.Status != "partial" || decision.AllocationAmountCents != 60000 || decision.Reason == "" {
		t.Fatalf("over-responsibility decision=%+v; want partial allocation capped at 60000", decision)
	}
}

func TestRentMatchRequestKeyIsStableForDuplicateConfirmation(t *testing.T) {
	first := rentMatchRequestKey(31, 21, 60000)
	second := rentMatchRequestKey(31, 21, 60000)
	if first == "" || first != second {
		t.Fatalf("rent match keys = %q and %q; want stable non-empty key", first, second)
	}
	if first == rentMatchRequestKey(31, 22, 60000) || first == rentMatchRequestKey(31, 21, 40000) {
		t.Fatalf("rent match key does not distinguish target facts: %q", first)
	}
}

func TestProjectTransactionMatchKeepsPayerSeparateFromCoveredResponsibilities(t *testing.T) {
	payerID := uint64(11)
	coveredTenantID := uint64(12)
	source := paymentTransaction{UserID: 7, AmountCents: 100000, MatchedTenantID: &payerID}
	projected := projectTransactionMatch(source, []paymentAllocation{
		{TenantID: &payerID, AmountCents: 60000, AllocationKind: allocationKindRent, Status: allocationStatusConfirmed},
		{TenantID: &coveredTenantID, AmountCents: 40000, AllocationKind: allocationKindRent, Status: allocationStatusConfirmed},
	}, "", "")
	if projected.Status != "matched" || projected.MatchedTenantID == nil || *projected.MatchedTenantID != payerID {
		t.Fatalf("payer projection=%+v; want payer %d and matched source", projected, payerID)
	}
}
