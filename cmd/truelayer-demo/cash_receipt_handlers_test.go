package main

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCashReceiptErrorCodeUsesSentinelErrors(t *testing.T) {
	if got := cashReceiptErrorCode(errCashReceiptOverbalance); got != "cash_overbalance" {
		t.Fatalf("overbalance error code=%q", got)
	}
	if got := cashReceiptErrorCode(ErrRentFactsConflict); got != "rent_facts_conflict" {
		t.Fatalf("rent facts conflict error code=%q", got)
	}
	if got := cashReceiptErrorCode(errors.New("unrelated balance note")); got != "cash_receipt_failed" {
		t.Fatalf("untyped balance error code=%q", got)
	}
}

func TestParseCashReceiptFormNormalizesCentsCurrencyAndDate(t *testing.T) {
	draft, err := parseCashReceiptForm(testFormValues{
		"tenant_id":       "7",
		"payer_tenant_id": "8",
		"period":          "2026-09",
		"amount":          "400.00",
		"currency":        "eur",
		"received_at":     "2026-09-12",
		"note":            "  hand delivered  ",
		"idempotency_key": "cash-request-1",
	}, 42)
	if err != nil {
		t.Fatal(err)
	}
	if draft.TenantID != 7 || draft.PayerTenantID != 8 || draft.Period != time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC) || draft.AmountCents != 40000 || draft.Currency != "EUR" || draft.ReceivedAt != time.Date(2026, 9, 12, 0, 0, 0, 0, time.UTC) || draft.Note != "hand delivered" {
		t.Fatalf("cash form draft=%+v", draft)
	}
}

func TestParseCashReceiptFormRejectsAmbiguousPayer(t *testing.T) {
	_, err := parseCashReceiptForm(testFormValues{
		"tenant_id": "7", "payer_tenant_id": "8", "payer_name": "External payer",
		"period": "2026-09", "amount": "400.00", "currency": "EUR",
		"received_at": "2026-09-12", "idempotency_key": "cash-request-1",
	}, 42)
	if err == nil || !strings.Contains(err.Error(), "not both") {
		t.Fatalf("ambiguous payer error=%v", err)
	}
}

func TestParseCashReceiptFormRejectsMissingPeriodDateAndCurrency(t *testing.T) {
	base := testFormValues{
		"tenant_id":       "7",
		"period":          "2026-09",
		"amount":          "400.00",
		"currency":        "EUR",
		"received_at":     "2026-09-12",
		"idempotency_key": "cash-request-1",
	}
	for _, tc := range []struct {
		name string
		key  string
		val  string
		want string
	}{
		{name: "period", key: "period", want: "period"},
		{name: "date", key: "received_at", val: "not-a-date", want: "received_at"},
		{name: "currency", key: "currency", val: "GBP", want: "EUR"},
		{name: "idempotency", key: "idempotency_key", want: "idempotency"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			values := testFormValues{}
			for key, value := range base {
				values[key] = value
			}
			values[tc.key] = tc.val
			if _, err := parseCashReceiptForm(values, 42); err == nil || !strings.Contains(strings.ToLower(err.Error()), strings.ToLower(tc.want)) {
				t.Fatalf("error=%v want %q", err, tc.want)
			}
		})
	}
}

func TestCashReceiptTemplateRendersPreviewAndCorrectionActions(t *testing.T) {
	var body strings.Builder
	err := cashReceiptTemplate.Execute(&body, cashReceiptFormData{
		Tenant:            tenant{ID: 7, Name: "Aoife Murphy"},
		TenantID:          "7",
		Period:            "2026-09",
		Amount:            "400.00",
		Currency:          "EUR",
		ReceivedAt:        "2026-09-12",
		IdempotencyKey:    "cash-request-1",
		ExpectedAmount:    "EUR 1,000.00",
		CurrentPaidAmount: "EUR 600.00",
		AfterRemaining:    "EUR 0.00",
		Preview:           true,
	})
	if err != nil {
		t.Fatal(err)
	}
	page := body.String()
	for _, expected := range []string{"确认现金入账", "EUR 1,000.00", "EUR 600.00", "cash-request-1", `action="/cash-receipts"`, "payer_tenant_id", "/cash-receipts/new?period=2026-09&amp;tenant_id=7"} {
		if !strings.Contains(page, expected) {
			t.Fatalf("cash preview missing %q: %s", expected, page)
		}
	}
}

func TestCashReceiptStandaloneFormUsesSearchableTenantPicker(t *testing.T) {
	var body strings.Builder
	if err := cashReceiptTemplate.Execute(&body, cashReceiptFormData{Tenants: []tenant{{ID: 7, Name: "Aoife Murphy"}}}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body.String(), `name="tenant_id" data-searchable`) {
		t.Fatal("standalone cash receipt form does not opt into searchable tenant selection")
	}
}

func TestCashReceiptVoidTemplateShowsReasonAndOriginalReceipt(t *testing.T) {
	var body strings.Builder
	err := cashReceiptVoidTemplate.Execute(&body, cashReceiptVoidPageData{
		Receipt:       cashReceipt{ID: 9, ReceiptNumber: "cash-9", Note: "hand delivered", Status: cashReceiptStatusConfirmed},
		Tenant:        tenant{Name: "Aoife Murphy"},
		AmountDisplay: "EUR 400.00",
		DateDisplay:   "2026-09-12",
	})
	if err != nil {
		t.Fatal(err)
	}
	page := body.String()
	for _, expected := range []string{"撤销现金收款", "Aoife Murphy", "cash-9", "hand delivered", "void_reason", "原始收据与撤销原因会保留"} {
		if !strings.Contains(page, expected) {
			t.Fatalf("cash void page missing %q: %s", expected, page)
		}
	}
	if strings.Contains(page, "作废") {
		t.Fatalf("cash void page still says 作废: %s", page)
	}
}

func TestCashReceiptNewRequiresAuthenticatedDatabaseSession(t *testing.T) {
	a := testApp()
	rec := httptest.NewRecorder()
	a.handleCashReceiptNew(rec, httptest.NewRequest(http.MethodGet, "/cash-receipts/new", nil))
	if rec.Code != http.StatusFound || rec.Header().Get("Location") != "/" {
		t.Fatalf("status=%d location=%q want unauthenticated redirect", rec.Code, rec.Header().Get("Location"))
	}
}
