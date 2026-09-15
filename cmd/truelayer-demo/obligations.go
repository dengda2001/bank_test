package main

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
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
	RecordStatus        string
	VoidedAt            *time.Time
	VoidedByUserID      *uint64
	VoidReason          *string
	GeneratedBy         string
	CreatedAt           time.Time
	UpdatedAt           time.Time
}

type obligationService struct {
	db *gorm.DB
}

type tenantBillingMonth struct {
	ObligationID   uint64
	Period         string
	PeriodLabel    string
	DueDate        string
	ExpectedAmount string
	PaidAmount     string
	BalanceAmount  string
	Status         string
	StatusLabel    string
	Payments       []rentPaymentDetail
}

type tenantBillingPaymentRow struct {
	PaymentID          uint64     `gorm:"column:payment_id"`
	TenantID           uint64     `gorm:"column:tenant_id"`
	ObligationID       uint64     `gorm:"column:obligation_id"`
	AmountCents        int64      `gorm:"column:amount_cents"`
	Currency           string     `gorm:"column:currency"`
	Source             string     `gorm:"column:source"`
	TransactionTime    *time.Time `gorm:"column:transaction_time"`
	Description        string     `gorm:"column:description"`
	Reference          string     `gorm:"column:reference"`
	ConfirmationSource string     `gorm:"column:confirmation_source"`
}

func sortTenantBillingPaymentRows(rows []tenantBillingPaymentRow) {
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].ObligationID != rows[j].ObligationID {
			return rows[i].ObligationID < rows[j].ObligationID
		}
		if rows[i].TransactionTime == nil && rows[j].TransactionTime != nil {
			return true
		}
		if rows[i].TransactionTime != nil && rows[j].TransactionTime == nil {
			return false
		}
		if rows[i].TransactionTime != nil && !rows[i].TransactionTime.Equal(*rows[j].TransactionTime) {
			return rows[i].TransactionTime.Before(*rows[j].TransactionTime)
		}
		if rows[i].Source != rows[j].Source {
			return rows[i].Source < rows[j].Source
		}
		return rows[i].Reference < rows[j].Reference
	})
}

