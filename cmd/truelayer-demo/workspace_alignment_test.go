package main

import (
	"html/template"
	"strconv"
	"strings"
	"testing"
	"time"
)

// Tests for the /rent-dashboard alignment (task 09-19-dashboard-alignment).
//
// The prototype puts the view switch, the filters and the dimension chips
// inside the list panel, adds a 收缴率 column and an 操作 column to every list,
// and reuses the collection settle form for a row action
// (figma/rentops-desktop-suite.html). These tests pin that contract on the live
// room-workspace template so a later edit cannot quietly drop a column, move the
// tabs back out of the panel, or turn 处理流水 into a bare 处理 button.

func renderRentWorkspace(t *testing.T, data rentWorkspacePageData) string {
	t.Helper()
	page, err := executeTemplate(rentWorkspaceTemplate, data)
	if err != nil {
		t.Fatalf("render rent workspace: %v", err)
	}
	return page
}

func workspaceRoomTreeFixture() []rentWorkspaceRoomTreeRow {
	return []rentWorkspaceRoomTreeRow{{
		Room: rentWorkspaceRoomRow{
			RoomID: 4, PropertyID: 1, PropertyName: "Rosewood Court", RoomLabel: "01",
			Address: "Rosewood Court, Dublin 8", Status: "partial", StatusLabel: "部分缴纳",
			TenantCount: 2, ExpectedCents: 200000, PaidCents: 84000, BalanceCents: 116000,
			ExpectedAmount: "EUR 2000.00", PaidAmount: "EUR 840.00", BalanceAmount: "EUR 1160.00",
			DueDate: "2026-09-05", CollectionPercent: 42,
		},
		Tenants: []rentWorkspaceTenantRow{
			{
				TenantID: 11, TenantName: "Aoife Murphy", ObligationID: 71, Period: "2026-09",
				ExpectedCents: 125000, PaidCents: 50000, BalanceCents: 75000,
				ExpectedAmount: "EUR 1250.00", PaidAmount: "EUR 500.00", BalanceAmount: "EUR 750.00",
				CollectionPercent: 40, Status: "partial", StatusLabel: "部分缴纳",
			},
			{
				TenantID: 12, TenantName: "Brian Doyle", ObligationID: 72, Period: "2026-09",
				ExpectedCents: 125000, PaidCents: 125000, BalanceCents: 0,
				ExpectedAmount: "EUR 1250.00", PaidAmount: "EUR 1250.00", BalanceAmount: "EUR 0.00",
				CollectionPercent: 100, Status: "paid", StatusLabel: "已缴清",
			},
		},
	}}
}

// The prototype's control band holds the view switch and the filters side by
// side, inside the list panel (figma .seg + .filterbar above the tree table).
// Before the alignment the tabs sat above the panel and between the queue and
// the table, which is why this asserts order rather than mere presence.
func TestRentWorkspaceTabsMoveInsideTheListPanel(t *testing.T) {
	page := renderRentWorkspace(t, rentWorkspacePageData{
		Period:      "2026-09",
		PeriodLabel: "2026年9月",
		View:        rentWorkspaceViewProperties,
	})

	panel := strings.Index(page, "workspace-list-panel")
	controls := strings.Index(page, `class="workspace-list-controls"`)
	tabs := strings.Index(page, `class="workspace-tabs"`)
	filters := strings.Index(page, `class="workspace-filters"`)
	for name, index := range map[string]int{"panel": panel, "controls": controls, "tabs": tabs, "filters": filters} {
		if index < 0 {
			t.Fatalf("rent workspace markup has no %s marker", name)
		}
	}
	if !(panel < controls && controls < tabs && tabs < filters) {
		t.Fatalf("control band order is wrong: panel=%d controls=%d tabs=%d filters=%d", panel, controls, tabs, filters)
	}
	if got := strings.Count(page, `class="workspace-tabs"`); got != 1 {
		t.Fatalf("view switch rendered %d times, want exactly one (inside the list panel)", got)
	}
}

