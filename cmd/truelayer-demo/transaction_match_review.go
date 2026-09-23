package main

import (
	"context"
	"errors"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
)

const transactionReviewHistoryPageSize = 10

type transactionReviewTenant struct {
	ID       uint64
	Name     string
	Selected bool
}

type transactionReviewEvidence struct {
	TransactionID uint64
	Date          string
	PayerName     string
	Description   string
	Amount        string
	DetailURL     string
}

type transactionReviewMonth struct {
	Period          string
	Label           string
	Expected        string
	Paid            string
	Remaining       string
	Coverage        string
	SourceRemainder string
	Note            string
	Highlighted     bool
	Selectable      bool
	Evidence        []transactionReviewEvidence
}

type transactionReviewHistory struct {
	Date          string
	PayerName     string
	Amount        string
	Description   string
	ParsedPeriod  string
	MatchedPeriod string
	Status        string
	DetailURL     string
}

type transactionMatchReviewData struct {
	Source             transactionPageRow
	Reference          string
	CloseURL           string
	ReturnURL          string
	FormAction         string
	AllowDefer         bool
	TenantOptions      []transactionReviewTenant
	SelectedTenantID   uint64
	IdentifiedTenant   bool
	IdentityNote       string
	Months             []transactionReviewMonth
	History            []transactionReviewHistory
	AllHistoryShown    bool
	HistoryPage        int
	HistoryPages       int
	CanMatch           bool
	PreviousHistoryURL string
	NextHistoryURL     string
	AllHistoryURL      string
	ListValues         url.Values
	Error              string
}

