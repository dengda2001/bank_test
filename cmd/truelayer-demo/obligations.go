package main

import (
	"context"
	"errors"
	"sort"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type rentObligation struct {
	ID                  uint64 `gorm:"primaryKey"`
	UserID              uint64
	TenantID            uint64
	PeriodMonth         time.Time
	DueDate             time.Time
	ExpectedAmountCents int64
	PaidAmountCents     int64
	Currency            string
	Status              string
	GeneratedBy         string
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

type obligationService struct {
	db *gorm.DB
}

func newObligationService(db *gorm.DB) *obligationService {
	return &obligationService{db: db}
}

func monthStart(t time.Time) time.Time {
	utc := t.UTC()
	return time.Date(utc.Year(), utc.Month(), 1, 0, 0, 0, 0, time.UTC)
}

func parsePeriodMonth(value string) (time.Time, error) {
	if value == "" {
		return monthStart(time.Now()), nil
	}
	if t, err := time.Parse("2006-01", value); err == nil {
		return monthStart(t), nil
	}
	t, err := time.Parse(dateLayout, value)
	if err != nil {
		return time.Time{}, err
	}
	return monthStart(t), nil
}

func tenantActiveInMonth(row tenant, periodMonth time.Time) bool {
	start := monthStart(periodMonth)
	end := start.AddDate(0, 1, 0).Add(-time.Nanosecond)
	if row.Status != "active" {
		return false
	}
	if row.RentStartDate.After(end) {
		return false
	}
	if !row.BillingStartDate.IsZero() && row.BillingStartDate.After(end) {
		return false
	}
	if row.RentEndDate != nil && row.RentEndDate.Before(start) {
		return false
	}
	return true
}

func dueDateForMonth(periodMonth time.Time, dueDay int) time.Time {
	if dueDay < 1 {
		dueDay = 1
	}
	last := periodMonth.AddDate(0, 1, -1).Day()
	if dueDay > last {
		dueDay = last
	}
	return time.Date(periodMonth.Year(), periodMonth.Month(), dueDay, 0, 0, 0, 0, time.UTC)
}

func obligationStatus(expected, paid int64, dueDate, now time.Time, needsReview bool) string {
	if needsReview {
		return "needs_review"
	}
	if paid >= expected {
		return "paid"
	}
	if paid > 0 {
		return "partial"
	}
	if now.After(dueDate) {
		return "overdue"
	}
	return "open"
}

func (s *obligationService) ensureMonthlyObligations(ctx context.Context, userID uint64, periodMonth time.Time) error {
	if userID == 0 {
		return errors.New("userID is required")
	}
	periodMonth = monthStart(periodMonth)
	var tenants []tenant
	if err := s.db.WithContext(ctx).Where("user_id = ?", userID).Find(&tenants).Error; err != nil {
		return err
	}
	for _, row := range tenants {
		if !tenantActiveInMonth(row, periodMonth) {
			continue
		}
		obligation := rentObligation{
			UserID:              userID,
			TenantID:            row.ID,
			PeriodMonth:         periodMonth,
			DueDate:             dueDateForMonth(periodMonth, row.DueDay),
			ExpectedAmountCents: row.MonthlyRentCents,
			PaidAmountCents:     0,
			Currency:            row.Currency,
			Status:              "open",
			GeneratedBy:         "lazy",
		}
		if err := s.db.WithContext(ctx).Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "user_id"}, {Name: "tenant_id"}, {Name: "period_month"}},
			DoNothing: true,
		}).Create(&obligation).Error; err != nil {
			return err
		}
	}
	return nil
}

func (s *obligationService) generateMonthlyObligations(ctx context.Context, userID uint64, fromMonth, toMonth time.Time) error {
	fromMonth = monthStart(fromMonth)
	toMonth = monthStart(toMonth)
	for current := fromMonth; !current.After(toMonth); current = current.AddDate(0, 1, 0) {
		if err := s.ensureMonthlyObligations(ctx, userID, current); err != nil {
			return err
		}
	}
	return nil
}

type rentDashboardSummary struct {
	Rows          []rentDashboardRow
	ExpectedCents int64
	PaidCents     int64
	BalanceCents  int64
	ExpenseCents  int64
	OpenCount     int
	PartialCount  int
	PaidCount     int
	ReviewCount   int
	TenantCount   int
	IncomeCount   int
	ExpenseCount  int
	Currency      string
}

