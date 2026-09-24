package main

import (
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"
)

func renderTransactionListPage(t *testing.T, data transactionListPageData) string {
	t.Helper()
	if data.CanonicalPath == "" {
		// 渲染流水列表的只有 /transactions。/billing 还在，但它只做 302 转发，
		// 不会走到这个模板，所以它不该是任何断言里的 CanonicalPath。
		data.CanonicalPath = "/transactions"
	}
	var body strings.Builder
	if err := transactionListTemplate.Execute(&body, data); err != nil {
		t.Fatal(err)
	}
	return body.String()
}

func TestTransactionRouteUsesPrototypeQueueAndKeepsLocalReturnPath(t *testing.T) {
	page := renderTransactionListPage(t, transactionListPageData{
		workspaceShell: workspaceShell{ActivePage: "transactions", CompactTitle: "流水处理"},
		CanonicalPath:  "/transactions", TransactionScope: "pending", PendingCount: 2,
		TransactionRows: []transactionPageRow{{
			ID: "7", InternalID: "7", DetailKey: "7", DetailURL: "/transactions?detail=7&match_status=pending", MatchURL: "/transactions?match=7&match_status=pending",
			Direction: "income", DirectionLabel: "收入", PayerName: "WAHAJULLAH KHAN", AmountDisplay: "€1,250.00",
			RemainingAmountDisplay: "€1,250.00", AllocationUseDisplay: "同住代付", DateDisplay: "01 Sep 2026",
			Description: "RENT SEPT", AccountName: "AIB", MatchStatus: "candidate", MatchStatusLabel: "待确认",
			CandidateTenantName: "WAHAJULLAH KHAN", CandidateRentObligationID: 301, CandidatePeriod: "2026-09", CanConfirm: true, ReturnURL: "/transactions",
			ManualMatchTenantOptions: []billingTenantOption{{ID: 7, Name: "WAHAJULLAH KHAN"}},
			ManualMatchOptions:       []billingRentMatchOption{{TenantID: 7, TenantName: "WAHAJULLAH KHAN", Period: "2026-09", PeriodLabel: "2026年9月", Remaining: "€1,250.00"}},
		}},
	})
	for _, marker := range []string{
		`class="content transaction-route-page"`,
		`<span class="transaction-mobile-copy">流水处理</span>`,
		`<select id="match_status" name="match_status"`,
		`class="transaction-route-mobile-list"`,
		`<th>用途</th>`,
		`class="transaction-review-card"`,
		`href="/transactions?match=7&amp;match_status=pending"`,
		`href="/transactions?detail=7&amp;match_status=pending"`,
	} {
		if !strings.Contains(page, marker) {
			t.Errorf("transaction route missing %q", marker)
		}
	}
	quickFilters := markupBetween(t, page, `<form class="transaction-route-quickfilter"`, `</form>`)
	for _, kept := range []string{`id="direction"`, `id="match_status"`, `name="payer"`, `name="period"`} {
		if !strings.Contains(quickFilters, kept) {
			t.Errorf("compact filter form lost %q: %s", kept, quickFilters)
		}
	}
	advancedFilters := markupBetween(t, page, `<form class="filterbar"`, `</form>`)
	if strings.Contains(advancedFilters, `<select id="match_status"`) {
		t.Errorf("advanced filter still owns the visible status selector: %s", advancedFilters)
	}
	actionStart := strings.Index(page, `<td class="route-txn-action">`)
	if actionStart < 0 {
		t.Fatal("transaction row has no action cell")
	}
	actionEnd := strings.Index(page[actionStart:], `</td>`)
	if actionEnd < 0 {
		t.Fatal("transaction row action cell is not closed")
	}
	actionCell := page[actionStart : actionStart+actionEnd]
	if !strings.Contains(actionCell, `href="/transactions?detail=7&amp;match_status=pending"`) || !strings.Contains(actionCell, `>处理分配</a>`) {
		t.Errorf("directly matchable transaction row must keep both detail and match actions: %s", actionCell)
	}
}