func (s *transactionService) transactionMatchReview(ctx context.Context, userID, transactionID, requestedTenantID uint64, historyPage int) (transactionMatchReviewData, error) {
	if userID == 0 || transactionID == 0 {
		return transactionMatchReviewData{}, gorm.ErrRecordNotFound
	}
	var source paymentTransaction
	if err := s.db.WithContext(ctx).Where("id = ? AND user_id = ?", transactionID, userID).First(&source).Error; err != nil {
		return transactionMatchReviewData{}, err
	}
	if source.Direction != "income" {
		return transactionMatchReviewData{}, gorm.ErrRecordNotFound
	}
	if source.ParsedPeriodMonth != nil {
		if err := newMonthlyRentFactsService(s.db).ensureMonthlyRentFacts(ctx, userID, monthStart(*source.ParsedPeriodMonth), rentFactsIntentRead); err != nil {
			return transactionMatchReviewData{}, err
		}
	}
	var tenants []tenant
	if err := s.db.WithContext(ctx).Where("user_id = ?", userID).Order("name ASC, id ASC").Find(&tenants).Error; err != nil {
		return transactionMatchReviewData{}, err
	}
	var payers []tenantPayer
	if err := s.db.WithContext(ctx).Where("user_id = ? AND removed_at IS NULL", userID).Find(&payers).Error; err != nil {
		return transactionMatchReviewData{}, err
	}
	var obligations []rentObligation
	if err := s.db.WithContext(ctx).Where("user_id = ?", userID).Find(&obligations).Error; err != nil {
		return transactionMatchReviewData{}, err
	}
	var sourceAllocations []paymentAllocation
	if err := s.db.WithContext(ctx).Where("user_id = ? AND payment_transaction_id = ?", userID, transactionID).Find(&sourceAllocations).Error; err != nil {
		return transactionMatchReviewData{}, err
	}
	summary := summarizeTransactionAllocations(source, sourceAllocations)
	if summary.RemainingCents <= 0 || source.MatchStatus == "ignored" {
		return transactionMatchReviewData{}, gorm.ErrRecordNotFound
	}
	decision := decideStrictRentMatch(paymentTransactionInputFromModel(source), payers, tenants, obligations)
	identifiedID := decision.TenantID
	identifiedFromAllocation := false
	if identifiedID == 0 {
		for _, allocation := range sourceAllocations {
			if !ledgerAllocationIsEffective(allocation) || ledgerAllocationKind(allocation) != allocationKindRent || allocation.TenantID == nil {
				continue
			}
			if identifiedID != 0 && identifiedID != *allocation.TenantID {
				identifiedID = 0
				break
			}
			identifiedID = *allocation.TenantID
			identifiedFromAllocation = true
		}
	}
	identityNote := "系统尚未确认付款人对应的租客，请核对后选择。"
	if identifiedID != 0 {
		identityNote = "根据已保存的付款人关系识别；请与银行原文核对。"
		if identifiedFromAllocation {
			identityNote = "根据这笔流水已有的租金分配预选；请与银行原文核对。"
		}
	}
	selectedID := identifiedID
	if requestedTenantID != 0 {
		selectedID = requestedTenantID
	}
	data := transactionMatchReviewData{
		Source:           enrichTransactionPageRow(transactionPageRowFromModel(source), source, sourceAllocations),
		Reference:        firstNonEmpty(source.Reference, "—"),
		SelectedTenantID: selectedID,
		IdentifiedTenant: identifiedID != 0 && selectedID == identifiedID,
		IdentityNote:     identityNote,
	}
	data.Source.DetailURL = "/transactions?detail=" + strconv.FormatUint(source.ID, 10)
	selectedExists := selectedID == 0
	for _, tenantRow := range tenants {
		name := firstNonEmpty(tenantRow.DisplayAlias, tenantRow.Name)
		data.TenantOptions = append(data.TenantOptions, transactionReviewTenant{ID: tenantRow.ID, Name: name, Selected: tenantRow.ID == selectedID})
		if tenantRow.ID == selectedID {
			selectedExists = true
		}
	}
	if !selectedExists {
		return transactionMatchReviewData{}, gorm.ErrRecordNotFound
	}
	if selectedID != 0 {
		selectedObligations := make([]rentObligation, 0)
		ids := make([]uint64, 0)
		for _, obligation := range obligations {
			if obligation.TenantID == selectedID && obligation.RecordStatus != obligationRecordVoided {
				selectedObligations = append(selectedObligations, obligation)
				ids = append(ids, obligation.ID)
			}
		}
		var allocations []paymentAllocation
		if len(ids) > 0 {
			if err := s.db.WithContext(ctx).Where("user_id = ? AND rent_obligation_id IN ? AND status = ?", userID, ids, allocationStatusConfirmed).Find(&allocations).Error; err != nil {
				return transactionMatchReviewData{}, err
			}
		}
		transactionIDs := make([]uint64, 0, len(allocations))
		for _, allocation := range allocations {
			if ledgerAllocationIsEffective(allocation) && ledgerAllocationKind(allocation) == allocationKindRent {
				transactionIDs = append(transactionIDs, allocation.PaymentTransactionID)
			}
		}
		transactions := make(map[uint64]paymentTransaction)
		if len(transactionIDs) > 0 {
			var rows []paymentTransaction
			if err := s.db.WithContext(ctx).Where("user_id = ? AND id IN ?", userID, transactionIDs).Find(&rows).Error; err != nil {
				return transactionMatchReviewData{}, err
			}
			for _, row := range rows {
				transactions[row.ID] = row
			}
		}
		data.Months = transactionReviewMonths(source, summary.RemainingCents, selectedObligations, allocations, transactions)
		for _, month := range data.Months {
			data.CanMatch = data.CanMatch || month.Selectable
		}
	}
	if historyPage < 1 {
		historyPage = 1
	}
	data.HistoryPage = historyPage
	stableID := stablePayerID(stringValue(source.PayerID))
	payerName := strings.TrimSpace(stringValue(source.PayerName))
	if stableID != "" || payerName != "" {
		q := s.db.WithContext(ctx).Model(&paymentTransaction{}).Where("user_id = ? AND id <> ?", userID, source.ID)
		if stableID != "" && payerName != "" {
			q = q.Where("(payer_id = ? OR LOWER(TRIM(payer_name)) = ?)", stableID, strings.ToLower(payerName))
		} else if stableID != "" {
			q = q.Where("payer_id = ?", stableID)
		} else {
			q = q.Where("LOWER(TRIM(payer_name)) = ?", strings.ToLower(payerName))
		}
		var count int64
		if err := q.Count(&count).Error; err != nil {
			return transactionMatchReviewData{}, err
		}
		data.HistoryPages = int((count + transactionReviewHistoryPageSize - 1) / transactionReviewHistoryPageSize)
		if data.HistoryPages > 0 && historyPage > data.HistoryPages {
			historyPage = data.HistoryPages
			data.HistoryPage = historyPage
		}
		var history []paymentTransaction
		q = q.Order("transaction_time DESC, id DESC").Limit(transactionReviewHistoryPageSize).Offset((historyPage - 1) * transactionReviewHistoryPageSize)
		if err := q.Find(&history).Error; err != nil {
			return transactionMatchReviewData{}, err
		}
		var err error
		data.History, err = s.transactionReviewHistoryRows(ctx, userID, history)
		if err != nil {
			return transactionMatchReviewData{}, err
		}
	}
	return data, nil
}

