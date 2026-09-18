package main

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	errPropertyNameRequired         = errors.New("property name is required")
	errRoomLabelRequired            = errors.New("room label is required")
	errAssetHasFutureArrangement    = errors.New("asset has current or future rent arrangements")
	errArrangementHistoryLocked     = errors.New("generated rent charges lock this arrangement")
	errTenantRoomConflict           = errors.New("tenant already has a room arrangement in the target month")
	errArrangementEffectiveRequired = errors.New("arrangement effective month is required")
)

type propertyInput struct {
	Name    string
	Address string
}

type roomInput struct {
	PropertyID uint64
	RoomLabel  string
	ActiveFrom time.Time
}

// rentArrangementInput is the write contract for a versioned room rent plan.
// When TenantIDs is provided without Responsibilities, the service evenly
// splits MonthlyRentCents (the remainder goes to the lowest tenant IDs). When
// Responsibilities is provided, the server validates the exact sum itself.
type rentArrangementInput struct {
	RoomID           uint64
	EffectiveMonth   time.Time
	StartDate        time.Time
	EndDate          *time.Time
	MonthlyRentCents int64
	Currency         string
	DueDay           int
	TenantIDs        []uint64
	Responsibilities []rentResponsibilityInput
}

type landlordDomainService struct {
	db *gorm.DB
}

func newLandlordDomainService(db *gorm.DB) *landlordDomainService {
	return &landlordDomainService{db: db}
}

func (s *landlordDomainService) createProperty(ctx context.Context, userID uint64, input propertyInput) (property, error) {
	if userID == 0 {
		return property{}, errLandlordRentUserRequired
	}
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" {
		return property{}, errPropertyNameRequired
	}
	if len([]rune(input.Name)) > 191 || len([]rune(input.Address)) > 1000 {
		return property{}, errors.New("property fields are too long")
	}
	row := property{UserID: userID, Name: input.Name, Address: nullableString(strings.TrimSpace(input.Address)), Status: "active"}
	if err := s.db.WithContext(ctx).Create(&row).Error; err != nil {
		return property{}, err
	}
	return row, nil
}

func (s *landlordDomainService) updateProperty(ctx context.Context, userID, propertyID uint64, input propertyInput) (property, error) {
	if userID == 0 || propertyID == 0 {
		return property{}, errors.New("userID and propertyID are required")
	}
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" {
		return property{}, errPropertyNameRequired
	}
	if len([]rune(input.Name)) > 191 || len([]rune(input.Address)) > 1000 {
		return property{}, errors.New("property fields are too long")
	}
	var row property
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND user_id = ?", propertyID, userID).First(&row).Error; err != nil {
			return err
		}
		return tx.Model(&row).Updates(map[string]any{"name": input.Name, "address": nullableString(strings.TrimSpace(input.Address))}).Error
	})
	return row, err
}

func (s *landlordDomainService) createRoom(ctx context.Context, userID uint64, input roomInput) (room, error) {
	if userID == 0 || input.PropertyID == 0 {
		return room{}, errors.New("userID and propertyID are required")
	}
	input.RoomLabel = strings.TrimSpace(input.RoomLabel)
	if input.RoomLabel == "" {
		return room{}, errRoomLabelRequired
	}
	if len([]rune(input.RoomLabel)) > 191 {
		return room{}, errors.New("room label is too long")
	}
	activeFrom := input.ActiveFrom
	if activeFrom.IsZero() {
		activeFrom = monthStart(time.Now().UTC())
	}
	activeFrom = monthStart(activeFrom)
	var row room
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var propertyRow property
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND user_id = ?", input.PropertyID, userID).First(&propertyRow).Error; err != nil {
			return err
		}
		if !propertyActiveInMonth(propertyRow, activeFrom) {
			return errors.New("property is not active for room start month")
		}
		row = room{UserID: userID, PropertyID: input.PropertyID, RoomLabel: input.RoomLabel, Status: "active", ActiveFrom: activeFrom}
		return tx.Create(&row).Error
	})
	return row, err
}

