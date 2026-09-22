package main

import "testing"

func TestTenantProfileSchemaContainsOnlyPersonFieldsOnMySQL(t *testing.T) {
	db, sqlDB := openLedgerMySQLTestDB(t)
	if err := runMigrations(sqlDB, "../../migrations"); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	for _, legacyColumn := range []string{"monthly_rent_cents", "currency", "due_day", "rent_start_date", "rent_end_date", "room_label", "room_address", "property_hint", "payer_id", "payer_name_hint"} {
		var count int64
		if err := db.Raw(`SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'tenants' AND COLUMN_NAME = ?`, legacyColumn).Scan(&count).Error; err != nil {
			t.Fatal(err)
		}
		if count != 0 {
			t.Errorf("tenants.%s still exists", legacyColumn)
		}
	}
	for _, requiredColumn := range []string{"user_id", "name", "display_alias", "email", "status", "created_at", "updated_at"} {
		var count int64
		if err := db.Raw(`SELECT COUNT(*) FROM information_schema.COLUMNS WHERE TABLE_SCHEMA = DATABASE() AND TABLE_NAME = 'tenants' AND COLUMN_NAME = ?`, requiredColumn).Scan(&count).Error; err != nil {
			t.Fatal(err)
		}
		if count != 1 {
			t.Errorf("tenants.%s missing", requiredColumn)
		}
	}
}
