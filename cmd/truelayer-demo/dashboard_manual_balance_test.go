package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

func TestDashboardManualBalanceRedirectPreservesDashboardFilters(t *testing.T) {
	got := dashboardManualBalanceRedirect(url.Values{
		"period":    {"2026-09"},
		"search":    {"Aoife Murphy"},
		"status":    {"unpaid"},
		"sort":      {"due_desc"},
		"page":      {"2"},
		"page_size": {"24"},
	}, "manual_balance_saved", "")
	parsed, err := url.Parse(got)
	if err != nil {
		t.Fatal(err)
	}
	if parsed.Path != "/rent-dashboard" {
		t.Fatalf("redirect path=%q", parsed.Path)
	}
	query := parsed.Query()
	for key, want := range map[string]string{
		"period":    "2026-09",
		"search":    "Aoife Murphy",
		"status":    "unpaid",
		"sort":      "due_desc",
		"page":      "2",
		"page_size": "24",
		"message":   "manual_balance_saved",
	} {
		if got := query.Get(key); got != want {
			t.Fatalf("redirect query %s=%q want %q: %q", key, got, want, got)
		}
	}
}

func TestDashboardManualBalanceHandlerAcceptsPostOnly(t *testing.T) {
	a := testApp()
	req := httptest.NewRequest(http.MethodGet, "/rent-dashboard/settle", nil)
	req.AddCookie(sessionCookie(a.cfg, time.Now().Add(sessionTTL)))
	rec := httptest.NewRecorder()

	a.handleDashboardManualBalance(rec, req)

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("status=%d want %d", rec.Code, http.StatusMethodNotAllowed)
	}
}
