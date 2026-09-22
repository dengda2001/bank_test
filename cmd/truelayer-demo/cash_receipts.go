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

const (
	cashReceiptSource          = "cash"
	cashReceiptStatusConfirmed = "confirmed"
	cashReceiptStatusVoided    = "voided"
)

var errCashReceiptOverbalance = errors.New("cash receipt exceeds rent obligation balance")

type cashReceipt struct {
	ID                   uint64 `gorm:"primaryKey"`
	UserID               uint64
	PaymentTransactionID *uint64
	PayerTenantID        *uint64
	PayerNameSnapshot    *string
	RentObligationID     uint64
	ReceiptNumber        string
	AmountCents          int64
	Currency             string
	ReceivedAt           time.Time
	Note                 string
	Status               string
	OperationID          string
	VoidOperationID      *string
	IdempotencyKey       *string
	RecordedByUserID     uint64
	RecordedAt           time.Time
	VoidedAt             *time.Time
	VoidedByUserID       *uint64
	VoidReason           *string
	CreatedAt            time.Time
	UpdatedAt            time.Time
}

type cashReceiptInput struct {
	UserID           uint64
	TenantID         uint64
	PayerTenantID    *uint64
	PayerName        string
	RentObligationID uint64
	AmountCents      int64
	Currency         string
	ReceivedAt       time.Time
	Note             string
	IdempotencyKey   string
}

