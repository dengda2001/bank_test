package main

import (
	"context"
	"fmt"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestTenantReceiptFinderFiltersNamesDatesAndURLs(t *testing.T) {
	clues := tenantReceiptNameClues(tenant{Name: "Qiang Li", DisplayAlias: "Q. Li"})
	if len(clues) < 4 || clues[0].Label != "Qiang Li" || clues[1].Label != "Q. Li" {
		t.Fatalf("name and token buttons missing: %+v", clues)
	}
	for _, value := range []url.Values{{"find_scope": {"year"}}, {"find_field": {"tenant"}}, {"find_page": {"0"}}, {"find_q": {strings.Repeat("a", 192)}}} {
		if _, err := tenantReceiptFinderFiltersFromQuery(value); err == nil {
			t.Fatalf("accepted invalid finder filters: %v", value)
		}
	}
	base := "/rent-dashboard?period=2026-09&view=tenants&search=Qiang&page=2"
	opened := tenantReceiptFinderURL(base, 9, tenantReceiptFinderFilters{Scope: "two", Mode: "name", Clue: "name", Field: "all", Page: 3}, 42)
	parsed, err := url.Parse(opened)
	if err != nil {
		t.Fatal(err)
	}
	q := parsed.Query()
	for key, want := range map[string]string{"period": "2026-09", "view": "tenants", "search": "Qiang", "page": "2", "find_tenant": "9", "find_page": "3", "match": "42", "match_tenant": "9", "match_month": "2026-09", "match_origin": "lookup"} {
		if q.Get(key) != want {
			t.Fatalf("%s=%q want %q in %s", key, q.Get(key), want, opened)
		}
	}
	if got := tenantReceiptLike("A_100%!"); got != "%a!_100!%!!%" {
		t.Fatalf("LIKE escaping = %q", got)
	}
	before := time.Date(2026, 9, 26, 9, 0, 0, 0, time.UTC)
	start, end := tenantReceiptDateBounds("two", before)
	if got := start.In(bankLocalTime).Format("2006-01-02 15:04"); got != "2026-08-01 00:00" {
		t.Fatalf("two-month local start = %s", got)
	}
	if got := end.In(bankLocalTime).Format("2006-01-02 15:04"); got != "2026-10-01 00:00" {
		t.Fatalf("two-month local end = %s", got)
	}
}

func TestTenantReceiptFinderTemplateHasOneDrawerAndSearchControls(t *testing.T) {
	filters := defaultRentWorkspaceFilters(time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC))
	filters.View = rentWorkspaceViewTenants
	base := rentWorkspaceURL(filters, 1)
	finder := tenantReceiptFinderData{
		TenantID: 7, TenantName: "Qiang Li", PeriodLabel: "2026年9月", Balance: "EUR 1200.00", HasBalance: true,
		Scope: "two", Mode: "all", Field: "all", CloseURL: base, SearchURL: base,
		Clues: []tenantReceiptClue{{Key: "name", Label: "Qiang Li", URL: base + "&find_clue=name"}},
		Rows:  []tenantReceiptFinderRow{{ID: 42, Date: "2026-09-12 10:45", Payer: "QIANG LI", Description: "September rent", Amount: "EUR 1200.00", Remaining: "EUR 600.00", Status: "部分关联", SelectURL: base + "&match=42"}},
		Total: 1, Page: 1,
	}
	page := renderRentWorkspace(t, rentWorkspacePageData{Filters: filters, Period: "2026-09", View: rentWorkspaceViewTenants, ReceiptFinder: &finder})
	for _, marker := range []string{`role="dialog"`, `为 Qiang Li 找银行流水`, `2026-09-12 10:45`, `September rent`, `QIANG LI`, `EUR 600.00`, `付款人`, `Description`, `用于本月租金`} {
		if !strings.Contains(page, marker) {
			t.Fatalf("finder missing %q", marker)
		}
	}
	if got := strings.Count(page, `role="dialog"`); got != 1 {
		t.Fatalf("finder rendered %d dialogs, want one", got)
	}
	finder.HasBalance = false
	finder.HasObligation = true
	settled := renderRentWorkspace(t, rentWorkspacePageData{Filters: filters, Period: "2026-09", View: rentWorkspaceViewTenants, ReceiptFinder: &finder})
	if !strings.Contains(settled, "本月已结清") || strings.Contains(settled, "未付 EUR 1200.00") {
		t.Fatal("settled month still displays an outstanding balance")
	}
}