// 老表格从流水列表里搬走了。它以前是在 /transactions 上被 CSS 藏起来的，所以
// 页面源码里一直长得像"这张表还在"——每页白送 8.5KB。这条断言把"确实没了"钉住，
// 也挡住哪天有人照着旧标记再抄一份回来。
func TestTransactionListNoLongerRendersTheLegacyTable(t *testing.T) {
	page := renderTransactionListPage(t, transactionListPageData{
		workspaceShell: workspaceShell{ActivePage: "transactions"},
		TransactionRows: []transactionPageRow{{
			ID: "7", DetailURL: "/transactions?detail=7", ReturnURL: "/transactions",
			Direction: "income", DirectionLabel: "收入", PayerName: "Aoife Murphy",
			AmountDisplay: "EUR 950.00", MatchStatus: "unmatched", MatchStatusLabel: "未关联",
			ManualMatchTenantOptions: []billingTenantOption{{ID: 7, Name: "Aoife Murphy"}},
			ManualMatchOptions:       []billingRentMatchOption{{TenantID: 7, TenantName: "Aoife Murphy", Period: "2026-09", PeriodLabel: "2026年9月", Remaining: "EUR 950.00"}},
		}},
	})
	for _, gone := range []string{
		`class="transaction-table"`, "billing-table-wrap", "txn-col-action", "txn-amount-mobile",
		"一键匹配", "rematch-details", "allocation-details", "month-choice-form", `action="/billing`,
	} {
		if strings.Contains(page, gone) {
			t.Fatalf("transaction list still renders the legacy table markup %q", gone)
		}
	}
}

func TestTransactionMatchDrawerKeepsPaidMonthEvidenceAndFullTenantList(t *testing.T) {
	data := transactionMatchReviewData{
		Source:           transactionPageRow{ID: "7", PayerName: "Aoife", Description: "August rent", AmountDisplay: "EUR 950.00", RemainingAmountDisplay: "EUR 950.00", ParsedPeriodDisplay: "2026-08"},
		SelectedTenantID: 7, SelectedTenantName: "Aoife", IdentifiedTenant: true, CanMatch: true,
		SourceAmountCents: 95000, SourceRemainingCents: 95000, RequestKey: "review-test",
		ReturnURL: "/transactions?match_status=pending", CloseURL: "/transactions?match_status=pending",
		TenantOptions: []transactionReviewTenant{{ID: 7, Name: "Aoife", Selected: true}, {ID: 8, Name: "Bríd"}},
		Months:        []transactionReviewMonth{{Period: "2026-08", Label: "2026年8月", Highlighted: true, Note: "本月已交清", Evidence: []transactionReviewEvidence{{Date: "2026-08-01", PayerName: "Aoife", Amount: "EUR 950.00", Description: "Original August rent", DetailURL: "/transactions?detail=42"}}}, {Period: "2026-09", Label: "2026年9月", Selectable: true, Remaining: "EUR 950.00", RemainingCents: 95000}},
	}
	page := renderTransactionListPage(t, transactionListPageData{MatchReview: &data})
	for _, marker := range []string{`role="dialog"`, `>Aoife</option>`, `>Bríd</option>`, `Original August rent`, `href="/transactions?detail=42"`, `data-period="2026-09"`, `data-tenant-id="7"`, `data-remaining-cents="95000"`, `action="/transactions/confirm-batch"`, `name="return_to" value="/transactions?match_status=pending"`} {
		if !strings.Contains(page, marker) {
			t.Errorf("drawer missing %q", marker)
		}
	}
	if strings.Contains(page, `data-period="2026-08"`) {
		t.Error("paid month is selectable")
	}
}

// 日历数据袋里的「应交」有两处来源：可匹配责任（billingRentMatchOption）和
// 已识别租客的月份列表（billingMonthOption）。两条都要带上金额，否则选完月份
// 那一行只能报未收，部分缴过的月份看起来就和一分没缴的一样。
func TestTenantPeriodCalendarCarriesTheRentDueFromBothSources(t *testing.T) {
	fromRentOptions := tenantPeriodMatchCalendar([]billingRentMatchOption{
		{TenantID: 7, Period: "2026-09", PeriodLabel: "2026年9月", Expected: "€950.00", Remaining: "€400.00"},
	}, 7, "")
	if got := fromRentOptions.Options; len(got) != 1 || got[0].Expected != "€950.00" || got[0].Remaining != "€400.00" {
		t.Fatalf("rent-match calendar options dropped the amounts: %+v", got)
	}
}

