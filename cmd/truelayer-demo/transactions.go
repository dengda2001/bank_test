package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net/url"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type paymentTransaction struct {
	ID                    uint64 `gorm:"primaryKey"`
	UserID                uint64
	Source                string
	SourceBatchID         *string
	ProviderTransactionID *string
	StableTransactionKey  string
	AccountID             *string
	AccountName           *string
	Direction             string
	AmountCents           int64
	Currency              string
	TransactionTime       *time.Time
	Description           string
	Reference             string
	PayerID               *string
	PayerName             *string
	PayerNameKind         string
	ParsedPeriodMonth     *time.Time
	ParsedPeriodSource    string
	ParsedPeriodNote      string
	MatchReason           string
	MatchedTenantID       *uint64
	MatchStatus           string
	RawPayloadJSON        []byte `gorm:"column:raw_payload_json"`
	CreatedAt             time.Time
	UpdatedAt             time.Time
}

type paymentTransactionInput struct {
	Source                string
	SourceBatchID         string
	ProviderTransactionID string
	StableTransactionKey  string
	AccountID             string
	AccountName           string
	Direction             string
	AmountCents           int64
	Currency              string
	TransactionTime       *time.Time
	Description           string
	Reference             string
	PayerID               string
	PayerName             string
	PayerNameKind         string
	ParsedPeriodMonth     *time.Time
	ParsedPeriodSource    string
	ParsedPeriodNote      string
	MatchReason           string
	MatchStatus           string
	RawPayloadJSON        []byte
}

type transactionService struct {
	db *gorm.DB
}

var pendingMatchStatuses = []string{"candidate", "needs_review", "unmatched", "partial"}

func isPendingMatchStatus(status string) bool {
	for _, pendingStatus := range pendingMatchStatuses {
		if status == pendingStatus {
			return true
		}
	}
	return false
}

func newTransactionService(db *gorm.DB) *transactionService {
	return &transactionService{db: db}
}

func normalizePaymentTransactions(result demoResult) []paymentTransactionInput {
	var rows []paymentTransactionInput
	for _, acct := range result.Accounts {
		if len(acct.Transactions) == 0 {
			continue
		}
		var txs rawTransactionList
		if err := json.Unmarshal(acct.Transactions, &txs); err != nil {
			continue
		}
		for _, tx := range txs.Results {
			direction, ok := transactionDirection(tx)
			if !ok {
				continue
			}
			raw, _ := json.Marshal(tx)
			ts := parseTransactionTime(tx.Timestamp)
			payerName, payerNameKind := payerName(tx)
			payerID := firstNonEmpty(tx.PayerID, tx.RemitterID, tx.CounterpartyID, metaString(tx.Meta, "payer_id"), metaString(tx.Meta, "remitter_id"), metaString(tx.Meta, "counterparty_id"))
			providerID := firstNonEmpty(tx.NormalisedProviderTransactionID, tx.ProviderTransactionID, metaString(tx.Meta, "normalised_provider_transaction_id"), metaString(tx.Meta, "provider_transaction_id"), metaString(tx.Meta, "bank_transaction_id"))
			reference := firstNonEmpty(tx.Reference, metaString(tx.Meta, "payment_reference"), metaString(tx.Meta, "reference"), metaString(tx.Meta, "provider_reference"), metaString(tx.Meta, "remittance_information"), metaString(tx.Meta, "remittanceInformation"))
			parsedPeriod, parsed := parseReferencedPeriod(tx.Description+" "+reference, firstNonZeroTime(ts))
			input := paymentTransactionInput{
				Source:                "truelayer",
				SourceBatchID:         firstNonEmpty(result.SyncRunID, result.FetchedAt),
				ProviderTransactionID: providerID,
				AccountID:             acct.Account.AccountID,
				AccountName:           firstNonEmpty(acct.Account.DisplayName, acct.Account.AccountID),
				Direction:             direction,
				AmountCents:           moneyToCents(mathAbs(tx.Amount)),
				Currency:              firstNonEmpty(tx.Currency, acct.Account.Currency, "EUR"),
				TransactionTime:       ts,
				Description:           tx.Description,
				Reference:             reference,
				PayerID:               payerID,
				PayerName:             payerName,
				PayerNameKind:         firstNonEmpty(payerNameKind, "unknown"),
				ParsedPeriodMonth:     optionalTime(parsedPeriod, parsed),
				ParsedPeriodSource:    parsedPeriodSource(parsed),
				ParsedPeriodNote:      parsedPeriodNote(tx.Description, reference, parsed),
				MatchStatus:           "unmatched",
				RawPayloadJSON:        raw,
			}
			input.StableTransactionKey = stableTransactionKey(input)
			rows = append(rows, input)
		}
	}
	return rows
}

