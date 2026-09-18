package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
)

type legacyImportSummary struct {
	BankResults int
	Tenants     int
	Expenses    int
}

func (a *app) handleLegacyImport(w http.ResponseWriter, r *http.Request) {
	if !a.requireAuth(w, r) {
		return
	}
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	userID, ok := a.currentUserID(r)
	if !ok || a.db == nil {
		http.Error(w, "database session required", http.StatusBadRequest)
		return
	}
	if _, err := ImportLegacyFiles(r.Context(), a.db, userID, a.cfg); err != nil {
		http.Redirect(w, r, "/billing?error=legacy_import_failed", http.StatusFound)
		return
	}
	http.Redirect(w, r, "/billing?message=legacy_imported", http.StatusFound)
}

// ImportLegacyFiles imports the old JSON/JSONL files into the current account.
// Re-running it is safe because transactions use stable keys and records are
// matched against their account-scoped natural fields before insertion.
func ImportLegacyFiles(ctx context.Context, db *gorm.DB, userID uint64, cfg config) (legacyImportSummary, error) {
	if db == nil || userID == 0 {
		return legacyImportSummary{}, errors.New("database and userID are required")
	}
	summary := legacyImportSummary{}
	results, err := readDemoResults(cfg.LogFile)
	if err != nil {
		return summary, err
	}
	transactionStore := newTransactionService(db)
	for _, result := range results {
		if err := transactionStore.ingestDemoResult(ctx, userID, result); err != nil {
			return summary, fmt.Errorf("import bank result: %w", err)
		}
		summary.BankResults++
	}

	var tenantRows []tenantRecord
	if err := readJSONFile(cfg.TenantFile, &tenantRows); err != nil {
		return summary, fmt.Errorf("read legacy tenants: %w", err)
	}
	tenantStore := newTenantService(db)
	for _, record := range tenantRows {
		input, ok := legacyTenantInput(record)
		if !ok {
			continue
		}
		var existing tenant
		query := db.WithContext(ctx).Where("user_id = ? AND name = ? AND room_address = ?", userID, input.Name, input.RoomAddress)
		if input.PayerID != "" {
			query = db.WithContext(ctx).Where("user_id = ? AND (payer_id = ? OR (name = ? AND room_address = ?))", userID, input.PayerID, input.Name, input.RoomAddress)
		}
		if err := query.First(&existing).Error; err == nil {
			continue
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return summary, fmt.Errorf("check legacy tenant: %w", err)
		}
		if _, err := tenantStore.createTenant(ctx, userID, input); err != nil {
			return summary, fmt.Errorf("import tenant %q: %w", input.Name, err)
		}
		summary.Tenants++
	}

	var expenseRows []expenseRecord
	if err := readJSONFile(cfg.ExpenseFile, &expenseRows); err != nil {
		return summary, fmt.Errorf("read legacy expenses: %w", err)
	}
	expenseStore := newExpenseService(db)
	for _, record := range expenseRows {
		input, ok := legacyExpenseInput(record)
		if !ok {
			continue
		}
		var existing manualExpense
		if err := db.WithContext(ctx).Where("user_id = ? AND description = ? AND amount_cents = ? AND expense_date = ?", userID, input.Description, moneyToCents(input.Amount), input.ExpenseDate).First(&existing).Error; err == nil {
			continue
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return summary, fmt.Errorf("check legacy expense: %w", err)
		}
		if _, err := expenseStore.createExpense(ctx, userID, input); err != nil {
			return summary, fmt.Errorf("import expense %q: %w", input.Description, err)
		}
		summary.Expenses++
	}
	return summary, nil
}

func readDemoResults(path string) ([]demoResult, error) {
	if strings.TrimSpace(path) == "" {
		return nil, nil
	}
	body, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(strings.NewReader(string(body)))
	results := make([]demoResult, 0)
	for {
		var result demoResult
		err := decoder.Decode(&result)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		results = append(results, result)
	}
	return results, nil
}

func legacyTenantInput(record tenantRecord) (tenantInput, bool) {
	createdAt := time.Now().UTC()
	if parsed, err := time.Parse(time.RFC3339, record.CreatedAt); err == nil {
		createdAt = parsed
	}
	billingStart := firstNonEmpty(record.BillingStartDate, createdAt.Format(dateLayout))
	rentStart := firstNonEmpty(record.RentStartDate, billingStart)
	input := tenantInput{
		Name:             strings.TrimSpace(record.Name),
		DisplayAlias:     strings.TrimSpace(record.DisplayAlias),
		Email:            strings.TrimSpace(record.Email),
		PayerID:          strings.TrimSpace(record.PayerID),
		PayerNameHint:    strings.TrimSpace(record.PayerNameHint),
		MonthlyRent:      record.MonthlyRent,
		Currency:         firstNonEmpty(strings.ToUpper(record.Currency), "EUR"),
		IntervalUnit:     firstNonEmpty(record.IntervalUnit, "month"),
		IntervalCount:    record.IntervalCount,
		BillingStartDate: billingStart,
		DueDay:           record.DueDay,
		RentStartDate:    rentStart,
		RentEndDate:      record.RentEndDate,
		Status:           firstNonEmpty(record.Status, "active"),
		RoomLabel:        record.RoomLabel,
		RoomAddress:      strings.TrimSpace(record.RoomAddress),
		PropertyHint:     record.PropertyHint,
	}
	if input.IntervalCount == 0 {
		input.IntervalCount = 1
	}
	if input.DueDay == 0 {
		input.DueDay = 1
	}
	return input, input.Name != "" && input.MonthlyRent > 0 && input.RoomAddress != ""
}

func legacyExpenseInput(record expenseRecord) (expenseInput, bool) {
	date := strings.TrimSpace(record.ExpenseDate)
	if _, err := parseDate(date); err != nil {
		date = time.Now().UTC().Format(dateLayout)
	}
	input := expenseInput{
		PropertyID:    optionalUint64Pointer(record.PropertyID),
		RoomID:        optionalUint64Pointer(record.RoomID),
		Description:   strings.TrimSpace(record.Description),
		Category:      firstNonEmpty(record.Category, "General"),
		Amount:        record.Amount,
		Currency:      firstNonEmpty(strings.ToUpper(record.Currency), "EUR"),
		ExpenseDate:   date,
		PaymentMethod: firstNonEmpty(record.PaymentMethod, "Manual"),
		RoomHint:      record.RoomHint,
		TenantHint:    record.TenantHint,
		InvoiceURL:    record.InvoiceURL,
	}
	return input, input.Description != "" && input.Amount > 0
}

func optionalUint64Pointer(value string) *uint64 {
	parsed, err := strconv.ParseUint(strings.TrimSpace(value), 10, 64)
	if err != nil || parsed == 0 {
		return nil
	}
	return &parsed
}
