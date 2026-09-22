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
	RoomRentPlanMemberID uint64
	TenantID             uint64
	AmountCents          int64
}

type rentChargePlan struct {
	ExpectedAmountCents int64
	Currency            string
	Responsibilities    []rentResponsibilityInput
}

func buildRentChargePlan(plan roomRentPlan, members []roomRentPlanMember, periodMonth time.Time) (rentChargePlan, error) {
	if !roomRentPlanCoversMonth(plan, periodMonth) {
		return rentChargePlan{}, errors.New("room rent plan does not cover target month")
	}
	currency, err := normalizeLedgerCurrency(plan.Currency)
	if err != nil {
		return rentChargePlan{}, err
	}
	responsibilities := make([]rentResponsibilityInput, 0, len(members))
	for _, member := range members {
		responsibilities = append(responsibilities, rentResponsibilityInput{
			RoomRentPlanMemberID: member.ID,
			TenantID:             member.TenantID,
			AmountCents:          member.ResponsibilityCents,
		})
	}
	if err := validateRentResponsibilityPlan(plan.MonthlyRentCents, responsibilities); err != nil {
		return rentChargePlan{}, err
	}
	return rentChargePlan{
		ExpectedAmountCents: plan.MonthlyRentCents,
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

func roomRentPlanCoversMonth(row roomRentPlan, periodMonth time.Time) bool {
	start := monthStart(periodMonth)
	if row.EffectiveFromMonth.IsZero() || monthStart(row.EffectiveFromMonth).After(start) {
		return false
	}
	if row.EffectiveToMonth != nil && monthStart(*row.EffectiveToMonth).Before(start) {
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
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var roomRow room
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_id = ? AND id = ?", userID, roomID).First(&roomRow).Error; err != nil {
			return err
		}
		if roomRow.PropertyID != propertyID {
			return gorm.ErrRecordNotFound
		}
		var propertyRow property
		if err := tx.Where("user_id = ? AND id = ?", userID, propertyID).First(&propertyRow).Error; err != nil {
			return err
		}

		var charge rentCharge
		lookupErr := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_id = ? AND room_id = ? AND period_month = ?", userID, roomID, periodMonth).First(&charge).Error
		if lookupErr == nil {
			return loadRentChargeLedger(tx, userID, charge, &result)
		}
		if !errors.Is(lookupErr, gorm.ErrRecordNotFound) {
			return lookupErr
		}

		var plans []roomRentPlan
		if err := tx.Where("user_id = ? AND room_id = ? AND effective_from_month <= ? AND (effective_to_month IS NULL OR effective_to_month >= ?)", userID, roomID, periodMonth, periodMonth).
			Order("effective_from_month DESC, id DESC").Find(&plans).Error; err != nil {
			return err
		}
		if len(plans) == 0 {
			return gorm.ErrRecordNotFound
		}
		if len(plans) != 1 {
			return ErrRentPlanTimelineConflict
		}
		planRow := plans[0]
		var members []roomRentPlanMember
		if err := tx.Where("user_id = ? AND room_rent_plan_id = ?", userID, planRow.ID).Order("tenant_id ASC, id ASC").Find(&members).Error; err != nil {
			return err
		}
		if len(members) == 0 {
			return gorm.ErrRecordNotFound
		}
		plan, err := buildRentChargePlan(planRow, members, periodMonth)
		if err != nil {
			return err
		}
		tenantIDs := make([]uint64, 0, len(plan.Responsibilities))
		for _, responsibility := range plan.Responsibilities {
			tenantIDs = append(tenantIDs, responsibility.TenantID)
		}
		var tenants []tenant
		if err := tx.Where("user_id = ? AND id IN ?", userID, tenantIDs).Find(&tenants).Error; err != nil {
			return err
		}
		if len(tenants) != len(tenantIDs) {
			return errors.New("room rent plan contains a tenant outside current user")
		}

		propertyNameSnapshot := propertyRow.Name
		roomLabelSnapshot := roomRow.RoomLabel
		charge = rentCharge{
			UserID:               userID,
			PropertyID:           propertyID,
			RoomID:               roomID,
			RoomRentPlanID:       planRow.ID,
			PeriodMonth:          periodMonth,
			DueDate:              dueDateForMonth(periodMonth, planRow.DueDay),
			ExpectedAmountCents:  plan.ExpectedAmountCents,
			Currency:             plan.Currency,
			RecordStatus:         obligationRecordActive,
			PropertyNameSnapshot: &propertyNameSnapshot,
			RoomLabelSnapshot:    &roomLabelSnapshot,
			RoomAddressSnapshot:  propertyRow.Address,
		}
		if err := tx.Create(&charge).Error; err != nil {
			return err
		}
		for _, responsibility := range plan.Responsibilities {
			obligation := rentObligation{
				UserID:               userID,
				RentChargeID:         charge.ID,
				RoomRentPlanID:       planRow.ID,
				RoomRentPlanMemberID: responsibility.RoomRentPlanMemberID,
				TenantID:             responsibility.TenantID,
				PeriodMonth:          periodMonth,
				DueDate:              charge.DueDate,
				ExpectedAmountCents:  responsibility.AmountCents,
				Currency:             plan.Currency,
				Status:               "open",
				RecordStatus:         obligationRecordActive,
			}
			for _, tenantRow := range tenants {
				if tenantRow.ID == responsibility.TenantID {
					snapshot := tenantRow.Name
					obligation.TenantNameSnapshot = &snapshot
					break
				}
			}
			if err := tx.Create(&obligation).Error; err != nil {
				return err
			}
		}
		return loadRentChargeLedger(tx, userID, charge, &result)
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
