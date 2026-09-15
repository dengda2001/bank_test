package main

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

type e2eMoney struct {
	Cents    int64  `json:"cents"`
	Currency string `json:"currency"`
}

type e2eTenantFixture struct {
	Name             string   `json:"name"`
	DisplayAlias     string   `json:"display_alias"`
	Email            string   `json:"email"`
	PayerID          string   `json:"payer_id"`
	PayerNameHint    string   `json:"payer_name_hint"`
	MonthlyRent      e2eMoney `json:"monthly_rent"`
	IntervalUnit     string   `json:"interval_unit"`
	IntervalCount    int      `json:"interval_count"`
	BillingStartDate string   `json:"billing_start_date"`
	DueDay           int      `json:"due_day"`
	RentStartDate    string   `json:"rent_start_date"`
	Status           string   `json:"status"`
	RoomLabel        string   `json:"room_label"`
	RoomAddress      string   `json:"room_address"`
}

type e2ePayerFixture struct {
	Name    string `json:"name"`
	PayerID string `json:"payer_id"`
}

type e2eBankTransactionFixture struct {
	TransactionID                   string   `json:"transaction_id"`
	NormalisedProviderTransactionID string   `json:"normalised_provider_transaction_id"`
	ProviderTransactionID           string   `json:"provider_transaction_id"`
	Timestamp                       string   `json:"timestamp"`
	Description                     string   `json:"description"`
	Amount                          e2eMoney `json:"amount"`
	Direction                       string   `json:"direction"`
	TransactionType                 string   `json:"transaction_type"`
	Reference                       string   `json:"reference"`
	PayerID                         string   `json:"payer_id,omitempty"`
	PayerName                       string   `json:"payer_name,omitempty"`
}

type e2EBankFixture struct {
	AccountID    string                      `json:"account_id"`
	AccountName  string                      `json:"account_name"`
	Currency     string                      `json:"currency"`
	BatchID      string                      `json:"batch_id"`
	Transactions []e2eBankTransactionFixture `json:"transactions"`
}

type e2EExpectedSnapshot struct {
	TenantCount          int   `json:"tenant_count"`
	PayerCount           int   `json:"payer_count"`
	BankTransactionCount int   `json:"bank_transaction_count"`
	EURIncomeCents       int64 `json:"eur_income_cents"`
	GBPIncomeCents       int64 `json:"gbp_income_cents"`
	RentAllocationCents  int64 `json:"rent_allocation_cents"`
	PartialRentCents     int64 `json:"partial_rent_cents"`
	PendingIncomeCount   int   `json:"pending_income_count"`
}

type e2eFixtureManifest struct {
	SchemaVersion int                 `json:"schema_version"`
	RunID         string              `json:"run_id"`
	GeneratedAt   time.Time           `json:"generated_at"`
	Tenant        e2eTenantFixture    `json:"tenant"`
	Payer         e2ePayerFixture     `json:"payer"`
	Bank          e2EBankFixture      `json:"bank"`
	Expected      e2EExpectedSnapshot `json:"expected"`
}

func newE2EFixtureManifest(runID string, now time.Time) (e2eFixtureManifest, error) {
	if err := validateE2ERunID(runID); err != nil {
		return e2eFixtureManifest{}, err
	}
	marker := strings.TrimSpace(runID)
	tenant := e2eTenantFixture{
		Name:             marker + " tenant",
		DisplayAlias:     marker + " alias",
		Email:            marker + "@invalid.test",
		PayerID:          marker + "-payer-1",
		PayerNameHint:    marker + " payer 1",
		MonthlyRent:      e2eMoney{Cents: 95000, Currency: "EUR"},
		IntervalUnit:     "month",
		IntervalCount:    1,
		BillingStartDate: "2026-08-01",
		DueDay:           5,
		RentStartDate:    "2026-08-01",
		Status:           "active",
		RoomLabel:        marker + " room",
		RoomAddress:      marker + " address",
	}
	payer := e2ePayerFixture{Name: marker + " payer 1", PayerID: marker + "-payer-1"}
	bank := e2EBankFixture{
		AccountID:   marker + "-account-eur",
		AccountName: marker + " EUR account",
		Currency:    "EUR",
		BatchID:     marker + "-bank-batch-1",
		Transactions: []e2eBankTransactionFixture{
			{
				TransactionID:                   marker + "-tx-eur-full",
				NormalisedProviderTransactionID: marker + "-stable-eur-full",
				ProviderTransactionID:           marker + "-provider-eur-full",
				Timestamp:                       "2026-09-03T10:00:00Z",
				Description:                     marker + " rent 2026-09 full",
				Amount:                          e2eMoney{Cents: 95000, Currency: "EUR"},
				Direction:                       "income",
				TransactionType:                 "CREDIT",
				Reference:                       marker + "-reference-eur-full",
				PayerID:                         payer.PayerID,
				PayerName:                       payer.Name,
			},
			{
				TransactionID:                   marker + "-tx-eur-partial",
				NormalisedProviderTransactionID: marker + "-stable-eur-partial",
				ProviderTransactionID:           marker + "-provider-eur-partial",
				Timestamp:                       "2026-09-04T10:00:00Z",
				Description:                     marker + " rent 2026-09 partial",
				Amount:                          e2eMoney{Cents: 40000, Currency: "EUR"},
				Direction:                       "income",
				TransactionType:                 "CREDIT",
				Reference:                       marker + "-reference-eur-partial",
				PayerID:                         payer.PayerID,
				PayerName:                       payer.Name,
			},
			{
				TransactionID:                   marker + "-tx-gbp",
				NormalisedProviderTransactionID: marker + "-stable-gbp",
				ProviderTransactionID:           marker + "-provider-gbp",
				Timestamp:                       "2026-09-05T10:00:00Z",
				Description:                     marker + " foreign currency 2026-09",
				Amount:                          e2eMoney{Cents: 2500, Currency: "GBP"},
				Direction:                       "income",
				TransactionType:                 "CREDIT",
				Reference:                       marker + "-reference-gbp",
			},
			{
				TransactionID:                   marker + "-tx-eur-cross-month",
				NormalisedProviderTransactionID: marker + "-stable-eur-cross-month",
				ProviderTransactionID:           marker + "-provider-eur-cross-month",
				Timestamp:                       "2026-09-01T10:00:00Z",
				Description:                     marker + " rent 2026-08 cross-month",
				Amount:                          e2eMoney{Cents: 30000, Currency: "EUR"},
				Direction:                       "income",
				TransactionType:                 "CREDIT",
				Reference:                       marker + "-reference-eur-cross-month",
				PayerID:                         payer.PayerID,
				PayerName:                       payer.Name,
			},
			{
				TransactionID:                   marker + "-tx-eur-pending",
				NormalisedProviderTransactionID: marker + "-stable-eur-pending",
				ProviderTransactionID:           marker + "-provider-eur-pending",
				Timestamp:                       "2026-09-06T10:00:00Z",
				Description:                     marker + " pending income",
				Amount:                          e2eMoney{Cents: 5000, Currency: "EUR"},
				Direction:                       "income",
				TransactionType:                 "CREDIT",
				Reference:                       marker + "-reference-eur-pending",
			},
		},
	}
	manifest := e2eFixtureManifest{
		SchemaVersion: 1,
		RunID:         marker,
		GeneratedAt:   now.UTC(),
		Tenant:        tenant,
		Payer:         payer,
		Bank:          bank,
		Expected: e2EExpectedSnapshot{
			TenantCount:          1,
			PayerCount:           1,
			BankTransactionCount: len(bank.Transactions),
			EURIncomeCents:       95000 + 40000 + 30000 + 5000,
			GBPIncomeCents:       2500,
			RentAllocationCents:  95000,
			PartialRentCents:     40000,
			PendingIncomeCount:   2,
		},
	}
	if err := manifest.validate(); err != nil {
		return e2eFixtureManifest{}, err
	}
	return manifest, nil
}

