package main

import (
	"context"
	"errors"
	"sort"
	"time"

	"gorm.io/gorm"
)

type rentFactsIntent uint8

const (
	rentFactsIntentRead rentFactsIntent = iota
	rentFactsIntentExplicitPayment
)

type monthlyRentFactsService struct {
	db *gorm.DB
}

type rentMonthlyFactTarget struct {
	PropertyID uint64 `gorm:"column:property_id"`
	RoomID     uint64 `gorm:"column:room_id"`
}

func newMonthlyRentFactsService(db *gorm.DB) *monthlyRentFactsService {
	return &monthlyRentFactsService{db: db}
}

func rentFactsCanMaterialize(periodMonth, now time.Time, intent rentFactsIntent) bool {
	if periodMonth.IsZero() || now.IsZero() {
		return false
	}
	if intent == rentFactsIntentExplicitPayment {
		return true
	}
	return intent == rentFactsIntentRead && !monthStart(periodMonth).After(monthStart(now))
}

// ensureMonthlyRentFacts is the only business entry point that creates rent
// obligations. A normal read may materialize a started month or history; a
// future month is materialized only when a caller records explicit payment
// intent for that month.
func (s *monthlyRentFactsService) ensureMonthlyRentFacts(ctx context.Context, userID uint64, periodMonth time.Time, intent rentFactsIntent) error {
	if userID == 0 {
		return errors.New("userID is required")
	}
	if periodMonth.IsZero() {
		return errors.New("period month is required")
	}
	if s == nil || s.db == nil {
		return errors.New("monthly rent facts database is required")
	}
	if intent != rentFactsIntentRead && intent != rentFactsIntentExplicitPayment {
		return errors.New("monthly rent facts intent is invalid")
	}
	periodMonth = monthStart(periodMonth)
	if !rentFactsCanMaterialize(periodMonth, time.Now().UTC(), intent) {
		return nil
	}

	targets := make(map[uint64]rentMonthlyFactTarget)
	var arrangements []rentMonthlyFactTarget
	if err := s.db.WithContext(ctx).Table("room_rent_plans AS ta").
		Select("ta.room_id AS room_id, r.property_id AS property_id").
		Joins("JOIN rooms AS r ON r.id = ta.room_id AND r.user_id = ta.user_id").
		Where("ta.user_id = ? AND ta.effective_from_month <= ? AND (ta.effective_to_month IS NULL OR ta.effective_to_month >= ?)", userID, periodMonth, periodMonth).
		Order("ta.room_id ASC").Scan(&arrangements).Error; err != nil {
		return err
	}
	for _, target := range arrangements {
		if target.RoomID != 0 && target.PropertyID != 0 {
			targets[target.RoomID] = target
		}
	}
	planRoomIDs := targetPlanRoomIDs(arrangements)
	var existingCharges []rentCharge
	if err := s.db.WithContext(ctx).Where("user_id = ? AND period_month = ?", userID, periodMonth).Order("room_id ASC, id ASC").Find(&existingCharges).Error; err != nil {
		return err
	}
	for _, charge := range existingCharges {
		if charge.RoomID != 0 && charge.PropertyID != 0 {
			targets[charge.RoomID] = rentMonthlyFactTarget{PropertyID: charge.PropertyID, RoomID: charge.RoomID}
		}
	}
	roomIDs := make([]uint64, 0, len(targets))
	for roomID := range targets {
		roomIDs = append(roomIDs, roomID)
	}
	sort.Slice(roomIDs, func(i, j int) bool { return roomIDs[i] < roomIDs[j] })

	for _, roomID := range roomIDs {
		target := targets[roomID]
		var chargeCount int64
		if err := s.db.WithContext(ctx).Model(&rentCharge{}).Where("user_id = ? AND room_id = ? AND period_month = ?", userID, roomID, periodMonth).Count(&chargeCount).Error; err != nil {
			return err
		}
		if chargeCount == 0 {
			if _, hasMatchingPlan := planRoomIDs[roomID]; !hasMatchingPlan {
				continue
			}
			if err := materializeRoomRentFactsInTx(s.db.WithContext(ctx), userID, target.RoomID, periodMonth); errors.Is(err, gorm.ErrRecordNotFound) {
				continue
			} else if err != nil {
				return err
			}
			continue
		}
	}
	return nil
}

func materializeRoomRentFactsInTx(tx *gorm.DB, userID, roomID uint64, periodMonth time.Time) error {
	if tx == nil || userID == 0 || roomID == 0 || periodMonth.IsZero() {
		return errors.New("transaction, user, room, and month are required")
	}
	var roomRow room
	if err := tx.Where("user_id = ? AND id = ?", userID, roomID).First(&roomRow).Error; err != nil {
		return err
	}
	returnedContext := tx.Statement.Context
	if returnedContext == nil {
		returnedContext = context.Background()
	}
	_, err := newRentLedgerService(tx).ensureRentCharge(returnedContext, userID, roomRow.PropertyID, roomID, monthStart(periodMonth))
	return err
}

func targetPlanRoomIDs(targets []rentMonthlyFactTarget) map[uint64]struct{} {
	ids := make(map[uint64]struct{}, len(targets))
	for _, target := range targets {
		ids[target.RoomID] = struct{}{}
	}
	return ids
}
