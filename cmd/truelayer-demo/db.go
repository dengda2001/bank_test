package main

import (
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

type migrationFile struct {
	version string
	path    string
}

const roomRentPlanResetMigrationVersion = "014_room_rent_plan_reset"

func initDatabase(cfg config) (*gorm.DB, error) {
	dsn, err := resolveDatabaseDSN(cfg)
	if err != nil {
		return nil, err
	}
	db, err := gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		return nil, fmt.Errorf("connect mysql: %w", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, fmt.Errorf("get sql db: %w", err)
	}
	sqlDB.SetMaxOpenConns(10)
	sqlDB.SetMaxIdleConns(5)
	sqlDB.SetConnMaxLifetime(30 * time.Minute)

	if err := runMigrations(sqlDB, cfg.MigrationsDir); err != nil {
		_ = sqlDB.Close()
		return nil, err
	}
	return db, nil
}

func runMigrations(db *sql.DB, dir string) error {
	if dir == "" {
		return errors.New("MIGRATIONS_DIR is empty")
	}
	files, err := discoverMigrations(dir)
	if err != nil {
		return err
	}
	if len(files) == 0 {
		return fmt.Errorf("no migration files found in %s", dir)
	}
	if _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
		version varchar(191) NOT NULL PRIMARY KEY,
		applied_at timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP
	) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci`); err != nil {
		return fmt.Errorf("ensure schema_migrations: %w", err)
	}
	for _, file := range files {
		applied, err := migrationApplied(db, file.version)
		if err != nil {
			return err
		}
		if applied {
			continue
		}
		if file.version == roomRentPlanResetMigrationVersion {
			if err := validateRoomRentPlanResetPreconditions(db); err != nil {
				return err
			}
		}
		body, err := os.ReadFile(file.path)
		if err != nil {
			return fmt.Errorf("read migration %s: %w", file.path, err)
		}
		if err := applyMigration(db, file.version, string(body)); err != nil {
			return err
		}
	}
	return nil
}

func validateRoomRentPlanResetPreconditions(db *sql.DB) error {
	if db == nil {
		return errors.New("validate room rent plan reset: database is required")
	}
	var schema string
	if err := db.QueryRow("SELECT DATABASE()").Scan(&schema); err != nil {
		return fmt.Errorf("validate room rent plan reset database: %w", err)
	}
	if schema == "" {
		return errors.New("validate room rent plan reset: selected database is required")
	}

	tables := []string{
		"users", "bank_connections", "properties", "rooms", "tenants", "tenant_payers",
		"tenancy_agreements", "agreement_parties", "rent_charges", "rent_obligations",
		"payment_transactions", "payment_allocations", "cash_receipts", "dunning_sender_configs",
		"dunning_send_attempts", "manual_expenses", "manual_expense_invoices", "bank_sync_runs",
		"bank_sync_run_accounts", "payment_transaction_actions",
	}
	columns := map[string][]string{
		"properties":            {"inactive_from", "status"},
		"rooms":                 {"active_from", "inactive_from", "monthly_rent_cents", "due_day"},
		"tenants":               {"payer_id", "payer_name_hint", "monthly_rent_cents", "billing_start_date", "rent_start_date", "rent_end_date", "room_label", "room_address", "property_hint"},
		"tenant_payers":         {"tenant_id", "payer_id", "payer_name_normalized"},
		"tenancy_agreements":    {"room_id", "start_date", "end_date", "contract_date", "move_in_date", "monthly_rent_cents", "due_day", "status"},
		"agreement_parties":     {"agreement_id", "joined_at", "left_at", "status"},
		"rent_charges":          {"tenancy_agreement_id", "room_id", "property_id", "period_month"},
		"rent_obligations":      {"rent_charge_id", "tenant_id", "period_month", "generated_by", "lazy_period_month"},
		"payment_allocations":   {"rent_obligation_id", "tenant_id"},
		"cash_receipts":         {"tenant_id", "rent_obligation_id"},
		"dunning_send_attempts": {"tenant_id", "rent_obligation_id"},
	}
	indexes := map[string][]string{
		"properties":         {"idx_properties_user_status"},
		"rooms":              {"idx_rooms_user_property_label", "idx_rooms_user_property_status"},
		"tenants":            {"idx_tenants_user_status", "idx_tenants_user_payer_id", "idx_tenants_user_alias"},
		"tenancy_agreements": {"idx_tenancy_agreements_user_room_dates"},
		"agreement_parties":  {"idx_agreement_parties_user_agreement_tenant", "idx_agreement_parties_user_agreement_status"},
		"rent_charges":       {"idx_rent_charges_user_room_period", "idx_rent_charges_user_agreement_period"},
		"rent_obligations":   {"idx_rent_obligations_user_charge_tenant", "idx_rent_obligations_user_charge", "idx_rent_obligations_lazy_tenant_period"},
	}
	foreignKeys := map[string][]string{
		"properties":            {"fk_properties_user"},
		"rooms":                 {"fk_rooms_user", "fk_rooms_property"},
		"tenants":               {"fk_tenants_user"},
		"tenant_payers":         {"fk_tenant_payers_tenant"},
		"tenancy_agreements":    {"fk_tenancy_agreements_user", "fk_tenancy_agreements_room"},
		"agreement_parties":     {"fk_agreement_parties_user", "fk_agreement_parties_agreement", "fk_agreement_parties_tenant"},
		"rent_charges":          {"fk_rent_charges_user", "fk_rent_charges_property", "fk_rent_charges_room", "fk_rent_charges_agreement"},
		"rent_obligations":      {"fk_rent_obligations_user", "fk_rent_obligations_tenant", "fk_rent_obligations_charge", "fk_rent_obligations_voided_by_user"},
		"payment_allocations":   {"fk_payment_allocations_obligation"},
		"cash_receipts":         {"fk_cash_receipts_tenant", "fk_cash_receipts_obligation"},
		"dunning_send_attempts": {"fk_dunning_attempts_tenant", "fk_dunning_attempts_obligation"},
	}
	for _, table := range tables {
		var count int
		if err := db.QueryRow("SELECT COUNT(*) FROM information_schema.tables WHERE table_schema = ? AND table_name = ? AND table_type = 'BASE TABLE'", schema, table).Scan(&count); err != nil {
			return fmt.Errorf("validate room rent plan reset table %s: %w", table, err)
		}
		if count != 1 {
			return fmt.Errorf("014 room rent plan reset requires the pre-014 schema; missing table %s", table)
		}
	}
	for table, names := range columns {
		for _, name := range names {
			var count int
			if err := db.QueryRow("SELECT COUNT(*) FROM information_schema.columns WHERE table_schema = ? AND table_name = ? AND column_name = ?", schema, table, name).Scan(&count); err != nil {
				return fmt.Errorf("validate room rent plan reset column %s.%s: %w", table, name, err)
			}
			if count != 1 {
				return fmt.Errorf("014 room rent plan reset requires the pre-014 schema; missing column %s.%s", table, name)
			}
		}
	}
	for table, names := range indexes {
		for _, name := range names {
			var count int
			if err := db.QueryRow("SELECT COUNT(*) FROM information_schema.statistics WHERE table_schema = ? AND table_name = ? AND index_name = ?", schema, table, name).Scan(&count); err != nil {
				return fmt.Errorf("validate room rent plan reset index %s.%s: %w", table, name, err)
			}
			if count == 0 {
				return fmt.Errorf("014 room rent plan reset requires the pre-014 schema; missing index %s.%s", table, name)
			}
		}
	}
	for table, names := range foreignKeys {
		for _, name := range names {
			var count int
			if err := db.QueryRow("SELECT COUNT(*) FROM information_schema.referential_constraints WHERE constraint_schema = ? AND table_name = ? AND constraint_name = ?", schema, table, name).Scan(&count); err != nil {
				return fmt.Errorf("validate room rent plan reset foreign key %s.%s: %w", table, name, err)
			}
			if count != 1 {
				return fmt.Errorf("014 room rent plan reset requires the pre-014 schema; missing foreign key %s.%s", table, name)
			}
		}
	}

	rows, err := db.Query("SELECT table_name FROM information_schema.tables WHERE table_schema = ? AND table_type = 'BASE TABLE' AND table_name <> 'schema_migrations' ORDER BY table_name", schema)
	if err != nil {
		return fmt.Errorf("validate room rent plan reset data tables: %w", err)
	}
	var dataTables []string
	for rows.Next() {
		var table string
		if err := rows.Scan(&table); err != nil {
			_ = rows.Close()
			return fmt.Errorf("read room rent plan reset data tables: %w", err)
		}
		dataTables = append(dataTables, table)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return fmt.Errorf("read room rent plan reset data tables: %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("close room rent plan reset data tables: %w", err)
	}
	for _, table := range dataTables {
		query := "SELECT COUNT(*) FROM `" + strings.ReplaceAll(table, "`", "``") + "`"
		var count int64
		if err := db.QueryRow(query).Scan(&count); err != nil {
			return fmt.Errorf("count room rent plan reset table %s: %w", table, err)
		}
		if count != 0 {
			return fmt.Errorf("014 room rent plan reset requires an empty database; table %s contains %d rows. Rebuild the development/test database before applying this migration", table, count)
		}
	}
	return nil
}