func (manifest e2eFixtureManifest) validate() error {
	if err := validateE2ERunID(manifest.RunID); err != nil {
		return err
	}
	if manifest.SchemaVersion != 1 {
		return errors.New("unsupported E2E fixture schema version")
	}
	if manifest.Tenant.PayerID != manifest.Payer.PayerID || manifest.Tenant.PayerNameHint != manifest.Payer.Name {
		return errors.New("tenant and payer fixture identity mismatch")
	}
	for _, value := range []string{
		manifest.Tenant.Name,
		manifest.Tenant.DisplayAlias,
		manifest.Tenant.Email,
		manifest.Tenant.PayerID,
		manifest.Tenant.PayerNameHint,
		manifest.Tenant.RoomLabel,
		manifest.Tenant.RoomAddress,
		manifest.Payer.Name,
		manifest.Payer.PayerID,
		manifest.Bank.AccountID,
		manifest.Bank.AccountName,
		manifest.Bank.BatchID,
	} {
		if !strings.HasPrefix(value, manifest.RunID) {
			return fmt.Errorf("fixture value %q is missing run ID prefix", value)
		}
	}
	if len(manifest.Bank.Transactions) != manifest.Expected.BankTransactionCount {
		return errors.New("bank fixture count does not match expected snapshot")
	}
	seen := make(map[string]struct{}, len(manifest.Bank.Transactions))
	var eurIncomeCents, gbpIncomeCents int64
	pendingIncomeCount := 0
	for _, transaction := range manifest.Bank.Transactions {
		if transaction.TransactionID == "" || transaction.NormalisedProviderTransactionID == "" || transaction.ProviderTransactionID == "" || transaction.Reference == "" {
			return errors.New("bank fixture stable identifiers are required")
		}
		for _, key := range []string{transaction.TransactionID, transaction.NormalisedProviderTransactionID, transaction.ProviderTransactionID, transaction.Reference} {
			if !strings.HasPrefix(key, manifest.RunID) {
				return fmt.Errorf("fixture identifier %q is missing run ID prefix", key)
			}
			if _, exists := seen[key]; exists {
				return fmt.Errorf("fixture identifier %q is not unique", key)
			}
			seen[key] = struct{}{}
		}
		if transaction.Amount.Cents <= 0 || transaction.Amount.Currency == "" {
			return errors.New("bank fixture amounts must be positive and have a currency")
		}
		if !strings.HasPrefix(transaction.Description, manifest.RunID) {
			return fmt.Errorf("transaction description %q is missing run ID prefix", transaction.Description)
		}
		if transaction.Direction == "income" {
			switch transaction.Amount.Currency {
			case "EUR":
				eurIncomeCents += transaction.Amount.Cents
			case "GBP":
				gbpIncomeCents += transaction.Amount.Cents
			}
			if transaction.PayerID == "" && transaction.PayerName == "" {
				pendingIncomeCount++
			}
		}
	}
	if eurIncomeCents != manifest.Expected.EURIncomeCents || gbpIncomeCents != manifest.Expected.GBPIncomeCents || pendingIncomeCount != manifest.Expected.PendingIncomeCount {
		return errors.New("bank fixture totals do not match expected snapshot")
	}
	return nil
}
