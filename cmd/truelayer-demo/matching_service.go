package main

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

type paymentAllocation struct {
	ID                   uint64 `gorm:"primaryKey"`
	UserID               uint64
	PaymentTransactionID uint64
	RentObligationID     *uint64
	TenantID             *uint64
	AmountCents          int64
	AllocationKind       string
	Note                 string
	Status               string
	OperationID          *string
	IdempotencyKey       *string
	ConfirmedByUserID    uint64
	ConfirmedAt          time.Time
	VoidedAt             *time.Time
	VoidedByUserID       *uint64
	VoidReason           *string
	ConfirmationSource   string
	CreatedAt            time.Time
}

func (s *transactionService) applyAllocation(ctx context.Context, userID uint64, transaction paymentTransaction, decision matchDecision, source string) error {
	if decision.RentObligationID == 0 || decision.TenantID == 0 {
		return errors.New("rent match target is incomplete")
	}
	amountCents := decision.AllocationAmountCents
	if amountCents <= 0 {
		amountCents = transaction.AmountCents
	}
	_, err := s.allocateTransaction(ctx, userID, transaction.ID, []transactionAllocationDraft{{
		TenantID:         decision.TenantID,
		RentObligationID: decision.RentObligationID,
		AmountCents:      amountCents,
		Kind:             allocationKindRent,
	}}, rentMatchRequestKey(transaction.ID, decision.RentObligationID, amountCents), source)
	return err
}

func rentMatchRequestKey(transactionID, obligationID uint64, amountCents int64) string {
	return fmt.Sprintf("rent-match:%d:%d:%d", transactionID, obligationID, amountCents)
}

func isReconcilableMatchStatus(status string) bool {
	switch status {
	case "unmatched", "candidate", "needs_review":
		return true
	default:
		return false
	}
}

func pendingMatchProjection(decision matchDecision) transactionMatchProjection {
	projection := transactionMatchProjection{Status: decision.Status, Reason: decision.Reason}
	if decision.TenantID != 0 {
		tenantID := decision.TenantID
		projection.MatchedTenantID = &tenantID
	}
	return projection
}

