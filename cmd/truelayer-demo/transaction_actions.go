package main

import (
	"context"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	transactionActionIgnore            = "ignore"
	transactionActionRestore           = "restore"
	transactionActionRevokeAllocations = "revoke_allocations"
)

type paymentTransactionAction struct {
	ID                   uint64 `gorm:"primaryKey"`
	UserID               uint64
	PaymentTransactionID uint64
	ActionKind           string
	Reason               string
	OperationID          string
	IdempotencyKey       *string
	ActedByUserID        uint64
	CreatedAt            time.Time
}

type transactionMatchProjection struct {
	Status          string
	MatchedTenantID *uint64
	Reason          string
}

func projectTransactionMatch(source paymentTransaction, allocations []paymentAllocation, actionKind, reason string) transactionMatchProjection {
	summary := summarizeTransactionAllocations(source, allocations)
	projection := transactionMatchProjection{Status: summary.Status, Reason: strings.TrimSpace(reason)}
	for _, allocation := range allocations {
		if !ledgerAllocationIsEffective(allocation) || allocation.TenantID == nil || *allocation.TenantID == 0 {
			continue
		}
		tenantID := *allocation.TenantID
		projection.MatchedTenantID = &tenantID
		break
	}
	if summary.AllocatedCents > 0 {
		projection.Reason = ""
		return projection
	}

	switch actionKind {
	case transactionActionIgnore:
		projection.Status = "ignored"
	case transactionActionRestore, transactionActionRevokeAllocations:
		projection.Status = "unmatched"
	case "":
		if source.MatchStatus == "candidate" || source.MatchStatus == "needs_review" {
			projection.Status = source.MatchStatus
			projection.Reason = source.MatchReason
		} else {
			projection.Status = "unmatched"
		}
	default:
		projection.Status = "unmatched"
	}
	projection.MatchedTenantID = nil
	return projection
}

func normalizeTransactionActionReason(value string) (string, error) {
	reason := strings.TrimSpace(value)
	if reason == "" {
		return "", errors.New("action reason is required")
	}
	runes := []rune(reason)
	if len(runes) > 512 {
		reason = string(runes[:512])
	}
	return reason, nil
}

func (s *transactionService) existingTransactionAction(txdb *gorm.DB, userID, transactionID uint64, actionKind, reason, idempotencyKey string) (bool, error) {
	key := strings.TrimSpace(idempotencyKey)
	if key == "" {
		return false, nil
	}
	var action paymentTransactionAction
	err := txdb.Where("user_id = ? AND idempotency_key = ?", userID, key).First(&action).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	if action.PaymentTransactionID != transactionID || action.ActionKind != actionKind || action.Reason != reason {
		return false, errors.New("idempotency key was already used for another transaction action")
	}
	return true, nil
}

func (s *transactionService) writeTransactionAction(txdb *gorm.DB, userID, transactionID uint64, actionKind, reason, idempotencyKey, operationID string, now time.Time) error {
	if strings.TrimSpace(operationID) == "" {
		operationID = recordID("transaction-action", now)
	}
	action := paymentTransactionAction{
		UserID:               userID,
		PaymentTransactionID: transactionID,
		ActionKind:           actionKind,
		Reason:               reason,
		OperationID:          operationID,
		IdempotencyKey:       nullableString(strings.TrimSpace(idempotencyKey)),
		ActedByUserID:        userID,
		CreatedAt:            now,
	}
	return txdb.Create(&action).Error
}

func (s *transactionService) ignoreTransaction(ctx context.Context, userID, transactionID uint64, reason, idempotencyKey string) error {
	if userID == 0 || transactionID == 0 {
		return errors.New("userID and transactionID are required")
	}
	reason, err := normalizeTransactionActionReason(reason)
	if err != nil {
		return err
	}
	return s.db.WithContext(ctx).Transaction(func(txdb *gorm.DB) error {
		var source paymentTransaction
		if err := txdb.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND user_id = ?", transactionID, userID).First(&source).Error; err != nil {
			return err
		}
		var allocations []paymentAllocation
		if err := txdb.Where("payment_transaction_id = ? AND user_id = ?", transactionID, userID).Find(&allocations).Error; err != nil {
			return err
		}
		alreadyDone, err := s.existingTransactionAction(txdb, userID, transactionID, transactionActionIgnore, reason, idempotencyKey)
		if err != nil {
			return err
		}
		if alreadyDone {
			return nil
		}
		if source.Direction != "income" {
			return errors.New("only income transactions can be ignored")
		}
		if summarizeTransactionAllocations(source, allocations).AllocatedCents > 0 {
			return errors.New("transaction has effective allocations; revoke allocations before ignoring")
		}
		now := time.Now().UTC()
		if err := s.writeTransactionAction(txdb, userID, transactionID, transactionActionIgnore, reason, idempotencyKey, "", now); err != nil {
			return err
		}
		projection := projectTransactionMatch(source, allocations, transactionActionIgnore, reason)
		return updateTransactionProjection(txdb, userID, transactionID, projection)
	})
}