type cashReceiptPreview struct {
	Tenant                tenant
	Obligation            rentObligation
	Input                 cashReceiptInput
	CurrentPaidCents      int64
	CurrentRemainingCents int64
	AfterPaidCents        int64
	AfterRemainingCents   int64
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
	if len([]rune(strings.TrimSpace(input.PayerName))) > 191 {
		return errors.New("cash payer name is too long")
	}
	if input.PayerTenantID != nil && *input.PayerTenantID == 0 {
		return errors.New("cash payer tenant is invalid")
	}
	if input.PayerTenantID != nil && strings.TrimSpace(input.PayerName) != "" {
		return errors.New("choose a tenant payer or enter a payer name, not both")
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

func (s *cashReceiptService) previewCashReceipt(ctx context.Context, input cashReceiptInput) (cashReceiptPreview, error) {
	if err := validateCashReceiptInput(input); err != nil {
		return cashReceiptPreview{}, err
	}
	if s == nil || s.db == nil {
		return cashReceiptPreview{}, errors.New("database is required")
	}
	var tenantRow tenant
	if err := s.db.WithContext(ctx).Where("id = ? AND user_id = ?", input.TenantID, input.UserID).First(&tenantRow).Error; err != nil {
		return cashReceiptPreview{}, err
	}
	obligation, bankAllocations, cashReceipts, err := s.loadRentObligationSources(ctx, s.db, input.UserID, input.RentObligationID)
	if err != nil {
		return cashReceiptPreview{}, err
	}
	if obligation.TenantID != input.TenantID {
		return cashReceiptPreview{}, errors.New("cash receipt tenant does not match rent obligation")
	}
	if input.PayerTenantID != nil {
		var payer tenant
		if err := s.db.WithContext(ctx).Where("id = ? AND user_id = ?", *input.PayerTenantID, input.UserID).First(&payer).Error; err != nil {
			return cashReceiptPreview{}, err
		}
	}
	if obligation.RecordStatus == obligationRecordVoided {
		return cashReceiptPreview{}, errors.New("rent obligation is voided")
	}
	if _, err := normalizeLedgerCurrency(obligation.Currency); err != nil {
		return cashReceiptPreview{}, err
	}
	projected := projectRentObligation(obligation, bankAllocations, cashReceipts, time.Now().UTC())
	if projected.PaidAmountCents > obligation.ExpectedAmountCents {
		return cashReceiptPreview{}, errCashReceiptOverbalance
	}
	remaining := maxInt64(obligation.ExpectedAmountCents-projected.PaidAmountCents, 0)
	if input.AmountCents > remaining {
		return cashReceiptPreview{}, errCashReceiptOverbalance
	}
	return cashReceiptPreview{
		Tenant:                tenantRow,
		Obligation:            obligation,
		Input:                 input,
		CurrentPaidCents:      projected.PaidAmountCents,
		CurrentRemainingCents: remaining,
		AfterPaidCents:        projected.PaidAmountCents + input.AmountCents,
		AfterRemainingCents:   remaining - input.AmountCents,
	}, nil
}

func (s *cashReceiptService) recordCashReceipt(ctx context.Context, input cashReceiptInput) (cashReceipt, error) {
	if err := validateCashReceiptInput(input); err != nil {
		return cashReceipt{}, err
	}
	if s == nil || s.db == nil {
		return cashReceipt{}, errors.New("database is required")
	}
	var receipt cashReceipt
	err := s.db.WithContext(ctx).Transaction(func(txdb *gorm.DB) error {
		if _, _, err := lockRoomForRentObligation(txdb, input.UserID, input.RentObligationID); err != nil {
			return err
		}
		var obligation rentObligation
		if err := txdb.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND user_id = ? AND tenant_id = ?", input.RentObligationID, input.UserID, input.TenantID).First(&obligation).Error; err != nil {
			return err
		}
		if obligation.RecordStatus == obligationRecordVoided {
			return errors.New("rent obligation is voided")
		}
		if input.PayerTenantID != nil {
			var payer tenant
			if err := txdb.Where("id = ? AND user_id = ?", *input.PayerTenantID, input.UserID).First(&payer).Error; err != nil {
				return errors.New("cash payer tenant does not belong to current user")
			}
		}
		if _, err := normalizeLedgerCurrency(obligation.Currency); err != nil {
			return err
		}
		if !strings.EqualFold(strings.TrimSpace(input.Currency), strings.TrimSpace(obligation.Currency)) {
			return errors.New("cash receipt currency does not match rent obligation")
		}

		key := strings.TrimSpace(input.IdempotencyKey)
		var previous cashReceipt
		if err := txdb.Where("user_id = ? AND idempotency_key = ?", input.UserID, key).First(&previous).Error; err == nil {
			payerTenantID, payerNameSnapshot := cashReceiptPayerFacts(input)
			if previous.RentObligationID != input.RentObligationID || previous.AmountCents != input.AmountCents || !strings.EqualFold(previous.Currency, input.Currency) || !sameDate(previous.ReceivedAt, input.ReceivedAt) || strings.TrimSpace(previous.Note) != strings.TrimSpace(input.Note) || !sameOptionalUint64(previous.PayerTenantID, payerTenantID) || stringValue(previous.PayerNameSnapshot) != stringValue(payerNameSnapshot) {
				return errors.New("cash receipt idempotency key was already used for different facts")
			}
			receipt = previous
			return nil
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}

		var bankAllocations []paymentAllocation
		if err := txdb.Where("rent_obligation_id = ? AND user_id = ?", obligation.ID, input.UserID).Find(&bankAllocations).Error; err != nil {
			return err
		}
		var cashReceipts []cashReceipt
		if err := txdb.Where("rent_obligation_id = ? AND user_id = ?", obligation.ID, input.UserID).Find(&cashReceipts).Error; err != nil {
			return err
		}
		projected := projectRentObligation(obligation, bankAllocations, cashReceipts, time.Now().UTC())
		remaining := maxInt64(obligation.ExpectedAmountCents-projected.PaidAmountCents, 0)
		if projected.PaidAmountCents > obligation.ExpectedAmountCents || input.AmountCents > remaining {
			return errCashReceiptOverbalance
		}
		now := time.Now().UTC()
		receivedAt := dateOnly(input.ReceivedAt)
		payerTenantID, payerNameSnapshot := cashReceiptPayerFacts(input)
		receipt = cashReceipt{
			UserID:            input.UserID,
			PayerTenantID:     payerTenantID,
			PayerNameSnapshot: payerNameSnapshot,
			RentObligationID:  input.RentObligationID,
			ReceiptNumber:     recordID("cash", now),
			AmountCents:       input.AmountCents,
			Currency:          ledgerCurrencyEUR,
			ReceivedAt:        receivedAt,
			Note:              strings.TrimSpace(input.Note),
			Status:            cashReceiptStatusConfirmed,
			OperationID:       recordID("cash-receipt", now),
			IdempotencyKey:    nullableString(key),
			RecordedByUserID:  input.UserID,
			RecordedAt:        now,
		}
		if err := txdb.Create(&receipt).Error; err != nil {
			return err
		}
		cashReceipts = append(cashReceipts, receipt)
		projected = projectRentObligation(obligation, bankAllocations, cashReceipts, now)
		if err := txdb.Model(&rentObligation{}).Where("id = ? AND user_id = ?", obligation.ID, input.UserID).Updates(map[string]any{
			"paid_amount_cents": projected.PaidAmountCents,
			"status":            projected.Status,
		}).Error; err != nil {
			return err
		}
		return nil
	})
	return receipt, err
}

func cashReceiptPayerFacts(input cashReceiptInput) (*uint64, *string) {
	if input.PayerTenantID != nil {
		payerTenantID := *input.PayerTenantID
		return &payerTenantID, nil
	}
	if payerName := strings.TrimSpace(input.PayerName); payerName != "" {
		return nil, nullableString(payerName)
	}
	payerTenantID := input.TenantID
	return &payerTenantID, nil
}

func sameOptionalUint64(left, right *uint64) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}

