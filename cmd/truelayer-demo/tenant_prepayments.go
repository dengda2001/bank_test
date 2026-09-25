package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type tenantPrepayment struct {
	ID                   uint64 `gorm:"primaryKey"`
	UserID               uint64
	TenantID             uint64
	PaymentTransactionID uint64
	OriginalAmountCents  int64
	Currency             string
	OperationID          string
	CreatedAt            time.Time
}

func (tenantPrepayment) TableName() string { return "tenant_prepayments" }

type tenantPrepaymentRow struct {
	ID             uint64
	Amount         string
	Available      string
	AvailableCents int64
	AvailableInput string
	State          string
	Source         string
	SourceURL      string
	RequestKey     string
}

func (s *transactionService) listTenantPrepayments(ctx context.Context, userID, tenantID uint64) ([]tenantPrepaymentRow, error) {
	var credits []tenantPrepayment
	if err := s.db.WithContext(ctx).Where("user_id = ? AND tenant_id = ?", userID, tenantID).Order("id DESC").Find(&credits).Error; err != nil {
		return nil, err
	}
	rows := make([]tenantPrepaymentRow, 0, len(credits))
	for _, credit := range credits {
		var allocations []paymentAllocation
		if err := s.db.WithContext(ctx).Where("user_id = ? AND prepayment_id = ?", userID, credit.ID).Find(&allocations).Error; err != nil {
			return nil, err
		}
		var source paymentTransaction
		if err := s.db.WithContext(ctx).Where("user_id = ? AND id = ?", userID, credit.PaymentTransactionID).First(&source).Error; err != nil {
			return nil, err
		}
		available := prepaymentAvailableCents(allocations, credit.ID)
		state := "待分配"
		if available == 0 {
			state = "已撤销"
			for _, allocation := range allocations {
				if ledgerAllocationIsEffective(allocation) && ledgerAllocationKind(allocation) == allocationKindRent {
					state = "已全部用于租金"
					break
				}
			}
		}
		rows = append(rows, tenantPrepaymentRow{
			ID: credit.ID, Amount: formatMoney(centsToMoney(credit.OriginalAmountCents), credit.Currency, 2),
			Available: formatMoney(centsToMoney(available), credit.Currency, 2), AvailableCents: available,
			AvailableInput: fmt.Sprintf("%.2f", centsToMoney(available)),
			State:          state,
			Source:         firstNonEmpty(source.Reference, source.StableTransactionKey),
			SourceURL:      fmt.Sprintf("/transactions?detail=%d", source.ID),
			RequestKey:     fmt.Sprintf("%s:%d", recordID("credit-form", time.Now().UTC()), credit.ID),
		})
	}
	return rows, nil
}

func prepaymentAvailableCents(rows []paymentAllocation, creditID uint64) int64 {
	var available int64
	for _, row := range rows {
		if row.PrepaymentID != nil && *row.PrepaymentID == creditID && ledgerAllocationIsEffective(row) && ledgerAllocationKind(row) == allocationKindPrepayment {
			available += row.AmountCents
		}
	}
	return available
}

var ErrPrepaymentUnavailable = errors.New("prepayment is unavailable for this rent bill")

