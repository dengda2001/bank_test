package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
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
	InvoiceURL     *string
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

type expenseService struct {
	db *gorm.DB
}

type expenseInput struct {
	PropertyID    *uint64
	RoomID        *uint64
	Description   string
	Category      string
	Amount        float64
	Currency      string
	ExpenseDate   string
	PaymentMethod string
	RoomHint      string
	TenantHint    string
	InvoiceURL    string
}

type expenseDrawerData struct {
	Period             string
	Today              string
	Properties         []expensePropertyOption
	Rooms              []expenseRoomOption
	SelectedPropertyID uint64
	SelectedRoomID     uint64
	ReturnURL          string
	PostReturnURL      string
	Error              string
}

func (a *app) loadExpenseDrawerData(ctx context.Context, userID uint64, period string, propertyID, roomID uint64, returnURL, errorCode string) (*expenseDrawerData, error) {
	data := &expenseDrawerData{
		Period:             validatedPeriodValue(period),
		Today:              time.Now().UTC().Format(dateLayout),
		SelectedPropertyID: propertyID,
		SelectedRoomID:     roomID,
		ReturnURL:          returnURL,
		PostReturnURL:      expenseFormRedirectURL(returnURL, "", ""),
		Error:              errorCode,
	}
	if userID == 0 || a.db == nil {
		return data, nil
	}
	repo := newLandlordRentRepository(a.db)
	properties, err := repo.listProperties(ctx, userID, propertyQuery{})
	if err != nil {
		return nil, err
	}
	rooms, err := repo.listRooms(ctx, userID, roomQuery{})
	if err != nil {
		return nil, err
	}
	propertyByID := make(map[uint64]property, len(properties))
	roomByID := make(map[uint64]room, len(rooms))
	for _, row := range properties {
		propertyByID[row.ID] = row
	}
	for _, row := range rooms {
		roomByID[row.ID] = row
	}
	if roomID > 0 {
		if roomRow, ok := roomByID[roomID]; ok {
			if propertyID == 0 || propertyID == roomRow.PropertyID {
				data.SelectedPropertyID = roomRow.PropertyID
			} else {
				data.SelectedRoomID = 0
			}
		} else {
			data.SelectedRoomID = 0
		}
	}
	for _, row := range properties {
		if row.Status == "active" || row.ID == data.SelectedPropertyID {
			data.Properties = append(data.Properties, expensePropertyOption{ID: row.ID, Name: row.Name})
		}
	}
	for _, row := range rooms {
		if row.Status == "active" || row.ID == data.SelectedRoomID {
			data.Rooms = append(data.Rooms, expenseRoomOption{ID: row.ID, PropertyID: row.PropertyID, Label: row.RoomLabel})
		}
	}
	if data.SelectedPropertyID > 0 {
		if _, ok := propertyByID[data.SelectedPropertyID]; !ok {
			data.SelectedPropertyID = 0
			data.SelectedRoomID = 0
		}
	}
	return data, nil
}

func expenseFormReturnURL(r *http.Request) string {
	if r == nil || r.URL == nil {
		return "/expenses"
	}
	query := r.URL.Query()
	for _, key := range []string{"expense", "add", "cash", "message", "error", "edit"} {
		query.Del(key)
	}
	returnURL := r.URL.Path
	if encoded := query.Encode(); encoded != "" {
		returnURL += "?" + encoded
	}
	return returnURL
}

func isExpenseFormError(code string) bool {
	switch code {
	case "invalid_form", "invalid_expense":
		return true
	default:
		return false
	}
}

func expenseFormRedirectURL(returnTo, message, errorCode string) string {
	target, err := url.ParseRequestURI(strings.TrimSpace(returnTo))
	if err != nil || target.IsAbs() || target.Host != "" || strings.HasPrefix(target.Path, "//") || !validExpenseReturnPath(target.Path) {
		target = &url.URL{Path: "/expenses"}
	}
	query := target.Query()
	query.Del("message")
	query.Del("error")
	if errorCode != "" {
		query.Set("error", errorCode)
		if target.Path == "/expenses" {
			query.Set("add", "1")
		} else {
			query.Set("expense", "1")
		}
	} else {
		query.Del("add")
		query.Del("expense")
		if message != "" {
			query.Set("message", message)
		}
	}
	target.RawQuery = query.Encode()
	return target.RequestURI()
}

