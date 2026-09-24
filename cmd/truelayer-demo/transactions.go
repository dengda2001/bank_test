package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type paymentTransaction struct {
	ID                     uint64 `gorm:"primaryKey"`
	UserID                 uint64
	Source                 string
	SourceBatchID          *string
	ProviderTransactionID  *string
	StableTransactionKey   string
	AccountID              *string
	AccountName            *string
	Direction              string
	AmountCents            int64
	Currency               string
	TransactionTime        *time.Time
	Description            string
	Reference              string
	PayerID                *string
	PayerName              *string
	PayerNameKind          string
	ParsedPeriodMonth      *time.Time
	ParsedPeriodSource     string
	ParsedPeriodNote       string
	MatchReason            string
	ManualAdjustmentReason string
	MatchedTenantID        *uint64
	MatchStatus            string
	RawPayloadJSON         []byte `gorm:"column:raw_payload_json"`
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

type paymentTransactionInput struct {
	Source                 string
	SourceBatchID          string
	ProviderTransactionID  string
	StableTransactionKey   string
	AccountID              string
	AccountName            string
	Direction              string
	AmountCents            int64
	Currency               string
	TransactionTime        *time.Time
	Description            string
	Reference              string
	PayerID                string
	PayerName              string
	PayerNameKind          string
	ParsedPeriodMonth      *time.Time
	ParsedPeriodSource     string
	ParsedPeriodNote       string
	MatchReason            string
	ManualAdjustmentReason string
	MatchStatus            string
	RawPayloadJSON         []byte
}

type transactionService struct {
	db *gorm.DB
}

var pendingMatchStatuses = []string{"candidate", "needs_review", "unmatched", "partial"}

// pendingMatchStatusFilter is the 匹配状态 option meaning "still needs
// attention". It is not a stored match status: it expands to the four unfinished
// ones and only ever applies to income. It used to be a separate 待处理 checkbox;
// folding it into the status dropdown keeps one control per question.
const pendingMatchStatusFilter = "pending"

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
			payerID := stablePayerID(firstNonEmpty(tx.PayerID, tx.RemitterID, tx.CounterpartyID, metaString(tx.Meta, "payer_id"), metaString(tx.Meta, "remitter_id"), metaString(tx.Meta, "counterparty_id")))
			providerID := firstNonEmpty(tx.NormalisedProviderTransactionID, tx.ProviderTransactionID, metaString(tx.Meta, "normalised_provider_transaction_id"), metaString(tx.Meta, "provider_transaction_id"), metaString(tx.Meta, "bank_transaction_id"))
			reference := firstNonEmpty(tx.Reference, metaString(tx.Meta, "payment_reference"), metaString(tx.Meta, "reference"), metaString(tx.Meta, "provider_reference"), metaString(tx.Meta, "remittance_information"), metaString(tx.Meta, "remittanceInformation"))
			period := bankTransactionPeriod(tx.Description, ts)
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
				ParsedPeriodMonth:     period.explicitMonth(),
				ParsedPeriodSource:    parsedPeriodSource(period.Explicit),
				ParsedPeriodNote:      parsedPeriodNote(tx.Description, period.Explicit),
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
	return s.reconcilePendingRentTransactions(ctx, userID)
}