func (s *landlordDomainService) updateRoom(ctx context.Context, userID, roomID uint64, input roomInput) (room, error) {
	if userID == 0 || roomID == 0 || input.PropertyID == 0 {
		return room{}, errors.New("userID, roomID, and propertyID are required")
	}
	input.RoomLabel = strings.TrimSpace(input.RoomLabel)
	if input.RoomLabel == "" {
		return room{}, errRoomLabelRequired
	}
	var row room
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND user_id = ?", roomID, userID).First(&row).Error; err != nil {
			return err
		}
		if row.PropertyID != input.PropertyID {
			var count int64
			if err := tx.Model(&tenancyAgreement{}).Where("user_id = ? AND room_id = ? AND status = ? AND (end_date IS NULL OR end_date >= ?)", userID, roomID, "active", monthStart(time.Now().UTC())).Count(&count).Error; err != nil {
				return err
			}
			if count > 0 {
				return errAssetHasFutureArrangement
			}
			var target property
			if err := tx.Where("id = ? AND user_id = ?", input.PropertyID, userID).First(&target).Error; err != nil {
				return err
			}
			if !propertyActiveInMonth(target, monthStart(time.Now().UTC())) {
				return errors.New("property is not active")
			}
		}
		return tx.Model(&row).Updates(map[string]any{"property_id": input.PropertyID, "room_label": input.RoomLabel}).Error
	})
	return row, err
}

func (s *landlordDomainService) deactivateProperty(ctx context.Context, userID, propertyID uint64, effectiveMonth time.Time) error {
	if userID == 0 || propertyID == 0 {
		return errors.New("userID and propertyID are required")
	}
	if effectiveMonth.IsZero() {
		effectiveMonth = monthStart(time.Now().UTC())
	}
	effectiveMonth = monthStart(effectiveMonth)
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row property
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND user_id = ?", propertyID, userID).First(&row).Error; err != nil {
			return err
		}
		var count int64
		if err := tx.Model(&tenancyAgreement{}).Where("user_id = ? AND room_id IN (SELECT id FROM rooms WHERE user_id = ? AND property_id = ?) AND status = ? AND (end_date IS NULL OR end_date >= ?)", userID, userID, propertyID, "active", effectiveMonth).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return errAssetHasFutureArrangement
		}
		if effectiveMonth.After(monthStart(time.Now().UTC())) {
			return tx.Model(&row).Updates(map[string]any{"inactive_from": effectiveMonth}).Error
		}
		return tx.Model(&row).Updates(map[string]any{"status": "inactive", "inactive_from": effectiveMonth}).Error
	})
}

func (s *landlordDomainService) deactivateRoom(ctx context.Context, userID, roomID uint64, effectiveMonth time.Time) error {
	if userID == 0 || roomID == 0 {
		return errors.New("userID and roomID are required")
	}
	if effectiveMonth.IsZero() {
		effectiveMonth = monthStart(time.Now().UTC())
	}
	effectiveMonth = monthStart(effectiveMonth)
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row room
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND user_id = ?", roomID, userID).First(&row).Error; err != nil {
			return err
		}
		var count int64
		if err := tx.Model(&tenancyAgreement{}).Where("user_id = ? AND room_id = ? AND status = ? AND (end_date IS NULL OR end_date >= ?)", userID, roomID, "active", effectiveMonth).Count(&count).Error; err != nil {
			return err
		}
		if count > 0 {
			return errAssetHasFutureArrangement
		}
		updates := map[string]any{"inactive_from": effectiveMonth}
		if !effectiveMonth.After(monthStart(time.Now().UTC())) {
			updates["status"] = "inactive"
		}
		return tx.Model(&row).Updates(updates).Error
	})
}

func (s *landlordDomainService) saveRentArrangement(ctx context.Context, userID uint64, input rentArrangementInput) (tenancyAgreement, error) {
	if userID == 0 {
		return tenancyAgreement{}, errLandlordRentUserRequired
	}
	if input.RoomID == 0 {
		return tenancyAgreement{}, errors.New("roomID is required")
	}
	if input.EffectiveMonth.IsZero() {
		return tenancyAgreement{}, errArrangementEffectiveRequired
	}
	var row tenancyAgreement
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var err error
		row, err = saveRentArrangementInTx(tx, userID, input)
		return err
	})
	return row, err
}

