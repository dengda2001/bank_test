package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// Contract pinned by 09-19-list-pages-alignment: the ten list pages share a
// page-head primary action, a data-backed filterbar and a row action, and the
// /bills manual-balance form gained the prototype's field set without changing
// any field name, action URL or accounting rule.

func TestSettleDispositionEnabledOnlyForMatchingPayment(t *testing.T) {
	for _, value := range []string{"", settleDispositionMatchPayment} {
		if !settleDispositionImplemented(value) {
			t.Fatalf("disposition %q should be implemented", value)
		}
	}
	for _, value := range []string{settleDispositionCashReceipt, settleDispositionWaiver, settleDispositionCarryForward, "unknown"} {
		if settleDispositionImplemented(value) {
			t.Fatalf("disposition %q has no accounting rule but is reported as implemented", value)
		}
	}
}

func TestCollectionSettleDispositionsRenderPrototypeOptions(t *testing.T) {
	options := collectionSettleDispositions("")
	want := []struct{ value, label string }{
		{settleDispositionMatchPayment, "匹配现有收款"},
		{settleDispositionCashReceipt, "登记现金收款"},
		{settleDispositionWaiver, "登记减免"},
		{settleDispositionCarryForward, "结转下月"},
	}
	if len(options) != len(want) {
		t.Fatalf("options=%+v", options)
	}
	for index, expected := range want {
		if options[index].Value != expected.value || options[index].Label != expected.label {
			t.Fatalf("option %d=%+v want %+v", index, options[index], expected)
		}
		if options[index].Selected != (expected.value == settleDispositionMatchPayment) {
			t.Fatalf("option %d selected=%v", index, options[index].Selected)
		}
	}
}

// The raw handler codes must keep rendering verbatim; only the codes /bills now
// emits itself get a Chinese sentence. TestBillsPageSurfacesInvalidFilterAndPeriodErrors
// depends on the fallthrough.
func TestBillsNoticeMappingKeepsUnknownCodesRaw(t *testing.T) {
	for _, code := range []string{"invalid_dashboard_filter", "invalid_period", "something_else"} {
		if got := billsErrorText(code); got != code {
			t.Fatalf("billsErrorText(%q)=%q want the raw code", code, got)
		}
	}
	if got := billsErrorText("manual_balance_disposition_unimplemented"); !strings.Contains(got, "尚未实现") {
		t.Fatalf("unimplemented disposition notice=%q", got)
	}
	if got := billsMessageText("bills_generated"); !strings.Contains(got, "已生成") {
		t.Fatalf("generate notice=%q", got)
	}
}

func TestListManualBalanceRedirectRetargetsTheSubmittingList(t *testing.T) {
	values := url.Values{
		"period":    {"2026-09"},
		"search":    {"Aoife Murphy"},
		"status":    {"unpaid"},
		"sort":      {"due_desc"},
		"page":      {"2"},
		"page_size": {"24"},
	}
	got := listManualBalanceRedirect("/bills", values, "manual_balance_saved", "")
	parsed, err := url.Parse(got)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Path != "/rent-dashboard" || parsed.Query().Get("view") != "tenants" {
		t.Fatalf("bills redirect path=%q", parsed.Path)
	}
	for key, want := range map[string]string{
		"period":    "2026-09",
		"search":    "Aoife Murphy",
		"status":    "outstanding",
		"sort":      "due_desc",
		"page":      "2",
		"page_size": "24",
		"message":   "manual_balance_saved",
	} {
		if value := parsed.Query().Get(key); value != want {
			t.Fatalf("bills redirect %s=%q want %q", key, value, want)
		}
	}
	// The dashboard contract is untouched -- the same helper still targets it.
	dashboard, err := url.Parse(dashboardManualBalanceRedirect(values, "manual_balance_saved", ""))
	if err != nil {
		t.Fatal(err)
	}
	if dashboard.Path != "/rent-dashboard" {
		t.Fatalf("dashboard redirect path=%q", dashboard.Path)
	}
}