func paymentTransactionFromInput(userID uint64, input paymentTransactionInput) paymentTransaction {
	return paymentTransaction{
		UserID:                 userID,
		Source:                 input.Source,
		SourceBatchID:          nullableString(input.SourceBatchID),
		ProviderTransactionID:  nullableString(input.ProviderTransactionID),
		StableTransactionKey:   input.StableTransactionKey,
		AccountID:              nullableString(input.AccountID),
		AccountName:            nullableString(input.AccountName),
		Direction:              input.Direction,
		AmountCents:            input.AmountCents,
		Currency:               input.Currency,
		TransactionTime:        input.TransactionTime,
		Description:            input.Description,
		Reference:              input.Reference,
		PayerID:                nullableString(stablePayerID(input.PayerID)),
		PayerName:              nullableString(input.PayerName),
		PayerNameKind:          input.PayerNameKind,
		ParsedPeriodMonth:      input.ParsedPeriodMonth,
		ParsedPeriodSource:     input.ParsedPeriodSource,
		ParsedPeriodNote:       input.ParsedPeriodNote,
		MatchReason:            input.MatchReason,
		ManualAdjustmentReason: input.ManualAdjustmentReason,
		MatchStatus:            input.MatchStatus,
		RawPayloadJSON:         input.RawPayloadJSON,
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
	page := 1
	if raw := strings.TrimSpace(q.Get("page")); raw != "" {
		page, _ = strconv.Atoi(raw)
	}
	pageSize := 10
	if raw := strings.TrimSpace(q.Get("page_size")); raw != "" {
		pageSize, _ = strconv.Atoi(raw)
	}
	tenantIDRaw := strings.TrimSpace(q.Get("tenant_id"))
	var tenantID uint64
	if tenantIDRaw != "" {
		tenantID, _ = strconv.ParseUint(tenantIDRaw, 10, 64)
	}
	matchStatus := strings.TrimSpace(q.Get("match_status"))
	pendingOnly := matchStatus == pendingMatchStatusFilter || strings.TrimSpace(q.Get("pending")) == "1"
	if matchStatus == pendingMatchStatusFilter {
		// Expanded into PendingOnly, so it must not also narrow match_status to a
		// status literally called "pending" — no such row could ever match.
		matchStatus = ""
	}
	return transactionFilters{
		ArrivalFrom:    strings.TrimSpace(q.Get("arrival_from")),
		ArrivalTo:      strings.TrimSpace(q.Get("arrival_to")),
		Payer:          strings.TrimSpace(q.Get("payer")),
		TenantID:       tenantID,
		TenantIDRaw:    tenantIDRaw,
		Direction:      strings.TrimSpace(q.Get("direction")),
		MatchStatus:    matchStatus,
		PeriodMonth:    strings.TrimSpace(q.Get("period")),
		RentPeriod:     strings.TrimSpace(q.Get("rent_period")),
		AllocationKind: strings.TrimSpace(q.Get("allocation")),
		Sort:           strings.TrimSpace(q.Get("sort")),
		Page:           page,
		PageSize:       pageSize,
		PendingOnly:    pendingOnly,
	}
}

// matchStatusSelection is the value the 匹配状态 dropdown should show. A pending
// filter arrives either as the dropdown option or as the legacy pending=1 link
// the dashboard cards still use; both have to light up the same option.
func matchStatusSelection(filters transactionFilters) string {
	if filters.PendingOnly && filters.MatchStatus == "" {
		return pendingMatchStatusFilter
	}
	return filters.MatchStatus
}

type transactionFilters struct {
	ArrivalFrom    string
	ArrivalTo      string
	Payer          string
	TenantID       uint64
	TenantIDRaw    string
	Direction      string
	MatchStatus    string
	PeriodMonth    string
	RentPeriod     string
	AllocationKind string
	Sort           string
	Page           int
	PageSize       int
	PendingOnly    bool
}

func validateTransactionFilters(filters transactionFilters) error {
	if filters.Direction != "" && filters.Direction != "income" && filters.Direction != "expense" {
		return errors.New("direction filter is invalid")
	}
	switch filters.MatchStatus {
	case "", "matched", "candidate", "unmatched", "needs_review", "partial", "ignored", pendingMatchStatusFilter:
	default:
		return errors.New("match status filter is invalid")
	}
	if filters.PeriodMonth != "" {
		if _, err := parsePeriodMonth(filters.PeriodMonth); err != nil {
			return errors.New("period filter is invalid")
		}
	}
	if filters.RentPeriod != "" {
		if _, err := parsePeriodMonth(filters.RentPeriod); err != nil {
			return errors.New("rent period filter is invalid")
		}
	}
	var arrivalFrom, arrivalTo time.Time
	var err error
	if filters.ArrivalFrom != "" {
		arrivalFrom, err = parseDate(filters.ArrivalFrom)
		if err != nil {
			return errors.New("arrival from filter is invalid")
		}
	}
	if filters.ArrivalTo != "" {
		arrivalTo, err = parseDate(filters.ArrivalTo)
		if err != nil {
			return errors.New("arrival to filter is invalid")
		}
	}
	if !arrivalFrom.IsZero() && !arrivalTo.IsZero() && arrivalTo.Before(arrivalFrom) {
		return errors.New("arrival date range is invalid")
	}
	if filters.TenantIDRaw != "" {
		parsed, err := strconv.ParseUint(filters.TenantIDRaw, 10, 64)
		if err != nil || parsed == 0 || parsed != filters.TenantID {
			return errors.New("tenant filter is invalid")
		}
	}
	if len([]rune(filters.Payer)) > 191 {
		return errors.New("payer filter is too long")
	}
	switch filters.AllocationKind {
	case "", allocationKindRent, allocationKindDeposit, allocationKindOther:
	default:
		return errors.New("allocation filter is invalid")
	}
	switch filters.Sort {
	case "", "arrival_desc", "arrival_asc", "amount_desc", "amount_asc", "payer_asc", "payer_desc", "object_asc", "object_desc", "rent_period_asc", "rent_period_desc", "reason_asc", "reason_desc", "status_asc", "status_desc":
	default:
		return errors.New("sort filter is invalid")
	}
	if filters.Page < 0 || filters.PageSize < 0 || filters.PageSize > 100 {
		return errors.New("pagination filter is invalid")
	}
	return nil
}

// transactionPageOrder maps the public sort token to a complete SQL fragment.
// Keeping the allowlist here means request text never reaches GORM's Order
// method, including when this service is called outside the HTTP handler.
func transactionPageOrder(sortValue string) string {
	switch sortValue {
	case "arrival_asc":
		return "payment_transactions.transaction_time ASC, payment_transactions.id ASC"
	case "amount_desc":
		return "payment_transactions.amount_cents DESC, payment_transactions.id DESC"
	case "amount_asc":
		return "payment_transactions.amount_cents ASC, payment_transactions.id ASC"
	case "payer_asc":
		return "payment_transactions.payer_name ASC, payment_transactions.id DESC"
	case "payer_desc":
		return "payment_transactions.payer_name DESC, payment_transactions.id DESC"
	case "object_asc":
		return "payment_transactions.account_name ASC, payment_transactions.id DESC"
	case "object_desc":
		return "payment_transactions.account_name DESC, payment_transactions.id DESC"
	case "rent_period_asc":
		return "payment_transactions.parsed_period_month ASC, payment_transactions.id DESC"
	case "rent_period_desc":
		return "payment_transactions.parsed_period_month DESC, payment_transactions.id DESC"
	case "reason_asc":
		return "payment_transactions.match_reason ASC, payment_transactions.id DESC"
	case "reason_desc":
		return "payment_transactions.match_reason DESC, payment_transactions.id DESC"
	case "status_asc":
		return "payment_transactions.match_status ASC, payment_transactions.id DESC"
	case "status_desc":
		return "payment_transactions.match_status DESC, payment_transactions.id DESC"
	default:
		return "payment_transactions.transaction_time DESC, payment_transactions.id DESC"
	}
}

type transactionPageRow struct {
	ID                        string
	InternalID                string
	DetailKey                 string
	DetailURL                 string
	ReturnURL                 string
	MatchURL                  string
	PayerSearchURL            string
	TenantSearchURL           string
	Direction                 string
	DirectionLabel            string
	PayerName                 string
	PayerNameKind             string
	PayerID                   string
	AmountDisplay             string
	AmountInput               string
	AllocatedAmountDisplay    string
	RemainingAmountDisplay    string
	RemainingAmountInput      string
	AllocationUseDisplay      string
	DateDisplay               string
	DateShort                 string
	ObjectLabel               string
	RoomOnlyLabel             string
	ParsedPeriodDisplay       string
	ParsedPeriodSourceLabel   string
	Description               string
	AccountName               string
	AccountID                 string
	TransactionID             string
	ProviderTransactionID     string
	Source                    string
	MatchStatus               string
	MatchStatusLabel          string
	MatchMethodLabel          string
	MatchReason               string
	ManualAdjustmentReason    string
	Deferred                  bool
	CandidateTenantID         uint64
	CandidateTenantName       string
	CandidateRentObligationID uint64
	CandidatePeriod           string
	MatchedTenantName         string
	CanConfirm                bool
	TenantID                  uint64
	NeedsMonthChoice          bool
	MonthOptions              []billingMonthOption
	ManualMatchOptions        []billingRentMatchOption
	ManualMatchTenantOptions  []billingTenantOption
	CanRematch                bool
	CanEditRentMatch          bool
	RematchOptions            []billingRentMatchOption
	RematchTenantOptions      []billingTenantOption
	RematchMonthOptions       []billingMonthOption
}

func paymentTransactionInputFromModel(row paymentTransaction) paymentTransactionInput {
	return paymentTransactionInput{
		Source:                 row.Source,
		ProviderTransactionID:  stringValue(row.ProviderTransactionID),
		StableTransactionKey:   row.StableTransactionKey,
		AccountID:              stringValue(row.AccountID),
		AccountName:            stringValue(row.AccountName),
		Direction:              row.Direction,
		AmountCents:            row.AmountCents,
		Currency:               row.Currency,
		TransactionTime:        row.TransactionTime,
		Description:            row.Description,
		Reference:              row.Reference,
		PayerID:                stablePayerID(stringValue(row.PayerID)),
		PayerName:              stringValue(row.PayerName),
		PayerNameKind:          row.PayerNameKind,
		ParsedPeriodMonth:      row.ParsedPeriodMonth,
		ParsedPeriodSource:     row.ParsedPeriodSource,
		ParsedPeriodNote:       row.ParsedPeriodNote,
		MatchReason:            row.MatchReason,
		ManualAdjustmentReason: row.ManualAdjustmentReason,
		MatchStatus:            row.MatchStatus,
		RawPayloadJSON:         row.RawPayloadJSON,
	}
}

func transactionPageRowFromModel(row paymentTransaction) transactionPageRow {
	dateDisplay := "Unknown"
	dateShort := ""
	if row.TransactionTime != nil {
		dateDisplay = row.TransactionTime.UTC().Format("02 Jan 2006 15:04")
		date := row.TransactionTime.UTC()
		dateShort = fmt.Sprintf("%d月%d日", int(date.Month()), date.Day())
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
	period := transactionPeriodForModel(row)
	return transactionPageRow{
		ID:                      strconv.FormatUint(row.ID, 10),
		InternalID:              strconv.FormatUint(row.ID, 10),
		Direction:               row.Direction,
		DirectionLabel:          directionLabel,
		PayerName:               firstNonEmpty(stringValue(row.PayerName), "未知付款人"),
		PayerNameKind:           firstNonEmpty(row.PayerNameKind, "unknown"),
		PayerID:                 firstNonEmpty(stringValue(row.PayerID), "无付款人编号"),
		AmountDisplay:           formatMoney(centsToMoney(row.AmountCents), row.Currency, 2),
		AmountInput:             strconv.FormatFloat(centsToMoney(row.AmountCents), 'f', 2, 64),
		AllocatedAmountDisplay:  formatMoney(0, row.Currency, 2),
		RemainingAmountDisplay:  formatMoney(centsToMoney(row.AmountCents), row.Currency, 2),
		RemainingAmountInput:    strconv.FormatFloat(centsToMoney(row.AmountCents), 'f', 2, 64),
		DateDisplay:             dateDisplay,
		DateShort:               dateShort,
		ParsedPeriodDisplay:     period.display(),
		ParsedPeriodSourceLabel: period.Label,
		Description:             firstNonEmpty(row.Description, "无描述"),
		AccountName:             firstNonEmpty(stringValue(row.AccountName), "未知账户"),
		AccountID:               stringValue(row.AccountID),
		TransactionID:           firstNonEmpty(stringValue(row.ProviderTransactionID), "#"+strconv.FormatUint(row.ID, 10)),
		ProviderTransactionID:   firstNonEmpty(stringValue(row.ProviderTransactionID), "无银行流水号"),
		Source:                  row.Source,
		MatchStatus:             row.MatchStatus,
		MatchStatusLabel:        statusLabel,
		MatchReason:             row.MatchReason,
		ManualAdjustmentReason:  row.ManualAdjustmentReason,
		TenantID:                tenantID,
	}
}

// enrichTransactionPageRow fills in the money side of a row from its effective
// allocations. The rent month those allocations point at used to be collected
// here too, for the legacy table's 租金月 column; that table is gone and the
// surviving list shows the month parsed out of the bank text instead, so the
// obligation walk that fed it went with it.
func enrichTransactionPageRow(row transactionPageRow, source paymentTransaction, allocations []paymentAllocation) transactionPageRow {
	summary := summarizeTransactionAllocations(source, allocations)
	var hasAutoRent, hasManualRent bool
	for _, allocation := range allocations {
		if !ledgerAllocationIsEffective(allocation) || ledgerAllocationKind(allocation) != allocationKindRent {
			continue
		}
		if strings.HasPrefix(allocation.ConfirmationSource, "auto_") {
			hasAutoRent = true
		} else {
			hasManualRent = true
		}
	}
	switch {
	case hasAutoRent && hasManualRent:
		row.MatchMethodLabel = "自动＋手动"
	case hasAutoRent:
		row.MatchMethodLabel = "自动匹配"
	case hasManualRent:
		row.MatchMethodLabel = "手动匹配"
	}
	row.AllocatedAmountDisplay = formatMoney(centsToMoney(summary.AllocatedCents), source.Currency, 2)
	row.RemainingAmountDisplay = formatMoney(centsToMoney(summary.RemainingCents), source.Currency, 2)
	row.RemainingAmountInput = strconv.FormatFloat(centsToMoney(summary.RemainingCents), 'f', 2, 64)
	if summary.AllocatedCents == 0 {
		row.AllocationUseDisplay = "未归类"
	} else {
		labels := make([]string, 0, 3)
		for _, kind := range []string{allocationKindRent, allocationKindDeposit, allocationKindOther} {
			if cents := summary.KindCents[kind]; cents > 0 {
				label := map[string]string{allocationKindRent: "房租", allocationKindDeposit: "押金", allocationKindOther: "其他收入"}[kind]
				labels = append(labels, fmt.Sprintf("%s %s", label, formatMoney(centsToMoney(cents), source.Currency, 2)))
			}
		}
		row.AllocationUseDisplay = strings.Join(labels, " · ")
	}
	for _, allocation := range allocations {
		if !ledgerAllocationIsEffective(allocation) {
			continue
		}
		if row.TenantID == 0 && allocation.TenantID != nil && *allocation.TenantID != 0 {
			row.TenantID = *allocation.TenantID
		}
	}
	return row
}

func (s *transactionService) listTransactions(ctx context.Context, userID uint64, filters transactionFilters) ([]paymentTransaction, error) {
	rows, _, err := s.listTransactionsPage(ctx, userID, filters)
	return rows, err
}

func (s *transactionService) listTransactionsPage(ctx context.Context, userID uint64, filters transactionFilters) ([]paymentTransaction, int64, error) {
	if userID == 0 {
		return nil, 0, errors.New("userID is required")
	}
	q := s.db.WithContext(ctx).Model(&paymentTransaction{}).Where("payment_transactions.user_id = ?", userID)
	var err error
	q, err = applyTransactionFilters(q, userID, filters)
	if err != nil {
		return nil, 0, err
	}
	var total int64
	if err := q.Count(&total).Error; err != nil {
		return nil, 0, err
	}
	order := transactionPageOrder(filters.Sort)
	page := filters.Page
	if page <= 0 {
		page = 1
	}
	pageSize := filters.PageSize
	if pageSize <= 0 {
		pageSize = 10
	}
	var rows []paymentTransaction
	if err := q.Order(order).Limit(pageSize).Offset((page - 1) * pageSize).Find(&rows).Error; err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

func applyTransactionFilters(q *gorm.DB, userID uint64, filters transactionFilters) (*gorm.DB, error) {
	if filters.Direction != "" {
		q = q.Where("payment_transactions.direction = ?", filters.Direction)
	}
	if filters.PendingOnly {
		q = q.Where("payment_transactions.direction = ?", "income").Where("payment_transactions.match_status IN ?", pendingMatchStatuses)
	}
	if filters.MatchStatus != "" {
		q = q.Where("payment_transactions.match_status = ?", filters.MatchStatus)
	}
	if filters.PeriodMonth != "" {
		start, err := parsePeriodMonth(filters.PeriodMonth)
		if err != nil {
			return q, err
		}
		q = q.Where("payment_transactions.transaction_time >= ? AND payment_transactions.transaction_time < ?", start, start.AddDate(0, 1, 0))
	}
	if filters.ArrivalFrom != "" {
		start, err := parseDate(filters.ArrivalFrom)
		if err != nil {
			return q, err
		}
		q = q.Where("payment_transactions.transaction_time >= ?", start)
	}
	if filters.ArrivalTo != "" {
		end, err := parseDate(filters.ArrivalTo)
		if err != nil {
			return q, err
		}
		q = q.Where("payment_transactions.transaction_time < ?", end)
	}
	if filters.Payer != "" {
		like := "%" + filters.Payer + "%"
		q = q.Where(`(payment_transactions.payer_name LIKE ?
            OR payment_transactions.payer_id LIKE ?
            OR payment_transactions.description LIKE ?
            OR EXISTS (
                SELECT 1 FROM tenants AS search_tenant
                WHERE search_tenant.user_id = payment_transactions.user_id
                  AND (search_tenant.name LIKE ? OR search_tenant.display_alias LIKE ?)
                  AND (search_tenant.id = payment_transactions.matched_tenant_id
                       OR EXISTS (
                           SELECT 1 FROM payment_allocations AS search_allocation
                           WHERE search_allocation.user_id = payment_transactions.user_id
                             AND search_allocation.payment_transaction_id = payment_transactions.id
                             AND search_allocation.tenant_id = search_tenant.id
                             AND search_allocation.status = ?
                             AND search_allocation.allocation_kind IN (?, ?, ?, '')
                       ))
            ))`, like, like, like, like, like, allocationStatusConfirmed, allocationKindRent, allocationKindDeposit, allocationKindOther)
	}
	if filters.TenantID != 0 {
		q = q.Where(`EXISTS (
            SELECT 1 FROM payment_allocations AS pa
            WHERE pa.user_id = payment_transactions.user_id
              AND pa.payment_transaction_id = payment_transactions.id
              AND pa.tenant_id = ?
              AND pa.status = ?
        )`, filters.TenantID, allocationStatusConfirmed)
	}
	if filters.RentPeriod != "" {
		period, err := parsePeriodMonth(filters.RentPeriod)
		if err != nil {
			return q, err
		}
		q = q.Where(`EXISTS (
            SELECT 1
            FROM payment_allocations AS pa
            INNER JOIN rent_obligations AS ro ON ro.id = pa.rent_obligation_id AND ro.user_id = pa.user_id
            WHERE pa.user_id = payment_transactions.user_id
              AND pa.payment_transaction_id = payment_transactions.id
              AND pa.status = ?
              AND pa.allocation_kind IN (?, '')
              AND ro.period_month = ?
        )`, allocationStatusConfirmed, allocationKindRent, period)
	}
	if filters.AllocationKind != "" {
		kindCondition := "pa.allocation_kind = ?"
		args := []any{allocationStatusConfirmed, filters.AllocationKind}
		if filters.AllocationKind == allocationKindRent {
			kindCondition = "pa.allocation_kind IN (?, '')"
			args = []any{allocationStatusConfirmed, allocationKindRent}
		}
		q = q.Where(`EXISTS (
            SELECT 1 FROM payment_allocations AS pa
            WHERE pa.user_id = payment_transactions.user_id
              AND pa.payment_transaction_id = payment_transactions.id
              AND pa.status = ?
              AND `+kindCondition+`
        )`, args...)
	}
	return q, nil
}

func (s *transactionService) countPendingTransactions(ctx context.Context, userID uint64, periodMonth string) (int64, error) {
	return s.countPendingTransactionsWithFilters(ctx, userID, transactionFilters{PeriodMonth: periodMonth, PendingOnly: true})
}

func (s *transactionService) countPendingTransactionsWithFilters(ctx context.Context, userID uint64, filters transactionFilters) (int64, error) {
	filters.PendingOnly = true
	filters.MatchStatus = ""
	q := s.db.WithContext(ctx).Model(&paymentTransaction{}).Where("payment_transactions.user_id = ?", userID)
	var err error
	q, err = applyTransactionFilters(q, userID, filters)
	if err != nil {
		return 0, err
	}
	var count int64
	if err := q.Count(&count).Error; err != nil {
		return 0, err
	}
	return count, nil
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
		return "description"
	}
	return ""
}

func parsedPeriodNote(description string, ok bool) string {
	if !ok {
		return ""
	}
	note := strings.TrimSpace(description)
	runes := []rune(note)
	if len(runes) > 512 {
		return string(runes[:512])
	}
	return note
}
