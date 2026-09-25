package main

import (
	"regexp"
	"strings"
	"testing"
	"time"
)

// The room detail page shipped its mobile-only fields on the same element as the
// desktop ones (`<dl class="room-facts room-mobile-facts">`). The base
// `.room-mobile-facts { display:none }` and the base `.room-facts { display:grid }`
// are equally specific, so the later rule won and the mobile fields re-rendered
// on PC. The fix moves display:none onto its own wrapper, the shape
// property-detail.html already used. These assertions fail on the old markup.
func TestRoomDetailMobileFactsOwnTheirWrapper(t *testing.T) {
	filters := defaultRentWorkspaceFilters(time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC))
	page, err := executeTemplate(rentRoomDetailTemplate, rentRoomDetailPageData{
		workspaceShell: workspaceShell{ActivePage: "rooms"},
		Filters:        filters,
		Period:         "2026-09",
		PeriodLabel:    "2026年9月",
		RoomID:         2,
		RoomLabel:      "A-01",
		RoomType:       "双人间",
		PropertyName:   "Canal House",
		DueDay:         5,
		Summary:        rentWorkspaceRoomRow{Status: "paid", StatusLabel: "已缴清", TenantCount: 3},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		`<section class="panel surface detail-section room-facts-section room-desktop-facts">`,
		`<section class="panel surface detail-section room-mobile-facts">`,
		// Rent timing belongs to the plan summary, never to the physical room.
		`<dt>当前月租</dt>`,
		`<dt>在住人数</dt>`,
		`<dt>缴租日</dt>`,
	} {
		if !strings.Contains(page, expected) {
			t.Fatalf("room detail is missing %q", expected)
		}
	}
	if strings.Contains(page, `class="room-facts room-mobile-facts"`) || strings.Contains(page, `class="room-mobile-facts room-facts"`) {
		t.Fatal("mobile-only facts are still welded onto the desktop .room-facts element, so display:none cannot win")
	}
	for _, removed := range []string{"房间类型", "可住人数", "双人间"} {
		if strings.Contains(page, removed) {
			t.Fatalf("room detail facts should not expose removed field %q", removed)
		}
	}
	for _, retained := range []string{`<dt>在住人数</dt><dd>3 位</dd>`, `<dt>当前租客</dt><dd>3 位</dd>`} {
		if !strings.Contains(page, retained) {
			t.Fatalf("room detail should retain occupant count %q", retained)
		}
	}

	pageCSS := embeddedWebText("web/static/css/pages/room-detail.css")
	// The base tier hides the mobile wrapper...
	if !strings.Contains(pageCSS, ".room-responsibility-mobile-list,.room-mobile-facts,.detail-back-mobile,.detail-mobile-edit { display:none; }") {
		t.Fatal("base tier no longer hides .room-mobile-facts")
	}
	// ...and the ≤640 tier swaps the two wrappers, not two facts on one wrapper.
	if !strings.Contains(pageCSS, ".room-desktop-facts{display:none}") || !strings.Contains(pageCSS, ".room-mobile-facts{display:block}") {
		t.Fatal("≤640 tier does not swap the desktop and mobile fact wrappers")
	}
	if strings.Contains(pageCSS, "room-facts.room-mobile-facts") || strings.Contains(pageCSS, "room-mobile-facts.room-facts") {
		t.Fatal("a compound .room-facts.room-mobile-facts selector is back; the specificity conflict returns with it")
	}
}

