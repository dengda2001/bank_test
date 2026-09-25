package main

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const (
	transactionActionIgnore            = "ignore"
	transactionActionRestore           = "restore"
	transactionActionDefer             = "defer"
	transactionActionUndefer           = "undefer"
	transactionActionRevokeAllocations = "revoke_allocations"
)

var ErrTransactionNotDeferrable = errors.New("only pending income transactions can be deferred")

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
	if source.MatchedTenantID != nil && *source.MatchedTenantID != 0 {
		matchedTenantID := *source.MatchedTenantID
		projection.MatchedTenantID = &matchedTenantID
	}
	for _, allocation := range allocations {
		if !ledgerAllocationIsEffective(allocation) || allocation.TenantID == nil || *allocation.TenantID == 0 {
			continue
		}
		if projection.MatchedTenantID == nil {
			tenantID := *allocation.TenantID
			projection.MatchedTenantID = &tenantID
		}
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

func (s *transactionService) deferTransaction(ctx context.Context, userID, transactionID uint64, reason string) error {
	return s.setTransactionDeferred(ctx, userID, transactionID, reason, true)
}

func (s *transactionService) undeferTransaction(ctx context.Context, userID, transactionID uint64, reason string) error {
	return s.setTransactionDeferred(ctx, userID, transactionID, reason, false)
}

func transactionDeferredState(ctx context.Context, db *gorm.DB, userID, transactionID uint64) (bool, error) {
	var latest paymentTransactionAction
	err := db.WithContext(ctx).
		Where("user_id = ? AND payment_transaction_id = ? AND action_kind IN ?", userID, transactionID, []string{transactionActionDefer, transactionActionUndefer}).
		Order("id DESC").First(&latest).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	return latest.ActionKind == transactionActionDefer, nil
}

func transactionDeferredStates(ctx context.Context, db *gorm.DB, userID uint64, transactionIDs []uint64) (map[uint64]bool, error) {
	states := make(map[uint64]bool, len(transactionIDs))
	if len(transactionIDs) == 0 {
		return states, nil
	}
	var actions []paymentTransactionAction
	if err := db.WithContext(ctx).Where("user_id = ? AND payment_transaction_id IN ? AND action_kind IN ?", userID, transactionIDs, []string{transactionActionDefer, transactionActionUndefer}).Order("id DESC").Find(&actions).Error; err != nil {
		return nil, err
	}
	seen := make(map[uint64]bool, len(transactionIDs))
	for _, action := range actions {
		if seen[action.PaymentTransactionID] {
			continue
		}
		seen[action.PaymentTransactionID] = true
		states[action.PaymentTransactionID] = action.ActionKind == transactionActionDefer
	}
	return states, nil
}

func (s *transactionService) clearTransactionDeferral(txdb *gorm.DB, userID, transactionID uint64, now time.Time) error {
	var latest paymentTransactionAction
	err := txdb.Where("user_id = ? AND payment_transaction_id = ? AND action_kind IN ?", userID, transactionID, []string{transactionActionDefer, transactionActionUndefer}).Order("id DESC").First(&latest).Error
	if errors.Is(err, gorm.ErrRecordNotFound) || (err == nil && latest.ActionKind == transactionActionUndefer) {
		return nil
	}
	if err != nil {
		return err
	}
	return s.writeTransactionAction(txdb, userID, transactionID, transactionActionUndefer, "已匹配，恢复待处理状态", "", "", now)
}

func (s *transactionService) setTransactionDeferred(ctx context.Context, userID, transactionID uint64, reason string, deferred bool) error {
	if userID == 0 || transactionID == 0 {
		return errors.New("userID and transactionID are required")
	}
	reason, err := normalizeTransactionActionReason(reason)
	if err != nil {
		return err
	}
	actionKind := transactionActionUndefer
	if deferred {
		actionKind = transactionActionDefer
	}
	return s.db.WithContext(ctx).Transaction(func(txdb *gorm.DB) error {
		var source paymentTransaction
		if err := txdb.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND user_id = ?", transactionID, userID).First(&source).Error; err != nil {
			return err
		}
		if source.Direction != "income" || !isPendingMatchStatus(source.MatchStatus) {
			return ErrTransactionNotDeferrable
		}
		var latest paymentTransactionAction
		err := txdb.Where("user_id = ? AND payment_transaction_id = ? AND action_kind IN ?", userID, transactionID, []string{transactionActionDefer, transactionActionUndefer}).Order("id DESC").First(&latest).Error
		currentlyDeferred := err == nil && latest.ActionKind == transactionActionDefer
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		if currentlyDeferred == deferred {
			return nil
		}
		return s.writeTransactionAction(txdb, userID, transactionID, actionKind, reason, "", "", time.Now().UTC())
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
		_, _, nextSummary, revokeErr := s.revokeTransactionAllocationsInTx(txdb, userID, transactionID, reason, idempotencyKey)
		summary = nextSummary
		return revokeErr
	})
	return summary, err
}

