package main

import (
	"context"
	"errors"
	"fmt"
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
	var tenantRow tenant
	if err := s.db.WithContext(ctx).Where("id = ? AND user_id = ?", tenantID, userID).First(&tenantRow).Error; err != nil {
		return err
	}
	var obligations []rentObligation
	if err := s.db.WithContext(ctx).Where("user_id = ? AND tenant_id = ?", userID, tenantID).Order("period_month ASC").Find(&obligations).Error; err != nil {
		return err
	}
	if period == nil {
		return errors.New("rent period is required for manual confirmation")
	}
	decision := decideForTenantInPeriod(paymentTransactionInputFromModel(transaction), tenantRow, obligations, *period, "manual_month")
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
	for _, tenantRow := range tenants {
		tenantNames[tenantRow.ID] = tenantRow.Name
	}
	for _, transaction := range transactions {
		allocations := allocationsByTransaction[transaction.ID]
		row := enrichTransactionPageRow(transactionPageRowFromModel(transaction), transaction, allocations, obligations)
		summary := summarizeTransactionAllocations(transaction, allocations)
		if row.TenantID != 0 && transaction.Direction == "income" && summary.AllocatedCents == 0 && transaction.MatchStatus != "ignored" {
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
				row.NeedsMonthChoice = decision.TenantID != 0
			}
		}
		if row.NeedsMonthChoice && len(row.MonthOptions) == 0 {
			row.MonthOptions = rentMonthOptionsForTenant(transaction, obligations, row.TenantID)
		}
		if transaction.Direction == "income" && summary.AllocatedCents == 0 && transaction.MatchStatus != "ignored" && !row.CanConfirm && !row.NeedsMonthChoice {
			row.ManualMatchOptions = availableRentMatchOptions(transaction, obligations, tenantNames, transaction.AmountCents, 0)
		}
		if effectiveRentAllocation, ok := singleEffectiveRentAllocation(allocations); ok {
			row.CanRematch = true
			row.RematchOptions = availableRentMatchOptions(transaction, obligations, tenantNames, effectiveRentAllocation.AmountCents, effectiveRentAllocation.RentObligationIDValue())
			row.CanEditRentMatch = len(row.RematchOptions) > 0
			row.RematchTenantOptions, row.RematchMonthOptions = rematchFilterOptions(row.RematchOptions)
		}
		rows = append(rows, row)
	}
	return rows, total, nil
}

func availableRentMatchOptions(source paymentTransaction, obligations []rentObligation, tenantNames map[uint64]string, amountCents int64, excludedObligationID uint64) []billingRentMatchOption {
	if amountCents <= 0 {
		return nil
	}
	options := make([]billingRentMatchOption, 0)
	for _, obligation := range obligations {
		if obligation.ID == excludedObligationID || obligation.RecordStatus == obligationRecordVoided || obligation.ExpectedAmountCents-obligation.PaidAmountCents < amountCents || !strings.EqualFold(firstNonEmpty(obligation.Currency, source.Currency), source.Currency) {
			continue
		}
		options = append(options, billingRentMatchOption{
			RentObligationID: obligation.ID,
			TenantID:         obligation.TenantID,
			TenantName:       firstNonEmpty(tenantNames[obligation.TenantID], "租客"),
			Period:           monthStart(obligation.PeriodMonth).Format("2006-01"),
			PeriodLabel:      formatMonthLabel(obligation.PeriodMonth),
			Remaining:        formatMoney(centsToMoney(obligation.ExpectedAmountCents-obligation.PaidAmountCents), source.Currency, 2),
			Label:            fmt.Sprintf("%s · %d年%d月 · 未收 %s", firstNonEmpty(tenantNames[obligation.TenantID], "租客"), obligation.PeriodMonth.Year(), obligation.PeriodMonth.Month(), formatMoney(centsToMoney(obligation.ExpectedAmountCents-obligation.PaidAmountCents), source.Currency, 2)),
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
