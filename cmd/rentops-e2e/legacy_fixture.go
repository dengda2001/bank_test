package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type e2eLegacyPaths struct {
	BankLogFile string
	TenantFile  string
	ExpenseFile string
}

type e2eLegacyResult struct {
	Environment string             `json:"environment"`
	FetchedAt   string             `json:"fetched_at"`
	SyncRunID   string             `json:"sync_run_id"`
	Accounts    []e2eLegacyAccount `json:"accounts"`
}

type e2eLegacyAccount struct {
	Account      e2eLegacyAccountInfo     `json:"account"`
	Transactions e2eLegacyTransactionList `json:"transactions"`
}

type e2eLegacyAccountInfo struct {
	AccountID   string `json:"account_id"`
	AccountType string `json:"account_type"`
	DisplayName string `json:"display_name"`
	Currency    string `json:"currency"`
}

type e2eLegacyTransactionList struct {
	Results []e2eLegacyTransaction `json:"results"`
}

type e2eLegacyTransaction struct {
	TransactionID                   string         `json:"transaction_id"`
	NormalisedProviderTransactionID string         `json:"normalised_provider_transaction_id"`
	ProviderTransactionID           string         `json:"provider_transaction_id"`
	Timestamp                       string         `json:"timestamp"`
	Description                     string         `json:"description"`
	Amount                          json.Number    `json:"amount"`
	Currency                        string         `json:"currency"`
	TransactionType                 string         `json:"transaction_type"`
	TransactionCategory             string         `json:"transaction_category"`
	Reference                       string         `json:"reference"`
	PayerID                         string         `json:"payer_id,omitempty"`
	PayerName                       string         `json:"payer_name,omitempty"`
	Meta                            map[string]any `json:"meta,omitempty"`
}

func materializeE2ELegacyFixtures(manifest e2eFixtureManifest, directory string) (e2eLegacyPaths, error) {
	if err := manifest.validate(); err != nil {
		return e2eLegacyPaths{}, fmt.Errorf("validate fixture manifest: %w", err)
	}
	directory = strings.TrimSpace(directory)
	if directory == "" {
		return e2eLegacyPaths{}, errors.New("fixture directory is required")
	}
	absoluteDirectory, err := filepath.Abs(directory)
	if err != nil {
		return e2eLegacyPaths{}, fmt.Errorf("resolve fixture directory: %w", err)
	}
	if absoluteDirectory == string(filepath.Separator) {
		return e2eLegacyPaths{}, errors.New("fixture directory cannot be filesystem root")
	}
	if err := os.MkdirAll(absoluteDirectory, 0o700); err != nil {
		return e2eLegacyPaths{}, fmt.Errorf("create fixture directory: %w", err)
	}
	directoryInfo, err := os.Lstat(absoluteDirectory)
	if err != nil {
		return e2eLegacyPaths{}, fmt.Errorf("inspect fixture directory metadata: %w", err)
	}
	if !directoryInfo.IsDir() {
		return e2eLegacyPaths{}, errors.New("fixture directory path is not a directory")
	}
	if directoryInfo.Mode().Perm()&0o077 != 0 {
		return e2eLegacyPaths{}, errors.New("fixture directory must not be accessible by group or other users")
	}
	entries, err := os.ReadDir(absoluteDirectory)
	if err != nil {
		return e2eLegacyPaths{}, fmt.Errorf("inspect fixture directory: %w", err)
	}
	if len(entries) != 0 {
		return e2eLegacyPaths{}, errors.New("fixture directory must be empty; refusing to overwrite existing data")
	}
	paths := e2eLegacyPaths{
		BankLogFile: filepath.Join(absoluteDirectory, "bank-results.jsonl"),
		TenantFile:  filepath.Join(absoluteDirectory, "tenants.json"),
		ExpenseFile: filepath.Join(absoluteDirectory, "expenses.json"),
	}
	contents := map[string][]byte{}
	bankResult, err := legacyResultFromManifest(manifest)
	if err != nil {
		return e2eLegacyPaths{}, err
	}
	bankBody, err := json.Marshal(bankResult)
	if err != nil {
		return e2eLegacyPaths{}, fmt.Errorf("encode bank fixture: %w", err)
	}
	contents[paths.BankLogFile] = append(bankBody, '\n')
	contents[paths.TenantFile] = []byte("[]\n")
	contents[paths.ExpenseFile] = []byte("[]\n")
	created := make([]string, 0, len(contents))
	for path, body := range contents {
		file, createErr := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if createErr != nil {
			for _, createdPath := range created {
				_ = os.Remove(createdPath)
			}
			return e2eLegacyPaths{}, fmt.Errorf("create fixture file %s: %w", filepath.Base(path), createErr)
		}
		written, writeErr := file.Write(body)
		if writeErr != nil {
			_ = file.Close()
			for _, createdPath := range append(created, path) {
				_ = os.Remove(createdPath)
			}
			return e2eLegacyPaths{}, fmt.Errorf("write fixture file %s: %w", filepath.Base(path), writeErr)
		}
		if written != len(body) {
			_ = file.Close()
			for _, createdPath := range append(created, path) {
				_ = os.Remove(createdPath)
			}
			return e2eLegacyPaths{}, fmt.Errorf("write fixture file %s: %w", filepath.Base(path), io.ErrShortWrite)
		}
		if closeErr := file.Close(); closeErr != nil {
			for _, createdPath := range append(created, path) {
				_ = os.Remove(createdPath)
			}
			return e2eLegacyPaths{}, fmt.Errorf("close fixture file %s: %w", filepath.Base(path), closeErr)
		}
		created = append(created, path)
	}
	return paths, nil
}

func legacyResultFromManifest(manifest e2eFixtureManifest) (e2eLegacyResult, error) {
	transactions := make([]e2eLegacyTransaction, 0, len(manifest.Bank.Transactions))
	for _, fixture := range manifest.Bank.Transactions {
		amount := strconv.FormatInt(fixture.Amount.Cents/100, 10) + "." + fmt.Sprintf("%02d", fixture.Amount.Cents%100)
		transactions = append(transactions, e2eLegacyTransaction{
			TransactionID:                   fixture.TransactionID,
			NormalisedProviderTransactionID: fixture.NormalisedProviderTransactionID,
			ProviderTransactionID:           fixture.ProviderTransactionID,
			Timestamp:                       fixture.Timestamp,
			Description:                     fixture.Description,
			Amount:                          json.Number(amount),
			Currency:                        fixture.Amount.Currency,
			TransactionType:                 fixture.TransactionType,
			TransactionCategory:             fixture.TransactionType,
			Reference:                       fixture.Reference,
			PayerID:                         fixture.PayerID,
			PayerName:                       fixture.PayerName,
			Meta: map[string]any{
				"provider_reference":           fixture.Reference,
				"counter_party_preferred_name": fixture.PayerName,
			},
		})
	}
	return e2eLegacyResult{
		Environment: "e2e",
		FetchedAt:   manifest.GeneratedAt.UTC().Format("2006-01-02T15:04:05Z"),
		SyncRunID:   manifest.Bank.BatchID,
		Accounts: []e2eLegacyAccount{{
			Account: e2eLegacyAccountInfo{
				AccountID:   manifest.Bank.AccountID,
				AccountType: "TRANSACTION",
				DisplayName: manifest.Bank.AccountName,
				Currency:    manifest.Bank.Currency,
			},
			Transactions: e2eLegacyTransactionList{Results: transactions},
		}},
	}, nil
}
