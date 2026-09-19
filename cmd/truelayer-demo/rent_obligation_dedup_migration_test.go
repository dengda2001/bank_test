package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"gorm.io/gorm"
)

func readDedupeMigration(t *testing.T) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("..", "..", "migrations", "013_dedupe_rent_obligations.sql"))
	if err != nil {
		t.Fatalf("read dedupe migration: %v", err)
	}
	return string(body)
}

// splitSQLStatements drops every fragment whose trimmed text starts with "--".
// Because the runner splits on ";", a comment line placed above a statement
// becomes the start of that statement's fragment and silently discards it -
// no error, no log line, the statement simply never runs. A migration about
// data repair must not be able to disappear that way. Block comments are just
// as unsafe: a ";" inside one still splits the file, leaving an unterminated
// "/*" and a syntax error. Migrations under this runner therefore carry no
// comments at all.
func TestDedupeMigrationContainsNoComments(t *testing.T) {
	sql := readDedupeMigration(t)
	if idx := strings.Index(sql, "--"); idx >= 0 {
		t.Fatalf("migration contains %q at offset %d; the runner would silently drop the statement that follows it", "--", idx)
	}
	for _, marker := range []string{"/*", "*/"} {
		if idx := strings.Index(sql, marker); idx >= 0 {
			t.Fatalf("migration contains %q at offset %d; a semicolon inside a block comment truncates the file", marker, idx)
		}
	}
}

func TestDedupeMigrationSplitsIntoExecutableStatements(t *testing.T) {
	stmts := splitSQLStatements(readDedupeMigration(t))
	if len(stmts) == 0 {
		t.Fatal("migration produced no statements")
	}
	for i, stmt := range stmts {
		if strings.TrimSpace(stmt) == "" {
			t.Fatalf("statement %d is empty", i)
		}
	}
	last := strings.ToLower(stmts[len(stmts)-1])
	if !strings.HasPrefix(last, "drop table if exists _mig013_keep") {
		t.Fatalf("last statement should clean up the helper table, got %q", last)
	}
}

// The contract being restored is database-guidelines.md "Tenant Billing History
// Projection": at most one lazy rent_obligations row per (user, tenant,
// period_month). MySQL has no partial indexes, so laziness is encoded into a
// stored generated column that is NULL - and therefore distinct - for
// charge-backed rows.
func TestDedupeMigrationRestoresLazyUniquenessWithoutConstrainingChargeRows(t *testing.T) {
	sql := strings.ToLower(readDedupeMigration(t))
	for _, fragment := range []string{
		"add column lazy_period_month date",
		"generated always as (case when rent_charge_id is null then period_month else null end) stored",
		"add unique key idx_rent_obligations_lazy_tenant_period (user_id, tenant_id, lazy_period_month)",
		"drop table if exists _mig013_keep",
		"create table _mig013_keep",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("migration missing %q", fragment)
		}
	}
	if strings.Contains(sql, "drop index idx_rent_obligations_user_charge_tenant") {
		t.Fatal("migration must not remove the charge-path unique key added by 009")
	}
}

// Every table with a foreign key onto rent_obligations cascades on delete, so
// references have to be moved onto the surviving row before duplicates are
// removed. Missing one silently destroys payment history.
func TestDedupeMigrationRemapsEveryCascadingReference(t *testing.T) {
	sql := strings.ToLower(readDedupeMigration(t))
	for _, table := range []string{"payment_allocations", "cash_receipts", "dunning_send_attempts"} {
		remap := "set " + map[string]string{
			"payment_allocations":   "pa.rent_obligation_id",
			"cash_receipts":         "cr.rent_obligation_id",
			"dunning_send_attempts": "da.rent_obligation_id",
		}[table] + " = k.obligation_id"
		if !strings.Contains(sql, remap) {
			t.Fatalf("migration does not remap %s onto the surviving row (want %q)", table, remap)
		}
	}
}

// paid_amount_cents is a denormalised cache that only the obligation a service
// write touches gets refreshed: allocateTransaction, recordCashReceipt, void and
// the transaction-action paths recompute it for their own target, while
// ensureMonthlyObligations always stores 0 for a fresh lazy row. Read paths such
// as summarizeRentDashboardWithFilters sum the stored column directly, so
// remapping a payment onto a duplicate survivor whose cache was written while it
// had no children would leave that payment invisible on the dashboard.
func TestDedupeMigrationRecomputesThePaidCache(t *testing.T) {
	sql := strings.ToLower(readDedupeMigration(t))
	for _, fragment := range []string{
		"set o.paid_amount_cents = p.paid_cents",
		"from payment_allocations pa",
		"from cash_receipts cr",
		"pa.status = 'confirmed'",
		"a.status = 'confirmed'",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("migration missing %q", fragment)
		}
	}
}