func (s *cashReceiptService) voidCashReceipt(ctx context.Context, userID, receiptID uint64, reason string) (cashReceipt, error) {
	if userID == 0 || receiptID == 0 {
		return cashReceipt{}, errors.New("userID and receiptID are required")
	}
	reason, err := normalizeTransactionActionReason(reason)
	if err != nil {
		return cashReceipt{}, err
	}
	if s == nil || s.db == nil {
		return cashReceipt{}, errors.New("database is required")
	}
	var receipt cashReceipt
	err = s.db.WithContext(ctx).Transaction(func(txdb *gorm.DB) error {
		var receiptRef cashReceipt
		if err := txdb.Where("id = ? AND user_id = ?", receiptID, userID).First(&receiptRef).Error; err != nil {
			return err
		}
		obligationRef, _, err := lockRoomForRentObligation(txdb, userID, receiptRef.RentObligationID)
		if err != nil {
			return err
		}
		var obligation rentObligation
		if err := txdb.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND user_id = ? AND tenant_id = ?", receiptRef.RentObligationID, userID, obligationRef.TenantID).First(&obligation).Error; err != nil {
			return err
		}
		if err := txdb.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND user_id = ?", receiptID, userID).First(&receipt).Error; err != nil {
			return err
		}
		if receipt.Status == cashReceiptStatusVoided {
			return nil
		}
		if receipt.Status != cashReceiptStatusConfirmed {
			return fmt.Errorf("cash receipt status %q cannot be voided", receipt.Status)
		}
		now := time.Now().UTC()
		voidOperationID := recordID("cash-void", now)
		if err := txdb.Model(&cashReceipt{}).Where("id = ? AND user_id = ? AND status = ?", receipt.ID, userID, cashReceiptStatusConfirmed).Updates(map[string]any{
			"status":            cashReceiptStatusVoided,
			"voided_at":         now,
			"voided_by_user_id": userID,
			"void_reason":       reason,
			"void_operation_id": voidOperationID,
		}).Error; err != nil {
			return err
		}
		var bankAllocations []paymentAllocation
		if err := txdb.Where("rent_obligation_id = ? AND user_id = ?", obligation.ID, userID).Find(&bankAllocations).Error; err != nil {
			return err
		}
		var cashReceipts []cashReceipt
		if err := txdb.Where("rent_obligation_id = ? AND user_id = ?", obligation.ID, userID).Find(&cashReceipts).Error; err != nil {
			return err
		}
		projected := projectRentObligation(obligation, bankAllocations, cashReceipts, now)
		if err := txdb.Model(&rentObligation{}).Where("id = ? AND user_id = ?", obligation.ID, userID).Updates(map[string]any{
			"paid_amount_cents": projected.PaidAmountCents,
			"status":            projected.Status,
		}).Error; err != nil {
			return err
		}
		receipt.Status = cashReceiptStatusVoided
		receipt.VoidedAt = &now
		receipt.VoidedByUserID = &userID
		receipt.VoidReason = nullableString(reason)
		receipt.VoidOperationID = &voidOperationID
		return nil
	})
	return receipt, err
}

func sameDate(left, right time.Time) bool {
	return dateOnly(left).Equal(dateOnly(right))
}

func dateOnly(value time.Time) time.Time {
	utc := value.UTC()
	return time.Date(utc.Year(), utc.Month(), utc.Day(), 0, 0, 0, 0, time.UTC)
}
