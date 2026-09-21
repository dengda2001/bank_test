package main

import (
	"context"
	"errors"
	"sort"
	"time"

	"gorm.io/gorm"
)

var errRentFactsConflict = errors.New("rent facts conflict")

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
	monthEnd := periodMonth.AddDate(0, 1, -1)
	var arrangements []rentMonthlyFactTarget
	if err := s.db.WithContext(ctx).Table("tenancy_agreements AS ta").
		Select("ta.room_id AS room_id, r.property_id AS property_id").
		Joins("JOIN rooms AS r ON r.id = ta.room_id AND r.user_id = ta.user_id").
		Where("ta.user_id = ? AND ta.status = ? AND ta.start_date <= ? AND (ta.end_date IS NULL OR ta.end_date >= ?)", userID, "active", monthEnd, periodMonth).
		Order("ta.room_id ASC").Scan(&arrangements).Error; err != nil {
		return err
	}
	for _, target := range arrangements {
		if target.RoomID != 0 && target.PropertyID != 0 {
			targets[target.RoomID] = target
		}
	}
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

	ledger := newRentLedgerService(s.db)
	for _, roomID := range roomIDs {
		target := targets[roomID]
		var chargeCount int64
		if err := s.db.WithContext(ctx).Model(&rentCharge{}).Where("user_id = ? AND room_id = ? AND period_month = ?", userID, roomID, periodMonth).Count(&chargeCount).Error; err != nil {
			return err
		}
		if chargeCount == 0 {
			var activePartyCount int64
			if err := s.db.WithContext(ctx).Table("agreement_parties AS ap").
				Joins("JOIN tenancy_agreements AS ta ON ta.id = ap.agreement_id AND ta.user_id = ap.user_id").
				Where("ap.user_id = ? AND ta.room_id = ? AND ta.status = ? AND ta.start_date <= ? AND (ta.end_date IS NULL OR ta.end_date >= ?) AND ap.status = ? AND (ap.joined_at IS NULL OR ap.joined_at <= ?) AND (ap.left_at IS NULL OR ap.left_at >= ?)", userID, roomID, "active", monthEnd, periodMonth, "active", monthEnd, periodMonth).
				Count(&activePartyCount).Error; err != nil {
				return err
			}
			if activePartyCount == 0 {
				continue
			}
		}
		if _, err := ledger.ensureRentCharge(ctx, userID, target.PropertyID, target.RoomID, periodMonth); err != nil {
			return err
		}
	}

	// The legacy generator is intentionally a fallback. It skips tenants whose
	// month is already represented by a structured room arrangement or charge.
	if err := newObligationService(s.db).ensureMonthlyObligations(ctx, userID, periodMonth); err != nil {
		return err
	}
	return nil
}

func tenantHasStructuredRentFacts(db *gorm.DB, userID, tenantID uint64, periodMonth time.Time) (bool, error) {
	var obligationCount int64
	if err := db.Model(&rentObligation{}).
		Where("user_id = ? AND tenant_id = ? AND period_month = ? AND rent_charge_id IS NOT NULL", userID, tenantID, monthStart(periodMonth)).
		Count(&obligationCount).Error; err != nil {
		return false, err
	}
	if obligationCount > 0 {
		return true, nil
	}
	monthEnd := monthStart(periodMonth).AddDate(0, 1, -1)
	var partyCount int64
	err := db.Table("agreement_parties AS ap").
		Joins("JOIN tenancy_agreements AS ta ON ta.id = ap.agreement_id AND ta.user_id = ap.user_id").
		Where("ap.user_id = ? AND ap.tenant_id = ? AND ap.status = ? AND ta.status = ? AND ta.start_date <= ? AND (ta.end_date IS NULL OR ta.end_date >= ?) AND (ap.joined_at IS NULL OR ap.joined_at <= ?) AND (ap.left_at IS NULL OR ap.left_at >= ?)", userID, tenantID, "active", "active", monthEnd, monthStart(periodMonth), monthEnd, monthStart(periodMonth)).
		Count(&partyCount).Error
	return partyCount > 0, err
}
