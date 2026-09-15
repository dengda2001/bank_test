package main

import (
	"context"
	"strings"
	"testing"
)

func TestDunningSMTPConfigRequiresExplicitServiceSender(t *testing.T) {
	valid := dunningSMTPConfig{Host: "mailpit.test", Port: 1025, FromEmail: "mailer@mailpit.test"}
	if err := validateDunningSMTPConfig(valid); err != nil {
		t.Fatal(err)
	}
	for name, config := range map[string]dunningSMTPConfig{
		"missing host":         {Port: 1025, FromEmail: valid.FromEmail},
		"missing service from": {Host: valid.Host, Port: valid.Port},
		"invalid port":         {Host: valid.Host, Port: 0, FromEmail: valid.FromEmail},
		"invalid service from": {Host: valid.Host, Port: valid.Port, FromEmail: "not-an-email"},
	} {
		t.Run(name, func(t *testing.T) {
			if err := validateDunningSMTPConfig(config); err == nil {
				t.Fatal("expected SMTP configuration error")
			}
		})
	}
}

func TestDunningRFC822UsesServiceFromAndReplyTo(t *testing.T) {
	raw, err := buildDunningRFC822Email(dunningEmail{
		FromEmail: "mailer@mailpit.test",
		FromName:  "Dublin Homes",
		To:        "tenant@example.test",
		ReplyTo:   "landlord@example.test",
		Subject:   "Rent reminder for September 2026",
		Body:      "Dear tenant,\nPlease pay.",
	})
	if err != nil {
		t.Fatal(err)
	}
	message := string(raw)
	for _, expected := range []string{
		`From: "Dublin Homes" <mailer@mailpit.test>`,
		"To: tenant@example.test",
		"Reply-To: landlord@example.test",
		"Subject: Rent reminder for September 2026",
		"Dear tenant,\nPlease pay.",
	} {
		if !strings.Contains(message, expected) {
			t.Fatalf("RFC822 message missing %q: %s", expected, message)
		}
	}
	if strings.Contains(message, "Bcc:") || strings.Contains(message, "mailer@mailpit.test\r\nReply-To") {
		t.Fatalf("RFC822 message exposes unintended recipient headers: %s", message)
	}
}

func TestSMTPDunningDeliveryRejectsMissingConfigWithoutNetwork(t *testing.T) {
	delivery := newSMTPDunningDelivery(dunningSMTPConfig{})
	err := delivery.Send(context.Background(), dunningEmail{To: "tenant@example.test", ReplyTo: "landlord@example.test", Subject: "subject", Body: "body"})
	if err == nil || !strings.Contains(err.Error(), "SMTP") {
		t.Fatalf("missing SMTP error=%v", err)
	}
}
