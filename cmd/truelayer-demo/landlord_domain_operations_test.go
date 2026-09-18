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

func TestRentArrangementResponsibilitiesAllowEmptyRoomButRequireExactSplit(t *testing.T) {
	if err := validateRentResponsibilityPlanAllowEmpty(10000, nil); err != nil {
		t.Fatalf("empty room rejected: %v", err)
	}
	responsibilities, err := splitRentAmountEvenly(10001, []uint64{7, 2, 9})
	if err != nil {
		t.Fatal(err)
	}
	if responsibilities[0].TenantID != 7 || responsibilities[0].AmountCents != 3334 {
		t.Fatalf("remainder was not deterministic: %+v", responsibilities)
	}
	if err := validateRentResponsibilityPlanAllowEmpty(10000, []rentResponsibilityInput{{TenantID: 1, AmountCents: 5000}, {TenantID: 2, AmountCents: 4999}}); err == nil {
		t.Fatal("responsibility sum mismatch unexpectedly accepted")
	}
}

func TestStructuredTenantInputCanRemainUnbound(t *testing.T) {
	input := tenantInput{Name: "Unbound tenant", Currency: "EUR", IntervalUnit: "month", IntervalCount: 1, Status: "active", Structured: true}
	if err := validateTenantInput(input); err != nil {
		t.Fatalf("structured unbound tenant rejected: %v", err)
	}
}

func TestManualBalanceReasonIsRequired(t *testing.T) {
	if _, err := (&transactionService{}).settleRentObligation(nil, 1, 1); err != errManualBalanceReasonRequired {
		t.Fatalf("missing reason error=%v", err)
	}
}
