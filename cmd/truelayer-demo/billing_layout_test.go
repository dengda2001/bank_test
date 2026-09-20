package main

import (
	"net/url"
	"strings"
	"testing"
)

func renderBillingPage(t *testing.T, data billingPageData) string {
	t.Helper()
	if data.CanonicalPath == "" {
		data.CanonicalPath = "/billing"
	}
	var body strings.Builder
	if err := billingTemplate.Execute(&body, data); err != nil {
		t.Fatal(err)
	}
	return body.String()
}

func TestTransactionRouteUsesPrototypeQueueAndKeepsLocalReturnPath(t *testing.T) {
	page := renderBillingPage(t, billingPageData{
		workspaceShell: workspaceShell{ActivePage: "transactions", CompactTitle: "流水处理"},
		PageKey:        "transactions", CanonicalPath: "/transactions", TransactionScope: "pending", PendingCount: 2,
		TransactionRows: []transactionPageRow{{
			ID: "7", InternalID: "7", DetailKey: "7", DetailURL: "/transactions?detail=7&match_status=pending",
			Direction: "income", DirectionLabel: "收入", PayerName: "WAHAJULLAH KHAN", AmountDisplay: "€1,250.00",
			RemainingAmountDisplay: "€1,250.00", AllocationUseDisplay: "同住代付", DateDisplay: "01 Sep 2026",
			Description: "RENT SEPT", AccountName: "AIB", MatchStatus: "candidate", MatchStatusLabel: "待确认",
			CandidateTenantName: "WAHAJULLAH KHAN", CandidateRentObligationID: 301, CandidatePeriod: "2026-09", CanConfirm: true,
		}},
	})
	for _, marker := range []string{
		`class="content transaction-route-page"`,
		`<span class="transaction-mobile-copy">流水处理</span>`,
		`class="transaction-route-tabs"`,
		`href="/transactions?match_status=pending"`,
		`href="/transactions?match_status=matched"`,
		`href="/transactions?scope=all"`,
		`class="transaction-route-mobile-list"`,
		`class="transaction-review-card"`,
		`action="/transactions/confirm"`,
		`name="return_to" value="/transactions"`,
		`href="/transactions?detail=7&amp;match_status=pending"`,
	} {
		if !strings.Contains(page, marker) {
			t.Errorf("transaction route missing %q", marker)
		}
	}
}

// The filter bar carried thirteen controls, including a 排序 dropdown and a 每页
// dropdown that duplicate what the table headings and the pager now do. It keeps
// one control per question asked of the reader.
func TestBillingFilterBarKeepsOnlyTheSixQuestions(t *testing.T) {
	page := renderBillingPage(t, billingPageData{
		Page: 1, PageSize: 50, TotalTransactions: 3, TotalPages: 1,
		PeriodFilter: "2026-09", MatchStatusSelection: "pending",
	})
	filterBar := markupBetween(t, page, `<form class="filterbar"`, `</form>`)

	for _, expected := range []string{`id="payer"`, `id="tenant_id"`, `id="period"`, `id="rent_period"`, `id="allocation"`, `id="direction"`, `id="match_status"`} {
		if !strings.Contains(filterBar, expected) {
			t.Fatalf("filter bar lost %q: %s", expected, filterBar)
		}
	}
	for _, removed := range []string{`id="arrival_from"`, `id="arrival_to"`, `id="sort"`, `id="pending"`, `id="page_size"`} {
		if strings.Contains(filterBar, removed) {
			t.Fatalf("filter bar still renders the removed control %q: %s", removed, filterBar)
		}
	}

	// 待处理 is no longer its own checkbox: it is the first option of 匹配状态, and
	// a link that still carries pending=1 has to light it up.
	matchStatus := markupBetween(t, filterBar, `<select id="match_status"`, `</select>`)
	if !strings.Contains(matchStatus, `<option value="pending" selected>待处理`) {
		t.Fatalf("匹配状态 does not absorb the 待处理 filter: %s", matchStatus)
	}
}

func TestBillingCalendarPopoverStaysInsideTheDesktopViewport(t *testing.T) {
	page := renderBillingPage(t, billingPageData{Page: 1, PageSize: 50})
	if !strings.Contains(page, "@media (min-width: 641px) { .filterbar .calendar-popover { left: auto; right: 0; transform-origin: top right; } }") {
		t.Fatal("desktop transaction month picker must open toward the content area instead of extending past the viewport")
	}
}

