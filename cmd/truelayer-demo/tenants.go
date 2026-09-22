package main

import (
	"context"
	"errors"
	"math"
	"net/mail"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type tenant struct {
	ID           uint64 `gorm:"primaryKey"`
	UserID       uint64
	Name         string
	DisplayAlias string
	Email        string
	Status       string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type tenantService struct{ db *gorm.DB }

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

type tenantPayerInput struct{ PayerID, Name string }

type tenantPayerRecord struct {
	ID, PayerID, Name string
	Shared            bool
	Source            string
	LastMatchedAt     string
	RemovedAt         string
}

// Tenant edits contain person/contact data only. Rent and room configuration
// is written through room rent plans on the room detail page.
type tenantInput struct {
	Name, DisplayAlias, Email string
	PayerID, PayerNameHint    string
	Status                    string
}

func newTenantService(db *gorm.DB) *tenantService { return &tenantService{db: db} }

func validateTenantPayerInput(input tenantPayerInput) error {
	if strings.TrimSpace(input.Name) == "" {
		return errors.New("payer name is required")
	}
	if len([]rune(strings.TrimSpace(input.Name))) > 191 || len([]rune(strings.TrimSpace(input.PayerID))) > 191 {
		return errors.New("payer name or id is too long")
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
		if row.PayerNameNormalized != "" {
			if activeNameTenants[row.PayerNameNormalized] == nil {
				activeNameTenants[row.PayerNameNormalized] = make(map[uint64]struct{})
			}
			activeNameTenants[row.PayerNameNormalized][row.TenantID] = struct{}{}
		}
		payerID := stringValue(row.PayerID)
		if payerID != "" {
			if activeIDTenants[payerID] == nil {
				activeIDTenants[payerID] = make(map[uint64]struct{})
			}
			activeIDTenants[payerID][row.TenantID] = struct{}{}
		}
	}
	result := make([]tenantPayerRecord, 0, len(rows))
	for _, row := range rows {
		record := tenantPayerRecord{
			ID: strconv.FormatUint(row.ID, 10), PayerID: stringValue(row.PayerID), Name: row.PayerNameOriginal,
			Source: row.Source,
			Shared: len(activeNameTenants[row.PayerNameNormalized]) > 1 || len(activeIDTenants[stringValue(row.PayerID)]) > 1,
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

type formValues interface{ Get(string) string }

func tenantInputFromForm(values formValues) (tenantInput, error) {
	input := tenantInput{
		Name: strings.TrimSpace(values.Get("name")), DisplayAlias: strings.TrimSpace(values.Get("display_alias")),
		Email: strings.TrimSpace(values.Get("email")), PayerID: strings.TrimSpace(values.Get("payer_id")),
		PayerNameHint: strings.TrimSpace(values.Get("payer_name_hint")),
		Status:        firstNonEmpty(strings.ToLower(strings.TrimSpace(values.Get("status"))), "active"),
	}
	if err := validateTenantInput(input); err != nil {
		return tenantInput{}, err
	}
	return input, nil
}

func validateTenantInput(input tenantInput) error {
	if strings.TrimSpace(input.Name) == "" {
		return errors.New("tenant name is required")
	}
	if len([]rune(input.Name)) > 191 || len([]rune(input.DisplayAlias)) > 191 {
		return errors.New("tenant name is too long")
	}
	if input.Email != "" {
		parsed, err := mail.ParseAddress(input.Email)
		if err != nil || parsed.Address != input.Email {
			return errors.New("email must be a valid email address")
		}
	}
	if input.Status != "active" && input.Status != "inactive" {
		return errors.New("status must be active or inactive")
	}
	if input.PayerNameHint != "" {
		return validateTenantPayerInput(tenantPayerInput{Name: input.PayerNameHint, PayerID: input.PayerID})
	}
	if input.PayerID != "" {
		return errors.New("payer name is required with payer id")
	}
	return nil
}

func isValidationError(err error) bool {
	if err == nil {
		return false
	}
	text := err.Error()
	return strings.Contains(text, "required") || strings.Contains(text, "must be") || strings.Contains(text, "invalid") || strings.Contains(text, "too long")
}

func (s *tenantService) createTenant(ctx context.Context, userID uint64, input tenantInput) (tenant, error) {
	if userID == 0 {
		return tenant{}, errors.New("userID is required")
	}
	if err := validateTenantInput(input); err != nil {
		return tenant{}, err
	}
	row := tenant{UserID: userID, Name: input.Name, DisplayAlias: input.DisplayAlias, Email: input.Email, Status: input.Status}
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&row).Error; err != nil {
			return err
		}
		if input.PayerNameHint != "" {
			return addTenantPayerInTx(tx, userID, row.ID, tenantPayerInput{PayerID: input.PayerID, Name: input.PayerNameHint}, "tenant_profile")
		}
		return nil
	})
	return row, err
}

func (s *tenantService) updateTenant(ctx context.Context, userID, tenantID uint64, input tenantInput) (tenant, error) {
	if userID == 0 || tenantID == 0 {
		return tenant{}, errors.New("userID and tenantID are required")
	}
	if err := validateTenantInput(input); err != nil {
		return tenant{}, err
	}
	var row tenant
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND user_id = ?", tenantID, userID).First(&row).Error; err != nil {
			return err
		}
		if err := tx.Model(&row).Updates(map[string]any{"name": input.Name, "display_alias": input.DisplayAlias, "email": input.Email, "status": input.Status}).Error; err != nil {
			return err
		}
		if input.PayerNameHint != "" {
			return addTenantPayerInTx(tx, userID, tenantID, tenantPayerInput{PayerID: input.PayerID, Name: input.PayerNameHint}, "tenant_profile")
		}
		return nil
	})
	if err != nil {
		return tenant{}, err
	}
	if err := s.db.WithContext(ctx).Where("id = ? AND user_id = ?", tenantID, userID).First(&row).Error; err != nil {
		return tenant{}, err
	}
	return row, nil
}

