package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func readRoomRentPlanResetMigration(t *testing.T) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("..", "..", "migrations", "014_room_rent_plan_reset.sql"))
	if err != nil {
		t.Fatalf("read room rent plan reset migration: %v", err)
	}
	return string(body)
}

func TestRoomRentPlanResetMigrationIsFlatAndCommentFree(t *testing.T) {
	body := readRoomRentPlanResetMigration(t)
	if strings.Contains(body, "--") || strings.Contains(body, "/*") {
		t.Fatal("reset migration must not contain comments because the flat SQL runner drops comment fragments")
	}
	statements := splitSQLStatements(body)
	if len(statements) == 0 {
		t.Fatal("reset migration produced no SQL statements")
	}
	for i, statement := range statements {
		if strings.TrimSpace(statement) == "" {
			t.Fatalf("reset migration statement %d is empty", i)
		}
	}
}

func TestRoomRentPlanResetMigrationDefinesTheNewRelationalContract(t *testing.T) {
	sql := strings.ToLower(readRoomRentPlanResetMigration(t))
	for _, fragment := range []string{
		"alter table tenancy_agreements",
		"rename to room_rent_plans",
		"alter table agreement_parties",
		"rename to room_rent_plan_members",
		"effective_from_month",
		"effective_to_month",
		"rent_plan_version",
		"room_rent_plan_member_id",
		"payer_tenant_id",
		"on delete restrict",
		"foreign key (user_id, room_id, room_rent_plan_id)",
		"foreign key (user_id, rent_charge_id, room_rent_plan_id, period_month)",
	} {
		if !strings.Contains(sql, fragment) {
			t.Errorf("reset migration is missing %q", fragment)
		}
	}
	if strings.Contains(sql, "insert into") || strings.Contains(sql, "update ") || strings.Contains(sql, "delete from") {
		t.Fatal("reset migration must not copy, transform, or delete business data")
	}
}
