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

type transactionAllocationDraft struct {
	TenantID         uint64
	RentObligationID uint64
	AmountCents      int64
	Kind             string
	Note             string
}

type transactionAllocationSummary struct {
	AllocatedCents int64
	RemainingCents int64
	Status         string
	KindCents      map[string]int64
}

func summarizeTransactionAllocations(source paymentTransaction, allocations []paymentAllocation) transactionAllocationSummary {
	summary := transactionAllocationSummary{KindCents: make(map[string]int64)}
	for _, allocation := range allocations {
		if !ledgerAllocationIsEffective(allocation) || allocation.AmountCents <= 0 {
			continue
		}
		kind := ledgerAllocationKind(allocation)
		summary.AllocatedCents += allocation.AmountCents
		summary.KindCents[kind] += allocation.AmountCents
	}
	summary.RemainingCents = source.AmountCents - summary.AllocatedCents
	if summary.RemainingCents < 0 {
		summary.RemainingCents = 0
	}
	switch {
	case summary.AllocatedCents == 0:
		summary.Status = "unmatched"
	case summary.RemainingCents > 0:
		summary.Status = "partial"
	default:
		summary.Status = "matched"
	}
	return summary
}

func validateTransactionAllocationDrafts(source paymentTransaction, existing []paymentAllocation, drafts []transactionAllocationDraft, obligations map[uint64]rentObligation) error {
	if source.UserID == 0 {
		return errors.New("source user is required")
	}
	if source.Direction != "income" {
		return errors.New("only income transactions can be allocated")
	}
	if source.AmountCents <= 0 {
		return errors.New("source amount must be positive")
	}
	if len(drafts) == 0 {
		return errors.New("at least one allocation is required")
	}

	summary := summarizeTransactionAllocations(source, existing)
	if summary.AllocatedCents > source.AmountCents {
		return errors.New("existing allocations exceed source amount")
	}
	for _, allocation := range existing {
		if !ledgerAllocationIsEffective(allocation) {
			continue
		}
		if allocation.UserID != 0 && allocation.UserID != source.UserID {
			return errors.New("existing allocation user ownership mismatch")
		}
	}

	rentDraftCents := make(map[uint64]int64)
	for index, draft := range drafts {
		if draft.AmountCents <= 0 {
			return fmt.Errorf("allocation %d amount must be positive", index+1)
		}
		if draft.Kind == allocationKindOther && len([]rune(draft.Note)) == 0 {
			return fmt.Errorf("allocation %d other income note is required", index+1)
		}
		if draft.Kind == allocationKindRent || draft.Kind == allocationKindDeposit {
			if draft.TenantID == 0 {
				return fmt.Errorf("allocation %d tenant is required", index+1)
			}
		}
		check := ledgerAllocationCheck{
			UserID:                 source.UserID,
			SourceUserID:           source.UserID,
			TenantID:               draft.TenantID,
			SourceAmountCents:      source.AmountCents,
			ExistingAllocatedCents: summary.AllocatedCents,
			AmountCents:            draft.AmountCents,
			SourceCurrency:         source.Currency,
			Kind:                   draft.Kind,
		}
		if draft.Kind == allocationKindRent {
			obligation, ok := obligations[draft.RentObligationID]
			if !ok {
				return fmt.Errorf("allocation %d rent obligation is not available", index+1)
			}
			if obligation.RecordStatus == obligationRecordVoided {
				return fmt.Errorf("allocation %d rent obligation is voided", index+1)
			}
			check.ObligationTenantID = obligation.TenantID
			check.ObligationExpectedCents = obligation.ExpectedAmountCents
			check.ObligationPaidCents = obligation.PaidAmountCents + rentDraftCents[obligation.ID]
			check.ObligationCurrency = obligation.Currency
		}
		if err := validateLedgerAllocation(check); err != nil {
			return fmt.Errorf("allocation %d: %w", index+1, err)
		}
		summary.AllocatedCents += draft.AmountCents
		if draft.Kind == allocationKindRent {
			rentDraftCents[draft.RentObligationID] += draft.AmountCents
		}
	}
	return nil
}

func (s *transactionService) allocateTransaction(ctx context.Context, userID, transactionID uint64, drafts []transactionAllocationDraft, idempotencyKey, confirmationSource string) (transactionAllocationSummary, error) {
	if userID == 0 || transactionID == 0 {
		return transactionAllocationSummary{}, errors.New("userID and transactionID are required")
	}
	var summary transactionAllocationSummary
	err := s.db.WithContext(ctx).Transaction(func(txdb *gorm.DB) error {
		var allocationErr error
		summary, allocationErr = s.allocateTransactionInTx(txdb, userID, transactionID, drafts, idempotencyKey, confirmationSource)
		return allocationErr
	})
	return summary, err
}

