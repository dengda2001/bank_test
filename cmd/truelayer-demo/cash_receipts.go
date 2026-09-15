package main

import (
	"context"
	"errors"
	"strings"
	"time"

	"gorm.io/gorm"
)

const (
	cashReceiptSource          = "cash"
	cashReceiptStatusConfirmed = "confirmed"
	cashReceiptStatusVoided    = "voided"
)

type cashReceipt struct {
	ID               uint64 `gorm:"primaryKey"`
	UserID           uint64
	TenantID         uint64
	RentObligationID uint64
	ReceiptNumber    string
	AmountCents      int64
	Currency         string
	ReceivedAt       time.Time
	Note             string
	Status           string
	OperationID      string
	IdempotencyKey   *string
	RecordedByUserID uint64
	RecordedAt       time.Time
	VoidedAt         *time.Time
	VoidedByUserID   *uint64
	VoidReason       *string
	CreatedAt        time.Time
	UpdatedAt        time.Time
}

type cashReceiptInput struct {
	UserID           uint64
	TenantID         uint64
	RentObligationID uint64
	AmountCents      int64
	Currency         string
	ReceivedAt       time.Time
	Note             string
	IdempotencyKey   string
}

type cashReceiptService struct {
	db *gorm.DB
}

func newCashReceiptService(db *gorm.DB) *cashReceiptService {
	return &cashReceiptService{db: db}
}

func validateCashReceiptInput(input cashReceiptInput) error {
	if input.UserID == 0 {
		return errors.New("userID is required")
	}
	if input.TenantID == 0 || input.RentObligationID == 0 {
		return errors.New("tenant and rent obligation are required")
	}
	if input.AmountCents <= 0 {
		return errors.New("cash receipt amount must be positive")
	}
	if _, err := normalizeLedgerCurrency(input.Currency); err != nil {
		return err
	}
	if input.ReceivedAt.IsZero() {
		return errors.New("cash receipt date is required")
	}
	if len([]rune(strings.TrimSpace(input.Note))) > 512 {
		return errors.New("cash receipt note is too long")
	}
	if strings.TrimSpace(input.IdempotencyKey) == "" {
		return errors.New("cash receipt idempotency key is required")
	}
	if len([]rune(strings.TrimSpace(input.IdempotencyKey))) > 191 {
		return errors.New("cash receipt idempotency key is too long")
	}
	return nil
}

func cashReceiptIsEffective(row cashReceipt) bool {
	if row.Status != cashReceiptStatusConfirmed || row.AmountCents <= 0 {
		return false
	}
	if _, err := normalizeLedgerCurrency(row.Currency); err != nil {
		return false
	}
	return true
}

func cashReceiptPaidAmount(rows []cashReceipt) int64 {
	var total int64
	for _, row := range rows {
		if cashReceiptIsEffective(row) {
			total += row.AmountCents
		}
	}
	return total
}

func projectRentObligation(obligation rentObligation, bankAllocations []paymentAllocation, cashReceipts []cashReceipt, now time.Time) rentObligation {
	bankPaid := ledgerPaidAmount(bankAllocations)
	cashPaid := int64(0)
	for _, receipt := range cashReceipts {
		if receipt.TenantID != obligation.TenantID {
			continue
		}
		if !strings.EqualFold(strings.TrimSpace(receipt.Currency), strings.TrimSpace(obligation.Currency)) {
			continue
		}
		if cashReceiptIsEffective(receipt) {
			cashPaid += receipt.AmountCents
		}
	}
	obligation.PaidAmountCents = bankPaid + cashPaid
	obligation.Status = ledgerObligationStatus(obligation.ExpectedAmountCents, obligation.PaidAmountCents, obligation.DueDate, now, obligation.RecordStatus)
	return obligation
}

func (s *cashReceiptService) loadRentObligationSources(ctx context.Context, db *gorm.DB, userID uint64, obligationID uint64) (rentObligation, []paymentAllocation, []cashReceipt, error) {
	var obligation rentObligation
	if err := db.WithContext(ctx).Where("id = ? AND user_id = ?", obligationID, userID).First(&obligation).Error; err != nil {
		return rentObligation{}, nil, nil, err
	}
	var bankAllocations []paymentAllocation
	if err := db.WithContext(ctx).Where("rent_obligation_id = ? AND user_id = ?", obligationID, userID).Find(&bankAllocations).Error; err != nil {
		return rentObligation{}, nil, nil, err
	}
	var cashReceipts []cashReceipt
	if err := db.WithContext(ctx).Where("rent_obligation_id = ? AND user_id = ?", obligationID, userID).Find(&cashReceipts).Error; err != nil {
		return rentObligation{}, nil, nil, err
	}
	return obligation, bankAllocations, cashReceipts, nil
}
