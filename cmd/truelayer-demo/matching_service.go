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

func (s *transactionService) reconcileTransactions(ctx context.Context, userID uint64) error {
	if userID == 0 {
		return errors.New("userID is required")
	}
	var transactions []paymentTransaction
	if err := s.db.WithContext(ctx).
		Where("user_id = ? AND direction = ? AND match_status IN ?", userID, "income", pendingMatchStatuses).
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
	obligations := make([]rentObligation, 0)
	for _, row := range transactions {
		if row.TransactionTime == nil {
			continue
		}
		periods := []time.Time{monthStart(*row.TransactionTime)}
		if period, ok := parseReferencedPeriod(row.Description+" "+row.Reference, *row.TransactionTime); ok {
			periods = append(periods, period)
		}
		for _, period := range periods {
			if err := newObligationService(s.db).ensureMonthlyObligations(ctx, userID, period); err != nil {
				return err
			}
		}
	}
	if err := s.db.WithContext(ctx).Where("user_id = ?", userID).Find(&obligations).Error; err != nil {
		return err
	}

	for _, row := range transactions {
		if row.MatchedTenantID != nil {
			continue
		}
		decision := decideRentMatch(paymentTransactionInputFromModel(row), tenants, obligations)
		if decision.Status == "matched" || decision.Status == "partial" {
			if decision.ConfirmationSource != "auto_id" && decision.ConfirmationSource != "auto_name" {
				if err := s.setMatchStatus(ctx, userID, row.ID, "candidate"); err != nil {
					return err
				}
				continue
			}
		}
		if decision.TenantID != 0 {
			if err := s.setMatchedTenant(ctx, userID, row.ID, decision.TenantID); err != nil {
				return err
			}
		}
		if decision.Status == "matched" || decision.Status == "partial" {
			if err := s.applyAllocation(ctx, userID, row, decision, decision.ConfirmationSource); err != nil {
				return err
			}
		} else if err := s.setMatchStatus(ctx, userID, row.ID, decision.Status); err != nil {
			return err
		}
	}
	return nil
}

func (s *transactionService) setMatchedTenant(ctx context.Context, userID, transactionID, tenantID uint64) error {
	return s.db.WithContext(ctx).Model(&paymentTransaction{}).
		Where("id = ? AND user_id = ?", transactionID, userID).
		Update("matched_tenant_id", tenantID).Error
}

func (s *transactionService) setMatchStatus(ctx context.Context, userID, transactionID uint64, status string) error {
	return s.db.WithContext(ctx).Model(&paymentTransaction{}).
		Where("id = ? AND user_id = ?", transactionID, userID).
		Update("match_status", status).Error
}

func (s *transactionService) applyAllocation(ctx context.Context, userID uint64, transaction paymentTransaction, decision matchDecision, source string) error {
	if decision.RentObligationID == 0 || decision.TenantID == 0 {
		return errors.New("rent match target is incomplete")
	}
	_, err := s.allocateTransaction(ctx, userID, transaction.ID, []transactionAllocationDraft{{
		TenantID:         decision.TenantID,
		RentObligationID: decision.RentObligationID,
		AmountCents:      transaction.AmountCents,
		Kind:             allocationKindRent,
	}}, "", source)
	return err
}

func (s *transactionService) confirmRentMatch(ctx context.Context, userID, transactionID, tenantID uint64, period *time.Time) error {
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
		return s.bindPayerAndBatch(ctx, userID, transaction, tenantRow, obligations)
	}
	decision := decideForTenantInPeriod(paymentTransactionInputFromModel(transaction), tenantRow, obligations, *period, "manual_month")
	if decision.Status != "matched" && decision.Status != "partial" {
		return errors.New("selected rent obligation needs review")
	}
	if err := s.applyAllocation(ctx, userID, transaction, decision, "manual_name"); err != nil {
		return err
	}
	if stringValue(tenantRow.PayerID) == "" && stringValue(transaction.PayerID) != "" {
		return s.db.WithContext(ctx).Model(&tenant{}).Where("id = ? AND user_id = ?", tenantID, userID).Update("payer_id", stringValue(transaction.PayerID)).Error
	}
	return nil
}

