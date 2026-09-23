package main

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type rentMatchBatchItem struct {
	TenantID    uint64
	Period      string
	AmountCents int64
}

// confirmRentMatchBatch keeps the selected targets in one ledger operation.
// Selecting a future month is explicit payment intent; the target is then
// reloaded under the current account before the locked allocation transaction.
func (s *transactionService) confirmRentMatchBatch(ctx context.Context, userID, transactionID uint64, items []rentMatchBatchItem, rememberTenantID uint64, requestKey string) (transactionAllocationSummary, error) {
	if userID == 0 || transactionID == 0 || len(items) == 0 || len(items) > 20 || strings.TrimSpace(requestKey) == "" || len(requestKey) > 120 {
		return transactionAllocationSummary{}, errors.New("invalid batch match request")
	}
	tenantIDs, periods, err := validateRentMatchBatchItems(items, rememberTenantID)
	if err != nil {
		return transactionAllocationSummary{}, err
	}
	// Verify the source before explicit month selection can materialize rent
	// facts. The locked allocation path checks it again before writing.
	var source paymentTransaction
	if err := s.db.WithContext(ctx).Where("id = ? AND user_id = ?", transactionID, userID).First(&source).Error; err != nil {
		return transactionAllocationSummary{}, err
	}
	if source.Direction != "income" || source.MatchStatus == "ignored" {
		return transactionAllocationSummary{}, errors.New("transaction is not available for rent matching")
	}
	var count int64
	if err := s.db.WithContext(ctx).Model(&tenant{}).Where("user_id = ? AND id IN ?", userID, mapUint64Keys(tenantIDs)).Count(&count).Error; err != nil {
		return transactionAllocationSummary{}, err
	}
	if count != int64(len(tenantIDs)) {
		return transactionAllocationSummary{}, gorm.ErrRecordNotFound
	}
	periodKeys := make([]string, 0, len(periods))
	for period := range periods {
		periodKeys = append(periodKeys, period)
	}
	sort.Strings(periodKeys)
	for _, periodKey := range periodKeys {
		period, _ := parsePeriodMonth(periodKey)
		if err := newMonthlyRentFactsService(s.db).ensureMonthlyRentFacts(ctx, userID, period, rentFactsIntentExplicitPayment); err != nil {
			return transactionAllocationSummary{}, err
		}
	}
	drafts := make([]transactionAllocationDraft, 0, len(items))
	for _, item := range items {
		period, _ := parsePeriodMonth(item.Period)
		var obligation rentObligation
		if err := s.db.WithContext(ctx).Where("user_id = ? AND tenant_id = ? AND period_month = ? AND record_status <> ?", userID, item.TenantID, period, obligationRecordVoided).First(&obligation).Error; err != nil {
			return transactionAllocationSummary{}, err
		}
		drafts = append(drafts, transactionAllocationDraft{TenantID: item.TenantID, RentObligationID: obligation.ID, AmountCents: item.AmountCents, Kind: allocationKindRent})
	}
	var summary transactionAllocationSummary
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Serialize retries before checking the request key. A second submit
		// must see the first operation, then verify the same target list.
		var lockedSource paymentTransaction
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND user_id = ?", transactionID, userID).First(&lockedSource).Error; err != nil {
			return err
		}
		var previous paymentAllocation
		lookup := tx.Where("user_id = ? AND idempotency_key = ?", userID, allocationRequestKey(requestKey, transactionID, 0)).First(&previous).Error
		if lookup == nil {
			if !sameRentMatchBatchInTx(tx, previous, transactionID, drafts) {
				return errors.New("batch match request key was reused for different targets")
			}
			var source paymentTransaction
			if err := tx.Where("id = ? AND user_id = ?", transactionID, userID).First(&source).Error; err != nil {
				return err
			}
			var allocations []paymentAllocation
			if err := tx.Where("user_id = ? AND payment_transaction_id = ?", userID, transactionID).Find(&allocations).Error; err != nil {
				return err
			}
			summary = summarizeTransactionAllocations(source, allocations)
			return nil
		}
		if !errors.Is(lookup, gorm.ErrRecordNotFound) {
			return lookup
		}
		var allocationErr error
		summary, allocationErr = s.allocateTransactionInTx(tx, userID, transactionID, drafts, requestKey, "manual_review")
		if allocationErr != nil || rememberTenantID == 0 {
			return allocationErr
		}
		var source paymentTransaction
		if err := tx.Where("id = ? AND user_id = ?", transactionID, userID).First(&source).Error; err != nil {
			return err
		}
		payerName := strings.TrimSpace(stringValue(source.PayerName))
		if payerName == "" {
			return errors.New("bank payer name is unavailable")
		}
		payerInput := tenantPayerInput{Name: payerName, PayerID: stablePayerID(stringValue(source.PayerID))}
		if err := validateTenantPayerInput(payerInput); err != nil {
			return err
		}
		return addTenantPayerInTx(tx, userID, rememberTenantID, payerInput, "matched_transaction")
	})
	if err == nil && rememberTenantID != 0 {
		// Other pending rows are independent of this confirmed batch. A later
		// reconciliation can retry if this best-effort pass is interrupted.
		_ = s.reconcilePendingRentTransactions(ctx, userID)
	}
	return summary, err
}

func validateRentMatchBatchItems(items []rentMatchBatchItem, rememberTenantID uint64) (map[uint64]bool, map[string]bool, error) {
	seen := make(map[string]bool, len(items))
	periods := make(map[string]bool)
	tenantIDs := make(map[uint64]bool)
	for _, item := range items {
		if item.TenantID == 0 || item.AmountCents <= 0 {
			return nil, nil, errors.New("invalid batch match item")
		}
		if _, err := parsePeriodMonth(item.Period); err != nil {
			return nil, nil, err
		}
		key := fmt.Sprintf("%d:%s", item.TenantID, item.Period)
		if seen[key] {
			return nil, nil, errors.New("duplicate tenant month")
		}
		seen[key], periods[item.Period], tenantIDs[item.TenantID] = true, true, true
	}
	if rememberTenantID != 0 && !tenantIDs[rememberTenantID] {
		return nil, nil, errors.New("remembered payer must be in this batch")
	}
	return tenantIDs, periods, nil
}

func mapUint64Keys(values map[uint64]bool) []uint64 {
	keys := make([]uint64, 0, len(values))
	for key := range values {
		keys = append(keys, key)
	}
	return keys
}

func sameRentMatchBatchInTx(tx *gorm.DB, first paymentAllocation, transactionID uint64, drafts []transactionAllocationDraft) bool {
	if first.PaymentTransactionID != transactionID || first.OperationID == nil || first.ConfirmationSource != "manual_review" {
		return false
	}
	var rows []paymentAllocation
	if err := tx.Where("user_id = ? AND operation_id = ?", first.UserID, *first.OperationID).Order("id ASC").Find(&rows).Error; err != nil || len(rows) != len(drafts) {
		return false
	}
	for index, row := range rows {
		draft := drafts[index]
		if row.PaymentTransactionID != transactionID || row.TenantID == nil || *row.TenantID != draft.TenantID || row.RentObligationID == nil || *row.RentObligationID != draft.RentObligationID || row.AmountCents != draft.AmountCents || ledgerAllocationKind(row) != allocationKindRent {
			return false
		}
	}
	return true
}
