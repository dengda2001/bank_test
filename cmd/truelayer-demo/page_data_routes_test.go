package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCanonicalPageRoutesRequireAuthentication(t *testing.T) {
	a := testApp()
	handler := newAppMux(&a)
	paths := []string{"/bills", "/transactions", "/dunning", "/properties", "/properties/1", "/properties/not-an-id", "/rooms", "/rooms/1", "/tenancies", "/cash-receipts", "/bank"}
	for _, path := range paths {
		t.Run(path, func(t *testing.T) {
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
			if rec.Code != http.StatusFound {
				t.Fatalf("status=%d want %d", rec.Code, http.StatusFound)
			}
			if location := rec.Header().Get("Location"); location != "/" {
				t.Fatalf("Location=%q want /", location)
			}
		})
	}
}

func TestCanonicalRoutesKeepLegacyRefreshTargetAndBankTarget(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/refresh", nil)
	if got := bankRefreshRedirect(request, "message=refreshed"); got != "/billing?message=refreshed" {
		t.Fatalf("legacy refresh target=%q", got)
	}
	request = httptest.NewRequest(http.MethodPost, "/bank/sync", nil)
	if got := bankRefreshRedirect(request, "message=refreshed"); got != "/bank?message=refreshed" {
		t.Fatalf("canonical refresh target=%q", got)
	}
}

func TestDashboardManualBalanceRequiresReasonField(t *testing.T) {
	var body strings.Builder
	if err := rentDashboardTemplate.Execute(&body, rentDashboardPageData{Period: "2026-09", PeriodLabel: "2026年9月", Rows: []rentDashboardRow{{ObligationID: 7, ExpectedCents: 100, PaidCents: 50, Status: "partial", StatusLabel: "部分缴纳"}}, Dunning: dunningDrawerData{Enabled: true}}); err != nil {
		t.Fatal(err)
	}
	page := body.String()
	for _, marker := range []string{`name="reason"`, `placeholder="填写平账原因"`, `required`, `onsubmit="return confirm('确认一键平账吗？')"`, `onclick="return confirm('确认发送催收邮件吗？')"`} {
		if !strings.Contains(page, marker) {
			t.Fatalf("dashboard missing manual-balance marker %q", marker)
		}
	}
}

func TestCanonicalPageTemplatesExposeSemanticEntityIDs(t *testing.T) {
	var propertyPage strings.Builder
	if err := propertyPageTemplate.Execute(&propertyPage, propertyPageData{Rows: []propertyPageRow{{ID: 1, Name: "House", Status: "active", StatusLabel: "有效"}}}); err != nil {
		t.Fatal(err)
	}
	var roomPage strings.Builder
	if err := roomPageTemplate.Execute(&roomPage, roomPageData{Rows: []roomPageRow{{ID: 2, RoomLabel: "A-01", Status: "active", StatusLabel: "有效"}}}); err != nil {
		t.Fatal(err)
	}
	var tenancyPage strings.Builder
	if err := tenancyPageTemplate.Execute(&tenancyPage, tenancyPageData{Rows: []tenancyPageRow{{ID: 3, RoomID: 2, RoomLabel: "A-01", Status: "active", StatusLabel: "有效"}}}); err != nil {
		t.Fatal(err)
	}
	var bankPage strings.Builder
	if err := bankPageTemplate.Execute(&bankPage, bankPageData{Runs: []bankPageRun{{ID: 4, Status: "succeeded", StatusLabel: "同步成功"}}}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(propertyPage.String(), `data-property-id`) {
		t.Fatal("property page does not expose a stable property id")
	}
	if !strings.Contains(roomPage.String(), `data-room-id`) {
		t.Fatal("room page does not expose a stable room id")
	}
	if !strings.Contains(tenancyPage.String(), `data-tenancy-id`) {
		t.Fatal("tenancy page does not expose a stable tenancy id")
	}
	if !strings.Contains(bankPage.String(), `data-sync-run-id`) {
		t.Fatal("bank page does not expose a stable sync run id")
	}
}
