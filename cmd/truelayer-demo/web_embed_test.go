package main

import (
	"io/fs"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestEmbeddedWorkspaceResourcesArePresentAndReferenceSameOrigin(t *testing.T) {
	for _, path := range []string{
		"web/templates/partials/workspace-nav.html",
		"web/templates/pages/rent-workspace.html",
		"web/templates/pages/room-detail.html",
		"web/templates/pages/property-detail.html",
		"web/templates/pages/properties.html",
		"web/templates/pages/rooms.html",
		"web/static/css/workspace.css",
		"web/static/css/calendar.css",
		"web/static/css/workspace-controls.css",
		"web/static/css/pages/rent-workspace.css",
		"web/static/css/pages/room-detail.css",
		"web/static/css/pages/property-detail.css",
		"web/static/css/pages/object-navigation.css",
		"web/static/css/pages/entity-drawers.css",
		"web/static/css/pages/object-lists.css",
		"web/static/js/calendar.js",
		"web/static/js/workspace-controls.js",
	} {
		if contents := embeddedWebText(path); strings.TrimSpace(contents) == "" {
			t.Fatalf("embedded resource %q is empty", path)
		}
	}

	page := withWorkspaceControlAssets(embeddedWebText("web/templates/pages/rent-workspace.html"))
	for _, href := range []string{
		`/static/css/workspace.css`,
		`/static/css/calendar.css`,
		`/static/css/workspace-controls.css`,
		`/static/js/calendar.js`,
		`/static/js/workspace-controls.js`,
		`/static/css/pages/rent-workspace.css`,
	} {
		if !strings.Contains(page, href) {
			t.Fatalf("rent workspace template does not reference %q", href)
		}
	}
	if strings.Contains(page, "参考号") || strings.Contains(page, "rent-2026-09") {
		t.Fatal("rent workspace template must not display payment reference numbers")
	}
}

func TestEmbeddedObjectListsKeepFiltersAndResponsiveDetailLinks(t *testing.T) {
	propertyPage, err := executeTemplate(propertyPageTemplate, propertyPageData{
		workspaceShell: workspaceShell{ActivePage: "properties"},
		Period:         "2026-09", PeriodLabel: "2026年9月", PeriodOptions: []pagePeriodOption{{Value: "2026-09", Label: "2026 年 9 月"}}, StatusFilter: "all", CollectionFilter: "unpaid", Search: "Canal", Sort: "expected_desc",
		Rows: []propertyPageRow{{ID: 4, Name: "Canal House", Address: "1 Main Street", ResponsibilityCount: 2, RoomCount: 3, ExpectedAmount: "EUR 1,000.00", PaidAmount: "EUR 800.00", Status: "active", StatusLabel: "有效", CollectionStatus: "partial", CollectionStatusLabel: "部分未收"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{`name="search"`, `name="status"`, `name="collection"`, `name="period"`, `aria-expanded="false"`, `data-property-id="4"`, `class="object-mobile-list"`, `class="object-mobile-money"`, `/properties/4?period=2026-09`, `list_sort=expected_desc`, `部分未收`, "ResponsibilityCount"} {
		if expected == "ResponsibilityCount" {
			if !strings.Contains(propertyPage, ">2</td>") {
				t.Fatal("property list does not render its responsibility count")
			}
			continue
		}
		if !strings.Contains(propertyPage, expected) {
			t.Fatalf("property list is missing %q", expected)
		}
	}

	roomPage, err := executeTemplate(roomPageTemplate, roomPageData{
		workspaceShell: workspaceShell{ActivePage: "rooms"},
		Period:         "2026-09", PeriodLabel: "2026年9月", PeriodOptions: []pagePeriodOption{{Value: "2026-09", Label: "2026 年 9 月"}}, StatusFilter: "active", CollectionFilter: "paid", Search: "A-01", Sort: "count_desc", PropertyID: 2,
		Rows: []roomPageRow{{ID: 9, PropertyID: 2, PropertyName: "Canal House", RoomLabel: "A-01", TenantNames: []string{"Aoife Murphy"}, ExpectedAmount: "EUR 1,000.00", PaidAmount: "EUR 800.00", BalanceAmount: "EUR 200.00", Status: "active", StatusLabel: "有效", CollectionStatus: "partial", CollectionStatusLabel: "部分未收"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{`name="search"`, `name="property_id"`, `name="status"`, `name="collection"`, `name="period"`, `data-room-id="9"`, `class="object-mobile-list"`, `class="object-mobile-money"`, `/rooms/9?period=2026-09&amp;from=rooms`, `return_collection=paid`, `return_sort=count_desc`, "Aoife Murphy"} {
		if !strings.Contains(roomPage, expected) {
			t.Fatalf("room list is missing %q", expected)
		}
	}
}

func TestObjectListCollectionFilterKeepsSelectedLabelReadable(t *testing.T) {
	css := embeddedWebText("web/static/css/pages/object-lists.css")
	for _, rule := range []string{
		`.object-list-filter-fields select[name="collection"] { width: 132px; min-width: 132px; }`,
		`.object-list-filter-fields label { min-width: 0; }`,
	} {
		if !strings.Contains(css, rule) {
			t.Fatalf("object-list collection filter is missing readable-width rule %q", rule)
		}
	}
}

func TestPropertyDetailEditDrawerMatchesPrototypeFieldLayout(t *testing.T) {
	page, err := executeTemplate(propertyDetailPageTemplate, propertyDetailPageData{
		workspaceShell: workspaceShell{ActivePage: "properties"},
		Period:         "2026-09", PeriodLabel: "2026年9月", Editing: true,
		Property: propertyPageRow{ID: 12, Name: "Canal House", CityRegion: "Dublin 2", Address: "1 Main Street", Timezone: "Europe/Dublin", Notes: "Canal-side house"},
	})
	if err != nil {
		t.Fatal(err)
	}
	previous := -1
	for _, marker := range []string{`name="name"`, `name="city_region"`, `name="address"`, `name="timezone"`, `name="notes"`} {
		position := strings.Index(page, marker)
		if position < 0 {
			t.Fatalf("property edit drawer is missing prototype field %q", marker)
		}
		if position <= previous {
			t.Fatalf("property edit field %q is out of prototype order", marker)
		}
		previous = position
	}
	for _, marker := range []string{`class="property-edit-grid"`, `class="drawer-eyebrow">编辑资料</div>`, `保存更改`} {
		if !strings.Contains(page, marker) {
			t.Fatalf("property edit drawer is missing prototype layout marker %q", marker)
		}
	}
}

func TestEmbeddedPropertyDetailRendersCollectionAndFactsPanels(t *testing.T) {
	page, err := executeTemplate(propertyDetailPageTemplate, propertyDetailPageData{
		workspaceShell: workspaceShell{ActivePage: "properties"},
		Period:         "2026-09", PeriodLabel: "2026年9月", ListCollection: "paid", ListSort: "paid_desc",
		Property: propertyPageRow{ID: 1, Name: "Canal House", Address: "1 Main Street", Status: "active", StatusLabel: "有效", ActiveRoomCount: 1, RoomCount: 1, ExpectedAmount: "EUR 1,000.00", PaidAmount: "EUR 600.00", ExpenseAmount: "EUR 100.00", NetAmount: "EUR 500.00"},
		Rooms:    []roomPageRow{{ID: 2, RoomLabel: "A-01", PropertyName: "Canal House", TenantNames: []string{"Aoife Murphy"}, MonthlyRent: "EUR 1,000.00", ExpectedAmount: "EUR 1,000.00", PaidAmount: "EUR 600.00", BalanceAmount: "EUR 400.00", BalanceCents: 40000, Status: "active", StatusLabel: "有效"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"Canal House", "房产资料", "房间收款概览", "Aoife Murphy", "EUR 400.00", "经营净额", `list_collection=paid`, `list_sort=paid_desc`} {
		if !strings.Contains(page, expected) {
			t.Fatalf("property detail is missing %q", expected)
		}
	}
}

func TestEmbeddedRentWorkspaceUsesOneViewModelForDesktopAndMobileLists(t *testing.T) {
	data := rentWorkspacePageData{
		workspaceShell: workspaceShell{ActivePage: "rent-dashboard", Environment: "test"},
		Period:         "2026-09",
		PeriodLabel:    "2026年9月",
		View:           rentWorkspaceViewProperties,
		Filters:        defaultRentWorkspaceFilters(parseTestPeriod(t, "2026-09")),
		Summary:        rentWorkspaceSummary{ExpectedAmount: "EUR 1,000.00"},
		PropertyTreeRows: []rentWorkspacePropertyTreeRow{{Property: rentWorkspacePropertyRow{
			PropertyID: 1, Name: "Shared view model house", TotalRooms: 3,
			ExpectedAmount: "EUR 1,000.00", Status: "open", StatusLabel: "未缴纳",
		}}},
	}

	page, err := executeTemplate(rentWorkspaceTemplate, data)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(page, `class="workspace-desktop-list workspace-tree-list"`); got != 1 {
		t.Fatalf("desktop list count=%d want 1", got)
	}
	if got := strings.Count(page, `class="workspace-mobile-list workspace-tree-mobile-list"`); got != 1 {
		t.Fatalf("mobile list count=%d want 1", got)
	}
	if got := strings.Count(page, "Shared view model house"); got != 2 {
		t.Fatalf("shared row value appears %d times, want once per layout", got)
	}
	for _, expected := range []string{
		`<div class="app">`,
		`<main class="content workspace">`,
		`href="/static/css/workspace.css"`,
		`href="/static/css/pages/rent-workspace.css"`,
	} {
		if !strings.Contains(page, expected) {
			t.Fatalf("rendered workspace missing %q", expected)
		}
	}
	pageCSS := embeddedWebText("web/static/css/pages/rent-workspace.css")
	for _, expected := range []string{
		".workspace-desktop-list { display: none; }",
		".workspace-mobile-list { display: grid;",
		"@media (max-width: 640px)",
	} {
		if !strings.Contains(pageCSS, expected) {
			t.Fatalf("rent workspace CSS missing %q", expected)
		}
	}
}

func TestEmbeddedRoomDetailUsesTypedDisplayFields(t *testing.T) {
	data := rentRoomDetailPageData{
		workspaceShell:   workspaceShell{ActivePage: "rent-dashboard", Environment: "test"},
		Filters:          defaultRentWorkspaceFilters(parseTestPeriod(t, "2026-09")),
		ReturnURL:        "/rooms?period=2026-09&status=all&search=A-01",
		FromList:         true,
		ReturnPropertyID: 3,
		ReturnStatus:     "all",
		ReturnSearch:     "A-01",
		ReturnSort:       "count_desc",
		Editing:          true,
		RoomID:           5,
		Form:             roomPageForm{ID: 5, PropertyID: 3, RoomLabel: "A-01"},
		PeriodLabel:      "2026年9月",
		RoomLabel:        "A-01",
		PropertyName:     "Typed property",
		PropertyAddress:  "1 Main Street",
		Summary:          rentWorkspaceRoomRow{Status: "paid", StatusLabel: "已缴清"},
		Expenses:         []rentWorkspaceExpenseView{{Description: "Repairs", Category: "Maintenance", ExpenseDate: "2026-09-04", Amount: "EUR 10.00"}},
	}

	page, err := executeTemplate(rentRoomDetailTemplate, data)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"A-01", "Typed property", "Typed property · A-01", "2026-09-04", "EUR 10.00", `href="/rooms?period=2026-09&amp;status=all&amp;search=A-01"`, `return_sort=count_desc`, `name="return_property_id" value="3"`} {
		if !strings.Contains(page, expected) {
			t.Fatalf("room detail missing typed display field %q", expected)
		}
	}
	if strings.Contains(page, "参考号") || strings.Contains(page, "rent-2026-09") {
		t.Fatal("room detail must not display payment reference numbers")
	}
}

// Every page template under web/templates/pages renders a full document, so each
// one has to pull in the shared stylesheet itself. workspace.css owns both the
// design tokens (--border, --surface, --accent, ...) and the chrome/layout
// classes (.app, .content, .sidebar, .panel, .btn, .status, .table-wrap), and
// none of them are defined anywhere else. A page that forgets the link does not
// degrade gracefully — it renders as unstyled HTML, which is exactly what
// transaction-detail.html did from the commit that introduced it (c3c992c) until
// this test existed.
//
// The Go-string page templates are not covered here: the ones in main.go inline
// workspacePageCSS inside a <style> block instead of linking it, which works just
// as well. This test only needs to hold for the file-based pages.
func TestEveryPageTemplateLinksTheSharedStylesheet(t *testing.T) {
	entries, err := fs.ReadDir(webFiles, "web/templates/pages")
	if err != nil {
		t.Fatal(err)
	}

	// Guard the guard: a renamed directory or an emptied embed must fail loudly
	// rather than scanning zero files and passing.
	scanned := 0
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".html") {
			continue
		}
		scanned++
		path := "web/templates/pages/" + entry.Name()
		body := embeddedWebText(path)
		if !strings.Contains(body, `href="/static/css/workspace.css"`) {
			t.Errorf("%s does not link /static/css/workspace.css", path)
		}
		// The page also has to load whatever page-specific stylesheet carries its
		// own layout, otherwise the tokens resolve but the page is still bare.
		if !strings.Contains(body, `href="/static/css/pages/`) {
			t.Errorf("%s does not link a page stylesheet under /static/css/pages/", path)
		}
	}
	if scanned < 8 {
		t.Fatalf("scanned %d page templates, want at least 8 after removing the tenancy page", scanned)
	}
}

func TestEmbeddedStaticHandlerServesResourcesWithoutSourceDirectory(t *testing.T) {
	handler := embeddedWebStaticHandler()

	request := httptest.NewRequest(http.MethodGet, "/static/css/workspace.css", nil)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "@media (max-width: 640px)") {
		t.Fatalf("workspace CSS response=%d body=%q", recorder.Code, recorder.Body.String())
	}

	missing := httptest.NewRequest(http.MethodGet, "/static/css/missing.css", nil)
	missingRecorder := httptest.NewRecorder()
	handler.ServeHTTP(missingRecorder, missing)
	if missingRecorder.Code != http.StatusNotFound {
		t.Fatalf("missing resource response=%d want %d", missingRecorder.Code, http.StatusNotFound)
	}
}

func parseTestPeriod(t *testing.T, value string) (period time.Time) {
	t.Helper()
	period, err := parsePeriodMonth(value)
	if err != nil {
		t.Fatal(err)
	}
	return period
}
