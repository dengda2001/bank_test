package main

import (
	"context"
	"encoding/json"
	"html/template"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func executeIdentityTemplate(tmpl *template.Template, data any) (string, error) {
	var body strings.Builder
	if err := tmpl.Execute(&body, data); err != nil {
		return "", err
	}
	return body.String(), nil
}

func TestNormalizePaymentTransactionsKeepsIEReferenceOutOfPayerID(t *testing.T) {
	result := demoResult{Accounts: []demoAccount{{
		Account: account{AccountID: "acct-1", DisplayName: "Rent account", Currency: "EUR"},
		Transactions: json.RawMessage(`{"results":[{
			"transaction_id":"bank-transaction-1",
			"timestamp":"2026-09-02T08:00:00Z",
			"description":"PART RENT SEPT 26",
			"amount":640,
			"currency":"EUR",
			"transaction_type":"CREDIT",
			"payer_id":"IE26090266821225",
			"meta":{"counter_party_preferred_name":"Domingo Jose Cristo Sanches","provider_reference":"IE26090266821225"}
		}]}`),
	}}}

	rows := normalizePaymentTransactions(result)
	if len(rows) != 1 {
		t.Fatalf("transaction count=%d want 1", len(rows))
	}
	if rows[0].PayerID != "" {
		t.Fatalf("transaction reference was normalized as payer id: %q", rows[0].PayerID)
	}
	if rows[0].Reference != "IE26090266821225" {
		t.Fatalf("reference=%q want bank transaction reference", rows[0].Reference)
	}
}

func TestStrictRentMatchIgnoresIEReferenceStoredAsPayerID(t *testing.T) {
	reference := "IE26090266821225"
	period := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	decision := decideStrictRentMatch(
		paymentTransactionInput{Direction: "income", AmountCents: 64000, Currency: "EUR", PayerID: reference, ParsedPeriodMonth: &period},
		[]tenantPayer{{TenantID: 7, PayerID: &reference}},
		[]tenant{{ID: 7, Name: "Domingo Jose Cristo Sanches"}},
		[]rentObligation{{ID: 9, TenantID: 7, PeriodMonth: period, ExpectedAmountCents: 64000, Currency: "EUR"}},
	)

	if decision.Status != "unmatched" || decision.TenantID != 0 {
		t.Fatalf("transaction reference produced payer-id match: %+v", decision)
	}
}

func TestStrictRentMatchMarksCoveredReferencedMonthAsCandidate(t *testing.T) {
	payerName := "domingo jose cristo sanches"
	period := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	decision := decideStrictRentMatch(
		paymentTransactionInput{
			Direction: "income", AmountCents: 60000, Currency: "EUR",
			PayerName: "DOMINGO JOSE CRISTO SANCHES", ParsedPeriodMonth: &period,
		},
		[]tenantPayer{{TenantID: 7, PayerNameOriginal: "DOMINGO JOSE CRISTO SANCHES", PayerNameNormalized: payerName}},
		[]tenant{{ID: 7, Name: "Domingo Jose Cristo Sanches"}},
		[]rentObligation{{
			ID: 9, TenantID: 7, PeriodMonth: period, ExpectedAmountCents: 60000,
			PaidAmountCents: 60000, Currency: "EUR",
		}},
	)

	if decision.Status != "candidate" || decision.TenantID != 7 || decision.RentObligationID != 9 || !decision.PeriodMonth.Equal(period) {
		t.Fatalf("covered-month decision=%+v want remembered-tenant candidate", decision)
	}
	projection := pendingMatchProjection(decision)
	if projection.Status != "candidate" || projection.MatchedTenantID == nil || *projection.MatchedTenantID != 7 {
		t.Fatalf("covered-month projection=%+v want candidate linked to remembered tenant", projection)
	}
}

func TestOnlyUnallocatedPendingStatusesAreReconciled(t *testing.T) {
	for _, status := range []string{"unmatched", "candidate", "needs_review"} {
		if !isReconcilableMatchStatus(status) {
			t.Errorf("status %q should be reconciled", status)
		}
	}
	for _, status := range []string{"matched", "partial", "ignored"} {
		if isReconcilableMatchStatus(status) {
			t.Errorf("status %q must not be reconciled", status)
		}
	}
}

func TestReconcilePendingRentTransactionsOnMySQL(t *testing.T) {
	db, sqlDB := openLedgerMySQLTestDB(t)
	if err := runMigrations(sqlDB, filepath.Join("..", "..", "migrations")); err != nil {
		t.Fatalf("run migrations: %v", err)
	}

	ctx := context.Background()
	owner := user{Username: "payer-reconcile-owner", PasswordHash: "x"}
	if err := db.Create(&owner).Error; err != nil {
		t.Fatalf("create owner: %v", err)
	}
	t.Cleanup(func() {
		for _, table := range []string{
			"payment_transaction_actions", "payment_allocations", "payment_transactions",
			"rent_obligations", "rent_charges", "room_rent_plan_members", "room_rent_plans",
			"rooms", "properties", "tenant_payers", "tenants",
		} {
			if err := db.Exec("DELETE FROM "+table+" WHERE user_id = ?", owner.ID).Error; err != nil {
				t.Logf("cleanup %s: %v", table, err)
			}
		}
		if err := db.Where("id = ?", owner.ID).Delete(&user{}).Error; err != nil {
			t.Logf("cleanup user: %v", err)
		}
	})

	tenantRow := tenant{UserID: owner.ID, Name: "Domingo Jose Cristo Sanches", Status: "active"}
	if err := db.Create(&tenantRow).Error; err != nil {
		t.Fatalf("create tenant: %v", err)
	}
	payer := tenantPayer{
		UserID: owner.ID, TenantID: tenantRow.ID, PayerNameOriginal: "DOMINGO JOSE CRISTO SANCHES",
		PayerNameNormalized: "domingo jose cristo sanches", Source: "manual",
	}
	if err := db.Create(&payer).Error; err != nil {
		t.Fatalf("create payer relation: %v", err)
	}
	propertyRow := property{UserID: owner.ID, Name: "Reconcile House", CityRegion: "Dublin", Timezone: "Europe/Dublin", Status: "active"}
	if err := db.Create(&propertyRow).Error; err != nil {
		t.Fatalf("create property: %v", err)
	}
	roomRow := room{UserID: owner.ID, PropertyID: propertyRow.ID, RoomLabel: "01", RoomType: "single", Capacity: 1, Status: "active"}
	if err := db.Create(&roomRow).Error; err != nil {
		t.Fatalf("create room: %v", err)
	}

	createObligation := func(period time.Time, expected, paid int64) rentObligation {
		t.Helper()
		plan := roomRentPlan{UserID: owner.ID, RoomID: roomRow.ID, EffectiveFromMonth: period, EffectiveToMonth: &period, MonthlyRentCents: expected, Currency: "EUR", DueDay: 5}
		if err := db.Create(&plan).Error; err != nil {
			t.Fatalf("create rent plan: %v", err)
		}
		member := roomRentPlanMember{UserID: owner.ID, RoomRentPlanID: plan.ID, TenantID: tenantRow.ID, ResponsibilityCents: expected}
		if err := db.Create(&member).Error; err != nil {
			t.Fatalf("create rent plan member: %v", err)
		}
		charge := rentCharge{
			UserID: owner.ID, PropertyID: propertyRow.ID, RoomID: roomRow.ID, RoomRentPlanID: plan.ID,
			PeriodMonth: period, DueDate: dueDateForMonth(period, 5), ExpectedAmountCents: expected,
			Currency: "EUR", RecordStatus: obligationRecordActive,
		}
		if err := db.Create(&charge).Error; err != nil {
			t.Fatalf("create rent charge: %v", err)
		}
		obligation := rentObligation{
			UserID: owner.ID, RentChargeID: charge.ID, RoomRentPlanID: plan.ID, RoomRentPlanMemberID: member.ID,
			TenantID: tenantRow.ID, PeriodMonth: period, DueDate: charge.DueDate,
			ExpectedAmountCents: expected, PaidAmountCents: paid, Currency: "EUR", Status: "open", RecordStatus: obligationRecordActive,
		}
		if paid >= expected {
			obligation.Status = "paid"
		}
		if err := db.Create(&obligation).Error; err != nil {
			t.Fatalf("create obligation: %v", err)
		}
		return obligation
	}

	august := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	september := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	openAugust := createObligation(august, 90000, 0)
	coveredSeptember := createObligation(september, 60000, 60000)
	createTransaction := func(key string, period time.Time, amount int64, status string) paymentTransaction {
		t.Helper()
		payerName := "DOMINGO JOSE CRISTO SANCHES"
		occurredAt := period.AddDate(0, 0, 2)
		row := paymentTransaction{
			UserID: owner.ID, Source: "truelayer", StableTransactionKey: key, Direction: "income",
			AmountCents: amount, Currency: "EUR", TransactionTime: &occurredAt, Description: "RENT " + period.Format("JAN 06"),
			PayerName: &payerName, PayerNameKind: "counterparty", ParsedPeriodMonth: &period,
			ParsedPeriodSource: "description", MatchStatus: status,
		}
		if err := db.Create(&row).Error; err != nil {
			t.Fatalf("create payment transaction: %v", err)
		}
		return row
	}

	augustTransaction := createTransaction("reconcile-august", august, 90000, "unmatched")
	septemberTransaction := createTransaction("reconcile-september", september, 60000, "unmatched")
	ignoredTransaction := createTransaction("reconcile-ignored", august, 90000, "ignored")

	if err := newTransactionService(db).reconcilePendingRentTransactions(ctx, owner.ID); err != nil {
		t.Fatalf("reconcile pending transactions: %v", err)
	}

	var gotAugust, gotSeptember, gotIgnored paymentTransaction
	for id, target := range map[uint64]*paymentTransaction{
		augustTransaction.ID: &gotAugust, septemberTransaction.ID: &gotSeptember, ignoredTransaction.ID: &gotIgnored,
	} {
		if err := db.First(target, "id = ? AND user_id = ?", id, owner.ID).Error; err != nil {
			t.Fatalf("reload transaction %d: %v", id, err)
		}
	}
	if gotAugust.MatchStatus != "matched" || gotAugust.MatchedTenantID == nil || *gotAugust.MatchedTenantID != tenantRow.ID {
		t.Fatalf("open-month transaction=%+v want automatic match", gotAugust)
	}
	if gotSeptember.MatchStatus != "candidate" || gotSeptember.MatchedTenantID == nil || *gotSeptember.MatchedTenantID != tenantRow.ID {
		t.Fatalf("covered-month transaction=%+v want tenant-linked candidate", gotSeptember)
	}
	if gotIgnored.MatchStatus != "ignored" || gotIgnored.MatchedTenantID != nil {
		t.Fatalf("ignored transaction changed during reconciliation: %+v", gotIgnored)
	}
	var allocationCount int64
	if err := db.Model(&paymentAllocation{}).Where("user_id = ? AND payment_transaction_id = ? AND rent_obligation_id = ?", owner.ID, augustTransaction.ID, openAugust.ID).Count(&allocationCount).Error; err != nil {
		t.Fatalf("count automatic allocation: %v", err)
	}
	if allocationCount != 1 {
		t.Fatalf("automatic allocation count=%d want 1", allocationCount)
	}
	if err := db.First(&openAugust, "id = ? AND user_id = ?", openAugust.ID, owner.ID).Error; err != nil {
		t.Fatalf("reload August obligation: %v", err)
	}
	if openAugust.PaidAmountCents != 90000 || openAugust.Status != "paid" {
		t.Fatalf("August obligation=%+v want paid 90000", openAugust)
	}
	var coveredAllocationCount int64
	if err := db.Model(&paymentAllocation{}).Where("user_id = ? AND payment_transaction_id = ? AND rent_obligation_id = ?", owner.ID, septemberTransaction.ID, coveredSeptember.ID).Count(&coveredAllocationCount).Error; err != nil {
		t.Fatalf("count covered-month allocations: %v", err)
	}
	if coveredAllocationCount != 0 {
		t.Fatalf("covered-month allocation count=%d want 0", coveredAllocationCount)
	}
}

func TestTenantPayerInputRejectsIETransactionReferenceAsStableID(t *testing.T) {
	err := validateTenantPayerInput(tenantPayerInput{Name: "Domingo Jose Cristo Sanches", PayerID: "IE26090266821225"})
	if err == nil || !strings.Contains(err.Error(), "transaction reference") {
		t.Fatalf("IE transaction reference validation error=%v", err)
	}

	reference := "IE26090266821225"
	records := classifyTenantPayerSharing([]tenantPayer{{ID: 1, TenantID: 7, PayerID: &reference, PayerNameOriginal: "Domingo"}})
	if len(records) != 1 || records[0].PayerID != "" {
		t.Fatalf("stored transaction reference was exposed as stable payer id: %+v", records)
	}
}

func TestTenantDetailKeepsBankReferenceWithPaymentNotPayerIdentity(t *testing.T) {
	page, err := executeIdentityTemplate(tenantDetailTemplate, tenantDetailPageData{
		Tenant: tenantRecord{ID: "7", Name: "Domingo Jose Cristo Sanches", Status: "active"},
		Payers: []tenantPayerRecord{{ID: "1", Name: "DOMINGO JOSE CRISTO SANCHES"}},
		History: tenantBillingHistoryPage{Rows: []tenantBillingMonth{{
			Period: "2026-09", PeriodLabel: "2026年9月", Status: "paid", StatusLabel: "已缴清",
			Payments: []rentPaymentDetail{{AmountDisplay: "EUR 640.00", DateDisplay: "02 Sep 2026 08:00", Source: "银行", Reference: "IE26090266821225"}},
		}}},
	})
	if err != nil {
		t.Fatal(err)
	}

	if !strings.Contains(page, "流水号：IE26090266821225") {
		t.Fatal("tenant payment history does not label the bank reference as a transaction number")
	}
	payerStart := strings.Index(page, `aria-labelledby="payer-title"`)
	if payerStart < 0 {
		t.Fatal("tenant payer identity panel is missing")
	}
	payerEnd := strings.Index(page[payerStart:], "</section>")
	if payerEnd < 0 {
		t.Fatal("tenant payer identity panel is unterminated")
	}
	payerPanel := page[payerStart : payerStart+payerEnd]
	if strings.Contains(payerPanel, "IE26090266821225") || strings.Contains(payerPanel, "付款参考码") {
		t.Fatalf("transaction reference leaked into payer identity panel: %s", payerPanel)
	}
}

func TestTransactionDetailUsesPreciseBankFieldLabels(t *testing.T) {
	page, err := executeIdentityTemplate(transactionDetailPageTemplate, transactionDetailPageData{
		Transaction: transactionPageRow{
			PayerName: "DOMINGO JOSE CRISTO SANCHES", PayerID: "无付款人编号",
			AccountName: "CURRENT-043", AccountID: "15c3beae54278f5c7e1f3a3b7477fe8d",
		},
		TransactionTime: "2026-09-02 08:00",
		Reference:       "IE26090266821225",
		ProviderID:      "bank-transaction-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{
		">付款人名称</dt>", ">流水号</dt><dd>IE26090266821225</dd>", ">收款账户</dt>",
		">银行交易 ID</dt><dd>bank-transaction-1</dd>", ">稳定付款人 ID</dt>", ">收款账户 ID</dt>",
	} {
		if !strings.Contains(page, marker) {
			t.Errorf("transaction detail missing precise label %q", marker)
		}
	}
	for _, oldLabel := range []string{"银行姓名", "摘要 / 参考", "外部流水号", "付款人编号", "账户标识"} {
		if strings.Contains(page, oldLabel) {
			t.Errorf("transaction detail still renders ambiguous label %q", oldLabel)
		}
	}
}
