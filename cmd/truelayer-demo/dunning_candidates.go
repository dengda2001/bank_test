package main

import (
	"context"
	"errors"
	"time"

	"gorm.io/gorm"
)

type dunningService struct {
	db *gorm.DB
}

func newDunningService(db *gorm.DB) *dunningService {
	return &dunningService{db: db}
}

func (s *dunningService) listCandidates(ctx context.Context, userID uint64, periodMonth time.Time, filters rentDashboardFilters) ([]dunningCandidate, error) {
	if userID == 0 {
		return nil, errors.New("userID is required")
	}
	if err := validateRentDashboardFilters(filters); err != nil {
		return nil, err
	}
	summary, err := newObligationService(s.db).summarizeRentDashboardWithFilters(ctx, userID, periodMonth, filters)
	if err != nil {
		return nil, err
	}
	return s.listCandidatesForRows(ctx, userID, summary.Rows, time.Now().UTC())
}

func (s *dunningService) listCandidatesForRows(ctx context.Context, userID uint64, rows []rentDashboardRow, now time.Time) ([]dunningCandidate, error) {
	if len(rows) == 0 {
		return []dunningCandidate{}, nil
	}
	tenantIDs := make([]uint64, 0, len(rows))
	obligationIDs := make([]uint64, 0, len(rows))
	for _, row := range rows {
		tenantIDs = append(tenantIDs, row.TenantID)
		obligationIDs = append(obligationIDs, row.ObligationID)
	}
	var tenants []tenant
	if err := s.db.WithContext(ctx).Where("user_id = ? AND id IN ?", userID, tenantIDs).Find(&tenants).Error; err != nil {
		return nil, err
	}
	var obligations []rentObligation
	if err := s.db.WithContext(ctx).Where("user_id = ? AND id IN ?", userID, obligationIDs).Find(&obligations).Error; err != nil {
		return nil, err
	}
	tenantByID := make(map[uint64]tenant, len(tenants))
	for _, row := range tenants {
		tenantByID[row.ID] = row
	}
	obligationByID := make(map[uint64]rentObligation, len(obligations))
	for _, row := range obligations {
		obligationByID[row.ID] = row
	}
	var attempts []dunningSendAttempt
	if err := s.db.WithContext(ctx).
		Where("user_id = ? AND rent_obligation_id IN ?", userID, obligationIDs).
		Order("created_at DESC, id DESC").Find(&attempts).Error; err != nil {
		return nil, err
	}
	latestByObligation := make(map[uint64]*dunningSendAttempt)
	sentTodayByObligation := make(map[uint64]bool)
	for index := range attempts {
		attempt := &attempts[index]
		if latestByObligation[attempt.RentObligationID] == nil {
			latestByObligation[attempt.RentObligationID] = attempt
		}
		if attempt.DeliveryStatus == dunningDeliverySent && attempt.SentAt != nil && dunningDublinDate(*attempt.SentAt).Equal(dunningDublinDate(now)) {
			sentTodayByObligation[attempt.RentObligationID] = true
		}
	}
	candidates := make([]dunningCandidate, 0, len(rows))
	for _, row := range rows {
		tenantRow, tenantOK := tenantByID[row.TenantID]
		obligation, obligationOK := obligationByID[row.ObligationID]
		if !tenantOK || !obligationOK {
			continue
		}
		candidate := buildDunningCandidate(obligation, tenantRow, now, latestByObligation[obligation.ID])
		candidate.SentToday = sentTodayByObligation[obligation.ID]
		candidate.DefaultSelected = candidate.Selectable && !candidate.SentToday
		candidates = append(candidates, candidate)
	}
	return candidates, nil
}