func (s *obligationService) listCashBillingPaymentRows(ctx context.Context, userID, tenantID uint64, fromMonth, toMonth time.Time) ([]tenantBillingPaymentRow, error) {
	query := s.db.WithContext(ctx).Table("cash_receipts AS cr").
		Select("cr.id AS payment_id, cr.tenant_id, cr.rent_obligation_id AS obligation_id, cr.amount_cents, cr.currency, 'cash' AS source, cr.received_at AS transaction_time, '现金租金补录' AS description, cr.receipt_number AS reference, 'manual_cash' AS confirmation_source").
		Joins("JOIN rent_obligations AS ro ON ro.id = cr.rent_obligation_id AND ro.user_id = cr.user_id").
		Where("cr.user_id = ? AND cr.status = ? AND ro.period_month >= ? AND ro.period_month < ?", userID, cashReceiptStatusConfirmed, fromMonth, toMonth.AddDate(0, 1, 0))
	if tenantID != 0 {
		query = query.Where("cr.tenant_id = ?", tenantID)
	}
	var rows []tenantBillingPaymentRow
	if err := query.Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func newObligationService(db *gorm.DB) *obligationService {
	return &obligationService{db: db}
}

func monthStart(t time.Time) time.Time {
	utc := t.UTC()
	return time.Date(utc.Year(), utc.Month(), 1, 0, 0, 0, 0, time.UTC)
}

func formatMonthLabel(month time.Time) string {
	month = monthStart(month)
	return fmt.Sprintf("%d年%d月", month.Year(), month.Month())
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
	return ledgerObligationStatus(expected, paid, dueDate, now, obligationRecordActive)
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
		currency, err := normalizeLedgerCurrency(row.Currency)
		if err != nil {
			return fmt.Errorf("tenant %d: %w", row.ID, err)
		}
		obligation := rentObligation{
			UserID:              userID,
			TenantID:            row.ID,
			PeriodMonth:         periodMonth,
			DueDate:             dueDateForMonth(periodMonth, row.DueDay),
			ExpectedAmountCents: row.MonthlyRentCents,
			PaidAmountCents:     0,
			Currency:            currency,
			Status:              "open",
			RecordStatus:        obligationRecordActive,
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

func (s *obligationService) listTenantBillingHistory(ctx context.Context, userID uint64, endMonth time.Time, monthCount int) (map[uint64][]tenantBillingMonth, error) {
	if userID == 0 {
		return nil, errors.New("userID is required")
	}
	if monthCount < 1 {
		return nil, errors.New("monthCount must be positive")
	}
	endMonth = monthStart(endMonth)
	startMonth := endMonth.AddDate(0, -(monthCount - 1), 0)
	if err := s.generateMonthlyObligations(ctx, userID, startMonth, endMonth); err != nil {
		return nil, err
	}

	var tenants []tenant
	if err := s.db.WithContext(ctx).Where("user_id = ?", userID).Find(&tenants).Error; err != nil {
		return nil, err
	}
	var obligations []rentObligation
	if err := s.db.WithContext(ctx).
		Where("user_id = ? AND period_month >= ? AND period_month < ?", userID, startMonth, endMonth.AddDate(0, 1, 0)).
		Order("tenant_id ASC, period_month DESC").Find(&obligations).Error; err != nil {
		return nil, err
	}
	var paymentRows []tenantBillingPaymentRow
	if err := s.db.WithContext(ctx).Table("payment_allocations AS pa").
		Select("pa.tenant_id, pa.rent_obligation_id AS obligation_id, pa.amount_cents, pt.currency, pt.source, pt.transaction_time, pt.description, pt.reference, pa.confirmation_source").
		Joins("JOIN rent_obligations AS ro ON ro.id = pa.rent_obligation_id AND ro.user_id = pa.user_id").
		Joins("JOIN payment_transactions AS pt ON pt.id = pa.payment_transaction_id AND pt.user_id = pa.user_id").
		Where("pa.user_id = ? AND pa.status = ? AND pa.allocation_kind = ? AND pt.direction = ? AND ro.period_month >= ? AND ro.period_month < ?", userID, allocationStatusConfirmed, allocationKindRent, "income", startMonth, endMonth.AddDate(0, 1, 0)).
		Order("pa.tenant_id ASC, ro.period_month DESC, pt.transaction_time ASC, pa.id ASC").
		Scan(&paymentRows).Error; err != nil {
		return nil, err
	}
	cashRows, err := s.listCashBillingPaymentRows(ctx, userID, 0, startMonth, endMonth)
	if err != nil {
		return nil, err
	}
	paymentRows = append(paymentRows, cashRows...)
	sortTenantBillingPaymentRows(paymentRows)
	return buildTenantBillingHistory(tenants, obligations, paymentRows, time.Now().UTC()), nil
}

func buildTenantBillingHistory(tenants []tenant, obligations []rentObligation, paymentRows []tenantBillingPaymentRow, now time.Time) map[uint64][]tenantBillingMonth {
	tenantCurrency := make(map[uint64]string, len(tenants))
	for _, row := range tenants {
		tenantCurrency[row.ID] = firstNonEmpty(row.Currency, "EUR")
	}
	paymentsByObligation := make(map[uint64][]rentPaymentDetail)
	for _, row := range paymentRows {
		paymentsByObligation[row.ObligationID] = append(paymentsByObligation[row.ObligationID], rentPaymentDetailFromRow(rentPaymentDetailRow{
			PaymentID:          row.PaymentID,
			AmountCents:        row.AmountCents,
			Currency:           row.Currency,
			Source:             row.Source,
			TransactionTime:    row.TransactionTime,
			Description:        row.Description,
			Reference:          row.Reference,
			ConfirmationSource: row.ConfirmationSource,
		}))
	}
	history := make(map[uint64][]tenantBillingMonth)
	for _, obligation := range obligations {
		if _, ok := tenantCurrency[obligation.TenantID]; !ok {
			continue
		}
		currency := firstNonEmpty(obligation.Currency, tenantCurrency[obligation.TenantID], "EUR")
		status := obligationStatus(obligation.ExpectedAmountCents, obligation.PaidAmountCents, obligation.DueDate, now, obligation.Status == "needs_review")
		if obligation.RecordStatus == obligationRecordVoided {
			status = obligationRecordVoided
		}
		period := monthStart(obligation.PeriodMonth)
		history[obligation.TenantID] = append(history[obligation.TenantID], tenantBillingMonth{
			ObligationID:   obligation.ID,
			Period:         period.Format("2006-01"),
			PeriodLabel:    formatMonthLabel(period),
			DueDate:        obligation.DueDate.Format(dateLayout),
			ExpectedAmount: formatMoney(centsToMoney(obligation.ExpectedAmountCents), currency, 2),
			PaidAmount:     formatMoney(centsToMoney(obligation.PaidAmountCents), currency, 2),
			BalanceAmount:  formatMoney(centsToMoney(maxInt64(obligation.ExpectedAmountCents-obligation.PaidAmountCents, 0)), currency, 2),
			Status:         status,
			StatusLabel:    rentStatusLabel(status),
			Payments:       paymentsByObligation[obligation.ID],
		})
	}
	for tenantID := range history {
		sort.SliceStable(history[tenantID], func(i, j int) bool {
			return history[tenantID][i].Period > history[tenantID][j].Period
		})
	}
	return history
}

type rentDashboardSummary struct {
	Rows                       []rentDashboardRow
	TotalRows                  int
	FilteredCount              int
	TotalPages                 int
	Page                       int
	PageSize                   int
	ExpectedCents              int64
	PaidCents                  int64
	BalanceCents               int64
	ExpenseCents               int64
	OpenCount                  int
	OverdueCount               int
	UnpaidCount                int
	PartialCount               int
	PaidCount                  int
	ReviewCount                int
	TenantCount                int
	IncomeCount                int
	ExpenseCount               int
	Currency                   string
	PendingCents               int64
	PendingCount               int
	OtherIncomeCents           int64
	OtherIncomeCount           int
	SyncCoverage               string
	SyncStatus                 string
	LastSuccessfulSyncCoverage string
}

func (s *obligationService) summarizeRentDashboard(ctx context.Context, userID uint64, periodMonth time.Time) (rentDashboardSummary, error) {
	return s.summarizeRentDashboardWithFilters(ctx, userID, periodMonth, defaultRentDashboardFilters())
}

func (s *obligationService) summarizeRentDashboardWithFilters(ctx context.Context, userID uint64, periodMonth time.Time, filters rentDashboardFilters) (rentDashboardSummary, error) {
	if err := validateRentDashboardFilters(filters); err != nil {
		return rentDashboardSummary{}, err
	}
	periodMonth = monthStart(periodMonth)
	if err := s.ensureMonthlyObligations(ctx, userID, periodMonth); err != nil {
		return rentDashboardSummary{}, err
	}
	var obligations []rentObligation
	if err := s.db.WithContext(ctx).Where("user_id = ? AND period_month = ? AND record_status = ?", userID, periodMonth, obligationRecordActive).Order("tenant_id ASC").Find(&obligations).Error; err != nil {
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
	now := time.Now().UTC()
	allRows := make([]rentDashboardRow, 0, len(obligations))
	for _, obligation := range obligations {
		tenantRow, ok := tenantByID[obligation.TenantID]
		if !ok {
			continue
		}
		status := obligationStatus(obligation.ExpectedAmountCents, obligation.PaidAmountCents, obligation.DueDate, now, obligation.Status == "needs_review")
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
		case "overdue":
			summary.OverdueCount++
		default:
			summary.OpenCount++
		}
		payments, err := s.listRentPayments(ctx, userID, obligation.ID)
		if err != nil {
			return rentDashboardSummary{}, err
		}
		allRows = append(allRows, rentDashboardRow{
			TenantID:       tenantRow.ID,
			TenantName:     tenantRow.Name,
			TenantAlias:    tenantRow.DisplayAlias,
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
			ExpectedCents:  obligation.ExpectedAmountCents,
			PaidCents:      obligation.PaidAmountCents,
			DueDateValue:   obligation.DueDate,
		})
	}
	summary.TotalRows = len(allRows)
	summary.UnpaidCount = summary.OpenCount + summary.OverdueCount + summary.PartialCount
	filteredRows := filterAndSortRentDashboardRows(allRows, filters)
	pageRows, totalPages := paginateRentDashboardRows(filteredRows, filters.Page, filters.PageSize)
	summary.Rows = pageRows
	summary.FilteredCount = len(filteredRows)
	summary.TotalPages = totalPages
	summary.Page = filters.Page
	summary.PageSize = filters.PageSize
	start := periodMonth
	end := start.AddDate(0, 1, 0)
	var transactions []paymentTransaction
	if err := s.db.WithContext(ctx).Where("user_id = ? AND transaction_time >= ? AND transaction_time < ?", userID, start, end).Find(&transactions).Error; err != nil {
		return rentDashboardSummary{}, err
	}
	incomeTransactions := make([]paymentTransaction, 0, len(transactions))
	for _, transaction := range transactions {
		if transaction.Direction == "expense" {
			summary.ExpenseCents += transaction.AmountCents
			summary.ExpenseCount++
		} else if transaction.Direction == "income" {
			summary.IncomeCount++
			incomeTransactions = append(incomeTransactions, transaction)
		}
	}
	var incomeAllocations []paymentAllocation
	if len(incomeTransactions) > 0 {
		transactionIDs := make([]uint64, 0, len(incomeTransactions))
		for _, transaction := range incomeTransactions {
			transactionIDs = append(transactionIDs, transaction.ID)
		}
		if err := s.db.WithContext(ctx).Where("user_id = ? AND payment_transaction_id IN ?", userID, transactionIDs).Find(&incomeAllocations).Error; err != nil {
			return rentDashboardSummary{}, err
		}
	}
	bankMetrics := summarizeRentDashboardBankMetrics(incomeTransactions, incomeAllocations)
	summary.PendingCents = bankMetrics.PendingCents
	summary.PendingCount = bankMetrics.PendingCount
	summary.OtherIncomeCents = bankMetrics.OtherIncomeCents
	summary.OtherIncomeCount = bankMetrics.OtherIncomeCount
	coverage, err := latestBankSyncCoverage(ctx, s.db, userID)
	if err != nil {
		return rentDashboardSummary{}, err
	}
	summary.SyncCoverage = coverage
	status, err := latestBankSyncStatus(ctx, s.db, userID)
	if err != nil {
		return rentDashboardSummary{}, err
	}
	summary.SyncStatus = status
	if summary.SyncStatus != bankSyncStatusSucceeded {
		lastSuccessful, err := latestSuccessfulBankSyncCoverage(ctx, s.db, userID)
		if err != nil {
			return rentDashboardSummary{}, err
		}
		summary.LastSuccessfulSyncCoverage = lastSuccessful
	} else {
		summary.LastSuccessfulSyncCoverage = summary.SyncCoverage
	}
	return summary, nil
}

type rentPaymentDetailRow struct {
	PaymentID          uint64     `gorm:"column:payment_id"`
	AmountCents        int64      `gorm:"column:amount_cents"`
	Currency           string     `gorm:"column:currency"`
	Source             string     `gorm:"column:source"`
	TransactionTime    *time.Time `gorm:"column:transaction_time"`
	Description        string     `gorm:"column:description"`
	Reference          string     `gorm:"column:reference"`
	ConfirmationSource string     `gorm:"column:confirmation_source"`
}

func sortRentPaymentDetailRows(rows []rentPaymentDetailRow) {
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].TransactionTime == nil && rows[j].TransactionTime != nil {
			return true
		}
		if rows[i].TransactionTime != nil && rows[j].TransactionTime == nil {
			return false
		}
		if rows[i].TransactionTime != nil && !rows[i].TransactionTime.Equal(*rows[j].TransactionTime) {
			return rows[i].TransactionTime.Before(*rows[j].TransactionTime)
		}
		if rows[i].Source != rows[j].Source {
			return rows[i].Source < rows[j].Source
		}
		return rows[i].Reference < rows[j].Reference
	})
}