func (s *transactionService) ingestDemoResult(ctx context.Context, userID uint64, result demoResult) error {
	for _, input := range normalizePaymentTransactions(result) {
		row := paymentTransactionFromInput(userID, input)
		if err := s.db.WithContext(ctx).Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "user_id"}, {Name: "stable_transaction_key"}},
			DoNothing: true,
		}).Create(&row).Error; err != nil {
			return err
		}
	}
	return nil
}

func paymentTransactionFromInput(userID uint64, input paymentTransactionInput) paymentTransaction {
	return paymentTransaction{
		UserID:                userID,
		Source:                input.Source,
		SourceBatchID:         nullableString(input.SourceBatchID),
		ProviderTransactionID: nullableString(input.ProviderTransactionID),
		StableTransactionKey:  input.StableTransactionKey,
		AccountID:             nullableString(input.AccountID),
		AccountName:           nullableString(input.AccountName),
		Direction:             input.Direction,
		AmountCents:           input.AmountCents,
		Currency:              input.Currency,
		TransactionTime:       input.TransactionTime,
		Description:           input.Description,
		Reference:             input.Reference,
		PayerID:               nullableString(input.PayerID),
		PayerName:             nullableString(input.PayerName),
		PayerNameKind:         input.PayerNameKind,
		ParsedPeriodMonth:     input.ParsedPeriodMonth,
		ParsedPeriodSource:    input.ParsedPeriodSource,
		ParsedPeriodNote:      input.ParsedPeriodNote,
		MatchReason:           input.MatchReason,
		MatchStatus:           input.MatchStatus,
		RawPayloadJSON:        input.RawPayloadJSON,
	}
}

func transactionDirection(tx rawTransaction) (string, bool) {
	t := strings.ToUpper(strings.TrimSpace(tx.TransactionType))
	switch t {
	case "CREDIT":
		return "income", true
	case "DEBIT":
		return "expense", true
	}
	if tx.Amount > 0 {
		return "income", true
	}
	if tx.Amount < 0 {
		return "expense", true
	}
	return "", false
}

func parseTransactionTime(value string) *time.Time {
	if value == "" {
		return nil
	}
	layouts := []string{time.RFC3339, "2006-01-02T15:04:05", dateLayout}
	for _, layout := range layouts {
		if ts, err := time.Parse(layout, value); err == nil {
			return &ts
		}
	}
	return nil
}

func stableTransactionKey(input paymentTransactionInput) string {
	if input.ProviderTransactionID != "" {
		return "provider:" + input.Source + ":" + input.ProviderTransactionID
	}
	parts := []string{
		input.Source,
		input.AccountID,
		input.Direction,
		strconv.FormatInt(input.AmountCents, 10),
		input.Currency,
		input.Description,
		input.Reference,
	}
	if input.TransactionTime != nil {
		parts = append(parts, input.TransactionTime.UTC().Format(time.RFC3339))
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "|")))
	return "hash:" + hex.EncodeToString(sum[:])
}

func mathAbs(value float64) float64 {
	if value < 0 {
		return -value
	}
	return value
}