func (s *obligationService) summarizeRentDashboard(ctx context.Context, userID uint64, periodMonth time.Time) (rentDashboardSummary, error) {
	periodMonth = monthStart(periodMonth)
	if err := s.ensureMonthlyObligations(ctx, userID, periodMonth); err != nil {
		return rentDashboardSummary{}, err
	}
	var obligations []rentObligation
	if err := s.db.WithContext(ctx).Where("user_id = ? AND period_month = ?", userID, periodMonth).Order("tenant_id ASC").Find(&obligations).Error; err != nil {
		return rentDashboardSummary{}, err
	}
	var tenants []tenant
	if err := s.db.WithContext(ctx).Where("user_id = ?", userID).Find(&tenants).Error; err != nil {
		return rentDashboardSummary{}, err
	}
	tenantByID := make(map[uint64]tenant, len(tenants))
	for _, row := range tenants {
		tenantByID[row.ID] = row
	}
	summary := rentDashboardSummary{
		Rows:        make([]rentDashboardRow, 0, len(obligations)),
		TenantCount: len(tenants),
	}
	for _, obligation := range obligations {
		tenantRow, ok := tenantByID[obligation.TenantID]
		if !ok {
			continue
		}
		status := obligationStatus(obligation.ExpectedAmountCents, obligation.PaidAmountCents, obligation.DueDate, time.Now().UTC(), obligation.Status == "needs_review")
		if status != obligation.Status {
			if err := s.db.WithContext(ctx).Model(&rentObligation{}).Where("id = ? AND user_id = ?", obligation.ID, userID).Update("status", status).Error; err != nil {
				return rentDashboardSummary{}, err
			}
		}
		currency := firstNonEmpty(obligation.Currency, "EUR")
		if summary.Currency == "" {
			summary.Currency = currency
		}
		summary.ExpectedCents += obligation.ExpectedAmountCents
		summary.PaidCents += obligation.PaidAmountCents
		if obligation.ExpectedAmountCents > obligation.PaidAmountCents {
			summary.BalanceCents += obligation.ExpectedAmountCents - obligation.PaidAmountCents
		}
		switch status {
		case "paid":
			summary.PaidCount++
		case "partial":
			summary.PartialCount++
		case "needs_review":
			summary.ReviewCount++
		default:
			summary.OpenCount++
		}
		payments, err := s.listRentPayments(ctx, userID, obligation.ID)
		if err != nil {
			return rentDashboardSummary{}, err
		}
		summary.Rows = append(summary.Rows, rentDashboardRow{
			TenantName:     tenantRow.Name,
			RoomLabel:      tenantRow.RoomLabel,
			RoomAddress:    tenantRow.RoomAddress,
			Period:         periodMonth.Format("2006-01"),
			DueDate:        obligation.DueDate.Format(dateLayout),
			ExpectedAmount: formatMoney(centsToMoney(obligation.ExpectedAmountCents), currency, 2),
			PaidAmount:     formatMoney(centsToMoney(obligation.PaidAmountCents), currency, 2),
			BalanceAmount:  formatMoney(centsToMoney(maxInt64(obligation.ExpectedAmountCents-obligation.PaidAmountCents, 0)), currency, 2),
			Status:         status,
			StatusLabel:    rentStatusLabel(status),
			ObligationID:   obligation.ID,
			Payments:       payments,
		})
	}
	sort.SliceStable(summary.Rows, func(i, j int) bool {
		priority := map[string]int{"overdue": 0, "needs_review": 1, "partial": 2, "open": 3, "paid": 4}
		return priority[summary.Rows[i].Status] < priority[summary.Rows[j].Status]
	})
	start := periodMonth
	end := start.AddDate(0, 1, 0)
	var transactions []paymentTransaction
	if err := s.db.WithContext(ctx).Where("user_id = ? AND transaction_time >= ? AND transaction_time < ?", userID, start, end).Find(&transactions).Error; err != nil {
		return rentDashboardSummary{}, err
	}
	for _, transaction := range transactions {
		if transaction.Direction == "expense" {
			summary.ExpenseCents += transaction.AmountCents
			summary.ExpenseCount++
		} else if transaction.Direction == "income" {
			summary.IncomeCount++
		}
	}
	return summary, nil
}

type rentPaymentDetailRow struct {
	AmountCents        int64      `gorm:"column:amount_cents"`
	Currency           string     `gorm:"column:currency"`
	TransactionTime    *time.Time `gorm:"column:transaction_time"`
	Description        string     `gorm:"column:description"`
	Reference          string     `gorm:"column:reference"`
	ConfirmationSource string     `gorm:"column:confirmation_source"`
}

func (s *obligationService) listRentPayments(ctx context.Context, userID, obligationID uint64) ([]rentPaymentDetail, error) {
	var rows []rentPaymentDetailRow
	if err := s.db.WithContext(ctx).Table("payment_allocations AS pa").
		Select("pa.amount_cents, pt.currency, pt.transaction_time, pt.description, pt.reference, pa.confirmation_source").
		Joins("JOIN payment_transactions AS pt ON pt.id = pa.payment_transaction_id AND pt.user_id = pa.user_id").
		Where("pa.user_id = ? AND pa.rent_obligation_id = ?", userID, obligationID).
		Order("pt.transaction_time ASC, pa.id ASC").Scan(&rows).Error; err != nil {
		return nil, err
	}
	payments := make([]rentPaymentDetail, 0, len(rows))
	for _, row := range rows {
		dateDisplay := "Unknown"
		if row.TransactionTime != nil {
			dateDisplay = row.TransactionTime.UTC().Format("02 Jan 2006 15:04")
		}
		payments = append(payments, rentPaymentDetail{
			AmountDisplay:      formatMoney(centsToMoney(row.AmountCents), firstNonEmpty(row.Currency, "EUR"), 2),
			DateDisplay:        dateDisplay,
			Description:        firstNonEmpty(row.Description, "No description"),
			Reference:          firstNonEmpty(row.Reference, "No reference"),
			ConfirmationSource: firstNonEmpty(row.ConfirmationSource, "manual"),
		})
	}
	return payments, nil
}

func maxInt64(value, minimum int64) int64 {
	if value < minimum {
		return minimum
	}
	return value
}

func rentStatusLabel(status string) string {
	return map[string]string{
		"open":         "未缴",
		"overdue":      "已逾期",
		"partial":      "部分缴纳",
		"paid":         "已缴清",
		"needs_review": "需处理",
	}[status]
}
