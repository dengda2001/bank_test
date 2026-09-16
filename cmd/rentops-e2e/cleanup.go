package main

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	"github.com/go-sql-driver/mysql"
)

type e2eCleanupTenantRow struct {
	ID           uint64
	Name         string
	DisplayAlias sql.NullString
	RoomAddress  string
}

type e2eCleanupPayerRow struct {
	TenantID          uint64
	PayerID           sql.NullString
	PayerNameOriginal string
}

type e2eCleanupObligationRow struct {
	ID                  uint64
	TenantID            uint64
	Period              string
	ExpectedAmountCents int64
	Currency            string
}

type e2eCleanupTransactionRow struct {
	ID                    uint64
	StableTransactionKey  string
	ProviderTransactionID sql.NullString
	SourceBatchID         sql.NullString
	AccountID             sql.NullString
	AccountName           sql.NullString
	Description           sql.NullString
	Reference             sql.NullString
}

type e2eCleanupAllocationRow struct {
	PaymentTransactionID uint64
	RentObligationID     sql.NullInt64
	TenantID             sql.NullInt64
	AmountCents          int64
	AllocationKind       string
	Status               string
}

type e2eCleanupActionRow struct {
	PaymentTransactionID uint64
	ActionKind           string
	Reason               string
	IdempotencyKey       sql.NullString
}

type e2eCleanupCashReceiptRow struct {
	TenantID         uint64
	RentObligationID uint64
	AmountCents      int64
	Currency         string
	Note             sql.NullString
	Status           string
	IdempotencyKey   string
}

type e2eCleanupDunningAttemptRow struct {
	TenantID          uint64
	RentObligationID  uint64
	RecipientEmail    string
	Subject           string
	Body              string
	SenderDisplayName string
	ReplyToEmail      string
	RequestKey        string
}

var e2ECleanupResidueTables = []string{
	"bank_connections",
	"tenants",
	"rent_obligations",
	"payment_transactions",
	"payment_allocations",
	"manual_expenses",
	"tenant_payers",
	"bank_sync_runs",
	"bank_sync_run_accounts",
	"payment_transaction_actions",
	"cash_receipts",
	"dunning_sender_configs",
	"dunning_send_attempts",
}