func (s *obligationService) listCashRentPaymentRows(ctx context.Context, userID, obligationID uint64) ([]rentPaymentDetailRow, error) {
	var rows []rentPaymentDetailRow
	if err := s.db.WithContext(ctx).Table("cash_receipts AS cr").
		Select("cr.id AS payment_id, cr.amount_cents, cr.currency, 'cash' AS source, cr.received_at AS transaction_time, '现金租金补录' AS description, cr.receipt_number AS reference, 'manual_cash' AS confirmation_source").
		Where("cr.user_id = ? AND cr.rent_obligation_id = ? AND cr.status = ?", userID, obligationID, cashReceiptStatusConfirmed).
		Order("cr.received_at ASC, cr.id ASC").Scan(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (s *obligationService) listRentPayments(ctx context.Context, userID, obligationID uint64) ([]rentPaymentDetail, error) {
	var rows []rentPaymentDetailRow
	if err := s.db.WithContext(ctx).Table("payment_allocations AS pa").
		Select("pa.amount_cents, pt.currency, pt.source, pt.transaction_time, pt.description, pt.reference, pa.confirmation_source").
		Joins("JOIN payment_transactions AS pt ON pt.id = pa.payment_transaction_id AND pt.user_id = pa.user_id").
		Where("pa.user_id = ? AND pa.rent_obligation_id = ? AND pa.status = ? AND pa.allocation_kind = ?", userID, obligationID, allocationStatusConfirmed, allocationKindRent).
		Order("pt.transaction_time ASC, pa.id ASC").Scan(&rows).Error; err != nil {
		return nil, err
	}
	cashRows, err := s.listCashRentPaymentRows(ctx, userID, obligationID)
	if err != nil {
		return nil, err
	}
	rows = append(rows, cashRows...)
	sortRentPaymentDetailRows(rows)
	payments := make([]rentPaymentDetail, 0, len(rows))
	for _, row := range rows {
		payments = append(payments, rentPaymentDetailFromRow(row))
	}
	return payments, nil
}

func rentPaymentDetailFromRow(row rentPaymentDetailRow) rentPaymentDetail {
	dateDisplay := "Unknown"
	if row.TransactionTime != nil {
		dateDisplay = row.TransactionTime.UTC().Format("02 Jan 2006 15:04")
	}
	return rentPaymentDetail{
		PaymentID:          row.PaymentID,
		AmountDisplay:      formatMoney(centsToMoney(row.AmountCents), firstNonEmpty(row.Currency, "EUR"), 2),
		DateDisplay:        dateDisplay,
		Description:        firstNonEmpty(row.Description, "No description"),
		Reference:          firstNonEmpty(row.Reference, "No reference"),
		Source:             paymentSourceLabel(row.Source),
		ConfirmationSource: firstNonEmpty(row.ConfirmationSource, "manual"),
	}
}

func paymentSourceLabel(source string) string {
	switch strings.ToLower(strings.TrimSpace(source)) {
	case "cash", "manual_cash":
		return "现金"
	case "bank", "truelayer", "legacy", "":
		return "银行"
	default:
		return source
	}
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
		"voided":       "已作废",
	}[status]
}
