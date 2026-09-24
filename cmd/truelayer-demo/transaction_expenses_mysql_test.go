package main

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"testing"
	"time"

	"gorm.io/gorm"
)

func TestBankExpenseAttributionIsEditableAndDoesNotDuplicateDebitOnMySQL(t *testing.T) {
	db, sqlDB := openLedgerMySQLTestDB(t)
	if err := runMigrations(sqlDB, "../../migrations"); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	owner := user{Username: fmt.Sprintf("expense-link-owner-%d", time.Now().UnixNano()), PasswordHash: "test"}
	other := user{Username: fmt.Sprintf("expense-link-other-%d", time.Now().UnixNano()), PasswordHash: "test"}
	for _, row := range []*user{&owner, &other} {
		if err := db.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		_ = db.Where("user_id IN ?", []uint64{owner.ID, other.ID}).Delete(&manualExpenseInvoice{}).Error
		_ = db.Where("user_id IN ?", []uint64{owner.ID, other.ID}).Delete(&manualExpense{}).Error
		_ = db.Where("user_id IN ?", []uint64{owner.ID, other.ID}).Delete(&paymentTransaction{}).Error
		_ = db.Where("user_id = ?", owner.ID).Delete(&room{}).Error
		_ = db.Where("user_id = ?", owner.ID).Delete(&property{}).Error
		_ = db.Delete(&user{}, owner.ID).Error
		_ = db.Delete(&user{}, other.ID).Error
	})
	first := property{UserID: owner.ID, Name: "First", Timezone: "Europe/Dublin", Status: "active"}
	second := property{UserID: owner.ID, Name: "Second", Timezone: "Europe/Dublin", Status: "active"}
	for _, row := range []*property{&first, &second} {
		if err := db.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	secondRoom := room{UserID: owner.ID, PropertyID: second.ID, RoomLabel: "02", Status: "active"}
	if err := db.Create(&secondRoom).Error; err != nil {
		t.Fatal(err)
	}
	date := time.Date(2026, 9, 13, 10, 0, 0, 0, time.UTC)
	source := paymentTransaction{UserID: owner.ID, Source: "truelayer", StableTransactionKey: fmt.Sprintf("expense-bank-%d", time.Now().UnixNano()), Direction: "expense", AmountCents: 7799, Currency: "EUR", TransactionTime: &date, Description: "Gas bill", MatchStatus: "unmatched"}
	otherSource := paymentTransaction{UserID: other.ID, Source: "truelayer", StableTransactionKey: fmt.Sprintf("expense-other-%d", time.Now().UnixNano()), Direction: "expense", AmountCents: 1000, Currency: "EUR", TransactionTime: &date, MatchStatus: "unmatched"}
	for _, row := range []*paymentTransaction{&source, &otherSource} {
		if err := db.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	pending, err := newTransactionService(db).listTransactionPageRows(ctx, owner.ID, transactionFilters{Page: 1, PageSize: 10, Direction: "expense", PendingOnly: true})
	if err != nil || len(pending) != 1 || pending[0].ID != fmt.Sprint(source.ID) {
		t.Fatalf("pending expense before attribution = %+v, %v", pending, err)
	}
	service := newExpenseService(db)
	if _, err := service.saveTransactionExpense(ctx, owner.ID, transactionExpenseInput{TransactionID: otherSource.ID, PropertyID: first.ID, Category: "水电"}); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("cross-account debit: %v", err)
	}
	if _, err := service.saveTransactionExpense(ctx, owner.ID, transactionExpenseInput{TransactionID: source.ID, PropertyID: first.ID, RoomID: &secondRoom.ID, Category: "水电"}); !errors.Is(err, errInvalidTransactionExpense) {
		t.Fatalf("room from another property: %v", err)
	}
	before := int64(0)
	if err := db.Model(&paymentTransaction{}).Where("user_id = ?", owner.ID).Count(&before).Error; err != nil {
		t.Fatal(err)
	}
	saved, err := service.saveTransactionExpense(ctx, owner.ID, transactionExpenseInput{TransactionID: source.ID, PropertyID: first.ID, Category: "水电"})
	if err != nil {
		t.Fatal(err)
	}
	if saved.PaymentTransactionID == nil || *saved.PaymentTransactionID != source.ID || saved.RoomID != nil || saved.AmountCents != source.AmountCents {
		t.Fatalf("property-only attribution = %+v", saved)
	}
	if _, err := service.saveTransactionExpense(ctx, owner.ID, transactionExpenseInput{TransactionID: source.ID, PropertyID: first.ID, Category: "水电"}); err != nil {
		t.Fatalf("repeat save: %v", err)
	}
	invoice, err := parseExpenseInvoiceUpload(url.Values{"invoice_number": {"GAS-1"}, "vendor": {"Gas Co"}, "invoice_date": {"2026-09-13"}, "invoice_amount": {"77.99"}}, []byte("%PDF-1.7\ninvoice"), "gas.pdf")
	if err != nil {
		t.Fatal(err)
	}
	edited, err := service.saveTransactionExpense(ctx, owner.ID, transactionExpenseInput{TransactionID: source.ID, PropertyID: second.ID, RoomID: &secondRoom.ID, Category: "维修", Invoice: &invoice})
	if err != nil {
		t.Fatal(err)
	}
	if edited.ID != saved.ID || edited.RoomID == nil || *edited.RoomID != secondRoom.ID || edited.Category != "维修" {
		t.Fatalf("edited attribution = %+v", edited)
	}
	var debitCount, expenseCount, currentInvoiceCount int64
	if err := db.Model(&paymentTransaction{}).Where("user_id = ?", owner.ID).Count(&debitCount).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&manualExpense{}).Where("user_id = ? AND payment_transaction_id = ?", owner.ID, source.ID).Count(&expenseCount).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.Model(&manualExpenseInvoice{}).Where("user_id = ? AND expense_id = ? AND is_current = ?", owner.ID, saved.ID, true).Count(&currentInvoiceCount).Error; err != nil {
		t.Fatal(err)
	}
	if debitCount != before || expenseCount != 1 || currentInvoiceCount != 1 {
		t.Fatalf("counts: debit=%d before=%d expense=%d current invoice=%d", debitCount, before, expenseCount, currentInvoiceCount)
	}
	pending, err = newTransactionService(db).listTransactionPageRows(ctx, owner.ID, transactionFilters{Page: 1, PageSize: 10, Direction: "expense", PendingOnly: true})
	if err != nil || len(pending) != 0 {
		t.Fatalf("pending expense after attribution = %+v, %v", pending, err)
	}
	expenses, err := service.listExpenses(ctx, owner.ID)
	if err != nil || len(expenses) != 1 || expenses[0].PropertyID != fmt.Sprint(second.ID) || expenses[0].RoomID != fmt.Sprint(secondRoom.ID) {
		t.Fatalf("expense ledger after edit = %+v, %v", expenses, err)
	}
	totalsByRoom, totalsByProperty := workspaceExpenseTotals(rentWorkspaceInput{UserID: owner.ID, Expenses: []manualExpense{edited}}, map[uint64]property{first.ID: first, second.ID: second}, map[uint64]uint64{secondRoom.ID: second.ID}, date)
	if totalsByProperty[second.ID] != source.AmountCents || totalsByRoom[secondRoom.ID] != source.AmountCents || totalsByProperty[first.ID] != 0 {
		t.Fatalf("expense totals after edit: properties=%v rooms=%v", totalsByProperty, totalsByRoom)
	}
	replacement, err := parseExpenseInvoiceUpload(url.Values{"invoice_number": {"GAS-2"}, "vendor": {"Gas Co"}, "invoice_date": {"2026-09-14"}, "invoice_amount": {"77.99"}}, []byte("%PDF-1.7\nreplacement"), "new.pdf")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.saveTransactionExpense(ctx, owner.ID, transactionExpenseInput{TransactionID: source.ID, PropertyID: second.ID, RoomID: &secondRoom.ID, Category: "维修", Invoice: &replacement}); err != nil {
		t.Fatal(err)
	}
	var invoiceHistory []manualExpenseInvoice
	if err := db.Where("user_id = ? AND expense_id = ?", owner.ID, saved.ID).Order("id ASC").Find(&invoiceHistory).Error; err != nil {
		t.Fatal(err)
	}
	if len(invoiceHistory) != 2 || invoiceHistory[0].IsCurrent || invoiceHistory[0].ReplacedAt == nil || !invoiceHistory[1].IsCurrent {
		t.Fatalf("invoice replacement history = %+v", invoiceHistory)
	}
	var reloaded paymentTransaction
	if err := db.Where("id = ? AND user_id = ?", source.ID, owner.ID).First(&reloaded).Error; err != nil {
		t.Fatal(err)
	}
	if reloaded.MatchStatus != "matched" {
		t.Fatalf("bank debit status = %q", reloaded.MatchStatus)
	}
	rows, err := newTransactionService(db).listTransactionPageRows(ctx, owner.ID, transactionFilters{Page: 1, PageSize: 10, Direction: "expense"})
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || !rows[0].ExpenseLinked || !rows[0].ExpenseInvoiceLinked || rows[0].ObjectLabel != "Second · 02" || rows[0].MatchStatusLabel != "已关联" {
		t.Fatalf("expense list row = %+v", rows)
	}
	manual, err := service.createExpense(ctx, owner.ID, expenseInput{
		PropertyID: &first.ID, Description: "Cash cleaning", Category: "清洁", Amount: 12.34,
		Currency: "EUR", ExpenseDate: "2026-09-13", PaymentMethod: "现金",
	})
	if err != nil {
		t.Fatal(err)
	}
	if manual.PaymentTransactionID == nil {
		t.Fatal("manual expense has no linked source transaction")
	}
	var manualSource paymentTransaction
	if err := db.Where("id = ? AND user_id = ?", *manual.PaymentTransactionID, owner.ID).First(&manualSource).Error; err != nil {
		t.Fatal(err)
	}
	if manualSource.Source != "manual_expense" || manualSource.MatchStatus != "matched" || manualSource.AmountCents != manual.AmountCents {
		t.Fatalf("manual expense source = %+v", manualSource)
	}
}
