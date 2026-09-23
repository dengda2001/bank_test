package main

import (
	"net/url"
	"strings"
	"testing"
)

func TestManualTenantSuggestionsFindCloseBankNamesOnly(t *testing.T) {
	tenants := []tenant{
		{ID: 1, Name: "Ardra M Punathil"},
		{ID: 2, Name: "Hardik Gattu"},
		{ID: 3, Name: "Aoife Murphy"},
	}
	for _, tc := range []struct {
		payer string
		kind  string
		want  uint64
	}{
		{"Ardra Punathil", "confirmed", 1},
		{"Hardhik Gattu", "confirmed", 2},
		{"MR ARDRA M PUNATHIL", "confirmed", 1},
		{"Refund from Bank", "confirmed", 0},
		{"Hardhik Gattu", "inferred", 0},
	} {
		got := manualTenantSuggestions(tc.payer, tc.kind, tenants)
		if tc.want == 0 && len(got) != 0 {
			t.Errorf("%q/%q got unexpected suggestions %+v", tc.payer, tc.kind, got)
		} else if tc.want != 0 && (len(got) == 0 || got[0].ID != tc.want) {
			t.Errorf("%q/%q got %+v, want tenant %d first", tc.payer, tc.kind, got, tc.want)
		}
	}
}

func TestManualTenantSuggestionLinkOnlyOpensReview(t *testing.T) {
	review := transactionMatchReviewData{
		Source:           transactionPageRow{ID: "42"},
		SuggestedTenants: []transactionReviewTenant{{ID: 7, Name: "Ardra M Punathil"}},
	}
	review.setURLs(url.Values{"match_status": {"pending"}, "match": {"42"}})
	link := review.SuggestedTenants[0].URL
	if !strings.Contains(link, "match=42") || !strings.Contains(link, "match_tenant=7") || !strings.Contains(link, "match_status=pending") || strings.Contains(link, "/confirm") {
		t.Fatalf("suggestion must only open the review drawer: %q", link)
	}
	page := renderTransactionListPage(t, transactionListPageData{MatchReview: &review})
	if !strings.Contains(page, "姓名相近的租客 · 仅供核对") || !strings.Contains(page, "Ardra M Punathil") {
		t.Fatalf("manual tenant suggestion is missing from review drawer")
	}
}

func TestMatchMethodLabelUsesOnlyEffectiveRentAllocations(t *testing.T) {
	auto := paymentAllocation{AllocationKind: allocationKindRent, Status: allocationStatusConfirmed, ConfirmationSource: "auto_exact_name"}
	manual := paymentAllocation{AllocationKind: allocationKindRent, Status: allocationStatusConfirmed, ConfirmationSource: "manual_name"}
	voided := paymentAllocation{AllocationKind: allocationKindRent, Status: "voided", ConfirmationSource: "auto_name"}
	for _, tc := range []struct {
		allocations []paymentAllocation
		want        string
	}{
		{nil, ""},
		{[]paymentAllocation{auto}, "自动匹配"},
		{[]paymentAllocation{manual}, "手动匹配"},
		{[]paymentAllocation{manual, auto}, "自动＋手动"},
		{[]paymentAllocation{manual, voided}, "手动匹配"},
	} {
		got := enrichTransactionPageRow(transactionPageRow{}, paymentTransaction{Currency: "EUR", AmountCents: 10000}, tc.allocations).MatchMethodLabel
		if got != tc.want {
			t.Errorf("allocations=%+v got %q want %q", tc.allocations, got, tc.want)
		}
	}
}

func TestTransactionListDisplaysAutoMatchMethod(t *testing.T) {
	page := renderTransactionListPage(t, transactionListPageData{TransactionRows: []transactionPageRow{{
		ID: "7", InternalID: "7", DetailKey: "7", PayerName: "Aoife Murphy", MatchStatus: "matched", MatchStatusLabel: "已关联", MatchMethodLabel: "自动匹配",
	}}})
	if strings.Count(page, "自动匹配") < 2 {
		t.Fatalf("desktop and mobile list should show the method label")
	}
}
