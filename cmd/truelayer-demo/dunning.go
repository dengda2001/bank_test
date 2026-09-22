package main

import (
	"errors"
	"fmt"
	"net/mail"
	"strings"
	"time"
)

const (
	dunningTemplateReminder = "rent_reminder"
	dunningTemplateOverdue  = "rent_overdue"

	dunningDeliveryAccepted = "accepted"
	dunningDeliverySent     = "sent"
	dunningDeliveryFailed   = "failed"
	dunningDeliverySkipped  = "skipped"
)

type dunningSenderConfig struct {
	ID           uint64 `gorm:"primaryKey"`
	UserID       uint64
	DisplayName  string
	ReplyToEmail string
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

type dunningSendAttempt struct {
	ID                  uint64 `gorm:"primaryKey"`
	UserID              uint64
	TenantID            uint64
	RentObligationID    uint64
	PeriodMonth         time.Time
	RecipientEmail      string
	TemplateKind        string
	Subject             string
	Body                string
	ExpectedAmountCents int64
	PaidAmountCents     int64
	BalanceAmountCents  int64
	Currency            string
	SenderDisplayName   string
	ReplyToEmail        string
	ServiceFromEmail    string
	DeliveryStatus      string
	ProviderMessageID   *string
	ErrorMessage        *string
	OperationID         string
	RequestKey          string
	RetryOfAttemptID    *uint64
	RequestedAt         time.Time
	SentAt              *time.Time
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

type dunningCandidate struct {
	TenantID          uint64
	ObligationID      uint64
	TenantName        string
	TenantAlias       string
	RoomLabel         string
	RoomAddress       string
	Period            string
	DueDate           string
	DueDateValue      time.Time
	ExpectedAmount    string
	PaidAmount        string
	BalanceAmount     string
	ExpectedCents     int64
	PaidCents         int64
	BalanceCents      int64
	Currency          string
	Status            string
	StatusLabel       string
	Email             string
	EmailValid        bool
	EmailError        string
	Selectable        bool
	DefaultSelected   bool
	Selected          bool
	LastDunningAt     *time.Time
	LastDunningStatus string
	SentToday         bool
}

type dunningMessage struct {
	RecipientEmail string
	TemplateKind   string
	Subject        string
	Body           string
	Period         string
	DueDate        string
	BalanceCents   int64
	Currency       string
	OverdueDays    int
	SenderDisplay  string
	ReplyToEmail   string
}

func validateDunningSenderConfig(config dunningSenderConfig) error {
	displayName := strings.TrimSpace(config.DisplayName)
	if displayName == "" {
		return errors.New("sender display name is required")
	}
	if len([]rune(displayName)) > 191 {
		return errors.New("sender display name is too long")
	}
	if strings.ContainsAny(displayName, "\r\n") {
		return errors.New("sender display name contains invalid header characters")
	}
	replyTo := strings.TrimSpace(config.ReplyToEmail)
	parsed, err := mail.ParseAddress(replyTo)
	if err != nil || parsed.Address != replyTo || strings.ContainsAny(replyTo, "\r\n") {
		return errors.New("reply-to email must be a valid email address")
	}
	if len([]rune(replyTo)) > 191 {
		return errors.New("reply-to email is too long")
	}
	return nil
}

func validateDunningRecipient(value string) error {
	value = strings.TrimSpace(value)
	parsed, err := mail.ParseAddress(value)
	if err != nil || parsed.Address != value || strings.ContainsAny(value, "\r\n") {
		return errors.New("recipient email is invalid")
	}
	return nil
}

func dunningDublinDate(value time.Time) time.Time {
	location, err := time.LoadLocation("Europe/Dublin")
	if err != nil {
		location = time.UTC
	}
	local := value.In(location)
	return time.Date(local.Year(), local.Month(), local.Day(), 0, 0, 0, 0, time.UTC)
}

func dunningStoredDate(value time.Time) time.Time {
	utc := value.UTC()
	return time.Date(utc.Year(), utc.Month(), utc.Day(), 0, 0, 0, 0, time.UTC)
}

func dunningDaysOverdue(dueDate, now time.Time) int {
	days := int(dunningDublinDate(now).Sub(dunningStoredDate(dueDate)).Hours() / 24)
	if days < 0 {
		return 0
	}
	return days
}

func dunningTemplateKind(dueDate, now time.Time) (string, int) {
	days := dunningDaysOverdue(dueDate, now)
	if days > 0 {
		return dunningTemplateOverdue, days
	}
	return dunningTemplateReminder, 0
}

func buildDunningMessage(candidate dunningCandidate, sender dunningSenderConfig, now time.Time) (dunningMessage, error) {
	if candidate.BalanceCents <= 0 {
		return dunningMessage{}, errors.New("dunning balance must be positive")
	}
	if err := validateDunningRecipient(candidate.Email); err != nil {
		return dunningMessage{}, err
	}
	if err := validateDunningSenderConfig(sender); err != nil {
		return dunningMessage{}, err
	}
	period, err := parsePeriodMonth(candidate.Period)
	if err != nil {
		return dunningMessage{}, fmt.Errorf("dunning period: %w", err)
	}
	dueDate := dunningStoredDate(candidate.DueDateValue)
	dueDateDisplay := candidate.DueDate
	if dueDateDisplay == "" {
		dueDateDisplay = dueDate.Format(dateLayout)
	}
	periodLabel := formatMonthLabel(period)
	englishPeriodLabel := fmt.Sprintf("%s %d", period.Month().String(), period.Year())
	amount := formatMoney(centsToMoney(candidate.BalanceCents), candidate.Currency, 2)
	templateKind, overdueDays := dunningTemplateKind(dueDate, now)
	message := dunningMessage{
		RecipientEmail: candidate.Email,
		TemplateKind:   templateKind,
		Period:         candidate.Period,
		DueDate:        dueDateDisplay,
		BalanceCents:   candidate.BalanceCents,
		Currency:       candidate.Currency,
		OverdueDays:    overdueDays,
		SenderDisplay:  sender.DisplayName,
		ReplyToEmail:   sender.ReplyToEmail,
	}
	name := strings.TrimSpace(candidate.TenantName)
	switch templateKind {
	case dunningTemplateOverdue:
		message.Subject = sanitizeMailHeaderText(fmt.Sprintf("Rent overdue for %s", englishPeriodLabel))
		message.Body = fmt.Sprintf("Dear %s,\n\nOur records show an outstanding rent balance of %s for %s. The rent was due on %s and is %d day overdue.\n\nPlease arrange payment at your earliest convenience. If you have already paid, please reply with the payment details.\n\nKind regards,\n%s", name, amount, periodLabel, dueDateDisplay, overdueDays, sender.DisplayName)
	default:
		message.Subject = sanitizeMailHeaderText(fmt.Sprintf("Rent reminder for %s", englishPeriodLabel))
		message.Body = fmt.Sprintf("Dear %s,\n\nThis is a reminder that rent of %s for %s is due on %s.\n\nPlease arrange payment by the due date. If you have already paid, please reply with the payment details.\n\nKind regards,\n%s", name, amount, periodLabel, dueDateDisplay, sender.DisplayName)
	}
	return message, nil
}

func sanitizeMailHeaderText(value string) string {
	return strings.NewReplacer("\r", " ", "\n", " ").Replace(strings.TrimSpace(value))
}

func buildDunningCandidate(obligation rentObligation, tenantRow tenant, now time.Time, last *dunningSendAttempt, charges ...rentCharge) dunningCandidate {
	currency := firstNonEmpty(obligation.Currency, ledgerCurrencyEUR)
	roomLabel, roomAddress := "", ""
	if len(charges) > 0 {
		roomLabel = stringValue(charges[0].RoomLabelSnapshot)
		roomAddress = stringValue(charges[0].RoomAddressSnapshot)
	}
	balance := maxInt64(obligation.ExpectedAmountCents-obligation.PaidAmountCents, 0)
	status := obligationStatus(obligation.ExpectedAmountCents, obligation.PaidAmountCents, obligation.DueDate, now, obligation.Status == "needs_review")
	if obligation.RecordStatus == obligationRecordVoided {
		status = obligationRecordVoided
	}
	email := strings.TrimSpace(tenantRow.Email)
	emailErr := validateDunningRecipient(email)
	candidate := dunningCandidate{
		TenantID:       tenantRow.ID,
		ObligationID:   obligation.ID,
		TenantName:     tenantRow.Name,
		TenantAlias:    tenantRow.DisplayAlias,
		RoomLabel:      roomLabel,
		RoomAddress:    roomAddress,
		Period:         monthStart(obligation.PeriodMonth).Format("2006-01"),
		DueDate:        dunningStoredDate(obligation.DueDate).Format(dateLayout),
		DueDateValue:   obligation.DueDate,
		ExpectedAmount: formatMoney(centsToMoney(obligation.ExpectedAmountCents), currency, 2),
		PaidAmount:     formatMoney(centsToMoney(obligation.PaidAmountCents), currency, 2),
		BalanceAmount:  formatMoney(centsToMoney(balance), currency, 2),
		ExpectedCents:  obligation.ExpectedAmountCents,
		PaidCents:      obligation.PaidAmountCents,
		BalanceCents:   balance,
		Currency:       currency,
		Status:         status,
		StatusLabel:    rentStatusLabel(status),
		Email:          email,
		EmailValid:     emailErr == nil,
		Selectable:     balance > 0 && emailErr == nil && status != obligationRecordVoided,
	}
	if emailErr != nil {
		candidate.EmailError = emailErr.Error()
	}
	if last != nil {
		if last.SentAt != nil {
			candidate.LastDunningAt = last.SentAt
		} else if !last.RequestedAt.IsZero() {
			requestedAt := last.RequestedAt
			candidate.LastDunningAt = &requestedAt
		}
		candidate.LastDunningStatus = last.DeliveryStatus
		candidate.SentToday = last.DeliveryStatus == dunningDeliverySent && last.SentAt != nil && dunningDublinDate(*last.SentAt).Equal(dunningDublinDate(now))
	}
	candidate.DefaultSelected = candidate.Selectable && !candidate.SentToday
	return candidate
}