func saveRentArrangementInTx(tx *gorm.DB, userID uint64, input rentArrangementInput) (tenancyAgreement, error) {
	effectiveMonth := monthStart(input.EffectiveMonth)
	startDate := input.StartDate
	if startDate.IsZero() {
		startDate = effectiveMonth
	}
	startDate = startDate.UTC()
	if monthStart(startDate).Before(effectiveMonth) || monthStart(startDate).After(effectiveMonth) {
		return tenancyAgreement{}, errors.New("arrangement start date must be in effective month")
	}
	if input.EndDate != nil && input.EndDate.Before(startDate) {
		return tenancyAgreement{}, errors.New("arrangement end date cannot precede start date")
	}
	var roomRow room
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND user_id = ?", input.RoomID, userID).First(&roomRow).Error; err != nil {
		return tenancyAgreement{}, err
	}
	var propertyRow property
	if err := tx.Where("id = ? AND user_id = ?", roomRow.PropertyID, userID).First(&propertyRow).Error; err != nil {
		return tenancyAgreement{}, err
	}
	if !propertyActiveInMonth(propertyRow, effectiveMonth) || !roomActiveInMonth(roomRow, effectiveMonth) {
		return tenancyAgreement{}, errors.New("room or property is not active for effective month")
	}
	var charges int64
	if err := tx.Model(&rentCharge{}).Where("user_id = ? AND room_id = ? AND period_month >= ?", userID, input.RoomID, effectiveMonth).Count(&charges).Error; err != nil {
		return tenancyAgreement{}, err
	}
	if charges > 0 {
		return tenancyAgreement{}, errArrangementHistoryLocked
	}
	var existing []tenancyAgreement
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_id = ? AND room_id = ? AND status = ? AND start_date <= ? AND (end_date IS NULL OR end_date >= ?)", userID, input.RoomID, "active", effectiveMonth.AddDate(0, 1, -1), effectiveMonth).Order("start_date DESC, id DESC").Find(&existing).Error; err != nil {
		return tenancyAgreement{}, err
	}
	var inheritedRent int64
	var inheritedCurrency string
	var inheritedDueDay int
	for _, agreement := range existing {
		if inheritedRent == 0 {
			inheritedRent, inheritedCurrency, inheritedDueDay = agreement.MonthlyRentCents, agreement.Currency, agreement.DueDay
		}
		endDate := effectiveMonth.AddDate(0, 0, -1)
		status := agreement.Status
		if !agreement.StartDate.Before(effectiveMonth) {
			status = "superseded"
		}
		if err := tx.Model(&agreement).Updates(map[string]any{"end_date": endDate, "status": status}).Error; err != nil {
			return tenancyAgreement{}, err
		}
		if err := tx.Model(&agreementParty{}).Where("user_id = ? AND agreement_id = ? AND status = ?", userID, agreement.ID, "active").Updates(map[string]any{"left_at": endDate, "status": status}).Error; err != nil {
			return tenancyAgreement{}, err
		}
	}
	currency := firstNonEmpty(strings.ToUpper(strings.TrimSpace(input.Currency)), inheritedCurrency, ledgerCurrencyEUR)
	if _, err := normalizeLedgerCurrency(currency); err != nil {
		return tenancyAgreement{}, err
	}
	monthlyRent := input.MonthlyRentCents
	if monthlyRent == 0 {
		monthlyRent = inheritedRent
	}
	if monthlyRent <= 0 {
		return tenancyAgreement{}, errors.New("monthly rent must be positive")
	}
	dueDay := input.DueDay
	if dueDay == 0 {
		dueDay = inheritedDueDay
	}
	if dueDay < 1 || dueDay > 31 {
		return tenancyAgreement{}, errors.New("due day must be between 1 and 31")
	}
	responsibilities := append([]rentResponsibilityInput(nil), input.Responsibilities...)
	if len(responsibilities) == 0 && len(input.TenantIDs) > 0 {
		ids := append([]uint64(nil), input.TenantIDs...)
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
		var err error
		responsibilities, err = splitRentAmountEvenly(monthlyRent, ids)
		if err != nil {
			return tenancyAgreement{}, err
		}
	}
	if err := validateRentResponsibilityPlanAllowEmpty(monthlyRent, responsibilities); err != nil {
		return tenancyAgreement{}, err
	}
	tenantIDs := make([]uint64, 0, len(responsibilities))
	for _, responsibility := range responsibilities {
		tenantIDs = append(tenantIDs, responsibility.TenantID)
	}
	if len(tenantIDs) > 0 {
		var tenants []tenant
		if err := tx.Where("user_id = ? AND id IN ?", userID, tenantIDs).Find(&tenants).Error; err != nil {
			return tenancyAgreement{}, err
		}
		if len(tenants) != len(tenantIDs) {
			return tenancyAgreement{}, gorm.ErrRecordNotFound
		}
		var conflictCount int64
		if err := tx.Table("agreement_parties AS ap").Joins("JOIN tenancy_agreements AS ta ON ta.id = ap.agreement_id AND ta.user_id = ap.user_id").Where("ap.user_id = ? AND ap.tenant_id IN ? AND ap.status = ? AND ta.status = ? AND ta.room_id <> ? AND ta.start_date <= ? AND (ta.end_date IS NULL OR ta.end_date >= ?) AND (ap.joined_at IS NULL OR ap.joined_at <= ?) AND (ap.left_at IS NULL OR ap.left_at >= ?)", userID, tenantIDs, "active", "active", input.RoomID, effectiveMonth.AddDate(0, 1, -1), effectiveMonth, effectiveMonth.AddDate(0, 1, -1), effectiveMonth).Count(&conflictCount).Error; err != nil {
			return tenancyAgreement{}, err
		}
		if conflictCount > 0 {
			return tenancyAgreement{}, errTenantRoomConflict
		}
	}
	agreement := tenancyAgreement{UserID: userID, RoomID: input.RoomID, StartDate: startDate, EndDate: input.EndDate, MonthlyRentCents: monthlyRent, Currency: currency, DueDay: dueDay, Status: "active"}
	if err := tx.Create(&agreement).Error; err != nil {
		return tenancyAgreement{}, err
	}
	for _, responsibility := range responsibilities {
		joined := effectiveMonth
		party := agreementParty{UserID: userID, AgreementID: agreement.ID, TenantID: responsibility.TenantID, ResponsibilityCents: responsibility.AmountCents, JoinedAt: &joined, LeftAt: input.EndDate, Status: "active"}
		if err := tx.Create(&party).Error; err != nil {
			return tenancyAgreement{}, err
		}
	}
	return agreement, nil
}