// The chips are the prototype's .dimension-summary: one count per status the
// status filter right beside them offers, with the active view's own noun.
func TestRentWorkspaceDimensionChipsFollowTheActiveView(t *testing.T) {
	cases := []struct {
		view     string
		total    int
		noun     string
		headline string
	}{
		{rentWorkspaceViewProperties, 2, "套房产", "按房产查看"},
		{rentWorkspaceViewRooms, 6, "间房", "按房间查看"},
		{rentWorkspaceViewTenants, 5, "条租客责任", "按租客查看"},
	}
	for _, tc := range cases {
		page := renderRentWorkspace(t, rentWorkspacePageData{
			Period: "2026-09", PeriodLabel: "2026年9月", View: tc.view,
			DimensionSummary: rentWorkspaceDimensionSummary{Total: tc.total, Overdue: 2, Partial: 1, Paid: 1},
		})
		chips := markupBetween(t, page, `class="workspace-dimension-summary"`, `class="panel-head workspace-dimension-head"`)
		for _, expected := range []string{
			`本月共 <strong>` + strconv.Itoa(tc.total) + `</strong> ` + tc.noun,
			`逾期未缴 <strong>2</strong>`,
			`部分缴纳 <strong>1</strong>`,
			`已缴满 <strong>1</strong>`,
		} {
			if !strings.Contains(chips, expected) {
				t.Fatalf("%s chips are missing %q: %s", tc.view, expected, chips)
			}
		}
		if !strings.Contains(page, "<h2>"+tc.headline+"</h2>") {
			t.Fatalf("%s view does not name its own section", tc.view)
		}
	}
}

// The prototype's section note reads "演示数据 · EUR"; 演示数据 is a
// prototype-only label (figma/DESIGN-HANDOFF.md:9) and must not ship. The
// currency half stays because it is the ledger currency the rows are in.
func TestRentWorkspaceSectionNoteKeepsTheCurrencyOnly(t *testing.T) {
	page := renderRentWorkspace(t, rentWorkspacePageData{
		Period: "2026-09", PeriodLabel: "2026年9月", View: rentWorkspaceViewProperties,
	})
	if !strings.Contains(page, `<span class="workspace-currency-note">`+ledgerCurrencyEUR+`</span>`) {
		t.Fatalf("section note does not carry the ledger currency: %s", page)
	}
	for _, forbidden := range []string{"演示数据", "sample-note", "参考号"} {
		if strings.Contains(page, forbidden) {
			t.Fatalf("workspace still renders the prototype-only label %q", forbidden)
		}
	}
}

// The 收缴率 column is a number plus a bar. The bar needs a real CSS width, and
// the width has to be clamped in Go: a stray percent from an upstream sum would
// otherwise escape the track.
func TestWorkspaceRateStyleClampsToTheTrack(t *testing.T) {
	style, ok := rentWorkspaceTemplateFuncs["workspaceRateStyle"].(func(int) template.CSS)
	if !ok {
		t.Fatal("workspaceRateStyle is not registered as a template function")
	}
	for percent, want := range map[int]string{-5: "width:0%", 0: "width:0%", 42: "width:42%", 100: "width:100%", 140: "width:100%"} {
		if got := string(style(percent)); got != want {
			t.Fatalf("workspaceRateStyle(%d)=%q want %q", percent, got, want)
		}
	}
}