// Until now 到账起/到账止 only existed as visible inputs; a saved URL that still
// carries them must keep filtering instead of silently showing everything.
func TestBillingLegacyArrivalRangeSurvivesAsHiddenInputs(t *testing.T) {
	page := renderBillingPage(t, billingPageData{
		Page: 1, PageSize: 50, TotalTransactions: 1, TotalPages: 1,
		ArrivalFromFilter: "2026-08-01", ArrivalToFilter: "2026-09-01",
	})
	filterBar := markupBetween(t, page, `<form class="filterbar"`, `</form>`)
	for _, expected := range []string{`name="arrival_from" value="2026-08-01"`, `name="arrival_to" value="2026-09-01"`} {
		if !strings.Contains(filterBar, expected) {
			t.Fatalf("filter bar dropped the in-force arrival range %q: %s", expected, filterBar)
		}
	}
}

func TestBillingTransactionRowsShowOnlyConfirmedRentMonthAndNoTechnicalIDs(t *testing.T) {
	page := renderBillingPage(t, billingPageData{
		Page: 1, PageSize: 50, TotalTransactions: 1, TotalPages: 1,
		TransactionRows: []transactionPageRow{{
			ID:                    "7",
			Direction:             "income",
			DirectionLabel:        "收入",
			PayerName:             "Aoife Murphy",
			PayerID:               "payer-123",
			InternalID:            "internal-456",
			ProviderTransactionID: "provider-789",
			AmountDisplay:         "EUR 950.00",
			DateDisplay:           "09 Sep 2026",
			ParsedPeriodDisplay:   "2026年8月",
			FinalPeriodDisplay:    "2026年9月",
			Description:           "September rent",
			AccountName:           "Rent account",
			AccountID:             "account-345",
			MatchStatus:           "matched",
			MatchStatusLabel:      "已关联",
		}},
	})

	for _, unwanted := range []string{
		"付款人 ID", "payer-123", "内部 ID", "internal-456", "银行流水号", "provider-789",
		"参考号", "REF:", "account-345", "解析租金月", "2026年8月",
	} {
		if strings.Contains(page, unwanted) {
			t.Fatalf("billing transaction row still renders %q: %s", unwanted, page)
		}
	}
	for _, expected := range []string{
		`<label for="period">流水到账月`, `>租金月：2026年9月<`, "Rent account",
	} {
		if !strings.Contains(page, expected) {
			t.Fatalf("billing transaction row is missing %q: %s", expected, page)
		}
	}
}

func TestBillingTemplateUsesExplicitOneClickMatchAndLimitedRematch(t *testing.T) {
	page := renderBillingPage(t, billingPageData{
		TransactionRows: []transactionPageRow{
			{
				ID:                        "7",
				Direction:                 "income",
				MatchStatus:               "unmatched",
				MatchStatusLabel:          "未关联",
				CandidateTenantID:         12,
				CandidateTenantName:       "Aoife Murphy",
				CandidateRentObligationID: 20,
				CandidatePeriod:           "2026-09",
				CanConfirm:                true,
			},
			{
				ID:                   "8",
				Direction:            "income",
				MatchStatus:          "matched",
				MatchStatusLabel:     "已关联",
				CanRematch:           true,
				CanEditRentMatch:     true,
				RematchTenantOptions: []billingTenantOption{{ID: 22, Name: "Bríd Murphy"}},
				RematchMonthOptions:  []billingMonthOption{{Period: "2026-10", Label: "2026年10月", Remaining: "EUR 950.00"}},
			},
		},
	})

	for _, expected := range []string{"一键匹配", `action="/billing/confirm"`, `name="rent_obligation_id" value="20"`, "租金月 2026-09", "修改匹配", `action="/billing/rematch"`, `name="tenant_id"`, `aria-label="修改匹配租客"`, `name="period"`, `aria-label="修改租金月份"`, "Bríd Murphy", "2026年10月"} {
		if !strings.Contains(page, expected) {
			t.Fatalf("billing page missing explicit match control %q: %s", expected, page)
		}
	}
	rematchStart := strings.Index(page, `<form class="rematch-form"`)
	if rematchStart < 0 {
		t.Fatal("billing page missing rematch form")
	}
	rematchEnd := strings.Index(page[rematchStart:], `</form>`)
	if rematchEnd < 0 {
		t.Fatal("billing page rematch form is not closed")
	}
	rematchMarkup := page[rematchStart : rematchStart+rematchEnd]
	if strings.Contains(rematchMarkup, `name="rent_obligation_id"`) {
		t.Fatalf("rematch form should not use a combined rent-obligation selector: %s", rematchMarkup)
	}
}