// "生成本月账单" stopped being a self-link: it is an explicit POST to
// /bills/generate that carries the list context back on the redirect.
func TestBillsGenerateActionReplacesTheSelfLink(t *testing.T) {
	page, err := executeTemplate(billsPageTemplate, rentDashboardPageData{
		Period: "2026-09", PeriodLabel: "2026年9月", Page: 1, PageSize: 12, TotalPages: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{
		`action="/bills/generate"`,
		`method="post"`,
		`name="period" value="2026-09"`,
	} {
		if !strings.Contains(page, marker) {
			t.Fatalf("bills page missing generate marker %q", marker)
		}
	}
}

// The settle form must be renderable from any page template, not just from
// /bills' row context: subtask ③ renders the same markup inline on the rent
// workspace tenant view. The partial lives in workspaceBase, so a page that has
// never heard of /bills can still call it with a hand-built view.
func TestSettleFormPartialIsAvailableToEveryPageTemplate(t *testing.T) {
	standalone := newWorkspacePageTemplate("settle-form-reuse-probe", nil, `{{template "collection-settle-form" .}}`)
	page, err := executeTemplate(standalone, collectionSettleFormView{
		ObligationID:      42,
		DutyLabel:         "陈先生 · 2026年9月 租金责任",
		OutstandingAmount: "EUR 640.00",
		EffectiveDate:     "2026-09-20",
		Dispositions:      collectionSettleDispositions(settleDispositionMatchPayment),
		ReturnPeriod:      "2026-09",
		ReturnSearch:      "",
		ReturnStatus:      "all",
		ReturnSort:        "status",
		ReturnPage:        1,
		ReturnPageSize:    12,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{
		`action="/rent-dashboard/settle"`,
		`name="return_to"`,
		`name="obligation_id" value="42"`,
		`待平账责任`,
		`陈先生 · 2026年9月 租金责任`,
		`name="disposition"`,
		`匹配现有收款`,
		`登记现金收款`,
		`登记减免`,
		`结转下月`,
		`name="amount" value="EUR 640.00" readonly`,
		`name="effective_date" type="date" value="2026-09-20" readonly`,
		`name="reason" maxlength="512" placeholder="平账原因"`,
		`required`,
		`name="page" value="1"`,
		`name="page_size" value="12"`,
	} {
		if !strings.Contains(page, marker) {
			t.Fatalf("reused settle form missing %q: %s", marker, page)
		}
	}
}

func TestDunningPageHeadCarriesTheBatchSendPrimary(t *testing.T) {
	page, err := executeTemplate(dunningPageTemplate, rentDashboardPageData{
		Period: "2026-09", PeriodLabel: "2026年9月",
		Dunning: dunningDrawerData{
			Period: "2026-09",
			Candidates: []dunningCandidate{{
				TenantID: 7, ObligationID: 11, TenantName: "Aoife Murphy", Period: "2026-09",
				BalanceAmount: "EUR 600.00", Status: "overdue", StatusLabel: "逾期",
				Email: "aoife@example.test", EmailValid: true, Selectable: true, Selected: true,
			}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	head := markupBetween(t, page, `<header class="collection-toolbar">`, `</header>`)
	for _, marker := range []string{`批量发送提醒`, `form="dunning-candidate-form"`, `formaction="/dunning/send"`} {
		if !strings.Contains(head, marker) {
			t.Fatalf("dunning page head missing %q: %s", marker, head)
		}
	}
	if !strings.Contains(page, `id="dunning-candidate-form"`) {
		t.Fatalf("dunning candidates form has no id to associate the head action: %s", page)
	}
	if strings.Contains(page, "发送已选") {
		t.Fatal("dunning page still carries the old 发送已选 label")
	}
	if !strings.Contains(page, `href="/tenants/7?from_month=2026-09&amp;to_month=2026-09">查看详情</a>`) {
		t.Fatalf("dunning candidate row has no detail action: %s", page)
	}
}

func TestBankPageExposesAddAccountAndAccountActions(t *testing.T) {
	connected, err := executeTemplate(bankPageTemplate, bankPageData{Connected: true, Provider: "truelayer", Environment: "sandbox"})
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{
		`href="/bank/connect">添加银行账户</a>`,
		`action="/bank/sync"`,
		`>立即同步</button>`,
		`href="/bank/connect">重新授权</a>`,
	} {
		if !strings.Contains(connected, marker) {
			t.Fatalf("connected bank page missing %q", marker)
		}
	}
	if strings.Contains(connected, "同步银行流水") {
		t.Fatal("bank page still renders the old 同步银行流水 label")
	}

	disconnected, err := executeTemplate(bankPageTemplate, bankPageData{Provider: "truelayer", Environment: "sandbox"})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(disconnected, `>添加银行账户</a>`) {
		t.Fatal("bank page head lost 添加银行账户")
	}
	if strings.Contains(disconnected, `>立即同步</button>`) {
		t.Fatal("bank page offers 立即同步 without a connection")
	}
}

func TestRoomsPageDoesNotRenderAssetValidityDates(t *testing.T) {
	page, err := executeTemplate(roomPageTemplate, roomPageData{
		Period: "2026-09",
		Rows:   []roomPageRow{{ID: 3, PropertyID: 1, PropertyName: "Rosewood", RoomLabel: "A-01"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, marker := range []string{"active_from", "inactive_from", "<small>自 2026-09</small>"} {
		if strings.Contains(page, marker) {
			t.Fatalf("room list contains asset validity marker %q", marker)
		}
	}
}

// Every list page's head carries a primary action and every rendered row carries
// an action, which is the "页头 + filterbar + 表格 + 操作列" contract of this
// subtask. Asserted on markup so it holds for the empty audit fixture too.
func TestListPageTemplatesKeepHeadActionAndActionColumn(t *testing.T) {
	rows := []struct {
		name    string
		render  func() (string, error)
		markers []string
	}{
		{"properties", func() (string, error) {
			return executeTemplate(propertyPageTemplate, propertyPageData{Rows: []propertyPageRow{{ID: 1, Name: "Rosewood"}}})
		}, []string{`href="/properties?add=1`, `>新增房产<`, `>操作</th>`, `>查看详情</a>`}},
		{"rooms", func() (string, error) {
			return executeTemplate(roomPageTemplate, roomPageData{Rows: []roomPageRow{{ID: 1, RoomLabel: "A-01"}}})
		}, []string{`href="/rooms?add=1`, `>新增房间<`, `>操作</th>`, `>查看详情</a>`}},
		{"tenants", func() (string, error) {
			return executeTemplate(tenantTemplate, tenantPageData{Rows: []tenantRecord{{ID: "7", Name: "Aoife Murphy"}}})
		}, []string{`href="/tenants?add=1">添加租客</a>`, `>操作</th>`, `>详情</a>`}},
		{"expenses", func() (string, error) {
			return executeTemplate(expenseTemplate, expensePageData{Rows: []expenseRecord{{ID: "1", Description: "维修"}}})
		}, []string{`href="/expenses?period=`, `>新增支出<`, `>操作</th>`, `>绑定发票</a>`}},
	}
	for _, tc := range rows {
		t.Run(tc.name, func(t *testing.T) {
			page, err := tc.render()
			if err != nil {
				t.Fatal(err)
			}
			for _, marker := range tc.markers {
				if !strings.Contains(page, marker) {
					t.Fatalf("%s page missing %q", tc.name, marker)
				}
			}
		})
	}
}

// The action column must read the same on every list page. /transactions is
// rendered by transaction_list_page.go rather than one of the page templates above, and
// this subtask left its row action as "处理" while every other page said
// "查看详情" -- the one row its own PRD asked for and did not get. Pinned
// separately so the next pass cannot quietly leave it behind again.
func TestTransactionRouteActionColumnSaysViewDetails(t *testing.T) {
	page := renderTransactionListPage(t, transactionListPageData{
		workspaceShell: workspaceShell{ActivePage: "transactions", CompactTitle: "流水处理"},
		CanonicalPath:  "/transactions", TransactionScope: "pending", PendingCount: 1,
		TransactionRows: []transactionPageRow{{
			ID: "7", InternalID: "7", DetailKey: "7", DetailURL: "/transactions?detail=7&match_status=pending",
			Direction: "income", PayerName: "WAHAJULLAH KHAN", AmountDisplay: "€1,250.00",
			MatchStatus: "candidate", MatchStatusLabel: "待确认",
		}},
	})
	// html/template escapes the & in the href to &amp;, so pin the two halves
	// rather than the literal URL the row was handed.
	for _, marker := range []string{`>状态</a></th><th>操作</th>`, `>查看详情</a>`, `/transactions?detail=7`} {
		if !strings.Contains(page, marker) {
			t.Fatalf("transactions action column missing %q", marker)
		}
	}
	if strings.Contains(page, `>处理</a>`) {
		t.Fatal(`the transactions row action still reads 处理; it must match the other nine pages`)
	}
}

// The three unimplemented dispositions must stop before any write: no income
// transaction, no allocation, no obligation change -- only the notice. The
// implemented branch (empty disposition == 匹配现有收款) is what the page used
// before this subtask and must keep settling exactly as it did.
func TestBillsSettleDispositionBranchesOnMySQL(t *testing.T) {
	db, sqlDB := openLedgerMySQLTestDB(t)
	if err := runMigrations(sqlDB, "../../migrations"); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	ctx := context.Background()
	owner := user{Username: fmt.Sprintf("list-pages-owner-%d", time.Now().UnixNano()), PasswordHash: "test"}
	if err := db.WithContext(ctx).Create(&owner).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.WithContext(ctx).Delete(&user{}, owner.ID).Error })

	input := validTenantInputForProfile()
	input.Name = "Disposition Tenant"
	tenantRow, err := newTenantService(db).createTenant(ctx, owner.ID, input)
	if err != nil {
		t.Fatal(err)
	}
	period := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	if err := newObligationService(db).ensureMonthlyObligations(ctx, owner.ID, period); err != nil {
		t.Fatal(err)
	}
	var obligation rentObligation
	if err := db.WithContext(ctx).Where("user_id = ? AND tenant_id = ? AND period_month = ?", owner.ID, tenantRow.ID, period).First(&obligation).Error; err != nil {
		t.Fatal(err)
	}

	a := testApp()
	a.db = db
	cookie := userSessionCookie(a.cfg, owner.ID, owner.Username, time.Now().Add(sessionTTL))

	ledgerCounts := func() (int64, int64) {
		var transactions, allocations int64
		if err := db.WithContext(ctx).Model(&paymentTransaction{}).Where("user_id = ?", owner.ID).Count(&transactions).Error; err != nil {
			t.Fatal(err)
		}
		if err := db.WithContext(ctx).Model(&paymentAllocation{}).Where("user_id = ?", owner.ID).Count(&allocations).Error; err != nil {
			t.Fatal(err)
		}
		return transactions, allocations
	}
	post := func(values url.Values) string {
		t.Helper()
		request := httptest.NewRequest(http.MethodPost, "/bills/settle", strings.NewReader(values.Encode()))
		request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		request.AddCookie(cookie)
		recorder := httptest.NewRecorder()
		a.handleDashboardManualBalance(recorder, request)
		if recorder.Code != http.StatusFound {
			t.Fatalf("settle status=%d body=%s", recorder.Code, recorder.Body.String())
		}
		return recorder.Header().Get("Location")
	}

	base := url.Values{
		"obligation_id": {fmt.Sprint(obligation.ID)},
		"period":        {"2026-09"},
		"status":        {"all"},
		"sort":          {dashboardDefaultSort},
		"page":          {"1"},
		"page_size":     {fmt.Sprint(dashboardDefaultPageSize)},
		"reason":        {"覆盖四个处理方式分支"},
	}

	beforeTransactions, beforeAllocations := ledgerCounts()
	for _, disposition := range []string{settleDispositionCashReceipt, settleDispositionWaiver, settleDispositionCarryForward} {
		values := url.Values{}
		for key, value := range base {
			values[key] = value
		}
		values.Set("disposition", disposition)
		location := post(values)
		if !strings.Contains(location, "error=manual_balance_disposition_unimplemented") {
			t.Fatalf("disposition %s redirect=%q", disposition, location)
		}
		if !strings.HasPrefix(location, "/bills?") {
			t.Fatalf("disposition %s redirected away from /bills: %q", disposition, location)
		}
		transactions, allocations := ledgerCounts()
		if transactions != beforeTransactions || allocations != beforeAllocations {
			t.Fatalf("disposition %s wrote to the ledger: transactions %d->%d allocations %d->%d", disposition, beforeTransactions, transactions, beforeAllocations, allocations)
		}
	}

	saved := post(base)
	if !strings.Contains(saved, "message=manual_balance_saved") || !strings.HasPrefix(saved, "/bills?") {
		t.Fatalf("default disposition redirect=%q", saved)
	}
	transactions, allocations := ledgerCounts()
	if transactions != beforeTransactions+1 || allocations != beforeAllocations+1 {
		t.Fatalf("default disposition did not settle: transactions %d->%d allocations %d->%d", beforeTransactions, transactions, beforeAllocations, allocations)
	}
	if location := post(base); !strings.Contains(location, "message=manual_balance_not_needed") {
		t.Fatalf("second settle redirect=%q", location)
	}
}
