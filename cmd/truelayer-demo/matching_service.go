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
	for _, tenantRow := range tenants {
		tenantNames[tenantRow.ID] = tenantRow.Name
		tenantByID[tenantRow.ID] = tenantRow
	}
	for _, transaction := range transactions {
		allocations := allocationsByTransaction[transaction.ID]
		row := enrichTransactionPageRow(transactionPageRowFromModel(transaction), transaction, allocations, obligations)
		decorateTransactionPageRow(&row, transaction, allocations, obligations, tenants, tenantByID, tenantNames, payers)
		rows = append(rows, row)
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
			row.NeedsMonthChoice = decision.TenantID != 0
		}
	}
	if row.NeedsMonthChoice && len(row.MonthOptions) == 0 {
		row.MonthOptions = rentMonthOptionsForTenant(transaction, obligations, row.TenantID)
	}
	contextTenantID := row.CandidateTenantID
	if contextTenantID == 0 {
		contextTenantID = row.TenantID
	}
	if contextTenant, ok := tenantByID[contextTenantID]; ok {
		row.ObjectLabel = transactionTenantObjectLabel(contextTenant)
		if strings.TrimSpace(contextTenant.RoomLabel) != "" {
			row.RoomOnlyLabel = "房间 " + strings.TrimSpace(contextTenant.RoomLabel)
		}
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

func transactionTenantObjectLabel(row tenant) string {
	property := strings.TrimSpace(row.RoomAddress)
	if property == "" {
		property = strings.TrimSpace(stringValue(row.PropertyHint))
	}
	room := strings.TrimSpace(row.RoomLabel)
	if room != "" {
		room = "房间 " + room
	}
	if property == "" {
		return room
	}
	if room == "" {
		return property
	}
	return property + " · " + room
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
