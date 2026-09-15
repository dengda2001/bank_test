package main

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/mail"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type tenant struct {
	ID               uint64 `gorm:"primaryKey"`
	UserID           uint64
	Name             string
	DisplayAlias     string
	Email            string
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

type tenantPayer struct {
	ID                  uint64 `gorm:"primaryKey"`
	UserID              uint64
	TenantID            uint64
	PayerID             *string
	PayerNameOriginal   string
	PayerNameNormalized string
	Source              string
	LastMatchedAt       *time.Time
	RemovedAt           *time.Time
	RemovedByUserID     *uint64
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

type tenantPayerInput struct {
	PayerID string
	Name    string
}

type tenantPayerRecord struct {
	ID            string
	PayerID       string
	Name          string
	Shared        bool
	Source        string
	LastMatchedAt string
	RemovedAt     string
}

type tenantInput struct {
	Name             string
	DisplayAlias     string
	Email            string
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

func validateTenantPayerInput(input tenantPayerInput) error {
	if strings.TrimSpace(input.Name) == "" {
		return errors.New("payer name is required")
	}
	if len([]rune(strings.TrimSpace(input.Name))) > 191 {
		return errors.New("payer name is too long")
	}
	if len([]rune(strings.TrimSpace(input.PayerID))) > 191 {
		return errors.New("payer id is too long")
	}
	return nil
}

func classifyTenantPayerSharing(rows []tenantPayer) []tenantPayerRecord {
	activeNameTenants := make(map[string]map[uint64]struct{})
	activeIDTenants := make(map[string]map[uint64]struct{})
	for _, row := range rows {
		if row.RemovedAt != nil {
			continue
		}
		if row.PayerNameNormalized != "" && activeNameTenants[row.PayerNameNormalized] == nil {
			activeNameTenants[row.PayerNameNormalized] = make(map[uint64]struct{})
		}
		if row.PayerNameNormalized != "" {
			activeNameTenants[row.PayerNameNormalized][row.TenantID] = struct{}{}
		}
		payerID := stringValue(row.PayerID)
		if payerID != "" && activeIDTenants[payerID] == nil {
			activeIDTenants[payerID] = make(map[uint64]struct{})
		}
		if payerID != "" {
			activeIDTenants[payerID][row.TenantID] = struct{}{}
		}
	}
	result := make([]tenantPayerRecord, 0, len(rows))
	for _, row := range rows {
		record := tenantPayerRecord{
			ID:      strconv.FormatUint(row.ID, 10),
			PayerID: stringValue(row.PayerID),
			Name:    row.PayerNameOriginal,
			Source:  row.Source,
			Shared:  len(activeNameTenants[row.PayerNameNormalized]) > 1 || len(activeIDTenants[stringValue(row.PayerID)]) > 1,
		}
		if row.LastMatchedAt != nil {
			record.LastMatchedAt = row.LastMatchedAt.Format(time.RFC3339)
		}
		if row.RemovedAt != nil {
			record.RemovedAt = row.RemovedAt.Format(time.RFC3339)
		}
		result = append(result, record)
	}
	return result
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
	rentStartDate := strings.TrimSpace(values.Get("rent_start_date"))
	billingStartDate := strings.TrimSpace(values.Get("billing_start_date"))
	if billingStartDate == "" {
		billingStartDate = rentStartDate
	}
	return tenantInput{
		Name:             strings.TrimSpace(values.Get("name")),
		DisplayAlias:     strings.TrimSpace(values.Get("display_alias")),
		Email:            strings.TrimSpace(values.Get("email")),
		PayerID:          strings.TrimSpace(values.Get("payer_id")),
		PayerNameHint:    strings.TrimSpace(values.Get("payer_name_hint")),
		MonthlyRent:      monthlyRent,
		Currency:         firstNonEmpty(strings.ToUpper(strings.TrimSpace(values.Get("currency"))), "EUR"),
		IntervalUnit:     firstNonEmpty(strings.ToLower(strings.TrimSpace(values.Get("interval_unit"))), "month"),
		IntervalCount:    intervalCount,
		BillingStartDate: billingStartDate,
		DueDay:           dueDay,
		RentStartDate:    rentStartDate,
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
	if input.Email != "" {
		parsed, err := mail.ParseAddress(input.Email)
		if err != nil || parsed.Address != input.Email {
			return errors.New("email must be a valid email address")
		}
	}
	if input.MonthlyRent <= 0 {
		return errors.New("monthly rent must be positive")
	}
	if _, err := normalizeLedgerCurrency(input.Currency); err != nil {
		return err
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
	rentStart, err := parseDate(input.RentStartDate)
	if err != nil {
		return fmt.Errorf("invalid rent start date: %w", err)
	}
	billingStart, err := parseDate(input.BillingStartDate)
	if err != nil {
		return fmt.Errorf("invalid billing start date: %w", err)
	}
	if billingStart.Before(rentStart) {
		return errors.New("billing start date cannot be before rent start date")
	}
	if input.RentEndDate != "" {
		rentEnd, err := parseDate(input.RentEndDate)
		if err != nil {
			return fmt.Errorf("invalid rent end date: %w", err)
		}
		if rentEnd.Before(rentStart) {
			return errors.New("rent end date cannot be before rent start date")
		}
		if billingStart.After(rentEnd) {
			return errors.New("billing start date must be within rent period")
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
		strings.Contains(text, "supported") ||
		strings.Contains(text, "too long") ||
		strings.Contains(text, "effective rent payments")
}

var errTenantLifecycleConflict = errors.New("tenant has effective rent payments in future obligations")

func (s *tenantService) createTenant(ctx context.Context, userID uint64, input tenantInput) (tenant, error) {
	if userID == 0 {
		return tenant{}, errors.New("userID is required")
	}
	if err := validateTenantInput(input); err != nil {
		return tenant{}, err
	}
	currency, _ := normalizeLedgerCurrency(input.Currency)
	billingStart, _ := parseDate(input.BillingStartDate)
	rentStart, _ := parseDate(input.RentStartDate)
	var rentEnd *time.Time
	if input.RentEndDate != "" {
		d, _ := parseDate(input.RentEndDate)
		rentEnd = &d
	}
	var row tenant
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		row = tenant{
			UserID:           userID,
			Name:             input.Name,
			DisplayAlias:     input.DisplayAlias,
			Email:            input.Email,
			PayerID:          nullableString(input.PayerID),
			PayerNameHint:    nullableString(input.PayerNameHint),
			MonthlyRentCents: moneyToCents(input.MonthlyRent),
			Currency:         currency,
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
		if err := tx.WithContext(ctx).Create(&row).Error; err != nil {
			return err
		}
		if strings.TrimSpace(input.PayerNameHint) != "" {
			payer := tenantPayer{
				UserID:              userID,
				TenantID:            row.ID,
				PayerID:             nullableString(input.PayerID),
				PayerNameOriginal:   strings.TrimSpace(input.PayerNameHint),
				PayerNameNormalized: normalizeTenantPayerName(input.PayerNameHint),
				Source:              "tenant_profile",
			}
			if err := tx.WithContext(ctx).Create(&payer).Error; err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
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
	currency, _ := normalizeLedgerCurrency(input.Currency)
	billingStart, _ := parseDate(input.BillingStartDate)
	rentStart, _ := parseDate(input.RentStartDate)
	var rentEnd *time.Time
	if input.RentEndDate != "" {
		d, _ := parseDate(input.RentEndDate)
		rentEnd = &d
	}
	var row tenant
	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.WithContext(ctx).Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND user_id = ?", tenantID, userID).First(&row).Error; err != nil {
			return err
		}
		if err := voidFutureTenantObligations(tx.WithContext(ctx), userID, tenantID, row.RentEndDate, rentEnd, userID); err != nil {
			return err
		}
		updates := map[string]any{
			"name":               input.Name,
			"display_alias":      input.DisplayAlias,
			"email":              input.Email,
			"payer_id":           nullableString(input.PayerID),
			"payer_name_hint":    nullableString(input.PayerNameHint),
			"monthly_rent_cents": moneyToCents(input.MonthlyRent),
			"currency":           currency,
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
		return tx.WithContext(ctx).Model(&row).Updates(updates).Error
	}); err != nil {
		return tenant{}, err
	}
	if err := s.db.WithContext(ctx).Where("id = ? AND user_id = ?", tenantID, userID).First(&row).Error; err != nil {
		return tenant{}, err
	}
	return row, nil
}

func obligationIsAfterRentEnd(periodMonth time.Time, rentEnd *time.Time) bool {
	return rentEnd != nil && monthStart(periodMonth).After(monthStart(*rentEnd))
}

func voidFutureTenantObligations(tx *gorm.DB, userID, tenantID uint64, previousEnd, newEnd *time.Time, voidedBy uint64) error {
	shortened := newEnd != nil && (previousEnd == nil || newEnd.Before(*previousEnd))
	if !shortened {
		return nil
	}
	cutoff := monthStart(*newEnd).AddDate(0, 1, 0)
	var obligations []rentObligation
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_id = ? AND tenant_id = ? AND period_month >= ? AND record_status = ?", userID, tenantID, cutoff, obligationRecordActive).Find(&obligations).Error; err != nil {
		return err
	}
	for _, obligation := range obligations {
		var count int64
		if err := tx.Model(&paymentAllocation{}).
			Where("user_id = ? AND rent_obligation_id = ? AND status = ?", userID, obligation.ID, allocationStatusConfirmed).
			Where("(allocation_kind = ? OR allocation_kind IS NULL OR allocation_kind = '')", allocationKindRent).
			Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return errTenantLifecycleConflict
		}
	}
	now := time.Now().UTC()
	return tx.Model(&rentObligation{}).
		Where("user_id = ? AND tenant_id = ? AND period_month >= ? AND record_status = ?", userID, tenantID, cutoff, obligationRecordActive).
		Updates(map[string]any{
			"status":            obligationRecordVoided,
			"record_status":     obligationRecordVoided,
			"voided_at":         now,
			"voided_by_user_id": voidedBy,
			"void_reason":       "tenant rent ended early",
		}).Error
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

func (s *tenantService) listTenantPayers(ctx context.Context, userID, tenantID uint64, includeRemoved bool) ([]tenantPayer, error) {
	if userID == 0 || tenantID == 0 {
		return nil, errors.New("userID and tenantID are required")
	}
	query := s.db.WithContext(ctx).Where("user_id = ? AND tenant_id = ?", userID, tenantID)
	if !includeRemoved {
		query = query.Where("removed_at IS NULL")
	}
	var rows []tenantPayer
	if err := query.Order("created_at ASC, id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (s *tenantService) addTenantPayer(ctx context.Context, userID, tenantID uint64, input tenantPayerInput) (tenantPayer, error) {
	if userID == 0 || tenantID == 0 {
		return tenantPayer{}, errors.New("userID and tenantID are required")
	}
	input.Name = strings.TrimSpace(input.Name)
	input.PayerID = strings.TrimSpace(input.PayerID)
	if err := validateTenantPayerInput(input); err != nil {
		return tenantPayer{}, err
	}
	var payerRow tenantPayer
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row tenant
		if err := tx.WithContext(ctx).Where("id = ? AND user_id = ?", tenantID, userID).First(&row).Error; err != nil {
			return err
		}
		name := normalizeTenantPayerName(input.Name)
		var existing []tenantPayer
		if err := tx.WithContext(ctx).Where("user_id = ? AND tenant_id = ? AND payer_name_normalized = ? AND removed_at IS NULL", userID, tenantID, name).Find(&existing).Error; err != nil {
			return err
		}
		for _, candidate := range existing {
			if stringValue(candidate.PayerID) == input.PayerID {
				payerRow = candidate
				return syncLegacyPayerFields(tx.WithContext(ctx), userID, tenantID)
			}
		}
		payerRow = tenantPayer{
			UserID:              userID,
			TenantID:            tenantID,
			PayerID:             nullableString(input.PayerID),
			PayerNameOriginal:   input.Name,
			PayerNameNormalized: name,
			Source:              "manual",
		}
		if err := tx.WithContext(ctx).Create(&payerRow).Error; err != nil {
			return err
		}
		return syncLegacyPayerFields(tx.WithContext(ctx), userID, tenantID)
	})
	if err != nil {
		return tenantPayer{}, err
	}
	return payerRow, nil
}

func (s *tenantService) removeTenantPayer(ctx context.Context, userID, tenantID, payerID, removedBy uint64, reason string) error {
	if userID == 0 || tenantID == 0 || payerID == 0 || removedBy == 0 {
		return errors.New("userID, tenantID, payerID, and removedBy are required")
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		now := time.Now().UTC()
		result := tx.Model(&tenantPayer{}).
			Where("id = ? AND user_id = ? AND tenant_id = ? AND removed_at IS NULL", payerID, userID, tenantID).
			Updates(map[string]any{"removed_at": now, "removed_by_user_id": removedBy})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		return syncLegacyPayerFields(tx, userID, tenantID)
	})
}

func syncLegacyPayerFields(tx *gorm.DB, userID, tenantID uint64) error {
	var rows []tenantPayer
	if err := tx.Where("user_id = ? AND tenant_id = ? AND removed_at IS NULL", userID, tenantID).Order("created_at ASC, id ASC").Find(&rows).Error; err != nil {
		return err
	}
	updates := map[string]any{"payer_id": nil, "payer_name_hint": nil}
	if len(rows) > 0 {
		updates["payer_id"] = rows[0].PayerID
		updates["payer_name_hint"] = rows[0].PayerNameOriginal
	}
	return tx.Model(&tenant{}).Where("id = ? AND user_id = ?", tenantID, userID).Updates(updates).Error
}

func tenantRecordFromModel(row tenant) tenantRecord {
	record := tenantRecord{
		ID:               strconv.FormatUint(row.ID, 10),
		Name:             row.Name,
		DisplayAlias:     row.DisplayAlias,
		Email:            row.Email,
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

func normalizeTenantPayerName(value string) string {
	return normalizeMatchText(value)
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
