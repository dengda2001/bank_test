package main

import (
	"context"
	"errors"
	"strings"
	"unicode/utf8"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

var errInvalidTransactionExpense = errors.New("invalid transaction expense attribution")

type transactionExpenseInput struct {
	TransactionID uint64
	PropertyID    uint64
	RoomID        *uint64
	Category      string
	Invoice       *manualExpenseInvoice
}

// saveTransactionExpense connects one debit to one expense fact. The bank
// transaction remains the only money movement; no synthetic debit is created.
func (s *expenseService) saveTransactionExpense(ctx context.Context, userID uint64, input transactionExpenseInput) (manualExpense, error) {
	if s == nil || s.db == nil || userID == 0 || input.TransactionID == 0 || input.PropertyID == 0 {
		return manualExpense{}, errInvalidTransactionExpense
	}
	input.Category = strings.TrimSpace(input.Category)
	if input.Category == "" || utf8.RuneCountInString(input.Category) > 191 {
		return manualExpense{}, errInvalidTransactionExpense
	}
	var saved manualExpense
	err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var source paymentTransaction
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("id = ? AND user_id = ?", input.TransactionID, userID).First(&source).Error; err != nil {
			return err
		}
		if source.Direction != "expense" || (source.Source != "truelayer" && source.Source != "manual_expense") {
			return errInvalidTransactionExpense
		}
		repo := newLandlordRentRepository(tx)
		if _, err := repo.findProperty(ctx, userID, input.PropertyID); err != nil {
			return err
		}
		if input.RoomID != nil {
			if *input.RoomID == 0 {
				return errInvalidTransactionExpense
			}
			roomRow, err := repo.findRoom(ctx, userID, *input.RoomID)
			if err != nil {
				return err
			}
			if roomRow.PropertyID != input.PropertyID {
				return errInvalidTransactionExpense
			}
		}
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).Where("user_id = ? AND payment_transaction_id = ?", userID, source.ID).First(&saved).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			if source.Source != "truelayer" || source.TransactionTime == nil || source.AmountCents <= 0 || source.MatchStatus != "unmatched" {
				return errInvalidTransactionExpense
			}
			saved = manualExpense{
				UserID: userID, PaymentTransactionID: &source.ID,
				PropertyID: &input.PropertyID, RoomID: input.RoomID,
				Description: firstNonEmpty(strings.TrimSpace(source.Description), strings.TrimSpace(source.Reference), "银行支出"),
				Category:    input.Category, AmountCents: source.AmountCents, Currency: source.Currency,
				ExpenseDate: dunningDublinDate(*source.TransactionTime), PaymentMethod: "银行流水", RecordStatus: obligationRecordActive,
			}
			if err := tx.Create(&saved).Error; err != nil {
				return err
			}
		} else if err != nil {
			return err
		} else {
			if saved.RecordStatus != obligationRecordActive {
				return errInvalidTransactionExpense
			}
			if err := tx.Model(&saved).Updates(map[string]any{
				"property_id": input.PropertyID, "room_id": input.RoomID, "category": input.Category,
			}).Error; err != nil {
				return err
			}
			saved.PropertyID, saved.RoomID, saved.Category = &input.PropertyID, input.RoomID, input.Category
		}
		if source.MatchStatus != "matched" {
			if err := tx.Model(&paymentTransaction{}).Where("id = ? AND user_id = ?", source.ID, userID).Update("match_status", "matched").Error; err != nil {
				return err
			}
		}
		if input.Invoice != nil {
			invoice := *input.Invoice
			invoice.ExpenseID = saved.ID
			if err := bindExpenseInvoiceInTx(tx, userID, invoice); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return manualExpense{}, err
	}
	return saved, nil
}