// revokeTransactionAllocationsInTx keeps the audit-preserving revoke work
// reusable by correction flows that must void and replace allocations atomically.
func (s *transactionService) revokeTransactionAllocationsInTx(txdb *gorm.DB, userID, transactionID uint64, reason, idempotencyKey string) (paymentTransaction, []paymentAllocation, transactionAllocationSummary, error) {
	var source paymentTransaction
	if err := txdb.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND user_id = ?", transactionID, userID).First(&source).Error; err != nil {
		return paymentTransaction{}, nil, transactionAllocationSummary{}, err
	}
	var allocations []paymentAllocation
	if err := txdb.Where("payment_transaction_id = ? AND user_id = ?", transactionID, userID).Order("id ASC").Find(&allocations).Error; err != nil {
		return paymentTransaction{}, nil, transactionAllocationSummary{}, err
	}
	alreadyDone, err := s.existingTransactionAction(txdb, userID, transactionID, transactionActionRevokeAllocations, reason, idempotencyKey)
	if err != nil {
		return paymentTransaction{}, nil, transactionAllocationSummary{}, err
	}
	if alreadyDone {
		return source, allocations, summarizeTransactionAllocations(source, allocations), nil
	}
	if summarizeTransactionAllocations(source, allocations).AllocatedCents == 0 {
		return paymentTransaction{}, nil, transactionAllocationSummary{}, errors.New("transaction has no effective allocations to revoke")
	}

	now := time.Now().UTC()
	operationID := recordID("transaction-revoke", now)
	obligationIDSet := make(map[uint64]struct{})
	for _, allocation := range allocations {
		if !ledgerAllocationIsEffective(allocation) {
			continue
		}
		if allocation.RentObligationID != nil && *allocation.RentObligationID != 0 && ledgerAllocationKind(allocation) == allocationKindRent {
			obligationIDSet[*allocation.RentObligationID] = struct{}{}
		}
	}
	obligationIDs := make([]uint64, 0, len(obligationIDSet))
	for obligationID := range obligationIDSet {
		obligationIDs = append(obligationIDs, obligationID)
	}
	sort.Slice(obligationIDs, func(i, j int) bool { return obligationIDs[i] < obligationIDs[j] })
	lockedObligations, err := lockRentObligationRoomsInTx(txdb, userID, obligationIDs)
	if err != nil {
		return paymentTransaction{}, nil, transactionAllocationSummary{}, err
	}

	for _, allocation := range allocations {
		if !ledgerAllocationIsEffective(allocation) {
			continue
		}
		if err := txdb.Model(&paymentAllocation{}).Where("id = ? AND user_id = ? AND payment_transaction_id = ? AND status = ?", allocation.ID, userID, transactionID, allocationStatusConfirmed).Updates(map[string]any{
			"status":            allocationStatusVoided,
			"operation_id":      operationID,
			"voided_at":         now,
			"voided_by_user_id": userID,
			"void_reason":       reason,
		}).Error; err != nil {
			return paymentTransaction{}, nil, transactionAllocationSummary{}, err
		}
	}

	for _, obligationID := range obligationIDs {
		obligation, ok := lockedObligations[obligationID]
		if !ok {
			return paymentTransaction{}, nil, transactionAllocationSummary{}, ErrRentFactsConflict
		}
		var current []paymentAllocation
		if err := txdb.Where("rent_obligation_id = ? AND user_id = ?", obligationID, userID).Find(&current).Error; err != nil {
			return paymentTransaction{}, nil, transactionAllocationSummary{}, err
		}
		var cashReceipts []cashReceipt
		if err := txdb.Where("rent_obligation_id = ? AND user_id = ?", obligationID, userID).Find(&cashReceipts).Error; err != nil {
			return paymentTransaction{}, nil, transactionAllocationSummary{}, err
		}
		projected := projectRentObligation(obligation, current, cashReceipts, now)
		if err := txdb.Model(&rentObligation{}).Where("id = ? AND user_id = ?", obligationID, userID).Updates(map[string]any{
			"paid_amount_cents": projected.PaidAmountCents,
			"status":            projected.Status,
		}).Error; err != nil {
			return paymentTransaction{}, nil, transactionAllocationSummary{}, err
		}
	}

	if err := s.writeTransactionAction(txdb, userID, transactionID, transactionActionRevokeAllocations, reason, idempotencyKey, operationID, now); err != nil {
		return paymentTransaction{}, nil, transactionAllocationSummary{}, err
	}
	allocationsAfter := make([]paymentAllocation, 0, len(allocations))
	if err := txdb.Where("payment_transaction_id = ? AND user_id = ?", transactionID, userID).Order("id ASC").Find(&allocationsAfter).Error; err != nil {
		return paymentTransaction{}, nil, transactionAllocationSummary{}, err
	}
	projection := projectTransactionMatch(source, allocationsAfter, transactionActionRevokeAllocations, reason)
	if err := updateTransactionProjection(txdb, userID, transactionID, projection); err != nil {
		return paymentTransaction{}, nil, transactionAllocationSummary{}, err
	}
	return source, allocationsAfter, summarizeTransactionAllocations(source, allocationsAfter), nil
}

