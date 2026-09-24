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
	setTransactionDetailLinks(query, rows)

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
	listHTML, err := executeTemplate(transactionListTemplate, transactionListPageData{TransactionRows: rows, Page: 1, PageSize: 50})
	if err != nil {
		t.Fatal(err)
	}
	// 稳定的锚点在手机卡片上：桌面表格 ≤640px 是藏起来的，卡片才是窄屏唯一那张列表。
	if !strings.Contains(listHTML, `href="/transactions?detail=42`) || !strings.Contains(listHTML, `id="mobile-transaction-demo-1"`) {
		t.Fatal("transaction list did not render its detail links and stable return anchors")
	}
	back := transactionListURL(url.Values{"detail": {"42"}, "payer": {"CHEN"}, "page": {"3"}})
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
		AllocatedAmount: "EUR 0.00",
		RemainingAmount: "EUR 640.00",
		AllocationCount: 0,
		CanReview:       true,
		MatchURL:        "/transactions?detail=42&match=42",
		SourceLabel:     "TrueLayer 银行同步",
		Reference:       `<script>alert("x")</script>`,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{
		"银行流水详情",
		"流水分配",
		"租客与入账分配",
		"处理记录",
		"原始流水",
		"原始描述",
		`class="code-block"`,
		"交易时间",
		"处理分配",
		"标记非租金",
		"&lt;script&gt;alert(&#34;x&#34;)&lt;/script&gt;",
	} {
		if !strings.Contains(page, marker) {
			t.Fatalf("transaction detail page missing escaped marker %q", marker)
		}
	}
	if strings.Contains(page, `<script>alert("x")</script>`) {
		t.Fatal("source reference rendered as executable markup")
	}

	progressIndex := strings.Index(page, "分配进度")
	allocationIndex := strings.Index(page, "租客与入账分配")
	sourceIndex := strings.Index(page, "原始流水")
	if progressIndex < 0 || allocationIndex < progressIndex || sourceIndex < allocationIndex {
		t.Fatalf("detail order: progress@%d, allocation@%d, source@%d", progressIndex, allocationIndex, sourceIndex)
	}

	// 摘掉的两块不能悄悄回来：匹配建议是系统的猜测，猜错了要先推翻才能动手；
	// 「返回此笔处理」和「返回流水」是同一份列表的两个入口。
	for _, removed := range []string{"匹配建议", "返回此笔处理", "前往流水列表查看可用操作", "#transaction-row-"} {
		if strings.Contains(page, removed) {
			t.Fatalf("transaction detail page still renders removed block %q", removed)
		}
	}
}

func TestTransactionDetailUsesReviewDrawerEntryAndSeparateNonRentAction(t *testing.T) {
	page, err := executeTemplate(transactionDetailPageTemplate, transactionDetailPageData{
		workspaceShell:  workspaceShell{ActivePage: "transactions"},
		ActionBase:      "/transactions",
		BackURL:         "/transactions?page=2",
		MatchURL:        "/transactions?detail=42&match=42",
		RevokeURL:       "/transactions/revoke?transaction_id=42",
		CanReview:       true,
		AllocationCount: 1,
		Transaction:     transactionPageRow{ID: "42", Direction: "income", MatchStatus: "matched", MatchStatusLabel: "已关联"},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{`>处理分配</a>`, `>撤销整笔匹配</a>`, `>非租金归类</summary>`, `>租客与入账分配</h3>`, `>标记非租金</summary>`} {
		if !strings.Contains(page, marker) {
			t.Fatalf("detail missing %q", marker)
		}
	}
	for _, old := range []string{`action="/transactions/rematch"`, `action="/transactions/confirm"`, `关联责任与对象`, `class="transaction-quick-match"`} {
		if strings.Contains(page, old) {
			t.Fatalf("detail still contains legacy flow %q", old)
		}
	}
	if got := strings.Count(page, `class="transaction-head-action"`); got != 2 {
		t.Fatalf("header actions = %d, want separate non-rent actions only", got)
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
		map[uint64]rentObligation{obligationID: {ID: obligationID, TenantID: tenantID, RentChargeID: chargeID, PeriodMonth: createdAt, DueDate: createdAt, ExpectedAmountCents: 128000, Currency: "EUR"}},
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

func TestTransactionObjectContextComesFromEffectiveRentOrSpecificSuggestion(t *testing.T) {
	firstObligationID, secondObligationID := uint64(10), uint64(11)
	obligations := map[uint64]rentObligation{
		firstObligationID:  {ID: firstObligationID, RentChargeID: 20},
		secondObligationID: {ID: secondObligationID, RentChargeID: 21},
	}
	charges := map[uint64]rentCharge{
		20: {PropertyNameSnapshot: nullableString("Rosewood"), RoomLabelSnapshot: nullableString("2B")},
		21: {PropertyNameSnapshot: nullableString("Oak House"), RoomLabelSnapshot: nullableString("3A")},
	}
	allocations := []paymentAllocation{
		{RentObligationID: &firstObligationID, AllocationKind: allocationKindRent, Status: allocationStatusConfirmed},
		{RentObligationID: &secondObligationID, AllocationKind: allocationKindRent, Status: allocationStatusVoided},
	}
	ids := transactionObjectChargeIDs(transactionPageRow{CandidateRentObligationID: secondObligationID}, allocations, obligations)
	if label, room := transactionRentObjectLabels(ids, charges); label != "Rosewood · 2B" || room != "2B" {
		t.Fatalf("confirmed allocation should take precedence over suggestion and voided allocation: %q, %q", label, room)
	}
	ids = transactionObjectChargeIDs(transactionPageRow{CandidateRentObligationID: secondObligationID}, nil, obligations)
	if label, room := transactionRentObjectLabels(ids, charges); label != "Oak House · 3A" || room != "3A" {
		t.Fatalf("specific suggestion did not resolve its room: %q, %q", label, room)
	}
	ids = transactionObjectChargeIDs(transactionPageRow{TenantID: 9}, nil, obligations)
	if label, _ := transactionRentObjectLabels(ids, charges); label != "" {
		t.Fatalf("tenant identity alone must not imply a room: %q", label)
	}
	allocations = []paymentAllocation{
		{RentObligationID: &firstObligationID, AllocationKind: allocationKindRent, Status: allocationStatusConfirmed},
		{RentObligationID: &secondObligationID, AllocationKind: allocationKindRent, Status: allocationStatusConfirmed},
	}
	ids = transactionObjectChargeIDs(transactionPageRow{}, allocations, obligations)
	if label, room := transactionRentObjectLabels(ids, charges); label != "Oak House · 3A、Rosewood · 2B" || room != "" {
		t.Fatalf("split payment should display both rooms without a single-room hint: %q, %q", label, room)
	}
}
