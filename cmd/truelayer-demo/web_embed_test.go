package main

import (
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
		"web/static/css/workspace.css",
		"web/static/css/calendar.css",
		"web/static/css/pages/rent-workspace.css",
		"web/static/css/pages/room-detail.css",
		"web/static/js/calendar.js",
	} {
		if contents := embeddedWebText(path); strings.TrimSpace(contents) == "" {
			t.Fatalf("embedded resource %q is empty", path)
		}
	}

	page := embeddedWebText("web/templates/pages/rent-workspace.html")
	for _, href := range []string{
		`/static/css/workspace.css`,
		`/static/css/calendar.css`,
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

func TestEmbeddedRentWorkspaceUsesOneViewModelForDesktopAndMobileLists(t *testing.T) {
	data := rentWorkspacePageData{
		workspaceShell: workspaceShell{ActivePage: "rent-dashboard", Environment: "test"},
		Period:         "2026-09",
		PeriodLabel:    "2026年9月",
		View:           rentWorkspaceViewProperties,
		Filters:        defaultRentWorkspaceFilters(parseTestPeriod(t, "2026-09")),
		Summary:        rentWorkspaceSummary{ExpectedAmount: "EUR 1,000.00"},
		PropertyRows: []rentWorkspacePropertyRow{{
			PropertyID: 1, Name: "Shared view model house", TotalRooms: 3,
			ExpectedAmount: "EUR 1,000.00", Status: "open", StatusLabel: "未缴纳",
		}},
	}

	page, err := executeTemplate(rentWorkspaceTemplate, data)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Count(page, `class="workspace-desktop-list"`); got != 1 {
		t.Fatalf("desktop list count=%d want 1", got)
	}
	if got := strings.Count(page, `class="workspace-mobile-list"`); got != 1 {
		t.Fatalf("mobile list count=%d want 1", got)
	}
	if got := strings.Count(page, "Shared view model house"); got != 2 {
		t.Fatalf("shared row value appears %d times, want once per layout", got)
	}
	for _, expected := range []string{
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
		workspaceShell:  workspaceShell{ActivePage: "rent-dashboard", Environment: "test"},
		Filters:         defaultRentWorkspaceFilters(parseTestPeriod(t, "2026-09")),
		PeriodLabel:     "2026年9月",
		RoomLabel:       "A-01",
		PropertyName:    "Typed property",
		PropertyAddress: "1 Main Street",
		Summary:         rentWorkspaceRoomRow{Status: "paid", StatusLabel: "已缴清"},
		Expenses:        []rentWorkspaceExpenseView{{Description: "Repairs", Category: "Maintenance", ExpenseDate: "2026-09-04", Amount: "EUR 10.00"}},
	}

	page, err := executeTemplate(rentRoomDetailTemplate, data)
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"A-01", "Typed property", "1 Main Street", "2026-09-04", "EUR 10.00"} {
		if !strings.Contains(page, expected) {
			t.Fatalf("room detail missing typed display field %q", expected)
		}
	}
	if strings.Contains(page, "参考号") || strings.Contains(page, "rent-2026-09") {
		t.Fatal("room detail must not display payment reference numbers")
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
