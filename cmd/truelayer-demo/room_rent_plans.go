package main

import (
	"context"
	"errors"
	"math"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var (
	ErrInvalidRentPlan          = errors.New("rent plan input is invalid")
	ErrRentPlanTimelineConflict = errors.New("room rent plan timeline conflicts")
	ErrTenantRoomMonthConflict  = errors.New("tenant is already assigned to another room in the rent plan")
	ErrStaleRentPlanTimeline    = errors.New("room rent plan timeline changed")
	ErrRoomPropertyLocked       = errors.New("room property is locked by rent history")
)

type RoomRentPlanMemberInput struct {
	TenantID            uint64
	ResponsibilityCents int64
}

type SaveRoomRentPlanCommand struct {
	UserID                  uint64
	RoomID                  uint64
	EffectiveMonth          time.Time
	MonthlyRentCents        int64
	Currency                string
	DueDay                  int
	Members                 []RoomRentPlanMemberInput
	ExpectedTimelineVersion uint64
}

type EndRoomRentPlanCommand struct {
	UserID                  uint64
	RoomID                  uint64
	VacantFromMonth         time.Time
	ExpectedTimelineVersion uint64
}

type roomRentPlanService struct {
	db *gorm.DB
}

func newRoomRentPlanService(db *gorm.DB) *roomRentPlanService {
	return &roomRentPlanService{db: db}
}

func (s *roomRentPlanService) SaveRoomRentPlan(ctx context.Context, command SaveRoomRentPlanCommand) (roomRentPlan, uint64, error) {
	command.EffectiveMonth = monthStart(command.EffectiveMonth)
	currency, currencyErr := normalizeLedgerCurrency(command.Currency)
	if currencyErr != nil {
		return roomRentPlan{}, 0, ErrInvalidRentPlan
	}
	command.Currency = currency
	members, err := validateRoomRentPlanCommand(command)
	if err != nil {
		return roomRentPlan{}, 0, err
	}
	if s == nil || s.db == nil {
		return roomRentPlan{}, 0, errors.New("room rent plan database is required")
	}
	var saved roomRentPlan
	var version uint64
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var roomRow room
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_id = ? AND id = ?", command.UserID, command.RoomID).First(&roomRow).Error; err != nil {
			return err
		}
		if roomRow.RentPlanVersion != command.ExpectedTimelineVersion {
			return ErrStaleRentPlanTimeline
		}
		if len(members) > 0 {
			if err := validatePlanMembersBelongToUser(tx, command.UserID, members); err != nil {
				return err
			}
			if err := validateTenantRoomMonthAvailability(tx, command.UserID, command.RoomID, command.EffectiveMonth, members); err != nil {
				return err
			}
		}
		locked, err := rentFactsLockedFromMonth(tx, command.UserID, command.RoomID, command.EffectiveMonth)
		if err != nil {
			return err
		}
		if locked {
			return ErrRentPlanFactsLocked
		}
		if err := deleteUnlockedRentFactsFromMonth(tx, command.UserID, command.RoomID, command.EffectiveMonth); err != nil {
			return err
		}
		if err := replaceRoomPlanTimelineFrom(tx, command.UserID, command.RoomID, command.EffectiveMonth); err != nil {
			return err
		}
		saved = roomRentPlan{
			UserID: command.UserID, RoomID: command.RoomID,
			EffectiveFromMonth: command.EffectiveMonth,
			MonthlyRentCents:   command.MonthlyRentCents,
			Currency:           command.Currency,
			DueDay:             command.DueDay,
		}
		if err := tx.Create(&saved).Error; err != nil {
			return translateRentPlanConstraintError(err)
		}
		for _, member := range members {
			row := roomRentPlanMember{
				UserID: command.UserID, RoomRentPlanID: saved.ID,
				TenantID: member.TenantID, ResponsibilityCents: member.ResponsibilityCents,
			}
			if err := tx.Create(&row).Error; err != nil {
				return err
			}
		}
		if err := verifyRoomPlanTimeline(tx, command.UserID, command.RoomID); err != nil {
			return err
		}
		version = roomRow.RentPlanVersion + 1
		if err := tx.Model(&room{}).Where("user_id = ? AND id = ? AND rent_plan_version = ?", command.UserID, command.RoomID, roomRow.RentPlanVersion).
			Update("rent_plan_version", version).Error; err != nil {
			return err
		}
		if len(members) > 0 && !command.EffectiveMonth.After(dublinCurrentMonth(time.Now())) {
			return materializeRoomRentFactsInTx(tx, command.UserID, command.RoomID, command.EffectiveMonth)
		}
		return nil
	})
	return saved, version, err
}

