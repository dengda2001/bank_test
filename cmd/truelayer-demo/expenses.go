package main

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type manualExpense struct {
	ID             uint64 `gorm:"primaryKey"`
	UserID         uint64
	PropertyID     *uint64
	RoomID         *uint64
	Description    string
	Category       string
	AmountCents    int64
	Currency       string
	ExpenseDate    time.Time
	PaymentMethod  string
	RecordStatus   string `gorm:"default:active"`
	VoidedAt       *time.Time
	VoidedByUserID *uint64
	VoidReason     *string
	RoomHint       *string
	TenantHint     *string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type expenseService struct {
	db *gorm.DB
}

type expenseInput struct {
	Description   string
	Category      string
	Amount        float64
	Currency      string
	ExpenseDate   string
	PaymentMethod string
	RoomHint      string
	TenantHint    string
}

func newExpenseService(db *gorm.DB) *expenseService {
	return &expenseService{db: db}
}

func expenseInputFromForm(values formValues, now time.Time) (expenseInput, error) {
	amount, err := parsePositiveAmount(values.Get("amount"))
	if err != nil {
		return expenseInput{}, err
	}
	expenseDate := strings.TrimSpace(values.Get("expense_date"))
	if _, err := time.Parse(dateLayout, expenseDate); expenseDate == "" || err != nil {
		expenseDate = now.UTC().Format(dateLayout)
	}
	return expenseInput{
		Description:   strings.TrimSpace(values.Get("description")),
		Category:      firstNonEmpty(values.Get("category"), "General"),
		Amount:        amount,
		Currency:      firstNonEmpty(strings.ToUpper(strings.TrimSpace(values.Get("currency"))), "EUR"),
		ExpenseDate:   expenseDate,
		PaymentMethod: firstNonEmpty(values.Get("payment_method"), "Manual"),
		RoomHint:      strings.TrimSpace(values.Get("room_hint")),
		TenantHint:    strings.TrimSpace(values.Get("tenant_hint")),
	}, nil
}

func validateExpenseInput(input expenseInput) error {
	if input.Description == "" {
		return errors.New("description is required")
	}
	if input.Amount <= 0 {
		return errors.New("amount must be positive")
	}
	if input.Currency == "" || len(input.Currency) != 3 {
		return errors.New("currency must be a 3-letter code")
	}
	if _, err := parseDate(input.ExpenseDate); err != nil {
		return errors.New("expense date is invalid")
	}
	return nil
}

func (s *expenseService) createExpense(ctx context.Context, userID uint64, input expenseInput) (manualExpense, error) {
	if userID == 0 {
		return manualExpense{}, errors.New("userID is required")
	}
	if err := validateExpenseInput(input); err != nil {
		return manualExpense{}, err
	}
	expenseDate, _ := parseDate(input.ExpenseDate)
	row := manualExpense{
		UserID:        userID,
		Description:   input.Description,
		Category:      input.Category,
		AmountCents:   moneyToCents(input.Amount),
		Currency:      input.Currency,
		ExpenseDate:   expenseDate,
		PaymentMethod: input.PaymentMethod,
		RoomHint:      nullableString(input.RoomHint),
		TenantHint:    nullableString(input.TenantHint),
	}
	if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
		return manualExpense{}, err
	}
	tx := manualExpenseTransaction(userID, row)
	if err := s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_id"}, {Name: "stable_transaction_key"}},
		DoNothing: true,
	}).Create(&tx).Error; err != nil {
		return manualExpense{}, err
	}
	return row, nil
}

func (s *expenseService) listExpenses(ctx context.Context, userID uint64) ([]expenseRecord, error) {
	var rows []manualExpense
	if err := s.db.WithContext(ctx).Where("user_id = ?", userID).Order("expense_date DESC, id DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	records := make([]expenseRecord, 0, len(rows))
	for _, row := range rows {
		records = append(records, expenseRecordFromModel(row))
	}
	return records, nil
}

func expenseRecordFromModel(row manualExpense) expenseRecord {
	record := expenseRecord{
		ID:            strconv.FormatUint(row.ID, 10),
		Description:   row.Description,
		Category:      row.Category,
		Amount:        centsToMoney(row.AmountCents),
		Currency:      row.Currency,
		ExpenseDate:   row.ExpenseDate.Format(dateLayout),
		PaymentMethod: row.PaymentMethod,
		RoomHint:      stringValue(row.RoomHint),
		TenantHint:    stringValue(row.TenantHint),
		CreatedAt:     row.CreatedAt.Format(time.RFC3339),
	}
	record.AmountDisplay = formatMoney(record.Amount, record.Currency, 2)
	record.DateDisplay = formatDate(record.ExpenseDate)
	return record
}

func manualExpenseTransaction(userID uint64, row manualExpense) paymentTransaction {
	txTime := row.ExpenseDate
	input := paymentTransactionInput{
		Source:          "manual_expense",
		SourceBatchID:   "manual",
		Direction:       "expense",
		AmountCents:     row.AmountCents,
		Currency:        row.Currency,
		TransactionTime: &txTime,
		Description:     row.Description,
		Reference:       row.Category,
		MatchStatus:     "unmatched",
	}
	input.StableTransactionKey = "manual_expense:" + strconv.FormatUint(row.ID, 10)
	return paymentTransactionFromInput(userID, input)
}
