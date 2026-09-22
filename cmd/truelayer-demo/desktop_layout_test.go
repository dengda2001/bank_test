package main

import (
	"bytes"
	"fmt"
	"strings"
	"testing"
)

func TestDesktopWorkspaceShellUsesFigmaTokens(t *testing.T) {
	for _, marker := range []string{
		"--sidebar: oklch(19% .018 240)",
		"--accent: oklch(58% .16 145)",
		"grid-template-columns: 236px minmax(0, 1fr)",
		".topbar {",
		".workspace-toolbar {",
		"background: var(--sidebar)",
	} {
		if !strings.Contains(workspacePageCSS, marker) {
			t.Fatalf("desktop shell missing Figma marker %q", marker)
		}
	}
	// The desktop head may be aligned to the prototype (responsive-conventions.md
	// §1), but the frozen-column mechanism is still mobile-only. This assertion
	// used to read "no sticky above 640 at all"; it was retargeted when the
	// desktop sidebar became sticky on purpose — prd.md「用户报告的滚动缺陷」
	// requires the prototype's `.sidebar{position:sticky;top:0;height:100vh}` so
	// the rail no longer scrolls away with the page. What must stay mobile-only
	// is the sticky *table cell* (right:0), not sticky positioning as such.
	aboveMobile := strings.Split(workspacePageCSS, "@media (max-width: 640px)")[0]
	if !strings.Contains(aboveMobile, "position: sticky;\n        top: 0;") {
		t.Fatal("the desktop sidebar must be sticky at top:0 so it stays pinned while the page scrolls")
	}
	if strings.Contains(aboveMobile, "position: sticky;\n        right: 0;") {
		t.Fatal("a table cell freezes above the 640 tier; the frozen column is mobile-only")
	}
}

func TestWorkspaceNavExposesDesktopSections(t *testing.T) {
	// The single "工作台" label became the prototype's three groups
	// (figma/rentops-desktop-suite.html: the 收租决策 / 资产与关系 / 资金与系统
	// headings), and each group owns its own <nav aria-label> so the sections are
	// navigable landmarks rather than decoration.
	for _, marker := range []string{
		`<div class="nav-label">收租决策</div>`,
		`<nav class="nav" aria-label="收租决策">`,
		`<div class="nav-label">资产与关系</div>`,
		`<nav class="nav" aria-label="资产与关系">`,
		`<div class="nav-label">系统</div>`,
		`<nav class="nav" aria-label="资金与系统">`,
		`href="/rent-dashboard"`,
		`href="/transactions"`,
		`href="/dunning"`,
		`href="/properties"`,
		`href="/bank"`,
	} {
		if !strings.Contains(workspaceNav, marker) {
			t.Fatalf("workspace nav missing %q", marker)
		}
	}
	for _, retired := range []string{`href="/bills"`, `href="/tenancies"`, "应收账单", "租约管理"} {
		if strings.Contains(workspaceNav, retired) {
			t.Fatalf("workspace nav still exposes retired module %q", retired)
		}
	}
	// The simplified navigation numbers its seven remaining destinations 01..07.
	for index := 1; index <= 7; index++ {
		marker := `<span class="nav-icon">` + fmt.Sprintf("%02d", index) + `</span>`
		if !strings.Contains(workspaceNav, marker) {
			t.Fatalf("workspace nav missing the prototype's two-digit icon %q", marker)
		}
	}
}

func TestDesktopAssetFormsExposeCreateAndEditControls(t *testing.T) {
	propertyPage := propertyPageData{
		workspaceShell: workspaceShell{Username: "owner", Environment: "test"},
		ShowForm:       true,
		Form:           propertyPageForm{},
	}
	var propertyHTML bytes.Buffer
	if err := propertyPageTemplate.Execute(&propertyHTML, propertyPage); err != nil {
		t.Fatalf("render property form: %v", err)
	}
	for _, marker := range []string{`action="/properties"`, `name="name"`, `name="address"`, `保存房产`} {
		if !strings.Contains(propertyHTML.String(), marker) {
			t.Fatalf("property create form missing %q", marker)
		}
	}
	for _, removed := range []string{`name="active_from"`, `name="inactive_from"`} {
		if strings.Contains(propertyHTML.String(), removed) {
			t.Fatalf("property form exposes asset validity field %q", removed)
		}
	}

	roomPage := roomPageData{
		workspaceShell: workspaceShell{Username: "owner", Environment: "test"},
		ShowForm:       true,
		Form:           roomPageForm{PropertyID: 7},
		Properties:     []propertyPageRow{{ID: 7, Name: "天河一号"}},
	}
	var roomHTML bytes.Buffer
	if err := roomPageTemplate.Execute(&roomHTML, roomPage); err != nil {
		t.Fatalf("render room form: %v", err)
	}
	for _, marker := range []string{`action="/rooms"`, `name="property_id"`, `name="room_label"`, `天河一号`} {
		if !strings.Contains(roomHTML.String(), marker) {
			t.Fatalf("room create form missing %q", marker)
		}
	}
	for _, removed := range []string{`name="active_from"`, `name="inactive_from"`} {
		if strings.Contains(roomHTML.String(), removed) {
			t.Fatalf("room create form exposes asset validity or rent-plan field %q", removed)
		}
	}

	var editHTML bytes.Buffer
	if err := roomEditPageTemplate.Execute(&editHTML, roomEditPageData{
		workspaceShell: workspaceShell{Username: "owner", Environment: "test"},
		Form:           roomPageForm{ID: 9, PropertyID: 7, RoomLabel: "B-201"},
		Properties:     []propertyPageRow{{ID: 7, Name: "天河一号"}},
	}); err != nil {
		t.Fatalf("render room edit form: %v", err)
	}
	for _, marker := range []string{`action="/rooms/9"`, `value="B-201"`, `selected`} {
		if !strings.Contains(editHTML.String(), marker) {
			t.Fatalf("room edit form missing %q", marker)
		}
	}
}