// Every list row keeps both actions the prototype has: the plain detail link on
// all rows, and the reused settle form only where money is still owed. The
// action column must exist as a column (its own header cell), not as markup
// tacked onto the last data cell.
func TestRentWorkspaceRowsCarryRateAndActionColumns(t *testing.T) {
	page := renderRentWorkspace(t, rentWorkspacePageData{
		Period: "2026-09", PeriodLabel: "2026年9月", View: rentWorkspaceViewRooms,
		RoomTreeRows: workspaceRoomTreeFixture(),
	})
	desktop := markupBetween(t, page, `class="workspace-desktop-list workspace-tree-list"`, `class="workspace-mobile-list workspace-tree-mobile-list"`)

	if !strings.Contains(desktop, `<span>收缴率</span>`) || !strings.Contains(desktop, `<span>操作</span>`) {
		t.Fatalf("room list is missing the 收缴率 / 操作 columns: %s", desktop)
	}
	if got := strings.Count(desktop, `style="width:42%"`); got != 1 {
		t.Fatalf("room rate bar count=%d want 1 with the row's own percentage: %s", got, desktop)
	}
	if got := strings.Count(desktop, `class="workspace-row-action"`); got != 3 {
		t.Fatalf("row action cells=%d want 3 (one room, two tenant obligations): %s", got, desktop)
	}
	if got := strings.Count(desktop, `>一键平账</summary>`); got != 1 {
		t.Fatalf("settle form count=%d want 1 (only the obligation with an outstanding balance): %s", got, desktop)
	}
	if got := strings.Count(desktop, `>查看详情</a>`); got != 3 {
		t.Fatalf("detail link count=%d want 3 (every row offers one): %s", got, desktop)
	}
	if !strings.Contains(desktop, `href="/rooms/4?period=2026-09"`) || !strings.Contains(desktop, `href="/tenants/11?from_month=2026-09&amp;to_month=2026-09"`) {
		t.Fatalf("detail links do not carry the month the user is looking at: %s", desktop)
	}
}

func TestRentWorkspaceTenantTableKeepsTheActionColumn(t *testing.T) {
	page := renderRentWorkspace(t, rentWorkspacePageData{
		Period: "2026-09", PeriodLabel: "2026年9月", View: rentWorkspaceViewTenants,
		TenantRows: []rentWorkspaceTenantRow{{
			TenantID: 11, PropertyName: "Rosewood Court", RoomLabel: "01", TenantName: "Aoife Murphy",
			ObligationID: 71, Period: "2026-09", ExpectedCents: 125000, PaidCents: 50000, BalanceCents: 75000,
			ExpectedAmount: "EUR 1250.00", PaidAmount: "EUR 500.00", BalanceAmount: "EUR 750.00",
			CollectionPercent: 40, Status: "partial", StatusLabel: "部分缴纳",
		}},
	})
	desktop := markupBetween(t, page, `<div class="workspace-desktop-list">`, `<div class="workspace-mobile-list"`)
	if !strings.Contains(desktop, `<th>收缴率</th>`) || !strings.Contains(desktop, `<th>操作</th>`) {
		t.Fatalf("tenant table is missing the 收缴率 / 操作 headers: %s", desktop)
	}
	if got := strings.Count(desktop, `class="collection-balance-form"`); got != 1 {
		t.Fatalf("tenant settle form count=%d want 1: %s", got, desktop)
	}
	if !strings.Contains(desktop, `style="width:40%"`) {
		t.Fatalf("tenant rate bar does not carry the row percentage: %s", desktop)
	}
}

