package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLedgerMigrationAddsAuditFieldsAndAllowsNonRentAllocations(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "..", "migrations", "003_rent_ledger_foundation.sql"))
	if err != nil {
		t.Fatalf("read ledger migration: %v", err)
	}
	sql := strings.ToLower(string(body))
	for _, fragment := range []string{
		"allocation_kind",
		"idempotency_key",
		"void_reason",
		"drop index idx_payment_allocations_tx_obligation",
		"modify column rent_obligation_id bigint unsigned null",
		"record_status",
		"voided_by_user_id",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("migration missing %q", fragment)
		}
	}
}