// availableRentOptions 是首页「处理流水」和流水列表匹配表单的共同数据源，
// 它得把责任的全额带出来，「应交」才有值可显示。
func TestAvailableRentOptionsCarryTheFullRentDue(t *testing.T) {
	obligations := []rentObligation{{
		ID: 11, TenantID: 7, PeriodMonth: time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
		ExpectedAmountCents: 95000, PaidAmountCents: 40000, Currency: "EUR",
	}}
	options := availableRentManualMatchOptions(paymentTransaction{Currency: "EUR"}, obligations, map[uint64]string{7: "Aoife Murphy"}, 55000)
	if len(options) != 1 {
		t.Fatalf("got %d options, want the one outstanding obligation", len(options))
	}
	if options[0].Expected != "EUR 950.00" || options[0].Remaining != "EUR 550.00" {
		t.Fatalf("option does not separate 应交 from 未收: %+v", options[0])
	}
}

// 提示行由 JS 现算，Go 侧只能断言"数据送到了"和"JS 会读"。这条盯住后半截：
// data-due 送到 <template> 之后得有人读它，否则页面照旧只报未收，而且不会有
// 任何测试或编译错误提醒——这正是上一版漏掉应交时的样子。
func TestTenantPeriodHelperReadsBothAmountsAndMarksThemSet(t *testing.T) {
	script := embeddedWebText("web/static/js/workspace-controls.js")
	for _, expected := range []string{"option.dataset.due", "option.dataset.rent", "helper.classList.add('is-set')"} {
		if !strings.Contains(script, expected) {
			t.Fatalf("tenant/month helper no longer wires up %q; the 应交/未收 line would silently fall back to 未收 only", expected)
		}
	}
}

func TestTransactionStatusSelectorIsOutsideAdvancedFiltersWithFourOptions(t *testing.T) {
	page := renderTransactionListPage(t, transactionListPageData{
		workspaceShell:       workspaceShell{ActivePage: "transactions"},
		CanonicalPath:        "/transactions",
		MatchStatusSelection: "ignored",
	})
	if got := strings.Count(page, `id="match_status"`); got != 1 {
		t.Fatalf("transactions page renders %d status selectors, want 1: %s", got, page)
	}
	quickFilters := markupBetween(t, page, `<form class="transaction-route-quickfilter"`, `</form>`)
	advancedFilters := markupBetween(t, page, `<form class="filterbar"`, `</form>`)
	if strings.Contains(advancedFilters, `<select id="match_status"`) || !strings.Contains(advancedFilters, `type="hidden" name="match_status" value="ignored"`) {
		t.Errorf("advanced filters do not preserve the visible status choice: %s", advancedFilters)
	}
	status := markupBetween(t, page, `<select id="match_status"`, `</select>`)
	for _, option := range []string{
		`value=""`, `value="pending"`, `value="matched"`, `value="ignored"`,
	} {
		if !strings.Contains(status, option) {
			t.Errorf("status selector missing %s: %s", option, status)
		}
	}
	if strings.Count(status, `<option`) != 4 || !strings.Contains(status, `<option value="ignored" selected>已忽略</option>`) {
		t.Errorf("status selector choices are not the four requested values: %s", status)
	}
	if !strings.Contains(quickFilters, status) {
		t.Error("status selector is not outside the advanced filter panel")
	}

	// 已关联 is the app-wide word for matched, borrowed from the status labels the
	// row markup already renders. The 匹配/关联 split must not come back.
	for _, stale := range []string{"已匹配", "未匹配", "部分匹配"} {
		if strings.Contains(page, stale) {
			t.Errorf("transactions page still renders the retired word %q", stale)
		}
	}
}

