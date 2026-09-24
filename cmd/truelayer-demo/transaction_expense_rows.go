package main

import (
	"context"
	"strings"
)

// decorateExpensePageRows projects one expense fact onto each debit in the
// transaction list. Property and room totals continue to read the same fact.
func (s *transactionService) decorateExpensePageRows(ctx context.Context, userID uint64, sources []paymentTransaction, rows []transactionPageRow) error {
	ids := make([]uint64, 0)
	for _, source := range sources {
		if source.Direction == "expense" {
			ids = append(ids, source.ID)
		}
	}
	if len(ids) == 0 {
		return nil
	}
	var expenses []manualExpense
	if err := s.db.WithContext(ctx).Where("user_id = ? AND payment_transaction_id IN ? AND record_status = ?", userID, ids, obligationRecordActive).Find(&expenses).Error; err != nil {
		return err
	}
	expenseByTransaction := make(map[uint64]manualExpense, len(expenses))
	propertyIDs, roomIDs, expenseIDs := make([]uint64, 0, len(expenses)), make([]uint64, 0, len(expenses)), make([]uint64, 0, len(expenses))
	for _, expense := range expenses {
		if expense.PaymentTransactionID == nil {
			continue
		}
		expenseByTransaction[*expense.PaymentTransactionID] = expense
		expenseIDs = append(expenseIDs, expense.ID)
		if expense.PropertyID != nil {
			propertyIDs = append(propertyIDs, *expense.PropertyID)
		}
		if expense.RoomID != nil {
			roomIDs = append(roomIDs, *expense.RoomID)
		}
	}
	propertyNames := make(map[uint64]string)
	if len(propertyIDs) > 0 {
		var properties []property
		if err := s.db.WithContext(ctx).Where("user_id = ? AND id IN ?", userID, propertyIDs).Find(&properties).Error; err != nil {
			return err
		}
		for _, propertyRow := range properties {
			propertyNames[propertyRow.ID] = propertyRow.Name
		}
	}
	roomLabels := make(map[uint64]string)
	if len(roomIDs) > 0 {
		var rooms []room
		if err := s.db.WithContext(ctx).Where("user_id = ? AND id IN ?", userID, roomIDs).Find(&rooms).Error; err != nil {
			return err
		}
		for _, roomRow := range rooms {
			roomLabels[roomRow.ID] = roomRow.RoomLabel
		}
	}
	invoices := make(map[uint64]bool)
	if len(expenseIDs) > 0 {
		var current []manualExpenseInvoice
		if err := s.db.WithContext(ctx).Select("expense_id").Where("user_id = ? AND expense_id IN ? AND is_current = ?", userID, expenseIDs, true).Find(&current).Error; err != nil {
			return err
		}
		for _, invoice := range current {
			invoices[invoice.ExpenseID] = true
		}
	}
	for index, source := range sources {
		if source.Direction != "expense" {
			continue
		}
		row := &rows[index]
		row.AllocationUseDisplay = "未归属"
		expense, ok := expenseByTransaction[source.ID]
		if !ok {
			continue
		}
		row.ExpenseLinked = true
		row.ExpenseCategory = expense.Category
		row.ExpenseInvoiceLinked = invoices[expense.ID]
		row.MatchStatus = "matched"
		row.MatchStatusLabel = "已关联"
		row.AllocationUseDisplay = "支出 · " + expense.Category
		parts := make([]string, 0, 2)
		if expense.PropertyID != nil {
			parts = append(parts, propertyNames[*expense.PropertyID])
		}
		if expense.RoomID != nil {
			parts = append(parts, roomLabels[*expense.RoomID])
		}
		row.ObjectLabel = strings.Join(parts, " · ")
	}
	return nil
}
