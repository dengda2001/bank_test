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
		AllocatedAmount: "EUR 0.00",
		RemainingAmount: "EUR 640.00",
		AllocationCount: 0,
		SourceLabel:     "TrueLayer 银行同步",
		Reference:       `<script>alert("x")</script>`,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{
		"银行流水详情",
		"关联责任与对象",
		"处理记录",
		"原始流水",
		"原始描述",
		`class="code-block"`,
		"交易时间",
		"编辑分配",
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

	// 原始流水是正文第一块：房东先看银行记了什么，再决定怎么匹配。位置是这次
	// 改造的要点，所以钉住它排在其他正文板块之前，而不是只钉"页面上有这块"。
	sourceIndex := strings.Index(page, "原始流水")
	allocationsIndex := strings.Index(page, "关联责任与对象")
	if sourceIndex < 0 || sourceIndex > allocationsIndex {
		t.Fatalf("原始流水 is not the first block of the main column (source@%d, allocations@%d)", sourceIndex, allocationsIndex)
	}

	// 摘掉的两块不能悄悄回来：匹配建议是系统的猜测，猜错了要先推翻才能动手；
	// 「返回此笔处理」和「返回流水」是同一份列表的两个入口。
	for _, removed := range []string{"匹配建议", "返回此笔处理", "前往流水列表查看可用操作", "#transaction-row-"} {
		if strings.Contains(page, removed) {
			t.Fatalf("transaction detail page still renders removed block %q", removed)
		}
	}
}

// The detail header's two entries must reuse the list-row actions: the same
// endpoints and parameters, plus the detail page's return target so acting from
// the detail page lands back on the list it came from. Both entries always
// render; which one carries a live form depends on the row state, exactly as it
// does in the list row (修改匹配 needs an existing rent match, 标记非租金 does not).
//
// 确认匹配不再是页头第三格：它和右栏的「匹配流水」是同一个 action 的重复入口，
// 页头那份已摘掉，所以下面确认相关的断言落在页面里唯一的那张匹配表单上。
func TestTransactionDetailHeaderActionsReuseListRowEndpoints(t *testing.T) {
	render := func(t *testing.T, row transactionPageRow) string {
		t.Helper()
		page, err := executeTemplate(transactionDetailPageTemplate, transactionDetailPageData{
			workspaceShell:  workspaceShell{ActivePage: "transactions"},
			ActionBase:      "/transactions",
			BackURL:         "/transactions?page=2",
			TransactionTime: "2026-09-12 09:18",
			Transaction:     row,
		})
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range []string{"编辑分配", "标记非租金"} {
			if !strings.Contains(page, entry) {
				t.Fatalf("transaction detail header is missing the %q entry", entry)
			}
		}
		if got := strings.Count(page, `class="transaction-head-action"`); got != 2 {
			t.Fatalf("transaction detail header renders %d entries, want 2", got)
		}
		return page
	}

	matched := render(t, transactionPageRow{
		ID:                   "42",
		Direction:            "income",
		MatchStatus:          "matched",
		MatchStatusLabel:     "已关联",
		PayerName:            "C. CHEN",
		Description:          "CREDIT TRANSFER\nC. CHEN",
		CanEditRentMatch:     true,
		RematchTenantOptions: []billingTenantOption{{ID: 9, Name: "Aoife"}},
		RematchMonthOptions:  []billingMonthOption{{Period: "2026-09", Label: "2026年9月", Remaining: "€0.00"}},
	})
	for _, marker := range []string{
		`action="/transactions/rematch"`,
		`aria-label="修改匹配租客" data-searchable`,
		`name="return_to" value="/transactions?page=2"`,
		"2026-09-12 09:18",
	} {
		if !strings.Contains(matched, marker) {
			t.Fatalf("matched header missing %q", marker)
		}
	}

	confirmable := render(t, transactionPageRow{
		ID:                        "42",
		Direction:                 "income",
		MatchStatus:               "candidate",
		MatchStatusLabel:          "待确认",
		CanConfirm:                true,
		CandidateTenantName:       "C. CHEN",
		CandidatePeriod:           "2026-09",
		CandidateRentObligationID: 77,
		RematchTenantOptions:      []billingTenantOption{{ID: 9, Name: "C. CHEN"}},
		// 生产路径里这两个是配对的：租客下拉由 ManualMatchOptions 推出来
		//（matching_service.go 的 rematchFilterOptions），所以 fixture 也照这个形状给。
		ManualMatchTenantOptions: []billingTenantOption{{ID: 9, Name: "C. CHEN"}},
		ManualMatchOptions:       []billingRentMatchOption{{TenantID: 9, TenantName: "C. CHEN", Period: "2026-09", PeriodLabel: "2026年9月", Remaining: "EUR 640.00"}},
	})
	for _, marker := range []string{
		`action="/transactions/confirm"`,
		`name="tenant_id" aria-label="选择匹配租客" data-searchable`,
		`name="period"`,
		`data-tenant="9"`,
		"C. CHEN",
		`action="/transactions/ignore"`,
		`name="return_to" value="/transactions?page=2"`,
	} {
		if !strings.Contains(confirmable, marker) {
			t.Fatalf("confirmable page missing %q", marker)
		}
	}
	if got := strings.Count(confirmable, `action="/transactions/confirm"`); got != 1 {
		t.Fatalf("transaction detail renders %d confirm forms, want the single one in the right rail", got)
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
