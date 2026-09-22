package main

import (
	"net/url"
	"regexp"
	"strings"
	"testing"
	"time"
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
			CandidateTenantName: "WAHAJULLAH KHAN", CandidateRentObligationID: 301, CandidatePeriod: "2026-09", CanConfirm: true, ReturnURL: "/transactions",
			ManualMatchTenantOptions: []billingTenantOption{{ID: 7, Name: "WAHAJULLAH KHAN"}},
			ManualMatchOptions:       []billingRentMatchOption{{TenantID: 7, TenantName: "WAHAJULLAH KHAN", Period: "2026-09", PeriodLabel: "2026年9月", Remaining: "€1,250.00"}},
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
		`name="tenant_id" aria-label="选择匹配租客" data-searchable`,
		`name="return_to" value="/transactions"`,
		`href="/transactions?detail=7&amp;match_status=pending"`,
	} {
		if !strings.Contains(page, marker) {
			t.Errorf("transaction route missing %q", marker)
		}
	}
	// The compact form used to carry its own 关联状态 selector next to this one.
	// It is gone: the two drifted apart, and the surviving selector owns the question.
	quickFilters := markupBetween(t, page, `<form class="transaction-route-quickfilter"`, `</form>`)
	if strings.Contains(quickFilters, `<select name="match_status"`) {
		t.Errorf("compact filter form duplicates the status selector the filter bar already owns: %s", quickFilters)
	}
	for _, kept := range []string{`name="scope"`, `name="payer"`, `name="period"`} {
		if !strings.Contains(quickFilters, kept) {
			t.Errorf("compact filter form lost %q: %s", kept, quickFilters)
		}
	}
	advancedFilters := markupBetween(t, page, `<form class="filterbar"`, `</form>`)
	advancedStatus := markupBetween(t, advancedFilters, `<select id="match_status"`, `</select>`)
	if !strings.Contains(advancedStatus, `onchange="this.form.requestSubmit()"`) {
		t.Errorf("match-status selector does not submit its filter form on change: %s", advancedStatus)
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
	if !strings.Contains(actionCell, `href="/transactions?detail=7&amp;match_status=pending"`) || !strings.Contains(actionCell, `>匹配流水</summary>`) {
		t.Errorf("directly matchable transaction row must keep both detail and match actions: %s", actionCell)
	}
}