func validateRentResponsibilityPlanAllowEmpty(totalCents int64, responsibilities []rentResponsibilityInput) error {
	if len(responsibilities) == 0 {
		if totalCents <= 0 {
			return errors.New("rent amount must be positive")
		}
		return nil
	}
	return validateRentResponsibilityPlan(totalCents, responsibilities)
}

func (s *landlordDomainService) bindTenantToRoom(ctx context.Context, userID, tenantID, roomID uint64, effectiveMonth time.Time, monthlyRentCents int64) (tenancyAgreement, error) {
	if tenantID == 0 {
		return tenancyAgreement{}, errors.New("tenantID is required")
	}
	return s.saveRentArrangement(ctx, userID, rentArrangementInput{RoomID: roomID, EffectiveMonth: effectiveMonth, MonthlyRentCents: monthlyRentCents, TenantIDs: []uint64{tenantID}})
}

// moveTenant closes the tenant's old whole-month arrangement and creates the
// destination arrangement in one transaction. Historical charges are never
// rewritten; saveRentArrangementInTx rejects a move once a charge exists from
// the requested effective month onward.
func (s *landlordDomainService) moveTenant(ctx context.Context, userID, tenantID, roomID uint64, effectiveMonth time.Time, monthlyRentCents int64) (tenancyAgreement, error) {
	if userID == 0 || tenantID == 0 || roomID == 0 {
		return tenancyAgreement{}, errors.New("userID, tenantID, and roomID are required")
	}
	effectiveMonth = monthStart(effectiveMonth)
	var destination tenancyAgreement
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var tenantRow tenant
		if err := tx.Where("id = ? AND user_id = ?", tenantID, userID).First(&tenantRow).Error; err != nil {
			return err
		}
		var oldAgreements []tenancyAgreement
		if err := tx.Table("tenancy_agreements AS ta").Joins("JOIN agreement_parties AS ap ON ap.agreement_id = ta.id AND ap.user_id = ta.user_id").Where("ta.user_id = ? AND ap.tenant_id = ? AND ta.status = ? AND ap.status = ? AND ta.start_date <= ? AND (ta.end_date IS NULL OR ta.end_date >= ?)", userID, tenantID, "active", "active", effectiveMonth.AddDate(0, 1, -1), effectiveMonth).Select("ta.*").Find(&oldAgreements).Error; err != nil {
			return err
		}
		for _, agreement := range oldAgreements {
			var parties []agreementParty
			if err := tx.Where("user_id = ? AND agreement_id = ? AND status = ? AND tenant_id <> ?", userID, agreement.ID, "active", tenantID).Find(&parties).Error; err != nil {
				return err
			}
			ids := make([]uint64, 0, len(parties))
			for _, party := range parties {
				ids = append(ids, party.TenantID)
			}
			if _, err := saveRentArrangementInTx(tx, userID, rentArrangementInput{RoomID: agreement.RoomID, EffectiveMonth: effectiveMonth, MonthlyRentCents: agreement.MonthlyRentCents, Currency: agreement.Currency, DueDay: agreement.DueDay, TenantIDs: ids}); err != nil {
				return err
			}
		}
		tenantIDs := []uint64{tenantID}
		var current tenancyAgreement
		lookupErr := tx.Where("user_id = ? AND room_id = ? AND status = ? AND start_date <= ? AND (end_date IS NULL OR end_date >= ?)", userID, roomID, "active", effectiveMonth.AddDate(0, 1, -1), effectiveMonth).Order("start_date DESC, id DESC").First(&current).Error
		if lookupErr == nil {
			var parties []agreementParty
			if err := tx.Where("user_id = ? AND agreement_id = ? AND status = ?", userID, current.ID, "active").Find(&parties).Error; err != nil {
				return err
			}
			for _, party := range parties {
				if party.TenantID != tenantID {
					tenantIDs = append(tenantIDs, party.TenantID)
				}
			}
		} else if !errors.Is(lookupErr, gorm.ErrRecordNotFound) {
			return lookupErr
		}
		var err error
		destination, err = saveRentArrangementInTx(tx, userID, rentArrangementInput{RoomID: roomID, EffectiveMonth: effectiveMonth, MonthlyRentCents: monthlyRentCents, TenantIDs: tenantIDs})
		return err
	})
	return destination, err
}

