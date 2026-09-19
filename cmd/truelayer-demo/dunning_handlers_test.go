package main

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestDunningDashboardActionParserPreservesCurrentPageContext(t *testing.T) {
	form := url.Values{
		"period":         {"2026-09"},
		"search":         {"Aoife"},
		"status":         {"unpaid"},
		"sort":           {"due_desc"},
		"page":           {"2"},
		"page_size":      {"24"},
		"obligation_id":  {"11", "11", "12"},
		"request_key":    {"dunning-request-1"},
		"confirm_resend": {"1"},
	}
	req := httptest.NewRequest(http.MethodPost, "/dunning/send", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	action, err := dunningDashboardActionFromRequest(req, dunningActionSend)
	if err != nil {
		t.Fatal(err)
	}
	if action.Period != (time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)) || action.Filters.Search != "Aoife" || action.Filters.Status != "unpaid" || action.Filters.Sort != "due_desc" || action.Filters.Page != 2 || action.Filters.PageSize != 24 {
		t.Fatalf("action context=%+v", action)
	}
	if len(action.SelectedIDs) != 2 || action.SelectedIDs[0] != 11 || action.SelectedIDs[1] != 12 || !action.ForceResend {
		t.Fatalf("action selection=%+v", action)
	}
}

func TestDunningDashboardActionParserRequiresRequestKeyForSend(t *testing.T) {
	form := url.Values{"period": {"2026-09"}, "obligation_id": {"11"}}
	req := httptest.NewRequest(http.MethodPost, "/dunning/send", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if _, err := dunningDashboardActionFromRequest(req, dunningActionSend); err == nil || !strings.Contains(err.Error(), "request key") {
		t.Fatalf("missing request key error=%v", err)
	}
}

func TestDunningPageSelectionRejectsObligationsOutsideCurrentPage(t *testing.T) {
	err := validateDunningPageSelection([]dunningCandidate{{ObligationID: 11}}, []uint64{12})
	if err == nil || !strings.Contains(err.Error(), "当前页") {
		t.Fatalf("selection error=%v", err)
	}
}

func TestDunningPOSTRejectsLegacySessionWithoutDatabase(t *testing.T) {
	a := testApp()
	req := httptest.NewRequest(http.MethodPost, "/dunning/send", strings.NewReader("period=2026-09&request_key=request-1&obligation_id=11"))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(sessionCookie(a.cfg, time.Now().Add(sessionTTL)))
	rec := httptest.NewRecorder()
	a.handleDunningSend(rec, req)
	if rec.Code != http.StatusServiceUnavailable {
		t.Fatalf("status=%d body=%s", rec.Code, rec.Body.String())
	}
}

// The preview/result rendering of the dunning flow lives on the dedicated
// /dunning page; the dashboard drawer this test used to render was removed with
// the legacy template. The failed-attempt status still round-trips through
// dunningDeliveryLabel, but the drawer chrome (id="dunning-drawer",
// data-dunning-open) and the per-attempt "重试此人" button had no other carrier:
// they are a prototype feature still missing on the real path and are registered
// as such in the backend spec for the dashboard-alignment subtask.
func TestDunningPageRendersPreviewResultsAndConfirmation(t *testing.T) {
	page, err := executeTemplate(dunningPageTemplate, rentDashboardPageData{
		workspaceShell: workspaceShell{Environment: "sandbox"},
		Period:         "2026-09",
		PeriodLabel:    "2026年9月",
		Dunning: dunningDrawerData{
			Enabled:          true,
			Open:             true,
			Period:           "2026-09",
			StatusFilter:     "unpaid",
			SortFilter:       "status",
			Page:             1,
			PageSize:         12,
			RequestKey:       "dunning-request-1",
			Sender:           dunningSenderConfig{DisplayName: "Dublin Homes", ReplyToEmail: "landlord@example.test"},
			SenderConfigured: true,
			Candidates: []dunningCandidate{{
				ObligationID: 11, TenantName: "Aoife Murphy", Email: "aoife@example.test", BalanceAmount: "EUR 600.00", Selectable: true, Selected: true,
			}},
			PreviewRows: []dunningPreview{{
				Candidate: dunningCandidate{TenantName: "Aoife Murphy"},
				Message:   &dunningMessage{Subject: "Rent reminder for September 2026", RecipientEmail: "aoife@example.test", Body: "Dear Aoife"},
			}},
			Results: []dunningSendResult{{
				Candidate:       dunningCandidate{ObligationID: 11, TenantName: "Aoife Murphy"},
				Attempt:         &dunningSendAttempt{ID: 41, DeliveryStatus: dunningDeliveryFailed},
				Error:           "provider unavailable",
				RetryRequestKey: "dunning-retry-1",
			}, {
				Candidate: dunningCandidate{TenantName: "Unavailable Tenant"},
				Error:     "账单不存在或不属于当前用户",
			}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		`action="/dunning/config"`,
		`/dunning/preview`,
		`/dunning/send`,
		"发送前预览",
		"Rent reminder for September 2026",
		"provider unavailable",
		"账单不存在或不属于当前用户",
		"发送失败",
		"确认同日重发",
	} {
		if !strings.Contains(page, expected) {
			t.Fatalf("dunning page missing %q: %s", expected, page)
		}
	}
}