func TestDirectTransactionMatchUsesCalendarWithoutRepeatedMonthOptions(t *testing.T) {
	page := renderBillingPage(t, billingPageData{
		workspaceShell: workspaceShell{ActivePage: "transactions", CompactTitle: "流水处理"},
		PageKey:        "transactions", CanonicalPath: "/transactions",
		TransactionRows: []transactionPageRow{{
			ID: "7", DetailURL: "/transactions?detail=7", ReturnURL: "/transactions",
			ManualMatchTenantOptions: []billingTenantOption{{ID: 7, Name: "Aoife Murphy"}, {ID: 8, Name: "Bríd Murphy"}},
			ManualMatchOptions: []billingRentMatchOption{
				{TenantID: 7, TenantName: "Aoife Murphy", Period: "2026-09", PeriodLabel: "2026年9月", Expected: "€950.00", Remaining: "€950.00"},
				{TenantID: 8, TenantName: "Bríd Murphy", Period: "2026-09", PeriodLabel: "2026年9月", Expected: "€1,100.00", Remaining: "€1,100.00"},
			},
		}},
	})

	form := markupBetween(t, page, `action="/transactions/confirm" data-tenant-period-match`, `</form>`)
	for _, marker := range []string{
		`type="month"`,
		`data-tenant-period-input`,
		`data-tenant-period-options`,
		`data-tenant-period-rent`,
		`aria-live="polite"`,
		// 选完租客和月份之后那一行要报出所选月份的交租数字，所以数据袋里必须
		// 同时有「应交」和「未收」——前端只负责显示，金额由服务端格式化好。
		`data-label="2026年9月"`,
		`data-due="€950.00"`,
		`data-rent="€950.00"`,
	} {
		if !strings.Contains(form, marker) {
			t.Errorf("calendar match form missing %q: %s", marker, form)
		}
	}
	if strings.Contains(form, `<select name="period"`) {
		t.Errorf("calendar match form still renders the repeated period select: %s", form)
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

	fromMonthOptions := tenantPeriodMatchCalendarForTenant([]billingMonthOption{
		{Period: "2026-09", Label: "2026年9月", Expected: "€800.00", Paid: "€300.00", Remaining: "€500.00"},
	}, 8, "")
	if got := fromMonthOptions.Options; len(got) != 1 || got[0].Expected != "€800.00" || got[0].Remaining != "€500.00" {
		t.Fatalf("month calendar options dropped the amounts: %+v", got)
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

// The compact form and the filter bar each carried a 关联状态 selector, and they
// drifted: the compact one never gained 已忽略, so a reader who used it could not
// reach ignored rows at all. The filter bar keeps the only one, and it has to stay
// able to reach every status the filter parser accepts.
func TestTransactionStatusSelectorExistsExactlyOnceAndReachesEveryStatus(t *testing.T) {
	page := renderBillingPage(t, billingPageData{
		workspaceShell: workspaceShell{ActivePage: "transactions"},
		PageKey:        "transactions", CanonicalPath: "/transactions",
		// Non-empty on purpose: the pager then renders its own hidden
		// name="match_status" field, which must not be mistaken for a selector.
		MatchStatusSelection: "partial",
	})

	// The survivor carries an id; the compact one did not. Counting the bare
	// name="match_status" would also catch the pager's hidden field, so both shapes
	// are asserted separately.
	if got := strings.Count(page, `id="match_status"`); got != 1 {
		t.Fatalf("transactions page renders %d status selectors, want 1: %s", got, page)
	}
	if strings.Contains(page, `<select name="match_status"`) {
		t.Error("transactions page still renders the retired compact status selector")
	}
	status := markupBetween(t, page, `<select id="match_status"`, `</select>`)
	for _, option := range []string{
		`value="pending"`, `value="matched"`, `value="partial"`,
		`value="candidate"`, `value="unmatched"`, `value="needs_review"`, `value="ignored"`,
	} {
		if !strings.Contains(status, option) {
			t.Errorf("the surviving status selector cannot reach %s: %s", option, status)
		}
	}

	// The selector's own label has to keep the word its options use. It read
	// 关联状态 while every option under it said 关联 — the same split, one line up.
	if !strings.Contains(page, `for="match_status">关联状态<`) {
		t.Error("the status selector's label does not use the 关联 word its own options use")
	}

	// 已关联 is the app-wide word for matched, borrowed from the status labels the
	// row markup already renders. The 匹配/关联 split must not come back.
	for _, stale := range []string{"已匹配", "未匹配", "部分匹配"} {
		if strings.Contains(page, stale) {
			t.Errorf("transactions page still renders the retired word %q", stale)
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

	for _, expected := range []string{`id="payer"`, `id="tenant_id"`, `id="tenant_id" name="tenant_id" data-searchable`, `id="period"`, `id="rent_period"`, `id="allocation"`, `id="direction"`, `id="match_status"`} {
		if !strings.Contains(filterBar, expected) {
			t.Fatalf("filter bar lost %q: %s", expected, filterBar)
		}
	}
	for _, removed := range []string{`id="arrival_from"`, `id="arrival_to"`, `id="sort"`, `id="pending"`, `id="page_size"`} {
		if strings.Contains(filterBar, removed) {
			t.Fatalf("filter bar still renders the removed control %q: %s", removed, filterBar)
		}
	}

	// 待处理 is no longer its own checkbox: it is the first option of 关联状态, and
	// a link that still carries pending=1 has to light it up.
	matchStatus := markupBetween(t, filterBar, `<select id="match_status"`, `</select>`)
	if !strings.Contains(matchStatus, `<option value="pending" selected>待处理`) {
		t.Fatalf("关联状态 does not absorb the 待处理 filter: %s", matchStatus)
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

// /transactions 的表格加了「识别租金月份」列。这里钉三件事：已识别的行直接显示月份
// （2026-09，和详情页同一款值）；没解析出来的行显示"未识别"而不是空格；窄屏卡片跟着
// 一起显示。
//
// 为什么要单独钉"未识别"：空格和"这笔压根不用管月份"在屏幕上长得一模一样。留空的话
// 房东分不清是没识别出来、还是不需要识别，这一列就白加了。
func TestTransactionRouteShowsTheParsedRentMonth(t *testing.T) {
	page := renderBillingPage(t, billingPageData{
		PageKey: "transactions", CanonicalPath: "/transactions", TransactionScope: "pending",
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

	if !strings.Contains(page, "<th>识别租金月份</th>") {
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

	// 这是第 10 列，而表格在加列之前就已经占满容器（1440 宽下 9 列正好铺满 1146px）。
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
func TestMatchedTransactionRowOffersRematchAndRevoke(t *testing.T) {
	page := renderBillingPage(t, billingPageData{
		PageKey: "transactions", CanonicalPath: "/transactions", TransactionScope: "pending",
		TransactionRows: []transactionPageRow{{
			ID: "12", DetailKey: "12", DetailURL: "/transactions?detail=12",
			Direction: "income", PayerName: "BRID NI BHRAONAIN", AmountDisplay: "€950.00",
			MatchStatus: "matched", MatchStatusLabel: "已关联", MatchedTenantName: "Bríd Ní Bhraonáin",
			CanRematch: true, CanEditRentMatch: true,
			RematchTenantOptions: []billingTenantOption{{ID: 11, Name: "Bríd Ní Bhraonáin"}},
			RematchMonthOptions:  []billingMonthOption{{Period: "2026-10", Label: "2026 年 10 月", Remaining: "€950.00"}},
			ReturnURL:            "/transactions?match_status=pending",
		}},
	})

	// 两种改错出口都在。
	for _, expected := range []string{`<summary class="btn">修改匹配</summary>`, `<summary class="btn danger">撤销匹配</summary>`} {
		if !strings.Contains(page, expected) {
			t.Fatalf("已关联的行没有 %q，点错了没法改", expected)
		}
	}
	// 修改：POST 到 /transactions/rematch，带回跳地址。
	if !strings.Contains(page, `<form method="post" action="/transactions/rematch">`) {
		t.Fatal("修改匹配没有指向 /transactions/rematch")
	}
	// 撤销：这里必须是 GET。撤销是两步走的——先跳到确认页把全部分配摊开，填完原因
	// 才真的 POST。写成 POST 就跳过确认页直接作废了。
	if !strings.Contains(page, `<form method="get" action="/transactions/revoke">`) {
		t.Fatal("撤销匹配应该是 GET 到确认页，不是直接提交作废")
	}
	// 桌面表格和窄屏卡片各一份，所以是 4。两边都得带 return_to——窄屏那侧少了它，
	// 手机上改完一笔会掉回没筛选的列表。
	if got := strings.Count(page, `name="return_to" value="/transactions?match_status=pending"`); got != 4 {
		t.Fatalf("return_to 出现 %d 次，want 4（桌面/窄屏 × 修改/撤销）；少的那份改完会掉回没筛选的列表", got)
	}
	// 窄屏那边单独钉一下：桌面表格 ≤640px 是 display:none，手机上只剩卡片这一份。
	// 只给"处理流水"的话，点进详情页也没有撤销——那里写的是"请先在流水列表撤销匹配"，
	// 而手机上根本没有那张列表。
	mobile := markupBetween(t, page, `<div class="transaction-route-mobile-list">`, `</article>`)
	if !strings.Contains(mobile, `summary class="btn">修改匹配`) || !strings.Contains(mobile, `summary class="btn danger">撤销匹配`) {
		t.Fatal("窄屏卡片没有改错出口：手机上点错了没地方改")
	}
	if !strings.Contains(mobile, `<form method="get" action="/transactions/revoke">`) {
		t.Fatal("窄屏的撤销不是 GET 到确认页")
	}
}

// 没有关联的行不该出现改错的出口——那两个按钮只会让人以为已经关联了。
func TestUnmatchedTransactionRowHidesRematchAndRevoke(t *testing.T) {
	page := renderBillingPage(t, billingPageData{
		PageKey: "transactions", CanonicalPath: "/transactions", TransactionScope: "pending",
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
