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
	Name       string
	CityRegion string
	Address    string
	Timezone   string
	Notes      string
}

type roomInput struct {
	PropertyID       uint64
	RoomLabel        string
	RoomType         string
	Capacity         int
	MonthlyRentCents int64
	DueDay           int
	Notes            string
	ActiveFrom       time.Time
	EffectiveMonth   time.Time
}

// rentArrangementInput is the write contract for a versioned room rent plan.
// When TenantIDs is provided without Responsibilities, the service evenly
// splits MonthlyRentCents (the remainder goes to the lowest tenant IDs). When
// Responsibilities is provided, the server validates the exact sum itself.
type rentArrangementInput struct {
	RoomID           uint64
	EffectiveMonth   time.Time
	ContractDate     *time.Time
	MoveInDate       *time.Time
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
	if len([]rune(input.Name)) > 191 || len([]rune(input.CityRegion)) > 191 || len([]rune(input.Address)) > 1000 || len([]rune(input.Timezone)) > 64 || len([]rune(input.Notes)) > 2000 {
		return property{}, errors.New("property fields are too long")
	}
	timezone := firstNonEmpty(strings.TrimSpace(input.Timezone), "Europe/Dublin")
	if _, err := time.LoadLocation(timezone); err != nil {
		return property{}, errors.New("property timezone is invalid")
	}
	row := property{UserID: userID, Name: input.Name, CityRegion: strings.TrimSpace(input.CityRegion), Address: nullableString(strings.TrimSpace(input.Address)), Timezone: timezone, Notes: nullableString(strings.TrimSpace(input.Notes)), Status: "active"}
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
	if len([]rune(input.Name)) > 191 || len([]rune(input.CityRegion)) > 191 || len([]rune(input.Address)) > 1000 || len([]rune(input.Timezone)) > 64 || len([]rune(input.Notes)) > 2000 {
		return property{}, errors.New("property fields are too long")
	}
	input.Timezone = firstNonEmpty(strings.TrimSpace(input.Timezone), "Europe/Dublin")
	if _, err := time.LoadLocation(input.Timezone); err != nil {
		return property{}, errors.New("property timezone is invalid")
	}
	var row property
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND user_id = ?", propertyID, userID).First(&row).Error; err != nil {
			return err
		}
		return tx.Model(&row).Updates(map[string]any{"name": input.Name, "city_region": strings.TrimSpace(input.CityRegion), "address": nullableString(strings.TrimSpace(input.Address)), "timezone": input.Timezone, "notes": nullableString(strings.TrimSpace(input.Notes))}).Error
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
	input.RoomType = strings.TrimSpace(input.RoomType)
	if len([]rune(input.RoomType)) > 64 || len([]rune(input.Notes)) > 2000 || input.Capacity < 0 || input.Capacity > 100 || input.MonthlyRentCents < 0 {
		return room{}, errors.New("room details are invalid")
	}
	input.RoomType = firstNonEmpty(input.RoomType, "其他")
	if input.Capacity == 0 {
		input.Capacity = 1
	}
	if input.DueDay == 0 {
		input.DueDay = 1
	}
	if input.DueDay < 1 || input.DueDay > 31 {
		return room{}, errors.New("due day must be between 1 and 31")
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
		row = room{UserID: userID, PropertyID: input.PropertyID, RoomLabel: input.RoomLabel, RoomType: strings.TrimSpace(input.RoomType), Capacity: input.Capacity, MonthlyRentCents: input.MonthlyRentCents, DueDay: input.DueDay, Notes: nullableString(strings.TrimSpace(input.Notes)), Status: "active", ActiveFrom: activeFrom}
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
	input.RoomType = strings.TrimSpace(input.RoomType)
	if len([]rune(input.RoomType)) > 64 || len([]rune(input.Notes)) > 2000 || input.Capacity < 0 || input.Capacity > 100 || input.MonthlyRentCents < 0 {
		return room{}, errors.New("room details are invalid")
	}
	if input.DueDay < 0 || input.DueDay > 31 {
		return room{}, errors.New("due day must be between 1 and 31")
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
		updates := map[string]any{"property_id": input.PropertyID, "room_label": input.RoomLabel, "notes": nullableString(strings.TrimSpace(input.Notes))}
		if input.RoomType != "" {
			updates["room_type"] = input.RoomType
		}
		if input.Capacity > 0 {
			updates["capacity"] = input.Capacity
		}
		if err := tx.Model(&row).Updates(updates).Error; err != nil {
			return err
		}
		if !input.EffectiveMonth.IsZero() {
			return updateRoomRentArrangementInTx(tx, userID, roomID, input)
		}
		if input.MonthlyRentCents > 0 || input.DueDay > 0 {
			defaults := map[string]any{}
			if input.MonthlyRentCents > 0 {
				defaults["monthly_rent_cents"] = input.MonthlyRentCents
			}
			if input.DueDay > 0 {
				defaults["due_day"] = input.DueDay
			}
			if len(defaults) > 0 {
				return tx.Model(&room{}).Where("id = ? AND user_id = ?", roomID, userID).Updates(defaults).Error
			}
		}
		return nil
	})
	return row, err
}