// validateTenantRoomMonthAvailability is called after the member tenant rows
// have been locked by validatePlanMembersBelongToUser. That tenant-row lock is
// the serialization point for all plan writes, including writes for different
// rooms. A locking read of the other-room plans then sees a concurrently
// committed assignment before this transaction can create its own member rows.
func validateTenantRoomMonthAvailability(tx *gorm.DB, userID, roomID uint64, effectiveMonth time.Time, members []RoomRentPlanMemberInput) error {
	if tx == nil || userID == 0 || roomID == 0 || len(members) == 0 {
		return ErrInvalidRentPlan
	}
	tenantIDs := make([]uint64, len(members))
	for i, member := range members {
		tenantIDs[i] = member.TenantID
	}
	type tenantRoomPlanConflict struct {
		TenantID           uint64
		RoomID             uint64
		EffectiveFromMonth time.Time
		EffectiveToMonth   *time.Time
	}
	var conflicts []tenantRoomPlanConflict
	query := tx.Table("room_rent_plan_members AS m").
		Select("m.tenant_id, p.room_id, p.effective_from_month, p.effective_to_month").
		Joins("JOIN room_rent_plans AS p ON p.user_id = m.user_id AND p.id = m.room_rent_plan_id").
		Where("m.user_id = ? AND m.tenant_id IN ? AND p.room_id <> ? AND (p.effective_to_month IS NULL OR p.effective_to_month >= ?)", userID, tenantIDs, roomID, monthStart(effectiveMonth)).
		Order("m.tenant_id ASC, p.room_id ASC, p.effective_from_month ASC, p.id ASC").
		Clauses(clause.Locking{Strength: "UPDATE"})
	if err := query.Scan(&conflicts).Error; err != nil {
		return err
	}
	if len(conflicts) > 0 {
		return ErrTenantRoomMonthConflict
	}
	return nil
}

func (s *roomRentPlanService) EndRoomRentPlan(ctx context.Context, command EndRoomRentPlanCommand) (uint64, error) {
	command.VacantFromMonth = monthStart(command.VacantFromMonth)
	if command.UserID == 0 || command.RoomID == 0 || command.VacantFromMonth.IsZero() {
		return 0, ErrInvalidRentPlan
	}
	if s == nil || s.db == nil {
		return 0, errors.New("room rent plan database is required")
	}
	var version uint64
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var roomRow room
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_id = ? AND id = ?", command.UserID, command.RoomID).First(&roomRow).Error; err != nil {
			return err
		}
		if roomRow.RentPlanVersion != command.ExpectedTimelineVersion {
			return ErrStaleRentPlanTimeline
		}
		var planCount int64
		if err := tx.Model(&roomRentPlan{}).Where("user_id = ? AND room_id = ? AND (effective_to_month IS NULL OR effective_to_month >= ?)", command.UserID, command.RoomID, command.VacantFromMonth).Count(&planCount).Error; err != nil {
			return err
		}
		if planCount == 0 {
			return gorm.ErrRecordNotFound
		}
		locked, err := rentFactsLockedFromMonth(tx, command.UserID, command.RoomID, command.VacantFromMonth)
		if err != nil {
			return err
		}
		if locked {
			return ErrRentPlanFactsLocked
		}
		if err := deleteUnlockedRentFactsFromMonth(tx, command.UserID, command.RoomID, command.VacantFromMonth); err != nil {
			return err
		}
		if err := replaceRoomPlanTimelineFrom(tx, command.UserID, command.RoomID, command.VacantFromMonth); err != nil {
			return err
		}
		version = roomRow.RentPlanVersion + 1
		return tx.Model(&room{}).Where("user_id = ? AND id = ? AND rent_plan_version = ?", command.UserID, command.RoomID, roomRow.RentPlanVersion).
			Update("rent_plan_version", version).Error
	})
	return version, err
}