func transactionReviewMonths(source paymentTransaction, sourceRemaining int64, obligations []rentObligation, allocations []paymentAllocation, transactions map[uint64]paymentTransaction) []transactionReviewMonth {
	byObligation := make(map[uint64][]paymentAllocation)
	for _, allocation := range allocations {
		if ledgerAllocationIsEffective(allocation) && ledgerAllocationKind(allocation) == allocationKindRent && allocation.RentObligationID != nil {
			byObligation[*allocation.RentObligationID] = append(byObligation[*allocation.RentObligationID], allocation)
		}
	}
	parsedPeriod := ""
	if source.ParsedPeriodMonth != nil {
		parsedPeriod = monthStart(*source.ParsedPeriodMonth).Format("2006-01")
	}
	rows := make([]transactionReviewMonth, 0, len(obligations)+1)
	seenPeriod := make(map[string]bool)
	for _, obligation := range obligations {
		period := monthStart(obligation.PeriodMonth).Format("2006-01")
		seenPeriod[period] = true
		remaining := max(int64(0), obligation.ExpectedAmountCents-obligation.PaidAmountCents)
		currency := firstNonEmpty(obligation.Currency, source.Currency, "EUR")
		row := transactionReviewMonth{
			Period:      period,
			Label:       formatMonthLabel(obligation.PeriodMonth),
			Expected:    formatMoney(centsToMoney(obligation.ExpectedAmountCents), currency, 2),
			Paid:        formatMoney(centsToMoney(obligation.PaidAmountCents), currency, 2),
			Remaining:   formatMoney(centsToMoney(remaining), currency, 2),
			Highlighted: period == parsedPeriod,
		}
		switch {
		case !strings.EqualFold(currency, source.Currency):
			row.Note = "币种不同，不能匹配这笔流水"
		case remaining == 0:
			row.Note = "本月已交清，请先核对下方原匹配流水"
		default:
			row.Selectable = true
			coverage := min(sourceRemaining, remaining)
			row.Coverage = formatMoney(centsToMoney(coverage), source.Currency, 2)
			row.SourceRemainder = formatMoney(centsToMoney(sourceRemaining-coverage), source.Currency, 2)
		}
		for _, allocation := range byObligation[obligation.ID] {
			transaction, ok := transactions[allocation.PaymentTransactionID]
			if !ok || transaction.Direction != "income" {
				continue
			}
			row.Evidence = append(row.Evidence, transactionReviewEvidence{
				TransactionID: transaction.ID,
				Date:          transactionReviewDate(transaction.TransactionTime),
				PayerName:     firstNonEmpty(stringValue(transaction.PayerName), "未知付款人"),
				Description:   firstNonEmpty(transaction.Description, "无描述"),
				Amount:        formatMoney(centsToMoney(allocation.AmountCents), transaction.Currency, 2),
				DetailURL:     "/transactions?detail=" + strconv.FormatUint(transaction.ID, 10),
			})
		}
		rows = append(rows, row)
	}
	if parsedPeriod != "" && !seenPeriod[parsedPeriod] {
		rows = append(rows, transactionReviewMonth{Period: parsedPeriod, Label: parsedPeriod, Highlighted: true, Note: "系统未找到该月租金责任，请核对入住与租金计划"})
	}
	sort.Slice(rows, func(i, j int) bool {
		if rows[i].Highlighted != rows[j].Highlighted {
			return rows[i].Highlighted
		}
		return rows[i].Period > rows[j].Period
	})
	return rows
}

