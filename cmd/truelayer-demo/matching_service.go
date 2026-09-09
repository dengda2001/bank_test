package main

import (
	"context"
	"errors"
	"strconv"
	"time"

	"gorm.io/gorm"
)

type paymentAllocation struct {
	ID                   uint64 `gorm:"primaryKey"`
	UserID               uint64
	PaymentTransactionID uint64
	RentObligationID     uint64
	TenantID             uint64
	AmountCents          int64
	Status               string
	ConfirmedByUserID    uint64
	ConfirmedAt          time.Time
	ConfirmationSource   string
	CreatedAt            time.Time
}

func (s *transactionService) reconcileTransactions(ctx context.Context, userID uint64) error {
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
		decision := decideRentMatch(paymentTransactionInputFromModel(row), tenants, obligations)
		switch decision.Status {
		case "matched", "partial":
			if decision.ConfirmationSource != "auto_id" {
				if err := s.setMatchStatus(ctx, userID, row.ID, "candidate"); err != nil {
					return err
				}
				continue
			}
			if err := s.applyAllocation(ctx, userID, row, decision, "auto_id"); err != nil {
				return err
			}
		case "candidate", "needs_review", "unmatched":
			if err := s.setMatchStatus(ctx, userID, row.ID, decision.Status); err != nil {
				return err
			}
		}
	}
	return nil
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
	return s.db.WithContext(ctx).Transaction(func(txdb *gorm.DB) error {
		var existing paymentAllocation
		err := txdb.Where("user_id = ? AND payment_transaction_id = ? AND rent_obligation_id = ?", userID, transaction.ID, decision.RentObligationID).First(&existing).Error
		if err == nil {
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		var obligation rentObligation
		if err := txdb.Where("id = ? AND user_id = ? AND tenant_id = ?", decision.RentObligationID, userID, decision.TenantID).First(&obligation).Error; err != nil {
			return err
		}
		remaining := obligation.ExpectedAmountCents - obligation.PaidAmountCents
		if transaction.AmountCents <= 0 || transaction.AmountCents > remaining {
			return errors.New("rent transaction amount needs review")
		}
		allocation := paymentAllocation{
			UserID:               userID,
			PaymentTransactionID: transaction.ID,
			RentObligationID:     obligation.ID,
			TenantID:             decision.TenantID,
			AmountCents:          transaction.AmountCents,
			Status:               "confirmed",
			ConfirmedByUserID:    userID,
			ConfirmedAt:          time.Now().UTC(),
			ConfirmationSource:   source,
		}
		if err := txdb.Create(&allocation).Error; err != nil {
			return err
		}
		paid := obligation.PaidAmountCents + allocation.AmountCents
		status := obligationStatus(obligation.ExpectedAmountCents, paid, obligation.DueDate, time.Now().UTC(), false)
		if err := txdb.Model(&rentObligation{}).Where("id = ? AND user_id = ?", obligation.ID, userID).Updates(map[string]any{
			"paid_amount_cents": paid,
			"status":            status,
		}).Error; err != nil {
			return err
		}
		if err := txdb.Model(&paymentTransaction{}).Where("id = ? AND user_id = ?", transaction.ID, userID).Update("match_status", "matched").Error; err != nil {
			return err
		}
		return nil
	})
}

func (s *transactionService) confirmRentMatch(ctx context.Context, userID, transactionID, tenantID uint64) error {
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
	decision := decideForTenant(paymentTransactionInputFromModel(transaction), tenantRow, obligations, "manual_name")
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