// reconcilePendingRentTransactions applies remembered payer relations to
// unallocated income transactions. An exact/open rent responsibility can be
// allocated automatically; a recognized payer whose referenced month is
// already covered remains a candidate for human confirmation.
func (s *transactionService) reconcilePendingRentTransactions(ctx context.Context, userID uint64) error {
	if userID == 0 {
		return errors.New("userID is required")
	}
	var transactions []paymentTransaction
	if err := s.db.WithContext(ctx).
		Where("user_id = ? AND direction = ? AND match_status IN ?", userID, "income", []string{"unmatched", "candidate", "needs_review"}).
		Order("transaction_time ASC, id ASC").Find(&transactions).Error; err != nil {
		return err
	}
	if len(transactions) == 0 {
		return nil
	}
	var tenants []tenant
	if err := s.db.WithContext(ctx).Where("user_id = ?", userID).Find(&tenants).Error; err != nil {
		return err
	}
	var payers []tenantPayer
	if err := s.db.WithContext(ctx).Where("user_id = ? AND removed_at IS NULL", userID).Find(&payers).Error; err != nil {
		return err
	}
	facts := newMonthlyRentFactsService(s.db)
	for _, transaction := range transactions {
		if !isReconcilableMatchStatus(transaction.MatchStatus) {
			continue
		}
		deferred, err := transactionDeferredState(ctx, s.db, userID, transaction.ID)
		if err != nil {
			return err
		}
		if deferred {
			continue
		}
		if transaction.ParsedPeriodMonth != nil && !transaction.ParsedPeriodMonth.IsZero() {
			if err := facts.ensureMonthlyRentFacts(ctx, userID, *transaction.ParsedPeriodMonth, rentFactsIntentRead); err != nil {
				return err
			}
		}
		var allocations []paymentAllocation
		if err := s.db.WithContext(ctx).Where("user_id = ? AND payment_transaction_id = ?", userID, transaction.ID).Find(&allocations).Error; err != nil {
			return err
		}
		if summarizeTransactionAllocations(transaction, allocations).AllocatedCents > 0 {
			continue
		}
		var obligations []rentObligation
		if err := s.db.WithContext(ctx).Where("user_id = ?", userID).Find(&obligations).Error; err != nil {
			return err
		}
		decision := decideStrictRentMatch(paymentTransactionInputFromModel(transaction), payers, tenants, obligations)
		switch decision.Status {
		case "matched", "partial":
			if decision.RentObligationID == 0 || decision.AllocationAmountCents <= 0 {
				decision.Status = "candidate"
				decision.Reason = "recognized payer requires confirmation"
				if err := updateTransactionProjection(s.db.WithContext(ctx), userID, transaction.ID, pendingMatchProjection(decision)); err != nil {
					return err
				}
				continue
			}
			if err := s.applyAllocation(ctx, userID, transaction, decision, decision.ConfirmationSource); err != nil {
				return err
			}
			if err := s.db.WithContext(ctx).Model(&paymentTransaction{}).Where("id = ? AND user_id = ?", transaction.ID, userID).Update("match_reason", nullableString(decision.Reason)).Error; err != nil {
				return err
			}
		default:
			if err := updateTransactionProjection(s.db.WithContext(ctx), userID, transaction.ID, pendingMatchProjection(decision)); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *transactionService) confirmRentMatch(ctx context.Context, userID, transactionID, tenantID uint64, period *time.Time, rememberPayer bool) error {
	if userID == 0 || transactionID == 0 || tenantID == 0 {
		return errors.New("transaction, tenant, and user are required")
	}
	var transaction paymentTransaction
	if err := s.db.WithContext(ctx).Where("id = ? AND user_id = ?", transactionID, userID).First(&transaction).Error; err != nil {
		return err
	}
	if transaction.Direction != "income" {
		return errors.New("only income transactions can be matched to rent")
	}
	var allocations []paymentAllocation
	if err := s.db.WithContext(ctx).Where("payment_transaction_id = ? AND user_id = ?", transactionID, userID).Order("id ASC").Find(&allocations).Error; err != nil {
		return err
	}
	remaining := summarizeTransactionAllocations(transaction, allocations).RemainingCents
	if remaining <= 0 {
		return errors.New("transaction has no remaining amount to match")
	}
	var tenantRow tenant
	if err := s.db.WithContext(ctx).Where("id = ? AND user_id = ?", tenantID, userID).First(&tenantRow).Error; err != nil {
		return err
	}
	if period == nil {
		return errors.New("rent period is required for manual confirmation")
	}
	if err := newMonthlyRentFactsService(s.db).ensureMonthlyRentFacts(ctx, userID, *period, rentFactsIntentExplicitPayment); err != nil {
		return err
	}
	var obligations []rentObligation
	if err := s.db.WithContext(ctx).Where("user_id = ? AND tenant_id = ?", userID, tenantID).Order("period_month ASC").Find(&obligations).Error; err != nil {
		return err
	}
	matchInput := paymentTransactionInputFromModel(transaction)
	matchInput.AmountCents = remaining
	decision := decideForTenantInPeriod(matchInput, tenantRow, obligations, *period, "manual_month")
	if decision.Status != "matched" && decision.Status != "partial" {
		return errors.New("selected rent obligation needs review")
	}
	if err := s.applyAllocation(ctx, userID, transaction, decision, "manual_name"); err != nil {
		return err
	}
	if rememberPayer {
		if err := newTenantService(s.db).rememberTenantPayer(ctx, userID, tenantID, stringValue(transaction.PayerID), stringValue(transaction.PayerName)); err != nil {
			return err
		}
		if err := s.reconcilePendingRentTransactions(ctx, userID); err != nil {
			return err
		}
	}
	return nil
}

func (s *transactionService) confirmRentMatchToObligation(ctx context.Context, userID, transactionID, rentObligationID uint64, rememberPayer bool) error {
	if userID == 0 || transactionID == 0 || rentObligationID == 0 {
		return errors.New("transaction, rent obligation, and user are required")
	}
	var obligation rentObligation
	if err := s.db.WithContext(ctx).Where("id = ? AND user_id = ?", rentObligationID, userID).First(&obligation).Error; err != nil {
		return err
	}
	period := monthStart(obligation.PeriodMonth)
	return s.confirmRentMatch(ctx, userID, transactionID, obligation.TenantID, &period, rememberPayer)
}

func (s *transactionService) listTransactionPageRows(ctx context.Context, userID uint64, filters transactionFilters) ([]transactionPageRow, error) {
	rows, _, err := s.listTransactionPageRowsWithTotal(ctx, userID, filters)
	return rows, err
}

func (s *transactionService) listTransactionPageRowsWithTotal(ctx context.Context, userID uint64, filters transactionFilters) ([]transactionPageRow, int64, error) {
	transactions, total, err := s.listTransactionsPage(ctx, userID, filters)
	if err != nil {
		return nil, 0, err
	}
	var tenants []tenant
	if err := s.db.WithContext(ctx).Where("user_id = ?", userID).Find(&tenants).Error; err != nil {
		return nil, 0, err
	}
	var obligations []rentObligation
	if err := s.db.WithContext(ctx).Where("user_id = ?", userID).Find(&obligations).Error; err != nil {
		return nil, 0, err
	}
	var payers []tenantPayer
	if err := s.db.WithContext(ctx).Where("user_id = ? AND removed_at IS NULL", userID).Find(&payers).Error; err != nil {
		return nil, 0, err
	}
	allocationsByTransaction := make(map[uint64][]paymentAllocation)
	if len(transactions) > 0 {
		ids := make([]uint64, 0, len(transactions))
		for _, transaction := range transactions {
			ids = append(ids, transaction.ID)
		}
		var allocations []paymentAllocation
		if err := s.db.WithContext(ctx).Where("user_id = ? AND payment_transaction_id IN ?", userID, ids).Order("id ASC").Find(&allocations).Error; err != nil {
			return nil, 0, err
		}
		for _, allocation := range allocations {
			allocationsByTransaction[allocation.PaymentTransactionID] = append(allocationsByTransaction[allocation.PaymentTransactionID], allocation)
		}
	}
	rows := make([]transactionPageRow, 0, len(transactions))
	tenantNames := make(map[uint64]string, len(tenants))
	tenantByID := make(map[uint64]tenant, len(tenants))
	obligationByID := make(map[uint64]rentObligation, len(obligations))
	for _, obligation := range obligations {
		obligationByID[obligation.ID] = obligation
	}
	for _, tenantRow := range tenants {
		tenantNames[tenantRow.ID] = tenantRow.Name
		tenantByID[tenantRow.ID] = tenantRow
	}
	for _, transaction := range transactions {
		allocations := allocationsByTransaction[transaction.ID]
		row := enrichTransactionPageRow(transactionPageRowFromModel(transaction), transaction, allocations)
		decorateTransactionPageRow(&row, transaction, allocations, obligations, tenants, tenantByID, tenantNames, payers)
		rows = append(rows, row)
	}
	chargeIDsByRow := make([][]uint64, len(rows))
	neededChargeIDs := make(map[uint64]struct{})
	for index := range rows {
		chargeIDsByRow[index] = transactionObjectChargeIDs(rows[index], allocationsByTransaction[transactions[index].ID], obligationByID)
		for _, chargeID := range chargeIDsByRow[index] {
			neededChargeIDs[chargeID] = struct{}{}
		}
	}
	if len(neededChargeIDs) > 0 {
		ids := make([]uint64, 0, len(neededChargeIDs))
		for id := range neededChargeIDs {
			ids = append(ids, id)
		}
		var chargeRows []rentCharge
		if err := s.db.WithContext(ctx).Where("user_id = ? AND id IN ?", userID, ids).Find(&chargeRows).Error; err != nil {
			return nil, 0, err
		}
		charges := make(map[uint64]rentCharge, len(chargeRows))
		for _, charge := range chargeRows {
			charges[charge.ID] = charge
		}
		for index := range rows {
			rows[index].ObjectLabel, rows[index].RoomOnlyLabel = transactionRentObjectLabels(chargeIDsByRow[index], charges)
		}
	}
	return rows, total, nil
}

// decorateTransactionPageRow fills the per-row action state that the transaction
// list's action cell renders. The transaction detail header reuses it verbatim,
// which is what keeps the detail page's 确认匹配 / 标记非租金 / 编辑分配 entries
// identical to the same-named list-row actions instead of a second implementation.
func decorateTransactionPageRow(row *transactionPageRow, transaction paymentTransaction, allocations []paymentAllocation, obligations []rentObligation, tenants []tenant, tenantByID map[uint64]tenant, tenantNames map[uint64]string, payers []tenantPayer) {
	summary := summarizeTransactionAllocations(transaction, allocations)
	matchedTenantIDs := make(map[uint64]struct{})
	if transaction.MatchedTenantID != nil && *transaction.MatchedTenantID != 0 {
		matchedTenantIDs[*transaction.MatchedTenantID] = struct{}{}
	}
	for _, allocation := range allocations {
		if ledgerAllocationIsEffective(allocation) && allocation.TenantID != nil && *allocation.TenantID != 0 {
			matchedTenantIDs[*allocation.TenantID] = struct{}{}
		}
	}
	matchedTenantNames := make([]string, 0, len(matchedTenantIDs))
	for tenantID := range matchedTenantIDs {
		if name := strings.TrimSpace(firstNonEmpty(tenantNames[tenantID], tenantByID[tenantID].DisplayAlias, tenantByID[tenantID].Name)); name != "" {
			matchedTenantNames = append(matchedTenantNames, name)
		}
	}
	sort.Strings(matchedTenantNames)
	row.MatchedTenantName = strings.Join(matchedTenantNames, "、")
	if row.TenantID != 0 && transaction.Direction == "income" && summary.RemainingCents > 0 && transaction.MatchStatus != "ignored" {
		row.NeedsMonthChoice = true
		row.MonthOptions = rentMonthOptionsForTenant(transaction, obligations, row.TenantID)
	}
	if transaction.Direction == "income" && summary.AllocatedCents == 0 && transaction.MatchStatus != "ignored" {
		decision := decideStrictRentMatch(paymentTransactionInputFromModel(transaction), payers, tenants, obligations)
		switch decision.Status {
		case "matched", "partial":
			row.CandidateTenantID = decision.TenantID
			row.CandidateTenantName = tenantNames[decision.TenantID]
			row.CandidateRentObligationID = decision.RentObligationID
			row.CandidatePeriod = monthStart(decision.PeriodMonth).Format("2006-01")
			row.CanConfirm = decision.TenantID != 0 && decision.RentObligationID != 0
		case "candidate":
			row.TenantID = decision.TenantID
			row.CandidateTenantID = decision.TenantID
			row.CandidateTenantName = tenantNames[decision.TenantID]
			row.CandidateRentObligationID = decision.RentObligationID
			if !decision.PeriodMonth.IsZero() {
				row.CandidatePeriod = monthStart(decision.PeriodMonth).Format("2006-01")
			}
			row.NeedsMonthChoice = decision.TenantID != 0
		}
	}
	if row.NeedsMonthChoice && len(row.MonthOptions) == 0 {
		row.MonthOptions = rentMonthOptionsForTenant(transaction, obligations, row.TenantID)
	}
	if transaction.Direction == "income" && summary.RemainingCents > 0 && transaction.MatchStatus != "ignored" {
		row.ManualMatchOptions = availableRentManualMatchOptions(transaction, obligations, tenantNames, summary.RemainingCents)
		row.ManualMatchTenantOptions, _ = rematchFilterOptions(row.ManualMatchOptions)
		if summary.AllocatedCents == 0 {
			row.RematchTenantOptions, row.RematchMonthOptions = rematchFilterOptions(row.ManualMatchOptions)
		}
	}
	if effectiveRentAllocation, ok := singleEffectiveRentAllocation(allocations); ok {
		row.CanRematch = true
		row.RematchOptions = availableRentMatchOptions(transaction, obligations, tenantNames, effectiveRentAllocation.AmountCents, effectiveRentAllocation.RentObligationIDValue())
		row.CanEditRentMatch = len(row.RematchOptions) > 0
		row.RematchTenantOptions, row.RematchMonthOptions = rematchFilterOptions(row.RematchOptions)
	}
}

// A confirmed rent allocation identifies a room through its obligation and
// charge. Before confirmation, only a specific suggested obligation can supply
// that context; a tenant alone may have occupied different rooms over time.
func transactionObjectChargeIDs(row transactionPageRow, allocations []paymentAllocation, obligations map[uint64]rentObligation) []uint64 {
	seen := make(map[uint64]struct{})
	for _, allocation := range allocations {
		if !ledgerAllocationIsEffective(allocation) || ledgerAllocationKind(allocation) != allocationKindRent || allocation.RentObligationID == nil {
			continue
		}
		if obligation, ok := obligations[*allocation.RentObligationID]; ok && obligation.RentChargeID != 0 {
			seen[obligation.RentChargeID] = struct{}{}
		}
	}
	if len(seen) == 0 && row.CandidateRentObligationID != 0 {
		if obligation, ok := obligations[row.CandidateRentObligationID]; ok && obligation.RentChargeID != 0 {
			seen[obligation.RentChargeID] = struct{}{}
		}
	}
	ids := make([]uint64, 0, len(seen))
	for id := range seen {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
	return ids
}

func transactionRentObjectLabels(chargeIDs []uint64, charges map[uint64]rentCharge) (string, string) {
	seen := make(map[string]struct{})
	labels := make([]string, 0, len(chargeIDs))
	roomOnly := ""
	for _, chargeID := range chargeIDs {
		charge, ok := charges[chargeID]
		if !ok {
			continue
		}
		propertyName := strings.TrimSpace(stringValue(charge.PropertyNameSnapshot))
		roomLabel := strings.TrimSpace(stringValue(charge.RoomLabelSnapshot))
		label := propertyName
		if roomLabel != "" {
			if label != "" {
				label += " · "
			}
			label += roomLabel
		}
		if label == "" {
			continue
		}
		if _, exists := seen[label]; exists {
			continue
		}
		seen[label] = struct{}{}
		labels = append(labels, label)
		roomOnly = roomLabel
	}
	sort.Strings(labels)
	if len(labels) != 1 {
		roomOnly = ""
	}
	return strings.Join(labels, "、"), roomOnly
}

func availableRentMatchOptions(source paymentTransaction, obligations []rentObligation, tenantNames map[uint64]string, amountCents int64, excludedObligationID uint64) []billingRentMatchOption {
	return availableRentOptions(source, obligations, tenantNames, amountCents, excludedObligationID, false)
}

func availableRentManualMatchOptions(source paymentTransaction, obligations []rentObligation, tenantNames map[uint64]string, amountCents int64) []billingRentMatchOption {
	return availableRentOptions(source, obligations, tenantNames, amountCents, 0, true)
}

func availableRentOptions(source paymentTransaction, obligations []rentObligation, tenantNames map[uint64]string, amountCents int64, excludedObligationID uint64, allowPartial bool) []billingRentMatchOption {
	if amountCents <= 0 {
		return nil
	}
	options := make([]billingRentMatchOption, 0)
	for _, obligation := range obligations {
		remaining := obligation.ExpectedAmountCents - obligation.PaidAmountCents
		if obligation.ID == excludedObligationID || obligation.RecordStatus == obligationRecordVoided || remaining <= 0 || (!allowPartial && remaining < amountCents) || !strings.EqualFold(firstNonEmpty(obligation.Currency, source.Currency), source.Currency) {
			continue
		}
		options = append(options, billingRentMatchOption{
			RentObligationID: obligation.ID,
			TenantID:         obligation.TenantID,
			TenantName:       firstNonEmpty(tenantNames[obligation.TenantID], "租客"),
			Period:           monthStart(obligation.PeriodMonth).Format("2006-01"),
			PeriodLabel:      formatMonthLabel(obligation.PeriodMonth),
			Expected:         formatMoney(centsToMoney(obligation.ExpectedAmountCents), source.Currency, 2),
			Remaining:        formatMoney(centsToMoney(remaining), source.Currency, 2),
			Label:            fmt.Sprintf("%s · %d年%d月 · 未收 %s", firstNonEmpty(tenantNames[obligation.TenantID], "租客"), obligation.PeriodMonth.Year(), obligation.PeriodMonth.Month(), formatMoney(centsToMoney(remaining), source.Currency, 2)),
		})
	}
	return options
}

func rematchFilterOptions(options []billingRentMatchOption) ([]billingTenantOption, []billingMonthOption) {
	tenants := make([]billingTenantOption, 0)
	months := make([]billingMonthOption, 0)
	seenTenants := make(map[uint64]struct{}, len(options))
	seenMonths := make(map[string]struct{}, len(options))
	for _, option := range options {
		if option.TenantID != 0 {
			if _, seen := seenTenants[option.TenantID]; !seen {
				seenTenants[option.TenantID] = struct{}{}
				tenants = append(tenants, billingTenantOption{ID: option.TenantID, Name: option.TenantName})
			}
		}
		if option.Period != "" {
			if _, seen := seenMonths[option.Period]; !seen {
				seenMonths[option.Period] = struct{}{}
				months = append(months, billingMonthOption{Period: option.Period, Label: option.PeriodLabel, Remaining: option.Remaining})
			}
		}
	}
	return tenants, months
}

func rentMonthOptionsForTenant(source paymentTransaction, obligations []rentObligation, tenantID uint64) []billingMonthOption {
	if tenantID == 0 || source.AmountCents <= 0 {
		return nil
	}
	options := make([]billingMonthOption, 0)
	for _, obligation := range obligations {
		if obligation.TenantID != tenantID || obligation.RecordStatus == obligationRecordVoided || obligation.PaidAmountCents >= obligation.ExpectedAmountCents || obligation.ExpectedAmountCents-obligation.PaidAmountCents < source.AmountCents || !strings.EqualFold(firstNonEmpty(obligation.Currency, source.Currency), source.Currency) {
			continue
		}
		currency := firstNonEmpty(obligation.Currency, source.Currency, "EUR")
		options = append(options, billingMonthOption{
			Period:    obligation.PeriodMonth.Format("2006-01"),
			Label:     fmt.Sprintf("%d年%d月", obligation.PeriodMonth.Year(), obligation.PeriodMonth.Month()),
			Expected:  formatMoney(centsToMoney(obligation.ExpectedAmountCents), currency, 2),
			Paid:      formatMoney(centsToMoney(obligation.PaidAmountCents), currency, 2),
			Remaining: formatMoney(centsToMoney(obligation.ExpectedAmountCents-obligation.PaidAmountCents), currency, 2),
		})
	}
	return options
}

func singleEffectiveRentAllocation(allocations []paymentAllocation) (paymentAllocation, bool) {
	var effective []paymentAllocation
	for _, allocation := range allocations {
		if ledgerAllocationIsEffective(allocation) {
			effective = append(effective, allocation)
		}
	}
	if len(effective) != 1 || ledgerAllocationKind(effective[0]) != allocationKindRent || effective[0].RentObligationID == nil || *effective[0].RentObligationID == 0 {
		return paymentAllocation{}, false
	}
	return effective[0], true
}

func (a paymentAllocation) RentObligationIDValue() uint64 {
	if a.RentObligationID == nil {
		return 0
	}
	return *a.RentObligationID
}

func transactionID(value string) (uint64, error) {
	return strconv.ParseUint(value, 10, 64)
}
