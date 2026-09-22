package main

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestDunningDashboardHTTPWorkflowOnMySQL(t *testing.T) {
	db, sqlDB := openLedgerMySQLTestDB(t)
	if err := runMigrations(sqlDB, "../../migrations"); err != nil {
		t.Fatalf("run migrations: %v", err)
	}
	ctx := context.Background()
	owner := user{Username: fmt.Sprintf("dunning-http-owner-%d", time.Now().UnixNano()), PasswordHash: "test"}
	if err := db.WithContext(ctx).Create(&owner).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.WithContext(ctx).Delete(&user{}, owner.ID).Error })

	input := validTenantInputForProfile()
	input.Name = "HTTP Tenant"
	input.Email = "http-tenant@example.test"
	tenantRow, err := newTenantService(db).createTenant(ctx, owner.ID, input)
	if err != nil {
		t.Fatal(err)
	}
	period := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	if err := newObligationService(db).ensureMonthlyObligations(ctx, owner.ID, period); err != nil {
		t.Fatal(err)
	}
	var obligation rentObligation
	if err := db.WithContext(ctx).Where("user_id = ? AND tenant_id = ? AND period_month = ?", owner.ID, tenantRow.ID, period).First(&obligation).Error; err != nil {
		t.Fatal(err)
	}
	if _, err := newDunningService(db).saveSenderConfig(ctx, owner.ID, dunningSenderConfig{DisplayName: "Dublin Homes", ReplyToEmail: "landlord@example.test"}); err != nil {
		t.Fatal(err)
	}

	a := testApp()
	a.db = db
	a.cfg.DunningSMTPFrom = "mailer@example.test"
	delivery := &recordingDunningDelivery{}
	a.dunningMailer = delivery
	cookie := userSessionCookie(a.cfg, owner.ID, owner.Username, time.Now().Add(sessionTTL))

	getRequest := httptest.NewRequest(http.MethodGet, "/rent-dashboard?period=2026-09", nil)
	getRequest.AddCookie(cookie)
	getResponse := httptest.NewRecorder()
	a.handleRentDashboard(getResponse, getRequest)
	if getResponse.Code != http.StatusOK || !strings.Contains(getResponse.Body.String(), "邮件催缴") || !strings.Contains(getResponse.Body.String(), input.Email) {
		t.Fatalf("dashboard status=%d body=%s", getResponse.Code, getResponse.Body.String())
	}

	previewForm := url.Values{"period": {"2026-09"}, "obligation_id": {fmt.Sprint(obligation.ID)}, "request_key": {"http-preview"}}
	previewRequest := httptest.NewRequest(http.MethodPost, "/dunning/preview", strings.NewReader(previewForm.Encode()))
	previewRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	previewRequest.AddCookie(cookie)
	previewResponse := httptest.NewRecorder()
	a.handleDunningPreview(previewResponse, previewRequest)
	if previewResponse.Code != http.StatusOK || !strings.Contains(previewResponse.Body.String(), "发送前预览") || len(delivery.calls) != 0 {
		t.Fatalf("preview status=%d calls=%d body=%s", previewResponse.Code, len(delivery.calls), previewResponse.Body.String())
	}
	var attemptCount int64
	if err := db.WithContext(ctx).Model(&dunningSendAttempt{}).Where("user_id = ?", owner.ID).Count(&attemptCount).Error; err != nil {
		t.Fatal(err)
	}
	if attemptCount != 0 {
		t.Fatalf("preview attempt count=%d want 0", attemptCount)
	}

	sendForm := url.Values{"period": {"2026-09"}, "obligation_id": {fmt.Sprint(obligation.ID)}, "request_key": {"http-send"}}
	sendRequest := httptest.NewRequest(http.MethodPost, "/dunning/send", strings.NewReader(sendForm.Encode()))
	sendRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	sendRequest.AddCookie(cookie)
	sendResponse := httptest.NewRecorder()
	a.handleDunningSend(sendResponse, sendRequest)
	if sendResponse.Code != http.StatusOK || !strings.Contains(sendResponse.Body.String(), "已发送") || len(delivery.calls) != 1 {
		t.Fatalf("send status=%d calls=%d body=%s", sendResponse.Code, len(delivery.calls), sendResponse.Body.String())
	}
}
