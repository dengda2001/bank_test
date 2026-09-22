package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"
)

type recordingDunningDelivery struct {
	mu    sync.Mutex
	calls []dunningEmail
	err   error
}

func (d *recordingDunningDelivery) Send(_ context.Context, message dunningEmail) error {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.calls = append(d.calls, message)
	return d.err
}

func TestDunningSendWorkflowOnMySQL(t *testing.T) {
	db, sqlDB := openLedgerMySQLTestDB(t)
	for i := 0; i < 2; i++ {
		if err := runMigrations(sqlDB, "../../migrations"); err != nil {
			t.Fatalf("migration run %d: %v", i+1, err)
		}
	}

	ctx := context.Background()
	now := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	period := time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	owner := user{Username: fmt.Sprintf("dunning-send-owner-%d", time.Now().UnixNano()), PasswordHash: "test"}
	otherUser := user{Username: fmt.Sprintf("dunning-send-other-%d", time.Now().UnixNano()), PasswordHash: "test"}
	if err := db.WithContext(ctx).Create(&owner).Error; err != nil {
		t.Fatal(err)
	}
	if err := db.WithContext(ctx).Create(&otherUser).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = db.WithContext(ctx).Delete(&user{}, owner.ID).Error
		_ = db.WithContext(ctx).Delete(&user{}, otherUser.ID).Error
	})

	createTenant := func(userID uint64, name, email string) tenant {
		t.Helper()
		input := validTenantInputForProfile()
		input.Name = name
		input.Email = email
		created, err := newTenantService(db).createTenant(ctx, userID, input)
		if err != nil {
			t.Fatal(err)
		}
		return created
	}

	successTenant := createTenant(owner.ID, "Successful Delivery", "success@example.test")
	retryTenant := createTenant(owner.ID, "Retry Delivery", "retry@example.test")
	paidTenant := createTenant(owner.ID, "Already Paid", "paid@example.test")
	otherTenant := createTenant(otherUser.ID, "Other Owner", "other@example.test")
	obligations := newObligationService(db)
	if err := obligations.ensureMonthlyObligations(ctx, owner.ID, period); err != nil {
		t.Fatal(err)
	}
	if err := obligations.ensureMonthlyObligations(ctx, otherUser.ID, period); err != nil {
		t.Fatal(err)
	}
	findObligation := func(userID, tenantID uint64) rentObligation {
		t.Helper()
		var obligation rentObligation
		if err := db.WithContext(ctx).Where("user_id = ? AND tenant_id = ? AND period_month = ?", userID, tenantID, period).First(&obligation).Error; err != nil {
			t.Fatal(err)
		}
		return obligation
	}
	successObligation := findObligation(owner.ID, successTenant.ID)
	retryObligation := findObligation(owner.ID, retryTenant.ID)
	paidObligation := findObligation(owner.ID, paidTenant.ID)
	otherObligation := findObligation(otherUser.ID, otherTenant.ID)

	service := newDunningService(db)
	sender, err := service.saveSenderConfig(ctx, owner.ID, dunningSenderConfig{DisplayName: "Dublin Homes", ReplyToEmail: "landlord@example.test"})
	if err != nil {
		t.Fatal(err)
	}
	if sender.ID == 0 {
		t.Fatal("sender configuration was not persisted")
	}

	preview, err := service.preview(ctx, owner.ID, period, []uint64{successObligation.ID}, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(preview.Rows) != 1 || preview.Rows[0].Message == nil || preview.Rows[0].Error != "" {
		t.Fatalf("preview=%+v", preview)
	}
	var previewAttempts int64
	if err := db.WithContext(ctx).Model(&dunningSendAttempt{}).Where("user_id = ?", owner.ID).Count(&previewAttempts).Error; err != nil {
		t.Fatal(err)
	}
	if previewAttempts != 0 {
		t.Fatalf("preview wrote %d attempts", previewAttempts)
	}

	delivery := &recordingDunningDelivery{}
	first, err := service.send(ctx, owner.ID, period, []uint64{successObligation.ID}, "success-request", false, 0, "mailer@example.test", delivery, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 1 || first[0].Attempt == nil || first[0].Attempt.DeliveryStatus != dunningDeliverySent || first[0].Error != "" {
		t.Fatalf("first send=%+v", first)
	}
	if len(delivery.calls) != 1 {
		t.Fatalf("delivery calls=%d want 1", len(delivery.calls))
	}
	message := delivery.calls[0]
	if message.FromEmail != "mailer@example.test" || message.To != "success@example.test" || message.ReplyTo != "landlord@example.test" {
		t.Fatalf("delivered message=%+v", message)
	}

	repeated, err := service.send(ctx, owner.ID, period, []uint64{successObligation.ID}, "success-request", false, 0, "mailer@example.test", delivery, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if len(repeated) != 1 || repeated[0].Attempt == nil || repeated[0].Attempt.ID != first[0].Attempt.ID || len(delivery.calls) != 1 {
		t.Fatalf("idempotent retry=%+v calls=%d", repeated, len(delivery.calls))
	}

	sameDaySkip, err := service.send(ctx, owner.ID, period, []uint64{successObligation.ID}, "same-day-skip", false, 0, "mailer@example.test", delivery, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if len(sameDaySkip) != 1 || !sameDaySkip[0].Skipped || sameDaySkip[0].Attempt == nil || sameDaySkip[0].Attempt.DeliveryStatus != dunningDeliverySkipped || len(delivery.calls) != 1 {
		t.Fatalf("same-day skip=%+v calls=%d", sameDaySkip, len(delivery.calls))
	}
	forced, err := service.send(ctx, owner.ID, period, []uint64{successObligation.ID}, "same-day-force", true, 0, "mailer@example.test", delivery, now.Add(2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if len(forced) != 1 || forced[0].Attempt == nil || forced[0].Attempt.DeliveryStatus != dunningDeliverySent || len(delivery.calls) != 2 {
		t.Fatalf("same-day force=%+v calls=%d", forced, len(delivery.calls))
	}

	delivery.err = errors.New("provider unavailable")
	failed, err := service.send(ctx, owner.ID, period, []uint64{retryObligation.ID}, "retry-request-1", false, 0, "mailer@example.test", delivery, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(failed) != 1 || failed[0].Attempt == nil || failed[0].Attempt.DeliveryStatus != dunningDeliveryFailed || !strings.Contains(failed[0].Error, "provider unavailable") {
		t.Fatalf("failed send=%+v", failed)
	}
	delivery.err = nil
	retried, err := service.send(ctx, owner.ID, period, []uint64{retryObligation.ID}, "retry-request-2", false, failed[0].Attempt.ID, "mailer@example.test", delivery, now.Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if len(retried) != 1 || retried[0].Attempt == nil || retried[0].Attempt.DeliveryStatus != dunningDeliverySent || retried[0].Attempt.RetryOfAttemptID == nil || *retried[0].Attempt.RetryOfAttemptID != failed[0].Attempt.ID {
		t.Fatalf("retried send=%+v", retried)
	}
	if len(delivery.calls) != 4 {
		t.Fatalf("delivery calls after retry=%d want 4", len(delivery.calls))
	}

	concurrentResults := make(chan []dunningSendResult, 2)
	concurrentErrors := make(chan error, 2)
	var concurrentWG sync.WaitGroup
	for i := 0; i < 2; i++ {
		concurrentWG.Add(1)
		go func() {
			defer concurrentWG.Done()
			results, sendErr := service.send(ctx, owner.ID, period, []uint64{successObligation.ID}, "concurrent-request", false, 0, "mailer@example.test", delivery, now.Add(48*time.Hour))
			concurrentResults <- results
			concurrentErrors <- sendErr
		}()
	}
	concurrentWG.Wait()
	close(concurrentResults)
	close(concurrentErrors)
	for sendErr := range concurrentErrors {
		if sendErr != nil {
			t.Fatalf("concurrent send error=%v", sendErr)
		}
	}
	for results := range concurrentResults {
		if len(results) != 1 || results[0].Attempt == nil || results[0].Attempt.DeliveryStatus != dunningDeliverySent {
			t.Fatalf("concurrent send=%+v", results)
		}
	}
	if len(delivery.calls) != 5 {
		t.Fatalf("delivery calls after concurrent duplicate=%d want 5", len(delivery.calls))
	}

	if _, err := newCashReceiptService(db).recordCashReceipt(ctx, cashReceiptInput{
		UserID:           owner.ID,
		TenantID:         paidTenant.ID,
		RentObligationID: paidObligation.ID,
		AmountCents:      paidObligation.ExpectedAmountCents,
		Currency:         "EUR",
		ReceivedAt:       now,
		IdempotencyKey:   "paid-before-dunning",
	}); err != nil {
		t.Fatal(err)
	}
	paidPreview, err := service.preview(ctx, owner.ID, period, []uint64{paidObligation.ID}, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(paidPreview.Rows) != 1 || paidPreview.Rows[0].Message != nil || !strings.Contains(paidPreview.Rows[0].Error, "缴清") {
		t.Fatalf("paid preview=%+v", paidPreview)
	}
	paidSend, err := service.send(ctx, owner.ID, period, []uint64{paidObligation.ID}, "paid-request", false, 0, "mailer@example.test", delivery, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(paidSend) != 1 || !paidSend[0].Skipped || paidSend[0].Attempt == nil || paidSend[0].Attempt.DeliveryStatus != dunningDeliverySkipped || len(delivery.calls) != 5 {
		t.Fatalf("paid send=%+v calls=%d", paidSend, len(delivery.calls))
	}

	isolated, err := service.send(ctx, owner.ID, period, []uint64{otherObligation.ID}, "isolation-request", false, 0, "mailer@example.test", delivery, now)
	if err != nil {
		t.Fatal(err)
	}
	if len(isolated) != 1 || isolated[0].Attempt != nil || !strings.Contains(isolated[0].Error, "不属于当前用户") || len(delivery.calls) != 5 {
		t.Fatalf("isolated send=%+v calls=%d", isolated, len(delivery.calls))
	}
}