func (s *transactionService) restoreTransaction(ctx context.Context, userID, transactionID uint64, reason, idempotencyKey string) error {
	if userID == 0 || transactionID == 0 {
		return errors.New("userID and transactionID are required")
	}
	reason, err := normalizeTransactionActionReason(reason)
	if err != nil {
		return err
	}
	return s.db.WithContext(ctx).Transaction(func(txdb *gorm.DB) error {
		var source paymentTransaction
		if err := txdb.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND user_id = ?", transactionID, userID).First(&source).Error; err != nil {
			return err
		}
		var allocations []paymentAllocation
		if err := txdb.Where("payment_transaction_id = ? AND user_id = ?", transactionID, userID).Find(&allocations).Error; err != nil {
			return err
		}
		alreadyDone, err := s.existingTransactionAction(txdb, userID, transactionID, transactionActionRestore, reason, idempotencyKey)
		if err != nil {
			return err
		}
		if alreadyDone {
			return nil
		}
		if source.Direction != "income" {
			return errors.New("only income transactions can be restored")
		}
		if summarizeTransactionAllocations(source, allocations).AllocatedCents > 0 {
			return errors.New("transaction has effective allocations; revoke allocations before restoring")
		}
		if source.MatchStatus != "ignored" {
			return errors.New("only ignored transactions can be restored")
		}
		now := time.Now().UTC()
		if err := s.writeTransactionAction(txdb, userID, transactionID, transactionActionRestore, reason, idempotencyKey, "", now); err != nil {
			return err
		}
		projection := projectTransactionMatch(source, allocations, transactionActionRestore, reason)
		return updateTransactionProjection(txdb, userID, transactionID, projection)
	})
}

func (s *transactionService) revokeTransactionAllocations(ctx context.Context, userID, transactionID uint64, reason, idempotencyKey string) (transactionAllocationSummary, error) {
	if userID == 0 || transactionID == 0 {
		return transactionAllocationSummary{}, errors.New("userID and transactionID are required")
	}
	reason, err := normalizeTransactionActionReason(reason)
	if err != nil {
		return transactionAllocationSummary{}, err
	}
	var summary transactionAllocationSummary
	err = s.db.WithContext(ctx).Transaction(func(txdb *gorm.DB) error {
		var source paymentTransaction
		if err := txdb.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND user_id = ?", transactionID, userID).First(&source).Error; err != nil {
			return err
		}
		var allocations []paymentAllocation
		if err := txdb.Where("payment_transaction_id = ? AND user_id = ?", transactionID, userID).Order("id ASC").Find(&allocations).Error; err != nil {
			return err
		}
		alreadyDone, err := s.existingTransactionAction(txdb, userID, transactionID, transactionActionRevokeAllocations, reason, idempotencyKey)
		if err != nil {
			return err
		}
		if alreadyDone {
			summary = summarizeTransactionAllocations(source, allocations)
			return nil
		}
		if summarizeTransactionAllocations(source, allocations).AllocatedCents == 0 {
			return errors.New("transaction has no effective allocations to revoke")
		}

		now := time.Now().UTC()
		operationID := recordID("transaction-revoke", now)
		obligationIDs := make(map[uint64]struct{})
		for _, allocation := range allocations {
			if !ledgerAllocationIsEffective(allocation) {
				continue
			}
			if allocation.RentObligationID != nil && *allocation.RentObligationID != 0 && ledgerAllocationKind(allocation) == allocationKindRent {
				obligationIDs[*allocation.RentObligationID] = struct{}{}
			}
			if err := txdb.Model(&paymentAllocation{}).Where("id = ? AND user_id = ? AND payment_transaction_id = ? AND status = ?", allocation.ID, userID, transactionID, allocationStatusConfirmed).Updates(map[string]any{
				"status":            allocationStatusVoided,
				"operation_id":      operationID,
				"voided_at":         now,
				"voided_by_user_id": userID,
				"void_reason":       reason,
			}).Error; err != nil {
				return err
			}
		}

		for obligationID := range obligationIDs {
			var obligation rentObligation
			if err := txdb.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND user_id = ?", obligationID, userID).First(&obligation).Error; err != nil {
				return err
			}
			var current []paymentAllocation
			if err := txdb.Where("rent_obligation_id = ? AND user_id = ?", obligationID, userID).Find(&current).Error; err != nil {
				return err
			}
			var cashReceipts []cashReceipt
			if err := txdb.Where("rent_obligation_id = ? AND user_id = ?", obligationID, userID).Find(&cashReceipts).Error; err != nil {
				return err
			}
			projected := projectRentObligation(obligation, current, cashReceipts, now)
			if err := txdb.Model(&rentObligation{}).Where("id = ? AND user_id = ?", obligationID, userID).Updates(map[string]any{
				"paid_amount_cents": projected.PaidAmountCents,
				"status":            projected.Status,
			}).Error; err != nil {
				return err
			}
		}

		if err := s.writeTransactionAction(txdb, userID, transactionID, transactionActionRevokeAllocations, reason, idempotencyKey, operationID, now); err != nil {
			return err
		}
		// The in-memory allocations were read before the void update above, so they
		// still read as effective. Project from the persisted rows instead.
		allocationsAfter := make([]paymentAllocation, 0, len(allocations))
		if err := txdb.Where("payment_transaction_id = ? AND user_id = ?", transactionID, userID).Order("id ASC").Find(&allocationsAfter).Error; err != nil {
			return err
		}
		projection := projectTransactionMatch(source, allocationsAfter, transactionActionRevokeAllocations, reason)
		if err := updateTransactionProjection(txdb, userID, transactionID, projection); err != nil {
			return err
		}
		summary = summarizeTransactionAllocations(source, allocationsAfter)
		return nil
	})
	return summary, err
}

func updateTransactionProjection(txdb *gorm.DB, userID, transactionID uint64, projection transactionMatchProjection) error {
	updates := map[string]any{
		"match_status":      projection.Status,
		"matched_tenant_id": nil,
		"match_reason":      nullableString(projection.Reason),
	}
	if projection.MatchedTenantID != nil {
		updates["matched_tenant_id"] = *projection.MatchedTenantID
	}
	return txdb.Model(&paymentTransaction{}).Where("id = ? AND user_id = ?", transactionID, userID).Updates(updates).Error
}