// Dedupe migration fixtures. The ids are far above anything the auto-increment
// tests in this package allocate so the seed cannot collide with test data that
// ran earlier in the same process against the disposable MySQL test database.
const (
	dedupeTestUserID          = 900001
	dedupeTestTenantID        = 900001
	dedupeTestRequestKey      = "dedupe-migration-request"
	dedupeTestSepUnreferenced = 900001
	dedupeTestSepSurvivor     = 900002
	dedupeTestSepDoomedA      = 900003
	dedupeTestSepDoomedB      = 900004
)

func dedupeTestExec(t *testing.T, db *gorm.DB, query string, args ...any) {
	t.Helper()
	if err := db.Exec(query, args...).Error; err != nil {
		t.Fatalf("seed query %q: %v", query, err)
	}
}

// TestDedupeMigrationRepairsDirtyDataOnMySQL reproduces the dirty shape that
// broke the first draft of migration 013 and asserts the repair survives it.
//
// The September group is deliberately ordered so the survivor is chosen by the
// reference ranking and is not the lowest id: 900001 carries nothing, 900002
// carries the first allocation, and 900003/900004 carry the second allocation
// and a dunning attempt. The important case is the dunning pair -
// dunningService.send applies one request_key to every obligation selected on
// the dashboard page, so two duplicate rows for one (tenant, month) can each
// carry an attempt with the same request_key. Because the survivor is picked for
// its allocations rather than for an attempt, remapping both attempts onto it
// violates idx_dunning_attempts_user_request_obligation. Pre-deleting only
// attempts that already sit on the survivor is not enough.
//
// Opt-in: point RENTOPS_MYSQL_TEST_DSN at a disposable database
// (scripts/run-mysql-test-clean.sh cleans it before and after).
func TestDedupeMigrationRepairsDirtyDataOnMySQL(t *testing.T) {
	db, sqlDB := openLedgerMySQLTestDB(t)
	migrationsDir := filepath.Join("..", "..", "migrations")
	if err := runMigrations(sqlDB, migrationsDir); err != nil {
		t.Fatalf("baseline migration run: %v", err)
	}

	// Roll the schema back to the pre-013 state so the duplicate lazy rows this
	// migration exists to repair can be created again.
	if err := db.Exec("ALTER TABLE rent_obligations DROP INDEX idx_rent_obligations_lazy_tenant_period, DROP COLUMN lazy_period_month").Error; err != nil {
		t.Fatalf("drop dedupe constraint: %v", err)
	}
	if err := db.Exec("DELETE FROM schema_migrations WHERE version = ?", "013_dedupe_rent_obligations").Error; err != nil {
		t.Fatalf("forget dedupe migration: %v", err)
	}
	t.Cleanup(func() {
		for _, query := range []string{
			"DELETE FROM dunning_send_attempts WHERE user_id = ?",
			"DELETE FROM cash_receipts WHERE user_id = ?",
			"DELETE FROM payment_allocations WHERE user_id = ?",
			"DELETE FROM rent_obligations WHERE user_id = ?",
			"DELETE FROM rent_charges WHERE user_id = ?",
			"DELETE FROM tenancy_agreements WHERE user_id = ?",
			"DELETE FROM rooms WHERE user_id = ?",
			"DELETE FROM properties WHERE user_id = ?",
			"DELETE FROM payment_transactions WHERE user_id = ?",
			"DELETE FROM tenants WHERE user_id = ?",
			"DELETE FROM users WHERE id = ?",
		} {
			if err := db.Exec(query, dedupeTestUserID).Error; err != nil {
				t.Logf("cleanup %q: %v", query, err)
			}
		}
	})

	dedupeTestExec(t, db, "INSERT INTO users (id, username, password_hash) VALUES (?, ?, ?)", dedupeTestUserID, "dedupe-migration-owner", "x")
	dedupeTestExec(t, db, "INSERT INTO tenants (id, user_id, name, monthly_rent_cents, currency, billing_start_date, due_day, rent_start_date, room_label, room_address) VALUES (?, ?, 'T1', 105000, 'EUR', '2026-01-01', 30, '2026-01-01', 'R1', 'Addr 1')", dedupeTestTenantID, dedupeTestUserID)
	dedupeTestExec(t, db, "INSERT INTO payment_transactions (id, user_id, source, stable_transaction_key, direction, amount_cents, currency, transaction_time, description) VALUES (900001, ?, 'truelayer', 'dedupe-tx-1', 'income', 25000, 'EUR', '2026-09-05 10:00:00', 'RENT SEP'), (900002, ?, 'truelayer', 'dedupe-tx-2', 'income', 15000, 'EUR', '2026-09-06 10:00:00', 'RENT SEP')", dedupeTestUserID, dedupeTestUserID)

	seedObligation := func(id uint64, period, recordStatus string, paidCents int64) {
		t.Helper()
		dedupeTestExec(t, db, "INSERT INTO rent_obligations (id, user_id, tenant_id, period_month, due_date, expected_amount_cents, paid_amount_cents, currency, status, record_status, generated_by) VALUES (?, ?, ?, ?, ?, 105000, ?, 'EUR', 'open', ?, 'lazy')",
			id, dedupeTestUserID, dedupeTestTenantID, period, period, paidCents, recordStatus)
	}
	seedObligation(dedupeTestSepUnreferenced, "2026-09-01", obligationRecordActive, 0)
	seedObligation(dedupeTestSepSurvivor, "2026-09-01", obligationRecordActive, 0)
	seedObligation(dedupeTestSepDoomedA, "2026-09-01", obligationRecordActive, 0)
	seedObligation(dedupeTestSepDoomedB, "2026-09-01", obligationRecordActive, 0)
	seedObligation(900005, "2026-03-01", obligationRecordVoided, 0)
	seedObligation(900006, "2026-03-01", obligationRecordVoided, 0)
	seedObligation(900007, "2026-08-01", obligationRecordActive, 0)

	seedAllocation := func(id, transactionID, obligationID uint64, amountCents int64) {
		t.Helper()
		dedupeTestExec(t, db, "INSERT INTO payment_allocations (id, user_id, payment_transaction_id, rent_obligation_id, tenant_id, amount_cents, allocation_kind, status, confirmed_by_user_id, confirmation_source) VALUES (?, ?, ?, ?, ?, ?, 'rent', 'confirmed', ?, 'manual')",
			id, dedupeTestUserID, transactionID, obligationID, dedupeTestTenantID, amountCents, dedupeTestUserID)
	}
	seedAllocation(900001, 900001, dedupeTestSepSurvivor, 25000)
	seedAllocation(900002, 900002, dedupeTestSepDoomedA, 15000)

	seedAttempt := func(id, obligationID uint64) {
		t.Helper()
		dedupeTestExec(t, db, "INSERT INTO dunning_send_attempts (id, user_id, tenant_id, rent_obligation_id, period_month, recipient_email, template_kind, subject, body, expected_amount_cents, paid_amount_cents, balance_amount_cents, currency, sender_display_name, reply_to_email, service_from_email, delivery_status, operation_id, request_key) VALUES (?, ?, ?, ?, '2026-09-01', 'a@x.test', 'reminder', 's', 'b', 105000, 0, 105000, 'EUR', 'L', 'r@x.test', 's@x.test', 'sent', ?, ?)",
			id, dedupeTestUserID, dedupeTestTenantID, obligationID, "op", dedupeTestRequestKey)
	}
	seedAttempt(900001, dedupeTestSepDoomedA)
	seedAttempt(900002, dedupeTestSepDoomedB)

	if err := runMigrations(sqlDB, migrationsDir); err != nil {
		t.Fatalf("apply dedupe migration to dirty data: %v", err)
	}

	var duplicateGroups int64
	if err := db.Raw("SELECT COUNT(*) FROM (SELECT 1 FROM rent_obligations WHERE user_id = ? AND rent_charge_id IS NULL GROUP BY tenant_id, period_month HAVING COUNT(*) > 1) x", dedupeTestUserID).Scan(&duplicateGroups).Error; err != nil {
		t.Fatalf("count duplicate groups: %v", err)
	}
	if duplicateGroups != 0 {
		t.Fatalf("duplicate lazy groups after migration = %d; want 0", duplicateGroups)
	}

	var survivor uint64
	if err := db.Raw("SELECT id FROM rent_obligations WHERE user_id = ? AND tenant_id = ? AND period_month = '2026-09-01'", dedupeTestUserID, dedupeTestTenantID).Row().Scan(&survivor); err != nil {
		t.Fatalf("read surviving September obligation: %v", err)
	}
	if survivor != dedupeTestSepSurvivor {
		t.Fatalf("September survivor = %d; want the referenced row %d (the lower id %d carries no reference and must lose)", survivor, dedupeTestSepSurvivor, dedupeTestSepUnreferenced)
	}
	var paidCents int64
	var status string
	if err := db.Raw("SELECT paid_amount_cents, status FROM rent_obligations WHERE id = ?", survivor).Row().Scan(&paidCents, &status); err != nil {
		t.Fatalf("read surviving paid cache: %v", err)
	}
	if paidCents != 40000 || status != "partial" {
		t.Fatalf("survivor paid/status = %d/%q; want 40000/partial from both remapped allocations", paidCents, status)
	}

	// The allocation that lived on the doomed row has to be moved onto the
	// survivor instead of being cascaded away with it.
	remappedAllocations := 0
	for _, allocationID := range []uint64{900001, 900002} {
		var allocationObligation uint64
		if err := db.Raw("SELECT rent_obligation_id FROM payment_allocations WHERE id = ?", allocationID).Row().Scan(&allocationObligation); err != nil {
			t.Fatalf("read allocation %d: %v", allocationID, err)
		}
		if allocationObligation == survivor {
			remappedAllocations++
		}
	}
	if remappedAllocations != 2 {
		t.Fatalf("allocations pointing at survivor = %d; want both", remappedAllocations)
	}

	var attemptCount int64
	if err := db.Raw("SELECT COUNT(*) FROM dunning_send_attempts WHERE user_id = ? AND request_key = ?", dedupeTestUserID, dedupeTestRequestKey).Scan(&attemptCount).Error; err != nil {
		t.Fatalf("count remapped attempts: %v", err)
	}
	if attemptCount != 1 {
		t.Fatalf("attempts sharing the request key = %d; want exactly one after dedupe", attemptCount)
	}
	var attemptObligation uint64
	if err := db.Raw("SELECT rent_obligation_id FROM dunning_send_attempts WHERE user_id = ? AND request_key = ?", dedupeTestUserID, dedupeTestRequestKey).Row().Scan(&attemptObligation); err != nil {
		t.Fatalf("read remapped attempt: %v", err)
	}
	if attemptObligation != survivor {
		t.Fatalf("attempt points at obligation %d; want survivor %d", attemptObligation, survivor)
	}

	var voidedCount int64
	if err := db.Raw("SELECT COUNT(*) FROM rent_obligations WHERE user_id = ? AND tenant_id = ? AND period_month = '2026-03-01' AND status = 'voided'", dedupeTestUserID, dedupeTestTenantID).Scan(&voidedCount).Error; err != nil {
		t.Fatalf("count voided group: %v", err)
	}
	if voidedCount != 1 {
		t.Fatalf("voided March obligations = %d; want exactly one voided survivor", voidedCount)
	}
	var untreatedCount int64
	if err := db.Raw("SELECT COUNT(*) FROM rent_obligations WHERE user_id = ? AND tenant_id = ? AND period_month = '2026-08-01'", dedupeTestUserID, dedupeTestTenantID).Scan(&untreatedCount).Error; err != nil {
		t.Fatalf("count singleton group: %v", err)
	}
	if untreatedCount != 1 {
		t.Fatalf("singleton August obligations = %d; want the untouched row to survive", untreatedCount)
	}

	// The restored key must reject a duplicate lazy row. Use the raw handle so an
	// expected 1062 does not print a GORM error log.
	duplicateInsert := "INSERT INTO rent_obligations (user_id, tenant_id, period_month, due_date, expected_amount_cents, currency, status, record_status, generated_by) VALUES (?, ?, '2026-09-01', '2026-09-30', 105000, 'EUR', 'open', 'active', 'lazy')"
	if _, err := sqlDB.Exec(duplicateInsert, dedupeTestUserID, dedupeTestTenantID); err == nil {
		t.Fatal("duplicate lazy obligation insert was accepted; the restored unique key is missing")
	}
	// ...while charge-backed rows for the same account, tenant and month stay
	// legal because their generated column is NULL.
	dedupeTestExec(t, db, "INSERT INTO properties (id, user_id, name) VALUES (900001, ?, 'P1')", dedupeTestUserID)
	dedupeTestExec(t, db, "INSERT INTO rooms (id, user_id, property_id, room_label, active_from) VALUES (900001, ?, 900001, 'R-1', '2026-01-01')", dedupeTestUserID)
	dedupeTestExec(t, db, "INSERT INTO tenancy_agreements (id, user_id, room_id, start_date, monthly_rent_cents, due_day) VALUES (900001, ?, 900001, '2026-01-01', 105000, 30)", dedupeTestUserID)
	dedupeTestExec(t, db, "INSERT INTO rent_charges (id, user_id, property_id, room_id, tenancy_agreement_id, period_month, due_date, expected_amount_cents, currency, record_status) VALUES (900001, ?, 900001, 900001, 900001, '2026-09-01', '2026-09-30', 105000, 'EUR', 'active')", dedupeTestUserID)
	dedupeTestExec(t, db, "INSERT INTO rent_obligations (user_id, rent_charge_id, tenant_id, period_month, due_date, expected_amount_cents, currency, status, record_status, generated_by) VALUES (?, 900001, ?, '2026-09-01', '2026-09-30', 105000, 'EUR', 'open', 'active', 'charge')", dedupeTestUserID, dedupeTestTenantID)
}
