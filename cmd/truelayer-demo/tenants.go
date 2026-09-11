package main

import (
	"context"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
)

type tenant struct {
	ID               uint64 `gorm:"primaryKey"`
	UserID           uint64
	Name             string
	PayerID          *string
	PayerNameHint    *string
	MonthlyRentCents int64
	Currency         string
	IntervalUnit     string
	IntervalCount    int
	BillingStartDate time.Time
	DueDay           int
	RentStartDate    time.Time
	RentEndDate      *time.Time
	Status           string
	RoomLabel        string
	RoomAddress      string
	PropertyHint     *string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type tenantService struct {
	db *gorm.DB
}

type tenantInput struct {
	Name             string
	PayerID          string
	PayerNameHint    string
	MonthlyRent      float64
	Currency         string
	IntervalUnit     string
	IntervalCount    int
	BillingStartDate string
	DueDay           int
	RentStartDate    string
	RentEndDate      string
	Status           string
	RoomLabel        string
	RoomAddress      string
	PropertyHint     string
}

func newTenantService(db *gorm.DB) *tenantService {
	return &tenantService{db: db}
}

func tenantInputFromForm(values formValues) (tenantInput, error) {
	monthlyRent, err := parsePositiveAmount(values.Get("monthly_rent"))
	if err != nil {
		return tenantInput{}, err
	}
	dueDay := 1
	if raw := strings.TrimSpace(values.Get("due_day")); raw != "" {
		dueDay, err = strconv.Atoi(raw)
		if err != nil {
			return tenantInput{}, err
		}
	}
	intervalCount := 1
	if raw := strings.TrimSpace(values.Get("interval_count")); raw != "" {
		intervalCount, err = strconv.Atoi(raw)
		if err != nil {
			return tenantInput{}, err
		}
	}
	return tenantInput{
		Name:             strings.TrimSpace(values.Get("name")),
		PayerID:          strings.TrimSpace(values.Get("payer_id")),
		PayerNameHint:    strings.TrimSpace(values.Get("payer_name_hint")),
		MonthlyRent:      monthlyRent,
		Currency:         firstNonEmpty(strings.ToUpper(strings.TrimSpace(values.Get("currency"))), "EUR"),
		IntervalUnit:     firstNonEmpty(strings.ToLower(strings.TrimSpace(values.Get("interval_unit"))), "month"),
		IntervalCount:    intervalCount,
		BillingStartDate: strings.TrimSpace(values.Get("billing_start_date")),
		DueDay:           dueDay,
		RentStartDate:    strings.TrimSpace(values.Get("rent_start_date")),
		RentEndDate:      strings.TrimSpace(values.Get("rent_end_date")),
		Status:           firstNonEmpty(strings.ToLower(strings.TrimSpace(values.Get("status"))), "active"),
		RoomLabel:        strings.TrimSpace(values.Get("room_label")),
		RoomAddress:      strings.TrimSpace(values.Get("room_address")),
		PropertyHint:     strings.TrimSpace(values.Get("property_hint")),
	}, nil
}

type formValues interface {
	Get(string) string
}

func validateTenantInput(input tenantInput) error {
	if input.Name == "" {
		return errors.New("tenant name is required")
	}
	if input.MonthlyRent <= 0 {
		return errors.New("monthly rent must be positive")
	}
	if input.Currency == "" || len(input.Currency) != 3 {
		return errors.New("currency must be a 3-letter code")
	}
	if input.IntervalUnit != "month" {
		return errors.New("only monthly rent interval is supported in phase 1")
	}
	if input.IntervalCount != 1 {
		return errors.New("only interval_count=1 is supported in phase 1")
	}
	if input.DueDay < 1 || input.DueDay > 31 {
		return errors.New("due day must be between 1 and 31")
	}
	if input.RoomAddress == "" {
		return errors.New("room address is required")
	}
	if input.RentStartDate == "" {
		return errors.New("rent start date is required")
	}
	if input.BillingStartDate == "" {
		return errors.New("billing start date is required")
	}
	if _, err := parseDate(input.RentStartDate); err != nil {
		return fmt.Errorf("invalid rent start date: %w", err)
	}
	if _, err := parseDate(input.BillingStartDate); err != nil {
		return fmt.Errorf("invalid billing start date: %w", err)
	}
	if input.RentEndDate != "" {
		if _, err := parseDate(input.RentEndDate); err != nil {
			return fmt.Errorf("invalid rent end date: %w", err)
		}
	}
	if input.Status != "active" && input.Status != "inactive" {
		return errors.New("status must be active or inactive")
	}
	return nil
}

func isValidationError(err error) bool {
	if err == nil {
		return false
	}
	text := err.Error()
	return strings.Contains(text, "required") ||
		strings.Contains(text, "must be") ||
		strings.Contains(text, "invalid") ||
		strings.Contains(text, "supported")
}

func (s *tenantService) createTenant(ctx context.Context, userID uint64, input tenantInput) (tenant, error) {
	if userID == 0 {
		return tenant{}, errors.New("userID is required")
	}
	if err := validateTenantInput(input); err != nil {
		return tenant{}, err
	}
	billingStart, _ := parseDate(input.BillingStartDate)
	rentStart, _ := parseDate(input.RentStartDate)
	var rentEnd *time.Time
	if input.RentEndDate != "" {
		d, _ := parseDate(input.RentEndDate)
		rentEnd = &d
	}
	row := tenant{
		UserID:           userID,
		Name:             input.Name,
		PayerID:          nullableString(input.PayerID),
		PayerNameHint:    nullableString(input.PayerNameHint),
		MonthlyRentCents: moneyToCents(input.MonthlyRent),
		Currency:         input.Currency,
		IntervalUnit:     input.IntervalUnit,
		IntervalCount:    input.IntervalCount,
		BillingStartDate: billingStart,
		DueDay:           input.DueDay,
		RentStartDate:    rentStart,
		RentEndDate:      rentEnd,
		Status:           input.Status,
		RoomLabel:        input.RoomLabel,
		RoomAddress:      input.RoomAddress,
		PropertyHint:     nullableString(input.PropertyHint),
	}
	if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
		return tenant{}, err
	}
	return row, nil
}