// 一笔已关联的流水本来在行里常驻两个 100% 宽的下拉框，状态早就定了，控件却占满整列。
// 它们现在收在 <details> 里：默认只渲染一颗「修改匹配」按钮，点开才出现选择控件。
func TestBillingRematchSelectsStayBehindTheRematchButton(t *testing.T) {
	page := renderBillingPage(t, billingPageData{TransactionRows: []transactionPageRow{{
		ID:                   "8",
		Direction:            "income",
		MatchStatus:          "matched",
		MatchStatusLabel:     "已关联",
		CanRematch:           true,
		CanEditRentMatch:     true,
		RematchTenantOptions: []billingTenantOption{{ID: 22, Name: "Bríd Murphy"}},
		RematchMonthOptions:  []billingMonthOption{{Period: "2026-10", Label: "2026年10月", Remaining: "EUR 950.00"}},
	}}})

	// 不带 open：默认收起，两个下拉才不会出现在每一行已关联的流水里。带上 open 属性
	// 这条断言就会读到 "open" 并失败，这正是要防的回归。
	if tag := markupBetween(t, page, `<details class="rematch-details"`, ">"); strings.Contains(tag, "open") {
		t.Fatalf("rematch disclosure starts expanded: %s", tag)
	}

	disclosure := markupBetween(t, page, `<details class="rematch-details"`, "</details>")
	if !strings.Contains(disclosure, `<summary class="btn">修改匹配</summary>`) {
		t.Fatalf("collapsed rematch row does not offer the 修改匹配 button: %s", disclosure)
	}
	for _, expected := range []string{`action="/billing/rematch"`, `name="tenant_id"`, `aria-label="修改匹配租客"`, `name="period"`, `aria-label="修改租金月份"`, "确认修改"} {
		if !strings.Contains(disclosure, expected) {
			t.Fatalf("rematch disclosure lost %q: %s", expected, disclosure)
		}
	}
}

func TestRematchFilterOptionsStayIndependentAndDeduplicated(t *testing.T) {
	tenantOptions, monthOptions := rematchFilterOptions([]billingRentMatchOption{
		{TenantID: 1, TenantName: "租客甲", Period: "2026-09", PeriodLabel: "2026年9月"},
		{TenantID: 1, TenantName: "租客甲", Period: "2026-10", PeriodLabel: "2026年10月"},
		{TenantID: 2, TenantName: "租客乙", Period: "2026-09", PeriodLabel: "2026年9月"},
	})
	if len(tenantOptions) != 2 || len(monthOptions) != 2 {
		t.Fatalf("independent options = tenants %d, months %d; want 2 each", len(tenantOptions), len(monthOptions))
	}
	if tenantOptions[0].ID != 1 || tenantOptions[1].ID != 2 || monthOptions[0].Period != "2026-09" || monthOptions[1].Period != "2026-10" {
		t.Fatalf("unexpected independent options: tenants=%+v months=%+v", tenantOptions, monthOptions)
	}
}

func TestBillingTemplateKeepsSplitMatchOnTheRevokeFlow(t *testing.T) {
	page := renderBillingPage(t, billingPageData{TransactionRows: []transactionPageRow{{
		ID:               "9",
		Direction:        "income",
		MatchStatus:      "matched",
		MatchStatusLabel: "已关联",
	}}})

	for _, expected := range []string{"该流水已拆分或含其他用途；请撤销后重新归类。", `action="/billing/revoke"`} {
		if !strings.Contains(page, expected) {
			t.Fatalf("billing page missing split-match revoke guidance %q: %s", expected, page)
		}
	}
	if strings.Contains(page, `action="/billing/rematch"`) {
		t.Fatalf("split match unexpectedly renders direct rematch: %s", page)
	}
}