func TestTransactionDirectionSelectorAndRowsDistinguishIncomeFromExpense(t *testing.T) {
	page := renderTransactionListPage(t, transactionListPageData{
		DirectionFilter: "expense",
		TransactionRows: []transactionPageRow{
			{ID: "7", DetailKey: "7", Direction: "income", DirectionLabel: "收入", AmountDisplay: "EUR 950.00"},
			{ID: "8", DetailKey: "8", Direction: "expense", DirectionLabel: "支出", AmountDisplay: "EUR 77.99"},
		},
	})
	quick := markupBetween(t, page, `<form class="transaction-route-quickfilter"`, `</form>`)
	advanced := markupBetween(t, page, `<form class="filterbar"`, `</form>`)
	if strings.Count(page, `id="direction"`) != 1 || !strings.Contains(quick, `<option value="expense" selected>支出</option>`) {
		t.Fatal("direction selector must be visible, unique, and show the active filter")
	}
	if !strings.Contains(advanced, `type="hidden" name="direction" value="expense"`) {
		t.Fatal("advanced filters lost the selected direction")
	}
	for _, marker := range []string{
		`id="transaction-row-7" data-direction="income"`,
		`id="transaction-row-8" data-direction="expense"`,
		`<span class="transaction-direction income">收入</span>EUR 950.00`,
		`<span class="transaction-direction expense">支出</span>EUR 77.99`,
		`id="mobile-transaction-7" data-direction="income"`,
		`id="mobile-transaction-8" data-direction="expense"`,
		`.transaction-route-desktop-table tbody tr[data-direction="income"] { background:`,
		`.transaction-route-desktop-table tbody tr[data-direction="expense"] { background:`,
		`.transaction-review-card[data-direction="income"] { background:`,
		`.transaction-review-card[data-direction="expense"] { background:`,
	} {
		if !strings.Contains(page, marker) {
			t.Errorf("transaction direction missing %q", marker)
		}
	}
}

func TestTransactionListStatusDefaultsToAllAndGroupsOldLinks(t *testing.T) {
	for _, tc := range []struct {
		query       url.Values
		wantScope   string
		wantStatus  string
		wantPending bool
	}{
		{query: url.Values{}, wantScope: "all"},
		{query: url.Values{"match_status": {"pending"}}, wantScope: "pending", wantPending: true},
		{query: url.Values{"match_status": {"matched"}}, wantScope: "matched", wantStatus: "matched"},
		{query: url.Values{"match_status": {"ignored"}}, wantScope: "ignored", wantStatus: "ignored"},
		{query: url.Values{"match_status": {"candidate"}}, wantScope: "pending", wantPending: true},
		{query: url.Values{"scope": {"all"}, "match_status": {"pending"}}, wantScope: "all"},
	} {
		filters, scope := transactionListFiltersFromQuery(tc.query)
		if scope != tc.wantScope || filters.MatchStatus != tc.wantStatus || filters.PendingOnly != tc.wantPending {
			t.Errorf("query %v: scope=%q status=%q pending=%t", tc.query, scope, filters.MatchStatus, filters.PendingOnly)
		}
	}
	page := renderTransactionListPage(t, transactionListPageData{})
	status := markupBetween(t, page, `<select id="match_status"`, `</select>`)
	if !strings.Contains(status, `<option value="" selected>全部</option>`) {
		t.Errorf("empty query does not show 全部 by default: %s", status)
	}
}