func filtersFromQuery(q url.Values) transactionFilters {
	return transactionFilters{
		Direction:   strings.TrimSpace(q.Get("direction")),
		MatchStatus: strings.TrimSpace(q.Get("match_status")),
		PeriodMonth: strings.TrimSpace(q.Get("period")),
		PendingOnly: strings.TrimSpace(q.Get("pending")) == "1",
	}
}

type transactionFilters struct {
	Direction   string
	MatchStatus string
	PeriodMonth string
	PendingOnly bool
}

func validateTransactionFilters(filters transactionFilters) error {
	if filters.Direction != "" && filters.Direction != "income" && filters.Direction != "expense" {
		return errors.New("direction filter is invalid")
	}
	switch filters.MatchStatus {
	case "", "matched", "candidate", "unmatched", "needs_review", "partial", "ignored":
	default:
		return errors.New("match status filter is invalid")
	}
	if filters.PeriodMonth != "" {
		if _, err := parsePeriodMonth(filters.PeriodMonth); err != nil {
			return errors.New("period filter is invalid")
		}
	}
	return nil
}

type transactionPageRow struct {
	ID                  string
	Direction           string
	DirectionLabel      string
	PayerName           string
	PayerNameKind       string
	PayerID             string
	AmountDisplay       string
	DateDisplay         string
	Description         string
	Reference           string
	AccountName         string
	AccountID           string
	TransactionID       string
	Source              string
	MatchStatus         string
	MatchStatusLabel    string
	CandidateTenantID   uint64
	CandidateTenantName string
	CanConfirm          bool
	TenantID            uint64
	NeedsMonthChoice    bool
	MonthOptions        []billingMonthOption
}

func paymentTransactionInputFromModel(row paymentTransaction) paymentTransactionInput {
	return paymentTransactionInput{
		Source:                row.Source,
		ProviderTransactionID: stringValue(row.ProviderTransactionID),
		StableTransactionKey:  row.StableTransactionKey,
		AccountID:             stringValue(row.AccountID),
		AccountName:           stringValue(row.AccountName),
		Direction:             row.Direction,
		AmountCents:           row.AmountCents,
		Currency:              row.Currency,
		TransactionTime:       row.TransactionTime,
		Description:           row.Description,
		Reference:             row.Reference,
		PayerID:               stringValue(row.PayerID),
		PayerName:             stringValue(row.PayerName),
		PayerNameKind:         row.PayerNameKind,
		ParsedPeriodMonth:     row.ParsedPeriodMonth,
		ParsedPeriodSource:    row.ParsedPeriodSource,
		ParsedPeriodNote:      row.ParsedPeriodNote,
		MatchReason:           row.MatchReason,
		MatchStatus:           row.MatchStatus,
		RawPayloadJSON:        row.RawPayloadJSON,
	}
}

func transactionPageRowFromModel(row paymentTransaction) transactionPageRow {
	dateDisplay := "Unknown"
	if row.TransactionTime != nil {
		dateDisplay = row.TransactionTime.UTC().Format("02 Jan 2006 15:04")
	}
	statusLabel := map[string]string{
		"matched":      "已关联",
		"candidate":    "待确认",
		"needs_review": "需处理",
		"partial":      "部分关联",
		"unmatched":    "未关联",
		"ignored":      "已忽略",
	}[row.MatchStatus]
	if statusLabel == "" {
		statusLabel = "未关联"
	}
	directionLabel := "支出"
	if row.Direction == "income" {
		directionLabel = "收入"
	}
	var tenantID uint64
	if row.MatchedTenantID != nil {
		tenantID = *row.MatchedTenantID
	}
	return transactionPageRow{
		ID:               strconv.FormatUint(row.ID, 10),
		Direction:        row.Direction,
		DirectionLabel:   directionLabel,
		PayerName:        firstNonEmpty(stringValue(row.PayerName), "未知付款人"),
		PayerNameKind:    firstNonEmpty(row.PayerNameKind, "unknown"),
		PayerID:          firstNonEmpty(stringValue(row.PayerID), "无付款人编号"),
		AmountDisplay:    formatMoney(centsToMoney(row.AmountCents), row.Currency, 2),
		DateDisplay:      dateDisplay,
		Description:      firstNonEmpty(row.Description, "无描述"),
		Reference:        firstNonEmpty(row.Reference, "无参考号"),
		AccountName:      firstNonEmpty(stringValue(row.AccountName), "未知账户"),
		AccountID:        stringValue(row.AccountID),
		TransactionID:    firstNonEmpty(stringValue(row.ProviderTransactionID), "#"+strconv.FormatUint(row.ID, 10)),
		Source:           row.Source,
		MatchStatus:      row.MatchStatus,
		MatchStatusLabel: statusLabel,
		TenantID:         tenantID,
	}
}

