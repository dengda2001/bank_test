package main

import (
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
		Summary:        rentWorkspaceRoomRow{Status: "paid", StatusLabel: "已缴清"},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		`<section class="panel surface detail-section room-facts-section room-desktop-facts">`,
		`<section class="panel surface detail-section room-mobile-facts">`,
		// The desktop panel keeps all ten facts; the mobile panel keeps only the
		// narrow-screen subset.
		`<dt>房间生效月份</dt>`,
		`<dt>在住人数</dt>`,
		`<dt>账单日</dt>`,
	} {
		if !strings.Contains(page, expected) {
			t.Fatalf("room detail is missing %q", expected)
		}
	}
	if strings.Contains(page, `class="room-facts room-mobile-facts"`) || strings.Contains(page, `class="room-mobile-facts room-facts"`) {
		t.Fatal("mobile-only facts are still welded onto the desktop .room-facts element, so display:none cannot win")
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
		cell := `<article><span>未分配</span><strong>` + unallocated + `</strong><small>收款中尚未归属责任</small></article>`
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
	if !strings.Contains(withBalance, `<article><span>未覆盖责任</span><strong>€640.00</strong>`) {
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
