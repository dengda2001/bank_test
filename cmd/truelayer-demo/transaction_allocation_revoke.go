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

const transactionActionRevokeAllocation = "revoke_allocation"

var ErrRentAllocationNotRevocable = errors.New("rent allocation is no longer available to revoke")

// revokeRentAllocation voids exactly one confirmed rent share. The source lock
// serializes this operation with allocation and whole-source correction writes.
func (s *transactionService) revokeRentAllocation(ctx context.Context, userID, allocationID, expectedTransactionID uint64, idempotencyKey string) (transactionAllocationSummary, error) {
	if userID == 0 || allocationID == 0 {
		return transactionAllocationSummary{}, ErrRentAllocationNotRevocable
	}
	var summary transactionAllocationSummary
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var lookup paymentAllocation
		if err := tx.Where("id = ? AND user_id = ?", allocationID, userID).First(&lookup).Error; err != nil {
			return ErrRentAllocationNotRevocable
		}
		if expectedTransactionID != 0 && lookup.PaymentTransactionID != expectedTransactionID {
			return ErrRentAllocationNotRevocable
		}
		var source paymentTransaction
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND user_id = ?", lookup.PaymentTransactionID, userID).First(&source).Error; err != nil {
			return ErrRentAllocationNotRevocable
		}
		var allocation paymentAllocation
		if err := tx.Where("id = ? AND user_id = ? AND payment_transaction_id = ?", allocationID, userID, source.ID).First(&allocation).Error; err != nil {
			return ErrRentAllocationNotRevocable
		}
		reason := fmt.Sprintf("撤销租金分配 #%d", allocationID)
		alreadyDone, err := s.existingTransactionAction(tx, userID, source.ID, transactionActionRevokeAllocation, reason, idempotencyKey)
		if err != nil {
			return err
		}
		if alreadyDone {
			var rows []paymentAllocation
			if err := tx.Where("user_id = ? AND payment_transaction_id = ?", userID, source.ID).Find(&rows).Error; err != nil {
				return err
			}
			summary = summarizeTransactionAllocations(source, rows)
			return nil
		}
		if source.Direction != "income" || !ledgerAllocationIsEffective(allocation) || ledgerAllocationKind(allocation) != allocationKindRent || allocation.RentObligationID == nil || allocation.TenantID == nil || allocation.AmountCents <= 0 {
			return ErrRentAllocationNotRevocable
		}
		locked, err := lockRentObligationRoomsInTx(tx, userID, []uint64{*allocation.RentObligationID})
		if err != nil {
			return err
		}
		obligation, ok := locked[*allocation.RentObligationID]
		if !ok || obligation.TenantID != *allocation.TenantID {
			return ErrRentFactsConflict
		}
		now := time.Now().UTC()
		operationID := recordID("allocation-revoke", now)
		result := tx.Model(&paymentAllocation{}).Where("id = ? AND user_id = ? AND payment_transaction_id = ? AND rent_obligation_id = ? AND status = ? AND voided_at IS NULL", allocationID, userID, source.ID, obligation.ID, allocationStatusConfirmed).Updates(map[string]any{
			"status": allocationStatusVoided, "operation_id": operationID,
			"voided_at": now, "voided_by_user_id": userID, "void_reason": reason,
		})
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return ErrRentAllocationNotRevocable
		}
		var obligationAllocations []paymentAllocation
		if err := tx.Where("user_id = ? AND rent_obligation_id = ?", userID, obligation.ID).Find(&obligationAllocations).Error; err != nil {
			return err
		}
		var cashReceipts []cashReceipt
		if err := tx.Where("user_id = ? AND rent_obligation_id = ?", userID, obligation.ID).Find(&cashReceipts).Error; err != nil {
			return err
		}
		projectedObligation := projectRentObligation(obligation, obligationAllocations, cashReceipts, now)
		if err := tx.Model(&rentObligation{}).Where("id = ? AND user_id = ?", obligation.ID, userID).Updates(map[string]any{
			"paid_amount_cents": projectedObligation.PaidAmountCents, "status": projectedObligation.Status,
		}).Error; err != nil {
			return err
		}
		if err := s.writeTransactionAction(tx, userID, source.ID, transactionActionRevokeAllocation, reason, strings.TrimSpace(idempotencyKey), operationID, now); err != nil {
			return err
		}
		var sourceAllocations []paymentAllocation
		if err := tx.Where("user_id = ? AND payment_transaction_id = ?", userID, source.ID).Order("id ASC").Find(&sourceAllocations).Error; err != nil {
			return err
		}
		projection := projectTransactionMatch(source, sourceAllocations, transactionActionRevokeAllocation, "已撤销一份租金分配，可重新匹配")
		if err := updateTransactionProjection(tx, userID, source.ID, projection); err != nil {
			return err
		}
		summary = summarizeTransactionAllocations(source, sourceAllocations)
		return nil
	})
	return summary, err
}