func transactionReviewDate(value *time.Time) string {
	if value == nil {
		return "日期未知"
	}
	return value.UTC().Format("2006-01-02")
}

func (s *transactionService) transactionReviewHistoryRows(ctx context.Context, userID uint64, history []paymentTransaction) ([]transactionReviewHistory, error) {
	rows := make([]transactionReviewHistory, 0, len(history))
	if len(history) == 0 {
		return rows, nil
	}
	ids := make([]uint64, 0, len(history))
	for _, source := range history {
		ids = append(ids, source.ID)
	}
	var allocations []paymentAllocation
	if err := s.db.WithContext(ctx).Where("user_id = ? AND payment_transaction_id IN ? AND status = ?", userID, ids, allocationStatusConfirmed).Find(&allocations).Error; err != nil {
		return nil, err
	}
	obligationIDs := make([]uint64, 0, len(allocations))
	for _, allocation := range allocations {
		if ledgerAllocationIsEffective(allocation) && ledgerAllocationKind(allocation) == allocationKindRent && allocation.RentObligationID != nil {
			obligationIDs = append(obligationIDs, *allocation.RentObligationID)
		}
	}
	obligations := make(map[uint64]rentObligation)
	if len(obligationIDs) > 0 {
		var records []rentObligation
		if err := s.db.WithContext(ctx).Where("user_id = ? AND id IN ?", userID, obligationIDs).Find(&records).Error; err != nil {
			return nil, err
		}
		for _, record := range records {
			obligations[record.ID] = record
		}
	}
	periodsByTransaction := transactionReviewMatchedPeriods(allocations, obligations)
	for _, source := range history {
		periods := periodsByTransaction[source.ID]
		parsed := "—"
		if source.ParsedPeriodMonth != nil {
			parsed = monthStart(*source.ParsedPeriodMonth).Format("2006-01")
		}
		rows = append(rows, transactionReviewHistory{
			Date:          transactionReviewDate(source.TransactionTime),
			PayerName:     firstNonEmpty(stringValue(source.PayerName), "未知付款人"),
			Amount:        formatMoney(centsToMoney(source.AmountCents), source.Currency, 2),
			Description:   firstNonEmpty(source.Description, "无描述"),
			ParsedPeriod:  parsed,
			MatchedPeriod: firstNonEmpty(strings.Join(periods, "、"), "—"),
			Status:        transactionPageRowFromModel(source).MatchStatusLabel,
			DetailURL:     "/transactions?detail=" + strconv.FormatUint(source.ID, 10),
		})
	}
	return rows, nil
}

func transactionReviewMatchedPeriods(allocations []paymentAllocation, obligations map[uint64]rentObligation) map[uint64][]string {
	periodsByTransaction := make(map[uint64]map[string]bool)
	for _, allocation := range allocations {
		if !ledgerAllocationIsEffective(allocation) || ledgerAllocationKind(allocation) != allocationKindRent || allocation.RentObligationID == nil {
			continue
		}
		obligation, ok := obligations[*allocation.RentObligationID]
		if !ok {
			continue
		}
		if periodsByTransaction[allocation.PaymentTransactionID] == nil {
			periodsByTransaction[allocation.PaymentTransactionID] = make(map[string]bool)
		}
		periodsByTransaction[allocation.PaymentTransactionID][monthStart(obligation.PeriodMonth).Format("2006-01")] = true
	}
	result := make(map[uint64][]string, len(periodsByTransaction))
	for transactionID, periods := range periodsByTransaction {
		for period := range periods {
			result[transactionID] = append(result[transactionID], period)
		}
		sort.Strings(result[transactionID])
	}
	return result
}

