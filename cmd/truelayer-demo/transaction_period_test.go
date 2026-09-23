package main

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestBankTransactionPeriodPrefersDescriptionAndKeepsTransferMonthTentative(t *testing.T) {
	transfer := time.Date(2026, 9, 12, 10, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		name        string
		description string
		transfer    *time.Time
		wantMonth   string
		explicit    bool
		wantLabel   string
	}{
		{"description overrides transfer", "AUGUST RENT", &transfer, "2026-08", true, "Description"},
		{"transfer is suggestion", "FASTER PAYMENT", &transfer, "2026-09", false, "转账月份 · 待确认"},
		{"no timestamp gives no suggestion", "FASTER PAYMENT", nil, "", false, ""},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := bankTransactionPeriod(test.description, test.transfer)
			if got.display() != test.wantMonth || got.Explicit != test.explicit || got.Label != test.wantLabel {
				t.Fatalf("evidence=%+v, want month=%q explicit=%v label=%q", got, test.wantMonth, test.explicit, test.wantLabel)
			}
		})
	}
}

func TestBankReferenceOnlyMonthNeverBecomesExplicitOrAutoMatched(t *testing.T) {
	transfer := time.Date(2026, 9, 12, 10, 0, 0, 0, time.UTC)
	august := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	request := demoResult{Accounts: []demoAccount{{
		Account:      account{AccountID: "account-1", Currency: "EUR"},
		Transactions: json.RawMessage(`{"results":[{"transaction_id":"tx-1","timestamp":"2026-09-12T10:00:00Z","description":"FASTER PAYMENT","reference":"RENT-2026-08","amount":950,"currency":"EUR","transaction_type":"CREDIT","payer_id":"payer-1"}]}`),
	}}}
	rows := normalizePaymentTransactions(request)
	if len(rows) != 1 || rows[0].ParsedPeriodMonth != nil || rows[0].ParsedPeriodSource != "" {
		t.Fatalf("reference-only month was persisted: %+v", rows)
	}
	payerID := "payer-1"
	payers := []tenantPayer{{TenantID: 7, PayerID: &payerID}}
	tenants := []tenant{{ID: 7, Name: "Tenant"}}
	obligations := []rentObligation{{ID: 11, TenantID: 7, PeriodMonth: august, ExpectedAmountCents: 95000, Currency: "EUR"}}
	decision := decideStrictRentMatch(rows[0], payers, tenants, obligations)
	if decision.Status != "candidate" || decision.RentObligationID != 0 {
		t.Fatalf("reference-only month auto matched: %+v", decision)
	}
	old := paymentTransaction{Source: "truelayer", Description: "FASTER PAYMENT", Reference: "RENT-2026-08", TransactionTime: &transfer, ParsedPeriodMonth: &august, ParsedPeriodSource: "description_reference", Direction: "income", AmountCents: 95000, Currency: "EUR", PayerID: &payerID}
	page := transactionPageRowFromModel(old)
	if page.ParsedPeriodDisplay != "2026-09" || page.ParsedPeriodSourceLabel != "转账月份 · 待确认" {
		t.Fatalf("legacy row did not show tentative transfer month: %+v", page)
	}
	decision = decideStrictRentMatch(paymentTransactionInputFromModel(old), payers, tenants, obligations)
	if decision.Status != "candidate" || decision.RentObligationID != 0 {
		t.Fatalf("legacy reference-only month auto matched: %+v", decision)
	}
}

func TestLegacyBankDescriptionOverridesPersistedReferenceMonth(t *testing.T) {
	transfer := time.Date(2026, 9, 12, 10, 0, 0, 0, time.UTC)
	august := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	old := paymentTransaction{Source: "truelayer", Description: "SEPT RENT", Reference: "RENT-2026-08", TransactionTime: &transfer, ParsedPeriodMonth: &august, ParsedPeriodSource: "description_reference"}
	page := transactionPageRowFromModel(old)
	if page.ParsedPeriodDisplay != "2026-09" || page.ParsedPeriodSourceLabel != "Description" {
		t.Fatalf("description did not override legacy parsed month: %+v", page)
	}
}

func TestExpenseWithoutRentMonthHasNoTransferMonthSuggestion(t *testing.T) {
	transfer := time.Date(2026, 9, 12, 10, 0, 0, 0, time.UTC)
	row := paymentTransaction{Source: "truelayer", Direction: "expense", Description: "SUPPLIES", TransactionTime: &transfer}
	if got := transactionPeriodForModel(row); got.Month != nil {
		t.Fatalf("expense received a rent-month suggestion: %+v", got)
	}
}

func TestTransferMonthSuggestionIsLabeledInListAndReview(t *testing.T) {
	transfer := time.Date(2026, 9, 12, 10, 0, 0, 0, time.UTC)
	source := paymentTransaction{ID: 7, Source: "truelayer", Direction: "income", Currency: "EUR", AmountCents: 95000, Description: "FASTER PAYMENT", TransactionTime: &transfer, MatchStatus: "candidate"}
	row := transactionPageRowFromModel(source)
	row.DetailKey = "7"
	page := renderTransactionListPage(t, transactionListPageData{
		CanonicalPath:   "/transactions",
		TransactionRows: []transactionPageRow{row},
		MatchReview: &transactionMatchReviewData{
			Source:  row,
			History: []transactionReviewHistory{{ParsedPeriod: row.ParsedPeriodDisplay, PeriodSource: row.ParsedPeriodSourceLabel}},
		},
	})
	for _, want := range []string{"2026-09 <small>转账月份 · 待确认</small>", "识别租金月份：2026-09 · 转账月份 · 待确认", "2026-09 · 转账月份 · 待确认"} {
		if !strings.Contains(page, want) {
			t.Fatalf("tentative source missing from list or review: %q", want)
		}
	}
	months := transactionReviewMonths(source, 95000, []rentObligation{{ID: 1, PeriodMonth: monthStart(transfer), ExpectedAmountCents: 95000, Currency: "EUR"}}, nil, nil)
	if len(months) != 1 || !months[0].Highlighted || !months[0].Selectable || !strings.Contains(months[0].Note, "转账月份建议") {
		t.Fatalf("review month did not distinguish suggestion: %+v", months)
	}
}