func validateRoomRentPlanCommand(command SaveRoomRentPlanCommand) ([]RoomRentPlanMemberInput, error) {
	if command.UserID == 0 || command.RoomID == 0 || command.EffectiveMonth.IsZero() || command.MonthlyRentCents <= 0 || command.DueDay < 1 || command.DueDay > 31 {
		return nil, ErrInvalidRentPlan
	}
	if monthStart(command.EffectiveMonth).Before(dublinCurrentMonth(time.Now())) {
		return nil, ErrInvalidRentPlan
	}
	currency, err := normalizeLedgerCurrency(command.Currency)
	if err != nil || currency != ledgerCurrencyEUR {
		return nil, ErrInvalidRentPlan
	}
	command.Currency = currency
	if len(command.Members) == 0 {
		return nil, nil
	}
	members := append([]RoomRentPlanMemberInput(nil), command.Members...)
	sort.Slice(members, func(i, j int) bool { return members[i].TenantID < members[j].TenantID })
	seen := make(map[uint64]struct{}, len(members))
	unspecifiedIDs := make([]uint64, 0, len(members))
	var specifiedTotal int64
	for _, member := range members {
		if member.TenantID == 0 {
			return nil, ErrInvalidRentPlan
		}
		if _, exists := seen[member.TenantID]; exists {
			return nil, ErrInvalidRentPlan
		}
		seen[member.TenantID] = struct{}{}
		if member.ResponsibilityCents == 0 {
			unspecifiedIDs = append(unspecifiedIDs, member.TenantID)
			continue
		}
		if member.ResponsibilityCents < 0 || member.ResponsibilityCents > math.MaxInt64-specifiedTotal {
			return nil, ErrInvalidRentPlan
		}
		specifiedTotal += member.ResponsibilityCents
	}
	if len(unspecifiedIDs) > 0 {
		remaining := command.MonthlyRentCents - specifiedTotal
		if remaining <= 0 {
			return nil, ErrInvalidRentPlan
		}
		shares, err := splitRentAmountEvenly(remaining, unspecifiedIDs)
		if err != nil {
			return nil, ErrInvalidRentPlan
		}
		amountByTenantID := make(map[uint64]int64, len(shares))
		for _, share := range shares {
			amountByTenantID[share.TenantID] = share.AmountCents
		}
		for i := range members {
			if members[i].ResponsibilityCents == 0 {
				members[i].ResponsibilityCents = amountByTenantID[members[i].TenantID]
			}
		}
		return members, nil
	}
	if specifiedTotal != command.MonthlyRentCents {
		return nil, ErrInvalidRentPlan
	}
	return members, nil
}

func validatePlanMembersBelongToUser(tx *gorm.DB, userID uint64, members []RoomRentPlanMemberInput) error {
	ids := make([]uint64, len(members))
	for i := range members {
		ids[i] = members[i].TenantID
	}
	var tenants []tenant
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_id = ? AND id IN ? AND status = ?", userID, ids, "active").Order("id ASC").Find(&tenants).Error; err != nil {
		return err
	}
	if len(tenants) != len(ids) {
		return gorm.ErrRecordNotFound
	}
	return nil
}

func replaceRoomPlanTimelineFrom(tx *gorm.DB, userID, roomID uint64, fromMonth time.Time) error {
	fromMonth = monthStart(fromMonth)
	cutoff := fromMonth.AddDate(0, -1, 0)
	var covered []roomRentPlan
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_id = ? AND room_id = ? AND effective_from_month < ? AND (effective_to_month IS NULL OR effective_to_month >= ?)", userID, roomID, fromMonth, fromMonth).Find(&covered).Error; err != nil {
		return err
	}
	if len(covered) > 1 {
		return ErrRentPlanTimelineConflict
	}
	for _, plan := range covered {
		if err := tx.Model(&plan).Update("effective_to_month", cutoff).Error; err != nil {
			return err
		}
	}
	var future []roomRentPlan
	if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_id = ? AND room_id = ? AND effective_from_month >= ?", userID, roomID, fromMonth).Find(&future).Error; err != nil {
		return err
	}
	if len(future) == 0 {
		return nil
	}
	ids := make([]uint64, len(future))
	for i := range future {
		ids[i] = future[i].ID
	}
	if err := tx.Where("user_id = ? AND room_rent_plan_id IN ?", userID, ids).Delete(&roomRentPlanMember{}).Error; err != nil {
		return err
	}
	return tx.Where("user_id = ? AND room_id = ? AND id IN ?", userID, roomID, ids).Delete(&roomRentPlan{}).Error
}

func verifyRoomPlanTimeline(tx *gorm.DB, userID, roomID uint64) error {
	var plans []roomRentPlan
	if err := tx.Where("user_id = ? AND room_id = ?", userID, roomID).Order("effective_from_month ASC, id ASC").Find(&plans).Error; err != nil {
		return err
	}
	for i := 1; i < len(plans); i++ {
		previousEnd := plans[i-1].EffectiveToMonth
		if previousEnd == nil || !monthStart(*previousEnd).Before(monthStart(plans[i].EffectiveFromMonth)) {
			return ErrRentPlanTimelineConflict
		}
	}
	return nil
}

func translateRentPlanConstraintError(err error) error {
	if err == nil {
		return nil
	}
	if strings.Contains(strings.ToLower(err.Error()), "duplicate") || strings.Contains(strings.ToLower(err.Error()), "chk_room_rent_plans_month_interval") {
		return ErrRentPlanTimelineConflict
	}
	return err
}

func dublinCurrentMonth(now time.Time) time.Time {
	location, err := time.LoadLocation("Europe/Dublin")
	if err != nil {
		location = time.UTC
	}
	return monthStart(now.In(location))
}