func (s *transactionService) listTransactions(ctx context.Context, userID uint64, filters transactionFilters) ([]paymentTransaction, error) {
	q := s.db.WithContext(ctx).Where("user_id = ?", userID)
	if filters.Direction != "" {
		q = q.Where("direction = ?", filters.Direction)
	}
	if filters.PendingOnly {
		q = q.Where("direction = ?", "income").Where("match_status IN ?", pendingMatchStatuses)
	}
	if filters.MatchStatus != "" {
		q = q.Where("match_status = ?", filters.MatchStatus)
	}
	if filters.PeriodMonth != "" {
		start, err := parseDate(filters.PeriodMonth + "-01")
		if err == nil {
			end := start.AddDate(0, 1, 0)
			q = q.Where("transaction_time >= ? AND transaction_time < ?", start, end)
		}
	}
	var rows []paymentTransaction
	if err := q.Order("transaction_time DESC, id DESC").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (s *transactionService) countPendingTransactions(ctx context.Context, userID uint64, periodMonth string) (int64, error) {
	q := s.db.WithContext(ctx).
		Model(&paymentTransaction{}).
		Where("user_id = ? AND direction = ? AND match_status IN ?", userID, "income", pendingMatchStatuses)
	if periodMonth != "" {
		start, err := parsePeriodMonth(periodMonth)
		if err != nil {
			return 0, err
		}
		q = q.Where("transaction_time >= ? AND transaction_time < ?", start, start.AddDate(0, 1, 0))
	}
	var count int64
	if err := q.Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
}

func fallbackTransactionPageRows(result demoResult, filters transactionFilters) []transactionPageRow {
	rows := normalizePaymentTransactions(result)
	pageRows := make([]transactionPageRow, 0, len(rows))
	for _, input := range rows {
		if filters.Direction != "" && input.Direction != filters.Direction {
			continue
		}
		if filters.PendingOnly && (input.Direction != "income" || !isPendingMatchStatus(input.MatchStatus)) {
			continue
		}
		if filters.MatchStatus != "" && input.MatchStatus != filters.MatchStatus {
			continue
		}
		if filters.PeriodMonth != "" {
			period, err := parsePeriodMonth(filters.PeriodMonth)
			if err != nil || input.TransactionTime == nil || monthStart(*input.TransactionTime) != period {
				continue
			}
		}
		pageRows = append(pageRows, transactionPageRowFromModel(paymentTransactionFromInput(0, input)))
	}
	return pageRows
}

func firstNonZeroTime(value *time.Time) time.Time {
	if value != nil {
		return *value
	}
	return time.Now().UTC()
}

func optionalTime(value time.Time, ok bool) *time.Time {
	if !ok {
		return nil
	}
	return &value
}

func parsedPeriodSource(ok bool) string {
	if ok {
		return "description_reference"
	}
	return ""
}

func parsedPeriodNote(description, reference string, ok bool) string {
	if !ok {
		return ""
	}
	note := strings.TrimSpace(strings.Join([]string{description, reference}, " "))
	runes := []rune(note)
	if len(runes) > 512 {
		return string(runes[:512])
	}
	return note
}
