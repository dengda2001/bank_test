package main

import (
	"context"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	errPropertyNameRequired    = errors.New("property name is required")
	errRoomLabelRequired       = errors.New("room label is required")
	errPropertyDeletionBlocked = errors.New("property has linked rooms or financial records")
	errRoomDeletionBlocked     = errors.New("room has linked rent or expense records")
)

type propertyInput struct {
	Name       string
	CityRegion string
	Address    string
	Timezone   string
	Notes      string
}

type roomInput struct {
	PropertyID uint64
	RoomLabel  string
	RoomType   string
	Capacity   int
	Notes      string
}

type roomRentPlanSetupInput struct {
	EffectiveMonth   time.Time
	MonthlyRentCents int64
	Currency         string
	DueDay           int
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
		return tx.Model(&row).Updates(map[string]any{
			"name": input.Name, "city_region": strings.TrimSpace(input.CityRegion),
			"address": nullableString(strings.TrimSpace(input.Address)), "timezone": input.Timezone,
			"notes": nullableString(strings.TrimSpace(input.Notes)),
		}).Error
	})
	return row, err
}

func (s *landlordDomainService) createRoom(ctx context.Context, userID uint64, input roomInput) (room, error) {
	if userID == 0 || input.PropertyID == 0 {
		return room{}, errors.New("userID and propertyID are required")
	}
	var err error
	input, err = normalizeNewRoomInput(input)
	if err != nil {
		return room{}, err
	}
	var row room
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var propertyRow property
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_id = ? AND id = ?", userID, input.PropertyID).First(&propertyRow).Error; err != nil {
			return err
		}
		row = room{UserID: userID, PropertyID: input.PropertyID, RoomLabel: input.RoomLabel, RoomType: input.RoomType, Capacity: input.Capacity, Notes: nullableString(strings.TrimSpace(input.Notes)), Status: "active"}
		return tx.Create(&row).Error
	})
	return row, err
}

func (s *landlordDomainService) createRoomWithRentPlan(ctx context.Context, userID uint64, input roomInput, setup roomRentPlanSetupInput) (room, roomRentPlan, error) {
	return s.createRoomWithRentPlanAndTenant(ctx, userID, input, setup, 0)
}

func (s *landlordDomainService) createRoomWithRentPlanAndTenant(ctx context.Context, userID uint64, input roomInput, setup roomRentPlanSetupInput, tenantID uint64) (room, roomRentPlan, error) {
	if userID == 0 || input.PropertyID == 0 {
		return room{}, roomRentPlan{}, errors.New("userID and propertyID are required")
	}
	var err error
	input, err = normalizeNewRoomInput(input)
	if err != nil {
		return room{}, roomRentPlan{}, err
	}
	setup, err = normalizeRoomRentPlanSetup(userID, setup)
	if err != nil {
		return room{}, roomRentPlan{}, err
	}
	var created room
	var plan roomRentPlan
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var propertyRow property
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_id = ? AND id = ?", userID, input.PropertyID).First(&propertyRow).Error; err != nil {
			return err
		}
		created = room{UserID: userID, PropertyID: input.PropertyID, RoomLabel: input.RoomLabel, RoomType: input.RoomType, Capacity: input.Capacity, Notes: nullableString(strings.TrimSpace(input.Notes)), Status: "active"}
		if err := tx.Create(&created).Error; err != nil {
			return err
		}
		plan = roomRentPlan{UserID: userID, RoomID: created.ID, EffectiveFromMonth: setup.EffectiveMonth, MonthlyRentCents: setup.MonthlyRentCents, Currency: setup.Currency, DueDay: setup.DueDay}
		if err := tx.Create(&plan).Error; err != nil {
			return translateRentPlanConstraintError(err)
		}
		if err := tx.Model(&room{}).Where("user_id = ? AND id = ? AND rent_plan_version = ?", userID, created.ID, 0).Update("rent_plan_version", 1).Error; err != nil {
			return err
		}
		created.RentPlanVersion = 1
		if tenantID != 0 {
			assignedPlan, version, err := newRoomRentPlanService(tx).SaveRoomRentPlan(ctx, SaveRoomRentPlanCommand{
				UserID: userID, RoomID: created.ID, EffectiveMonth: setup.EffectiveMonth,
				MonthlyRentCents: setup.MonthlyRentCents, Currency: setup.Currency, DueDay: setup.DueDay,
				Members:                 []RoomRentPlanMemberInput{{TenantID: tenantID, ResponsibilityCents: setup.MonthlyRentCents}},
				ExpectedTimelineVersion: created.RentPlanVersion,
			})
			if err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return ErrInvalidRentPlan
				}
				return err
			}
			plan = assignedPlan
			created.RentPlanVersion = version
		}
		return nil
	})
	return created, plan, err
}

func normalizeNewRoomInput(input roomInput) (roomInput, error) {
	input.RoomLabel = strings.TrimSpace(input.RoomLabel)
	if input.RoomLabel == "" {
		return roomInput{}, errRoomLabelRequired
	}
	input.RoomType = strings.TrimSpace(input.RoomType)
	if len([]rune(input.RoomLabel)) > 191 || len([]rune(input.RoomType)) > 64 || len([]rune(input.Notes)) > 2000 || input.Capacity < 0 || input.Capacity > 100 {
		return roomInput{}, errors.New("room details are invalid")
	}
	if input.RoomType == "" {
		input.RoomType = "其他"
	}
	if input.Capacity == 0 {
		input.Capacity = 1
	}
	return input, nil
}