func TestTransactionFilterBarKeepsDetailedQuestions(t *testing.T) {
	page := renderTransactionListPage(t, transactionListPageData{
		Page: 1, PageSize: 50, TotalTransactions: 3, TotalPages: 1,
		PeriodFilter: "2026-09", MatchStatusSelection: "pending", DirectionFilter: "expense",
	})
	filterBar := markupBetween(t, page, `<form class="filterbar"`, `</form>`)

	for _, expected := range []string{`id="payer"`, `id="tenant_id"`, `id="tenant_id" name="tenant_id" data-searchable`, `id="period"`, `id="rent_period"`, `id="allocation"`, `type="hidden" name="direction" value="expense"`, `type="hidden" name="match_status" value="pending"`} {
		if !strings.Contains(filterBar, expected) {
			t.Fatalf("filter bar lost %q: %s", expected, filterBar)
		}
	}
	for _, removed := range []string{`id="arrival_from"`, `id="arrival_to"`, `id="sort"`, `id="pending"`, `id="page_size"`} {
		if strings.Contains(filterBar, removed) {
			t.Fatalf("filter bar still renders the removed control %q: %s", removed, filterBar)
		}
	}

	matchStatus := markupBetween(t, page, `<select id="match_status"`, `</select>`)
	if !strings.Contains(matchStatus, `<option value="pending" selected>待处理`) {
		t.Fatalf("关联状态 does not absorb the 待处理 filter: %s", matchStatus)
	}
}

func TestTransactionCalendarPopoverStaysInsideTheDesktopViewport(t *testing.T) {
	page := renderTransactionListPage(t, transactionListPageData{Page: 1, PageSize: 50})
	if !strings.Contains(page, "@media (min-width: 641px) { .filterbar .calendar-popover { left: auto; right: 0; transform-origin: top right; } }") {
		t.Fatal("desktop transaction month picker must open toward the content area instead of extending past the viewport")
	}
}