func validExpenseReturnPath(path string) bool {
	if path == "/expenses" || path == "/transactions" {
		return true
	}
	for _, prefix := range []string{"/properties/", "/rooms/"} {
		if !strings.HasPrefix(path, prefix) {
			continue
		}
		id := strings.TrimPrefix(path, prefix)
		if id == "" || strings.Contains(id, "/") {
			return false
		}
		parsed, err := strconv.ParseUint(id, 10, 64)
		return err == nil && parsed > 0
	}
	return false
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
	var propertyID, roomID *uint64
	for key, target := range map[string]**uint64{"property_id": &propertyID, "room_id": &roomID} {
		raw := strings.TrimSpace(values.Get(key))
		if raw == "" {
			continue
		}
		parsed, parseErr := strconv.ParseUint(raw, 10, 64)
		if parseErr != nil || parsed == 0 {
			return expenseInput{}, errors.New("asset id must be a positive integer")
		}
		*target = &parsed
	}
	return expenseInput{
		PropertyID:    propertyID,
		RoomID:        roomID,
		Description:   strings.TrimSpace(values.Get("description")),
		Category:      firstNonEmpty(values.Get("category"), "General"),
		Amount:        amount,
		Currency:      firstNonEmpty(strings.ToUpper(strings.TrimSpace(values.Get("currency"))), "EUR"),
		ExpenseDate:   expenseDate,
		PaymentMethod: firstNonEmpty(values.Get("payment_method"), "Manual"),
		RoomHint:      strings.TrimSpace(values.Get("room_hint")),
		TenantHint:    strings.TrimSpace(values.Get("tenant_hint")),
		InvoiceURL:    strings.TrimSpace(values.Get("invoice_url")),
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
	if input.InvoiceURL != "" {
		if err := validateExternalHTTPURL(input.InvoiceURL); err != nil {
			return fmt.Errorf("invoice url: %w", err)
		}
	}
	return nil
}

func validateExternalHTTPURL(value string) error {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	if len(value) > 2048 {
		return errors.New("url is too long")
	}
	parsed, err := url.ParseRequestURI(value)
	if err != nil || !parsed.IsAbs() || parsed.Host == "" {
		return errors.New("url must be an absolute http(s) url")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return errors.New("url must use http or https")
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
	if input.PropertyID != nil {
		if _, err := newLandlordRentRepository(s.db).findProperty(ctx, userID, *input.PropertyID); err != nil {
			return manualExpense{}, err
		}
	}
	if input.RoomID != nil {
		roomRow, err := newLandlordRentRepository(s.db).findRoom(ctx, userID, *input.RoomID)
		if err != nil {
			return manualExpense{}, err
		}
		if input.PropertyID != nil && roomRow.PropertyID != *input.PropertyID {
			return manualExpense{}, gorm.ErrRecordNotFound
		}
	}
	expenseDate, _ := parseDate(input.ExpenseDate)
	row := manualExpense{
		UserID:        userID,
		PropertyID:    input.PropertyID,
		RoomID:        input.RoomID,
		Description:   input.Description,
		Category:      input.Category,
		AmountCents:   moneyToCents(input.Amount),
		Currency:      input.Currency,
		ExpenseDate:   expenseDate,
		PaymentMethod: input.PaymentMethod,
		RoomHint:      nullableString(input.RoomHint),
		TenantHint:    nullableString(input.TenantHint),
		InvoiceURL:    nullableString(input.InvoiceURL),
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
		PropertyID:    optionalUint64String(row.PropertyID),
		RoomID:        optionalUint64String(row.RoomID),
		Description:   row.Description,
		Category:      row.Category,
		Amount:        centsToMoney(row.AmountCents),
		Currency:      row.Currency,
		ExpenseDate:   row.ExpenseDate.Format(dateLayout),
		PaymentMethod: row.PaymentMethod,
		RoomHint:      stringValue(row.RoomHint),
		TenantHint:    stringValue(row.TenantHint),
		InvoiceURL:    stringValue(row.InvoiceURL),
		CreatedAt:     row.CreatedAt.Format(time.RFC3339),
	}
	record.AmountDisplay = formatMoney(record.Amount, record.Currency, 2)
	record.DateDisplay = formatDate(record.ExpenseDate)
	return record
}

func optionalUint64String(value *uint64) string {
	if value == nil || *value == 0 {
		return ""
	}
	return strconv.FormatUint(*value, 10)
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