// allocateTransactionInTx writes allocations and their ledger projections into
// an existing transaction. Callers that create a new payment transaction as
// part of the same business action use this to prevent an orphaned receipt.
func (s *transactionService) allocateTransactionInTx(txdb *gorm.DB, userID, transactionID uint64, drafts []transactionAllocationDraft, idempotencyKey, confirmationSource string) (transactionAllocationSummary, error) {
	if userID == 0 || transactionID == 0 {
		return transactionAllocationSummary{}, errors.New("userID and transactionID are required")
	}
	if strings.TrimSpace(confirmationSource) == "" {
		confirmationSource = "manual"
	}
	var source paymentTransaction
	if err := txdb.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND user_id = ?", transactionID, userID).First(&source).Error; err != nil {
		return transactionAllocationSummary{}, err
	}
	var existing []paymentAllocation
	if err := txdb.Where("payment_transaction_id = ? AND user_id = ?", transactionID, userID).Order("id ASC").Find(&existing).Error; err != nil {
		return transactionAllocationSummary{}, err
	}
	if idempotencyKey != "" {
		var previous paymentAllocation
		firstKey := allocationRequestKey(idempotencyKey, transactionID, 0)
		err := txdb.Where("user_id = ? AND idempotency_key = ?", userID, firstKey).First(&previous).Error
		if err == nil {
			return summarizeTransactionAllocations(source, existing), nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return transactionAllocationSummary{}, err
		}
	}
	if source.MatchStatus == "ignored" {
		return transactionAllocationSummary{}, errors.New("restore ignored transaction before allocating it")
	}

	obligations := make(map[uint64]rentObligation)
	now := time.Now().UTC()
	for _, draft := range drafts {
		if draft.Kind != allocationKindRent || draft.RentObligationID == 0 {
			continue
		}
		if _, ok := obligations[draft.RentObligationID]; ok {
			continue
		}
		var obligation rentObligation
		if err := txdb.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND user_id = ?", draft.RentObligationID, userID).First(&obligation).Error; err != nil {
			return transactionAllocationSummary{}, err
		}
		var obligationAllocations []paymentAllocation
		if err := txdb.Where("rent_obligation_id = ? AND user_id = ?", obligation.ID, userID).Find(&obligationAllocations).Error; err != nil {
			return transactionAllocationSummary{}, err
		}
		var cashReceipts []cashReceipt
		if err := txdb.Where("rent_obligation_id = ? AND user_id = ?", obligation.ID, userID).Find(&cashReceipts).Error; err != nil {
			return transactionAllocationSummary{}, err
		}
		obligations[obligation.ID] = projectRentObligation(obligation, obligationAllocations, cashReceipts, now)
	}
	if err := validateTransactionAllocationDrafts(source, existing, drafts, obligations); err != nil {
		return transactionAllocationSummary{}, err
	}

	operationID := recordID("allocation", now)
	for index, draft := range drafts {
		var rentObligationID *uint64
		if draft.Kind == allocationKindRent {
			rentObligationID = &draft.RentObligationID
		}
		var tenantID *uint64
		if draft.TenantID != 0 {
			tenantID = &draft.TenantID
		}
		allocation := paymentAllocation{
			UserID:               userID,
			PaymentTransactionID: transactionID,
			RentObligationID:     rentObligationID,
			TenantID:             tenantID,
			AmountCents:          draft.AmountCents,
			AllocationKind:       draft.Kind,
			Note:                 strings.TrimSpace(draft.Note),
			Status:               allocationStatusConfirmed,
			OperationID:          &operationID,
			IdempotencyKey:       nullableString(allocationRequestKey(idempotencyKey, transactionID, index)),
			ConfirmedByUserID:    userID,
			ConfirmedAt:          now,
			ConfirmationSource:   confirmationSource,
		}
		if err := txdb.Create(&allocation).Error; err != nil {
			return transactionAllocationSummary{}, err
		}
		existing = append(existing, allocation)
	}

	for obligationID, obligation := range obligations {
		var allocations []paymentAllocation
		if err := txdb.Where("rent_obligation_id = ? AND user_id = ?", obligationID, userID).Find(&allocations).Error; err != nil {
			return transactionAllocationSummary{}, err
		}
		var cashReceipts []cashReceipt
		if err := txdb.Where("rent_obligation_id = ? AND user_id = ?", obligationID, userID).Find(&cashReceipts).Error; err != nil {
			return transactionAllocationSummary{}, err
		}
		projected := projectRentObligation(obligation, allocations, cashReceipts, now)
		if err := txdb.Model(&rentObligation{}).Where("id = ? AND user_id = ?", obligationID, userID).Updates(map[string]any{
			"paid_amount_cents": projected.PaidAmountCents,
			"status":            projected.Status,
		}).Error; err != nil {
			return transactionAllocationSummary{}, err
		}
	}

	summary := summarizeTransactionAllocations(source, existing)
	var matchedTenantID uint64
	for _, allocation := range existing {
		if !ledgerAllocationIsEffective(allocation) || allocation.TenantID == nil || *allocation.TenantID == 0 {
			continue
		}
		matchedTenantID = *allocation.TenantID
		break
	}
	projection := projectTransactionMatch(source, existing, "", "")
	if matchedTenantID != 0 && projection.MatchedTenantID == nil {
		projection.MatchedTenantID = &matchedTenantID
	}
	if err := updateTransactionProjection(txdb, userID, transactionID, projection); err != nil {
		return transactionAllocationSummary{}, err
	}
	return summary, nil
}

func allocationRequestKey(requestKey string, transactionID uint64, index int) string {
	if strings.TrimSpace(requestKey) == "" {
		return ""
	}
	return fmt.Sprintf("%s:%d:%d", strings.TrimSpace(requestKey), transactionID, index)
}
