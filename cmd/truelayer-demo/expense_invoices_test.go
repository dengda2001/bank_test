package main

import (
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestParseExpenseInvoiceMetadataAcceptsPDFAndSanitizesName(t *testing.T) {
	values := url.Values{
		"expense_id": {"19"}, "invoice_number": {"INV-2026-019"}, "vendor": {"North Dublin Maintenance"},
		"invoice_date": {"2026-09-01"}, "invoice_amount": {"400.00"}, "note": {"September repair"},
	}
	got, err := parseExpenseInvoiceMetadata(values, []byte("%PDF-1.7\ninvoice"), "../../invoice.pdf")
	if err != nil {
		t.Fatal(err)
	}
	if got.ExpenseID != 19 || got.InvoiceNumber != "INV-2026-019" || got.AmountCents != 40000 || got.ContentType != "application/pdf" {
		t.Fatalf("invoice metadata = %+v", got)
	}
	if got.FileName != "invoice.pdf" || len(got.SHA256) != 64 {
		t.Fatalf("unsafe or missing file metadata: %+v", got)
	}
}

func TestParseExpenseInvoiceMetadataRejectsUnsupportedContentAndOversize(t *testing.T) {
	values := url.Values{
		"expense_id": {"19"}, "invoice_number": {"INV-19"}, "vendor": {"Vendor"},
		"invoice_date": {"2026-09-01"}, "invoice_amount": {"400.00"},
	}
	if _, err := parseExpenseInvoiceMetadata(values, []byte("<html>not an invoice</html>"), "invoice.html"); err == nil {
		t.Fatal("HTML upload was accepted")
	}
	if _, err := parseExpenseInvoiceMetadata(values, make([]byte, maxExpenseInvoiceBytes+1), "invoice.pdf"); err == nil {
		t.Fatal("oversize upload was accepted")
	}
}

func TestExpenseInvoiceMigrationPersistsScopedHistory(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "..", "migrations", "011_prototype_tenancy_and_expense_invoices.sql"))
	if err != nil {
		t.Fatal(err)
	}
	sql := string(body)
	for _, fragment := range []string{
		"contract_date date NULL", "move_in_date date NULL", "CREATE TABLE IF NOT EXISTS manual_expense_invoices",
		"ADD COLUMN city_region varchar(191) NULL", "ADD COLUMN timezone varchar(64) NOT NULL DEFAULT 'Europe/Dublin'",
		"ADD COLUMN room_type varchar(64) NULL", "ADD COLUMN capacity int NOT NULL DEFAULT 1",
		"user_id bigint unsigned NOT NULL", "expense_id bigint unsigned NOT NULL", "file_data longblob NOT NULL",
		"is_current tinyint(1) NOT NULL DEFAULT 1", "idx_expense_invoices_user_expense_current",
		"fk_expense_invoices_user", "fk_expense_invoices_expense",
	} {
		if !strings.Contains(sql, fragment) {
			t.Errorf("prototype migration missing %q", fragment)
		}
	}
}
