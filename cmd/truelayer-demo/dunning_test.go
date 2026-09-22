package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDunningMessageUsesDublinDueDayAndFixedEnglishTemplates(t *testing.T) {
	candidate := dunningCandidate{
		TenantName:   "Aoife Murphy",
		Period:       "2026-09",
		DueDateValue: time.Date(2026, 9, 5, 0, 0, 0, 0, time.UTC),
		DueDate:      "2026-09-05",
		BalanceCents: 60000,
		Currency:     "EUR",
		Email:        "aoife@example.test",
		EmailValid:   true,
	}
	sender := dunningSenderConfig{DisplayName: "Dublin Homes", ReplyToEmail: "landlord@example.test"}

	reminder, err := buildDunningMessage(candidate, sender, time.Date(2026, 9, 5, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if reminder.TemplateKind != dunningTemplateReminder || reminder.Subject != "Rent reminder for September 2026" || reminder.OverdueDays != 0 {
		t.Fatalf("reminder=%+v", reminder)
	}
	if !strings.Contains(reminder.Body, "EUR 600.00") || strings.Contains(strings.ToLower(reminder.Body), "overdue") {
		t.Fatalf("unexpected reminder body=%q", reminder.Body)
	}

	overdue, err := buildDunningMessage(candidate, sender, time.Date(2026, 9, 6, 0, 1, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if overdue.TemplateKind != dunningTemplateOverdue || overdue.Subject != "Rent overdue for September 2026" || overdue.OverdueDays != 1 {
		t.Fatalf("overdue=%+v", overdue)
	}
	if !strings.Contains(overdue.Body, "1 day overdue") {
		t.Fatalf("overdue body missing Dublin day count=%q", overdue.Body)
	}
}

func TestDunningMessageRejectsInvalidRecipientAndEmptyBalance(t *testing.T) {
	sender := dunningSenderConfig{DisplayName: "Dublin Homes", ReplyToEmail: "landlord@example.test"}
	candidate := dunningCandidate{TenantName: "Aoife", Period: "2026-09", DueDateValue: time.Date(2026, 9, 5, 0, 0, 0, 0, time.UTC), DueDate: "2026-09-05", BalanceCents: 50000, Email: "not-an-email"}
	if _, err := buildDunningMessage(candidate, sender, time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)); err == nil || !strings.Contains(err.Error(), "recipient") {
		t.Fatalf("invalid recipient error=%v", err)
	}
	candidate.Email = "aoife@example.test"
	candidate.EmailValid = true
	candidate.BalanceCents = 0
	if _, err := buildDunningMessage(candidate, sender, time.Now()); err == nil || !strings.Contains(err.Error(), "balance") {
		t.Fatalf("empty balance error=%v", err)
	}
}

func TestDunningCandidateSkipsSameDaySuccessByDefault(t *testing.T) {
	now := time.Date(2026, 9, 16, 12, 0, 0, 0, time.UTC)
	sentAt := time.Date(2026, 9, 16, 9, 0, 0, 0, time.UTC)
	candidate := buildDunningCandidate(rentObligation{
		ID:                  7,
		TenantID:            3,
		PeriodMonth:         time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC),
		DueDate:             time.Date(2026, 9, 20, 0, 0, 0, 0, time.UTC),
		ExpectedAmountCents: 100000,
		PaidAmountCents:     40000,
		Currency:            "EUR",
		Status:              "partial",
		RecordStatus:        obligationRecordActive,
	}, tenant{ID: 3, Name: "Aoife", Email: "aoife@example.test"}, now, &dunningSendAttempt{DeliveryStatus: dunningDeliverySent, SentAt: &sentAt})
	if !candidate.SentToday || candidate.DefaultSelected || !candidate.Selectable {
		t.Fatalf("candidate=%+v", candidate)
	}
}

func TestValidateDunningSenderConfig(t *testing.T) {
	valid := dunningSenderConfig{DisplayName: "Dublin Homes", ReplyToEmail: "landlord@example.test"}
	if err := validateDunningSenderConfig(valid); err != nil {
		t.Fatal(err)
	}
	for name, config := range map[string]dunningSenderConfig{
		"missing display name": {ReplyToEmail: valid.ReplyToEmail},
		"invalid reply-to":     {DisplayName: valid.DisplayName, ReplyToEmail: "not-an-email"},
		"header injection":     {DisplayName: "Homes\r\nBcc: bad@example.test", ReplyToEmail: valid.ReplyToEmail},
	} {
		t.Run(name, func(t *testing.T) {
			if err := validateDunningSenderConfig(config); err == nil {
				t.Fatal("expected sender config validation error")
			}
		})
	}
}

func TestDunningMigrationDefinesSenderAndAttemptAuditTables(t *testing.T) {
	body, err := os.ReadFile(filepath.Join("..", "..", "migrations", "008_dunning_mail.sql"))
	if err != nil {
		t.Fatal(err)
	}
	schema := string(body)
	for _, fragment := range []string{
		"CREATE TABLE IF NOT EXISTS dunning_sender_configs",
		"UNIQUE KEY idx_dunning_sender_configs_user (user_id)",
		"CREATE TABLE IF NOT EXISTS dunning_send_attempts",
		"UNIQUE KEY idx_dunning_attempts_user_request_obligation (user_id, request_key, rent_obligation_id)",
		"retry_of_attempt_id",
		"subject varchar(512)",
		"body text",
	} {
		if !strings.Contains(schema, fragment) {
			t.Fatalf("migration missing %q", fragment)
		}
	}
}