// The queue expands each independent card into an inline process panel with a
// detail link and a defer action.
func TestRentWorkspacePendingCardsExposeInlineActions(t *testing.T) {
	page := renderRentWorkspace(t, rentWorkspacePageData{
		Period: "2026-09", PeriodLabel: "2026年9月", View: rentWorkspaceViewTenants,
		PendingCount: 2,
		PendingItems: []rentWorkspacePendingItem{
			{Index: 1, Title: "待确认付款人", Subtitle: "09-06 · Rent payment", Amount: "EUR 1200.00", TenantOptions: []billingTenantOption{{ID: 11, Name: "Aoife Murphy"}}, MatchOptions: []billingRentMatchOption{{TenantID: 11, TenantName: "Aoife Murphy", Period: "2026-09", PeriodLabel: "2026年9月", Remaining: "EUR 1200.00"}}, DetailURL: "/transactions/9?period=2026-09", ListURL: "/transactions?period=2026-09&match_status=pending"},
			{Index: 2, Title: "责任人待匹配", Subtitle: "09-07 · Rent payment", Amount: "EUR 700.00", DetailURL: "/transactions/10?period=2026-09", ListURL: "/transactions?period=2026-09&match_status=pending"},
		},
		TenantRows: []rentWorkspaceTenantRow{{
			TenantID: 11, TenantName: "Aoife Murphy", ObligationID: 71, Period: "2026-09",
			BalanceCents: 75000, ExpectedAmount: "EUR 1250.00", BalanceAmount: "EUR 750.00", Status: "partial", StatusLabel: "部分缴纳",
		}},
	})

	queue := markupBetween(t, page, `class="workspace-queue-list"`, `</section>`)
	if got := strings.Count(queue, `>查看流水详情</a>`); got != 2 {
		t.Fatalf("detail link count=%d want one per queued row: %s", got, queue)
	}
	if got := strings.Count(queue, `>处理流水</button>`); got != 2 {
		t.Fatalf("process control count=%d want one per queued row: %s", got, queue)
	}
	if got := strings.Count(queue, `>暂不处理</button>`); got != 2 {
		t.Fatalf("defer action count=%d want one per queued row: %s", got, queue)
	}
	if !strings.Contains(queue, `name="tenant_id" data-searchable`) {
		t.Fatal("dashboard tenant picker does not opt into searchable tenant selection")
	}
	// The prototype numbers each queued row (01/02/03) in front of its body.
	for _, badge := range []string{`class="workspace-queue-index">01<`, `class="workspace-queue-index">02<`} {
		if !strings.Contains(queue, badge) {
			t.Fatalf("queue index badge %q is missing: %s", badge, queue)
		}
	}
	// The see-all link stays in the panel heading above the process cards.
	head := strings.Index(page, `class="workspace-queue-head-meta"`)
	more := strings.Index(page, `class="workspace-queue-more"`)
	list := strings.Index(page, `class="workspace-queue-list"`)
	if !(head >= 0 && head < more && more < list) {
		t.Fatalf("the see-all link is not in the queue panel head: head=%d more=%d list=%d", head, more, list)
	}
}

func TestRentWorkspaceEmptyQueueUsesGreenState(t *testing.T) {
	emptyPage := renderRentWorkspace(t, rentWorkspacePageData{Period: "2026-09", PeriodLabel: "2026年9月", PendingCount: 0})
	if !strings.Contains(emptyPage, `class="panel surface workspace-action-queue is-empty"`) {
		t.Fatal("empty manual-review queue does not render the empty-state modifier")
	}
	if !strings.Contains(emptyPage, "当前没有待处理流水。") {
		t.Fatal("empty manual-review queue lost its empty message")
	}

	activePage := renderRentWorkspace(t, rentWorkspacePageData{Period: "2026-09", PeriodLabel: "2026年9月", PendingCount: 1})
	if strings.Contains(activePage, `workspace-action-queue is-empty`) {
		t.Fatal("non-empty manual-review queue should retain its attention state")
	}

	css := embeddedWebText("web/static/css/pages/rent-workspace.css")
	for _, marker := range []string{`.workspace-action-queue.is-empty`, `.workspace-action-queue.is-empty .workspace-queue-count`, `.workspace-action-queue.is-empty .workspace-queue-empty`, `color: #256444`} {
		if !strings.Contains(css, marker) {
			t.Fatalf("empty queue green treatment is missing %q", marker)
		}
	}
}