func (s *tenantService) updateTenant(ctx context.Context, userID, tenantID uint64, input tenantInput) (tenant, error) {
	if userID == 0 || tenantID == 0 {
		return tenant{}, errors.New("userID and tenantID are required")
	}
	if err := validateTenantInput(input); err != nil {
		return tenant{}, err
	}
	billingStart, _ := parseDate(input.BillingStartDate)
	rentStart, _ := parseDate(input.RentStartDate)
	var rentEnd *time.Time
	if input.RentEndDate != "" {
		d, _ := parseDate(input.RentEndDate)
		rentEnd = &d
	}
	var row tenant
	if err := s.db.WithContext(ctx).Where("id = ? AND user_id = ?", tenantID, userID).First(&row).Error; err != nil {
		return tenant{}, err
	}
	updates := map[string]any{
		"name":               input.Name,
		"payer_id":           nullableString(input.PayerID),
		"payer_name_hint":    nullableString(input.PayerNameHint),
		"monthly_rent_cents": moneyToCents(input.MonthlyRent),
		"currency":           input.Currency,
		"interval_unit":      input.IntervalUnit,
		"interval_count":     input.IntervalCount,
		"billing_start_date": billingStart,
		"due_day":            input.DueDay,
		"rent_start_date":    rentStart,
		"rent_end_date":      rentEnd,
		"status":             input.Status,
		"room_label":         input.RoomLabel,
		"room_address":       input.RoomAddress,
		"property_hint":      nullableString(input.PropertyHint),
	}
	if err := s.db.WithContext(ctx).Model(&row).Updates(updates).Error; err != nil {
		return tenant{}, err
	}
	if err := s.db.WithContext(ctx).Where("id = ? AND user_id = ?", tenantID, userID).First(&row).Error; err != nil {
		return tenant{}, err
	}
	return row, nil
}

func (s *tenantService) listTenants(ctx context.Context, userID uint64) ([]tenantRecord, error) {
	var rows []tenant
	if err := s.db.WithContext(ctx).Where("user_id = ?", userID).Order("name ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	records := make([]tenantRecord, 0, len(rows))
	for _, row := range rows {
		records = append(records, tenantRecordFromModel(row))
	}
	return records, nil
}

func tenantRecordFromModel(row tenant) tenantRecord {
	record := tenantRecord{
		ID:               strconv.FormatUint(row.ID, 10),
		Name:             row.Name,
		PayerID:          stringValue(row.PayerID),
		PayerNameHint:    stringValue(row.PayerNameHint),
		MonthlyRent:      centsToMoney(row.MonthlyRentCents),
		Currency:         row.Currency,
		IntervalUnit:     row.IntervalUnit,
		IntervalCount:    row.IntervalCount,
		BillingStartDate: row.BillingStartDate.Format(dateLayout),
		DueDay:           row.DueDay,
		RentStartDate:    row.RentStartDate.Format(dateLayout),
		Status:           row.Status,
		RoomLabel:        row.RoomLabel,
		RoomAddress:      row.RoomAddress,
		PropertyHint:     stringValue(row.PropertyHint),
		CreatedAt:        row.CreatedAt.Format(time.RFC3339),
	}
	if row.RentEndDate != nil {
		record.RentEndDate = row.RentEndDate.Format(dateLayout)
	}
	record.RentDisplay = formatMoney(record.MonthlyRent, record.Currency, 2)
	return record
}

func parseDate(value string) (time.Time, error) {
	return time.Parse(dateLayout, strings.TrimSpace(value))
}

func moneyToCents(amount float64) int64 {
	return int64(math.Round(amount * 100))
}

func centsToMoney(cents int64) float64 {
	return float64(cents) / 100
}

func nullableString(value string) *string {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil
	}
	return &value
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