func updateRoomRentArrangementInTx(tx *gorm.DB, userID, roomID uint64, input roomInput) error {
	effectiveMonth := monthStart(input.EffectiveMonth)
	var existing []tenancyAgreement
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_id = ? AND room_id = ? AND status = ? AND start_date <= ? AND (end_date IS NULL OR end_date >= ?)", userID, roomID, "active", effectiveMonth.AddDate(0, 1, -1), effectiveMonth).Order("start_date DESC, id DESC").Find(&existing).Error; err != nil {
		return err
	}
	if len(existing) == 0 {
		if input.DueDay > 0 && (input.DueDay < 1 || input.DueDay > 31) {
			return errors.New("due day must be between 1 and 31")
		}
		defaults := map[string]any{}
		if input.MonthlyRentCents > 0 {
			defaults["monthly_rent_cents"] = input.MonthlyRentCents
		}
		if input.DueDay > 0 {
			defaults["due_day"] = input.DueDay
		}
		if len(defaults) == 0 {
			return nil
		}
		return tx.Model(&room{}).Where("id = ? AND user_id = ?", roomID, userID).Updates(defaults).Error
	}

	current := existing[0]
	newRent := input.MonthlyRentCents
	if newRent == 0 {
		newRent = current.MonthlyRentCents
	}
	newDueDay := input.DueDay
	if newDueDay == 0 {
		newDueDay = current.DueDay
	}
	if newRent <= 0 || newDueDay < 1 || newDueDay > 31 {
		return errors.New("room rent and due day are invalid")
	}
	if newRent == current.MonthlyRentCents && newDueDay == current.DueDay {
		return tx.Model(&room{}).Where("id = ? AND user_id = ?", roomID, userID).Updates(map[string]any{"monthly_rent_cents": current.MonthlyRentCents, "due_day": current.DueDay}).Error
	}

	responsibilitiesByTenant := make(map[uint64]int64)
	const maxInt64 = int64(^uint64(0) >> 1)
	for _, agreement := range existing {
		var parties []agreementParty
		if err := tx.Where("user_id = ? AND agreement_id = ? AND status = ?", userID, agreement.ID, "active").Find(&parties).Error; err != nil {
			return err
		}
		for _, party := range parties {
			if !agreementPartyActiveInMonth(party, effectiveMonth) {
				continue
			}
			if party.TenantID == 0 || party.ResponsibilityCents <= 0 || responsibilitiesByTenant[party.TenantID] > maxInt64-party.ResponsibilityCents {
				return errors.New("current rent responsibilities are invalid")
			}
			responsibilitiesByTenant[party.TenantID] += party.ResponsibilityCents
		}
	}
	currentPlan := make([]rentResponsibilityInput, 0, len(responsibilitiesByTenant))
	for tenantID, cents := range responsibilitiesByTenant {
		currentPlan = append(currentPlan, rentResponsibilityInput{TenantID: tenantID, AmountCents: cents})
	}
	sort.Slice(currentPlan, func(i, j int) bool { return currentPlan[i].TenantID < currentPlan[j].TenantID })
	responsibilities := currentPlan
	if newRent != current.MonthlyRentCents && len(currentPlan) > 0 {
		var err error
		responsibilities, err = scaleRentResponsibilities(newRent, currentPlan)
		if err != nil {
			return err
		}
	}
	_, err := saveRentArrangementInTx(tx, userID, rentArrangementInput{
		RoomID: roomID, EffectiveMonth: effectiveMonth, MonthlyRentCents: newRent, DueDay: newDueDay,
		Responsibilities: responsibilities,
	})
	return err
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
	var inheritedContractDate, inheritedMoveInDate *time.Time
	for _, agreement := range existing {
		if inheritedRent == 0 {
			inheritedRent, inheritedCurrency, inheritedDueDay = agreement.MonthlyRentCents, agreement.Currency, agreement.DueDay
			inheritedContractDate, inheritedMoveInDate = agreement.ContractDate, agreement.MoveInDate
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
		lockedTenantIDs := append([]uint64(nil), tenantIDs...)
		sort.Slice(lockedTenantIDs, func(i, j int) bool { return lockedTenantIDs[i] < lockedTenantIDs[j] })
		for _, tenantID := range lockedTenantIDs {
			var tenantRow tenant
			if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_id = ? AND id = ?", userID, tenantID).First(&tenantRow).Error; err != nil {
				return tenancyAgreement{}, err
			}
		}
		var legacy rentObligation
		legacyErr := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("user_id = ? AND tenant_id IN ? AND period_month >= ? AND rent_charge_id IS NULL", userID, tenantIDs, effectiveMonth).
			Order("period_month ASC, tenant_id ASC, id ASC").First(&legacy).Error
		if legacyErr == nil {
			return tenancyAgreement{}, fmt.Errorf("%w: tenant %d already has a legacy obligation for %s", errRentFactsConflict, legacy.TenantID, monthStart(legacy.PeriodMonth).Format("2006-01"))
		}
		if !errors.Is(legacyErr, gorm.ErrRecordNotFound) {
			return tenancyAgreement{}, legacyErr
		}
		var conflictCount int64
		if err := tx.Table("agreement_parties AS ap").Joins("JOIN tenancy_agreements AS ta ON ta.id = ap.agreement_id AND ta.user_id = ap.user_id").Where("ap.user_id = ? AND ap.tenant_id IN ? AND ap.status = ? AND ta.status = ? AND ta.room_id <> ? AND ta.start_date <= ? AND (ta.end_date IS NULL OR ta.end_date >= ?) AND (ap.joined_at IS NULL OR ap.joined_at <= ?) AND (ap.left_at IS NULL OR ap.left_at >= ?)", userID, tenantIDs, "active", "active", input.RoomID, effectiveMonth.AddDate(0, 1, -1), effectiveMonth, effectiveMonth.AddDate(0, 1, -1), effectiveMonth).Count(&conflictCount).Error; err != nil {
			return tenancyAgreement{}, err
		}
		if conflictCount > 0 {
			return tenancyAgreement{}, errTenantRoomConflict
		}
	}
	contractDate := input.ContractDate
	if contractDate == nil {
		contractDate = inheritedContractDate
	}
	if contractDate == nil {
		contractDate = &startDate
	}
	moveInDate := input.MoveInDate
	if moveInDate == nil {
		moveInDate = inheritedMoveInDate
	}
	if moveInDate == nil {
		moveInDate = &startDate
	}
	agreement := tenancyAgreement{UserID: userID, RoomID: input.RoomID, ContractDate: contractDate, MoveInDate: moveInDate, StartDate: startDate, EndDate: input.EndDate, MonthlyRentCents: monthlyRent, Currency: currency, DueDay: dueDay, Status: "active"}
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
	if err := tx.Model(&room{}).Where("id = ? AND user_id = ?", input.RoomID, userID).Updates(map[string]any{"monthly_rent_cents": monthlyRent, "due_day": dueDay}).Error; err != nil {
		return tenancyAgreement{}, err
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

// syncTenantRoomBindingInTx changes the current-month room membership for a
// tenant while keeping each room's versioned rent arrangement intact. All
// arrangement writes go through saveRentArrangementInTx so generated charges
// continue to lock the affected month and every change rolls back together.
func syncTenantRoomBindingInTx(tx *gorm.DB, userID, tenantID, roomID uint64, effectiveMonth time.Time, monthlyRentCents int64, currency string, dueDay int, tenantStatus string, roomTenantIDs []uint64, responsibilities []rentResponsibilityInput) error {
	if userID == 0 || tenantID == 0 {
		return errors.New("userID and tenantID are required")
	}
	effectiveMonth = monthStart(effectiveMonth)
	if tenantStatus != "active" {
		roomID = 0
	}

	type membership struct {
		RoomID uint64
	}
	var memberships []membership
	periodEnd := effectiveMonth.AddDate(0, 1, 0).Add(-time.Nanosecond)
	if err := tx.Table("tenancy_agreements AS ta").
		Joins("JOIN agreement_parties AS ap ON ap.agreement_id = ta.id AND ap.user_id = ta.user_id").
		Where("ta.user_id = ? AND ap.tenant_id = ? AND ta.status = ? AND ap.status = ? AND ta.start_date <= ? AND (ta.end_date IS NULL OR ta.end_date >= ?) AND (ap.joined_at IS NULL OR ap.joined_at <= ?) AND (ap.left_at IS NULL OR ap.left_at >= ?)", userID, tenantID, "active", "active", periodEnd, effectiveMonth, periodEnd, effectiveMonth).
		Select("ta.room_id").Order("ta.room_id ASC").Find(&memberships).Error; err != nil {
		return err
	}

	roomIDs := make([]uint64, 0, len(memberships))
	seenRooms := make(map[uint64]struct{}, len(memberships))
	alreadyBound := false
	for _, row := range memberships {
		if row.RoomID == roomID && roomID != 0 {
			alreadyBound = true
		}
		if _, seen := seenRooms[row.RoomID]; seen {
			continue
		}
		seenRooms[row.RoomID] = struct{}{}
		roomIDs = append(roomIDs, row.RoomID)
	}
	loadCurrentArrangement := func(targetRoomID uint64) (tenancyAgreement, []agreementParty, error) {
		var arrangement tenancyAgreement
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("user_id = ? AND room_id = ? AND status = ? AND start_date <= ? AND (end_date IS NULL OR end_date >= ?)", userID, targetRoomID, "active", periodEnd, effectiveMonth).
			Order("start_date DESC, id DESC").First(&arrangement).Error
		if err != nil {
			return tenancyAgreement{}, nil, err
		}
		var parties []agreementParty
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("user_id = ? AND agreement_id = ? AND status = ? AND (joined_at IS NULL OR joined_at <= ?) AND (left_at IS NULL OR left_at >= ?)", userID, arrangement.ID, "active", periodEnd, effectiveMonth).
			Order("tenant_id ASC, id ASC").Find(&parties).Error; err != nil {
			return tenancyAgreement{}, nil, err
		}
		return arrangement, parties, nil
	}

	if alreadyBound && len(roomIDs) == 1 {
		if roomTenantIDs == nil {
			return nil
		}
		currentArrangement, currentParties, err := loadCurrentArrangement(roomID)
		if err != nil {
			return err
		}
		currentPlan := make(map[uint64]int64, len(currentParties))
		for _, party := range currentParties {
			currentPlan[party.TenantID] = party.ResponsibilityCents
		}
		requestedPlan := make(map[uint64]int64, len(responsibilities))
		for _, responsibility := range responsibilities {
			requestedPlan[responsibility.TenantID] = responsibility.AmountCents
		}
		plansEqual := len(currentPlan) == len(requestedPlan) && len(requestedPlan) == len(roomTenantIDs)
		for id, amount := range currentPlan {
			if requestedPlan[id] != amount {
				plansEqual = false
				break
			}
		}
		if plansEqual && currentArrangement.MonthlyRentCents == monthlyRentCents {
			return nil
		}
	}

	for _, sourceRoomID := range roomIDs {
		if sourceRoomID == roomID && roomID != 0 {
			continue
		}
		arrangement, parties, err := loadCurrentArrangement(sourceRoomID)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			continue
		}
		if err != nil {
			return err
		}
		remainingTenantIDs := make([]uint64, 0, len(parties))
		for _, party := range parties {
			if party.TenantID != tenantID {
				remainingTenantIDs = append(remainingTenantIDs, party.TenantID)
			}
		}
		if _, err := saveRentArrangementInTx(tx, userID, rentArrangementInput{
			RoomID: sourceRoomID, EffectiveMonth: effectiveMonth,
			MonthlyRentCents: arrangement.MonthlyRentCents, Currency: arrangement.Currency,
			DueDay: arrangement.DueDay, TenantIDs: remainingTenantIDs,
		}); err != nil {
			return err
		}
	}

	if roomID == 0 {
		return nil
	}
	destinationTenantIDs := []uint64{tenantID}
	destinationHasArrangement := false
	if _, parties, err := loadCurrentArrangement(roomID); err == nil {
		destinationHasArrangement = true
		for _, party := range parties {
			if party.TenantID != tenantID {
				destinationTenantIDs = append(destinationTenantIDs, party.TenantID)
			}
		}
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	if roomTenantIDs != nil {
		destinationTenantIDs = append([]uint64(nil), roomTenantIDs...)
		containsTenant := false
		for _, candidate := range destinationTenantIDs {
			if candidate == tenantID {
				containsTenant = true
				break
			}
		}
		if !containsTenant {
			return errors.New("current tenant must remain in room responsibilities")
		}
	}
	destinationCurrency, destinationDueDay := currency, dueDay
	if destinationHasArrangement {
		// The room owns its currency and due date. The tenant form can adjust
		// the room total, but its profile values must not overwrite these fields.
		destinationCurrency, destinationDueDay = "", 0
	}
	_, err := saveRentArrangementInTx(tx, userID, rentArrangementInput{
		RoomID: roomID, EffectiveMonth: effectiveMonth,
		MonthlyRentCents: monthlyRentCents, Currency: destinationCurrency,
		DueDay: destinationDueDay, TenantIDs: destinationTenantIDs, Responsibilities: responsibilities,
	})
	return err
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
