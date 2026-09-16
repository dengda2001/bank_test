package main

import (
	"net/url"
	"strings"
	"testing"
)

func renderBillingPage(t *testing.T, data billingPageData) string {
	t.Helper()
	var body strings.Builder
	if err := billingTemplate.Execute(&body, data); err != nil {
		t.Fatal(err)
	}
	return body.String()
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
			Reference:             "ref-012",
			AccountName:           "Rent account",
			AccountID:             "account-345",
			MatchStatus:           "matched",
			MatchStatusLabel:      "已关联",
		}},
	})

	for _, unwanted := range []string{
		"付款人 ID", "payer-123", "内部 ID", "internal-456", "银行流水号", "provider-789",
		"参考号", "REF:", "ref-012", "account-345", "解析租金月", "2026年8月",
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

// Sorting by heading has to survive filtering and paging: the current column
// rides along in the filter form and in the pager, or 应用筛选 would reorder the
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
