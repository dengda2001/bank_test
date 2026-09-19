package main

import (
	"context"
	"errors"
	"fmt"
	"math/big"
	"sort"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type rentResponsibilityInput struct {
	TenantID    uint64
	AmountCents int64
}

type rentChargePlan struct {
	ExpectedAmountCents int64
	Currency            string
	Responsibilities    []rentResponsibilityInput
}

func buildRentChargePlan(agreement tenancyAgreement, parties []agreementParty, periodMonth time.Time) (rentChargePlan, error) {
	if !tenancyAgreementCoversMonth(agreement, periodMonth) {
		return rentChargePlan{}, errors.New("tenancy agreement does not cover target month")
	}
	currency, err := normalizeLedgerCurrency(agreement.Currency)
	if err != nil {
		return rentChargePlan{}, err
	}
	responsibilities := make([]rentResponsibilityInput, 0, len(parties))
	for _, party := range parties {
		if !agreementPartyActiveInMonth(party, periodMonth) {
			continue
		}
		responsibilities = append(responsibilities, rentResponsibilityInput{
			TenantID:    party.TenantID,
			AmountCents: party.ResponsibilityCents,
		})
	}
	if err := validateRentResponsibilityPlan(agreement.MonthlyRentCents, responsibilities); err != nil {
		return rentChargePlan{}, err
	}
	return rentChargePlan{
		ExpectedAmountCents: agreement.MonthlyRentCents,
		Currency:            currency,
		Responsibilities:    responsibilities,
	}, nil
}

func splitRentAmountEvenly(totalCents int64, tenantIDs []uint64) ([]rentResponsibilityInput, error) {
	if totalCents <= 0 {
		return nil, errors.New("rent amount must be positive")
	}
	if len(tenantIDs) == 0 {
		return nil, errors.New("at least one tenant is required")
	}
	responsibilities := make([]rentResponsibilityInput, len(tenantIDs))
	base := totalCents / int64(len(tenantIDs))
	remainder := totalCents % int64(len(tenantIDs))
	for index, tenantID := range tenantIDs {
		responsibilities[index] = rentResponsibilityInput{
			TenantID:    tenantID,
			AmountCents: base,
		}
		if int64(index) < remainder {
			responsibilities[index].AmountCents++
		}
	}
	if err := validateRentResponsibilityPlan(totalCents, responsibilities); err != nil {
		return nil, err
	}
	return responsibilities, nil
}

func scaleRentResponsibilities(totalCents int64, current []rentResponsibilityInput) ([]rentResponsibilityInput, error) {
	if totalCents <= 0 {
		return nil, errors.New("rent amount must be positive")
	}
	if len(current) == 0 {
		return nil, nil
	}
	const maxInt64 = int64(^uint64(0) >> 1)
	var currentTotal int64
	for _, row := range current {
		if row.AmountCents <= 0 || row.AmountCents > maxInt64-currentTotal {
			return nil, errors.New("current rent responsibilities are invalid")
		}
		currentTotal += row.AmountCents
	}
	if err := validateRentResponsibilityPlan(currentTotal, current); err != nil {
		return nil, err
	}

	type scaledShare struct {
		row       rentResponsibilityInput
		remainder *big.Int
	}
	newTotal := big.NewInt(totalCents)
	oldTotal := big.NewInt(currentTotal)
	shares := make([]scaledShare, 0, len(current))
	var assigned int64
	for _, row := range current {
		numerator := new(big.Int).Mul(newTotal, big.NewInt(row.AmountCents))
		quotient, remainder := new(big.Int), new(big.Int)
		quotient.QuoRem(numerator, oldTotal, remainder)
		if !quotient.IsInt64() {
			return nil, errors.New("scaled rent responsibility is outside the supported range")
		}
		cents := quotient.Int64()
		if cents > totalCents-assigned {
			return nil, errors.New("scaled rent responsibilities exceed the new room rent")
		}
		assigned += cents
		shares = append(shares, scaledShare{row: rentResponsibilityInput{TenantID: row.TenantID, AmountCents: cents}, remainder: remainder})
	}
	leftover := totalCents - assigned
	if leftover < 0 || leftover >= int64(len(shares)) {
		return nil, errors.New("scaled rent remainder is invalid")
	}
	sort.Slice(shares, func(i, j int) bool {
		if comparison := shares[i].remainder.Cmp(shares[j].remainder); comparison != 0 {
			return comparison > 0
		}
		return shares[i].row.TenantID < shares[j].row.TenantID
	})
	for index := int64(0); index < leftover; index++ {
		shares[index].row.AmountCents++
	}
	result := make([]rentResponsibilityInput, len(shares))
	for index := range shares {
		result[index] = shares[index].row
	}
	sort.Slice(result, func(i, j int) bool { return result[i].TenantID < result[j].TenantID })
	if err := validateRentResponsibilityPlan(totalCents, result); err != nil {
		return nil, err
	}
	return result, nil
}

func validateRentResponsibilityPlan(totalCents int64, responsibilities []rentResponsibilityInput) error {
	if totalCents <= 0 {
		return errors.New("rent amount must be positive")
	}
	if len(responsibilities) == 0 {
		return errors.New("at least one tenant responsibility is required")
	}
	seen := make(map[uint64]struct{}, len(responsibilities))
	var total int64
	for _, responsibility := range responsibilities {
		if responsibility.TenantID == 0 {
			return errors.New("responsibility tenant is required")
		}
		if _, exists := seen[responsibility.TenantID]; exists {
			return fmt.Errorf("duplicate responsibility tenant %d", responsibility.TenantID)
		}
		seen[responsibility.TenantID] = struct{}{}
		if responsibility.AmountCents <= 0 {
			return errors.New("responsibility amount must be positive")
		}
		total += responsibility.AmountCents
	}
	if total != totalCents {
		return fmt.Errorf("responsibility sum %d does not equal rent amount %d", total, totalCents)
	}
	return nil
}

func agreementPartyActiveInMonth(party agreementParty, periodMonth time.Time) bool {
	if party.Status != "active" {
		return false
	}
	start := monthStart(periodMonth)
	end := start.AddDate(0, 1, 0).Add(-time.Nanosecond)
	if party.JoinedAt != nil && party.JoinedAt.After(end) {
		return false
	}
	if party.LeftAt != nil && party.LeftAt.Before(start) {
		return false
	}
	return true
}

func roomActiveInMonth(row room, periodMonth time.Time) bool {
	if row.Status != "active" && row.InactiveFrom == nil {
		return false
	}
	start := monthStart(periodMonth)
	end := start.AddDate(0, 1, 0).Add(-time.Nanosecond)
	if row.Status != "active" && row.InactiveFrom != nil && !start.Before(monthStart(*row.InactiveFrom)) {
		return false
	}
	if !row.ActiveFrom.IsZero() && row.ActiveFrom.After(end) {
		return false
	}
	if row.InactiveFrom != nil && row.InactiveFrom.Before(start) {
		return false
	}
	return true
}

func propertyActiveInMonth(row property, periodMonth time.Time) bool {
	if row.Status != "active" && row.InactiveFrom == nil {
		return false
	}
	start := monthStart(periodMonth)
	if row.InactiveFrom != nil && !start.Before(monthStart(*row.InactiveFrom)) {
		return false
	}
	return true
}

func tenancyAgreementCoversMonth(row tenancyAgreement, periodMonth time.Time) bool {
	if row.Status != "active" {
		return false
	}
	start := monthStart(periodMonth)
	end := start.AddDate(0, 1, 0).Add(-time.Nanosecond)
	if !row.StartDate.IsZero() && row.StartDate.After(end) {
		return false
	}
	if row.EndDate != nil && row.EndDate.Before(start) {
		return false
	}
	return true
}

type rentChargeLedger struct {
	Charge      rentCharge
	Obligations []rentObligation
}

type rentLedgerService struct {
	db *gorm.DB
}

func newRentLedgerService(db *gorm.DB) *rentLedgerService {
	return &rentLedgerService{db: db}
}

func (s *rentLedgerService) ensureRentCharge(ctx context.Context, userID, propertyID, roomID uint64, periodMonth time.Time) (rentChargeLedger, error) {
	if userID == 0 || propertyID == 0 || roomID == 0 {
		return rentChargeLedger{}, errors.New("userID, propertyID, and roomID are required")
	}
	if s == nil || s.db == nil {
		return rentChargeLedger{}, errors.New("rent ledger database is required")
	}
	periodMonth = monthStart(periodMonth)
	var result rentChargeLedger
	err := s.db.WithContext(ctx).Transaction(func(txdb *gorm.DB) error {
		var propertyRow property
		if err := txdb.Where("id = ? AND user_id = ?", propertyID, userID).First(&propertyRow).Error; err != nil {
			return err
		}
		if !propertyActiveInMonth(propertyRow, periodMonth) {
			return errors.New("property is not active for target month")
		}
		var roomRow room
		if err := txdb.Where("id = ? AND user_id = ? AND property_id = ?", roomID, userID, propertyID).First(&roomRow).Error; err != nil {
			return err
		}
		if !roomActiveInMonth(roomRow, periodMonth) {
			return errors.New("room is not active for target month")
		}

		var existingCharge rentCharge
		lookupErr := txdb.Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_id = ? AND room_id = ? AND period_month = ?", userID, roomID, periodMonth).First(&existingCharge).Error
		if lookupErr == nil {
			return loadRentChargeLedger(txdb, userID, existingCharge, &result)
		}
		if !errors.Is(lookupErr, gorm.ErrRecordNotFound) {
			return lookupErr
		}

		monthEnd := periodMonth.AddDate(0, 1, -1)
		var agreements []tenancyAgreement
		if err := txdb.Where("user_id = ? AND room_id = ? AND status = ? AND start_date <= ? AND (end_date IS NULL OR end_date >= ?)", userID, roomID, "active", monthEnd, periodMonth).
			Order("start_date DESC, id DESC").Find(&agreements).Error; err != nil {
			return err
		}
		if len(agreements) == 0 {
			return errors.New("no active tenancy agreement covers target month")
		}
		if len(agreements) > 1 {
			return errors.New("multiple active tenancy agreements cover target month")
		}
		agreement := agreements[0]
		var parties []agreementParty
		if err := txdb.Where("user_id = ? AND agreement_id = ?", userID, agreement.ID).Order("tenant_id ASC, id ASC").Find(&parties).Error; err != nil {
			return err
		}
		plan, err := buildRentChargePlan(agreement, parties, periodMonth)
		if err != nil {
			return err
		}
		tenantIDs := make([]uint64, 0, len(plan.Responsibilities))
		for _, responsibility := range plan.Responsibilities {
			tenantIDs = append(tenantIDs, responsibility.TenantID)
		}
		var tenants []tenant
		if err := txdb.Where("user_id = ? AND id IN ?", userID, tenantIDs).Find(&tenants).Error; err != nil {
			return err
		}
		if len(tenants) != len(tenantIDs) {
			return errors.New("rent agreement contains a tenant outside current user")
		}

		propertyNameSnapshot := propertyRow.Name
		roomLabelSnapshot := roomRow.RoomLabel
		charge := rentCharge{
			UserID:               userID,
			PropertyID:           propertyID,
			RoomID:               roomID,
			TenancyAgreementID:   agreement.ID,
			PeriodMonth:          periodMonth,
			DueDate:              dueDateForMonth(periodMonth, agreement.DueDay),
			ExpectedAmountCents:  plan.ExpectedAmountCents,
			Currency:             plan.Currency,
			RecordStatus:         obligationRecordActive,
			PropertyNameSnapshot: &propertyNameSnapshot,
			RoomLabelSnapshot:    &roomLabelSnapshot,
			RoomAddressSnapshot:  propertyRow.Address,
		}
		createResult := txdb.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "user_id"}, {Name: "room_id"}, {Name: "period_month"}},
			DoNothing: true,
		}).Create(&charge)
		if createResult.Error != nil {
			return createResult.Error
		}
		if createResult.RowsAffected == 0 {
			if err := txdb.Where("user_id = ? AND room_id = ? AND period_month = ?", userID, roomID, periodMonth).First(&charge).Error; err != nil {
				return err
			}
			return loadRentChargeLedger(txdb, userID, charge, &result)
		}

		chargeID := charge.ID
		for _, responsibility := range plan.Responsibilities {
			obligation := rentObligation{
				UserID:              userID,
				RentChargeID:        &chargeID,
				TenantID:            responsibility.TenantID,
				PeriodMonth:         periodMonth,
				DueDate:             charge.DueDate,
				ExpectedAmountCents: responsibility.AmountCents,
				Currency:            plan.Currency,
				Status:              "open",
				RecordStatus:        obligationRecordActive,
				GeneratedBy:         "rent_charge",
			}
			var tenantRow tenant
			for _, candidate := range tenants {
				if candidate.ID == responsibility.TenantID {
					tenantRow = candidate
					break
				}
			}
			if tenantRow.Name != "" {
				snapshot := tenantRow.Name
				obligation.TenantNameSnapshot = &snapshot
			}
			if err := txdb.Create(&obligation).Error; err != nil {
				return err
			}
		}
		return loadRentChargeLedger(txdb, userID, charge, &result)
	})
	return result, err
}

func loadRentChargeLedger(db *gorm.DB, userID uint64, charge rentCharge, result *rentChargeLedger) error {
	var obligations []rentObligation
	if err := db.Where("user_id = ? AND rent_charge_id = ?", userID, charge.ID).Order("tenant_id ASC, id ASC").Find(&obligations).Error; err != nil {
		return err
	}
	if len(obligations) == 0 {
		return errors.New("rent charge has no obligations")
	}
	*result = rentChargeLedger{Charge: charge, Obligations: obligations}
	return nil
}