func TestRoomDetailEditDrawerOmitsRoomTypeAndCapacity(t *testing.T) {
	var page bytes.Buffer
	err := rentRoomDetailTemplate.Execute(&page, rentRoomDetailPageData{
		workspaceShell: workspaceShell{ActivePage: "rooms", Username: "owner", Environment: "test"},
		Period:         "2026-09",
		PeriodLabel:    "2026 年 9 月",
		RoomID:         9,
		RoomLabel:      "03",
		RoomType:       "双人间",
		Capacity:       2,
		MonthlyRent:    "€1,250",
		DueDay:         1,

		Editing:    true,
		Form:       roomPageForm{ID: 9, PropertyID: 7, RoomLabel: "03", RoomType: "双人间", Capacity: 2, Notes: "两人同住"},
		Properties: []propertyPageRow{{ID: 7, Name: "78 Old County Road"}},
		Summary:    rentWorkspaceRoomRow{PropertyID: 7, TenantCount: 3},
	})
	if err != nil {
		t.Fatalf("render room detail edit state: %v", err)
	}
	html := page.String()
	previous := -1
	for _, marker := range []string{`name="room_label"`, `name="property_id"`, `name="notes"`} {
		position := strings.Index(html, marker)
		if position < 0 {
			t.Fatalf("room edit drawer is missing prototype field %q", marker)
		}
		if position <= previous {
			t.Fatalf("room edit field %q is out of prototype order", marker)
		}
		previous = position
	}
	for _, removed := range []string{`name="room_type"`, `name="capacity"`, "房间类型", "可住人数", "双人间"} {
		if strings.Contains(html, removed) {
			t.Fatalf("room detail should not expose removed field %q", removed)
		}
	}
	if !strings.Contains(html, `<dt>在住人数</dt><dd>3 位</dd>`) {
		t.Fatal("room detail must retain the actual current occupant count")
	}
	for _, removed := range []string{`name="active_from"`, `name="monthly_rent"`, `name="due_day"`} {
		if strings.Contains(html, removed) {
			t.Fatalf("physical room edit should not expose rent-plan field %q", removed)
		}
	}
	if !strings.Contains(html, `class="room-edit-grid"`) {
		t.Fatal("room edit fields should use the prototype's paired desktop layout")
	}
}

func TestTenantCreateFormContainsIdentityPayerAndOptionalRoomPlanFields(t *testing.T) {
	var page bytes.Buffer
	if err := tenantTemplate.Execute(&page, tenantPageData{
		workspaceShell: workspaceShell{Username: "owner", Environment: "test"},
		ShowForm:       true,
		Form:           tenantRecord{Status: "active"},
	}); err != nil {
		t.Fatalf("render tenant form: %v", err)
	}
	for _, marker := range []string{`name="name"`, `name="email"`, `name="payer_id"`, `name="payer_name_hint"`} {
		if !strings.Contains(page.String(), marker) {
			t.Fatalf("tenant identity form missing %q", marker)
		}
	}
	for _, expected := range []string{`name="property_id"`, `name="room_id"`, `name="arrangement_start_month"`, `name="room_plan"`, `name="new_responsibility"`, "留空则按照入住人数均分房间租金"} {
		if !strings.Contains(page.String(), expected) {
			t.Fatalf("tenant create form missing optional room-plan field %q", expected)
		}
	}
	for _, removed := range []string{`name="monthly_rent"`, `name="due_day"`, `name="active_from"`} {
		if strings.Contains(page.String(), removed) {
			t.Fatalf("tenant create form must not contain room-owned field %q", removed)
		}
	}
}
