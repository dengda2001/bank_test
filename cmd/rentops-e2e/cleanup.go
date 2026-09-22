package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/go-sql-driver/mysql"
)

var e2ECleanupUserTables = []string{
	"dunning_send_attempts",
	"cash_receipts",
	"payment_allocations",
	"payment_transaction_actions",
	"rent_obligations",
	"rent_charges",
	"room_rent_plan_members",
	"room_rent_plans",
	"manual_expense_invoices",
	"manual_expenses",
	"bank_sync_run_accounts",
	"bank_sync_runs",
	"dunning_sender_configs",
	"bank_connections",
	"payment_transactions",
	"tenant_payers",
	"rooms",
	"properties",
	"tenants",
}

func cleanupE2ERun(ctx context.Context, options e2eOptions, manifest e2eFixtureManifest) e2eCleanupReport {
	if err := validateE2ECleanupOptions(options); err != nil {
		return failedE2ECleanup(err.Error())
	}
	if err := manifest.validate(); err != nil {
		return failedE2ECleanup("cleanup fixture manifest is invalid")
	}
	dsn, err := mysql.ParseDSN(options.MySQLDSN)
	if err != nil || dsn.DBName != options.DatabaseAllowlist {
		return failedE2ECleanup("cleanup database name does not match the explicit allowlist")
	}
	db, err := sql.Open("mysql", options.MySQLDSN)
	if err != nil {
		return failedE2ECleanup("open cleanup database connection failed")
	}
	defer db.Close()
	if err := db.PingContext(ctx); err != nil {
		return failedE2ECleanup("ping cleanup database failed")
	}
	if !verifyE2EDatabaseIdentity(ctx, db, options.DatabaseAllowlist) {
		return failedE2ECleanup("connected cleanup database identity did not match the allowlist")
	}
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return failedE2ECleanup("begin cleanup transaction failed")
	}
	defer tx.Rollback()
	userID, username, err := loadE2ECleanupUser(ctx, tx, options.Username)
	if err != nil || username != options.Username || !strings.HasPrefix(username, options.RunID) {
		return failedE2ECleanup("cleanup account identity could not be verified")
	}
	if !verifyE2EDatabaseIdentity(ctx, tx, options.DatabaseAllowlist) {
		return failedE2ECleanup("cleanup database identity changed before deletion")
	}
	for _, table := range e2ECleanupUserTables {
		if _, err := tx.ExecContext(ctx, "DELETE FROM "+table+" WHERE user_id = ?", userID); err != nil {
			return failedE2ECleanup("cleanup deletion failed for " + table)
		}
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM users WHERE id = ? AND username = ?", userID, username); err != nil {
		return failedE2ECleanup("cleanup could not remove the run-scoped account")
	}
	if err := tx.Commit(); err != nil {
		return failedE2ECleanup("commit cleanup transaction failed")
	}
	if err := verifyE2ENoResidue(ctx, db, userID); err != nil {
		return failedE2ECleanup("cleanup residue verification failed: " + err.Error())
	}
	return e2eCleanupReport{Status: "passed", Verified: true}
}

func failedE2ECleanup(message string) e2eCleanupReport {
	return e2eCleanupReport{Status: "failed", Error: message}
}

func validateE2ECleanupOptions(options e2eOptions) error {
	if !options.Execute {
		return errors.New("cleanup requires execute mode")
	}
	if err := validateE2ERunID(options.RunID); err != nil {
		return err
	}
	if options.TargetName == "" || options.TargetName != options.TargetAllowlist {
		return errors.New("cleanup target does not match the explicit allowlist")
	}
	if strings.TrimSpace(options.DatabaseAllowlist) == "" {
		return errors.New("cleanup database allowlist is required")
	}
	if options.ConfirmCleanup != e2eCleanupConfirmation {
		return errors.New("run-ID-only cleanup confirmation is required")
	}
	if !strings.HasPrefix(options.Username, options.RunID) {
		return errors.New("cleanup username must start with the run ID")
	}
	if strings.TrimSpace(options.MySQLDSN) == "" {
		return errors.New("cleanup MySQL DSN is required")
	}
	return nil
}

type e2EQueryRowContext interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func verifyE2EDatabaseIdentity(ctx context.Context, db e2EQueryRowContext, expected string) bool {
	var actual string
	if err := db.QueryRowContext(ctx, "SELECT DATABASE()").Scan(&actual); err != nil {
		return false
	}
	return strings.TrimSpace(actual) == expected
}

func loadE2ECleanupUser(ctx context.Context, tx *sql.Tx, username string) (uint64, string, error) {
	var userID uint64
	var actualUsername string
	err := tx.QueryRowContext(ctx, "SELECT id, username FROM users WHERE username = ? LIMIT 1", username).Scan(&userID, &actualUsername)
	if err != nil || userID == 0 {
		return 0, "", err
	}
	return userID, actualUsername, nil
}

func verifyE2ENoResidue(ctx context.Context, db *sql.DB, userID uint64) error {
	for _, table := range e2ECleanupUserTables {
		var count int64
		if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+table+" WHERE user_id = ?", userID).Scan(&count); err != nil {
			return fmt.Errorf("count %s: %w", table, err)
		}
		if count != 0 {
			return fmt.Errorf("%s still contains %d rows for the deleted account", table, count)
		}
	}
	var userCount int64
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM users WHERE id = ?", userID).Scan(&userCount); err != nil {
		return fmt.Errorf("count deleted user: %w", err)
	}
	if userCount != 0 {
		return errors.New("run-scoped user row remains")
	}
	return nil
}
