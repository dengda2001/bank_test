package main

import (
	"net/url"
	"strings"
	"testing"
)

func TestCashReceiptPageRendersPrototypeListCardsAndDrawer(t *testing.T) {
	var body strings.Builder
	err := cashReceiptPageTemplate.Execute(&body, cashReceiptPageData{
		Period: "2026-09", StatusFilter: "all", ShowForm: true,
		Form: cashReceiptFormData{Period: "2026-09", Currency: "EUR", ReceivedAt: "2026-09-19", IdempotencyKey: "cash-test", Tenants: []tenant{{ID: 3, Name: "Tenant A"}}},
		Rows: []cashReceiptPageRow{{ID: 8, ReceiptNumber: "CASH-8", TenantID: 3, TenantName: "Tenant A", RoomLabel: "03", Amount: "€400.00", Period: "2026-09", ReceivedAt: "2026-09-19", Status: cashReceiptStatusConfirmed, StatusLabel: "已确认"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	page := body.String()
	for _, marker := range []string{"现金收款", "搜索当前列表", "cash-receipt-table", "cash-receipt-mobile-list", "entity-drawer-backdrop", `name="tenant_id" data-searchable`, "return_to", "预览入账"} {
		if !strings.Contains(page, marker) {
			t.Errorf("cash receipt page missing %q", marker)
		}
	}
}

func TestCashReceiptListURLPreservesListContext(t *testing.T) {
	values := url.Values{
		"list_period": {"2026-09"}, "list_status": {cashReceiptStatusVoided}, "list_search": {"Tenant A"},
	}
	got := cashReceiptListReturnURL(values, "", "cash_receipt_saved", false, "")
	parsed, err := url.Parse(got)
	if err != nil {
		t.Fatal(err)
	}
	query := parsed.Query()
	if query.Get("period") != "2026-09" || query.Get("status") != cashReceiptStatusVoided || query.Get("search") != "Tenant A" || query.Get("message") != "cash_receipt_saved" {
		t.Fatalf("cash receipt list redirect=%q", got)
	}
}

// One error code, one message, one render point. Both cash receipt codes are
// reachable from two parallel forms, and on /cash-receipts the page-level banner
// and the drawer sit inside a single render: before this was fixed the same code
// was printed there twice, in two different sentences. Whether the second copy was
// legible depended on the width (a scrim on desktop, a covering bottom sheet at
// <=640), so neither sentence could be assumed to be the one the user read.
//
// The two assertions catch the two halves of that separately, which is why both
// are needed. The error-notice count is what fails on the old list page (two
// elements, one per wording) even though the canonical sentence alone appeared
// only once there. The canonical count is what fails on the old inline page, whose
// copy read 币种 instead of 日期 and so contained the canonical sentence not at all.
func TestCashReceiptErrorCodeRendersOneMessage(t *testing.T) {
	for _, tc := range []struct{ code, text string }{
		{"cash_receipt_failed", cashReceiptFailedText},
		{"cash_overbalance", cashOverbalanceText},
	} {
		t.Run(tc.code, func(t *testing.T) {
			for _, de := range []struct {
				name   string
				render func(*strings.Builder) error
			}{
				{name: "drawer", render: func(body *strings.Builder) error {
					// ShowForm is what the list loader derives from the same error
					// code, so a page carrying the code always has it set.
					return cashReceiptPageTemplate.Execute(body, cashReceiptPageData{
						Period: "2026-09", StatusFilter: "all", Error: tc.code, ShowForm: true,
					})
				}},
				{name: "inline", render: func(body *strings.Builder) error {
					return cashReceiptTemplate.Execute(body, cashReceiptFormData{Error: tc.code})
				}},
			} {
				t.Run(de.name, func(t *testing.T) {
					var body strings.Builder
					if err := de.render(&body); err != nil {
						t.Fatal(err)
					}
					page := body.String()
					if got := strings.Count(page, tc.text); got != 1 {
						t.Fatalf("%s rendered %d copies of %q, want exactly one", tc.code, got, tc.text)
					}
					if got := strings.Count(page, `class="notice error"`); got != 1 {
						t.Fatalf("%s rendered %d error notices, want exactly one", tc.code, got)
					}
				})
			}
		})
	}
}

// The page-level copy of those messages was deleted, so the drawer has to carry
// them alone -- which holds only because an error code opens the drawer even
// without add=1. The two redirect producers that land on the list page do both set
// add (/cash-receipts?error=...&add=1 and cashReceiptFormErrorURL, which hard-codes
// add=true), but the removal must not depend on that: if the guard ever dropped the
// `error` arm, the message would render nowhere.
func TestCashReceiptDrawerIsOpenWheneverAnErrorCodeIsSet(t *testing.T) {
	if !cashReceiptDrawerIsOpen(url.Values{"error": {"cash_receipt_failed"}}) {
		t.Fatal("an error code without add=1 must still open the drawer, or removing the page-level notice silences the message")
	}
	if !cashReceiptDrawerIsOpen(url.Values{"error": {"cash_overbalance"}}) {
		t.Fatal("an error code without add=1 must still open the drawer")
	}
	if !cashReceiptDrawerIsOpen(url.Values{"add": {"1"}}) {
		t.Fatal("add=1 must open the drawer for a fresh entry")
	}
	if cashReceiptDrawerIsOpen(url.Values{"period": {"2026-09"}, "status": {"all"}}) {
		t.Fatal("a plain list visit must not open the drawer")
	}
}
