package main

import (
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestTransactionReviewMonthsShowsPaidMonthEvidenceButOnlyOpenMonthSelectable(t *testing.T) {
	august := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	september := august.AddDate(0, 1, 0)
	source := paymentTransaction{Currency: "EUR", ParsedPeriodMonth: &august}
	paidID := uint64(10)
	voidedAt := time.Now()
	allocations := []paymentAllocation{
		{RentObligationID: &paidID, PaymentTransactionID: 20, AmountCents: 95000, Status: allocationStatusConfirmed, AllocationKind: allocationKindRent},
		{RentObligationID: &paidID, PaymentTransactionID: 21, AmountCents: 30000, Status: allocationStatusVoided, AllocationKind: allocationKindRent, VoidedAt: &voidedAt},
	}
	rows := transactionReviewMonths(source, 95000, []rentObligation{{ID: paidID, PeriodMonth: august, ExpectedAmountCents: 95000, PaidAmountCents: 95000, Currency: "EUR"}, {ID: 11, PeriodMonth: september, ExpectedAmountCents: 95000, Currency: "EUR"}}, allocations, map[uint64]paymentTransaction{20: {ID: 20, Direction: "income", Currency: "EUR", Description: "Original August rent"}, 21: {ID: 21, Direction: "income", Currency: "EUR", Description: "Voided payment"}})
	if len(rows) != 2 || !rows[0].Highlighted || rows[0].Selectable || !rows[1].Selectable || len(rows[0].Evidence) != 1 || rows[0].Evidence[0].Description != "Original August rent" {
		t.Fatalf("month review lost payment evidence or allowed a paid month: %+v", rows)
	}
}

func TestTransactionReviewHistoryUsesEffectiveMatchedMonths(t *testing.T) {
	august := time.Date(2026, 8, 1, 0, 0, 0, 0, time.UTC)
	september := august.AddDate(0, 1, 0)
	augustID, septemberID := uint64(7), uint64(8)
	periods := transactionReviewMatchedPeriods([]paymentAllocation{
		{PaymentTransactionID: 50, RentObligationID: &septemberID, Status: allocationStatusConfirmed, AllocationKind: allocationKindRent},
		{PaymentTransactionID: 50, RentObligationID: &augustID, Status: allocationStatusConfirmed, AllocationKind: allocationKindRent},
		{PaymentTransactionID: 50, RentObligationID: &augustID, Status: allocationStatusConfirmed, AllocationKind: allocationKindRent},
		{PaymentTransactionID: 51, RentObligationID: &augustID, Status: allocationStatusVoided, AllocationKind: allocationKindRent},
		{PaymentTransactionID: 52, RentObligationID: &augustID, Status: allocationStatusConfirmed, AllocationKind: allocationKindDeposit},
	}, map[uint64]rentObligation{augustID: {PeriodMonth: august}, septemberID: {PeriodMonth: september}})
	if got := strings.Join(periods[50], "、"); got != "2026-08、2026-09" {
		t.Fatalf("effective matched periods = %s", got)
	}
	if len(periods[51]) != 0 || len(periods[52]) != 0 {
		t.Fatalf("voided or deposit allocations appeared as rent history: %+v", periods)
	}
}

func TestTransactionReviewURLsKeepListContextAndRejectExternalReturn(t *testing.T) {
	query := url.Values{"match_status": {"pending"}, "payer": {"Aoife"}, "page": {"3"}, "detail": {"5"}}
	opened := transactionReviewURL(query, 7)
	if !strings.Contains(opened, "match=7") || !strings.Contains(opened, "page=3") || strings.Contains(opened, "detail=") {
		t.Fatalf("drawer URL lost list context: %s", opened)
	}
	closed := transactionListURL(url.Values{"match": {"7"}, "match_tenant": {"9"}, "match_history_page": {"2"}, "error": {"confirmation_failed"}, "page": {"3"}})
	if closed != "/transactions?page=3" {
		t.Fatalf("close URL retained drawer state: %s", closed)
	}
	failure, err := transactionReviewFailureURL("/transactions?match_status=pending&page=3", 7, 9, "confirmation_failed")
	if err != nil || !strings.Contains(failure, "match=7") || !strings.Contains(failure, "match_tenant=9") || !strings.Contains(failure, "page=3") {
		t.Fatalf("failed confirmation did not return to drawer: %s, %v", failure, err)
	}
	if _, err := transactionReviewFailureURL("https://evil.example/transactions", 7, 9, "confirmation_failed"); err == nil {
		t.Fatal("external return was accepted")
	}
	workspaceFailure, err := transactionReviewFailureURL("/rent-dashboard?period=2026-09&view=tenants", 7, 9, "confirmation_failed")
	if err != nil || !strings.HasPrefix(workspaceFailure, "/rent-dashboard?") || !strings.Contains(workspaceFailure, "match=7") || !strings.Contains(workspaceFailure, "period=2026-09") {
		t.Fatalf("failed dashboard confirmation did not reopen its drawer: %s, %v", workspaceFailure, err)
	}
}

func TestTransactionReviewWorkspaceHistoryKeepsDrawerContext(t *testing.T) {
	review := transactionMatchReviewData{Source: transactionPageRow{PayerName: "Aoife"}, HistoryPage: 2, HistoryPages: 3}
	filters := defaultRentWorkspaceFilters(time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC))
	review.setWorkspaceURLs(filters, url.Values{"period": {"2026-09"}, "match": {"7"}, "match_tenant": {"9"}, "match_history_page": {"2"}})
	if !strings.Contains(review.PreviousHistoryURL, "match_history_page=1") || !strings.Contains(review.NextHistoryURL, "match_history_page=3") || !strings.Contains(review.NextHistoryURL, "match_tenant=9") || review.FormAction != "/rent-dashboard" {
		t.Fatalf("workspace history links lost drawer selection: %+v", review)
	}
}