func TestTenantReceiptSelectedSourceStaysInOneDrawerAndKeepsReturnContext(t *testing.T) {
	filters := defaultRentWorkspaceFilters(time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC))
	filters.View = rentWorkspaceViewTenants
	review := transactionMatchReviewData{
		Source:            transactionPageRow{ID: "42", AmountDisplay: "EUR 1200.00", RemainingAmountDisplay: "EUR 600.00", PayerName: "QIANG LI", Description: "September rent", MatchStatus: "partial", MatchStatusLabel: "部分关联"},
		SourceAmountCents: 120000, SourceRemainingCents: 60000,
		FinderMode: true, FinderCanPrefill: true, FinderTenantName: "Qiang Li", FinderPeriod: "2026-09", FinderBackURL: "/rent-dashboard?find_tenant=7&period=2026-09&view=tenants", FinderCloseURL: rentWorkspaceURL(filters, 1),
		SelectedTenantID: 7, SelectedTenantName: "Qiang Li", FormAction: "/rent-dashboard", ReturnURL: "/rent-dashboard?find_tenant=7&period=2026-09&view=tenants",
		Months: []transactionReviewMonth{{Period: "2026-09", Label: "2026年9月", Remaining: "EUR 800.00", RemainingCents: 80000, Selectable: true}},
	}
	page := renderRentWorkspace(t, rentWorkspacePageData{Filters: filters, Period: "2026-09", View: rentWorkspaceViewTenants, MatchReview: &review})
	for _, marker := range []string{`data-prefill-period="2026-09"`, `data-finder-context="7:2026-09"`, `返回流水列表`, `action="/transactions/confirm-batch"`, `data-add-match data-period="2026-09"`, `name="return_to" value="/rent-dashboard?find_tenant=7`} {
		if !strings.Contains(page, marker) {
			t.Fatalf("selected receipt drawer missing %q", marker)
		}
	}
	if strings.Count(page, `class="entity-drawer-backdrop transaction-review-backdrop"`) != 1 || strings.Contains(page, `class="entity-drawer-backdrop tenant-finder-backdrop"`) {
		t.Fatal("selected receipt opened a second drawer instead of reusing the drawer position")
	}
	review.FinderCanPrefill = false
	unavailable := renderRentWorkspace(t, rentWorkspacePageData{Filters: filters, Period: "2026-09", View: rentWorkspaceViewTenants, MatchReview: &review})
	if !strings.Contains(unavailable, "本月暂无可分配余额") {
		t.Fatal("unavailable target month needs a visible reason")
	}
	request := httptest.NewRequest("POST", "/transactions/confirm-batch", nil)
	request.Form = url.Values{"return_to": {review.ReturnURL}}
	if target := transactionReturnTarget(request); !strings.Contains(target, "find_tenant=7") || !strings.Contains(target, "period=2026-09") {
		t.Fatalf("confirmation lost finder context: %s", target)
	}
}