func cleanupE2ERun(ctx context.Context, options e2eOptions, manifest e2eFixtureManifest) e2eCleanupReport {
	if err := validateE2ECleanupOptions(options); err != nil {
		return failedE2ECleanup(err.Error())
	}
	if err := manifest.validate(); err != nil {
		return failedE2ECleanup("cleanup fixture manifest is invalid")
	}
	parsedDSN, err := mysql.ParseDSN(options.MySQLDSN)
	if err != nil || parsedDSN.DBName != options.DatabaseAllowlist {
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
	if ok := verifyE2EDatabaseIdentity(ctx, db, options.DatabaseAllowlist); !ok {
		return failedE2ECleanup("connected cleanup database identity did not match the allowlist")
	}

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return failedE2ECleanup("begin cleanup transaction failed")
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()
	userID, username, err := loadE2ECleanupUser(ctx, tx, options.Username)
	if err != nil {
		return failedE2ECleanup("cleanup account identity could not be verified")
	}
	if username != options.Username || !strings.HasPrefix(username, options.RunID) {
		return failedE2ECleanup("cleanup account username is not run-ID scoped")
	}
	if ok := verifyE2EDatabaseIdentity(ctx, tx, options.DatabaseAllowlist); !ok {
		return failedE2ECleanup("cleanup database identity changed before deletion")
	}
	if err := validateE2ECleanupRecords(ctx, tx, userID, manifest, options.SkipDunning); err != nil {
		// The cause stays in the report so an aborted cleanup is diagnosable
		// without re-running the whole acceptance suite.
		return failedE2ECleanup("cleanup ownership validation failed: " + err.Error())
	}
	if err := deleteE2ERunRows(ctx, tx, userID, username); err != nil {
		return failedE2ECleanup("cleanup delete transaction failed")
	}
	if err := tx.Commit(); err != nil {
		return failedE2ECleanup("commit cleanup transaction failed")
	}
	committed = true
	if err := verifyE2ENoResidue(ctx, db, userID, username); err != nil {
		return failedE2ECleanup("cleanup residue verification failed")
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

func validateE2ECleanupRecords(ctx context.Context, tx *sql.Tx, userID uint64, manifest e2eFixtureManifest, skipDunningDelivery bool) error {
	tenants, err := loadE2ECleanupTenants(ctx, tx, userID)
	if err != nil {
		return err
	}
	if len(tenants) != manifest.Expected.TenantCount {
		return errors.New("unexpected tenant count")
	}
	tenantIDs := make(map[string]uint64, len(tenants))
	expectedTenants := map[string]struct {
		alias   string
		address string
	}{
		manifest.Tenant.Name: {
			alias:   manifest.Tenant.DisplayAlias + " updated",
			address: manifest.Tenant.RoomAddress + " updated",
		},
		manifest.DunningTenant.Name: {
			alias:   manifest.DunningTenant.DisplayAlias,
			address: manifest.DunningTenant.RoomAddress,
		},
	}
	for _, row := range tenants {
		expected, ok := expectedTenants[row.Name]
		if !ok || row.ID == 0 || !row.DisplayAlias.Valid || row.DisplayAlias.String != expected.alias || row.RoomAddress != expected.address {
			return errors.New("tenant row is outside the run manifest")
		}
		if _, exists := tenantIDs[row.Name]; exists {
			return errors.New("duplicate tenant row")
		}
		tenantIDs[row.Name] = row.ID
	}
	if len(tenantIDs) != len(expectedTenants) {
		return errors.New("not every manifest tenant was found")
	}

	if err := validateE2ECleanupPayers(ctx, tx, userID, manifest, tenantIDs); err != nil {
		return err
	}
	obligations, err := loadE2ECleanupObligations(ctx, tx, userID)
	if err != nil {
		return err
	}
	obligationIDs, err := validateE2ECleanupObligations(obligations, manifest, tenantIDs)
	if err != nil {
		return err
	}
	transactions, err := loadE2ECleanupTransactions(ctx, tx, userID)
	if err != nil {
		return err
	}
	transactionIDs, err := validateE2ECleanupTransactions(transactions, manifest)
	if err != nil {
		return err
	}
	if err := validateE2ECleanupAllocations(ctx, tx, userID, transactionIDs, obligationIDs, tenantIDs, manifest); err != nil {
		return err
	}
	if err := validateE2ECleanupActions(ctx, tx, userID, transactionIDs, manifest); err != nil {
		return err
	}
	if err := validateE2ECleanupCashReceipts(ctx, tx, userID, tenantIDs, obligationIDs, manifest); err != nil {
		return err
	}
	if err := validateE2ECleanupDunning(ctx, tx, userID, tenantIDs, obligationIDs, manifest, skipDunningDelivery); err != nil {
		return err
	}
	if err := validateE2ECleanupEmptyTables(ctx, tx, userID); err != nil {
		return err
	}
	return validateE2ECleanupForeignReferences(ctx, tx, userID)
}

func loadE2ECleanupTenants(ctx context.Context, tx *sql.Tx, userID uint64) ([]e2eCleanupTenantRow, error) {
	rows, err := tx.QueryContext(ctx, "SELECT id, name, display_alias, room_address FROM tenants WHERE user_id = ? ORDER BY id", userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]e2eCleanupTenantRow, 0)
	for rows.Next() {
		var row e2eCleanupTenantRow
		if err := rows.Scan(&row.ID, &row.Name, &row.DisplayAlias, &row.RoomAddress); err != nil {
			return nil, err
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

func validateE2ECleanupPayers(ctx context.Context, tx *sql.Tx, userID uint64, manifest e2eFixtureManifest, tenantIDs map[string]uint64) error {
	rows, err := tx.QueryContext(ctx, "SELECT tenant_id, payer_id, payer_name_original FROM tenant_payers WHERE user_id = ? ORDER BY id", userID)
	if err != nil {
		return err
	}
	defer rows.Close()
	expected := map[uint64]struct {
		payerID string
		name    string
	}{
		tenantIDs[manifest.Tenant.Name]:        {payerID: manifest.Payer.PayerID, name: manifest.Payer.Name},
		tenantIDs[manifest.DunningTenant.Name]: {payerID: manifest.DunningTenant.PayerID, name: manifest.DunningTenant.PayerNameHint},
	}
	seen := make(map[uint64]bool, len(expected))
	count := 0
	for rows.Next() {
		var row e2eCleanupPayerRow
		if err := rows.Scan(&row.TenantID, &row.PayerID, &row.PayerNameOriginal); err != nil {
			return err
		}
		want, ok := expected[row.TenantID]
		if !ok || !row.PayerID.Valid || row.PayerID.String != want.payerID || row.PayerNameOriginal != want.name || seen[row.TenantID] {
			return errors.New("payer row is outside the run manifest")
		}
		seen[row.TenantID] = true
		count++
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if count != manifest.Expected.PayerCount || len(seen) != len(expected) {
		return errors.New("unexpected payer rows")
	}
	return nil
}

func loadE2ECleanupObligations(ctx context.Context, tx *sql.Tx, userID uint64) ([]e2eCleanupObligationRow, error) {
	rows, err := tx.QueryContext(ctx, "SELECT id, tenant_id, DATE_FORMAT(period_month, '%Y-%m'), expected_amount_cents, currency FROM rent_obligations WHERE user_id = ? ORDER BY id", userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]e2eCleanupObligationRow, 0)
	for rows.Next() {
		var row e2eCleanupObligationRow
		if err := rows.Scan(&row.ID, &row.TenantID, &row.Period, &row.ExpectedAmountCents, &row.Currency); err != nil {
			return nil, err
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

func validateE2ECleanupObligations(rows []e2eCleanupObligationRow, manifest e2eFixtureManifest, tenantIDs map[string]uint64) (map[string]uint64, error) {
	expected := map[string]struct {
		amount   int64
		currency string
	}{
		fmt.Sprintf("%d/2026-08", tenantIDs[manifest.Tenant.Name]):        {amount: manifest.Tenant.MonthlyRent.Cents, currency: "EUR"},
		fmt.Sprintf("%d/2026-09", tenantIDs[manifest.Tenant.Name]):        {amount: manifest.Tenant.MonthlyRent.Cents, currency: "EUR"},
		fmt.Sprintf("%d/2026-09", tenantIDs[manifest.DunningTenant.Name]): {amount: manifest.DunningTenant.MonthlyRent.Cents, currency: "EUR"},
	}
	if len(rows) != len(expected) {
		return nil, errors.New("unexpected rent obligation count")
	}
	result := make(map[string]uint64, len(rows))
	for _, row := range rows {
		key := fmt.Sprintf("%d/%s", row.TenantID, row.Period)
		want, ok := expected[key]
		if !ok || row.ID == 0 || row.ExpectedAmountCents != want.amount || row.Currency != want.currency || result[key] != 0 {
			return nil, errors.New("rent obligation row is outside the run manifest")
		}
		result[key] = row.ID
	}
	return result, nil
}

func loadE2ECleanupTransactions(ctx context.Context, tx *sql.Tx, userID uint64) ([]e2eCleanupTransactionRow, error) {
	rows, err := tx.QueryContext(ctx, "SELECT id, stable_transaction_key, provider_transaction_id, source_batch_id, account_id, account_name, description, reference FROM payment_transactions WHERE user_id = ? ORDER BY id", userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]e2eCleanupTransactionRow, 0)
	for rows.Next() {
		var row e2eCleanupTransactionRow
		if err := rows.Scan(&row.ID, &row.StableTransactionKey, &row.ProviderTransactionID, &row.SourceBatchID, &row.AccountID, &row.AccountName, &row.Description, &row.Reference); err != nil {
			return nil, err
		}
		result = append(result, row)
	}
	return result, rows.Err()
}

func validateE2ECleanupTransactions(rows []e2eCleanupTransactionRow, manifest e2eFixtureManifest) (map[string]uint64, error) {
	if len(rows) != manifest.Expected.BankTransactionCount {
		return nil, errors.New("unexpected payment transaction count")
	}
	result := make(map[string]uint64, len(rows))
	for _, row := range rows {
		if row.ID == 0 || result[row.StableTransactionKey] != 0 {
			return nil, errors.New("duplicate payment transaction row")
		}
		var fixture *e2eBankTransactionFixture
		for index := range manifest.Bank.Transactions {
			candidate := &manifest.Bank.Transactions[index]
			if row.StableTransactionKey == "provider:truelayer:"+candidate.NormalisedProviderTransactionID {
				fixture = candidate
				break
			}
		}
		if fixture == nil || !row.ProviderTransactionID.Valid || row.ProviderTransactionID.String != fixture.StoredProviderTransactionID() || !row.SourceBatchID.Valid || row.SourceBatchID.String != manifest.Bank.BatchID || !row.AccountID.Valid || row.AccountID.String != manifest.Bank.AccountID || !row.AccountName.Valid || row.AccountName.String != manifest.Bank.AccountName || !row.Description.Valid || row.Description.String != fixture.Description || !row.Reference.Valid || row.Reference.String != fixture.Reference {
			return nil, errors.New("payment transaction row is outside the run manifest")
		}
		result[fixture.StoredProviderTransactionID()] = row.ID
	}
	if len(result) != len(manifest.Bank.Transactions) {
		return nil, errors.New("not every manifest transaction was found")
	}
	return result, nil
}

func validateE2ECleanupAllocations(ctx context.Context, tx *sql.Tx, userID uint64, transactionIDs map[string]uint64, obligationIDs map[string]uint64, tenantIDs map[string]uint64, manifest e2eFixtureManifest) error {
	rows, err := tx.QueryContext(ctx, "SELECT payment_transaction_id, rent_obligation_id, tenant_id, amount_cents, allocation_kind, status FROM payment_allocations WHERE user_id = ? ORDER BY id", userID)
	if err != nil {
		return err
	}
	defer rows.Close()
	mainTenantID := tenantIDs[manifest.Tenant.Name]
	mainAugustID := obligationIDs[fmt.Sprintf("%d/2026-08", mainTenantID)]
	mainSeptemberID := obligationIDs[fmt.Sprintf("%d/2026-09", mainTenantID)]
	expected := map[string]map[string]int{
		cleanupAllocationKey(transactionIDs[manifest.Bank.Transactions[0].StoredProviderTransactionID()], mainSeptemberID, mainTenantID, 95000, "rent"): {"confirmed": 1, "voided": 1},
		cleanupAllocationKey(transactionIDs[manifest.Bank.Transactions[1].StoredProviderTransactionID()], mainAugustID, mainTenantID, 30000, "rent"):    {"confirmed": 1},
		cleanupAllocationKey(transactionIDs[manifest.Bank.Transactions[1].StoredProviderTransactionID()], 0, mainTenantID, 10000, "deposit"):            {"confirmed": 1},
		cleanupAllocationKey(transactionIDs[manifest.Bank.Transactions[3].StoredProviderTransactionID()], mainAugustID, mainTenantID, 30000, "rent"):    {"confirmed": 1},
		cleanupAllocationKey(transactionIDs[manifest.Bank.Transactions[4].StoredProviderTransactionID()], 0, 0, 5000, "other_income"):                   {"confirmed": 1},
	}
	seen := make(map[string]map[string]int, len(expected))
	count := 0
	for rows.Next() {
		var row e2eCleanupAllocationRow
		if err := rows.Scan(&row.PaymentTransactionID, &row.RentObligationID, &row.TenantID, &row.AmountCents, &row.AllocationKind, &row.Status); err != nil {
			return err
		}
		rentObligationID := nullableE2EInt64(row.RentObligationID)
		tenantID := nullableE2EInt64(row.TenantID)
		key := cleanupAllocationKey(row.PaymentTransactionID, rentObligationID, tenantID, row.AmountCents, row.AllocationKind)
		statuses, ok := expected[key]
		if !ok {
			return errors.New("payment allocation row is outside the run manifest")
		}
		if seen[key] == nil {
			seen[key] = make(map[string]int)
		}
		seen[key][row.Status]++
		if seen[key][row.Status] > statuses[row.Status] {
			return errors.New("payment allocation row has an unexpected duplicate")
		}
		count++
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if count != 6 || len(seen) != len(expected) {
		return errors.New("unexpected payment allocation count")
	}
	for key, statuses := range expected {
		for status, want := range statuses {
			if seen[key][status] != want {
				return errors.New("payment allocation status set is incomplete")
			}
		}
	}
	return nil
}

func cleanupAllocationKey(transactionID, obligationID, tenantID uint64, amountCents int64, kind string) string {
	return fmt.Sprintf("%d/%d/%d/%d/%s", transactionID, obligationID, tenantID, amountCents, kind)
}

func nullableE2EInt64(value sql.NullInt64) uint64 {
	if !value.Valid || value.Int64 <= 0 {
		return 0
	}
	return uint64(value.Int64)
}

func validateE2ECleanupActions(ctx context.Context, tx *sql.Tx, userID uint64, transactionIDs map[string]uint64, manifest e2eFixtureManifest) error {
	rows, err := tx.QueryContext(ctx, "SELECT payment_transaction_id, action_kind, reason, idempotency_key FROM payment_transaction_actions WHERE user_id = ? ORDER BY id", userID)
	if err != nil {
		return err
	}
	defer rows.Close()
	expected := map[string]struct {
		kind   string
		reason string
		key    string
	}{
		fmt.Sprintf("%d", transactionIDs[manifest.Bank.Transactions[2].StoredProviderTransactionID()]): {kind: "ignore", reason: manifest.RunID + " foreign currency does not match EUR ledger", key: manifest.RunID + "-ignore-foreign"},
		fmt.Sprintf("%d", transactionIDs[manifest.Bank.Transactions[0].StoredProviderTransactionID()]): {kind: "revoke_allocations", reason: manifest.RunID + " revoke and re-match", key: manifest.RunID + "-revoke-full"},
	}
	seen := make(map[string]bool, len(expected))
	count := 0
	for rows.Next() {
		var row e2eCleanupActionRow
		if err := rows.Scan(&row.PaymentTransactionID, &row.ActionKind, &row.Reason, &row.IdempotencyKey); err != nil {
			return err
		}
		want, ok := expected[fmt.Sprintf("%d", row.PaymentTransactionID)]
		if !ok || row.ActionKind != want.kind || row.Reason != want.reason || !row.IdempotencyKey.Valid || row.IdempotencyKey.String != want.key || seen[fmt.Sprintf("%d", row.PaymentTransactionID)] {
			return errors.New("payment transaction action row is outside the run manifest")
		}
		seen[fmt.Sprintf("%d", row.PaymentTransactionID)] = true
		count++
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if count != len(expected) || len(seen) != len(expected) {
		return errors.New("unexpected payment transaction action count")
	}
	return nil
}

func validateE2ECleanupCashReceipts(ctx context.Context, tx *sql.Tx, userID uint64, tenantIDs map[string]uint64, obligationIDs map[string]uint64, manifest e2eFixtureManifest) error {
	rows, err := tx.QueryContext(ctx, "SELECT tenant_id, rent_obligation_id, amount_cents, currency, note, status, idempotency_key FROM cash_receipts WHERE user_id = ? ORDER BY id", userID)
	if err != nil {
		return err
	}
	defer rows.Close()
	mainTenantID := tenantIDs[manifest.Tenant.Name]
	wantObligationID := obligationIDs[fmt.Sprintf("%d/2026-08", mainTenantID)]
	expected := map[string]struct {
		amount int64
		status string
		note   string
	}{
		manifest.RunID + "-cash-first":      {amount: 30000, status: "voided", note: manifest.RunID + " first cash receipt"},
		manifest.RunID + "-cash-correction": {amount: 35000, status: "confirmed", note: manifest.RunID + " corrected cash receipt"},
	}
	seen := make(map[string]bool, len(expected))
	count := 0
	for rows.Next() {
		var row e2eCleanupCashReceiptRow
		if err := rows.Scan(&row.TenantID, &row.RentObligationID, &row.AmountCents, &row.Currency, &row.Note, &row.Status, &row.IdempotencyKey); err != nil {
			return err
		}
		want, ok := expected[row.IdempotencyKey]
		if !ok || seen[row.IdempotencyKey] || row.TenantID != mainTenantID || row.RentObligationID != wantObligationID || row.AmountCents != want.amount || row.Currency != "EUR" || !row.Note.Valid || row.Note.String != want.note || row.Status != want.status {
			return errors.New("cash receipt row is outside the run manifest")
		}
		seen[row.IdempotencyKey] = true
		count++
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if count != len(expected) || len(seen) != len(expected) {
		return errors.New("unexpected cash receipt count")
	}
	return nil
}

func validateE2ECleanupDunning(ctx context.Context, tx *sql.Tx, userID uint64, tenantIDs map[string]uint64, obligationIDs map[string]uint64, manifest e2eFixtureManifest, skipDunningDelivery bool) error {
	var displayName, replyTo string
	err := tx.QueryRowContext(ctx, "SELECT display_name, reply_to_email FROM dunning_sender_configs WHERE user_id = ?", userID).Scan(&displayName, &replyTo)
	if err != nil || displayName != manifest.RunID+" landlord" || replyTo != manifest.RunID+"@invalid.test" {
		return errors.New("dunning sender configuration is outside the run manifest")
	}
	rows, err := tx.QueryContext(ctx, "SELECT tenant_id, rent_obligation_id, recipient_email, subject, body, sender_display_name, reply_to_email, request_key FROM dunning_send_attempts WHERE user_id = ? ORDER BY id", userID)
	if err != nil {
		return err
	}
	defer rows.Close()
	dunningTenantID := tenantIDs[manifest.DunningTenant.Name]
	dunningObligationID := obligationIDs[fmt.Sprintf("%d/2026-09", dunningTenantID)]
	count := 0
	for rows.Next() {
		var row e2eCleanupDunningAttemptRow
		if err := rows.Scan(&row.TenantID, &row.RentObligationID, &row.RecipientEmail, &row.Subject, &row.Body, &row.SenderDisplayName, &row.ReplyToEmail, &row.RequestKey); err != nil {
			return err
		}
		if row.TenantID != dunningTenantID || row.RentObligationID != dunningObligationID || row.RecipientEmail != manifest.DunningTenant.Email || !strings.Contains(row.Body, manifest.DunningTenant.Name) || row.SenderDisplayName != manifest.RunID+" landlord" || row.ReplyToEmail != manifest.RunID+"@invalid.test" || row.RequestKey != manifest.RunID+"-dunning-send" {
			return errors.New("dunning attempt is outside the run manifest")
		}
		count++
	}
	if err := rows.Err(); err != nil {
		return err
	}
	expectedAttempts := 1
	if skipDunningDelivery {
		expectedAttempts = 0
	}
	if count != expectedAttempts {
		return errors.New("unexpected dunning attempt count")
	}
	return nil
}

func validateE2ECleanupEmptyTables(ctx context.Context, tx *sql.Tx, userID uint64) error {
	for _, table := range []string{"bank_connections", "manual_expenses", "bank_sync_runs", "bank_sync_run_accounts"} {
		var count int64
		query := fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE user_id = ?", table)
		if err := tx.QueryRowContext(ctx, query, userID).Scan(&count); err != nil {
			return err
		}
		if count != 0 {
			return fmt.Errorf("table %s is not empty", table)
		}
	}
	return nil
}

func validateE2ECleanupForeignReferences(ctx context.Context, tx *sql.Tx, userID uint64) error {
	queries := []string{
		"SELECT COUNT(*) FROM payment_allocations WHERE user_id <> ? AND (confirmed_by_user_id = ? OR voided_by_user_id = ?)",
		"SELECT COUNT(*) FROM payment_transaction_actions WHERE user_id <> ? AND acted_by_user_id = ?",
		"SELECT COUNT(*) FROM cash_receipts WHERE user_id <> ? AND (recorded_by_user_id = ? OR voided_by_user_id = ?)",
		"SELECT COUNT(*) FROM rent_obligations WHERE user_id <> ? AND voided_by_user_id = ?",
		"SELECT COUNT(*) FROM tenant_payers WHERE user_id <> ? AND removed_by_user_id = ?",
	}
	for index, query := range queries {
		args := []any{userID, userID}
		if index == 0 || index == 2 {
			args = append(args, userID)
		}
		var count int64
		if err := tx.QueryRowContext(ctx, query, args...).Scan(&count); err != nil {
			return err
		}
		if count != 0 {
			return errors.New("run account is referenced by another user")
		}
	}
	return nil
}

func deleteE2ERunRows(ctx context.Context, tx *sql.Tx, userID uint64, username string) error {
	for _, table := range []string{
		"dunning_send_attempts",
		"dunning_sender_configs",
		"payment_transaction_actions",
		"cash_receipts",
		"payment_allocations",
		"bank_sync_run_accounts",
		"bank_sync_runs",
		"manual_expenses",
		"rent_obligations",
		"payment_transactions",
		"tenant_payers",
		"tenants",
		"bank_connections",
	} {
		query := fmt.Sprintf("DELETE FROM %s WHERE user_id = ?", table)
		if _, err := tx.ExecContext(ctx, query, userID); err != nil {
			return err
		}
	}
	result, err := tx.ExecContext(ctx, "DELETE FROM users WHERE id = ? AND username = ?", userID, username)
	if err != nil {
		return err
	}
	affected, err := result.RowsAffected()
	if err != nil || affected != 1 {
		return errors.New("cleanup account delete affected an unexpected number of users")
	}
	return nil
}

func verifyE2ENoResidue(ctx context.Context, db *sql.DB, userID uint64, username string) error {
	var userCount int64
	if err := db.QueryRowContext(ctx, "SELECT COUNT(*) FROM users WHERE id = ? OR username = ?", userID, username).Scan(&userCount); err != nil {
		return err
	}
	if userCount != 0 {
		return errors.New("cleanup user residue remains")
	}
	for _, table := range e2ECleanupResidueTables {
		var count int64
		query := fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE user_id = ?", table)
		if err := db.QueryRowContext(ctx, query, userID).Scan(&count); err != nil {
			return err
		}
		if count != 0 {
			return fmt.Errorf("cleanup residue remains in %s", table)
		}
	}
	return nil
}