// The prototype badges each queued row 01/02/03. The number is assigned where
// the rows are read, so it is asserted there too: the template assertions above
// only prove the badge renders a field, not that the field is numbered.
func TestRentWorkspacePendingItemsAreNumbered(t *testing.T) {
	period := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	when := time.Date(2026, 9, 6, 8, 30, 0, 0, time.UTC)
	payer := "Priya Nair"
	items := rentWorkspacePendingItems([]paymentTransaction{
		{ID: 9, PayerName: &payer, AmountCents: 120000, Currency: "EUR", TransactionTime: &when},
		{ID: 10, AmountCents: 70000, Currency: "EUR"},
	}, period)
	if len(items) != 2 {
		t.Fatalf("pending items=%d want 2", len(items))
	}
	for i, want := range []int{1, 2} {
		if items[i].Index != want {
			t.Fatalf("pending item %d carries index %d, want %d", i, items[i].Index, want)
		}
	}
	if items[0].Title != "Priya Nair" {
		t.Fatalf("payer title=%q, want the transaction's payer", items[0].Title)
	}
	if items[1].Title != "付款人待识别" {
		t.Fatalf("blank payer title=%q, want the placeholder", items[1].Title)
	}
	if !strings.HasPrefix(items[0].Subtitle, "09-06 · ") {
		t.Fatalf("subtitle=%q, want the transaction date prefix", items[0].Subtitle)
	}
}

// Chips are read off the rows the active view has already loaded and filtered,
// so a chip can never disagree with the table underneath it. The buckets reuse
// the status filter's own vocabulary; other statuses count only towards the
// total.
func TestWorkspaceDimensionSummaryCountsTheActiveViewsRows(t *testing.T) {
	properties := []rentWorkspacePropertyRow{{Status: "overdue"}, {Status: "paid"}}
	rooms := []rentWorkspaceRoomRow{{Status: "overdue"}, {Status: "partial"}, {Status: "vacant"}}
	tenants := []rentWorkspaceTenantRow{{Status: "needs_review"}, {Status: "open"}, {Status: "paid"}}

	for _, tc := range []struct {
		view string
		want rentWorkspaceDimensionSummary
	}{
		{rentWorkspaceViewProperties, rentWorkspaceDimensionSummary{Total: 2, Overdue: 1, Paid: 1}},
		{rentWorkspaceViewRooms, rentWorkspaceDimensionSummary{Total: 3, Overdue: 1, Partial: 1}},
		{rentWorkspaceViewTenants, rentWorkspaceDimensionSummary{Total: 3, Paid: 1}},
	} {
		if got := workspaceDimensionSummaryForView(tc.view, properties, rooms, tenants); got != tc.want {
			t.Fatalf("summary for %s=%+v want %+v", tc.view, got, tc.want)
		}
	}
}

// 收缴率 ties are common (two rooms both at 0%), so each view needs a stable
// second key instead of whatever order the query returned. The expected rent is
// the tie-break the alignment added; the view's own column still decides after.
func TestWorkspaceSortBreaksExpectedTiesDeterministically(t *testing.T) {
	filters := rentWorkspaceFilters{PeriodMonth: parseTestPeriod(t, "2026-09"), Status: "all", Sort: "status"}

	// The lower expected rent carries the name that sorts first, so a test that
	// passed by accident on the name tie-break would fail here.
	properties := filterAndSortWorkspaceProperties([]rentWorkspacePropertyRow{
		{PropertyID: 1, Name: "Alpha", Status: "overdue", BalanceCents: 100, ExpectedCents: 100},
		{PropertyID: 2, Name: "Zeta", Status: "overdue", BalanceCents: 100, ExpectedCents: 900},
	}, filters)
	if properties[0].PropertyID != 2 {
		t.Fatalf("properties are not ordered by expected rent: %+v", properties)
	}

	rooms := filterAndSortWorkspaceRooms([]rentWorkspaceRoomRow{
		{RoomID: 1, RoomLabel: "01", Status: "overdue", BalanceCents: 100, ExpectedCents: 100},
		{RoomID: 2, RoomLabel: "09", Status: "overdue", BalanceCents: 100, ExpectedCents: 900},
	}, filters)
	if rooms[0].RoomID != 2 {
		t.Fatalf("rooms are not ordered by expected rent: %+v", rooms)
	}

	tenants := filterAndSortWorkspaceTenants([]rentWorkspaceTenantRow{
		{TenantID: 1, TenantName: "Alpha", Status: "overdue", BalanceCents: 100, ExpectedCents: 100},
		{TenantID: 2, TenantName: "Zeta", Status: "overdue", BalanceCents: 100, ExpectedCents: 900},
	}, filters)
	if tenants[0].TenantID != 2 {
		t.Fatalf("tenants are not ordered by expected rent: %+v", tenants)
	}
}