func TestTenantReceiptFinderQueryOnMySQL(t *testing.T) {
	db, sqlDB := openLedgerMySQLTestDB(t)
	if err := runMigrations(sqlDB, "../../migrations"); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	stamp := time.Now().UnixNano()
	owner := user{Username: fmt.Sprintf("finder-%d", stamp), PasswordHash: "test"}
	other := user{Username: fmt.Sprintf("finder-other-%d", stamp), PasswordHash: "test"}
	for _, row := range []*user{&owner, &other} {
		if err := db.Create(row).Error; err != nil {
			t.Fatal(err)
		}
	}
	t.Cleanup(func() {
		ids := []uint64{owner.ID, other.ID}
		_ = db.Where("user_id IN ?", ids).Delete(&paymentTransactionAction{}).Error
		_ = db.Where("user_id IN ?", ids).Delete(&paymentAllocation{}).Error
		_ = db.Where("user_id IN ?", ids).Delete(&paymentTransaction{}).Error
		_ = db.Where("user_id IN ?", ids).Delete(&tenant{}).Error
		_ = db.Delete(&user{}, owner.ID).Error
		_ = db.Delete(&user{}, other.ID).Error
	})
	tenantRow := tenant{UserID: owner.ID, Name: "Qiang Li", Status: "active"}
	if err := db.Create(&tenantRow).Error; err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 26, 12, 0, 0, 0, time.UTC)
	august := time.Date(2026, 8, 15, 10, 0, 0, 0, time.UTC)
	september := time.Date(2026, 9, 12, 10, 0, 0, 0, time.UTC)
	old := time.Date(2026, 5, 10, 10, 0, 0, 0, time.UTC)
	rows := []paymentTransaction{
		{UserID: owner.ID, Source: "test", StableTransactionKey: fmt.Sprintf("finder-payer-%d", stamp), Direction: "income", AmountCents: 120000, Currency: "EUR", TransactionTime: &september, MatchStatus: "partial", PayerName: nullableString("QIANG LI"), Description: "monthly payment"},
		{UserID: owner.ID, Source: "test", StableTransactionKey: fmt.Sprintf("finder-desc-%d", stamp), Direction: "income", AmountCents: 60000, Currency: "EUR", TransactionTime: &august, MatchStatus: "candidate", PayerName: nullableString("Someone Else"), Description: "Rent for Qiang"},
		{UserID: owner.ID, Source: "test", StableTransactionKey: fmt.Sprintf("finder-old-%d", stamp), Direction: "income", AmountCents: 60000, Currency: "EUR", TransactionTime: &old, MatchStatus: "unmatched", Description: "older Qiang transfer"},
		{UserID: owner.ID, Source: "test", StableTransactionKey: fmt.Sprintf("finder-expense-%d", stamp), Direction: "expense", AmountCents: 60000, Currency: "EUR", TransactionTime: &september, MatchStatus: "unmatched", Description: "Qiang repair"},
		{UserID: other.ID, Source: "test", StableTransactionKey: fmt.Sprintf("finder-private-%d", stamp), Direction: "income", AmountCents: 60000, Currency: "EUR", TransactionTime: &september, MatchStatus: "unmatched", Description: "Qiang private"},
		{UserID: owner.ID, Source: "test", StableTransactionKey: fmt.Sprintf("finder-defer-%d", stamp), Direction: "income", AmountCents: 60000, Currency: "EUR", TransactionTime: &september, MatchStatus: "unmatched", Description: "deferred Qiang"},
	}
	for i := range rows {
		if err := db.Create(&rows[i]).Error; err != nil {
			t.Fatal(err)
		}
	}
	allocation := paymentAllocation{UserID: owner.ID, PaymentTransactionID: rows[0].ID, AmountCents: 60000, AllocationKind: allocationKindDeposit, Status: allocationStatusConfirmed, ConfirmedByUserID: owner.ID, ConfirmedAt: now}
	if err := db.Create(&allocation).Error; err != nil {
		t.Fatal(err)
	}
	deferAction := paymentTransactionAction{UserID: owner.ID, PaymentTransactionID: rows[5].ID, ActionKind: transactionActionDefer, Reason: "later", OperationID: fmt.Sprintf("finder-defer-%d", stamp), ActedByUserID: owner.ID}
	if err := db.Create(&deferAction).Error; err != nil {
		t.Fatal(err)
	}
	workspace := defaultRentWorkspaceFilters(now)
	workspace.View = rentWorkspaceViewTenants
	load := func(values url.Values) tenantReceiptFinderData {
		t.Helper()
		result, err := loadTenantReceiptFinder(ctx, db, owner.ID, tenantRow.ID, workspace, values, now)
		if err != nil {
			t.Fatal(err)
		}
		return result
	}
	defaultList := load(nil)
	if defaultList.Total != 2 || len(defaultList.Rows) != 2 || defaultList.Rows[0].ID != rows[0].ID || defaultList.Rows[0].Remaining != "EUR 600.00" {
		t.Fatalf("default finder included excluded receipts or lost partial balance: %+v", defaultList)
	}
	nameList := load(url.Values{"find_mode": {"name"}, "find_clue": {"name"}})
	if nameList.Total != 1 || nameList.Rows[0].ID != rows[0].ID {
		t.Fatalf("whole-name clue matched wrong receipts: %+v", nameList.Rows)
	}
	firstToken := tenantReceiptNameClues(tenantRow)[1].Key
	tokenList := load(url.Values{"find_mode": {"name"}, "find_clue": {firstToken}})
	if tokenList.Total != 2 {
		t.Fatalf("token clue count=%d, want payer and description", tokenList.Total)
	}
	textList := load(url.Values{"find_q": {"someone"}, "find_field": {"payer"}})
	if textList.Total != 1 || textList.Rows[0].ID != rows[1].ID {
		t.Fatalf("payer search = %+v", textList.Rows)
	}
	olderList := load(url.Values{"find_scope": {"all"}})
	if olderList.Total != 3 {
		t.Fatalf("all dates count=%d want 3", olderList.Total)
	}
	clamped := load(url.Values{"find_page": {"100"}})
	if clamped.Page != 1 || strings.Contains(clamped.SearchURL, "find_page=") {
		t.Fatalf("stale page was not clamped in the return URL: %+v", clamped)
	}
	if _, err := loadTenantReceiptFinder(ctx, db, owner.ID, tenantRow.ID+999999, workspace, nil, now); err == nil {
		t.Fatal("unknown tenant finder was accessible")
	}
}