// Until now 到账起/到账止 only existed as visible inputs; a saved URL that still
// carries them must keep filtering instead of silently showing everything.
func TestTransactionListKeepsTheArrivalRangeAsHiddenInputs(t *testing.T) {
	page := renderTransactionListPage(t, transactionListPageData{
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

// 行里只许出现人读得懂的东西：付款人姓名、金额、从银行附言里识别出来的租金月份。
// 付款人编号、内部 ID、银行流水号、参考号一个都不许漏到页面上。
func TestTransactionRowsShowParsedRentMonthAndNoTechnicalIDs(t *testing.T) {
	page := renderTransactionListPage(t, transactionListPageData{
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
			Description:           "September rent",
			AccountName:           "Rent account",
			AccountID:             "account-345",
			MatchStatus:           "matched",
			MatchStatusLabel:      "已关联",
		}},
	})

	for _, unwanted := range []string{
		"付款人 ID", "payer-123", "内部 ID", "internal-456", "银行流水号", "provider-789",
		"参考号", "REF:", "account-345",
	} {
		if strings.Contains(page, unwanted) {
			t.Fatalf("transaction row still renders %q: %s", unwanted, page)
		}
	}
	for _, expected := range []string{
		`>识别租金月份</a></th>`, `>2026年8月</td>`, "Rent account",
	} {
		if !strings.Contains(page, expected) {
			t.Fatalf("transaction row is missing %q: %s", expected, page)
		}
	}
}

func TestTransactionListShowsDescriptionAndOnlyKnownPropertyRoom(t *testing.T) {
	page := renderTransactionListPage(t, transactionListPageData{
		TransactionRows: []transactionPageRow{
			{Description: "RENT <SEPT>", ObjectLabel: "Rosewood Court · 2B", AccountName: "AIB"},
			{Description: "FASTER PAYMENT", AccountName: "AIB"},
		},
	})
	for _, expected := range []string{
		`<th>Description</th>`,
		`<td class="route-txn-description">RENT &lt;SEPT&gt;</td>`,
		`<td class="route-txn-context">Rosewood Court · 2B</td>`,
		`<td class="route-txn-context">—</td>`,
		`class="transaction-review-description">RENT &lt;SEPT&gt;</p>`,
		`账户：AIB`,
	} {
		if !strings.Contains(page, expected) {
			t.Fatalf("transaction list missing %q", expected)
		}
	}
	if strings.Contains(page, `<td class="route-txn-context">AIB</td>`) {
		t.Fatal("bank account was presented as a property/room")
	}
	if strings.Contains(page, `.transaction-review-head .transaction-review-description { display: none; }`) {
		t.Fatal("description is hidden on mobile cards")
	}
}

func TestTransactionListUsesSharedReviewForNewAndMatchedIncome(t *testing.T) {
	page := renderTransactionListPage(t, transactionListPageData{
		TransactionRows: []transactionPageRow{
			{ID: "7", MatchURL: "/transactions?match=7", Direction: "income", MatchStatus: "unmatched", MatchStatusLabel: "未关联"},
			{ID: "8", MatchURL: "/transactions?match=8", Direction: "income", MatchStatus: "matched", MatchStatusLabel: "已关联", ReturnURL: "/transactions?scope=all"},
		},
	})
	for _, expected := range []string{`href="/transactions?match=7"`, `href="/transactions?match=8"`, ">处理分配</a>", ">撤销整笔匹配</a>"} {
		if !strings.Contains(page, expected) {
			t.Fatalf("transaction list missing %q", expected)
		}
	}
	if strings.Contains(page, `action="/transactions/rematch"`) || strings.Contains(page, `action="/transactions/confirm"`) {
		t.Fatal("list still renders a competing rent-match form")
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

func TestTransactionListKeepsFullRevokeSeparateFromShareCorrection(t *testing.T) {
	page := renderTransactionListPage(t, transactionListPageData{TransactionRows: []transactionPageRow{{
		ID: "9", MatchURL: "/transactions?match=9", Direction: "income", MatchStatus: "matched", MatchStatusLabel: "已关联",
	}}})
	if !strings.Contains(page, ">调整分配</a>") || !strings.Contains(page, ">撤销整笔匹配</a>") {
		t.Fatal("matched source needs both exact-share review and full-source revoke")
	}
	if strings.Contains(page, `action="/transactions/rematch"`) {
		t.Fatal("legacy rematch form remains")
	}
}

// Sorting by heading has to survive filtering and paging: the current column
// rides along in the filter form and in the pager, or 搜索 would reorder the
// list behind the reader's back.
func TestTransactionListKeepsTheSortAcrossFilteringAndPaging(t *testing.T) {
	page := renderTransactionListPage(t, transactionListPageData{
		Page: 2, PageSize: 25, TotalTransactions: 60, TotalPages: 3,
		PayerFilter: "Aoife", SortFilter: "amount_asc",
		PreviousPageURL: "/transactions?page=1", NextPageURL: "/transactions?page=3",
		TransactionRows: []transactionPageRow{{ID: "7", Direction: "income", DirectionLabel: "收入"}},
	})

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

// 待处理 used to be a checkbox of its own. It is now one option of 关联状态, so the
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

// /transactions 的表格加了「识别租金月份」列。这里钉三件事：已识别的行直接显示月份
// （2026-09，和详情页同一款值）；没解析出来的行显示"未识别"而不是空格；窄屏卡片跟着
// 一起显示。
//
// 为什么要单独钉"未识别"：空格和"这笔压根不用管月份"在屏幕上长得一模一样。留空的话
// 房东分不清是没识别出来、还是不需要识别，这一列就白加了。
func TestTransactionRouteShowsTheParsedRentMonth(t *testing.T) {
	page := renderTransactionListPage(t, transactionListPageData{
		CanonicalPath: "/transactions", TransactionScope: "pending",
		TransactionRows: []transactionPageRow{
			{
				ID: "7", DetailKey: "7", Direction: "income", PayerName: "WAHAJULLAH KHAN",
				AmountDisplay: "€1,250.00", Description: "RENT SEPT",
				MatchStatus: "candidate", MatchStatusLabel: "待确认", ParsedPeriodDisplay: "2026-09",
			},
			{
				ID: "8", DetailKey: "8", Direction: "income", PayerName: "NO REFERENCE LTD",
				AmountDisplay: "€40.00", Description: "TRANSFER",
				MatchStatus: "unmatched", MatchStatusLabel: "未匹配",
			},
		},
	})

	if !strings.Contains(page, ">识别租金月份</a></th>") {
		t.Fatal("流水表没有「识别租金月份」表头")
	}
	// 除了表头，每一行都要有这一格；少一格就等于那一行的月份在桌面上看不出来。
	if got := strings.Count(page, `class="route-txn-period"`); got != 2 {
		t.Fatalf("识别租金月份列渲染了 %d 格，want 2（两行各一格）", got)
	}
	if !strings.Contains(page, `<td class="route-txn-period">2026-09</td>`) {
		t.Fatal("已识别出来的月份没有直接显示在行里")
	}
	if !strings.Contains(page, `class="route-txn-period-none">未识别<`) {
		t.Fatal("未识别的行渲染成了空单元格：空格看不出是没识别还是不需要识别")
	}
	for _, expected := range []string{"识别租金月份：2026-09", "识别租金月份：未识别"} {
		if !strings.Contains(page, expected) {
			t.Fatalf("窄屏卡片没有跟着显示 %q，两端说法不一致", expected)
		}
	}

	// 状态列现在排第 10，新增 Description 后整张表有 11 列；1440 宽下正好铺满 1146px。
	// 短列不锁 nowrap，浏览器被挤窄时会挑软柿子：日期从连字符处断成 "09-"/"01"、
	// "同住代付"四个汉字断成两行。锁住之后被压缩的只剩「匹配依据」那句本就该折行的
	// 说明。加列之前没有这个现象，所以这条断言是跟着新列一起来的。
	nowrap := regexp.MustCompile(`\.transaction-route-desktop-table th, \.transaction-route-desktop-table \.route-txn-fixed \{[^}]*white-space:\s*nowrap`)
	if !nowrap.MatchString(page) {
		t.Fatal("短列丢了 nowrap：多出来的这一列会把日期和用途挤成两行")
	}
	if !strings.Contains(page, `<td class="mono route-txn-fixed">`) {
		t.Fatal("日期格没有挂上 route-txn-fixed，nowrap 落不到它身上")
	}
}

// 详情页对已关联的流水写着「如需标记为非租金，请先在流水列表撤销匹配」——而新的
// /transactions 表格里根本没有撤销，那句话指的就是个死胡同：点错了只能一笔笔点进
// 详情页看，看完也没地方改。老表格（/billing）一直有修改匹配 + 撤销匹配，新的这套
// 建的时候只搬了「匹配流水」，把改错的出口漏了。
//
// 这条盯住出口本身：已关联的行必须同时给出改法和撤销，且两个表单都要带 return_to，
// 否则改完会被扔回没筛选的列表。
func TestMatchedTransactionRowOffersReviewAndFullRevokeOnMobile(t *testing.T) {
	page := renderTransactionListPage(t, transactionListPageData{TransactionRows: []transactionPageRow{{
		ID: "12", DetailKey: "12", DetailURL: "/transactions?detail=12", MatchURL: "/transactions?match=12",
		Direction: "income", PayerName: "BRID NI BHRAONAIN", MatchStatus: "matched", MatchStatusLabel: "已关联",
		ReturnURL: "/transactions?match_status=pending",
	}}})
	for _, expected := range []string{`href="/transactions?match=12"`, `href="/transactions/revoke?transaction_id=12`, ">调整分配</a>", ">撤销整笔匹配</a>"} {
		if got := strings.Count(page, expected); got != 2 {
			t.Fatalf("desktop/mobile %q count=%d", expected, got)
		}
	}
	if strings.Contains(page, `action="/transactions/rematch"`) {
		t.Fatal("legacy rematch form remains")
	}
}

// 没有关联的行不该出现改错的出口——那两个按钮只会让人以为已经关联了。
func TestUnmatchedTransactionRowHidesRematchAndRevoke(t *testing.T) {
	page := renderTransactionListPage(t, transactionListPageData{
		CanonicalPath: "/transactions", TransactionScope: "pending",
		TransactionRows: []transactionPageRow{{
			ID: "13", DetailKey: "13", DetailURL: "/transactions?detail=13",
			Direction: "income", PayerName: "NO REFERENCE LTD", AmountDisplay: "€40.00",
			MatchStatus: "unmatched", MatchStatusLabel: "未匹配",
		}},
	})
	for _, unexpected := range []string{"修改匹配", "撤销匹配", "/transactions/rematch", "/transactions/revoke"} {
		if strings.Contains(page, unexpected) {
			t.Fatalf("未关联的行不该出现 %q", unexpected)
		}
	}
}