// Sorting by heading has to survive filtering and paging: the current column
// rides along in the filter form and in the pager, or 搜索 would reorder the
// list behind the reader's back.
func TestBillingListCarriesTheColumnSort(t *testing.T) {
	page := renderBillingPage(t, billingPageData{
		Page: 2, PageSize: 25, TotalTransactions: 60, TotalPages: 3,
		PayerFilter: "Aoife", SortFilter: "amount_asc",
		PreviousPageURL: "/billing?page=1", NextPageURL: "/billing?page=3",
		ArrivalSort:     tableSortLink{URL: "/billing?sort=arrival_desc", Arrow: "▼"},
		PayerSort:       tableSortLink{URL: "/billing?sort=payer_asc", Arrow: "▲"},
		AmountSort:      tableSortLink{URL: "/billing?sort=amount_desc", Arrow: "▼", Active: true},
		TransactionRows: []transactionPageRow{{ID: "7", Direction: "income", DirectionLabel: "收入"}},
	})
	for _, heading := range []string{
		`<a class="sort-link" href="/billing?sort=payer_asc">付款人<span class="sort-arrow">▲</span></a>`,
		`<a class="sort-link active" href="/billing?sort=amount_desc">金额／余额<span class="sort-arrow">▼</span></a>`,
		`<a class="sort-link" href="/billing?sort=arrival_desc">到账／租金月<span class="sort-arrow">▼</span></a>`,
	} {
		if !strings.Contains(page, heading) {
			t.Fatalf("heading %q is not rendered as a sort link: %s", heading, page)
		}
	}

	filterBar := markupBetween(t, page, `<form class="filterbar"`, `</form>`)
	if !strings.Contains(filterBar, `<input type="hidden" name="sort" value="amount_asc">`) {
		t.Fatalf("filtering would reset the sort: %s", filterBar)
	}

	pager := markupBetween(t, page, `<div class="pagination">`, "</div>\n        </div>")
	for _, expected := range []string{`id="page_size"`, `name="payer" value="Aoife"`, `name="sort" value="amount_asc"`, "第 2 / 3 页，共 60 笔"} {
		if !strings.Contains(pager, expected) {
			t.Fatalf("pager is missing %q: %s", expected, pager)
		}
	}
}

// 待处理 used to be a checkbox of its own. It is now one option of 匹配状态, so the
// query parameter it used to send and the option it now sends must parse alike.
func TestPendingFilterAndPendingMatchStatusParseAlike(t *testing.T) {
	legacy := filtersFromQuery(url.Values{"pending": {"1"}, "period": {"2026-09"}})
	option := filtersFromQuery(url.Values{"match_status": {"pending"}, "period": {"2026-09"}})
	for name, filters := range map[string]transactionFilters{"legacy pending=1": legacy, "match_status=pending": option} {
		if !filters.PendingOnly {
			t.Fatalf("%s did not set the pending filter: %+v", name, filters)
		}
		if filters.MatchStatus != "" {
			t.Fatalf("%s also narrowed to the literal status %q, which no row has", name, filters.MatchStatus)
		}
		if got := matchStatusSelection(filters); got != "pending" {
			t.Fatalf("%s shows %q in the dropdown, want pending", name, got)
		}
	}

	// A real status still has to work, and must not be mistaken for pending.
	matched := filtersFromQuery(url.Values{"match_status": {"matched"}})
	if matched.PendingOnly || matched.MatchStatus != "matched" || matchStatusSelection(matched) != "matched" {
		t.Fatalf("a plain status filter was rewritten: %+v", matched)
	}
}

// The 排序 dropdown offered 付款人 A-Z with no way back; the heading has to flip.
func TestBillingSortFlipsBothWays(t *testing.T) {
	base := url.Values{"payer": {"Aoife"}, "page": {"3"}, "page_size": {"25"}}

	first := billingSortURL(base, "payer_desc")
	parsed, err := url.Parse(first)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Query().Get("sort") != "payer_desc" || parsed.Query().Get("payer") != "Aoife" || parsed.Query().Get("page_size") != "25" {
		t.Fatalf("sorting dropped the current filters: %s", first)
	}
	if parsed.Query().Has("page") {
		t.Fatalf("sorting kept the old page, which no longer holds the same rows: %s", first)
	}

	back := billingSortURL(base, "payer_asc")
	if !strings.Contains(back, "sort=payer_asc") {
		t.Fatalf("flipping the heading did not change the sort: %s", back)
	}
}