// The row action reuses the /bills settle form, so the form's hidden fields must
// carry the workspace's own filter state: submitting from the workspace used to
// be impossible to return from, and a wrong month here would settle the wrong
// period.
func TestTenantSettleFormCarriesTheWorkspaceContext(t *testing.T) {
	data := rentWorkspacePageData{
		Period: "2026-09", PeriodLabel: "2026年9月", Page: 3, PageSize: 12,
		Filters: rentWorkspaceFilters{Search: "Rosewood", Status: "overdue", Sort: "balance_desc"},
	}
	row := rentWorkspaceTenantRow{
		ObligationID: 71, TenantID: 11, TenantName: "Aoife Murphy", Period: "2026-09",
		ExpectedCents: 125000, BalanceCents: 75000,
		ExpectedAmount: "EUR 1250.00", BalanceAmount: "EUR 750.00",
	}

	view := data.TenantSettleForm(row)
	if view.ObligationID != 71 || view.OutstandingAmount != "EUR 750.00" {
		t.Fatalf("settle form did not take the obligation and its outstanding amount: %+v", view)
	}
	if !strings.Contains(view.DutyLabel, "Aoife Murphy") || !strings.Contains(view.DutyLabel, "租金责任") {
		t.Fatalf("settle form duty label lost the responsible party or the duty: %q", view.DutyLabel)
	}
	if view.ReturnPeriod != "2026-09" || view.ReturnSearch != "Rosewood" || view.ReturnStatus != "overdue" ||
		view.ReturnSort != "balance_desc" || view.ReturnPage != 3 || view.ReturnPageSize != 12 {
		t.Fatalf("settle form does not return to the workspace filter state: %+v", view)
	}

	// The fallback matters for a row that has an amount but no balance string.
	if fallback := data.TenantSettleForm(rentWorkspaceTenantRow{ObligationID: 9, ExpectedAmount: "EUR 30.00"}); fallback.OutstandingAmount != "EUR 30.00" {
		t.Fatalf("settle form did not fall back to the expected amount: %+v", fallback)
	}

	page := renderRentWorkspace(t, rentWorkspacePageData{
		Period: "2026-09", PeriodLabel: "2026年9月", View: rentWorkspaceViewTenants,
		Page: 3, PageSize: 12,
		Filters:    rentWorkspaceFilters{Search: "Rosewood", Status: "overdue", Sort: "balance_desc"},
		TenantRows: []rentWorkspaceTenantRow{row},
	})
	for _, expected := range []string{
		`action="/rent-dashboard/settle"`,
		`name="return_to"`,
		`name="obligation_id" value="71"`,
		`name="period" value="2026-09"`,
		`name="search" value="Rosewood"`,
		`name="status" value="overdue"`,
		`name="sort" value="balance_desc"`,
		`name="page" value="3"`,
		`name="page_size" value="12"`,
	} {
		if !strings.Contains(page, expected) {
			t.Fatalf("workspace settle form is missing %q", expected)
		}
	}
}