func (s *tenantService) listTenants(ctx context.Context, userID uint64) ([]tenantRecord, error) {
	var rows []tenant
	if err := s.db.WithContext(ctx).Where("user_id = ?", userID).Order("name ASC, id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	var payers []tenantPayer
	if err := s.db.WithContext(ctx).Where("user_id = ? AND removed_at IS NULL", userID).Order("created_at ASC, id ASC").Find(&payers).Error; err != nil {
		return nil, err
	}
	firstPayer := make(map[uint64]tenantPayer)
	for _, payer := range payers {
		if _, ok := firstPayer[payer.TenantID]; !ok {
			firstPayer[payer.TenantID] = payer
		}
	}
	result := make([]tenantRecord, 0, len(rows))
	for _, row := range rows {
		record := tenantRecordFromModel(row)
		if payer, ok := firstPayer[row.ID]; ok {
			record.PayerID = stringValue(payer.PayerID)
			record.PayerNameHint = payer.PayerNameOriginal
		}
		result = append(result, record)
	}
	return result, nil
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
	err := query.Order("created_at ASC, id ASC").Find(&rows).Error
	return rows, err
}

func (s *tenantService) addTenantPayer(ctx context.Context, userID, tenantID uint64, input tenantPayerInput) (tenantPayer, error) {
	if userID == 0 || tenantID == 0 {
		return tenantPayer{}, errors.New("userID and tenantID are required")
	}
	input.Name, input.PayerID = strings.TrimSpace(input.Name), strings.TrimSpace(input.PayerID)
	if err := validateTenantPayerInput(input); err != nil {
		return tenantPayer{}, err
	}
	var result tenantPayer
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("id = ? AND user_id = ?", tenantID, userID).First(&tenant{}).Error; err != nil {
			return err
		}
		result = tenantPayer{UserID: userID, TenantID: tenantID, PayerID: nullableString(input.PayerID), PayerNameOriginal: input.Name, PayerNameNormalized: normalizeTenantPayerName(input.Name), Source: "manual"}
		return addTenantPayerInTx(tx, userID, tenantID, input, "manual")
	})
	if err != nil {
		return tenantPayer{}, err
	}
	_ = s.db.WithContext(ctx).Where("user_id = ? AND tenant_id = ? AND payer_name_normalized = ? AND removed_at IS NULL", userID, tenantID, result.PayerNameNormalized).Order("id DESC").First(&result).Error
	return result, nil
}

func addTenantPayerInTx(tx *gorm.DB, userID, tenantID uint64, input tenantPayerInput, source string) error {
	name := normalizeTenantPayerName(input.Name)
	var existing tenantPayer
	err := tx.Where("user_id = ? AND tenant_id = ? AND payer_name_normalized = ? AND removed_at IS NULL", userID, tenantID, name).Order("id DESC").First(&existing).Error
	if err == nil {
		if stringValue(existing.PayerID) == input.PayerID {
			return nil
		}
		return tx.Model(&existing).Update("payer_id", nullableString(input.PayerID)).Error
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	return tx.Create(&tenantPayer{UserID: userID, TenantID: tenantID, PayerID: nullableString(input.PayerID), PayerNameOriginal: strings.TrimSpace(input.Name), PayerNameNormalized: name, Source: source}).Error
}

func (s *tenantService) rememberTenantPayer(ctx context.Context, userID, tenantID uint64, payerID, payerName string) error {
	payerName, payerID = strings.TrimSpace(payerName), strings.TrimSpace(payerID)
	if payerName == "" {
		return nil
	}
	if err := validateTenantPayerInput(tenantPayerInput{Name: payerName, PayerID: payerID}); err != nil {
		return err
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row tenant
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND user_id = ?", tenantID, userID).First(&row).Error; err != nil {
			return err
		}
		return addTenantPayerInTx(tx, userID, tenantID, tenantPayerInput{Name: payerName, PayerID: payerID}, "matched_transaction")
	})
}

func (s *tenantService) removeTenantPayer(ctx context.Context, userID, tenantID, payerID, removedBy uint64, reason string) error {
	if userID == 0 || tenantID == 0 || payerID == 0 || removedBy == 0 {
		return errors.New("userID, tenantID, payerID, and removedBy are required")
	}
	result := s.db.WithContext(ctx).Model(&tenantPayer{}).Where("id = ? AND user_id = ? AND tenant_id = ? AND removed_at IS NULL", payerID, userID, tenantID).Updates(map[string]any{"removed_at": time.Now().UTC(), "removed_by_user_id": removedBy})
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func tenantRecordFromModel(row tenant) tenantRecord {
	return tenantRecord{ID: strconv.FormatUint(row.ID, 10), Name: row.Name, DisplayAlias: row.DisplayAlias, Email: row.Email, Status: row.Status, CreatedAt: row.CreatedAt.Format(time.RFC3339)}
}

func normalizeTenantPayerName(value string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(value))), " ")
}

func parseDate(value string) (time.Time, error) {
	return time.Parse(dateLayout, strings.TrimSpace(value))
}

func moneyToCents(amount float64) int64 { return int64(math.Round(amount * 100)) }
func centsToMoney(cents int64) float64  { return float64(cents) / 100 }

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
