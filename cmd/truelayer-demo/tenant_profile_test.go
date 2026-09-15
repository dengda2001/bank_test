package main

import (
	"os"
	"strings"
	"testing"
)

func TestNormalizeTenantPayerNameKeepsNameOnlyBankIdentity(t *testing.T) {
	if got := normalizeTenantPayerName("  Mike   "); got != "mike" {
		t.Fatalf("normalized payer name=%q want %q", got, "mike")
	}
	if got := normalizeTenantPayerName("ZR Institute"); got != "zr institute" {
		t.Fatalf("normalized payer name=%q want %q", got, "zr institute")
	}
}

func TestTenantInputDefaultsBillingStartToRentStart(t *testing.T) {
	input, err := tenantInputFromForm(testFormValues{
		"name": "Aoife Murphy", "monthly_rent": "950", "currency": "EUR",
		"rent_start_date": "2026-09-20", "room_address": "Dublin", "status": "active",
	})
	if err != nil {
		t.Fatal(err)
	}
	if input.BillingStartDate != "2026-09-20" {
		t.Fatalf("billing start=%q want rent start date", input.BillingStartDate)
	}
}

func TestValidateTenantInputRejectsBillingOutsideRentPeriod(t *testing.T) {
	input := validTenantInputForProfile()
	input.RentStartDate = "2026-09-20"
	input.RentEndDate = "2026-10-02"
	input.BillingStartDate = "2026-10-03"

	if err := validateTenantInput(input); err == nil || !strings.Contains(err.Error(), "billing start") {
		t.Fatalf("validation error=%v want billing start boundary error", err)
	}
}

func TestValidateTenantInputRejectsInvalidEmail(t *testing.T) {
	input := validTenantInputForProfile()
	input.Email = "not-an-email"

	if err := validateTenantInput(input); err == nil || !strings.Contains(err.Error(), "email") {
		t.Fatalf("validation error=%v want email error", err)
	}
}

func TestTenantPayerMigrationSupportsNameOnlyRelationsAndLegacyBackfill(t *testing.T) {
	body, err := os.ReadFile("../../migrations/004_tenant_profile_and_payers.sql")
	if err != nil {
		t.Fatal(err)
	}
	sql := string(body)
	for _, fragment := range []string{
		"ADD COLUMN display_alias",
		"ADD COLUMN email",
		"DROP INDEX idx_tenants_user_payer_id",
		"CREATE TABLE IF NOT EXISTS tenant_payers",
		"payer_name_original varchar(191) NOT NULL",
		"payer_name_normalized varchar(191) NOT NULL",
		"payer_id varchar(191) NULL",
		"removed_at timestamp NULL",
		"INSERT INTO tenant_payers",
		"NOT EXISTS",
	} {
		if !strings.Contains(sql, fragment) {
			t.Fatalf("migration missing %q", fragment)
		}
	}
}

func TestValidateTenantPayerInputAllowsNameOnlyIdentity(t *testing.T) {
	if err := validateTenantPayerInput(tenantPayerInput{Name: " Mike "}); err != nil {
		t.Fatalf("name-only payer rejected: %v", err)
	}
}

func TestValidateTenantPayerInputRequiresName(t *testing.T) {
	if err := validateTenantPayerInput(tenantPayerInput{PayerID: "payer-123"}); err == nil {
		t.Fatal("expected payer name to be required when payer id is present")
	}
}

func TestClassifyTenantPayersMarksSharedNameWithoutStableID(t *testing.T) {
	rows := classifyTenantPayerSharing([]tenantPayer{
		{ID: 1, TenantID: 10, PayerNameNormalized: "mike"},
		{ID: 2, TenantID: 11, PayerNameNormalized: "mike"},
		{ID: 3, TenantID: 10, PayerID: ptrString("payer-3"), PayerNameNormalized: "other"},
	})
	if !rows[0].Shared || !rows[1].Shared {
		t.Fatalf("shared payer rows=%+v want both mike rows shared", rows)
	}
	if rows[2].Shared {
		t.Fatalf("unshared payer row=%+v unexpectedly shared", rows[2])
	}
}

type testFormValues map[string]string

func (v testFormValues) Get(key string) string { return v[key] }

func validTenantInputForProfile() tenantInput {
	return tenantInput{
		Name: "Aoife Murphy", MonthlyRent: 950, Currency: "EUR",
		IntervalUnit: "month", IntervalCount: 1, BillingStartDate: "2026-09-01",
		DueDay: 5, RentStartDate: "2026-09-01", Status: "active",
		RoomAddress: "14 Harcourt Street, Dublin",
	}
}