func transactionReviewURL(query url.Values, transactionID uint64) string {
	values := cloneQueryValues(query)
	values.Del("detail")
	values.Del("match")
	values.Del("match_tenant")
	values.Del("match_history_page")
	values.Set("match", strconv.FormatUint(transactionID, 10))
	return "/transactions?" + values.Encode()
}

func transactionReviewHistoryURL(query url.Values, page int) string {
	values := cloneQueryValues(query)
	values.Set("match_history_page", strconv.Itoa(page))
	return "/transactions?" + values.Encode()
}

func transactionReviewErrorText(code string) string {
	switch code {
	case "confirmation_failed":
		return "匹配未完成。请刷新后核对租金余额和月份，再试一次。"
	case "rent_facts_conflict":
		return "该月份的租金计划刚刚变化，请重新核对。"
	case "invalid_confirmation":
		return "匹配请求无效，请重新选择租客和月份。"
	default:
		return ""
	}
}

func transactionReviewFailureURL(returnTo string, transactionID, tenantID uint64, code string) (string, error) {
	parsed, err := url.ParseRequestURI(returnTo)
	if err != nil || (parsed.Path != "/transactions" && parsed.Path != "/rent-dashboard") || parsed.IsAbs() || parsed.Host != "" || transactionID == 0 {
		return "", errors.New("invalid transaction review return path")
	}
	query := parsed.Query()
	query.Set("match", strconv.FormatUint(transactionID, 10))
	if tenantID != 0 {
		query.Set("match_tenant", strconv.FormatUint(tenantID, 10))
	}
	query.Set("error", code)
	return parsed.Path + "?" + query.Encode(), nil
}

func (d *transactionMatchReviewData) setURLs(query url.Values) {
	d.CloseURL = transactionListURL(query)
	d.ReturnURL = d.CloseURL
	d.FormAction = "/transactions"
	d.ListValues = cloneQueryValues(query)
	for _, key := range []string{"match", "match_tenant", "match_history_page", "detail", "error", "message"} {
		d.ListValues.Del(key)
	}
	if d.HistoryPage > 1 {
		d.PreviousHistoryURL = transactionReviewHistoryURL(query, d.HistoryPage-1)
	}
	if d.HistoryPage < d.HistoryPages {
		d.NextHistoryURL = transactionReviewHistoryURL(query, d.HistoryPage+1)
	}
	if d.Source.PayerName != "" && d.Source.PayerName != "未知付款人" {
		d.AllHistoryURL = "/transactions?scope=all&payer=" + url.QueryEscape(d.Source.PayerName)
	}
	d.Error = transactionReviewErrorText(query.Get("error"))
}

func (d *transactionMatchReviewData) setWorkspaceURLs(filters rentWorkspaceFilters, query url.Values) {
	d.CloseURL = rentWorkspaceURL(filters, filters.Page)
	d.ReturnURL = d.CloseURL
	d.FormAction = "/rent-dashboard"
	d.AllowDefer = isPendingMatchStatus(d.Source.MatchStatus)
	d.AllHistoryShown = true
	d.ListValues = cloneQueryValues(query)
	for _, key := range []string{"match", "match_tenant", "match_history_page", "detail", "error", "message"} {
		d.ListValues.Del(key)
	}
	d.AllHistoryURL = "/transactions?scope=all&payer=" + url.QueryEscape(d.Source.PayerName)
	if d.HistoryPage > 1 {
		values := cloneQueryValues(query)
		values.Set("match_history_page", strconv.Itoa(d.HistoryPage-1))
		d.PreviousHistoryURL = "/rent-dashboard?" + values.Encode()
	}
	if d.HistoryPage < d.HistoryPages {
		values := cloneQueryValues(query)
		values.Set("match_history_page", strconv.Itoa(d.HistoryPage+1))
		d.NextHistoryURL = "/rent-dashboard?" + values.Encode()
	}
	d.Error = transactionReviewErrorText(query.Get("error"))
}
