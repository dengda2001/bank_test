package main

import (
	"net/url"
	"strings"
	"testing"
)

type testFormValues map[string]string

func (values testFormValues) Get(name string) string { return values[name] }

func validTenantInputForProfile() tenantInput {
	return tenantInput{Name: "Aoife Murphy", Email: "aoife@example.test", Status: "active"}
}

func repositoryTestTenant(userID uint64, name string) tenant {
	return tenant{UserID: userID, Name: name, Status: "active"}
}

func TestTenantInputParsesOnlyPersonAndPayerFields(t *testing.T) {
	input, err := tenantInputFromForm(url.Values{
		"name": {"Aoife Murphy"}, "display_alias": {"Aoife"}, "email": {"aoife@example.test"},
		"payer_id": {"payer-1"}, "payer_name_hint": {"AOIFE MURPHY"}, "status": {"active"},
		"monthly_rent": {"950.00"}, "room_id": {"9"}, "active_from": {"2026-09"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if input.Name != "Aoife Murphy" || input.DisplayAlias != "Aoife" || input.Email != "aoife@example.test" || input.PayerID != "payer-1" || input.PayerNameHint != "AOIFE MURPHY" {
		t.Fatalf("profile input=%+v", input)
	}
}

func TestTenantInputRequiresPayerNameWhenPayerIDIsSet(t *testing.T) {
	_, err := tenantInputFromForm(url.Values{"name": {"Aoife Murphy"}, "payer_id": {"payer-1"}})
	if err == nil || !strings.Contains(err.Error(), "payer name is required") {
		t.Fatalf("payer-only input error=%v", err)
	}
}

func TestValidateTenantInputRejectsInvalidEmail(t *testing.T) {
	err := validateTenantInput(tenantInput{Name: "Aoife Murphy", Email: "not-an-email", Status: "active"})
	if err == nil || !strings.Contains(err.Error(), "email") {
		t.Fatalf("invalid email error=%v", err)
	}
}

func TestTenantPayerNormalizationAndSharing(t *testing.T) {
	if got := normalizeTenantPayerName("  AOIFE   MURPHY "); got != "aoife murphy" {
		t.Fatalf("normalized payer=%q", got)
	}
	payerID := "shared-id"
	rows := classifyTenantPayerSharing([]tenantPayer{
		{ID: 1, TenantID: 7, PayerID: &payerID, PayerNameOriginal: "Aoife", PayerNameNormalized: "aoife"},
		{ID: 2, TenantID: 8, PayerID: &payerID, PayerNameOriginal: "Other", PayerNameNormalized: "other"},
	})
	if len(rows) != 2 || !rows[0].Shared || !rows[1].Shared {
		t.Fatalf("shared payer classification=%+v", rows)
	}
}

func TestTenantCreateFormExposesOnlyItsOptionalRoomPlanFields(t *testing.T) {
	page, err := executeTemplate(tenantTemplate, tenantPageData{ShowForm: true, Form: tenantRecord{Status: "active"}, ReturnURL: "/tenants", PostReturnURL: "/tenants?add=1"})
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []string{"property_id", "room_id", "arrangement_start_month", "room_plan", "new_responsibility"} {
		if !strings.Contains(page, `name="`+field+`"`) {
			t.Errorf("tenant create form is missing room-plan field %q", field)
		}
	}
	for _, field := range []string{"monthly_rent", "due_day", "rent_effective_from_month"} {
		if strings.Contains(page, `name="`+field+`"`) {
			t.Errorf("tenant form exposes room-owned rent-plan field %q", field)
		}
	}
}
