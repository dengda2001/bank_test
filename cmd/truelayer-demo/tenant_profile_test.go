package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
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

func TestClassifyTenantPayersMarksSharedStableIDAcrossDifferentNames(t *testing.T) {
	rows := classifyTenantPayerSharing([]tenantPayer{
		{ID: 1, TenantID: 10, PayerID: ptrString("payer-1"), PayerNameNormalized: "parent"},
		{ID: 2, TenantID: 11, PayerID: ptrString("payer-1"), PayerNameNormalized: "guardian"},
	})
	if !rows[0].Shared || !rows[1].Shared {
		t.Fatalf("shared stable id rows=%+v want both shared", rows)
	}
}

func TestRentEndChangeOnlyAffectsMonthsAfterEndMonth(t *testing.T) {
	end := time.Date(2026, 10, 2, 0, 0, 0, 0, time.UTC)
	if !obligationIsAfterRentEnd(time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC), &end) {
		t.Fatal("November obligation should be affected")
	}
	if obligationIsAfterRentEnd(time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), &end) {
		t.Fatal("end month obligation should remain applicable")
	}
}

func TestPaginateTenantBillingMonthsReturnsStablePageAndTotal(t *testing.T) {
	rows := []tenantBillingMonth{
		{Period: "2026-05"}, {Period: "2026-04"}, {Period: "2026-03"},
		{Period: "2026-02"}, {Period: "2026-01"},
	}
	page, totalPages, err := paginateTenantBillingMonths(rows, 2, 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(page) != 2 || page[0].Period != "2026-03" || page[1].Period != "2026-02" || totalPages != 3 {
		t.Fatalf("page=%+v totalPages=%d want March/February and 3 pages", page, totalPages)
	}
}

func TestPaginateTenantBillingMonthsDoesNotOverflowOnLargePage(t *testing.T) {
	rows, totalPages, err := paginateTenantBillingMonths([]tenantBillingMonth{{Period: "2026-05"}}, int(^uint(0)>>1), 12)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 0 || totalPages != 1 {
		t.Fatalf("rows=%+v totalPages=%d want empty page and one total page", rows, totalPages)
	}
}

func TestParseTenantHistoryRangeDefaultsToTwelveMonths(t *testing.T) {
	from, to, page, pageSize, err := parseTenantHistoryRange(testQueryValues{}, time.Date(2026, 9, 16, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if from.Format("2006-01") != "2025-10" || to.Format("2006-01") != "2026-09" || page != 1 || pageSize != 12 {
		t.Fatalf("range=%s..%s page=%d size=%d", from.Format("2006-01"), to.Format("2006-01"), page, pageSize)
	}
}

func TestTenantDetailTemplateShowsNameOnlyPayerAndHistoryControls(t *testing.T) {
	var body strings.Builder
	err := tenantDetailTemplate.Execute(&body, tenantDetailPageData{
		Tenant:  tenantRecord{ID: "7", Name: "Aoife Murphy", DisplayAlias: "Aoife", Email: "aoife@example.test"},
		Payers:  []tenantPayerRecord{{ID: "8", Name: "Mike", Shared: true}},
		History: tenantBillingHistoryPage{FromPeriod: "2025-10", ToPeriod: "2026-09", Page: 1, PageSize: 12, TotalRows: 1, Rows: []tenantBillingMonth{{PeriodLabel: "2026年9月", StatusLabel: "已缴清"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	page := body.String()
	for _, expected := range []string{
		"aoife@example.test", "Mike", "共享／冲突候选", "from_month", "to_month", "/tenants/7/payers",
		"2026年9月", "租客详情",
	} {
		if !strings.Contains(page, expected) {
			t.Fatalf("tenant detail template missing %q", expected)
		}
	}
}

func TestTenantDetailRequiresAuthenticatedDatabaseSession(t *testing.T) {
	a := testApp()
	rec := httptest.NewRecorder()
	a.handleTenantSubroute(rec, httptest.NewRequest(http.MethodGet, "/tenants/7", nil))
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/" {
		t.Fatalf("status=%d location=%q want unauthenticated redirect", rec.Code, rec.Header().Get("Location"))
	}
}

func TestPaymentSourceLabelDistinguishesBankAndCash(t *testing.T) {
	if got := paymentSourceLabel("truelayer"); got != "银行" {
		t.Fatalf("truelayer source=%q want 银行", got)
	}
	if got := paymentSourceLabel("cash"); got != "现金" {
		t.Fatalf("cash source=%q want 现金", got)
	}
}

type testQueryValues map[string]string

func (v testQueryValues) Get(key string) string { return v[key] }

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