// applyTenantPrepayment reclassifies bank-backed credit into rent without
// changing the source transaction total. The source lock serializes uses and
// reversals of credit originating from the same bank receipt.
func (s *transactionService) applyTenantPrepayment(ctx context.Context, userID, creditID, obligationID uint64, amountCents int64, requestKey string) (transactionAllocationSummary, error) {
	requestKey = strings.TrimSpace(requestKey)
	if userID == 0 || creditID == 0 || obligationID == 0 || amountCents <= 0 || requestKey == "" || len(requestKey) > 100 {
		return transactionAllocationSummary{}, ErrPrepaymentUnavailable
	}
	var summary transactionAllocationSummary
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var credit tenantPrepayment
		if err := tx.Where("id = ? AND user_id = ?", creditID, userID).First(&credit).Error; err != nil {
			return ErrPrepaymentUnavailable
		}
		var source paymentTransaction
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND user_id = ?", credit.PaymentTransactionID, userID).First(&source).Error; err != nil {
			return ErrPrepaymentUnavailable
		}
		if source.Direction != "income" || !strings.EqualFold(source.Currency, credit.Currency) {
			return ErrPrepaymentUnavailable
		}
		key := "prepayment-use:" + requestKey
		var previous paymentAllocation
		lookup := tx.Where("user_id = ? AND idempotency_key = ?", userID, key).First(&previous).Error
		if lookup == nil {
			if previous.PrepaymentID == nil || *previous.PrepaymentID != creditID || previous.RentObligationID == nil || *previous.RentObligationID != obligationID || previous.AmountCents != amountCents || ledgerAllocationKind(previous) != allocationKindRent {
				return errors.New("prepayment request key was reused for a different application")
			}
			var all []paymentAllocation
			if err := tx.Where("user_id = ? AND payment_transaction_id = ?", userID, source.ID).Find(&all).Error; err != nil {
				return err
			}
			summary = summarizeTransactionAllocations(source, all)
			return nil
		}
		if !errors.Is(lookup, gorm.ErrRecordNotFound) {
			return lookup
		}
		locked, err := lockRentObligationRoomsInTx(tx, userID, []uint64{obligationID})
		if err != nil {
			return err
		}
		obligation, ok := locked[obligationID]
		if !ok || obligation.TenantID != credit.TenantID || obligation.RecordStatus == obligationRecordVoided || !strings.EqualFold(obligation.Currency, credit.Currency) {
			return ErrPrepaymentUnavailable
		}
		var creditRows []paymentAllocation
		if err := tx.Where("user_id = ? AND prepayment_id = ?", userID, creditID).Order("id ASC").Find(&creditRows).Error; err != nil {
			return err
		}
		available := prepaymentAvailableCents(creditRows, creditID)
		if amountCents > available || available <= 0 {
			return ErrPrepaymentUnavailable
		}
		var obligationAllocations []paymentAllocation
		if err := tx.Where("user_id = ? AND rent_obligation_id = ?", userID, obligationID).Find(&obligationAllocations).Error; err != nil {
			return err
		}
		var cashReceipts []cashReceipt
		if err := tx.Where("user_id = ? AND rent_obligation_id = ?", userID, obligationID).Find(&cashReceipts).Error; err != nil {
			return err
		}
		now := time.Now().UTC()
		projectedBefore := projectRentObligation(obligation, obligationAllocations, cashReceipts, now)
		if amountCents > obligation.ExpectedAmountCents-projectedBefore.PaidAmountCents {
			return ErrPrepaymentUnavailable
		}
		operationID := recordID("prepayment-use", now)
		for _, row := range creditRows {
			if !ledgerAllocationIsEffective(row) || ledgerAllocationKind(row) != allocationKindPrepayment {
				continue
			}
			result := tx.Model(&paymentAllocation{}).Where("id = ? AND user_id = ? AND status = ?", row.ID, userID, allocationStatusConfirmed).Updates(map[string]any{
				"status": allocationStatusVoided, "voided_at": now, "voided_by_user_id": userID, "void_reason": "手动用于租金",
			})
			if result.Error != nil {
				return result.Error
			}
			if result.RowsAffected != 1 {
				return ErrPrepaymentUnavailable
			}
		}
		newRent := paymentAllocation{
			UserID: userID, PaymentTransactionID: source.ID, RentObligationID: &obligationID, PrepaymentID: &creditID,
			TenantID: &credit.TenantID, AmountCents: amountCents, AllocationKind: allocationKindRent,
			Status: allocationStatusConfirmed, OperationID: &operationID, IdempotencyKey: &key,
			ConfirmedByUserID: userID, ConfirmedAt: now, ConfirmationSource: "manual_prepayment",
		}
		if err := tx.Create(&newRent).Error; err != nil {
			return err
		}
		if residue := available - amountCents; residue > 0 {
			residueKey := fmt.Sprintf("%s:remainder", key)
			newCredit := paymentAllocation{
				UserID: userID, PaymentTransactionID: source.ID, PrepaymentID: &creditID,
				TenantID: &credit.TenantID, AmountCents: residue, AllocationKind: allocationKindPrepayment,
				Status: allocationStatusConfirmed, OperationID: &operationID, IdempotencyKey: &residueKey,
				ConfirmedByUserID: userID, ConfirmedAt: now, ConfirmationSource: "manual_prepayment",
			}
			if err := tx.Create(&newCredit).Error; err != nil {
				return err
			}
		}
		obligationAllocations = append(obligationAllocations, newRent)
		projectedAfter := projectRentObligation(obligation, obligationAllocations, cashReceipts, now)
		if err := tx.Model(&rentObligation{}).Where("id = ? AND user_id = ?", obligationID, userID).Updates(map[string]any{"paid_amount_cents": projectedAfter.PaidAmountCents, "status": projectedAfter.Status}).Error; err != nil {
			return err
		}
		var all []paymentAllocation
		if err := tx.Where("user_id = ? AND payment_transaction_id = ?", userID, source.ID).Order("id ASC").Find(&all).Error; err != nil {
			return err
		}
		summary = summarizeTransactionAllocations(source, all)
		if summary.AllocatedCents > source.AmountCents {
			return ErrPrepaymentUnavailable
		}
		return updateTransactionProjection(tx, userID, source.ID, projectTransactionMatch(source, all, "", ""))
	})
	return summary, err
}