func (s *landlordDomainService) endTenantArrangement(ctx context.Context, userID, tenantID uint64, effectiveMonth time.Time) error {
	if userID == 0 || tenantID == 0 {
		return errors.New("userID and tenantID are required")
	}
	effectiveMonth = monthStart(effectiveMonth)
	var agreements []tenancyAgreement
	if err := s.db.WithContext(ctx).Table("tenancy_agreements AS ta").Joins("JOIN agreement_parties AS ap ON ap.agreement_id = ta.id AND ap.user_id = ta.user_id").Where("ta.user_id = ? AND ap.tenant_id = ? AND ta.status = ? AND ap.status = ? AND ta.start_date <= ? AND (ta.end_date IS NULL OR ta.end_date >= ?)", userID, tenantID, "active", "active", effectiveMonth.AddDate(0, 1, -1), effectiveMonth).Select("ta.*").Find(&agreements).Error; err != nil {
		return err
	}
	for _, agreement := range agreements {
		var parties []agreementParty
		if err := s.db.WithContext(ctx).Where("user_id = ? AND agreement_id = ? AND status = ? AND tenant_id <> ?", userID, agreement.ID, "active", tenantID).Find(&parties).Error; err != nil {
			return err
		}
		ids := make([]uint64, 0, len(parties))
		for _, party := range parties {
			ids = append(ids, party.TenantID)
		}
		if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			_, err := saveRentArrangementInTx(tx, userID, rentArrangementInput{RoomID: agreement.RoomID, EffectiveMonth: effectiveMonth, MonthlyRentCents: agreement.MonthlyRentCents, Currency: agreement.Currency, DueDay: agreement.DueDay, TenantIDs: ids})
			return err
		}); err != nil {
			return err
		}
	}
	return nil
}

func (s *landlordDomainService) property(ctx context.Context, userID, propertyID uint64) (property, error) {
	return newLandlordRentRepository(s.db).findProperty(ctx, userID, propertyID)
}

func formatArrangementConflict(err error) error {
	if errors.Is(err, errTenantRoomConflict) {
		return fmt.Errorf("%w: tenant already has another room", err)
	}
	return err
}