// The 代付分配详情 panel needs a fixed 未分配 cell that is correct with and
// without unallocated money. 未分配 (received but unowned) is not 未覆盖
// (responsibility not yet covered) and the two must render different numbers.
func TestRoomDetailUnallocatedCellRendersBothStates(t *testing.T) {
	render := func(t *testing.T, unallocated string, balance string) string {
		t.Helper()
		filters := defaultRentWorkspaceFilters(time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC))
		page, err := executeTemplate(rentRoomDetailTemplate, rentRoomDetailPageData{
			workspaceShell:    workspaceShell{ActivePage: "rooms"},
			Filters:           filters,
			Period:            "2026-09",
			PeriodLabel:       "2026年9月",
			RoomID:            2,
			RoomLabel:         "A-01",
			PropertyName:      "Canal House",
			UnallocatedAmount: unallocated,
			Summary:           rentWorkspaceRoomRow{Status: "partial", StatusLabel: "部分缴纳", PaidAmount: "€640.00", BalanceAmount: balance},
		})
		if err != nil {
			t.Fatal(err)
		}
		cell := `<article><span>未分配</span><strong>` + unallocated + `</strong><small>收款尚未分配给租客</small></article>`
		if !strings.Contains(page, cell) {
			t.Fatalf("room detail 代付分配详情 has no 未分配 cell for value %q", unallocated)
		}
		return page
	}

	// Money received into the room that no responsibility claimed.
	withBalance := render(t, "€120.00", "€640.00")
	// Fully allocated: the cell must read a real zero, not be dropped.
	withoutBalance := render(t, "€0.00", "€640.00")

	// 未分配 and 未覆盖 are different quantities; a fix that substituted one for
	// the other would collapse these two values.
	if !strings.Contains(withBalance, `<article><span>未收金额</span><strong>€640.00</strong>`) {
		t.Fatal("未覆盖责任 cell no longer renders the responsibility balance")
	}
	if !strings.Contains(withoutBalance, `<strong>€0.00</strong>`) {
		t.Fatal("未分配 cell disappeared when there is nothing unallocated")
	}
}

// The property detail page's main stack gains the prototype's 本月收支构成 bridge:
// 已收租金 + 已确认其他收入 − 有效支出.
func TestPropertyDetailRendersFinancialBridge(t *testing.T) {
	var body strings.Builder
	if err := propertyDetailPageTemplate.Execute(&body, propertyDetailPageData{
		workspaceShell: workspaceShell{ActivePage: "properties"},
		Period:         "2026-09",
		Property: propertyPageRow{
			ID: 1, Name: "Canal House", Status: "active", StatusLabel: "有效",
			PaidAmount: "€17,840.00", OtherIncomeAmount: "€120.00", ExpenseAmount: "€1,240.00", NetAmount: "€16,720.00",
		},
	}); err != nil {
		t.Fatal(err)
	}
	page := body.String()
	for _, expected := range []string{
		"本月收支构成",
		`<div class="financial-bridge">`,
		"已收租金", "€17,840.00",
		"已确认其他收入", "€120.00",
		"有效支出", "€1,240.00",
		// The bridge is additive: the pre-existing expense list stays reachable.
		"关联支出",
	} {
		if !strings.Contains(page, expected) {
			t.Fatalf("property detail 本月收支构成 missing %q", expected)
		}
	}
}