func normalizeRoomRentPlanSetup(userID uint64, setup roomRentPlanSetupInput) (roomRentPlanSetupInput, error) {
	setup.EffectiveMonth = monthStart(setup.EffectiveMonth)
	currency, err := normalizeLedgerCurrency(setup.Currency)
	if err != nil {
		return roomRentPlanSetupInput{}, ErrInvalidRentPlan
	}
	setup.Currency = currency
	if _, err := validateRoomRentPlanCommand(SaveRoomRentPlanCommand{
		UserID: userID, RoomID: 1, EffectiveMonth: setup.EffectiveMonth,
		MonthlyRentCents: setup.MonthlyRentCents, Currency: setup.Currency, DueDay: setup.DueDay,
	}); err != nil {
		return roomRentPlanSetupInput{}, err
	}
	return setup, nil
}

func (s *landlordDomainService) updateRoom(ctx context.Context, userID, roomID uint64, input roomInput) (room, error) {
	if userID == 0 || roomID == 0 || input.PropertyID == 0 {
		return room{}, errors.New("userID, roomID, and propertyID are required")
	}
	input.RoomLabel = strings.TrimSpace(input.RoomLabel)
	input.RoomType = strings.TrimSpace(input.RoomType)
	if input.RoomLabel == "" {
		return room{}, errRoomLabelRequired
	}
	if len([]rune(input.RoomLabel)) > 191 || len([]rune(input.RoomType)) > 64 || len([]rune(input.Notes)) > 2000 || input.Capacity < 0 || input.Capacity > 100 {
		return room{}, errors.New("room details are invalid")
	}
	var row room
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_id = ? AND id = ?", userID, roomID).First(&row).Error; err != nil {
			return err
		}
		if row.PropertyID != input.PropertyID {
			var chargeCount int64
			if err := tx.Model(&rentCharge{}).Where("user_id = ? AND room_id = ?", userID, roomID).Count(&chargeCount).Error; err != nil {
				return err
			}
			if chargeCount > 0 {
				return ErrRoomPropertyLocked
			}
			var target property
			if err := tx.Where("user_id = ? AND id = ?", userID, input.PropertyID).First(&target).Error; err != nil {
				return err
			}
		}
		updates := map[string]any{
			"property_id": input.PropertyID,
			"room_label":  input.RoomLabel,
			"room_type":   firstNonEmpty(input.RoomType, "其他"),
			"notes":       nullableString(strings.TrimSpace(input.Notes)),
		}
		if input.Capacity > 0 {
			updates["capacity"] = input.Capacity
		}
		return tx.Model(&row).Updates(updates).Error
	})
	return row, err
}

func (s *landlordDomainService) deactivateProperty(ctx context.Context, userID, propertyID uint64) error {
	if userID == 0 || propertyID == 0 {
		return errors.New("userID and propertyID are required")
	}
	return s.db.WithContext(ctx).Model(&property{}).Where("user_id = ? AND id = ?", userID, propertyID).Update("status", "inactive").Error
}

func (s *landlordDomainService) deactivateRoom(ctx context.Context, userID, roomID uint64, _ time.Time) error {
	if userID == 0 || roomID == 0 {
		return errors.New("userID and roomID are required")
	}
	return s.db.WithContext(ctx).Model(&room{}).Where("user_id = ? AND id = ?", userID, roomID).Update("status", "inactive").Error
}

func scopedDependencyExists(tx *gorm.DB, value any, query string, args ...any) (bool, error) {
	var count int64
	if err := tx.Model(value).Where(query, args...).Count(&count).Error; err != nil {
		return false, err
	}
	return count > 0, nil
}

// deleteProperty only removes an object that has no child rooms or financial
// history. Historical ledger rows must stay traceable, so callers can use the
// existing deactivation action when this guard rejects the deletion.
func (s *landlordDomainService) deleteProperty(ctx context.Context, userID, propertyID uint64) error {
	if userID == 0 || propertyID == 0 {
		return errors.New("userID and propertyID are required")
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row property
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND user_id = ?", propertyID, userID).First(&row).Error; err != nil {
			return err
		}
		for _, dependency := range []struct {
			value any
			query string
			args  []any
		}{
			{&room{}, "user_id = ? AND property_id = ?", []any{userID, propertyID}},
			{&rentCharge{}, "user_id = ? AND property_id = ?", []any{userID, propertyID}},
			{&manualExpense{}, "user_id = ? AND property_id = ?", []any{userID, propertyID}},
		} {
			exists, err := scopedDependencyExists(tx, dependency.value, dependency.query, dependency.args...)
			if err != nil {
				return err
			}
			if exists {
				return errPropertyDeletionBlocked
			}
		}
		return tx.Delete(&row).Error
	})
}

// deleteRoom follows the same historical-record guard as property deletion.
// A rent plan, charge, or expense means the room remains part of the audit
// trail and should be deactivated rather than removed.
func (s *landlordDomainService) deleteRoom(ctx context.Context, userID, roomID uint64) error {
	if userID == 0 || roomID == 0 {
		return errors.New("userID and roomID are required")
	}
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row room
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND user_id = ?", roomID, userID).First(&row).Error; err != nil {
			return err
		}
		for _, dependency := range []struct {
			value any
			query string
			args  []any
		}{
			{&roomRentPlan{}, "user_id = ? AND room_id = ?", []any{userID, roomID}},
			{&rentCharge{}, "user_id = ? AND room_id = ?", []any{userID, roomID}},
			{&manualExpense{}, "user_id = ? AND room_id = ?", []any{userID, roomID}},
		} {
			exists, err := scopedDependencyExists(tx, dependency.value, dependency.query, dependency.args...)
			if err != nil {
				return err
			}
			if exists {
				return errRoomDeletionBlocked
			}
		}
		return tx.Delete(&row).Error
	})
}
