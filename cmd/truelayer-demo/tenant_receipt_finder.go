package main

import (
	"context"
	"errors"
	"net/url"
	"strconv"
	"strings"
	"time"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

const tenantReceiptFinderPageSize = 12

var errInvalidTenantReceiptFinder = errors.New("invalid tenant receipt finder filter")

type tenantReceiptClue struct {
	Key      string
	Label    string
	URL      string
	Selected bool
}

type tenantReceiptFinderRow struct {
	ID          uint64
	Date        string
	Payer       string
	Description string
	Amount      string
	Remaining   string
	Status      string
	Hint        string
	SelectURL   string
}

type tenantReceiptFinderData struct {
	TenantID      uint64
	TenantName    string
	Period        string
	PeriodLabel   string
	Balance       string
	HasBalance    bool
	HasObligation bool
	Scope         string
	Mode          string
	Field         string
	Query         string
	Clues         []tenantReceiptClue
	Rows          []tenantReceiptFinderRow
	Total         int64
	Page          int
	Pages         int
	CloseURL      string
	SearchURL     string
	FormValues    url.Values
	AllURL        string
	NameURL       string
	TwoMonthURL   string
	SixMonthURL   string
	AllTimeURL    string
	AllFieldURL   string
	PayerFieldURL string
	DescFieldURL  string
	ClearURL      string
	PreviousURL   string
	NextURL       string
}

type tenantReceiptFinderFilters struct {
	Scope string
	Mode  string
	Clue  string
	Field string
	Query string
	Page  int
}

func tenantReceiptFinderFiltersFromQuery(values url.Values) (tenantReceiptFinderFilters, error) {
	filters := tenantReceiptFinderFilters{Scope: "two", Mode: "all", Field: "all", Page: 1}
	if value := values.Get("find_scope"); value != "" {
		filters.Scope = value
	}
	if value := values.Get("find_mode"); value != "" {
		filters.Mode = value
	}
	if value := values.Get("find_field"); value != "" {
		filters.Field = value
	}
	filters.Clue = values.Get("find_clue")
	filters.Query = strings.TrimSpace(values.Get("find_q"))
	if value := values.Get("find_page"); value != "" {
		page, err := strconv.Atoi(value)
		if err != nil || page < 1 || page > 100000 {
			return filters, errInvalidTenantReceiptFinder
		}
		filters.Page = page
	}
	if (filters.Scope != "two" && filters.Scope != "six" && filters.Scope != "all") ||
		(filters.Mode != "all" && filters.Mode != "name") ||
		(filters.Field != "all" && filters.Field != "payer" && filters.Field != "description") ||
		len([]rune(filters.Query)) > 191 || len(filters.Clue) > 32 {
		return filters, errInvalidTenantReceiptFinder
	}
	return filters, nil
}

func tenantReceiptNameClues(row tenant) []tenantReceiptClue {
	clues := make([]tenantReceiptClue, 0, 8)
	seen := make(map[string]bool)
	add := func(key, label string) {
		label = strings.TrimSpace(label)
		if label == "" || seen[strings.ToLower(label)] {
			return
		}
		seen[strings.ToLower(label)] = true
		clues = append(clues, tenantReceiptClue{Key: key, Label: label})
	}
	add("name", row.Name)
	add("alias", row.DisplayAlias)
	for _, token := range tenantNameTokens(row.Name + " " + row.DisplayAlias) {
		if len([]rune(token)) < 2 {
			continue
		}
		add("token-"+strconv.Itoa(len(clues)), token)
	}
	return clues
}

func tenantReceiptClueText(clues []tenantReceiptClue, key string) (string, bool) {
	for _, clue := range clues {
		if clue.Key == key {
			return clue.Label, true
		}
	}
	return "", false
}

func tenantReceiptDateBounds(scope string, now time.Time) (time.Time, time.Time) {
	local := now.In(bankLocalTime)
	start := time.Date(local.Year(), local.Month(), 1, 0, 0, 0, 0, bankLocalTime)
	end := start.AddDate(0, 1, 0)
	if scope == "six" {
		start = start.AddDate(0, -5, 0)
	} else {
		start = start.AddDate(0, -1, 0)
	}
	return start.UTC(), end.UTC()
}

func tenantReceiptLike(value string) string {
	value = strings.ToLower(value)
	value = strings.ReplaceAll(value, "!", "!!")
	value = strings.ReplaceAll(value, "%", "!%")
	value = strings.ReplaceAll(value, "_", "!_")
	return "%" + value + "%"
}

func tenantReceiptSearchQuery(q *gorm.DB, field, value string) *gorm.DB {
	like := tenantReceiptLike(value)
	switch field {
	case "payer":
		return q.Where("LOWER(COALESCE(payment_transactions.payer_name, '')) LIKE ? ESCAPE '!'", like)
	case "description":
		return q.Where("LOWER(payment_transactions.description) LIKE ? ESCAPE '!'", like)
	default:
		return q.Where("(LOWER(COALESCE(payment_transactions.payer_name, '')) LIKE ? ESCAPE '!' OR LOWER(payment_transactions.description) LIKE ? ESCAPE '!')", like, like)
	}
}

func tenantReceiptFinderURL(base string, tenantID uint64, filters tenantReceiptFinderFilters, sourceID uint64) string {
	parsed, _ := url.Parse(base)
	values := parsed.Query()
	values.Set("find_tenant", strconv.FormatUint(tenantID, 10))
	values.Set("find_scope", filters.Scope)
	values.Set("find_mode", filters.Mode)
	values.Set("find_field", filters.Field)
	values.Del("find_clue")
	values.Del("find_q")
	values.Del("find_page")
	values.Del("match")
	values.Del("match_tenant")
	values.Del("match_month")
	values.Del("match_origin")
	if filters.Clue != "" {
		values.Set("find_clue", filters.Clue)
	}
	if filters.Query != "" {
		values.Set("find_q", filters.Query)
	}
	if filters.Page > 1 {
		values.Set("find_page", strconv.Itoa(filters.Page))
	}
	if sourceID != 0 {
		values.Set("match", strconv.FormatUint(sourceID, 10))
		values.Set("match_tenant", strconv.FormatUint(tenantID, 10))
		values.Set("match_month", values.Get("period"))
		values.Set("match_origin", "lookup")
	}
	parsed.RawQuery = values.Encode()
	return parsed.String()
}

func loadTenantReceiptFinder(ctx context.Context, db *gorm.DB, userID, tenantID uint64, workspace rentWorkspaceFilters, values url.Values, now time.Time) (tenantReceiptFinderData, error) {
	var tenantRow tenant
	if err := db.WithContext(ctx).Where("id = ? AND user_id = ?", tenantID, userID).First(&tenantRow).Error; err != nil {
		return tenantReceiptFinderData{}, err
	}
	filters, err := tenantReceiptFinderFiltersFromQuery(values)
	if err != nil {
		return tenantReceiptFinderData{}, err
	}
	clues := tenantReceiptNameClues(tenantRow)
	if filters.Mode == "name" && filters.Clue == "" && len(clues) > 0 {
		filters.Clue = clues[0].Key
	}
	clueText, clueOK := tenantReceiptClueText(clues, filters.Clue)
	if filters.Mode == "name" && !clueOK {
		return tenantReceiptFinderData{}, errInvalidTenantReceiptFinder
	}
	base := rentWorkspaceURL(workspace, workspace.Page)
	data := tenantReceiptFinderData{
		TenantID: tenantID, TenantName: firstNonEmpty(tenantRow.DisplayAlias, tenantRow.Name),
		Period: workspace.PeriodMonth.Format("2006-01"), PeriodLabel: formatMonthLabel(workspace.PeriodMonth),
		Scope: filters.Scope, Mode: filters.Mode, Field: filters.Field, Query: filters.Query,
		Page: filters.Page, CloseURL: base, SearchURL: tenantReceiptFinderURL(base, tenantID, filters, 0),
	}
	formValues, _ := url.Parse(data.SearchURL)
	data.FormValues = formValues.Query()
	data.FormValues.Del("find_q")
	data.FormValues.Del("find_clue")
	data.FormValues.Set("find_mode", "all")
	data.FormValues.Del("find_page")
	var obligation rentObligation
	err = db.WithContext(ctx).Where("user_id = ? AND tenant_id = ? AND period_month = ? AND (record_status <> ? OR record_status IS NULL)", userID, tenantID, monthStart(workspace.PeriodMonth), obligationRecordVoided).First(&obligation).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return tenantReceiptFinderData{}, err
	}
	if err == nil {
		remaining := max(int64(0), obligation.ExpectedAmountCents-obligation.PaidAmountCents)
		data.HasObligation = true
		data.Balance = formatWorkspaceAmount(remaining)
		data.HasBalance = remaining > 0
	}
	link := func(change func(*tenantReceiptFinderFilters)) string {
		copy := filters
		change(&copy)
		copy.Page = 1
		return tenantReceiptFinderURL(base, tenantID, copy, 0)
	}
	data.AllURL = link(func(f *tenantReceiptFinderFilters) { f.Mode = "all"; f.Clue = "" })
	data.NameURL = link(func(f *tenantReceiptFinderFilters) { f.Mode = "name"; f.Query = "" })
	data.TwoMonthURL = link(func(f *tenantReceiptFinderFilters) { f.Scope = "two" })
	data.SixMonthURL = link(func(f *tenantReceiptFinderFilters) { f.Scope = "six" })
	data.AllTimeURL = link(func(f *tenantReceiptFinderFilters) { f.Scope = "all" })
	data.AllFieldURL = link(func(f *tenantReceiptFinderFilters) { f.Field = "all"; f.Mode = "all"; f.Clue = "" })
	data.PayerFieldURL = link(func(f *tenantReceiptFinderFilters) { f.Field = "payer"; f.Mode = "all"; f.Clue = "" })
	data.DescFieldURL = link(func(f *tenantReceiptFinderFilters) { f.Field = "description"; f.Mode = "all"; f.Clue = "" })
	data.ClearURL = link(func(f *tenantReceiptFinderFilters) { f.Query = ""; f.Mode = "all"; f.Clue = "" })
	for i := range clues {
		clue := &clues[i]
		key := clue.Key
		clue.Selected = filters.Mode == "name" && filters.Clue == key
		clue.URL = link(func(f *tenantReceiptFinderFilters) { f.Mode = "name"; f.Clue = key; f.Query = "" })
	}
	data.Clues = clues
	q := rentWorkspacePendingIncomeQuery(db, ctx, userID)
	if filters.Scope != "all" {
		start, end := tenantReceiptDateBounds(filters.Scope, now)
		q = q.Where("payment_transactions.transaction_time >= ? AND payment_transactions.transaction_time < ?", start, end)
	}
	if filters.Mode == "name" {
		q = tenantReceiptSearchQuery(q, "all", clueText)
	} else if filters.Query != "" {
		q = tenantReceiptSearchQuery(q, filters.Field, filters.Query)
	}
	if err := q.Count(&data.Total).Error; err != nil {
		return tenantReceiptFinderData{}, err
	}
	data.Pages = int((data.Total + tenantReceiptFinderPageSize - 1) / tenantReceiptFinderPageSize)
	if data.Pages > 0 && data.Page > data.Pages {
		data.Page = data.Pages
		filters.Page = data.Page
		data.SearchURL = tenantReceiptFinderURL(base, tenantID, filters, 0)
	}
	if data.Page > 1 {
		copy := filters
		copy.Page--
		data.PreviousURL = tenantReceiptFinderURL(base, tenantID, copy, 0)
	}
	if data.Page < data.Pages {
		copy := filters
		copy.Page++
		data.NextURL = tenantReceiptFinderURL(base, tenantID, copy, 0)
	}
	if filters.Mode == "name" {
		like := tenantReceiptLike(clueText)
		q = q.Order(clause.OrderBy{Expression: gorm.Expr("CASE WHEN LOWER(TRIM(payment_transactions.payer_name)) = ? THEN 0 WHEN LOWER(payment_transactions.payer_name) LIKE ? ESCAPE '!' THEN 1 ELSE 2 END ASC", strings.ToLower(clueText), like)})
	}
	var sources []paymentTransaction
	if err := q.Order("payment_transactions.transaction_time DESC, payment_transactions.id DESC").Limit(tenantReceiptFinderPageSize).Offset((data.Page - 1) * tenantReceiptFinderPageSize).Find(&sources).Error; err != nil {
		return tenantReceiptFinderData{}, err
	}
	ids := make([]uint64, 0, len(sources))
	for _, source := range sources {
		ids = append(ids, source.ID)
	}
	var allocations []paymentAllocation
	if len(ids) > 0 {
		if err := db.WithContext(ctx).Where("user_id = ? AND payment_transaction_id IN ?", userID, ids).Find(&allocations).Error; err != nil {
			return tenantReceiptFinderData{}, err
		}
	}
	bySource := make(map[uint64][]paymentAllocation)
	for _, allocation := range allocations {
		bySource[allocation.PaymentTransactionID] = append(bySource[allocation.PaymentTransactionID], allocation)
	}
	for _, source := range sources {
		row := enrichTransactionPageRow(transactionPageRowFromModel(source), source, bySource[source.ID])
		when := "到账时间待确认"
		if source.TransactionTime != nil {
			when = source.TransactionTime.In(bankLocalTime).Format("2006-01-02 15:04")
		}
		hint := ""
		if filters.Mode == "name" {
			if strings.Contains(strings.ToLower(stringValue(source.PayerName)), strings.ToLower(clueText)) {
				hint = "付款人包含姓名线索"
			} else {
				hint = "Description 包含姓名线索"
			}
		}
		data.Rows = append(data.Rows, tenantReceiptFinderRow{
			ID: source.ID, Date: when, Payer: row.PayerName, Description: row.Description,
			Amount: row.AmountDisplay, Remaining: row.RemainingAmountDisplay, Status: row.MatchStatusLabel,
			Hint: hint, SelectURL: tenantReceiptFinderURL(base, tenantID, filters, source.ID),
		})
	}
	return data, nil
}
