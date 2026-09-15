package main

import (
	"database/sql"
	"os"
	"path/filepath"
	"testing"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

// These tests are opt-in because they mutate the configured schema by running
// the real migration runner. Point RENTOPS_MYSQL_TEST_DSN at a disposable
// MySQL database when running them locally or in CI.
func openLedgerMySQLTestDB(t *testing.T) (*gorm.DB, *sql.DB) {
	t.Helper()
	dsn := os.Getenv("RENTOPS_MYSQL_TEST_DSN")
	if dsn == "" {
		t.Skip("RENTOPS_MYSQL_TEST_DSN is not set")
	}
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open MySQL test database: %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get MySQL test database handle: %v", err)
	}
	t.Cleanup(func() { _ = sqlDB.Close() })
	return db, sqlDB
}

func TestLedgerMigrationIsIdempotentOnMySQL(t *testing.T) {
	db, sqlDB := openLedgerMySQLTestDB(t)
	migrationsDir := filepath.Join("..", "..", "migrations")
	if err := runMigrations(sqlDB, migrationsDir); err != nil {
		t.Fatalf("first migration run: %v", err)
	}
	if err := runMigrations(sqlDB, migrationsDir); err != nil {
		t.Fatalf("second migration run: %v", err)
	}

	var count int64
	if err := db.Table("schema_migrations").Where("version = ?", "003_rent_ledger_foundation").Count(&count).Error; err != nil {
		t.Fatalf("count ledger migration: %v", err)
	}
	if count != 1 {
		t.Fatalf("ledger migration count = %d; want exactly one", count)
	}

	var nullable string
	row := db.Raw(`
		SELECT pa.IS_NULLABLE
		FROM information_schema.COLUMNS pa
		WHERE pa.TABLE_SCHEMA = DATABASE()
		  AND pa.TABLE_NAME = 'payment_allocations'
		  AND pa.COLUMN_NAME = 'tenant_id'`).Row()
	if err := row.Scan(&nullable); err != nil {
		t.Fatalf("read migrated allocation columns: %v", err)
	}
	if nullable != "YES" {
		t.Fatalf("payment_allocations.tenant_id nullable = %s; want YES", nullable)
	}
	var allocationKindDefault string
	row = db.Raw(`
		SELECT COALESCE(pa.COLUMN_DEFAULT, '')
		FROM information_schema.COLUMNS pa
		WHERE pa.TABLE_SCHEMA = DATABASE()
		  AND pa.TABLE_NAME = 'payment_allocations'
		  AND pa.COLUMN_NAME = 'allocation_kind'`).Row()
	if err := row.Scan(&allocationKindDefault); err != nil {
		t.Fatalf("read allocation kind default: %v", err)
	}
	if allocationKindDefault != allocationKindRent {
		t.Fatalf("allocation_kind default = %q; want %q", allocationKindDefault, allocationKindRent)
	}
}
