package main

import (
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestTransactionReturnTargetStaysOnKnownLocalLists(t *testing.T) {
	request := &http.Request{
		URL:  &url.URL{Path: "/transactions/confirm"},
		Form: url.Values{"return_to": {"/transactions?match_status=pending&period=2026-09&next=https%3A%2F%2Fevil.example"}},
	}
	if got, want := transactionReturnTarget(request), "/transactions?match_status=pending&period=2026-09"; got != want {
		t.Fatalf("validated return target = %q, want %q", got, want)
	}

	request.Form.Set("return_to", "https://evil.example/transactions")
	if got, want := transactionReturnTarget(request), "/transactions"; got != want {
		t.Fatalf("external return target fallback = %q, want %q", got, want)
	}
}

func TestTransactionDetailLinksPreserveListState(t *testing.T) {
	query := url.Values{
		"payer":        {"CHEN"},
		"match_status": {"needs_review"},
		"page":         {"3"},
		"sort":         {"amount_desc"},
	}
	rows := []transactionPageRow{{InternalID: "42"}, {InternalID: ""}}
	setTransactionDetailLinks("/transactions", query, rows)

	first, err := url.Parse(rows[0].DetailURL)
	if err != nil {
		t.Fatal(err)
	}
	if first.Path != "/transactions" || first.Query().Get("detail") != "42" || first.Query().Get("payer") != "CHEN" || first.Query().Get("page") != "3" || first.Query().Get("sort") != "amount_desc" {
		t.Fatalf("detail link did not preserve list state: %s", rows[0].DetailURL)
	}
	if rows[1].DetailKey != "demo-1" {
		t.Fatalf("fallback row key = %q, want demo-1", rows[1].DetailKey)
	}
	listHTML, err := executeTemplate(billingTemplate, billingPageData{TransactionRows: rows, Page: 1, PageSize: 50})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(listHTML, `href="/transactions?detail=42`) || !strings.Contains(listHTML, `id="transaction-row-demo-1"`) {
		t.Fatal("transaction list did not render its detail links and stable return anchors")
	}
	back := transactionListURL("/transactions", url.Values{"detail": {"42"}, "payer": {"CHEN"}, "page": {"3"}})
	if back != "/transactions?page=3&payer=CHEN" {
		t.Fatalf("return URL = %q", back)
	}
}

func TestTransactionDetailTemplateRendersPrototypeSectionsAndEscapesSourceData(t *testing.T) {
	page, err := executeTemplate(transactionDetailPageTemplate, transactionDetailPageData{
		workspaceShell:  workspaceShell{ActivePage: "transactions"},
		Title:           "EUR 640.00 · 收入",
		Subtitle:        "AIB Current Account · 12 Sep 2026 09:18",
		StatusClass:     "candidate",
		BackURL:         "/transactions?page=2",
		BackRowURL:      "/transactions?page=2#transaction-row-42",
		AllocatedAmount: "EUR 0.00",
		RemainingAmount: "EUR 640.00",
		AllocationCount: 0,
		SourceLabel:     "TrueLayer 银行同步",
		Reference:       `<script>alert("x")</script>`,
		Suggestion:      transactionDetailSuggestion{Status: "待确认", Reason: "请核对付款人与租金责任。"},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{
		"银行流水详情",
		"匹配建议",
		"关联账单与对象",
		"处理记录",
		"付款信息",
		`href="/transactions?page=2#transaction-row-42"`,
		"&lt;script&gt;alert(&#34;x&#34;)&lt;/script&gt;",
	} {
		if !strings.Contains(page, marker) {
			t.Fatalf("transaction detail page missing escaped marker %q", marker)
		}
	}
	if strings.Contains(page, `<script>alert("x")</script>`) {
		t.Fatal("source reference rendered as executable markup")
	}
}

func TestTransactionDetailAllocationRowsIncludeRoomContextAndVoidedHistory(t *testing.T) {
	tenantID := uint64(9)
	obligationID := uint64(10)
	chargeID := uint64(11)
	roomID := uint64(12)
	propertyID := uint64(13)
	createdAt := time.Date(2026, 9, 12, 9, 18, 0, 0, time.UTC)
	rows := transactionDetailAllocationRows(
		[]paymentAllocation{
			{ID: 1, TenantID: &tenantID, RentObligationID: &obligationID, AmountCents: 64000, AllocationKind: allocationKindRent, Status: allocationStatusConfirmed, CreatedAt: createdAt},
			{ID: 2, TenantID: &tenantID, AmountCents: 1000, AllocationKind: allocationKindOther, Status: allocationStatusVoided, CreatedAt: createdAt},
		},
		"EUR",
		map[uint64]rentObligation{obligationID: {ID: obligationID, TenantID: tenantID, RentChargeID: &chargeID, PeriodMonth: createdAt, DueDate: createdAt, ExpectedAmountCents: 128000, Currency: "EUR"}},
		map[uint64]rentCharge{chargeID: {ID: chargeID, RoomID: roomID, PropertyID: propertyID, PropertyNameSnapshot: nullableString("Rosewood Court"), RoomLabelSnapshot: nullableString("2B")}},
		map[uint64]room{roomID: {ID: roomID, PropertyID: propertyID, RoomLabel: "2B"}},
		map[uint64]property{propertyID: {ID: propertyID, Name: "Rosewood Court"}},
		map[uint64]string{tenantID: "陈先生"},
	)
	if len(rows) != 2 {
		t.Fatalf("got %d allocation rows, want confirmed and voided history", len(rows))
	}
	if rows[0].TenantName != "陈先生" || rows[0].PropertyName != "Rosewood Court" || rows[0].RoomLabel != "2B" || rows[0].Period != "2026年9月" || rows[0].DueDate != "2026-09-12" || !rows[0].Effective {
		t.Fatalf("allocation room context was not resolved: %+v", rows[0])
	}
	if rows[1].Effective || rows[1].StatusLabel != "已撤销" {
		t.Fatalf("voided allocation history was presented as effective: %+v", rows[1])
	}
}