// The prototype's room-detail 租客责任 table lets the 责任人 jump to that
// tenant's detail page. The row carried TenantID all along but rendered it as
// plain text, so the affordance was missing until the check pass added it.
// Pinned on markup so a later restyle cannot drop the link again.
func TestRoomDetailResponsiblePartyLinksToTheTenant(t *testing.T) {
	filters := defaultRentWorkspaceFilters(time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC))
	page, err := executeTemplate(rentRoomDetailTemplate, rentRoomDetailPageData{
		workspaceShell: workspaceShell{ActivePage: "rooms"},
		Filters:        filters,
		Period:         "2026-09",
		PeriodLabel:    "2026年9月",
		RoomID:         2,
		RoomLabel:      "A-01",
		PropertyName:   "Canal House",
		Summary:        rentWorkspaceRoomRow{Status: "paid", StatusLabel: "已缴清"},
		Tenants: []rentWorkspaceTenantRow{{
			TenantID: 11, TenantName: "WAHAJULLAH KHAN",
			ExpectedAmount: "€625.00", PaidAmount: "€625.00", BalanceAmount: "€0.00",
			Status: "paid", StatusLabel: "已缴清",
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	// Both the desktop table and the narrow-screen row must link. A row with no
	// TenantID still has to render its name rather than an empty href.
	if !strings.Contains(page, `<a href="/tenants/11"><strong>WAHAJULLAH KHAN</strong></a>`) {
		t.Fatalf("room detail 责任人 is not linked to the tenant page: %s", page)
	}

	orphan, err := executeTemplate(rentRoomDetailTemplate, rentRoomDetailPageData{
		workspaceShell: workspaceShell{ActivePage: "rooms"},
		Filters:        filters,
		Period:         "2026-09", PeriodLabel: "2026年9月",
		RoomID: 2, RoomLabel: "A-01", PropertyName: "Canal House",
		Summary: rentWorkspaceRoomRow{Status: "paid", StatusLabel: "已缴清"},
		Tenants: []rentWorkspaceTenantRow{{TenantName: "无 ID 的责任人", Status: "paid", StatusLabel: "已缴清"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(orphan, `<strong>无 ID 的责任人</strong>`) {
		t.Fatal("a responsibility row without a TenantID lost its name")
	}
	if strings.Contains(orphan, `href="/tenants/0"`) {
		t.Fatal("a responsibility row without a TenantID rendered an empty tenant link")
	}
}

// The ≤640 tier expressed "hide the expense list" as
// `.property-detail-stack > section:nth-child(2)`. That class sits on BOTH the
// main column and the aside, so the rule also swallowed the aside's second
// section -- which is `property-mobile-facts`, the panel that exists only for
// narrow screens. Between that rule (0,2,0) and the base tier's
// `.property-mobile-facts { display:none }` (0,1,0) the card was unreachable at
// every width. Asserted on the stylesheet because no Go test can evaluate CSS
// specificity.
func TestPropertyDetailHidesTheExpenseListByClassNotByChildIndex(t *testing.T) {
	pageCSS := embeddedWebText("web/static/css/pages/property-detail.css")
	if strings.Contains(pageCSS, ".property-detail-stack > section:nth-child(2)") {
		t.Fatal("the child-index rule is back; it also hides the aside's .property-mobile-facts card")
	}
	if !strings.Contains(pageCSS, ".property-expense-list { display:none; }") {
		t.Fatal("the narrow tier no longer hides the expense list")
	}
	// The mobile card must be told to show in the tier that also hides the
	// desktop card, and the two must name different elements.
	if !strings.Contains(pageCSS, ".property-mobile-facts, .property-mobile-recent { display:block; }") {
		t.Fatal("the narrow tier no longer reveals .property-mobile-facts")
	}
	if !strings.Contains(pageCSS, ".property-desktop-facts") {
		t.Fatal("the narrow tier no longer hides the desktop facts card")
	}
}

// 09-19-pc-ui-fidelity-alignment's 行内操作 sweep renamed the list pages' row
// action to 查看详情, and the sweep missed one page: 房产详情 embeds a 房间收款概览
// table with its own 操作 column, still reading 详情 on both its desktop table and
// its mobile card. Pinned on markup because no other test renders this table's
// action column.
func TestPropertyDetailRoomActionColumnSaysViewDetails(t *testing.T) {
	var body strings.Builder
	if err := propertyDetailPageTemplate.Execute(&body, propertyDetailPageData{
		workspaceShell: workspaceShell{ActivePage: "properties"},
		Period:         "2026-09",
		Property:       propertyPageRow{ID: 1, Name: "Canal House", Status: "active", StatusLabel: "有效"},
		Rooms: []roomPageRow{{
			ID: 11, PropertyName: "Canal House", RoomLabel: "A-01", Status: "active", StatusLabel: "在租",
			TenantNames: []string{"WAHAJULLAH KHAN"},
			MonthlyRent: "€1,250.00", ExpectedAmount: "€1,250.00", PaidAmount: "€0.00", BalanceAmount: "€1,250.00",
		}},
	}); err != nil {
		t.Fatal(err)
	}
	page := body.String()
	for _, marker := range []string{
		`>操作</th>`,
		`href="/rooms/11?period=2026-09">查看详情</a>`,
	} {
		if !strings.Contains(page, marker) {
			t.Fatalf("property detail room table missing %q", marker)
		}
	}
	// The desktop table and the mobile card carry the same action pair, so a
	// correct render has 查看详情 twice and a bare 详情 zero times.
	if got := strings.Count(page, `>查看详情</a>`); got != 2 {
		t.Fatalf("property detail room actions: got %d 查看详情, want 2 (desktop table + mobile card)", got)
	}
	if strings.Contains(page, `>详情</a>`) {
		t.Fatal(`a property detail room action still reads 详情; it must match the list pages`)
	}
}

func TestObjectDetailsExposeConfirmedDeleteActions(t *testing.T) {
	// 房产和房间的删除按钮搬进了编辑抽屉：平时浏览（抽屉关着）看不到，
	// 打开「编辑资料」才出现。所以两边各渲染两次——关着断言没有，开着断言有。
	propertyClosed, err := executeTemplate(propertyDetailPageTemplate, propertyDetailPageData{
		workspaceShell: workspaceShell{ActivePage: "properties"}, Period: "2026-09",
		Property: propertyPageRow{ID: 12, Name: "Canal House", Status: "active"},
	})
	if err != nil {
		t.Fatal(err)
	}
	propertyPage, err := executeTemplate(propertyDetailPageTemplate, propertyDetailPageData{
		workspaceShell: workspaceShell{ActivePage: "properties"}, Period: "2026-09",
		Property: propertyPageRow{ID: 12, Name: "Canal House", Status: "active"}, Editing: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	roomClosed, err := executeTemplate(rentRoomDetailTemplate, rentRoomDetailPageData{
		workspaceShell: workspaceShell{ActivePage: "rooms"}, Period: "2026-09", RoomID: 8,
		RoomLabel: "A-01", PropertyName: "Canal House", Summary: rentWorkspaceRoomRow{Status: "active"},
	})
	if err != nil {
		t.Fatal(err)
	}
	roomPage, err := executeTemplate(rentRoomDetailTemplate, rentRoomDetailPageData{
		workspaceShell: workspaceShell{ActivePage: "rooms"}, Period: "2026-09", RoomID: 8,
		RoomLabel: "A-01", PropertyName: "Canal House", Summary: rentWorkspaceRoomRow{Status: "active"}, Editing: true,
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, closed := range []struct{ name, page string }{
		{"property detail", propertyClosed},
		{"room detail", roomClosed},
	} {
		if strings.Contains(closed.page, `name="action" value="delete"`) {
			t.Errorf("%s offers a delete action while the edit drawer is closed", closed.name)
		}
	}
	tenantPage, err := executeTemplate(tenantDetailTemplate, tenantDetailPageData{
		workspaceShell: workspaceShell{ActivePage: "tenants"}, Tenant: tenantRecord{ID: "9", Name: "Aoife Murphy", Status: "active"},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		page, action, label string
	}{
		{propertyPage, `action="/properties/12"`, "隐藏房产"},
		{roomPage, `action="/rooms/8"`, "隐藏房间"},
		{tenantPage, `action="/tenants/9"`, "删除租客"},
	} {
		if !strings.Contains(test.page, test.action) || !strings.Contains(test.page, `name="action" value="delete"`) || !strings.Contains(test.page, `data-confirm="true"`) || !strings.Contains(test.page, test.label) {
			t.Errorf("detail page is missing confirmed delete action %q", test.label)
		}
	}
}

func TestTenantPickersOptIntoSharedFuzzySearch(t *testing.T) {
	for _, test := range []struct {
		path    string
		markers []string
	}{
		{
			path: "web/templates/pages/room-detail.html",
			markers: []string{
				`<select name="tenant_id" data-searchable required>`,
				`<select name="tenant_id" data-searchable>`,
			},
		},
		{
			path: "web/templates/partials/cash-receipt-drawer.html",
			markers: []string{
				`name="tenant_id" data-searchable`,
			},
		},
		{
			path: "web/static/js/workspace-controls.js",
			markers: []string{
				`labelText.toLocaleLowerCase().includes(query)`,
				// 过滤的落点：不匹配的项靠 hidden 藏起来。
				`item.hidden = !matches`,
			},
		},
	} {
		contents := embeddedWebText(test.path)
		for _, marker := range test.markers {
			if !strings.Contains(contents, marker) {
				t.Fatalf("tenant picker asset %s is missing %q", test.path, marker)
			}
		}
	}
}

// 模糊搜索是两半：JS 给不匹配的项设 hidden，CSS 负责把它藏住。JS 那半单独在
// 上面钉住了，CSS 这半要单独钉——因为 .workspace-select-option 上的 display:flex
// 是作者样式，会压过浏览器默认的 [hidden]{display:none}。少了这条兜底，搜索框
// 照样收字、列表一个不少，从外面看就是"模糊搜索没生效"，而上面的 JS 标记全都还在，
// 光靠那一半断言发现不了。
func TestFuzzySearchFilteredOptionsAreActuallyHidden(t *testing.T) {
	css := embeddedWebText("web/static/css/workspace-controls.css")

	// 只取 .workspace-select-option[hidden] 那一条规则本身，别让文件里其他
	// [hidden] 兜底（-clear/-popup/-empty）替我证明。
	start := strings.Index(css, ".workspace-select-option[hidden]")
	if start < 0 {
		t.Fatal("选项没有 [hidden] 兜底：display:flex 会压过浏览器的 [hidden]{display:none}，模糊搜索过滤完列表不会变")
	}
	open := strings.Index(css[start:], "{")
	end := strings.Index(css[start:], "}")
	if open < 0 || end < open {
		t.Fatalf("选项的 [hidden] 规则不是一条完整规则：%q", css[start:])
	}
	rule := css[start+open : start+end]
	if !regexp.MustCompile(`display:\s*none`).MatchString(rule) {
		t.Fatalf("选项的 [hidden] 规则没有把它藏起来：%q", rule)
	}
}

// 房产详情页的「未收」卡是窄屏专用的（`.property-mobile-unpaid` 默认 display:none，
// ≤640px 才顶掉第 3 张卡露出来），所以它在桌面上根本看不见——欠着钱也是灰的这件事
// 只有拿手机看才发现。房间详情页的同名卡（room-detail.html 的「未付」）早就按余额
// 上了 metric-warning，房产页这张一直是裸的 .metric。
//
// workspace.css 里那三个语义修饰类的注释写得很清楚：「调用方是按状态条件加类的」，
// 也就是说类出现即代表状态成立。房产页就是当时漏掉的那个调用方。
func TestPropertyDetailUnpaidCardWarnsOnlyWhenMoneyIsOwed(t *testing.T) {
	render := func(balanceCents int64) string {
		t.Helper()
		var body strings.Builder
		if err := propertyDetailPageTemplate.Execute(&body, propertyDetailPageData{
			workspaceShell: workspaceShell{ActivePage: "properties"},
			Period:         "2026-09",
			Property: propertyPageRow{
				ID: 1, Name: "Canal House", Status: "active", StatusLabel: "有效",
				BalanceCents: balanceCents, BalanceAmount: "€1,250.00",
			},
		}); err != nil {
			t.Fatal(err)
		}
		return body.String()
	}

	// 欠着钱：这张卡得带上警示色，且类必须和 property-mobile-unpaid 挂在同一个
	// 元素上——挂到隔壁那张卡上，着色就落在一个窄屏根本不显示的元素里了。
	owed := render(125000)
	if !strings.Contains(owed, `class="panel metric property-mobile-unpaid metric-warning"`) {
		t.Fatal("未收卡欠着钱却没有看色：欠 1250 和欠 0 元长得一样")
	}
	// 没欠钱（或已结清）：不该有警示色。0 元的红会被读成"出事了"。
	clear := render(0)
	if strings.Contains(clear, "metric-warning") {
		t.Fatal("未收为 0 的卡不该报红")
	}
	if !strings.Contains(clear, `class="panel metric property-mobile-unpaid"`) {
		t.Fatal("未收卡本身不见了：着色条件写错会连卡片一起改没")
	}

	// 类只是开关，颜色得真有人给。这一段钉住 workspace.css 那侧还在定义它，
	// 否则测试绿着、屏幕上仍然是灰的。
	warning := regexp.MustCompile(`\.metric\.metric-warning\s*\{[^}]*color-mix\(in oklch, ?var\(--danger\)`).FindString(embeddedWebText("web/static/css/workspace.css"))
	if warning == "" {
		t.Fatal("workspace.css 不再给 metric-warning 上警示底色，模板加了类也看不出差别")
	}
	if !strings.Contains(embeddedWebText("web/static/css/workspace.css"), ".metric.metric-warning strong { color: var(--danger); }") {
		t.Fatal("metric-warning 的金额不再是警示色")
	}
}
