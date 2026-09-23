package main

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestBankTransactionPeriodPrefersDescriptionAndKeepsTransferMonthTentative(t *testing.T) {
	t.Setenv("RENT_NEXT_MONTH_FROM_DAY", "")
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
		{"transfer is suggestion", "FASTER PAYMENT", &transfer, "2026-09", false, "入账日期推测 · 待确认"},
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
	if page.ParsedPeriodDisplay != "2026-09" || page.ParsedPeriodSourceLabel != "入账日期推测 · 待确认" {
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

func TestLegacyTxnDatePeriodIsOnlyADisplaySuggestion(t *testing.T) {
	t.Setenv("RENT_NEXT_MONTH_FROM_DAY", "")
	transfer := time.Date(2026, 8, 31, 23, 0, 0, 0, time.UTC)
	april := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	old := paymentTransaction{Source: "truelayer", Direction: "income", Description: "TxnDate: 25Apr2026", TransactionTime: &transfer, ParsedPeriodMonth: &april, ParsedPeriodSource: "description_reference", MatchStatus: "matched"}
	page := transactionPageRowFromModel(old)
	if page.ParsedPeriodDisplay != "2026-09" || page.ParsedPeriodSourceLabel != "入账日期推测 · 待确认" || page.MatchStatus != "matched" {
		t.Fatalf("legacy transaction projection changed allocation state or trusted TxnDate: %+v", page)
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
	for _, want := range []string{"2026-09 <small>入账日期推测 · 待确认</small>", "识别租金月份：2026-09 · 入账日期推测 · 待确认", "2026-09 · 入账日期推测 · 待确认"} {
		if !strings.Contains(page, want) {
			t.Fatalf("tentative source missing from list or review: %q", want)
		}
	}
	months := transactionReviewMonths(source, 95000, []rentObligation{{ID: 1, PeriodMonth: monthStart(transfer), ExpectedAmountCents: 95000, Currency: "EUR"}}, nil, nil)
	if len(months) != 1 || !months[0].Highlighted || !months[0].Selectable || !strings.Contains(months[0].Note, "入账日期建议") {
		t.Fatalf("review month did not distinguish suggestion: %+v", months)
	}
}

func TestBankTransactionPeriodRequiresRentContextAndIgnoresBankDates(t *testing.T) {
	t.Setenv("RENT_NEXT_MONTH_FROM_DAY", "")
	transfer := time.Date(2026, 8, 31, 23, 0, 0, 0, time.UTC)
	for _, test := range []struct {
		name, description, wantMonth string
		explicit                     bool
	}{
		{"bank date only", "M FOLEY IE26042498608289 TxnDate: 25Apr2026", "2026-09", false},
		{"bank date and rent without month", "TxnDate: 25Apr2026 RENT", "2026-09", false},
		{"explicit rent after bank date", "TxnDate: 25Apr2026 Sep Rent", "2026-09", true},
		{"explicit rent before bank date", "Sep Rent TxnDate: 25Apr2026", "2026-09", true},
		{"deposit month", "DESPOSIT SEP-1 *MOBI DEPOSITSEP 1", "2026-09", false},
		{"rent and deposit together need review", "Deposit and Sep Rent", "2026-09", false},
		{"expense months", "Material Expense May and Jun", "2026-09", false},
		{"refund month", "Repayment May tax Refund the tax", "2026-09", false},
		{"full date and rent", "Rent paid 25Apr2026", "2026-09", false},
		{"numeric rent month", "REMAIN 50 *MOBI RENT 4/26", "2026-04", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			got := bankTransactionPeriod(test.description, &transfer)
			if got.display() != test.wantMonth || got.Explicit != test.explicit {
				t.Fatalf("evidence=%+v, want month=%s explicit=%v", got, test.wantMonth, test.explicit)
			}
		})
	}
}

func TestBankTransactionPeriodSuggestionCutoffUsesDublinDate(t *testing.T) {
	t.Setenv("RENT_NEXT_MONTH_FROM_DAY", "")
	for _, test := range []struct {
		name, timestamp, wantMonth string
	}{
		{"fourteenth", "2026-09-14T12:00:00Z", "2026-09"},
		{"fifteenth", "2026-09-15T12:00:00Z", "2026-10"},
		{"dublin midnight", "2026-08-31T23:00:00Z", "2026-09"},
		{"dublin fifteenth", "2026-09-14T23:00:00Z", "2026-10"},
		{"year rollover", "2026-12-15T12:00:00Z", "2027-01"},
	} {
		t.Run(test.name, func(t *testing.T) {
			transfer, err := time.Parse(time.RFC3339, test.timestamp)
			if err != nil {
				t.Fatal(err)
			}
			got := bankTransactionPeriod("FASTER PAYMENT", &transfer)
			if got.display() != test.wantMonth || got.Explicit || got.explicitMonth() != nil {
				t.Fatalf("evidence=%+v, want tentative %s", got, test.wantMonth)
			}
		})
	}
}

func TestBankTransactionPeriodSuggestionCutoffIsConfigurable(t *testing.T) {
	transfer := time.Date(2026, 9, 12, 12, 0, 0, 0, time.UTC)
	t.Setenv("RENT_NEXT_MONTH_FROM_DAY", "10")
	if got := bankTransactionPeriod("FASTER PAYMENT", &transfer); got.display() != "2026-10" || got.Explicit {
		t.Fatalf("configured cutoff was ignored: %+v", got)
	}
	for _, invalid := range []string{"bad", "0", "32"} {
		t.Setenv("RENT_NEXT_MONTH_FROM_DAY", invalid)
		if got := bankTransactionPeriod("FASTER PAYMENT", &transfer); got.display() != "2026-09" || got.Explicit {
			t.Fatalf("invalid cutoff %q did not fall back to 15: %+v", invalid, got)
		}
	}
}

func TestBankTxnDateOnlyNeverAutoMatches(t *testing.T) {
	transfer := time.Date(2026, 8, 31, 23, 0, 0, 0, time.UTC)
	payerID := "payer-1"
	tx := paymentTransactionInput{Source: "truelayer", Direction: "income", AmountCents: 95000, Currency: "EUR", PayerID: payerID, Description: "TxnDate: 25Apr2026 RENT", TransactionTime: &transfer}
	tenants := []tenant{{ID: 7, Name: "Tenant"}}
	payers := []tenantPayer{{TenantID: 7, PayerID: &payerID}}
	april := time.Date(2026, 4, 1, 0, 0, 0, 0, time.UTC)
	september := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	decision := decideStrictRentMatch(tx, payers, tenants, []rentObligation{
		{ID: 10, TenantID: 7, PeriodMonth: april, ExpectedAmountCents: 95000, Currency: "EUR"},
		{ID: 11, TenantID: 7, PeriodMonth: september, ExpectedAmountCents: 95000, Currency: "EUR"},
	})
	if decision.Status != "candidate" || decision.RentObligationID != 0 {
		t.Fatalf("date suggestion auto matched: %+v", decision)
	}
	if obligation, ok := selectObligationForTransaction(tx, 7, []rentObligation{{ID: 10, TenantID: 7, PeriodMonth: april, ExpectedAmountCents: 95000, Currency: "EUR"}}); ok {
		t.Fatalf("legacy candidate path selected bank date as a rent period: %+v", obligation)
	}
}

func TestNewBankTxnDateOnlyIsStoredWithoutExplicitPeriod(t *testing.T) {
	request := demoResult{Accounts: []demoAccount{{
		Account:      account{AccountID: "account-1", Currency: "EUR"},
		Transactions: json.RawMessage(`{"results":[{"transaction_id":"tx-date","timestamp":"2026-08-31T23:00:00Z","description":"TxnDate: 25Apr2026 RENT","amount":950,"currency":"EUR","transaction_type":"CREDIT"}]}`),
	}}}
	rows := normalizePaymentTransactions(request)
	if len(rows) != 1 || rows[0].ParsedPeriodMonth != nil || rows[0].ParsedPeriodSource != "" {
		t.Fatalf("date-only bank transaction received an explicit period on import: %+v", rows)
	}
}