// The aligned lists are grids whose money columns are sized to the app's own
// "EUR 1234.56" format, and the head and the rows share one grid class so the
// two cannot drift apart while the wrapper scrolls. The indent levels are the
// tree's whole hierarchy, and an element-level `padding` shorthand silently
// outranks them, so both are pinned here as well as in the browser audit
// (scripts/audit/dashboard-alignment.mjs).
//
// The grids must NOT carry a `min-width` floor: pinning the whole grid to the
// sum of its column minima ignores the column gaps and the grid's own padding,
// which is what used to push the 1366/1440 property view into a 142-216px
// horizontal scroll. probe-grid-fit.mjs measures the real geometry; this test
// only pins the declaration so a future edit cannot quietly reinstate a floor.
func TestRentWorkspaceListCSSKeepsItsColumnAndIndentContract(t *testing.T) {
	css := embeddedWebText("web/static/css/pages/rent-workspace.css")
	for _, expected := range []string{
		".workspace-tree-property-grid { grid-template-columns: minmax(98px, 1.4fr) minmax(28px, .3fr) repeat(2, minmax(36px, .42fr)) repeat(5, minmax(104px, .8fr)) minmax(44px, .6fr) minmax(68px, .7fr) minmax(72px, .7fr); }",
		".workspace-tree-room-grid { grid-template-columns: minmax(104px, 1.4fr) minmax(50px, .5fr) repeat(3, minmax(104px, .8fr)) minmax(44px, .6fr) minmax(72px, .7fr) minmax(68px, .7fr) minmax(72px, .7fr); }",
		".tree-level-1 { padding-left: 22px; }",
		".tree-level-2 { padding-left: 30px; }",
	} {
		if !strings.Contains(css, expected) {
			t.Fatalf("rent workspace CSS is missing %q", expected)
		}
	}
	// A grid-wide floor on either tree grid puts the scroll back.
	for _, banned := range []string{
		".workspace-tree-property-grid { grid-template-columns: minmax(98px, 1.4fr) minmax(28px, .3fr) repeat(2, minmax(36px, .42fr)) repeat(5, minmax(104px, .8fr)) minmax(44px, .6fr) minmax(68px, .7fr) minmax(72px, .7fr); min-width:",
		".workspace-tree-room-grid { grid-template-columns: minmax(104px, 1.4fr) minmax(50px, .5fr) repeat(3, minmax(104px, .8fr)) minmax(44px, .6fr) minmax(72px, .7fr) minmax(68px, .7fr) minmax(72px, .7fr); min-width:",
	} {
		if strings.Contains(css, banned) {
			t.Fatalf("the tree grid gained a min-width floor, which re-creates the horizontal scroll: %q", banned)
		}
	}
	// The queue's divider/column rules are the prototype's, not a modulo rule.
	if !strings.Contains(css, ".workspace-queue-item:not(:last-child) { border-right:") {
		t.Fatal("the queue item divider no longer matches the prototype's :not(:last-child)")
	}
	if strings.Contains(css, ":not(:nth-child(3n))") {
		t.Fatal("the queue item divider is back to a column-modulo rule, which strands a border on a partial last row")
	}
	if strings.Contains(css, ".workspace-obligation-list { padding:") {
		t.Fatal("the obligation list sets a padding shorthand, which resets the level-2 indent to zero")
	}

	page := embeddedWebText("web/templates/pages/rent-workspace.html")
	for _, expected := range []string{
		`class="workspace-tree-head workspace-tree-property-grid"`,
		`class="workspace-tree-summary workspace-tree-property-grid"`,
		`class="workspace-tree-head workspace-tree-room-grid"`,
		`class="workspace-tree-summary workspace-tree-room-grid"`,
		// The prototype puts a 01/02/03 badge in front of every queued row.
		`class="workspace-queue-index">{{printf "%02d" .Index}}`,
	} {
		if !strings.Contains(page, expected) {
			t.Fatalf("the head and the rows do not share a grid class: %q is missing", expected)
		}
	}
	// The settle form's stylesheet has to travel with the workspace page, or the
	// reused row action renders unstyled.
	if !strings.Contains(page, `/static/css/pages/collection-pages.css`) {
		t.Fatal("the workspace page does not load the stylesheet the reused settle form is written against")
	}
}
