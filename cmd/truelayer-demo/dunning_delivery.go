package main

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"net/smtp"
	"strconv"
	"strings"
)

type dunningEmail struct {
	FromEmail string
	FromName  string
	To        string
	ReplyTo   string
	Subject   string
	Body      string
}

type dunningMailDelivery interface {
	Send(context.Context, dunningEmail) error
}

type dunningDeliveryValidator interface {
	Validate() error
}

type dunningSMTPConfig struct {
	Host      string
	Port      int
	Username  string
	Password  string
	FromEmail string
}

func dunningSMTPConfigFromConfig(cfg config) dunningSMTPConfig {
	port := cfg.DunningSMTPPort
	if port == 0 {
		port = 587
	}
	return dunningSMTPConfig{
		Host:      cfg.DunningSMTPHost,
		Port:      port,
		Username:  cfg.DunningSMTPUsername,
		Password:  cfg.DunningSMTPPassword,
		FromEmail: cfg.DunningSMTPFrom,
	}
}

func validateDunningSMTPConfig(config dunningSMTPConfig) error {
	if strings.TrimSpace(config.Host) == "" {
		return errors.New("SMTP host is required")
	}
	if config.Port < 1 || config.Port > 65535 {
		return errors.New("SMTP port is invalid")
	}
	if strings.TrimSpace(config.FromEmail) == "" {
		return errors.New("SMTP service from email is required")
	}
	if err := validateDunningRecipient(config.FromEmail); err != nil {
		return errors.New("SMTP service from email is invalid")
	}
	if config.Password != "" && strings.TrimSpace(config.Username) == "" {
		return errors.New("SMTP username is required when password is set")
	}
	return nil
}

func buildDunningRFC822Email(message dunningEmail) ([]byte, error) {
	if err := validateDunningRecipient(message.FromEmail); err != nil {
		return nil, errors.New("SMTP service from email is invalid")
	}
	if err := validateDunningRecipient(message.To); err != nil {
		return nil, err
	}
	if err := validateDunningRecipient(message.ReplyTo); err != nil {
		return nil, errors.New("reply-to email is invalid")
	}
	if strings.TrimSpace(message.Subject) == "" || strings.ContainsAny(message.Subject, "\r\n") {
		return nil, errors.New("email subject is invalid")
	}
	from := (&mail.Address{Name: sanitizeMailHeaderText(message.FromName), Address: message.FromEmail}).String()
	lines := []string{
		"From: " + from,
		"To: " + message.To,
		"Reply-To: " + message.ReplyTo,
		"Subject: " + sanitizeMailHeaderText(message.Subject),
		"MIME-Version: 1.0",
		"Content-Type: text/plain; charset=UTF-8",
		"Content-Transfer-Encoding: 8bit",
		"",
		message.Body,
	}
	return []byte(strings.Join(lines, "\r\n")), nil
}

type smtpDunningDelivery struct {
	config dunningSMTPConfig
}

func newSMTPDunningDelivery(config dunningSMTPConfig) *smtpDunningDelivery {
	return &smtpDunningDelivery{config: config}
}

func (d *smtpDunningDelivery) Send(ctx context.Context, message dunningEmail) error {
	if err := d.Validate(); err != nil {
		return fmt.Errorf("SMTP is not configured: %w", err)
	}
	payload, err := buildDunningRFC822Email(message)
	if err != nil {
		return err
	}
	auth := smtp.Auth(nil)
	if d.config.Username != "" {
		auth = smtp.PlainAuth("", d.config.Username, d.config.Password, d.config.Host)
	}
	address := d.config.Host + ":" + strconv.Itoa(d.config.Port)
	result := make(chan error, 1)
	go func() {
		result <- smtp.SendMail(address, auth, d.config.FromEmail, []string{message.To}, payload)
	}()
	select {
	case err := <-result:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (d *smtpDunningDelivery) Validate() error {
	return validateDunningSMTPConfig(d.config)
}
