package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFigmaDomainMigrationAddsAuditAndInvoiceFields(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "..", "migrations", "010_figma_domain_operations.sql"))
	if err != nil {
		t.Fatal(err)
	}
	sql := strings.ToLower(string(body))
	for _, fragment := range []string{
		"manual_adjustment_reason varchar(512)",
		"invoice_url varchar(2048)",
		"inactive_from date",
		"alter table payment_transactions",
		"alter table manual_expenses",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("migration missing %q", fragment)
		}
	}
}

func TestExternalInvoiceURLAcceptsOnlyHTTPFamily(t *testing.T) {
	for _, value := range []string{"https://billing.example.test/invoice/1", "http://localhost:8080/invoice"} {
		if err := validateExternalHTTPURL(value); err != nil {
			t.Fatalf("url %q rejected: %v", value, err)
		}
	}
	for _, value := range []string{"javascript:alert(1)", "file:///tmp/invoice.pdf", "//example.test/invoice", "https:///missing-host"} {
		if err := validateExternalHTTPURL(value); err == nil {
			t.Fatalf("url %q unexpectedly accepted", value)
		}
	}
	if err := validateExternalHTTPURL(""); err != nil {
		t.Fatalf("empty invoice url rejected: %v", err)
	}
}

func TestManualBalanceReasonIsRequired(t *testing.T) {
	if _, err := (&transactionService{}).settleRentObligation(nil, 1, 1); err != errManualBalanceReasonRequired {
		t.Fatalf("missing reason error=%v", err)
	}
}
