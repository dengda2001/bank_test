package main

import (
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"gorm.io/gorm/schema"
)

func readLandlordRentMigration(t *testing.T) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("..", "..", "migrations", "009_landlord_rent_model.sql"))
	if err != nil {
		t.Fatalf("read landlord rent migration: %v", err)
	}
	return strings.ToLower(string(body))
}

func TestLandlordRentMigrationDefinesStableRentalHierarchy(t *testing.T) {
	sql := readLandlordRentMigration(t)
	for _, fragment := range []string{
		"create table if not exists properties",
		"create table if not exists rooms",
		"create table if not exists tenancy_agreements",
		"create table if not exists agreement_parties",
		"create table if not exists rent_charges",
		"property_id bigint unsigned not null",
		"foreign key (property_id)",
		"foreign key (room_id)",
		"foreign key (agreement_id)",
		"foreign key (tenant_id)",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("migration missing %q", fragment)
		}
	}
}

func TestLandlordRentMigrationExtendsHistoricalFactsWithoutReplacingThem(t *testing.T) {
	sql := readLandlordRentMigration(t)
	for _, fragment := range []string{
		"alter table rent_obligations",
		"add column rent_charge_id bigint unsigned null",
		"add column tenant_name_snapshot varchar(191) null",
		"drop index idx_rent_obligations_user_tenant_period",
		"add unique key idx_rent_obligations_user_charge_tenant",
		"alter table manual_expenses",
		"add column property_id bigint unsigned null",
		"add column room_id bigint unsigned null",
		"add column record_status varchar(32) not null default 'active'",
		"alter table cash_receipts",
		"add column payment_transaction_id bigint unsigned null",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("migration missing %q", fragment)
		}
	}
}

func TestLandlordRentMigrationKeepsNewFactsUserScopedAndIdempotent(t *testing.T) {
	sql := readLandlordRentMigration(t)
	for _, table := range []string{
		"properties",
		"rooms",
		"tenancy_agreements",
		"agreement_parties",
		"rent_charges",
	} {
		needle := "create table if not exists " + table
		start := strings.Index(sql, needle)
		if start < 0 {
			t.Fatalf("migration missing table declaration %q", table)
		}
		end := strings.Index(sql[start:], "engine=innodb")
		if end < 0 {
			t.Fatalf("migration missing table terminator for %q", table)
		}
		body := sql[start : start+end]
		if !strings.Contains(body, "user_id bigint unsigned not null") {
			t.Fatalf("table %q is missing user_id ownership column", table)
		}
		if !strings.Contains(body, "if not exists") {
			t.Fatalf("table %q is not idempotent", table)
		}
	}
}

func TestPrototypeRoomRentMigrationPersistsRoomDefaultRentAndDueDay(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "..", "migrations", "012_prototype_room_rent_defaults.sql"))
	if err != nil {
		t.Fatalf("read prototype room rent migration: %v", err)
	}
	sql := strings.ToLower(string(body))
	for _, fragment := range []string{
		"alter table rooms",
		"add column monthly_rent_cents bigint not null default 0",
		"add column due_day int not null default 1",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("room rent migration missing %q", fragment)
		}
	}
}

func TestLandlordRentModelsUseMigrationTableNames(t *testing.T) {
	cases := []struct {
		name string
		got  string
		want string
	}{
		{name: "property", got: (property{}).TableName(), want: "properties"},
		{name: "room", got: (room{}).TableName(), want: "rooms"},
		{name: "room rent plan", got: (roomRentPlan{}).TableName(), want: "room_rent_plans"},
		{name: "room rent plan member", got: (roomRentPlanMember{}).TableName(), want: "room_rent_plan_members"},
		{name: "rent charge", got: (rentCharge{}).TableName(), want: "rent_charges"},
	}
	for _, tc := range cases {
		if tc.got != tc.want {
			t.Errorf("%s table name = %q; want %q", tc.name, tc.got, tc.want)
		}
	}
}

func TestLandlordRentModelsDefaultNewRecordsToActive(t *testing.T) {
	parsed, err := schema.Parse(&manualExpense{}, &sync.Map{}, schema.NamingStrategy{})
	if err != nil {
		t.Fatalf("parse manual expense schema: %v", err)
	}
	field := parsed.LookUpField("RecordStatus")
	if field == nil {
		t.Fatal("manual expense schema is missing RecordStatus")
	}
	if field.DefaultValue != "active" {
		t.Fatalf("manual expense RecordStatus default = %q; want active", field.DefaultValue)
	}

	parsed, err = schema.Parse(&rentCharge{}, &sync.Map{}, schema.NamingStrategy{})
	if err != nil {
		t.Fatalf("parse rent charge schema: %v", err)
	}
	field = parsed.LookUpField("RecordStatus")
	if field == nil || field.DefaultValue != "active" {
		got := "<missing>"
		if field != nil {
			got = field.DefaultValue
		}
		t.Fatalf("rent charge RecordStatus default = %q; want active", got)
	}
}
