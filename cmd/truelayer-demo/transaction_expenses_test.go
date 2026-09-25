package main

import (
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTransactionExpenseActionKeepsFiltersAndClosesCleanly(t *testing.T) {
	query := url.Values{"direction": {"expense"}, "period": {"2026-09"}, "page": {"2"}, "detail": {"4"}, "error": {"old"}}
	got := transactionExpenseActionURL(query, 7)
	parsed, err := url.Parse(got)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Path != "/transactions" || parsed.Query().Get("expense_link") != "7" || parsed.Query().Get("direction") != "expense" || parsed.Query().Get("period") != "2026-09" || parsed.Query().Get("page") != "2" || parsed.Query().Has("detail") || parsed.Query().Has("error") {
		t.Fatalf("expense action URL = %q", got)
	}
	if closed := transactionListURL(parsed.Query()); closed != "/transactions?direction=expense&page=2&period=2026-09" {
		t.Fatalf("expense drawer close URL = %q", closed)
	}
}

func TestTransactionExpenseDrawerAndActionsRenderForBothStates(t *testing.T) {
	page := renderTransactionListPage(t, transactionListPageData{
		ExpenseLinkDrawer: &transactionExpenseDrawerData{
			TransactionID: "7", Description: "Gas bill", Amount: "EUR 77.99", Category: "水电",
			Properties: []expensePropertyOption{{ID: 3, Name: "House"}},
			Rooms:      []expenseRoomOption{{ID: 5, PropertyID: 3, Label: "05"}}, CloseURL: "/transactions?direction=expense",
			PostURL: "/transactions/expense-link?transaction_id=7&return_to=%2Ftransactions%3Fdirection%3Dexpense",
		},
		TransactionRows: []transactionPageRow{
			{ID: "7", DetailKey: "7", Direction: "expense", DirectionLabel: "支出", ExpenseLinkURL: "/transactions?expense_link=7", MatchStatus: "unmatched", MatchStatusLabel: "未关联"},
			{ID: "8", DetailKey: "8", Direction: "expense", DirectionLabel: "支出", ExpenseLinkURL: "/transactions?expense_link=8", ExpenseLinked: true, ExpenseInvoiceLinked: true, MatchStatus: "matched", MatchStatusLabel: "已关联", AllocationUseDisplay: "支出 · 水电"},
		},
	})
	for _, marker := range []string{
		`action="/transactions/expense-link?transaction_id=7&amp;return_to=%2Ftransactions%3Fdirection%3Dexpense"`, `enctype="multipart/form-data"`,
		`name="property_id" required`, `name="room_id"`, `name="category" required`, `name="attachments" multiple`,
		`>关联房间</a>`, `>编辑关联</a>`, `>附件已上传</small>`, `>已关联</span>`,
	} {
		if !strings.Contains(page, marker) {
			t.Errorf("expense attribution page missing %q", marker)
		}
	}
	if strings.Contains(page, `>撤销整笔匹配</a>`) || strings.Contains(page, `>处理分配</a>`) {
		t.Fatal("expense row exposed rent-only actions")
	}
}

func TestTransactionExpenseParseFailureKeepsFilterContext(t *testing.T) {
	r := httptest.NewRequest("POST", "/transactions/expense-link?transaction_id=7&return_to=%2Ftransactions%3Fdirection%3Dexpense%26page%3D2", nil)
	w := httptest.NewRecorder()
	redirectTransactionExpenseResult(w, r, 7, "error", "invalid_expense_invoice")
	location := w.Header().Get("Location")
	parsed, err := url.Parse(location)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Path != "/transactions" || parsed.Query().Get("direction") != "expense" || parsed.Query().Get("page") != "2" || parsed.Query().Get("expense_link") != "7" || parsed.Query().Get("error") != "invalid_expense_invoice" {
		t.Fatalf("parse failure redirect = %q", location)
	}
}

func TestExpenseTransactionMigrationLinksOneSourceWithoutDuplicatingIt(t *testing.T) {
	for name, markers := range map[string][]string{
		"015_expense_transaction_link.sql": {
			"ADD COLUMN payment_transaction_id bigint unsigned NULL", "ADD UNIQUE KEY idx_manual_expenses_user_payment_transaction (user_id, payment_transaction_id)",
			"FOREIGN KEY (payment_transaction_id) REFERENCES payment_transactions(id)",
		},
		"016_backfill_manual_expense_transactions.sql": {
			"JOIN payment_transactions", "transaction.source = 'manual_expense'", "expense.payment_transaction_id = transaction.id", "transaction.match_status = 'matched'",
		},
	} {
		body, err := os.ReadFile(filepath.Join("..", "..", "migrations", name))
		if err != nil {
			t.Fatal(err)
		}
		for _, marker := range markers {
			if !strings.Contains(string(body), marker) {
				t.Errorf("%s missing %q", name, marker)
			}
		}
		if strings.Contains(string(body), "--") {
			t.Errorf("%s has SQL comments that the flat migration runner would skip", name)
		}
	}
}

func TestParseExpenseInvoiceUploadCanBindAfterCreatingExpense(t *testing.T) {
	values := url.Values{"invoice_number": {"INV-7"}, "vendor": {"Gas supplier"}, "invoice_date": {"2026-09-13"}, "invoice_amount": {"77.99"}}
	invoice, err := parseExpenseInvoiceUpload(values, []byte("%PDF-1.7\ninvoice"), "gas.pdf")
	if err != nil {
		t.Fatal(err)
	}
	if invoice.ExpenseID != 0 || invoice.AmountCents != 7799 || !invoice.IsCurrent {
		t.Fatalf("upload prepared before expense creation = %+v", invoice)
	}
}