func discoverMigrations(dir string) ([]migrationFile, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("read migrations dir: %w", err)
	}
	files := make([]migrationFile, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		version := strings.TrimSuffix(entry.Name(), ".sql")
		files = append(files, migrationFile{
			version: version,
			path:    filepath.Join(dir, entry.Name()),
		})
	}
	sort.Slice(files, func(i, j int) bool {
		return files[i].version < files[j].version
	})
	return files, nil
}

func migrationApplied(db *sql.DB, version string) (bool, error) {
	var found string
	err := db.QueryRow("SELECT version FROM schema_migrations WHERE version = ?", version).Scan(&found)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("check migration %s: %w", version, err)
	}
	return true, nil
}

func applyMigration(db *sql.DB, version, sqlText string) error {
	tx, err := db.Begin()
	if err != nil {
		return fmt.Errorf("begin migration %s: %w", version, err)
	}
	defer tx.Rollback()

	for _, stmt := range splitSQLStatements(sqlText) {
		if _, err := tx.Exec(stmt); err != nil {
			return fmt.Errorf("apply migration %s: %w", version, err)
		}
	}
	if _, err := tx.Exec("INSERT INTO schema_migrations (version) VALUES (?)", version); err != nil {
		return fmt.Errorf("record migration %s: %w", version, err)
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migration %s: %w", version, err)
	}
	return nil
}

func splitSQLStatements(sqlText string) []string {
	parts := strings.Split(sqlText, ";")
	statements := make([]string, 0, len(parts))
	for _, part := range parts {
		stmt := strings.TrimSpace(part)
		if stmt == "" || strings.HasPrefix(stmt, "--") {
			continue
		}
		statements = append(statements, stmt)
	}
	return statements
}