func (s *transactionService) bindPayerAndBatch(ctx context.Context, userID uint64, source paymentTransaction, tenantRow tenant, obligations []rentObligation) error {
	if source.PayerName == nil || strings.TrimSpace(stringValue(source.PayerName)) == "" {
		decision := decideForTenant(paymentTransactionInputFromModel(source), tenantRow, obligations, "manual_name")
		if decision.Status != "matched" && decision.Status != "partial" {
			if err := s.setMatchedTenant(ctx, userID, source.ID, tenantRow.ID); err != nil {
				return err
			}
			return s.setMatchStatus(ctx, userID, source.ID, "needs_review")
		}
		return s.applyAllocation(ctx, userID, source, decision, "manual_name")
	}

	name := normalizeMatchText(stringValue(source.PayerName))
	var rows []paymentTransaction
	if err := s.db.WithContext(ctx).Where("user_id = ? AND direction = ?", userID, "income").Order("transaction_time ASC, id ASC").Find(&rows).Error; err != nil {
		return err
	}
	if err := s.db.WithContext(ctx).Model(&tenant{}).Where("id = ? AND user_id = ?", tenantRow.ID, userID).Updates(map[string]any{
		"payer_name_hint": stringValue(source.PayerName),
		"payer_id":        firstNonEmpty(stringValue(tenantRow.PayerID), stringValue(source.PayerID)),
	}).Error; err != nil {
		return err
	}
	for _, row := range rows {
		if normalizeMatchText(stringValue(row.PayerName)) != name || row.MatchStatus == "matched" {
			continue
		}
		var freshObligations []rentObligation
		if err := s.db.WithContext(ctx).Where("user_id = ?", userID).Find(&freshObligations).Error; err != nil {
			return err
		}
		decision := decideForTenant(paymentTransactionInputFromModel(row), tenantRow, freshObligations, "manual_name")
		if decision.Status == "matched" || decision.Status == "partial" {
			if err := s.applyAllocation(ctx, userID, row, decision, "manual_name"); err != nil {
				return err
			}
			continue
		}
		if err := s.setMatchedTenant(ctx, userID, row.ID, tenantRow.ID); err != nil {
			return err
		}
		if err := s.setMatchStatus(ctx, userID, row.ID, "needs_review"); err != nil {
			return err
		}
	}
	return nil
}

func (s *transactionService) listTransactionPageRows(ctx context.Context, userID uint64, filters transactionFilters) ([]transactionPageRow, error) {
	if err := s.reconcileTransactions(ctx, userID); err != nil {
		return nil, err
	}
	transactions, err := s.listTransactions(ctx, userID, filters)
	if err != nil {
		return nil, err
	}
	var tenants []tenant
	if err := s.db.WithContext(ctx).Where("user_id = ?", userID).Find(&tenants).Error; err != nil {
		return nil, err
	}
	var obligations []rentObligation
	if err := s.db.WithContext(ctx).Where("user_id = ?", userID).Find(&obligations).Error; err != nil {
		return nil, err
	}
	rows := make([]transactionPageRow, 0, len(transactions))
	for _, transaction := range transactions {
		row := transactionPageRowFromModel(transaction)
		if row.TenantID != 0 && transaction.Direction == "income" && transaction.MatchStatus != "matched" {
			row.NeedsMonthChoice = true
			for _, obligation := range obligations {
				if obligation.TenantID != row.TenantID || obligation.PaidAmountCents >= obligation.ExpectedAmountCents {
					continue
				}
				currency := firstNonEmpty(obligation.Currency, transaction.Currency, "EUR")
				row.MonthOptions = append(row.MonthOptions, billingMonthOption{
					Period:    obligation.PeriodMonth.Format("2006-01"),
					Label:     fmt.Sprintf("%d年%d月", obligation.PeriodMonth.Year(), obligation.PeriodMonth.Month()),
					Expected:  formatMoney(centsToMoney(obligation.ExpectedAmountCents), currency, 2),
					Paid:      formatMoney(centsToMoney(obligation.PaidAmountCents), currency, 2),
					Remaining: formatMoney(centsToMoney(obligation.ExpectedAmountCents-obligation.PaidAmountCents), currency, 2),
				})
			}
		}
		if transaction.Direction == "income" && transaction.MatchStatus == "candidate" {
			decision := decideRentMatch(paymentTransactionInputFromModel(transaction), tenants, obligations)
			if decision.Status == "candidate" {
				row.CandidateTenantID = decision.TenantID
				row.CanConfirm = decision.TenantID != 0
				for _, tenantRow := range tenants {
					if tenantRow.ID == decision.TenantID {
						row.CandidateTenantName = tenantRow.Name
						break
					}
				}
			}
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func transactionID(value string) (uint64, error) {
	return strconv.ParseUint(value, 10, 64)
}
