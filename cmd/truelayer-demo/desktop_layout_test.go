package main

import (
	"bytes"
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
	// §1), but the frozen-column mechanism is still mobile-only: the desktop shell
	// breadcrumb is `position: fixed`, never sticky.
	if strings.Contains(strings.Split(workspacePageCSS, "@media (max-width: 640px)")[0], "position: sticky") {
		t.Fatal("the stylesheet above the 640 tier contains a sticky rule; the frozen column is mobile-only")
	}
}

func TestWorkspaceNavExposesDesktopSections(t *testing.T) {
	for _, marker := range []string{
		`<div class="nav-label">工作台</div>`,
		`href="/bills"`,
		`href="/transactions"`,
		`href="/properties"`,
		`href="/bank"`,
	} {
		if !strings.Contains(workspaceNav, marker) {
			t.Fatalf("workspace nav missing %q", marker)
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

	roomPage := roomPageData{
		workspaceShell: workspaceShell{Username: "owner", Environment: "test"},
		ShowForm:       true,
		Form:           roomPageForm{PropertyID: 7, ActiveFrom: "2026-09"},
		Properties:     []propertyPageRow{{ID: 7, Name: "天河一号"}},
	}
	var roomHTML bytes.Buffer
	if err := roomPageTemplate.Execute(&roomHTML, roomPage); err != nil {
		t.Fatalf("render room form: %v", err)
	}
	for _, marker := range []string{`action="/rooms"`, `name="property_id"`, `name="room_label"`, `name="active_from"`, `天河一号`} {
		if !strings.Contains(roomHTML.String(), marker) {
			t.Fatalf("room create form missing %q", marker)
		}
	}

	var editHTML bytes.Buffer
	if err := roomEditPageTemplate.Execute(&editHTML, roomEditPageData{
		workspaceShell: workspaceShell{Username: "owner", Environment: "test"},
		Form:           roomPageForm{ID: 9, PropertyID: 7, RoomLabel: "B-201", ActiveFrom: "2026-09"},
		Properties:     []propertyPageRow{{ID: 7, Name: "天河一号"}},
	}); err != nil {
		t.Fatalf("render room edit form: %v", err)
	}
	for _, marker := range []string{`action="/rooms/9"`, `value="B-201"`, `value="2026-09"`, `selected`} {
		if !strings.Contains(editHTML.String(), marker) {
			t.Fatalf("room edit form missing %q", marker)
		}
	}
}

func TestRoomDetailEditDrawerIncludesPrototypeFields(t *testing.T) {
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
		RoomActiveFrom: "2026-09",
		Editing:        true,
		Form:           roomPageForm{ID: 9, PropertyID: 7, RoomLabel: "03", RoomType: "双人间", Capacity: 2, MonthlyRentValue: "1250.00", DueDay: 1, Notes: "两人同住", ActiveFrom: "2026-09"},
		Properties:     []propertyPageRow{{ID: 7, Name: "78 Old County Road"}},
		Summary:        rentWorkspaceRoomRow{PropertyID: 7},
	})
	if err != nil {
		t.Fatalf("render room detail edit state: %v", err)
	}
	html := page.String()
	previous := -1
	for _, marker := range []string{`name="room_label"`, `name="property_id"`, `name="room_type"`, `name="capacity"`, `name="monthly_rent"`, `name="due_day"`, `name="notes"`} {
		position := strings.Index(html, marker)
		if position < 0 {
			t.Fatalf("room edit drawer is missing prototype field %q", marker)
		}
		if position <= previous {
			t.Fatalf("room edit field %q is out of prototype order", marker)
		}
		previous = position
	}
	if strings.Contains(html, `name="active_from"`) {
		t.Fatal("room edit drawer should not expose the active-from field omitted by the prototype")
	}
	if !strings.Contains(html, `class="room-edit-grid"`) {
		t.Fatal("room edit fields should use the prototype's paired desktop layout")
	}
}

func TestTenantCreateFormCanBindAnExistingRoom(t *testing.T) {
	var page bytes.Buffer
	if err := tenantTemplate.Execute(&page, tenantPageData{
		workspaceShell: workspaceShell{Username: "owner", Environment: "test"},
		ShowForm:       true,
		Form:           tenantRecord{Currency: "EUR", RentStartDate: "2026-09-01"},
		Rooms:          []tenantRoomOption{{ID: 11, PropertyName: "天河一号", RoomLabel: "A-201", MonthlyRentValue: "1200.00", Currency: "EUR"}},
	}); err != nil {
		t.Fatalf("render tenant form: %v", err)
	}
	for _, marker := range []string{`name="room_id"`, `data-room-rent="1200.00"`, `name="structured"`, `name="arrangement_start_month"`, `tenantRoomSelect`} {
		if !strings.Contains(page.String(), marker) {
			t.Fatalf("tenant room binding form missing %q", marker)
		}
	}
}