func (s *transactionService) rematchRentAllocation(ctx context.Context, userID, transactionID, targetTenantID uint64, targetPeriod time.Time) error {
	if userID == 0 || transactionID == 0 || targetTenantID == 0 || targetPeriod.IsZero() {
		return errors.New("userID, transactionID, target tenant, and target rent month are required")
	}
	targetPeriod = monthStart(targetPeriod)
	var targetTenant tenant
	if err := s.db.WithContext(ctx).Where("id = ? AND user_id = ?", targetTenantID, userID).First(&targetTenant).Error; err != nil {
		return err
	}
	if err := newMonthlyRentFactsService(s.db).ensureMonthlyRentFacts(ctx, userID, targetPeriod, rentFactsIntentExplicitPayment); err != nil {
		return err
	}
	return s.db.WithContext(ctx).Transaction(func(txdb *gorm.DB) error {
		var source paymentTransaction
		if err := txdb.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND user_id = ?", transactionID, userID).First(&source).Error; err != nil {
			return err
		}
		var allocations []paymentAllocation
		if err := txdb.Where("payment_transaction_id = ? AND user_id = ?", transactionID, userID).Order("id ASC").Find(&allocations).Error; err != nil {
			return err
		}
		previous, ok := singleEffectiveRentAllocation(allocations)
		if !ok {
			return errors.New("only a transaction with one effective rent allocation can be rematched")
		}
		if previous.PrepaymentID != nil {
			return errors.New("先撤销这份预收款租金，再选择新的账单使用")
		}
		var target rentObligation
		if err := txdb.Where("user_id = ? AND tenant_id = ? AND period_month = ?", userID, targetTenantID, targetPeriod).First(&target).Error; err != nil {
			return err
		}
		if previous.RentObligationIDValue() == target.ID {
			return errors.New("selected rent obligation is already matched")
		}

		obligations, err := lockRentObligationRoomsInTx(txdb, userID, []uint64{previous.RentObligationIDValue(), target.ID})
		if err != nil {
			return err
		}
		lockedTarget, ok := obligations[target.ID]
		if !ok {
			return ErrRentFactsConflict
		}
		target = lockedTarget
		if _, _, _, err := s.revokeTransactionAllocationsInTx(txdb, userID, transactionID, "修改匹配", ""); err != nil {
			return err
		}
		_, err = s.allocateTransactionInTx(txdb, userID, transactionID, []transactionAllocationDraft{{
			TenantID:         target.TenantID,
			RentObligationID: target.ID,
			AmountCents:      previous.AmountCents,
			Kind:             allocationKindRent,
		}}, "", "manual_rematch")
		return err
	})
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
