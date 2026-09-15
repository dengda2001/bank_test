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
		"modify column tenant_id bigint unsigned null",
		"record_status",
		"voided_by_user_id",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("migration missing %q", fragment)
		}
	}
}

func TestBankReceiptMigrationAddsSyncCoverageAndParsedPeriodFacts(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "..", "migrations", "005_bank_receipt_management.sql"))
	if err != nil {
		t.Fatalf("read bank receipt migration: %v", err)
	}
	sql := strings.ToLower(string(body))
	for _, fragment := range []string{
		"create table if not exists bank_sync_runs",
		"create table if not exists bank_sync_run_accounts",
		"requested_from",
		"covered_from",
		"parsed_period_month",
		"parsed_period_source",
		"payment_transaction_actions",
		"idx_payment_transactions_user_parsed_period",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("migration missing %q", fragment)
		}
	}
}
